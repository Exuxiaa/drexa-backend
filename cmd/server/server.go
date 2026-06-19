package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"drexa/internal/auth"
	authRepo "drexa/internal/auth/repository"
	authSvc "drexa/internal/auth/service"
	authUc "drexa/internal/auth/usecase"
	"drexa/internal/checkout"
	"drexa/internal/kyc"
	kycRepo "drexa/internal/kyc/repository"
	kycSvc "drexa/internal/kyc/service"
	kycUc "drexa/internal/kyc/usecase"
	"drexa/internal/market"
	"drexa/internal/matching"
	"drexa/internal/order"
	orderRepo "drexa/internal/order/repository"
	"drexa/internal/p2p"
	p2pChain "drexa/internal/p2p/chain"
	p2pRepo "drexa/internal/p2p/repository"
	p2pUc "drexa/internal/p2p/usecase"
	"drexa/internal/platform/middleware"
	"drexa/internal/wallet"
	walletRepo "drexa/internal/wallet/repository"
	walletSvc "drexa/internal/wallet/service"
	walletUc "drexa/internal/wallet/usecase"
	"drexa/pkg/config"
)

// kycUserServiceAdapter bridges auth.UserRepository to kyc.UserService
// so the kyc domain never imports internal/auth.
type kycUserServiceAdapter struct {
	repo auth.UserRepository
}

func (a *kycUserServiceAdapter) FindByID(ctx context.Context, userID string) (*kyc.UserSnapshot, error) {
	u, err := a.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, kyc.ErrUserNotFound
	}
	return &kyc.UserSnapshot{UserID: u.UserID, Email: u.Email}, nil
}

func (a *kycUserServiceAdapter) UpdateKycLevel(ctx context.Context, userID, reviewedBy string, level int) error {
	return a.repo.UpdateKycLevel(ctx, userID, reviewedBy, level)
}

// depthSourceAdapter bridges the order service to market.DepthSource so the
// market feed reads order-book depth without importing internal/order.
type depthSourceAdapter struct {
	orders order.Service
}

func (a *depthSourceAdapter) OrderBookDepth(ctx context.Context, pairID string, maxLevels int) (*market.OrderBookSnapshot, error) {
	snap, err := a.orders.OrderBookDepth(ctx, pairID, maxLevels)
	if err != nil {
		return nil, err
	}
	return &market.OrderBookSnapshot{
		PairID:  snap.PairID,
		Version: snap.Version,
		Bids:    toMarketLevels(snap.Bids),
		Asks:    toMarketLevels(snap.Asks),
	}, nil
}

func toMarketLevels(levels []order.OrderBookLevel) []market.BookLevel {
	out := make([]market.BookLevel, len(levels))
	for i, l := range levels {
		out[i] = market.BookLevel{Price: l.Price, Quantity: l.Quantity}
	}
	return out
}

// orderWalletAdapter bridges wallet.WalletRepository to order.WalletService
// so the order domain can check available balance without importing internal/wallet.
type orderWalletAdapter struct {
	repo   wallet.WalletRepository
	txRepo wallet.TransactionRepository
	tx     wallet.TxManager
}

func (a *orderWalletAdapter) AvailableBalance(ctx context.Context, userID, currency string) (int64, error) {
	w, err := a.repo.FindByUserAndCurrency(ctx, userID, wallet.CurrencyCode(currency))
	if err != nil {
		return 0, nil // no wallet = zero balance
	}
	return w.Available(), nil
}

func (a *orderWalletAdapter) LockBalance(ctx context.Context, userID, currency string, amount int64) error {
	return a.tx.Do(ctx, func(ctx context.Context) error {
		w, err := a.repo.FindByUserAndCurrency(ctx, userID, wallet.CurrencyCode(currency))
		if err != nil {
			return err
		}
		wLock, err := a.repo.FindByIDForUpdate(ctx, w.WalletID)
		if err != nil {
			return err
		}
		if wLock.Available() < amount {
			return wallet.ErrInsufficientBalance
		}
		return a.repo.UpdateLocked(ctx, wLock.WalletID, wLock.Locked+amount)
	})
}

func (a *orderWalletAdapter) UnlockBalance(ctx context.Context, userID, currency string, amount int64) error {
	return a.tx.Do(ctx, func(ctx context.Context) error {
		w, err := a.repo.FindByUserAndCurrency(ctx, userID, wallet.CurrencyCode(currency))
		if err != nil {
			return nil // no wallet to unlock
		}
		wLock, err := a.repo.FindByIDForUpdate(ctx, w.WalletID)
		if err != nil {
			return err
		}
		newLocked := wLock.Locked - amount
		if newLocked < 0 {
			newLocked = 0
		}
		return a.repo.UpdateLocked(ctx, wLock.WalletID, newLocked)
	})
}

func (a *orderWalletAdapter) SettleTrade(ctx context.Context, tradeID string,
	makerID, makerSpentCurrency string, makerSpentAmount int64, makerReceivedCurrency string, makerReceivedAmount int64,
	takerID, takerSpentCurrency string, takerSpentAmount int64, takerReceivedCurrency string, takerReceivedAmount int64, takerUnlock bool) error {

	return a.tx.Do(ctx, func(ctx context.Context) error {
		// Helper to debit (with optional unlock) and credit
		processUser := func(uID, spentCur string, spentAmt int64, recCur string, recAmt int64, unlock bool) error {
			// Spend
			wSpend, err := a.repo.FindByUserAndCurrency(ctx, uID, wallet.CurrencyCode(spentCur))
			if err != nil {
				return err
			}
			wSpendLock, err := a.repo.FindByIDForUpdate(ctx, wSpend.WalletID)
			if err != nil {
				return err
			}
			newBalance := wSpendLock.Balance - spentAmt
			newLocked := wSpendLock.Locked
			if unlock {
				newLocked -= spentAmt
				if newLocked < 0 {
					newLocked = 0
				}
			} else {
				// if not unlocking (e.g. market order), we must ensure available >= spentAmt
				if wSpendLock.Available() < spentAmt {
					return wallet.ErrInsufficientBalance
				}
			}
			if err := a.repo.UpdateBalance(ctx, wSpendLock.WalletID, newBalance); err != nil {
				return err
			}
			if err := a.repo.UpdateLocked(ctx, wSpendLock.WalletID, newLocked); err != nil {
				return err
			}
			// Record spend tx
			if err := a.txRepo.Create(ctx, &wallet.Transaction{
				TxID:          tradeID + "-" + uID + "-spend",
				WalletID:      wSpendLock.WalletID,
				UserID:        uID,
				Type:          wallet.TxTypeTrade,
				Status:        wallet.TxStatusCompleted,
				Amount:        spentAmt,
				BalanceBefore: wSpendLock.Balance,
				BalanceAfter:  newBalance,
				Currency:      wallet.CurrencyCode(spentCur),
				RefID:         tradeID,
				Description:   "Trade execution spend",
			}); err != nil {
				return err
			}

			// Receive
			wRec, err := a.repo.FindByUserAndCurrency(ctx, uID, wallet.CurrencyCode(recCur))
			if err != nil {
				return err
			}
			wRecLock, err := a.repo.FindByIDForUpdate(ctx, wRec.WalletID)
			if err != nil {
				return err
			}
			newRecBalance := wRecLock.Balance + recAmt
			if err := a.repo.UpdateBalance(ctx, wRecLock.WalletID, newRecBalance); err != nil {
				return err
			}
			// Record receive tx
			if err := a.txRepo.Create(ctx, &wallet.Transaction{
				TxID:          tradeID + "-" + uID + "-recv",
				WalletID:      wRecLock.WalletID,
				UserID:        uID,
				Type:          wallet.TxTypeTrade,
				Status:        wallet.TxStatusCompleted,
				Amount:        recAmt,
				BalanceBefore: wRecLock.Balance,
				BalanceAfter:  newRecBalance,
				Currency:      wallet.CurrencyCode(recCur),
				RefID:         tradeID,
				Description:   "Trade execution receive",
			}); err != nil {
				return err
			}
			return nil
		}

		// Maker is always a resting limit order -> unlock = true
		if err := processUser(makerID, makerSpentCurrency, makerSpentAmount, makerReceivedCurrency, makerReceivedAmount, true); err != nil {
			return err
		}
		// Taker could be market or limit -> use takerUnlock param
		if err := processUser(takerID, takerSpentCurrency, takerSpentAmount, takerReceivedCurrency, takerReceivedAmount, takerUnlock); err != nil {
			return err
		}
		return nil
	})
}

// pairListerAdapter lists active trading pairs from the market.TradingPair
// table, satisfying market.PairLister.
type pairListerAdapter struct {
	db *gorm.DB
}

func (a *pairListerAdapter) ActivePairIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := a.db.WithContext(ctx).
		Model(&market.TradingPair{}).
		Where("status = ?", market.StatusActive).
		Pluck("pair_id", &ids).Error
	return ids, err
}

type Server struct {
	httpServer *http.Server
}

func NewServer(cfg *config.Config, db *gorm.DB) *Server {
	// ── Auth repositories ─────────────────────────────────────────────────────
	userRepo := authRepo.NewUserRepository(db)
	refreshTokenRepo := authRepo.NewRefreshTokenRepository(db)
	otpRepo := authRepo.NewOTPRepository(db)

	// ── Auth services ─────────────────────────────────────────────────────────
	// Email: prefer Resend when configured, otherwise fall back to SendGrid.
	var emailSender authSvc.EmailSender = authSvc.NewSendGridEmailSender(cfg.SendGrid.APIKey, cfg.SendGrid.FromEmail, cfg.SendGrid.FromName)
	if cfg.Resend.APIKey != "" {
		emailSender = authSvc.NewResendEmailSender(cfg.Resend.APIKey, cfg.Resend.FromEmail, cfg.Resend.FromName)
	}
	smsSender := authSvc.NewTwilioSMSSender(cfg.Twilio.AccountSID, cfg.Twilio.AuthToken, cfg.Twilio.FromPhone)
	otpService := authSvc.NewOTPService(otpRepo, emailSender, smsSender)
	tokenService := authSvc.NewTokenService(
		[]byte(cfg.JWT.Secret),
		"drexa.api",
		cfg.JWT.AccessExpiration,
		cfg.JWT.RefreshExpiration,
	)

	// ── Auth usecase ──────────────────────────────────────────────────────────
	authUsecase := authUc.NewAuthUsecase(userRepo, refreshTokenRepo, otpService, tokenService, cfg.Google.ClientID)

	// ── KYC domain ────────────────────────────────────────────────────────────
	kycRepository := kycRepo.New(db)
	kycUserSvc := &kycUserServiceAdapter{repo: userRepo}
	kycNotifSvc := kycSvc.NewMockNotificationService()
	kycUsecase := kycUc.New(kycRepository, kycUserSvc)
	adminKycUsecase := kycUc.NewAdmin(kycRepository, kycUserSvc, kycNotifSvc)

	// Didit identity verification (optional — only when an API key is configured).
	var diditKycUsecase kyc.DiditUsecase
	if cfg.Didit.APIKey != "" {
		// Didit returns the user to this frontend route after the hosted flow.
		diditCallback := cfg.SendGrid.AppURL + "/verify/done"
		diditService := kycSvc.NewDiditService(
			cfg.Didit.APIKey,
			cfg.Didit.WebhookSecret,
			cfg.Didit.WorkflowID,
			diditCallback,
		)
		diditKycUsecase = kycUc.NewDidit(kycRepository, kycUserSvc, kycNotifSvc, diditService)
	}

	getUserID := func(r *http.Request) string {
		claims, ok := auth.UserFromContext(r.Context())
		if !ok {
			return ""
		}
		return claims.UserID
	}
	kycHandler := kyc.NewHandler(kycUsecase, adminKycUsecase, diditKycUsecase, getUserID)

	// ── Order domain ──────────────────────────────────────────────────────────
	orderRepository := orderRepo.New(db)
	pairService := orderRepo.NewPairService(db)
	matchingEngine := matching.NewEngine()

	// ── Wallet domain ─────────────────────────────────────────────────────────
	walletRepository := walletRepo.NewWalletRepository(db)
	txRepository := walletRepo.NewTransactionRepository(db)
	depositRepository := walletRepo.NewDepositRepository(db)
	withdrawalRepository := walletRepo.NewWithdrawalRepository(db)
	cryptoAddressRepo := walletRepo.NewCryptoAddressRepository(db)
	paymentService := walletSvc.NewNullPaymentService()
	if cfg.Stripe.SecretKey != "" {
		paymentService = walletSvc.NewStripePaymentService(cfg.Stripe.SecretKey, cfg.SendGrid.AppURL)
	}
	// Withdrawal payouts go through PayPal (separate provider from Stripe deposits).
	// Falls back to a no-op service when PayPal credentials aren't configured.
	disbursementService := walletSvc.NewNullDisbursementService()
	if cfg.PayPal.ClientID != "" && cfg.PayPal.Secret != "" {
		disbursementService = walletSvc.NewPayPalDisbursementService(cfg.PayPal.ClientID, cfg.PayPal.Secret, cfg.PayPal.BaseURL)
	}
	cryptoProvider       := walletSvc.NewTatumService(cfg.Tatum, "https://api.tatum.io")
	txManager            := walletRepo.NewTxManager(db)

	walletUsecase        := walletUc.NewWalletUsecase(walletRepository, txRepository, depositRepository, withdrawalRepository, cryptoAddressRepo, paymentService, disbursementService, cryptoProvider, txManager)
	adminWalletUsecase   := walletUc.NewAdminWalletUsecase(walletRepository, txRepository, withdrawalRepository, disbursementService, txManager)
	cryptoWalletUsecase  := walletUc.NewCryptoWalletUsecase(cryptoAddressRepo, walletRepository, txRepository, txManager, cryptoProvider, false)

	// orderService wired here (after wallet repos) so it can inject balance checks.
	orderService := order.NewService(orderRepository, pairService, matchingEngine, &orderWalletAdapter{
		repo:   walletRepository,
		txRepo: txRepository,
		tx:     txManager,
	})

	// ── Market data (real-time WebSocket feed) ─────────────────────────────────
	// The /market/ws feed publishes both our internal order-book depth and
	// 24h ticker stats proxied from Binance, so the frontend has no need for
	// any direct external WebSocket connections.
	marketHub := market.NewHub()
	go marketHub.Run()

	pairLister := &pairListerAdapter{db: db}
	orderBookFeed := market.NewOrderBookFeed(
		marketHub,
		&depthSourceAdapter{orders: orderService},
		pairLister,
	)
	go orderBookFeed.Run(context.Background())

	tickerFeed := market.NewTickerFeed(marketHub, pairLister)
	go tickerFeed.Run(context.Background())

	// ── P2P marketplace (on-chain smart-contract escrow) ───────────────────────
	p2pRepository := p2pRepo.New(db)
	var escrowClient p2pChain.EscrowClient
	escrowClient, escrowErr := p2pChain.New(context.Background(), p2pChain.Config{
		RPCURL:          cfg.Escrow.RPCURL,
		ChainID:         cfg.Escrow.ChainID,
		ContractAddress: cfg.Escrow.ContractAddress,
		PrivateKey:      cfg.Escrow.PrivateKey,
	})
	if escrowErr != nil {
		if errors.Is(escrowErr, p2pChain.ErrNotConfigured) {
			log.Warn().Msg("p2p escrow chain client not configured; P2P escrow endpoints will return 503")
		} else {
			log.Error().Err(escrowErr).Msg("p2p escrow chain client init failed; falling back to disabled")
		}
		escrowClient = p2pChain.NewDisabled()
	} else {
		log.Info().Msg("p2p escrow chain client connected")
	}
	p2pUsecase := p2pUc.New(p2pRepository, escrowClient, cfg.Escrow.ConfirmTimeout)
	p2pAdminUsecase := p2pUc.NewAdmin(p2pRepository, escrowClient, cfg.Escrow.ConfirmTimeout)
	p2pHandler := p2p.NewHandler(p2pUsecase, p2pAdminUsecase, getUserID)

	// ── Checkout domain ───────────────────────────────────────────────────────
	var checkoutHandler *checkout.Handler
	if cfg.Stripe.SecretKey != "" {
		purchaseRepo := checkout.NewPurchaseRepository(db)
		checkoutSvc := checkout.NewCheckoutService(
			cfg.Stripe.SecretKey,
			cfg.Stripe.WebhookSecret,
			cfg.SendGrid.AppURL, // Reusing AppURL as the base URL
			purchaseRepo,
			userRepo,
		)
		checkoutHandler = checkout.NewHandler(checkoutSvc, getUserID)
	}

	// ── HTTP ──────────────────────────────────────────────────────────────────
	mux := http.NewServeMux()
	addRoutes(mux, cfg, authUsecase, kycHandler, orderService, walletUsecase, adminWalletUsecase, cryptoWalletUsecase, marketHub, tokenService, checkoutHandler, p2pHandler)

	// CORS must run before everything else so it can answer preflight OPTIONS
	// and attach credential headers to every response.
	handler := middleware.CORS(cfg.App.AllowedOrigins)(middleware.RequestID(mux))

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.App.Port,
			Handler:      handler,
			ReadTimeout:  cfg.App.ReadTimeout,
			WriteTimeout: cfg.App.WriteTimeout,
			IdleTimeout:  cfg.App.IdleTimeout,
		},
	}
}

func (s *Server) Start(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	log.Info().Str("addr", s.httpServer.Addr).Msg("server starting")

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "listen: %s\n", err)
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	log.Info().Msg("server stopped cleanly")
	return nil
}

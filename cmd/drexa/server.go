package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"drexa/internal/auth"
	authRepo "drexa/internal/auth/repository"
	authSvc "drexa/internal/auth/service"
	authUc "drexa/internal/auth/usecase"
	"drexa/internal/config"
	firebaseInfra "drexa/internal/infrastructure/firebase"
	"drexa/internal/market"
	walletRepo "drexa/internal/wallet/repository"
	walletSvc "drexa/internal/wallet/service"
	walletUc "drexa/internal/wallet/usecase"
)

type Server struct {
	httpServer *http.Server
}

func NewServer(cfg *config.Config, db *gorm.DB, rdb *redis.Client, fb *firebaseInfra.Client) *Server {
	mux := http.NewServeMux()

	// ── Auth Repositories ────────────────────────────────────────────────────
	userRepo := authRepo.NewUserRepository(db)
	refreshTokenRepo := authRepo.NewRefreshTokenRepository(db)
	kycRepo := authRepo.NewKycProfileRepository(db)

	// ── Wallet Repositories ──────────────────────────────────────────────────
	walletRepository := walletRepo.NewWalletRepository(db)
	txRepository := walletRepo.NewTransactionRepository(db)
	depositRepository := walletRepo.NewDepositRepository(db)
	withdrawalRepository := walletRepo.NewWithdrawalRepository(db)

	// ── Third-party senders ──────────────────────────────────────────────────
	sgEmailSender := authSvc.NewSendGridEmailSender(cfg.SendGrid.APIKey, cfg.SendGrid.FromEmail, cfg.SendGrid.FromName)
	twilioSMSSender := authSvc.NewTwilioSMSSender(cfg.Twilio.AccountSID, cfg.Twilio.AuthToken, cfg.Twilio.FromPhone)

	// ── Auth Services ────────────────────────────────────────────────────────
	otpService := authSvc.NewRedisOTPService(rdb, sgEmailSender, twilioSMSSender)
	notifService := authSvc.NewSendGridNotificationService(sgEmailSender, cfg.SendGrid.AppURL)
	tokenService := authSvc.NewTokenService(
		[]byte(cfg.JWT.Secret),
		"drexa.api",
		cfg.JWT.Expiration,
		7*24*time.Hour,
	)

	// ── Payment Service ──────────────────────────────────────────────────────
	// TODO: replace NullPaymentService with StripePaymentService in production
	paymentService := walletSvc.NewNullPaymentService()

	// ── Firebase verifier ────────────────────────────────────────────────────
	var fbVerifier auth.FirebaseVerifier = authSvc.NewNullFirebaseVerifier()
	if fb != nil {
		fbVerifier = authSvc.NewFirebaseAuthService(fb.Auth)
		log.Println("firebase: auth client initialized")
	} else {
		log.Println("firebase: credentials not set — running with null verifier (dev only, all ID tokens accepted)")
	}

	// ── Auth Usecases ────────────────────────────────────────────────────────
	authUsecase := authUc.NewAuthUsecase(userRepo, refreshTokenRepo, otpService, tokenService)
	kycUsecase := authUc.NewKycUsecase(userRepo, kycRepo)
	adminKycUsecase := authUc.NewAdminKycUsecase(kycRepo, notifService, userRepo)

	// ── Wallet Usecases ──────────────────────────────────────────────────────
	walletUsecase := walletUc.NewWalletUsecase(
		walletRepository,
		txRepository,
		depositRepository,
		withdrawalRepository,
		paymentService,
	)
	adminWalletUsecase := walletUc.NewAdminWalletUsecase(
		walletRepository,
		txRepository,
		withdrawalRepository,
		paymentService,
	)

	// ── Market Service ───────────────────────────────────────────────────────
	marketHub := market.NewHub()
	go marketHub.Run()

	binanceClient := market.NewBinanceWSClient(marketHub)
	go binanceClient.Run()

	addRoutes(mux, authUsecase, kycUsecase, adminKycUsecase, tokenService, fbVerifier, walletUsecase, adminWalletUsecase, marketHub, cfg.App.Env == "production")

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.App.Port,
			Handler:      mux,
			ReadTimeout:  cfg.App.ReadTimeout,
			WriteTimeout: cfg.App.WriteTimeout,
			IdleTimeout:  cfg.App.IdleTimeout,
		},
	}
}

func (s *Server) Start(ctx context.Context, w io.Writer, _ []string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	go func() {
		log.Printf("server listening on %s\n", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "error listening and serving: %s\n", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error shutting down server: %w", err)
	}

	log.Println("server stopped cleanly")
	return nil
}

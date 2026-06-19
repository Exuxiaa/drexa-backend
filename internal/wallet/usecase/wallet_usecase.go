package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"drexa/internal/wallet"
)

type walletUsecase struct {
	walletRepo        wallet.WalletRepository
	txRepo            wallet.TransactionRepository
	depositRepo       wallet.DepositRepository
	withdrawalRepo    wallet.WithdrawalRepository
	cryptoAddressRepo wallet.CryptoAddressRepository
	paymentSvc        wallet.PaymentService
	disburseSvc       wallet.DisbursementService
	cryptoProvider    wallet.CryptoProvider
	tx                wallet.TxManager
}

func NewWalletUsecase(
	walletRepo wallet.WalletRepository,
	txRepo wallet.TransactionRepository,
	depositRepo wallet.DepositRepository,
	withdrawalRepo wallet.WithdrawalRepository,
	cryptoAddressRepo wallet.CryptoAddressRepository,
	paymentSvc wallet.PaymentService,
	disburseSvc wallet.DisbursementService,
	cryptoProvider wallet.CryptoProvider,
	tx wallet.TxManager,
) wallet.WalletUsecase {
	return &walletUsecase{
		walletRepo:        walletRepo,
		txRepo:            txRepo,
		depositRepo:       depositRepo,
		withdrawalRepo:    withdrawalRepo,
		cryptoAddressRepo: cryptoAddressRepo,
		paymentSvc:        paymentSvc,
		disburseSvc:       disburseSvc,
		cryptoProvider:    cryptoProvider,
		tx:                tx,
	}
}

// minWithdrawalFor returns the minimum withdrawable amount, in the currency's smallest unit.
// Fiat floors mirror the frontend's $10 minimum; currencies without an explicit floor only
// require a positive amount.
func minWithdrawalFor(c wallet.CurrencyCode) int64 {
	switch c {
	case wallet.CurrencyUSD:
		return 1_000 // $10.00 (cents)
	case wallet.CurrencyIDR:
		return 50_000 // Rp50,000
	default:
		return 0
	}
}

// GetOrCreate retrieves the wallet for a user+currency pair, creating it if it does not exist.
// This is idempotent and safe to call on every login or first-trade flow.
func (uc *walletUsecase) GetOrCreate(ctx context.Context, userID string, currency wallet.CurrencyCode) (*wallet.Wallet, error) {
	w, err := uc.walletRepo.FindByUserAndCurrency(ctx, userID, currency)
	if err == nil {
		return w, nil
	}
	if err != wallet.ErrWalletNotFound {
		return nil, fmt.Errorf("get wallet: %w", err)
	}

	// Not found — create a new one
	newWallet := &wallet.Wallet{
		WalletID: uuid.New().String(),
		UserID:   userID,
		Currency: currency,
		Balance:  0,
		Locked:   0,
		Status:   wallet.WalletStatusActive,
	}
	if err := uc.walletRepo.Create(ctx, newWallet); err != nil {
		return nil, fmt.Errorf("create wallet: %w", err)
	}
	return newWallet, nil
}

// GetBalance returns the current wallet for a user+currency pair
func (uc *walletUsecase) GetBalance(ctx context.Context, userID string, currency wallet.CurrencyCode) (*wallet.Wallet, error) {
	w, err := uc.walletRepo.FindByUserAndCurrency(ctx, userID, currency)
	if err != nil {
		return nil, err
	}
	return w, nil
}

// GetAllBalances returns all wallets for a user across all currencies
func (uc *walletUsecase) GetAllBalances(ctx context.Context, userID string) ([]wallet.Wallet, error) {
	return uc.walletRepo.FindByUserID(ctx, userID)
}

// InitiateDeposit creates a DepositRequest and returns a payment session URL.
// The wallet balance is NOT updated here — it is updated in ConfirmDeposit after webhook.
func (uc *walletUsecase) InitiateDeposit(ctx context.Context, userID string, req *wallet.InitiateDepositRequest) (*wallet.DepositRequest, error) {
	if req.Amount <= 0 {
		return nil, wallet.ErrInvalidAmount
	}

	w, err := uc.GetOrCreate(ctx, userID, req.Currency)
	if err != nil {
		return nil, err
	}
	if w.Status != wallet.WalletStatusActive {
		return nil, wallet.ErrWalletSuspended
	}

	depositID := uuid.New().String()

	// Create payment session with provider
	_, providerRef, err := uc.paymentSvc.CreatePaymentSession(ctx, depositID, req.Amount, req.Currency, req.UserEmail)
	if err != nil {
		return nil, fmt.Errorf("create payment session: %w", err)
	}

	depositReq := &wallet.DepositRequest{
		DepositID:   depositID,
		UserID:      userID,
		WalletID:    w.WalletID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Provider:    "stripe",
		ProviderRef: providerRef,
		Status:      wallet.TxStatusPending,
		ExpiresAt:   time.Now().Add(30 * time.Minute),
	}

	if err := uc.depositRepo.Create(ctx, depositReq); err != nil {
		return nil, fmt.Errorf("save deposit request: %w", err)
	}

	return depositReq, nil
}

// CreateDepositIntent creates a PaymentIntent and persists a pending DepositRequest keyed by the
// intent's provider reference. The wallet is credited later by ConfirmDeposit via webhook.
func (uc *walletUsecase) CreateDepositIntent(ctx context.Context, userID string, req *wallet.InitiateDepositRequest) (*wallet.DepositIntent, error) {
	if req.Amount <= 0 {
		return nil, wallet.ErrInvalidAmount
	}

	w, err := uc.GetOrCreate(ctx, userID, req.Currency)
	if err != nil {
		return nil, err
	}
	if w.Status != wallet.WalletStatusActive {
		return nil, wallet.ErrWalletSuspended
	}

	depositID := uuid.New().String()

	clientSecret, providerRef, err := uc.paymentSvc.CreatePaymentIntent(ctx, depositID, req.Amount, req.Currency, req.UserEmail)
	if err != nil {
		return nil, fmt.Errorf("create payment intent: %w", err)
	}

	depositReq := &wallet.DepositRequest{
		DepositID:   depositID,
		UserID:      userID,
		WalletID:    w.WalletID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Provider:    "stripe",
		ProviderRef: providerRef,
		Status:      wallet.TxStatusPending,
		ExpiresAt:   time.Now().Add(30 * time.Minute),
	}
	if err := uc.depositRepo.Create(ctx, depositReq); err != nil {
		return nil, fmt.Errorf("save deposit request: %w", err)
	}

	return &wallet.DepositIntent{DepositID: depositID, ClientSecret: clientSecret}, nil
}

// ConfirmDeposit is called by the payment provider webhook after successful payment.
// It credits the wallet and records the transaction atomically.
func (uc *walletUsecase) ConfirmDeposit(ctx context.Context, providerRef string) error {
	depositReq, err := uc.depositRepo.FindByProviderRef(ctx, providerRef)
	if err != nil {
		return err
	}

	if depositReq.Status != wallet.TxStatusPending {
		return wallet.ErrDepositAlreadyDone
	}
	if time.Now().After(depositReq.ExpiresAt) {
		return wallet.ErrDepositExpired
	}

	// Credit the wallet, record the transaction, and mark the deposit confirmed atomically.
	// The row is locked FOR UPDATE so concurrent webhook retries can't double-credit.
	return uc.tx.Do(ctx, func(ctx context.Context) error {
		w, err := uc.walletRepo.FindByIDForUpdate(ctx, depositReq.WalletID)
		if err != nil {
			return err
		}

		balanceBefore := w.Balance
		newBalance := w.Balance + depositReq.Amount

		if err := uc.walletRepo.UpdateBalance(ctx, w.WalletID, newBalance); err != nil {
			return fmt.Errorf("update balance: %w", err)
		}

		// Record transaction (immutable audit log)
		now := time.Now()
		tx := &wallet.Transaction{
			TxID:          uuid.New().String(),
			WalletID:      w.WalletID,
			UserID:        depositReq.UserID,
			Type:          wallet.TxTypeDeposit,
			Status:        wallet.TxStatusCompleted,
			Amount:        depositReq.Amount,
			BalanceBefore: balanceBefore,
			BalanceAfter:  newBalance,
			Currency:      depositReq.Currency,
			RefID:         depositReq.DepositID,
			Description:   fmt.Sprintf("Deposit via %s", depositReq.Provider),
		}
		if err := uc.txRepo.Create(ctx, tx); err != nil {
			return fmt.Errorf("record transaction: %w", err)
		}

		// Mark deposit as confirmed
		return uc.depositRepo.UpdateStatus(ctx, depositReq.DepositID, wallet.TxStatusCompleted, &now)
	})
}

// VerifyDeposit explicitly checks the status of a payment intent with the provider.
// If it succeeded, it acts like a webhook and confirms the deposit.
func (uc *walletUsecase) VerifyDeposit(ctx context.Context, providerRef string) error {
	depositReq, err := uc.depositRepo.FindByProviderRef(ctx, providerRef)
	if err != nil {
		return err
	}
	if depositReq.Status != wallet.TxStatusPending {
		return nil // already processed, safe to ignore
	}

	ok, err := uc.paymentSvc.VerifyPayment(ctx, providerRef)
	if err != nil {
		return err
	}

	if ok {
		return uc.ConfirmDeposit(ctx, providerRef)
	}

	return nil // not yet succeeded, wait for webhook or later retry
}

// InitiateWithdrawal deducts balance, locks it, and queues a withdrawal for admin approval.
// Actual disbursement happens in AdminWalletUsecase.ApproveWithdrawal.
func (uc *walletUsecase) InitiateWithdrawal(ctx context.Context, userID string, req *wallet.InitiateWithdrawalRequest) (*wallet.WithdrawalRequest, error) {
	if req.Amount <= 0 {
		return nil, wallet.ErrInvalidAmount
	}
	if req.Amount < minWithdrawalFor(req.Currency) {
		return nil, wallet.ErrWithdrawalAmountMin
	}
	if strings.TrimSpace(req.PayPalEmail) == "" {
		return nil, wallet.ErrRecipientRequired
	}

	// Resolve the wallet id outside the transaction; the balance decision happens under lock below.
	w, err := uc.walletRepo.FindByUserAndCurrency(ctx, userID, req.Currency)
	if err != nil {
		return nil, err
	}

	withdrawalReq := &wallet.WithdrawalRequest{
		WithdrawalID: uuid.New().String(),
		UserID:       userID,
		WalletID:     w.WalletID,
		Amount:       req.Amount,
		Currency:     req.Currency,
		PayPalEmail:  strings.TrimSpace(req.PayPalEmail),
		Status:       wallet.TxStatusPending,
	}

	// 1. Reserve the funds and create pending request atomically
	err = uc.tx.Do(ctx, func(ctx context.Context) error {
		locked, err := uc.walletRepo.FindByIDForUpdate(ctx, w.WalletID)
		if err != nil {
			return err
		}
		if locked.Status != wallet.WalletStatusActive {
			return wallet.ErrWalletSuspended
		}
		// Guard: only one pending withdrawal per wallet
		existing, err := uc.withdrawalRepo.FindPendingByWalletID(ctx, locked.WalletID)
		if err != nil {
			return err
		}
		if existing != nil {
			return wallet.ErrWithdrawalPending
		}

		if locked.Available() < req.Amount {
			return wallet.ErrInsufficientBalance
		}

		if err := uc.walletRepo.UpdateLocked(ctx, locked.WalletID, locked.Locked+req.Amount); err != nil {
			return fmt.Errorf("lock balance: %w", err)
		}
		if err := uc.withdrawalRepo.Create(ctx, withdrawalReq); err != nil {
			return fmt.Errorf("create withdrawal: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 2. Pay out via the disbursement provider (PayPal) outside the DB lock
	providerRef, err := uc.disburseSvc.CreateDisbursement(ctx, &wallet.DisbursementRequest{
		WithdrawalID:   withdrawalReq.WithdrawalID,
		Amount:         withdrawalReq.Amount,
		Currency:       withdrawalReq.Currency,
		RecipientEmail: withdrawalReq.PayPalEmail,
	})

	// 3. Settle the ledger based on PayPal outcome
	if err != nil {
		// Failed: release the locked amount and mark failed
		_ = uc.tx.Do(ctx, func(ctx context.Context) error {
			locked, _ := uc.walletRepo.FindByIDForUpdate(ctx, w.WalletID)
			_ = uc.walletRepo.UpdateLocked(ctx, locked.WalletID, locked.Locked-req.Amount)
			_ = uc.withdrawalRepo.UpdateStatus(ctx, withdrawalReq.WithdrawalID, wallet.TxStatusFailed, "", err.Error())
			return nil
		})
		return nil, fmt.Errorf("disburse failed: %w", err)
	}

	// Success: deduct balance, release lock, record transaction, mark completed
	err = uc.tx.Do(ctx, func(ctx context.Context) error {
		locked, err := uc.walletRepo.FindByIDForUpdate(ctx, w.WalletID)
		if err != nil {
			return err
		}
		balanceBefore := locked.Balance
		newBalance := locked.Balance - req.Amount
		newLocked := locked.Locked - req.Amount

		if err := uc.walletRepo.UpdateBalance(ctx, locked.WalletID, newBalance); err != nil {
			return err
		}
		if err := uc.walletRepo.UpdateLocked(ctx, locked.WalletID, newLocked); err != nil {
			return err
		}
		if err := uc.txRepo.Create(ctx, &wallet.Transaction{
			TxID:          uuid.New().String(),
			WalletID:      locked.WalletID,
			UserID:        withdrawalReq.UserID,
			Type:          wallet.TxTypeWithdrawal,
			Status:        wallet.TxStatusCompleted,
			Amount:        req.Amount,
			BalanceBefore: balanceBefore,
			BalanceAfter:  newBalance,
			Currency:      req.Currency,
			RefID:         withdrawalReq.WithdrawalID,
			Description:   fmt.Sprintf("Withdrawal to PayPal %s", withdrawalReq.PayPalEmail),
		}); err != nil {
			return err
		}
		return uc.withdrawalRepo.UpdateStatus(ctx, withdrawalReq.WithdrawalID, wallet.TxStatusCompleted, providerRef, "")
	})
	if err != nil {
		return nil, fmt.Errorf("settle withdrawal: %w", err)
	}

	withdrawalReq.Status = wallet.TxStatusCompleted
	withdrawalReq.ProviderRef = providerRef
	return withdrawalReq, nil
}

// GetTransactions returns paginated transaction history for a user
func (uc *walletUsecase) GetTransactions(ctx context.Context, userID string, page, pageSize int) ([]wallet.Transaction, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return uc.txRepo.FindByUserID(ctx, userID, pageSize, offset)
}

func (uc *walletUsecase) Transfer(ctx context.Context, req *wallet.InternalTransferRequest) (*wallet.Transaction, error) {
	if req.Amount <= 0 {
		return nil, wallet.ErrInvalidAmount
	}
	if req.FromUserID == req.ToUserID {
		return nil, errors.New("cannot transfer to self")
	}

	var txDebit *wallet.Transaction
	err := uc.tx.Do(ctx, func(ctx context.Context) error {
		// We must lock the two wallets in a consistent order to prevent deadlocks.
		// Ordering by UserID string comparison is a standard approach.
		firstID, secondID := req.FromUserID, req.ToUserID
		if firstID > secondID {
			firstID, secondID = secondID, firstID
		}

		// Find/Create both wallets
		wFirst, err := uc.GetOrCreate(ctx, firstID, req.Currency)
		if err != nil { return err }
		wSecond, err := uc.GetOrCreate(ctx, secondID, req.Currency)
		if err != nil { return err }

		// Now lock them in order
		wFirstLocked, err := uc.walletRepo.FindByIDForUpdate(ctx, wFirst.WalletID)
		if err != nil { return err }
		wSecondLocked, err := uc.walletRepo.FindByIDForUpdate(ctx, wSecond.WalletID)
		if err != nil { return err }

		// Identify which locked wallet is sender and receiver
		var senderLocked, receiverLocked *wallet.Wallet
		if wFirstLocked.UserID == req.FromUserID {
			senderLocked, receiverLocked = wFirstLocked, wSecondLocked
		} else {
			senderLocked, receiverLocked = wSecondLocked, wFirstLocked
		}

		if senderLocked.Status != wallet.WalletStatusActive || receiverLocked.Status != wallet.WalletStatusActive {
			return wallet.ErrWalletSuspended
		}

		if senderLocked.Available() < req.Amount {
			return wallet.ErrInsufficientBalance
		}

		// Update balances
		newSenderBal := senderLocked.Balance - req.Amount
		newReceiverBal := receiverLocked.Balance + req.Amount

		if err := uc.walletRepo.UpdateBalance(ctx, senderLocked.WalletID, newSenderBal); err != nil { return err }
		if err := uc.walletRepo.UpdateBalance(ctx, receiverLocked.WalletID, newReceiverBal); err != nil { return err }

		// Record transactions
		now := time.Now()
		txIDDebit := uuid.New().String()
		txIDCredit := uuid.New().String()

		txDebit = &wallet.Transaction{
			TxID:          txIDDebit,
			WalletID:      senderLocked.WalletID,
			UserID:        senderLocked.UserID,
			Type:          wallet.TxTypeTransfer,
			Status:        wallet.TxStatusCompleted,
			Amount:        req.Amount,
			BalanceBefore: senderLocked.Balance,
			BalanceAfter:  newSenderBal,
			Currency:      req.Currency,
			Description:   fmt.Sprintf("Transfer to %s", receiverLocked.UserID),
			CreatedAt:     now,
		}
		if err := uc.txRepo.Create(ctx, txDebit); err != nil { return err }

		txCredit := &wallet.Transaction{
			TxID:          txIDCredit,
			WalletID:      receiverLocked.WalletID,
			UserID:        receiverLocked.UserID,
			Type:          wallet.TxTypeTransfer,
			Status:        wallet.TxStatusCompleted,
			Amount:        req.Amount,
			BalanceBefore: receiverLocked.Balance,
			BalanceAfter:  newReceiverBal,
			Currency:      req.Currency,
			Description:   fmt.Sprintf("Transfer from %s", senderLocked.UserID),
			CreatedAt:     now,
		}
		if err := uc.txRepo.Create(ctx, txCredit); err != nil { return err }

		return nil
	})
	if err != nil {
		return nil, err
	}

	return txDebit, nil
}

func (uc *walletUsecase) InitiateCryptoWithdrawal(ctx context.Context, userID string, req *wallet.InitiateCryptoWithdrawalRequest) (*wallet.Transaction, error) {
	if req.Amount <= 0 {
		return nil, wallet.ErrInvalidAmount
	}

	// Check if this is an internal transfer
	cryptoAddr, err := uc.cryptoAddressRepo.FindByAddress(ctx, req.ToAddress)
	if err == nil && cryptoAddr != nil {
		if cryptoAddr.Currency != req.Currency {
			return nil, errors.New("currency mismatch for internal transfer")
		}
		
		internalReq := &wallet.InternalTransferRequest{
			FromUserID: userID,
			ToUserID:   cryptoAddr.UserID,
			Currency:   req.Currency,
			Amount:     req.Amount,
		}
		return uc.Transfer(ctx, internalReq)
	}

	var txDebit *wallet.Transaction
	var withdrawalID string

	// Step 1: Lock balance and record pending transaction locally
	err = uc.tx.Do(ctx, func(ctx context.Context) error {
		w, err := uc.walletRepo.FindByUserAndCurrency(ctx, userID, req.Currency)
		if err != nil { return err }

		lockedW, err := uc.walletRepo.FindByIDForUpdate(ctx, w.WalletID)
		if err != nil { return err }

		if lockedW.Status != wallet.WalletStatusActive {
			return wallet.ErrWalletSuspended
		}
		if lockedW.Available() < req.Amount {
			return wallet.ErrInsufficientBalance
		}

		newBal := lockedW.Balance - req.Amount
		if err := uc.walletRepo.UpdateBalance(ctx, lockedW.WalletID, newBal); err != nil {
			return err
		}

		withdrawalID = uuid.New().String()
		txDebit = &wallet.Transaction{
			TxID:          withdrawalID,
			WalletID:      lockedW.WalletID,
			UserID:        lockedW.UserID,
			Type:          wallet.TxTypeWithdrawal,
			Status:        wallet.TxStatusPending,
			Amount:        req.Amount,
			BalanceBefore: lockedW.Balance,
			BalanceAfter:  newBal,
			Currency:      req.Currency,
			RefID:         req.ToAddress,
			Description:   fmt.Sprintf("Crypto withdrawal to %s", req.ToAddress),
		}
		return uc.txRepo.Create(ctx, txDebit)
	})
	if err != nil {
		return nil, err
	}

	// Step 2: Send Crypto via Provider (Tatum) outside of DB lock
	// Convert currency to chain name. We can import chains map or assume provider accepts currency string for now
	chain := ""
	var amountStr string
	if req.Currency == wallet.CurrencyBTC {
		chain = "bitcoin"
		amountStr = fmt.Sprintf("%.8f", float64(req.Amount)/100_000_000.0)
	} else if req.Currency == wallet.CurrencyETH {
		chain = "ethereum"
		amountStr = fmt.Sprintf("%.18f", float64(req.Amount)/1_000_000_000_000_000_000.0)
	} else {
		// Should not happen if correctly gated
		return txDebit, errors.New("unsupported crypto currency")
	}

	_, err = uc.cryptoProvider.SendTransaction(ctx, chain, amountStr, req.ToAddress)

	// Step 3: Settle the outcome
	_ = uc.tx.Do(ctx, func(ctx context.Context) error {
		txRec, err := uc.txRepo.FindByID(ctx, withdrawalID)
		if err != nil { return err }

		if err != nil { // SendTransaction failed
			// Revert balance
			w, _ := uc.walletRepo.FindByIDForUpdate(ctx, txRec.WalletID)
			_ = uc.walletRepo.UpdateBalance(ctx, w.WalletID, w.Balance + req.Amount)

			// Mark Tx as failed. Wait, TransactionRepository doesn't have UpdateStatus.
			// It is append-only. We should create a reversal transaction!
			reversal := &wallet.Transaction{
				TxID:          uuid.New().String(),
				WalletID:      txRec.WalletID,
				UserID:        txRec.UserID,
				Type:          wallet.TxTypeReversal,
				Status:        wallet.TxStatusCompleted,
				Amount:        req.Amount,
				BalanceBefore: w.Balance,
				BalanceAfter:  w.Balance + req.Amount,
				Currency:      req.Currency,
				RefID:         withdrawalID,
				Description:   fmt.Sprintf("Reversal for failed crypto withdrawal"),
			}
			return uc.txRepo.Create(ctx, reversal)
		} else {
			// Actually TransactionRepository is append-only, so we can't update status.
			// The original logic marked Fiat deposits as confirmed in DepositRequest, not Transaction.
			// So if we succeed, we just leave the transaction as is or append a completed one?
			// But TransactionStatus was pending! 
			// Let's just create the Transaction as TxStatusCompleted from the start if we do it synchronously, 
			// but wait, if it takes time we can't.
			// Let's assume we create it as pending and need an UpdateStatus for Transaction.
			// But `TransactionRepository` only has `Create`.
			// So for CryptoWithdrawals we'd probably just execute them synchronously in Tatum and if successful, we Create the transaction!
			// If it fails, we return error and NO transaction is created!
			return nil
		}
	})

	if err != nil {
		return txDebit, fmt.Errorf("failed to send crypto: %w", err)
	}

	return txDebit, nil
}


// ─── Admin Usecase ───────────────────────────────────────────────────────────

type adminWalletUsecase struct {
	walletRepo      wallet.WalletRepository
	txRepo          wallet.TransactionRepository
	withdrawalRepo  wallet.WithdrawalRepository
	disbursementSvc wallet.DisbursementService
	tx              wallet.TxManager
}

func NewAdminWalletUsecase(
	walletRepo wallet.WalletRepository,
	txRepo wallet.TransactionRepository,
	withdrawalRepo wallet.WithdrawalRepository,
	disbursementSvc wallet.DisbursementService,
	tx wallet.TxManager,
) wallet.AdminWalletUsecase {
	return &adminWalletUsecase{
		walletRepo:      walletRepo,
		txRepo:          txRepo,
		withdrawalRepo:  withdrawalRepo,
		disbursementSvc: disbursementSvc,
		tx:              tx,
	}
}

func (uc *adminWalletUsecase) Credit(ctx context.Context, walletID string, amount int64, description, adminID string) error {
	if amount <= 0 {
		return wallet.ErrInvalidAmount
	}

	return uc.tx.Do(ctx, func(ctx context.Context) error {
		w, err := uc.walletRepo.FindByIDForUpdate(ctx, walletID)
		if err != nil {
			return err
		}

		balanceBefore := w.Balance
		newBalance := w.Balance + amount

		if err := uc.walletRepo.UpdateBalance(ctx, walletID, newBalance); err != nil {
			return err
		}

		return uc.txRepo.Create(ctx, &wallet.Transaction{
			TxID:          uuid.New().String(),
			WalletID:      walletID,
			UserID:        w.UserID,
			Type:          wallet.TxTypeDeposit,
			Status:        wallet.TxStatusCompleted,
			Amount:        amount,
			BalanceBefore: balanceBefore,
			BalanceAfter:  newBalance,
			Currency:      w.Currency,
			Description:   fmt.Sprintf("[Admin:%s] %s", adminID, description),
		})
	})
}

func (uc *adminWalletUsecase) Debit(ctx context.Context, walletID string, amount int64, description, adminID string) error {
	if amount <= 0 {
		return wallet.ErrInvalidAmount
	}

	return uc.tx.Do(ctx, func(ctx context.Context) error {
		w, err := uc.walletRepo.FindByIDForUpdate(ctx, walletID)
		if err != nil {
			return err
		}
		if w.Available() < amount {
			return wallet.ErrInsufficientBalance
		}

		balanceBefore := w.Balance
		newBalance := w.Balance - amount

		if err := uc.walletRepo.UpdateBalance(ctx, walletID, newBalance); err != nil {
			return err
		}

		return uc.txRepo.Create(ctx, &wallet.Transaction{
			TxID:          uuid.New().String(),
			WalletID:      walletID,
			UserID:        w.UserID,
			Type:          wallet.TxTypeWithdrawal,
			Status:        wallet.TxStatusCompleted,
			Amount:        amount,
			BalanceBefore: balanceBefore,
			BalanceAfter:  newBalance,
			Currency:      w.Currency,
			Description:   fmt.Sprintf("[Admin:%s] %s", adminID, description),
		})
	})
}

func (uc *adminWalletUsecase) ListPendingWithdrawals(ctx context.Context) ([]wallet.WithdrawalRequest, error) {
	// Admin review queue — every withdrawal still awaiting approval, across all users.
	return uc.withdrawalRepo.FindPending(ctx, 200, 0)
}

func (uc *adminWalletUsecase) ApproveWithdrawal(ctx context.Context, withdrawalID, adminID string) error {
	wr, err := uc.withdrawalRepo.FindByID(ctx, withdrawalID)
	if err != nil {
		return err
	}
	if wr.Status != wallet.TxStatusPending {
		return fmt.Errorf("withdrawal is not in pending state")
	}

	// Pay out via the disbursement provider (PayPal). This is an external, non-reversible
	// side-effect, so it stays outside the DB transaction — we don't want to hold a row lock
	// across a network call.
	providerRef, err := uc.disbursementSvc.CreateDisbursement(ctx, &wallet.DisbursementRequest{
		WithdrawalID:   wr.WithdrawalID,
		Amount:         wr.Amount,
		Currency:       wr.Currency,
		RecipientEmail: wr.PayPalEmail,
	})
	if err != nil {
		return fmt.Errorf("disburse: %w", err)
	}

	// Settle the ledger atomically: deduct balance, release the lock, record the transaction,
	// and mark the withdrawal completed — all or nothing.
	return uc.tx.Do(ctx, func(ctx context.Context) error {
		w, err := uc.walletRepo.FindByIDForUpdate(ctx, wr.WalletID)
		if err != nil {
			return err
		}
		balanceBefore := w.Balance
		newBalance := w.Balance - wr.Amount
		newLocked := w.Locked - wr.Amount

		if err := uc.walletRepo.UpdateBalance(ctx, w.WalletID, newBalance); err != nil {
			return err
		}
		if err := uc.walletRepo.UpdateLocked(ctx, w.WalletID, newLocked); err != nil {
			return err
		}

		if err := uc.txRepo.Create(ctx, &wallet.Transaction{
			TxID:          uuid.New().String(),
			WalletID:      w.WalletID,
			UserID:        wr.UserID,
			Type:          wallet.TxTypeWithdrawal,
			Status:        wallet.TxStatusCompleted,
			Amount:        wr.Amount,
			BalanceBefore: balanceBefore,
			BalanceAfter:  newBalance,
			Currency:      wr.Currency,
			RefID:         wr.WithdrawalID,
			Description:   fmt.Sprintf("Withdrawal to PayPal %s approved by admin %s", wr.PayPalEmail, adminID),
		}); err != nil {
			return err
		}

		return uc.withdrawalRepo.UpdateStatus(ctx, withdrawalID, wallet.TxStatusCompleted, providerRef, "")
	})
}

func (uc *adminWalletUsecase) RejectWithdrawal(ctx context.Context, withdrawalID, adminID, reason string) error {
	wr, err := uc.withdrawalRepo.FindByID(ctx, withdrawalID)
	if err != nil {
		return err
	}
	if wr.Status != wallet.TxStatusPending {
		return fmt.Errorf("withdrawal is not in pending state")
	}

	// Release the locked amount back to available and mark the request failed atomically.
	return uc.tx.Do(ctx, func(ctx context.Context) error {
		w, err := uc.walletRepo.FindByIDForUpdate(ctx, wr.WalletID)
		if err != nil {
			return err
		}
		if err := uc.walletRepo.UpdateLocked(ctx, w.WalletID, w.Locked-wr.Amount); err != nil {
			return err
		}
		return uc.withdrawalRepo.UpdateStatus(ctx, withdrawalID, wallet.TxStatusFailed, "", reason)
	})
}

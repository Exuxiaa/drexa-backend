package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"drexa/internal/wallet"
)

type walletUsecase struct {
	walletRepo     wallet.WalletRepository
	txRepo         wallet.TransactionRepository
	depositRepo    wallet.DepositRepository
	withdrawalRepo wallet.WithdrawalRepository
	paymentSvc     wallet.PaymentService
	tx             wallet.TxManager
}

func NewWalletUsecase(
	walletRepo wallet.WalletRepository,
	txRepo wallet.TransactionRepository,
	depositRepo wallet.DepositRepository,
	withdrawalRepo wallet.WithdrawalRepository,
	paymentSvc wallet.PaymentService,
	tx wallet.TxManager,
) wallet.WalletUsecase {
	return &walletUsecase{
		walletRepo:     walletRepo,
		txRepo:         txRepo,
		depositRepo:    depositRepo,
		withdrawalRepo: withdrawalRepo,
		paymentSvc:     paymentSvc,
		tx:             tx,
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

// InitiateWithdrawal deducts balance, locks it, and queues a withdrawal for admin approval.
// Actual disbursement happens in AdminWalletUsecase.ApproveWithdrawal.
func (uc *walletUsecase) InitiateWithdrawal(ctx context.Context, userID string, req *wallet.InitiateWithdrawalRequest) (*wallet.WithdrawalRequest, error) {
	if req.Amount <= 0 {
		return nil, wallet.ErrInvalidAmount
	}
	if req.Amount < minWithdrawalFor(req.Currency) {
		return nil, wallet.ErrWithdrawalAmountMin
	}

	// Resolve the wallet id outside the transaction; the balance decision happens under lock below.
	w, err := uc.walletRepo.FindByUserAndCurrency(ctx, userID, req.Currency)
	if err != nil {
		return nil, err
	}

	withdrawalReq := &wallet.WithdrawalRequest{
		WithdrawalID:  uuid.New().String(),
		UserID:        userID,
		WalletID:      w.WalletID,
		Amount:        req.Amount,
		Currency:      req.Currency,
		BankCode:      req.BankCode,
		AccountNumber: req.AccountNumber, // TODO: encrypt before storing
		AccountName:   req.AccountName,
		Status:        wallet.TxStatusPending,
	}

	// Lock the row, re-check the guard and balance against fresh values, then reserve the funds
	// and create the request atomically. A failed Create rolls the lock back automatically.
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

// ─── Admin Usecase ───────────────────────────────────────────────────────────

type adminWalletUsecase struct {
	walletRepo     wallet.WalletRepository
	txRepo         wallet.TransactionRepository
	withdrawalRepo wallet.WithdrawalRepository
	paymentSvc     wallet.PaymentService
	tx             wallet.TxManager
}

func NewAdminWalletUsecase(
	walletRepo wallet.WalletRepository,
	txRepo wallet.TransactionRepository,
	withdrawalRepo wallet.WithdrawalRepository,
	paymentSvc wallet.PaymentService,
	tx wallet.TxManager,
) wallet.AdminWalletUsecase {
	return &adminWalletUsecase{
		walletRepo:     walletRepo,
		txRepo:         txRepo,
		withdrawalRepo: withdrawalRepo,
		paymentSvc:     paymentSvc,
		tx:             tx,
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
	// Fetch all pending withdrawals — using page 1 with large limit for admin queue
	return uc.withdrawalRepo.FindByUserID(ctx, "", 200, 0)
}

func (uc *adminWalletUsecase) ApproveWithdrawal(ctx context.Context, withdrawalID, adminID string) error {
	wr, err := uc.withdrawalRepo.FindByID(ctx, withdrawalID)
	if err != nil {
		return err
	}
	if wr.Status != wallet.TxStatusPending {
		return fmt.Errorf("withdrawal is not in pending state")
	}

	// Disburse via payment provider. This is an external, non-reversible side-effect, so it stays
	// outside the DB transaction — we don't want to hold a row lock across a network call.
	providerRef, err := uc.paymentSvc.CreateDisbursement(ctx, &wallet.DisbursementRequest{
		WithdrawalID:  wr.WithdrawalID,
		Amount:        wr.Amount,
		Currency:      wr.Currency,
		BankCode:      wr.BankCode,
		AccountNumber: wr.AccountNumber,
		AccountName:   wr.AccountName,
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
			Description:   fmt.Sprintf("Withdrawal to %s %s approved by admin %s", wr.BankCode, wr.AccountNumber, adminID),
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

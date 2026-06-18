package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"drexa/internal/wallet"
)

// ─── WalletRepository ────────────────────────────────────────────────────────

type walletRepository struct {
	db *gorm.DB
}

func NewWalletRepository(db *gorm.DB) wallet.WalletRepository {
	return &walletRepository{db: db}
}

func (r *walletRepository) Create(ctx context.Context, w *wallet.Wallet) error {
	return dbFromContext(ctx, r.db).Create(w).Error
}

func (r *walletRepository) Update(ctx context.Context, w *wallet.Wallet) error {
	return dbFromContext(ctx, r.db).Save(w).Error
}

func (r *walletRepository) UpdateBalance(ctx context.Context, walletID string, newBalance int64) error {
	return dbFromContext(ctx, r.db).
		Model(&wallet.Wallet{}).
		Where("wallet_id = ?", walletID).
		Update("balance", newBalance).Error
}

func (r *walletRepository) UpdateLocked(ctx context.Context, walletID string, newLocked int64) error {
	return dbFromContext(ctx, r.db).
		Model(&wallet.Wallet{}).
		Where("wallet_id = ?", walletID).
		Update("locked", newLocked).Error
}

func (r *walletRepository) UpdateStatus(ctx context.Context, walletID string, status wallet.WalletStatus) error {
	return dbFromContext(ctx, r.db).
		Model(&wallet.Wallet{}).
		Where("wallet_id = ?", walletID).
		Update("status", status).Error
}

func (r *walletRepository) FindByID(ctx context.Context, walletID string) (*wallet.Wallet, error) {
	var w wallet.Wallet
	err := dbFromContext(ctx, r.db).Where("wallet_id = ?", walletID).First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, wallet.ErrWalletNotFound
	}
	return &w, err
}

func (r *walletRepository) FindByIDForUpdate(ctx context.Context, walletID string) (*wallet.Wallet, error) {
	var w wallet.Wallet
	err := dbFromContext(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("wallet_id = ?", walletID).
		First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, wallet.ErrWalletNotFound
	}
	return &w, err
}

func (r *walletRepository) FindByUserID(ctx context.Context, userID string) ([]wallet.Wallet, error) {
	var wallets []wallet.Wallet
	err := dbFromContext(ctx, r.db).Where("user_id = ?", userID).Find(&wallets).Error
	return wallets, err
}

func (r *walletRepository) FindByUserAndCurrency(ctx context.Context, userID string, currency wallet.CurrencyCode) (*wallet.Wallet, error) {
	var w wallet.Wallet
	err := dbFromContext(ctx, r.db).
		Where("user_id = ? AND currency = ?", userID, currency).
		First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, wallet.ErrWalletNotFound
	}
	return &w, err
}

// ─── TransactionRepository ───────────────────────────────────────────────────

type transactionRepository struct {
	db *gorm.DB
}

func NewTransactionRepository(db *gorm.DB) wallet.TransactionRepository {
	return &transactionRepository{db: db}
}

func (r *transactionRepository) Create(ctx context.Context, tx *wallet.Transaction) error {
	return dbFromContext(ctx, r.db).Create(tx).Error
}

func (r *transactionRepository) FindByID(ctx context.Context, txID string) (*wallet.Transaction, error) {
	var tx wallet.Transaction
	err := dbFromContext(ctx, r.db).Where("tx_id = ?", txID).First(&tx).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, wallet.ErrTransactionNotFound
	}
	return &tx, err
}

func (r *transactionRepository) FindByWalletID(ctx context.Context, walletID string, limit, offset int) ([]wallet.Transaction, error) {
	var txs []wallet.Transaction
	err := dbFromContext(ctx, r.db).
		Where("wallet_id = ?", walletID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&txs).Error
	return txs, err
}

func (r *transactionRepository) FindByUserID(ctx context.Context, userID string, limit, offset int) ([]wallet.Transaction, error) {
	var txs []wallet.Transaction
	err := dbFromContext(ctx, r.db).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&txs).Error
	return txs, err
}

func (r *transactionRepository) FindByRefID(ctx context.Context, refID string) (*wallet.Transaction, error) {
	var tx wallet.Transaction
	err := dbFromContext(ctx, r.db).Where("ref_id = ?", refID).First(&tx).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, wallet.ErrTransactionNotFound
	}
	return &tx, err
}

// ─── DepositRepository ───────────────────────────────────────────────────────

type depositRepository struct {
	db *gorm.DB
}

func NewDepositRepository(db *gorm.DB) wallet.DepositRepository {
	return &depositRepository{db: db}
}

func (r *depositRepository) Create(ctx context.Context, req *wallet.DepositRequest) error {
	return dbFromContext(ctx, r.db).Create(req).Error
}

func (r *depositRepository) UpdateStatus(ctx context.Context, depositID string, status wallet.TransactionStatus, confirmedAt *time.Time) error {
	updates := map[string]any{"status": status}
	if confirmedAt != nil {
		updates["confirmed_at"] = confirmedAt
	}
	return dbFromContext(ctx, r.db).
		Model(&wallet.DepositRequest{}).
		Where("deposit_id = ?", depositID).
		Updates(updates).Error
}

func (r *depositRepository) FindByID(ctx context.Context, depositID string) (*wallet.DepositRequest, error) {
	var req wallet.DepositRequest
	err := dbFromContext(ctx, r.db).Where("deposit_id = ?", depositID).First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, wallet.ErrDepositNotFound
	}
	return &req, err
}

func (r *depositRepository) FindByProviderRef(ctx context.Context, providerRef string) (*wallet.DepositRequest, error) {
	var req wallet.DepositRequest
	err := dbFromContext(ctx, r.db).Where("provider_ref = ?", providerRef).First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, wallet.ErrDepositNotFound
	}
	return &req, err
}

func (r *depositRepository) FindByUserID(ctx context.Context, userID string, limit, offset int) ([]wallet.DepositRequest, error) {
	var reqs []wallet.DepositRequest
	err := dbFromContext(ctx, r.db).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&reqs).Error
	return reqs, err
}

// ─── WithdrawalRepository ────────────────────────────────────────────────────

type withdrawalRepository struct {
	db *gorm.DB
}

func NewWithdrawalRepository(db *gorm.DB) wallet.WithdrawalRepository {
	return &withdrawalRepository{db: db}
}

func (r *withdrawalRepository) Create(ctx context.Context, req *wallet.WithdrawalRequest) error {
	return dbFromContext(ctx, r.db).Create(req).Error
}

func (r *withdrawalRepository) UpdateStatus(ctx context.Context, withdrawalID string, status wallet.TransactionStatus, providerRef, rejectionReason string) error {
	return dbFromContext(ctx, r.db).
		Model(&wallet.WithdrawalRequest{}).
		Where("withdrawal_id = ?", withdrawalID).
		Updates(map[string]any{
			"status":           status,
			"provider_ref":     providerRef,
			"rejection_reason": rejectionReason,
		}).Error
}

func (r *withdrawalRepository) FindByID(ctx context.Context, withdrawalID string) (*wallet.WithdrawalRequest, error) {
	var req wallet.WithdrawalRequest
	err := dbFromContext(ctx, r.db).Where("withdrawal_id = ?", withdrawalID).First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, wallet.ErrWithdrawalNotFound
	}
	return &req, err
}

func (r *withdrawalRepository) FindByUserID(ctx context.Context, userID string, limit, offset int) ([]wallet.WithdrawalRequest, error) {
	var reqs []wallet.WithdrawalRequest
	err := dbFromContext(ctx, r.db).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).Offset(offset).
		Find(&reqs).Error
	return reqs, err
}

func (r *withdrawalRepository) FindPendingByWalletID(ctx context.Context, walletID string) (*wallet.WithdrawalRequest, error) {
	var req wallet.WithdrawalRequest
	err := dbFromContext(ctx, r.db).
		Where("wallet_id = ? AND status = ?", walletID, wallet.TxStatusPending).
		First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil // nil means no pending withdrawal — not an error
	}
	return &req, err
}

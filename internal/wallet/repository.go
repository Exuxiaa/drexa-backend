package wallet

import (
	"context"
	"time"
)

// WalletRepository handles persistence for Wallet entities
type WalletRepository interface {
	// Write
	Create(ctx context.Context, wallet *Wallet) error
	Update(ctx context.Context, wallet *Wallet) error

	// Targeted balance updates — use explicit column updates to avoid GORM zero-value pitfalls
	// These must be executed inside a DB transaction in the usecase layer
	UpdateBalance(ctx context.Context, walletID string, newBalance int64) error
	UpdateLocked(ctx context.Context, walletID string, newLocked int64) error
	UpdateStatus(ctx context.Context, walletID string, status WalletStatus) error

	// Read
	FindByID(ctx context.Context, walletID string) (*Wallet, error)
	FindByUserID(ctx context.Context, userID string) ([]Wallet, error)
	FindByUserAndCurrency(ctx context.Context, userID string, currency CurrencyCode) (*Wallet, error)

	// FindByIDForUpdate reads a wallet row with a pessimistic write lock (SELECT ... FOR UPDATE).
	// It must be called inside a TxManager.Do block; concurrent callers block until the
	// surrounding transaction commits, which serializes balance mutations on the same row.
	FindByIDForUpdate(ctx context.Context, walletID string) (*Wallet, error)
}

// TxManager runs a function inside a single database transaction. Repository methods invoked
// with the context passed to fn execute against that transaction, so a failure anywhere rolls
// back every write. Nested Do calls reuse the outer transaction.
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// TransactionRepository handles persistence for transaction records (append-only — no updates)
type TransactionRepository interface {
	// Write — transactions are immutable once written; never update, only create or reverse
	Create(ctx context.Context, tx *Transaction) error

	// Read
	FindByID(ctx context.Context, txID string) (*Transaction, error)
	FindByWalletID(ctx context.Context, walletID string, limit, offset int) ([]Transaction, error)
	FindByUserID(ctx context.Context, userID string, limit, offset int) ([]Transaction, error)
	FindByRefID(ctx context.Context, refID string) (*Transaction, error)
}

// DepositRepository handles persistence for deposit requests
type DepositRepository interface {
	Create(ctx context.Context, req *DepositRequest) error
	UpdateStatus(ctx context.Context, depositID string, status TransactionStatus, confirmedAt *time.Time) error

	FindByID(ctx context.Context, depositID string) (*DepositRequest, error)
	FindByProviderRef(ctx context.Context, providerRef string) (*DepositRequest, error)
	FindByUserID(ctx context.Context, userID string, limit, offset int) ([]DepositRequest, error)
}

// CryptoAddressRepository handles persistence for derived on-chain deposit addresses
type CryptoAddressRepository interface {
	Create(ctx context.Context, addr *CryptoAddress) error
	FindByUserAndCurrency(ctx context.Context, userID string, currency CurrencyCode) (*CryptoAddress, error)
}

// WithdrawalRepository handles persistence for withdrawal requests
type WithdrawalRepository interface {
	Create(ctx context.Context, req *WithdrawalRequest) error
	UpdateStatus(ctx context.Context, withdrawalID string, status TransactionStatus, providerRef, rejectionReason string) error

	FindByID(ctx context.Context, withdrawalID string) (*WithdrawalRequest, error)
	FindByUserID(ctx context.Context, userID string, limit, offset int) ([]WithdrawalRequest, error)
	FindPendingByWalletID(ctx context.Context, walletID string) (*WithdrawalRequest, error)
}

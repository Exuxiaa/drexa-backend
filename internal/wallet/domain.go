package wallet

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ─── Enums ───────────────────────────────────────────────────────────────────

// CurrencyCode is a defined type to prevent arbitrary string values
type CurrencyCode string

const (
	CurrencyIDR  CurrencyCode = "IDR"
	CurrencyUSD  CurrencyCode = "USD"
	CurrencyBTC  CurrencyCode = "BTC"
	CurrencyETH  CurrencyCode = "ETH"
	CurrencySOL  CurrencyCode = "SOL"
	CurrencyUSDT CurrencyCode = "USDT"
	CurrencyBNB  CurrencyCode = "BNB"
)

// WalletStatus is a defined type to control wallet lifecycle
type WalletStatus string

const (
	WalletStatusActive    WalletStatus = "active"
	WalletStatusSuspended WalletStatus = "suspended"
	WalletStatusClosed    WalletStatus = "closed"
)

// TransactionType describes the direction and nature of a balance movement
type TransactionType string

const (
	TxTypeDeposit    TransactionType = "deposit"
	TxTypeWithdrawal TransactionType = "withdrawal"
	TxTypeTransfer   TransactionType = "transfer"
	TxTypeFee        TransactionType = "fee"
	TxTypeReversal   TransactionType = "reversal"
	TxTypeTrade      TransactionType = "trade"
)

// TransactionStatus tracks the lifecycle of a transaction
type TransactionStatus string

const (
	TxStatusPending   TransactionStatus = "pending"
	TxStatusCompleted TransactionStatus = "completed"
	TxStatusFailed    TransactionStatus = "failed"
	TxStatusReversed  TransactionStatus = "reversed"
)

// ─── Entities ────────────────────────────────────────────────────────────────

// Wallet represents a user's balance account for a specific currency.
// Balance is stored in the smallest unit (cents for IDR, satoshi for BTC).
type Wallet struct {
	WalletID   string         `gorm:"primaryKey;column:wallet_id"`
	UserID     string         `gorm:"column:user_id;index"`     // FK to users
	Currency   CurrencyCode   `gorm:"column:currency;index"`    // e.g. "IDR", "BTC"
	Balance    int64          `gorm:"column:balance;default:0"` // stored in smallest unit (cents/satoshi)
	Locked     int64          `gorm:"column:locked;default:0"`  // amount reserved for open orders
	Status     WalletStatus   `gorm:"column:status;default:active"`
	CreatedAt  time.Time      `gorm:"column:created_at;autoCreateTime"`
	ModifiedAt time.Time      `gorm:"column:modified_at;autoUpdateTime"`
	DeletedAt  gorm.DeletedAt `gorm:"column:deleted_at;index"` // soft delete — OJK audit trail
}

// Available returns the spendable balance (balance minus locked amount)
func (w *Wallet) Available() int64 {
	return w.Balance - w.Locked
}

// Transaction records every balance movement for full audit trail (OJK requirement)
type Transaction struct {
	TxID          string            `gorm:"primaryKey;column:tx_id"`
	WalletID      string            `gorm:"column:wallet_id;index"` // FK to wallets
	UserID        string            `gorm:"column:user_id;index"`   // denormalized for faster user history queries
	Type          TransactionType   `gorm:"column:type"`
	Status        TransactionStatus `gorm:"column:status;default:pending"`
	Amount        int64             `gorm:"column:amount"` // always positive; direction inferred from Type
	BalanceBefore int64             `gorm:"column:balance_before"`
	BalanceAfter  int64             `gorm:"column:balance_after"`
	Currency      CurrencyCode      `gorm:"column:currency"`
	RefID         string            `gorm:"column:ref_id"`             // external reference: payment ID, order ID, etc.
	Description   string            `gorm:"column:description"`
	Metadata      string            `gorm:"column:metadata;type:text"` // JSON blob for provider-specific data
	CreatedAt     time.Time         `gorm:"column:created_at;autoCreateTime"`
}

// DepositRequest tracks a pending fiat deposit before it is confirmed by the payment provider
type DepositRequest struct {
	DepositID   string            `gorm:"primaryKey;column:deposit_id"`
	UserID      string            `gorm:"column:user_id;index"`
	WalletID    string            `gorm:"column:wallet_id;index"`
	Amount      int64             `gorm:"column:amount"`
	Currency    CurrencyCode      `gorm:"column:currency"`
	Provider    string            `gorm:"column:provider"`                 // "xendit", "midtrans", etc.
	ProviderRef string            `gorm:"column:provider_ref;uniqueIndex"` // provider's payment/session ID
	Status      TransactionStatus `gorm:"column:status;default:pending"`
	ExpiresAt   time.Time         `gorm:"column:expires_at"`
	ConfirmedAt *time.Time        `gorm:"column:confirmed_at"`
	CreatedAt   time.Time         `gorm:"column:created_at;autoCreateTime"`
	ModifiedAt  time.Time         `gorm:"column:modified_at;autoUpdateTime"`
}

// WithdrawalRequest tracks a pending fiat withdrawal pending compliance review
type WithdrawalRequest struct {
	WithdrawalID    string            `gorm:"primaryKey;column:withdrawal_id"`
	UserID          string            `gorm:"column:user_id;index"`
	WalletID        string            `gorm:"column:wallet_id;index"`
	Amount          int64             `gorm:"column:amount"`
	Currency        CurrencyCode      `gorm:"column:currency"`
	PayPalEmail     string            `gorm:"column:paypal_email"`   // recipient PayPal account for the payout
	BankCode        string            `gorm:"column:bank_code"`      // legacy/unused for PayPal payouts
	AccountNumber   string            `gorm:"column:account_number"` // legacy/unused for PayPal payouts
	AccountName     string            `gorm:"column:account_name"`   // legacy/unused for PayPal payouts
	Status          TransactionStatus `gorm:"column:status;default:pending"`
	ProviderRef     string            `gorm:"column:provider_ref"` // disbursement ID from provider
	RejectionReason string            `gorm:"column:rejection_reason"`
	ProcessedAt     *time.Time        `gorm:"column:processed_at"`
	CreatedAt       time.Time         `gorm:"column:created_at;autoCreateTime"`
	ModifiedAt      time.Time         `gorm:"column:modified_at;autoUpdateTime"`
}

// CryptoAddress is a user's on-chain deposit address for a given currency/chain,
// derived from an HD wallet generated via the crypto provider (Tatum).
type CryptoAddress struct {
	ID              string       `gorm:"primaryKey;column:id;size:36"`
	UserID          string       `gorm:"column:user_id;size:36;uniqueIndex:idx_crypto_user_currency"`
	Currency        CurrencyCode `gorm:"column:currency;size:16;uniqueIndex:idx_crypto_user_currency"`
	Chain           string       `gorm:"column:chain;size:32"`    // e.g. "bitcoin", "ethereum"
	Address         string       `gorm:"column:address;size:128;index"`
	Xpub            string       `gorm:"column:xpub;size:255"`    // extended public key (deposit-only; cannot spend)
	DerivationIndex int       `gorm:"column:derivation_index"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
}

// ─── Sharedwallet DTOs ────────────────────────────────────────────────────────────

type WebhookPayload struct {
	Address       string `json:"address"`
	Amount        string `json:"amount"` // decimal string
	Asset         string `json:"asset"`
	BlockNumber   int    `json:"blockNumber"`
	Confirmations int    `json:"confirmations"`
	TxId          string `json:"txId"`
	Type          string `json:"type"`
}

type InternalTransferRequest struct {
	FromUserID string       `json:"from_user_id"`
	ToUserID   string       `json:"to_user_id"`
	Currency   CurrencyCode `json:"currency"`
	Amount     int64        `json:"amount"` // in smallest unit
}

type InitiateCryptoWithdrawalRequest struct {
	Amount    int64        `json:"amount"` // in smallest unit
	Currency  CurrencyCode `json:"currency"`
	ToAddress string       `json:"to_address"`
}

// ─── Domain Errors ───────────────────────────────────────────────────────────

var (
	// Wallet
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletSuspended     = errors.New("wallet is suspended")
	ErrWalletClosed        = errors.New("wallet is closed")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrInvalidAmount       = errors.New("amount must be greater than zero")
	ErrCurrencyMismatch    = errors.New("currency mismatch between wallets")

	// Deposit
	ErrDepositNotFound    = errors.New("deposit request not found")
	ErrDepositExpired     = errors.New("deposit request has expired")
	ErrDepositAlreadyDone = errors.New("deposit already confirmed or failed")

	// Withdrawal
	ErrWithdrawalNotFound  = errors.New("withdrawal request not found")
	ErrWithdrawalPending   = errors.New("a withdrawal is already pending for this wallet")
	ErrWithdrawalAmountMin = errors.New("withdrawal amount is below the minimum")
	ErrRecipientRequired   = errors.New("a recipient PayPal email is required for withdrawal")

	// Transaction
	ErrTransactionNotFound = errors.New("transaction not found")

	// Crypto
	ErrUnsupportedCurrency    = errors.New("currency is not supported for on-chain operations")
	ErrCryptoAddressNotFound  = errors.New("crypto address not found")
)

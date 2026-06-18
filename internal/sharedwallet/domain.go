package sharedwallet

import (
	"errors"
	"time"
)

// ─── Entities ────────────────────────────────────────────────────────────────

type Wallet struct {
	UserID          string    `firestore:"user_id"`
	Currency        string    `firestore:"currency"`
	DepositAddress  string    `firestore:"deposit_address"`
	DerivationIndex int       `firestore:"derivation_index"`
	Balance         string    `firestore:"balance"`
	LockedBalance   string    `firestore:"locked_balance"`
	Network         string    `firestore:"network"`
	CreatedAt       time.Time `firestore:"created_at"`
	UpdatedAt       time.Time `firestore:"updated_at"`
}

type Transaction struct {
	ID            string    `firestore:"id"`
	UserID        string    `firestore:"user_id"`
	Type          string    `firestore:"type"` // "DEPOSIT", "WITHDRAWAL", "INTERNAL"
	Currency      string    `firestore:"currency"`
	Network       string    `firestore:"network"`
	Amount        string    `firestore:"amount"`
	Fee           string    `firestore:"fee"`
	Status        string    `firestore:"status"` // "PENDING", "CONFIRMED", "FAILED"
	FromAddress   string    `firestore:"from_address"`
	ToAddress     string    `firestore:"to_address"`
	TxHash        string    `firestore:"tx_hash"`
	TatumRefID    string    `firestore:"tatum_ref_id"`
	Confirmations int       `firestore:"confirmations"`
	CreatedAt     time.Time `firestore:"created_at"`
	UpdatedAt     time.Time `firestore:"updated_at"`
}

type MasterWallet struct {
	Currency            string    `firestore:"currency"`
	Network             string    `firestore:"network"`
	Xpub                string    `firestore:"xpub"`
	LastDerivationIndex int       `firestore:"last_derivation_index"`
	CreatedAt           time.Time `firestore:"created_at"`
}

// ─── Constants ───────────────────────────────────────────────────────────────

const (
	TxTypeDeposit    = "DEPOSIT"
	TxTypeWithdrawal = "WITHDRAWAL"
	TxTypeInternal   = "INTERNAL"

	TxStatusPending   = "PENDING"
	TxStatusConfirmed = "CONFIRMED"
	TxStatusFailed    = "FAILED"
)

// ─── Errors ──────────────────────────────────────────────────────────────────

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletExists        = errors.New("wallet already exists")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrInvalidAmount       = errors.New("invalid amount")
	ErrIdempotencyKeyFound = errors.New("idempotency key already processed")
)

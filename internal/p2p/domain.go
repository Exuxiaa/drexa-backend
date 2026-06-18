package p2p

import (
	"errors"
	"time"
)

// ─── Enums ───────────────────────────────────────────────────────────────────

type AdvertisementStatus string

const (
	AdStatusActive    AdvertisementStatus = "active"
	AdStatusPaused    AdvertisementStatus = "paused"
	AdStatusCompleted AdvertisementStatus = "completed"
)

type OrderStatus string

const (
	P2POrderCreated  OrderStatus = "created"
	P2POrderPaid     OrderStatus = "paid"
	P2POrderReleased OrderStatus = "released"
	P2POrderDisputed OrderStatus = "disputed"
	P2POrderCancelled OrderStatus = "cancelled"
)

type DisputeStatus string

const (
	DisputeOpen     DisputeStatus = "open"
	DisputeResolved DisputeStatus = "resolved"
)

// ─── Entities ────────────────────────────────────────────────────────────────

// P2PAdvertisement is a seller's offer to sell crypto for IDR.
type P2PAdvertisement struct {
	AdvertisementID string              `gorm:"primaryKey;column:advertisement_id"`
	SellerID        string              `gorm:"column:seller_id;index"`
	PairID          string              `gorm:"column:pair_id"`        // FK to trading_pairs
	Price           float64             `gorm:"column:price;type:numeric(36,18)"`
	MinAmount       float64             `gorm:"column:min_amount;type:numeric(36,18)"`
	MaxAmount       float64             `gorm:"column:max_amount;type:numeric(36,18)"`
	PaymentMethod   string              `gorm:"column:payment_method"` // e.g. "BCA", "Mandiri"
	PaymentWindow   int                 `gorm:"column:payment_window"` // minutes buyer has to pay
	Status          AdvertisementStatus `gorm:"column:status;default:active"`
	CreatedAt       time.Time           `gorm:"column:created_at;autoCreateTime"`
}

// P2POrder is a buyer's order against an advertisement.
// Crypto is held in escrow until the seller confirms payment.
type P2POrder struct {
	P2POrderID      string      `gorm:"primaryKey;column:p2p_order_id"`
	AdvertisementID string      `gorm:"column:advertisement_id;index"`
	BuyerID         string      `gorm:"column:buyer_id;index"`
	SellerID        string      `gorm:"column:seller_id;index"`
	Amount          float64     `gorm:"column:amount;type:numeric(36,18)"`    // crypto amount
	TotalIDR        float64     `gorm:"column:total_idr;type:numeric(20,2)"`  // fiat equivalent
	Status          OrderStatus `gorm:"column:status;default:created"`
	PaymentProofURL *string     `gorm:"column:payment_proof_url"`
	EscrowWalletID  string      `gorm:"column:escrow_wallet_id"` // FK to wallets
	CreatedAt       time.Time   `gorm:"column:created_at;autoCreateTime"`
	PaidAt          *time.Time  `gorm:"column:paid_at"`
	ReleasedAt      *time.Time  `gorm:"column:released_at"`
	ExpiredAt       time.Time   `gorm:"column:expired_at"`
}

// P2PDispute is raised when a buyer/seller cannot resolve a P2POrder themselves.
type P2PDispute struct {
	P2PDisputeID string        `gorm:"primaryKey;column:p2p_dispute_id"`
	P2POrderID   string        `gorm:"column:p2p_order_id;index"`
	RaisedBy     string        `gorm:"column:raised_by"` // user ID
	Reason       string        `gorm:"column:reason"`
	EvidenceURL  *string       `gorm:"column:evidence_url"`
	Status       DisputeStatus `gorm:"column:status;default:open"`
	ResolvedBy   *string       `gorm:"column:resolved_by"` // admin user ID
	Resolution   string        `gorm:"column:resolution;default:''"`
	ResolvedAt   *time.Time    `gorm:"column:resolved_at"`
	CreatedAt    time.Time     `gorm:"column:created_at;autoCreateTime"`
}

// ─── Domain Errors ───────────────────────────────────────────────────────────

var (
	ErrAdvertisementNotFound = errors.New("advertisement not found")
	ErrP2POrderNotFound      = errors.New("p2p order not found")
	ErrP2POrderExpired       = errors.New("p2p order has expired")
	ErrDisputeNotFound       = errors.New("dispute not found")
	ErrEscrowNotReleased     = errors.New("escrow has not been released")
)

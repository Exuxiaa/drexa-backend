package checkout

import (
	"errors"
	"time"
)

type PurchaseStatus string

const (
	StatusPending   PurchaseStatus = "pending"
	StatusCompleted PurchaseStatus = "completed"
)

type Purchase struct {
	PurchaseID      string         `gorm:"primaryKey;column:purchase_id"`
	UserID          string         `gorm:"column:user_id"`
	StripeSessionID string         `gorm:"column:stripe_session_id"`
	Status          PurchaseStatus `gorm:"column:status;default:pending"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

var (
	ErrPurchaseNotFound = errors.New("purchase not found")
)

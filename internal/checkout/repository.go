package checkout

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type PurchaseRepository interface {
	Create(ctx context.Context, purchase *Purchase) error
	FindBySessionID(ctx context.Context, sessionID string) (*Purchase, error)
	UpdateStatus(ctx context.Context, purchaseID string, status PurchaseStatus) error
}

type purchaseRepository struct {
	db *gorm.DB
}

func NewPurchaseRepository(db *gorm.DB) PurchaseRepository {
	return &purchaseRepository{db: db}
}

func (r *purchaseRepository) Create(ctx context.Context, purchase *Purchase) error {
	if err := r.db.WithContext(ctx).Create(purchase).Error; err != nil {
		return fmt.Errorf("purchase_repo: create: %w", err)
	}
	return nil
}

func (r *purchaseRepository) FindBySessionID(ctx context.Context, sessionID string) (*Purchase, error) {
	var p Purchase
	err := r.db.WithContext(ctx).Where("stripe_session_id = ?", sessionID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPurchaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("purchase_repo: find by session id: %w", err)
	}
	return &p, nil
}

func (r *purchaseRepository) UpdateStatus(ctx context.Context, purchaseID string, status PurchaseStatus) error {
	return r.db.WithContext(ctx).Model(&Purchase{}).
		Where("purchase_id = ?", purchaseID).
		Update("status", status).Error
}

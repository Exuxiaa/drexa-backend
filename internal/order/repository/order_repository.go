package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"drexa/internal/order"
)

type orderRepository struct{ db *gorm.DB }

// New returns a GORM-backed order.Repository.
func New(db *gorm.DB) order.Repository {
	return &orderRepository{db: db}
}

func (r *orderRepository) Create(ctx context.Context, o *order.Order) error {
	if err := r.db.WithContext(ctx).Create(o).Error; err != nil {
		return fmt.Errorf("order_repo: create: %w", err)
	}
	return nil
}

func (r *orderRepository) FindByID(ctx context.Context, orderID string) (*order.Order, error) {
	var o order.Order
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, order.ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("order_repo: find by id: %w", err)
	}
	return &o, nil
}

func (r *orderRepository) FindByUserID(ctx context.Context, userID string) ([]order.Order, error) {
	var orders []order.Order
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("order_repo: find by user: %w", err)
	}
	return orders, nil
}

func (r *orderRepository) Update(ctx context.Context, o *order.Order) error {
	if err := r.db.WithContext(ctx).
		Model(&order.Order{}).
		Where("order_id = ?", o.OrderID).
		Updates(map[string]any{
			"status":          o.Status,
			"filled_quantity": o.FilledQuantity,
			"fee":             o.Fee,
		}).Error; err != nil {
		return fmt.Errorf("order_repo: update: %w", err)
	}
	return nil
}

func (r *orderRepository) SaveTrades(ctx context.Context, trades []order.Trade) error {
	if len(trades) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&trades).Error; err != nil {
		return fmt.Errorf("order_repo: save trades: %w", err)
	}
	return nil
}

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

// activeStatuses are the lifecycle states shown under "open orders".
var activeStatuses = []order.OrderStatus{
	order.StatusPending, order.StatusOpen, order.StatusPartiallyFilled, order.StatusUntriggered,
}

// closedStatuses are the terminal states shown under "order history".
var closedStatuses = []order.OrderStatus{
	order.StatusFilled, order.StatusCancelled,
}

func (r *orderRepository) FindOrders(ctx context.Context, userID string, f order.OrderFilter) ([]order.Order, error) {
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if f.PairID != "" {
		q = q.Where("pair_id = ?", f.PairID)
	}
	switch f.Status {
	case order.FilterOpen:
		q = q.Where("status IN ?", activeStatuses)
	case order.FilterClosed:
		q = q.Where("status IN ?", closedStatuses)
	}
	q = q.Order("created_at DESC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}

	var orders []order.Order
	if err := q.Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("order_repo: find orders: %w", err)
	}
	return orders, nil
}

func (r *orderRepository) FindUntriggered(ctx context.Context) ([]order.Order, error) {
	var orders []order.Order
	if err := r.db.WithContext(ctx).
		Where("status = ?", order.StatusUntriggered).
		Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("order_repo: find untriggered: %w", err)
	}
	return orders, nil
}

func (r *orderRepository) FindByOCOGroup(ctx context.Context, groupID string) ([]order.Order, error) {
	var orders []order.Order
	if err := r.db.WithContext(ctx).
		Where("oco_group_id = ?", groupID).
		Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("order_repo: find by oco group: %w", err)
	}
	return orders, nil
}

// FindTradesByUserID returns the user's fills annotated with the side and
// maker/taker role of *their* order in each trade. A user is never both maker
// and taker in one trade (self-trade prevention), so each row maps to one side.
func (r *orderRepository) FindTradesByUserID(ctx context.Context, userID string, limit int) ([]order.TradeView, error) {
	q := r.db.WithContext(ctx).
		Table("trades AS t").
		Select(`t.trade_id, t.pair_id, o.order_id,
			o.side AS side,
			CASE WHEN t.maker_order_id = o.order_id THEN 'maker' ELSE 'taker' END AS role,
			t.price, t.quantity,
			CASE WHEN t.maker_order_id = o.order_id THEN t.maker_fee ELSE t.taker_fee END AS fee,
			t.executed_at`).
		Joins("JOIN orders o ON o.order_id = t.maker_order_id OR o.order_id = t.taker_order_id").
		Where("o.user_id = ?", userID).
		Order("t.executed_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}

	var views []order.TradeView
	if err := q.Scan(&views).Error; err != nil {
		return nil, fmt.Errorf("order_repo: find trades by user: %w", err)
	}
	return views, nil
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

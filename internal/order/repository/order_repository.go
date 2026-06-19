package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func (r *orderRepository) FindByUserIDFiltered(ctx context.Context, userID, status, pairID string, limit, offset int) ([]order.Order, int64, error) {
	q := r.db.WithContext(ctx).Model(&order.Order{}).Where("user_id = ?", userID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if pairID != "" {
		q = q.Where("pair_id = ?", pairID)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("order_repo: count filtered: %w", err)
	}

	var orders []order.Order
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&orders).Error; err != nil {
		return nil, 0, fmt.Errorf("order_repo: find filtered: %w", err)
	}
	return orders, total, nil
}

// userTradeRow is a GORM scan target for the trades+orders join query.
type userTradeRow struct {
	TradeID    string          `gorm:"column:trade_id"`
	PairID     string          `gorm:"column:pair_id"`
	Price      float64         `gorm:"column:price"`
	Quantity   float64         `gorm:"column:quantity"`
	MakerFee   float64         `gorm:"column:maker_fee"`
	TakerFee   float64         `gorm:"column:taker_fee"`
	ExecutedAt time.Time       `gorm:"column:executed_at"`
	Side       order.OrderSide `gorm:"column:side"`
	Role       string          `gorm:"column:role"`
}

func (r *orderRepository) FindTradesByUserID(ctx context.Context, userID, pairID string, limit, offset int) ([]order.UserTrade, int64, error) {
	// Join trades against the user's orders. Because self-trade is prevented,
	// each trade has the user on at most one side, so the join produces no
	// duplicate rows per trade.
	base := r.db.WithContext(ctx).
		Table("trades t").
		Select(`t.trade_id, t.pair_id, t.price, t.quantity, t.maker_fee, t.taker_fee, t.executed_at,
			o.side,
			CASE WHEN t.taker_order_id = o.order_id THEN 'taker' ELSE 'maker' END AS role`).
		Joins("JOIN orders o ON o.user_id = ? AND (t.taker_order_id = o.order_id OR t.maker_order_id = o.order_id)", userID)
	if pairID != "" {
		base = base.Where("t.pair_id = ?", pairID)
	}

	// COUNT via subquery to avoid re-doing the join expression twice.
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("order_repo: count trades: %w", err)
	}

	var rows []userTradeRow
	if err := base.Order("t.executed_at DESC").Limit(limit).Offset(offset).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("order_repo: find trades: %w", err)
	}

	out := make([]order.UserTrade, len(rows))
	for i, row := range rows {
		fee := row.TakerFee
		if row.Role == "maker" {
			fee = row.MakerFee
		}
		out[i] = order.UserTrade{
			TradeID:    row.TradeID,
			PairID:     row.PairID,
			Side:       row.Side,
			Price:      row.Price,
			Quantity:   row.Quantity,
			Fee:        fee,
			Role:       row.Role,
			ExecutedAt: row.ExecutedAt,
		}
	}
	return out, total, nil
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

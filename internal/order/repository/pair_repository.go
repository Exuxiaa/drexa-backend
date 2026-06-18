package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"drexa/internal/market"
	"drexa/internal/order"
)

type pairRepository struct{ db *gorm.DB }

// NewPairService returns an order.PairService backed by the market.TradingPair
// table. It translates the market entity into the narrow order.PairInfo so the
// order domain never imports internal/market directly.
func NewPairService(db *gorm.DB) order.PairService {
	return &pairRepository{db: db}
}

func (r *pairRepository) GetPair(ctx context.Context, pairID string) (*order.PairInfo, error) {
	var p market.TradingPair
	err := r.db.WithContext(ctx).Where("pair_id = ?", pairID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, order.ErrPairNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pair_repo: get pair: %w", err)
	}

	return &order.PairInfo{
		PairID:        p.PairID,
		Active:        p.Status == market.StatusActive,
		MinOrderSize:  p.MinOrderSize,
		PriceDecimals: p.PriceDecimalPlaces,
	}, nil
}

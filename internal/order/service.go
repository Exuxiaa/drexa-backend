package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"drexa/internal/matching"
)

type service struct {
	repo    Repository
	pairs   PairService
	matcher Matcher
}

// NewService wires the order service with its persistence, the market-backed
// trading-pair lookup, and the in-memory matching engine.
func NewService(repo Repository, pairs PairService, matcher Matcher) Service {
	return &service{repo: repo, pairs: pairs, matcher: matcher}
}

// CreateOrder validates the request, persists the order in a pending state,
// then runs it through the matching engine. Resulting trades are persisted and
// every affected order's fill/status is updated.
//
// Settlement (ledger debit/credit, fee capture) is Fase 4A.3 and not done here;
// fees are recorded as zero.
func (s *service) CreateOrder(ctx context.Context, userID string, req OrderRequest) (*Order, error) {
	if req.Side != SideBuy && req.Side != SideSell {
		return nil, ErrInvalidSide
	}

	switch req.Type {
	case TypeMarket:
		if req.Price != nil {
			return nil, ErrPriceNotAllowed
		}
	case TypeLimit:
		if req.Price == nil || *req.Price <= 0 {
			return nil, ErrPriceRequired
		}
	default:
		return nil, ErrInvalidType
	}

	pair, err := s.pairs.GetPair(ctx, req.PairID)
	if err != nil {
		return nil, err
	}
	if !pair.Active {
		return nil, ErrPairSuspended
	}
	if req.Quantity < pair.MinOrderSize {
		return nil, ErrBelowMinOrderSize
	}

	o := &Order{
		OrderID:  uuid.NewString(),
		UserID:   userID,
		PairID:   req.PairID,
		Side:     req.Side,
		Type:     req.Type,
		Status:   StatusPending,
		Price:    req.Price,
		Quantity: req.Quantity,
	}

	// For limit orders the amount to lock is deterministic: buyers lock quote
	// currency (quantity × price), sellers lock base currency (quantity).
	// Market orders are priced at match time, so nothing is locked up front.
	if req.Type == TypeLimit {
		switch req.Side {
		case SideBuy:
			o.LockedAmount = req.Quantity * *req.Price
		case SideSell:
			o.LockedAmount = req.Quantity
		}
	}

	if err := s.repo.Create(ctx, o); err != nil {
		return nil, err
	}

	// Match against the live book for this pair.
	result := s.matcher.Submit(req.PairID, toEngineOrder(o, pair.PriceDecimals))

	if err := s.applyResult(ctx, o, result, pair.PriceDecimals); err != nil {
		return nil, err
	}

	return o, nil
}

// applyResult persists the fills and the new state of every order touched by a
// match: the taker (o, in memory), every maker that traded, and any maker
// canceled by self-trade prevention.
func (s *service) applyResult(ctx context.Context, taker *Order, result matching.MatchResult, priceDec int) error {
	if len(result.Trades) > 0 {
		trades := make([]Trade, 0, len(result.Trades))
		now := time.Now()
		for _, t := range result.Trades {
			trades = append(trades, Trade{
				TradeID:      uuid.NewString(),
				PairID:       taker.PairID,
				MakerOrderID: t.MakerOrderID,
				TakerOrderID: t.TakerOrderID,
				Price:        ticksToPrice(t.Price, priceDec),
				Quantity:     lotsToQty(t.Quantity),
				ExecutedAt:   now,
			})
		}
		if err := s.repo.SaveTrades(ctx, trades); err != nil {
			return err
		}
	}

	// Sum this round's fill delta per order id (in float, pair units).
	fillDelta := make(map[string]float64)
	for _, t := range result.Trades {
		qty := lotsToQty(t.Quantity)
		fillDelta[t.MakerOrderID] += qty
		fillDelta[t.TakerOrderID] += qty
	}

	// Update each resting maker that traded (skip the taker; handled below).
	for id, delta := range fillDelta {
		if id == taker.OrderID {
			continue
		}
		maker, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return err
		}
		maker.FilledQuantity += delta
		maker.Status = deriveStatus(maker, true) // a traded maker either filled out or still rests
		if err := s.repo.Update(ctx, maker); err != nil {
			return err
		}
	}

	// Makers removed by self-trade prevention are canceled.
	for _, c := range result.Canceled {
		maker, err := s.repo.FindByID(ctx, c.ID)
		if err != nil {
			return err
		}
		maker.Status = StatusCancelled
		if err := s.repo.Update(ctx, maker); err != nil {
			return err
		}
	}

	// Finally the taker itself.
	taker.FilledQuantity += fillDelta[taker.OrderID]
	taker.Status = deriveStatus(taker, result.Rested)
	return s.repo.Update(ctx, taker)
}

// CancelOrder removes a still-open order from the book and marks it cancelled.
func (s *service) CancelOrder(ctx context.Context, userID, orderID string) (*Order, error) {
	o, err := s.repo.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	// Don't disclose existence of another user's order.
	if o.UserID != userID {
		return nil, ErrOrderNotFound
	}

	switch o.Status {
	case StatusPending, StatusOpen, StatusPartiallyFilled:
		// cancellable
	default:
		return nil, ErrOrderNotCancellable
	}

	if _, err := s.matcher.Cancel(o.PairID, orderID); err != nil {
		// Not resting on the book (already fully filled/canceled in the engine).
		return nil, ErrOrderNotCancellable
	}

	o.Status = StatusCancelled
	if err := s.repo.Update(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

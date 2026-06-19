package order

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"drexa/internal/matching"
)

// maxTriggerRounds caps how many cascading stop-trigger rounds a single match
// may set off, a safety net against pathological self-feeding stop chains.
const maxTriggerRounds = 16

type service struct {
	repo    Repository
	pairs   PairService
	matcher Matcher
	onTrade TradeObserver // optional; notified after each match that prints trades
	stops   *stopBook     // dormant stop orders awaiting their trigger price
}

// NewService wires the order service with its persistence, the market-backed
// trading-pair lookup, and the in-memory matching engine. onTrade is optional
// (may be nil) and is invoked with the last executed price after every match
// that produces trades, driving the real-time market ticker.
func NewService(repo Repository, pairs PairService, matcher Matcher, onTrade TradeObserver) Service {
	return &service{
		repo:    repo,
		pairs:   pairs,
		matcher: matcher,
		onTrade: onTrade,
		stops:   newStopBook(),
	}
}

// CreateOrder validates the request and routes it by type:
//   - market/limit  → priced into the book immediately
//   - stop-limit     → parked in the trigger book until its stop price is hit
//   - oco            → a limit leg placed now + a dormant stop-limit leg; a
//     fill or trigger of either leg cancels the other
//
// Settlement (ledger debit/credit, fee capture) is Fase 4A.3 and not done here;
// fees are recorded as zero.
func (s *service) CreateOrder(ctx context.Context, userID string, req OrderRequest) (*Order, error) {
	if req.Side != SideBuy && req.Side != SideSell {
		return nil, ErrInvalidSide
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

	switch req.Type {
	case TypeMarket:
		if req.Price != nil {
			return nil, ErrPriceNotAllowed
		}
		return s.placeImmediate(ctx, userID, req, pair, nil)

	case TypeLimit:
		if req.Price == nil || *req.Price <= 0 {
			return nil, ErrPriceRequired
		}
		return s.placeImmediate(ctx, userID, req, pair, nil)

	case TypeStopLimit:
		if req.Price == nil || *req.Price <= 0 {
			return nil, ErrPriceRequired
		}
		if req.StopPrice == nil || *req.StopPrice <= 0 {
			return nil, ErrStopPriceRequired
		}
		return s.placeStop(ctx, userID, req, pair, *req.Price, *req.StopPrice, nil)

	case TypeOCO:
		if req.Price == nil || *req.Price <= 0 {
			return nil, ErrPriceRequired
		}
		if req.StopPrice == nil || *req.StopPrice <= 0 {
			return nil, ErrStopPriceRequired
		}
		return s.placeOCO(ctx, userID, req, pair)

	default:
		return nil, ErrInvalidType
	}
}

// placeImmediate persists a market/limit order, runs it through the engine, and
// applies the result. ocoGroup links it to an OCO sibling when non-nil.
func (s *service) placeImmediate(ctx context.Context, userID string, req OrderRequest, pair *PairInfo, ocoGroup *string) (*Order, error) {
	o := &Order{
		OrderID:    uuid.NewString(),
		UserID:     userID,
		PairID:     req.PairID,
		Side:       req.Side,
		Type:       req.Type,
		Status:     StatusPending,
		Price:      req.Price,
		Quantity:   req.Quantity,
		OCOGroupID: ocoGroup,
	}
	if req.Type == TypeLimit {
		o.LockedAmount = lockedFor(req.Side, req.Quantity, *req.Price)
	}

	if err := s.repo.Create(ctx, o); err != nil {
		return nil, err
	}

	result := s.matcher.Submit(req.PairID, toEngineOrder(o, pair.PriceDecimals))
	if err := s.applyResult(ctx, o, result, pair.PriceDecimals); err != nil {
		return nil, err
	}

	// A fresh print may push the price through resting stop orders.
	if len(result.Trades) > 0 {
		last := result.Trades[len(result.Trades)-1]
		s.processTriggers(ctx, req.PairID, ticksToPrice(last.Price, pair.PriceDecimals), pair.PriceDecimals)
	}
	return o, nil
}

// placeStop persists a dormant stop-limit order and parks it in the trigger
// book. limitPrice is where it will rest once triggered; stopPrice is the
// trigger. ocoGroup links it to an OCO sibling when non-nil.
func (s *service) placeStop(ctx context.Context, userID string, req OrderRequest, pair *PairInfo, limitPrice, stopPrice float64, ocoGroup *string) (*Order, error) {
	lp := limitPrice
	sp := stopPrice
	o := &Order{
		OrderID:      uuid.NewString(),
		UserID:       userID,
		PairID:       req.PairID,
		Side:         req.Side,
		Type:         TypeStopLimit,
		Status:       StatusUntriggered,
		Price:        &lp,
		StopPrice:    &sp,
		Quantity:     req.Quantity,
		LockedAmount: lockedFor(req.Side, req.Quantity, limitPrice),
		OCOGroupID:   ocoGroup,
	}
	if err := s.repo.Create(ctx, o); err != nil {
		return nil, err
	}
	s.stops.add(req.PairID, stopEntry{orderID: o.OrderID, side: req.Side, stopPrice: stopPrice})
	return o, nil
}

// placeOCO creates a take-profit limit leg and a protective stop-limit leg that
// share an oco_group_id. Both rows are persisted before the limit leg is sent
// to the engine, so an immediate fill can cancel the (already existing) stop
// sibling. Returns the limit leg.
func (s *service) placeOCO(ctx context.Context, userID string, req OrderRequest, pair *PairInfo) (*Order, error) {
	group := uuid.NewString()

	// Stop leg (dormant). Its limit price defaults to the stop price.
	stopReq := req
	stopReq.Type = TypeStopLimit
	if _, err := s.placeStop(ctx, userID, stopReq, pair, *req.StopPrice, *req.StopPrice, &group); err != nil {
		return nil, err
	}

	// Take-profit limit leg, placed now.
	limitReq := req
	limitReq.Type = TypeLimit
	limitReq.StopPrice = nil
	return s.placeImmediate(ctx, userID, limitReq, pair, &group)
}

// lockedFor returns the balance an order reserves: buyers lock quote currency
// (quantity × price), sellers lock base currency (quantity).
func lockedFor(side OrderSide, qty, price float64) float64 {
	if side == SideBuy {
		return qty * price
	}
	return qty
}

// applyResult persists the fills and the new state of every order touched by a
// match: the taker (o, in memory), every maker that traded, and any maker
// canceled by self-trade prevention. It also enforces OCO linkage and notifies
// the ticker feed.
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
	if err := s.repo.Update(ctx, taker); err != nil {
		return err
	}

	// OCO: any leg that received a fill cancels its sibling.
	for id := range fillDelta {
		o2, err := s.repo.FindByID(ctx, id)
		if err != nil {
			continue
		}
		if o2.OCOGroupID != nil {
			s.cancelOCOSibling(ctx, o2)
		}
	}

	// Notify the ticker feed of the last executed price so it can publish a
	// real-time, trade-driven ticker. Only fires when a match actually printed.
	if s.onTrade != nil && len(result.Trades) > 0 {
		last := result.Trades[len(result.Trades)-1]
		var totalQty float64
		for _, t := range result.Trades {
			totalQty += lotsToQty(t.Quantity)
		}
		s.onTrade(TradeEvent{
			PairID:   taker.PairID,
			Price:    ticksToPrice(last.Price, priceDec),
			Quantity: totalQty,
		})
	}
	return nil
}

// processTriggers fires any dormant stop orders whose stop price the last print
// has reached, cascading until the book settles or the round cap is hit.
func (s *service) processTriggers(ctx context.Context, pairID string, lastPrice float64, priceDec int) {
	for round := 0; round < maxTriggerRounds; round++ {
		ids := s.stops.triggered(pairID, lastPrice)
		if len(ids) == 0 {
			return
		}
		for _, id := range ids {
			ord, err := s.repo.FindByID(ctx, id)
			if err != nil {
				continue
			}
			if ord.Status != StatusUntriggered {
				continue // already cancelled (e.g. by its OCO sibling)
			}
			// Triggering one OCO leg cancels the other immediately.
			if ord.OCOGroupID != nil {
				s.cancelOCOSibling(ctx, ord)
			}
			ord.Status = StatusPending
			result := s.matcher.Submit(ord.PairID, toEngineOrder(ord, priceDec))
			if err := s.applyResult(ctx, ord, result, priceDec); err != nil {
				log.Ctx(ctx).Error().Err(err).Str("order_id", ord.OrderID).Msg("order: apply triggered stop failed")
				continue
			}
			if len(result.Trades) > 0 {
				last := result.Trades[len(result.Trades)-1]
				lastPrice = ticksToPrice(last.Price, priceDec)
			}
		}
	}
}

// cancelOCOSibling cancels the still-active counterpart of an OCO leg.
func (s *service) cancelOCOSibling(ctx context.Context, ord *Order) {
	if ord.OCOGroupID == nil {
		return
	}
	legs, err := s.repo.FindByOCOGroup(ctx, *ord.OCOGroupID)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("order: load oco group failed")
		return
	}
	for i := range legs {
		leg := &legs[i]
		if leg.OrderID == ord.OrderID {
			continue
		}
		switch leg.Status {
		case StatusUntriggered:
			s.stops.remove(leg.OrderID)
			leg.Status = StatusCancelled
			if err := s.repo.Update(ctx, leg); err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("order: cancel oco stop leg failed")
			}
		case StatusPending, StatusOpen, StatusPartiallyFilled:
			// A resting limit leg — pull it off the book.
			_, _ = s.matcher.Cancel(leg.PairID, leg.OrderID)
			leg.Status = StatusCancelled
			if err := s.repo.Update(ctx, leg); err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("order: cancel oco limit leg failed")
			}
		}
	}
}

// OrderBookDepth returns the live aggregated book for a pair, converting the
// engine's integer ticks/lots back into real prices using the pair's quoted
// precision.
func (s *service) OrderBookDepth(ctx context.Context, pairID string, maxLevels int) (*OrderBookSnapshot, error) {
	pair, err := s.pairs.GetPair(ctx, pairID)
	if err != nil {
		return nil, err
	}

	depth := s.matcher.Depth(pairID, maxLevels)
	snap := &OrderBookSnapshot{
		PairID:  pairID,
		Version: depth.Version,
		Bids:    toBookLevels(depth.Bids, pair.PriceDecimals),
		Asks:    toBookLevels(depth.Asks, pair.PriceDecimals),
	}
	return snap, nil
}

// toBookLevels converts engine tick/lot levels into float price levels.
func toBookLevels(levels []matching.DepthLevel, priceDec int) []OrderBookLevel {
	out := make([]OrderBookLevel, len(levels))
	for i, l := range levels {
		out[i] = OrderBookLevel{
			Price:    ticksToPrice(l.Price, priceDec),
			Quantity: lotsToQty(l.Volume),
		}
	}
	return out
}

// CancelOrder removes a still-open order from the book (or the trigger book for
// a dormant stop) and marks it cancelled.
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
	case StatusUntriggered:
		// Dormant stop order: only lives in the in-memory trigger book.
		s.stops.remove(orderID)
	case StatusPending, StatusOpen, StatusPartiallyFilled:
		if _, err := s.matcher.Cancel(o.PairID, orderID); err != nil {
			// Not resting on the book (already fully filled/canceled in the engine).
			return nil, ErrOrderNotCancellable
		}
	default:
		return nil, ErrOrderNotCancellable
	}

	o.Status = StatusCancelled
	if err := s.repo.Update(ctx, o); err != nil {
		return nil, err
	}

	// Cancelling one OCO leg cancels the other.
	if o.OCOGroupID != nil {
		s.cancelOCOSibling(ctx, o)
	}
	return o, nil
}

// ListOrders returns the caller's orders, newest first, narrowed by filter.
func (s *service) ListOrders(ctx context.Context, userID string, f OrderFilter) ([]Order, error) {
	return s.repo.FindOrders(ctx, userID, f)
}

// ListTrades returns the caller's executed fills, newest first.
func (s *service) ListTrades(ctx context.Context, userID string, limit int) ([]TradeView, error) {
	return s.repo.FindTradesByUserID(ctx, userID, limit)
}

// RestoreStops rehydrates the in-memory trigger book from dormant orders so
// stop/OCO orders survive a restart.
func (s *service) RestoreStops(ctx context.Context) error {
	dormant, err := s.repo.FindUntriggered(ctx)
	if err != nil {
		return err
	}
	for _, o := range dormant {
		if o.StopPrice == nil {
			continue
		}
		s.stops.add(o.PairID, stopEntry{orderID: o.OrderID, side: o.Side, stopPrice: *o.StopPrice})
	}
	if len(dormant) > 0 {
		log.Ctx(ctx).Info().Int("count", len(dormant)).Msg("order: restored dormant stop orders")
	}
	return nil
}

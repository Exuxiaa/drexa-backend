package order

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"drexa/internal/matching"
)

type service struct {
	repo       Repository
	pairs      PairService
	matcher    Matcher
	walletSvc  WalletService
}

// NewService wires the order service with its persistence, the market-backed
// trading-pair lookup, the in-memory matching engine, and wallet balance checks.
func NewService(repo Repository, pairs PairService, matcher Matcher, walletSvc WalletService) Service {
	return &service{repo: repo, pairs: pairs, matcher: matcher, walletSvc: walletSvc}
}

// smallestUnitFactor returns how many of the currency's base units equal one
// whole unit. Used to convert float order amounts into the int64 stored by wallets.
func smallestUnitFactor(currency string) int64 {
	switch currency {
	case "BTC":
		return 100_000_000
	case "ETH", "BNB":
		return 1_000_000_000_000_000_000
	case "SOL":
		return 1_000_000_000
	case "USDT":
		return 1_000_000
	default: // USD, IDR
		return 100
	}
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

	// Validate that the user holds enough balance before persisting.
	// Market buy orders are priced at match time so we skip the exact check for them,
	// but limit buys, limit sells, and market sells have known required balances.
	if req.Type == TypeLimit || req.Side == SideSell {
		var checkCurrency string
		var checkAmount float64
		switch req.Side {
		case SideBuy:
			// Must be a limit buy (market buys skipped by condition)
			checkCurrency = pair.QuoteCoin
			checkAmount = req.Quantity * *req.Price
		case SideSell:
			// Market or Limit sell: needed amount is exactly the quantity
			checkCurrency = pair.BaseCoin
			checkAmount = req.Quantity
		}
		available, err := s.walletSvc.AvailableBalance(ctx, userID, checkCurrency)
		if err != nil {
			return nil, fmt.Errorf("balance check: %w", err)
		}
		needed := int64(checkAmount * float64(smallestUnitFactor(checkCurrency)))
		if available < needed {
			return nil, ErrInsufficientBalance
		}
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

	var lockCurrency string
	var lockAmount int64
	var didLock bool
	if req.Type == TypeLimit {
		switch req.Side {
		case SideBuy:
			lockCurrency = pair.QuoteCoin
		case SideSell:
			lockCurrency = pair.BaseCoin
		}
		lockAmount = int64(o.LockedAmount * float64(smallestUnitFactor(lockCurrency)))
		if err := s.walletSvc.LockBalance(ctx, userID, lockCurrency, lockAmount); err != nil {
			return nil, err
		}
		didLock = true
	}

	if err := s.repo.Create(ctx, o); err != nil {
		if didLock {
			_ = s.walletSvc.UnlockBalance(context.Background(), userID, lockCurrency, lockAmount)
		}
		return nil, err
	}

	// Match against the live book for this pair.
	result := s.matcher.Submit(req.PairID, toEngineOrder(o, pair.PriceDecimals))

	if err := s.applyResult(ctx, o, result, pair); err != nil {
		return nil, err
	}

	return o, nil
}

// applyResult persists the fills and the new state of every order touched by a
// match: the taker (o, in memory), every maker that traded, and any maker
// canceled by self-trade prevention.
func (s *service) applyResult(ctx context.Context, taker *Order, result matching.MatchResult, pair *PairInfo) error {
	if len(result.Trades) > 0 {
		trades := make([]Trade, 0, len(result.Trades))
		now := time.Now()
		for _, t := range result.Trades {
			price := ticksToPrice(t.Price, pair.PriceDecimals)
			quantity := lotsToQty(t.Quantity)
			tradeID := uuid.NewString()

			trades = append(trades, Trade{
				TradeID:      tradeID,
				PairID:       taker.PairID,
				MakerOrderID: t.MakerOrderID,
				TakerOrderID: t.TakerOrderID,
				Price:        price,
				Quantity:     quantity,
				ExecutedAt:   now,
			})

			// Settle the trade by updating wallets
			maker, err := s.repo.FindByID(ctx, t.MakerOrderID)
			if err != nil {
				return err
			}

			// Determine exactly what amounts were exchanged
			// Base is quantity, Quote is quantity * price
			var makerSpentCur, makerRecCur string
			var makerSpentAmt, makerRecAmt int64
			var takerSpentCur, takerRecCur string
			var takerSpentAmt, takerRecAmt int64

			// Maker is always a limit order resting on the book.
			if maker.Side == SideSell {
				// Maker sold Base, received Quote
				makerSpentCur = pair.BaseCoin
				makerSpentAmt = int64(quantity * float64(smallestUnitFactor(makerSpentCur)))
				makerRecCur = pair.QuoteCoin
				makerRecAmt = int64((quantity * price) * float64(smallestUnitFactor(makerRecCur)))

				// Taker bought Base, spent Quote
				takerSpentCur = pair.QuoteCoin
				takerSpentAmt = makerRecAmt
				takerRecCur = pair.BaseCoin
				takerRecAmt = makerSpentAmt
			} else {
				// Maker bought Base, spent Quote
				makerSpentCur = pair.QuoteCoin
				makerSpentAmt = int64((quantity * price) * float64(smallestUnitFactor(makerSpentCur)))
				makerRecCur = pair.BaseCoin
				makerRecAmt = int64(quantity * float64(smallestUnitFactor(makerRecCur)))

				// Taker sold Base, received Quote
				takerSpentCur = pair.BaseCoin
				takerSpentAmt = makerRecAmt
				takerRecCur = pair.QuoteCoin
				takerRecAmt = makerSpentAmt
			}

			takerUnlock := taker.Type == TypeLimit

			if err := s.walletSvc.SettleTrade(ctx, tradeID,
				maker.UserID, makerSpentCur, makerSpentAmt, makerRecCur, makerRecAmt,
				taker.UserID, takerSpentCur, takerSpentAmt, takerRecCur, takerRecAmt, takerUnlock); err != nil {
				return err
			}
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
		
		// Unlock remaining locked balance
		if maker.Type == TypeLimit && maker.LockedAmount > 0 {
			var unlockCurrency string
			switch maker.Side {
			case SideBuy:
				unlockCurrency = pair.QuoteCoin
			case SideSell:
				unlockCurrency = pair.BaseCoin
			}
			if unlockCurrency != "" {
				// remaining locked = initial locked - filled
				// wait, LockedAmount doesn't decrease on fill currently in our logic?
				// Ah, applyResult's SettleTrade takes from 'locked' directly! 
				// The wallet `locked` value was subtracted in `SettleTrade` automatically because `unlock=true`.
				// So the remaining wallet `locked` is exactly what's left.
				// We just need to calculate the remaining amount for THIS order to unlock.
				var remaining float64
				if maker.Side == SideBuy {
					remaining = (maker.Quantity - maker.FilledQuantity) * *maker.Price
				} else {
					remaining = maker.Quantity - maker.FilledQuantity
				}
				if remaining > 0 {
					unlockAmt := int64(remaining * float64(smallestUnitFactor(unlockCurrency)))
					_ = s.walletSvc.UnlockBalance(context.Background(), maker.UserID, unlockCurrency, unlockAmt)
				}
			}
		}
	}

	// Finally the taker itself.
	taker.FilledQuantity += fillDelta[taker.OrderID]
	taker.Status = deriveStatus(taker, result.Rested)
	return s.repo.Update(ctx, taker)
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

func (s *service) ListOrders(ctx context.Context, userID, status, pairID string, limit, offset int) ([]Order, int64, error) {
	return s.repo.FindByUserIDFiltered(ctx, userID, status, pairID, limit, offset)
}

func (s *service) ListTrades(ctx context.Context, userID, pairID string, limit, offset int) ([]UserTrade, int64, error) {
	return s.repo.FindTradesByUserID(ctx, userID, pairID, limit, offset)
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

	// Unlock any remaining locked balance for this order.
	if o.Type == TypeLimit && o.LockedAmount > 0 {
		var unlockCurrency string
		switch o.Side {
		case SideBuy:
			pair, _ := s.pairs.GetPair(ctx, o.PairID)
			unlockCurrency = pair.QuoteCoin
		case SideSell:
			pair, _ := s.pairs.GetPair(ctx, o.PairID)
			unlockCurrency = pair.BaseCoin
		}
		if unlockCurrency != "" {
			var remaining float64
			if o.Side == SideBuy {
				remaining = (o.Quantity - o.FilledQuantity) * *o.Price
			} else {
				remaining = o.Quantity - o.FilledQuantity
			}
			if remaining > 0 {
				unlockAmt := int64(remaining * float64(smallestUnitFactor(unlockCurrency)))
				_ = s.walletSvc.UnlockBalance(context.Background(), userID, unlockCurrency, unlockAmt)
			}
		}
	}

	return o, nil
}

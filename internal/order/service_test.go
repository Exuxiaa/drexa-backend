package order

import (
	"context"
	"sync"
	"testing"

	"drexa/internal/matching"
)

// ─── In-memory test doubles ──────────────────────────────────────────────────

type memRepo struct {
	mu     sync.Mutex
	orders map[string]*Order
	trades []Trade
}

func newMemRepo() *memRepo { return &memRepo{orders: make(map[string]*Order)} }

func clone(o *Order) *Order { c := *o; return &c }

func (r *memRepo) Create(_ context.Context, o *Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.orders[o.OrderID] = clone(o)
	return nil
}

func (r *memRepo) FindByID(_ context.Context, id string) (*Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.orders[id]
	if !ok {
		return nil, ErrOrderNotFound
	}
	return clone(o), nil
}

func (r *memRepo) FindByUserID(_ context.Context, userID string) ([]Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Order
	for _, o := range r.orders {
		if o.UserID == userID {
			out = append(out, *o)
		}
	}
	return out, nil
}

func (r *memRepo) FindOrders(_ context.Context, userID string, f OrderFilter) ([]Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Order
	for _, o := range r.orders {
		if o.UserID != userID {
			continue
		}
		if f.PairID != "" && o.PairID != f.PairID {
			continue
		}
		out = append(out, *o)
	}
	return out, nil
}

func (r *memRepo) FindUntriggered(_ context.Context) ([]Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Order
	for _, o := range r.orders {
		if o.Status == StatusUntriggered {
			out = append(out, *o)
		}
	}
	return out, nil
}

func (r *memRepo) FindByOCOGroup(_ context.Context, group string) ([]Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Order
	for _, o := range r.orders {
		if o.OCOGroupID != nil && *o.OCOGroupID == group {
			out = append(out, *o)
		}
	}
	return out, nil
}

func (r *memRepo) FindTradesByUserID(_ context.Context, _ string, _ int) ([]TradeView, error) {
	return nil, nil
}

func (r *memRepo) Update(_ context.Context, o *Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.orders[o.OrderID]
	if !ok {
		return ErrOrderNotFound
	}
	cur.Status = o.Status
	cur.FilledQuantity = o.FilledQuantity
	cur.Fee = o.Fee
	return nil
}

func (r *memRepo) SaveTrades(_ context.Context, trades []Trade) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.trades = append(r.trades, trades...)
	return nil
}

type fakePairs struct{}

func (fakePairs) GetPair(_ context.Context, pairID string) (*PairInfo, error) {
	return &PairInfo{PairID: pairID, Active: true, MinOrderSize: 0, PriceDecimals: 2}, nil
}

func newTestService() (*memRepo, Service) {
	repo := newMemRepo()
	svc := NewService(repo, fakePairs{}, matching.NewEngine(), nil)
	return repo, svc
}

func fp(v float64) *float64 { return &v }

// ─── Tests ───────────────────────────────────────────────────────────────────

// TestStopLimitTriggersOnPrice verifies a dormant buy stop-limit stays off-book
// until a trade prints at/above its stop price, then activates and fills.
func TestStopLimitTriggersOnPrice(t *testing.T) {
	repo, svc := newTestService()
	ctx := context.Background()
	const pair = "BTC_USDC"

	// Resting ask @105 for 2 units (the future counterparty).
	if _, err := svc.CreateOrder(ctx, "seller", OrderRequest{
		PairID: pair, Side: SideSell, Type: TypeLimit, Quantity: 2, Price: fp(105),
	}); err != nil {
		t.Fatalf("seller limit: %v", err)
	}

	// Dormant buy stop: triggers when price reaches 105, then fills @106.
	stop, err := svc.CreateOrder(ctx, "buyer", OrderRequest{
		PairID: pair, Side: SideBuy, Type: TypeStopLimit, Quantity: 1, Price: fp(106), StopPrice: fp(105),
	})
	if err != nil {
		t.Fatalf("buy stop: %v", err)
	}
	if stop.Status != StatusUntriggered {
		t.Fatalf("stop status = %s, want untriggered", stop.Status)
	}

	// A different buyer lifts the ask @105 → trade prints at 105 → stop triggers.
	if _, err := svc.CreateOrder(ctx, "buyer2", OrderRequest{
		PairID: pair, Side: SideBuy, Type: TypeLimit, Quantity: 1, Price: fp(105),
	}); err != nil {
		t.Fatalf("buyer2 limit: %v", err)
	}

	got, err := repo.FindByID(ctx, stop.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFilled {
		t.Fatalf("triggered stop status = %s (filled=%v), want filled", got.Status, got.FilledQuantity)
	}
	if got.FilledQuantity != 1 {
		t.Fatalf("triggered stop filled = %v, want 1", got.FilledQuantity)
	}
}

// TestOCOCancelsSiblingOnFill verifies that filling the take-profit leg of an
// OCO order cancels its protective stop leg.
func TestOCOCancelsSiblingOnFill(t *testing.T) {
	repo, svc := newTestService()
	ctx := context.Background()
	const pair = "BTC_USDC"

	// Sell OCO: take-profit limit @110, protective stop @90, qty 1.
	limitLeg, err := svc.CreateOrder(ctx, "trader", OrderRequest{
		PairID: pair, Side: SideSell, Type: TypeOCO, Quantity: 1, Price: fp(110), StopPrice: fp(90),
	})
	if err != nil {
		t.Fatalf("oco: %v", err)
	}
	if limitLeg.OCOGroupID == nil {
		t.Fatal("oco limit leg should carry an oco_group_id")
	}

	// Find the dormant stop sibling.
	legs, _ := repo.FindByOCOGroup(ctx, *limitLeg.OCOGroupID)
	var stopID string
	for _, l := range legs {
		if l.Type == TypeStopLimit {
			stopID = l.OrderID
		}
	}
	if stopID == "" {
		t.Fatal("expected a stop-limit sibling leg")
	}

	// A buyer lifts the take-profit ask @110 → limit leg fills → stop cancelled.
	if _, err := svc.CreateOrder(ctx, "buyer", OrderRequest{
		PairID: pair, Side: SideBuy, Type: TypeLimit, Quantity: 1, Price: fp(110),
	}); err != nil {
		t.Fatalf("buyer: %v", err)
	}

	filled, _ := repo.FindByID(ctx, limitLeg.OrderID)
	if filled.Status != StatusFilled {
		t.Fatalf("oco limit leg status = %s, want filled", filled.Status)
	}
	sibling, _ := repo.FindByID(ctx, stopID)
	if sibling.Status != StatusCancelled {
		t.Fatalf("oco stop leg status = %s, want cancelled", sibling.Status)
	}
}

// TestCancelDormantStop verifies a user can cancel an untriggered stop order.
func TestCancelDormantStop(t *testing.T) {
	repo, svc := newTestService()
	ctx := context.Background()
	const pair = "ETH_USDC"

	stop, err := svc.CreateOrder(ctx, "u1", OrderRequest{
		PairID: pair, Side: SideSell, Type: TypeStopLimit, Quantity: 1, Price: fp(90), StopPrice: fp(95),
	})
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := svc.CancelOrder(ctx, "u1", stop.OrderID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got, _ := repo.FindByID(ctx, stop.OrderID)
	if got.Status != StatusCancelled {
		t.Fatalf("status = %s, want cancelled", got.Status)
	}
}

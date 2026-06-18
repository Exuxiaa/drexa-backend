package matching

import "testing"

// limit builds a limit order.
func limit(id, user string, side Side, price, qty int64) *Order {
	return &Order{ID: id, UserID: user, Side: side, Type: Limit, Price: price, Quantity: qty}
}

// market builds a market order.
func market(id, user string, side Side, qty int64) *Order {
	return &Order{ID: id, UserID: user, Side: side, Type: Market, Quantity: qty}
}

func TestLimitRestsWhenNoCross(t *testing.T) {
	ob := NewOrderBook()
	res := ob.Place(limit("b1", "u1", SideBuy, 100, 10))

	if len(res.Trades) != 0 {
		t.Fatalf("expected no trades, got %d", len(res.Trades))
	}
	if !res.Rested {
		t.Fatal("expected order to rest")
	}
	if got := ob.BestBid(); got == nil || got.Price != 100 {
		t.Fatalf("expected best bid 100, got %+v", got)
	}
}

func TestLimitFullCrossExecutesAtMakerPrice(t *testing.T) {
	ob := NewOrderBook()
	ob.Place(limit("a1", "maker", SideSell, 100, 10)) // resting ask @100

	res := ob.Place(limit("b1", "taker", SideBuy, 105, 10)) // crosses

	if len(res.Trades) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(res.Trades))
	}
	tr := res.Trades[0]
	if tr.Price != 100 {
		t.Errorf("expected execution at maker price 100, got %d", tr.Price)
	}
	if tr.Quantity != 10 {
		t.Errorf("expected qty 10, got %d", tr.Quantity)
	}
	if !res.Taker.IsFilled() {
		t.Error("taker should be fully filled")
	}
	if res.Rested {
		t.Error("fully-filled taker should not rest")
	}
	if ob.BestAsk() != nil {
		t.Error("ask book should be empty")
	}
}

func TestLimitPartialFillRestsRemainder(t *testing.T) {
	ob := NewOrderBook()
	ob.Place(limit("a1", "maker", SideSell, 100, 4))

	res := ob.Place(limit("b1", "taker", SideBuy, 100, 10))

	if res.Taker.Filled != 4 {
		t.Errorf("expected 4 filled, got %d", res.Taker.Filled)
	}
	if !res.Rested {
		t.Error("remainder of 6 should rest on the bid book")
	}
	if lvl := ob.BestBid(); lvl == nil || lvl.Volume() != 6 {
		t.Errorf("expected resting bid volume 6, got %+v", lvl)
	}
}

func TestPriceTimePriority(t *testing.T) {
	ob := NewOrderBook()
	// Two asks at the same price; a1 arrived first.
	ob.Place(limit("a1", "m1", SideSell, 100, 5))
	ob.Place(limit("a2", "m2", SideSell, 100, 5))
	// Better-priced ask should be hit before worse-priced.
	ob.Place(limit("a3", "m3", SideSell, 99, 5))

	res := ob.Place(limit("b1", "taker", SideBuy, 100, 6))

	if len(res.Trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(res.Trades))
	}
	if res.Trades[0].MakerOrderID != "a3" { // best price first
		t.Errorf("expected a3 (best price) first, got %s", res.Trades[0].MakerOrderID)
	}
	if res.Trades[1].MakerOrderID != "a1" { // then oldest at 100
		t.Errorf("expected a1 (time priority) second, got %s", res.Trades[1].MakerOrderID)
	}
}

func TestMarketOrderSweepsAndDropsRemainder(t *testing.T) {
	ob := NewOrderBook()
	ob.Place(limit("a1", "m1", SideSell, 100, 3))
	ob.Place(limit("a2", "m2", SideSell, 101, 3))

	res := ob.Place(market("m1taker", "taker", SideBuy, 10))

	if got := res.Taker.Filled; got != 6 {
		t.Errorf("expected 6 filled across both levels, got %d", got)
	}
	if res.Rested {
		t.Error("market remainder must not rest")
	}
	if ob.BestAsk() != nil {
		t.Error("ask book should be fully consumed")
	}
}

func TestSelfTradePreventionCancelsResting(t *testing.T) {
	ob := NewOrderBook()
	ob.Place(limit("a1", "same", SideSell, 100, 5)) // own resting ask

	res := ob.Place(limit("b1", "same", SideBuy, 100, 5)) // would self-trade

	if len(res.Trades) != 0 {
		t.Fatalf("expected no self-trade, got %d trades", len(res.Trades))
	}
	if len(res.Canceled) != 1 || res.Canceled[0].ID != "a1" {
		t.Fatalf("expected resting a1 canceled, got %+v", res.Canceled)
	}
	// Taker had no other liquidity, so it rests.
	if !res.Rested {
		t.Error("taker should rest after STP removed the only maker")
	}
}

func TestCancelRemovesResting(t *testing.T) {
	ob := NewOrderBook()
	ob.Place(limit("b1", "u1", SideBuy, 100, 5))

	o, err := ob.Cancel("b1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.ID != "b1" {
		t.Errorf("expected b1, got %s", o.ID)
	}
	if ob.BestBid() != nil {
		t.Error("bid book should be empty after cancel")
	}

	if _, err := ob.Cancel("b1"); err != ErrOrderNotResting {
		t.Errorf("expected ErrOrderNotResting on second cancel, got %v", err)
	}
}

func TestEngineRoutesByPair(t *testing.T) {
	e := NewEngine()
	e.Submit("BTC_IDR", limit("a1", "m1", SideSell, 100, 5))
	// Same price/qty taker on a different pair must not match.
	res := e.Submit("ETH_IDR", limit("b1", "taker", SideBuy, 100, 5))
	if len(res.Trades) != 0 {
		t.Fatalf("orders on different pairs must not match, got %d trades", len(res.Trades))
	}

	res = e.Submit("BTC_IDR", limit("b2", "taker", SideBuy, 100, 5))
	if len(res.Trades) != 1 {
		t.Fatalf("expected match on same pair, got %d", len(res.Trades))
	}
}

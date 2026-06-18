package market

import "testing"

// TestSetLastTradeOverridesPrice verifies that a price executed by our matching
// engine becomes the ticker's price, both for a pair already seeded with Binance
// 24h stats and for a pair Binance never returned.
func TestSetLastTradeOverridesPrice(t *testing.T) {
	c := NewTickerCache(nil, nil)

	// Seed BTC_USD as if Binance had filled in the 24h stats.
	c.mu.Lock()
	c.stats["BTC_USD"] = TickerStats{Pair: "BTC_USD", Price: 60000, Ch: 1.5, Vol: 1000, High: 61000, Low: 59000}
	c.mu.Unlock()

	// A trade prints at 60500: price updates, 24h stats are preserved.
	c.SetLastTrade("BTC_USD", 60500)
	got, ok := c.Get("BTC_USD")
	if !ok {
		t.Fatal("BTC_USD should be present after SetLastTrade")
	}
	if got.Price != 60500 {
		t.Errorf("price = %v, want 60500 (internal last trade)", got.Price)
	}
	if got.Ch != 1.5 || got.Vol != 1000 || got.High != 61000 || got.Low != 59000 {
		t.Errorf("24h stats should be preserved, got %+v", got)
	}

	// A pair with no Binance data still surfaces from an internal trade.
	c.SetLastTrade("FOO_USD", 12.34)
	foo, ok := c.Get("FOO_USD")
	if !ok || foo.Price != 12.34 || foo.Pair != "FOO_USD" {
		t.Errorf("FOO_USD ticker = %+v, ok=%v; want price 12.34", foo, ok)
	}
}

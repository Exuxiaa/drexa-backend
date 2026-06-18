package market

import (
	"context"
	"log"
	"sync"
	"time"
)

const tickerRefreshInterval = 30 * time.Second

// TickerCache holds the latest 24hr ticker stats for all active pairs, refreshed
// periodically from Binance. Reads are lock-free via atomic pointer swap.
type TickerCache struct {
	mu      sync.RWMutex
	stats   map[string]TickerStats // keyed by our pairID (e.g. "BTC_USD")
	binance *BinanceClient
	pairs   PairLister

	// lastTrade holds the most recent price executed by our own matching
	// engine, per pair. When set it overrides the Binance last price so the
	// ticker reflects real internal executions; the 24h stats (change, volume,
	// high, low) still come from Binance.
	lastTrade map[string]float64
}

// NewTickerCache creates an empty cache; call Run to start background refresh.
func NewTickerCache(binance *BinanceClient, pairs PairLister) *TickerCache {
	return &TickerCache{
		stats:     make(map[string]TickerStats),
		binance:   binance,
		pairs:     pairs,
		lastTrade: make(map[string]float64),
	}
}

// SetLastTrade records the latest price executed by our matching engine for a
// pair and immediately reflects it in the cached ticker. It is the authoritative
// source for the ticker's price field, overriding the Binance last price.
func (c *TickerCache) SetLastTrade(pairID string, price float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastTrade[pairID] = price
	s, ok := c.stats[pairID]
	if !ok {
		s = TickerStats{Pair: pairID}
	}
	s.Price = price
	c.stats[pairID] = s
}

// All returns a snapshot of the current ticker stats for every known pair.
func (c *TickerCache) All() []TickerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]TickerStats, 0, len(c.stats))
	for _, s := range c.stats {
		out = append(out, s)
	}
	return out
}

// Get returns the ticker for a single pair. ok is false if not yet cached.
func (c *TickerCache) Get(pairID string) (TickerStats, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.stats[pairID]
	return s, ok
}

// Run refreshes the cache every tickerRefreshInterval until ctx is cancelled.
// Call as a goroutine: go cache.Run(ctx).
func (c *TickerCache) Run(ctx context.Context) {
	c.refresh(ctx)
	ticker := time.NewTicker(tickerRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refresh(ctx)
		}
	}
}

func (c *TickerCache) refresh(ctx context.Context) {
	pairIDs, err := c.pairs.ActivePairIDs(ctx)
	if err != nil {
		log.Printf("market/ticker_cache: list pairs: %v", err)
		return
	}
	if len(pairIDs) == 0 {
		return
	}

	// Map each pair to its Binance symbol and remember the reverse mapping.
	binanceSymbols := make([]string, 0, len(pairIDs))
	binanceToPair := make(map[string]string, len(pairIDs))
	for _, id := range pairIDs {
		sym, ok := pairToBinanceSymbol(id)
		if !ok {
			continue
		}
		binanceSymbols = append(binanceSymbols, sym)
		binanceToPair[sym] = id
	}

	fetched, err := c.binance.Tickers24h(ctx, binanceSymbols)
	if err != nil {
		log.Printf("market/ticker_cache: fetch tickers: %v", err)
		return
	}

	next := make(map[string]TickerStats, len(fetched))
	for sym, stats := range fetched {
		pairID, ok := binanceToPair[sym]
		if !ok {
			continue
		}
		stats.Pair = pairID
		next[pairID] = *stats
	}

	c.mu.Lock()
	// Keep the internal last-trade price as the authoritative price field, and
	// carry over pairs that only have internal trades (no Binance listing).
	for pairID, price := range c.lastTrade {
		s, ok := next[pairID]
		if !ok {
			s = TickerStats{Pair: pairID}
		}
		s.Price = price
		next[pairID] = s
	}
	c.stats = next
	c.mu.Unlock()
}

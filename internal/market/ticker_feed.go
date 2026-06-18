package market

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

const tickerBroadcastInterval = 5 * time.Second

// tickerMessage is the JSON envelope broadcast to WebSocket clients.
type tickerMessage struct {
	Type  string  `json:"type"`
	Pair  string  `json:"pair"`
	Price float64 `json:"price"`
	Ch    float64 `json:"ch"`
	Vol   float64 `json:"vol"`
	High  float64 `json:"high"`
	Low   float64 `json:"low"`
}

// TickerFeed broadcasts ticker data from the TickerCache to the WebSocket hub
// on a fixed interval.
type TickerFeed struct {
	hub   *Hub
	cache *TickerCache
}

// NewTickerFeed creates a TickerFeed.
func NewTickerFeed(hub *Hub, cache *TickerCache) *TickerFeed {
	return &TickerFeed{hub: hub, cache: cache}
}

// Run broadcasts ticker messages until ctx is cancelled.
func (f *TickerFeed) Run(ctx context.Context) {
	ticker := time.NewTicker(tickerBroadcastInterval)
	defer ticker.Stop()

	log.Println("market/ticker: feed started")
	for {
		select {
		case <-ctx.Done():
			log.Println("market/ticker: feed stopped")
			return
		case <-ticker.C:
			f.publish()
		}
	}
}

func (f *TickerFeed) publish() {
	for _, s := range f.cache.All() {
		f.broadcast(s)
	}
}

// RecordTrade records the last executed price for a pair (from the matching
// engine) and immediately broadcasts a fresh ticker for it, so clients see the
// new price the instant a trade prints rather than waiting for the next interval.
// Satisfies the order.TradeObserver contract when wired in cmd/server.
func (f *TickerFeed) RecordTrade(pairID string, price float64) {
	f.cache.SetLastTrade(pairID, price)
	if s, ok := f.cache.Get(pairID); ok {
		f.broadcast(s)
	}
}

// broadcast marshals one ticker and pushes it to all WebSocket clients.
func (f *TickerFeed) broadcast(s TickerStats) {
	payload, err := json.Marshal(tickerMessage{
		Type:  "ticker",
		Pair:  s.Pair,
		Price: s.Price,
		Ch:    s.Ch,
		Vol:   s.Vol,
		High:  s.High,
		Low:   s.Low,
	})
	if err != nil {
		log.Printf("market/ticker: marshal %s: %v", s.Pair, err)
		return
	}
	f.hub.Broadcast <- payload
}

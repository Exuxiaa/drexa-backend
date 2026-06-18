package market

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// depthLevels are the levels the order-book feed publishes per side.
const depthLevels = 20

// publishInterval is how often the feed polls each book for changes.
const publishInterval = 500 * time.Millisecond

// BookLevel is one aggregated price level on the wire.
type BookLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

// OrderBookSnapshot is the depth view the feed reads from the matching engine,
// in real (float) prices, best prices first. It is decoupled from the order
// domain's own snapshot type and mapped by an adapter in cmd/server.
type OrderBookSnapshot struct {
	PairID  string      `json:"pair"`
	Version uint64      `json:"version"`
	Bids    []BookLevel `json:"bids"`
	Asks    []BookLevel `json:"asks"`
}

// orderBookMessage is the JSON envelope broadcast to WebSocket clients.
type orderBookMessage struct {
	Type string `json:"type"` // always "orderbook"
	*OrderBookSnapshot
}

// DepthSource provides live order-book depth for a pair. Satisfied by an
// adapter over the order service / matching engine, wired in cmd/server.
type DepthSource interface {
	OrderBookDepth(ctx context.Context, pairID string, maxLevels int) (*OrderBookSnapshot, error)
}

// PairLister enumerates the pairs whose books should be published. Satisfied by
// a market.TradingPair-backed adapter, wired in cmd/server.
type PairLister interface {
	ActivePairIDs(ctx context.Context) ([]string, error)
}

// OrderBookFeed periodically snapshots each active pair's book from our own
// matching engine and broadcasts depth updates to the hub. It replaces the
// external Binance ticker stream as the source of the /market/ws feed.
type OrderBookFeed struct {
	hub    *Hub
	source DepthSource
	pairs  PairLister

	// lastVersion tracks the last published book version per pair so unchanged
	// books are not rebroadcast.
	lastVersion map[string]uint64
}

func NewOrderBookFeed(hub *Hub, source DepthSource, pairs PairLister) *OrderBookFeed {
	return &OrderBookFeed{
		hub:         hub,
		source:      source,
		pairs:       pairs,
		lastVersion: make(map[string]uint64),
	}
}

// Run polls the books on a fixed interval until ctx is cancelled.
func (f *OrderBookFeed) Run(ctx context.Context) {
	ticker := time.NewTicker(publishInterval)
	defer ticker.Stop()

	log.Println("market/orderbook: feed started (source: internal matching engine)")
	for {
		select {
		case <-ctx.Done():
			log.Println("market/orderbook: feed stopped")
			return
		case <-ticker.C:
			f.publish(ctx)
		}
	}
}

// publish broadcasts a fresh depth snapshot for every pair whose book changed
// since the last tick.
func (f *OrderBookFeed) publish(ctx context.Context) {
	pairIDs, err := f.pairs.ActivePairIDs(ctx)
	if err != nil {
		log.Printf("market/orderbook: list pairs: %v", err)
		return
	}

	for _, pairID := range pairIDs {
		snap, err := f.source.OrderBookDepth(ctx, pairID, depthLevels)
		if err != nil {
			log.Printf("market/orderbook: depth for %s: %v", pairID, err)
			continue
		}

		// Skip pairs whose book is unchanged since the last publish.
		if last, ok := f.lastVersion[pairID]; ok && last == snap.Version {
			continue
		}
		f.lastVersion[pairID] = snap.Version

		payload, err := json.Marshal(orderBookMessage{Type: "orderbook", OrderBookSnapshot: snap})
		if err != nil {
			log.Printf("market/orderbook: marshal %s: %v", pairID, err)
			continue
		}
		f.hub.Broadcast <- payload
	}
}

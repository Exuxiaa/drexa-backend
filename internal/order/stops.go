package order

import "sync"

// stopEntry is a dormant stop order awaiting its trigger price.
type stopEntry struct {
	orderID   string
	side      OrderSide
	stopPrice float64
}

// stopBook holds dormant stop orders per pair and decides when they trigger
// from the pair's last traded price. Safe for concurrent use.
//
// A stop order rests here (off the matching book) until its trigger price is
// hit, at which point the order service pops it and submits it to the engine as
// a regular limit order.
type stopBook struct {
	mu     sync.Mutex
	byPair map[string][]stopEntry
}

func newStopBook() *stopBook {
	return &stopBook{byPair: make(map[string][]stopEntry)}
}

// add registers a dormant stop order for a pair.
func (b *stopBook) add(pairID string, e stopEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.byPair[pairID] = append(b.byPair[pairID], e)
}

// remove drops a dormant stop by order id (cancel, or OCO sibling cancel).
// Returns true if it was present.
func (b *stopBook) remove(orderID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for pair, list := range b.byPair {
		for i, e := range list {
			if e.orderID == orderID {
				b.byPair[pair] = append(list[:i], list[i+1:]...)
				return true
			}
		}
	}
	return false
}

// triggered removes and returns the ids of stops whose condition is met at the
// given last price. A buy stop fires when the price rises to/through its stop;
// a sell stop fires when the price falls to/through its stop.
func (b *stopBook) triggered(pairID string, lastPrice float64) []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	list := b.byPair[pairID]
	if len(list) == 0 {
		return nil
	}

	var fired []string
	kept := list[:0] // in-place filter: writes never outrun reads
	for _, e := range list {
		hit := (e.side == SideBuy && lastPrice >= e.stopPrice) ||
			(e.side == SideSell && lastPrice <= e.stopPrice)
		if hit {
			fired = append(fired, e.orderID)
		} else {
			kept = append(kept, e)
		}
	}
	b.byPair[pairID] = kept
	return fired
}

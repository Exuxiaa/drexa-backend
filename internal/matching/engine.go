package matching

import "sync"

// Engine is the concurrency-safe public entrypoint to the matching system. It
// owns one OrderBook per trading pair and serializes all operations on a given
// book, so callers never touch a book directly.
type Engine struct {
	mu    sync.RWMutex
	books map[string]*OrderBook
}

// NewEngine returns an empty engine with no books.
func NewEngine() *Engine {
	return &Engine{books: make(map[string]*OrderBook)}
}

// book returns the book for pairID, creating it on first use.
func (e *Engine) book(pairID string) *OrderBook {
	e.mu.RLock()
	b := e.books[pairID]
	e.mu.RUnlock()
	if b != nil {
		return b
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if b = e.books[pairID]; b == nil { // re-check after upgrading the lock
		b = NewOrderBook()
		e.books[pairID] = b
	}
	return b
}

// Submit matches an order against the given pair's book and returns the result.
func (e *Engine) Submit(pairID string, o *Order) MatchResult {
	b := e.book(pairID)
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Place(o)
}

// Cancel removes a resting order from the given pair's book.
func (e *Engine) Cancel(pairID, orderID string) (*Order, error) {
	b := e.book(pairID)
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Cancel(orderID)
}

// Depth returns a snapshot of up to maxLevels aggregated levels per side for a
// pair, in tick/lot integers. maxLevels <= 0 returns every level.
func (e *Engine) Depth(pairID string, maxLevels int) Depth {
	b := e.book(pairID)
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Snapshot(maxLevels)
}

// BestBidAsk returns the best bid and ask tick prices for a pair. ok is false
// for a side with no resting orders. Useful for pricing market orders and for
// a top-of-book feed.
func (e *Engine) BestBidAsk(pairID string) (bid int64, ask int64, bidOK, askOK bool) {
	b := e.book(pairID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if lvl := b.BestBid(); lvl != nil {
		bid, bidOK = lvl.Price, true
	}
	if lvl := b.BestAsk(); lvl != nil {
		ask, askOK = lvl.Price, true
	}
	return bid, ask, bidOK, askOK
}

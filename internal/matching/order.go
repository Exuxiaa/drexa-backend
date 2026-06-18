package matching

import "container/list"

// Order is the engine's view of a resting or incoming order. Price and
// Quantity are integers (ticks / lots) so all comparisons are exact.
//
// The element and level back-pointers let a resting order be removed from the
// book in O(1) without searching the price level's FIFO queue.
type Order struct {
	ID       string
	UserID   string
	Side     Side
	Type     Type
	Price    int64 // tick price; ignored for Market orders
	Quantity int64 // total size in lots
	Filled   int64 // cumulative matched size in lots

	// Sequence is assigned by the book on entry; it gives a stable,
	// monotonic time priority across price levels for external consumers.
	Sequence uint64

	element *list.Element // position within level.orders, nil while not resting
	level   *PriceLevel   // owning price level, nil while not resting
}

// Remaining is the still-unmatched size.
func (o *Order) Remaining() int64 { return o.Quantity - o.Filled }

// IsFilled reports whether the order has been fully matched.
func (o *Order) IsFilled() bool { return o.Filled >= o.Quantity }

// resting reports whether the order currently sits in a book.
func (o *Order) resting() bool { return o.element != nil }

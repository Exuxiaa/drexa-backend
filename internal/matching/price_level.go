package matching

import "container/list"

// PriceLevel is the FIFO queue of orders resting at a single price. Time
// priority is preserved by appending new orders to the back and always
// matching from the front.
type PriceLevel struct {
	Price  int64
	volume int64 // sum of Remaining() across queued orders

	orders *list.List // of *Order, oldest at Front
}

// NewPriceLevel creates an empty level at the given tick price.
func NewPriceLevel(price int64) *PriceLevel {
	return &PriceLevel{
		Price:  price,
		orders: list.New(),
	}
}

// push appends an order to the back of the queue and wires its back-pointers.
func (pl *PriceLevel) push(o *Order) {
	o.level = pl
	o.element = pl.orders.PushBack(o)
	pl.volume += o.Remaining()
}

// remove detaches an order from the queue. The caller is responsible for any
// volume already consumed via consume; remove subtracts the order's current
// remaining size.
func (pl *PriceLevel) remove(o *Order) {
	if o.element == nil {
		return
	}
	pl.volume -= o.Remaining()
	pl.orders.Remove(o.element)
	o.element = nil
	o.level = nil
}

// front returns the oldest resting order, or nil if the level is empty.
func (pl *PriceLevel) front() *Order {
	e := pl.orders.Front()
	if e == nil {
		return nil
	}
	return e.Value.(*Order)
}

// empty reports whether the level has no resting orders.
func (pl *PriceLevel) empty() bool { return pl.orders.Len() == 0 }

// Len is the number of resting orders at this level.
func (pl *PriceLevel) Len() int { return pl.orders.Len() }

// Volume is the total remaining size resting at this level.
func (pl *PriceLevel) Volume() int64 { return pl.volume }

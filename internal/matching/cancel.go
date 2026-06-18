package matching

import "errors"

// ErrOrderNotResting is returned when canceling an order the book is not
// holding (already filled, already canceled, or never rested).
var ErrOrderNotResting = errors.New("matching: order is not resting on the book")

// Cancel removes a resting order from the book and returns it in its final
// state (Filled reflects any prior partial fills). The caller releases the
// order's remaining locked balance.
func (ob *OrderBook) Cancel(orderID string) (*Order, error) {
	o, ok := ob.orders[orderID]
	if !ok || !o.resting() {
		return nil, ErrOrderNotResting
	}
	ob.removeOrder(o)
	return o, nil
}

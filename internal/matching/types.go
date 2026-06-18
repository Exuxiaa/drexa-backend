// Package matching is an in-memory, price-time-priority matching engine.
//
// It is intentionally decoupled from the persistence layer (internal/order):
// it speaks in integer "ticks" and "lots" so price/quantity comparisons are
// exact and deterministic, never subject to float rounding. The order domain
// converts its float64 / *float64 fields to and from these integers at the
// boundary (see internal/order/engine.go).
//
// Concurrency: a single OrderBook is NOT safe for concurrent use. The Engine
// wraps each book with its own mutex and is the safe public entrypoint.
package matching

// Side is the direction of an order.
type Side uint8

const (
	SideBuy Side = iota
	SideSell
)

func (s Side) String() string {
	if s == SideBuy {
		return "buy"
	}
	return "sell"
}

// Type is the execution style of an order.
type Type uint8

const (
	// Limit rests on the book at its price if it does not fully match.
	Limit Type = iota
	// Market crosses the book at any price; any unfilled remainder is
	// canceled rather than rested.
	Market
)

func (t Type) String() string {
	if t == Market {
		return "market"
	}
	return "limit"
}

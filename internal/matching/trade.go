package matching

// Trade is a single fill produced when an incoming (taker) order crosses a
// resting (maker) order. It always executes at the maker's resting price.
//
// Prices and quantities are integer ticks/lots, consistent with Order. Fees
// are not computed here; the order/settlement layer applies its fee schedule
// when persisting the resulting order.Trade.
type Trade struct {
	MakerOrderID string
	TakerOrderID string
	MakerUserID  string
	TakerUserID  string
	Price        int64 // maker's resting price, in ticks
	Quantity     int64 // matched size, in lots
	TakerSide    Side  // side of the aggressor
}

// MatchResult is the outcome of submitting one order to the engine.
type MatchResult struct {
	// Taker is the submitted order in its post-match state (Filled updated).
	Taker *Order
	// Trades are the fills generated, in execution order.
	Trades []Trade
	// Canceled holds resting maker orders that were removed by self-trade
	// prevention; their owners' locked balances should be released.
	Canceled []*Order
	// Rested is true when a limit taker had remaining size and now sits on
	// the book. Market remainders are never rested.
	Rested bool
}

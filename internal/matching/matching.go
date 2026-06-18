package matching

// Place submits an order to the book, matching it against the opposite side in
// price-time priority and resting any limit remainder. It returns the fills,
// the taker's final state, and any maker orders canceled by self-trade
// prevention.
//
// Self-trade prevention policy: cancel-resting. If the taker would match its
// own resting order, that maker is removed from the book (and reported in
// Canceled) and matching continues with the next order. This lets the taker
// proceed while guaranteeing no wash trade is printed.
func (ob *OrderBook) Place(taker *Order) MatchResult {
	taker.Sequence = ob.nextSeq()
	result := MatchResult{Taker: taker}

	ob.match(taker, &result)

	// Rest a crossing-exhausted limit remainder; market remainders are dropped.
	if taker.Type == Limit && taker.Remaining() > 0 {
		ob.rest(taker)
		result.Rested = true
	}

	ob.version++ // book may have changed; signal the depth feed

	return result
}

// match repeatedly crosses the taker against the best opposite level until the
// taker is filled, the spread no longer crosses (limit), or the book is empty.
func (ob *OrderBook) match(taker *Order, result *MatchResult) {
	for taker.Remaining() > 0 {
		level := ob.bestOpposite(taker.Side)
		if level == nil || !crosses(taker, level.Price) {
			return
		}

		for taker.Remaining() > 0 && !level.empty() {
			maker := level.front()

			// Self-trade prevention: drop the resting maker, keep matching.
			if maker.UserID == taker.UserID {
				ob.removeOrder(maker)
				result.Canceled = append(result.Canceled, maker)
				continue
			}

			qty := min(taker.Remaining(), maker.Remaining())
			taker.Filled += qty
			maker.Filled += qty
			level.volume -= qty

			result.Trades = append(result.Trades, Trade{
				MakerOrderID: maker.ID,
				TakerOrderID: taker.ID,
				MakerUserID:  maker.UserID,
				TakerUserID:  taker.UserID,
				Price:        maker.Price, // execute at the resting price
				Quantity:     qty,
				TakerSide:    taker.Side,
			})

			if maker.IsFilled() {
				ob.removeOrder(maker) // also drops the level if it empties
			}
		}
	}
}

// bestOpposite returns the best level on the side the taker will hit.
func (ob *OrderBook) bestOpposite(takerSide Side) *PriceLevel {
	if takerSide == SideBuy {
		return ob.BestAsk()
	}
	return ob.BestBid()
}

// crosses reports whether a taker can trade at the given resting price. Market
// orders cross at any price; limit orders require a favorable spread.
func crosses(taker *Order, restingPrice int64) bool {
	if taker.Type == Market {
		return true
	}
	if taker.Side == SideBuy {
		return restingPrice <= taker.Price
	}
	return restingPrice >= taker.Price
}

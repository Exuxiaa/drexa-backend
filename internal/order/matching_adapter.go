package order

import (
	"math"

	"drexa/internal/matching"
)

// The matching engine works in exact integers. Prices are scaled by the pair's
// quoted decimal places; quantities use a fixed scale so any pair shares one
// lot size. quantityDecimals=8 mirrors the 8-dp granularity common to crypto
// base amounts.
//
// Range note: a tick/lot is int64. price*10^priceDecimals and
// quantity*10^8 must each stay under ~9.2e18. For local dev this is ample
// (e.g. a 1e9 IDR price at 2 dp = 1e11 ticks).
const quantityDecimals = 8

// epsilon absorbs float rounding when comparing accumulated fills to quantity.
const epsilon = 1e-9

func pow10(n int) float64 { return math.Pow(10, float64(n)) }

func priceToTicks(price float64, dec int) int64 { return int64(math.Round(price * pow10(dec))) }
func ticksToPrice(ticks int64, dec int) float64 { return float64(ticks) / pow10(dec) }

func qtyToLots(q float64) int64    { return int64(math.Round(q * pow10(quantityDecimals))) }
func lotsToQty(lots int64) float64 { return float64(lots) / pow10(quantityDecimals) }

// toEngineOrder converts a persisted order into the engine's integer view.
// priceDec is the pair's quoted price precision.
func toEngineOrder(o *Order, priceDec int) *matching.Order {
	eo := &matching.Order{
		ID:       o.OrderID,
		UserID:   o.UserID,
		Quantity: qtyToLots(o.Quantity),
		Filled:   qtyToLots(o.FilledQuantity),
	}

	if o.Side == SideBuy {
		eo.Side = matching.SideBuy
	} else {
		eo.Side = matching.SideSell
	}

	if o.Type == TypeMarket {
		eo.Type = matching.Market
	} else {
		eo.Type = matching.Limit
		eo.Price = priceToTicks(*o.Price, priceDec)
	}

	return eo
}

// deriveStatus maps an order's fill progress to its lifecycle status. rested is
// true when a limit order still sits on the book after matching.
func deriveStatus(o *Order, rested bool) OrderStatus {
	switch {
	case o.FilledQuantity >= o.Quantity-epsilon:
		return StatusFilled
	case o.FilledQuantity > epsilon:
		// Partially filled: limit remainder rests, market remainder is dropped
		// but the order is still terminal-partial either way.
		return StatusPartiallyFilled
	case rested:
		return StatusOpen
	default:
		// Zero fill and nothing resting → a market order that found no
		// liquidity, or a limit order that did not cross.
		return StatusCancelled
	}
}

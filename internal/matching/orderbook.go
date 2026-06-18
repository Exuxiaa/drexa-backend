package matching

import (
	"sync"

	"drexa/pkg/redblacktree"
)

// OrderBook holds the resting bids and asks for a single trading pair.
//
// Bids and asks are red-black trees keyed by tick price, giving O(log n)
// best-price access and level insertion/removal:
//   - best bid  = highest bid price  = bids.Right()
//   - best ask  = lowest ask price   = asks.Left()
//
// Not safe for concurrent use; the Engine serializes access per book.
type OrderBook struct {
	mu sync.Mutex

	bids *redblacktree.Tree[int64, *PriceLevel]
	asks *redblacktree.Tree[int64, *PriceLevel]

	orders map[string]*Order // id -> resting order, for O(1) cancel
	seq    uint64
}

// NewOrderBook returns an empty book.
func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids:   redblacktree.New[int64, *PriceLevel](),
		asks:   redblacktree.New[int64, *PriceLevel](),
		orders: make(map[string]*Order),
	}
}

// BestBid returns the highest-priced bid level, or nil if there are no bids.
func (ob *OrderBook) BestBid() *PriceLevel {
	node := ob.bids.Right()
	if node == nil {
		return nil
	}
	return node.Value
}

// BestAsk returns the lowest-priced ask level, or nil if there are no asks.
func (ob *OrderBook) BestAsk() *PriceLevel {
	node := ob.asks.Left()
	if node == nil {
		return nil
	}
	return node.Value
}

func (ob *OrderBook) nextSeq() uint64 {
	ob.seq++
	return ob.seq
}

// treeFor returns the tree an order of the given side rests in.
func (ob *OrderBook) treeFor(side Side) *redblacktree.Tree[int64, *PriceLevel] {
	if side == SideBuy {
		return ob.bids
	}
	return ob.asks
}

// rest inserts an order into its own side of the book, creating the price
// level if needed.
func (ob *OrderBook) rest(o *Order) {
	tree := ob.treeFor(o.Side)
	level, found := tree.Get(o.Price)
	if !found {
		level = NewPriceLevel(o.Price)
		tree.Put(o.Price, level)
	}
	level.push(o)
	ob.orders[o.ID] = o
}

// removeOrder detaches a resting order from the book, dropping its price level
// if it becomes empty. Used by both fills and self-trade-prevention cancels.
func (ob *OrderBook) removeOrder(o *Order) {
	level := o.level
	if level == nil {
		return
	}
	price := level.Price
	side := o.Side
	level.remove(o)
	delete(ob.orders, o.ID)
	if level.empty() {
		ob.treeFor(side).Remove(price)
	}
}

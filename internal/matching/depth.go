package matching

import "drexa/pkg/redblacktree"

// DepthLevel is one aggregated price level in a depth snapshot: the total
// resting volume at a single tick price.
type DepthLevel struct {
	Price  int64 // tick price
	Volume int64 // sum of remaining lots resting at this price
}

// Depth is a point-in-time view of an order book, best prices first.
//
//   - Bids are ordered highest price first.
//   - Asks are ordered lowest price first.
//
// Version is the book's mutation counter at the time of the snapshot; clients
// can use it to detect gaps and to skip redundant updates.
type Depth struct {
	Version uint64
	Bids    []DepthLevel
	Asks    []DepthLevel
}

// Snapshot returns up to maxLevels aggregated levels per side. maxLevels <= 0
// returns every level. The book must be held by the caller (the Engine
// serializes this with Place/Cancel).
func (ob *OrderBook) Snapshot(maxLevels int) Depth {
	return Depth{
		Version: ob.version,
		Bids:    collectLevels(ob.bids, false, maxLevels), // highest price first
		Asks:    collectLevels(ob.asks, true, maxLevels),  // lowest price first
	}
}

// collectLevels walks a side's tree into aggregated levels. ascending reads
// low→high (asks); otherwise high→low (bids).
func collectLevels(tree *redblacktree.Tree[int64, *PriceLevel], ascending bool, maxLevels int) []DepthLevel {
	it := tree.Iterator()
	advance := it.Prev
	if ascending {
		it.Begin()
		advance = it.Next
	} else {
		it.End()
	}

	var levels []DepthLevel
	for advance() {
		lvl := it.Value()
		if lvl.empty() {
			continue
		}
		levels = append(levels, DepthLevel{Price: lvl.Price, Volume: lvl.volume})
		if maxLevels > 0 && len(levels) == maxLevels {
			break
		}
	}
	return levels
}

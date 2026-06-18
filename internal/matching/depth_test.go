package matching

import "testing"

func TestSnapshotOrdersAndAggregates(t *testing.T) {
	ob := NewOrderBook()
	// Bids at two prices; the 100 level aggregates two orders.
	ob.Place(limit("b1", "u1", SideBuy, 100, 5))
	ob.Place(limit("b2", "u2", SideBuy, 100, 3))
	ob.Place(limit("b3", "u3", SideBuy, 99, 4))
	// Asks at two prices.
	ob.Place(limit("a1", "u4", SideSell, 101, 2))
	ob.Place(limit("a2", "u5", SideSell, 102, 6))

	d := ob.Snapshot(0)

	// Bids: highest price first, volumes aggregated per level.
	if len(d.Bids) != 2 {
		t.Fatalf("expected 2 bid levels, got %d", len(d.Bids))
	}
	if d.Bids[0].Price != 100 || d.Bids[0].Volume != 8 {
		t.Errorf("expected best bid 100 vol 8, got %+v", d.Bids[0])
	}
	if d.Bids[1].Price != 99 || d.Bids[1].Volume != 4 {
		t.Errorf("expected next bid 99 vol 4, got %+v", d.Bids[1])
	}

	// Asks: lowest price first.
	if len(d.Asks) != 2 {
		t.Fatalf("expected 2 ask levels, got %d", len(d.Asks))
	}
	if d.Asks[0].Price != 101 || d.Asks[0].Volume != 2 {
		t.Errorf("expected best ask 101 vol 2, got %+v", d.Asks[0])
	}
	if d.Asks[1].Price != 102 || d.Asks[1].Volume != 6 {
		t.Errorf("expected next ask 102 vol 6, got %+v", d.Asks[1])
	}
}

func TestSnapshotRespectsMaxLevels(t *testing.T) {
	ob := NewOrderBook()
	ob.Place(limit("a1", "u1", SideSell, 101, 1))
	ob.Place(limit("a2", "u2", SideSell, 102, 1))
	ob.Place(limit("a3", "u3", SideSell, 103, 1))

	d := ob.Snapshot(2)
	if len(d.Asks) != 2 {
		t.Fatalf("expected 2 ask levels with maxLevels=2, got %d", len(d.Asks))
	}
	if d.Asks[0].Price != 101 || d.Asks[1].Price != 102 {
		t.Errorf("expected the two best asks 101,102, got %+v", d.Asks)
	}
}

func TestVersionBumpsOnPlaceAndCancel(t *testing.T) {
	ob := NewOrderBook()
	if v := ob.Snapshot(0).Version; v != 0 {
		t.Fatalf("expected initial version 0, got %d", v)
	}

	ob.Place(limit("b1", "u1", SideBuy, 100, 5))
	v1 := ob.Snapshot(0).Version
	if v1 == 0 {
		t.Fatal("expected version to bump after Place")
	}

	if _, err := ob.Cancel("b1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	v2 := ob.Snapshot(0).Version
	if v2 <= v1 {
		t.Errorf("expected version to bump after Cancel, got %d then %d", v1, v2)
	}
}

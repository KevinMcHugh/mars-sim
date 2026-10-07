package wire

import (
	"reflect"
	"slices"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// orderSnap is the fixture with a silo at (5,5) and three orders in its iron
// book, two bids and an ask, the colony's bid partly filled by Uma Xu.
func orderSnap() *sim.Snapshot {
	snap := fixture(true)
	snap.Tick, snap.TicksPerDay = 1000, 400 // two and a half days in
	snap.Entities[0].Profile = &sim.Profile{Name: "Uma Xu"}
	uma := sim.ColonistOwner(snap.Entities[0].ID)
	silo := sim.Point{X: 5, Y: 5, Level: sim.LandingLevel}
	snap.Economy.Silo, snap.Economy.HasSilo = silo, true
	snap.Economy.Orders = []sim.OrderView{
		{ID: 3, Side: sim.Bid, Item: sim.IronOre, Qty: 4, Price: 5, Actor: sim.Community, Depot: silo, Posted: 100,
			Escrow: 20, Filled: 6, Fills: []sim.Fill{{With: uma, Qty: 6, Total: 30}}},
		{ID: 4, Side: sim.Ask, Item: sim.IronOre, Qty: 2, Price: 9, Actor: uma, Depot: silo, Posted: 900, Expires: 1100},
		{ID: 7, Side: sim.Bid, Item: sim.IronOre, Qty: 1, Price: 6, Actor: uma, Depot: silo, Posted: 950},
		{ID: 8, Side: sim.Bid, Item: sim.Clay, Qty: 1, Price: 6, Actor: uma, Depot: silo, Posted: 950},
	}
	return snap
}

// A book lists only its own item and depot, each side in matching order.
func TestBookTopic(t *testing.T) {
	var b BookTopic
	due(t, orderSnap(), "book:5,5:"+sim.IronOre.String(), &b)
	if b.Depot != "silo" || b.Item != sim.IronOre.String() {
		t.Errorf("book = %+v", b)
	}
	ids := func(os []BookOrder) (out []uint64) {
		for _, o := range os {
			out = append(out, o.ID)
		}
		return out
	}
	if !slices.Equal(ids(b.Bids), []uint64{7, 3}) || !slices.Equal(ids(b.Asks), []uint64{4}) {
		t.Fatalf("bids %v, asks %v; want the $6 bid ahead of the $5 one", ids(b.Bids), ids(b.Asks))
	}
	if c := b.Bids[1]; c.Owner != "the colony" || c.OwnerKey != "colony" || c.Filled != 6 {
		t.Errorf("the colony's bid = %+v", c)
	}
	if a := b.Asks[0]; a.Owner != "Uma Xu" || a.OwnerKey != "1" {
		t.Errorf("Uma's ask = %+v", a)
	}
}

// An order's page has its terms, its fills, and its times on the colony clock.
func TestOrderTopic(t *testing.T) {
	snap := orderSnap()
	var o OrderTopic
	due(t, snap, "order:3", &o)
	want := OrderTopic{Found: true, ID: 3, Side: "bid", Item: sim.IronOre.String(), Owner: "the colony", OwnerKey: "colony",
		Price: 5, Qty: 10, Filled: 6, Open: 4, Total: 50, Escrow: 20, X: 5, Y: 5, Depot: "silo",
		Posted: 100, PostedDay: sim.DayOf(100, 400), PostedMinute: sim.MinuteOfDay(100, 400), OpenMinutes: 900 * 1440 / 400,
		Fills: []OrderFill{{Key: "1", Label: "Uma Xu", Qty: 6, Total: 30}}, Paid: 30}
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("order = %+v\nwant    %+v", o, want)
	}
	if o.PostedDay != 1 || o.PostedMinute != 12*60 {
		t.Errorf("posted day %d minute %d; tick 100 of a 400-tick day landing at 06:00 is day 1, 12:00", o.PostedDay, o.PostedMinute)
	}

	var ask OrderTopic
	due(t, snap, "order:4", &ask)
	if ask.Escrow != 0 || ask.ExpiresMinutes != 100*1440/400 || ask.Paid != 0 || len(ask.Fills) != 0 {
		t.Errorf("ask = %+v", ask)
	}

	var gone OrderTopic
	due(t, snap, "order:99", &gone)
	if gone.Found || gone.ID != 99 {
		t.Errorf("a closed order = %+v", gone)
	}
}

func TestOrderTopicNames(t *testing.T) {
	tp := NewTopics()
	for _, ok := range []string{"order:3", "book:5,5:" + sim.IronOre.String()} {
		if err := tp.Subscribe(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"order:", "order:x", "book:5,5", "book:5:" + sim.IronOre.String(), "book:5,5:unobtainium"} {
		if err := tp.Subscribe(bad); err == nil {
			t.Errorf("%s subscribed", bad)
		}
	}
}

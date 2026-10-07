package sim

import "testing"

// wideWorld is marketWorld with two of the colony's kitchens (no pantries, so
// each stove's own depot is where its meals come out).
func wideWorld(t *testing.T) (w *World, silo Point, houses [2]Point, cs [3]*Entity) {
	t.Helper()
	w, silo, cs = marketWorld(t)
	houses = [2]Point{{8, 6, LandingLevel}, {16, 6, LandingLevel}}
	for _, h := range houses {
		w.SetTerrain(h, Scumhouse)
	}
	w.refreshSpatial()
	return w, silo, houses, cs
}

func wideOrdersOf(w *World, side Side, item ItemKind) []*Order {
	return w.sortedOrders(func(o *Order) bool { return o.wide && o.Side == side && o.Item == item })
}

// "Buy 10 meals at $10, sell at $5": the bid is split across the kitchens,
// the ask offers what the colony holds wherever it is, the colony never
// trades with itself, and upkeep tops the bid back up once it fills.
func TestAColonyWideOrderNeedsNoDepot(t *testing.T) {
	w, silo, houses, cs := wideWorld(t)
	stock(w, silo, Community, Meal, 3)
	stock(w, houses[0], Community, Meal, 2)
	before := w.treasury
	if !w.setColonyWideOrder(SetColonyWideOrder{Side: Bid, Item: Meal, Qty: 10, Price: 10}) {
		t.Fatalf("bid refused; log: %v", w.log.tail(1))
	}
	if !w.setColonyWideOrder(SetColonyWideOrder{Side: Ask, Item: Meal, Qty: 100, Price: 5}) {
		t.Fatalf("ask refused; log: %v", w.log.tail(1))
	}
	for _, h := range houses {
		if n := w.wideOpen(Bid, Meal, h); n != 5 {
			t.Errorf("kitchen %v: %d bid for, want 5", h, n)
		}
	}
	if w.treasury != before-100 {
		t.Errorf("treasury %v, want %v less 100 in escrow", w.treasury, before)
	}
	if n := w.wideOpen(Ask, Meal, silo); n != 3 {
		t.Errorf("%d on offer at the silo, want the 3 held there", n)
	}
	if n := w.wideOpen(Ask, Meal, houses[0]); n != 2 {
		t.Errorf("%d on offer at the kitchen, want the 2 held there, not sold to the colony's own bid", n)
	}

	// A cook's meal sells to the colony's bid at the bid's price...
	cook := ColonistOwner(cs[0].ID)
	stock(w, houses[1], cook, Meal, 1)
	if _, n := w.post(Ask, Meal, 1, 4, cook, houses[1], 0); n != 1 || cs[0].wallet != 110 {
		t.Fatalf("cook's meal: %d traded, wallet %v; want 1 sold for $10", n, cs[0].wallet)
	}
	// ...and a hungry colonist buys one of the colony's at its ask.
	buyer := ColonistOwner(cs[1].ID)
	if _, n := w.post(Bid, Meal, 1, 6, buyer, houses[0], 0); n != 1 || cs[1].wallet != 95 {
		t.Fatalf("buyer: %d traded, wallet %v; want 1 bought for $5", n, cs[1].wallet)
	}

	w.refreshWideOrders()
	if n := w.wideOpen(Bid, Meal, houses[1]); n != 5 {
		t.Errorf("upkeep left %d bid for at the kitchen that sold, want 5", n)
	}
	// The meal the colony bought is on offer too, at its ask price.
	if n := w.wideOpen(Ask, Meal, houses[1]); n != 1 {
		t.Errorf("%d of the bought meal on offer, want 1", n)
	}
	assertMoneyConserved(t, w)
	for p, c := range w.storageContainers {
		if !c.ledgerBalanced() {
			t.Fatalf("ledger at %v unbalanced", p)
		}
	}
}

// Setting a colony-wide order again replaces it; clearing it takes its
// orders off the book and returns the escrow.
func TestAColonyWideOrderIsReplacedAndCleared(t *testing.T) {
	w, _, _, _ := wideWorld(t)
	before := w.treasury
	w.setColonyWideOrder(SetColonyWideOrder{Side: Bid, Item: CaveScum, Qty: 4, Price: 3})
	w.setColonyWideOrder(SetColonyWideOrder{Side: Bid, Item: CaveScum, Qty: 6, Price: 2})
	open := 0
	for _, o := range wideOrdersOf(w, Bid, CaveScum) {
		if o.Price != 2 {
			t.Fatalf("an order at the old price is open: %+v", o)
		}
		open += o.Qty
	}
	if open != 6 || w.treasury != before-12 {
		t.Fatalf("%d bid for, treasury %v (was %v); want 6 at $2", open, w.treasury, before)
	}
	if !w.clearColonyWideOrder(ClearColonyWideOrder{Side: Bid, Item: CaveScum}) {
		t.Fatal("clear refused")
	}
	if len(wideOrdersOf(w, Bid, CaveScum)) != 0 || w.treasury != before {
		t.Fatalf("after clearing: orders %v, treasury %v (was %v)", wideOrdersOf(w, Bid, CaveScum), w.treasury, before)
	}
	w.refreshWideOrders()
	if len(wideOrdersOf(w, Bid, CaveScum)) != 0 {
		t.Fatal("upkeep re-posted a cleared order")
	}
	if w.clearColonyWideOrder(ClearColonyWideOrder{Side: Bid, Item: CaveScum}) {
		t.Fatal("cleared an order that was not set")
	}
	assertMoneyConserved(t, w)
}

// While a colony-wide order is set, the colony's own standing orders for that
// side and item stay off the book, and the upkeep that withdraws its scum
// bids leaves the player's alone.
func TestAColonyWideOrderReplacesTheStandingOrder(t *testing.T) {
	w, silo, _, _ := wideWorld(t)
	stock(w, silo, Community, Meal, 4)
	w.runMarket() // tick 0: the standing meal asks go up
	if len(w.sortedOrders(func(o *Order) bool { return o.Actor == Community && o.Side == Ask && o.Item == Meal && !o.wide })) == 0 {
		t.Fatal("expected the colony's standing meal ask (testConfig posts them)")
	}
	w.setColonyWideOrder(SetColonyWideOrder{Side: Ask, Item: Meal, Qty: 50, Price: 2})
	w.setColonyWideOrder(SetColonyWideOrder{Side: Bid, Item: CaveScum, Qty: 4, Price: 3})
	w.runMarket()
	for _, o := range w.sortedOrders(func(o *Order) bool { return o.Actor == Community && o.Item == Meal }) {
		if !o.wide || o.Price != 2 {
			t.Fatalf("a standing meal order is open beside the colony-wide one: %+v", o)
		}
	}
	for _, p := range w.colonyKitchens() {
		w.withdrawColonyBids(CaveScum, p)
	}
	if n := len(wideOrdersOf(w, Bid, CaveScum)); n == 0 {
		t.Fatal("withdrawing the colony's standing scum bids took the colony-wide ones")
	}
}

// A bid for anything else stands at the silo; one the treasury cannot cover
// in full goes up as far as it stretches.
func TestAColonyWideBidForOreStandsAtTheSilo(t *testing.T) {
	w, silo, _, _ := wideWorld(t)
	w.treasury, w.moneyIssued = 30, w.moneyIssued-(w.treasury-30)
	w.setColonyWideOrder(SetColonyWideOrder{Side: Bid, Item: IronOre, Qty: 20, Price: 4})
	os := wideOrdersOf(w, Bid, IronOre)
	if len(os) != 1 || os[0].Depot != silo || os[0].Qty != 7 {
		t.Fatalf("orders %+v; want one bid for 7 (what $30 buys at $4) at the silo", os)
	}
	assertMoneyConserved(t, w)
}

// The colony never trades with itself: its bid passes over its own ask to
// the next one. A colonist still may.
func TestTheColonyNeverTradesWithItself(t *testing.T) {
	w, silo, cs := marketWorld(t)
	a := ColonistOwner(cs[0].ID)
	stock(w, silo, Community, IronOre, 2)
	stock(w, silo, a, IronOre, 2)
	w.post(Ask, IronOre, 2, 3, Community, silo, 0)
	w.post(Ask, IronOre, 2, 4, a, silo, 0)
	o, n := w.post(Bid, IronOre, 3, 5, Community, silo, 0)
	if n != 2 || o.Qty != 1 {
		t.Fatalf("filled %d, %d left; want the colonist's 2 bought and 1 resting", n, o.Qty)
	}
	if w.openQty(Ask, IronOre, silo, Community) != 2 {
		t.Fatal("the colony's own ask traded")
	}
	stock(w, silo, a, IronOre, 1)
	w.post(Ask, IronOre, 1, 9, a, silo, 0)
	if _, n := w.post(Bid, IronOre, 1, 9, a, silo, 0); n != 1 {
		t.Fatal("a colonist could not buy back its own ask")
	}
	assertMoneyConserved(t, w)
}

func TestAColonyWideOrderIsValidated(t *testing.T) {
	w, _, _, _ := wideWorld(t)
	for _, c := range []SetColonyWideOrder{
		{Side: Bid, Item: Meal, Qty: 0, Price: 5},
		{Side: Bid, Item: Meal, Qty: maxColonyOrderQty + 1, Price: 5},
		{Side: Ask, Item: Meal, Qty: 5, Price: 0},
		{Side: Ask, Item: ColonistCorpse, Qty: 5, Price: 5},
	} {
		if w.setColonyWideOrder(c) {
			t.Errorf("%+v was accepted", c)
		}
	}
	if len(w.wideOrders()) != 0 {
		t.Fatal("a refused order was recorded")
	}
}

package sim

import "testing"

// A player's bid is paid from the treasury and rests; repricing it re-posts
// it at the new price with the escrow trued up; cancelling refunds it.
func TestAColonyBidIsPlacedRepricedAndCancelled(t *testing.T) {
	w, silo, _ := marketWorld(t)
	before := w.treasury
	if !w.placeColonyOrder(PlaceColonyOrder{Side: Bid, Item: SteelIngot, Qty: 10, Price: 7, Depot: silo}) {
		t.Fatalf("bid refused; log: %v", w.log.tail(1))
	}
	o := w.sortedOrders(func(o *Order) bool { return o.Actor == Community && o.Item == SteelIngot })
	if len(o) != 1 || !o[0].manual || o[0].Qty != 10 || o[0].Price != 7 || o[0].Expires != 0 {
		t.Fatalf("orders = %+v, want one manual bid of 10 at $7 that never expires", o)
	}
	if w.treasury != before-70 || w.moneyEscrowed() != 70 {
		t.Fatalf("treasury %v (was %v), escrow %v", w.treasury, before, w.moneyEscrowed())
	}
	assertMoneyConserved(t, w)

	if !w.repriceColonyOrder(RepriceColonyOrder{ID: o[0].ID, Price: 9}) {
		t.Fatalf("reprice refused; log: %v", w.log.tail(1))
	}
	if w.orders[o[0].ID] != nil {
		t.Fatal("the old order is still on the book")
	}
	n := w.sortedOrders(func(o *Order) bool { return o.Actor == Community && o.Item == SteelIngot })
	if len(n) != 1 || n[0].Price != 9 || n[0].Qty != 10 || !n[0].manual {
		t.Fatalf("after reprice: %+v", n)
	}
	if w.treasury != before-90 || w.moneyEscrowed() != 90 {
		t.Fatalf("treasury %v, escrow %v after repricing to $9", w.treasury, w.moneyEscrowed())
	}
	assertMoneyConserved(t, w)

	if !w.cancelColonyOrder(n[0].ID) {
		t.Fatal("cancel refused")
	}
	if w.treasury != before || w.moneyEscrowed() != 0 || len(w.orders) != 0 {
		t.Fatalf("treasury %v (was %v), escrow %v, %d orders open", w.treasury, before, w.moneyEscrowed(), len(w.orders))
	}
	assertMoneyConserved(t, w)
}

// An order the colony cannot cover is refused whole, and nothing moves.
func TestAColonyOrderMustBeCovered(t *testing.T) {
	w, silo, _ := marketWorld(t)
	before := w.treasury
	if w.placeColonyOrder(PlaceColonyOrder{Side: Bid, Item: IronOre, Qty: 1, Price: before + 1, Depot: silo}) {
		t.Fatal("a bid beyond the treasury was placed")
	}
	stock(w, silo, Community, Meal, 3)
	if w.placeColonyOrder(PlaceColonyOrder{Side: Ask, Item: Meal, Qty: 4, Price: 5, Depot: silo}) {
		t.Fatal("an ask for more than the colony holds was placed")
	}
	if w.placeColonyOrder(PlaceColonyOrder{Side: Ask, Item: Meal, Qty: 1, Price: 5, Depot: Point{1, 1}}) {
		t.Fatal("an order at no depot was placed")
	}
	if w.placeColonyOrder(PlaceColonyOrder{Side: Ask, Item: ColonistCorpse, Qty: 1, Price: 5, Depot: silo}) {
		t.Fatal("a colonist's body was put up for sale")
	}
	if w.treasury != before || len(w.orders) != 0 {
		t.Fatalf("a refused order moved something: treasury %v, %d orders", w.treasury, len(w.orders))
	}
	// A bid repriced past what the treasury and its escrow can cover stays.
	if !w.placeColonyOrder(PlaceColonyOrder{Side: Bid, Item: IronOre, Qty: 1, Price: 10, Depot: silo}) {
		t.Fatal("bid refused")
	}
	id := w.sortedOrders(nil)[0].ID
	if w.repriceColonyOrder(RepriceColonyOrder{ID: id, Price: before + 1}) {
		t.Fatal("repriced beyond the treasury")
	}
	if o := w.orders[id]; o == nil || o.Price != 10 {
		t.Fatal("a refused reprice touched the order")
	}
	assertMoneyConserved(t, w)
}

// A player's ask sells to a resting bid at once, and repricing one into the
// book trades it.
func TestAColonyAskTradesAgainstTheBook(t *testing.T) {
	w, silo, cs := marketWorld(t)
	buyer := ColonistOwner(cs[0].ID)
	stock(w, silo, Community, Meal, 5)
	w.post(Bid, Meal, 2, 6, buyer, silo, 0)
	before := w.treasury
	if !w.placeColonyOrder(PlaceColonyOrder{Side: Ask, Item: Meal, Qty: 5, Price: 8, Depot: silo}) {
		t.Fatal("ask refused")
	}
	if w.storageContainers[silo].held(buyer, Meal) != 0 {
		t.Fatal("an $8 ask sold to a $6 bid")
	}
	ask := w.sortedOrders(func(o *Order) bool { return o.Actor == Community })[0]
	if !w.repriceColonyOrder(RepriceColonyOrder{ID: ask.ID, Price: 6}) {
		t.Fatal("reprice refused")
	}
	if got := w.storageContainers[silo].held(buyer, Meal); got != 2 {
		t.Fatalf("the buyer holds %d meals, want 2", got)
	}
	if w.treasury != before+12 || w.openQty(Ask, Meal, silo, Community) != 3 {
		t.Fatalf("treasury %v (was %v), %d still on offer", w.treasury, before, w.openQty(Ask, Meal, silo, Community))
	}
	if !w.storageContainers[silo].ledgerBalanced() {
		t.Fatal("ledger unbalanced")
	}
	assertMoneyConserved(t, w)
}

// The colony's upkeep leaves a player's orders alone: the silo moving does not
// retire them, and withdrawing the colony's meal asks to haul or reprice them
// skips them.
func TestUpkeepLeavesManualOrdersAlone(t *testing.T) {
	w, silo, _ := marketWorld(t)
	stock(w, silo, Community, Meal, 4)
	if !w.placeColonyOrder(PlaceColonyOrder{Side: Ask, Item: Meal, Qty: 2, Price: 50, Depot: silo}) {
		t.Fatal("ask refused")
	}
	if !w.placeColonyOrder(PlaceColonyOrder{Side: Bid, Item: Clay, Qty: 3, Price: 4, Depot: silo}) {
		t.Fatal("bid refused")
	}
	w.post(Ask, Meal, 2, 9, Community, silo, 0) // the colony's own
	w.withdrawColonyAsks(Meal, silo)
	if got := w.openQty(Ask, Meal, silo, Community); got != 2 {
		t.Fatalf("%d meals on offer after withdrawing the colony's asks, want the player's 2", got)
	}

	w.tick = marketInterval
	w.runMarket()
	nearer := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(nearer, Storage)
	w.refreshSpatial()
	w.tick += marketInterval
	w.runMarket()
	manual := w.sortedOrders(func(o *Order) bool { return o.Depot == silo && o.manual })
	if len(manual) != 2 {
		t.Fatalf("%d manual orders left at the old silo, want 2", len(manual))
	}
	for _, o := range w.sortedOrders(func(o *Order) bool { return o.Depot == silo && !o.manual && o.Actor == Community }) {
		t.Errorf("the colony's own %v of %v still open at the old silo", o.Side, o.Item)
	}
	assertMoneyConserved(t, w)
}

// Only the colony's orders can be repriced or cancelled this way.
func TestOnlyColonyOrdersAreTouched(t *testing.T) {
	w, silo, cs := marketWorld(t)
	o, _ := w.post(Bid, Meal, 1, 5, ColonistOwner(cs[0].ID), silo, 0)
	if w.cancelColonyOrder(o.ID) || w.repriceColonyOrder(RepriceColonyOrder{ID: o.ID, Price: 6}) {
		t.Fatal("a colonist's order was changed as the colony's")
	}
	if w.orders[o.ID] == nil || o.Price != 5 {
		t.Fatal("the colonist's order moved")
	}
}

func TestParseItemKindRoundTrips(t *testing.T) {
	for _, k := range TradableItems() {
		if got, ok := ParseItemKind(k.String()); !ok || got != k {
			t.Errorf("ParseItemKind(%q) = %v, %v", k.String(), got, ok)
		}
	}
	if _, ok := ParseItemKind("unobtainium"); ok {
		t.Error("parsed an unknown item")
	}
}

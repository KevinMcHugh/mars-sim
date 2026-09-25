package sim

import "sort"

// ---- Hauling and arbitrage ---------------------------------------------------------
//
// Orders only match within a depot, so a bid at one depot and a cheaper ask
// at another never cross by themselves. Closing that gap is work, done two
// ways:
//
//   - Arbitrage, on a colonist's own account: the producer planner (see
//     producer.go) treats moving goods as one more recipe — buy at the cheap
//     depot, carry, sell into the dear one's bid — and takes it on when the
//     spread pays for the walk. The goods are the hauler's in transit, and so
//     is the risk that the bid is gone when it arrives.
//   - Hauling for hire: a WorkHaul order pays per unit to move the issuer's
//     goods from one depot to another. The goods stay the issuer's
//     throughout (the cargo record says so). The colony uses it to keep meals
//     at its silo.
//
// The colony is a seller too: what it bought at the silo beyond a reserve for
// public works, it offers back at a markup, which is what lets the founding
// grant work as capital rather than run down. See docs/hauling.md.

// ---- Arbitrage -------------------------------------------------------------------

// cheapestAskElsewhere finds the best ask for bid b's item at any other depot
// e can reach and use, from anyone but e and b's bidder, below b's price.
// Ties break by position.
func (w *World) cheapestAskElsewhere(e *Entity, b *Order) (*Order, Point, bool) {
	me := ColonistOwner(e.ID)
	room := w.roomOf(e.Pos)
	var best *Order
	var at Point
	for key, bk := range w.books {
		if key.Item != b.Item || key.Depot == b.Depot || len(bk.asks) == 0 {
			continue
		}
		ask := bk.asks[0]
		if ask.Actor == me || ask.Actor == b.Actor || ask.Price >= b.Price {
			continue
		}
		if best != nil && (ask.Price > best.Price || (ask.Price == best.Price && !lessPoint(key.Depot, at))) {
			continue
		}
		if !w.canUseFixture(e, key.Depot) || !w.taskReachable(key.Depot, room) {
			continue
		}
		best, at = ask, key.Depot
	}
	return best, at, best != nil
}

// planArbitrage buys at src and takes the goods to bid b, if the spread pays
// for the walk.
func (w *World) planArbitrage(e *Entity, b, ask *Order, src Point) bool {
	me := ColonistOwner(e.ID)
	qty := min(ask.Qty, b.Qty-w.plannedQty(b))
	for qty > 0 && !e.Inventory.CanAdd(b.Item, qty) {
		qty--
	}
	if qty <= 0 || e.wallet < Money(qty)*ask.Price {
		return false
	}
	walk := e.Pos.Chebyshev(src) + src.Chebyshev(b.Depot)
	if Money(qty)*(b.Price-ask.Price)-w.laborCost(walk) < Money(w.cfg.PlanMinProfit) {
		return false
	}
	o, filled := w.post(Bid, b.Item, qty, ask.Price, me, src, 0)
	if o != nil && o.Qty > 0 {
		w.cancel(o)
	}
	if filled == 0 {
		return false
	}
	p := w.newPlan(e, planHaul, b, filled)
	p.workshop, p.crafted = src, true
	w.remember(e, event(EvtWentToMarket, "Bought %d %s at (%d, %d) for %v to sell at (%d, %d) for %v.",
		filled, b.Item, src.X, src.Y, ask.Price, b.Depot.X, b.Depot.Y, b.Price))
	w.assignCarry(e, p, src, carryFetch, filled)
	return true
}

// ---- Hauling for hire ---------------------------------------------------------------

// tryAssignHaul takes on the oldest open haul order e can do: its goods are
// at the source, both depots are reachable and usable, and nobody else has
// it. A colonist already carrying goods for one delivers them first.
func (w *World) tryAssignHaul(e *Entity) bool {
	room := w.roomOf(e.Pos)
	for _, o := range w.sortedWork(func(o *WorkOrder) bool { return o.Kind == WorkHaul }) {
		if e.Inventory.Count(o.Item) > 0 && e.cargo[o.Item] == o.Issuer {
			w.startHaul(e, o, carryDeliver, e.Inventory.Count(o.Item))
			return true
		}
		if id := w.haulClaims[o.ID]; id != 0 && id != e.ID {
			continue
		}
		src := w.storageContainers[o.From]
		if src == nil || !w.canUseFixture(e, o.From) || !w.canUseFixture(e, o.Pos) ||
			!w.taskReachable(o.From, room) || !w.taskReachable(o.Pos, room) {
			continue
		}
		n := min(o.Units, src.held(o.Issuer, o.Item))
		for n > 0 && !e.Inventory.CanAdd(o.Item, n) {
			n--
		}
		if n <= 0 {
			continue
		}
		w.startHaul(e, o, carryFetch, n)
		return true
	}
	return false
}

// startHaul sets e carrying n units for haul order o.
func (w *World) startHaul(e *Entity, o *WorkOrder, stage carryStage, n int) {
	w.haulClaims[o.ID] = e.ID
	target := o.From
	if stage == carryDeliver {
		target = o.Pos
	}
	e.Job, e.Target, e.Progress = JobCarry, target, 0
	e.carry, e.carryItem, e.carryQty, e.carryTo = stage, o.Item, n, o.Pos
	e.carryFor, e.carryWork, e.carryPrice = o.Issuer, o.ID, 0
}

// finishHaul pays e for the n units it just delivered on its haul order.
func (w *World) finishHaul(e *Entity, n int) {
	o := w.workOrders[e.carryWork]
	for i := 0; i < n && o != nil && w.workOrders[o.ID] == o; i++ {
		w.payWork(o, e)
	}
	w.remember(e, event(EvtHauled, "Hauled %d %s for hire.", n, e.carryItem))
	w.clearJob(e)
}

// refreshSiloStock keeps silo-meal-stock of the colony's meals at its silo,
// posting haul orders to bring them in from its scumhouses (nearest first)
// for what is missing and not already on its way.
func (w *World) refreshSiloStock() {
	silo, ok := w.marketDepot()
	if !ok || w.cfg.SiloMealStock <= 0 {
		return
	}
	coming := map[Point]int{} // open haul units by source
	total := 0
	for _, o := range w.workOrders {
		if o.Kind == WorkHaul && o.Issuer == Community && o.Item == Meal && o.Pos == silo {
			coming[o.From] += o.Units
			total += o.Units
		}
	}
	want := w.cfg.SiloMealStock - w.storageContainers[silo].held(Community, Meal) - total
	if want <= 0 {
		return
	}
	houses := make([]Point, 0, len(w.facilityTiles[Scumhouse]))
	for p := range w.facilityTiles[Scumhouse] {
		houses = append(houses, p)
	}
	sort.Slice(houses, func(i, j int) bool {
		di, dj := houses[i].Chebyshev(silo), houses[j].Chebyshev(silo)
		if di != dj {
			return di < dj
		}
		return lessPoint(houses[i], houses[j])
	})
	for _, h := range houses {
		c := w.storageContainers[h]
		if c == nil || want <= 0 {
			continue
		}
		n := min(want, c.held(Community, Meal)-coming[h])
		if n <= 0 {
			continue
		}
		if o := w.postWork(WorkHaul, Community, Money(w.cfg.HaulPay), n, silo); o != nil {
			o.From, o.Item = h, Meal
			want -= n
		}
	}
}

// ---- The colony as seller ----------------------------------------------------------

// colonyAskPrice is what the colony asks for a good it bought: its reference
// price plus colony-markup, and always at least a dollar over it, so the
// colony never sells at what it pays.
func (w *World) colonyAskPrice(k ItemKind) Money {
	ref := w.refPrice(k)
	return max(ref+1, ref*Money(100+max(0, w.cfg.ColonyMarkup))/100)
}

// refreshColonyAsks offers, at the silo, whatever the colony holds of each
// good it buys beyond colony-stock-reserve units — the reserve is what public
// works draw on (see construction.go).
func (w *World) refreshColonyAsks() {
	silo, ok := w.marketDepot()
	if !ok || !w.cfg.ColonySells {
		return
	}
	c := w.storageContainers[silo]
	for _, k := range prospectingGoods {
		if w.refPrice(k) <= 0 {
			continue
		}
		if spare := c.held(Community, k) - w.cfg.ColonyStockReserve; spare > 0 {
			w.post(Ask, k, spare, w.colonyAskPrice(k), Community, silo, 0)
		}
	}
}

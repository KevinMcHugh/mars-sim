package sim

import "testing"

// arbitrageWorld is producerWorld with a second communal chest at (5, 15) —
// farther from the map center than the silo, so not the silo itself — and
// the colony holding iron at its silo: the setting for a price gap.
func arbitrageWorld(t *testing.T, sells bool) (w *World, silo, far Point, cols []*Entity) {
	t.Helper()
	w, _, silo, cols = producerWorld(t, 3)
	w.cfg.ColonySells, w.cfg.ColonyStockReserve = sells, 0
	far = Point{5, 15}
	w.SetTerrain(far, Storage)
	w.refreshSpatial()
	c := w.storageContainers[silo]
	c.Inventory.Add(IronOre, 12)
	c.credit(Community, IronOre, 12)
	return w, silo, far, cols
}

// stepFed steps w n times, keeping every colonist's needs at zero so a test
// is about work, not survival, and checks the money audit on every tick.
func stepFed(t *testing.T, w *World, n int, stop func() bool) {
	t.Helper()
	for i := 0; i < n && (stop == nil || !stop()); i++ {
		w.step()
		for _, e := range w.entities {
			if e.Kind == Colonist {
				for k := range e.Needs {
					e.Needs[k] = 0
				}
			}
		}
		assertMoneyConserved(t, w)
	}
}

// The colony offers what it bought beyond its reserve, above what it paid.
func TestColonySellsBeyondItsReserve(t *testing.T) {
	w, silo, _, _ := arbitrageWorld(t, true)
	w.cfg.ColonyStockReserve = 8
	w.refreshColonyAsks()
	ask, ok := w.bestAsk(IronOre, silo)
	if !ok || ask.Actor != Community || ask.Qty != 4 {
		t.Fatalf("colony ask %+v, want 4 iron (12 held, 8 kept back)", ask)
	}
	if ask.Price <= w.refPrice(IronOre) {
		t.Fatalf("colony asks %v for iron it bids %v for", ask.Price, w.refPrice(IronOre))
	}
	w.refreshColonyAsks()
	if got := w.openQty(Ask, IronOre, silo, Community); got != 4 {
		t.Fatalf("refreshing again left %d iron on offer, want still 4", got)
	}
}

// The E7 gate, first half: with iron cheap at the silo and dear at another
// depot, a colonist hauls it across unprompted — buying from the colony,
// carrying, selling into the bid — and the gap closes.
func TestArbitrageClosesAPriceGap(t *testing.T) {
	w, silo, far, cols := arbitrageWorld(t, true)
	buyer := ColonistOwner(cols[0].ID)
	if o, _ := w.post(Bid, IronOre, 8, 12, buyer, far, 0); o == nil {
		t.Fatal("the buyer could not post its bid")
	}
	bought := func() bool { return w.storageContainers[far].held(buyer, IronOre) >= 8 }
	stepFed(t, w, 3000, bought)
	if !bought() {
		t.Fatalf("the buyer holds %d iron at the far depot, want 8", w.storageContainers[far].held(buyer, IronOre))
	}
	if b, ok := w.bestBid(IronOre, far); ok && b.Price >= 12 {
		t.Fatalf("a bid at %v is still open at the far depot: the gap never closed", b.Price)
	}
	soldByColony := false
	for _, tr := range w.trades {
		soldByColony = soldByColony || (tr.Item == IronOre && tr.Depot == silo && tr.Seller == Community)
	}
	if !soldByColony {
		t.Fatal("the iron did not come from the colony's stock at the silo")
	}
}

// The E7 gate, second half: over a long run, a colony that sells what it
// bought ends with more money than one that only buys.
func TestColonySellingPaysOverALongRun(t *testing.T) {
	run := func(sells bool) Money {
		w, _, far, cols := arbitrageWorld(t, sells)
		w.post(Bid, IronOre, 8, 12, ColonistOwner(cols[0].ID), far, 0)
		stepFed(t, w, 4000, nil)
		return w.treasury
	}
	selling, buying := run(true), run(false)
	if selling <= buying {
		t.Fatalf("treasury %v selling vs %v only buying: selling should pay", selling, buying)
	}
}

// The colony hires haulers to keep meals at its silo, and pays per unit.
func TestHaulersStockTheSilo(t *testing.T) {
	w, house, silo, cols := producerWorld(t, 2)
	w.cfg.SiloMealStock = 3
	c := w.storageContainers[house]
	c.Inventory.Add(Meal, 5)
	c.credit(Community, Meal, 5)
	before := map[EntityID]Money{}
	for _, e := range cols {
		before[e.ID] = e.wallet
	}
	stocked := func() bool { return w.storageContainers[silo].held(Community, Meal) >= 3 }
	stepFed(t, w, 2000, stocked)
	if !stocked() {
		t.Fatalf("the silo holds %d of the colony's meals, want 3", w.storageContainers[silo].held(Community, Meal))
	}
	if got := c.held(Community, Meal); got != 2 {
		t.Fatalf("the scumhouse holds %d of the colony's meals, want the 2 not needed", got)
	}
	for _, o := range w.workOrders {
		if o.Kind == WorkHaul {
			t.Fatalf("haul order %+v still open after the silo was stocked", o)
		}
	}
	var earned Money
	for _, e := range cols {
		earned += e.wallet - before[e.ID]
	}
	if earned < 3*Money(w.cfg.HaulPay) {
		t.Fatalf("haulers earned %v, want at least %v", earned, 3*Money(w.cfg.HaulPay))
	}
}

// With construction costs on, a public work is built from the colony's own
// stock before the builder's.
func TestPublicWorksUseTheColonysStock(t *testing.T) {
	w, silo, _, cols := arbitrageWorld(t, true)
	w.cfg.ConstructionCosts = true
	c := w.storageContainers[silo]
	c.Inventory.Add(RawRock, 2)
	c.credit(Community, RawRock, 2)
	e := cols[0]
	if w.canAffordBuild(e, Storage, Owner{}) {
		t.Fatal("a builder with nothing could afford a chest on its own account")
	}
	if !w.canAffordBuild(e, Storage, Community) {
		t.Fatal("a builder could not afford a public chest from the colony's stock")
	}
	e.task = &buildTask{pos: Point{12, 12}, terrain: Storage, proj: &project{issuer: Community}}
	e.Job, e.BuildKind = JobBuild, Storage
	for i := 0; i < 200; i++ {
		if ready, ok := w.gatherBuildMaterials(e); ready || !ok {
			break
		}
	}
	if missing := missingMaterials(e, w.buildCost(Storage)); len(missing) > 0 {
		t.Fatalf("the builder still lacks %v", missing)
	}
	if got := c.held(Community, IronOre); got != 11 {
		t.Fatalf("the colony holds %d iron at the silo, want 11 after one went to the chest", got)
	}
	if e.cargo[IronOre] != Community {
		t.Fatal("the fetched iron is not marked the colony's until it is built with")
	}
}

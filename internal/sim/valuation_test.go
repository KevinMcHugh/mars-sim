package sim

import "testing"

// producerWorld is scumhouseWorld with a silo at (6, 6), cave scum on the rock
// along the west wall, and n colonists, none of them hungry. The colony makes
// no food of its own (meal-reserve 0) and posts no bids for biomatter, so any
// food work that happens is somebody filling a customer's bid.
func producerWorld(t *testing.T, n int) (w *World, house, silo Point, cols []*Entity) {
	t.Helper()
	w, house = scumhouseWorld(t, false)
	w.cfg.MealReserve, w.cfg.ScumhouseBidQty = 0, 0
	silo = Point{6, 6}
	w.SetTerrain(silo, Storage)
	for y := 5; y <= 15; y++ {
		p := Point{4, y}
		w.scum[p] = scumPatch{amount: w.cfg.ScumMax, since: w.tick}
		w.refreshScumExposure(p)
	}
	w.refreshSpatial()
	for i := 0; i < n; i++ {
		e := w.spawn(Colonist, Point{10 + 2*i, 12})
		for k := range e.Needs {
			e.Needs[k] = 0
		}
		cols = append(cols, e)
	}
	return w, house, silo, cols
}

// The E6 gate: a bid for a finished good nobody has in stock produces trades
// two recipe links down with no scripted help. A customer bids for a meal; a
// cook's planner bids for the scum to make it; a scraper's planner fills that
// bid off the cave wall; the cook cooks and sells the customer the meal.
func TestAMealBidReachesTheCaveWall(t *testing.T) {
	w, house, silo, cols := producerWorld(t, 3)
	customer := ColonistOwner(cols[0].ID)
	if o, _ := w.post(Bid, Meal, 1, 15, customer, silo, 0); o == nil {
		t.Fatal("the customer could not post its bid")
	}
	scumTraded, mealTraded, depth := false, false, 0
	for i := 0; i < 3000 && !mealTraded; i++ {
		w.step()
		for _, e := range w.entities {
			if e.Kind == Colonist {
				for k := range e.Needs {
					e.Needs[k] = 0 // keep the test about work, not survival
				}
			}
		}
		depth = max(depth, w.chainDepth())
		for _, tr := range w.trades {
			scumTraded = scumTraded || (tr.Item == CaveScum && tr.Depot == house)
			mealTraded = mealTraded || (tr.Item == Meal && tr.Depot == silo && tr.Buyer == customer)
		}
		assertMoneyConserved(t, w)
	}
	if !scumTraded {
		t.Fatal("no scum ever traded at the scumhouse: the cook's derived bid was never filled")
	}
	if !mealTraded {
		t.Fatal("the customer never bought its meal")
	}
	if depth < 2 {
		t.Fatalf("deepest plan was %d links below the meal bid, want 2", depth)
	}
	if got := w.storageContainers[silo].held(customer, Meal); got != 1 {
		t.Fatalf("the customer holds %d meals at the silo, want 1", got)
	}
}

// An item's remembered price starts at its reference value, jumps to its
// first trade, then moves an eighth of the way toward each later one.
func TestPricesRememberTrades(t *testing.T) {
	w, _, _, _ := producerWorld(t, 0)
	if got := w.valueOf(Meal); got != w.refPrice(Meal) {
		t.Fatalf("untraded meal worth %v, want its reference %v", got, w.refPrice(Meal))
	}
	if got := w.valueOf(CaveScum); got != Money(w.cfg.PriceCaveScum) {
		t.Fatalf("untraded scum worth %v, want %v", got, w.cfg.PriceCaveScum)
	}
	w.recordPrice(Meal, 13)
	if got := w.valueOf(Meal); got != 13 {
		t.Fatalf("after one trade at 13 a meal is worth %v", got)
	}
	w.recordPrice(Meal, 5)
	if got := w.valueOf(Meal); got != 12 { // 13 + (5-13)/8
		t.Fatalf("after a trade at 5 a meal is worth %v, want 12", got)
	}
}

// A hungrier colonist bids more for a meal, and never more than it has.
func TestHungerRaisesTheMealBid(t *testing.T) {
	w, _, _, cols := producerWorld(t, 1)
	e := cols[0]
	max := w.cfg.Needs[NeedFood].Max
	e.Needs[NeedFood] = 0
	fed := w.mealBidLimit(e)
	e.Needs[NeedFood] = max / 2
	peckish := w.mealBidLimit(e)
	e.Needs[NeedFood] = max
	starving := w.mealBidLimit(e)
	if !(fed < peckish && peckish < starving) {
		t.Fatalf("bid limits fed %v, peckish %v, starving %v: want strictly rising", fed, peckish, starving)
	}
	if want := w.refPrice(Meal) * Money(w.cfg.MealWillingness); starving != want {
		t.Fatalf("a starving colonist bids %v, want %v", starving, want)
	}
	e.wallet = 4
	if got := w.mealBidLimit(e); got != 4 {
		t.Fatalf("a starving colonist with $4 bids %v", got)
	}
	e.Needs[NeedFood] = max / 2
	if got := w.mealBidLimit(e); got != 2 {
		t.Fatalf("a peckish colonist with $4 bids %v, want half its money", got)
	}
}

// With nothing on offer, a hungry colonist's bid rests as demand — one at a
// time — and expires after demand-ttl.
func TestHungryBidRestsAsDemand(t *testing.T) {
	w, _, silo, cols := producerWorld(t, 1)
	e := cols[0]
	e.Needs[NeedFood] = w.cfg.Needs[NeedFood].Max
	me := ColonistOwner(e.ID)
	if w.tryBuyMeal(e) {
		t.Fatal("bought a meal nobody sells")
	}
	w.tryBuyMeal(e)
	if got := w.openQty(Bid, Meal, silo, me); got != 1 {
		t.Fatalf("%d units of meal bid resting, want 1", got)
	}
	w.tick += w.cfg.DemandTTL
	w.expireOrders()
	if got := w.openQty(Bid, Meal, silo, me); got != 0 {
		t.Fatalf("the demand bid outlived demand-ttl: %d open", got)
	}
	assertMoneyConserved(t, w)
}

// A plan's derived bids go when the bid it served goes: nobody should be
// asked for scum to make a meal nobody wants any more.
func TestDerivedBidsDieWithTheirPlan(t *testing.T) {
	w, house, silo, cols := producerWorld(t, 2)
	customer, cook := cols[0], cols[1]
	target, _ := w.post(Bid, Meal, 1, 15, ColonistOwner(customer.ID), silo, 0)
	started, planned := w.planCraft(cook, target)
	if !planned || started {
		t.Fatalf("planCraft: started %v, planned %v; want a plan waiting on scum", started, planned)
	}
	p := w.plans[cook.plan]
	if p == nil || len(p.derived) != 1 {
		t.Fatalf("plan %+v, want one derived bid", p)
	}
	derived := w.orders[p.derived[0]]
	if derived == nil || derived.Item != CaveScum || derived.Depot != house || derived.depth != 1 {
		t.Fatalf("derived bid %+v, want a depth-1 scum bid at the scumhouse", derived)
	}
	w.cancel(target)
	w.prunePlans()
	if w.orders[derived.ID] != nil || w.plans[p.id] != nil || cook.plan != 0 {
		t.Fatal("the derived bid or its plan outlived the bid it served")
	}
	assertMoneyConserved(t, w)
}

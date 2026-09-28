package sim

import "testing"

// scumBidWorld is a floor with scum patches beside a colonist, a scumhouse
// next to them and another across the map, and the colony bidding for scum
// at each: more across the map than next door.
func scumBidWorld(t *testing.T) (w *World, e *Entity, near, far *Order) {
	t.Helper()
	w = propertyWorld(t)
	noScum(w)
	for _, p := range []Point{{6, 10}, {6, 11}} {
		w.scum[p] = scumPatch{amount: w.cfg.ScumMax, since: w.tick}
		w.refreshScumExposure(p)
	}
	w.SetTerrain(Point{8, 10}, Scumhouse)
	w.SetTerrain(Point{22, 5}, Scumhouse)
	w.refreshSpatial()
	e = w.spawn(Colonist, Point{7, 10})
	near, _ = w.post(Bid, CaveScum, 3, 3, Community, Point{8, 10}, 0)
	far, _ = w.post(Bid, CaveScum, 3, 4, Community, Point{22, 5}, 0)
	return w, e, near, far
}

// A colonist takes the plan that pays best per tick of its time, not the
// first that pays at all: the dearer bid across the map loses to the cheaper
// one next to the scum.
func TestThePlannerTakesTheBestRate(t *testing.T) {
	w, e, near, far := scumBidWorld(t)
	var o planOffer
	if !w.planGather(e, far, &o) {
		t.Fatal("the far bid doesn't pay at all, so this test proves nothing")
	}
	if !w.tryAssignProduce(e) {
		t.Fatal("no plan taken")
	}
	if p := w.plans[e.plan]; p == nil || p.target != near.ID {
		t.Fatalf("took a plan for bid %v, want the nearer bid %d (the far one is %d)", p, near.ID, far.ID)
	}
}

// A bid another colonist already means to fill is still open to the next:
// they compete for it, and the first to deliver fills it.
func TestColonistsCompeteForABid(t *testing.T) {
	w, a, near, _ := scumBidWorld(t)
	b := w.spawn(Colonist, Point{7, 11})
	if !w.tryAssignProduce(a) || !w.tryAssignProduce(b) {
		t.Fatal("both colonists should take a plan")
	}
	pa, pb := w.plans[a.plan], w.plans[b.plan]
	if pa == nil || pb == nil || pa.target != near.ID || pb.target != near.ID {
		t.Fatalf("plans %v and %v; want both after the near bid %d", pa, pb, near.ID)
	}
}

// What a colonist reckons its time is worth: labor-price, or what it has
// been earning if that's more, fading back to labor-price over rate-memory
// ticks without earning there again.
func TestReservationFadesBackToLaborPrice(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.LaborPrice, w.cfg.RateMemory = 2, 1000
	e := w.spawn(Colonist, Point{8, 8})
	base := int64(2000)
	if got := w.reservation(e); got != base {
		t.Fatalf("with no earnings: %d, want %d", got, base)
	}
	w.tick = 100
	w.noteEarnings(e, SkillSmithing, 10_000)
	if got := w.reservation(e); got != 10_000 {
		t.Fatalf("just after earning $10 per 100 ticks: %d, want 10000", got)
	}
	w.tick = 600
	if got := w.reservation(e); got != base+(10_000-base)/2 {
		t.Fatalf("halfway through rate-memory: %d, want %d", got, base+(10_000-base)/2)
	}
	w.tick = 1100
	if got := w.reservation(e); got != base {
		t.Fatalf("after rate-memory: %d, want %d", got, base)
	}
	if got, cheap := w.laborCostFor(e, 100), w.laborCost(100); got != cheap {
		t.Fatalf("labor at a faded reservation: %v, want labor-price's %v", got, cheap)
	}
}

// A colonist whose time is worth more to it passes on work that pays less.
func TestAWellPaidColonistPassesOnPoorWork(t *testing.T) {
	w, e, _, _ := scumBidWorld(t)
	w.tick = 10
	w.noteEarnings(e, SkillSmithing, 500_000) // $500 per 100 ticks
	if w.tryAssignProduce(e) {
		t.Fatalf("took plan %v for scum while its time is worth $500 per 100 ticks", w.plans[e.plan])
	}
}

package sim

import "testing"

// incubatorWorld is scumhouseWorld with an incubator at (14, 6).
func incubatorWorld(t *testing.T) (w *World, house, inc Point) {
	t.Helper()
	w, house = scumhouseWorld(t, false)
	inc = Point{14, 6}
	w.SetTerrain(inc, Incubator)
	w.refreshSpatial()
	return w, house, inc
}

// seed puts n units of the colony's scum in the incubator at p.
func seedIncubator(w *World, p Point, n int) {
	c := w.storageContainers[p]
	c.Inventory.AddAll(ItemStack{CaveScum, n})
	c.credit(Community, CaveScum, n)
}

// An incubator holding its seed grows a unit every incubator-grow-ticks, to its
// capacity and no further; one with no seed grows nothing.
func TestAnIncubatorGrowsScumOnSchedule(t *testing.T) {
	w, _, inc := incubatorWorld(t)
	w.cfg.IncubatorGrowTicks, w.cfg.IncubatorCapacity, w.cfg.IncubatorSeed = 10, 6, 2
	c := w.storageContainers[inc]

	for i := 0; i < 100; i++ {
		w.step()
	}
	if n := c.Inventory.Count(CaveScum); n != 0 {
		t.Fatalf("an unseeded incubator grew %d scum", n)
	}

	seedIncubator(w, inc, 2)
	start := w.tick
	for w.tick < start+25 {
		w.step()
	}
	if n := c.Inventory.Count(CaveScum); n != 4 {
		t.Fatalf("after 25 ticks the incubator holds %d, want its seed of 2 plus 2 grown", n)
	}
	for i := 0; i < 500; i++ {
		w.step()
	}
	if n := c.Inventory.Count(CaveScum); n != 6 {
		t.Fatalf("a long-running incubator holds %d, want its capacity of 6", n)
	}
	if !c.ledgerBalanced() || c.held(Community, CaveScum) != 6 {
		t.Fatalf("ledger = %+v, want 6 of the colony's scum, balanced", c.Ledger)
	}
}

// Seeding: a colonist scrapes a load, loads it, and the colony buys it outright.
func TestColonistsSeedAnIncubatorAndTheColonyBuysTheSeed(t *testing.T) {
	w, _, inc := incubatorWorld(t)
	w.cfg.IncubatorSeed = 2
	putScum(w, Point{8, 8}, 3)
	e := w.spawn(Colonist, Point{10, 8})
	purse := w.balance(ColonistOwner(e.ID))
	treasury := w.treasury

	if !w.tryAssignSeed(e) {
		t.Fatal("no seeding job for an unseeded incubator with scum in reach")
	}
	for i := 0; i < 400 && e.Job != JobNone; i++ {
		w.step()
	}
	c := w.storageContainers[inc]
	if got := c.held(Community, CaveScum); got < 2 {
		t.Fatalf("the colony holds %d scum in the incubator, want at least its seed (ledger %+v)", got, c.Ledger)
	}
	if w.balance(ColonistOwner(e.ID)) <= purse || w.treasury >= treasury {
		t.Fatalf("the colony did not pay for the seed: wallet %v -> %v, treasury %v -> %v",
			purse, w.balance(ColonistOwner(e.ID)), treasury, w.treasury)
	}
	if w.tryAssignSeed(e) {
		t.Fatal("a seeded incubator still wants more")
	}
}

// Harvesting: a colonist carries what the incubator has grown above its seed to
// a scumhouse, for the colony, and is paid for the trip.
func TestHarvestersCarryRipeScumToTheScumhouse(t *testing.T) {
	w, house, inc := incubatorWorld(t)
	w.cfg.IncubatorSeed = 2
	seedIncubator(w, inc, 6)
	e := w.spawn(Colonist, Point{10, 8})
	purse := w.balance(ColonistOwner(e.ID))

	if !w.tryAssignHarvest(e) {
		t.Fatal("no harvest job with 4 ripe units")
	}
	for i := 0; i < 400 && e.Job != JobNone; i++ {
		w.step()
	}
	ic, hc := w.storageContainers[inc], w.storageContainers[house]
	if got := ic.held(Community, CaveScum); got != 2 {
		t.Fatalf("the incubator holds %d, want its seed of 2 kept back", got)
	}
	if got := hc.held(Community, CaveScum); got != 4 {
		t.Fatalf("the scumhouse holds %d of the colony's scum, want 4 (ledger %+v)", got, hc.Ledger)
	}
	if w.balance(ColonistOwner(e.ID)) != purse+Money(w.cfg.WageHarvest) {
		t.Fatalf("wallet %v, want the harvest wage on top of %v", w.balance(ColonistOwner(e.ID)), purse)
	}
	if !ic.ledgerBalanced() || !hc.ledgerBalanced() {
		t.Fatal("a ledger no longer balances")
	}
	if w.tryAssignHarvest(e) {
		t.Fatal("nothing is ripe, and a harvest was assigned")
	}
}

// Wild scum: allowed until an incubator stands, then only in dire times.
func TestWildScumIsForDireTimesOnceIncubatorsStand(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	w.spawn(Colonist, Point{10, 8})
	if !w.wildScumAllowed() {
		t.Fatal("with no incubator, the colony has nothing but the rock to feed from")
	}

	inc := Point{14, 6}
	w.SetTerrain(inc, Incubator)
	w.refreshSpatial()
	w.cfg.ScumDireMeals = 1
	w.storageContainers[house].Inventory.AddAll(ItemStack{Meal, 5})
	w.storageContainers[house].credit(Community, Meal, 5)
	w.storedMealsTick = -1
	if w.wildScumAllowed() {
		t.Fatal("stores are fine and an incubator stands, but wild scum is allowed")
	}

	w.storageContainers[house].Inventory.Remove(Meal, 5)
	w.storageContainers[house].debit(Community, Meal, 0)
	w.storageContainers[house].Ledger = nil
	w.storedMealsTick = -1
	if !w.wildScumAllowed() {
		t.Fatal("the shelves are bare and nothing is ripe: that is dire, and wild scum should be allowed")
	}
	seedIncubator(w, inc, w.incubatorSeed()+2)
	if w.wildScumAllowed() {
		t.Fatal("ripe scum waits in the incubator, so it is not dire yet")
	}
}

// The routine food work does not scrape the rock once incubators stand, but a
// hungry colonist's own foraging still does.
func TestRoutineScrapingWaitsOnTheIncubator(t *testing.T) {
	w, house, _ := incubatorWorld(t)
	putScum(w, Point{8, 8}, 3)
	w.storageContainers[house].Inventory.AddAll(ItemStack{Meal, 30})
	w.storageContainers[house].credit(Community, Meal, 30)
	w.storedMealsTick = -1
	e := w.spawn(Colonist, Point{9, 8})

	if w.tryAssignScrape(e, false) {
		t.Fatal("the colony scraped wild scum for its stoves with an incubator standing and stores full")
	}
	if !w.tryAssignScrape(e, true) {
		t.Fatal("a colonist scraping to keep for itself was refused")
	}
}

// With nothing ordered, the colony plans and builds incubators for its stoves,
// and its food comes from them.
func TestTheColonyBuildsIncubatorsAndFeedsFromThem(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 36
	cfg.StartAliens = 0
	cfg.InfiniteFood = false
	w := newTestWorld(t, cfg)
	for i := 0; i < 3000; i++ {
		w.step()
	}
	if got, want := w.countTerrain(Incubator), w.desiredIncubators(); got != want {
		t.Fatalf("%d incubators built, want %d", got, want)
	}
	if w.ripeTotal() == 0 && w.scumInKitchens() == 0 {
		t.Fatal("the incubators have produced nothing for the kitchens")
	}
	if w.countKind(Colonist) != cfg.StartColonists {
		t.Fatalf("%d of %d colonists alive", w.countKind(Colonist), cfg.StartColonists)
	}
}

// putScum lays an exposed patch of n units on floor at p, with nothing else
// on the map.
func putScum(w *World, p Point, n int) {
	noScum(w)
	w.setScum(p, n)
	w.exposedScum[p] = struct{}{}
}

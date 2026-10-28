package sim

import "testing"

// poweredKitchen is scumhouseWorld with power on and the colony's goods in
// the stove's depot.
func poweredKitchen(t *testing.T, stock ...ItemStack) (*World, *StorageContainer) {
	t.Helper()
	w, house := scumhouseWorld(t, false)
	w.cfg.Power = true
	c := w.storageContainers[house]
	for _, s := range stock {
		c.Inventory.Add(s.Kind, s.Count)
		c.credit(Community, s.Kind, s.Count)
	}
	return w, c
}

// cookOnce has a new colonist cook at the kitchen until it leaves, and
// reports how many ticks it worked and whether it took the job at all.
func cookOnce(w *World) (ticks int, cooked bool) {
	cook := w.spawn(Colonist, Point{12, 10, LandingLevel})
	if !w.tryAssignCraft(cook) {
		return 0, false
	}
	for ; ticks < 1000 && cook.Job == JobCraft; ticks++ {
		w.jobCraft(cook)
	}
	return ticks, true
}

// A stove with a battery cooks as it always did, and spends the battery.
func TestAPoweredStoveSpendsABattery(t *testing.T) {
	w, c := poweredKitchen(t, ItemStack{CaveScum, 2}, ItemStack{Battery, 1})
	if _, ok := cookOnce(w); !ok {
		t.Fatal("no cooking job")
	}
	if c.held(Community, Battery) != 0 || c.held(Community, CaveScum) != 0 {
		t.Fatalf("after cooking: %+v", c.Ledger)
	}
	if w.openQty(Ask, Meal, c.Pos, Community) != 1 || !c.ledgerBalanced() {
		t.Fatalf("one meal should be on sale: %+v", c.Ledger)
	}
}

// Without a battery a stove still cooks, cold: the same scum for a meal,
// three times as long.
func TestAStoveWithoutABatteryCooksCold(t *testing.T) {
	powered, _ := poweredKitchen(t, ItemStack{CaveScum, 2}, ItemStack{Battery, 1})
	hot, _ := cookOnce(powered)
	w, c := poweredKitchen(t, ItemStack{CaveScum, 2})
	cold, ok := cookOnce(w)
	if !ok {
		t.Fatal("a stove without a battery did not cook cold")
	}
	if c.held(Community, CaveScum) != 0 || w.openQty(Ask, Meal, c.Pos, Community) != 1 {
		t.Fatalf("cold cooking should turn two scum into one meal: %+v", c.Ledger)
	}
	if cold < 2*hot {
		t.Fatalf("cold cooking took %d ticks against %d powered", cold, hot)
	}
}

// A stove holding a battery spends it before it would cook cold.
func TestAStovePrefersPowerToCold(t *testing.T) {
	w, c := poweredKitchen(t, ItemStack{CaveScum, 2}, ItemStack{Battery, 1})
	cookOnce(w)
	if c.held(Community, Battery) != 0 {
		t.Fatal("cooked cold with a battery in the depot")
	}
}

// With power off nothing needs or spends a battery, and the cold rows are
// never worked: the rules from before power.
func TestWithPowerOffCookingNeedsNoBattery(t *testing.T) {
	w, c := poweredKitchen(t, ItemStack{CaveScum, 2}, ItemStack{Battery, 1})
	w.cfg.Power = false
	ticks, ok := cookOnce(w)
	if !ok {
		t.Fatal("no cooking job")
	}
	if c.held(Community, Battery) != 1 || c.held(Community, CaveScum) != 0 {
		t.Fatalf("power off should cook two scum into a meal and leave the battery: %+v", c.Ledger)
	}
	if ticks > 3*recipes[3].Ticks {
		t.Fatalf("power off took %d ticks, as if cold", ticks)
	}
}

// The meals a stove's stock would make count each kind of biomatter once,
// under the powered row or the cold one, not both.
func TestColonyMealsInCountsEachKindOnce(t *testing.T) {
	w, c := poweredKitchen(t, ItemStack{CaveScum, 6})
	if got := w.colonyMealsIn(c); got != 3 {
		t.Fatalf("six scum cold = %d meals, want 3", got)
	}
	c.Inventory.Add(Battery, 1)
	c.credit(Community, Battery, 1)
	if got := w.colonyMealsIn(c); got != 3 {
		t.Fatalf("six scum powered = %d meals, want 3", got)
	}
}

// The ships land the colony's batteries in the passengers' lockers, and the
// colony has them carried to its stove for hire: every battery accounted
// for, and every dollar.
func TestTheColonyStocksItsStovesFromTheShips(t *testing.T) {
	cfg := testConfig()
	cfg.Power = true
	cfg.StartAliens = 0
	cfg.InfiniteFood = false
	w := newTestWorld(t, cfg)
	total := func() (all, kitchen int) {
		w.eachContainer(func(c *StorageContainer) {
			n := c.held(Community, Battery)
			all += n
			if w.TerrainAt(c.Pos) == Scumhouse {
				kitchen += n
			}
		})
		for _, id := range w.entityIDsSorted() {
			all += w.entities[id].carriedFor(Community, Battery)
		}
		return all, kitchen
	}
	landed := cfg.ShipBatteries * w.countKind(Colonist)
	if all, _ := total(); all != landed {
		t.Fatalf("the ships landed %d of the colony's batteries, want %d", all, landed)
	}
	stocked := false
	for i := 0; i < 10000 && !stocked; i++ {
		w.step()
		_, kitchen := total()
		stocked = kitchen > 0
	}
	if !stocked {
		t.Fatal("no battery ever reached a stove")
	}
	if all, _ := total(); all > landed {
		t.Fatalf("%d batteries, more than the %d landed", all, landed)
	}
	assertMoneyConserved(t, w)
}

// A supply drop can bring the colony batteries, landed at its silo.
func TestASupplyDropBringsBatteries(t *testing.T) {
	w, _ := poweredKitchen(t)
	silo := Point{8, 8, LandingLevel}
	w.SetTerrain(silo, Storage)
	w.refreshSpatial()
	w.fireSupplyDrop(scheduledEvent{Name: "resupply", Occurrence: DirectorOccurrence{Kind: OccSupplyDrop, Batteries: 30}})
	if got := w.storageContainers[silo].held(Community, Battery); got != 30 {
		t.Fatalf("the silo holds %d of the colony's batteries, want 30", got)
	}
}

// reactorWorld is propertyWorld with power on, a silo, and a reactor holding
// the colony's goods.
func reactorWorld(t *testing.T, stock ...ItemStack) (*World, *StorageContainer) {
	t.Helper()
	w := propertyWorld(t)
	w.cfg.Power = true
	w.SetTerrain(Point{8, 8, LandingLevel}, Storage) // the silo
	at := Point{20, 6, LandingLevel}
	w.SetTerrain(at, Reactor)
	w.refreshSpatial()
	c := w.storageContainers[at]
	for _, s := range stock {
		c.Inventory.Add(s.Kind, s.Count)
		c.credit(Community, s.Kind, s.Count)
	}
	return w, c
}

// An operator presses the colony's ore into pellets, then charges batteries
// from a pellet and a unit of ice, paid wage-reactor a recipe.
func TestTheReactorTurnsUraniumAndIceIntoBatteries(t *testing.T) {
	w, c := reactorWorld(t, ItemStack{UraniumOre, 1}, ItemStack{WaterIce, 2})
	op := w.spawn(Colonist, Point{12, 10, LandingLevel})
	wage := op.wallet
	for job := 0; job < 4 && c.held(Community, WaterIce) > 0; job++ {
		if !w.tryAssignReactor(op) {
			t.Fatalf("job %d: no reactor work (%+v)", job, c.Ledger)
		}
		for i := 0; i < 1000 && op.Job == JobCraft; i++ {
			w.jobCraft(op)
		}
	}
	if c.held(Community, UraniumOre) != 0 || c.held(Community, FuelPellet) != 38 || c.held(Community, Battery) != 24 {
		t.Fatalf("one ore and two ice should make 38 pellets left over and 24 batteries: %+v", c.Ledger)
	}
	if want := wage + Money(3*w.cfg.WageReactor); op.wallet != want {
		t.Fatalf("the operator has %v, want %v for three recipes", op.wallet, want)
	}
	if !c.ledgerBalanced() {
		t.Fatal("the reactor's ledger does not balance")
	}
	assertMoneyConserved(t, w)
}

// The reactor runs only while the colony is short of batteries.
func TestTheReactorRestsWhileTheColonyHasBatteries(t *testing.T) {
	w, c := reactorWorld(t, ItemStack{FuelPellet, 1}, ItemStack{WaterIce, 1})
	if !w.batteriesWanted() {
		t.Fatal("a colony with no batteries does not want any")
	}
	c.Inventory.Add(Battery, w.cfg.BatteryReserve)
	c.credit(Community, Battery, w.cfg.BatteryReserve)
	w.tick++ // past the memo
	if w.batteriesWanted() {
		t.Fatal("a colony at its battery reserve still wants more")
	}
}

// The colony plans a reactor only once it holds uranium to burn.
func TestTheColonyWantsAReactorOnceItHoldsUranium(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.Power = true
	silo := Point{8, 8, LandingLevel}
	w.SetTerrain(silo, Storage)
	w.refreshSpatial()
	if w.wantsReactor() {
		t.Fatal("wants a reactor with no uranium")
	}
	c := w.storageContainers[silo]
	c.Inventory.Add(UraniumOre, 1)
	c.credit(Community, UraniumOre, 1)
	if !w.wantsReactor() {
		t.Fatal("holds uranium and wants no reactor")
	}
	w.cfg.Power = false
	if w.wantsReactor() {
		t.Fatal("wants a reactor with power off")
	}
}

// The colony bids for uranium at its silo, even under
// standing-orders-build-only, and for ice only once a reactor stands; it
// never resells either.
func TestTheColonyBuysTheReactorsFuel(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.Power = true
	silo := Point{8, 8, LandingLevel}
	w.SetTerrain(silo, Storage)
	w.refreshSpatial()
	w.refreshReactorFuel()
	if got := w.openQty(Bid, UraniumOre, silo, Community); got != w.cfg.UraniumReserve {
		t.Fatalf("uranium bid for %d, want %d", got, w.cfg.UraniumReserve)
	}
	if got := w.openQty(Bid, WaterIce, silo, Community); got != 0 {
		t.Fatalf("bid for %d ice with no reactor", got)
	}
	w.SetTerrain(Point{20, 6, LandingLevel}, Reactor)
	w.refreshSpatial()
	w.refreshReactorFuel()
	if got := w.openQty(Bid, WaterIce, silo, Community); got != w.cfg.ReactorIceStock {
		t.Fatalf("ice bid for %d, want %d", got, w.cfg.ReactorIceStock)
	}
	c := w.storageContainers[silo]
	c.Inventory.Add(UraniumOre, 9)
	c.credit(Community, UraniumOre, 9)
	w.cfg.ColonySells = true
	w.refreshColonyAsks()
	if got := w.openQty(Ask, UraniumOre, silo, Community); got != 0 {
		t.Fatalf("the colony offered %d of its reactor's uranium for sale", got)
	}
}

// A reactor holding fuel doses whoever stands beside it; an empty one, or
// one with reactor-exposure off, doesn't.
func TestAFuelledReactorDosesItsNeighbours(t *testing.T) {
	w, c := reactorWorld(t)
	e := w.spawn(Colonist, Point{19, 6, LandingLevel})
	if w.uraniumExposed(e) {
		t.Fatal("an empty reactor dosed its neighbour")
	}
	c.Inventory.Add(FuelPellet, 1)
	c.credit(Community, FuelPellet, 1)
	if !w.uraniumExposed(e) {
		t.Fatal("a fuelled reactor did not dose its neighbour")
	}
	w.cfg.ReactorExposure = false
	if w.uraniumExposed(e) {
		t.Fatal("reactor-exposure off, and the reactor still dosed")
	}
}

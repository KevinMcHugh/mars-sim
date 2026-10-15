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

// Without a battery a stove still cooks, cold: half again the scum for a
// meal, and three times as long. Two units of scum, a powered meal's worth,
// make nothing cold.
func TestAStoveWithoutABatteryCooksCold(t *testing.T) {
	w, _ := poweredKitchen(t, ItemStack{CaveScum, 2})
	if _, ok := cookOnce(w); ok {
		t.Fatal("cooked a meal from two scum without a battery")
	}

	powered, _ := poweredKitchen(t, ItemStack{CaveScum, 2}, ItemStack{Battery, 1})
	hot, _ := cookOnce(powered)
	w, c := poweredKitchen(t, ItemStack{CaveScum, 3})
	cold, ok := cookOnce(w)
	if !ok {
		t.Fatal("a stove without a battery did not cook cold")
	}
	if c.held(Community, CaveScum) != 0 || w.openQty(Ask, Meal, c.Pos, Community) != 1 {
		t.Fatalf("cold cooking should turn three scum into one meal: %+v", c.Ledger)
	}
	if cold < 2*hot {
		t.Fatalf("cold cooking took %d ticks against %d powered", cold, hot)
	}
}

// A stove holding a battery spends it before it would cook cold.
func TestAStovePrefersPowerToCold(t *testing.T) {
	w, c := poweredKitchen(t, ItemStack{CaveScum, 3}, ItemStack{Battery, 1})
	cookOnce(w)
	if c.held(Community, Battery) != 0 {
		t.Fatal("cooked cold with a battery in the depot")
	}
}

// With power off nothing needs or spends a battery, and the cold rows are
// never worked: the rules from before power.
func TestWithPowerOffCookingNeedsNoBattery(t *testing.T) {
	w, c := poweredKitchen(t, ItemStack{CaveScum, 3}, ItemStack{Battery, 1})
	w.cfg.Power = false
	ticks, ok := cookOnce(w)
	if !ok {
		t.Fatal("no cooking job")
	}
	if c.held(Community, Battery) != 1 || c.held(Community, CaveScum) != 1 {
		t.Fatalf("power off should cook two scum into a meal and leave the battery: %+v", c.Ledger)
	}
	if ticks > 3*recipes[3].Ticks {
		t.Fatalf("power off took %d ticks, as if cold", ticks)
	}
}

// The meals a stove's stock would make count each kind of biomatter once:
// powered while the stove has a battery, cold without.
func TestColonyMealsInCountsEachKindOnce(t *testing.T) {
	w, c := poweredKitchen(t, ItemStack{CaveScum, 6})
	if got := w.colonyMealsIn(c); got != 2 {
		t.Fatalf("six scum cold = %d meals, want 2", got)
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

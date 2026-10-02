package sim

import "testing"

// Carried goods belong to their owners unit by unit. A colonist carrying the
// colony's meals does not eat them, and does not sell them as its own.
func TestAHaulerDoesNotEatTheColonysMeals(t *testing.T) {
	w := propertyWorld(t)
	e := w.spawn(Colonist, Point{10, 10, LandingLevel})
	e.Inventory.Add(Meal, 2)
	e.addCargo(Community, Meal, 2)
	if w.tryStartEating(e) {
		t.Fatal("a hauler ate a meal it was carrying for the colony")
	}
	e.Inventory.Add(Meal, 1) // one of its own
	if !w.tryStartEating(e) || e.carriedFor(Community, Meal) != 2 {
		t.Fatalf("the hauler should eat its own meal and still carry the colony's 2 (carries %d)", e.carriedFor(Community, Meal))
	}
	if silo := (Point{12, 6, LandingLevel}); w.surplusMeals(e, silo) > 0 {
		t.Fatal("the colony's meals counted as the hauler's surplus to sell")
	}
}

// A builder that fetched the colony's iron and then mined some of its own
// unloads each share to its owner.
func TestUnloadingCreditsEachShareToItsOwner(t *testing.T) {
	w := propertyWorld(t)
	chest := Point{12, 6, LandingLevel}
	w.SetTerrain(chest, Storage)
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{12, 7, LandingLevel})
	me := ColonistOwner(e.ID)
	e.Inventory.Add(IronOre, 2)
	e.addCargo(Community, IronOre, 2) // fetched for a public work
	e.Inventory.Add(IronOre, 3)       // mined on its own account
	e.Inventory.Add(RawRock, (InventorySlotCount-2)*MaxStackSize)
	if !w.tryAssignStore(e) {
		t.Fatal("no store job")
	}
	for i := 0; i < 50 && e.Job == JobStore; i++ {
		w.jobStore(e)
	}
	c := w.landing().storageContainers[chest]
	mine := c.held(me, IronOre) + w.openQty(Ask, IronOre, chest, me) // at the silo it goes on sale
	if c.held(Community, IronOre) != 2 || mine != 3 {
		t.Fatalf("the chest credits the colony %d iron and the miner %d; want 2 and 3",
			c.held(Community, IronOre), mine)
	}
}

// A colonist's own build is paid from its own materials only: rock it carries
// for the colony does not count.
func TestTheColonysRockDoesNotPayForYourOwnBuild(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.ConstructionCosts = true
	e := w.spawn(Colonist, Point{10, 10, LandingLevel})
	cost := w.buildCost(Storage)
	for _, c := range cost {
		e.Inventory.Add(c.Kind, c.Count)
		e.addCargo(Community, c.Kind, c.Count)
	}
	if missing := missingMaterials(e, cost, materialPayers(e, Owner{})); len(missing) == 0 {
		t.Fatal("the colony's rock counted toward the builder's own build")
	}
	if missing := missingMaterials(e, cost, materialPayers(e, Community)); len(missing) != 0 {
		t.Fatalf("the colony's rock did not pay for the colony's build: missing %v", missing)
	}
}

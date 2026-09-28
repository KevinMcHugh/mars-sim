package sim

import "testing"

// crowdStoves puts a cook's claim on every scumhouse, as if the colony's
// stoves were all in use.
func crowdStoves(w *World, by *Entity) {
	for _, p := range w.scumhousesSorted() {
		w.workshopClaims[p] = by.ID
	}
}

// When the shared stoves are crowded a chef, and only a chef, commissions a
// kitchen of its own, paid from its wallet; once built, the kitchen is the
// chef's.
func TestAChefBuysAKitchenWhenTheStovesAreCrowded(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	var cols []*Entity
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Colonist {
			cols = append(cols, e)
		}
	}
	cook, chef := cols[0], cols[1]
	w.setRank(cook, SkillCooking, cfg.KitchenRank-1)
	w.setRank(chef, SkillCooking, cfg.KitchenRank)
	for _, e := range []*Entity{cook, chef} {
		e.wallet += 500
		w.moneyIssued += 500
	}
	shared := chef.Pos
	w.SetTerrain(shared, Scumhouse)
	w.refreshSpatial()

	w.commissionKitchens()
	if chef.kitchenCommissioned || len(w.projects) != 0 {
		t.Fatalf("a chef bought a kitchen with the stoves free: projects %v", projectNames(w.projects))
	}
	crowdStoves(w, cook)
	before := chef.wallet
	w.commissionKitchens()
	if cook.kitchenCommissioned {
		t.Fatal("a cook below kitchen-rank commissioned a kitchen")
	}
	if !chef.kitchenCommissioned || len(w.projects) != 1 || w.projects[0].issuer != ColonistOwner(chef.ID) {
		t.Fatalf("the chef commissioned nothing: projects %v", projectNames(w.projects))
	}
	kitchen := w.projects[0]
	if chef.wallet != before-w.projectCost(kitchen) {
		t.Fatalf("wallet %v, want %v less the kitchen's %v", chef.wallet, before, w.projectCost(kitchen))
	}
	delete(w.workshopClaims, shared)
	for i := 0; i < 6000 && len(w.projects) > 0 && w.projects[0] == kitchen; i++ {
		w.step()
	}
	if !chef.hasKitchen || w.TerrainAt(chef.kitchen) != Scumhouse {
		t.Fatal("the chef's kitchen was never finished")
	}
	f := w.fixtures[chef.kitchen]
	if f.Owner != ColonistOwner(chef.ID) || f.Access != AccessCommunal {
		t.Fatalf("kitchen fixture: %+v", f)
	}
	if !w.privateKitchen(chef.kitchen) {
		t.Fatal("the chef's kitchen counts as the colony's")
	}
	assertMoneyConserved(t, w)
}

// ownKitchenWorld is scumhouseWorld with its scumhouse owned by a chef.
func ownKitchenWorld(t *testing.T) (w *World, house Point, chef *Entity) {
	t.Helper()
	w, house = scumhouseWorld(t, false)
	chef = w.spawn(Colonist, Point{19, 7})
	w.setFixtureOwner(house, ColonistOwner(chef.ID), AccessCommunal)
	chef.kitchen, chef.hasKitchen, chef.kitchenCommissioned = house, true, true
	return w, house, chef
}

// Only its owner cooks at a chef's kitchen, and the colony neither cooks nor
// buys biomatter there, nor counts it among its own when planning kitchens.
func TestOnlyTheOwnerCooksInAChefsKitchen(t *testing.T) {
	w, house, chef := ownKitchenWorld(t)
	other := w.spawn(Colonist, Point{12, 10})
	c := w.storageContainers[house]
	for _, e := range []*Entity{chef, other} {
		c.Inventory.Add(CaveScum, 2)
		c.credit(ColonistOwner(e.ID), CaveScum, 2)
	}
	c.Inventory.Add(CaveScum, 2)
	c.credit(Community, CaveScum, 2)
	w.cfg.MealReserve = 10 // the colony wants food

	if w.tryAssignCraftFor(other, []Owner{ColonistOwner(other.ID), Community}) {
		t.Fatal("another colonist cooks in the chef's kitchen")
	}
	if !w.tryAssignCraftFor(chef, []Owner{ColonistOwner(chef.ID)}) || chef.Target != house {
		t.Fatal("the chef doesn't cook in its own kitchen")
	}
	w.refreshBiomatterBids()
	if w.openQty(Bid, CaveScum, house, Community) != 0 {
		t.Fatal("the colony bids for scum in the chef's kitchen")
	}
	if len(w.colonyKitchens()) != 0 || w.plannedColonyKitchens() != 0 {
		t.Fatalf("the chef's kitchen counts as the colony's: %v, planned %d", w.colonyKitchens(), w.plannedColonyKitchens())
	}
}

// A chef stocks its kitchen by bidding for scum there from its own wallet,
// and a scraper may leave scum in someone else's kitchen only when a bid
// there would buy it: scum left anywhere else there could be neither cooked
// nor sold.
func TestAChefBuysScumForItsKitchen(t *testing.T) {
	w, house, chef := ownKitchenWorld(t)
	scraper := w.spawn(Colonist, Point{12, 10})
	if w.mayStockAt(scraper, house, CaveScum) {
		t.Fatal("scum may be left in a chef's kitchen with nobody bidding for it")
	}
	me := ColonistOwner(chef.ID)
	w.mint(me, 100)
	before := chef.wallet
	w.refreshChefBids()
	bid := w.openQty(Bid, CaveScum, house, me)
	if bid == 0 {
		t.Fatal("the chef posted no bid for scum in its kitchen")
	}
	if spent := before - chef.wallet; spent != Money(bid)*w.biomatterPrice(CaveScum) {
		t.Fatalf("the chef escrowed %v for %d units of scum", spent, bid)
	}
	if !w.mayStockAt(scraper, house, CaveScum) {
		t.Fatal("a scraper may not sell into the chef's bid")
	}
	if !w.mayStockAt(chef, house, CaveScum) {
		t.Fatal("the chef may not stock its own kitchen")
	}
	assertMoneyConserved(t, w)
}

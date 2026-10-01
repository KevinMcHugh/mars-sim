package sim

import "testing"

// petWorld lands colonists whose one rare item is always the given one.
func petWorld(t *testing.T, gun, hen, cat, colonists int) *World {
	t.Helper()
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = colonists, 0, 0, 0
	cfg.CrashPodGunWeight, cfg.CrashPodChickenWeight, cfg.CrashPodCatWeight = gun, hen, cat
	return newTestWorld(t, cfg)
}

// A chicken keeper's pod has a stocked trough of its own, and its hen steps
// out beside it; a cat owner's cat steps out the same way. Neither lands with
// a gun: everyone gets exactly one rare item.
func TestPetsLandInTheirKeepersPods(t *testing.T) {
	w := petWorld(t, 0, 1, 0, 3)
	for _, e := range w.entities {
		if e.Kind != Colonist {
			continue
		}
		if bestWeapon(e.Inventory) != ItemNone {
			t.Fatalf("%s keeps chickens and also carries a gun", e.displayName())
		}
		trough := e.podOrigin.Add(podTrough.X, podTrough.Y)
		c, ok := w.troughOf(e)
		if !ok || e.trough != trough {
			t.Fatalf("%s has no trough at %v", e.displayName(), trough)
		}
		if f := w.fixtures[trough]; f == nil || f.Owner != ColonistOwner(e.ID) || f.Access != AccessPrivate {
			t.Fatalf("%s's trough is not its own: %+v", e.displayName(), f)
		}
		if c.Inventory.Count(Feed) != w.cfg.TroughFill || !c.ledgerBalanced() {
			t.Fatalf("%s's trough landed with %d feed, want %d", e.displayName(), c.Inventory.Count(Feed), w.cfg.TroughFill)
		}
		if !w.keepsChickens(e) {
			t.Fatalf("%s landed without its chicken", e.displayName())
		}
	}
	if w.kindCounts[Chicken] != 3 {
		t.Fatalf("3 keepers landed with %d chickens", w.kindCounts[Chicken])
	}

	w = petWorld(t, 0, 0, 1, 3)
	if w.kindCounts[Cat] != 3 {
		t.Fatalf("3 cat owners landed with %d cats", w.kindCounts[Cat])
	}
	for id := range w.kindEntities[Cat] {
		cat := w.entities[id]
		owner := w.entities[cat.keeper]
		if owner == nil || owner.Kind != Colonist || cat.Pos != owner.podOrigin.Add(podPet.X, podPet.Y) {
			t.Fatalf("cat #%d at %v did not step out of its owner's pod", cat.ID, cat.Pos)
		}
		if owner.hasTrough {
			t.Fatalf("a cat owner landed with a trough")
		}
	}
}

// A hungry chicken eats feed from its trough, and with the trough empty it
// grazes exposed cave scum.
func TestChickensEatFeedThenGrazeScum(t *testing.T) {
	w := propertyWorld(t)
	noScum(w)
	trough := Point{10, 10}
	w.SetTerrain(trough, Trough)
	c := w.storageContainers[trough]
	c.Inventory.Add(Feed, 2)
	c.credit(Community, Feed, 2)
	hen := w.spawn(Chicken, Point{14, 10})
	hen.trough, hen.hasTrough = trough, true

	hungry := func() {
		hen.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt + 10
		hen.needSince[NeedFood] = w.tick
	}
	hungry()
	for i := 0; i < 100 && c.Inventory.Count(Feed) == 2; i++ {
		w.step()
	}
	if c.Inventory.Count(Feed) != 1 || !c.ledgerBalanced() {
		t.Fatalf("the hen left %d of 2 feed in the trough", c.Inventory.Count(Feed))
	}
	if w.needLevel(hen, NeedFood) >= w.cfg.Needs[NeedFood].SeekAt {
		t.Fatal("eating feed did not sate the hen")
	}

	c.debit(Community, Feed, 1)
	patch := Point{18, 12}
	putScum(w, patch, 2)
	hungry()
	for i := 0; i < 200 && w.scumAt(patch) == 2; i++ {
		w.step()
	}
	if w.scumAt(patch) != 1 {
		t.Fatalf("with an empty trough the hen left %d of 2 scum", w.scumAt(patch))
	}
}

// A keeper whose trough runs low scrapes scum, mixes it into feed at a
// scumhouse even while a cook holds the stove, and fills its trough.
func TestKeepersFillTheirTroughs(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	keeper := w.spawn(Colonist, Point{12, 12})
	trough := Point{8, 12}
	w.SetTerrain(trough, Trough)
	w.setFixtureOwner(trough, ColonistOwner(keeper.ID), AccessPrivate)
	keeper.trough, keeper.hasTrough = trough, true
	hen := w.spawn(Chicken, Point{6, 14})
	hen.keeper, hen.trough, hen.hasTrough = keeper.ID, trough, true
	putScum(w, Point{16, 14}, 2)
	w.setScum(Point{18, 8}, 2)
	w.exposedScum[Point{18, 8}] = struct{}{}
	w.workshopClaims[house] = 999 // a cook at the stove

	if !w.tryAssignTend(keeper) || keeper.tend != tendGather {
		t.Fatalf("a keeper with an empty trough did not set out to scrape (job %d)", keeper.Job)
	}
	want := w.feedScumFor(w.cfg.TroughFill)
	if keeper.scrapeQty != want {
		t.Fatalf("set out for %d scum, want %d", keeper.scrapeQty, want)
	}
	for i := 0; i < 2000 && keeper.Job == JobTend; i++ {
		w.jobTend(keeper)
		w.refreshSpatial()
	}
	c := w.storageContainers[trough]
	if got := c.Inventory.Count(Feed); got != want*feedRecipe.Outputs[0].Count {
		t.Fatalf("the trough holds %d feed, want %d", got, want*feedRecipe.Outputs[0].Count)
	}
	if c.held(ColonistOwner(keeper.ID), Feed) != c.Inventory.Count(Feed) || !c.ledgerBalanced() {
		t.Fatal("the feed in the trough is not the keeper's")
	}
	if keeper.Inventory.Has(CaveScum) || keeper.Inventory.Has(Feed) {
		t.Fatalf("the keeper still carries %v", keeper.Inventory)
	}
	if w.workshopClaims[house] != 999 {
		t.Fatal("the keeper took the cook's claim on the stove")
	}
	if w.tryAssignTend(keeper) {
		t.Fatal("a keeper with a full trough went tending again")
	}
	hen.HP = 0
	w.remove(hen.ID, "test")
	c.debit(ColonistOwner(keeper.ID), Feed, c.Inventory.Count(Feed))
	if w.tryAssignTend(keeper) {
		t.Fatal("a keeper with no living chickens went tending")
	}
}

// Cats and chickens don't interact: a cat with only a chicken in reach
// doesn't hunt it, and the chicken doesn't flee the cat.
func TestCatsAndChickensIgnoreEachOther(t *testing.T) {
	w := propertyWorld(t)
	cat := w.spawn(Cat, Point{10, 10})
	hen := w.spawn(Chicken, Point{12, 10})
	for i := 0; i < 300; i++ {
		w.step()
		if cat.State == Hunting || cat.Quarry != 0 {
			t.Fatalf("tick %d: the cat is hunting (quarry %d)", w.tick, cat.Quarry)
		}
		if hen.State == Fleeing {
			t.Fatalf("tick %d: the hen is fleeing", w.tick)
		}
	}
	if w.entities[hen.ID] == nil {
		t.Fatal("the hen did not survive the cat")
	}
}

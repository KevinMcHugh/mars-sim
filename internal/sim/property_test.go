package sim

import "testing"

// propertyWorld is an open 18x11 floor with no starting population, for
// placing fixtures and colonists by hand.
func propertyWorld(t *testing.T) *World {
	t.Helper()
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	for y := 5; y <= 15; y++ {
		for x := 5; x <= 22; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	w.refreshSpatial()
	return w
}

// Every fixture terrain gets a record owned by the colony and open to all —
// which is why ownership changed nothing about how the colony plays — and the
// record goes when the terrain does.
func TestFixturesTrackFixtureTerrain(t *testing.T) {
	w := propertyWorld(t)
	p := Point{8, 8}
	for _, kind := range []Terrain{NutrientPod, Toilet, Bed, Incinerator, Storage} {
		w.SetTerrain(p, kind)
		f := w.fixtures[p]
		if f == nil || f.Terrain != kind || f.Owner != Community || f.Access != AccessCommunal {
			t.Fatalf("%v: fixture = %+v, want a communal colony-owned record", kind, f)
		}
		w.SetTerrain(p, Floor)
		if w.fixtures[p] != nil {
			t.Fatalf("%v: fixture survived its terrain being replaced", kind)
		}
	}
	w.SetTerrain(p, Wall)
	if w.fixtures[p] != nil {
		t.Fatal("a wall is not a fixture")
	}
}

// The restricted count follows ownership changes and demolition, and a change
// in whether a fixture is communal reaches its shared field.
func TestSetFixtureOwnerKeepsRestrictedCount(t *testing.T) {
	w := propertyWorld(t)
	a := w.spawn(Colonist, Point{6, 6})
	p := Point{10, 10}
	w.SetTerrain(p, Bed)
	w.refreshSpatial()
	w.facilityField(Bed) // freshen

	if !w.setFixtureOwner(p, ColonistOwner(a.ID), AccessPrivate) {
		t.Fatal("setFixtureOwner found no fixture")
	}
	if w.restrictedFixtures[Bed] != 1 {
		t.Fatalf("restricted=%d after making the bed private", w.restrictedFixtures[Bed])
	}
	w.tick++ // fields refresh at most once a tick
	if d := w.facilityField(Bed).at(Point{10, 11}); d == 0 {
		t.Fatal("the shared field still treats a private bed's access tile as a goal")
	}
	w.setFixtureOwner(p, ColonistOwner(a.ID), AccessPrivate) // no change
	if w.restrictedFixtures[Bed] != 1 {
		t.Fatalf("restricted=%d after a no-op change", w.restrictedFixtures[Bed])
	}
	w.SetTerrain(p, Floor)
	if w.restrictedFixtures[Bed] != 0 {
		t.Fatalf("restricted=%d after the private bed was demolished", w.restrictedFixtures[Bed])
	}
	if w.setFixtureOwner(Point{12, 12}, Community, AccessCommunal) {
		t.Fatal("setFixtureOwner succeeded on a tile with no fixture")
	}
}

// The shared field only leads to fixtures everyone may use; a private one is
// reachable to its owner alone.
func TestPrivateFixtureIsReachableOnlyToItsOwner(t *testing.T) {
	w := propertyWorld(t)
	owner := w.spawn(Colonist, Point{6, 6})
	other := w.spawn(Colonist, Point{20, 14})
	rat := w.spawn(Rat, Point{21, 14})
	pod := Point{12, 10}
	w.SetTerrain(pod, NutrientPod)
	w.setFixtureOwner(pod, ColonistOwner(owner.ID), AccessPrivate)
	w.refreshSpatial()

	if f := w.facilityField(NutrientPod); f.at(other.Pos) >= 0 {
		t.Fatal("the shared field leads to a private pod")
	}
	if !w.facilityReachable(owner, NutrientPod) {
		t.Fatal("owner cannot reach its own pod")
	}
	if w.facilityReachable(other, NutrientPod) || w.facilityReachable(rat, NutrientPod) {
		t.Fatal("a non-owner (colonist or rat) can reach a private pod")
	}

	// Like a terrain change, an access change reaches the shared field on the
	// next tick: fields rebuild at most once per tick (see ensureFresh).
	w.setFixtureOwner(pod, ColonistOwner(owner.ID), AccessCommunal)
	w.tick++
	if !w.facilityReachable(other, NutrientPod) || !w.facilityReachable(rat, NutrientPod) {
		t.Fatal("opening the pod to all did not make it reachable")
	}
}

// A tired colonist sleeps in its own bunk, and another tired colonist never
// uses it — it takes the communal bed across the room even though the private
// one is right beside it.
func TestColonistsSleepOnlyInBedsTheyMayUse(t *testing.T) {
	w := propertyWorld(t)
	sleepy := w.cfg.Needs[NeedSleep].SeekAt + 50
	mine := Point{9, 10}
	shared := Point{20, 10}
	w.SetTerrain(mine, Bed)
	w.SetTerrain(shared, Bed)
	w.refreshSpatial()

	owner := w.spawn(Colonist, Point{7, 12})
	guest := w.spawn(Colonist, Point{10, 12}) // nearer the private bed than the shared one
	w.setFixtureOwner(mine, ColonistOwner(owner.ID), AccessPrivate)
	for _, e := range []*Entity{owner, guest} {
		e.Needs[NeedSleep] = sleepy
		w.syncNeedPhase(e, NeedSleep)
	}

	if got := w.chooseFacility(guest, Bed); got != shared {
		t.Fatalf("guest chose %v, want the shared bed %v", got, shared)
	}
	if got := w.chooseFacility(owner, Bed); got != mine {
		t.Fatalf("owner chose %v, want its own bed %v", got, mine)
	}

	ownerSlept, guestSlept := false, false
	for i := 0; i < 400 && !(ownerSlept && guestSlept); i++ {
		w.step()
		if guest.Job == JobUse && guest.useFacilitySet && guest.useFacility == mine {
			t.Fatalf("tick %d: guest is using the owner's private bed", w.tick)
		}
		ownerSlept = ownerSlept || w.needLevel(owner, NeedSleep) < sleepy/2
		guestSlept = guestSlept || w.needLevel(guest, NeedSleep) < sleepy/2
	}
	if !ownerSlept || !guestSlept {
		t.Fatalf("owner slept=%v guest slept=%v", ownerSlept, guestSlept)
	}
}

// A colonist whose only reachable bed is its own private one still gets to
// it: the field shortcut for a single facility must not strand it.
func TestOwnerReachesItsOnlyPrivateBed(t *testing.T) {
	w := propertyWorld(t)
	bed := Point{20, 10}
	w.SetTerrain(bed, Bed)
	w.refreshSpatial()
	owner := w.spawn(Colonist, Point{6, 12})
	w.setFixtureOwner(bed, ColonistOwner(owner.ID), AccessPrivate)
	owner.Needs[NeedSleep] = w.cfg.Needs[NeedSleep].SeekAt + 50
	w.syncNeedPhase(owner, NeedSleep)

	for i := 0; i < 300; i++ {
		w.step()
		if w.needLevel(owner, NeedSleep) == 0 {
			return
		}
	}
	t.Fatalf("owner never slept in its own bed (sleep %d, at %v)", w.needLevel(owner, NeedSleep), owner.Pos)
}

// A private chest is not somewhere another colonist can unload.
func TestChooseStorageSkipsOthersPrivateChest(t *testing.T) {
	w, e, chest := storageBehaviorWorld(t, true)
	other := w.spawn(Colonist, Point{12, 12})
	w.setFixtureOwner(chest, ColonistOwner(other.ID), AccessPrivate)
	load := []ItemStack{{Kind: RawRock, Count: 1}}
	if _, ok := w.chooseStorage(e, load); ok {
		t.Fatal("chose someone else's private chest")
	}
	if got, ok := w.chooseStorage(other, load); !ok || got != chest {
		t.Fatalf("owner could not choose its own chest: %v %v", got, ok)
	}
}

// Deposits are credited to the colonist that carried them, and the ledger
// stays sorted by owner then item.
func TestStoringCreditsTheDepositor(t *testing.T) {
	w, e, chest := storageBehaviorWorld(t, true)
	rock := (InventorySlotCount - 1) * MaxStackSize // every slot but one, full
	if !e.Inventory.Add(RawRock, rock) || !e.Inventory.Add(IronOre, 3) {
		t.Fatal("test load did not fit")
	}
	container := w.storageContainers[chest]
	container.Inventory.Add(Clay, 2)
	container.credit(Community, Clay, 2)

	if !w.tryAssignStore(e) {
		t.Fatal("colonist would not store")
	}
	for i := 0; i < 60 && e.Job == JobStore; i++ {
		w.jobStore(e)
	}
	me := ColonistOwner(e.ID)
	if container.held(me, RawRock) != rock || container.held(me, IronOre) != 3 {
		t.Fatalf("ledger = %+v", container.Ledger)
	}
	if !container.ledgerBalanced() {
		t.Fatalf("ledger does not match contents: %+v", container.Ledger)
	}
	want := []LedgerLine{{Community, Clay, 2}, {me, RawRock, rock}, {me, IronOre, 3}}
	if len(container.Ledger) != len(want) {
		t.Fatalf("ledger = %+v, want %+v", container.Ledger, want)
	}
	for i := range want {
		if container.Ledger[i] != want[i] {
			t.Fatalf("ledger[%d] = %+v, want %+v (lines must sort by owner, then item)", i, container.Ledger[i], want[i])
		}
	}
}

// In a live colony the ledger balances on every tick. Seed 1 without natural
// caverns is used because it fills a chest within a few thousand ticks;
// caverns give the miners open ground and delay the first deposit past the run.
func TestLedgerBalancesThroughALongRun(t *testing.T) {
	cfg := testConfig()
	cfg.Seed = 1
	cfg.CavernPercent = 0
	cfg.Width, cfg.Height = 80, 50
	cfg.StartColonists, cfg.StartCats, cfg.StartRats = 16, 2, 10
	w := newTestWorld(t, cfg)
	lines := 0
	for i := 0; i < 5000; i++ {
		w.step()
		lines = 0
		for p, c := range w.storageContainers {
			if !c.ledgerBalanced() {
				t.Fatalf("tick %d: chest %v ledger %+v does not match its contents", w.tick, p, c.Ledger)
			}
			for _, l := range c.Ledger {
				if l.Owner.Kind != OwnerColonist || l.Count <= 0 {
					t.Fatalf("tick %d: unexpected ledger line %+v", w.tick, l)
				}
			}
			lines += len(c.Ledger)
		}
	}
	if lines == 0 {
		t.Fatal("no colonist ever stored anything; the test no longer covers deposits")
	}
}

// Snapshots publish fixtures sorted by position, find them by position, and
// reuse the previous frame's list when nothing changed.
func TestSnapshotPublishesFixtures(t *testing.T) {
	w := propertyWorld(t)
	a := w.spawn(Colonist, Point{6, 6})
	w.SetTerrain(Point{15, 9}, Toilet)
	w.SetTerrain(Point{8, 12}, Bed)
	w.SetTerrain(Point{9, 9}, NutrientPod)
	w.setFixtureOwner(Point{8, 12}, ColonistOwner(a.ID), AccessPrivate)

	first := w.snapshot(false, 10)
	if len(first.Fixtures) != 3 {
		t.Fatalf("fixtures = %+v", first.Fixtures)
	}
	for i := 1; i < len(first.Fixtures); i++ {
		if !lessPoint(first.Fixtures[i-1].Pos, first.Fixtures[i].Pos) {
			t.Fatalf("fixtures not sorted: %+v", first.Fixtures)
		}
	}
	if f, ok := first.FixtureAt(Point{8, 12}); !ok || f.Owner != ColonistOwner(a.ID) || f.Access != AccessPrivate {
		t.Fatalf("FixtureAt(8,12) = %+v %v", f, ok)
	}
	if _, ok := first.FixtureAt(Point{10, 10}); ok {
		t.Fatal("FixtureAt found a fixture on bare floor")
	}

	second := w.snapshot(false, 10)
	if &second.Fixtures[0] != &first.Fixtures[0] {
		t.Fatal("an unchanged fixture list was rebuilt")
	}
	w.setFixtureOwner(Point{8, 12}, Community, AccessCommunal)
	third := w.snapshot(false, 10)
	if &third.Fixtures[0] == &first.Fixtures[0] {
		t.Fatal("a changed fixture list reused the published slice")
	}
	if f, _ := first.FixtureAt(Point{8, 12}); f.Access != AccessPrivate {
		t.Fatal("publishing a change rewrote an older frame")
	}
}

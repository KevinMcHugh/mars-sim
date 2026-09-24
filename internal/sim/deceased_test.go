package sim

import "testing"

// A dead colonist should be archived in World.deceasedColonists permanently
// — unlike the bounded graveyard, this record is keyed by ID and never
// evicted, so a colonist's own page keeps working after death.
func TestDeceasedColonistArchivedPermanently(t *testing.T) {
	w := kinWorld()
	colonist := w.spawn(Colonist, Point{1, 1})
	id := colonist.ID

	w.remove(id, "test")

	if w.entities[id] != nil {
		t.Fatal("dead colonist should be removed from the living entities")
	}
	dead, ok := w.deceasedColonists[id]
	if !ok {
		t.Fatal("dead colonist should be archived in deceasedColonists")
	}
	if !dead.Dead || dead.Cause != "test" || dead.DiedTick != w.tick {
		t.Fatalf("archived record = %+v, want Dead/Cause/DiedTick set", dead)
	}
}

// Only colonists get a permanent archive entry — rats/cats/aliens rely on
// the bounded graveyard only, since their death volume has no natural cap
// the way a colony's population does.
func TestNonColonistsNotArchivedPermanently(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)
	spot := Point{2, 0}
	w.SetTerrain(spot, Floor)
	rat := w.spawn(Rat, spot)

	w.remove(rat.ID, "test")

	if _, ok := w.deceasedColonists[rat.ID]; ok {
		t.Fatal("a dead rat should not be archived in deceasedColonists")
	}
}

// A colonist's archive entry should outlive the bounded graveyard: once the
// graveyard fills up with later deaths and evicts the colonist's own entry,
// the permanent archive should still resolve them by ID.
func TestDeceasedSurvivesGraveyardEviction(t *testing.T) {
	cfg := testConfig()
	cfg.GraveyardSize = 1
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{0, 0})
	victim := w.spawn(Colonist, Point{1, 0})
	w.remove(victim.ID, "starved")

	// Fill the small bounded graveyard with unrelated deaths so it evicts
	// the colonist's entry.
	for i := 0; i < 3; i++ {
		spot := Point{2, 0}
		w.SetTerrain(spot, Floor)
		rat := w.spawn(Rat, spot)
		w.stomp(colonist, rat)
	}

	for _, g := range w.graveyard {
		if g.ID == victim.ID {
			t.Fatal("test setup should have evicted the colonist from the bounded graveyard")
		}
	}
	if _, ok := w.deceasedColonists[victim.ID]; !ok {
		t.Fatal("deceasedColonists should still resolve the colonist after graveyard eviction")
	}
}

// The permanent archive is not gated by GraveyardSize: even with the bounded
// graveyard disabled entirely (GraveyardSize 0), a dead colonist should still
// be durably archived for lookups.
func TestDeceasedArchivedEvenWhenGraveyardDisabled(t *testing.T) {
	cfg := testConfig()
	cfg.GraveyardSize = 0
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{0, 0})
	w.remove(colonist.ID, "starved")

	if len(w.graveyard) != 0 {
		t.Fatalf("graveyard length = %d, want 0 with GraveyardSize disabled", len(w.graveyard))
	}
	if _, ok := w.deceasedColonists[colonist.ID]; !ok {
		t.Fatal("deceasedColonists should archive the colonist even with GraveyardSize 0")
	}
}

// A colonist's carried inventory should still be readable off their archived
// record after death, since remove() never touches Inventory before
// snapshotting it.
func TestDeceasedRecordRetainsInventory(t *testing.T) {
	w := kinWorld()
	colonist := w.spawn(Colonist, Point{1, 1})
	colonist.Inventory[0] = ItemStack{Kind: RawRock, Count: 3}

	w.remove(colonist.ID, "test")

	got := w.deceasedColonists[colonist.ID].Inventory[0]
	want := ItemStack{Kind: RawRock, Count: 3}
	if got != want {
		t.Fatalf("archived inventory[0] = %+v, want %+v", got, want)
	}
}

// A dead colonist should stay in their surviving relative's family tree
// instead of dropping out silently: relativesOf should keep returning a
// Relation for them by ID, resolvable through Snapshot.Deceased.
func TestDeadColonistStaysInLivingRelativeFamilyTree(t *testing.T) {
	w := kinWorld()
	parent := w.spawn(Colonist, Point{1, 1})
	child := w.spawn(Colonist, Point{2, 1})
	parent.Profile.Age = 45
	child.Profile.Age = 20
	if !w.wireRelation(child, parent, RelChild) {
		t.Fatal("could not wire child to parent")
	}

	w.remove(parent.ID, "starved")

	rel, ok := relationTo(w, child, parent)
	if !ok || rel != RelParent {
		t.Fatalf("child's relation to dead parent = %v, %v, want RelParent, true", rel, ok)
	}
}

// A dead colonist's own archived record should carry their family relations
// as of the moment of death, so their own page (not just their relatives')
// keeps showing who they were related to.
func TestDeceasedRecordHasOwnRelations(t *testing.T) {
	w := kinWorld()
	parent := w.spawn(Colonist, Point{1, 1})
	child := w.spawn(Colonist, Point{2, 1})
	parent.Profile.Age = 45
	child.Profile.Age = 20
	if !w.wireRelation(child, parent, RelChild) {
		t.Fatal("could not wire child to parent")
	}

	w.remove(parent.ID, "starved")

	dead := w.deceasedColonists[parent.ID]
	found := false
	for _, rel := range dead.Relations {
		if rel.Other == child.ID && rel.Kind == RelChild {
			found = true
		}
	}
	if !found {
		t.Fatalf("dead parent's archived relations = %+v, want a RelChild tie to %d", dead.Relations, child.ID)
	}
}

// Snapshots between two deaths share one copy of the archive, and a death
// publishes a new copy rather than writing into the one earlier snapshots
// hold.
func TestSnapshotDeceasedIsCopiedPerDeathNotPerFrame(t *testing.T) {
	w := kinWorld()
	first := w.spawn(Colonist, Point{1, 1})
	second := w.spawn(Colonist, Point{2, 1})
	w.remove(first.ID, "test")

	const probeID = EntityID(1 << 30)
	a, b := w.snapshot(false, 8), w.snapshot(false, 8)
	if len(a.Deceased) != 1 {
		t.Fatalf("snapshot has %d deceased, want 1", len(a.Deceased))
	}
	a.Deceased[probeID] = EntityView{} // probe for sharing, then undo
	shared := len(b.Deceased) == 2
	delete(a.Deceased, probeID)
	if !shared {
		t.Error("two snapshots with no death between them copied the archive twice")
	}
	if _, leaked := w.deceasedColonists[probeID]; leaked {
		t.Error("a snapshot's Deceased map is the world's own archive")
	}

	w.remove(second.ID, "test")
	c := w.snapshot(false, 8)
	if len(c.Deceased) != 2 {
		t.Errorf("snapshot after the second death has %d deceased, want 2", len(c.Deceased))
	}
	if len(a.Deceased) != 1 {
		t.Errorf("an earlier snapshot's Deceased grew to %d: a death wrote into a published map", len(a.Deceased))
	}
}

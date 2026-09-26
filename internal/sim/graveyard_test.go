package sim

import "testing"

// A stomped rat should be frozen into the graveyard with the crushing
// colonist named in its cause, and no longer appear as a living entity.
func TestGraveyardRecordsDeath(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	ratSpot := Point{5, 5}
	w.SetTerrain(ratSpot, Floor)
	colonist := w.spawn(Colonist, Point{4, 5})
	rat := w.spawn(Rat, ratSpot)
	w.stomp(colonist, rat)

	if w.entities[rat.ID] != nil {
		t.Fatal("stomped rat should be removed from the living entities")
	}
	if len(w.graveyard) != 1 {
		t.Fatalf("graveyard length = %d, want 1", len(w.graveyard))
	}
	got := w.graveyard[0]
	if got.ID != rat.ID || got.Kind != Rat || !got.Dead {
		t.Fatalf("graveyard entry = %+v, want a dead record for rat %d", got, rat.ID)
	}
	if got.Pos != ratSpot {
		t.Errorf("graveyard entry pos = %v, want %v (frozen at death)", got.Pos, ratSpot)
	}
	wantCause := "crushed by " + colonist.displayName()
	if got.Cause != wantCause {
		t.Errorf("graveyard entry cause = %q, want %q", got.Cause, wantCause)
	}
}

// The graveyard should never grow past Config.GraveyardSize, dropping the
// oldest entries first.
func TestGraveyardIsBounded(t *testing.T) {
	cfg := testConfig()
	cfg.GraveyardSize = 2
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{0, 0})
	var lastRatID EntityID
	for i := 0; i < 5; i++ {
		spot := Point{2, 0}
		w.SetTerrain(spot, Floor)
		rat := w.spawn(Rat, spot)
		lastRatID = rat.ID
		w.stomp(colonist, rat)
	}

	if len(w.graveyard) != cfg.GraveyardSize {
		t.Fatalf("graveyard length = %d, want %d", len(w.graveyard), cfg.GraveyardSize)
	}
	newest := w.graveyard[len(w.graveyard)-1]
	if newest.ID != lastRatID {
		t.Errorf("newest graveyard entry = %d, want the last rat stomped (%d)", newest.ID, lastRatID)
	}
}

// GraveyardSize 0 should disable death tracking entirely rather than keeping
// an empty-but-growing slice.
func TestGraveyardDisabledWhenSizeZero(t *testing.T) {
	cfg := testConfig()
	cfg.GraveyardSize = 0
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{0, 0})
	spot := Point{2, 0}
	w.SetTerrain(spot, Floor)
	rat := w.spawn(Rat, spot)
	w.stomp(colonist, rat)

	if len(w.graveyard) != 0 {
		t.Fatalf("graveyard length = %d, want 0 with GraveyardSize disabled", len(w.graveyard))
	}
}

// The snapshot should expose the graveyard as its own list, separate from
// still-living entities.
func TestSnapshotExposesGraveyard(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{0, 0})
	spot := Point{2, 0}
	w.SetTerrain(spot, Floor)
	rat := w.spawn(Rat, spot)
	w.stomp(colonist, rat)

	snap := w.snapshot(false, 8)
	if len(snap.Graveyard) != 1 || snap.Graveyard[0].ID != rat.ID {
		t.Fatalf("snapshot graveyard = %+v, want one entry for rat %d", snap.Graveyard, rat.ID)
	}
	for _, e := range snap.Entities {
		if e.ID == rat.ID {
			t.Fatal("a dead entity should not also appear in snapshot.Entities")
		}
	}
}

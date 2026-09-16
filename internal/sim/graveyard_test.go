package sim

import "testing"

// A stomped mouse should be frozen into the graveyard with the crushing
// colonist named in its cause, and no longer appear as a living entity.
func TestGraveyardRecordsDeath(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	mouseSpot := Point{5, 5}
	w.SetTerrain(mouseSpot, Floor)
	colonist := w.spawn(Colonist, Point{4, 5})
	mouse := w.spawn(Mouse, mouseSpot)
	w.stomp(colonist, mouse)

	if w.entities[mouse.ID] != nil {
		t.Fatal("stomped mouse should be removed from the living entities")
	}
	if len(w.graveyard) != 1 {
		t.Fatalf("graveyard length = %d, want 1", len(w.graveyard))
	}
	got := w.graveyard[0]
	if got.ID != mouse.ID || got.Kind != Mouse || !got.Dead {
		t.Fatalf("graveyard entry = %+v, want a dead record for mouse %d", got, mouse.ID)
	}
	if got.Pos != mouseSpot {
		t.Errorf("graveyard entry pos = %v, want %v (frozen at death)", got.Pos, mouseSpot)
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
	var lastMouseID EntityID
	for i := 0; i < 5; i++ {
		spot := Point{2, 0}
		w.SetTerrain(spot, Floor)
		mouse := w.spawn(Mouse, spot)
		lastMouseID = mouse.ID
		w.stomp(colonist, mouse)
	}

	if len(w.graveyard) != cfg.GraveyardSize {
		t.Fatalf("graveyard length = %d, want %d", len(w.graveyard), cfg.GraveyardSize)
	}
	newest := w.graveyard[len(w.graveyard)-1]
	if newest.ID != lastMouseID {
		t.Errorf("newest graveyard entry = %d, want the last mouse stomped (%d)", newest.ID, lastMouseID)
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
	mouse := w.spawn(Mouse, spot)
	w.stomp(colonist, mouse)

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
	mouse := w.spawn(Mouse, spot)
	w.stomp(colonist, mouse)

	snap := w.snapshot(false, 8)
	if len(snap.Graveyard) != 1 || snap.Graveyard[0].ID != mouse.ID {
		t.Fatalf("snapshot graveyard = %+v, want one entry for mouse %d", snap.Graveyard, mouse.ID)
	}
	for _, e := range snap.Entities {
		if e.ID == mouse.ID {
			t.Fatal("a dead entity should not also appear in snapshot.Entities")
		}
	}
}

package sim

import "testing"

// A stomped rat's body is named on its tile, the snapshot says whose it is,
// and the name goes with the body when it is picked up — while an unnamed
// body dumped on the same tile stays unnamed.
func TestRemainsNameTheDead(t *testing.T) {
	w := propertyWorld(t)
	spot := Point{12, 10, LandingLevel}
	colonist := w.spawn(Colonist, Point{11, 10, LandingLevel})
	rat := w.spawn(Rat, spot)
	w.stomp(colonist, rat)
	w.addCorpse(spot, AnimalCorpse) // a hauler's anonymous load

	snap := w.snapshot(false, 1)
	got := snap.RemainsAt(spot)
	if len(got) != 1 || got[0].ID != rat.ID || got[0].Body != AnimalCorpse {
		t.Fatalf("remains = %+v, want rat #%d", got, rat.ID)
	}
	if n := snap.TileAt(spot).Corpses; n != 2 {
		t.Fatalf("corpses = %d, want 2", n)
	}

	if !w.takeCorpse(spot, AnimalCorpse) {
		t.Fatal("no body to take")
	}
	if got := w.snapshot(false, 1).RemainsAt(spot); len(got) != 0 {
		t.Fatalf("remains after pickup = %+v, want none", got)
	}
	if len(snap.RemainsAt(spot)) != 1 {
		t.Fatal("an earlier snapshot's remains changed after publication")
	}
}

// Building over a tile scrapes its bodies away, names and all.
func TestRemainsClearedByBuilding(t *testing.T) {
	w := propertyWorld(t)
	spot := Point{12, 10, LandingLevel}
	w.stomp(w.spawn(Colonist, Point{11, 10, LandingLevel}), w.spawn(Rat, spot))
	w.SetTerrain(spot, Wall)
	if got := w.snapshot(false, 1).RemainsAt(spot); len(got) != 0 {
		t.Fatalf("remains under a wall = %+v", got)
	}
}

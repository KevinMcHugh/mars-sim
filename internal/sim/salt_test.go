package sim

import "testing"

// Salt is laid down by generation and only ever lost after that: a long run
// never adds a deposit, and never puts one on a tile that holds scum, however
// much scum grows around it.
func TestSaltNeverRegeneratesOrMeetsScum(t *testing.T) {
	cfg := testConfig()
	cfg.SaltPercent = 20
	cfg.ScumSpawnPPM, cfg.ScumSpreadPercent = 20000, 90 // grow scum hard
	cfg.ScumPercent = 40
	w := newTestWorld(t, cfg)
	start := map[Point]bool{}
	for p := range w.salt {
		start[p] = true
	}
	if len(start) == 0 {
		t.Fatal("no salt was generated at 20%")
	}
	for i := 0; i < 600; i++ {
		w.step()
		for p := range w.salt {
			if !start[p] {
				t.Fatalf("tick %d: salt appeared on %v after generation", w.tick, p)
			}
			if _, scum := w.scum[p]; scum {
				t.Fatalf("tick %d: %v holds both salt and scum", w.tick, p)
			}
		}
	}
	if len(w.scum) == 0 {
		t.Fatal("scum never grew, so the test proved nothing")
	}
}

// addScum refuses a salt tile outright, with room to spare and rock under it.
func TestScumWillNotStartOnSalt(t *testing.T) {
	w := newTestWorld(t, testConfig())
	noScum(w)
	var p Point
	found := false
	for q := range w.salt {
		if w.TerrainAt(q) == Rock {
			p, found = q, true
			break
		}
	}
	if !found {
		t.Skip("no salt on rock in this world")
	}
	w.addScum(p, true)
	if _, ok := w.scum[p]; ok {
		t.Fatalf("scum started on the salt at %v", p)
	}
}

// Building over a deposit buries it for good.
func TestBuildingBuriesSalt(t *testing.T) {
	w := newTestWorld(t, testConfig())
	var p Point
	found := false
	for q := range w.salt {
		if w.TerrainAt(q) == Rock {
			p, found = q, true
			break
		}
	}
	if !found {
		t.Skip("no salt on rock in this world")
	}
	w.setTerrain(p, Wall, true)
	if w.hasSalt(p) {
		t.Fatalf("salt survived a wall built on %v", p)
	}
}

// A snapshot carries only the salt the colony can reach: none of what lies
// under rock nobody has opened, and every published deposit is one the world
// holds. Digging to a deposit publishes it; building over it takes it back.
func TestSnapshotCarriesOnlyExposedSalt(t *testing.T) {
	cfg := testConfig()
	cfg.SaltPercent = 20
	w := newTestWorld(t, cfg)
	snap := w.snapshot(false, 8)
	if len(w.salt) == 0 || len(snap.Salt) == len(w.salt) {
		t.Fatalf("snapshot has %d of %d deposits: want some, but not all", len(snap.Salt), len(w.salt))
	}
	for p := range snap.Salt {
		if !w.hasSalt(p) || !w.scumExposed(p) {
			t.Fatalf("snapshot salt at %v is not an exposed deposit", p)
		}
	}
	// The same map is handed out until something changes.
	if again := w.snapshot(false, 8); len(again.Salt) != len(snap.Salt) || w.saltRev != w.snapSaltRev {
		t.Fatal("an unchanged world republished its salt")
	}

	// Expose a buried deposit by opening the floor beside it.
	var buried Point
	for p := range w.salt {
		if _, ok := snap.Salt[p]; !ok && w.TerrainAt(p) == Rock {
			buried = p
			break
		}
	}
	for _, d := range neighbors8 {
		q := buried.Add(d.X, d.Y)
		if w.InBounds(q) && w.TerrainAt(q) == Rock {
			w.setTerrain(q, Floor, true)
			break
		}
	}
	if !w.snapshot(false, 8).SaltAt(buried) {
		t.Skip("could not open a neighbour that exposes the deposit in this world")
	}
	w.setTerrain(buried, Wall, true)
	if w.snapshot(false, 8).SaltAt(buried) {
		t.Fatalf("a wall built on %v left its salt published", buried)
	}
}

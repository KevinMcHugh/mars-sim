package sim

import (
	"strings"
	"testing"
)

// sandboxConfig is testConfig in a sandbox, with everything a sandbox has to
// hold back turned up: aliens, rats and cats to place, a nest in every
// cavern, a director wave, and two species to choose between.
func sandboxConfig() Config {
	cfg := testConfig()
	cfg.Sandbox = true
	cfg.StartAliens, cfg.StartRats, cfg.StartCats = 3, 8, 2
	cfg.CavernNestPercent = 100
	cfg.AlienSpeciesCount = 2
	cfg.Schedules = []Schedule{{
		Name: "wave", EarliestTick: 1, LatestTick: 1,
		Occurrences: []DirectorOccurrence{{Kind: OccArrival, Count: 3}},
	}}
	return cfg
}

func TestSandboxStartsEmpty(t *testing.T) {
	cfg := sandboxConfig()
	w := NewEngine(cfg).world
	center := Point{w.Width / 2, w.Height / 2, LandingLevel}
	if !w.Walkable(center) || !w.discovered(center) {
		t.Fatalf("no landing cavern at %v", center)
	}
	if len(w.aloft) != 0 || len(w.ships) != 0 {
		t.Fatalf("%d ships aloft and %d landed, want none", len(w.aloft), len(w.ships))
	}
	// A cavern broken into rolls no nest, and the director's wave never comes.
	w.rollNests([]Point{center})
	for range 5 {
		w.step()
	}
	for k := Kind(0); k < numKinds; k++ {
		if n := w.countKind(k); n != 0 {
			t.Errorf("%d %s in a sandbox nobody placed anything in", n, k)
		}
	}
}

func TestPlaceInSandbox(t *testing.T) {
	eng := NewEngine(sandboxConfig())
	w := eng.world
	center := Point{w.Width / 2, w.Height / 2, LandingLevel}
	next := center.Add(1, 0)

	eng.apply(Place{Kind: Colonist, At: center})
	col := w.entities[w.lay(center).occ.at(center.X, center.Y)]
	if col == nil || col.Kind != Colonist {
		t.Fatalf("no colonist on %v after placing one", center)
	}
	// It brings its locker's meals in its pockets, and testConfig's one
	// rare item, a pistol.
	if col.ownCarried(Meal) == 0 || !col.Inventory.Has(Pistol) {
		t.Errorf("placed colonist carries %d meals and pistol=%v, want some and true", col.ownCarried(Meal), col.Inventory.Has(Pistol))
	}
	eng.apply(Place{Kind: Alien, At: next, Species: 1})
	a := w.entities[w.lay(next).occ.at(next.X, next.Y)]
	if a == nil || a.Kind != Alien || a.Species != 1 {
		t.Fatalf("placed alien = %+v, want species 1 on %v", a, next)
	}
	if w.dormant(a) {
		t.Errorf("an alien placed on the colony's floor is dormant")
	}
	if !strings.Contains(lastLog(w), "Sandbox:") {
		t.Errorf("last log line %q, want the placement", lastLog(w))
	}

	// Refusals change nothing and say why.
	for _, c := range []struct {
		name string
		cmd  Place
		why  string
	}{
		{"occupied", Place{Kind: Colonist, At: center}, "already stands"},
		{"rock", Place{Kind: Alien, At: Point{0, 0, LandingLevel}, Species: -1}, "only on open floor"},
		{"off the map", Place{Kind: Colonist, At: Point{-1, 0, LandingLevel}}, "off the map"},
		{"no such species", Place{Kind: Alien, At: center.Add(2, 0), Species: 2}, "no alien species 2"},
	} {
		eng.apply(c.cmd)
		if !strings.Contains(lastLog(w), c.why) {
			t.Errorf("%s: last log line %q, want it to say %q", c.name, lastLog(w), c.why)
		}
	}
	if w.countKind(Colonist) != 1 || w.countKind(Alien) != 1 {
		t.Errorf("%d colonists and %d aliens after the refusals, want 1 and 1", w.countKind(Colonist), w.countKind(Alien))
	}
}

func TestPlaceRefusedOutsideSandbox(t *testing.T) {
	eng := NewEngine(testConfig())
	w := eng.world
	before := w.countKind(Alien)
	p, ok := w.randomFloor()
	if !ok {
		t.Fatal("no free floor")
	}
	eng.apply(Place{Kind: Alien, At: p, Species: -1})
	if w.countKind(Alien) != before || !strings.Contains(lastLog(w), "sandbox") {
		t.Fatalf("placed an alien outside a sandbox (log %q)", lastLog(w))
	}
}

// On the game's own defaults (no free food), a placed colonist outlives its
// first day: it brings its meals, as a ship's passenger finds them in its
// locker. Without them, the first sandbox colonists starved by nightfall.
func TestPlacedColonistEats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed, cfg.Width, cfg.Height = 7, 128, 128
	cfg.Sandbox = true
	eng := NewEngine(cfg)
	w := eng.world
	eng.apply(Place{Kind: Colonist, At: Point{w.Width / 2, w.Height / 2, LandingLevel}})
	for range 2000 {
		w.step()
	}
	if w.countKind(Colonist) != 1 {
		t.Fatalf("the placed colonist died: %q", lastLog(w))
	}
}

// A sandbox asks for no counts: its landing cavern is the same whatever the
// colonists setting says, and a species pack goes into its roster whole,
// whatever alien-species-count says.
func TestSandboxIgnoresCounts(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 200, 200
	cfg.Sandbox = true
	cfg.StartColonists = 1
	few := NewEngine(cfg).world.countTerrain(Floor)
	cfg.StartColonists = 60
	if many := NewEngine(cfg).world.countTerrain(Floor); few != many {
		t.Errorf("landing cavern of %d tiles for 1 colonist, %d for 60: want the same", few, many)
	}

	cfg = testConfig()
	cfg.Sandbox = true
	cfg.SpeciesPack = testPack(t, seedsWithLife(t, 4)...)
	cfg.AlienSpeciesCount = 1
	if n := len(NewEngine(cfg).world.alienSpecies); n != 4 {
		t.Errorf("sandbox roster of %d species from a pack of 4, want all 4", n)
	}
}

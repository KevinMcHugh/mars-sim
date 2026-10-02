package sim

import (
	"reflect"
	"slices"
	"testing"
)

// sortedSalt lists the world's deposits in row order, so a test that picks
// one picks the same one every run (map order would not).
func sortedSalt(w *World) []Point {
	out := make([]Point, 0, len(w.landing().salt))
	for p := range w.landing().salt {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Point) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		return a.X - b.X
	})
	return out
}

// firstSaltOnRock is the first deposit, in row order, that lies on rock.
func firstSaltOnRock(t *testing.T, w *World) Point {
	t.Helper()
	for _, p := range sortedSalt(w) {
		if w.TerrainAt(p) == Rock {
			return p
		}
	}
	t.Fatal("no salt on rock in this world")
	return Point{}
}

// Salt is laid down by generation and only ever lost after that: through a
// long run, every deposit is one the pure generator placed (so nothing grows
// salt, however big the map and whatever chunks play generates), and none
// shares a tile with scum, however hard scum grows around it.
func TestSaltNeverRegeneratesOrMeetsScum(t *testing.T) {
	cfg := testConfig()
	cfg.SaltPercent = 20
	cfg.ScumSpawnPPM, cfg.ScumSpreadPercent = 20000, 90 // grow scum hard
	cfg.ScumPercent = 40
	w := newTestWorld(t, cfg)
	if len(w.landing().salt) == 0 {
		t.Fatal("no salt was generated at 20%")
	}
	fresh := newWorldGen(cfg, LandingLevel)
	generated := func(p Point) bool {
		c := fresh.chunk(p.X>>genChunkBits, p.Y>>genChunkBits)
		return c.isSalt(offset(p.X, p.Y))
	}
	for i := 0; i < 600; i++ {
		w.step()
		for _, p := range sortedSalt(w) {
			if !generated(p) {
				t.Fatalf("tick %d: salt on %v that generation never placed", w.tick, p)
			}
			if _, scum := w.landing().scum[p]; scum {
				t.Fatalf("tick %d: %v holds both salt and scum", w.tick, p)
			}
		}
	}
	if len(w.landing().scum) == 0 {
		t.Fatal("scum never grew, so the test proved nothing")
	}
}

// addScum refuses a salt tile outright, with room to spare and rock under it.
func TestScumWillNotStartOnSalt(t *testing.T) {
	w := newTestWorld(t, testConfig())
	noScum(w)
	p := firstSaltOnRock(t, w)
	w.addScum(p, true)
	if _, ok := w.landing().scum[p]; ok {
		t.Fatalf("scum started on the salt at %v", p)
	}
}

// Building over a deposit buries it for good: tearing the structure back out
// to floor, and playing on, does not bring it back.
func TestBuildingBuriesSalt(t *testing.T) {
	w := newTestWorld(t, testConfig())
	p := firstSaltOnRock(t, w)
	w.setTerrain(p, Wall, true)
	if w.hasSalt(p) {
		t.Fatalf("salt survived a wall built on %v", p)
	}
	w.setTerrain(p, Floor, true)
	for range 50 {
		w.step()
	}
	if w.hasSalt(p) {
		t.Fatalf("salt came back on %v after the wall was removed", p)
	}
}

// openFrontier is the first rock tile, in row order, that borders discovered
// floor and has a rock neighbour of its own that does not (so salt on that
// neighbour is buried until the frontier tile is dug). It returns both.
func openFrontier(t *testing.T, w *World) (dig, buried Point) {
	t.Helper()
	exposed := func(p Point) bool {
		for _, d := range neighbors8 {
			if q := p.Add(d.X, d.Y); w.Walkable(q) && w.discovered(q) {
				return true
			}
		}
		return false
	}
	for y := 1; y < w.Height-1; y++ {
		for x := 1; x < w.Width-1; x++ {
			dig = Point{x, y, LandingLevel}
			if w.TerrainAt(dig) != Rock || !exposed(dig) {
				continue
			}
			for _, d := range neighbors8 {
				b := dig.Add(d.X, d.Y)
				if w.InBounds(b) && w.TerrainAt(b) == Rock && !exposed(b) {
					return dig, b
				}
			}
		}
	}
	t.Fatal("no frontier rock with buried rock behind it")
	return Point{}, Point{}
}

// A snapshot carries exactly the salt the colony can reach: every exposed
// deposit and nothing under rock nobody has opened. Digging to a deposit
// publishes it, and building over it takes it back.
func TestSnapshotCarriesExactlyExposedSalt(t *testing.T) {
	cfg := testConfig()
	cfg.SaltPercent = 20
	w := newTestWorld(t, cfg)
	exact := func(when string, snap *Snapshot) {
		t.Helper()
		published := 0
		for _, p := range sortedSalt(w) {
			if want := w.scumExposed(p); snap.SaltAt(p) != want {
				t.Fatalf("%s: salt at %v published=%v, exposed=%v", when, p, !want, want)
			}
			if snap.SaltAt(p) {
				published++
			}
		}
		if published != len(snap.Salt) {
			t.Fatalf("%s: snapshot has %d deposits, %d of them the world's", when, len(snap.Salt), published)
		}
	}
	snap := w.snapshot(false, 8)
	exact("tick 0", snap)
	if len(snap.Salt) == 0 || len(snap.Salt) == len(w.landing().salt) {
		t.Fatalf("snapshot has %d of %d deposits: want some, but not all", len(snap.Salt), len(w.landing().salt))
	}

	// An unchanged world hands out the very same map, which is what the wire
	// encoder's change signal relies on.
	if again := w.snapshot(false, 8); reflect.ValueOf(again.Salt).Pointer() != reflect.ValueOf(snap.Salt).Pointer() {
		t.Fatal("an unchanged world published a new salt map")
	}

	// Put a deposit behind the frontier, where nobody can reach it yet.
	dig, buried := openFrontier(t, w)
	w.clearScum(buried)
	w.landing().salt[buried] = struct{}{}
	if w.snapshot(false, 8).SaltAt(buried) {
		t.Fatalf("salt at %v is published before anything opens it", buried)
	}
	w.setTerrain(dig, Floor, true) // a colonist digs the frontier tile
	exact("after the dig", w.snapshot(false, 8))
	if !w.snapshot(false, 8).SaltAt(buried) {
		t.Fatalf("digging %v did not publish the salt beside it at %v", dig, buried)
	}
	w.setTerrain(buried, Wall, true)
	exact("after the wall", w.snapshot(false, 8))
	if w.snapshot(false, 8).SaltAt(buried) {
		t.Fatalf("a wall built on %v left its salt published", buried)
	}
}

// Breaking into a natural cavern publishes the salt on its floor and its rim
// all at once. Cavern floor is revealed without a TileChanged, so this is
// discoverCavernTile's job; the tiles here are far from the breach, where the
// dig's own TileChanged reaches nothing.
func TestBreachingACavernPublishesItsSalt(t *testing.T) {
	w := newTestWorld(t, cavernTestConfig())
	h := hiddenFloorTiles(w)[0]
	var breach Point
	found := false
	for _, d := range veinNeighbors {
		if q := h.Add(d.X, d.Y); w.TerrainAt(q) == Rock {
			breach, found = q, true
			break
		}
	}
	if !found {
		t.Fatalf("hidden tile %v has no rock beside it to dig through", h)
	}
	// The floor tile farthest from the breach, and a rock tile of the rim
	// that is also out of the dig's reach (more than one tile from it).
	cave := floorComponent(w, h)
	floor := cave[0]
	for _, p := range cave {
		if p.Chebyshev(breach) > floor.Chebyshev(breach) {
			floor = p
		}
	}
	var rim Point
	found = false
	for _, p := range cave {
		for _, d := range neighbors8 {
			if q := p.Add(d.X, d.Y); !found && w.InBounds(q) && w.TerrainAt(q) == Rock && q.Chebyshev(breach) > 2 {
				rim, found = q, true
			}
		}
	}
	if floor.Chebyshev(breach) <= 2 || !found {
		t.Fatalf("cave at %v is too small to put salt out of the breach's reach", h)
	}
	for _, p := range []Point{floor, rim} {
		w.clearScum(p)
		w.landing().salt[p] = struct{}{}
		if w.snapshot(false, 8).SaltAt(p) {
			t.Fatalf("salt at %v in an unfound cavern is published", p)
		}
	}
	w.SetTerrain(breach, Floor)
	snap := w.snapshot(false, 8)
	for _, p := range []Point{floor, rim} {
		if !snap.SaltAt(p) {
			t.Fatalf("salt at %v not published after the cavern was breached at %v", p, breach)
		}
	}
}

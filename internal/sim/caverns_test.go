package sim

import (
	"math/rand"
	"strings"
	"testing"
)

func cavernTestConfig() Config {
	cfg := testConfig()
	cfg.Width, cfg.Height = 120, 60
	return cfg
}

// floorComponent returns every Floor tile 8-connected to start.
func floorComponent(w *World, start Point) []Point {
	seen := map[Point]bool{start: true}
	out := []Point{start}
	for i := 0; i < len(out); i++ {
		for _, d := range neighbors8 {
			q := out[i].Add(d.X, d.Y)
			if !seen[q] && w.InBounds(q) && w.TerrainAt(q) == Floor {
				seen[q] = true
				out = append(out, q)
			}
		}
	}
	return out
}

func hiddenFloorTiles(w *World) []Point {
	var out []Point
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if p := (Point{x, y}); w.TerrainAt(p) != Rock && !w.discovered(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// Worldgen hollows out caverns the colony knows nothing about: they are floor,
// but unexplored, well clear of the landing site, not mining frontier, not
// counted as dug, and no one is put in them.
func TestCavernsGenerateHidden(t *testing.T) {
	cfg := cavernTestConfig()
	w := newTestWorld(t, cfg)

	hidden := hiddenFloorTiles(w)
	if len(hidden) == 0 {
		t.Fatal("no natural caverns generated")
	}
	if w.hiddenFloor != len(hidden) {
		t.Fatalf("hiddenFloor = %d, but %d unexplored floor tiles exist", w.hiddenFloor, len(hidden))
	}
	target := len(w.tiles) * cfg.CavernPercent / 100
	if len(hidden) < target {
		t.Fatalf("caverns cover %d tiles, want at least %d (%d%%)", len(hidden), target, cfg.CavernPercent)
	}

	var known []Point
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if p := (Point{x, y}); w.TerrainAt(p) == Floor && w.discovered(p) {
				known = append(known, p)
			}
		}
	}
	for _, h := range hidden {
		for _, k := range known {
			if d := h.Chebyshev(k); d <= cavernLandingClearance {
				t.Fatalf("hidden cavern tile %v is only %d from landing floor %v", h, d, k)
			}
		}
		for _, d := range neighbors8 {
			if q := h.Add(d.X, d.Y); w.board.isFrontier(q) {
				t.Fatalf("rock %v beside undiscovered cavern %v is mining frontier", q, h)
			}
		}
	}
	if got := w.snapshot(false, 8).Stats.FloorDug; got != len(known) {
		t.Fatalf("FloorDug = %d, want the %d discovered floor tiles", got, len(known))
	}
	for _, e := range w.entities {
		if e.Kind != Alien && !w.discovered(e.Pos) {
			t.Fatalf("%v spawned at %v, in an undiscovered cavern", e.Kind, e.Pos)
		}
	}
	for i := 0; i < 500; i++ {
		if p, ok := w.randomFloor(); ok && !w.discovered(p) {
			t.Fatalf("randomFloor returned undiscovered cavern floor %v", p)
		}
	}
}

func TestCavernPercentZeroGeneratesNone(t *testing.T) {
	cfg := cavernTestConfig()
	cfg.CavernPercent = 0
	w := newTestWorld(t, cfg)
	if w.hiddenFloor != 0 || len(hiddenFloorTiles(w)) != 0 {
		t.Fatalf("CavernPercent 0 still generated %d hidden floor tiles", w.hiddenFloor)
	}
}

// Digging through to a cavern lifts the fog off the whole connected cave
// system at once, turns its walls into mining frontier, folds it into the
// colony's room, and says so in the log.
func TestBreachingACavernRevealsItsWholeSystem(t *testing.T) {
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
	cave := floorComponent(w, h)
	before := w.hiddenFloor

	w.SetTerrain(breach, Floor)
	w.refreshSpatial()

	if got, want := w.hiddenFloor, before-len(cave); got != want {
		t.Fatalf("hiddenFloor = %d after the breach, want %d (%d - the %d-tile system)", got, want, before, len(cave))
	}
	frontier := 0
	for _, p := range cave {
		if !w.Explored(p) {
			t.Fatalf("cavern tile %v still unexplored after the breach", p)
		}
		for _, d := range neighbors8 {
			q := p.Add(d.X, d.Y)
			if !w.InBounds(q) {
				continue
			}
			if !w.Explored(q) {
				t.Fatalf("rim tile %v of the breached cavern still unexplored", q)
			}
			if w.TerrainAt(q) == Rock {
				if !w.board.isFrontier(q) {
					t.Fatalf("cavern wall %v is not mining frontier after the breach", q)
				}
				frontier++
			}
		}
	}
	if frontier == 0 {
		t.Fatal("breached cavern has no rock walls at all")
	}
	if _, ok := w.discoveredRooms[w.roomOf(h)]; !ok {
		t.Fatalf("breached cavern's room %d is not counted as discovered", w.roomOf(h))
	}
	logged := false
	for _, msg := range w.log.tail(5) {
		logged = logged || strings.Contains(msg, "natural cavern")
	}
	if !logged {
		t.Fatalf("no log line for the breach; log tail: %q", w.log.tail(5))
	}
	checkRoomLabels(t, w)
}

// caverns generates caverns alone (no landing site or entities) so a test can
// look at them by cavern.
func cavernsOnly(t *testing.T, passagePercent int) (*World, []cavern) {
	t.Helper()
	cfg := cavernTestConfig()
	cfg.Width, cfg.Height = 200, 100
	cfg.CavernPassagePercent = passagePercent
	w := newWorld(cfg, rand.New(rand.NewSource(1)))
	c := Point{w.Width / 2, w.Height / 2}
	caves := w.generateCaverns(rand.New(rand.NewSource(9)), c, c)
	w.refreshSpatial()
	if len(caves) < 4 {
		t.Fatalf("only %d caverns on a %dx%d map", len(caves), w.Width, w.Height)
	}
	return w, caves
}

// With passages off, every cavern is its own sealed pocket: carving keeps them
// apart.
func TestCavernsWithoutPassagesAreSeparate(t *testing.T) {
	w, caves := cavernsOnly(t, 0)
	if w.roomCount != len(caves) {
		t.Fatalf("%d caverns form %d rooms; without passages each should be its own", len(caves), w.roomCount)
	}
}

// With passages always on, every cavern is joined to its nearest neighbor
// (within passage range).
func TestCavernPassagesJoinNearestNeighbors(t *testing.T) {
	w, caves := cavernsOnly(t, 100)
	joined := 0
	for i, c := range caves {
		nearest, best := -1, 1<<30
		for j, d := range caves {
			if j != i && c.center.Chebyshev(d.center) < best {
				nearest, best = j, c.center.Chebyshev(d.center)
			}
		}
		if best > passageMaxSpan {
			continue
		}
		if !w.sameRoom(c.center, caves[nearest].center) {
			// A passage may legitimately fail to route (it wanders too long);
			// count rather than demand every one.
			continue
		}
		joined++
	}
	if joined == 0 || w.roomCount >= len(caves) {
		t.Fatalf("passages joined %d caverns into %d rooms from %d caverns", joined, w.roomCount, len(caves))
	}
}

// Same seed, same caverns.
func TestCavernsAreDeterministic(t *testing.T) {
	a := newTestWorld(t, cavernTestConfig())
	b := newTestWorld(t, cavernTestConfig())
	for i := range a.tiles {
		if a.tiles[i] != b.tiles[i] {
			t.Fatalf("tile %d differs between two worlds from the same seed", i)
		}
	}
}

// Room labels stay exact under random digging and building on a map full of
// hidden caverns, including edits that breach them.
func TestRoomLabelsWithCaverns(t *testing.T) {
	w := newTestWorld(t, cavernTestConfig())
	checkRoomLabels(t, w)
	rng := rand.New(rand.NewSource(11))
	hidden := hiddenFloorTiles(w)
	for step := 0; step < 300; step++ {
		var p Point
		if step%10 == 0 && len(hidden) > 0 {
			p = hidden[rng.Intn(len(hidden))].Add(rng.Intn(3)-1, rng.Intn(3)-1)
		} else {
			p = Point{rng.Intn(w.Width), rng.Intn(w.Height)}
		}
		switch rng.Intn(3) {
		case 0:
			w.SetTerrain(p, Floor)
		case 1:
			w.SetTerrain(p, Rock)
		default:
			w.SetTerrain(p, Wall)
		}
		w.refreshSpatial()
		checkRoomLabels(t, w)
		if got := len(hiddenFloorTiles(w)); got != w.hiddenFloor {
			t.Fatalf("edit %d at %v: hiddenFloor %d, but %d unexplored floor tiles", step, p, w.hiddenFloor, got)
		}
	}
}

package sim

import "testing"

// nearCarved reports whether p or one of its eight neighbors has been carved or
// built — the predicate SetTerrain's reveal implements. Terrain only ever moves
// away from Rock in a real game, so for a freshly generated (or freshly mined)
// world this is exactly the explored set.
func nearCarved(w *World, p Point) bool {
	if w.TerrainAt(p) != Rock {
		return true
	}
	for _, d := range neighbors8 {
		q := p.Add(d.X, d.Y)
		if w.InBounds(q) && w.TerrainAt(q) != Rock {
			return true
		}
	}
	return false
}

// Fog of war lifts exactly one tile past whatever the colony has touched: after
// worldgen the landing cavern and the rock rim around it are explored, and the
// rest of the map — including the rock the aliens are lurking in — is not.
func TestWorldgenRevealsTheCavernAndItsRim(t *testing.T) {
	w := newTestWorld(t, testConfig())

	dark, litRock := 0, 0
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Point{x, y}
			want := nearCarved(w, p)
			if got := w.Explored(p); got != want {
				t.Fatalf("explored(%v) = %v, want %v (terrain %v)", p, got, want, w.TerrainAt(p))
			}
			switch {
			case !want:
				dark++
			case w.TerrainAt(p) == Rock:
				litRock++
			}
		}
	}
	if dark == 0 {
		t.Error("the whole map starts explored; there is no fog to lift")
	}
	if litRock == 0 {
		t.Error("no rock is visible around the cavern; the player cannot see what to mine")
	}
}

// Stats.ExploredTiles is kept incrementally (see World.exploredCount) rather
// than recomputed by scanning the grid, so it has to be checked against an
// actual scan rather than trusted on its own.
func TestExploredTilesCountMatchesTheExploredSet(t *testing.T) {
	w := newTestWorld(t, testConfig())

	want := 0
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if w.Explored(Point{x, y}) {
				want++
			}
		}
	}
	if got := w.snapshot(false, 8).Stats.ExploredTiles; got != want {
		t.Fatalf("Stats.ExploredTiles = %d, want %d (scanned)", got, want)
	}

	// Digging further must move the counter by exactly the newly revealed
	// tiles, not double-count ones already lit (reveal is a no-op past the
	// first time — see World.reveal).
	before := w.snapshot(false, 8).Stats.ExploredTiles
	var target Point
	for p := range w.board.frontier {
		target = p
		break
	}
	newlyRevealed := 0
	for _, d := range append([]Point{{0, 0}}, neighbors8[:]...) {
		if q := target.Add(d.X, d.Y); w.InBounds(q) && !w.Explored(q) {
			newlyRevealed++
		}
	}
	w.SetTerrain(target, Floor)
	if got, want := w.snapshot(false, 8).Stats.ExploredTiles, before+newlyRevealed; got != want {
		t.Fatalf("Stats.ExploredTiles after digging = %d, want %d (%d before + %d newly revealed)",
			got, want, before, newlyRevealed)
	}
}

// With fog of war off the counter means nothing to a frontend (every tile reads
// as explored), so it is published as zero even though the simulation still
// keeps it.
func TestExploredTilesStaysZeroWithFogOff(t *testing.T) {
	cfg := testConfig()
	cfg.FogOfWar = false
	w := newTestWorld(t, cfg)

	if got := w.snapshot(false, 8).Stats.ExploredTiles; got != 0 {
		t.Fatalf("Stats.ExploredTiles with fog off = %d, want 0", got)
	}
}

// Digging lifts the fog one tile further: the ring of rock behind the tile just
// mined out becomes visible, and nothing beyond it does.
func TestDiggingLiftsTheFogAheadOfIt(t *testing.T) {
	w := newTestWorld(t, testConfig())

	// A frontier tile with something still dark behind it, which is every
	// frontier tile on a map bigger than the cavern.
	var target Point
	var hidden []Point
	for p := range w.board.frontier {
		for _, d := range neighbors8 {
			if q := p.Add(d.X, d.Y); w.InBounds(q) && !w.Explored(q) {
				hidden = append(hidden, q)
			}
		}
		if len(hidden) > 0 {
			target = p
			break
		}
	}
	if len(hidden) == 0 {
		t.Fatal("no frontier tile has unexplored rock behind it; nothing to test")
	}

	w.SetTerrain(target, Floor)
	for _, p := range hidden {
		if !w.Explored(p) {
			t.Errorf("mining %v left its neighbor %v in the dark", target, p)
		}
	}
	// One tile, not a sightline: the tile two steps past what was just dug is
	// still unknown.
	for _, p := range hidden {
		for _, d := range neighbors8 {
			q := p.Add(d.X, d.Y)
			if w.InBounds(q) && w.Explored(q) && !nearCarved(w, q) {
				t.Errorf("mining %v revealed %v, two tiles from any excavation", target, q)
			}
		}
	}
}

// A reveal has to reach the published grid, which means dirtying the page it
// landed on. Forgetting that leaves a frontend rendering fog over rock the
// colony dug up to several frames ago.
func TestRevealsReachThePublishedSnapshot(t *testing.T) {
	w := newTestWorld(t, testConfig())

	var target, hidden Point
	found := false
	for p := range w.board.frontier {
		for _, d := range neighbors8 {
			if q := p.Add(d.X, d.Y); w.InBounds(q) && !w.Explored(q) {
				target, hidden, found = p, q, true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("no frontier tile has unexplored rock behind it; nothing to test")
	}

	before := w.snapshot(false, 8)
	if !before.FogOfWar {
		t.Fatal("snapshot does not report fog of war as on")
	}
	if before.ExploredAt(hidden) {
		t.Fatalf("%v is published as explored before anything dug toward it", hidden)
	}

	w.SetTerrain(target, Floor)
	after := w.snapshot(false, 8)
	if !after.ExploredAt(hidden) {
		t.Errorf("the reveal of %v never reached the published grid", hidden)
	}
	// The frame already in a frontend's hands is immutable, reveals included.
	if before.ExploredAt(hidden) {
		t.Errorf("publishing a reveal changed a snapshot already handed out")
	}
}

// With fog of war off every in-bounds tile reads as explored, even though the
// simulation still tracks what the colony has discovered underneath (natural
// caverns depend on it — see docs/caverns.md).
func TestFogOfWarOffHidesNothing(t *testing.T) {
	cfg := testConfig()
	cfg.FogOfWar = false
	w := newTestWorld(t, cfg)

	snap := w.snapshot(false, 8)
	if snap.FogOfWar {
		t.Error("snapshot reports fog of war on with the setting off")
	}
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Point{x, y}
			if !w.Explored(p) || !snap.ExploredAt(p) {
				t.Fatalf("tile %v reads as unexplored with fog of war off", p)
			}
		}
	}
	if snap.ExploredAt(Point{-1, 0}) || snap.ExploredAt(Point{w.Width, 0}) {
		t.Error("out-of-bounds tiles read as explored")
	}
}

// Exploration never goes backwards, whatever the colony does to the terrain: a
// tile it has seen stays seen even once a wall is raised over the floor that
// revealed it.
func TestExplorationOnlyGrows(t *testing.T) {
	w := newTestWorld(t, testConfig())

	seen := map[Point]bool{}
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if p := (Point{x, y}); w.Explored(p) {
				seen[p] = true
			}
		}
	}

	for i := 0; i < 200; i++ {
		w.step()
	}
	// Wall off the cavern's rim for good measure: a build is a terrain change
	// that makes a tile less walkable, not more.
	for p := range w.board.frontier {
		w.SetTerrain(p, Wall)
		break
	}

	for p := range seen {
		if !w.Explored(p) {
			t.Fatalf("%v was explored and is not any more", p)
		}
	}
}

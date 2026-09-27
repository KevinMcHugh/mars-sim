package sim

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

func gridWorld(t testing.TB, size int) *World {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Seed = 7
	cfg.Width, cfg.Height = size, size
	return newWorld(cfg, rand.New(rand.NewSource(7)))
}

// A published grid must never change under a frontend that is still holding it:
// the engine keeps mutating the world between frames, and the renderer reads a
// snapshot on another goroutine.
func TestSnapshotTilesAreStableAfterLaterEdits(t *testing.T) {
	w := gridWorld(t, 200)
	p := Point{10, 10}
	w.SetTerrain(p, Floor)

	before := w.snapshot(false, 8)
	if got := before.TerrainAt(p); got != Floor {
		t.Fatalf("snapshot terrain = %v, want Floor", got)
	}

	w.SetTerrain(p, Wall)
	after := w.snapshot(false, 8)

	if got := before.TerrainAt(p); got != Floor {
		t.Errorf("earlier snapshot changed under us: %v, want Floor", got)
	}
	if got := after.TerrainAt(p); got != Wall {
		t.Errorf("new snapshot terrain = %v, want Wall", got)
	}
}

// Publishing must not re-copy the map. A tick that changed no terrain reuses the
// previous grid outright; a tick that changed one tile replaces exactly the page
// holding it and shares every other page with the frame before.
func TestPublishedTilesShareUnchangedPages(t *testing.T) {
	w := gridWorld(t, 200)
	w.SetTerrain(Point{10, 10}, Floor)
	first, _ := w.publishedTiles()

	if second, _ := w.publishedTiles(); second != first {
		t.Error("publishing with no terrain change allocated a new grid")
	}

	p := Point{100, 100}
	w.SetTerrain(p, Floor)
	third, _ := w.publishedTiles()
	if third == first {
		t.Fatal("publishing after a terrain change reused the stale grid")
	}

	changed := w.index(p) >> tilePageBits
	shared := 0
	for pi := range third.pages {
		switch {
		case pi == changed:
			if &third.pages[pi][0] == &first.pages[pi][0] {
				t.Error("the changed page was not copied before it diverged")
			}
		case &third.pages[pi][0] == &first.pages[pi][0]:
			shared++
		}
	}
	if want := len(third.pages) - 1; shared != want {
		t.Errorf("shared %d unchanged pages, want %d", shared, want)
	}
}

// Terrain totals are reported from the incremental counts, so they must agree
// with the grid the same snapshot published.
func TestSnapshotStatsMatchGrid(t *testing.T) {
	w := gridWorld(t, 64)
	for x := 5; x < 15; x++ {
		w.SetTerrain(Point{x, 5}, Floor)
	}
	w.SetTerrain(Point{5, 5}, NutrientPod)
	w.SetTerrain(Point{6, 5}, Toilet)
	w.SetTerrain(Point{7, 5}, Bed)
	w.SetTerrain(Point{8, 5}, Wall)

	s := w.snapshot(false, 8)
	var floor, pods, toilets, beds int
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			switch s.TerrainAt(Point{x, y}) {
			case Floor:
				floor++
			case NutrientPod:
				pods++
			case Toilet:
				toilets++
			case Bed:
				beds++
			}
		}
	}
	if s.Stats.FloorDug != floor || s.Stats.Pods != pods ||
		s.Stats.Toilets != toilets || s.Stats.Beds != beds {
		t.Errorf("stats %+v disagree with the grid (floor %d, pods %d, toilets %d, beds %d)",
			s.Stats, floor, pods, toilets, beds)
	}
}

// Out-of-bounds reads read as solid rock, in both the grid and the snapshot, so
// the renderer can walk a camera off the edge of the world.
func TestTileGridOutOfBoundsIsRock(t *testing.T) {
	w := gridWorld(t, 32)
	s := w.snapshot(false, 8)
	for _, p := range []Point{{-1, 0}, {0, -1}, {32, 0}, {0, 32}} {
		if got := s.TerrainAt(p); got != Rock {
			t.Errorf("TerrainAt(%v) = %v, want Rock", p, got)
		}
		if got := s.Tiles.At(p).Terrain; got != Rock {
			t.Errorf("Tiles.At(%v) = %v, want Rock", p, got)
		}
	}
}

// A grid built from a raw slice must read back the same tiles, including on the
// short final page when the tile count is not a multiple of the page size.
func TestNewTileGridRoundTrips(t *testing.T) {
	const width, height = 97, 61 // 5917 tiles: two pages, the second partial
	tiles := make([]Tile, width*height)
	for i := range tiles {
		tiles[i].Terrain = Terrain(i % int(numTerrains))
	}
	g := NewTileGrid(width, height, tiles)
	if g.Width() != width || g.Height() != height {
		t.Fatalf("grid is %dx%d, want %dx%d", g.Width(), g.Height(), width, height)
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if got, want := g.At(Point{x, y}).Terrain, tiles[y*width+x].Terrain; got != want {
				t.Fatalf("At(%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

// The whole point of the shared grid is that it is safe to hand across
// goroutines while the engine keeps digging. Run a real engine and read the
// terrain out of every frame from another goroutine: under -race, a page the
// engine mutated in place instead of copying would show up here.
func TestSnapshotTilesSafeForConcurrentReaders(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 3
	cfg.Width, cfg.Height = 80, 40
	cfg.TicksPerSecond = 60
	eng := NewEngine(cfg)
	snaps := eng.Subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Run(ctx)

	deadline := time.After(250 * time.Millisecond)
	frames := 0
	for {
		select {
		case s, ok := <-snaps:
			if !ok {
				return
			}
			frames++
			for y := 0; y < s.Height; y++ {
				for x := 0; x < s.Width; x++ {
					_ = s.TerrainAt(Point{x, y})
				}
			}
		case <-deadline:
			if frames == 0 {
				t.Fatal("no frames published")
			}
			return
		}
	}
}

// Refuse no longer rides the tile pages — it reaches frontends through the
// refuse index's own revision (see World.publishedRefuse). These pin the two
// halves of that: a refuse change with no terrain change still reaches the next
// frame, and a frame already handed out never changes underneath its reader.
func TestPublishedRefuseReachesFrontendsWithoutTerrainChange(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)
	spot := Point{cfg.Width / 2, cfg.Height / 2}
	w.SetTerrain(spot, Floor)
	w.snapshot(false, 1) // publish, so the tile's page is clean from here on

	w.addGore(spot)
	w.addCorpse(spot)
	got := w.snapshot(false, 1).TileAt(spot)
	if got.Gore != 1 || got.Corpses != 1 {
		t.Fatalf("published tile has gore %d, corpses %d; want 1 and 1 — a refuse change reached no frame",
			got.Gore, got.Corpses)
	}

	// The frame above is now in a frontend's hands. Cleaning the tile must not
	// alter it, and must show up in the frame after.
	held := w.snapshot(false, 1)
	if !w.takeGore(spot) || !w.takeCorpse(spot) {
		t.Fatal("expected refuse to take")
	}
	if got := held.TileAt(spot); got.Gore != 1 || got.Corpses != 1 {
		t.Errorf("a published frame changed under its reader: gore %d, corpses %d; want 1 and 1",
			got.Gore, got.Corpses)
	}
	if got := w.snapshot(false, 1).TileAt(spot); got.Gore != 0 || got.Corpses != 0 {
		t.Errorf("cleaned tile still publishes gore %d, corpses %d; want 0 and 0", got.Gore, got.Corpses)
	}
}

// Building on a tile scrapes off whatever was lying on it, and that has to
// reach frontends too — the refuse index and the terrain page change together.
func TestPublishedRefuseClearedByConstruction(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)
	spot := Point{cfg.Width / 2, cfg.Height / 2}
	w.SetTerrain(spot, Floor)
	w.addGore(spot)
	w.addCorpse(spot)
	if got := w.snapshot(false, 1).TileAt(spot); got.Gore == 0 || got.Corpses == 0 {
		t.Fatalf("setup: expected refuse on the tile, got %+v", got)
	}
	w.SetTerrain(spot, Wall)
	got := w.snapshot(false, 1).TileAt(spot)
	if got.Gore != 0 || got.Corpses != 0 {
		t.Errorf("wall published with gore %d, corpses %d; want both 0", got.Gore, got.Corpses)
	}
	if w.refuseTotal() != 0 {
		t.Errorf("refuseTotal = %d after clearing the only dirty tile, want 0", w.refuseTotal())
	}
	if len(w.refuse) != 0 {
		t.Errorf("refuse index kept %d entries for a clean map; setRefuse should drop them", len(w.refuse))
	}
}

// TileChanges must name exactly the pages a consumer has to resend, in both
// sharing modes: everything on the first frame, nothing on a quiet tick, and
// the one page a single edit touched.
func TestTileChangesReportDirtyPages(t *testing.T) {
	for _, mode := range []TileSharing{TilesCopyOnWrite, TilesLive} {
		w := gridWorld(t, 200)
		w.SetTileSharing(mode)

		if c := w.snapshot(false, 8).TileChanges; !c.All || len(c.Pages) != 0 {
			t.Errorf("mode %d: first frame changes = %+v, want All", mode, c)
		}
		if c := w.snapshot(false, 8).TileChanges; c.All || len(c.Pages) != 0 || c.Refuse {
			t.Errorf("mode %d: quiet frame changes = %+v, want none", mode, c)
		}
		p := Point{100, 100}
		w.SetTerrain(p, Floor)
		c := w.snapshot(false, 8).TileChanges
		if want := w.index(p) / TilePageLen; c.All || len(c.Pages) != 1 || c.Pages[0] != want {
			t.Errorf("mode %d: after one edit changes = %+v, want page %d", mode, c, want)
		}
		w.setRefuse(p, refuseCell{Gore: 1})
		if c := w.snapshot(false, 8).TileChanges; !c.Refuse || len(c.Pages) != 0 {
			t.Errorf("mode %d: after a death changes = %+v, want Refuse only", mode, c)
		}
	}
}

// Under TilesLive the published grid is the world's own map: no page is
// copied, and a frame read after an edit (on the engine's goroutine, which is
// the only place a live frame may be read) sees it.
func TestLiveTilesAliasTheWorld(t *testing.T) {
	w := gridWorld(t, 200)
	w.SetTileSharing(TilesLive)
	snap := w.snapshot(false, 8)
	for pi, page := range snap.Tiles.pages {
		if &page[0] != &w.tiles[pi*TilePageLen] {
			t.Fatalf("page %d was copied, want it to alias World.tiles", pi)
		}
	}
	p := Point{10, 10}
	w.SetTerrain(p, Floor)
	w.setRefuse(p, refuseCell{Corpses: 1})
	if got := snap.TileAt(p); got.Terrain != Floor || got.Corpses != 1 {
		t.Errorf("live grid read %+v after the edit, want Floor with a corpse", got)
	}
	if next := w.snapshot(false, 8); next.Tiles != snap.Tiles {
		t.Error("live mode allocated a new grid instead of reusing the live one")
	}
}

// Switching back to copy-on-write rebuilds a private grid, so frames published
// afterwards are immutable again.
func TestSwitchingOffLiveTilesRestoresCopies(t *testing.T) {
	w := gridWorld(t, 200)
	w.SetTileSharing(TilesLive)
	_ = w.snapshot(false, 8)
	w.SetTileSharing(TilesCopyOnWrite)
	snap := w.snapshot(false, 8)
	if !snap.TileChanges.All {
		t.Error("switching modes did not report a full resend")
	}
	p := Point{10, 10}
	w.SetTerrain(p, Floor)
	if got := snap.TerrainAt(p); got != Rock {
		t.Errorf("copy-on-write frame changed under us: %v, want Rock", got)
	}
}

// A live grid cannot cross goroutines, so the engine refuses to combine it
// with a channel subscriber in either order.
func TestShareLiveTilesExcludesSubscribers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Width, cfg.Height = 60, 40

	mustPanic := func(name string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		f()
	}

	e := NewEngine(cfg)
	e.ShareLiveTiles()
	mustPanic("Subscribe after ShareLiveTiles", func() { e.Subscribe() })

	e = NewEngine(cfg)
	e.Subscribe()
	mustPanic("ShareLiveTiles after Subscribe", e.ShareLiveTiles)

	// Once Run owns the world, switching its sharing mode would race.
	e = NewEngine(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	for {
		e.mu.Lock()
		running := e.running
		e.mu.Unlock()
		if running {
			break
		}
		time.Sleep(time.Millisecond)
	}
	mustPanic("ShareLiveTiles after Run", e.ShareLiveTiles)
	cancel()
	<-done
}

// Frame numbers every published snapshot consecutively, so a consumer that
// skips one (a Subscribe channel dropping a stale frame) can tell it missed
// the changes in between.
func TestTileChangesFrameCountsEveryPublish(t *testing.T) {
	w := gridWorld(t, 200)
	for want := uint64(1); want <= 3; want++ {
		if got := w.snapshot(false, 8).TileChanges.Frame; got != want {
			t.Fatalf("frame = %d, want %d", got, want)
		}
	}
	w.SetTileSharing(TilesLive)
	if c := w.snapshot(false, 8).TileChanges; c.Frame != 4 || !c.All {
		t.Errorf("after a mode switch changes = %+v, want frame 4 with All", c)
	}
}

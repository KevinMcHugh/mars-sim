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
	first := w.publishedTiles()

	if second := w.publishedTiles(); second != first {
		t.Error("publishing with no terrain change allocated a new grid")
	}

	p := Point{100, 100}
	w.SetTerrain(p, Floor)
	third := w.publishedTiles()
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

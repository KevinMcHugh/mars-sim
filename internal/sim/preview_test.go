package sim

import (
	"sync"
	"testing"
)

// With the fog off, a frontend reads ungenerated chunks through the preview.
// What it shows must be exactly what the chunk holds once the simulation
// generates it.
func TestPreviewMatchesGeneration(t *testing.T) {
	cfg, _ := lazyGoldenConfig(t)
	cfg.FogOfWar = false
	w := NewEngine(cfg).world
	snap := w.snapshot(false, 8)

	type seen struct {
		p Point
		t Tile
	}
	var previewed []seen
	for y := 0; y < w.Height; y += 5 {
		for x := 0; x < w.Width; x += 3 {
			p := Point{x, y, LandingLevel}
			if snap.Tiles.hasPage(p) {
				continue
			}
			previewed = append(previewed, seen{p, snap.TileAt(p)})
		}
	}
	if len(previewed) == 0 {
		t.Fatal("nothing was previewed: every chunk is already generated")
	}
	for cy := 0; cy < w.landing().gen.chunkRows(); cy++ {
		for cx := 0; cx < w.landing().gen.chunkCols(); cx++ {
			w.generateChunk(w.landing(), cx, cy)
		}
	}
	for _, s := range previewed {
		if got := w.TileAt(s.p); got.Terrain != s.t.Terrain || got.Composition != s.t.Composition {
			t.Fatalf("preview showed %v/%v at %v, generation made %v/%v", s.t.Terrain, s.t.Composition, s.p, got.Terrain, got.Composition)
		}
	}
}

// With the fog on, an ungenerated chunk is unexplored rock, as it always was:
// the preview is for showing the whole map, never for seeing through the fog.
func TestPreviewStaysBehindTheFog(t *testing.T) {
	cfg, _ := lazyGoldenConfig(t)
	cfg.FogOfWar = true
	snap := NewEngine(cfg).world.snapshot(false, 8)
	for y := 0; y < snap.Height; y += 11 {
		for x := 0; x < snap.Width; x += 7 {
			p := Point{x, y, LandingLevel}
			if snap.Tiles.hasPage(p) {
				continue
			}
			if tile := snap.TileAt(p); tile.Terrain != Rock || tile.Composition != OrdinaryRock || snap.ExploredAt(p) {
				t.Fatalf("ungenerated %v reads as %+v (explored=%v) with fog on", p, tile, snap.ExploredAt(p))
			}
		}
	}
}

// A frontend looking all over the map with the fog off must not change the
// game: the same seed plays out identically with and without it.
func TestPreviewNeverAffectsTheSimulation(t *testing.T) {
	cfg, ticks := lazyGoldenConfig(t)
	cfg.FogOfWar = false
	run := func(look bool) string {
		w := NewEngine(cfg).world
		for i := 0; i < ticks; i++ {
			w.step()
			if look && i%50 == 0 {
				snap := w.snapshot(false, 8)
				for y := 0; y < w.Height; y += 13 {
					for x := 0; x < w.Width; x += 13 {
						snap.TileAt(Point{x, y, LandingLevel})
						snap.TerrainAt(Point{x, y, LandingLevel})
					}
				}
			}
		}
		return goldenHash(w)
	}
	if plain, looked := run(false), run(true); plain != looked {
		t.Fatalf("previewing the map changed the game:\n without %s\n with    %s", plain, looked)
	}
}

// Every frontend shares one preview through the snapshots, on its own
// goroutine. Run under -race.
func TestPreviewIsSafeForConcurrentReaders(t *testing.T) {
	cfg, _ := lazyGoldenConfig(t)
	cfg.FogOfWar = false
	snap := NewEngine(cfg).world.snapshot(false, 8)
	var wg sync.WaitGroup
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			for y := r; y < snap.Height; y += 17 {
				for x := 0; x < snap.Width; x += 19 {
					snap.TileAt(Point{x, y, LandingLevel})
				}
			}
		}(r)
	}
	wg.Wait()
}

package sim

import (
	"slices"
	"testing"
)

func lazyGoldenConfig(t *testing.T) (Config, int) {
	t.Helper()
	for _, gc := range goldenCases {
		if gc.grows {
			return gc.cfg(), gc.ticks
		}
	}
	t.Fatal("no golden case generates chunks during play")
	return Config{}, 0
}

// Every chunk a lazily generated world holds is exactly what the pure
// generator says, however late in the game it was generated: same ore on
// every tile, same hidden floor on every tile the colony has not touched.
func TestLazyChunksMatchThePureGenerator(t *testing.T) {
	cfg, ticks := lazyGoldenConfig(t)
	// Scum accretes after generation (growScum); this test is about what the
	// generator laid down.
	cfg.ScumSpawnPPM, cfg.ScumSpreadPercent = 0, 0
	w := NewEngine(cfg).world
	check := func(when string) {
		fresh := newWorldGen(cfg)
		for _, k := range w.genChunks {
			want := fresh.chunk(int(k.cx), int(k.cy))
			lo, hi, _ := fresh.chunkBounds(k)
			for y := lo.Y; y <= hi.Y; y++ {
				for x := lo.X; x <= hi.X; x++ {
					got, off := w.tiles.at(x, y), offset(x, y)
					if got.Composition != want.comp[off] {
						t.Fatalf("%s: tile (%d,%d) has %v, the generator says %v", when, x, y, got.Composition, want.comp[off])
					}
					if !got.Explored && (got.Terrain == Floor) != want.isFloor(off) {
						t.Fatalf("%s: untouched tile (%d,%d) is %v, the generator says floor=%v", when, x, y, got.Terrain, want.isFloor(off))
					}
					if _, scum := w.scum[Point{x, y}]; !got.Explored && scum != want.isScum(off) {
						t.Fatalf("%s: untouched tile (%d,%d) has scum=%v, the generator says %v", when, x, y, scum, want.isScum(off))
					}
					// Salt is only ever lost, never grown, so a tile the colony
					// has not built on holds exactly what was generated.
					if _, salt := w.salt[Point{x, y}]; salt && !want.isSalt(off) {
						t.Fatalf("%s: tile (%d,%d) has salt the generator did not place", when, x, y)
					} else if !salt && want.isSalt(off) && w.tiles.at(x, y).Terrain == Rock {
						t.Fatalf("%s: untouched tile (%d,%d) lost its salt", when, x, y)
					}
				}
			}
		}
	}
	check("tick 0")
	// The run has to generate a chunk for the second check to mean anything.
	// How soon the colony digs into a new one depends on everything it does
	// (a change to drives moved it past the golden run's length), so run on,
	// up to three times as long, until it has.
	before := len(w.genChunks)
	for i := 0; i < 3*ticks && (i < ticks || len(w.genChunks) == before); i++ {
		w.step()
	}
	if len(w.genChunks) == before {
		t.Fatalf("no chunk was generated in %d ticks", 3*ticks)
	}
	check("after the run")
}

// The invariant the lazy scheme rests on: every chunk within WorldgenHalo of
// one holding a tile the colony has seen is generated, no tile outside a
// generated chunk has ever been written, and every creature stands in a
// generated chunk. Checked after every tick of a run that generates chunks
// and breaks into caves.
func TestGenerationStaysAheadOfExploration(t *testing.T) {
	cfg, ticks := lazyGoldenConfig(t)
	w := NewEngine(cfg).world
	cols, rows := w.gen.chunkCols(), w.gen.chunkRows()
	floodGenerated := false
	generated := func(cx, cy int) bool {
		return w.genDone[w.tiles.pageIndex(cx<<genChunkBits, cy<<genChunkBits)]
	}
	for tick := 0; tick <= ticks; tick++ {
		for cy := 0; cy < rows; cy++ {
			for cx := 0; cx < cols; cx++ {
				page := w.tiles.pageAt(cx<<genChunkBits, cy<<genChunkBits)
				if !generated(cx, cy) {
					if page != nil {
						t.Fatalf("tick %d: chunk (%d,%d) has tiles but was never generated", tick, cx, cy)
					}
					continue
				}
				seen := false
				for _, c := range page {
					seen = seen || c.Explored
				}
				if !seen {
					continue
				}
				h := max(1, cfg.WorldgenHalo)
				for y := max(0, cy-h); y <= min(rows-1, cy+h); y++ {
					for x := max(0, cx-h); x <= min(cols-1, cx+h); x++ {
						if !generated(x, y) {
							t.Fatalf("tick %d: chunk (%d,%d) has been seen but its neighbour (%d,%d) is not generated", tick, cx, cy, x, y)
						}
					}
				}
			}
		}
		for _, id := range w.entityIDsSorted() {
			if p := w.entities[id].Pos; !generated(p.X>>genChunkBits, p.Y>>genChunkBits) {
				t.Fatalf("tick %d: %v %d stands at %v, in a chunk that was never generated", tick, w.entities[id].Kind, id, p)
			}
		}
		if tick < ticks {
			chunks, breaches := len(w.genChunks), w.cavernBreaches
			w.step()
			if w.cavernBreaches > breaches && len(w.genChunks) > chunks {
				floodGenerated = true
			}
		}
	}
	if w.cavernBreaches == 0 {
		t.Fatal("the run never breached a cave, so the flood was not exercised")
	}
	// The case that matters most: a flood running into ground nobody had
	// generated yet, which has to generate it mid-flood.
	if !floodGenerated {
		t.Fatal("no breach generated a chunk, so a flood crossing into new ground was not exercised")
	}
}

// Reading the world, as a frontend or the golden hash does, must never
// generate a chunk: only the simulation's own exploration may, or two
// machines rendering different views would end up with different worlds.
func TestReadingNeverGenerates(t *testing.T) {
	cfg, _ := lazyGoldenConfig(t)
	w := NewEngine(cfg).world
	before := append([]chunkKey(nil), w.genChunks...)
	snap := w.snapshot(false, 8)
	for y := -1; y <= w.Height; y += 7 {
		for x := -1; x <= w.Width; x += 7 {
			p := Point{x, y}
			w.TileAt(p)
			w.TerrainAt(p)
			w.Walkable(p)
			w.discovered(p)
			w.Explored(p)
			snap.TerrainAt(p)
			snap.Tiles.At(p)
		}
	}
	goldenHash(w)
	if len(w.genChunks) != len(before) {
		t.Fatalf("reading the world generated %d chunks", len(w.genChunks)-len(before))
	}
}

// A new game on a huge map generates the landing site's neighbourhood and
// nothing else, and publishes only that.
func TestNewGameOnHugeMapGeneratesOnlyTheLandingSite(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 7
	cfg.Width, cfg.Height = 10000, 10000
	w := NewEngine(cfg).world
	// The landing cavern and its revealed rim are the only ground seen. On
	// this map they span a rectangle of chunks (the ellipse reaches its
	// bounding box at the ends of both axes), so exactly the chunks within
	// WorldgenHalo of that rectangle are generated.
	rx, ry := caveRadii(cfg.Width, cfg.Height, cfg.StartColonists)
	c := Point{cfg.Width / 2, cfg.Height / 2}
	h := cfg.WorldgenHalo
	x0, x1 := (c.X-rx-1)>>genChunkBits-h, (c.X+rx+1)>>genChunkBits+h
	y0, y1 := (c.Y-ry-1)>>genChunkBits-h, (c.Y+ry+1)>>genChunkBits+h
	var want []chunkKey
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			want = append(want, chunkKey{int32(x), int32(y)})
		}
	}
	if !slices.Equal(w.genChunks, want) {
		t.Fatalf("generated chunks %v for a new game, want %v", w.genChunks, want)
	}
	if n := w.tiles.pagesAllocated(); n != len(w.genChunks) {
		t.Fatalf("%d tile pages allocated for %d generated chunks", n, len(w.genChunks))
	}
	published := 0
	grid, _ := w.publishedTiles()
	for _, p := range grid.pages {
		if p != nil {
			published++
		}
	}
	if published != len(w.genChunks) {
		t.Fatalf("published %d pages for %d generated chunks", published, len(w.genChunks))
	}
	if aliens := w.countKind(Alien); aliens != cfg.StartAliens {
		t.Fatalf("placed %d of %d starting aliens", aliens, cfg.StartAliens)
	}
}

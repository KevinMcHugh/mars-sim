package sim

import (
	"math"
	"math/rand/v2"
	"testing"
)

func chunkTestConfig(seed int64) Config {
	cfg := DefaultConfig()
	cfg.Seed = seed
	cfg.Width, cfg.Height = 330, 200 // ragged: the last column and row of chunks are partial
	cfg.CavernPercent = 12
	return cfg
}

// A chunk's content must not depend on which chunks were generated before it,
// in what order, or on what the plan cache happened to remember. Generate
// every chunk in raster order with one generator, then again in a shuffled
// order with a generator whose cache holds almost nothing, then each chunk
// with a brand-new generator, and demand identical results.
func TestChunkGenerationIsOrderIndependent(t *testing.T) {
	for _, seed := range []int64{1, 7, 20260927} {
		// A cache this small makes plans get evicted and recomputed while
		// something still holds the old copy, which is how pointer identity
		// bugs show up. It also recomputes nearly everything, so keep the
		// sweep short.
		cacheSize := 16
		cfg := chunkTestConfig(seed)
		if seed == 1 {
			cacheSize = 3
			cfg.Width, cfg.Height = 150, 140
		}
		base := newWorldGen(cfg)
		cols, rows := base.chunkCols(), base.chunkRows()
		want := map[chunkKey]*chunkContent{}
		for cy := 0; cy < rows; cy++ {
			for cx := 0; cx < cols; cx++ {
				want[chunkKey{int32(cx), int32(cy)}] = base.chunk(cx, cy)
			}
		}

		forgetful := newWorldGen(cfg)
		for i := range forgetful.veins {
			forgetful.veins[i] = newGenCache[chunkKey, []Point](cacheSize)
		}
		forgetful.cands = newGenCache[chunkKey, []*genCavern](cacheSize)
		forgetful.kept = newGenCache[chunkKey, []*genCavern](cacheSize)
		forgetful.passages = newGenCache[chunkKey, [][]Point](cacheSize)
		forgetful.scum = newGenCache[chunkKey, []Point](cacheSize)
		forgetful.salt = newGenCache[chunkKey, []Point](cacheSize)
		forgetful.nearest = newGenCache[caveID, nearestRef](cacheSize)

		// Raster order, then a seeded shuffle, so a failure reproduces.
		order := make([]chunkKey, 0, len(want))
		for cy := 0; cy < rows; cy++ {
			for cx := 0; cx < cols; cx++ {
				order = append(order, chunkKey{int32(cx), int32(cy)})
			}
		}
		rng := rand.New(rand.NewPCG(uint64(seed), 99))
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

		for _, k := range order {
			if got := forgetful.chunk(int(k.cx), int(k.cy)); !sameChunk(got, want[k]) {
				t.Fatalf("seed %d: chunk %v differs when generated out of order with a tiny cache", seed, k)
			}
			if got := newWorldGen(cfg).chunk(int(k.cx), int(k.cy)); !sameChunk(got, want[k]) {
				t.Fatalf("seed %d: chunk %v differs when generated alone", seed, k)
			}
		}
	}
}

func sameChunk(a, b *chunkContent) bool {
	if a.comp != b.comp || a.floor != b.floor || a.scum != b.scum || a.salt != b.salt || len(a.caverns) != len(b.caverns) {
		return false
	}
	for i := range a.caverns {
		if a.caverns[i] != b.caverns[i] {
			return false
		}
	}
	return true
}

// Every feature a chunk plans must stay within the chunks around its owner,
// which is all a chunk generated alone looks at, or it would miss part of the
// feature. Caverns are also held to cavernReach from their center.
func TestChunkFeaturesStayWithinNeighbours(t *testing.T) {
	cfg := chunkTestConfig(3)
	cfg.RockVeinMax = 200 // a vein this long must stop at its box, not escape it
	g := newWorldGen(cfg)
	within := func(k chunkKey, p Point) bool {
		lo, hi, _ := g.chunkBounds(k)
		return p.X >= lo.X-genChunkSize && p.X <= hi.X+genChunkSize &&
			p.Y >= lo.Y-genChunkSize && p.Y <= hi.Y+genChunkSize
	}
	for cy := 0; cy < g.chunkRows(); cy++ {
		for cx := 0; cx < g.chunkCols(); cx++ {
			k := chunkKey{int32(cx), int32(cy)}
			for level := range veinLevels {
				for _, p := range g.veinPlan(level, k) {
					if !within(k, p) || !g.inMap(p) {
						t.Fatalf("chunk %v: level %d vein tile %v is out of reach", k, level, p)
					}
				}
			}
			for _, c := range g.keptCaverns(k) {
				for _, p := range c.tiles {
					if c.center.Chebyshev(p) > cavernReach || !within(k, p) {
						t.Fatalf("chunk %v: cavern at %v reaches %v", k, c.center, p)
					}
				}
			}
			for _, path := range g.passagePlan(k) {
				for _, p := range path {
					if !within(k, p) {
						t.Fatalf("chunk %v: passage tile %v is out of reach", k, p)
					}
				}
			}
			for _, p := range g.scumPlan(k) {
				if !within(k, p) || !g.inMap(p) {
					t.Fatalf("chunk %v: scum tile %v is out of reach", k, p)
				}
			}
			for _, p := range g.saltPlan(k) {
				if !within(k, p) || !g.inMap(p) {
					t.Fatalf("chunk %v: salt tile %v is out of reach", k, p)
				}
			}
		}
	}
}

// The order-independence test is only worth something if features really do
// cross chunk edges: check that veins, caverns and passages all do.
func TestChunkFeaturesCrossChunkEdges(t *testing.T) {
	g := newWorldGen(chunkTestConfig(7))
	chunkOf := func(p Point) chunkKey { return chunkKey{int32(p.X / genChunkSize), int32(p.Y / genChunkSize)} }
	var veins, caves, passages bool
	for cy := 0; cy < g.chunkRows(); cy++ {
		for cx := 0; cx < g.chunkCols(); cx++ {
			k := chunkKey{int32(cx), int32(cy)}
			for _, p := range g.veinPlan(0, k) {
				veins = veins || chunkOf(p) != k
			}
			for _, c := range g.keptCaverns(k) {
				for _, p := range c.tiles {
					caves = caves || chunkOf(p) != k
				}
			}
			for _, path := range g.passagePlan(k) {
				for _, p := range path {
					passages = passages || chunkOf(p) != k
				}
			}
		}
	}
	if !veins || !caves || !passages {
		t.Fatalf("crossing chunk edges: veins %v, caverns %v, passages %v; want all", veins, caves, passages)
	}
}

// Generation writes hidden cavern floor straight into the grid instead of
// through carveHidden, so no TileChanged fires for it. Prove that changes
// nothing: the same game, played with the events fired, ends identically,
// including across a breach into a cave.
func TestChunkApplyNeedsNoTileEvents(t *testing.T) {
	var gc goldenCase
	for _, c := range goldenCases {
		if c.breach {
			gc = c
		}
	}
	run := func(viaEvents bool) (string, int) {
		applyChunkViaCarveHidden = viaEvents
		defer func() { applyChunkViaCarveHidden = false }()
		w := NewEngine(gc.cfg()).world
		for i := 0; i < gc.ticks; i++ {
			w.step()
		}
		return goldenHash(w), w.cavernBreaches
	}
	direct, breaches := run(false)
	events, _ := run(true)
	if breaches == 0 {
		t.Fatal("the run never breached a cave, so it proves nothing about hidden floor")
	}
	if direct != events {
		t.Fatalf("direct write and carveHidden disagree after %d ticks:\n direct %s\n events %s", gc.ticks, direct, events)
	}
}

// Salt and scum never share a tile, however their runs cross chunk edges, and
// salt still lands on its share: it steps around scum rather than giving up
// tiles to it.
func TestSaltNeverSharesATileWithScum(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		cfg := chunkTestConfig(seed)
		cfg.ScumPercent, cfg.SaltPercent = 40, 40 // crowded, so runs of both meet at every edge
		g := newWorldGen(cfg)
		var salt int
		for cy := 0; cy < g.chunkRows(); cy++ {
			for cx := 0; cx < g.chunkCols(); cx++ {
				c := g.chunk(cx, cy)
				for off := range genChunkArea {
					if c.isSalt(off) && c.isScum(off) {
						t.Fatalf("seed %d chunk (%d,%d): tile offset %d holds both salt and scum", seed, cx, cy, off)
					}
					if c.isSalt(off) {
						salt++
					}
				}
			}
		}
		if got := 100 * float64(salt) / float64(cfg.Width*cfg.Height); math.Abs(got-40) > 4 {
			t.Errorf("seed %d: salt covers %.1f%% of the map, want about 40%%", seed, got)
		}
	}
}

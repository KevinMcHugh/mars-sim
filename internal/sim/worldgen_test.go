package sim

import "testing"

// Same seed, same deposits; and a map-sized sample lands near every target.
// Abundance is an expected value under chunked generation, not an exact count
// (see TestAbundanceDriftWithinTolerance for the tight check).
func TestRockVeinsAreDeterministicAndNearAbundanceTargets(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 200, 150
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.IronRockPercent, cfg.IceRockPercent, cfg.ClayRockPercent = 10, 5, 7
	cfg.RockVeinMin, cfg.RockVeinMax = 8, 16
	cfg.Seed = 314159

	first := NewEngine(cfg).world
	second := NewEngine(cfg).world
	ft, st := denseTiles(first), denseTiles(second)
	counts := map[RockComposition]int{}
	for i, tile := range ft {
		if tile.Composition != st[i].Composition {
			t.Fatalf("composition differs at tile %d for the same seed", i)
		}
		counts[tile.Composition]++
	}
	for comp, pct := range map[RockComposition]int{
		IronBearingRock:     cfg.IronRockPercent,
		WaterIceBearingRock: cfg.IceRockPercent,
		ClayBearingRock:     cfg.ClayRockPercent,
	} {
		want := float64(len(ft) * pct / 100)
		if got := float64(counts[comp]); got < 0.8*want || got > 1.2*want {
			t.Errorf("%s tiles = %.0f, want within 20%% of %.0f", comp, got, want)
		}
	}
}

// isolatedDeposit returns a deposit tile with no orthogonal neighbour of the
// same composition, if the world has one.
func isolatedDeposit(w *World) (Point, bool) {
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Point{x, y}
			composition := w.TileAt(p).Composition
			if composition == OrdinaryRock {
				continue
			}
			connected := false
			for _, d := range veinNeighbors {
				if w.InBounds(p.Add(d.X, d.Y)) &&
					w.TileAt(p.Add(d.X, d.Y)).Composition == composition {
					connected = true
					break
				}
			}
			if !connected {
				return p, true
			}
		}
	}
	return Point{}, false
}

func TestRockDepositsAreVeinsRatherThanIsolatedTiles(t *testing.T) {
	// The ordinary config, and a crowded one where veins of every
	// composition keep running into each other and the map edge.
	for _, tc := range []struct {
		name                   string
		iron, ice, uran, clay  int
		veinMin, veinMax, size int
	}{
		{"ordinary", 10, 5, 1, 7, 8, 16, 40},
		{"crowded", 30, 20, 10, 20, 4, 24, 40},
		{"tiny-veins", 10, 10, 10, 10, 1, 2, 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for seed := int64(1); seed <= 300; seed++ {
				cfg := testConfig()
				cfg.Width, cfg.Height = tc.size, tc.size*3/4
				cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
				cfg.IronRockPercent, cfg.IceRockPercent = tc.iron, tc.ice
				cfg.UraniumRockPercent, cfg.ClayRockPercent = tc.uran, tc.clay
				cfg.RockVeinMin, cfg.RockVeinMax = tc.veinMin, tc.veinMax
				cfg.Seed = seed
				if p, ok := isolatedDeposit(NewEngine(cfg).world); ok {
					t.Fatalf("seed %d: %s deposit at %v is an isolated tile", seed, NewEngine(cfg).world.TileAt(p).Composition, p)
				}
			}
		})
	}
	// The maps above fit in one chunk. Veins must also stay connected where
	// they meet veins of other levels, or of their own level, planned by
	// neighbouring chunks: check a crowded multi-chunk map straight from the
	// generator.
	t.Run("multi-chunk", func(t *testing.T) {
		for seed := int64(1); seed <= 30; seed++ {
			cfg := testConfig()
			cfg.Width, cfg.Height = 200, 130
			cfg.IronRockPercent, cfg.IceRockPercent = 30, 20
			cfg.UraniumRockPercent, cfg.ClayRockPercent = 10, 20
			cfg.RockVeinMin, cfg.RockVeinMax = 2, 24
			cfg.Seed = seed
			g := newWorldGen(cfg)
			chunks := map[chunkKey]*chunkContent{}
			comp := func(p Point) RockComposition {
				if !g.inMap(p) {
					return OrdinaryRock
				}
				k := chunkKey{int32(p.X / genChunkSize), int32(p.Y / genChunkSize)}
				if chunks[k] == nil {
					chunks[k] = g.chunk(int(k.cx), int(k.cy))
				}
				return chunks[k].comp[offset(p.X, p.Y)]
			}
			for y := 0; y < cfg.Height; y++ {
				for x := 0; x < cfg.Width; x++ {
					p := Point{x, y}
					c := comp(p)
					if c == OrdinaryRock {
						continue
					}
					connected := false
					for _, d := range veinNeighbors {
						connected = connected || comp(p.Add(d.X, d.Y)) == c
					}
					if !connected {
						t.Fatalf("seed %d: %s deposit at %v is an isolated tile", seed, c, p)
					}
				}
			}
		}
	})
}

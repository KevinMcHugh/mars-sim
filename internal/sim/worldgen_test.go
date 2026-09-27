package sim

import "testing"

func TestRockVeinsAreDeterministicAndMeetAbundanceTargets(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 40, 30
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.IronRockPercent, cfg.IceRockPercent, cfg.ClayRockPercent = 10, 5, 7
	cfg.RockVeinMin, cfg.RockVeinMax = 8, 16
	cfg.Seed = 314159

	first := NewEngine(cfg).world
	second := NewEngine(cfg).world
	if len(first.tiles) != len(second.tiles) {
		t.Fatal("same config generated different world sizes")
	}

	counts := map[RockComposition]int{}
	for i, tile := range first.tiles {
		if tile.Composition != second.tiles[i].Composition {
			t.Fatalf("composition differs at tile %d for the same seed", i)
		}
		counts[tile.Composition]++
	}
	if got, want := counts[IronBearingRock], len(first.tiles)*cfg.IronRockPercent/100; got != want {
		t.Fatalf("iron-bearing tiles = %d, want %d", got, want)
	}
	if got, want := counts[WaterIceBearingRock], len(first.tiles)*cfg.IceRockPercent/100; got != want {
		t.Fatalf("water ice-bearing tiles = %d, want %d", got, want)
	}
	if got, want := counts[ClayBearingRock], len(first.tiles)*cfg.ClayRockPercent/100; got != want {
		t.Fatalf("clay-bearing tiles = %d, want %d", got, want)
	}
}

func TestRockDepositsAreVeinsRatherThanIsolatedTiles(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 40, 30
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.IronRockPercent, cfg.IceRockPercent, cfg.ClayRockPercent = 10, 5, 7
	cfg.RockVeinMin, cfg.RockVeinMax = 8, 16
	cfg.Seed = 271828
	w := NewEngine(cfg).world

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
				t.Fatalf("%s deposit at %v is an isolated tile", composition, p)
			}
		}
	}
}

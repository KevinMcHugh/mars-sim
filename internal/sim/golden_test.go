package sim

import (
	"fmt"
	"testing"
)

// goldenCase pins what one seed produces: the generated world at tick 0 and
// the whole simulation after a number of ticks. The hashes are constants, not
// a comparison between two runs, so they catch what a lockstep test cannot:
// the same seed producing a different world on a different machine
// (tools/determinism-check.sh runs this test natively, under amd64 and under
// js/wasm) or after a change nobody meant to be a seed break.
//
// When a change is *meant* to alter what seeds produce (a worldgen rewrite, a
// new RNG draw on the simulation stream), update the constants in the same
// change and say so in the commit message. See docs/determinism.md.
type goldenCase struct {
	name  string
	cfg   func() Config
	ticks int
	gen   string // goldenHash at tick 0
	run   string // goldenHash after ticks
	// breach requires the run to break into a natural cavern, so the case
	// keeps covering the breach flood when a seed break re-pins it.
	breach bool
}

var goldenCases = []goldenCase{
	{
		name: "default-80x40",
		cfg: func() Config {
			c := DefaultConfig()
			c.Seed = 7
			return c
		},
		ticks: 400,
		gen:   "tiles=3097a8b594818f5a entities=b3bee26800da242b n=19 floor=277 hidden=180",
		run:   "tiles=23f0e5999ea404d2 entities=922fc2ea20977826 n=11 floor=385 hidden=180",
	},
	{
		// Bigger than one worldgen chunk in both directions, with enough cave
		// that digging breaks into some within the run.
		name: "caves-300x150",
		cfg: func() Config {
			c := DefaultConfig()
			c.Seed = 20260927
			c.Width, c.Height = 300, 150
			c.StartColonists = 20
			c.CavernPercent = 15
			return c
		},
		ticks:  600,
		breach: true,
		gen:    "tiles=c5135e7117d9eac4 entities=1168e71108ae1768 n=33 floor=7615 hidden=7394",
		run:    "tiles=4ad7e77b53e4c1c9 entities=6dcbc445eebcb6c5 n=25 floor=7943 hidden=7186",
	},
}

// goldenHash condenses the simulation-visible world into one line: every
// tile's terrain, composition and discovery, then every entity in ID order.
// It reads through TileAt and discovered rather than the backing store, so it
// describes what the simulation sees however tiles happen to be stored.
func goldenHash(w *World) string {
	tiles := fnvSeed
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Point{x, y}
			t := w.TileAt(p)
			v := uint64(t.Terrain) | uint64(t.Composition)<<8
			if w.discovered(p) {
				v |= 1 << 16
			}
			tiles = fnvAdd(tiles, v)
		}
	}
	ents := fnvSeed
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		for _, v := range []int{int(id), int(e.Kind), e.Pos.X, e.Pos.Y, e.HP, int(e.State)} {
			ents = fnvAdd(ents, uint64(v))
		}
	}
	return fmt.Sprintf("tiles=%016x entities=%016x n=%d floor=%d hidden=%d",
		tiles, ents, len(w.entities), w.terrainCounts[Floor], w.hiddenFloor)
}

func TestGoldenWorldHash(t *testing.T) {
	for _, gc := range goldenCases {
		t.Run(gc.name, func(t *testing.T) {
			w := NewEngine(gc.cfg()).world
			hidden := w.hiddenFloor
			if got := goldenHash(w); got != gc.gen {
				t.Errorf("tick 0:\n got  %s\n want %s", got, gc.gen)
			}
			for i := 0; i < gc.ticks; i++ {
				w.step()
			}
			if got := goldenHash(w); got != gc.run {
				t.Errorf("tick %d:\n got  %s\n want %s", gc.ticks, got, gc.run)
			}
			if gc.breach && w.hiddenFloor == hidden {
				t.Errorf("no cavern was breached in %d ticks; pick a config that breaks into one", gc.ticks)
			}
		})
	}
}

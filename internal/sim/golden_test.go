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
	// grows requires the run to generate chunks after tick 0, so the case
	// covers generation during play, not just at startup.
	grows bool
}

var goldenCases = []goldenCase{
	{
		// A big map with the smallest halo, so the colony's digging (and
		// breaches) generate chunks during the run.
		name: "lazy-1000x1010",
		cfg: func() Config {
			c := DefaultConfig()
			c.Seed = 3
			c.Width, c.Height = 1000, 1010
			c.StartColonists = 40
			c.CavernPercent = 15
			c.WorldgenHalo = 1
			return c
		},
		ticks:  600,
		breach: true,
		grows:  true,
		gen:    "tiles=911ee6d87732a215 entities=09ccf8326ab15b67 rng=aef18907a6e92720 chunks=622fc087dedf9013/15 n=53 gore=0 corpses=0",
		run:    "tiles=8a7eba9151d46728 entities=148fae0dfddcd1ed rng=1edd3483d7e0941e chunks=562663220ec91645/16 n=49 gore=0 corpses=2",
	},
	{
		name: "default-80x40",
		cfg: func() Config {
			c := DefaultConfig()
			c.Seed = 7
			return c
		},
		ticks: 400,
		gen:   "tiles=3bf4ed49f557125b entities=f08f76c37fee1a73 rng=54976373abdf4683 chunks=4d22107f9dcb30cc/2 n=19 gore=0 corpses=0",
		run:   "tiles=40e2480531c8f90a entities=223f797e8f9391e8 rng=a9d8b2619047466b chunks=4d22107f9dcb30cc/2 n=11 gore=0 corpses=0",
	},
	{
		// Bigger than one worldgen chunk in both directions, with enough cave
		// that digging breaks into some within the run.
		name: "caves-300x150",
		cfg: func() Config {
			c := DefaultConfig()
			c.Seed = 1
			c.Width, c.Height = 300, 150
			c.StartColonists = 40
			c.CavernPercent = 15
			return c
		},
		ticks:  600,
		breach: true,
		gen:    "tiles=6b0ca3236d95b6b0 entities=7e7318a4509dbd72 rng=25f88bfb6e34d1c8 chunks=0e4ca4c26f1b3a34/15 n=53 gore=0 corpses=0",
		run:    "tiles=449f87727c0c8805 entities=07060d07fbc89b07 rng=697d938a253059e4 chunks=0e4ca4c26f1b3a34/15 n=45 gore=0 corpses=6",
	},
}

// goldenHash condenses the simulation-visible world into one line: every
// tile's terrain, composition and discovery, every entity in ID order, and
// the simulation stream's state, and which chunks have been generated. It reads through TileAt and discovered
// rather than the backing store, so it describes what the simulation sees
// however tiles happen to be stored; neither ever generates a chunk. The RNG
// state is the catch-all: any draw added, lost or reordered anywhere moves it
// even when nothing visible has diverged yet.
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
		for _, v := range []int{int(id), int(e.Kind), e.Pos.X, e.Pos.Y, e.HP, int(e.State), e.Species} {
			ents = fnvAdd(ents, uint64(v))
		}
	}
	rng := fnvSeed
	if w.rngSrc.sim != nil {
		b, _ := w.rngSrc.sim.MarshalBinary()
		for _, c := range b {
			rng = fnvAdd(rng, uint64(c))
		}
	}
	chunks := fnvSeed
	for _, k := range w.genChunks {
		chunks = fnvAdd(fnvAdd(chunks, uint64(k.cx)), uint64(k.cy))
	}
	return fmt.Sprintf("tiles=%016x entities=%016x rng=%016x chunks=%016x/%d n=%d gore=%d corpses=%d",
		tiles, ents, rng, chunks, len(w.genChunks), len(w.entities), w.goreTotal, w.corpseTotal)
}

func TestGoldenWorldHash(t *testing.T) {
	for _, gc := range goldenCases {
		t.Run(gc.name, func(t *testing.T) {
			w := NewEngine(gc.cfg()).world
			chunks := len(w.genChunks)
			if got := goldenHash(w); got != gc.gen {
				t.Errorf("tick 0:\n got  %s\n want %s", got, gc.gen)
			}
			for i := 0; i < gc.ticks; i++ {
				w.step()
			}
			if got := goldenHash(w); got != gc.run {
				t.Errorf("tick %d:\n got  %s\n want %s", gc.ticks, got, gc.run)
			}
			if gc.grows && len(w.genChunks) == chunks {
				t.Errorf("no chunk was generated after tick 0 in %d ticks; pick a config that explores further", gc.ticks)
			}
			if gc.breach && w.cavernBreaches == 0 {
				t.Errorf("no cavern was breached in %d ticks; pick a config that breaks into one", gc.ticks)
			}
		})
	}
}

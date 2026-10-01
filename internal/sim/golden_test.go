package sim

import (
	"fmt"
	"slices"
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
		// A big map with the smallest halo, so the colony's digging, and a
		// breach flood running into ground nobody had generated, generate
		// chunks during the run (see TestGenerationStaysAheadOfExploration).
		name: "lazy-1000x1010",
		cfg: func() Config {
			c := DefaultConfig()
			c.Seed = 5
			c.Width, c.Height = 1000, 1010
			c.StartColonists = 40
			c.CavernPercent = 20
			c.WorldgenHalo = 1
			return c
		},
		ticks:  1200,
		breach: true,
		grows:  true,
		gen:    "tiles=6eba3d92a4e07cbb entities=f7ee7a1387568a13 rng=a78d6b0ac12b9e48 chunks=562663220ec91645/16 scum=694edd31ec9d434c/3893 salt=04dbe99fdb4ddc47/1954 n=53 gore=0 corpses=0",
		run:    "tiles=8fa844e558bd2852 entities=9887e95798b14546 rng=2e22e6e0cdf7b3db chunks=693d999c36049d09/20 scum=d381cde74c0dc896/4937 salt=b61c93a1ac1a38af/2441 n=45 gore=0 corpses=0",
	},
	{
		name: "default-80x40",
		cfg: func() Config {
			c := DefaultConfig()
			c.Seed = 7
			return c
		},
		ticks: 400,
		gen:   "tiles=fb413f93fa91bc92 entities=7aab2da63776690c rng=fee2103ce8a0059c chunks=4d22107f9dcb30cc/2 scum=af3afd423d126e36/182 salt=4a28046f7f756038/95 n=19 gore=0 corpses=0",
		run:   "tiles=9269709159b49b9d entities=1f850bc1eab90b54 rng=f20c8fdc04db1dc6 chunks=4d22107f9dcb30cc/2 scum=e6c102b5cef67211/193 salt=4a28046f7f756038/95 n=11 gore=0 corpses=0",
	},
	{
		// Bigger than one worldgen chunk in both directions, with enough cave
		// that digging breaks into some within the run.
		name: "caves-300x150",
		cfg: func() Config {
			c := DefaultConfig()
			c.Seed = 2
			c.Width, c.Height = 300, 150
			c.StartColonists = 40
			c.CavernPercent = 15
			return c
		},
		ticks:  1700, // long enough to breach: the first is at about tick 1635
		breach: true,
		gen:    "tiles=e81568a6449387ae entities=851ce35d5a35cf74 rng=c60777d3a7dcdb4d chunks=0e4ca4c26f1b3a34/15 scum=d17b29c1b71c8ea6/2663 salt=bb21b3e4454b76e2/1341 n=53 gore=0 corpses=0",
		run:    "tiles=ea593dd53692a48c entities=82e77fe4c4d27ad9 rng=898d7bc038b4864a chunks=0e4ca4c26f1b3a34/15 scum=107fea9c7481c3f9/2702 salt=a480c64662f96beb/1339 n=45 gore=0 corpses=0",
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
	// Scum patches, in a fixed order: they are generated content too, and a
	// map, so their order must not leak into the hash.
	patches := make([]Point, 0, len(w.scum))
	for p := range w.scum {
		patches = append(patches, p)
	}
	slices.SortFunc(patches, func(a, b Point) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		return a.X - b.X
	})
	scum := fnvSeed
	for _, p := range patches {
		scum = fnvAdd(fnvAdd(fnvAdd(scum, uint64(p.X)), uint64(p.Y)), uint64(w.scumAt(p)))
	}
	deposits := make([]Point, 0, len(w.salt))
	for p := range w.salt {
		deposits = append(deposits, p)
	}
	slices.SortFunc(deposits, func(a, b Point) int {
		if a.Y != b.Y {
			return a.Y - b.Y
		}
		return a.X - b.X
	})
	salt := fnvSeed
	for _, p := range deposits {
		salt = fnvAdd(fnvAdd(salt, uint64(p.X)), uint64(p.Y))
	}
	chunks := fnvSeed
	for _, k := range w.genChunks {
		chunks = fnvAdd(fnvAdd(chunks, uint64(k.cx)), uint64(k.cy))
	}
	return fmt.Sprintf("tiles=%016x entities=%016x rng=%016x chunks=%016x/%d scum=%016x/%d salt=%016x/%d n=%d gore=%d corpses=%d",
		tiles, ents, rng, chunks, len(w.genChunks), scum, len(patches), salt, len(deposits), len(w.entities), w.goreTotal, w.corpseTotal)
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

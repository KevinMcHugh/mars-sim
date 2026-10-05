package sim

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
)

// goldenCase pins what one seed produces: the generated world at tick 0 and
// the whole simulation after a number of ticks. The hashes are pinned in
// testdata/golden-hashes.txt, not a comparison between two runs, so they catch what a lockstep test cannot:
// the same seed producing a different world on a different machine
// (tools/determinism-check.sh runs this test natively, under amd64 and under
// js/wasm) or after a change nobody meant to be a seed break.
//
// When a change is *meant* to alter what seeds produce (a worldgen rewrite, a
// new RNG draw on the simulation stream), re-pin them in the same change with
//
//	go test ./internal/sim -run TestGoldenWorldHash -update
//
// and say so in the commit message. See docs/determinism.md.
type goldenCase struct {
	name  string
	cfg   func() Config
	ticks int
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
			// Seed and length re-picked when ships replaced crash pods:
			// a ship packs its settlers tighter than pods did, so the
			// landing cavern is smaller and the colony takes longer to dig
			// past the chunks generated at the start.
			c.Seed = 4
			c.ZoningAuto = true // the colony builds as it always has
			c.Width, c.Height = 1000, 1010
			c.StartColonists = 40
			c.CavernPercent = 20
			c.WorldgenHalo = 1
			return c
		},
		// 3000: with a night lasting until the sleep drive is met
		// (docs/days.md), this colony reaches new ground well after tick
		// 1200, and the scum-free copy TestLazyChunksMatchThePureGenerator
		// plays it later still.
		ticks:  3000,
		breach: true,
		grows:  true,
	},
	{
		name: "default-80x40",
		cfg: func() Config {
			c := DefaultConfig()
			c.ZoningAuto = true // the colony builds as it always has
			c.Seed = 7
			return c
		},
		ticks: 400,
	},
	{
		// Bigger than one worldgen chunk in both directions, with enough cave
		// that digging breaks into some within the run.
		name: "caves-300x150",
		cfg: func() Config {
			c := DefaultConfig()
			c.ZoningAuto = true // the colony builds as it always has
			c.Seed = 2
			c.Width, c.Height = 300, 150
			c.StartColonists = 40
			c.CavernPercent = 15
			return c
		},
		ticks:  1700, // long enough to breach: the first is at about tick 1635
		breach: true,
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

// goldenFile holds the pinned hashes, one "<case> gen|run <hash>" line each,
// so a deliberate seed break is re-pinned with -update rather than by hand.
const goldenFile = "testdata/golden-hashes.txt"

var update = flag.Bool("update", false, "rewrite the pinned golden hashes in "+goldenFile)

// readGoldenHashes returns the pinned hashes keyed by "<case> gen" and
// "<case> run". A missing file reads as empty, so -update can create it.
func readGoldenHashes(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(goldenFile)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}
	}
	if err != nil {
		t.Fatal(err)
	}
	pinned := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.SplitN(line, " ", 3)
		if len(f) != 3 {
			t.Fatalf("%s: malformed line %q", goldenFile, line)
		}
		pinned[f[0]+" "+f[1]] = f[2]
	}
	return pinned
}

// writeGoldenHashes writes the hashes in goldenCases order. Entries for cases
// a -run filter skipped keep their old value; cases no longer in goldenCases
// are dropped.
func writeGoldenHashes(t *testing.T, pinned map[string]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Pinned goldenHash values for goldenCases in golden_test.go: \"<case> gen|run <hash>\".\n")
	b.WriteString("# Regenerate with: go test ./internal/sim -run TestGoldenWorldHash -update\n")
	b.WriteString("# See docs/determinism.md.\n")
	for _, gc := range goldenCases {
		for _, phase := range []string{"gen", "run"} {
			if h, ok := pinned[gc.name+" "+phase]; ok {
				fmt.Fprintf(&b, "%s %s %s\n", gc.name, phase, h)
			}
		}
	}
	if err := os.WriteFile(goldenFile, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGoldenWorldHash(t *testing.T) {
	pinned := readGoldenHashes(t)
	check := func(t *testing.T, key, when, got string) {
		t.Helper()
		if *update {
			pinned[key] = got
			return
		}
		want, ok := pinned[key]
		if !ok {
			t.Errorf("%s: no hash pinned in %s; run with -update to pin it", when, goldenFile)
			return
		}
		if got != want {
			t.Errorf("%s:\n got  %s\n want %s\nIf this change is meant to alter what seeds produce, re-pin with "+
				"go test ./internal/sim -run TestGoldenWorldHash -update and say so in the commit message", when, got, want)
		}
	}
	for _, gc := range goldenCases {
		t.Run(gc.name, func(t *testing.T) {
			w := NewEngine(gc.cfg()).world
			chunks := len(w.genChunks)
			check(t, gc.name+" gen", "tick 0", goldenHash(w))
			for i := 0; i < gc.ticks; i++ {
				w.step()
			}
			check(t, gc.name+" run", fmt.Sprintf("tick %d", gc.ticks), goldenHash(w))
			if gc.grows && len(w.genChunks) == chunks {
				t.Errorf("no chunk was generated after tick 0 in %d ticks; pick a config that explores further", gc.ticks)
			}
			if gc.breach && w.cavernBreaches == 0 {
				t.Errorf("no cavern was breached in %d ticks; pick a config that breaks into one", gc.ticks)
			}
		})
	}
	if *update {
		writeGoldenHashes(t, pinned)
	}
}

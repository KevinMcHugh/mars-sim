package sim

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
)

// Chunked generation makes ore and cavern abundance an expected value rather
// than an exact one: each chunk rolls its own feature count, same-composition
// veins can overlap, and a cavern crowding a higher-ranked one is dropped.
// These tests measure how far the result drifts from the configured
// percentages over many seeds. See docs/worldgen-chunks.md for the numbers.

// abundance is one generated world's actual percentages, by composition level
// (see veinLevels), then cavern floor, then cave scum, then salt.
type abundance [len(veinLevels) + 3]float64

var abundanceNames = [len(veinLevels) + 3]string{"iron", "ice", "uranium", "clay", "cavern", "scum", "salt"}

func abundanceTargets(cfg Config) abundance {
	var a abundance
	for l := range veinLevels {
		a[l] = float64(veinPercent(&cfg, l))
	}
	a[len(veinLevels)] = float64(cfg.CavernPercent)
	a[len(veinLevels)+1] = float64(cfg.ScumPercent)
	a[len(veinLevels)+2] = float64(cfg.SaltPercent)
	return a
}

// measureAbundance generates every chunk of a cfg-sized map (no World, no
// landing-site carving) and returns its percentages. The landing exclusion box
// still applies, as it does in a real world.
func measureAbundance(cfg Config) abundance {
	g := newWorldGen(cfg)
	g.withCacheSize(2 * g.chunkCols() * (2*genHorizon + 1))
	var counts [len(veinLevels) + 3]int
	for cy := 0; cy < g.chunkRows(); cy++ {
		for cx := 0; cx < g.chunkCols(); cx++ {
			c := g.chunk(cx, cy)
			lo, hi, _ := g.chunkBounds(chunkKey{int32(cx), int32(cy)})
			for y := lo.Y; y <= hi.Y; y++ {
				for x := lo.X; x <= hi.X; x++ {
					off := (y&(genChunkSize-1))<<genChunkBits | x&(genChunkSize-1)
					if comp := c.comp[off]; comp != OrdinaryRock {
						counts[slices.Index(veinLevels[:], comp)]++
					}
					if c.isFloor(off) {
						counts[len(veinLevels)]++
					}
					if c.isScum(off) {
						counts[len(veinLevels)+1]++
					}
					if c.isSalt(off) {
						counts[len(veinLevels)+2]++
					}
				}
			}
		}
	}
	var a abundance
	area := float64(cfg.Width * cfg.Height)
	for i, n := range counts {
		a[i] = 100 * float64(n) / area
	}
	return a
}

type driftStats struct {
	mean, sd, lo, hi float64
}

func summarize(xs []float64) driftStats {
	s := driftStats{lo: math.Inf(1), hi: math.Inf(-1)}
	for _, x := range xs {
		s.mean += x
		s.lo, s.hi = min(s.lo, x), max(s.hi, x)
	}
	s.mean /= float64(len(xs))
	for _, x := range xs {
		s.sd += (x - s.mean) * (x - s.mean)
	}
	s.sd = math.Sqrt(s.sd / float64(len(xs)))
	return s
}

func driftSweep(size [2]int, seeds int) [len(veinLevels) + 3]driftStats {
	cfg := DefaultConfig()
	cfg.Width, cfg.Height = size[0], size[1]
	per := make([][]float64, len(abundanceNames))
	for seed := 1; seed <= seeds; seed++ {
		cfg.Seed = int64(seed)
		a := measureAbundance(cfg)
		for i := range a {
			per[i] = append(per[i], a[i])
		}
	}
	var out [len(veinLevels) + 3]driftStats
	for i := range out {
		out[i] = summarize(per[i])
	}
	return out
}

// TestAbundanceDriftReport prints the drift table in docs/worldgen-chunks.md.
// It is slow, so it only runs with MARS_DRIFT_REPORT=1:
//
//	MARS_DRIFT_REPORT=1 go test ./internal/sim -run TestAbundanceDriftReport -v
func TestAbundanceDriftReport(t *testing.T) {
	if os.Getenv("MARS_DRIFT_REPORT") == "" {
		t.Skip("set MARS_DRIFT_REPORT=1 to print the abundance drift table")
	}
	targets := abundanceTargets(DefaultConfig())
	var b strings.Builder
	fmt.Fprintf(&b, "\n| map | seeds | feature | target %% | mean %% | mean error | sd | min | max |\n")
	fmt.Fprintf(&b, "| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, run := range []struct {
		size  [2]int
		seeds int
	}{
		{[2]int{80, 40}, 500},
		{[2]int{256, 256}, 500},
		{[2]int{1024, 1024}, 100},
		{[2]int{4096, 4096}, 10},
	} {
		stats := driftSweep(run.size, run.seeds)
		for i, s := range stats {
			fmt.Fprintf(&b, "| %dx%d | %d | %s | %.0f | %.2f | %+.1f%% | %.2f | %.2f | %.2f |\n",
				run.size[0], run.size[1], run.seeds, abundanceNames[i], targets[i],
				s.mean, 100*(s.mean-targets[i])/targets[i], s.sd, s.lo, s.hi)
		}
	}
	t.Log(b.String())
}

// TestAbundanceDriftWithinTolerance is the gate: on a map big enough for the
// law of large numbers, the mean over seeds lands near every target, and on
// the default map every configured feature still appears on every seed.
func TestAbundanceDriftWithinTolerance(t *testing.T) {
	targets := abundanceTargets(DefaultConfig())
	stats := driftSweep([2]int{256, 256}, 40)
	for i, s := range stats {
		if rel := math.Abs(s.mean-targets[i]) / targets[i]; rel > abundanceTolerance {
			t.Errorf("%s: mean %.2f%% over 40 seeds on 256x256, target %.0f%% (off by %.1f%%, tolerance %.0f%%)",
				abundanceNames[i], s.mean, targets[i], 100*rel, 100*abundanceTolerance)
		}
	}
	small := driftSweep([2]int{80, 40}, 200)
	for i, s := range small {
		if s.lo == 0 {
			t.Errorf("%s: some seed generated none on the default 80x40 map", abundanceNames[i])
		}
	}
}

// abundanceTolerance is how far (relative) the mean may sit from its target.
const abundanceTolerance = 0.05

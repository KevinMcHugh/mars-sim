package sim

import (
	"math/rand"
	"testing"
)

// benchWorld builds a large world with a carved central chamber and many
// colonists, for measuring per-tick cost as population scales. There is a rock
// frontier around the chamber so colonists have real mining and building work.
func benchWorld(colonists int) *World {
	cfg := DefaultConfig()
	cfg.Seed = 1
	cfg.Width, cfg.Height = 160, 160
	w := newWorld(cfg, rand.New(rand.NewSource(1)))

	const border = 20
	for y := border; y < w.Height-border; y++ {
		for x := border; x < w.Width-border; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	center := Point{w.Width / 2, w.Height / 2}
	for i := 0; i < colonists; i++ {
		if p, ok := w.randomFloorNear(center, w.Width); ok {
			w.spawn(Colonist, p)
		}
	}
	return w
}

func benchmarkStep(b *testing.B, colonists int) {
	w := benchWorld(colonists)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.step()
	}
}

func BenchmarkStep500(b *testing.B)  { benchmarkStep(b, 500) }
func BenchmarkStep2000(b *testing.B) { benchmarkStep(b, 2000) }

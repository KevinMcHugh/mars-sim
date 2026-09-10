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
	floors := w.freeFloorTiles()
	w.rng.Shuffle(len(floors), func(i, j int) { floors[i], floors[j] = floors[j], floors[i] })
	if colonists > len(floors) {
		colonists = len(floors)
	}
	for i := 0; i < colonists; i++ {
		w.spawn(Colonist, floors[i])
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

// BenchmarkRoomRefresh measures the incremental cost of one terrain change in a
// large open map: one chunk re-flooded plus a room relabel over the region
// graph. It should stay flat as the map grows, unlike a global flood fill.
func BenchmarkRoomRefresh(b *testing.B) {
	w := benchWorld(0) // 160x160 with a large carved chamber, no colonists
	w.refreshSpatial()
	p := Point{w.Width / 2, w.Height / 2}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			w.SetTerrain(p, Wall)
		} else {
			w.SetTerrain(p, Floor)
		}
		w.refreshSpatial()
	}
}

// BenchmarkPathfind measures one A* search across a large open room (a common
// case: a colonist near the middle routing to a frontier tile at the edge).
func BenchmarkPathfind(b *testing.B) {
	w := benchWorld(0) // 160x160 with a large carved chamber
	w.refreshSpatial()
	from := Point{w.Width / 2, w.Height / 2}
	target := Point{20, 20} // rock at the chamber's rock border
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.pathToAdjacent(from, target)
	}
}

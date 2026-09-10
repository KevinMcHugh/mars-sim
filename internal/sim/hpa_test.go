package sim

import (
	"math/rand"
	"testing"
)

// routeError checks a route is a contiguous run of walkable steps from `from`
// that ends adjacent to target; returns "" if valid.
func routeError(w *World, from, target Point, route []Point) string {
	prev := from
	for _, step := range route {
		if !prev.Adjacent(step) {
			return "route not contiguous"
		}
		if !w.Walkable(step) {
			return "route steps onto non-walkable tile"
		}
		prev = step
	}
	if !prev.Adjacent(target) {
		return "route does not end adjacent to target"
	}
	return ""
}

// detourWorld: one big room split by a wall with a gap at the bottom, so getting
// from the left side to the right requires a long detour — the case where a flat
// search over-explores and HPA* pays off.
func detourWorld(size int) *World {
	w := roomsTestWorld(size, size)
	carve(w, Point{1, 1}, Point{size - 2, size - 2}, Floor)
	wallX := size / 2
	for y := 0; y < size-6; y++ {
		w.SetTerrain(Point{wallX, y}, Wall) // gap left near the bottom
	}
	w.refreshSpatial()
	return w
}

// HPA* returns valid, near-optimal routes across a detour, matching a flat
// search's reachability and staying close to the BFS shortest length.
func TestHPAValidAndNearOptimal(t *testing.T) {
	w := detourWorld(96)
	rng := rand.New(rand.NewSource(5))
	floors := w.freeFloorTiles()

	checked := 0
	for i := 0; i < 300 && checked < 60; i++ {
		from := floors[rng.Intn(len(floors))]
		target := floors[rng.Intn(len(floors))]
		if from.Equal(target) || from.Adjacent(target) {
			continue
		}
		route, ok := w.pathToAdjacent(from, target)
		best := bfsStepsToAdjacent(w, from, target)
		if best < 0 {
			if ok {
				t.Fatalf("HPA* routed %v->%v that BFS says is unreachable", from, target)
			}
			continue
		}
		if !ok {
			t.Fatalf("HPA* found no route %v->%v (BFS %d)", from, target, best)
		}
		if msg := routeError(w, from, target, route); msg != "" {
			t.Fatalf("%s: %v->%v route %v", msg, from, target, route)
		}
		// Corridor-constrained search is near-optimal, not always optimal.
		if len(route) > best*3/2+2 {
			t.Fatalf("HPA* route %v->%v length %d far exceeds BFS %d", from, target, len(route), best)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no cross-region pairs were checked")
	}
}

// Routes are deterministic: the same query yields the same route.
func TestHPADeterministic(t *testing.T) {
	w := detourWorld(96)
	from, target := Point{5, 20}, Point{90, 20}
	a, ok1 := w.pathToAdjacent(from, target)
	b, ok2 := w.pathToAdjacent(from, target)
	if !ok1 || !ok2 || len(a) != len(b) {
		t.Fatalf("nondeterministic length: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("routes differ at %d: %v vs %v", i, a[i], b[i])
		}
	}
}

// The abstract corridor connects the two regions and contains both endpoints'
// regions.
func TestAbstractCorridorConnects(t *testing.T) {
	w := detourWorld(96)
	sr := w.regionOf[w.index(Point{5, 20})]
	gr := w.regionOf[w.index(Point{90, 20})]
	if sr == 0 || gr == 0 || sr == gr {
		t.Fatalf("expected two distinct regions, got %d and %d", sr, gr)
	}
	corridor, ok := w.abstractCorridor(sr, gr)
	if !ok {
		t.Fatal("no abstract corridor between connected regions")
	}
	if !corridor[sr] || !corridor[gr] {
		t.Fatal("corridor should contain both endpoint regions")
	}
}

// BenchmarkPathfindDetourFlat / HPA compare a flat tile search against the
// abstract-routed one on the same long detour query.
func benchDetourQuery(b *testing.B, useHPA bool) {
	w := detourWorld(160)
	from, target := Point{5, 40}, Point{150, 40}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if useHPA {
			w.pathToAdjacent(from, target)
		} else {
			w.pf.toAdjacent(from, target, false)
		}
	}
}

func BenchmarkPathfindDetourFlat(b *testing.B) { benchDetourQuery(b, false) }
func BenchmarkPathfindDetourHPA(b *testing.B)  { benchDetourQuery(b, true) }

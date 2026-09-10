package sim

import (
	"math/rand"
	"testing"
)

// bfsStepsToAdjacent returns the shortest number of 8-connected steps from start
// to any walkable tile adjacent to target (the ground truth for A* optimality),
// or -1 if unreachable.
func bfsStepsToAdjacent(w *World, start, target Point) int {
	if start.Adjacent(target) {
		return 0
	}
	dist := make(map[Point]int)
	dist[start] = 0
	queue := []Point{start}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range neighbors8 {
			n := p.Add(d.X, d.Y)
			if !w.Walkable(n) {
				continue
			}
			if _, ok := dist[n]; ok {
				continue
			}
			dist[n] = dist[p] + 1
			if n.Adjacent(target) {
				return dist[n]
			}
			queue = append(queue, n)
		}
	}
	return -1
}

// A* must return routes whose length equals the BFS shortest distance, over many
// random start/target pairs in a cluttered room.
func TestPathOptimalMatchesBFS(t *testing.T) {
	w := roomsTestWorld(60, 40)
	rng := rand.New(rand.NewSource(21))
	// Carve a large room, then pepper it with wall obstacles.
	carve(w, Point{2, 2}, Point{57, 37}, Floor)
	for i := 0; i < 300; i++ {
		w.SetTerrain(Point{2 + rng.Intn(56), 2 + rng.Intn(36)}, Wall)
	}
	w.refreshSpatial()

	floors := w.freeFloorTiles()
	checked := 0
	for i := 0; i < 400 && checked < 200; i++ {
		start := floors[rng.Intn(len(floors))]
		target := floors[rng.Intn(len(floors))]
		if start.Equal(target) {
			continue
		}
		want := bfsStepsToAdjacent(w, start, target)
		// Flat A* is optimal; pathToAdjacent may use near-optimal HPA* for long
		// trips (covered by TestHPAValidAndNearOptimal), so test the flat search.
		route, ok := w.pf.toAdjacent(start, target, false)
		if want < 0 {
			if ok {
				t.Fatalf("A* found a route %v->%v that BFS says is unreachable", start, target)
			}
			continue
		}
		if !ok {
			t.Fatalf("A* found no route %v->%v but BFS distance is %d", start, target, want)
		}
		if len(route) != want {
			t.Fatalf("A* route %v->%v length %d, BFS shortest %d", start, target, len(route), want)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no reachable pairs were checked")
	}
}

// A colonist must navigate a serpentine corridor to reach and mine a target that
// greedy step-toward could never reach (it would wedge against a wall).
func TestPathfindingNavigatesMaze(t *testing.T) {
	w := roomsTestWorld(24, 12)
	// A snake: floor corridors separated by walls with alternating gaps.
	//   col 2 open top..bottom; wall col 3 with gap at bottom; col 4 open; wall
	//   col 5 with gap at top; col 6 open; etc. Forces an up-down-up route.
	carve(w, Point{1, 1}, Point{1, 10}, Floor) // start column
	for x := 2; x <= 18; x++ {
		carve(w, Point{x, 1}, Point{x, 10}, Floor)
	}
	// Walls with a single gap, alternating ends.
	for i, x := 0, 3; x <= 17; x, i = x+2, i+1 {
		carve(w, Point{x, 1}, Point{x, 10}, Wall)
		if i%2 == 0 {
			w.SetTerrain(Point{x, 10}, Floor) // gap at bottom
		} else {
			w.SetTerrain(Point{x, 1}, Floor) // gap at top
		}
	}
	w.refreshSpatial()

	start := Point{1, 1}
	col := w.spawn(Colonist, start)
	target := Point{19, 5} // rock just past the far corridor
	if w.TerrainAt(target) != Rock {
		t.Fatalf("expected rock at target, got %v", w.TerrainAt(target))
	}

	// Drive travelTo directly until it arrives or gives up.
	arrivedTick := -1
	for tick := 0; tick < 500; tick++ {
		arrived, ok := w.travelTo(col, target)
		if !ok {
			t.Fatalf("colonist gave up navigating the maze at %v", col.Pos)
		}
		if arrived {
			arrivedTick = tick
			break
		}
	}
	if arrivedTick < 0 {
		t.Fatalf("colonist never reached the target (stuck at %v)", col.Pos)
	}
	if !col.Pos.Adjacent(target) {
		t.Fatalf("arrived but not adjacent: at %v, target %v", col.Pos, target)
	}
}

// A target walled off in a different room is reported unreachable without running
// a full search.
func TestPathfindingRejectsUnreachable(t *testing.T) {
	w := roomsTestWorld(40, 24)
	carve(w, Point{3, 5}, Point{5, 7}, Floor)   // room A
	carve(w, Point{20, 5}, Point{22, 7}, Floor) // room B
	w.refreshSpatial()

	// A rock tile touching room B only.
	target := Point{23, 6}
	if w.TerrainAt(target) != Rock {
		t.Fatalf("expected rock at %v", target)
	}
	if _, ok := w.pathToAdjacent(Point{4, 6}, target); ok {
		t.Fatal("A* should not find a route from room A to room B's rock")
	}
}

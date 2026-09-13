package sim

import (
	"math/rand"
	"testing"
)

// bfsFacilityDist returns the true step distance from every walkable tile to the
// nearest tile adjacent to a facility of kind t (multi-source BFS ground truth).
func bfsFacilityDist(w *World, t Terrain) map[Point]int {
	dist := make(map[Point]int)
	var q []Point
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if w.TerrainAt(Point{x, y}) != t {
				continue
			}
			for _, d := range neighbors8 {
				n := Point{x + d.X, y + d.Y}
				if w.Walkable(n) {
					if _, ok := dist[n]; !ok {
						dist[n] = 0
						q = append(q, n)
					}
				}
			}
		}
	}
	for i := 0; i < len(q); i++ {
		p := q[i]
		for _, d := range neighbors8 {
			n := p.Add(d.X, d.Y)
			if !w.Walkable(n) {
				continue
			}
			if _, ok := dist[n]; !ok {
				dist[n] = dist[p] + 1
				q = append(q, n)
			}
		}
	}
	return dist
}

// The flow field's distances must equal a multi-source BFS over the walkable
// grid, across a cluttered room with several facilities.
func TestFlowFieldMatchesBFS(t *testing.T) {
	w := roomsTestWorld(60, 40)
	rng := rand.New(rand.NewSource(31))
	carve(w, Point{2, 2}, Point{57, 37}, Floor)
	for i := 0; i < 200; i++ {
		w.SetTerrain(Point{2 + rng.Intn(56), 2 + rng.Intn(36)}, Wall)
	}
	// Scatter a few pods on floor tiles.
	for i := 0; i < 6; i++ {
		p := Point{2 + rng.Intn(56), 2 + rng.Intn(36)}
		if w.TerrainAt(p) == Floor {
			w.SetTerrain(p, NutrientPod)
		}
	}
	w.refreshSpatial()

	f := w.facilityField(NutrientPod)
	if f == nil {
		t.Fatal("expected a flow field for NutrientPod")
	}
	want := bfsFacilityDist(w, NutrientPod)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Point{x, y}
			if !w.Walkable(p) {
				continue
			}
			got := int(f.at(p))
			wd, reachable := want[p]
			if reachable && got != wd {
				t.Fatalf("dist at %v: field %d, BFS %d", p, got, wd)
			}
			if !reachable && got != -1 {
				t.Fatalf("dist at %v: field %d, BFS unreachable", p, got)
			}
		}
	}
}

// Following the field descends to distance 0 (adjacent to a facility) from any
// reachable tile.
func TestFlowFieldFollowReachesFacility(t *testing.T) {
	w := roomsTestWorld(40, 20)
	carve(w, Point{2, 2}, Point{37, 17}, Floor)
	w.SetTerrain(Point{34, 15}, NutrientPod)
	w.refreshSpatial()

	f := w.facilityField(NutrientPod)
	start := Point{4, 4}
	e := w.spawn(Colonist, start)
	steps := 0
	for f.at(e.Pos) > 0 && steps < 500 {
		if !w.followField(e, f) {
			t.Fatalf("stopped descending at %v (dist %d)", e.Pos, f.at(e.Pos))
		}
		steps++
	}
	if f.at(e.Pos) != 0 {
		t.Fatalf("did not reach a facility-adjacent tile (dist %d at %v)", f.at(e.Pos), e.Pos)
	}
	if _, ok := w.adjacentFacility(e.Pos, NutrientPod); !ok {
		t.Fatalf("arrived at dist 0 but no adjacent pod at %v", e.Pos)
	}
}

func TestFlowFieldPassesThroughCrowdToFreeLanding(t *testing.T) {
	w := roomsTestWorld(10, 5)
	carve(w, Point{1, 2}, Point{7, 2}, Floor)
	w.SetTerrain(Point{8, 2}, NutrientPod)
	w.refreshSpatial()

	mover := w.spawn(Colonist, Point{1, 2})
	for x := 2; x <= 6; x++ {
		w.spawn(Colonist, Point{x, 2})
	}

	if !w.followField(mover, w.facilityField(NutrientPod)) {
		t.Fatal("flow-field movement could not pass through the crowded corridor")
	}
	if want := (Point{7, 2}); !mover.Pos.Equal(want) {
		t.Fatalf("mover stopped at %v, want free facility access tile %v", mover.Pos, want)
	}
	for x := 2; x <= 6; x++ {
		if got := w.entityAt(Point{x, 2}); got == nil || got == mover {
			t.Fatalf("transit changed occupant at {%d 2}: %v", x, got)
		}
	}
}

// When every downhill tile is occupied and the only free space is behind it, a
// colonist must step uphill to break the gridlock. Refusing any uphill landing
// leaves a full facility room permanently jammed even though it has room for
// colonists to move out of one another's way.
func TestFlowFieldStepsAsideWhenCrowdBlocksEveryDownhillTile(t *testing.T) {
	w := roomsTestWorld(7, 5)
	carve(w, Point{1, 2}, Point{4, 2}, Floor)
	w.SetTerrain(Point{5, 2}, NutrientPod)
	w.refreshSpatial()

	mover := w.spawn(Colonist, Point{2, 2})
	w.spawn(Colonist, Point{3, 2})
	w.spawn(Colonist, Point{4, 2})
	field := w.facilityField(NutrientPod)
	before := field.at(mover.Pos)

	if !w.followField(mover, field) {
		t.Fatal("flow-field movement would not step aside from a fully occupied route")
	}
	if after := field.at(mover.Pos); after <= before {
		t.Fatalf("mover did not take the necessary uphill escape step: distance %d -> %d", before, after)
	}
}

// A hungry colonist walks to and uses an existing pod, resetting its need — end
// to end through the step loop.
func TestColonistSeeksFacilityViaField(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	// A short corridor with a pod at the end.
	carve(w, center, center.Add(6, 0), Floor)
	w.SetTerrain(center.Add(7, 0), NutrientPod)
	w.refreshSpatial()

	col := w.spawn(Colonist, center)
	col.Needs[NeedFood] = cfg.Needs[NeedFood].SeekAt // urgent now

	ate := false
	for i := 0; i < 200; i++ {
		w.step()
		if w.needLevel(col, NeedFood) == 0 {
			ate = true
			break
		}
	}
	if !ate {
		t.Fatalf("colonist never reached/used the pod (food %d at %v)", w.needLevel(col, NeedFood), col.Pos)
	}
}

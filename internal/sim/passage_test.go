package sim

import "testing"

// passageWorld is rock with an open main area, x 10..30, y 5..18.
func passageWorld(t *testing.T) *World {
	t.Helper()
	w := rockSiteWorld(t)
	carve(w, Point{10, 5}, Point{30, 18}, Floor)
	w.refreshSpatial()
	return w
}

// A colonist cut off by a single wall tile, with rock beside it, digs round
// the wall: one rock tile costs less work than breaking the wall down.
func TestEscapeDigsRoundAWallWhenRockIsCheaper(t *testing.T) {
	w := passageWorld(t)
	pocket := Point{8, 10}
	w.SetTerrain(pocket, Floor)
	w.SetTerrain(Point{9, 10}, Wall)
	w.refreshSpatial()
	if w.roomOf(pocket) == w.mainRoom {
		t.Fatal("test setup: the pocket is not cut off")
	}
	target, ok := w.escapeTarget(pocket)
	if !ok {
		t.Fatal("no way out of the pocket")
	}
	if w.TerrainAt(target) != Rock || !target.Adjacent(pocket) {
		t.Fatalf("escape target %v is %v, want rock beside the pocket", target, w.TerrainAt(target))
	}
}

// A colonist walled in on every side, with the colony just past one wall and
// thick rock everywhere else, breaks through the wall.
func TestEscapeBreaksThroughAWallWhenRockIsDearer(t *testing.T) {
	w := passageWorld(t)
	// A sealed 3x3 room above the main area: walls x 17..21, y 1..4 with
	// its bottom wall on y 4, right against the main area's top row.
	carve(w, Point{17, 0}, Point{21, 4}, Wall)
	carve(w, Point{18, 1}, Point{20, 3}, Floor)
	w.refreshSpatial()
	inside := Point{19, 2}
	if w.roomOf(inside) == w.mainRoom {
		t.Fatal("test setup: the room is not sealed")
	}
	target, ok := w.escapeTarget(inside)
	if !ok {
		t.Fatal("no way out of the sealed room")
	}
	if w.TerrainAt(target) != Wall || target.Y != 4 {
		t.Fatalf("escape target %v is %v, want the bottom wall on y=4", target, w.TerrainAt(target))
	}
}

// A facility walled off with nobody beside it gets a passage from the colony's
// side, and the colony's builders reopen it.
func TestColonyDigsAPassageToACutOffFacility(t *testing.T) {
	w := passageWorld(t)
	pod := Point{25, 12}
	w.SetTerrain(pod, NutrientPod)
	for _, d := range neighbors8 {
		w.SetTerrain(pod.Add(d.X, d.Y), Wall)
	}
	w.refreshSpatial()
	if !w.fixtureCutOff(pod) {
		t.Fatal("test setup: the pod is not cut off")
	}

	w.planRooms()
	if !named(w.projects, PassageName) {
		t.Fatalf("no passage planned to a walled-off pod; projects = %v", projectNames(w.projects))
	}
	p := w.projects[0]
	if len(p.tasks) != 1 || p.tasks[0].clears != Wall {
		t.Fatalf("passage tasks = %d (first clears %v), want one wall to break", len(p.tasks), p.tasks[0].clears)
	}
	w.planRooms()
	if len(w.projects) != 1 {
		t.Fatalf("projects = %v, want the one passage while it is under way", projectNames(w.projects))
	}

	w.spawn(Colonist, Point{12, 8})
	w.spawn(Colonist, Point{12, 9})
	for i := 0; i < 600 && w.fixtureCutOff(pod); i++ {
		w.step()
	}
	if w.fixtureCutOff(pod) {
		t.Fatal("the colony never broke through to the pod")
	}
	w.step() // pruned at the start of the next tick's planning
	if named(w.projects, PassageName) {
		t.Fatal("the passage is still planned with the pod reached")
	}
}

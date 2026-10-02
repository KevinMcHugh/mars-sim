package sim

import (
	"fmt"
	"testing"
)

// stairWorld is an all-rock world three levels deep with no generation: a
// room on the landing level, a stair down from it, and a room at the bottom.
//
//	level 1: floor x 5..20, y 5..12, stair down at (10, 8)
//	level 2: floor x 10..30, y 6..10, stair up at (10, 8)
func stairWorld(t *testing.T) (w *World, top, bottom Point) {
	t.Helper()
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 30
	cfg.DeepestLevel = 3
	w = newWorld(cfg, newPCG(1))
	carveOn(w, LandingLevel, Point{5, 5, LandingLevel}, Point{20, 12, LandingLevel}, Floor)
	top = Point{10, 8, LandingLevel}
	if !w.digStair(top) {
		t.Fatal("digStair refused open floor on the landing level")
	}
	bottom = Point{10, 8, LandingLevel + 1}
	carveOn(w, LandingLevel+1, Point{11, 6, 0}, Point{30, 10, 0}, Floor)
	w.refreshSpatial()
	return w, top, bottom
}

// carveOn sets every tile of the box from..to on level l to t.
func carveOn(w *World, l Level, from, to Point, t Terrain) {
	for y := from.Y; y <= to.Y; y++ {
		for x := from.X; x <= to.X; x++ {
			w.SetTerrain(Point{x, y, l}, t)
		}
	}
}

func TestDigStairMakesBothEnds(t *testing.T) {
	w, top, bottom := stairWorld(t)
	if got := w.TerrainAt(top); got != StairDown {
		t.Errorf("top of the stair is %v, want stair down", got)
	}
	if got := w.TerrainAt(bottom); got != StairUp {
		t.Errorf("bottom of the stair is %v, want stair up", got)
	}
	if q, ok := w.linkFrom(top); !ok || q != bottom {
		t.Errorf("linkFrom(top) = %v, %v; want %v", q, ok, bottom)
	}
	if q, ok := w.linkFrom(bottom); !ok || q != top {
		t.Errorf("linkFrom(bottom) = %v, %v; want %v", q, ok, top)
	}
	if !w.discovered(bottom) || !w.discovered(bottom.Add(1, 0)) {
		t.Error("breaking into the level below did not reveal around the bottom of the stair")
	}
	if w.layer(LandingLevel+2) != nil {
		t.Error("a level nobody dug to exists")
	}
}

func TestDigStairRespectsDeepestLevel(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 40, 20
	w := newWorld(cfg, newPCG(1)) // DeepestLevel 1: no digging down
	carveOn(w, LandingLevel, Point{5, 5, 0}, Point{10, 10, 0}, Floor)
	if w.digStair(Point{7, 7, LandingLevel}) {
		t.Fatal("dug a stair below the deepest level the config allows")
	}
	if w.layer(LandingLevel+1) != nil || w.TerrainAt(Point{7, 7, LandingLevel}) != Floor {
		t.Fatal("a refused stair changed the world")
	}
}

// Rooms are the reachability gate every job runs through, so a stair has to
// make the two levels one room, and losing either end has to split it again.
func TestStairJoinsRoomsAcrossLevels(t *testing.T) {
	w, top, bottom := stairWorld(t)
	far := Point{30, 10, LandingLevel + 1}
	if !w.sameRoom(Point{5, 5, LandingLevel}, far) {
		t.Fatal("the rooms at either end of the stair are not one room")
	}
	if w.roomOf(top) == 0 || w.roomOf(bottom) == 0 {
		t.Fatal("a stair tile is in no room")
	}
	w.SetTerrain(top, Wall) // the top is walled over
	w.refreshSpatial()
	if w.sameRoom(Point{5, 5, LandingLevel}, far) {
		t.Fatal("walling over the top of the stair left the levels joined")
	}
	if _, ok := w.linkFrom(bottom); ok {
		t.Error("the bottom of a stair with no top still links")
	}
	checkRoomLabelsAllLevels(t, w)
}

// A route between levels goes through the stair, one legal step at a time.
func TestPathCrossesTheStair(t *testing.T) {
	w, top, bottom := stairWorld(t)
	from := Point{28, 9, LandingLevel + 1}
	target := Point{6, 6, LandingLevel}
	route, ok := w.pathToAdjacent(from, target)
	if !ok {
		t.Fatal("no route from the level below to the landing level")
	}
	checkRoute(t, w, from, route)
	if !route[len(route)-1].Adjacent(target) {
		t.Errorf("route ends at %v, not beside %v", route[len(route)-1], target)
	}
	crossed := false
	for i := 1; i < len(route); i++ {
		if route[i-1] == bottom && route[i] == top {
			crossed = true
		}
	}
	if !crossed {
		t.Errorf("route %v never climbs the stair", route)
	}
	// And the other way, down.
	back, ok := w.pathToAdjacent(Point{6, 6, LandingLevel}, Point{29, 9, LandingLevel + 1})
	if !ok {
		t.Fatal("no route from the landing level down")
	}
	checkRoute(t, w, Point{6, 6, LandingLevel}, back)
}

// checkRoute fails unless every step of route is one move: to a walkable
// neighbour on the same level, or through a stair.
func checkRoute(t *testing.T, w *World, from Point, route []Point) {
	t.Helper()
	prev := from
	for _, p := range route {
		if !w.Walkable(p) {
			t.Fatalf("route steps onto %v, which is %v", p, w.TerrainAt(p))
		}
		if q, ok := w.linkFrom(prev); !(prev.Adjacent(p) || ok && q == p) {
			t.Fatalf("route jumps from %v to %v", prev, p)
		}
		prev = p
	}
}

// A shared flow field routes a colonist on another level to a facility on
// this one, and a repair after the stair is cut matches a rebuild.
func TestFlowFieldCrossesTheStair(t *testing.T) {
	w, top, bottom := stairWorld(t)
	w.SetTerrain(Point{5, 5, LandingLevel}, Toilet)
	w.refreshSpatial()
	f := w.facilityField(Toilet)
	far := Point{30, 10, LandingLevel + 1}
	got := f.at(far)
	// From far: 20 steps west along the bottom room to the stair's foot is
	// covered by Chebyshev; then the stair, then 4 more on the landing level
	// to the toilet's neighbour (6, 6). The field counts steps to a goal
	// tile, which is beside the toilet.
	want := int32(far.Chebyshev(bottom)) + 1 + int32(max(abs(top.X-6), abs(top.Y-6)))
	if got != want {
		t.Fatalf("toilet distance from the level below = %d, want %d", got, want)
	}
	if err := sameAsRebuild(w, f); err != nil {
		t.Fatal(err)
	}
	w.tick++ // fields freshen at most once a tick
	w.SetTerrain(top, Wall)
	w.refreshSpatial()
	f = w.facilityField(Toilet)
	if d := f.at(far); d != -1 {
		t.Errorf("toilet still %d steps away after the stair was walled over", d)
	}
	if err := sameAsRebuild(w, f); err != nil {
		t.Fatalf("repair after cutting the stair: %v", err)
	}
}

// A colonist below follows the field up the stair to the landing level.
func TestColonistClimbsTheStairByField(t *testing.T) {
	w, _, _ := stairWorld(t)
	w.SetTerrain(Point{5, 5, LandingLevel}, Toilet)
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{28, 8, LandingLevel + 1})
	f := w.facilityField(Toilet)
	for i := 0; i < 80 && f.at(e.Pos) > 0; i++ {
		if !w.followField(e, f) {
			t.Fatalf("stuck at %v, %d from the toilet", e.Pos, f.at(e.Pos))
		}
	}
	if e.Pos.Level != LandingLevel || f.at(e.Pos) != 0 {
		t.Fatalf("ended at %v, %d from the toilet", e.Pos, f.at(e.Pos))
	}
	if w.entityAt(e.Pos) != e {
		t.Fatal("occupancy lost track of the colonist across the stair")
	}
	if ids := w.entityIDsNearSorted(Point{28, 8, LandingLevel + 1}, 3); len(ids) != 0 {
		t.Fatalf("the level below still lists %v near where the colonist started", ids)
	}
}

// travelEstimate goes by the stairs that exist, not straight through rock.
func TestTravelEstimateUsesTheStair(t *testing.T) {
	w, _, _ := stairWorld(t)
	a := Point{30, 8, LandingLevel}
	b := Point{30, 8, LandingLevel + 1}
	// Straight down is 1 by Chebyshev; through the stair at x=10 it is 20
	// over, 1 down, 20 back.
	if got := w.travelEstimate(a, b); got != 41 {
		t.Errorf("travelEstimate straight down = %d, want 41 via the stair", got)
	}
	if got := w.travelEstimate(a, Point{2, 2, LandingLevel + 2}); got != unreachableEstimate {
		t.Errorf("travelEstimate to a level with no stair = %d, want unreachable", got)
	}
	if got, want := w.travelEstimate(a, Point{3, 4, LandingLevel}), a.Chebyshev(Point{3, 4, LandingLevel}); got != want {
		t.Errorf("travelEstimate on one level = %d, want Chebyshev %d", got, want)
	}
}

// Nothing sees, reaches or is adjacent through a floor.
func TestNothingReachesThroughAFloor(t *testing.T) {
	w, top, bottom := stairWorld(t)
	if w.hasLineOfSight(top, bottom) {
		t.Error("line of sight through a floor")
	}
	a := w.spawn(Colonist, Point{12, 8, LandingLevel})
	b := w.spawn(Colonist, Point{12, 8, LandingLevel + 1})
	if got := w.colonistsWithin(a.Pos, 2, a.ID); len(got) != 0 {
		t.Errorf("colonistsWithin found %d through the floor", len(got))
	}
	if w.entityAt(Point{12, 8, LandingLevel}) != a || w.entityAt(Point{12, 8, LandingLevel + 1}) != b {
		t.Error("two levels' occupancy at the same (x, y) collided")
	}
}

// checkRoomLabelsAllLevels checks every walkable component, across stairs,
// is exactly one room: the multi-level version of checkRoomLabels.
func checkRoomLabelsAllLevels(t *testing.T, w *World) {
	t.Helper()
	seen := map[Point]bool{}
	labels := map[RoomID]bool{}
	for _, l := range w.layers {
		if l == nil {
			continue
		}
		for y := 0; y < w.Height; y++ {
			for x := 0; x < w.Width; x++ {
				start := Point{x, y, l.Level}
				if !w.Walkable(start) || seen[start] {
					continue
				}
				room := w.roomOf(start)
				if labels[room] {
					t.Fatalf("room %d labels two separate components", room)
				}
				labels[room] = true
				stack := []Point{start}
				seen[start] = true
				for len(stack) > 0 {
					p := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					if got := w.roomOf(p); got != room {
						t.Fatalf("%v is in room %d, its component in %d", p, got, room)
					}
					next := []Point{}
					for _, d := range neighbors8 {
						next = append(next, p.Add(d.X, d.Y))
					}
					if q, ok := w.linkFrom(p); ok {
						next = append(next, q)
					}
					for _, q := range next {
						if w.Walkable(q) && !seen[q] {
							seen[q] = true
							stack = append(stack, q)
						}
					}
				}
			}
		}
	}
}

func (p Point) String() string { return fmt.Sprintf("(%d,%d)@%d", p.X, p.Y, p.Level) }

// stairColony is a generated colony allowed one level down.
func stairColony(t *testing.T, seed int64) *World {
	t.Helper()
	cfg := testConfig()
	cfg.Seed = seed
	cfg.Width, cfg.Height = 120, 70
	cfg.StartColonists = 8
	cfg.DeepestLevel = 2
	return newTestWorld(t, cfg)
}

// runUntil steps w until done reports true, failing after limit ticks.
func runUntil(t *testing.T, w *World, limit int, what string, done func() bool) {
	t.Helper()
	for i := 0; i < limit; i++ {
		if done() {
			return
		}
		w.step()
	}
	if !done() {
		t.Fatalf("%s did not happen within %d ticks", what, limit)
	}
}

// The whole path a player sees: order a stair, the colony marks it out, a
// builder digs it, and the level below exists, generated around its foot.
func TestOrderedStairIsDug(t *testing.T) {
	w := stairColony(t, 3)
	w.manualStairs = 1
	runUntil(t, w, 3000, "a stair", func() bool { return len(w.stairs) > 0 })
	s := w.stairs[0]
	if s.Level != LandingLevel {
		t.Fatalf("stair dug from level %d, want the landing level", s.Level)
	}
	below := w.layer(LandingLevel + 1)
	if below == nil || below.gen == nil || len(below.genChunks) == 0 {
		t.Fatal("the level below was not made, or nothing on it was generated")
	}
	if w.manualStairs != 0 || w.stairPlanned() {
		t.Error("the order is still pending after the stair was dug")
	}
	checkRoomLabelsAllLevels(t, w)
	// The level below is its own rock, not a copy of the landing level.
	same := true
	for _, k := range below.genChunks {
		p := Point{int(k.cx) << genChunkBits, int(k.cy) << genChunkBits, LandingLevel}
		q := Point{p.X, p.Y, LandingLevel + 1}
		for dy := 0; dy < genChunkSize && same; dy++ {
			for dx := 0; dx < genChunkSize && same; dx++ {
				a, b := p.Add(dx, dy), q.Add(dx, dy)
				if w.InBounds(a) && w.lay(a).genDone[w.lay(a).tiles.pageIndex(a.X, a.Y)] &&
					w.TileAt(a).Composition != w.TileAt(b).Composition {
					same = false
				}
			}
		}
	}
	if same {
		t.Error("the level below generated the same ore as the landing level")
	}
}

// A colonist sent to mine below walks down the stair and back up with it.
func TestMinerWorksTheLevelBelow(t *testing.T) {
	w := stairColony(t, 3)
	w.manualStairs = 1
	runUntil(t, w, 3000, "a stair", func() bool { return len(w.stairs) > 0 })
	w.step() // fold the stair into the rooms
	foot := Point{w.stairs[0].X, w.stairs[0].Y, LandingLevel + 1}
	var rock Point
	found := false
	for p := range w.layer(LandingLevel + 1).board.frontier {
		if !w.layer(LandingLevel+1).board.isClaimed(p) && (!found || lessPoint(p, rock)) {
			rock, found = p, true
		}
	}
	if !found {
		t.Fatal("no mineable rock around the foot of the stair")
	}
	var miner *Entity
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Colonist && e.Pos.Level == LandingLevel {
			miner = e
			break
		}
	}
	w.clearJob(miner)
	w.layer(LandingLevel + 1).board.claimMine(rock, miner.ID)
	w.assignMineTarget(miner, rock)
	reachedBelow := false
	runUntil(t, w, 2000, "mining below", func() bool {
		reachedBelow = reachedBelow || miner.Pos.Level == LandingLevel+1
		return w.TerrainAt(rock) != Rock
	})
	if !reachedBelow {
		t.Fatalf("the rock at %v was mined without anyone going below (stair foot %v)", rock, foot)
	}
}

// Two runs of one seed that dig down agree tick for tick, levels and all.
func TestStairRunsAreDeterministic(t *testing.T) {
	run := func() []string {
		w := stairColony(t, 9)
		w.manualStairs = 1
		var hashes []string
		for i := 0; i < 1500; i++ {
			w.step()
			if i%100 == 99 {
				hashes = append(hashes, levelsHash(w))
			}
		}
		if len(w.stairs) == 0 {
			t.Fatal("no stair was dug: the test proves nothing about levels")
		}
		return hashes
	}
	a, b := run(), run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("runs diverge by tick %d:\n%s\n%s", (i+1)*100, a[i], b[i])
		}
	}
}

// levelsHash is goldenHash over every level: tiles and entities, with each
// entity's level.
func levelsHash(w *World) string {
	h := fnvSeed
	for _, l := range w.layers {
		if l == nil {
			continue
		}
		for y := 0; y < w.Height; y++ {
			for x := 0; x < w.Width; x++ {
				t := w.TileAt(Point{x, y, l.Level})
				h = fnvAdd(h, uint64(t.Terrain)|uint64(t.Composition)<<8|uint64(l.Level)<<16)
			}
		}
	}
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		for _, v := range []int{int(id), e.Pos.X, e.Pos.Y, int(e.Pos.Level), e.HP, int(e.State)} {
			h = fnvAdd(h, uint64(v))
		}
	}
	return fmt.Sprintf("%016x levels=%d", h, len(w.layers))
}

// A colony that is not allowed to dig down never plans a stair, however the
// frontier stands, so nothing about a one-level game changes.
func TestNoStairsAtDeepestLevelOne(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 120, 70
	w := newTestWorld(t, cfg)
	w.manualStairs = 1
	for i := 0; i < 500; i++ {
		w.step()
	}
	if len(w.stairs) > 0 || w.stairPlanned() || len(w.layers) != int(LandingLevel)+1 {
		t.Fatal("a colony limited to the landing level planned or dug a stair")
	}
}

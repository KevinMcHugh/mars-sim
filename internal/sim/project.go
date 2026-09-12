package sim

// Construction projects are how the colony builds structures it plans as a group
// rather than one colonist at a time. A project is a set of tile designations
// (buildTasks); the colony creates the project (collective planning) and any
// number of colonists each claim and build individual tasks (collaboration).
// This is a general coordination backbone — facility rooms are the first project
// kind, but barracks, storage, and so on would be new task generators over the
// same machinery.
//
// A buildTask converts one tile to a desired terrain (a facility, today). Tasks
// are independent conversions of open floor, so colonists can work them in any
// order and in parallel, and each facility comes online the moment its task is
// finished rather than waiting on the rest of the room.

// buildTask is a single tile to construct as part of a project.
type buildTask struct {
	pos     Point
	terrain Terrain  // desired terrain for pos
	owner   EntityID // colonist currently building it; 0 if unclaimed
}

// project is a planned structure: the colony builds all its tasks, then it is
// retired.
type project struct {
	id    int
	name  string
	tasks []*buildTask
}

// taskDone reports whether a task's tile already holds its desired terrain.
func (w *World) taskDone(t *buildTask) bool {
	return w.TerrainAt(t.pos) == t.terrain
}

// taskWorkable reports whether a task can be built right now: not done and its
// tile is open floor (every task converts open floor into a facility).
func (w *World) taskWorkable(t *buildTask) bool {
	return !w.taskDone(t) && w.TerrainAt(t.pos) == Floor
}

// taskReachable reports whether a builder in the given room can reach a task:
// some neighbor tile is walkable and in that room (the builder stands there).
func (w *World) taskReachable(pos Point, room RoomID) bool {
	for _, d := range neighbors8 {
		n := pos.Add(d.X, d.Y)
		if w.Walkable(n) && w.roomOf(n) == room {
			return true
		}
	}
	return false
}

// claimNearestTask claims, for colonist id, the nearest workable and unclaimed
// task reachable from its room, across all projects. Ties break by position so
// the choice is deterministic.
func (w *World) claimNearestTask(from Point, id EntityID) (*buildTask, bool) {
	room := w.roomOf(from)
	if room == 0 {
		return nil, false
	}
	var best *buildTask
	bestDist := 1 << 30
	for _, p := range w.projects {
		for _, t := range p.tasks {
			if t.owner != 0 || !w.taskWorkable(t) || !w.taskReachable(t.pos, room) {
				continue
			}
			if d := from.Chebyshev(t.pos); best == nil || d < bestDist ||
				(d == bestDist && lessPoint(t.pos, best.pos)) {
				best, bestDist = t, d
			}
		}
	}
	if best == nil {
		return nil, false
	}
	best.owner = id
	return best, true
}

// projectFacilityTasks counts not-yet-done facility tasks of a kind across all
// projects, so demand planning accounts for facilities already designated.
func (w *World) projectFacilityTasks(kind Terrain) int {
	n := 0
	for _, p := range w.projects {
		for _, t := range p.tasks {
			if t.terrain == kind && !w.taskDone(t) {
				n++
			}
		}
	}
	return n
}

// rebuildBuildTiles refreshes the set of not-yet-built task tiles. Kept as a set
// so movement can test a tile in O(1); rebuilt once per tick because tasks are
// few and completions happen throughout a tick.
func (w *World) rebuildBuildTiles() {
	for p := range w.buildTiles {
		delete(w.buildTiles, p)
	}
	for _, p := range w.projects {
		for _, t := range p.tasks {
			if !w.taskDone(t) {
				w.buildTiles[t.pos] = true
			}
		}
	}
}

// pruneProjects drops projects whose every task is done.
func (w *World) pruneProjects() {
	kept := w.projects[:0]
	for _, p := range w.projects {
		done := true
		for _, t := range p.tasks {
			if !w.taskDone(t) {
				done = false
				break
			}
		}
		if done {
			w.log.add("A " + p.name + " is complete.")
			continue
		}
		kept = append(kept, p)
	}
	w.projects = kept
}

// Facility-bay geometry: a single-kind row of facilities carved against the
// cavern's rock face — a nutrient bay of pods, or a latrine of toilets. The
// cavern rock is the back wall; the facilities line up against it and the whole
// front is open to the cavern.
//
// Deliberately, a bay builds no walls of its own. Every walled design tried here
// starved the colony, each by a different mechanism, because colonists roam and
// mine the whole cavern so someone is always on the far side of, or crowded
// against, any wall: an enclosed room traps its builders; a free-standing wall
// funnels seekers through its last unbuilt gap and deadlocks the crowd; a wall
// tile next to a facility can never be built because a colonist using the
// facility always stands on it. Walls are cosmetic today, so bays skip them and
// lean on the rock instead. The remaining rules keep a bay both buildable and
// usable under a dense crowd:
//
//   - Backed by rock. Nothing is ever behind a bay, so no one queues behind it.
//   - Open front. Seekers spread across every facility instead of funneling.
//   - Facilities spaced one tile apart, never adjacent. A colonist using a
//     facility stands on its neighbor tiles; if a neighbor were another unbuilt
//     facility, that facility could never be built (a user is always standing on
//     it) and the project would deadlock. A gap keeps every facility buildable
//     no matter how big the crowd — which is also why pods and toilets get
//     separate bays rather than one interleaved row.
const (
	roomFacilities = 4 // facilities designated in a full room (alternating kinds)
	roomFrontClear = 2 // open rows required in front for approach and spread
)

// roomKinds is the facility to place at each slot of a room, alternating so one
// room serves both needs. Pods lead so that a room forced to shrink to a single
// facility still gets the fatal-need one.
var roomKinds = []Terrain{NutrientPod, Toilet, NutrientPod, Toilet}

// bayWidth is the row width spanned by n facilities spaced one tile apart.
func bayWidth(n int) int { return 2*n - 1 }

// planFacilities keeps enough life-support planned or built for the population,
// creating a facility-room project when the colony is short of either pods or
// toilets. Called on a cadence from step.
//
// Only one room is under construction at a time: a second concurrent project
// would split builders across two sites and, in a tight early cavern, mob the
// colony into a gridlock where nothing finishes and no one mines for space. One
// room at a time keeps most colonists mining (growing the cavern) while a small
// crew finishes the current room, then the next is planned.
func (w *World) planFacilities() {
	if len(w.projects) > 0 {
		return
	}
	desired := w.desiredFacilities(w.countKind(Colonist))
	if w.plannedFacilities(NutrientPod) >= desired && w.plannedFacilities(Toilet) >= desired {
		return
	}
	w.planFacilityRoom()
}

// planFacilityRoom designates a new room of facilities at a suitable rock-backed
// site. It prefers a full room (roomFacilities) but falls back to fewer when
// only a shorter rock-backed run is open, so progress is made even in a cramped
// cavern.
func (w *World) planFacilityRoom() {
	for n := roomFacilities; n >= 1; n-- {
		o, ok := w.findRoomSite(bayWidth(n))
		if !ok {
			continue // no rock-backed run this wide; try a smaller room
		}
		w.designateRoom(o, n)
		return
	}
	// No rock-backed site of any width yet; colonists dig on, retry later.
}

// designateRoom adds a room project: n facilities against the rock, on every
// other tile of the row (so no two are adjacent), alternating pod and toilet.
func (w *World) designateRoom(o Point, n int) {
	p := &project{id: w.nextProjectID, name: "facility room"}
	w.nextProjectID++
	for i, dx := 0, 0; i < n; i, dx = i+1, dx+2 {
		p.tasks = append(p.tasks, &buildTask{pos: Point{o.X + dx, o.Y}, terrain: roomKinds[i]})
	}
	w.projects = append(w.projects, p)
	w.log.add("The colony marks out a new facility room.")
}

// findRoomSite returns the left end of a width-long facility row backed by rock:
// a run whose row is open undesignated floor, whose row behind (into the rock) is
// solid rock, and which has roomFrontClear open rows in front for approach. The
// site nearest the colony center is chosen.
func (w *World) findRoomSite(width int) (Point, bool) {
	designated := make(map[Point]bool)
	for _, p := range w.projects {
		for _, t := range p.tasks {
			designated[t.pos] = true
		}
	}
	center := Point{w.Width / 2, w.Height / 2}
	var best Point
	found := false
	bestDist := 1 << 30
	for oy := 1; oy+1+roomFrontClear <= w.Height; oy++ {
		for ox := 0; ox+width <= w.Width; ox++ {
			if !w.rockBackedSiteClear(ox, oy, width, designated) {
				continue
			}
			rc := Point{ox + width/2, oy}
			if d := center.Chebyshev(rc); d < bestDist {
				best, bestDist, found = Point{ox, oy}, d, true
			}
		}
	}
	return best, found
}

// rockBackedSiteClear reports whether a width-long facility row at (ox,oy) is
// buildable: the row itself is open undesignated floor, the row behind it is
// solid rock (the back wall), and roomFrontClear rows in front are open floor so
// a crowd can approach and spread.
func (w *World) rockBackedSiteClear(ox, oy, width int, designated map[Point]bool) bool {
	for dx := 0; dx < width; dx++ {
		if w.TerrainAt(Point{ox + dx, oy - 1}) != Rock { // back must be solid rock
			return false
		}
		row := Point{ox + dx, oy}
		if w.TerrainAt(row) != Floor || designated[row] { // facility row: open floor
			return false
		}
		for fy := 1; fy <= roomFrontClear; fy++ {
			front := Point{ox + dx, oy + fy}
			if !w.Walkable(front) || designated[front] {
				return false
			}
		}
	}
	return true
}

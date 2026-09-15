package sim

// Construction projects are how the colony builds structures it plans as a group
// rather than one colonist at a time. A project is a set of tile designations
// (buildTasks); the colony creates the project (collective planning) and any
// number of colonists each claim and build individual tasks (collaboration).
// This is a general coordination backbone — facility rooms are the first project
// kind, but barracks, storage, and so on would be new task generators over the
// same machinery.
//
// A buildTask converts one tile to a desired terrain. Tasks in the same phase
// can be built in parallel; a project advances only when the current phase is
// complete. This lets a room raise its walls before its facilities come online.

// buildTask is a single tile to construct as part of a project.
type buildTask struct {
	pos     Point
	terrain Terrain  // desired terrain for pos
	owner   EntityID // colonist currently building it; 0 if unclaimed
	phase   int      // lower phases in this project must finish first
}

// project is a planned structure: the colony builds all its tasks, then it is
// retired.
type project struct {
	id         int
	name       string
	queuedTick int // w.tick when the project was designated, for job board display
	tasks      []*buildTask
}

// taskDone reports whether a task's tile already holds its desired terrain.
func (w *World) taskDone(t *buildTask) bool {
	return w.TerrainAt(t.pos) == t.terrain
}

// taskWorkable reports whether a task can be built right now: not done and its
// tile is open floor.
func (w *World) taskWorkable(t *buildTask) bool {
	return !w.taskDone(t) && w.TerrainAt(t.pos) == Floor
}

// activeProjectPhase returns the earliest phase with unfinished work.
func (w *World) activeProjectPhase(p *project) (int, bool) {
	phase, found := 0, false
	for _, t := range p.tasks {
		if w.taskDone(t) {
			continue
		}
		if !found || t.phase < phase {
			phase, found = t.phase, true
		}
	}
	return phase, found
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
		phase, ok := w.activeProjectPhase(p)
		if !ok {
			continue
		}
		for _, t := range p.tasks {
			if t.phase != phase || t.owner != 0 || w.occupied(t.pos) || !w.taskWorkable(t) ||
				!w.taskReachable(t.pos, room) {
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

// reachableFacilityConstruction reports whether construction already underway
// in from's room will provide kind. Global project counts are insufficient here:
// a colonist disconnected from that project needs to build its own life support.
func (w *World) reachableFacilityConstruction(from Point, kind Terrain) bool {
	room := w.roomOf(from)
	if room == 0 {
		return false
	}
	for _, p := range w.projects {
		provides := false
		for _, t := range p.tasks {
			if t.terrain == kind && !w.taskDone(t) {
				provides = true
				break
			}
		}
		if !provides {
			continue
		}
		phase, ok := w.activeProjectPhase(p)
		if !ok {
			continue
		}
		for _, t := range p.tasks {
			if t.phase == phase && w.taskWorkable(t) && w.taskReachable(t.pos, room) {
				return true
			}
		}
	}
	for _, e := range w.entities {
		if e.Kind == Colonist && e.Job == JobBuild && e.task == nil &&
			e.BuildKind == kind && w.sameRoom(from, e.Target) {
			return true
		}
	}
	return false
}

// rebuildBuildTiles refreshes the set of tasks in each project's active phase.
// Future-phase tiles remain usable as construction access until their phase
// begins. Kept as a set so movement can test a tile in O(1).
func (w *World) rebuildBuildTiles() {
	for p := range w.buildTiles {
		delete(w.buildTiles, p)
	}
	for _, p := range w.projects {
		phase, ok := w.activeProjectPhase(p)
		if !ok {
			continue
		}
		for _, t := range p.tasks {
			if t.phase == phase && !w.taskDone(t) {
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

// Room geometry: a row of facilities inside a complete placed-wall perimeter. A
// one-tile doorway in the front wall is the room's permanent entrance. Colonists
// can now traverse occupied tiles, so a crowd in that doorway does not cut the
// room off.
//
// Construction is phased to avoid the old wall deadlock: every wall is raised
// before any facility comes online. A facility user therefore cannot stand on a
// pending wall tile, and the permanent doorway means the last wall cannot trap
// builders. Facilities remain spaced apart so each retains several access tiles.
const (
	roomFacilities = 4 // facilities designated in a full room
	roomFrontClear = 2 // interior rows between facilities and the front wall
	roomApproach   = 1 // open row outside the doorway
	roomWallPhase  = 0
	roomFitPhase   = 1
)

// roomRecipe describes a buildable room kind. The one wall-and-doorway shell is
// shared; recipes differ only in the facilities they line up along the back and
// how few of them still make a worthwhile room. Adding a room kind (barracks,
// storage, ...) is a recipe here plus a demand check in planRooms.
type roomRecipe struct {
	name    string    // project name, also logged on completion
	kinds   []Terrain // facilities placed left to right, cycled to fill the bay
	minFac  int       // fewest facilities worth building as a partial room
	planLog string    // logged when the room is marked out
}

var (
	// lifeSupportRoom alternates pods and toilets so one room serves both the
	// food and bladder needs; a partial room must still serve both.
	lifeSupportRoom = roomRecipe{
		name: "facility room", kinds: []Terrain{NutrientPod, Toilet}, minFac: 2,
		planLog: "The colony marks out a new facility room.",
	}
	// dormRoom is a bay of bunks. Even a single bunk is worth raising.
	dormRoom = roomRecipe{
		name: "dormitory", kinds: []Terrain{Bed}, minFac: 1,
		planLog: "The colony marks out a new dormitory.",
	}
)

// bayWidth is the row width spanned by n facilities spaced one tile apart.
func bayWidth(n int) int { return 2*n - 1 }

// roomFrontWallY returns the front-wall row for a room whose facility row is y.
func roomFrontWallY(y int) int { return y + roomFrontClear + 1 }

// concurrentProjectColonists is how many colonists it takes to justify one
// more room under construction at once — see maxConcurrentProjects.
const concurrentProjectColonists = 8

// maxConcurrentProjects caps how many rooms can be under construction at once,
// scaling with population: a small colony still builds one room at a time (a
// second concurrent project splits its handful of builders across two sites
// and, in a tight early cavern, can mob the colony into a gridlock where
// nothing finishes and no one mines for space), while a larger one can run
// more crews in parallel so facility supply keeps pace with growth.
// cfg.MaxConcurrentProjects is the ceiling on that growth.
func (w *World) maxConcurrentProjects() int {
	n := 1 + w.countKind(Colonist)/concurrentProjectColonists
	ceiling := w.cfg.MaxConcurrentProjects
	if ceiling < 1 {
		ceiling = 1
	}
	return min(n, ceiling)
}

// planRooms keeps enough of each need's facility planned or built for the
// population, marking out at most one new room per call. Called on a cadence
// from step.
//
// Life support comes before bunks: food is fatal, so a colony short of pods or
// toilets builds a facility room before a dormitory.
func (w *World) planRooms() {
	if len(w.projects) >= w.maxConcurrentProjects() {
		return
	}
	if w.manualFacilityRooms > 0 {
		before := len(w.projects)
		w.planRoom(lifeSupportRoom)
		if len(w.projects) > before {
			w.manualFacilityRooms--
		}
		return
	}
	if w.manualDormitories > 0 {
		before := len(w.projects)
		w.planRoom(dormRoom)
		if len(w.projects) > before {
			w.manualDormitories--
		}
		return
	}
	desired := w.desiredFacilities(w.countKind(Colonist))
	if w.plannedFacilities(NutrientPod) < desired || w.plannedFacilities(Toilet) < desired {
		w.planRoom(lifeSupportRoom)
		return
	}
	if w.plannedFacilities(Bed) < desired {
		w.planRoom(dormRoom)
	}
}

// planRoom designates a new room from a recipe at a suitable open site. It
// prefers a full room (roomFacilities) but falls back to fewer when only a
// shorter clear area is available, so progress is made even in a cramped cavern.
func (w *World) planRoom(r roomRecipe) {
	for n := roomFacilities; n >= r.minFac; n-- {
		o, ok := w.findRoomSite(bayWidth(n))
		if !ok {
			continue // no rock-backed run this wide; try a smaller room
		}
		w.designateRoom(r, o, n)
		return
	}
	// No rock-backed site large enough for a worthwhile room yet; colonists dig
	// on and planning retries later.
}

// designateRoom adds a phased room project from a recipe. Its complete perimeter
// is built first, except for the centered front doorway; then n facilities are
// built one tile inside the back wall, drawn from the recipe's kinds in order.
func (w *World) designateRoom(r roomRecipe, o Point, n int) {
	p := &project{id: w.nextProjectID, name: r.name, queuedTick: w.tick}
	w.nextProjectID++
	width := bayWidth(n)
	backY := o.Y - 1
	frontY := roomFrontWallY(o.Y)
	for y := backY; y <= frontY; y++ {
		p.tasks = append(p.tasks,
			&buildTask{pos: Point{o.X - 1, y}, terrain: Wall, phase: roomWallPhase},
			&buildTask{pos: Point{o.X + width, y}, terrain: Wall, phase: roomWallPhase},
		)
	}
	doorX := o.X + width/2
	for x := o.X; x < o.X+width; x++ {
		p.tasks = append(p.tasks,
			&buildTask{pos: Point{x, backY}, terrain: Wall, phase: roomWallPhase})
		if x != doorX {
			p.tasks = append(p.tasks,
				&buildTask{pos: Point{x, frontY}, terrain: Wall, phase: roomWallPhase})
		}
	}
	for i, dx := 0, 0; i < n; i, dx = i+1, dx+2 {
		p.tasks = append(p.tasks,
			&buildTask{pos: Point{o.X + dx, o.Y}, terrain: r.kinds[i%len(r.kinds)], phase: roomFitPhase})
	}
	w.projects = append(w.projects, p)
	w.log.add(r.planLog)
}

// findRoomSite returns the left end of a width-long facility row in a niche at
// the cavern edge. Solid rock — or another room's already-placed wall — beyond
// the placed back wall keeps the room from becoming a free-standing obstacle
// across an open route; backing onto a neighbor's wall lets rooms sit flush
// against each other, sharing that boundary instead of each needing its own
// untouched rock vein. The room footprint and its side/front construction lane
// must be clear floor.
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
	for oy := 2; roomFrontWallY(oy)+roomApproach < w.Height; oy++ {
		for ox := 2; ox+width+1 < w.Width; ox++ {
			if !w.roomSiteClear(ox, oy, width, designated) {
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

// roomSiteClear reports whether a room at (ox,oy) is buildable. The full room
// starts as floor, its placed rear wall is backed by solid rock or another
// room's wall, and a connected exterior lane keeps the side and front tasks
// reachable.
func (w *World) roomSiteClear(ox, oy, width int, designated map[Point]bool) bool {
	backY := oy - 1
	frontY := roomFrontWallY(oy)
	for x := ox; x < ox+width; x++ {
		if t := w.TerrainAt(Point{x, backY - 1}); t != Rock && t != Wall {
			return false
		}
	}
	for y := backY; y <= frontY; y++ {
		for x := ox - 1; x <= ox+width; x++ {
			p := Point{x, y}
			if w.TerrainAt(p) != Floor || designated[p] {
				return false
			}
		}
		// Keep an exterior lane beside each side wall so every wall task stays
		// reachable even after its neighbors have been raised.
		for _, x := range []int{ox - 2, ox + width + 1} {
			p := Point{x, y}
			if !w.Walkable(p) || designated[p] {
				return false
			}
		}
	}
	// Connect both exterior side lanes in front of the room. The back lies
	// against rock; builders reach its wall tasks from the future facility row.
	for x := ox - 2; x <= ox+width+1; x++ {
		p := Point{x, frontY + roomApproach}
		if !w.Walkable(p) || designated[p] {
			return false
		}
	}
	return true
}

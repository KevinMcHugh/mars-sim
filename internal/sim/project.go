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

// taskDone reports whether a task's tile already holds its desired terrain —
// or, for a dig task (terrain Floor), has moved past Rock at all. A dig and a
// wall/facility task can target the same position (excavate it, then build on
// it); once the later task converts that floor into a wall or facility, the
// dig task must count as done too, or it can never be satisfied again and
// permanently blocks the project's phase from advancing.
func (w *World) taskDone(t *buildTask) bool {
	if t.terrain == Floor {
		return w.TerrainAt(t.pos) != Rock
	}
	return w.TerrainAt(t.pos) == t.terrain
}

// taskWorkable reports whether a task can be built right now: not done, and
// its tile holds the right starting terrain — open floor for a build task,
// or still-solid rock for a dig task (terrain Floor; see roomDigPhase).
func (w *World) taskWorkable(t *buildTask) bool {
	if w.taskDone(t) {
		return false
	}
	if t.terrain == Floor {
		return w.TerrainAt(t.pos) == Rock
	}
	return w.TerrainAt(t.pos) == Floor
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
// the choice is deterministic. Used for general work-seeking, where any
// project's work is equally good.
func (w *World) claimNearestTask(from Point, id EntityID) (*buildTask, bool) {
	return w.claimNearestTaskIn(from, id, w.projects)
}

// claimNearestTaskProviding is claimNearestTask restricted to projects that
// provide a facility of the given kind (i.e. some task in them targets that
// terrain). Use this — never the unrestricted claimNearestTask — when a
// colonist is responding to a specific urgent need: claiming a task from an
// unrelated project (say, digging a dormitory while starving) still marks the
// colonist as "handling" its need via e.task != nil, but the starvation grace
// period only covers reachable construction that actually provides the
// needed facility (see applyStarvation, reachableFacilityConstruction) — so
// an unrestricted claim can starve a colonist while it is technically busy.
func (w *World) claimNearestTaskProviding(from Point, id EntityID, kind Terrain) (*buildTask, bool) {
	var providing []*project
	for _, p := range w.projects {
		for _, t := range p.tasks {
			if t.terrain == kind {
				providing = append(providing, p)
				break
			}
		}
	}
	return w.claimNearestTaskIn(from, id, providing)
}

func (w *World) claimNearestTaskIn(from Point, id EntityID, projects []*project) (*buildTask, bool) {
	room := w.roomOf(from)
	if room == 0 {
		return nil, false
	}
	var best *buildTask
	bestDist := 1 << 30
	for _, p := range projects {
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
//
// A room's interior need not be pre-mined: any footprint tile still solid rock
// gets a roomDigPhase task (see designateRoom) that must clear before the wall
// phase can start. Digging tasks share the normal claiming/reachability rules,
// so they naturally clear from the edge inward as each newly-opened tile makes
// its neighbor reachable — no special ordering logic needed.
const (
	roomFacilities = 4  // facilities designated in a full room
	roomFrontClear = 2  // interior rows between facilities and the front wall
	roomApproach   = 1  // open row outside the doorway
	roomDigPhase   = -1 // excavate any not-yet-floor interior tile, before walls
	roomWallPhase  = 0
	roomFitPhase   = 1
)

// roomRecipe describes a buildable room kind. The one wall-and-doorway shell is
// shared; recipes differ only in the facilities they line up along the back and
// how few — or how many — of them still make a worthwhile room. Adding a room kind (barracks,
// storage, ...) is a recipe here plus a demand check in planRooms.
type roomRecipe struct {
	name   string    // project name, also logged on completion
	kinds  []Terrain // facilities placed left to right, cycled to fill the bay
	minFac int       // fewest facilities worth building as a partial room
	// maxFac caps the bay for a recipe whose facility is not wanted in bulk;
	// 0 means the usual full-size room (roomFacilities). A trash room sets it
	// to 1: the colony wants exactly one incinerator, and without the cap
	// planRoom would happily fit a closet with three of them into the same
	// walls.
	maxFac  int
	planLog string // logged when the room is marked out
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
	// trashRoom houses the incinerator that refuse is hauled to and burned in.
	// One machine is a working trash room, so its minimum is one — and the
	// planner only ever wants a single one (see planRooms), because an
	// incinerator serves the whole colony rather than a share of its
	// population the way a pod or a bunk does. Walling it in is the point:
	// the bodies and the burning happen somewhere the colony chose, not
	// wherever someone happened to die.
	trashRoom = roomRecipe{
		name: "trash room", kinds: []Terrain{Incinerator}, minFac: 1, maxFac: 1,
		planLog: "The colony marks out a new trash room.",
	}
	// storageRoom encloses one large trunk. Containers are deliberately placed
	// one at a time: unlike need facilities, their useful capacity is already
	// six full colonist inventories and demand is player-directed.
	storageRoom = roomRecipe{
		name: "storage room", kinds: []Terrain{Storage}, minFac: 1, maxFac: 1,
		planLog: "The colony marks out a new storage room.",
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
// Life support comes before bunks, strictly: food is fatal and sleep is not,
// so while the colony is short of pods or toilets it holds off on dormitories
// entirely — even on a cycle where life support fails to find a site (its
// two-facility minimum is pickier than a dormitory's one). Letting a
// dormitory use that cycle instead was tried and reverted: it let dormitories
// win a scarce concurrent-build slot ahead of life support and measurably
// delayed food, starving a colonist in testing. A colony stuck unable to site
// life support at all is a siting problem (see roomSiteClear's wall-sharing)
// to fix there, not a priority order to bend here.
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
	if w.manualTrashRooms > 0 {
		before := len(w.projects)
		w.planRoom(trashRoom)
		if len(w.projects) > before {
			w.manualTrashRooms--
		}
		return
	}
	if w.manualStorageRooms > 0 {
		before := len(w.projects)
		w.planRoom(storageRoom)
		if len(w.projects) > before {
			w.manualStorageRooms--
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
		return
	}
	// Sanitation last, and only once there is actually a mess: an incinerator
	// nothing has been killed near is a room's worth of digging spent on
	// nothing. Demand is one — not desiredFacilities — because the colony's
	// refuse is not proportional to its headcount the way its appetite is, and
	// a second incinerator would only split the haulers.
	if w.refuseTotal() > 0 && w.plannedFacilities(Incinerator) < 1 {
		w.planRoom(trashRoom)
	}
}

// planRoom designates a new room from a recipe at a suitable open site. It
// prefers the recipe's largest bay (roomFacilities, unless it caps itself with
// maxFac) but falls back to fewer facilities when only a shorter clear area is
// available, so progress is made even in a cramped cavern.
func (w *World) planRoom(r roomRecipe) {
	largest := r.maxFac
	if largest <= 0 || largest > roomFacilities {
		largest = roomFacilities
	}
	for n := largest; n >= r.minFac; n-- {
		o, ok := w.findRoomSite(bayWidth(n))
		if !ok {
			continue // no site this wide; try a smaller room
		}
		w.designateRoom(r, o, n)
		return
	}
	// No site large enough for even this recipe's minimum yet; colonists dig
	// on and planning retries later.
}

// designateRoom adds a phased room project from a recipe. Any interior tile
// still solid rock is dug first (roomDigPhase); its complete perimeter is
// then built, except for the centered front doorway; then n facilities are
// built one tile inside the back wall, drawn from the recipe's kinds in order.
func (w *World) designateRoom(r roomRecipe, o Point, n int) {
	p := &project{id: w.nextProjectID, name: r.name, queuedTick: w.tick}
	w.nextProjectID++
	width := bayWidth(n)
	backY := o.Y - 1
	frontY := roomFrontWallY(o.Y)
	for y := backY; y <= frontY; y++ {
		for x := o.X; x < o.X+width; x++ {
			if pos := (Point{x, y}); w.TerrainAt(pos) == Rock {
				p.tasks = append(p.tasks, &buildTask{pos: pos, terrain: Floor, phase: roomDigPhase})
			}
		}
	}
	for y := backY; y <= frontY; y++ {
		// A side tile that is already a wall is a party wall shared with a
		// neighboring room (see roomSiteClear): this room needs no task of its
		// own there.
		if left := (Point{o.X - 1, y}); w.TerrainAt(left) != Wall {
			p.tasks = append(p.tasks, &buildTask{pos: left, terrain: Wall, phase: roomWallPhase})
		}
		if right := (Point{o.X + width, y}); w.TerrainAt(right) != Wall {
			p.tasks = append(p.tasks, &buildTask{pos: right, terrain: Wall, phase: roomWallPhase})
		}
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
// untouched rock vein.
//
// It tries a fully pre-cleared site first, and only falls back to one whose
// interior still needs excavating (see roomSiteClear, designateRoom) if no
// clear site exists at all. A clear site is strictly faster to finish — no
// dig phase — so preferring it, rather than just picking whichever candidate
// is nearest map center, keeps a life-support room from landing somewhere
// slower to complete than it needed to be merely because that spot happened
// to be a little closer to center; that measurably delayed food in testing.
func (w *World) findRoomSite(width int) (Point, bool) {
	if site, ok := w.findRoomSiteAllowingRock(width, false); ok {
		return site, true
	}
	return w.findRoomSiteAllowingRock(width, true)
}

// roomSearchStartRadius is the first box half-width findRoomSiteAllowingRock
// searches around the map center, in tiles.
const roomSearchStartRadius = 64

// roomSearchMaxRadius bounds how far findRoomSiteAllowingRock's box search
// grows around the map center when nothing has been carved yet (so
// carvedSearchRadius has no box to measure). Comfortably covers a starting
// chamber without ever approaching a huge map's full extent.
const roomSearchMaxRadius = roomSearchStartRadius * 4

// carvedSearchRadius returns the largest radius findRoomSiteAllowingRock could
// ever need to try around center: past this, the box already contains every
// tile that has ever been carved out of Rock, plus a margin for a room's side
// walls and lanes, and roomSiteClear requires a site's side walls to already
// be Floor or Wall — so no valid site can lie any further out, on a map of
// any size. Without this cap, failing to find a site (routine — e.g. no
// perimeter is ready yet for the next room) would double the search box all
// the way out to the full map before giving up.
func (w *World) carvedSearchRadius(center Point, width int) int {
	if !w.carvedAny {
		return roomSearchMaxRadius
	}
	margin := width + 2
	corners := [4]Point{
		{w.carvedMin.X - margin, w.carvedMin.Y - margin},
		{w.carvedMin.X - margin, w.carvedMax.Y + margin},
		{w.carvedMax.X + margin, w.carvedMin.Y - margin},
		{w.carvedMax.X + margin, w.carvedMax.Y + margin},
	}
	r := roomSearchStartRadius
	for _, c := range corners {
		r = max(r, center.Chebyshev(c))
	}
	return r
}

func (w *World) findRoomSiteAllowingRock(width int, allowRock bool) (Point, bool) {
	designated := make(map[Point]bool)
	for _, p := range w.projects {
		for _, t := range p.tasks {
			designated[t.pos] = true
		}
	}
	center := Point{w.Width / 2, w.Height / 2}

	// Valid domain, per the original bounds check: oy in [oyLo, oyHi), ox in
	// [oxLo, oxHi).
	const oxLo, oyLo = 2, 2
	oxHi := w.Width - width - 1
	oyHi := w.Height - roomFrontClear - 1 - roomApproach

	// A usable site needs an already-cleared floor lane beside it (see
	// roomSiteClear), so sites cluster near the existing colony rather than
	// scattering across untouched rock — and a colony starts at the map
	// center. Search outward in growing boxes instead of scanning the whole
	// grid: once a box comes up empty, any site a larger box finds is
	// guaranteed to be the true nearest overall (nothing closer was skipped),
	// so this returns exactly what a full scan would, but pays only for the
	// area actually searched — flat cost near the colony instead of O(map
	// area) on a huge, mostly empty map. maxRadius caps that growth at the
	// carved area's own extent, so the "no site fits yet" case — the common
	// one — also stays cheap instead of expanding to the full map.
	maxRadius := w.carvedSearchRadius(center, width)
	for radius := min(roomSearchStartRadius, maxRadius); ; radius = min(radius*2, maxRadius) {
		boxOxLo, boxOxHi := max(oxLo, center.X-radius), min(oxHi, center.X+radius)
		boxOyLo, boxOyHi := max(oyLo, center.Y-radius), min(oyHi, center.Y+radius)

		var best Point
		found := false
		bestDist := 1 << 30
		for oy := boxOyLo; oy < boxOyHi; oy++ {
			for ox := boxOxLo; ox < boxOxHi; ox++ {
				if !w.roomSiteClear(ox, oy, width, designated, allowRock) {
					continue
				}
				rc := Point{ox + width/2, oy}
				if d := center.Chebyshev(rc); d < bestDist {
					best, bestDist, found = Point{ox, oy}, d, true
				}
			}
		}
		if found {
			return best, true
		}
		atCap := radius >= maxRadius
		fullDomain := boxOxLo <= oxLo && boxOxHi >= oxHi && boxOyLo <= oyLo && boxOyHi >= oyHi
		if atCap || fullDomain {
			return Point{}, false // no site can exist any further out
		}
	}
}

// roomSiteClear reports whether a room at (ox,oy) is buildable. The interior
// (where this room's own back/front walls and facilities go) must be clear
// floor — or, when allowRock is set, may also be still-solid rock, which a
// dig task excavates before the wall phase starts (see designateRoom) — as
// long as it is unclaimed by another project; the placed rear wall is backed
// by solid rock or another room's wall; and each side wall is either freshly
// built (with a connected exterior lane keeping it reachable) or reused
// outright from an already-placed, unclaimed neighboring wall — the two rooms
// then sit flush, sharing that one tile as a party wall instead of each
// building its own. The exterior lanes and front approach (never dug) always
// touch the interior's front row, so when allowRock permits solid rock there
// is always at least one already-reachable tile to start digging from.
func (w *World) roomSiteClear(ox, oy, width int, designated map[Point]bool, allowRock bool) bool {
	backY := oy - 1
	frontY := roomFrontWallY(oy)
	for x := ox; x < ox+width; x++ {
		if t := w.TerrainAt(Point{x, backY - 1}); t != Rock && t != Wall {
			return false
		}
	}
	for y := backY; y <= frontY; y++ {
		for x := ox; x < ox+width; x++ {
			p := Point{x, y}
			t := w.TerrainAt(p)
			if designated[p] || (t != Floor && !(allowRock && t == Rock)) {
				return false
			}
		}
		// Each side wall is clear floor (this room will build its own wall
		// there, so keep an exterior lane beside it reachable even after its
		// neighbors have been raised) or an unclaimed wall already placed by
		// another room (shared outright: nothing more is needed on that side).
		for _, side := range [2]struct{ wall, lane int }{{ox - 1, ox - 2}, {ox + width, ox + width + 1}} {
			p := Point{side.wall, y}
			if designated[p] {
				return false
			}
			switch w.TerrainAt(p) {
			case Floor:
				lane := Point{side.lane, y}
				if !w.Walkable(lane) || designated[lane] {
					return false
				}
			case Wall:
				// Shared party wall: this room needs nothing beyond it.
			default:
				return false
			}
		}
	}
	// Connect both exterior side lanes in front of the room, for whichever
	// sides are freshly built — a shared side has no lane to connect. The room's
	// own front row (ox-1..ox+width) is always required either way, so builders
	// can reach the door and front wall tasks.
	loX, hiX := ox-1, ox+width
	if w.TerrainAt(Point{ox - 1, frontY}) != Wall {
		loX = ox - 2
	}
	if w.TerrainAt(Point{ox + width, frontY}) != Wall {
		hiX = ox + width + 1
	}
	for x := loX; x <= hiX; x++ {
		p := Point{x, frontY + roomApproach}
		if !w.Walkable(p) || designated[p] {
			return false
		}
	}
	return true
}

package sim

import (
	"cmp"
	"slices"
)

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
	// clears is what a dig task (terrain Floor) clears out of the way: Rock,
	// the zero value, for every room's excavation; Wall or Hull for a passage
	// (see planPassage) that breaks a structure down to get through it.
	clears Terrain
	// order is the work order paying for this task, and proj the project it
	// belongs to (see workorder.go). A task built by hand in a test has
	// neither: it is unpaid, and its fixture stays the colony's.
	order *WorkOrder
	proj  *project
}

// project is a planned structure: the colony builds all its tasks, then it is
// retired.
type project struct {
	id         int
	name       string
	queuedTick int // w.tick when the project was designated, for job board display
	tasks      []*buildTask
	// issuer pays for the work and owns what is built: the colony for public
	// works, a colonist for a commission (see workorder.go).
	issuer Owner
	// workKind is the kind of work order each task is bought with: WorkBuild
	// (the zero value) for a room, WorkDig for an excavation order.
	workKind WorkKind
}

// taskDone reports whether a task's tile already holds its desired terrain —
// or, for a dig task (terrain Floor), no longer holds what it clears at all.
// A dig and a wall/facility task can target the same position (excavate it,
// then build on it); once the later task converts that floor into a wall or
// facility, the dig task must count as done too, or it can never be satisfied
// again and permanently blocks the project's phase from advancing.
func (w *World) taskDone(t *buildTask) bool {
	if t.terrain == Floor {
		return w.TerrainAt(t.pos) != t.clears
	}
	return w.TerrainAt(t.pos) == t.terrain
}

// taskWorkable reports whether a task can be built right now: not done, and
// its tile holds the right starting terrain — open floor for a build task,
// or what it clears (still-solid rock, or a wall or hull to break down) for a
// dig task (terrain Floor; see roomDigPhase).
func (w *World) taskWorkable(t *buildTask) bool {
	if w.taskDone(t) {
		return false
	}
	if t.terrain == Floor {
		return w.TerrainAt(t.pos) == t.clears
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
			// A colonist never claims what it could not pay for; it mines
			// instead, and the rock is what it builds with next time.
			if builder := w.entities[id]; builder != nil && !w.canAffordBuild(builder, t.terrain, p.issuer) {
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
			// A task can be done without its order being paid: a room's dig
			// tiles are ordinary mining targets too, and a miner that does
			// not hold the task still digs one out. Its order would then sit
			// open forever, its escrow neither paid nor refunded.
			for _, t := range p.tasks {
				if t.order != nil && w.workOrders[t.order.ID] == t.order {
					w.closeWork(t.order)
				}
			}
			w.logEvent(LogBuildComplete, capitalizeFirst(withArticle(p.name))+" is complete.")
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
	maxFac int
	// aisle widens the room by a tile of floor either side of its bay. A
	// one-fixture room is otherwise one tile wide, so exactly one tile can
	// reach its fixture: fine for a pod or a toilet, used in a moment, but a
	// workshop or a shop has a cook working there for long stretches while
	// others need the same depot — to sell, to buy, to fetch a meal they own.
	// With the scumhouse one tile wide, the queue behind a cook starved three
	// tiles from its own food.
	aisle bool
	// aisleRequired refuses the narrow fallback (see planRoomFor): the room
	// waits for a site wide enough for its aisle. Every scumhouse but the
	// colony's first sets it — a one-tile scumhouse whose cook never left
	// its only access tile starved a colonist beside a meal of its own.
	aisleRequired bool
	planLog       string // logged when the room is marked out
}

// roomWidth is the interior width of a room of r with n facilities.
func (r roomRecipe) roomWidth(n int) int {
	if r.aisle {
		return bayWidth(n) + 2
	}
	return bayWidth(n)
}

// bayOffset is how far in from a room's left wall its first facility sits.
func (r roomRecipe) bayOffset() int {
	if r.aisle {
		return 1
	}
	return 0
}

var (
	// lifeSupportRoom alternates pods and toilets so one room serves both the
	// food and bladder needs; a partial room must still serve both.
	lifeSupportRoom = roomRecipe{
		name: "facility room", kinds: []Terrain{NutrientPod, Toilet}, minFac: 2,
		planLog: "The colony marks out a new facility room.",
	}
	// toiletRoom is the facility room with the safety net off: a nutrient pod
	// feeds nobody then (see podsFeed), so the bay is all toilets.
	toiletRoom = roomRecipe{
		name: "facility room", kinds: []Terrain{Toilet}, minFac: 1,
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
		name: "storage room", kinds: []Terrain{Storage}, minFac: 1, maxFac: 1, aisle: true,
		planLog: "The colony marks out a new storage room.",
	}
	// scumhouseRoom is a kitchen laid out as an assembly line: the scumhouse
	// (the stove, whose depot holds the inputs) and, two tiles along, a
	// pantry — a chest the cooked meals go straight into, where they are
	// sold and fetched. Cooking and collecting use different fixtures with
	// their own access tiles, so a cook at the stove never stands between a
	// hungry colonist and its meal. A cramped site may leave room for the
	// stove alone; then meals stay in its depot, as before pantries. The
	// planner wants one per colonists-per-scumhouse when food is not free
	// (see planRooms); otherwise it is player-ordered. It has an aisle (see
	// roomRecipe.aisle), as does a storage room, since the first is the
	// colony's silo. See docs/scumhouse.md.
	scumhouseRoom = roomRecipe{
		name: "scumhouse", kinds: []Terrain{Scumhouse, Storage}, minFac: 1, maxFac: 2, aisle: true,
		planLog: "The colony marks out a scumhouse.",
	}
)

// bayWidth is the row width spanned by n facilities spaced one tile apart.
func bayWidth(n int) int { return 2*n - 1 }

// roomFrontWallY returns the front-wall row for a south-facing room whose
// facility row is y.
func roomFrontWallY(y int) int { return y + roomFrontV }

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
	// Reaching a facility the colony is cut off from comes before any new
	// room, and ahead of the concurrency cap: what is walled off is already
	// built and already counted as capacity, so nothing planned below would
	// replace it.
	if w.planPassage() {
		return
	}
	if len(w.projects) >= w.maxConcurrentProjects() {
		// A full inventory can halt the dig phase of every project already in
		// flight. Permit one storage room beyond the normal concurrency cap to
		// break that circular dependency; no other recipe gets this exception.
		if w.colonyNeedsStorage() {
			w.planRoom(storageRoom)
		}
		return
	}
	// A house waits for the colony's first scumhouse: without the safety net
	// it is life support, planned before any other room, and a commission
	// taking the only construction slot first put food behind somebody's
	// bunk.
	if w.podsFeed() || w.plannedFacilities(Scumhouse) > 0 {
		w.commissionHouses()
		w.commissionKitchens()
	}
	if len(w.projects) >= w.maxConcurrentProjects() {
		return
	}
	if w.manualFacilityRooms > 0 {
		before := len(w.projects)
		w.planRoom(w.facilityRoomRecipe())
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
	if w.manualScumhouses > 0 {
		before := len(w.projects)
		w.planRoom(scumhouseRoom)
		if len(w.projects) > before {
			w.manualScumhouses--
		}
		return
	}
	if w.manualIncubators > 0 {
		before := len(w.projects)
		w.planRoom(incubatorRoom)
		if len(w.projects) > before {
			w.manualIncubators--
		}
		return
	}
	if w.manualFoundries > 0 {
		before := len(w.projects)
		w.planRoom(foundryRoom)
		if len(w.projects) > before {
			w.manualFoundries--
		}
		return
	}
	if w.manualHalls > 0 {
		before := len(w.projects)
		w.planRoom(hallRoom)
		if len(w.projects) > before {
			w.manualHalls--
		}
		return
	}
	// Without the safety net, food has to be made, and the scumhouse is the
	// only place that makes it: it comes before every other room, as life
	// support always has. The meals in the lockers buy the time to build it.
	if !w.podsFeed() && w.plannedColonyKitchens() < w.desiredScumhouses() {
		// Life support does not wait on money: a colony that cannot fund its
		// first scumhouse still marks it out, as unpaid community work, the
		// way colonists always built themselves pods and toilets. Without
		// this a colony founded with no grant starved to a colonist.
		// Only the first comes free of charge, and only the first holds up
		// everything else: later ones are ordinary public works, which wait
		// on the treasury like any other room and let the planner move on
		// when they cannot be placed or paid for.
		first := w.plannedFacilities(Scumhouse) == 0
		r := scumhouseRoom
		r.aisleRequired = !first
		if w.planRoomFor(r, Community) {
			return
		}
		if first {
			w.planRoomFor(scumhouseRoom, Nobody)
			return
		}
	}
	// The incubator feeds the scumhouse: a steady supply of scum that replaces
	// scraping the rock. An ordinary public work, so it waits on the treasury,
	// and until it stands the colony scrapes as before (wildScumAllowed).
	if !w.podsFeed() && w.wantsIncubator() && w.planRoomFor(incubatorRoom, Community) {
		return
	}
	desired := w.desiredFacilities(w.countKind(Colonist))
	if (w.wantsFacility(NutrientPod) && w.plannedFacilities(NutrientPod) < desired) ||
		w.plannedFacilities(Toilet) < desired {
		w.planRoom(w.facilityRoomRecipe())
		return
	}
	// A full inventory stops mining and can also deadlock a room whose active
	// phase consists of dig tasks. Storage therefore outranks non-fatal bunks:
	// make somewhere to unload before asking the same workers to excavate more.
	if w.colonyNeedsStorage() {
		w.planRoom(storageRoom)
		return
	}
	// The colony trades at a communal chest, its silo (see market.go). Crash
	// pods bring every settler a locker, so nothing else ever calls for a
	// shared one: without this, a colony would never have a market at all.
	if _, ok := w.marketDepot(); !ok && w.projectFacilityTasks(Storage) == 0 {
		w.planRoom(storageRoom)
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
		if w.planRoomFor(trashRoom, Community) {
			return
		}
	}
	// A meeting hall after everything above: company is not fatal, and its
	// walls and chairs cost real rock. Unlike the foundry it is a headcount
	// matter (see wantsHall), so it outranks it.
	if w.wantsHall() {
		if w.planRoomFor(hallRoom, Community) {
			return
		}
	}
	// The foundry last of all: nothing about rifles keeps anyone alive, and
	// everything above does. One is enough; its demand is the armory's, not
	// a headcount (see foundry.go).
	if w.wantsFoundry() {
		w.planRoom(foundryRoom)
	}
}

// facilityRoomRecipe is the life-support room the colony builds: pods and
// toilets while pods feed anyone, toilets alone once they do not.
func (w *World) facilityRoomRecipe() roomRecipe {
	if w.wantsFacility(NutrientPod) {
		return lifeSupportRoom
	}
	return toiletRoom
}

// planRoom designates a new room from a recipe at a suitable open site. It
// prefers the recipe's largest bay (roomFacilities, unless it caps itself with
// maxFac) but falls back to fewer facilities when only a shorter clear area is
// available, so progress is made even in a cramped cavern.
func (w *World) planRoom(r roomRecipe) {
	w.planRoomFor(r, Community)
}

// planRoomFor plans a room paid for, and owned, by issuer, reporting whether
// it did. A room whose site is found but whose work issuer cannot pay for is
// not planned: that is how an empty treasury halts public works.
//
// It looks for a backed site (findRoomSite) at every bay size before it
// settles for a free-standing one (findFreeStandingSite): a big room standing
// out on open floor is not worth more than a smaller one tucked against the
// rock, which costs nobody a route round it.
func (w *World) planRoomFor(r roomRecipe, issuer Owner) bool {
	if planned, sited := w.placeRoom(r, issuer, w.findRoomSite); sited {
		return planned
	}
	planned, _ := w.placeRoom(r, issuer, w.findFreeStandingSite)
	return planned
}

// placeRoom plans r at the first site find offers, trying the largest bay
// first, and reports whether it planned the room and whether find offered a
// site at all (a site whose work issuer cannot pay for is sited, not planned).
func (w *World) placeRoom(r roomRecipe, issuer Owner, find func(width int) (roomFrame, bool)) (planned, sited bool) {
	largest := r.maxFac
	if largest <= 0 || largest > roomFacilities {
		largest = roomFacilities
	}
	for n := largest; n >= r.minFac; n-- {
		f, ok := find(r.roomWidth(n))
		if !ok && r.aisle && !r.aisleRequired {
			// A cramped cavern with no site wide enough for the aisle still
			// gets the room, narrow: a scumhouse one tile can reach beats
			// none at all.
			narrow := r
			narrow.aisle = false
			if f, ok = find(narrow.roomWidth(n)); ok {
				return w.designateRoom(narrow, f, n, issuer), true
			}
		}
		if !ok {
			continue // no site this wide; try a smaller room
		}
		return w.designateRoom(r, f, n, issuer), true
	}
	// No site large enough for even this recipe's minimum yet; colonists dig
	// on and planning retries later.
	return false, false
}

// designateRoom adds a phased room project from a recipe, laid out in frame f
// (whose width it sets from the recipe). Any interior tile still solid rock
// is dug first (roomDigPhase); its complete perimeter is then built, except
// for the centered front doorway; then n facilities are built one tile inside
// the back wall, drawn from the recipe's kinds in order.
func (w *World) designateRoom(r roomRecipe, f roomFrame, n int, issuer Owner) bool {
	p := &project{id: w.nextProjectID, name: r.name, queuedTick: w.tick, issuer: issuer}
	f.width = r.roomWidth(n)
	for v := roomBackV; v <= roomFrontV; v++ {
		for u := 0; u < f.width; u++ {
			if pos := f.at(u, v); w.TerrainAt(pos) == Rock {
				p.tasks = append(p.tasks, &buildTask{pos: pos, terrain: Floor, phase: roomDigPhase})
			}
		}
	}
	for v := roomBackV; v <= roomFrontV; v++ {
		// A side tile that is already a wall is a party wall shared with a
		// neighboring room (see roomSiteClear): this room needs no task of its
		// own there.
		if left := f.at(-1, v); w.TerrainAt(left) != Wall {
			p.tasks = append(p.tasks, &buildTask{pos: left, terrain: Wall, phase: roomWallPhase})
		}
		if right := f.at(f.width, v); w.TerrainAt(right) != Wall {
			p.tasks = append(p.tasks, &buildTask{pos: right, terrain: Wall, phase: roomWallPhase})
		}
	}
	doorU := f.doorU()
	for u := 0; u < f.width; u++ {
		p.tasks = append(p.tasks,
			&buildTask{pos: f.at(u, roomBackV), terrain: Wall, phase: roomWallPhase})
		if u != doorU {
			p.tasks = append(p.tasks,
				&buildTask{pos: f.at(u, roomFrontV), terrain: Wall, phase: roomWallPhase})
		}
	}
	for i, du := 0, 0; i < n; i, du = i+1, du+2 {
		p.tasks = append(p.tasks,
			&buildTask{pos: f.at(r.bayOffset()+du, 0), terrain: r.kinds[i%len(r.kinds)], phase: roomFitPhase})
	}
	for _, t := range p.tasks {
		t.proj = p
	}
	// The room is bought before anything about it becomes permanent: an
	// issuer that cannot pay for all its work gets nothing marked out.
	if !w.fundProject(p) {
		return false
	}
	w.nextProjectID++
	// Reserve the tile directly outside the door, permanently: without this,
	// nothing stops a later room from sitting its own wall or facility row
	// right on top of it once the colony has grown enough to prefer that
	// spot, sealing this room's only way out behind a wall its own doorway
	// invariant never anticipated. See roomSiteClear.
	w.doorTiles[f.doorStep()] = true
	w.projects = append(w.projects, p)
	if r.name == scumhouseRoom.name {
		w.linkPantry(p)
	}
	w.logEvent(LogBuildStart, r.planLog)
	return true
}

// findRoomSite returns a frame for a width-wide room in a niche at the cavern
// edge, its doorway facing whichever way puts it nearest the map center.
// Solid rock — or another room's already-placed wall — beyond the placed back
// wall keeps the room from becoming a free-standing obstacle across an open
// route; backing onto a neighbor's wall lets rooms sit flush against each
// other, sharing that boundary instead of each needing its own untouched rock
// vein.
//
// It tries a fully pre-cleared site first, and only falls back to one whose
// interior still needs excavating (see roomSiteClear, designateRoom) if no
// clear site exists at all. A clear site is strictly faster to finish — no
// dig phase — so preferring it, rather than just picking whichever candidate
// is nearest map center, keeps a life-support room from landing somewhere
// slower to complete than it needed to be merely because that spot happened
// to be a little closer to center; that measurably delayed food in testing.
func (w *World) findRoomSite(width int) (roomFrame, bool) {
	if site, ok := w.findRoomSiteWith(width, siteRules{}); ok {
		return site, true
	}
	return w.findRoomSiteWith(width, siteRules{allowRock: true})
}

// findFreeStandingSite is findRoomSite without the backing: whatever lies
// behind the back wall. It is planRoomFor's last resort. The colony mines for
// rock as well as for space, so a small cavern can be mined bare; with no
// rock or wall left to back onto, every backed site is gone for good and
// digging can never make one. The meeting hall, planned after everything
// else and wider than most, was the room that hit it first: wanted, and never
// planned again. Standing free is safe only because no site is accepted that
// would cut the colony in two (siteKeepsColonyWhole).
func (w *World) findFreeStandingSite(width int) (roomFrame, bool) {
	if site, ok := w.findRoomSiteWith(width, siteRules{unbacked: true}); ok {
		return site, true
	}
	return w.findRoomSiteWith(width, siteRules{allowRock: true, unbacked: true})
}

// siteRules says what findRoomSiteWith and roomSiteClear accept.
type siteRules struct {
	// allowRock lets the interior still be solid rock, dug out first.
	allowRock bool
	// unbacked drops the requirement for rock or a wall behind the back
	// wall: the room may stand free on open floor.
	unbacked bool
}

// roomSearchStartRadius is the first box half-width findRoomSiteWith
// searches around the map center, in tiles.
const roomSearchStartRadius = 64

// roomSearchMaxRadius bounds how far findRoomSiteWith's box search
// grows around the map center when nothing has been carved yet (so
// carvedSearchRadius has no box to measure). Comfortably covers a starting
// chamber without ever approaching a huge map's full extent.
const roomSearchMaxRadius = roomSearchStartRadius * 4

// carvedSearchRadius returns the largest radius findRoomSiteWith could
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

// siteCandidate is a site that passed roomSiteClear, with its distance from
// the map center and its facing's place in roomFacings (rank), for breaking
// ties.
type siteCandidate struct {
	f    roomFrame
	dist int
	rank int
}

// compareSiteCandidates orders sites nearest the map center first, then by
// row-major anchor, then by roomFacings' order.
func compareSiteCandidates(a, b siteCandidate) int {
	aa, ba := a.f.anchor(), b.f.anchor()
	return cmp.Or(
		cmp.Compare(a.dist, b.dist),
		cmp.Compare(aa.Y, ba.Y),
		cmp.Compare(aa.X, ba.X),
		cmp.Compare(a.rank, b.rank),
	)
}

// findRoomSiteWith returns the site nearest the map center that rules allow
// and that keeps the colony whole. A site's distance is its facility row's
// middle tile (roomFrame.anchor) from the center, whichever way it faces;
// ties go to the row-major first anchor, then to roomFacings' order.
func (w *World) findRoomSiteWith(width int, rules siteRules) (roomFrame, bool) {
	designated := make(map[Point]bool)
	for _, p := range w.projects {
		for _, t := range p.tasks {
			designated[t.pos] = true
		}
	}
	center := Point{w.Width / 2, w.Height / 2}

	// A usable site needs an already-cleared floor lane beside it (see
	// roomSiteClear), so sites cluster near the existing colony rather than
	// scattering across untouched rock — and a colony starts at the map
	// center. Search outward in growing boxes of anchors instead of scanning
	// the whole grid: once a box comes up empty, any site a larger box finds
	// is guaranteed to be the true nearest overall (nothing closer was
	// skipped), so this returns exactly what a full scan would, but pays only
	// for the area actually searched — flat cost near the colony instead of
	// O(map area) on a huge, mostly empty map. maxRadius caps that growth at
	// the carved area's own extent, so the "no site fits yet" case — the
	// common one — also stays cheap instead of expanding to the full map.
	//
	// The split check (siteKeepsColonyWhole) is a flood fill, far dearer than
	// roomSiteClear, so it runs only on candidates in order of distance and
	// stops at the first that passes.
	maxRadius := w.carvedSearchRadius(center, width)
	var cands []siteCandidate
	for radius := min(roomSearchStartRadius, maxRadius); ; radius = min(radius*2, maxRadius) {
		x0, x1 := max(0, center.X-radius), min(w.Width-1, center.X+radius)
		y0, y1 := max(0, center.Y-radius), min(w.Height-1, center.Y+radius)
		// Only anchors near carved ground can pass: every site's side walls
		// are already Floor or Wall (roomSiteClear), and the anchor is never
		// more than width+2 from one, whichever way the room faces. Skipping
		// the rest changes no answer, and in a young colony is most of the
		// box: four facings and two more passes for free-standing sites made
		// a search that found nothing five times dearer before this.
		ax0, ax1, ay0, ay1 := x0, x1, y0, y1
		if w.carvedAny {
			m := width + 2
			ax0, ax1 = max(ax0, w.carvedMin.X-m), min(ax1, w.carvedMax.X+m)
			ay0, ay1 = max(ay0, w.carvedMin.Y-m), min(ay1, w.carvedMax.Y+m)
		}

		cands = cands[:0]
		for rank, face := range roomFacings {
			cands = w.appendRoomSites(cands, face, rank, width, Point{ax0, ay0}, Point{ax1, ay1}, center, designated, rules)
		}
		slices.SortFunc(cands, compareSiteCandidates)
		for _, c := range cands {
			if w.siteKeepsColonyWhole(c.f, designated) {
				return c.f, true
			}
		}
		atCap := radius >= maxRadius
		fullDomain := x0 == 0 && y0 == 0 && x1 == w.Width-1 && y1 == w.Height-1
		if atCap || fullDomain {
			return roomFrame{}, false // no site can exist any further out
		}
	}
}

// appendRoomSites appends every site facing face, with its anchor in the box
// lo..hi, that passes roomSiteClear.
//
// Every site needs walkable ground the whole way along its approach row, the
// row outside its front wall from lane to lane (u -1..width; see
// roomSiteClear's last loop). So the scan runs along that row: south- and
// north-facing rooms row by row, east- and west-facing ones column by column.
// A tile in it that is not walkable rules out every anchor whose row covers
// it, so the scan jumps past them all instead of testing each. Nothing it
// skips could have passed, so the sites found are the same.
//
// Nearly all of a search box is solid rock, which passes roomSiteClear's first
// tests, so this is what keeps a search that finds nothing cheap. See
// docs/construction.md, "Search cost".
func (w *World) appendRoomSites(cands []siteCandidate, face roomFacing, rank, width int, lo, hi, center Point, designated map[Point]bool, rules siteRules) []siteCandidate {
	half := width / 2
	ahead := roomFrontV + roomApproach // how far in front of the anchor the approach row lies
	try := func(anchor Point) {
		if f := frameAt(anchor, face, width); w.roomSiteClear(f, designated, rules) {
			cands = append(cands, siteCandidate{f, center.Chebyshev(anchor), rank})
		}
	}
	switch face {
	case faceEast, faceWest:
		for x := lo.X; x <= hi.X; x++ {
			ax := x + ahead
			if face == faceWest {
				ax = x - ahead
			}
			for y := lo.Y; y <= hi.Y; y++ {
				// The approach column spans y-half-1..y-half+width.
				if b, ok := w.lastUnwalkableInColumn(ax, y-half-1, y-half+width); ok {
					y = b + half + 1 // the loop's y++ lands on the first anchor clear of b
					continue
				}
				try(Point{x, y})
			}
		}
	default:
		for y := lo.Y; y <= hi.Y; y++ {
			ay := y + ahead
			if face == faceNorth {
				ay = y - ahead
			}
			for x := lo.X; x <= hi.X; x++ {
				// The approach row spans x-half-1..x-half+width.
				if b, ok := w.lastUnwalkableInRow(ay, x-half-1, x-half+width); ok {
					x = b + half + 1 // the loop's x++ lands on the first anchor clear of b
					continue
				}
				try(Point{x, y})
			}
		}
	}
	return cands
}

// lastUnwalkableInRow returns the greatest x in [x0, x1] where (x, y) is not
// walkable, if there is one: the blocker that rules out the most anchors (see
// appendRoomSites).
func (w *World) lastUnwalkableInRow(y, x0, x1 int) (int, bool) {
	for x := x1; x >= x0; x-- {
		if !w.Walkable(Point{x, y}) {
			return x, true
		}
	}
	return 0, false
}

// lastUnwalkableInColumn is lastUnwalkableInRow down column x.
func (w *World) lastUnwalkableInColumn(x, y0, y1 int) (int, bool) {
	for y := y1; y >= y0; y-- {
		if !w.Walkable(Point{x, y}) {
			return y, true
		}
	}
	return 0, false
}

// roomSiteClear reports whether a room in frame f is buildable. The interior
// (where this room's own back/front walls and facilities go) must be clear
// floor — or, when rules.allowRock is set, may also be still-solid rock, which
// a dig task excavates before the wall phase starts (see designateRoom) — as
// long as it is unclaimed by another project; unless rules.unbacked, the
// placed rear wall is backed by solid rock or another room's wall; and each
// side wall is either freshly built (with a connected exterior lane keeping it
// reachable) or reused outright from an already-placed, unclaimed neighboring
// wall — the two rooms then sit flush, sharing that one tile as a party wall
// instead of each building its own. The exterior lanes and front approach
// (never dug) always touch the interior's front row, so when allowRock
// permits solid rock there is always at least one already-reachable tile to
// start digging from. Everything from the lanes to the row behind the back
// wall must lie on the map.
//
// The interior and side-wall checks also reject any tile in w.doorTiles: the
// permanently-reserved exit tile of an existing room's doorway. Without this,
// a room sited to reuse rock or wall backing near the colony's center — the
// very spot an older room's door faces — could build a wall or facility
// straight over that tile and seal the older room shut.
//
// This is the cheap, local half of siting. Whether the room would cut the
// colony in two is siteKeepsColonyWhole's question.
//
// Terrain reads come before the designated and doorTiles lookups, which hash
// a Point each. The checks are all pure, so their order changes only the cost.
func (w *World) roomSiteClear(f roomFrame, designated map[Point]bool, rules siteRules) bool {
	if !w.InBounds(f.at(-2, roomBackV-1)) || !w.InBounds(f.at(f.width+1, roomFrontV+roomApproach)) {
		return false
	}
	if !rules.unbacked {
		for u := 0; u < f.width; u++ {
			if t := w.TerrainAt(f.at(u, roomBackV-1)); t != Rock && t != Wall {
				return false
			}
		}
	}
	for v := roomBackV; v <= roomFrontV; v++ {
		for u := 0; u < f.width; u++ {
			p := f.at(u, v)
			t := w.TerrainAt(p)
			if (t != Floor && !(rules.allowRock && t == Rock)) || designated[p] || w.doorTiles[p] {
				return false
			}
			if t == Floor && !w.discovered(p) {
				return false // an undiscovered cavern: see docs/caverns.md
			}
		}
		// Each side wall is clear floor (this room will build its own wall
		// there, so keep an exterior lane beside it reachable even after its
		// neighbors have been raised) or an unclaimed wall already placed by
		// another room (shared outright: nothing more is needed on that side).
		for _, side := range [2]struct{ wall, lane int }{{-1, -2}, {f.width, f.width + 1}} {
			p := f.at(side.wall, v)
			switch w.TerrainAt(p) {
			case Floor:
				lane := f.at(side.lane, v)
				if !w.discovered(p) || !w.Walkable(lane) || !w.discovered(lane) || designated[lane] {
					return false
				}
			case Wall:
				// Shared party wall: this room needs nothing beyond it.
			default:
				return false
			}
			if designated[p] || w.doorTiles[p] {
				return false
			}
		}
	}
	// Connect both exterior side lanes in front of the room, for whichever
	// sides are freshly built — a shared side has no lane to connect. The room's
	// own front row (from side wall to side wall) is always required either
	// way, so builders can reach the door and front wall tasks.
	loU, hiU := -1, f.width
	if w.TerrainAt(f.at(-1, roomFrontV)) != Wall {
		loU = -2
	}
	if w.TerrainAt(f.at(f.width, roomFrontV)) != Wall {
		hiU = f.width + 1
	}
	for u := loU; u <= hiU; u++ {
		p := f.at(u, roomFrontV+roomApproach)
		if !w.Walkable(p) || !w.discovered(p) || designated[p] {
			return false
		}
	}
	return true
}

// desiredScumhouses is how many scumhouses the colony wants with scarcity on:
// one per colonists-per-scumhouse colonists, and always at least one, and one
// more whenever its kitchens are behind (kitchensBehind) and every one it
// planned is built, up to one per three colonists. One cook works a scumhouse
// at a time, so a growing colony that kept one kitchen starved beside a pile
// of uncooked scum. A kitchen per ten colonists was a guess at what a colony
// needs; kitchens that are behind are a measurement of it.
//
// A chef's own kitchen is not the colony's: the colony can't cook there or buy
// scum there, so it's counted neither built nor planned. When a chef's kitchen
// was counted, every one a chef bought was one the colony stopped building.
func (w *World) desiredScumhouses() int {
	per := max(1, w.cfg.ColonistsPerScumhouse)
	n := w.countKind(Colonist)
	want := max(1, (n+per-1)/per)
	built := len(w.colonyKitchens())
	if built >= want && built < max(1, n/3) && w.plannedColonyKitchens() == built && w.kitchensBehind() {
		want = built + 1
	}
	return want
}

// plannedColonyKitchens is plannedFacilities(Scumhouse) less the kitchens
// living chefs have bought or are having built.
func (w *World) plannedColonyKitchens() int {
	chefs := 0
	for _, e := range w.entities {
		if e.Kind == Colonist && e.Alive() && e.kitchenCommissioned {
			chefs++
		}
	}
	return max(0, w.plannedFacilities(Scumhouse)-chefs)
}

// kitchensBehind reports whether cooking is what holds the colony's food
// back: it's short of its meal reserve while its scumhouses hold, on average,
// half their stock cap of biomatter waiting to be cooked.
func (w *World) kitchensBehind() bool {
	if !w.foodWanted() {
		return false
	}
	houses := w.colonyKitchens()
	if len(houses) == 0 {
		return false
	}
	waiting := 0
	for _, p := range houses {
		if c := w.storageContainers[p]; c != nil {
			for _, k := range biomatterKinds {
				waiting += c.held(Community, k)
			}
		}
	}
	return waiting >= len(houses)*max(1, w.cfg.ScumhouseStockCap/2)
}

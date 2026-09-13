package sim

import (
	"fmt"
	"sort"
)

// step advances the world by one tick. Colonists act before other kinds (as their
// spawn IDs already arrange in generated worlds), with the hungriest acting first
// so a fixed low ID cannot repeatedly win newly opened facility access. Ties and
// all non-colonists remain in ascending ID order, preserving deterministic runs.
// The dead are removed the moment they are eaten or starve, so we re-check
// liveness as we go.
func (w *World) step() {
	w.tick++
	for _, id := range w.entityTurnOrder() {
		e := w.entities[id]
		if e == nil || !e.Alive() {
			continue
		}
		switch e.Kind {
		case Colonist:
			w.colonistTurn(e)
		case Alien:
			w.alienTurn(e)
		case Cat:
			w.catTurn(e)
		case Mouse:
			w.mouseTurn(e)
		}
	}
	w.refreshSpatial() // fold in any digging/building from this tick
	w.pruneProjects()
	if w.tick >= w.nextPlanTick {
		w.planFacilities()
		w.nextPlanTick = w.tick + planInterval
	}
	w.rebuildBuildTiles() // reflect this tick's completions and any new project
}

// planInterval is how often the colony re-plans construction, in ticks. Facility
// needs are slow, so a coarse cadence keeps planning cheap.
const planInterval = 16

// entityTurnOrder returns the deterministic per-tick action order. Fatal need
// urgency is a scheduling concern as well as a job-selection concern: in a full
// facility room, acting first gives a colonist first claim on access space that
// another colonist vacated on the previous tick.
func (w *World) entityTurnOrder() []EntityID {
	ids := w.entityIDsSorted()
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := w.entities[ids[i]], w.entities[ids[j]]
		if a.Kind != b.Kind {
			return a.Kind == Colonist
		}
		if a.Kind == Colonist {
			ah, bh := w.needLevel(a, NeedFood), w.needLevel(b, NeedFood)
			if ah != bh {
				return ah > bh
			}
		}
		return a.ID < b.ID
	})
	return ids
}

// entityIDsSorted returns current entity IDs in ascending order.
func (w *World) entityIDsSorted() []EntityID {
	ids := make([]EntityID, 0, len(w.entities))
	for id := range w.entities {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// ---- Colonists ---------------------------------------------------------------

func (w *World) colonistTurn(e *Entity) {
	w.applyStarvation(e)
	if !e.Alive() { // starved this tick
		w.clearJob(e) // release any board claim before removal
		w.remove(e.ID)
		w.log.add(fmt.Sprintf("Colonist #%d starved to death.", e.ID))
		return
	}

	// Survival comes first: if an alien is close, drop everything and run.
	if threat, ok := w.nearestAlien(e.Pos, w.cfg.FleeRadius); ok {
		w.clearJob(e)
		e.resting = false
		e.State = Fleeing
		w.fleeStep(e, threat.Pos)
		return
	}

	// A need at its threshold preempts the current task — the colonist stays on
	// task until either the task finishes (below) or a need crosses, whichever
	// comes first. Head to the facility if one is reachable.
	need, urgent := w.mostUrgentNeed(e)
	handlingNeed := (e.Job == JobUse && e.Need == need) ||
		(e.Job == JobBuild && (e.BuildKind == w.cfg.Needs[need].Facility || e.task != nil))
	if urgent && !handlingNeed {
		spec := w.cfg.Needs[need]
		e.resting = false
		if field := w.facilityField(spec.Facility); field != nil && field.at(e.Pos) >= 0 {
			// A facility of this kind is reachable: follow its shared flow field.
			w.clearJob(e)
			e.Job, e.Need, e.Progress = JobUse, need, 0
		} else {
			// No completed facility is reachable. Drop unrelated work and help with
			// reachable planned construction rather than mining until death merely
			// because a facility task exists somewhere in the world.
			w.clearJob(e)
			if task, ok := w.claimNearestTask(e.Pos, e.ID); ok {
				w.assignTask(e, task)
			} else if spec.Fatal && !w.reachableFacilityConstruction(e.Pos, spec.Facility) {
				// A project in a disconnected room must not suppress this fallback.
				if spot, ok := w.findBuildSpot(e.Pos, 20); ok {
					w.assignBuild(e, spec.Facility, spot)
				}
			} else {
				// All reachable project tasks are claimed. Wait for their builders
				// instead of taking unrelated work and losing our place in the queue.
				e.State = Idle
				if w.idleWouldBlock(e.Pos) {
					w.stepAside(e)
				} else {
					w.wanderStep(e)
				}
				return
			}
		}
	}

	// Stay on the current task: movement and work progress happen here every tick
	// until the job completes or is abandoned.
	if e.Job != JobNone {
		w.runJob(e)
		return
	}

	// Idle. If there was no work last time we looked and no need is pressing,
	// rest (skip the work search) until wakeTick. This keeps an established colony
	// with nothing available from re-scanning the map every tick.
	if !urgent && e.resting && w.tick < e.wakeTick {
		e.State = Idle
		return
	}
	w.assignWorkJob(e)
	if e.Job == JobNone {
		if w.idleWouldBlock(e.Pos) {
			// Never idle where resting would block others. A satisfied colonist
			// parked on a facility's access tile (often right where it just ate)
			// keeps starving colonists from reaching it; one parked on a pending
			// build tile keeps that structure from ever being built, stalling the
			// whole project. Either way, step aside and re-check next tick rather
			// than freezing here.
			e.resting = false
			e.State = Idle
			w.stepAside(e)
			return
		}
		e.resting = true
		e.wakeTick = w.tick + e.restTicks
		e.State = Idle
		return
	}
	e.resting = false
	w.runJob(e)
}

// idleWouldBlock reports whether an idle colonist resting at p would get in the
// colony's way: p is a facility's access tile (blocking users) or a pending
// build tile (blocking construction).
func (w *World) idleWouldBlock(p Point) bool {
	return w.onFacilityAccess(p) || w.onPendingBuild(p)
}

// onFacilityAccess reports whether p is next to any need-satisfying facility, so
// an idle colonist standing there would block others from using it.
func (w *World) onFacilityAccess(p Point) bool {
	for _, d := range neighbors8 {
		t := w.TerrainAt(p.Add(d.X, d.Y))
		for i := 0; i < int(numNeeds); i++ {
			if w.cfg.Needs[i].Facility == t {
				return true
			}
		}
	}
	return false
}

// onPendingBuild reports whether p is a not-yet-built task tile of some project,
// which a builder must find clear to construct.
func (w *World) onPendingBuild(p Point) bool {
	return w.buildTiles[p]
}

// assignMine / assignBuild set a colonist's job and update the board's
// bookkeeping (claim the mine tile / count the in-progress build). clearJob
// reverses whichever bookkeeping the current job holds and returns the colonist
// to idle. Routing every job start and end through these keeps the board's claim
// set and build counts exact.
func (w *World) assignMine(e *Entity) {
	e.Job, e.Progress, e.mineClaimed = JobMine, 0, false
}

// assignMineTarget commits a colonist to a specific rock claimed up front (the
// A* mining path, used for small colonies), to be reached via travelTo.
func (w *World) assignMineTarget(e *Entity, target Point) {
	e.Job, e.Target, e.mineClaimed, e.Progress = JobMine, target, true, 0
}

// useFrontierMining reports whether miners should follow the shared frontier
// flow field (worth it for big colonies / big maps) rather than each running
// cached A* to a claimed tile. It is checked dynamically so a growing colony
// switches over on its own.
func (w *World) useFrontierMining() bool {
	return w.countKind(Colonist) >= w.cfg.FrontierFieldMinColonists ||
		w.Width*w.Height >= w.cfg.FrontierFieldMinArea
}

// claimNearestMine claims (for id) the nearest unclaimed frontier rock reachable
// from the colonist's room, for the A* mining path. Returns the claimed tile.
func (w *World) claimNearestMine(from Point, id EntityID) (Point, bool) {
	room := w.roomOf(from)
	if room == 0 {
		return Point{}, false
	}
	var best Point
	found := false
	bestDist := 1 << 30
	for p := range w.board.frontier {
		if w.board.isClaimed(p) || !w.frontierReachable(p, room) {
			continue
		}
		if d := from.Chebyshev(p); !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	if found {
		w.board.claimMine(best, id)
	}
	return best, found
}

// frontierReachable reports whether a frontier rock has a floor neighbor in room.
func (w *World) frontierReachable(rock Point, room RoomID) bool {
	for _, d := range neighbors8 {
		n := rock.Add(d.X, d.Y)
		if w.Walkable(n) && w.roomOf(n) == room {
			return true
		}
	}
	return false
}

// lessPoint gives a stable row-major ordering for deterministic tie-breaks.
func lessPoint(a, b Point) bool {
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.X < b.X
}

// claimAdjacentFrontier claims (for e) the first unclaimed frontier rock next to
// the colonist, in fixed neighbor order for determinism.
func (w *World) claimAdjacentFrontier(e *Entity) (Point, bool) {
	for _, d := range neighbors8 {
		n := e.Pos.Add(d.X, d.Y)
		if w.board.isFrontier(n) && !w.board.isClaimed(n) {
			w.board.claimMine(n, e.ID)
			return n, true
		}
	}
	return Point{}, false
}

// assignBuild commits a colonist to a lone, ad-hoc build (the emergency
// facility fallback), tracked by the board's in-progress counter.
func (w *World) assignBuild(e *Entity, kind Terrain, target Point) {
	e.Job, e.BuildKind, e.Target, e.Progress = JobBuild, kind, target, 0
	w.board.startBuild(kind)
}

// assignTask commits a colonist to a claimed construction-project task. The task
// is already marked owned by claimNearestTask; e.task links back to it so
// completion and abandonment can release it.
func (w *World) assignTask(e *Entity, t *buildTask) {
	e.Job, e.BuildKind, e.Target, e.Progress = JobBuild, t.terrain, t.pos, 0
	e.task = t
}

func (w *World) clearJob(e *Entity) {
	switch e.Job {
	case JobMine:
		if e.mineClaimed {
			w.board.releaseMine(e.Target, e.ID)
			e.mineClaimed = false
		}
	case JobBuild:
		if e.task != nil {
			e.task.owner = 0 // release the project task for someone else
			e.task = nil
		} else {
			w.board.endBuild(e.BuildKind) // lone emergency build
		}
	}
	e.Job, e.Progress = JobNone, 0
	e.clearPath()
}

// runJob executes the colonist's current job for one tick.
func (w *World) runJob(e *Entity) {
	switch e.Job {
	case JobMine:
		w.jobMine(e)
	case JobBuild:
		w.jobBuild(e)
	case JobUse:
		w.jobUse(e)
	default:
		e.State = Idle
		w.wanderStep(e)
	}
}

// assignWorkJob picks something productive to do: help build a planned project
// first (life-support rooms), otherwise mine the frontier. Leaves JobNone if
// nothing suitable is reachable.
func (w *World) assignWorkJob(e *Entity) {
	// Collaborate on planned construction (facility rooms, etc.): claim the
	// nearest reachable task from the shared project pool.
	if task, ok := w.claimNearestTask(e.Pos, e.ID); ok {
		w.assignTask(e, task)
		return
	}
	// Mining: big colonies/maps follow the shared frontier field (claim on
	// arrival); small ones use cached A* to the nearest claimed tile. Either way
	// only take a job when unclaimed frontier remains.
	if w.board.unclaimedCount() > 0 {
		// Mining produces raw rock. Do not begin work that cannot yield its
		// resource; construction and needs remain available to a full colonist.
		if !e.Inventory.CanAdd(RawRock, 1) {
			e.Job = JobNone
			return
		}
		if w.useFrontierMining() {
			if w.frontierField().at(e.Pos) >= 0 {
				w.assignMine(e)
				return
			}
		} else if target, ok := w.claimNearestMine(e.Pos, e.ID); ok {
			w.assignMineTarget(e, target)
			return
		}
	}
	e.Job = JobNone
}

// plannedFacilities counts a facility kind that already exists plus those a
// colonist is currently building, so the colony converges on the desired number
// instead of every idle colonist starting one at the same instant. The
// in-progress count comes from the board's O(1) counter, not an entity scan.
func (w *World) plannedFacilities(kind Terrain) int {
	return w.countTerrain(kind) + w.board.inProgress(kind) + w.projectFacilityTasks(kind)
}

// desiredFacilities is how many of each life-support structure the colony wants
// for a given headcount (at least one).
func (w *World) desiredFacilities(colonists int) int {
	d := colonists / w.cfg.ColonistsPerFacility
	if d < 1 {
		d = 1
	}
	return d
}

func (w *World) jobMine(e *Entity) {
	// Committed to a specific rock (A* mining claimed it up front, or the field
	// path claimed it on arrival).
	if e.mineClaimed {
		if w.TerrainAt(e.Target) != Rock { // mined out from under us
			w.clearJob(e)
			return
		}
		if e.Pos.Adjacent(e.Target) {
			e.State = Mining
			e.Progress++
			if e.Progress >= scaleTicks(w.cfg.MineTicks, e.workScale) {
				// Award the resource before changing terrain so a full inventory
				// can never make mined material disappear.
				if !e.Inventory.Add(RawRock, 1) {
					w.clearJob(e)
					return
				}
				w.SetTerrain(e.Target, Floor) // TileChanged drops it from the frontier
				w.clearJob(e)
			}
			return
		}
		// A* path: travel to the distant claimed tile.
		if _, ok := w.travelTo(e, e.Target); !ok {
			w.clearJob(e)
			return
		}
		e.State = Moving
		return
	}

	// Field path: follow the shared frontier field and claim a rock on arrival.
	field := w.frontierField()
	if field.at(e.Pos) < 0 { // no reachable unclaimed frontier left
		w.clearJob(e)
		return
	}
	if rock, ok := w.claimAdjacentFrontier(e); ok {
		e.Target, e.mineClaimed, e.Progress, e.State = rock, true, 0, Mining
		return
	}
	if !w.followField(e, field) {
		e.stuck++
		if e.stuck > w.cfg.StuckLimit {
			w.clearJob(e)
		}
		return
	}
	e.stuck, e.State = 0, Moving
}

func (w *World) jobBuild(e *Entity) {
	if w.TerrainAt(e.Target) != Floor { // already built, or no longer valid
		w.clearJob(e)
		return
	}
	arrived, ok := w.travelTo(e, e.Target)
	if !ok {
		w.clearJob(e)
		return
	}
	if !arrived {
		e.State = Moving
		return
	}
	if w.occupiedByOther(e.Target, e.ID) {
		// Someone is on the build tile. Colonists route around pending build
		// tiles, so this clears quickly; wait a few ticks rather than abandoning
		// the job outright, and give up only if it stays blocked.
		e.State = Building
		e.stuck++
		if e.stuck > w.cfg.StuckLimit {
			w.clearJob(e)
		}
		return
	}
	e.stuck = 0
	e.State = Building
	e.Progress++
	if e.Progress >= scaleTicks(w.buildTicks(e.BuildKind), e.workScale) {
		w.SetTerrain(e.Target, e.BuildKind)
		w.noteBuild(e.BuildKind)
		w.clearJob(e) // endBuild decrements the in-progress counter
	}
}

func (w *World) jobUse(e *Entity) {
	spec := w.cfg.Needs[e.Need]
	field := w.facilityField(spec.Facility)
	if field == nil || field.at(e.Pos) < 0 {
		w.clearJob(e) // no facility of this kind is reachable anymore
		return
	}
	// Arrived: standing next to a facility of the right kind — use it.
	if fac, ok := w.adjacentFacility(e.Pos, spec.Facility); ok {
		e.Target = fac
		e.State = useState(e.Need)
		e.Progress++
		if e.Progress >= spec.UseTicks {
			w.resetNeed(e, e.Need)
			w.clearJob(e)
		}
		return
	}
	// Otherwise follow the shared flow field one step toward the nearest one.
	if !w.followField(e, field) {
		e.stuck++
		if e.stuck > w.cfg.StuckLimit {
			w.clearJob(e)
		}
		return
	}
	e.stuck = 0
	e.State = Moving
}

// buildTicks is how long a given structure takes to raise.
func (w *World) buildTicks(kind Terrain) int {
	if kind == Wall {
		return w.cfg.BuildTicks
	}
	return w.cfg.FacilityBuildTicks
}

// noteBuild logs the completion of notable structures.
func (w *World) noteBuild(kind Terrain) {
	switch kind {
	case NutrientPod:
		w.log.add("A nutrient pod comes online.")
	case Toilet:
		w.log.add("A latrine is installed.")
	}
}

// travelTo advances a colonist one step along a cached A* route toward a tile
// adjacent to target, computing (or recomputing) the route as needed. It returns
// arrived (now adjacent to target) and ok (false => give up: the target is
// unreachable, or the colonist has been wedged too long).
func (w *World) travelTo(e *Entity, target Point) (arrived, ok bool) {
	if e.Pos.Adjacent(target) {
		e.clearPath()
		return true, true
	}
	// (Re)plan when we have no route, it was for a different goal, or it ran out
	// without arriving.
	if len(e.path) == 0 || e.pathGoal != target || e.pathAt >= len(e.path) {
		route, found := w.pathToAdjacent(e.Pos, target)
		if !found {
			e.clearPath()
			return false, false
		}
		e.path, e.pathAt, e.pathGoal, e.stuck = route, 0, target, 0
	}
	// A colonist may pass through other colonists on its route, but it must end
	// the tick on a free tile. Scan the occupied prefix and land on the first
	// available route cell. Non-colonists still block movement.
	landing := e.pathAt
	for landing < len(e.path) {
		next := e.path[landing]
		if !w.Walkable(next) { // terrain changed under the route; replan next tick
			e.clearPath()
			return false, true
		}
		blocker := w.entityAt(next)
		if blocker == nil || blocker.ID == e.ID {
			break
		}
		if blocker.Kind != Colonist {
			landing = len(e.path)
			break
		}
		landing++
	}
	if landing == len(e.path) {
		e.stuck++
		if e.stuck > w.cfg.StuckLimit {
			e.clearPath()
			return false, false
		}
		return false, true
	}
	w.moveEntity(e, e.path[landing])
	e.pathAt = landing + 1
	e.stuck = 0
	return e.Pos.Adjacent(target), true
}

// findBuildSpot returns the nearest open Floor tile that sits against Rock or
// Wall — an edge where new structure extends the colony rather than plugging a
// walkway at random. The colonist's own tile is excluded.
func (w *World) findBuildSpot(from Point, radius int) (Point, bool) {
	var best Point
	found := false
	w.forEachInRadius(from, radius, func(p Point) bool {
		if p.Equal(from) || w.TerrainAt(p) != Floor || w.occupied(p) ||
			w.onPendingBuild(p) || !w.bordersSolid(p) {
			return false
		}
		best, found = p, true
		return true // nearest-first: first valid spot is closest
	})
	return best, found
}

// bordersFloor reports whether p has at least one walkable neighbor.
func (w *World) bordersFloor(p Point) bool {
	for _, d := range neighbors8 {
		if w.Walkable(p.Add(d.X, d.Y)) {
			return true
		}
	}
	return false
}

// bordersSolid reports whether p has at least one Rock or Wall neighbor.
func (w *World) bordersSolid(p Point) bool {
	for _, d := range neighbors8 {
		switch w.TerrainAt(p.Add(d.X, d.Y)) {
		case Rock, Wall:
			return true
		}
	}
	return false
}

// ---- Aliens ------------------------------------------------------------------

func (w *World) alienTurn(e *Entity) {
	if e.Cooldown > 0 {
		e.Cooldown-- // still digesting or mid-stride between slow steps
		return
	}

	prey, ok := w.nearestColonist(e.Pos, 1<<30)
	if !ok {
		e.State, e.Quarry = Idle, 0
		w.wanderStep(e)
		e.Cooldown = w.cfg.AlienSlowness - 1
		return
	}
	e.Quarry = prey.ID

	if e.Pos.Adjacent(prey.Pos) {
		w.bite(e, prey)
		e.Cooldown = w.cfg.AlienBiteRest
		return
	}

	// Aliens burrow: they step toward prey through any terrain.
	e.State = Hunting
	w.burrowStep(e, prey.Pos)
	e.Cooldown = w.cfg.AlienSlowness - 1
}

// bite deals damage to a colonist and eats it if the wound is fatal.
func (w *World) bite(alien, prey *Entity) {
	prey.HP -= w.cfg.AlienDamage
	if prey.HP <= 0 {
		alien.State = Feeding
		w.remove(prey.ID)
		w.log.add(fmt.Sprintf("An alien devours colonist #%d.", prey.ID))
	} else {
		alien.State = Hunting
	}
}

// ---- Cats --------------------------------------------------------------------

// catTurn walks the cat toward the nearest mouse and pounces when adjacent. Cats
// have no needs; they simply hunt. Unlike aliens they cannot burrow, so they
// travel the floor with cached A* and give up on prey they cannot reach.
func (w *World) catTurn(e *Entity) {
	if e.Cooldown > 0 {
		e.Cooldown-- // mid-stride between slow steps, or resting after a catch
		return
	}

	prey, ok := w.nearestMouse(e.Pos, 1<<30)
	if !ok {
		e.State, e.Quarry = Idle, 0
		w.wanderStep(e)
		e.Cooldown = w.cfg.CatSlowness - 1
		return
	}
	e.Quarry = prey.ID

	if e.Pos.Adjacent(prey.Pos) {
		w.pounce(e, prey)
		e.Cooldown = w.cfg.CatPounceRest
		return
	}

	e.State = Hunting
	if _, ok := w.travelTo(e, prey.Pos); !ok {
		// The mouse is unreachable on foot (walled off, or the cat is wedged):
		// prowl instead of standing still.
		w.wanderStep(e)
	}
	e.Cooldown = w.cfg.CatSlowness - 1
}

// pounce catches and eats an adjacent mouse. A mouse is tiny, so a single pounce
// is fatal.
func (w *World) pounce(cat, prey *Entity) {
	cat.State = Feeding
	w.remove(prey.ID)
	w.log.add(fmt.Sprintf("A cat catches mouse #%d.", prey.ID))
}

// ---- Mice --------------------------------------------------------------------

// mouseTurn runs one mouse tick: starve, flee cats, feed at a nutrient pod when
// hungry, otherwise scurry about. Mice reuse the colonists' food need and the
// generic JobUse machinery, but never build — they depend on pods the colony
// has already raised, and go hungry if none is reachable.
func (w *World) mouseTurn(e *Entity) {
	w.applyStarvation(e)
	if !e.Alive() { // starved this tick
		w.clearJob(e)
		w.remove(e.ID)
		w.log.add(fmt.Sprintf("Mouse #%d starves.", e.ID))
		return
	}

	// Survival first: bolt from a nearby cat.
	if threat, ok := w.nearestCat(e.Pos, w.cfg.MouseFleeRadius); ok {
		w.clearJob(e)
		e.State = Fleeing
		w.fleeStep(e, threat.Pos)
		return
	}

	// Hungry? Head for a nutrient pod if one is reachable. Mice care only about
	// food, so we check it directly rather than scanning every need.
	hungry := w.needLevel(e, NeedFood) >= w.cfg.Needs[NeedFood].SeekAt
	if hungry && e.Job != JobUse {
		if field := w.facilityField(NutrientPod); field != nil && field.at(e.Pos) >= 0 {
			e.Job, e.Need, e.Progress = JobUse, NeedFood, 0
		}
	}
	if e.Job == JobUse {
		w.jobUse(e)
		return
	}

	e.State = Idle
	w.wanderStep(e)
}

// ---- Movement primitives -----------------------------------------------------

// burrowStep moves an alien one step toward dest through any terrain. It avoids
// tiles already occupied by another entity (one body per tile); it attacks
// colonists from an adjacent tile rather than stepping onto them.
func (w *World) burrowStep(e *Entity, dest Point) {
	target := stepToward(e.Pos, dest)
	if w.InBounds(target) && !w.occupiedByOther(target, e.ID) {
		w.moveEntity(e, target)
		return
	}
	bestDist := e.Pos.Chebyshev(dest)
	best := e.Pos
	for _, d := range neighbors8 {
		n := e.Pos.Add(d.X, d.Y)
		if !w.InBounds(n) || w.occupiedByOther(n, e.ID) {
			continue
		}
		if dd := n.Chebyshev(dest); dd < bestDist {
			best, bestDist = n, dd
		}
	}
	w.moveEntity(e, best)
}

// fleeStep moves a colonist one walkable step that maximizes distance from a
// threat.
func (w *World) fleeStep(e *Entity, threat Point) {
	best := e.Pos
	bestDist := e.Pos.Chebyshev(threat)
	for _, d := range neighbors8 {
		n := e.Pos.Add(d.X, d.Y)
		if !w.Walkable(n) || w.occupiedByOther(n, e.ID) {
			continue
		}
		if dd := n.Chebyshev(threat); dd > bestDist {
			best, bestDist = n, dd
		}
	}
	w.moveEntity(e, best)
}

// wanderStep takes a small random step. Only aliens burrow; every other kind
// (colonists, cats, mice) stays on walkable floor. Used when there is nothing
// better to do.
func (w *World) wanderStep(e *Entity) {
	if w.rng.Intn(2) == 0 {
		return // often stay put so idlers do not jitter constantly
	}
	d := neighbors8[w.rng.Intn(len(neighbors8))]
	n := e.Pos.Add(d.X, d.Y)
	if !w.InBounds(n) || w.occupiedByOther(n, e.ID) {
		return
	}
	if e.Kind != Alien && !w.Walkable(n) {
		return // only aliens burrow; colonists, cats, and mice stay on floor
	}
	if e.Kind == Colonist && w.buildTiles[n] {
		return // colonists keep off tiles a builder needs clear
	}
	w.moveEntity(e, n)
}

// stepAside moves a colonist off a facility-access or pending-build tile. It may
// pass through a packed group of colonists to find the nearest genuinely clear
// landing, just as job navigation can pass through a crowd. A random one-step
// wander is insufficient here: in a full room there may be no adjacent vacancy,
// leaving a builder or food queue blocked indefinitely.
func (w *World) stepAside(e *Entity) bool {
	start := w.index(e.Pos)
	seen := map[int]bool{start: true}
	q := []int{start}
	var candidates []Point
	for head := 0; head < len(q); {
		levelEnd := len(q)
		candidates = candidates[:0]
		for ; head < levelEnd; head++ {
			ci := q[head]
			from := Point{ci % w.Width, ci / w.Width}
			for _, d := range neighbors8 {
				p := from.Add(d.X, d.Y)
				if !w.Walkable(p) || w.buildTiles[p] {
					continue
				}
				pi := w.index(p)
				if seen[pi] {
					continue
				}
				seen[pi] = true
				if blocker := w.entityAt(p); blocker != nil && blocker.ID != e.ID {
					if blocker.Kind == Colonist {
						q = append(q, pi)
					}
					continue
				}
				if !w.onFacilityAccess(p) {
					candidates = append(candidates, p)
				}
			}
		}
		if len(candidates) > 0 {
			w.moveEntity(e, candidates[w.rng.Intn(len(candidates))])
			return true
		}
	}
	return false
}

// ---- Queries -----------------------------------------------------------------

func (w *World) nearestColonist(from Point, within int) (*Entity, bool) {
	return w.nearestOfKind(from, Colonist, within)
}

func (w *World) nearestAlien(from Point, within int) (*Entity, bool) {
	return w.nearestOfKind(from, Alien, within)
}

func (w *World) nearestCat(from Point, within int) (*Entity, bool) {
	return w.nearestOfKind(from, Cat, within)
}

func (w *World) nearestMouse(from Point, within int) (*Entity, bool) {
	return w.nearestOfKind(from, Mouse, within)
}

func (w *World) nearestOfKind(from Point, kind Kind, within int) (*Entity, bool) {
	var best *Entity
	bestDist := within + 1
	fcx, fcy := from.X/chunkSize, from.Y/chunkSize
	maxRing := w.chunkCols + w.chunkRows

	// Expand in chunk rings around the query point. A chunk at ring r holds no
	// cell closer than (r-1)*chunkSize+1, so once we have a candidate we can stop
	// as soon as the next ring cannot beat it. Ties break toward the lower ID, so
	// the result is deterministic regardless of bucket order.
	for r := 0; r <= maxRing; r++ {
		minPossible := 0
		if r >= 1 {
			minPossible = (r-1)*chunkSize + 1
		}
		if minPossible > within || (best != nil && minPossible > bestDist) {
			break
		}
		loRow, hiRow, loCol, hiCol := fcy-r, fcy+r, fcx-r, fcx+r
		for cy := loRow; cy <= hiRow; cy++ {
			if cy < 0 || cy >= w.chunkRows {
				continue
			}
			onRowEdge := cy == loRow || cy == hiRow
			for cx := loCol; cx <= hiCol; cx++ {
				if cx < 0 || cx >= w.chunkCols {
					continue
				}
				if !onRowEdge && cx != loCol && cx != hiCol {
					continue // interior chunk, already covered by a smaller ring
				}
				for _, id := range w.chunkEntities[cy*w.chunkCols+cx] {
					e := w.entities[id]
					if e == nil || e.Kind != kind || !e.Alive() {
						continue
					}
					d := from.Chebyshev(e.Pos)
					if d > within {
						continue
					}
					if best == nil || d < bestDist || (d == bestDist && e.ID < best.ID) {
						best, bestDist = e, d
					}
				}
			}
		}
	}
	return best, best != nil
}

// forEachInRadius visits the in-bounds tiles within a square radius of center,
// nearest ring first, until visit returns true. It allocates nothing, so it is
// safe to call per entity per tick.
func (w *World) forEachInRadius(center Point, radius int, visit func(Point) bool) {
	for r := 1; r <= radius; r++ {
		for y := -r; y <= r; y++ {
			for x := -r; x <= r; x++ {
				if abs(x) != r && abs(y) != r {
					continue // only the ring at exactly distance r
				}
				p := center.Add(x, y)
				if w.InBounds(p) && visit(p) {
					return
				}
			}
		}
	}
}

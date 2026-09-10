package sim

import (
	"fmt"
	"sort"
)

// step advances the world by one tick: every living entity takes a turn in
// ascending ID order so a given seed always produces the same run. The dead are
// removed the moment they are eaten or starve, so we re-check liveness as we go.
func (w *World) step() {
	w.tick++
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e == nil || !e.Alive() {
			continue
		}
		switch e.Kind {
		case Colonist:
			w.colonistTurn(e)
		case Alien:
			w.alienTurn(e)
		}
	}
	w.refreshSpatial() // fold in any digging/building from this tick
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
	// comes first. Head to the facility, or build one if none exists yet.
	need, urgent := w.mostUrgentNeed(e)
	if urgent && !(e.Job == JobUse && e.Need == need) {
		spec := w.cfg.Needs[need]
		if field := w.facilityField(spec.Facility); field != nil && field.at(e.Pos) >= 0 {
			// A facility of this kind is reachable: follow its shared flow field.
			w.clearJob(e)
			e.Job, e.Need, e.Progress = JobUse, need, 0
		} else if spot, ok := w.findBuildSpot(e.Pos, 20); ok {
			// None reachable yet: build one rather than perish.
			w.clearJob(e)
			w.assignBuild(e, spec.Facility, spot)
		}
		e.resting = false
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
		e.resting = true
		e.wakeTick = w.tick + w.cfg.RestTicks
		e.State = Idle
		return
	}
	e.resting = false
	w.runJob(e)
}

// assignMine / assignBuild set a colonist's job and update the board's
// bookkeeping (claim the mine tile / count the in-progress build). clearJob
// reverses whichever bookkeeping the current job holds and returns the colonist
// to idle. Routing every job start and end through these keeps the board's claim
// set and build counts exact.
func (w *World) assignMine(e *Entity, target Point) {
	e.Job, e.Target, e.Progress = JobMine, target, 0
}

func (w *World) assignBuild(e *Entity, kind Terrain, target Point) {
	e.Job, e.BuildKind, e.Target, e.Progress = JobBuild, kind, target, 0
	w.board.startBuild(kind)
}

func (w *World) clearJob(e *Entity) {
	switch e.Job {
	case JobMine:
		w.board.releaseMine(e.Target)
	case JobBuild:
		w.board.endBuild(e.BuildKind)
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

// assignWorkJob picks something productive to do: keep the colony's life-support
// stocked first, then occasionally wall things off, otherwise mine. Leaves
// JobNone if nothing suitable is nearby.
func (w *World) assignWorkJob(e *Entity) {
	desired := w.desiredFacilities(w.countKind(Colonist))

	if w.plannedFacilities(NutrientPod) < desired {
		if spot, ok := w.findBuildSpot(e.Pos, 16); ok {
			w.assignBuild(e, NutrientPod, spot)
			return
		}
	}
	if w.plannedFacilities(Toilet) < desired {
		if spot, ok := w.findBuildSpot(e.Pos, 16); ok {
			w.assignBuild(e, Toilet, spot)
			return
		}
	}
	if w.rng.Intn(100) < w.cfg.BuildChance {
		if spot, ok := w.findBuildSpot(e.Pos, 12); ok {
			w.assignBuild(e, Wall, spot)
			return
		}
	}
	// Mining is pulled from the job board: claim the nearest reachable frontier
	// tile instead of scanning a radius of the map.
	if target, ok := w.board.claimNearestMine(e.Pos, w.roomOf(e.Pos)); ok {
		w.assignMine(e, target)
		return
	}
	e.Job = JobNone
}

// plannedFacilities counts a facility kind that already exists plus those a
// colonist is currently building, so the colony converges on the desired number
// instead of every idle colonist starting one at the same instant. The
// in-progress count comes from the board's O(1) counter, not an entity scan.
func (w *World) plannedFacilities(kind Terrain) int {
	return w.countTerrain(kind) + w.board.inProgress(kind)
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
	if w.TerrainAt(e.Target) != Rock { // already mined by someone
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
	e.State = Mining
	e.Progress++
	if e.Progress >= w.cfg.MineTicks {
		w.SetTerrain(e.Target, Floor) // TileChanged drops the tile from the frontier
		w.clearJob(e)
	}
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
	if w.occupiedByOther(e.Target, e.ID) { // can't build where someone stands
		w.clearJob(e)
		return
	}
	e.State = Building
	e.Progress++
	if e.Progress >= w.buildTicks(e.BuildKind) {
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
	next := e.path[e.pathAt]
	switch {
	case !w.Walkable(next): // terrain changed under the route; replan next tick
		e.clearPath()
		return false, true
	case w.occupiedByOther(next, e.ID): // wait for the blocker to move
		e.stuck++
		if e.stuck > w.cfg.StuckLimit {
			e.clearPath()
			return false, false
		}
		return false, true
	}
	w.moveEntity(e, next)
	e.pathAt++
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
		if p.Equal(from) || w.TerrainAt(p) != Floor || w.occupied(p) || !w.bordersSolid(p) {
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

// wanderStep takes a small random step: colonists only onto floor, aliens
// anywhere. Used when there is nothing better to do.
func (w *World) wanderStep(e *Entity) {
	if w.rng.Intn(2) == 0 {
		return // often stay put so idlers do not jitter constantly
	}
	d := neighbors8[w.rng.Intn(len(neighbors8))]
	n := e.Pos.Add(d.X, d.Y)
	if !w.InBounds(n) || w.occupiedByOther(n, e.ID) {
		return
	}
	if e.Kind == Colonist && !w.Walkable(n) {
		return
	}
	w.moveEntity(e, n)
}

// ---- Queries -----------------------------------------------------------------

func (w *World) nearestColonist(from Point, within int) (*Entity, bool) {
	return w.nearestOfKind(from, Colonist, within)
}

func (w *World) nearestAlien(from Point, within int) (*Entity, bool) {
	return w.nearestOfKind(from, Alien, within)
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

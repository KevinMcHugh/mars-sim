package sim

import (
	"fmt"
	"sort"
)

// step advances the world by one tick: every living entity takes a turn, then
// the dead are already gone (removed the moment they are eaten). Entities act in
// ascending ID order so that a given seed always produces the same run.
func (w *World) step() {
	w.tick++

	ids := make([]EntityID, 0, len(w.entities))
	for id := range w.entities {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		e := w.entities[id]
		if e == nil || !e.Alive() {
			continue // eaten earlier this tick
		}
		switch e.Kind {
		case Colonist:
			w.colonistTurn(e)
		case Alien:
			w.alienTurn(e)
		}
	}
}

// ---- Colonists ---------------------------------------------------------------

func (w *World) colonistTurn(e *Entity) {
	// Survival comes first: if an alien is close, drop everything and run.
	if threat, ok := w.nearestAlien(e.Pos, w.cfg.FleeRadius); ok {
		e.State = Fleeing
		e.HasTarget = false
		e.Progress = 0
		w.fleeStep(e, threat.Pos)
		return
	}

	// No current job? Decide on one.
	if !e.HasTarget {
		w.assignColonistJob(e)
		if !e.HasTarget {
			e.State = Idle
			w.wanderStep(e)
			return
		}
	}

	switch e.State {
	case Mining:
		w.doMining(e)
	case Building:
		w.doBuilding(e)
	default:
		// Moving toward a job site.
		w.travelToJob(e)
	}
}

// assignColonistJob picks a new mining or building task and sets the colonist's
// Target/State, or leaves HasTarget false if nothing suitable is nearby.
func (w *World) assignColonistJob(e *Entity) {
	e.Progress = 0
	if w.rng.Intn(100) < w.cfg.BuildChance {
		if spot, ok := w.findBuildSpot(e.Pos, 12); ok {
			e.Target, e.HasTarget, e.State = spot, true, Moving
			return
		}
	}
	if rock, ok := w.findMineable(e.Pos, 18); ok {
		e.Target, e.HasTarget, e.State = rock, true, Moving
		return
	}
	e.HasTarget = false
}

// travelToJob walks one step toward the current Target and switches to the work
// state once adjacent. It abandons the job if it cannot make progress (greedy
// movement can get boxed in; the colonist simply picks a new task next tick).
func (w *World) travelToJob(e *Entity) {
	// Mining is done from an adjacent floor tile; building is done from an
	// adjacent tile as well. Either way, "close enough" means adjacent.
	if e.Pos.Adjacent(e.Target) {
		// Arrived. Choose the work state based on the target terrain.
		if w.TerrainAt(e.Target) == Rock {
			e.State = Mining
		} else {
			e.State = Building
		}
		return
	}
	before := e.Pos.Chebyshev(e.Target)
	moved := w.walkStep(e, e.Target)
	if !moved || e.Pos.Chebyshev(e.Target) >= before {
		e.HasTarget = false // stuck; repick next tick
	}
}

func (w *World) doMining(e *Entity) {
	// The target may have been mined by someone else, or we drifted away.
	if w.TerrainAt(e.Target) != Rock || !e.Pos.Adjacent(e.Target) {
		e.HasTarget = false
		return
	}
	e.Progress++
	if e.Progress >= w.cfg.MineTicks {
		w.SetTerrain(e.Target, Floor)
		e.HasTarget, e.Progress, e.State = false, 0, Idle
	}
}

func (w *World) doBuilding(e *Entity) {
	// Can only build on still-open, unoccupied floor next to us.
	if w.TerrainAt(e.Target) != Floor || !e.Pos.Adjacent(e.Target) || w.occupied(e.Target) {
		e.HasTarget = false
		return
	}
	e.Progress++
	if e.Progress >= w.cfg.BuildTicks {
		w.SetTerrain(e.Target, Wall)
		e.HasTarget, e.Progress, e.State = false, 0, Idle
	}
}

// findMineable returns the nearest Rock tile that borders open Floor (so a
// colonist can reach and excavate it), searched within radius.
func (w *World) findMineable(from Point, radius int) (Point, bool) {
	best, found := Point{}, false
	bestDist := radius + 1
	for _, p := range w.scanRadius(from, radius) {
		if w.TerrainAt(p) != Rock {
			continue
		}
		if !w.bordersFloor(p) {
			continue
		}
		if d := from.Chebyshev(p); d < bestDist {
			best, bestDist, found = p, d, true
		}
	}
	return best, found
}

// findBuildSpot returns the nearest open Floor tile that sits against Rock or
// Wall, i.e. an edge where a new wall would extend structure rather than plug a
// walkway at random. The colonist's own tile is excluded.
func (w *World) findBuildSpot(from Point, radius int) (Point, bool) {
	best, found := Point{}, false
	bestDist := radius + 1
	for _, p := range w.scanRadius(from, radius) {
		if p.Equal(from) || w.TerrainAt(p) != Floor || w.occupied(p) {
			continue
		}
		if !w.bordersSolid(p) {
			continue
		}
		if d := from.Chebyshev(p); d < bestDist {
			best, bestDist, found = p, d, true
		}
	}
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
		t := w.TerrainAt(p.Add(d.X, d.Y))
		if t == Rock || t == Wall {
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
		e.State = Idle
		e.Quarry = 0
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

// walkStep moves a colonist one step toward dest across walkable, unoccupied
// tiles, choosing the neighbor that most reduces distance. Returns whether it
// moved.
func (w *World) walkStep(e *Entity, dest Point) bool {
	best := e.Pos
	bestDist := e.Pos.Chebyshev(dest)
	for _, d := range neighbors8 {
		n := e.Pos.Add(d.X, d.Y)
		if !w.Walkable(n) || w.occupiedByOther(n, e.ID) {
			continue
		}
		if dd := n.Chebyshev(dest); dd < bestDist {
			best, bestDist = n, dd
		}
	}
	if best.Equal(e.Pos) {
		return false
	}
	e.Pos = best
	return true
}

// burrowStep moves an alien one step toward dest through any terrain, avoiding
// only tiles held by other aliens.
func (w *World) burrowStep(e *Entity, dest Point) {
	target := stepToward(e.Pos, dest)
	if w.InBounds(target) && !w.occupiedByKind(target, Alien, e.ID) {
		e.Pos = target
		return
	}
	// Preferred step is blocked by another alien; try any inbound neighbor that
	// gets us closer.
	bestDist := e.Pos.Chebyshev(dest)
	best := e.Pos
	for _, d := range neighbors8 {
		n := e.Pos.Add(d.X, d.Y)
		if !w.InBounds(n) || w.occupiedByKind(n, Alien, e.ID) {
			continue
		}
		if dd := n.Chebyshev(dest); dd < bestDist {
			best, bestDist = n, dd
		}
	}
	e.Pos = best
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
	e.Pos = best
}

// wanderStep takes a small random step: colonists only onto floor, aliens
// anywhere. Used when there is nothing better to do.
func (w *World) wanderStep(e *Entity) {
	if w.rng.Intn(2) == 0 {
		return // often just stay put, so idlers do not jitter constantly
	}
	d := neighbors8[w.rng.Intn(len(neighbors8))]
	n := e.Pos.Add(d.X, d.Y)
	if !w.InBounds(n) || w.occupiedByOther(n, e.ID) {
		return
	}
	if e.Kind == Colonist && !w.Walkable(n) {
		return
	}
	if e.Kind == Alien && w.occupiedByKind(n, Alien, e.ID) {
		return
	}
	e.Pos = n
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
	// Iterate in ID order so equal-distance ties resolve deterministically
	// (same seed => same run); ranging the map directly would not.
	for _, e := range w.sortedEntities() {
		if e.Kind != kind || !e.Alive() {
			continue
		}
		if d := from.Chebyshev(e.Pos); d <= within && d < bestDist {
			best, bestDist = e, d
		}
	}
	return best, best != nil
}

func (w *World) occupiedByOther(p Point, self EntityID) bool {
	for _, e := range w.entities {
		if e.ID != self && e.Pos.Equal(p) {
			return true
		}
	}
	return false
}

func (w *World) occupiedByKind(p Point, kind Kind, self EntityID) bool {
	for _, e := range w.entities {
		if e.ID != self && e.Kind == kind && e.Pos.Equal(p) {
			return true
		}
	}
	return false
}

// scanRadius returns the tiles within a square radius of center that lie in
// bounds, nearest rings first so callers that want the closest match can stop
// early if they wish.
func (w *World) scanRadius(center Point, radius int) []Point {
	pts := make([]Point, 0, (2*radius+1)*(2*radius+1))
	for r := 1; r <= radius; r++ {
		for y := -r; y <= r; y++ {
			for x := -r; x <= r; x++ {
				if abs(x) != r && abs(y) != r {
					continue // only the ring at exactly distance r
				}
				p := center.Add(x, y)
				if w.InBounds(p) {
					pts = append(pts, p)
				}
			}
		}
	}
	return pts
}

// sortedEntities returns all entities ordered by ascending ID. Used wherever
// iteration order would otherwise leak into game state (e.g. tie-breaking the
// nearest target), keeping runs reproducible for a given seed.
func (w *World) sortedEntities() []*Entity {
	out := make([]*Entity, 0, len(w.entities))
	for _, e := range w.entities {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

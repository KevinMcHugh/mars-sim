package sim

import "fmt"

// Holes: the one-way way down. A Hole is an open drop through the floor to
// the level below; whatever goes in lands on the tile underneath. Nothing
// walks onto one, so a hole is not walkable and joins no rooms: reachability
// stays symmetric, and only the behaviors that want a drop use one. A
// colonist cornered by a threat leaps down; refuse is tipped down as a chute;
// anyone standing on a tile when it becomes a hole falls. Falling hurts, legs
// first, by levels dropped. A hole with a ladder fitted becomes a shaft
// (see planLadders), which is also how the colony fetches back someone who
// fell. See docs/holes.md.

// holeProjectName and ladderProjectName name the two projects in the job
// board and the log.
const (
	holeProjectName   = "hole"
	ladderProjectName = "ladder"
)

// OrderHole asks the planner to dig one hole down from the deepest level the
// colony has reached, sited as a stair would be.
type OrderHole struct{}

func (OrderHole) isCommand() {}

// OrderLadder asks the planner to fit a ladder into the colony's first hole
// without one, turning it into a shaft.
type OrderLadder struct{}

func (OrderLadder) isCommand() {}

// fallTarget is where something dropped down the hole at p lands, and how
// many levels it falls: the first walkable tile straight below, through any
// holes on the way. ok is false when the column below is closed (rock, a
// wall, or a level nobody has broken into), and nothing can fall.
func (w *World) fallTarget(p Point) (land Point, levels int, ok bool) {
	q := p
	for {
		q = Point{q.X, q.Y, q.Level + 1}
		levels++
		if !w.levelExists(q.Level) {
			return Point{}, 0, false
		}
		switch t := w.TerrainAt(q); {
		case t == Hole:
			continue
		case t.Walkable():
			return q, levels, true
		default:
			return Point{}, 0, false
		}
	}
}

// openHole reports whether p is a hole something can drop down.
func (w *World) openHole(p Point) bool {
	if w.TerrainAt(p) != Hole {
		return false
	}
	_, _, ok := w.fallTarget(p)
	return ok
}

// canDigHole reports whether a hole can be dug at p: known open floor on the
// landing level or deeper, above a level Config allows digging to, whose
// tile below is rock or floor (a hole never opens onto a structure).
func (w *World) canDigHole(p Point) bool {
	if !w.canDigStairAt(p) {
		return false
	}
	t := w.TerrainAt(Point{p.X, p.Y, p.Level + 1})
	return t == Rock || t == Floor
}

// digHole breaks through the floor at p: the tile below becomes open floor
// (the rubble lands there), making the level below if this is the first way
// into it and revealing around it, and p becomes the Hole. Anyone standing
// on p falls on their next turn (see step). It reports false, changing
// nothing, when canDigHole says no.
func (w *World) digHole(p Point) bool {
	if !w.canDigHole(p) {
		return false
	}
	below := Point{p.X, p.Y, p.Level + 1}
	w.addLayer(below.Level)
	w.SetTerrain(below, Floor)
	w.SetTerrain(p, Hole)
	w.logEvent(LogBuildComplete, fmt.Sprintf("A hole is broken through to level %d.", below.Level))
	return true
}

// planHoles marks out the hole the player ordered (OrderHole), one at a
// time, and fits ladders (planLadders). It draws nothing random.
func (w *World) planHoles() {
	w.planLadders()
	if w.manualHoles == 0 || w.taskPlanned(Hole) {
		return
	}
	if int(w.deepestLevel()) >= w.cfg.DeepestLevel {
		w.manualHoles = 0 // nowhere further to dig
		return
	}
	site, ok := w.findStairSite(w.deepestLevel())
	if !ok || !w.canDigHole(site) {
		return // try again next planning round
	}
	pr := &project{id: w.nextProjectID, name: holeProjectName, queuedTick: w.tick, issuer: Community}
	pr.tasks = []*buildTask{{pos: site, terrain: Hole, proj: pr}}
	if !w.fundProject(pr) {
		return
	}
	w.nextProjectID++
	w.projects = append(w.projects, pr)
	w.manualHoles--
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony marks out a hole down to level %d.", site.Level+1))
}

// taskPlanned reports whether a task of terrain t is planned and unbuilt.
func (w *World) taskPlanned(t Terrain) bool {
	for _, p := range w.projects {
		for _, tk := range p.tasks {
			if tk.terrain == t && !w.taskDone(tk) {
				return true
			}
		}
	}
	return false
}

// finishHole is jobBuild's last step for a hole task: dig it, and pay and
// credit the digger as for any build.
func (w *World) finishHole(e *Entity) {
	if !w.digHole(e.Target) {
		w.clearJob(e)
		return
	}
	w.practise(e, SkillMining, w.buildTicks(Hole))
	if t := e.task; t != nil {
		w.payWork(t.order, e)
	}
	o := w.occurrence(e, ActionConstruct, nil, e.Target, "Broke a hole through to level %d at (%d, %d).",
		e.Target.Level+1, e.Target.X, e.Target.Y)
	o.Object = FactRef{Noun: NounStructure, Label: Hole.String()}
	w.emitOccurrence(o)
	w.clearJob(e)
}

// ---- Ladders -----------------------------------------------------------------

// planLadders fits a ladder into a hole, turning it into a shaft one level
// deep (a shaft task on the hole: see canDigShaft), when someone has fallen
// down it and is stranded below (its landing is in a room with a colonist
// in it that is not the colony's main room), or when the player ordered one
// (OrderLadder). A ladder goes only into a hole the colony can stand beside.
// One at a time, holes in sorted order, nothing random.
func (w *World) planLadders() {
	if len(w.holes) == 0 || w.mainRoom == 0 || w.taskPlanned(ShaftTop) {
		return
	}
	for _, h := range w.holes {
		land, levels, ok := w.fallTarget(h)
		if !ok || levels != 1 || w.fixtureCutOff(h) {
			continue
		}
		if w.manualLadders == 0 && !w.strandedIn(w.roomOf(land)) {
			continue
		}
		if w.designateLadder(h) {
			if w.manualLadders > 0 {
				w.manualLadders--
			}
			return
		}
	}
	if w.manualLadders > 0 {
		w.manualLadders = 0 // no hole can take one
	}
}

// strandedIn reports whether a living colonist is in room, and room is not
// the colony's main room.
func (w *World) strandedIn(room RoomID) bool {
	if room == 0 || room == w.mainRoom {
		return false
	}
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Colonist && e.Alive() && w.roomOf(e.Pos) == room {
			return true
		}
	}
	return false
}

// designateLadder plans the ladder into the hole at h: a one-level shaft
// task on the hole, which digShaft turns into a ShaftTop over a ShaftBottom.
func (w *World) designateLadder(h Point) bool {
	if !w.canDigShaft(h, h.Level+1) {
		return false
	}
	pr := &project{id: w.nextProjectID, name: ladderProjectName, queuedTick: w.tick, issuer: Community}
	pr.tasks = []*buildTask{{pos: h, terrain: ShaftTop, depth: 1, proj: pr}}
	if !w.fundProject(pr) {
		return false
	}
	w.nextProjectID++
	w.projects = append(w.projects, pr)
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony means to fit a ladder into the hole at (%d, %d).", h.X, h.Y))
	return true
}

// ---- Falling -----------------------------------------------------------------

// fall drops e from the hole it is on to where the hole lands it, hurting it
// for every level it falls: Config.FallDamage a level, to a leg (left, then
// right, then left...) while it has one, else its torso. It lands on the
// nearest free tile to the foot of the drop. A fall can kill; the body stays
// where it landed. It reports whether e is still alive. A hole closed below
// (see fallTarget) or nowhere free to land holds e where it is.
func (w *World) fall(e *Entity) bool {
	from := e.Pos
	land, levels, ok := w.fallTarget(from)
	if !ok {
		return true
	}
	dest, ok := w.freeTileNear(land, e.ID)
	if !ok {
		return true
	}
	w.moveEntity(e, dest)
	e.clearPath()
	if e.Kind == Colonist {
		w.clearJob(e)
		w.markMindDirty(e)
	}
	fatal := false
	for i := 0; i < levels && !fatal; i++ {
		// Legs first, alternating; the torso when there are none.
		first, second := LeftLeg, RightLeg
		if i%2 == 1 {
			first, second = RightLeg, LeftLeg
		}
		part := Torso
		if e.hasPart(first) {
			part = first
		} else if e.hasPart(second) {
			part = second
		}
		fatal = applyDamage(e, part, w.cfg.FallDamage)
	}
	name := e.displayName()
	if fatal {
		w.addCorpse(dest, w.bodyOf(e))
		w.remove(e.ID, "fell to its death")
		w.logEvent(LogDeath, fmt.Sprintf("%s falls %d level(s) down a hole and is killed.", capitalizeFirst(name), levels))
		return false
	}
	w.logEvent(LogNote, fmt.Sprintf("%s falls %d level(s) down a hole to level %d.", capitalizeFirst(name), levels, dest.Level))
	return true
}

// bodyOf is the corpse e leaves when nothing eats it.
func (w *World) bodyOf(e *Entity) ItemKind {
	switch {
	case e.Kind == Colonist:
		return ColonistCorpse
	case e.Kind == Alien:
		return AlienCorpse
	case w.speciesOf(e).Corpse != ItemNone:
		return w.speciesOf(e).Corpse
	default:
		return AnimalCorpse
	}
}

// freeTileNear is the nearest walkable tile to p on its level that nobody
// but id stands on, searching outward through walkable tiles, ties in
// neighbour order.
func (w *World) freeTileNear(p Point, id EntityID) (Point, bool) {
	if !w.Walkable(p) {
		return Point{}, false
	}
	seen := map[Point]bool{p: true}
	q := []Point{p}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		if o := w.entityAt(c); o == nil || o.ID == id {
			return c, true
		}
		for _, d := range neighbors8 {
			n := c.Add(d.X, d.Y)
			if !seen[n] && w.Walkable(n) {
				seen[n] = true
				q = append(q, n)
			}
		}
	}
	return Point{}, false
}

// leap sends e down the open hole at h beside it: a colonist cornered by a
// threat with no step away takes the drop rather than the bite (see
// fleeStep). It reports whether e is still alive.
func (w *World) leap(e *Entity, h Point) bool {
	w.logEvent(LogNote, fmt.Sprintf("%s leaps down a hole to get away.", capitalizeFirst(e.displayName())))
	w.lay(e.Pos).occ.set(e.Pos.X, e.Pos.Y, 0)
	w.lay(h).occ.set(h.X, h.Y, e.ID)
	if oc, nc := w.chunkIndexOf(e.Pos), w.chunkIndexOf(h); oc != nc {
		w.removeFromChunkIndex(w.lay(e.Pos), oc, e.ID)
		w.lay(h).chunkEntities[nc] = append(w.lay(h).chunkEntities[nc], e.ID)
	}
	e.Pos = h
	return w.fall(e)
}

// openHoleBeside is the first open hole next to p, in neighbour order.
func (w *World) openHoleBeside(p Point) (Point, bool) {
	if len(w.holes) == 0 {
		return Point{}, false
	}
	for _, d := range neighbors8 {
		if n := p.Add(d.X, d.Y); w.openHole(n) {
			return n, true
		}
	}
	return Point{}, false
}

// ---- Chutes ------------------------------------------------------------------

// nearestChute is the open hole a colonist hauling refuse could tip it down:
// one it can stand beside, nearest by travelEstimate, ties to the first in
// sorted order. Only with Config.HoleChutes on.
func (w *World) nearestChute(e *Entity) (Point, bool) {
	if !w.cfg.HoleChutes || len(w.holes) == 0 {
		return Point{}, false
	}
	room := w.roomOf(e.Pos)
	var best Point
	bestD, found := 0, false
	for _, h := range w.holes {
		if !w.openHole(h) || !w.taskReachable(h, room) {
			continue
		}
		if d := w.travelEstimate(e.Pos, h); !found || d < bestD {
			best, bestD, found = h, d, true
		}
	}
	return best, found
}

// tipDown drops every piece of refuse e carries down the hole at h: bodies
// land as bodies, viscera as gore, on the tile the hole lands on. It is the
// incinerator's job done for free, but only downward: the level below keeps
// what it is sent.
func (w *World) tipDown(e *Entity, h Point) {
	land, _, ok := w.fallTarget(h)
	if !ok {
		return
	}
	corpses := 0
	for _, kind := range corpseKinds {
		n := e.Inventory.RemoveAll(kind)
		e.dropCargoOf(kind)
		for i := 0; i < n; i++ {
			w.addCorpse(land, kind)
		}
		corpses += n
	}
	viscera := e.Inventory.RemoveAll(Viscera)
	e.dropCargoOf(Viscera)
	for i := 0; i < viscera; i++ {
		w.addGore(land)
	}
	if corpses+viscera == 0 {
		return
	}
	phrase := refusePhrase(corpses, viscera)
	o := w.occurrence(e, ActionIncinerate, nil, e.Pos, "Tipped %s down a hole.", phrase)
	o.Object = FactRef{Noun: NounRefuse, Label: "refuse"}
	w.emitOccurrence(o)
	w.logEvent(LogBurn, fmt.Sprintf("%s tips %s down a hole to level %d.", e.displayName(), phrase, land.Level))
}

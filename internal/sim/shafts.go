package sim

import "fmt"

// Shafts: a laddered column straight down through one or more levels. A
// ShaftTop on the level it starts from, a ShaftMid on every level it passes
// through and a ShaftBottom at its foot, all at one (x, y); see links. It is
// cheaper to dig than a stair (one column, no landing, and many levels in one
// job), and slow to use: a climber spends Config.ShaftClimbTicks on every
// level, or ShaftLadenClimbTicks with its hands full, and some aliens cannot
// climb at all. See docs/shafts.md.

// shaftProjectName names a shaft's project in the job board and the log.
const shaftProjectName = "shaft"

// OrderShaft asks the planner to dig a shaft Levels levels deep (at least
// one), or to deepen the colony's deepest shaft by that much, down to as far
// as Config.DeepestLevel allows.
type OrderShaft struct{ Levels int }

func (OrderShaft) isCommand() {}

// shaftBottom is the level of the foot of the shaft whose top is at top: the
// deepest level the column reaches unbroken.
func (w *World) shaftBottom(top Point) Level {
	l := top.Level
	for {
		t := w.TerrainAt(Point{top.X, top.Y, l + 1})
		if t != ShaftMid && t != ShaftBottom {
			return l
		}
		l++
		if t == ShaftBottom {
			return l
		}
	}
}

// canDigShaft reports whether a shaft can be dug at top down to level
// bottom: top is known open floor (a new shaft) or an existing shaft's top
// above bottom (deepening it), on the landing level or deeper; bottom is no
// deeper than Config allows; and every tile the column would cut through on
// the way is rock or floor, or already this shaft. A shaft never cuts
// through a wall, a fixture or a stair.
func (w *World) canDigShaft(top Point, bottom Level) bool {
	if top.Level < LandingLevel || bottom <= top.Level || int(bottom) > w.cfg.DeepestLevel || !w.InBounds(top) {
		return false
	}
	from := top.Level
	switch w.TerrainAt(top) {
	case Floor:
		if !w.discovered(top) {
			return false
		}
	case ShaftTop:
		from = w.shaftBottom(top)
		if from >= bottom {
			return false
		}
	default:
		return false
	}
	for l := from + 1; l <= bottom; l++ {
		if t := w.TerrainAt(Point{top.X, top.Y, l}); t != Rock && t != Floor {
			return false
		}
	}
	return true
}

// shaftTaskBottom is the level a shaft task digs down to.
func shaftTaskBottom(t *buildTask) Level { return t.pos.Level + Level(t.depth) }

// shaftTaskDone reports whether a shaft task's column reaches its bottom.
func (w *World) shaftTaskDone(t *buildTask) bool {
	return w.TerrainAt(t.pos) == ShaftTop && w.shaftBottom(t.pos) >= shaftTaskBottom(t)
}

// shaftWorkTicks is the work left in digging a shaft task: ShaftTicks for
// every level not yet dug.
func (w *World) shaftWorkTicks(t *buildTask) int {
	from := t.pos.Level
	if w.TerrainAt(t.pos) == ShaftTop {
		from = w.shaftBottom(t.pos)
	}
	return w.cfg.ShaftTicks * max(1, int(shaftTaskBottom(t)-from))
}

// digShaft cuts the column at top down to level bottom: every level it
// reaches is made if this is the first way into it, the old foot (if
// deepening) becomes a ShaftMid, each level passed through gets a ShaftMid,
// the new foot a ShaftBottom, and the top a ShaftTop. Each tile it breaks
// into is revealed around, caverns and their nests included, as digging a
// stair does. The rock dug out becomes the shaft: nobody gets its ore. It
// reports false, changing nothing, when canDigShaft says no.
func (w *World) digShaft(top Point, bottom Level) bool {
	if !w.canDigShaft(top, bottom) {
		return false
	}
	from := top.Level
	if w.TerrainAt(top) == ShaftTop {
		from = w.shaftBottom(top)
	}
	// Bottom up, so no tile is ever a shaft end whose other end is missing
	// for longer than this call.
	for l := bottom; l > from; l-- {
		w.addLayer(l)
		t := ShaftMid
		if l == bottom {
			t = ShaftBottom
		}
		w.SetTerrain(Point{top.X, top.Y, l}, t)
	}
	if from != top.Level {
		w.SetTerrain(Point{top.X, top.Y, from}, ShaftMid) // the old foot
	} else {
		w.SetTerrain(top, ShaftTop)
	}
	w.logEvent(LogBuildComplete, fmt.Sprintf("A shaft is sunk from level %d down to level %d.", top.Level, bottom))
	return true
}

// planShafts marks out the shaft the player ordered (OrderShaft), one at a
// time: it deepens the deepest shaft the colony can reach, or starts a new
// one on the deepest level the colony has reached, sited as a stair is (see
// findStairSite). Like planStairs it draws nothing random, and does nothing
// without an order.
func (w *World) planShafts() {
	n := Level(w.manualShaftLevels)
	if n == 0 || w.shaftPlanned() {
		return
	}
	deepest := Level(w.cfg.DeepestLevel)
	// The deepest shaft the colony can reach, if it can go deeper.
	var top Point
	found := false
	for _, p := range w.shafts {
		if w.TerrainAt(p) != ShaftTop || w.mainRoom == 0 || w.roomOf(p) != w.mainRoom ||
			w.shaftBottom(p) >= deepest {
			continue
		}
		if !found || w.shaftBottom(p) > w.shaftBottom(top) {
			top, found = p, true
		}
	}
	bottom := Level(0)
	if found {
		bottom = min(w.shaftBottom(top)+n, deepest)
	} else {
		if w.deepestLevel() >= deepest {
			w.manualShaftLevels = 0 // nowhere further to dig
			return
		}
		site, ok := w.findStairSite(w.deepestLevel())
		if !ok {
			return // try again next planning round
		}
		top, bottom = site, min(site.Level+n, deepest)
	}
	if w.designateShaft(top, bottom, Community) {
		w.manualShaftLevels = 0
	}
}

// shaftPlanned reports whether a shaft is already planned and undug.
func (w *World) shaftPlanned() bool {
	for _, p := range w.projects {
		for _, t := range p.tasks {
			if t.terrain == ShaftTop && !w.taskDone(t) {
				return true
			}
		}
	}
	return false
}

// designateShaft plans a shaft at top down to level bottom, paid for by
// issuer, as a one-task project: one digger cuts the whole column.
func (w *World) designateShaft(top Point, bottom Level, issuer Owner) bool {
	if !w.canDigShaft(top, bottom) {
		return false
	}
	pr := &project{id: w.nextProjectID, name: shaftProjectName, queuedTick: w.tick, issuer: issuer}
	pr.tasks = []*buildTask{{pos: top, terrain: ShaftTop, depth: int(bottom - top.Level), proj: pr}}
	if !w.fundProject(pr) {
		return false
	}
	w.nextProjectID++
	w.projects = append(w.projects, pr)
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony marks out a shaft from level %d down to level %d.", top.Level, bottom))
	return true
}

// finishShaft is jobBuild's last step for a shaft task: dig it, and pay and
// credit the digger as for any build.
func (w *World) finishShaft(e *Entity) {
	t := e.task
	if t == nil || !w.digShaft(e.Target, shaftTaskBottom(t)) {
		w.clearJob(e)
		return
	}
	w.practise(e, SkillMining, w.cfg.ShaftTicks*t.depth)
	w.payWork(t.order, e)
	o := w.occurrence(e, ActionConstruct, nil, e.Target, "Sank a shaft down to level %d at (%d, %d).",
		shaftTaskBottom(t), e.Target.X, e.Target.Y)
	o.Object = FactRef{Noun: NounStructure, Label: ShaftTop.String()}
	w.emitOccurrence(o)
	w.clearJob(e)
}

// ---- Climbing ----------------------------------------------------------------

// canClimb reports whether e can use a shaft at all. Colonists, cats and
// rats climb. A chicken does not. An alien climbs if its species has arms to
// grip the rungs with; a legs-only build (and every winged one: no species
// flies) cannot, so it reaches a level joined to its own only by shafts not
// at all. See docs/shafts.md.
func (w *World) canClimb(e *Entity) bool {
	switch e.Kind {
	case Colonist, Cat, Rat:
		return true
	case Alien:
		return w.alienSpeciesFor(e).Arms > 0
	default:
		return false
	}
}

// canReach reports whether hunter e can get to c: they share a room, and,
// since rooms join across shafts, e can climb or c is on its level. (A stair
// down and back up to prey on the same level counts as reachable too, which
// is rarely the way.)
func (w *World) canReach(e, c *Entity) bool {
	return w.sameRoom(e.Pos, c.Pos) && (c.Pos.Level == e.Pos.Level || !w.hasShafts() || w.canClimb(e))
}

// laden reports whether e's hands are too full to climb at the usual pace:
// it carries more than ShaftCarry bulky goods (rock, ore, ice, clay, and
// carcasses).
func (w *World) laden(e *Entity) bool {
	n := 0
	for _, s := range e.Inventory {
		if s.Kind.isStorableMaterial() || s.Kind == AlienCorpse || s.Kind == AnimalCorpse {
			n += s.Count
		}
	}
	return n > w.cfg.ShaftCarry
}

// climbTicks is how long e takes to climb a shaft one level.
func (w *World) climbTicks(e *Entity) int {
	if w.laden(e) {
		return w.cfg.ShaftLadenClimbTicks
	}
	return w.cfg.ShaftClimbTicks
}

// climbCost is what one level of shaft costs e's route (the pathfinder's
// climb): -1 if e cannot climb, else its climb ticks.
func (w *World) climbCost(e *Entity) int32 {
	if !w.hasShafts() {
		return 0
	}
	if !w.canClimb(e) {
		return -1
	}
	return int32(w.climbTicks(e))
}

// crossesShaft reports whether a move from a to b is a climb: b is straight
// above or below a, and both are shaft tiles. Movers never pass through a
// crowd across a shaft (see travelTo and followField), so a climb's move is
// always from one end of the link to the other.
func (w *World) crossesShaft(a, b Point) bool {
	return a.Level != b.Level && a.X == b.X && a.Y == b.Y &&
		w.TerrainAt(a).isShaft() && w.TerrainAt(b).isShaft()
}

// startClimb is moveEntity's hook for a move across a shaft: the mover is
// on the ladder until its climb is over, and spends its turns till then
// climbing (see climbing).
func (w *World) startClimb(e *Entity) {
	e.climbUntil = w.tick + w.climbTicks(e) - 1 // the move itself took this tick
	e.State = Climbing
}

// climbing reports whether e is still on a ladder this tick, and if so
// spends its turn there: it does nothing else, cannot fight back or flee,
// and a colonist's drives go on as they would walking.
func (w *World) climbing(e *Entity) bool {
	if e.climbUntil < w.tick {
		return false
	}
	e.State = Climbing
	return true
}

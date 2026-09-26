package sim

// ---- Rats scavenging -----------------------------------------------------------
//
// Rats eat what the scumhouse eats. A hungry rat looks for the nearest tile it
// can reach with a body on it (any body, a colonist's included — it is a rat),
// gore, or cave scum, and eats a unit of it where it lies; only with nothing
// in range does it go raiding a nutrient pod. Rats claim nothing, so they race
// cleaners and scrapers for the same biomatter, and every unit a rat eats is
// one the colony cannot turn into slurry. Cats are what keep that in check.
// See docs/entities-and-ai.md and docs/scumhouse.md.

// scavengeable reports whether a rat could eat something on p right now.
func (w *World) scavengeable(p Point) bool {
	if w.refuseAt(p) > 0 {
		return true
	}
	if w.scumAt(p) == 0 {
		return false
	}
	_, exposed := w.exposedScum[p]
	return exposed
}

// nearestScavenge finds the nearest tile within the rat's scavenging radius
// holding something it can eat and can get to: refuse or scum on floor in its
// own room, or scum on a rock face that room touches. Nearest ring first, in
// forEachInRadius's fixed order, so the choice is the same for a given world.
func (w *World) nearestScavenge(e *Entity) (Point, bool) {
	room := w.roomOf(e.Pos)
	if room == 0 {
		return Point{}, false
	}
	if w.scavengeable(e.Pos) {
		return e.Pos, true
	}
	var best Point
	found := false
	w.forEachInRadius(e.Pos, w.cfg.RatScavengeRadius, func(p Point) bool {
		if !w.scavengeable(p) {
			return false
		}
		if w.Walkable(p) {
			if w.roomOf(p) != room {
				return false
			}
		} else if !w.taskReachable(p, room) {
			return false
		}
		best, found = p, true
		return true
	})
	return best, found
}

// jobScavenge walks a rat to the food at Target and eats a unit of it: a body
// first, then gore, then scum. If something else got there first, the rat
// thinks again next tick.
func (w *World) jobScavenge(e *Entity) {
	if !w.scavengeable(e.Target) {
		w.clearJob(e)
		return
	}
	if e.Pos.Chebyshev(e.Target) > 1 {
		arrived, ok := w.travelTo(e, e.Target)
		if !ok {
			w.clearJob(e)
			return
		}
		if !arrived {
			e.State = Moving
			return
		}
	}
	e.State = Eating
	e.Progress++
	if e.Progress < w.cfg.Needs[NeedFood].UseTicks {
		return
	}
	if w.eatScavenge(e.Target) {
		w.resetNeed(e, NeedFood)
	}
	w.clearJob(e)
}

// eatScavenge removes one unit of whatever a rat eats from p, reporting
// whether there was any. Bodies go first — the most food, and what a rat
// would go for — then gore, then scum.
func (w *World) eatScavenge(p Point) bool {
	for _, kind := range [...]ItemKind{AnimalCorpse, AlienCorpse, ColonistCorpse} {
		if w.takeCorpse(p, kind) {
			return true
		}
	}
	if w.takeGore(p) {
		return true
	}
	if _, exposed := w.exposedScum[p]; exposed {
		return w.takeScum(p)
	}
	return false
}

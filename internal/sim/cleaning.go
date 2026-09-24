package sim

import "fmt"

// ---- Sanitation ----------------------------------------------------------
//
// Refuse is the mess a colony leaves behind: gore splattered by a violent death
// (Tile.Gore) and the bodies of the dead (Tile.Corpses). A colonist with no
// urgent need scrubs it up, carries it to an incinerator, and burns it. That is
// one job, JobClean, run in two stages (see cleanStage): gather, then haul.
//
// The loop only turns when there is somewhere to put the refuse, which is the
// whole reason the colony builds a trash room (see trashRoom in project.go).
// Without a reachable incinerator a colonist never starts cleaning — picking a
// body up with nowhere to take it would just move the mess into an inventory
// slot. See docs/sanitation.md.

// cleanRadius is how far a colonist will go looking for a mess. A Tidy colonist
// — the one the sight of gore hits hardest (see EvtSawGore) — ranges twice as
// far to get rid of it, which is the trait's first behavioral effect rather
// than only a mood modifier.
func (w *World) cleanRadius(e *Entity) int {
	r := w.cfg.CleanRadius
	if e.Profile != nil && e.Profile.HasTrait(TraitTidy) {
		r *= 2
	}
	return r
}

// carryingRefuse reports whether a colonist is holding refuse that still needs
// burning.
func carryingRefuse(e *Entity) bool {
	for _, stack := range e.Inventory {
		if stack.Count > 0 && stack.Kind.isRefuse() {
			return true
		}
	}
	return false
}

// tryAssignClean commits a colonist to a cleaning job if there is one worth
// taking, reporting whether it did. It is work, not an idle whim: rock is
// effectively infinite, so a colonist that only cleaned when it had nothing
// else to do would never clean at all. assignWorkJob therefore offers cleaning
// ahead of mining and behind construction — life support first, then tidy up,
// then dig.
//
// A mess is only worth picking up if there is somewhere to take it. Biomatter
// (viscera, and every body but a colonist's) goes to a scumhouse; a colonist's
// body, and biomatter no scumhouse can take, goes to the incinerator. So with
// only a scumhouse, a cleaner gathers biomatter and leaves colonists' bodies
// where they lie; with only an incinerator, it burns everything, as the colony
// always did. See docs/sanitation.md.
func (w *World) tryAssignClean(e *Entity) bool {
	// Cheap checks first: this runs for every work-seeking colonist every tick,
	// and in a tidy colony it must not cost more than a couple of comparisons.
	hauling := carryingRefuse(e)
	if !hauling && (w.refuseTotal() == 0 || !e.Inventory.CanAdd(Viscera, 1)) {
		return false
	}
	burn, feed := w.refuseDestinations(e)
	if !burn && !feed {
		return false
	}
	// A colonist already holding a load (its last haul was interrupted, or a
	// destination only just came online) delivers that before touching
	// anything else. Refuse must never be stranded in an inventory slot.
	if hauling {
		dest, ok := w.haulTarget(e)
		if !ok {
			return false
		}
		e.Job, e.Target, e.clean, e.Progress = JobClean, dest, cleanHaul, 0
		return true
	}
	mess, ok := w.nearestRefuse(e.Pos, w.cleanRadius(e), burn)
	if !ok {
		return false
	}
	w.board.claimClean(mess, e.ID)
	e.Job, e.Target, e.clean, e.Progress = JobClean, mess, cleanGather, 0
	return true
}

// refuseDestinations reports whether e can reach an incinerator (burn) and a
// scumhouse with room for more biomatter (feed).
func (w *World) refuseDestinations(e *Entity) (burn, feed bool) {
	if field := w.facilityField(Incinerator); field != nil && field.at(e.Pos) >= 0 {
		burn = true
	}
	if w.countTerrain(Scumhouse) > 0 {
		_, feed = w.nearestScumhouse(e, func(c *StorageContainer) bool {
			return c.Inventory.CanAdd(Viscera, 1)
		})
	}
	return burn, feed
}

// nearestRefuse finds the closest unclaimed refuse tile within radius that a
// colonist standing at from could actually get to: it must be walkable floor in
// the same room. A stain that ended up under a wall, or one across an
// unexcavated vein, is not cleanable — and pretending otherwise would have
// cleaners walk at a tile they can never reach. Without an incinerator to burn
// it in (burn false), a tile holding only colonists' bodies is not worth the
// walk: nothing but an incinerator takes one.
func (w *World) nearestRefuse(from Point, radius int, burn bool) (Point, bool) {
	room := w.roomOf(from)
	if room == 0 {
		return Point{}, false
	}
	gatherable := func(p Point) bool {
		if w.refuseAt(p) == 0 {
			return false
		}
		return burn || w.goreAt(p) > 0 || w.corpsesAt(p) > w.corpsesOfAt(p, ColonistCorpse)
	}
	var best Point
	found := false
	if gatherable(from) && !w.board.isCleanClaimed(from) {
		return from, true // standing in it
	}
	w.forEachInRadius(from, radius, func(p Point) bool {
		if !gatherable(p) || !w.Walkable(p) || w.roomOf(p) != room ||
			w.board.isCleanClaimed(p) {
			return false
		}
		best, found = p, true
		return true // nearest ring first: the first match is the closest
	})
	return best, found
}

// nearestIncinerator picks the incinerator a colonist should haul to, if any is
// reachable. It reuses the shared facility field as the reachability gate and
// chooseFacility for the choice itself, so haulers spread across incinerators
// the same way eaters spread across nutrient pods.
func (w *World) nearestIncinerator(e *Entity) (Point, bool) {
	field := w.facilityField(Incinerator)
	if field == nil || field.at(e.Pos) < 0 {
		return Point{}, false
	}
	fac := w.chooseFacility(e, Incinerator)
	if w.TerrainAt(fac) != Incinerator {
		return Point{}, false
	}
	return fac, true
}

// haulTarget is where a colonist carrying refuse takes it next: a scumhouse
// that can take all the biomatter it carries, if it carries any, and otherwise
// the incinerator.
func (w *World) haulTarget(e *Entity) (Point, bool) {
	if bio := biomatterStacks(e); len(bio) > 0 {
		if p, ok := w.nearestScumhouse(e, func(c *StorageContainer) bool {
			return c.Inventory.CanAddAll(bio...)
		}); ok {
			return p, true
		}
	}
	return w.nearestIncinerator(e)
}

// jobClean runs one tick of a cleaning job: scrub the refuse at Target into the
// inventory, then carry it where it goes.
func (w *World) jobClean(e *Entity) {
	switch e.clean {
	case cleanGather:
		w.jobCleanGather(e)
	default:
		w.jobCleanHaul(e)
	}
}

// jobCleanGather walks to the claimed mess and scrubs it up. A colonist may
// stand on the refuse tile itself or beside it — unlike a build task there is
// nothing to keep clear, and a splatter underfoot is the common case.
func (w *World) jobCleanGather(e *Entity) {
	if w.refuseAt(e.Target) == 0 || !w.Walkable(e.Target) {
		w.clearJob(e) // somebody else got there first, or it was built over
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
	e.State = Cleaning
	e.Progress++
	if e.Progress < scaleTicks(w.cfg.CleanTicks, e.workScale) {
		return
	}
	burn, _ := w.refuseDestinations(e)
	w.gatherRefuse(e, e.Target, burn)
	// Hand the load off to the haul leg. Losing every destination between
	// stages (it was never built, or the route closed) ends the job with the
	// refuse still carried; tryAssignClean picks that up again later rather
	// than dropping it back on the floor.
	dest, ok := w.haulTarget(e)
	if !ok {
		w.clearJob(e)
		return
	}
	w.board.releaseClean(e.Target, e.ID) // the tile is clean; stop holding it
	e.Target, e.clean, e.Progress = dest, cleanHaul, 0
}

// gatherRefuse moves as much of a tile's refuse into the colonist's inventory
// as will fit, taking bodies before stains (a corpse is the more urgent eyesore
// and the more likely to be what the colonist came for). A colonist's body is
// only picked up when there is an incinerator to take it to (burn). Biomatter
// is gathered as community work, so the cargo record marks it the colony's.
// A tile it cannot empty in one trip stays claimed-free for the next cleaner.
func (w *World) gatherRefuse(e *Entity, p Point, burn bool) {
	corpses, viscera := 0, 0
	for _, kind := range corpseKinds {
		if kind == ColonistCorpse && !burn {
			continue
		}
		for w.corpsesOfAt(p, kind) > 0 && e.Inventory.Add(kind, 1) {
			w.takeCorpse(p, kind)
			corpses++
			if kind.isBiomatter() {
				e.cargo[kind] = Community
			}
		}
	}
	for w.goreAt(p) > 0 && e.Inventory.Add(Viscera, 1) {
		w.takeGore(p)
		viscera++
		e.cargo[Viscera] = Community
	}
	if corpses+viscera == 0 {
		return
	}
	w.remember(e, event(EvtCleanedRefuse, "Cleaned up %s at (%d, %d).",
		refusePhrase(corpses, viscera), p.X, p.Y))
}

// jobCleanHaul carries a gathered load where it goes. At a scumhouse it
// unloads the biomatter, then carries on to the incinerator with whatever is
// left; at the incinerator it burns the lot.
func (w *World) jobCleanHaul(e *Entity) {
	if !carryingRefuse(e) {
		w.clearJob(e) // nothing left to deliver
		return
	}
	switch t := w.TerrainAt(e.Target); {
	case t == Scumhouse && len(biomatterStacks(e)) > 0:
	case t == Incinerator:
	default:
		dest, ok := w.haulTarget(e)
		if !ok {
			w.clearJob(e) // the load stays carried until somewhere can take it
			return
		}
		e.Target, e.Progress = dest, 0
	}
	arrived, ok := w.travelTo(e, e.Target)
	if !ok {
		w.clearJob(e)
		return
	}
	if !arrived {
		e.State = Hauling
		return
	}
	if w.TerrainAt(e.Target) == Scumhouse {
		c := w.storageContainers[e.Target]
		if c == nil || !w.deliverBiomatter(e, c) {
			e.Target = Point{-1, -1} // full after all: pick again next tick
			return
		}
		if !carryingRefuse(e) {
			w.clearJob(e)
		}
		return
	}
	e.State = Cleaning
	e.Progress++
	if e.Progress >= scaleTicks(w.cfg.IncinerateTicks, e.workScale) {
		w.incinerate(e)
		w.clearJob(e)
	}
}

// incinerate destroys everything burnable a colonist is carrying. The load goes
// in whole rather than piece by piece: one trip, one burn.
func (w *World) incinerate(e *Entity) {
	corpses := 0
	for _, kind := range corpseKinds {
		corpses += e.Inventory.RemoveAll(kind)
		e.cargo[kind] = Owner{}
	}
	viscera := e.Inventory.RemoveAll(Viscera)
	e.cargo[Viscera] = Owner{}
	if corpses+viscera == 0 {
		return
	}
	phrase := refusePhrase(corpses, viscera)
	w.remember(e, event(EvtIncineratedRefuse, "Burned %s in the incinerator.", phrase))
	w.log.add(fmt.Sprintf("%s incinerates %s.", e.displayName(), phrase))
}

// refusePhrase describes a load of refuse for a log line or a memory.
func refusePhrase(corpses, viscera int) string {
	switch {
	case corpses > 0 && viscera > 0:
		return fmt.Sprintf("%s and %s", bodyPhrase(corpses), visceraPhrase(viscera))
	case corpses > 0:
		return bodyPhrase(corpses)
	default:
		return visceraPhrase(viscera)
	}
}

func bodyPhrase(n int) string {
	if n == 1 {
		return "a body"
	}
	return fmt.Sprintf("%d bodies", n)
}

func visceraPhrase(n int) string {
	if n == 1 {
		return "some viscera"
	}
	return fmt.Sprintf("%d splatters of viscera", n)
}

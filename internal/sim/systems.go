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
		w.planRooms()
		w.nextPlanTick = w.tick + planInterval
	}
	w.rebuildBuildTiles() // reflect this tick's completions and any new project
	w.runDirector()       // fire any scripted occurrence whose tick has arrived
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
		w.addCorpse(e.Pos)
		w.remove(e.ID, "starved")
		w.log.add(fmt.Sprintf("%s starved to death.", e.displayName()))
		return
	}

	// Previously accumulated affect decays before this turn's observations, so a
	// new event can influence arbitration immediately without decaying first.
	w.decayAffect(e)

	// Room connectivity is tracked regardless of what the colonist is doing this
	// tick — a sleeping or resting colonist sealed in by construction elsewhere
	// must still notice, the same way uranium exposure or starvation do.
	w.updateDisconnected(e)

	// A stable resting/sleeping colonist has no perception product to ingest when
	// no creature or gore is nearby. The fast path still performs threat, fatal
	// need, facility, and uranium checks before it can return.
	if !w.alwaysArbitrate && w.tryCognitionFastPath(e) {
		return
	}

	// Record the first sighting of each nearby creature. Seeing is edge-triggered:
	// a colonist fleeing for many ticks remembers one encounter, not one memory
	// per tick.
	w.observeNearby(e)

	// Uranium does its work regardless of what the colonist is doing — fleeing,
	// eating, or digging the vein itself — so the dose is taken before any
	// branch below can return. See mutation.go.
	w.applyUraniumExposure(e)
	w.syncCognitionDeadlines(e)
	w.runCognition(e)
}

func (w *World) runCognition(e *Entity) {
	threat, _ := w.nearestAlien(e.Pos, w.cfg.FleeRadius)
	shouldThink := w.alwaysArbitrate || e.mindDirty || w.tick >= e.nextThinkTick ||
		!w.currentFocusEligible(e, threat)
	selected := cachedFocusCandidate(e, threat)
	if shouldThink {
		var candidates [numFocusKinds]FocusCandidate
		selected = w.chooseFocus(e, &candidates)
		e.mindDirty = false
		e.nextThinkTick = w.nextCognitionTick(e)
	}
	if selected.Kind != e.focus {
		w.clearJob(e)
		e.focus = selected.Kind
		e.focusSince = w.tick
		w.markMindDirty(e)
	}
	w.runFocus(e, selected)
}

// syncCognitionDeadlines applies only transitions whose cached boundary has
// arrived. Fatal needs are also read every tick as a safety belt, independently
// of the cache. Stimulus expiry remains exact at ExpiresAt.
func (w *World) syncCognitionDeadlines(e *Entity) {
	for n := NeedKind(0); n < numNeeds; n++ {
		crossing := e.nextNeedPhaseTick[n]
		if w.cfg.Needs[n].Fatal || crossing > 0 && w.tick >= crossing {
			w.syncNeedPhase(e, n)
		}
	}
	if e.nextStimulusExpiry > 0 && w.tick >= e.nextStimulusExpiry {
		w.expireStimuli(e)
	}
}

func (w *World) hasGoreNearby(e *Entity) bool {
	r := w.cfg.GoreSightRadius
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			p := e.Pos.Add(x, y)
			if w.InBounds(p) && w.goreAt(p) > 0 {
				return true
			}
		}
	}
	return false
}

func (w *World) tryCognitionFastPath(e *Entity) bool {
	restingIdle := e.focus == FocusIdle && e.Job == JobNone && e.resting && w.tick < e.wakeTick
	sleeping := e.focus == FocusSleep && e.Job == JobUse && e.Need == NeedSleep && !e.carrying &&
		e.useFacilitySet && e.Pos.Adjacent(e.useFacility) &&
		w.TerrainAt(e.useFacility) == w.cfg.Needs[NeedSleep].Facility
	if (!restingIdle && !sleeping) || e.mindDirty || w.tick >= e.nextThinkTick ||
		len(e.seen) != 0 || e.seeingGore {
		return false
	}
	if threat, ok := w.nearestAlien(e.Pos, w.cfg.FleeRadius); ok && threat != nil {
		return false
	}
	if mouse, ok := w.nearestMouse(e.Pos, w.cfg.ColonistStompRadius); ok && mouse != nil {
		return false
	}
	if w.hasGoreNearby(e) {
		return false
	}

	// Exposure can mutate a resting colonist and dirty cognition; it is never
	// skipped merely because observation/arbitration are cached.
	w.applyUraniumExposure(e)
	w.syncCognitionDeadlines(e)
	if e.mindDirty || w.tick >= e.nextThinkTick {
		w.runCognition(e)
		return true
	}

	if restingIdle {
		e.State = Idle
		return true
	}
	// In-place sleep has no path, target search, or claim machinery to run. Its
	// only changing score strengthens the incumbent; nextCognitionTick caps this
	// path at every competing need boundary and the fallback deadline.
	e.State = Sleeping
	e.Progress++
	if e.Progress >= w.cfg.Needs[NeedSleep].UseTicks {
		w.finishUse(e, w.cfg.Needs[NeedSleep])
	}
	return true
}

func (w *World) runFocus(e *Entity, selected FocusCandidate) {
	switch selected.Kind {
	case FocusEat, FocusRelieve, FocusSocialize, FocusSleep:
		need, _ := needForFocus(selected.Kind)
		w.runNeedFocus(e, need)
	case FocusFlee:
		threat := w.entities[selected.Threat]
		if threat == nil || !threat.Alive() {
			w.finishFocus(e)
			return
		}
		e.resting = false
		e.State = Fleeing
		w.fleeStep(e, threat.Pos)
	case FocusFight:
		threat := w.entities[selected.Threat]
		weapon := bestWeapon(e.Inventory)
		if threat == nil || !threat.Alive() || weapon == ItemNone {
			w.finishFocus(e)
			return
		}
		e.resting = false
		w.fightAlien(e, threat, weapon)
	case FocusWork:
		if e.Job == JobNone {
			w.assignWorkJob(e)
		}
		if e.Job == JobNone {
			w.finishFocus(e)
			w.runIdleFocus(e)
			return
		}
		e.resting = false
		w.runJob(e)
	case FocusEscape:
		if e.Job != JobDemolish && !w.assignDemolish(e) {
			// Nothing reachable to break through (the pocket is bounded by rock,
			// not a built wall) — fall back rather than spinning on this focus
			// every tick with nothing to execute.
			w.finishFocus(e)
			w.runIdleFocus(e)
			return
		}
		e.resting = false
		w.runJob(e)
	default:
		w.runIdleFocus(e)
	}
}

func (w *World) finishFocus(e *Entity) {
	w.clearJob(e)
	e.focus = FocusIdle
	e.focusSince = w.tick
	w.markMindDirty(e)
}

func (w *World) runNeedFocus(e *Entity, need NeedKind) {
	e.resting = false
	if need == NeedSocial {
		// A conversation already under way is the executor for this focus. Never
		// restart it and reset the pair's shared timer.
		if _, ok := w.talkPartner(e); ok {
			w.runJob(e)
			return
		}
		w.clearJob(e)
		if w.tryStartTalk(e, true) {
			w.runJob(e)
		} else {
			e.State = Idle
		}
		return
	}

	handlingNeed := (e.Job == JobUse && e.Need == need) ||
		(e.Job == JobBuild && (e.BuildKind == w.cfg.Needs[need].Facility || e.task != nil))
	if handlingNeed {
		w.runJob(e)
		return
	}

	spec := w.cfg.Needs[need]
	field := w.facilityField(spec.Facility)
	existingReachable := field != nil && field.at(e.Pos) >= 0
	needMore := w.plannedFacilities(spec.Facility) < w.desiredFacilities(w.countKind(Colonist))
	var task *buildTask
	var hasTask bool
	if needMore || !existingReachable {
		task, hasTask = w.claimNearestTaskProviding(e.Pos, e.ID, spec.Facility)
	}

	w.clearJob(e)
	switch {
	case needMore && hasTask:
		w.assignTask(e, task)
	case existingReachable:
		e.Job, e.Need, e.Progress = JobUse, need, 0
		e.useFacility, e.useFacilitySet = w.chooseFacility(e, spec.Facility), true
	case hasTask:
		w.assignTask(e, task)
	case !w.reachableFacilityConstruction(e.Pos, spec.Facility):
		if spot, ok := w.findBuildSpot(e.Pos, 20); ok {
			w.assignBuild(e, spec.Facility, spot)
		}
	default:
		// All matching tasks are claimed. Wait rather than taking unrelated
		// work and losing the focus that explains why this colonist is idle.
		e.State = Idle
		if w.idleWouldBlock(e.Pos) {
			w.stepAside(e)
		} else {
			w.wanderStep(e)
		}
		return
	}
	if e.Job != JobNone {
		w.runJob(e)
	}
}

func (w *World) runIdleFocus(e *Entity) {
	// An opportunistic conversation is idle execution, so preserve it until its
	// shared timer completes.
	if e.Job == JobTalk {
		w.runJob(e)
		return
	}
	if e.resting && w.tick < e.wakeTick {
		e.State = Idle
		return
	}

	// Exact work discovery belongs in execution rather than candidate scoring.
	// Finding a job transitions to work immediately; finding none enters the
	// existing bounded resting path.
	w.assignWorkJob(e)
	if e.Job != JobNone {
		e.focus = FocusWork
		e.focusSince = w.tick
		w.markMindDirty(e)
		e.resting = false
		w.runJob(e)
		return
	}
	if w.idleWouldBlock(e.Pos) {
		e.resting = false
		e.State = Idle
		w.stepAside(e)
		return
	}
	if w.tryStartTalk(e, false) {
		w.runJob(e)
		return
	}
	if w.stompNearbyMouse(e) {
		return
	}
	e.resting = true
	e.wakeTick = w.tick + e.restTicks
	e.State = Idle
}

// observeNearby records the first sighting of each nearby creature (edge-
// triggered on e.seen, so a colonist fleeing for many ticks remembers one
// encounter, not one memory per tick) and, via observeGore, the first sight
// of gore in the same visit.
func (w *World) observeNearby(e *Entity) {
	hadThreat := false
	for id := range e.seen {
		if seen := w.entities[id]; seen != nil && seen.Kind == Alien {
			hadThreat = true
			break
		}
	}
	visible := make(map[EntityID]bool)
	seesThreat := false
	for _, id := range w.entityIDsSorted() {
		other := w.entities[id]
		if other == e || !other.Alive() {
			continue
		}
		kind, radius := other.Kind, 0
		switch kind {
		case Alien:
			radius = w.cfg.FleeRadius
		case Mouse:
			radius = w.cfg.ColonistStompRadius
		default:
			continue
		}
		if e.Pos.Chebyshev(other.Pos) > radius {
			continue
		}
		visible[other.ID] = true
		if kind == Alien {
			seesThreat = true
		}
		if e.seen[other.ID] {
			if kind == Alien {
				// This is a refresh of ongoing context, not another life-event
				// occurrence; memory remains edge-triggered.
				w.addStimulus(e, LifeEvent{Kind: EvtSawAlien, Source: other.ID})
			}
			continue
		}
		evtKind := EvtSawMouse
		if kind == Alien {
			evtKind = EvtSawAlien
		}
		w.remember(e, eventFrom(evtKind, other.ID, "Saw %s #%d.", kind, other.ID))
	}
	e.seen = visible
	if hadThreat != seesThreat {
		w.markMindDirty(e)
	}

	w.observeGore(e)
}

// observeGore is observeNearby's counterpart for the environment rather than
// other entities. Unlike e.seen it is a single edge-triggering flag, not a
// per-tile map: "in sight of gore" is one memory-worthy fact whether it's one
// stained tile or a whole battlefield, not one memory per tile.
func (w *World) observeGore(e *Entity) {
	seeing := false
	r := w.cfg.GoreSightRadius
outer:
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			p := e.Pos.Add(x, y)
			if w.InBounds(p) && w.goreAt(p) > 0 {
				seeing = true
				break outer
			}
		}
	}
	if seeing && !e.seeingGore {
		w.remember(e, event(EvtSawGore, "Saw the aftermath of violence nearby."))
	}
	e.seeingGore = seeing
}

// stompNearbyMouse lets a colonist with nothing pressing to do chase down and
// crush a mouse it notices. Stomping is an idle whim, not work: colonistTurn has
// already ruled out threats, urgent needs, and available jobs before this runs.
// A stomp is instantly fatal to the tiny mouse. Returns whether the colonist
// spent its tick on the hunt (closing in or stomping).
func (w *World) stompNearbyMouse(e *Entity) bool {
	prey, ok := w.nearestMouse(e.Pos, w.cfg.ColonistStompRadius)
	if !ok {
		return false
	}
	e.resting = false
	e.State = Stomping
	if e.Pos.Adjacent(prey.Pos) {
		w.stomp(e, prey)
		return true
	}
	// Close in on the pest. If it cannot be reached on foot (walled off, or the
	// colonist is wedged), drop the whim and let the caller rest instead.
	if _, ok := w.travelTo(e, prey.Pos); !ok {
		return false
	}
	return true
}

// stomp crushes a mouse underfoot. A stomp is always fatal to the mouse and
// leaves it behind as gore. Any other colonist close enough to have noticed
// the mouse remembers seeing it happen.
func (w *World) stomp(colonist, mouse *Entity) {
	witnesses := w.colonistsWithin(mouse.Pos, w.cfg.ColonistStompRadius, colonist.ID)
	w.addGore(mouse.Pos)
	w.addCorpse(mouse.Pos) // a crushed pest still has to be carried off
	w.remove(mouse.ID, fmt.Sprintf("crushed by %s", colonist.displayName()))
	w.remember(colonist, event(EvtCrushedMouse, "Crushed mouse #%d.", mouse.ID))
	for _, wit := range witnesses {
		w.remember(wit, event(EvtWitnessedMouseCrushed, "Watched a colonist crush mouse #%d.", mouse.ID))
	}
	w.log.add(fmt.Sprintf("Colonist #%d stomps mouse #%d.", colonist.ID, mouse.ID))
}

// idleWouldBlock reports whether an idle colonist resting at p would get in the
// colony's way: p is a facility's access tile (blocking users) or a pending
// build tile (blocking construction).
func (w *World) idleWouldBlock(p Point) bool {
	return w.onFacilityAccess(p) || w.onPendingBuild(p)
}

// onFacilityAccess reports whether p is next to a facility colonists walk to —
// any need-satisfying structure, or the incinerator a hauler has to reach — so
// an idle colonist standing there would block others from using it.
func (w *World) onFacilityAccess(p Point) bool {
	for _, d := range neighbors8 {
		t := w.TerrainAt(p.Add(d.X, d.Y))
		if t == Incinerator {
			return true
		}
		for i := 0; i < int(numNeeds); i++ {
			if w.cfg.Needs[i].Facility != Rock && w.cfg.Needs[i].Facility == t {
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

// claimNearestMine claims the nearest unclaimed frontier rock whose complete
// yield the colonist can carry and which is reachable from their room.
func (w *World) claimNearestMine(e *Entity) (Point, bool) {
	room := w.roomOf(e.Pos)
	if room == 0 {
		return Point{}, false
	}
	var best Point
	found := false
	bestDist := 1 << 30
	for p := range w.board.frontier {
		if w.board.isClaimed(p) || !w.frontierReachable(p, room) ||
			!e.Inventory.CanAddAll(miningYield(w.TileAt(p))...) {
			continue
		}
		if d := e.Pos.Chebyshev(p); !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	if found {
		w.board.claimMine(best, e.ID)
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
		if w.board.isFrontier(n) && !w.board.isClaimed(n) &&
			e.Inventory.CanAddAll(miningYield(w.TileAt(n))...) {
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
	hadJob := e.Job != JobNone
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
	case JobClean:
		if e.clean == cleanGather {
			w.board.releaseClean(e.Target, e.ID) // reopen the mess for someone else
		}
		e.clean = cleanGather
	}
	e.Job, e.Progress, e.partner = JobNone, 0, 0
	e.useFacility, e.useFacilitySet, e.carrying = Point{}, false, false
	e.clearPath()
	if hadJob {
		w.markMindDirty(e)
	}
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
	case JobTalk:
		w.jobTalk(e)
	case JobClean:
		w.jobClean(e)
	case JobStore:
		w.jobStore(e)
	case JobDemolish:
		w.jobDemolish(e)
	default:
		e.State = Idle
		w.wanderStep(e)
	}
}

// ---- Talking -----------------------------------------------------------------

// tryStartTalk lets an idle colonist strike up a conversation with a nearby free
// colonist, committing both to JobTalk. It reports whether a conversation began.
// TalkChance gates it (0 disables talking entirely, and the sim then plays as it
// did before the activity existed).
func (w *World) tryStartTalk(e *Entity, forced bool) bool {
	if !forced && (w.cfg.TalkChance <= 0 || w.rng.Intn(100) >= w.cfg.TalkChance) {
		return false
	}
	partner, ok := w.nearestMatch(e.Pos, w.cfg.TalkRadius, func(o *Entity) bool {
		return o.Kind == Colonist && o.ID != e.ID && w.availableToTalk(o)
	})
	if !ok {
		return false
	}
	w.beginTalk(e, partner)
	return true
}

// availableToTalk reports whether a colonist is free to be pulled into a chat:
// idle with no committed job, not fleeing or seeking a facility, and not parked
// on a tile others need clear. A candidate whose own most urgent need is
// social is still available — otherwise two colonists who both urgently need
// company can never talk to each other, since each disqualifies the other as
// a partner, and social need sits permanently pinned at its ceiling in any
// colony busy enough that nobody is ever fully need-free.
func (w *World) availableToTalk(o *Entity) bool {
	if o.Job != JobNone || o.State == Fleeing {
		return false
	}
	if need, urgent := w.mostUrgentNeed(o); urgent && need != NeedSocial {
		return false
	}
	return !w.idleWouldBlock(o.Pos)
}

// beginTalk commits two colonists to a mutual conversation.
func (w *World) beginTalk(a, b *Entity) {
	a.resting, b.resting = false, false
	a.Job, a.partner, a.Progress = JobTalk, b.ID, 0
	b.Job, b.partner, b.Progress = JobTalk, a.ID, 0
	for _, e := range []*Entity{a, b} {
		if w.needLevel(e, NeedSocial) >= w.cfg.Needs[NeedSocial].SeekAt {
			e.focus = FocusSocialize
		} else {
			e.focus = FocusIdle
		}
		e.focusSince = w.tick
		w.markMindDirty(e)
	}
	a.clearPath()
	b.clearPath()
	a.State, b.State = Talking, Talking
}

// talkPartner returns the colonist e is in a conversation with, if both sides
// still claim each other. A conversation is only real while it is mutual: one
// side being pulled away (a fatal need, a threat, a torn-down claim) ends it for
// both, which is what stops a colonist chatting with someone who has wandered
// off to eat.
func (w *World) talkPartner(e *Entity) (*Entity, bool) {
	if e.Job != JobTalk {
		return nil, false
	}
	p := w.entities[e.partner]
	if p == nil || !p.Alive() || p.Kind != Colonist || p.Job != JobTalk || p.partner != e.ID {
		return nil, false
	}
	return p, true
}

// jobTalk runs one tick of a conversation: partners converge, then chat for
// TalkTicks before the pair's affinity rises. The two must claim each other
// mutually or the talk is abandoned. To avoid chasing each other, the higher-ID
// partner walks over while the lower-ID one waits; the lower-ID partner also
// hosts the shared timer so a conversation is credited once, not once per side.
func (w *World) jobTalk(e *Entity) {
	p, ok := w.talkPartner(e)
	if !ok {
		w.clearJob(e)
		return
	}
	if w.idleWouldBlock(e.Pos) {
		// Never hold a chat on a tile others need (a facility's access tile or a
		// pending build tile); drop it and step aside on the next idle turn.
		w.clearJob(e)
		return
	}
	if !e.Pos.Adjacent(p.Pos) {
		if e.ID > p.ID {
			if _, ok := w.travelTo(e, p.Pos); !ok {
				w.clearJob(e)
				return
			}
			e.State = Moving
		} else {
			e.State = Talking // wait in place for the partner to arrive
		}
		return
	}
	e.State = Talking
	if e.ID < p.ID { // host drives the shared timer
		e.Progress++
		if e.Progress >= w.cfg.TalkTicks {
			w.finishTalk(e, p)
			w.resetNeed(e, NeedSocial)
			w.resetNeed(p, NeedSocial)
			w.clearJob(p)
			w.clearJob(e)
		}
	}
}

// finishTalk applies a completed conversation's outcome: it rolls the chat's
// quality, shifts the pair's affinity (exacerbating its existing valence, with
// diminishing returns), and records each participant's memory of it with the
// signed affect outcome that this conversation earned — a company term (how it
// feels to spend time with the other), a quality term, and each participant's
// social-fatigue penalty (noteConversation), which must still be called exactly
// once because it advances the rolling window. The per-occurrence outcome
// travels on LifeEvent and is deterministically converted to charge/grip in the
// ingestion funnel. See docs/memories.md.
func (w *World) finishTalk(a, b *Entity) {
	existing := w.mutualAffinity(a.ID, b.ID)
	quality := w.rollTalkQuality(existing)
	// Affinity is credited per direction rather than through addAffinity: the
	// conversation itself moves both sides by the same step, but a trait-driven
	// bonus is one-sided (a Mutant-Lover's warmth toward a mutant is not
	// returned in kind), and only a per-direction credit can express that.
	step := w.talkAffinityDelta(existing, quality)
	w.bumpAffinity(a.ID, b.ID, step+w.mutantAffinityBonus(a, b))
	w.bumpAffinity(b.ID, a.ID, step+w.mutantAffinityBonus(b, a))
	outcome := w.talkMoodDelta(quality, existing)
	w.remember(a, eventOutcome(EvtConversation, outcome+w.noteConversation(a), "Had a conversation with %s.", b.displayName()))
	w.remember(b, eventOutcome(EvtConversation, outcome+w.noteConversation(b), "Had a conversation with %s.", a.displayName()))
}

// assignWorkJob picks something productive to do: help build a planned project
// first (life-support rooms), otherwise mine the frontier. Leaves JobNone if
// nothing suitable is reachable.
func (w *World) assignWorkJob(e *Entity) {
	// A colonist whose load blocks further mining first unloads into reachable
	// storage. If no chest exists yet, help build the project that will provide
	// one rather than claiming unrelated work and leaving the storage job stalled.
	if w.inventoryNeedsStorage(e) {
		if w.tryAssignStore(e) {
			return
		}
		if task, ok := w.claimNearestTaskProviding(e.Pos, e.ID, Storage); ok {
			w.assignTask(e, task)
			return
		}
		e.Job = JobNone
		return
	}
	// Collaborate on planned construction (facility rooms, etc.): claim the
	// nearest reachable task from the shared project pool.
	if task, ok := w.claimNearestTask(e.Pos, e.ID); ok {
		w.assignTask(e, task)
		return
	}
	// Tidy up before digging more: refuse is finite and demoralizing (every
	// colonist that walks past a splatter takes the EvtSawGore hit), while the
	// mining frontier is effectively endless. Cleaning placed after mining
	// would therefore never come up at all. It still sits behind construction:
	// life support outranks housekeeping.
	if w.tryAssignClean(e) {
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
		} else if target, ok := w.claimNearestMine(e); ok {
			w.assignMineTarget(e, target)
			return
		}
	}
	e.Job = JobNone
}

// inventoryNeedsStorage is the work-pressure trigger for unloading. Raw rock is
// part of every mining yield, so inability to fit one more means the current
// load blocks all further excavation. A load containing only weapons/refuse is
// not diverted: weapons stay equipped and refuse belongs in the incinerator.
func (w *World) inventoryNeedsStorage(e *Entity) bool {
	return !e.Inventory.CanAdd(RawRock, 1) && len(e.Inventory.storableStacks()) > 0
}

// colonyNeedsStorage reports whether any blocked colonist lacks a reachable
// chest that can take its complete material load. An existing storage project
// suppresses duplicates while its dig/wall phases are still under construction.
func (w *World) colonyNeedsStorage() bool {
	if w.projectFacilityTasks(Storage) > 0 {
		return false
	}
	for _, e := range w.entities {
		if e.Kind != Colonist || !e.Alive() || !w.inventoryNeedsStorage(e) {
			continue
		}
		if _, ok := w.chooseStorage(e, e.Inventory.storableStacks()); !ok {
			return true
		}
	}
	return false
}

func (w *World) tryAssignStore(e *Entity) bool {
	target, ok := w.chooseStorage(e, e.Inventory.storableStacks())
	if !ok {
		return false
	}
	e.Job, e.Target, e.Progress = JobStore, target, 0
	return true
}

// chooseStorage picks the nearest position-stable reachable chest that can take
// the complete load. Room reachability is exact for connected floor, and ties
// break by position to preserve seeded determinism.
func (w *World) chooseStorage(e *Entity, stacks []ItemStack) (Point, bool) {
	room := w.roomOf(e.Pos)
	var best Point
	bestDist := 1 << 30
	found := false
	for p, container := range w.storageContainers {
		if !container.Inventory.CanAddAll(stacks...) || !w.taskReachable(p, room) {
			continue
		}
		d := e.Pos.Chebyshev(p)
		if !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	return best, found
}

func (w *World) jobStore(e *Entity) {
	container := w.storageContainers[e.Target]
	stacks := e.Inventory.storableStacks()
	if container == nil || len(stacks) == 0 || !container.Inventory.CanAddAll(stacks...) {
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
	if !container.Inventory.AddAll(stacks...) {
		w.clearJob(e)
		return
	}
	e.Inventory.removeStorable()
	e.State = Storing
	w.log.add(fmt.Sprintf("%s unloads materials into storage at (%d, %d).",
		e.displayName(), e.Target.X, e.Target.Y))
	w.clearJob(e)
}

// plannedFacilities counts a facility kind that already exists plus those a
// colonist is currently building, so the colony converges on the desired number
// instead of every idle colonist starting one at the same instant. The
// in-progress count comes from the board's O(1) counter, not an entity scan.
func (w *World) plannedFacilities(kind Terrain) int {
	return w.countTerrain(kind) + w.board.inProgress(kind) + w.projectFacilityTasks(kind)
}

// desiredFacilities is how many of each need-satisfying structure (pods,
// toilets, bunks) the colony wants for a given headcount (at least one).
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
		if !e.Inventory.CanAddAll(miningYield(w.TileAt(e.Target))...) {
			w.clearJob(e)
			return
		}
		if e.Pos.Adjacent(e.Target) {
			e.State = Mining
			e.Progress++
			if e.Progress >= scaleTicks(w.cfg.MineTicks, e.workScale) {
				// Award the complete composition-dependent yield before changing
				// terrain so limited inventory can never make material disappear.
				if !e.Inventory.AddAll(miningYield(w.TileAt(e.Target))...) {
					w.clearJob(e)
					return
				}
				w.SetTerrain(e.Target, Floor) // TileChanged drops it from the frontier
				w.remember(e, event(EvtFinishedMining, "Finished mining at (%d, %d).", e.Target.X, e.Target.Y))
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

// ---- Escaping a sealed room ---------------------------------------------------

// updateDisconnected tracks how many consecutive ticks a colonist's own room
// has been cut off from the colony's main connected network (w.mainRoom; see
// rooms.go). It runs every tick regardless of what else the colonist is doing,
// the same way starvation and uranium exposure do, so a colonist sleeping or
// tending a facility inside a pocket that construction elsewhere just sealed
// still notices. See docs/escape.md.
func (w *World) updateDisconnected(e *Entity) {
	room := w.roomOf(e.Pos)
	if room == 0 || room == w.mainRoom {
		if e.disconnectedTicks > 0 {
			w.markMindDirty(e) // just reconnected; worth reconsidering focus now
		}
		e.disconnectedTicks = 0
		return
	}
	e.disconnectedTicks++
	if e.disconnectedTicks == w.cfg.EscapeGraceTicks {
		w.markMindDirty(e) // just became eligible; do not wait for the next think tick
	}
}

// assignDemolish commits a colonist to breaking down the nearest reachable
// wall bounding its own (cut-off) room. Reports whether one was found.
func (w *World) assignDemolish(e *Entity) bool {
	wall, ok := w.nearestEscapeWall(e.Pos)
	if !ok {
		return false
	}
	e.Job, e.Target, e.Progress = JobDemolish, wall, 0
	return true
}

// nearestEscapeWall finds the closest Wall tile bounding from's own room, by
// walking distance within that room rather than a global scan: a sealed
// pocket is by definition small, so this stays cheap exactly where it matters
// (a colony-wide scan for one trapped colonist would not). Returns false if
// the room is bounded entirely by solid rock rather than any built wall — a
// natural cavern separation JobDemolish cannot do anything about.
func (w *World) nearestEscapeWall(from Point) (Point, bool) {
	room := w.roomOf(from)
	if room == 0 {
		return Point{}, false
	}
	seen := map[Point]bool{from: true}
	queue := []Point{from}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range neighbors8 {
			n := p.Add(d.X, d.Y)
			if !w.InBounds(n) || seen[n] {
				continue
			}
			seen[n] = true
			if w.TerrainAt(n) == Wall {
				return n, true
			}
			if w.Walkable(n) && w.roomOf(n) == room {
				queue = append(queue, n)
			}
		}
	}
	return Point{}, false
}

// jobDemolish walks to the wall claimed by assignDemolish and breaks it down
// over DemolishTicks, converting it back to Floor — mirroring jobMine's dig,
// but reversing a wall instead of clearing rock. Reconnecting is implicit:
// refreshSpatial folds the new Floor tile in at the end of this tick, and
// updateDisconnected notices the room is whole again on the next.
func (w *World) jobDemolish(e *Entity) {
	if w.TerrainAt(e.Target) != Wall {
		w.clearJob(e) // reconnected some other way, or someone else broke it first
		return
	}
	if e.Pos.Adjacent(e.Target) {
		e.State = Demolishing
		e.Progress++
		if e.Progress >= scaleTicks(w.cfg.DemolishTicks, e.workScale) {
			w.SetTerrain(e.Target, Floor)
			w.log.add(fmt.Sprintf("Colonist #%d breaks through a wall to escape a sealed room.", e.ID))
			w.clearJob(e)
		}
		return
	}
	if _, ok := w.travelTo(e, e.Target); !ok {
		w.clearJob(e)
		return
	}
	e.State = Moving
}

func (w *World) jobBuild(e *Entity) {
	// A dig task (BuildKind Floor) works rock down to floor; every other kind
	// builds atop existing floor. Anything else at the target — already
	// finished, or changed to something unexpected — ends the job.
	prereq := Floor
	if e.BuildKind == Floor {
		prereq = Rock
	}
	if w.TerrainAt(e.Target) != prereq {
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
	if e.Progress < scaleTicks(w.buildTicks(e.BuildKind), e.workScale) {
		return
	}
	if e.BuildKind == Floor {
		// Digging: award the resource like ordinary mining, before changing
		// the terrain, so a full inventory can never make it disappear. If
		// the colonist can't carry more, release the task rather than
		// finishing it emptyhanded — someone else (or this colonist once
		// unloaded) picks it up.
		if !e.Inventory.AddAll(miningYield(w.TileAt(e.Target))...) {
			w.clearJob(e)
			return
		}
		w.SetTerrain(e.Target, Floor)
		w.remember(e, event(EvtClearedRock, "Cleared rock for a room at (%d, %d).", e.Target.X, e.Target.Y))
		w.clearJob(e)
		return
	}
	w.SetTerrain(e.Target, e.BuildKind)
	w.noteBuild(e.BuildKind)
	w.remember(e, event(EvtFinishedConstruction, "Finished construction of %s at (%d, %d).",
		e.BuildKind, e.Target.X, e.Target.Y))
	w.clearJob(e) // endBuild decrements the in-progress counter
}

// chooseFacility assigns a concrete facility to a need. Facilities are ranked
// by walkable distance, with crowded approaches excluded when another reachable
// facility exists. The assignment is retained on the entity for the whole use
// job, so a user never ping-pongs between queues as their counts change.
func (w *World) chooseFacility(e *Entity, kind Terrain) Point {
	// The distance cells are reused across calls (and across ticks) via a
	// generation stamp, the same trick flowField uses (and the same flowCell),
	// so a call costs O(reachable area) rather than allocating and zeroing a
	// Width*Height slice every time — critical on a large map, where the
	// walkable area a colonist can actually reach is a tiny fraction of the
	// grid. The cells are paged for the same reason: only reachable tiles are
	// ever stamped. See pagedgrid.go.
	w.facilityGen++
	gen := w.facilityGen
	cells := &w.facilityCells
	reached := func(p Point) (int32, bool) {
		c := cells.at(p.X, p.Y)
		return c.dist, c.gen == gen
	}
	cells.set(e.Pos.X, e.Pos.Y, flowCell{gen: gen, dist: 0})
	queue := append(w.facilityQueue[:0], e.Pos)
	// Layered like flowField.rebuild, and off the same interior-page fast
	// path: uniform-cost BFS visits in non-decreasing distance order, so the
	// depth is the layer, and a node away from a page edge reaches all eight
	// neighbours through one page lookup.
	pd, levelEnd := int32(0), len(queue)
	for head := 0; head < len(queue); head++ {
		if head == levelEnd {
			pd++
			levelEnd = len(queue)
		}
		p := queue[head]
		page := cells.interiorPage(p.X, p.Y)
		for _, d := range neighbors8 {
			n := p.Add(d.X, d.Y)
			if !w.Walkable(n) {
				continue
			}
			np := page
			if np == nil {
				np = cells.pageAtAlloc(n.X, n.Y)
			}
			c := &np[offset(n.X, n.Y)]
			if c.gen == gen {
				continue
			}
			c.gen, c.dist = gen, pd+1
			queue = append(queue, n)
		}
	}
	w.facilityQueue = queue

	// Facilities of this kind are few even on a huge map, so iterate the
	// tracked set (see World.facilityTiles) instead of scanning every tile.
	facilities := w.facilityTiles[kind]

	best, bestDist := Point{}, int32(^uint32(0)>>1)
	for fac := range facilities {
		accessible := false
		congested := false
		for _, d := range neighbors8 {
			access := fac.Add(d.X, d.Y)
			if !w.Walkable(access) {
				continue
			}
			if _, ok := reached(access); !ok {
				continue
			}
			accessible = true
			if w.entityAt(access) != nil {
				congested = true
			}
			// A committed user in the approach counts as a queue even when
			// the access tile itself is currently free. This — not mere
			// nearby foot traffic — is what "congested" means: an earlier
			// version also flagged a facility whenever any other colonist
			// stood within one tile of any of its access tiles, whatever
			// that colonist was actually doing. Facilities are packed one
			// tile apart in a room (see construction.md), so their access
			// neighborhoods overlap; in a merely busy room — colonists
			// resting, chatting, walking through, using the facility next
			// door — that overbroad check could flag every facility in it as
			// "congested" at once, so this function's whole point (spread
			// users across reachable facilities) gave up and fell back to
			// "nearest for everyone," funneling a crowd onto one facility
			// while others sat genuinely idle beside it.
			queueCount := 0
			for _, other := range w.entities {
				if other == e || !other.Alive() || other.Kind != Colonist ||
					other.Job != JobUse || !other.useFacilitySet ||
					!other.useFacility.Equal(fac) {
					continue
				}
				queueCount++
			}
			if queueCount >= 1 {
				congested = true
			}
		}
		if !accessible {
			continue
		}
		// Prefer the nearest facility unless its approach is congested. If
		// every reachable option is busy, retain nearest as a fair fallback.
		//
		// d starts at "unreached", not at bestDist: seeding it from the running
		// best made a farther facility score as an exact tie (its own accesses
		// never beat bestDist, so d stayed there) and then win the lessPoint
		// tie-break. Which facility that hit depended on the iteration order of
		// w.facilityTiles -- a map -- so the same seed sent a colonist to a
		// different sink on different runs.
		d := int32(^uint32(0) >> 1)
		for _, n := range neighbors8 {
			access := fac.Add(n.X, n.Y)
			if !w.InBounds(access) {
				continue
			}
			if nd, ok := reached(access); ok && nd < d {
				d = nd
			}
		}
		if congested {
			continue
		}
		if d < bestDist || (d == bestDist && lessPoint(fac, best)) {
			best, bestDist = fac, d
		}
	}
	if bestDist < int32(^uint32(0)>>1) {
		return best
	}
	// If all facilities are congested, choosing the nearest still guarantees
	// progress once its current users leave rather than declaring the need
	// unreachable and starving the colonist.
	bestDist = int32(^uint32(0) >> 1)
	for fac := range facilities {
		for _, d := range neighbors8 {
			access := fac.Add(d.X, d.Y)
			if !w.InBounds(access) {
				continue
			}
			if nd, ok := reached(access); ok &&
				(nd < bestDist || (nd == bestDist && lessPoint(fac, best))) {
				best, bestDist = fac, nd
			}
		}
	}
	return best
}

func (w *World) jobUse(e *Entity) {
	spec := w.cfg.Needs[e.Need]
	if e.carrying {
		w.jobUseCarrying(e, spec)
		return
	}
	field := w.facilityField(spec.Facility)
	if field == nil || field.at(e.Pos) < 0 {
		w.clearJob(e) // no facility of this kind is reachable anymore
		return
	}
	if !e.useFacilitySet || w.TerrainAt(e.useFacility) != spec.Facility {
		e.useFacility, e.useFacilitySet = w.chooseFacility(e, spec.Facility), true
	}
	// Arrived: standing next to a facility of the right kind — use it.
	if e.useFacilitySet && e.Pos.Adjacent(e.useFacility) {
		fac := e.useFacility
		if w.TerrainAt(fac) != spec.Facility {
			w.clearJob(e)
			return
		}
		e.State = useState(e.Need)
		e.Progress++
		portable := spec.GrabTicks > 0 && spec.GrabTicks < spec.UseTicks
		if portable {
			if e.Progress >= spec.GrabTicks {
				// Grabbed it: free the facility's access tile immediately and
				// finish away from it, instead of occupying the tile for the
				// whole UseTicks.
				e.useFacility, e.useFacilitySet, e.carrying, e.Progress = Point{}, false, true, 0
				if !w.stepAside(e) {
					w.wanderStep(e)
				}
			}
			return
		}
		if e.Progress >= spec.UseTicks {
			w.finishUse(e, spec)
		}
		return
	}
	// Keep the established shared-field behavior while there is no alternative
	// room. Concrete routing is only needed once multiple facilities can split a
	// queue; this also lets builders retain the field's crowd-transit behavior.
	if w.countTerrain(spec.Facility) < 2 {
		if w.followField(e, field) {
			e.stuck = 0
			e.State = Moving
			return
		}
	}
	// Route to the facility selected when the need became urgent. The shared field
	// is still the reachability gate, but routing to a concrete facility prevents
	// every user from converging on its nearest seed.
	arrived, ok := w.travelTo(e, e.useFacility)
	if !ok || !arrived {
		if !ok {
			// A facility can become unreachable after a terrain change while the
			// shared field still has another reachable goal. Fall back to that
			// field rather than dropping the need and risking starvation.
			if w.followField(e, field) {
				e.stuck = 0
				e.State = Moving
				return
			}
		}
		if !ok {
			e.stuck++
		} else {
			e.stuck = 0
		}
		// A blocked access tile can be at distance zero in the shared field. Move
		// aside rather than repeatedly abandoning and reacquiring the same queue.
		if !arrived && e.stuck > 0 && e.stuck%w.cfg.StuckLimit == 0 {
			w.stepAside(e)
		}
		if e.stuck > w.cfg.StuckLimit*2 {
			w.clearJob(e)
		}
		if arrived {
			e.State = Moving
		}
		return
	}
	if !arrived {
		e.stuck++
		return
	}
	e.stuck = 0
	e.State = Moving
}

// jobUseCarrying finishes a portable need (see NeedSpec.GrabTicks) away from
// the facility. The colonist already grabbed it, so completion is guaranteed
// — no travel, no facility reachability or crowding to worry about — freeing
// the access tile it used to occupy for the rest of the process.
func (w *World) jobUseCarrying(e *Entity, spec NeedSpec) {
	e.State = useState(e.Need)
	e.Progress++
	if e.Progress >= spec.UseTicks-spec.GrabTicks {
		w.finishUse(e, spec)
	}
}

// finishUse applies a completed JobUse: resets the need, records a memory,
// and clears the job. Shared by an in-place use and a carried-away one.
func (w *World) finishUse(e *Entity, spec NeedSpec) {
	w.resetNeed(e, e.Need)
	switch e.Need {
	case NeedFood:
		w.remember(e, event(EvtAte, "Had a meal."))
	case NeedBladder:
		w.remember(e, event(EvtUsedToilet, "Used the toilet."))
	case NeedSleep:
		w.remember(e, event(EvtSlept, "Slept in a bed."))
	default:
		w.remember(e, event(EvtNeedSatisfied, "Satisfied %s.", spec.Name))
	}
	w.clearJob(e)
}

// buildTicks is how long a given structure takes to raise.
func (w *World) buildTicks(kind Terrain) int {
	switch kind {
	case Wall:
		return w.cfg.BuildTicks
	case Floor: // a project dig task: excavating rock, not constructing
		return w.cfg.MineTicks
	case Incinerator: // a machine, not a fixture: more work than a bunk or a latrine
		return w.cfg.IncineratorBuildTicks
	default:
		return w.cfg.FacilityBuildTicks
	}
}

// noteBuild logs the completion of notable structures.
func (w *World) noteBuild(kind Terrain) {
	switch kind {
	case NutrientPod:
		w.log.add("A nutrient pod comes online.")
	case Toilet:
		w.log.add("A latrine is installed.")
	case Bed:
		w.log.add("A bunk is bolted into the dormitory.")
	case Incinerator:
		w.log.add("The incinerator roars to life.")
	}
}

// travelTo advances a colonist one step along a cached A* route toward a tile
// adjacent to target, computing (or recomputing) the route as needed. It returns
// arrived (now adjacent to target) and ok (false => give up: the target is
// unreachable, or the colonist has been wedged too long).
func (w *World) travelTo(e *Entity, target Point) (arrived, ok bool) {
	if e.Pos.Adjacent(target) {
		// Drop any cached route, but not e.stuck: this path runs every tick a
		// colonist is already adjacent, including every tick it is blocked
		// (e.g. jobBuild's occupied-tile wait). Resetting stuck here clobbers
		// it back to 0 before the caller's own stuck++ can ever accumulate
		// past 1, so a StuckLimit timeout never fires — two colonists that
		// end up swapped onto each other's build tiles deadlock forever
		// instead of one abandoning the task. The caller resets stuck itself
		// once it confirms real progress (see jobBuild, jobUse).
		e.path, e.pathAt = e.path[:0], 0
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
	// Any entity may pass through another mid-route, but it must end the tick
	// on a free tile. Scan the occupied prefix and land on the first available
	// route cell. An alien is the one exception: it is a real obstacle (and a
	// threat), not clutter, so it still blocks movement outright — a cat, a
	// mouse, or a fellow colonist standing in a narrow corridor must not. A
	// stray cat used to wedge a whole queue of colonists there, each abandoning
	// and immediately re-claiming the same path with nothing ever able to make
	// it past — StuckLimit just reset the standoff instead of resolving it.
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
		if blocker.Kind == Alien {
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

// bordersFloor reports whether p has at least one walkable neighbor the colony
// has discovered. The walls of a natural cavern nobody has broken into are not
// mining frontier: no colonist could reach them (see docs/caverns.md).
func (w *World) bordersFloor(p Point) bool {
	for _, d := range neighbors8 {
		if q := p.Add(d.X, d.Y); w.Walkable(q) && w.discovered(q) {
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

// alienTurn dispatches on the alien's rolled species' Temperament
// (AlienSpecies, see lore.go): Friendly never fights and only wanders;
// Cautious reacts once a colonist comes within Config.AlienCautiousRadius but
// does not chase one further off; Hostile hunts the nearest colonist
// anywhere on the map, unconditionally, the way every alien behaved before
// temperament existed.
func (w *World) alienTurn(e *Entity) {
	if e.Cooldown > 0 {
		e.Cooldown-- // still digesting or mid-stride between slow steps
		return
	}
	sp := w.alienSpeciesFor(e)

	if sp.Temperament == TemperamentFriendly {
		e.State, e.Quarry = Idle, 0
		w.wanderStep(e)
		e.Cooldown = sp.Slowness - 1
		return
	}

	var prey *Entity
	var ok bool
	if sp.Temperament == TemperamentHostile {
		prey, ok = w.nearestOfKindAnywhere(e.Pos, Colonist)
	} else { // Cautious: reacts, but does not go looking beyond its radius
		prey, ok = w.nearestOfKind(e.Pos, Colonist, w.cfg.AlienCautiousRadius)
	}
	if !ok {
		e.State, e.Quarry = Idle, 0
		w.wanderStep(e)
		e.Cooldown = sp.Slowness - 1
		return
	}
	e.Quarry = prey.ID

	if e.Pos.Adjacent(prey.Pos) {
		w.bite(e, prey)
		e.Cooldown = sp.BiteRest
		return
	}

	// Aliens burrow: they step toward prey through any terrain.
	e.State = Hunting
	w.burrowStep(e, prey.Pos)
	e.Cooldown = sp.Slowness - 1
}

// bite deals damage to a random body part of a colonist and eats it if the
// wound is fatal (a vital part destroyed, or HP exhausted). The victim
// remembers the attack, and any other colonist close enough to have noticed
// the alien (observeNearby's own sighting radius) remembers watching it
// happen. A fatal bite leaves gore behind.
func (w *World) bite(alien, prey *Entity) {
	part := w.rollHit(prey)
	fatal := applyDamage(prey, part, w.alienSpeciesFor(alien).BiteDamage)
	witnesses := w.colonistsWithin(prey.Pos, w.cfg.FleeRadius, prey.ID)
	noun := w.alienNounFor(alien)
	if fatal {
		alien.State = Feeding
		name := prey.displayName()
		w.addGore(prey.Pos)
		w.remove(prey.ID, fmt.Sprintf("devoured by %s", noun))
		w.log.add(fmt.Sprintf("%s devours %s.", capitalizeFirst(noun), name))
		for _, wit := range witnesses {
			w.remember(wit, eventFrom(EvtWitnessedColonistKilled, alien.ID, "Watched %s kill %s.", noun, name))
		}
	} else {
		alien.State = Hunting
		w.remember(prey, eventFrom(EvtBitten, alien.ID, "Bitten in the %s by %s!", part, noun))
		for _, wit := range witnesses {
			w.remember(wit, eventFrom(EvtWitnessedColonistAttacked, alien.ID, "Watched %s attack %s.", noun, prey.displayName()))
		}
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

	prey, ok := w.nearestOfKindAnywhere(e.Pos, Mouse)
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
// is fatal. Any colonist close enough to have noticed the mouse remembers
// seeing it happen.
func (w *World) pounce(cat, prey *Entity) {
	cat.State = Feeding
	for _, wit := range w.colonistsWithin(prey.Pos, w.cfg.ColonistStompRadius, 0) {
		w.remember(wit, event(EvtWitnessedCatCatch, "Watched a cat catch mouse #%d.", prey.ID))
	}
	w.remove(prey.ID, "caught by a cat")
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
		w.addCorpse(e.Pos)
		w.remove(e.ID, "starved")
		w.log.add(fmt.Sprintf("Mouse #%d starves.", e.ID))
		return
	}

	// A carried litter arrives once gestation completes, whatever else the mouse
	// does with the rest of its tick.
	if e.pregnant && w.tick >= e.dueTick {
		w.giveBirth(e)
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

	// Nothing pressing: a mouse with no cat to flee and no hunger to sate looks
	// to breed with an adjacent mate.
	if w.tryMate(e) {
		return
	}

	e.State = Idle
	w.wanderStep(e)
}

// rollMouseSex assigns a mouse its sex, an even male/female split. It draws from
// the simulation RNG (not the personality stream) because breeding is a
// simulation mechanic, not cosmetic flavor.
func (w *World) rollMouseSex() Sex {
	if w.rng.Intn(2) == 0 {
		return SexMale
	}
	return SexFemale
}

// canBreed reports whether a mouse may mate this tick: it is not already
// carrying a litter and is past mateReadyTick, which gates both a newborn's
// maturation and a mother's post-birth cooldown.
func (w *World) canBreed(e *Entity) bool {
	return e.Kind == Mouse && !e.pregnant && w.tick >= e.mateReadyTick
}

// tryMate pairs a mouse with an adjacent eligible mouse of the opposite sex. The
// female of the pair conceives a litter, and both go on a breeding cooldown so a
// warren does not multiply every tick. Returns whether a mating happened.
func (w *World) tryMate(e *Entity) bool {
	if !w.canBreed(e) {
		return false
	}
	for _, d := range neighbors8 {
		mate := w.entityAt(e.Pos.Add(d.X, d.Y))
		if mate == nil || !w.canBreed(mate) || mate.sex == e.sex {
			continue
		}
		female, male := e, mate
		if female.sex != SexFemale {
			female, male = mate, e
		}
		female.pregnant = true
		female.dueTick = w.tick + w.cfg.MouseGestationTicks
		e.mateReadyTick = w.tick + w.cfg.MouseBreedCooldown
		mate.mateReadyTick = w.tick + w.cfg.MouseBreedCooldown
		e.State, mate.State = Idle, Idle
		w.log.add(fmt.Sprintf("Mice #%d and #%d mate.", male.ID, female.ID))
		return true
	}
	return false
}

// giveBirth delivers a pregnant mouse's litter onto free floor tiles around her,
// then resets her to a post-birth breeding cooldown. Litter size is random
// within the configured range; pups with nowhere to land are simply not born (a
// crowded cavern limits the warren). Newborns cannot breed until they mature.
func (w *World) giveBirth(e *Entity) {
	e.pregnant = false
	e.mateReadyTick = w.tick + w.cfg.MouseBreedCooldown
	litter := w.cfg.MouseLitterMin
	if span := w.cfg.MouseLitterMax - w.cfg.MouseLitterMin; span > 0 {
		litter += w.rng.Intn(span + 1)
	}
	born := 0
	for _, d := range neighbors8 {
		if born >= litter {
			break
		}
		p := e.Pos.Add(d.X, d.Y)
		if !w.Walkable(p) || w.occupied(p) {
			continue
		}
		pup := w.spawn(Mouse, p)
		pup.mateReadyTick = w.tick + w.cfg.MouseMaturityTicks
		born++
	}
	if born > 0 {
		w.log.add(fmt.Sprintf("Mouse #%d gives birth to a litter of %d.", e.ID, born))
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
// search through a packed group of colonists, cats, and mice to find the
// nearest genuinely clear landing, just as job navigation can pass through a
// crowd (see travelTo) — only an alien stops the search. A random one-step
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
					if blocker.Kind != Alien {
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

// colonistsWithin returns every living colonist other than exclude within
// radius of pos, in deterministic ID order. It backs witness memories: a
// bystander close enough to have noticed a creature (the same radius
// observeNearby uses for that creature kind) also notices what happens to it.
// Pass 0 for exclude when no colonist should be excluded.
func (w *World) colonistsWithin(pos Point, radius int, exclude EntityID) []*Entity {
	var witnesses []*Entity
	for _, id := range w.entityIDsSorted() {
		if id == exclude {
			continue
		}
		e := w.entities[id]
		if e.Kind != Colonist || !e.Alive() {
			continue
		}
		if pos.Chebyshev(e.Pos) <= radius {
			witnesses = append(witnesses, e)
		}
	}
	return witnesses
}

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
	return w.nearestMatch(from, within, func(e *Entity) bool { return e.Kind == kind })
}

// nearestOfKindAnywhere returns the globally nearest living entity of kind, with
// no range limit — for a hunter whose prey can be anywhere on the map (an
// alien after the nearest colonist, a cat after the nearest mouse). It scans
// World.kindEntities[kind] directly rather than going through nearestMatch's
// chunk-ring expansion: that expansion is cheap when a match is nearby, but an
// unbounded search forces it to visit every chunk on the map to confirm none
// is closer. Entities of a given kind are typically few, so a direct scan is
// far cheaper — same nearest-wins-ties-toward-lower-ID result as nearestMatch.
func (w *World) nearestOfKindAnywhere(from Point, kind Kind) (*Entity, bool) {
	var best *Entity
	bestDist := 0
	for id := range w.kindEntities[kind] {
		e := w.entities[id]
		if e == nil || !e.Alive() {
			continue
		}
		d := from.Chebyshev(e.Pos)
		if best == nil || d < bestDist || (d == bestDist && e.ID < best.ID) {
			best, bestDist = e, d
		}
	}
	return best, best != nil
}

// nearestMatch returns the nearest living entity within range that satisfies
// match, sharing the chunk-ring scan (and deterministic ID tie-break) with
// nearestOfKind.
func (w *World) nearestMatch(from Point, within int, match func(*Entity) bool) (*Entity, bool) {
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
					if e == nil || !e.Alive() || !match(e) {
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

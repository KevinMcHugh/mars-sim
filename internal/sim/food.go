package sim

// ---- Food ---------------------------------------------------------------------
//
// Food is an item now. A hungry colonist eats, in order: a meal it is carrying,
// a meal of its own in a depot it can reach (its crash pod's locker, to begin
// with), a meal it buys — the colony's scumhouse sells what it cooks — and
// only then the safety net: a nutrient pod, which makes gruel out of nothing
// while infinite-food is on and serves nothing when it is off. The first
// three are JobEat; the last is the old JobUse at a pod. See docs/food.md.

// eatStage is where a JobEat colonist is. In eatMeal the meal is in hand —
// out of the inventory and the depot both — so clearJob puts it back in the
// colonist's pocket if eating is interrupted; a meal is never lost to a
// colonist looking up at an alien.
type eatStage uint8

const (
	eatFetch eatStage = iota // walking to the depot at Target to take a meal out
	eatMeal                  // eating the meal in hand
)

// tryStartEating commits a hungry colonist to eating a real meal if it has, or
// can reach, one it may eat. It reports whether it did; false leaves the
// safety net (or going hungry) to the caller.
//
// Whatever the colonist was doing is dropped first (releasing its claims), so
// a hungry builder eats its own food rather than finishing a pod for someone
// else's gruel.
func (w *World) tryStartEating(e *Entity) bool {
	// Only a meal of its own: a hauler's pockets may hold the colony's.
	if e.ownCarried(Meal) > 0 {
		w.clearJob(e)
		e.Inventory.Remove(Meal, 1)
		e.Job, e.eat, e.Progress = JobEat, eatMeal, 0
		return true
	}
	depot, ok := w.nearestMealDepot(e)
	if !ok {
		return false
	}
	w.clearJob(e)
	e.Job, e.eat, e.Target, e.Progress = JobEat, eatFetch, depot, 0
	return true
}

// mealOwners lists whose meals e may take: only its own. The colony's meals
// are for sale, not for the taking (see refreshColonyMealAsks).
func mealOwners(e *Entity) [1]Owner {
	return [1]Owner{ColonistOwner(e.ID)}
}

// nearestMealDepot finds the depot e should fetch a meal from: the nearest
// reachable one holding a meal of e's own. Distance is straight-line, ties by position, the same
// rule chooseStorage uses, so the choice never depends on map order.
func (w *World) nearestMealDepot(e *Entity) (Point, bool) {
	room := w.roomOf(e.Pos)
	for _, owner := range mealOwners(e) {
		var best Point
		bestDist, found := 1<<30, false
		for p, c := range w.storageContainers {
			if c.held(owner, Meal) == 0 || !w.canUseFixture(e, p) || !w.taskReachable(p, room) {
				continue
			}
			d := e.Pos.Chebyshev(p)
			if !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
				best, bestDist, found = p, d, true
			}
		}
		if found {
			return best, true
		}
	}
	return Point{}, false
}

// jobEat runs one tick of eating: fetch a meal from the depot at Target if the
// colonist is not already holding one, then eat it where it stands. Taking
// the meal out steps aside first, like a pod's grab-and-go, so a shared depot's
// access tile is free for the next person while this one eats.
func (w *World) jobEat(e *Entity) {
	spec := w.cfg.Needs[NeedFood]
	if e.eat == eatFetch {
		arrived, ok := w.travelTo(e, e.Target)
		if !ok {
			w.clearJob(e)
			return
		}
		if !arrived {
			e.State = Moving
			return
		}
		c := w.storageContainers[e.Target]
		if c == nil || !w.takeMeal(e, c) {
			w.clearJob(e) // somebody got the last one first; think again
			return
		}
		e.eat, e.Progress = eatMeal, 0
		if !w.stepAside(e) {
			w.wanderStep(e)
		}
		return
	}
	e.State = Eating
	e.Progress++
	if e.Progress >= spec.UseTicks {
		// Eaten: out of hand before clearJob, which would otherwise pocket it
		// again as an interrupted meal — and so feed the colony forever.
		e.eat = eatFetch
		w.resetNeed(e, NeedFood)
		w.cancelMealBids(ColonistOwner(e.ID)) // fed: stop queuing for another
		w.emitDone(e, ActionEat, NounMeal, "Had a meal.")
		w.clearJob(e)
	}
}

// takeMeal withdraws one meal e may eat from c, preferring its own.
func (w *World) takeMeal(e *Entity, c *StorageContainer) bool {
	if !w.canUseFixture(e, c.Pos) {
		return false
	}
	for _, owner := range mealOwners(e) {
		if c.debit(owner, Meal, 1) {
			return true
		}
	}
	return false
}

// podsFeed reports whether nutrient pods serve food at all: only while the
// safety net is on.
func (w *World) podsFeed() bool {
	return w.cfg.InfiniteFood
}

// wantsFacility reports whether the colony plans and builds kind for a need.
// A nutrient pod is worth building only while it feeds anyone.
func (w *World) wantsFacility(kind Terrain) bool {
	return kind != NutrientPod || w.podsFeed()
}

// runFoodFocus handles a hungry colonist's turn when real food is involved,
// reporting whether it did. False hands the turn back to the ordinary need
// logic, which walks to the safety-net pod.
func (w *World) runFoodFocus(e *Entity) bool {
	if e.Job == JobEat {
		w.runJob(e)
		return true
	}
	if e.Job == JobUse && e.Need == NeedFood && w.podsFeed() {
		return false // already queued at the safety net; let it finish
	}
	if w.tryStartEating(e) {
		w.runJob(e)
		return true
	}
	// Nothing of its own or the colony's: buy a meal before settling for gruel.
	if w.tryBuyMeal(e) && w.tryStartEating(e) {
		w.runJob(e)
		return true
	}
	if w.podsFeed() {
		return false
	}
	if w.tryRation(e) && w.tryStartEating(e) {
		w.runJob(e)
		return true
	}
	w.hungryWithoutFood(e)
	return true
}

// tryRation gives a colonist at critical hunger that could not buy a meal one
// of the colony's, from the nearest reachable depot holding one, and reports
// whether it did. The meal changes hands on the depot's ledger (taken off the
// colony's ask first, since goods on offer are in escrow); the ordinary
// eating job then fetches it.
//
// It is a lifeline, not a living: only at critical hunger, and only after
// buying failed. Late in long runs, colonists with a few dollars — less than a
// meal — starved a few tiles from shelves holding a hundred of the colony's
// meals, scraping scum for a supper they would not live to cook.
func (w *World) tryRation(e *Entity) bool {
	if !w.cfg.Rations || e.needPhase[NeedFood] != NeedCritical {
		return false
	}
	room := w.roomOf(e.Pos)
	var best Point
	found := false
	for _, p := range w.mealDepots() {
		c := w.storageContainers[p]
		if c.held(Community, Meal)+w.openQty(Ask, Meal, p, Community) == 0 ||
			!w.canUseFixture(e, p) || !w.taskReachable(p, room) {
			continue
		}
		if !found || e.Pos.Chebyshev(p) < e.Pos.Chebyshev(best) ||
			(e.Pos.Chebyshev(p) == e.Pos.Chebyshev(best) && lessPoint(p, best)) {
			best, found = p, true
		}
	}
	if !found {
		return false
	}
	c := w.storageContainers[best]
	w.withdrawColonyAsks(Meal, best)
	given := c.moveLine(Community, ColonistOwner(e.ID), Meal, 1)
	w.offerColonyMeals(best) // the rest go back on sale
	if !given {
		return false
	}
	w.rationsGiven++
	return true
}

// hungryWithoutFood is a turn for a colonist with nothing to eat and no
// safety net. Waiting by an empty locker helps nobody, so whenever it picks
// new work it picks food work first — cooking, then scraping scum, whatever
// the colony's stock says — and otherwise keeps working. A job already under
// way is left to finish rather than dropped mid-tile, until hunger is
// pressing: then anything but feeding itself is dropped (feedingItself). Left
// to finish, a long dig or haul let colonists starve with their own scum
// sitting in a free scumhouse; waiting for critical hunger left too little
// time to scrape, haul, and cook. Cooking the colony's meals is food work
// too (makingMeals) and is never dropped for hunger. It checks for food again
// every turn, since runFoodFocus runs first.
func (w *World) hungryWithoutFood(e *Entity) {
	if workJob(e.Job) && e.needPhase[NeedFood] >= NeedPressing && !w.feedingItself(e) && !w.makingMeals(e) {
		w.clearJob(e)
	}
	if !workJob(e.Job) {
		w.clearJob(e)
		if !w.tryAssignFoodWork(e, true) && !w.tryEmergencyScumhouse(e) {
			w.assignWorkJob(e)
		}
	}
	if e.Job != JobNone {
		e.resting = false
		w.runJob(e)
		return
	}
	e.State = Idle
	w.wanderStep(e)
}

// makingMeals reports whether e is working a recipe that makes meals, for
// anyone. A hungry colonist cooking the colony's scum is making the meal it
// will buy, or be rationed at critical hunger. Dropping that job for hunger
// was a livelock: with no food work of its own to take, assignWorkJob handed
// the same colony cooking straight back, the next turn dropped it again, and
// the recipe never got past its first tick while the colony's last colonists
// starved at the stove beside its scum. A colony cook stops batching once it
// is hungry (cooksOn), so this keeps it for one recipe, not a shift.
func (w *World) makingMeals(e *Entity) bool {
	if e.Job != JobCraft {
		return false
	}
	for _, o := range recipes[e.recipe].Outputs {
		if o.Kind == Meal && o.Count > 0 {
			return true
		}
	}
	return false
}

// feedingItself reports whether e's job is making food it will own: cooking
// its own inputs, or scraping scum to keep.
func (w *World) feedingItself(e *Entity) bool {
	switch e.Job {
	case JobCraft:
		return e.craftFor == ColonistOwner(e.ID)
	case JobScrape:
		return e.scrapeKeep
	}
	return false
}

// tryEmergencyScumhouse is the scarcity version of the safety net's emergency
// pod: a hungry colonist with nothing to eat and no scumhouse it can reach
// helps build the one the colony has planned, or failing that raises one
// itself, unpaid — the same way it would build itself a pod or a toilet.
// Without it, a colony whose treasury cannot fund a scumhouse room has no way
// to make food at all, and starves (see docs/food.md).
func (w *World) tryEmergencyScumhouse(e *Entity) bool {
	if w.podsFeed() {
		return false
	}
	if _, ok := w.nearestScumhouse(e, nil); ok {
		return false // there is one: food work, not building, is the answer
	}
	if task, ok := w.claimNearestTaskProviding(e.Pos, e.ID, Scumhouse); ok {
		w.assignTask(e, task)
		return true
	}
	if w.reachableFacilityConstruction(e.Pos, Scumhouse) {
		return false // someone is already raising one within reach
	}
	if spot, ok := w.findBuildSpot(e.Pos, 20); ok && w.canAffordBuild(e, Scumhouse, Owner{}) {
		w.assignBuild(e, Scumhouse, spot)
		return true
	}
	return false
}

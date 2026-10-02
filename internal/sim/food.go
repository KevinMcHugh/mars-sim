package sim

import "fmt"

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
	eatWalk                  // meal in hand, walking to a chair in the meeting hall at Target
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
		if seat, ok := w.mealSeat(e); ok {
			e.eat, e.Target = eatWalk, seat
		}
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

// pocketMeals is how many meals a colonist keeps on it: one with pocket
// meals on, none without.
func (w *World) pocketMeals() int {
	if w.cfg.PocketMealAt > 0 {
		return 1
	}
	return 0
}

// tryPocketMeal sends e to fetch one of its own meals to carry, from the
// nearest depot holding one, before it's hungry enough to eat. It reports
// whether e has that job. It waits for pocket-meal-at, so the meal it carries
// is the next one it eats, not a spare.
//
// A hungry colonist has about 175 ticks from pressing hunger, when it goes to
// eat, to critical, and about 40 more to death. On a big map one walk to its
// locker used most of that; with a meal on it, hunger starts with food in
// hand, and the trip happens while it has time to spare.
//
// It never buys a pocket meal. A colonist that isn't hungry yet buying one
// takes a meal off the shelf from a colonist who is: in a 100-colonist
// shortage, buying pocket meals starved 150 of 400 colonists, against 96
// without pocket meals (see docs/food.md).
func (w *World) tryPocketMeal(e *Entity) bool {
	at := w.cfg.PocketMealAt
	if at <= 0 || e.Kind != Colonist || e.ownCarried(Meal) > 0 || !e.Inventory.CanAdd(Meal, 1) {
		return false
	}
	if e.needPhase[NeedFood] >= NeedPressing || w.needLevel(e, NeedFood) < at {
		return false // pressing hunger eats (runFoodFocus); before at, it has time
	}
	depot, ok := w.nearestMealDepot(e)
	if !ok {
		return false
	}
	e.Job, e.eat, e.eatKeep, e.Target, e.Progress = JobEat, eatFetch, true, depot, 0
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
		for p, c := range w.home.storageContainers {
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
		c := w.home.storageContainers[e.Target]
		if c == nil || !w.takeMeal(e, c) {
			w.clearJob(e) // somebody got the last one first; think again
			return
		}
		if e.eatKeep {
			e.Inventory.Add(Meal, 1) // for later: in the pocket, not the mouth
			w.clearJob(e)
			return
		}
		e.eat, e.Progress = eatMeal, 0
		if seat, ok := w.mealSeat(e); ok {
			// Take it to the hall: the colony eats together, and the depot's
			// access tile is free the moment the meal is out of it.
			e.eat, e.Target = eatWalk, seat
			e.State = Moving
			return
		}
		if !w.stepAside(e) {
			w.wanderStep(e)
		}
		return
	}
	if e.eat == eatWalk {
		if e.needPhase[NeedFood] < NeedCritical {
			if arrived, ok := w.travelTo(e, e.Target); ok && !arrived {
				e.State = Moving
				return
			}
		}
		// Seated, the way is blocked, or hunger turned critical on the walk:
		// eat where it stands.
		e.eat, e.Progress = eatMeal, 0
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
		c := w.home.storageContainers[p]
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
	c := w.home.storageContainers[best]
	w.withdrawColonyAsks(Meal, best)
	given := c.moveLine(Community, ColonistOwner(e.ID), Meal, 1)
	w.offerColonyMeals(best) // the rest go back on sale
	if !given {
		return false
	}
	w.rationsGiven++
	return true
}

// hungryWithoutFood is a turn for a colonist with nothing to eat, no meal it
// can buy, and no safety net: it forages (planForage). It checks for food
// again every turn first, since runFoodFocus runs before this.
//
// Work it was doing when hunger turned pressing is dropped, once, unless it
// makes food: cooking or scraping for itself (feedingItself), or cooking
// meals for anyone (makingMeals), which makes the meal it will buy. Whatever
// it then picks as a forager it sees through (e.foraging), re-planning only
// when that job ends.
//
// This used to drop any other work and then, finding no food work, fall back
// on assignWorkJob, which handed it the same dig or build straight back; the
// next turn dropped it again. The job never got past its first tick: a
// colonist stood "walking to" the same rock for 200 ticks until it starved,
// and a whole colony died that way on seed 1790737522337000000 with rock
// full of scum all round it. Work that feeds nobody is now never taken while
// hungry. See docs/food.md.
func (w *World) hungryWithoutFood(e *Entity) {
	if e.Job != JobNone && (e.foraging || w.feedingItself(e) || w.makingMeals(e)) {
		e.resting = false
		w.runJob(e)
		return
	}
	w.clearJob(e)
	if w.tick >= e.forageRetry {
		if w.planForage(e) {
			e.foraging, e.resting = true, false
			w.noteForaging(e)
			w.runJob(e)
			return
		}
		// Nothing to do yet (a meal on the stove, or nothing to dig): look
		// again shortly rather than searching every tick.
		e.forageRetry = w.tick + forageRetryTicks
	}
	e.State = Idle
	if w.idleWouldBlock(e.Pos) {
		w.stepAside(e)
	} else {
		w.wanderStep(e)
	}
}

// forageRetryTicks is how long a hungry colonist that found nothing to forage
// waits before looking again. Eating is still checked every turn.
const forageRetryTicks = 8

// planForage gives a hungry colonist with nothing to eat a job that gets it
// food, and reports whether it did. In order:
//
//  1. no scumhouse it can reach: build one (tryEmergencyScumhouse);
//  2. cook what a scumhouse already holds: its own scum, or the colony's
//     while the colony is short (a meal it can buy, or be rationed);
//  3. enough meals on the colony's stoves for everyone hungry: wait for
//     them (false), rather than walking off to dig (foodCooking);
//  4. carrying a meal's worth of its own scum, counting any it has in the
//     scumhouse: take it in to cook;
//  5. scrape exposed scum, keeping it, unloading first if its pack has no
//     room for scum;
//  6. prospect: dig into rock nobody has seen, to expose more scum;
//  7. bank what little scum it carries, so a later find makes a meal.
//
// Scraping and prospecting carry a part load from patch to patch rather than
// walking each unit home (finishScraping), so a forager is out at the rock
// face until it has a meal's worth.
func (w *World) planForage(e *Entity) bool {
	if w.tryEmergencyScumhouse(e) {
		return true
	}
	if w.tryAssignCraftFor(e, []Owner{ColonistOwner(e.ID)}) ||
		(w.foodWanted() && w.tryAssignCraft(e)) {
		return true
	}
	if w.foodCooking(e) {
		return false
	}
	carried := e.ownCarried(CaveScum)
	if carried > 0 && carried+w.ownScumBanked(e) >= w.scumPerMeal() && w.tryAssignScrape(e, true) {
		return true // tryAssignScrape hauls what it is carrying
	}
	// Room in the pack for what it finds. A miner's pack is often seven
	// stacks of rubble: it could still dig plain rock, and dug past patch
	// after patch it had no room to scrape.
	if !e.Inventory.CanAdd(CaveScum, 1) && w.tryForageUnload(e) {
		return true
	}
	if w.tryForageScrape(e) {
		return true
	}
	if _, ok := w.scrapeDestination(e, true); ok && w.tryProspect(e, true) {
		return true // only where there is a scumhouse to cook what it finds
	}
	return carried > 0 && w.tryAssignScrape(e, true)
}

// noteForaging logs, once a hunger, that e has gone looking for food.
func (w *World) noteForaging(e *Entity) {
	if e.forageNoted {
		return
	}
	e.forageNoted = true
	w.logEvent(LogNote, fmt.Sprintf("%s has nothing to eat and goes looking for scum.", e.displayName()))
}

// foodCooking reports whether the colony's cooks, at scumhouses e can
// reach, will make enough meals from the stock at their stoves for every
// hungry colonist without one: food is on its way, for sale, and e should
// wait for it rather than dig. Only the colony's meals count. A colonist
// cooking its own scum is cooking its own supper, and counting it once kept
// a forager standing beside 40 units of scum it had just dug out while a
// neighbour ate. And a cook's meals feed as many as they feed: in a
// 100-colonist colony, 30 hungry colonists waited on stoves making a meal
// at a time, beside scum they could have scraped, and died there.
//
// It only sums, so the order it visits the claims in cannot matter.
func (w *World) foodCooking(e *Entity) bool {
	room := w.roomOf(e.Pos)
	coming := 0
	for p, id := range w.home.workshopClaims {
		cook := w.entities[id]
		if cook == nil || cook.Job != JobCraft || cook.craftFor != Community || w.TerrainAt(p) != Scumhouse ||
			!recipeMakesMeals(recipes[cook.recipe]) || !w.taskReachable(p, room) {
			continue
		}
		coming += colonyMealsIn(w.home.storageContainers[p])
	}
	return coming > 0 && coming >= w.hungryWithoutMeals()
}

// colonyMealsIn is how many meals the colony's stock in scumhouse depot c
// would make, recipe by recipe.
func colonyMealsIn(c *StorageContainer) int {
	if c == nil {
		return 0
	}
	n := 0
	for _, r := range recipes {
		if r.Facility != Scumhouse || !recipeMakesMeals(r) {
			continue
		}
		times := 1 << 30
		for _, in := range r.Inputs {
			times = min(times, c.held(Community, in.Kind)/max(1, in.Count))
		}
		for _, o := range r.Outputs {
			if o.Kind == Meal {
				n += times * o.Count
			}
		}
	}
	return n
}

// hungryWithoutMeals is how many colonists are hungry enough to eat and not
// eating: everyone a meal coming off the stove might go to. Memoized for the
// tick, since every waiting forager asks.
func (w *World) hungryWithoutMeals() int {
	if w.hungryTick == w.tick {
		return w.hungryCache
	}
	n := 0
	for _, e := range w.entities {
		if e.Kind == Colonist && e.focus == FocusEat && e.Job != JobEat {
			n++
		}
	}
	w.hungryTick, w.hungryCache = w.tick, n
	return n
}

// recipeMakesMeals reports whether r has a meal among its outputs.
func recipeMakesMeals(r Recipe) bool {
	for _, o := range r.Outputs {
		if o.Kind == Meal && o.Count > 0 {
			return true
		}
	}
	return false
}

// scumPerMeal is how much cave scum one meal takes.
func (w *World) scumPerMeal() int {
	if r, ok := scumMealRecipe(); ok {
		return r.Inputs[0].Count
	}
	return 1
}

// ownScumBanked is how much of its own scum e has in the nearest scumhouse
// it may cook at: what a forager's carried scum adds to.
func (w *World) ownScumBanked(e *Entity) int {
	p, ok := w.nearestScumhouse(e, func(c *StorageContainer) bool { return w.mayCookAt(e, c.Pos) })
	if !ok {
		return 0
	}
	return w.home.storageContainers[p].held(ColonistOwner(e.ID), CaveScum)
}

// tryForageScrape sends a forager to the nearest exposed patch to scrape and
// keep, whatever it is already carrying: tryAssignScrape would take that
// straight home instead. It still needs a scumhouse to cook at, since scum
// it cannot cook feeds nobody.
func (w *World) tryForageScrape(e *Entity) bool {
	if _, ok := w.scrapeDestination(e, true); !ok || !e.Inventory.CanAdd(CaveScum, 1) {
		return false
	}
	patch, ok := w.nearestScum(e)
	if !ok {
		return false
	}
	w.home.scumClaims[patch] = e.ID
	e.Job, e.Target, e.scrape, e.Progress = JobScrape, patch, scrapeGather, 0
	e.scrapeKeep = true
	return true
}

// tryProspect sends a forager to dig into rock the colony has not seen, to
// expose the scum in it, and reports whether it did. Scum covers a fixed
// share of the rock, but only rock beside discovered floor can be scraped,
// so once the colony has scraped its walls bare the rest of it is in rock
// nobody has dug to. Digging one tile reveals its eight neighbours (see
// docs/fog-of-war.md), so the best dig is the one that reveals the most
// unseen tiles per tick spent walking to it and digging it:
//
//	fresh / (dig ticks + distance)
//
// That keeps a prospector tunnelling into new rock (three fresh tiles a dig)
// rather than squaring off a room it has already seen round. A forager
// (unload) whose pack is too full for any dig's yield unloads first; a
// colonist prospecting as ordinary work leaves that to assignWorkJob.
func (w *World) tryProspect(e *Entity, unload bool) bool {
	room := w.roomOf(e.Pos)
	if room == 0 {
		return false
	}
	dig := w.workTicks(e, SkillMining, w.cfg.MineTicks)
	var best Point
	bestFresh, bestCost := 0, 0
	found, packFull := false, false
	for p := range w.home.board.frontier {
		cost := dig + e.Pos.Chebyshev(p)
		// Even all eight neighbours unseen couldn't beat the best so far:
		// skip it before the neighbour walks. Only strictly worse rock is
		// skipped, so the choice never depends on the order p comes in.
		if found && len(neighbors8)*bestCost < bestFresh*cost {
			continue
		}
		if w.home.board.isClaimed(p) || !w.frontierReachable(p, room) {
			continue
		}
		fresh := w.unexploredAround(p)
		if fresh == 0 {
			continue
		}
		if !e.Inventory.CanAddAll(miningYield(w.TileAt(p))...) {
			packFull = true
			continue
		}
		// fresh/cost > bestFresh/bestCost, without dividing.
		better := !found || fresh*bestCost > bestFresh*cost ||
			(fresh*bestCost == bestFresh*cost && lessPoint(p, best))
		if better {
			best, bestFresh, bestCost, found = p, fresh, cost, true
		}
	}
	if !found {
		return unload && packFull && w.tryForageUnload(e)
	}
	w.home.board.claimMine(best, e.ID)
	w.assignMineTarget(e, best)
	return true
}

// prospectingForFood reports whether the colony has no exposed scum left for
// anyone to scrape: every patch beside open floor is bare or claimed, so more
// has to be dug out. It only asks whether one patch exists, so the order it
// visits them in cannot matter.
func (w *World) prospectingForFood() bool {
	for p := range w.home.exposedScum {
		if w.scumAt(p) > 0 && w.home.scumClaims[p] == 0 {
			return false
		}
	}
	return true
}

// tryForageUnload empties a forager's pack of rock and ore in one trip, to
// the nearest chest that takes it all. tryAssignStore takes what sells to the
// silo first and the rest on a second trip, which is a fair trade for a miner
// and cost a starving forager the walk that would have fed it.
func (w *World) tryForageUnload(e *Entity) bool {
	stacks := e.Inventory.storableStacks()
	if len(stacks) == 0 {
		return false
	}
	if p, ok := w.chooseStorage(e, stacks); ok {
		e.Job, e.Target, e.Progress = JobStore, p, 0
		return true
	}
	return w.tryAssignStore(e)
}

// unexploredAround counts p's neighbours the colony has not seen: what
// digging p would reveal.
func (w *World) unexploredAround(p Point) int {
	n := 0
	for _, d := range neighbors8 {
		if q := p.Add(d.X, d.Y); w.InBounds(q) && !w.discovered(q) {
			n++
		}
	}
	return n
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
	return e.Job == JobCraft && recipeMakesMeals(recipes[e.recipe])
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

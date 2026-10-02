package sim

import "fmt"

// ---- Chickens ----------------------------------------------------------------
//
// A chicken is one of the three rare items a colonist can land with (see
// podRareItem): it steps out of its keeper's crash pod beside a trough. It has
// one need, food, and two ways to meet it: feed from its trough, or cave scum
// grazed off the rock the way a peaceful alien grazes it. With neither it
// starves, like a rat. Cats and chickens ignore each other: a cat hunts only
// rats (catTurn), and a chicken flees nothing.
//
// Feed is the keeper's work (JobTend): scrape scum, mix it into feed at a
// scumhouse (feedRecipe), and carry the feed to the trough. See
// docs/chickens.md.

// feedRecipe is how a keeper mixes chicken feed at a scumhouse. It is kept
// out of the recipes table on purpose: a cook works the first recipe in that
// table it has inputs for, so listing feed there would let the colony's cooks
// turn the scum it bought for meals into feed nobody ordered. Only a keeper
// works it, from scum in its own pockets (jobTend).
var feedRecipe = Recipe{
	Name:     "mix chicken feed",
	Inputs:   []ItemStack{{CaveScum, 1}},
	Outputs:  []ItemStack{{Feed, 4}},
	Facility: Scumhouse,
	Ticks:    6,
	Skill:    SkillCooking,
}

// chickenTurn runs one chicken tick: starve, then, when hungry, eat from the
// trough or graze scum; otherwise wander, drifting home to the trough when
// it has strayed.
func (w *World) chickenTurn(e *Entity) {
	w.applyStarvation(e)
	if !e.Alive() {
		w.clearJob(e)
		w.addCorpse(e.Pos, AnimalCorpse)
		w.remove(e.ID, "starved")
		w.logEvent(LogDeath, fmt.Sprintf("Chicken #%d starves.", e.ID))
		return
	}
	if e.Cooldown > 0 {
		e.Cooldown--
		return
	}
	e.Cooldown = w.cfg.ChickenSlowness - 1
	if w.driveLevel(e, DriveFood) >= w.cfg.Drives[DriveFood].SeekAt {
		if w.chickenFeed(e) || w.chickenGraze(e) {
			return
		}
	}
	if e.hasTrough && e.Pos.Chebyshev(e.trough) > w.cfg.ChickenRoam {
		if _, ok := w.travelTo(e, e.trough); ok {
			e.State = Moving
			return
		}
	}
	e.State = Idle
	w.wanderStep(e)
}

// chickenFeed sends a hungry chicken to its trough and eats a unit of feed
// there, reporting whether it is busy with that this tick. A trough out of
// feed, gone, or cut off from the chicken's room leaves it to graze.
func (w *World) chickenFeed(e *Entity) bool {
	if !e.hasTrough {
		return false
	}
	c := w.storageContainers[e.trough]
	if c == nil || c.Terrain != Trough || c.Inventory.Count(Feed) == 0 {
		return false
	}
	if e.Pos.Adjacent(e.trough) {
		if takeFeed(c) {
			e.State = Feeding
			w.resetDrive(e, DriveFood)
		}
		return true
	}
	if !w.taskReachable(e.trough, w.roomOf(e.Pos)) {
		return false
	}
	if _, ok := w.travelTo(e, e.trough); !ok {
		return false
	}
	e.State = Moving
	return true
}

// takeFeed takes one unit of feed out of a trough, on whoever's account
// holds some: a chicken eats what is there, and a dead keeper's feed is still
// feed.
func takeFeed(c *StorageContainer) bool {
	for _, l := range c.Ledger {
		if l.Item == Feed && l.Count > 0 {
			return c.debit(l.Owner, Feed, 1)
		}
	}
	return false
}

// chickenGraze walks a hungry chicken to the nearest exposed cave scum within
// chicken-graze-radius and pecks a unit of it, the way a grazing alien does
// (alienGraze). It reports whether the chicken is busy with that this tick.
func (w *World) chickenGraze(e *Entity) bool {
	target, ok := w.nearestEdible(e, w.cfg.ChickenGrazeRadius, w.grazeable)
	if !ok {
		return false
	}
	if e.Pos.Chebyshev(target) <= 1 {
		if w.takeScum(target) {
			e.State = Feeding
			w.resetDrive(e, DriveFood)
		}
		return true
	}
	if _, ok := w.travelTo(e, target); !ok {
		return false
	}
	e.State = Moving
	return true
}

// ---- Keeping chickens -----------------------------------------------------------

// tendStage is where a JobTend keeper is.
type tendStage uint8

const (
	tendGather tendStage = iota // scraping the scum patch at Target into its pockets
	tendMix                     // mixing feed at the scumhouse at Target
	tendFill                    // carrying feed to its trough at Target
)

// keepsChickens reports whether any living chicken has e as its keeper.
// Chickens are few, so this scans them outright.
func (w *World) keepsChickens(e *Entity) bool {
	for id := range w.kindEntities[Chicken] {
		if c := w.entities[id]; c != nil && c.keeper == e.ID && c.Alive() {
			return true
		}
	}
	return false
}

// troughOf is e's trough, if it has one still standing.
func (w *World) troughOf(e *Entity) (*StorageContainer, bool) {
	if !e.hasTrough {
		return nil, false
	}
	c := w.storageContainers[e.trough]
	return c, c != nil && c.Terrain == Trough
}

// troughWants is how much feed e's trough wants: enough to bring it up to
// trough-fill, once it is below trough-low and some chicken of e's is alive
// to eat it; 0 otherwise.
func (w *World) troughWants(e *Entity) int {
	c, ok := w.troughOf(e)
	if !ok || w.cfg.TroughLow <= 0 {
		return 0
	}
	have := c.Inventory.Count(Feed)
	if have >= w.cfg.TroughLow || !w.keepsChickens(e) {
		return 0
	}
	return max(0, w.cfg.TroughFill-have)
}

// feedScumFor is how much scum makes enough feed for want: whole batches of
// feedRecipe, as many as cover want but no more than one scraped load.
func (w *World) feedScumFor(want int) int {
	in, out := feedRecipe.Inputs[0].Count, feedRecipe.Outputs[0].Count
	batches := (want + out - 1) / out
	batches = max(1, min(batches, w.scrapeLoad()/in))
	return batches * in
}

// tryAssignTend gives a keeper the next leg of keeping its trough in feed:
// carry feed it already has to the trough, mix scum it already has into
// feed, or scrape scum to mix. Feed in its pockets goes to the trough
// whenever the trough has room, so a keeper interrupted on the way never
// keeps carrying it.
func (w *World) tryAssignTend(e *Entity) bool {
	c, ok := w.troughOf(e)
	if !ok {
		return false
	}
	room := w.roomOf(e.Pos)
	if n := e.ownCarried(Feed); n > 0 {
		if !c.Inventory.CanAdd(Feed, n) || !w.taskReachable(e.trough, room) {
			return false
		}
		e.Job, e.Target, e.tend, e.Progress = JobTend, e.trough, tendFill, 0
		return true
	}
	want := w.troughWants(e)
	if want == 0 || !w.taskReachable(e.trough, room) {
		return false
	}
	need := w.feedScumFor(want)
	house, ok := w.nearestScumhouse(e, nil)
	if !ok {
		return false // nowhere to mix it: leave the chickens to graze
	}
	if e.ownCarried(CaveScum) >= need {
		e.Job, e.Target, e.tend, e.Progress = JobTend, house, tendMix, 0
		e.scrapeQty = need
		return true
	}
	if !e.Inventory.CanAdd(CaveScum, need-e.ownCarried(CaveScum)) {
		return false
	}
	patch, ok := w.nearestScum(e)
	if !ok {
		return false
	}
	w.scumClaims[patch] = e.ID
	e.Job, e.Target, e.tend, e.Progress = JobTend, patch, tendGather, 0
	e.scrapeQty = need
	return true
}

// releaseTend gives back the scum patch a JobTend keeper was scraping, if
// it had claimed one. clearJob calls it.
//
// A keeper mixing feed claims no scumhouse. Nothing it does goes through the
// depot, so it can share the stove with the cook; and a colony cook holds
// its claim for as long as there is scum to cook (cooksOn), so a keeper that
// waited for a free stove waited for good, and its hens starved beside an
// empty trough while it lived (seed 3, 20 colonists: four of four).
func (w *World) releaseTend(e *Entity) {
	if e.tend == tendGather && w.scumClaims[e.Target] == e.ID {
		delete(w.scumClaims, e.Target)
	}
	e.tend, e.scrapeQty = tendGather, 0
}

// jobTend runs one tick of keeping chickens.
func (w *World) jobTend(e *Entity) {
	switch e.tend {
	case tendGather:
		w.tendGatherTick(e)
	case tendMix:
		w.tendMixTick(e)
	case tendFill:
		w.tendFillTick(e)
	}
}

// tendGatherTick scrapes the patch at Target a unit at a time until the
// keeper has the scum it set out for, then heads for a scumhouse to mix it.
func (w *World) tendGatherTick(e *Entity) {
	if e.ownCarried(CaveScum) >= e.scrapeQty || w.scumAt(e.Target) == 0 || !e.Inventory.CanAdd(CaveScum, 1) {
		need := e.scrapeQty
		w.releaseTend(e)
		have := e.ownCarried(CaveScum)
		if have < need && e.Inventory.CanAdd(CaveScum, 1) {
			// This patch ran out first: on to the next, rather than ending
			// the job and waiting to be given it again while the hens go
			// hungry.
			if patch, ok := w.nearestScum(e); ok {
				w.scumClaims[patch] = e.ID
				e.Target, e.Progress, e.scrapeQty = patch, 0, need
				return
			}
		}
		// No more to be had: mix the whole batches it has.
		in := feedRecipe.Inputs[0].Count
		if need = min(need, have/in*in); need == 0 {
			w.clearJob(e)
			return
		}
		house, ok := w.nearestScumhouse(e, nil)
		if !ok {
			w.clearJob(e)
			return
		}
		e.Target, e.tend, e.Progress, e.scrapeQty = house, tendMix, 0, need
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
	e.State = Scraping
	e.Progress++
	if e.Progress < w.workTicks(e, SkillForaging, w.cfg.ScrapeTicks) {
		return
	}
	e.Progress = 0
	if w.takeScum(e.Target) {
		w.practise(e, SkillForaging, w.cfg.ScrapeTicks)
		e.Inventory.Add(CaveScum, 1)
	}
}

// tendMixTick works feedRecipe at the scumhouse at Target on the scum in the
// keeper's own pockets, all the batches it gathered for at once, then sends
// it on to the trough with the feed. Nothing goes in or out of the depot:
// the scumhouse is the stove, and the scum and feed stay in hand.
func (w *World) tendMixTick(e *Entity) {
	c := w.storageContainers[e.Target]
	in, out := feedRecipe.Inputs[0].Count, feedRecipe.Outputs[0].Count
	batches := min(e.ownCarried(CaveScum), e.scrapeQty) / in
	if c == nil || c.Terrain != Scumhouse || batches == 0 {
		w.clearJob(e)
		return
	}
	after := e.Inventory
	after.Remove(CaveScum, batches*in)
	if !after.CanAdd(Feed, batches*out) {
		w.clearJob(e) // pockets too full to hold the feed
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
	e.State = Crafting
	e.Progress++
	if e.Progress < w.workTicks(e, feedRecipe.Skill, feedRecipe.Ticks*batches) {
		return
	}
	e.Inventory.Remove(CaveScum, batches*in)
	e.Inventory.Add(Feed, batches*out)
	w.practise(e, feedRecipe.Skill, feedRecipe.Ticks*batches)
	w.emitDone(e, ActionCook, NounGoods, "Mixed %d units of chicken feed at the scumhouse.", batches*out)
	w.releaseTend(e)
	if _, ok := w.troughOf(e); !ok {
		w.clearJob(e)
		return
	}
	e.Target, e.tend, e.Progress = e.trough, tendFill, 0
}

// tendFillTick carries the keeper's feed to its trough and tips it in.
func (w *World) tendFillTick(e *Entity) {
	c, ok := w.troughOf(e)
	n := e.ownCarried(Feed)
	if !ok || n == 0 || !c.Inventory.CanAdd(Feed, n) {
		w.clearJob(e)
		return
	}
	arrived, ok := w.travelTo(e, e.trough)
	if !ok {
		w.clearJob(e)
		return
	}
	if !arrived {
		e.State = Hauling
		return
	}
	if e.Inventory.Remove(Feed, n) && c.Inventory.Add(Feed, n) {
		c.credit(ColonistOwner(e.ID), Feed, n)
	}
	w.clearJob(e)
}

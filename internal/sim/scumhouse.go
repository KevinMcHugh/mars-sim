package sim

import (
	"fmt"
	"sort"
)

// ---- Scumhouse ---------------------------------------------------------------
//
// A scumhouse turns biomatter into meals of slurry. Biomatter is cave scum
// scraped off the rock, viscera scrubbed off the floor, and every body but a
// colonist's. Colonists bring it in as community work — scraping scum,
// cleaning up after a death — and it goes into the scumhouse's depot on the
// colony's account; a colonist working the scumhouse turns the colony's
// biomatter into the colony's meals, which anyone may eat (see food.go).
//
// What turns into what is the recipe table, below. See docs/scumhouse.md.

// SkillKind is the expertise a recipe calls for. It is a placeholder: everyone
// can do everything for now, and every recipe asks for SkillNone. It exists so
// the recipe table's shape does not change when skills arrive (see
// docs/economy.md).
type SkillKind uint8

const SkillNone SkillKind = 0

// Recipe is one way of making something: consume Inputs from a workshop's
// depot, spend Ticks of labor at it, and put Outputs in the same depot. Whoever
// owns the inputs owns the outputs — the workshop's owner does not, which is
// what will let a machine be capital someone rents out rather than a job
// someone holds.
type Recipe struct {
	Name     string
	Inputs   []ItemStack
	Outputs  []ItemStack
	Facility Terrain
	Ticks    int // labor, scaled by the worker's workScale
	Skill    SkillKind
}

// recipes is every recipe in the game, in preference order: a cook works the
// first one it has the inputs for. Rendering an alien carcass comes first
// because it is the most food for the work and the least pleasant thing to
// leave lying in a depot.
var recipes = []Recipe{
	{Name: "render an alien carcass", Inputs: []ItemStack{{AlienCorpse, 1}}, Outputs: []ItemStack{{Meal, 4}}, Facility: Scumhouse, Ticks: 30},
	{Name: "render an animal carcass", Inputs: []ItemStack{{AnimalCorpse, 1}}, Outputs: []ItemStack{{Meal, 1}}, Facility: Scumhouse, Ticks: 10},
	{Name: "press viscera", Inputs: []ItemStack{{Viscera, 2}}, Outputs: []ItemStack{{Meal, 1}}, Facility: Scumhouse, Ticks: 10},
	{Name: "culture cave scum", Inputs: []ItemStack{{CaveScum, 2}}, Outputs: []ItemStack{{Meal, 1}}, Facility: Scumhouse, Ticks: 12},
}

// ---- Cave scum ------------------------------------------------------------------

// scumPatch is the biofilm on one tile. Like a need, its level is lazy: amount
// is what it held at tick since, and it regrows a unit every ScumRegrowTicks
// from there, up to ScumMax, without any per-tick work.
type scumPatch struct {
	amount int
	since  int
}

// scumAt is how much scum is on p now.
func (w *World) scumAt(p Point) int {
	s, ok := w.scum[p]
	if !ok {
		return 0
	}
	if w.cfg.ScumRegrowTicks > 0 {
		s.amount += (w.tick - s.since) / w.cfg.ScumRegrowTicks
	}
	return min(s.amount, w.cfg.ScumMax)
}

// takeScum scrapes one unit off p, reporting whether there was any. Regrowth
// restarts from now, so a patch scraped bare takes a full ScumRegrowTicks to
// show a unit again.
func (w *World) takeScum(p Point) bool {
	n := w.scumAt(p)
	if n == 0 {
		return false
	}
	w.scum[p] = scumPatch{amount: n - 1, since: w.tick}
	w.scumRev++
	return true
}

// clearScum destroys the patch on p: something was built over it.
func (w *World) clearScum(p Point) {
	if _, ok := w.scum[p]; ok {
		delete(w.scum, p)
		delete(w.exposedScum, p)
		w.scumRev++
	}
}

// scumExposed reports whether a colonist can get at p's patch: it is on
// walkable floor, or on rock with walkable floor beside it — floor the colony
// has discovered. The rim of a natural cavern nobody has broken into is not
// exposed: no colonist or rat can reach it, and counting it once made nearly
// every patch on a big map "exposed", so the scrapers' search and every
// published frame walked thousands of patches nobody could touch (see
// docs/caverns.md). discoverCavernTile re-checks a cavern's rim when it is
// found.
func (w *World) scumExposed(p Point) bool {
	if w.Walkable(p) {
		return w.discovered(p)
	}
	if w.TerrainAt(p) != Rock {
		return false
	}
	for _, d := range neighbors8 {
		if q := p.Add(d.X, d.Y); w.Walkable(q) && w.discovered(q) {
			return true
		}
	}
	return false
}

// refreshScumExposure re-evaluates p and its neighbors after a terrain change,
// the way the job board refreshes the mining frontier.
func (w *World) refreshScumExposure(p Point) {
	check := func(q Point) {
		if _, ok := w.scum[q]; !ok {
			return
		}
		_, was := w.exposedScum[q]
		if now := w.scumExposed(q); now == was {
			return
		} else if now {
			w.exposedScum[q] = struct{}{}
		} else {
			delete(w.exposedScum, q)
		}
		w.scumRev++
	}
	check(p)
	for _, d := range neighbors8 {
		check(p.Add(d.X, d.Y))
	}
}

// Cave scum is laid down by world generation, chunk by chunk, as a pure
// function of the seed like the ore veins (see scumPlan in
// worldgen_chunks.go and applyChunk).

// ---- Food work ------------------------------------------------------------------

// communityMeals is how many meals the colony owns across every depot. It is
// memoized for the tick: every work-seeking colonist asks, and a depot per
// settler (crash-pod lockers) makes the walk cost a colony's size.
func (w *World) communityMeals() int {
	if w.communityMealsTick == w.tick {
		return w.communityMealsCache
	}
	n := 0
	for _, c := range w.storageContainers {
		n += c.held(Community, Meal)
	}
	for _, o := range w.orders {
		if o.Side == Ask && o.Item == Meal && o.Actor == Community {
			n += o.Qty // on offer: still the colony's until it sells
		}
	}
	w.communityMealsTick, w.communityMealsCache = w.tick, n
	return n
}

// foodWanted reports whether the colony should be making food: it has a
// scumhouse, and holds fewer than MealReserve meals per colonist.
func (w *World) foodWanted() bool {
	return w.countTerrain(Scumhouse) > 0 &&
		w.communityMeals() < w.cfg.MealReserve*w.countKind(Colonist)
}

// tryAssignFoodWork commits e to whichever food job is available: cooking
// what the scumhouse already holds, then gathering more. force is a hungry
// colonist with nothing to eat and no meal it can buy: it skips the colony's
// reserve check, cooks only its own scum, and scrapes to keep rather than to
// sell, so a colonist with no money can still feed itself.
func (w *World) tryAssignFoodWork(e *Entity, force bool) bool {
	if force {
		return w.tryAssignCraftFor(e, []Owner{ColonistOwner(e.ID)}) || w.tryAssignScrape(e, true)
	}
	if !w.foodWanted() {
		return false
	}
	return w.tryAssignCraft(e) || w.tryAssignScrape(e, false)
}

// nearestScumhouse finds the nearest reachable scumhouse e may use that
// passes ok, ties by position.
func (w *World) nearestScumhouse(e *Entity, ok func(*StorageContainer) bool) (Point, bool) {
	room := w.roomOf(e.Pos)
	var best Point
	bestDist, found := 1<<30, false
	for p := range w.facilityTiles[Scumhouse] {
		c := w.storageContainers[p]
		if c == nil || !w.canUseFixture(e, p) || !w.taskReachable(p, room) || (ok != nil && !ok(c)) {
			continue
		}
		d := e.Pos.Chebyshev(p)
		if !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	return best, found
}

// canCraft reports whether owner has the inputs for r in the workshop depot
// c, and the outputs would fit where they go: the workshop's pantry if it has
// one, else c itself once the inputs are gone. A stove whose depot is full of
// scum can still cook into an empty pantry.
func (w *World) canCraft(c *StorageContainer, r Recipe, owner Owner) bool {
	for _, in := range r.Inputs {
		if c.held(owner, in.Kind) < in.Count {
			return false
		}
	}
	if out := w.outputDepot(c.Pos); out != c.Pos {
		return w.storageContainers[out].Inventory.CanAddAll(r.Outputs...)
	}
	after := c.Inventory
	for _, in := range r.Inputs {
		after.Remove(in.Kind, in.Count)
	}
	return after.CanAddAll(r.Outputs...)
}

// tryAssignCraft sends e to the nearest free scumhouse holding the inputs for
// a recipe that e may use: its own, or the colony's (paid work).
func (w *World) tryAssignCraft(e *Entity) bool {
	return w.tryAssignCraftFor(e, []Owner{ColonistOwner(e.ID), Community})
}

// tryAssignCraftFor is tryAssignCraft with the inputs' owners to look for, in
// preference order.
func (w *World) tryAssignCraftFor(e *Entity, owners []Owner) bool {
	// The filter only answers "could e cook here?". Which recipe, for whom,
	// is worked out afterwards for the scumhouse actually chosen: the filter
	// runs on every candidate in map order, and recording the recipe from
	// inside it handed the cook whichever candidate happened to be checked
	// last — harmless with one scumhouse, a determinism bug with several.
	p, ok := w.nearestScumhouse(e, func(c *StorageContainer) bool {
		if id := w.workshopClaims[c.Pos]; id != 0 && id != e.ID {
			return false // one cook per workshop
		}
		if w.mealFetchesAt(c.Pos) > 0 {
			return false // someone is coming for a meal: don't stand on the counter
		}
		_, _, ok := w.craftableRecipe(c, owners)
		return ok
	})
	if !ok {
		return false
	}
	recipe, owner, _ := w.craftableRecipe(w.storageContainers[p], owners)
	w.workshopClaims[p] = e.ID
	e.Job, e.Target, e.Progress = JobCraft, p, 0
	e.recipe, e.craftFor, e.craftRun = recipe, owner, 0
	return true
}

// craftableRecipe is the first recipe (in table order) that c can work for
// one of owners (in preference order), if any.
func (w *World) craftableRecipe(c *StorageContainer, owners []Owner) (int, Owner, bool) {
	for i, r := range recipes {
		if r.Facility != c.Terrain {
			continue
		}
		for _, o := range owners {
			if w.canCraft(c, r, o) {
				return i, o, true
			}
		}
	}
	return 0, Owner{}, false
}

// jobCraft walks to the claimed workshop and works its recipe.
func (w *World) jobCraft(e *Entity) {
	c := w.storageContainers[e.Target]
	r := recipes[e.recipe]
	if c == nil || !w.canCraft(c, r, e.craftFor) {
		w.clearJob(e) // somebody used the inputs, or the workshop is gone
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
	if e.Progress < scaleTicks(r.Ticks, e.workScale) {
		return
	}
	for _, in := range r.Inputs {
		c.debit(e.craftFor, in.Kind, in.Count)
	}
	// Down the line: the output goes straight into the pantry, if the
	// kitchen has one, or back into the stove's own depot if not.
	out := w.storageContainers[w.outputDepot(e.Target)]
	outputs := r.Outputs
	if w.cooksOwnSupper(e, r) {
		// A hungry colonist cooking its own food keeps one meal in hand to
		// eat at the stove, rather than walking to the pantry for it.
		e.Inventory.Add(Meal, 1)
		outputs = withoutOneMeal(outputs)
	}
	out.Inventory.AddAll(outputs...)
	for _, o := range outputs {
		out.credit(e.craftFor, o.Kind, o.Count)
	}
	w.emitDone(e, ActionCook, NounMeal, "Worked the scumhouse: %s.", r.Name)
	if e.craftFor == Community {
		if w.cfg.WageCook > 0 {
			w.transfer(Community, ColonistOwner(e.ID), Money(w.cfg.WageCook)) // as far as the treasury goes
		}
		w.offerColonyMeals(out.Pos) // straight onto the counter: a waiting bid takes it now
	}
	if p := w.plans[e.plan]; p != nil && p.kind == planCraft && p.workshop == e.Target {
		p.crafted = true
	}
	if w.cooksOn(e, c, r) {
		e.Progress = 0
		e.craftRun++
		return
	}
	w.clearJob(e)
}

// cookBatch is how many recipes a colony cook works back to back before it
// gives the stove up.
const cookBatch = 6

// cooksOn reports whether a colony cook that just finished r at c starts the
// same recipe again rather than leaving: while the colony still wants food,
// the stove still holds the inputs, the cook is not hungry itself, and it has
// worked fewer than cookBatch in a row. A cook walked across the colony for
// one twelve-tick recipe and left, so late in long runs twenty colonists
// shared two stoves that stood idle most of the time, and ate faster than
// the few cooks who came by could cook.
func (w *World) cooksOn(e *Entity, c *StorageContainer, r Recipe) bool {
	if e.craftFor != Community || e.plan != 0 || e.craftRun+1 >= cookBatch || !w.foodWanted() {
		return false
	}
	if e.needPhase[NeedFood] >= NeedPressing || w.mealFetchesAt(c.Pos) > 0 {
		return false
	}
	return w.canCraft(c, r, Community)
}

// scrapeLoad is how much scum a scraper gathers before hauling it in: one
// patch's worth.
func (w *World) scrapeLoad() int { return max(1, w.cfg.ScumMax) }

// tryAssignScrape sends e to scrape scum: deliver any it is already carrying,
// or head for the nearest unclaimed exposed patch with scum on it — but only
// while a reachable scumhouse has room for the load. The scum is e's own; at
// the scumhouse it sells into the colony's bid, unless keep, when it holds on
// to it to cook for itself.
func (w *World) tryAssignScrape(e *Entity, keep bool) bool {
	load := w.scrapeLoad()
	me := ColonistOwner(e.ID)
	house, ok := w.nearestScumhouse(e, func(c *StorageContainer) bool {
		if !c.Inventory.CanAdd(CaveScum, load) {
			return false
		}
		if keep {
			return true
		}
		// Scraping to sell needs a buyer. Without this check scrapers kept
		// bringing scum to a scumhouse whose colony had stopped buying,
		// and hundreds of unsold units piled up in its depot.
		bid, ok := w.bestBid(CaveScum, c.Pos)
		return ok && bid.Actor != me
	})
	if !ok {
		return false
	}
	if e.Inventory.Has(CaveScum) {
		e.Job, e.Target, e.scrape, e.Progress = JobScrape, house, scrapeHaul, 0
		e.scrapeKeep = keep
		return true
	}
	if !e.Inventory.CanAdd(CaveScum, load) {
		return false
	}
	patch, ok := w.nearestScum(e)
	if !ok {
		return false
	}
	w.scumClaims[patch] = e.ID
	e.Job, e.Target, e.scrape, e.Progress = JobScrape, patch, scrapeGather, 0
	e.scrapeKeep = keep
	return true
}

// nearestScum finds the nearest exposed, unclaimed patch with scum on it that
// e can reach. The exposed set is small next to the map, so it is walked
// outright; ties break by position.
func (w *World) nearestScum(e *Entity) (Point, bool) {
	room := w.roomOf(e.Pos)
	var best Point
	bestDist, found := 1<<30, false
	for p := range w.exposedScum {
		if id := w.scumClaims[p]; id != 0 && id != e.ID {
			continue
		}
		if w.scumAt(p) == 0 {
			continue
		}
		reachable := w.taskReachable(p, room)
		if w.Walkable(p) {
			reachable = w.roomOf(p) == room
		}
		if !reachable {
			continue
		}
		d := e.Pos.Chebyshev(p)
		if !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	return best, found
}

// scrapeStage is where a JobScrape colonist is.
type scrapeStage uint8

const (
	scrapeGather scrapeStage = iota // scraping the patch at Target
	scrapeHaul                      // carrying the load to the scumhouse at Target
)

// jobScrape runs one tick of scraping: work the patch a unit at a time until
// it is bare or the load is full, then haul it in.
func (w *World) jobScrape(e *Entity) {
	if e.scrape == scrapeHaul {
		w.jobDeliverBiomatter(e)
		return
	}
	load := w.scrapeLoad()
	if e.scrapeQty > 0 {
		load = e.scrapeQty
	}
	if w.scumAt(e.Target) == 0 || e.Inventory.Count(CaveScum) >= load ||
		!e.Inventory.CanAdd(CaveScum, 1) {
		w.finishScraping(e)
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
	if e.Progress < scaleTicks(w.cfg.ScrapeTicks, e.workScale) {
		return
	}
	e.Progress = 0
	if w.takeScum(e.Target) {
		e.Inventory.Add(CaveScum, 1)
		e.addCargo(e.scrapeFor, CaveScum, 1) // the scraper's own unless set
	}
}

// finishScraping ends the gather leg: hand the load to the haul leg, or end
// the job if there is nothing to haul or nowhere to take it.
func (w *World) finishScraping(e *Entity) {
	delete(w.scumClaims, e.Target)
	if !e.Inventory.Has(CaveScum) {
		w.clearJob(e)
		return
	}
	house, ok := w.nearestScumhouse(e, func(c *StorageContainer) bool {
		return c.Inventory.CanAdd(CaveScum, e.Inventory.Count(CaveScum))
	})
	if p := w.plans[e.plan]; p != nil && p.kind == planGather {
		house, ok = p.depot, true // to the scumhouse whose bid it is filling
	}
	if !ok {
		w.clearJob(e) // the load stays carried until a scumhouse can take it
		return
	}
	w.emitDone(e, ActionScrape, NounScum, "Scraped %d units of cave scum at (%d, %d).",
		e.Inventory.Count(CaveScum), e.Target.X, e.Target.Y)
	e.Target, e.scrape, e.Progress = house, scrapeHaul, 0
}

// jobDeliverBiomatter carries a load of biomatter to the scumhouse at Target
// and puts it in the depot.
func (w *World) jobDeliverBiomatter(e *Entity) {
	c := w.storageContainers[e.Target]
	if c == nil || c.Terrain != Scumhouse || !c.Inventory.CanAddAll(biomatterStacks(e)...) {
		w.clearJob(e)
		return
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
	w.deliverBiomatter(e, c)
	if p := w.plans[e.plan]; p != nil && p.kind == planGather && p.depot == c.Pos {
		w.sellGathered(e, p)
	}
	w.clearJob(e)
}

// biomatterStacks lists the biomatter e is carrying.
func biomatterStacks(e *Entity) []ItemStack {
	var out []ItemStack
	for _, s := range e.Inventory {
		if s.Count > 0 && s.Kind.isBiomatter() {
			out = append(out, s)
		}
	}
	return out
}

// deliverBiomatter moves every unit of biomatter e carries into c, credited to
// whoever it is carried for (see carriedOwner). All or nothing, like any
// deposit. What was e's own it then sells into the colony's standing bids
// there (sellBiomatter), unless it is scraping to keep.
func (w *World) deliverBiomatter(e *Entity, c *StorageContainer) bool {
	stacks := biomatterStacks(e)
	if len(stacks) == 0 || !c.Inventory.AddAll(stacks...) {
		return false
	}
	me := ColonistOwner(e.ID)
	var mine []ItemStack
	for _, s := range stacks {
		for _, share := range e.unloadCargo(s.Kind) {
			c.credit(share.Owner, s.Kind, share.N)
			if share.Owner == me {
				mine = append(mine, ItemStack{s.Kind, share.N})
			}
		}
		e.Inventory.RemoveAll(s.Kind)
	}
	if !e.scrapeKeep {
		w.sellBiomatter(e, c, mine)
	}
	w.emitDone(e, ActionDeliver, NounScumhouse, "Brought %s to the scumhouse.", stackPhrase(stacks))
	return true
}

// stackPhrase renders a load for a memory: "3 cave scum and 1 viscera".
func stackPhrase(stacks []ItemStack) string {
	out := ""
	for i, s := range stacks {
		if i > 0 {
			if i == len(stacks)-1 {
				out += " and "
			} else {
				out += ", "
			}
		}
		out += fmt.Sprintf("%d %s", s.Count, s.Kind)
	}
	return out
}

// ---- The colony's scumhouse trade --------------------------------------------------
//
// The colony runs its scumhouses as a business: it buys biomatter at a
// standing bid, pays a cook to work it, and sells the meals. Nothing it cooks
// is free for the taking. See docs/scumhouse.md.

// biomatterPrice is what the colony pays for a unit of k at its scumhouses; 0
// for anything that is not biomatter, or that it does not buy.
func (w *World) biomatterPrice(k ItemKind) Money {
	switch k {
	case CaveScum:
		return Money(w.cfg.PriceCaveScum)
	case Viscera:
		return Money(w.cfg.PriceViscera)
	case AnimalCorpse:
		return Money(w.cfg.PriceAnimalCorpse)
	case AlienCorpse:
		return Money(w.cfg.PriceAlienCorpse)
	default:
		return 0
	}
}

// biomatterKinds are the goods the colony bids for at its scumhouses.
var biomatterKinds = [...]ItemKind{CaveScum, Viscera, AnimalCorpse, AlienCorpse}

// scumhousesSorted lists every scumhouse by position.
func (w *World) scumhousesSorted() []Point {
	out := make([]Point, 0, len(w.facilityTiles[Scumhouse]))
	for p := range w.facilityTiles[Scumhouse] {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return lessPoint(out[i], out[j]) })
	return out
}

// refreshBiomatterBids keeps the colony's standing bids for biomatter at each
// scumhouse topped up to scumhouse-bid-qty units per kind, at biomatterPrice,
// as far as the treasury stretches — and only while the depot has room for
// what it would buy.
func (w *World) refreshBiomatterBids() {
	for _, p := range w.scumhousesSorted() {
		c := w.storageContainers[p]
		if c == nil {
			continue
		}
		for _, k := range biomatterKinds {
			price := w.biomatterPrice(k)
			if price <= 0 {
				continue
			}
			want := w.cfg.ScumhouseBidQty - w.openQty(Bid, k, p, Community)
			if cap := w.cfg.ScumhouseStockCap; cap > 0 {
				want = min(want, cap-c.held(Community, k)-w.openQty(Bid, k, p, Community))
			}
			want = min(want, int(w.treasury/price))
			for want > 0 && !c.Inventory.CanAdd(k, want) {
				want--
			}
			if want > 0 {
				w.post(Bid, k, want, price, Community, p, 0)
			}
		}
	}
}

// sellBiomatter offers the biomatter e just delivered, its own, into the best
// bids at c: the colony's standing bids, or a producer's. Whatever no bid
// takes rests at the best bid's price for order-ttl; with no bid at all it
// simply stays e's, in the depot, for e to cook or sell later.
func (w *World) sellBiomatter(e *Entity, c *StorageContainer, stacks []ItemStack) {
	me := ColonistOwner(e.ID)
	for _, s := range stacks {
		bid, ok := w.bestBid(s.Kind, c.Pos)
		if !ok || bid.Actor == me {
			continue
		}
		if n := min(s.Count, c.held(me, s.Kind)); n > 0 {
			w.post(Ask, s.Kind, n, bid.Price, me, c.Pos, w.cfg.OrderTTL)
		}
	}
}

// pendingHaul is how many units of the colony's item open haul orders will
// take out of the depot at from.
func (w *World) pendingHaul(item ItemKind, from Point) int {
	n := 0
	for _, o := range w.workOrders {
		if o.Kind == WorkHaul && o.Issuer == Community && o.Item == item && o.From == from {
			n += o.Units
		}
	}
	return n
}

// refreshColonyMealAsks offers every meal the colony holds, at each scumhouse
// and at the silo, at the charter's meal price — less any a haul order is
// about to take to the silo. This is the scumhouse charging for its meals.
func (w *World) refreshColonyMealAsks() {
	price := w.refPrice(Meal)
	if price <= 0 {
		return
	}
	for _, p := range w.mealDepots() {
		w.offerColonyMeals(p)
	}
}

// offerColonyMeals offers every meal the colony holds at p, less any a haul
// order is about to take, at the charter's meal price. Resting bids there —
// hungry colonists queued for a meal — fill at once.
func (w *World) offerColonyMeals(p Point) {
	c := w.storageContainers[p]
	price := w.refPrice(Meal)
	if c == nil || price <= 0 {
		return
	}
	if spare := c.held(Community, Meal) - w.pendingHaul(Meal, p); spare > 0 {
		w.post(Ask, Meal, spare, price, Community, p, 0)
	}
}

// withdrawColonyAsks takes the colony's asks for item at p off the book,
// returning the goods to its ledger line — so they can be hauled.
func (w *World) withdrawColonyAsks(item ItemKind, p Point) {
	for _, o := range w.sortedOrders(func(o *Order) bool {
		return o.Side == Ask && o.Item == item && o.Depot == p && o.Actor == Community
	}) {
		w.cancel(o)
	}
}

// mealFetchesAt is how many colonists are on their way to take a meal out of
// the depot at p. A cook stands on a workshop's access tile for the whole of
// a recipe; one that went straight on to the next recipe, and the next, could
// hold a narrow room's only access tile against a starving colonist coming
// for a meal it owned. So a cook does not start a recipe while anyone is
// coming. It is memoized for the tick: every work-seeking colonist asks.
func (w *World) mealFetchesAt(p Point) int {
	if w.mealFetchTick != w.tick || w.mealFetches == nil {
		if w.mealFetches == nil {
			w.mealFetches = make(map[Point]int)
		}
		clear(w.mealFetches)
		for _, e := range w.entities {
			if e.Kind == Colonist && e.Job == JobEat && e.eat == eatFetch {
				w.mealFetches[e.Target]++
			}
		}
		w.mealFetchTick = w.tick
	}
	return w.mealFetches[p]
}

// ---- Pantries ---------------------------------------------------------------------
//
// A kitchen is an assembly line: the scumhouse cooks, and a pantry beside it
// — an ordinary chest the room planner links to it (linkPantry) — takes the
// meals. Selling and fetching meals happen at the pantry, so they never
// compete with the cook for the stove's access tiles. A scumhouse without a
// pantry (a cramped first kitchen, or one built before pantries) keeps its
// meals in its own depot. See docs/scumhouse.md.

// linkPantry records which chest of a newly designated scumhouse room is its
// pantry. The link is made when the room is marked out, where both positions
// are known, rather than guessed later from what stands near what.
func (w *World) linkPantry(p *project) {
	var house, pantry Point
	var hasHouse, hasPantry bool
	for _, t := range p.tasks {
		switch t.terrain {
		case Scumhouse:
			house, hasHouse = t.pos, true
		case Storage:
			pantry, hasPantry = t.pos, true
		}
	}
	if hasHouse && hasPantry {
		w.pantryOf[house] = pantry
		w.pantryHouse[pantry] = house
	}
}

// pantryFor is the built pantry of the scumhouse at house, if it has one.
func (w *World) pantryFor(house Point) (Point, bool) {
	p, ok := w.pantryOf[house]
	if !ok || w.TerrainAt(p) != Storage || w.storageContainers[p] == nil {
		return Point{}, false
	}
	return p, true
}

// isPantry reports whether the chest at p is some kitchen's pantry. A pantry
// is for meals: it is not the silo, and nobody unloads ore into it.
func (w *World) isPantry(p Point) bool {
	_, ok := w.pantryHouse[p]
	return ok && w.TerrainAt(p) == Storage
}

// outputDepot is where the scumhouse at house puts what it cooks: its
// pantry, or its own depot if it has none.
func (w *World) outputDepot(house Point) Point {
	if p, ok := w.pantryFor(house); ok {
		return p
	}
	return house
}

// mealDepots is every depot the colony sells its meals from: each scumhouse,
// each pantry, and the silo, in a fixed order.
func (w *World) mealDepots() []Point {
	var out []Point
	for _, h := range w.scumhousesSorted() {
		out = append(out, h)
		if p, ok := w.pantryFor(h); ok {
			out = append(out, p)
		}
	}
	if silo, ok := w.marketDepot(); ok {
		out = append(out, silo)
	}
	return out
}

// cooksOwnSupper reports whether e, cooking r, is a hungry colonist cooking
// its own food with room in its pockets for a meal of the output.
func (w *World) cooksOwnSupper(e *Entity, r Recipe) bool {
	if e.craftFor != ColonistOwner(e.ID) || !e.Inventory.CanAdd(Meal, 1) {
		return false
	}
	if phase := e.needPhase[NeedFood]; phase != NeedPressing && phase != NeedCritical {
		return false
	}
	for _, o := range r.Outputs {
		if o.Kind == Meal && o.Count > 0 {
			return true
		}
	}
	return false
}

// withoutOneMeal is outputs less one meal.
func withoutOneMeal(outputs []ItemStack) []ItemStack {
	out := make([]ItemStack, 0, len(outputs))
	taken := false
	for _, o := range outputs {
		if !taken && o.Kind == Meal && o.Count > 0 {
			taken = true
			if o.Count--; o.Count == 0 {
				continue
			}
		}
		out = append(out, o)
	}
	return out
}

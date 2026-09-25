package sim

import (
	"fmt"
	"math/rand"
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

// growScum seeds patches of cave scum at worldgen: ScumPercent of the map's
// rock, in short meandering runs, each patch full. It draws from its own
// stream (like rock composition) so that scum never shifts where anything else
// lands for a seed.
func (w *World) growScum(rng *rand.Rand) {
	if w.cfg.ScumPercent <= 0 || w.cfg.ScumMax <= 0 {
		return
	}
	target := len(w.tiles) * w.cfg.ScumPercent / 100
	for placed, guard := 0, 0; placed < target && guard < target*8; guard++ {
		p := Point{rng.Intn(w.Width), rng.Intn(w.Height)}
		for run := 3 + rng.Intn(6); run > 0 && placed < target; run-- {
			if w.InBounds(p) && w.TerrainAt(p) == Rock {
				if _, ok := w.scum[p]; !ok {
					w.scum[p] = scumPatch{amount: w.cfg.ScumMax}
					placed++
				}
			}
			d := veinNeighbors[rng.Intn(len(veinNeighbors))]
			p = p.Add(d.X, d.Y)
		}
	}
}

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

// craftable reports whether owner has the inputs for r in c, and the outputs
// would fit once those inputs are gone.
func craftable(c *StorageContainer, r Recipe, owner Owner) bool {
	for _, in := range r.Inputs {
		if c.held(owner, in.Kind) < in.Count {
			return false
		}
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
	var recipe int
	var owner Owner
	p, ok := w.nearestScumhouse(e, func(c *StorageContainer) bool {
		if id := w.workshopClaims[c.Pos]; id != 0 && id != e.ID {
			return false // one cook per workshop
		}
		for i, r := range recipes {
			if r.Facility != c.Terrain {
				continue
			}
			for _, o := range owners {
				if craftable(c, r, o) {
					recipe, owner = i, o
					return true
				}
			}
		}
		return false
	})
	if !ok {
		return false
	}
	w.workshopClaims[p] = e.ID
	e.Job, e.Target, e.Progress = JobCraft, p, 0
	e.recipe, e.craftFor = recipe, owner
	return true
}

// jobCraft walks to the claimed workshop and works its recipe.
func (w *World) jobCraft(e *Entity) {
	c := w.storageContainers[e.Target]
	r := recipes[e.recipe]
	if c == nil || !craftable(c, r, e.craftFor) {
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
	c.Inventory.AddAll(r.Outputs...)
	for _, out := range r.Outputs {
		c.credit(e.craftFor, out.Kind, out.Count)
	}
	w.remember(e, event(EvtMadeSlurry, "Worked the scumhouse: %s.", r.Name))
	if e.craftFor == Community && w.cfg.WageCook > 0 {
		w.transfer(Community, ColonistOwner(e.ID), Money(w.cfg.WageCook)) // as far as the treasury goes
	}
	if p := w.plans[e.plan]; p != nil && p.kind == planCraft && p.workshop == e.Target {
		p.crafted = true
	}
	w.clearJob(e)
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
	house, ok := w.nearestScumhouse(e, func(c *StorageContainer) bool {
		return c.Inventory.CanAdd(CaveScum, load)
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
		e.cargo[CaveScum] = e.scrapeFor // the scraper's own unless set
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
	w.remember(e, event(EvtScrapedScum, "Scraped %d units of cave scum at (%d, %d).",
		e.Inventory.Count(CaveScum), e.Target.X, e.Target.Y))
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
		owner := w.carriedOwner(e, s.Kind)
		c.credit(owner, s.Kind, s.Count)
		e.Inventory.RemoveAll(s.Kind)
		e.cargo[s.Kind] = Owner{}
		if owner == me {
			mine = append(mine, s)
		}
	}
	if !e.scrapeKeep {
		w.sellBiomatter(e, c, mine)
	}
	w.remember(e, event(EvtFedScumhouse, "Brought %s to the scumhouse.", stackPhrase(stacks)))
	return true
}

// carriedOwner is whose the items of kind a colonist is carrying are: the
// cargo record's owner, if a job put one there, and otherwise the carrier's
// own. See docs/property.md.
func (w *World) carriedOwner(e *Entity, kind ItemKind) Owner {
	if o := e.cargo[kind]; o.Kind != OwnerNone {
		return o
	}
	return ColonistOwner(e.ID)
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
	depots := w.scumhousesSorted()
	if silo, ok := w.marketDepot(); ok {
		depots = append(depots, silo)
	}
	for _, p := range depots {
		c := w.storageContainers[p]
		if c == nil {
			continue
		}
		if spare := c.held(Community, Meal) - w.pendingHaul(Meal, p); spare > 0 {
			w.post(Ask, Meal, spare, price, Community, p, 0)
		}
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

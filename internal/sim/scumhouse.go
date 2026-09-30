package sim

import (
	"cmp"
	"fmt"
	"slices"
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
	Ticks    int       // labor at base skill, before the worker's skill and workScale
	Skill    SkillKind // what working it practises, and whose rank speeds it up (skills.go)
}

// recipes is every recipe in the game, in preference order: a cook works the
// first one it has the inputs for. Rendering an alien carcass comes first
// because it is the most food for the work and the least pleasant thing to
// leave lying in a depot.
var recipes = []Recipe{
	{Name: "render an alien carcass", Inputs: []ItemStack{{AlienCorpse, 1}}, Outputs: []ItemStack{{Meal, 4}}, Facility: Scumhouse, Ticks: 30, Skill: SkillCooking},
	{Name: "render an animal carcass", Inputs: []ItemStack{{AnimalCorpse, 1}}, Outputs: []ItemStack{{Meal, 1}}, Facility: Scumhouse, Ticks: 10, Skill: SkillCooking},
	{Name: "press viscera", Inputs: []ItemStack{{Viscera, 2}}, Outputs: []ItemStack{{Meal, 1}}, Facility: Scumhouse, Ticks: 10, Skill: SkillCooking},
	{Name: "culture cave scum", Inputs: []ItemStack{{CaveScum, 2}}, Outputs: []ItemStack{{Meal, 1}}, Facility: Scumhouse, Ticks: 12, Skill: SkillCooking},
	// The foundry's chain: ore to steel at the forge, steel to rifles at the
	// gun bench. See docs/foundry.md.
	{Name: "smelt steel", Inputs: []ItemStack{{IronOre, 2}}, Outputs: []ItemStack{{SteelIngot, 1}}, Facility: Forge, Ticks: 40, Skill: SkillSmithing},
	{Name: "machine an assault rifle", Inputs: []ItemStack{{SteelIngot, 3}}, Outputs: []ItemStack{{AssaultRifle, 1}}, Facility: GunBench, Ticks: 60, Skill: SkillSmithing},
}

// ---- Cave scum ------------------------------------------------------------------

// scumPatch is the biofilm on one tile: up to ScumMax units. Nothing on the
// tile regrows it by itself. Patches accrete: see growScum.
type scumPatch struct {
	amount int
}

// scumAt is how much scum is on p now.
func (w *World) scumAt(p Point) int {
	return min(w.scum[p].amount, w.cfg.ScumMax)
}

// takeScum scrapes one unit off p, reporting whether there was any. A patch
// scraped bare is gone: it comes back only the way scum first arrives, by
// spawning or by spreading from a neighbour.
func (w *World) takeScum(p Point) bool {
	n := w.scumAt(p)
	if n == 0 {
		return false
	}
	if n == 1 {
		w.clearScum(p)
		return true
	}
	w.scum[p] = scumPatch{amount: n - 1}
	w.scumRev++
	return true
}

// clearScum destroys the patch on p: something was built over it.
func (w *World) clearScum(p Point) {
	if _, ok := w.scum[p]; ok {
		delete(w.scum, p)
		delete(w.exposedScum, p)
		if i, found := slices.BinarySearchFunc(w.scumPatches, p, cmpScumPatch); found {
			w.scumPatches = slices.Delete(w.scumPatches, i, i+1)
		}
		w.scumRev++
	}
}

// setScum puts amount units on p, listing p in scumPatches if the patch is
// new. Every patch goes on the map through here or applyChunk, so the two
// agree.
func (w *World) setScum(p Point, amount int) {
	if _, ok := w.scum[p]; !ok {
		i, _ := slices.BinarySearchFunc(w.scumPatches, p, cmpScumPatch)
		w.scumPatches = slices.Insert(w.scumPatches, i, p)
	}
	w.scum[p] = scumPatch{amount: amount}
}

// cmpScumPatch orders patches by chunk, the way genChunks is ordered, then
// row by row within the chunk, so a newly generated chunk's patches sit
// together in scumPatches and go in with one insert.
func cmpScumPatch(a, b Point) int {
	ka := chunkKey{int32(a.X >> genChunkBits), int32(a.Y >> genChunkBits)}
	kb := chunkKey{int32(b.X >> genChunkBits), int32(b.Y >> genChunkBits)}
	if c := cmpChunkKey(ka, kb); c != 0 {
		return c
	}
	if a.Y != b.Y {
		return cmp.Compare(a.Y, b.Y)
	}
	return cmp.Compare(a.X, b.X)
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
// worldgen_chunks.go and applyChunk). That is only the starting stock: after
// that it grows by growScum.

// scumTrialDivisor is how many ticks, on average, pass between growScum's
// visits to any one tile.
const scumTrialDivisor = 16

// growScum lets cave scum accrete. The model is a visit to every generated
// tile once per scumTrialDivisor ticks, on average, and at each:
//
//   - with a fixed low chance (ScumSpawnPPM) scum appears there from nothing;
//   - it looks at one of the nine tiles in and around it, and if that one
//     holds scum, adds a unit with chance ScumSpreadPercent.
//
// A tile with more scum around it is therefore more likely to grow, and one
// with none only ever spawns. New patches start at one unit, appear only on
// rock in a generated chunk, and stop when the map holds ScumPercent of its
// tiles in patches, so the colony's scraping is what makes room for more.
//
// It does not walk the visits; it draws only the ones that change
// something, so the rate is the same on every map and the cost follows the
// scum, not the map (scumDraws):
//
//   - Spawns: generated tiles × ScumSpawnPPM / 16 a tick, each on a uniform
//     generated tile.
//   - Spreads: turned around to start from the scum. A visit to p picks each
//     of its nine tiles with chance 1/9, so each patch is picked by each of
//     its nine tiles' visits at 1/16 × 1/9 a tick. That comes to each patch
//     sending a unit to one of its nine tiles, at random, at ScumSpreadPercent
//     / 16 a tick. Drawing from the patches (scumPatches) rather than the
//     tiles skips the visits that land beside no scum, most of them.
//
// Draws are a hash of the seed, tick and draw index, so growth needs no
// stream of its own; scumPatches is sorted, so which patch a draw picks
// depends only on which patches exist.
//
// The first version sampled tiles over the whole map, skipped the ones in
// chunks not yet generated, and capped the samples at 4,096 a tick with the
// chances scaled up to make the difference good. Spread's 40% passed 100% on
// any map over about 405×405, and past that growth ran at 4,096 × generated
// / area samples a tick: about 600 times too slow on a 10,000×10,000 map the
// colony had barely explored, and a different rate for the same ground on
// every map size. Scum near the colony stopped coming back, and the colony
// starved (see docs/scumhouse.md).
func (w *World) growScum() {
	if w.cfg.ScumMax <= 0 {
		return
	}
	// span is the tiles spawns land on: whole chunks, so a draw past the
	// map's edge in an edge chunk is a miss and every real tile gets the same
	// rate. A world built without generate (tests) is all generated.
	span := w.Width * w.Height
	if w.gen != nil {
		span = len(w.genChunks) * genChunkArea
	}
	if span == 0 {
		return
	}
	room := len(w.scum) < min(w.Width*w.Height, span)*w.cfg.ScumPercent/100
	h := uint64(w.cfg.Seed)*0x9E3779B97F4A7C15 ^ uint64(w.tick)*0xD1B54A32D192ED03
	hs := h ^ 0x5CA1AB1E // spawns draw apart from spreads
	for i := range scumDraws(span, w.cfg.ScumSpawnPPM, hs) {
		r := hs + uint64(i+1)*2*0x9E3779B97F4A7C15
		if p, ok := w.scumDrawTile(splitmix64(&r)); ok {
			w.addScum(p, room)
		}
	}
	for i := range scumDraws(len(w.scumPatches), w.cfg.ScumSpreadPercent*10000, h) {
		r := h + uint64(i+1)*2*0x9E3779B97F4A7C15 // two draws each, so no two share one
		a, b := splitmix64(&r), splitmix64(&r)
		from := w.scumPatches[a%uint64(len(w.scumPatches))]
		d := neighbors9[b%9]
		if p := from.Add(d.X, d.Y); w.generatedTile(p) {
			w.addScum(p, room)
		}
	}
}

// scumDraws is how many of n things' visits this tick roll a chance of ppm
// in a million: n/scumTrialDivisor visits, so n×ppm/16 millionths of a roll,
// whole rolls outright and the fraction on a draw from h. Its mean is exact
// for any n, which a fixed sample with its chance scaled up is not once the
// scaled chance passes certainty.
func scumDraws(n, ppm int, h uint64) int {
	if n <= 0 || ppm <= 0 {
		return 0
	}
	micro := int64(n) * int64(ppm) / scumTrialDivisor
	rolls := int(micro / 1_000_000)
	if splitmix64(&h)%1_000_000 < uint64(micro%1_000_000) {
		rolls++
	}
	return rolls
}

// scumDrawTile turns a draw into a uniform tile of the generated chunks (the
// whole map, for a world built without generate), reporting false for a tile
// past the map's edge in a partial edge chunk.
func (w *World) scumDrawTile(a uint64) (Point, bool) {
	if w.gen == nil {
		return Point{int(a % uint64(w.Width)), int((a >> 32) % uint64(w.Height))}, true
	}
	k := w.genChunks[a%uint64(len(w.genChunks))]
	off := int((a >> 32) % genChunkArea)
	p := Point{int(k.cx)<<genChunkBits + (off & (genChunkSize - 1)), int(k.cy)<<genChunkBits + (off >> genChunkBits)}
	return p, w.InBounds(p)
}

// generatedTile reports whether p is on the map in a generated chunk (every
// tile, for a world built without generate).
func (w *World) generatedTile(p Point) bool {
	return w.InBounds(p) && (w.gen == nil || w.genDone[w.tiles.pageIndex(p.X, p.Y)])
}

// neighbors9 is a tile and its eight neighbours.
var neighbors9 = [9]Point{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

// addScum adds a unit to p: thickening a patch already there, or, when room
// allows, starting one on rock.
func (w *World) addScum(p Point, room bool) {
	if s, ok := w.scum[p]; ok {
		if s.amount >= w.cfg.ScumMax {
			return
		}
		w.scum[p] = scumPatch{amount: s.amount + 1}
		if _, exposed := w.exposedScum[p]; exposed {
			w.scumRev++
		}
		return
	}
	if !room || w.TerrainAt(p) != Rock {
		return
	}
	w.setScum(p, 1)
	w.refreshScumExposure(p)
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
	return w.nearestWorkshop(e, Scumhouse, ok)
}

// nearestWorkshop finds the nearest reachable workshop of kind e may use that
// passes ok, ties by position.
func (w *World) nearestWorkshop(e *Entity, kind Terrain, ok func(*StorageContainer) bool) (Point, bool) {
	room := w.roomOf(e.Pos)
	var best Point
	bestDist, found := 1<<30, false
	for p := range w.facilityTiles[kind] {
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
		if !w.mayCookAt(e, c.Pos) {
			return false // someone else's own kitchen
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
	if e.Progress < w.workTicks(e, r.Skill, r.Ticks) {
		return
	}
	for _, in := range r.Inputs {
		c.debit(e.craftFor, in.Kind, in.Count)
	}
	// Down the line: the output goes straight into the pantry, if the
	// kitchen has one, or back into the stove's own depot if not.
	out := w.storageContainers[w.outputDepot(e.Target)]
	outputs := r.Outputs
	if len(outputs) > 0 && w.skillEffect(e, r.Skill).YieldPct > 100 {
		// A skilled worker's yield: now and then one unit more of the
		// recipe's first output, if the depot has room for it (skillYield).
		more := append([]ItemStack(nil), outputs...)
		more[0].Count++
		if w.skillYield(e, r.Skill, func() bool { return out.Inventory.CanAddAll(more...) }) {
			outputs = more
		}
	}
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
	w.practise(e, r.Skill, r.Ticks)
	if r.Facility == Scumhouse {
		w.emitDone(e, ActionCook, NounMeal, "Worked the scumhouse: %s.", r.Name)
	} else {
		w.emitDone(e, ActionCook, NounGoods, "Worked the %s: %s.", r.Facility, r.Name)
	}
	if e.craftFor == ColonistOwner(e.ID) && e.plan == 0 {
		w.offerOwnMeals(e, out.Pos)
	}
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

// cooksOn reports whether a cook that just finished r at c starts it again
// rather than leaving: it stays at the stove as long as the stove holds the
// inputs, the cook isn't hungry, and nobody is coming for a meal there. A
// colony cook also stops once the colony has its reserve; a colonist cooking
// its own scum cooks all of it.
//
// A cook at the stove is the kitchen's throughput. Every time one leaves,
// the stove waits for the next to walk over: with cooks leaving after six
// recipes, a 100-colonist colony's stoves spent 53% of the time claimed by a
// cook who wasn't cooking and only 30% cooking, and the colony starved beside
// hundreds of units of uncooked scum. A cook who stays is the division of
// labor the skills plan wants: scrapers bring scum, cooks cook, and the cook
// gets better at it (see docs/skills.md).
func (w *World) cooksOn(e *Entity, c *StorageContainer, r Recipe) bool {
	if e.plan != 0 || e.needPhase[NeedFood] >= NeedPressing || w.mealFetchesAt(c.Pos) > 0 {
		return false
	}
	switch e.craftFor {
	case Community:
		return w.foodWanted() && w.canCraft(c, r, Community)
	case ColonistOwner(e.ID):
		return w.canCraft(c, r, e.craftFor)
	}
	return false
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
			return w.mayCookAt(e, c.Pos) // scum to cook goes where it may cook
		}
		// Scraping to sell needs a buyer. Without this check scrapers kept
		// bringing scum to a scumhouse whose colony had stopped buying,
		// and hundreds of unsold units piled up in its depot.
		bid, ok := w.bestBid(CaveScum, c.Pos)
		return ok && bid.Actor != me
	})
	if own, mine := w.ownKitchen(e); mine && keep {
		if c := w.storageContainers[own]; c.Inventory.CanAdd(CaveScum, load) {
			house, ok = own, true // a chef's scum goes to its own kitchen
		}
	}
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
	if e.Progress < w.workTicks(e, SkillForaging, w.cfg.ScrapeTicks) {
		return
	}
	e.Progress = 0
	if w.takeScum(e.Target) {
		w.practise(e, SkillForaging, w.cfg.ScrapeTicks)
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
		return w.mayStockAt(e, c.Pos, CaveScum) && c.Inventory.CanAdd(CaveScum, e.Inventory.Count(CaveScum))
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
	for _, p := range w.colonyKitchens() {
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

// refreshChefBids has every chef with a kitchen of its own bid for cave scum
// there, at the colony's price, from its own wallet, while a meal it cooks
// sells for more than the scum costs. It's how a chef's kitchen is stocked:
// scrapers selling their scum take the chef's bid as readily as the colony's,
// the chef cooks what it bought (tryAssignCraftFor, for itself) and sells the
// meals from the pantry (offerOwnMeals). The colony neither buys nor cooks
// there, so without its own bids a chef's kitchen stood empty and idle while
// the colony's stoves were the ones it was bought to relieve.
//
// Scum only. Carcasses and viscera come in as refuse, now and then; bidding
// for every kind escrowed a chef's whole wallet in bids that never filled,
// and it had nothing left to buy the scum it could have cooked.
func (w *World) refreshChefBids() {
	r, ok := scumMealRecipe()
	if !ok {
		return
	}
	k, need := r.Inputs[0].Kind, r.Inputs[0].Count
	price := w.biomatterPrice(k)
	meals := 0
	for _, o := range r.Outputs {
		if o.Kind == Meal {
			meals += o.Count
		}
	}
	if price <= 0 || w.mealSellPrice()*Money(meals) <= price*Money(need) {
		return
	}
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e.Kind != Colonist || !e.Alive() {
			continue
		}
		p, ok := w.ownKitchen(e)
		if !ok {
			continue
		}
		c := w.storageContainers[p]
		me := ColonistOwner(e.ID)
		want := w.cfg.ScumhouseBidQty - w.openQty(Bid, k, p, me)
		if cap := w.cfg.ScumhouseStockCap; cap > 0 {
			want = min(want, cap-c.held(me, k)-w.openQty(Bid, k, p, me))
		}
		want = min(want, int(w.balance(me)/price))
		for want > 0 && !c.Inventory.CanAdd(k, want) {
			want--
		}
		if want > 0 {
			w.post(Bid, k, want, price, me, p, 0)
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
// and at the silo, at colonyMealPrice — less any a haul order is about to
// take to the silo. This is the scumhouse charging for its meals. When the
// price has moved since the asks were posted, they're withdrawn and posted
// again at the new one.
func (w *World) refreshColonyMealAsks() {
	price := w.colonyMealPrice()
	if price <= 0 {
		return
	}
	reprice := price != w.colonyMealAsk
	w.colonyMealAsk = price
	for _, p := range w.mealDepots() {
		if reprice {
			w.withdrawColonyAsks(Meal, p)
		}
		w.offerColonyMeals(p)
	}
}

// colonyMealPrice is what the colony asks for a meal: the charter's price
// while the colony's stores hold meal-reserve meals per colonist, rising in
// step as they fall short of that, to meal-price-max percent of the charter's
// price with the shelves bare.
//
// Scarcity is every meal in storage, not only the colony's: at landing the
// colony has none, and colonists' lockers hold ten each. Priced on the
// colony's stock alone, the price started at its maximum, every colonist
// turned to cooking for itself, and the colony's food never got going.
//
// A fixed price sent no signal: a colony whose stock was running out still
// sold its last meals at $5, first come first served, and a shortage never
// made cooking pay better. Rising, it rations the last meals toward the
// hungriest (a hungry colonist bids more the hungrier it is; see
// mealBidLimit), and it makes cooking to sell worth a colonist's while
// (tryAssignScrapeToSell).
func (w *World) colonyMealPrice() Money {
	ref := w.refPrice(Meal)
	top := int64(w.cfg.MealPriceMax)
	target := int64(w.cfg.MealReserve) * int64(w.countKind(Colonist))
	if ref <= 0 || top <= 100 || target <= 0 {
		return ref
	}
	short := target - int64(w.storedMeals())
	if short <= 0 {
		return ref
	}
	return Money((int64(ref)*(100*target+(top-100)*short) + 50*target) / (100 * target))
}

// offerColonyMeals offers every meal the colony holds at p, less any a haul
// order is about to take, at colonyMealPrice. Resting bids there — hungry
// colonists queued for a meal — fill at once.
func (w *World) offerColonyMeals(p Point) {
	c := w.storageContainers[p]
	price := w.colonyMealPrice()
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

// mealFetchRadius is how close a colonist coming for a meal has to be before
// a cook gives way to it.
const mealFetchRadius = 3

// mealFetchesAt is how many colonists are about to take a meal out of the
// depot at p: on their way to it, and within mealFetchRadius of it. A cook
// stands on a workshop's access tile for the whole of a recipe; one that went
// straight on to the next recipe, and the next, could hold a narrow room's
// only access tile against a starving colonist coming for a meal it owned.
// So a cook doesn't start a recipe while anyone is at the door. One arriving
// mid-recipe waits one recipe at most.
//
// It counts only colonists nearby. Counting everyone on their way from
// anywhere kept stoves idle: in a 100-colonist colony whose cramped kitchens
// had no pantries, and so kept their meals in the stove's own depot, somebody
// was nearly always walking toward one, and that was why a stove stood idle
// beside scum waiting to be cooked 90% of the time it did. It is memoized
// for the tick: every work-seeking colonist asks.
func (w *World) mealFetchesAt(p Point) int {
	if w.mealFetchTick != w.tick || w.mealFetches == nil {
		if w.mealFetches == nil {
			w.mealFetches = make(map[Point]int)
		}
		clear(w.mealFetches)
		for _, e := range w.entities {
			if e.Kind == Colonist && e.Job == JobEat && e.eat == eatFetch && e.Pos.Chebyshev(e.Target) <= mealFetchRadius {
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

// mealSellPrice is what a colonist asks for a meal it sells: a dollar under
// the colony's price when scarcity has raised it, so a colonist's meal sells
// first, and the charter's price otherwise.
func (w *World) mealSellPrice() Money {
	p, ref := w.colonyMealPrice(), w.refPrice(Meal)
	if p > ref {
		return p - 1
	}
	return p
}

// scumMealRecipe is the recipe that turns cave scum into meals.
func scumMealRecipe() (Recipe, bool) {
	for _, r := range recipes {
		if len(r.Inputs) == 1 && r.Inputs[0].Kind == CaveScum {
			return r, true
		}
	}
	return Recipe{}, false
}

// foodPays reports whether a meal sells for enough more than making one costs
// e: scum at its value, and e's labor scraping it, cooking it and walking to
// the nearest scumhouse and back, at labor-price:
//
//	margin = mealSellPrice × meals − scum × its value − labor
//
// It's the producer planner's test, and it has to clear plan-min-profit. At
// the charter's price it rarely does; when scarcity raises the colony's
// price (colonyMealPrice), it does.
func (w *World) foodPays(e *Entity) bool {
	r, ok := scumMealRecipe()
	if !ok || w.countTerrain(r.Facility) == 0 {
		return false
	}
	house, ok := w.nearestScumhouse(e, nil)
	if !ok {
		return false
	}
	meals := 0
	for _, o := range r.Outputs {
		if o.Kind == Meal {
			meals += o.Count
		}
	}
	scum := r.Inputs[0].Count
	ticks := w.ownWorkTicks(e, r.Skill, r.Ticks) + scum*w.ownWorkTicks(e, SkillForaging, w.cfg.ScrapeTicks) + 2*e.Pos.Chebyshev(house)
	margin := w.mealSellPrice()*Money(meals) - Money(scum)*w.valueOf(CaveScum) - w.laborCostFor(e, ticks)
	return margin >= Money(w.cfg.PlanMinProfit)
}

// tryAssignScrapeToSell sends e to scrape scum of its own, to cook into meals
// and sell, when food pays (foodPays). The rest is existing work: scum kept
// at a scumhouse is cooked by tryAssignCraftFor, and jobCraft offers the meals
// beyond meal-keep for sale where they're made, at mealSellPrice. So a
// colonist makes food on its own account whenever food pays, whatever the
// treasury holds. The colony's own food chain runs on the treasury: in a
// 100-colonist colony, building its rooms spent it to $0 and the colony
// stopped buying scum.
func (w *World) tryAssignScrapeToSell(e *Entity) bool {
	return w.foodPays(e) && w.tryAssignScrape(e, true)
}

// offerOwnMeals offers the meals e has just cooked at p for sale where they
// are, at mealSellPrice: those beyond what it keeps (surplusMeals, meal-keep
// and its pocket meal). Hungry colonists' bids rest at a scumhouse's pantry,
// so a meal on offer there sells to the next of them at once, without a walk
// to the silo.
func (w *World) offerOwnMeals(e *Entity, p Point) {
	c := w.storageContainers[p]
	silo, _ := w.marketDepot()
	if c == nil || p == silo {
		return
	}
	n := min(c.held(ColonistOwner(e.ID), Meal), w.surplusMeals(e, silo))
	if price := w.mealSellPrice(); n > 0 && price > 0 {
		w.post(Ask, Meal, n, price, ColonistOwner(e.ID), p, w.cfg.OrderTTL)
	}
}

// storedMeals is every meal in every depot, whoever owns it, memoized for
// the tick: colonyMealPrice reads it for every colonist deciding on work.
func (w *World) storedMeals() int {
	if w.storedMealsTick == w.tick {
		return w.storedMealsCache
	}
	n := 0
	for _, c := range w.storageContainers {
		n += c.Inventory.Count(Meal)
	}
	w.storedMealsTick, w.storedMealsCache = w.tick, n
	return n
}

// mayCookAt reports whether e may cook at the workshop at p: a colonist's own
// kitchen is only its owner's to cook at, while anyone may bring scum to it
// or fetch meals from its pantry. Once its owner is dead, it's anyone's.
func (w *World) mayCookAt(e *Entity, p Point) bool {
	f := w.fixtures[p]
	if f == nil || f.Owner.Kind != OwnerColonist || f.Owner.ID == e.ID {
		return true
	}
	owner := w.entities[f.Owner.ID]
	return owner == nil || !owner.Alive()
}

// mayStockAt reports whether e may leave its item at the scumhouse at p:
// where it may cook it, or where someone else bids for it. In someone else's
// own kitchen with no bid for it, the item could be neither cooked by the
// one who left it nor sold, and there it would sit: scrapers left hundreds of
// units of their scum in chefs' kitchens, and big colonies starved.
func (w *World) mayStockAt(e *Entity, p Point, item ItemKind) bool {
	if w.mayCookAt(e, p) {
		return true
	}
	bid, ok := w.bestBid(item, p)
	return ok && bid.Actor != ColonistOwner(e.ID)
}

// privateKitchen reports whether the scumhouse at p belongs to a living
// colonist: the colony neither cooks nor buys biomatter there.
func (w *World) privateKitchen(p Point) bool {
	f := w.fixtures[p]
	if f == nil || f.Owner.Kind != OwnerColonist {
		return false
	}
	owner := w.entities[f.Owner.ID]
	return owner != nil && owner.Alive()
}

// colonyKitchens is every scumhouse that isn't a living colonist's own, in
// position order.
func (w *World) colonyKitchens() []Point {
	var out []Point
	for _, p := range w.scumhousesSorted() {
		if !w.privateKitchen(p) {
			out = append(out, p)
		}
	}
	return out
}

// kitchensCrowded reports whether at least three in four of the colony's
// kitchens have a cook at them: stoves, not scum, are what's short.
func (w *World) kitchensCrowded() bool {
	houses := w.colonyKitchens()
	if len(houses) == 0 {
		return false
	}
	busy := 0
	for _, p := range houses {
		if w.workshopClaims[p] != 0 {
			busy++
		}
	}
	return 4*busy >= 3*len(houses)
}

// ownKitchen is e's own kitchen, if it has one it can reach.
func (w *World) ownKitchen(e *Entity) (Point, bool) {
	if !e.hasKitchen || w.TerrainAt(e.kitchen) != Scumhouse || w.storageContainers[e.kitchen] == nil {
		return Point{}, false
	}
	return e.kitchen, w.taskReachable(e.kitchen, w.roomOf(e.Pos))
}

// ownKitchenStocked reports whether e's own kitchen holds enough of e's
// biomatter for a recipe.
func (w *World) ownKitchenStocked(e *Entity) bool {
	p, ok := w.ownKitchen(e)
	if !ok {
		return false
	}
	_, _, ok = w.craftableRecipe(w.storageContainers[p], []Owner{ColonistOwner(e.ID)})
	return ok
}

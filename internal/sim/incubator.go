package sim

import "sort"

// ---- The scum incubator ------------------------------------------------------------
//
// An incubator grows cave scum on a schedule from a seed of it. Colonists load
// the seed (scraped off the rock, sold into the colony's standing bid there);
// from then on the incubator adds a unit every incubator-grow-ticks until it
// holds incubator-capacity, wherever the rock's scum happens to be. Colonists
// harvest what it has grown above the seed and carry it to a scumhouse, so the
// colony's food work draws on a steady source it controls instead of scraping
// the rock. Wild scraping is for seeding and for dire times (wildScumAllowed).
// See docs/incubator.md.

// incubatorRoom is one or two incubators a tile apart, with an aisle so a
// harvester and a loader are never fighting for the one access tile. Nothing
// in it is life support on its own, so a cramped cavern gets the narrow room.
var incubatorRoom = roomRecipe{
	name: "incubator", kinds: []Terrain{Incubator}, minFac: 1, maxFac: 2, aisle: true,
	planLog: "The colony marks out a scum incubator.", structure: StructIncubatorRoom,
}

// incubatorsOn reports whether the colony uses incubators at all.
func (w *World) incubatorsOn() bool {
	return w.cfg.IncubatorGrowTicks > 0 && w.cfg.IncubatorCapacity > 0
}

// incubatorSeed is the units of scum an incubator needs to grow and keeps back
// when harvested.
func (w *World) incubatorSeed() int { return max(1, w.cfg.IncubatorSeed) }

// desiredIncubators is how many incubators the colony wants: one per
// colonists-per-incubator colonists, and always at least one.
func (w *World) desiredIncubators() int {
	per := max(1, w.cfg.ColonistsPerIncubator)
	return max(1, (w.countKind(Colonist)+per-1)/per)
}

// wantsIncubator reports whether the colony wants an incubator it has not
// planned. It waits for the first scumhouse to be planned, since an incubator
// feeds one.
func (w *World) wantsIncubator() bool {
	return w.incubatorsOn() && w.plannedFacilities(Scumhouse) > 0 &&
		w.plannedFacilities(Incubator) < w.desiredIncubators()
}

// incubatorsSorted lists every incubator by position.
func (w *World) incubatorsSorted() []Point {
	out := make([]Point, 0, len(w.facilityTiles[Incubator]))
	for p := range w.facilityTiles[Incubator] {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return lessPoint(out[i], out[j]) })
	return out
}

// ripeScum is the colony's scum in the incubator c that is ready to harvest:
// what it holds above the seed.
func (w *World) ripeScum(c *StorageContainer) int {
	return max(0, c.held(Community, CaveScum)-w.incubatorSeed())
}

// ripeTotal is the ripe scum in every incubator.
func (w *World) ripeTotal() int {
	n := 0
	for p := range w.facilityTiles[Incubator] {
		if c := w.storageContainers[p]; c != nil {
			n += w.ripeScum(c)
		}
	}
	return n
}

// growIncubators lets every seeded incubator grow a unit of scum each
// incubator-grow-ticks, up to its capacity. There is no chance in it: the rate
// is the point. Each incubator is independent of the others, so the order they
// are visited in decides nothing.
func (w *World) growIncubators() {
	if !w.incubatorsOn() || w.tick%w.cfg.IncubatorGrowTicks != 0 {
		return
	}
	for p := range w.facilityTiles[Incubator] {
		c := w.storageContainers[p]
		if c == nil {
			continue
		}
		n := c.Inventory.Count(CaveScum)
		if n < w.incubatorSeed() || n >= w.cfg.IncubatorCapacity {
			continue
		}
		if c.Inventory.AddAll(ItemStack{CaveScum, 1}) {
			c.credit(Community, CaveScum, 1)
		}
	}
}

// ---- When wild scum is allowed -------------------------------------------------------

// scumInKitchens is the colony's scum waiting to be cooked, in every kitchen.
func (w *World) scumInKitchens() int {
	n := 0
	for _, p := range w.colonyKitchens() {
		if c := w.storageContainers[p]; c != nil {
			n += c.held(Community, CaveScum)
		}
	}
	return n
}

// wildScumAllowed reports whether the colony's routine food work may scrape
// scum off the rock. Once it has an incubator it may not, except in dire
// circumstances: the colony's stores are below scum-dire-meals a colonist, and
// there is nothing ripe to harvest and no scum waiting at a stove — the
// incubators have failed it. Until the first incubator is built the colony has
// nothing else to eat from, so it scrapes as it always did. A hungry
// colonist's own foraging is never held back by this (planForage): it is for
// itself, and it is already dire.
func (w *World) wildScumAllowed() bool {
	if !w.incubatorsOn() || w.countTerrain(Incubator) == 0 {
		return true
	}
	short := w.storedMeals() < w.cfg.ScumDireMeals*w.countKind(Colonist)
	return short && w.ripeTotal() == 0 && w.scumInKitchens() < w.scumPerMeal()
}

// ---- Seeding -------------------------------------------------------------------------

// seedDeficit is how much scum the incubator c is short of a seed.
func (w *World) seedDeficit(c *StorageContainer) int {
	return w.incubatorSeed() - c.Inventory.Count(CaveScum)
}

// seedIncubator finds the nearest reachable incubator short of its seed that
// nobody else is already loading.
func (w *World) seedIncubator(e *Entity) (Point, bool) {
	return w.nearestWorkshop(e, Incubator, func(c *StorageContainer) bool {
		if id := w.workshopClaims[c.Pos]; id != 0 && id != e.ID {
			return false // one seeder at a time, or the colony pays for far more than it needs
		}
		return w.seedDeficit(c) > 0 && c.Inventory.CanAdd(CaveScum, 1)
	})
}

// tryAssignSeed sends e to load scum into an incubator short of its seed: haul
// what it carries, or scrape a load from the rock for it. This is the one
// routine use of wild scum once incubators stand. The colony buys what is
// loaded at the price it pays for scum (deliverBiomatter), so the incubator's
// stock is the colony's from the first unit.
func (w *World) tryAssignSeed(e *Entity) bool {
	if !w.incubatorsOn() {
		return false
	}
	inc, ok := w.seedIncubator(e)
	if !ok {
		return false
	}
	c := w.storageContainers[inc]
	if e.Inventory.Has(CaveScum) {
		if !c.Inventory.CanAdd(CaveScum, e.Inventory.Count(CaveScum)) {
			return false
		}
		w.workshopClaims[inc] = e.ID
		e.Job, e.Target, e.scrape, e.Progress = JobScrape, inc, scrapeHaul, 0
		e.scrapeKeep, e.scrapeSeed, e.seedAt = false, true, inc
		return true
	}
	load := min(w.scrapeLoad(), w.seedDeficit(c))
	if load <= 0 || !e.Inventory.CanAdd(CaveScum, load) {
		return false
	}
	patch, ok := w.nearestScum(e)
	if !ok {
		return false
	}
	w.scumClaims[patch] = e.ID
	w.workshopClaims[inc] = e.ID
	e.Job, e.Target, e.scrape, e.Progress = JobScrape, patch, scrapeGather, 0
	e.scrapeKeep, e.scrapeSeed, e.seedAt, e.scrapeQty = false, true, inc, load
	return true
}

// ---- Harvesting ----------------------------------------------------------------------

// harvestLoad is the most scum a colonist carries from an incubator in a trip.
func (w *World) harvestLoad() int { return 2 * w.scrapeLoad() }

// harvestDestination is the nearest colony kitchen that can take n units of
// scum without going over its stock cap, which is what keeps harvesters from
// burying a stove in scum its cooks cannot get through.
func (w *World) harvestDestination(e *Entity, n int) (Point, bool) {
	return w.nearestScumhouse(e, func(c *StorageContainer) bool {
		if w.privateKitchen(c.Pos) || !c.Inventory.CanAdd(CaveScum, n) {
			return false
		}
		cap := w.cfg.ScumhouseStockCap
		return cap <= 0 || c.held(Community, CaveScum)+n <= cap
	})
}

// tryAssignHarvest sends e to collect scum an incubator has grown, for a
// scumhouse that has room for it. One harvester at an incubator at a time.
func (w *World) tryAssignHarvest(e *Entity) bool {
	if !w.incubatorsOn() || e.Inventory.Has(CaveScum) || !e.Inventory.CanAdd(CaveScum, w.scumPerMeal()) {
		return false
	}
	if _, ok := w.harvestDestination(e, w.scumPerMeal()); !ok {
		return false
	}
	inc, ok := w.nearestWorkshop(e, Incubator, func(c *StorageContainer) bool {
		if id := w.scumClaims[c.Pos]; id != 0 && id != e.ID {
			return false
		}
		return w.ripeScum(c) >= w.scumPerMeal()
	})
	if !ok {
		return false
	}
	w.scumClaims[inc] = e.ID
	e.Job, e.Target, e.scrape, e.Progress = JobScrape, inc, scrapeHarvest, 0
	e.scrapeKeep, e.scrapeSeed = false, false
	return true
}

// jobHarvest walks to the incubator at Target, takes what it has grown above
// its seed, and starts the haul to a scumhouse. The scum is the colony's, and
// the colony pays wage-harvest for the trip.
func (w *World) jobHarvest(e *Entity) {
	c := w.storageContainers[e.Target]
	if c == nil || c.Terrain != Incubator || w.ripeScum(c) < w.scumPerMeal() {
		w.clearJob(e) // somebody got there first, or it was demolished
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
	n := min(w.ripeScum(c), w.harvestLoad())
	for n > 0 && !e.Inventory.CanAdd(CaveScum, n) {
		n--
	}
	if n <= 0 {
		w.clearJob(e)
		return
	}
	house, ok := w.harvestDestination(e, n)
	if !ok || !c.debit(Community, CaveScum, n) {
		w.clearJob(e)
		return
	}
	e.Inventory.Add(CaveScum, n)
	e.addCargo(Community, CaveScum, n)
	if w.cfg.WageHarvest > 0 {
		w.transfer(Community, ColonistOwner(e.ID), Money(w.cfg.WageHarvest)) // as far as the treasury goes
	}
	w.emitDone(e, ActionScrape, NounScum, "Harvested %d units of cave scum from an incubator.", n)
	delete(w.scumClaims, e.Target)
	e.Target, e.scrape, e.Progress = house, scrapeHaul, 0
}

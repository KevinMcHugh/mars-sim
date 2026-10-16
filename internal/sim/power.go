package sim

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
)

// ---- Power ----------------------------------------------------------------------
//
// Power moves around the colony as batteries: a good, not a grid. A powered
// recipe spends one as an input; without one, a stove cooks its cold row.
// The ships land the colony's first batteries in each passenger's locker, and
// the colony keeps its stoves stocked by paying each passenger to carry its
// share out. See docs/power.md.

// recipeEnabled reports whether r can be worked at all: a cold row only with
// power on. Off, the powered row stands in for both, needing no battery.
func (w *World) recipeEnabled(r Recipe) bool {
	return !r.Cold || w.cfg.Power
}

// needs reports whether a recipe input has to be in the depot and is spent:
// every input, except a battery with power off.
func (w *World) needs(in ItemStack) bool {
	return in.Kind != Battery || w.cfg.Power
}

// refreshKitchenBatteries keeps kitchen-battery-stock of the colony's
// batteries at each of its stoves (keepStocked). At landing they come from
// the passengers' lockers: a locker is private, so only its passenger may take
// the colony's batteries out of it, and each one is paid haul-pay to carry its
// share to the stove. Later they come from the reactor and the silo.
func (w *World) refreshKitchenBatteries() {
	if !w.cfg.Power {
		return
	}
	w.keepStocked(Battery, w.colonyKitchens(), w.cfg.KitchenBatteryStock)
}

// keepStocked keeps stock units of the colony's item at each depot in dests,
// posting haul orders (WorkHaul, haul-pay a unit) to bring them in from
// wherever else the colony holds them, nearest first, for what is missing and
// not already on its way.
func (w *World) keepStocked(item ItemKind, dests []Point, stock int) {
	if stock <= 0 || len(dests) == 0 {
		return
	}
	coming := map[Point]int{} // open haul units by destination
	taking := map[Point]int{} // open haul units by source
	for _, o := range w.workOrders {
		if o.Kind == WorkHaul && o.Issuer == Community && o.Item == item {
			coming[o.Pos] += o.Units
			taking[o.From] += o.Units
		}
	}
	wants := make([]int, len(dests))
	short := false
	for i, d := range dests {
		wants[i] = stock - w.storageContainers[d].held(Community, item) - coming[d]
		short = short || wants[i] > 0
	}
	if !short {
		return // the usual case: one look at each depot, no scan
	}
	isDest := map[Point]bool{}
	for _, d := range dests {
		isDest[d] = true
	}
	var sources []Point
	w.eachContainer(func(c *StorageContainer) {
		if !isDest[c.Pos] && c.held(Community, item)-taking[c.Pos] > 0 {
			sources = append(sources, c.Pos)
		}
	})
	type source struct {
		at   Point
		dist int
	}
	near := make([]source, len(sources))
	for i, d := range dests {
		if wants[i] <= 0 {
			continue
		}
		// eachContainer is map order: sort by distance, then position, so
		// which depot is drawn on first is the seed's.
		for j, src := range sources {
			near[j] = source{src, w.travelEstimate(src, d)}
		}
		slices.SortFunc(near, func(a, b source) int {
			if c := cmp.Compare(a.dist, b.dist); c != 0 {
				return c
			}
			if lessPoint(a.at, b.at) {
				return -1
			}
			if lessPoint(b.at, a.at) {
				return 1
			}
			return 0
		})
		want := wants[i]
		for _, src := range near {
			if want <= 0 {
				break
			}
			n := min(want, w.storageContainers[src.at].held(Community, item)-taking[src.at])
			if n <= 0 {
				continue
			}
			if o := w.postWork(WorkHaul, Community, Money(w.cfg.HaulPay), n, d); o != nil {
				o.From, o.Item = src.at, item
				taking[src.at] += n
				want -= n
			}
		}
	}
}

// ---- The reactor (P2) -----------------------------------------------------------
//
// The colony is the power company: it builds the reactor once it holds
// uranium, buys the uranium and water ice at its silo, has them hauled over,
// and pays an operator to charge batteries while it is short of them. The
// batteries go to the stoves by refreshKitchenBatteries, from the reactor's
// depot like any other. See docs/power.md.

// reactorRoom is one reactor in a walled room, with an aisle: the operator
// works at it while haulers bring ice and take batteries away.
var reactorRoom = roomRecipe{
	name: "reactor", kinds: []Terrain{Reactor}, minFac: 1, maxFac: 1, aisle: true,
	planLog: "The colony marks out a reactor room.",
}

// reactorFuel reports whether k is the reactor's fuel, which the colony bids
// for itself (refreshReactorFuel) and never resells, with power on.
func (w *World) reactorFuel(k ItemKind) bool {
	return w.cfg.Power && (k == UraniumOre || k == WaterIce)
}

// wantsReactor reports whether the colony wants a reactor it has not
// planned: power is on, it has none, and it holds uranium to burn. A reactor
// with no fuel would be a monument.
func (w *World) wantsReactor() bool {
	if !w.cfg.Power || w.plannedFacilities(Reactor) > 0 {
		return false
	}
	return w.colonyHolds(UraniumOre) > 0
}

// colonyReactors is every reactor, sorted. Under the monopoly every one is
// the colony's.
func (w *World) colonyReactors() []Point {
	var out []Point
	w.eachFacility(Reactor, func(p Point) {
		if w.storageContainers[p] != nil {
			out = append(out, p)
		}
	})
	sort.Slice(out, func(i, j int) bool { return lessPoint(out[i], out[j]) })
	return out
}

// colonyHolds is how many units of k the colony owns, in every depot.
func (w *World) colonyHolds(k ItemKind) int {
	n := 0
	w.eachContainer(func(c *StorageContainer) { n += c.held(Community, k) })
	return n
}

// colonyBatteries is how many batteries the colony owns, in every depot and
// in haulers' hands, memoized for the tick.
func (w *World) colonyBatteries() int {
	if w.batteriesTick == int(w.tick) {
		return w.batteriesCache
	}
	n := w.colonyHolds(Battery)
	for id := range w.kindEntities[Colonist] { // a sum: map order can't change it
		n += w.entities[id].carriedFor(Community, Battery)
	}
	w.batteriesTick, w.batteriesCache = int(w.tick), n
	return n
}

// batteriesWanted reports whether the colony's reactors should charge: power
// is on, a reactor stands, and the colony holds fewer than battery-reserve.
func (w *World) batteriesWanted() bool {
	return w.cfg.Power && w.countTerrain(Reactor) > 0 && w.colonyBatteries() < w.cfg.BatteryReserve
}

// tryAssignReactor sends e to work the nearest free reactor holding the
// colony's inputs for one of its recipes, paid wage-reactor a recipe.
func (w *World) tryAssignReactor(e *Entity) bool {
	owners := []Owner{Community}
	p, ok := w.nearestWorkshop(e, Reactor, func(c *StorageContainer) bool {
		if id := w.workshopClaims[c.Pos]; id != 0 && id != e.ID {
			return false // one operator per reactor
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

// refreshReactorFuel keeps the reactors fuelled. The colony bids at its silo
// for uranium ore until it holds uranium-reserve (counting a reactor's
// pellets as the ore they came from), and, once a reactor stands, for water
// ice until it holds reactor-ice-stock a reactor; then it has them hauled to
// each reactor: the ice, and one ore whenever a reactor is out of pellets.
func (w *World) refreshReactorFuel() {
	if !w.cfg.Power {
		return
	}
	reactors := w.colonyReactors()
	if silo, ok := w.marketDepot(); ok {
		pellets := 0
		for _, p := range reactors {
			pellets += w.storageContainers[p].held(Community, FuelPellet)
		}
		ore := w.colonyHolds(UraniumOre)
		if pellets > 0 {
			ore++ // a reactor with pellets is a unit of ore not yet spent
		}
		w.fuelBid(UraniumOre, w.cfg.UraniumReserve-ore, silo)
		if len(reactors) > 0 {
			w.fuelBid(WaterIce, w.cfg.ReactorIceStock*len(reactors)-w.colonyHolds(WaterIce), silo)
		}
	}
	w.keepStocked(WaterIce, reactors, w.cfg.ReactorIceStock)
	var dry []Point
	for _, p := range reactors {
		if w.storageContainers[p].held(Community, FuelPellet) == 0 {
			dry = append(dry, p)
		}
	}
	w.keepStocked(UraniumOre, dry, 1)
}

// fuelBid tops the colony's standing bid for k at the silo up to want units,
// at its reference price, as far as the treasury stretches.
func (w *World) fuelBid(k ItemKind, want int, silo Point) {
	price := w.refPrice(k)
	if price <= 0 {
		return
	}
	want -= w.openQty(Bid, k, silo, Community)
	want = min(want, int(w.treasury/price))
	for want > 0 && !w.storageContainers[silo].Inventory.CanAdd(k, want) {
		want--
	}
	if want > 0 {
		w.postStanding(Bid, k, want, price, silo)
	}
}

// reactorFuelled reports whether the reactor at p holds uranium ore or fuel
// pellets: a dose to whoever stands beside it (see mutation.go).
func (w *World) reactorFuelled(p Point) bool {
	c := w.storageContainers[p]
	return c != nil && (c.Inventory.Count(UraniumOre) > 0 || c.Inventory.Count(FuelPellet) > 0)
}

// deliverBatteries puts n batteries of the colony's in its silo, reporting
// how many found room. A supply drop lands them there; refreshKitchenBatteries
// takes them on to the stoves.
func (w *World) deliverBatteries(n int) int {
	silo, ok := w.marketDepot()
	if !ok || n <= 0 {
		return 0
	}
	c := w.storageContainers[silo]
	for n > 0 && !c.Inventory.CanAdd(Battery, n) {
		n--
	}
	if n > 0 {
		c.Inventory.Add(Battery, n)
		c.credit(Community, Battery, n)
	}
	return n
}

// batteryPhrase is "1 battery" or "n batteries".
func batteryPhrase(n int) string {
	if n == 1 {
		return "1 battery"
	}
	return fmt.Sprintf("%d batteries", n)
}

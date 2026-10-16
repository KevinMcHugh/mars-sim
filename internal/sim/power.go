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
	w.keepStockedFor(Community, item, dests, stock)
}

// keepStockedFor is keepStocked for any owner's goods, paid from its own
// wallet: a reactor's owner keeps its reactor fuelled from what it bought at
// the silo the same way.
func (w *World) keepStockedFor(owner Owner, item ItemKind, dests []Point, stock int) {
	if stock <= 0 || len(dests) == 0 {
		return
	}
	coming := map[Point]int{} // open haul units by destination
	taking := map[Point]int{} // open haul units by source
	for _, o := range w.workOrders {
		if o.Kind == WorkHaul && o.Issuer == owner && o.Item == item {
			coming[o.Pos] += o.Units
			taking[o.From] += o.Units
		}
	}
	wants := make([]int, len(dests))
	short := false
	for i, d := range dests {
		wants[i] = stock - w.storageContainers[d].held(owner, item) - coming[d]
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
		if !isDest[c.Pos] && c.held(owner, item)-taking[c.Pos] > 0 {
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
			n := min(want, w.storageContainers[src.at].held(owner, item)-taking[src.at])
			if n <= 0 {
				continue
			}
			if o := w.postWork(WorkHaul, owner, Money(w.cfg.HaulPay), n, d); o != nil {
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
	if !w.colonyRunsPower() || len(w.colonyReactors()) > 0 || w.colonyPlansReactor() {
		return false
	}
	return w.colonyHolds(UraniumOre) > 0
}

// colonyPlansReactor reports whether a public work of the colony's has a
// reactor still to raise. A colonist's commission doesn't count: under the
// public option the colony's reactor is its own, beside any baron's.
func (w *World) colonyPlansReactor() bool {
	for _, p := range w.projects {
		if p.issuer != Community {
			continue
		}
		for _, t := range p.tasks {
			if t.terrain == Reactor && !w.taskDone(t) {
				return true
			}
		}
	}
	return false
}

// colonyReactors is every reactor the colony owns, sorted.
func (w *World) colonyReactors() []Point {
	var out []Point
	w.eachFacility(Reactor, func(p Point) {
		if w.storageContainers[p] != nil && w.fixtureOwner(p) == Community {
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
	return w.cfg.Power && w.countTerrain(Reactor) > 0 && w.colonyBatteries() < w.cfg.BatteryReserve && len(w.colonyReactors()) > 0
}

// tryAssignReactor sends e to work the nearest free reactor holding the
// colony's inputs for one of its recipes, paid wage-reactor a recipe.
func (w *World) tryAssignReactor(e *Entity) bool {
	owners := []Owner{Community}
	p, ok := w.nearestWorkshop(e, Reactor, func(c *StorageContainer) bool {
		if id := w.workshopClaims[c.Pos]; id != 0 && id != e.ID {
			return false // one operator per reactor
		}
		if w.fixtureOwner(c.Pos) != Community {
			return false // a baron's, which only its owner works
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
	if !w.colonyRunsPower() {
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

// ---- Policy (P3) ------------------------------------------------------------------

// PowerPolicy is who runs the colony's reactors: the colony itself, the
// colonists who can afford one, or both, with the colony's only as a last
// resort. See docs/power.md.
type PowerPolicy uint8

const (
	PolicyMonopoly     PowerPolicy = iota // the colony builds, fuels and runs every reactor
	PolicyMarket                          // colonists commission reactors; the colony buys batteries
	PolicyPublicOption                    // a market, and a colony reactor once it goes without power
	numPowerPolicies
)

var powerPolicyNames = [numPowerPolicies]string{"monopoly", "market", "public-option"}

func (p PowerPolicy) String() string {
	if p < numPowerPolicies {
		return powerPolicyNames[p]
	}
	return "unknown"
}

// Set parses a policy by name, for the command line and mars-sim.yaml.
func (p *PowerPolicy) Set(s string) error {
	for i, name := range powerPolicyNames {
		if name == s {
			*p = PowerPolicy(i)
			return nil
		}
	}
	return fmt.Errorf("want monopoly, market or public-option, got %q", s)
}

// choiceSetting is a setting that takes one of a set of names: what a
// PowerPolicy field is to the config file and the flags (assign, main.go).
type choiceSetting interface {
	String() string
	Set(string) error
}

// fixtureOwner is who owns the fixture at p (Nobody if it has no record).
func (w *World) fixtureOwner(p Point) Owner {
	if f := w.fixtures[p]; f != nil {
		return f.Owner
	}
	return Nobody
}

// colonyRunsPower reports whether the colony builds, fuels and works
// reactors of its own: always under the monopoly, and under the public
// option once it has gone without power (notePowerShortage).
func (w *World) colonyRunsPower() bool {
	if !w.cfg.Power {
		return false
	}
	switch w.cfg.PowerPolicy {
	case PolicyMonopoly:
		return true
	case PolicyPublicOption:
		return w.publicReactor
	}
	return false
}

// colonyBuysBatteries reports whether the colony bids for batteries for its
// stoves: under the market and the public option, where it doesn't (or not
// only) make them.
func (w *World) colonyBuysBatteries() bool {
	if !w.cfg.Power {
		return false
	}
	switch w.cfg.PowerPolicy {
	case PolicyMarket, PolicyPublicOption:
		return true
	}
	return false
}

// refreshColonyBatteryBids keeps a standing bid at each colony stove for
// what it is short of kitchen-battery-stock, less what is already on its way
// there, at price-battery, as far as the treasury stretches. The batteries a
// baron sells into it come straight to the stove (planSupply).
func (w *World) refreshColonyBatteryBids() {
	price := Money(w.cfg.PriceBattery)
	if !w.colonyBuysBatteries() || price <= 0 {
		return
	}
	coming := map[Point]int{}
	for _, o := range w.workOrders {
		if o.Kind == WorkHaul && o.Issuer == Community && o.Item == Battery {
			coming[o.Pos] += o.Units
		}
	}
	for _, k := range w.colonyKitchens() {
		c := w.storageContainers[k]
		want := w.cfg.KitchenBatteryStock - c.held(Community, Battery) - coming[k] - w.openQty(Bid, Battery, k, Community)
		want = min(want, int(w.treasury/price))
		for want > 0 && !c.Inventory.CanAdd(Battery, want) {
			want--
		}
		if want > 0 {
			w.postStanding(Bid, Battery, want, price, k)
		}
	}
}

// commissionReactors has the first colonist (by ID) with reactor-savings and
// no reactor commissioned commission one, paid from its wallet, under the
// market and the public option. Only while the colony's stoves want
// batteries nobody has sold them (an open battery bid), and only while every
// reactor already standing has fuel and none is still going up: one baron at
// a time tries, and a second follows when there is room for one. One per
// planning cycle.
func (w *World) commissionReactors() {
	if !w.colonyBuysBatteries() || w.cfg.ReactorSavings <= 0 || w.projectFacilityTasks(Reactor) > 0 {
		return
	}
	demand := false
	for _, k := range w.colonyKitchens() {
		if w.openQty(Bid, Battery, k, Community) > 0 {
			demand = true
			break
		}
	}
	if !demand {
		return
	}
	idle := false
	w.eachFacility(Reactor, func(p Point) { idle = idle || !w.reactorFuelled(p) })
	if idle {
		return
	}
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e.Kind != Colonist || !e.Alive() || e.reactorCommissioned || e.wallet < Money(w.cfg.ReactorSavings) {
			continue
		}
		if w.planRoomFor(reactorRoom, ColonistOwner(e.ID)) {
			e.reactorCommissioned = true
			w.logEventProject(LogBuildStart, w.lastProject(), fmt.Sprintf("%s commissions a reactor of %s own.", e.displayName(), e.possessive()))
		}
		return
	}
}

// ownReactors is every reactor a living colonist owns, sorted, with its
// owner.
func (w *World) ownReactors() []Point {
	var out []Point
	w.eachFacility(Reactor, func(p Point) {
		if o := w.fixtureOwner(p); o.Kind == OwnerColonist && w.storageContainers[p] != nil {
			if e := w.entities[o.ID]; e != nil && e.Alive() {
				out = append(out, p)
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return lessPoint(out[i], out[j]) })
	return out
}

// refreshOwnerReactorBids has every reactor's owner buy what keeps it
// running, from its own wallet, the way the colony buys its own reactor's
// fuel: bids at the silo for water ice up to reactor-ice-stock, and for one
// uranium ore whenever it has neither ore nor pellets; then haul orders, paid
// haul-pay a unit, to carry what it bought to its reactor (keepStockedFor).
//
// The first version bid at the reactor itself. Miners offer their ore at the
// silo, and a bid across the colony at the same price paid no hauler to
// bring it: barons sat beside open bids with dozens of ore on sale at the
// silo. A premium on the bid fixed the uranium and not the ice. Buying where
// the miners sell, as the monopoly does, fixes both.
func (w *World) refreshOwnerReactorBids() {
	if !w.cfg.Power {
		return
	}
	silo, ok := w.marketDepot()
	if !ok {
		return
	}
	sc := w.storageContainers[silo]
	for _, p := range w.ownReactors() {
		me := w.fixtureOwner(p)
		c := w.storageContainers[p]
		bid := func(k ItemKind, want int) {
			price := ownerFuelPrice(w.refPrice(k))
			if price <= 0 {
				return
			}
			want -= c.held(me, k) + sc.held(me, k) + w.openQty(Bid, k, silo, me)
			want = min(want, int(w.balance(me)/price))
			for want > 0 && !sc.Inventory.CanAdd(k, want) {
				want--
			}
			if want > 0 {
				w.post(Bid, k, want, price, me, silo, 0)
			}
		}
		bid(WaterIce, w.cfg.ReactorIceStock)
		w.keepStockedFor(me, WaterIce, []Point{p}, w.cfg.ReactorIceStock)
		if c.held(me, FuelPellet) == 0 {
			bid(UraniumOre, 1)
			w.keepStockedFor(me, UraniumOre, []Point{p}, 1)
		}
	}
}

// ownerFuelPrice is what a reactor's owner bids at the silo for its fuel:
// the reference price and a tenth, at least $2 over. The colony's prospecting
// bids are at the reference price, so a miner holding ore offers it to a bid
// only above that (planSupply); a baron at the reference price got none of
// the uranium already lying on miners' lines at the silo.
func ownerFuelPrice(ref Money) Money {
	if ref <= 0 {
		return 0
	}
	return ref + max(2, ref/10)
}

// tryAssignOwnReactor has e work a reactor of its own when it holds e's
// inputs for a recipe, while e holds fewer than battery-reserve batteries
// there: pressing pellets from its ore, charging from its pellets and ice. It
// sells the batteries itself, into the colony's bids (planSupply).
func (w *World) tryAssignOwnReactor(e *Entity) bool {
	me := ColonistOwner(e.ID)
	room := w.roomOf(e.Pos)
	for _, p := range w.ownReactors() {
		if w.fixtureOwner(p) != me || !w.taskReachable(p, room) {
			continue
		}
		if id := w.workshopClaims[p]; id != 0 && id != e.ID {
			continue
		}
		c := w.storageContainers[p]
		if c.held(me, Battery) >= w.cfg.BatteryReserve {
			continue
		}
		recipe, owner, ok := w.craftableRecipe(c, []Owner{me})
		if !ok {
			continue
		}
		w.workshopClaims[p] = e.ID
		e.Job, e.Target, e.Progress = JobCraft, p, 0
		e.recipe, e.craftFor, e.craftRun = recipe, owner, 0
		return true
	}
	return false
}

// notePowerShortage watches, under the public option, for the colony going
// without a battery, and latches publicReactor once it has for
// public-option-days: the market left it without power, so it will run a
// reactor of its own (colonyRunsPower).
func (w *World) notePowerShortage() {
	if !w.cfg.Power || w.cfg.PowerPolicy != PolicyPublicOption || w.publicReactor {
		return
	}
	if w.colonyBatteries() > 0 {
		w.powerDrySince = -1
		return
	}
	if w.powerDrySince < 0 {
		w.powerDrySince = int(w.tick)
		return
	}
	if int(w.tick)-w.powerDrySince >= w.cfg.PublicOptionDays*w.cfg.TicksPerDay() {
		w.publicReactor = true
		w.logEvent(LogBuildStart, "The colony has gone without power long enough: it will run a reactor of its own.")
	}
}

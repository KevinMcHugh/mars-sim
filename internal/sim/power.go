package sim

import (
	"cmp"
	"fmt"
	"slices"
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
// batteries at each of its stoves, posting haul orders to bring them in from
// wherever the colony holds them (nearest first) for what is missing and not
// already on its way. At landing that is the passengers' lockers: a locker is
// private, so only its passenger may take the colony's batteries out of it,
// and each one is paid haul-pay to carry its share to the stove.
func (w *World) refreshKitchenBatteries() {
	if !w.cfg.Power || w.cfg.KitchenBatteryStock <= 0 {
		return
	}
	kitchens := w.colonyKitchens()
	if len(kitchens) == 0 {
		return
	}
	coming := map[Point]int{} // open haul units by destination
	taking := map[Point]int{} // open haul units by source
	for _, o := range w.workOrders {
		if o.Kind == WorkHaul && o.Issuer == Community && o.Item == Battery {
			coming[o.Pos] += o.Units
			taking[o.From] += o.Units
		}
	}
	wants := make([]int, len(kitchens))
	short := false
	for i, k := range kitchens {
		wants[i] = w.cfg.KitchenBatteryStock - w.storageContainers[k].held(Community, Battery) - coming[k]
		short = short || wants[i] > 0
	}
	if !short {
		return // the usual case: one look at each stove, no scan
	}
	isKitchen := map[Point]bool{}
	for _, k := range kitchens {
		isKitchen[k] = true
	}
	var sources []Point
	w.eachContainer(func(c *StorageContainer) {
		if !isKitchen[c.Pos] && c.held(Community, Battery)-taking[c.Pos] > 0 {
			sources = append(sources, c.Pos)
		}
	})
	type source struct {
		at   Point
		dist int
	}
	near := make([]source, len(sources))
	for i, k := range kitchens {
		if wants[i] <= 0 {
			continue
		}
		// eachContainer is map order: sort by distance, then position, so
		// which locker is emptied first is the seed's.
		for j, src := range sources {
			near[j] = source{src, w.travelEstimate(src, k)}
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
			n := min(want, w.storageContainers[src.at].held(Community, Battery)-taking[src.at])
			if n <= 0 {
				continue
			}
			if o := w.postWork(WorkHaul, Community, Money(w.cfg.HaulPay), n, k); o != nil {
				o.From, o.Item = src.at, Battery
				taking[src.at] += n
				want -= n
			}
		}
	}
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

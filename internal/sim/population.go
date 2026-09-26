package sim

// ---- Population history ----------------------------------------------------------
//
// The colony's vital signs over the whole game, for the Population tab: how
// many colonists, how many meals in storage, how big the colony is, and how
// many fixtures it has, sampled on the simulation clock (every popEvery
// ticks), not the wall clock, so a history is the same for a seed however
// fast it was played. It is read-only bookkeeping: nothing in the simulation
// reads it back, so it cannot disturb determinism.
//
// A game has no set length, so the history is kept at a fixed size by halving
// its resolution when it fills: every other sample is dropped and the interval
// doubles. The chart always spans the whole game, finer early on and coarser
// as it runs. See docs/population-screen.md.

// PopulationSample is one reading of the colony's vital signs.
type PopulationSample struct {
	Tick       int
	Colonists  int
	Meals      int // meals in any depot, whoever owns them and whether or not they are on offer
	ColonySize int // floor tiles the colony has dug or discovered
	Fixtures   int // placed fixtures: bunks, toilets, pods, lockers, workshops...
}

const (
	// popFirstEvery is the sampling interval a game starts with.
	popFirstEvery = 50
	// popHistory is the most samples kept before the resolution halves.
	popHistory = 512
)

// samplePopulation records a sample when one is due.
func (w *World) samplePopulation() {
	if w.popEvery == 0 {
		w.popEvery = popFirstEvery
	}
	if w.tick%w.popEvery != 0 {
		return
	}
	s := PopulationSample{
		Tick:       w.tick,
		Colonists:  w.countKind(Colonist),
		ColonySize: w.terrainCounts[Floor] - w.hiddenFloor,
		Fixtures:   len(w.fixtures),
	}
	for _, c := range w.storageContainers {
		s.Meals += c.Inventory.Count(Meal)
	}
	h := w.popHist
	if len(h) >= popHistory {
		// Halve: keep the samples still on the doubled interval.
		w.popEvery *= 2
		kept := make([]PopulationSample, 0, popHistory)
		for _, x := range h {
			if x.Tick%w.popEvery == 0 {
				kept = append(kept, x)
			}
		}
		h = kept
		if w.tick%w.popEvery != 0 {
			w.popHist = h
			return
		}
	}
	// Published snapshots share the history, so it is never written in place:
	// a full-capacity slice makes append copy, as with the perf history.
	w.popHist = append(h[:len(h):len(h)], s)
}

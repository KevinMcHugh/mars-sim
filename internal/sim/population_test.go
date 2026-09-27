package sim

import (
	"slices"
	"testing"
)

// The population history samples on the simulation clock, halves its
// resolution when full so it always spans the whole game, and never changes a
// slice a snapshot has already published.
func TestPopulationHistorySpansTheWholeGame(t *testing.T) {
	w := propertyWorld(t)
	w.spawn(Colonist, Point{8, 8})
	var published []PopulationSample
	var frozen []PopulationSample
	for tick := 1; tick <= 60000; tick++ {
		w.tick = tick
		w.samplePopulation()
		if tick == 20000 {
			published = w.snapshot(false, 0).Population
			frozen = slices.Clone(published)
		}
	}
	h := w.popHist
	if len(h) == 0 || len(h) > popHistory {
		t.Fatalf("history holds %d samples, want 1..%d", len(h), popHistory)
	}
	if w.popEvery <= popFirstEvery {
		t.Fatalf("interval still %d after 60000 ticks: the history never halved", w.popEvery)
	}
	for i, s := range h {
		if s.Tick%w.popEvery != 0 || (i > 0 && s.Tick-h[i-1].Tick != w.popEvery) {
			t.Fatalf("sample %d at tick %d: not evenly spaced at %d", i, s.Tick, w.popEvery)
		}
		if s.Colonists != 1 {
			t.Fatalf("sample at tick %d counts %d colonists, want 1", s.Tick, s.Colonists)
		}
	}
	if h[0].Tick != w.popEvery || h[len(h)-1].Tick < 60000-w.popEvery {
		t.Fatalf("history runs from tick %d to %d: it does not span the game", h[0].Tick, h[len(h)-1].Tick)
	}
	if !slices.Equal(published, frozen) {
		t.Fatal("a published history was changed by later sampling")
	}
}

// Meals are counted wherever they are stored, and fixtures as they are placed.
func TestPopulationSampleCountsMealsAndFixtures(t *testing.T) {
	w := propertyWorld(t)
	w.SetTerrain(Point{10, 10}, Storage)
	w.SetTerrain(Point{12, 10}, Bed)
	c := w.storageContainers[Point{10, 10}]
	c.Inventory.Add(Meal, 7)
	c.credit(Community, Meal, 7)
	w.tick = popFirstEvery
	w.samplePopulation()
	s := w.popHist[len(w.popHist)-1]
	if s.Meals != 7 || s.Fixtures != 2 || s.ColonySize <= 0 {
		t.Fatalf("sample %+v, want 7 meals, 2 fixtures, and some floor", s)
	}
}

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

// Fixtures are counted by kind, and colonists by their rank in each skill.
func TestPopulationSampleCountsFixtureKindsAndSkillRanks(t *testing.T) {
	w := propertyWorld(t)
	w.SetTerrain(Point{10, 10}, Scumhouse)
	w.SetTerrain(Point{12, 10}, Scumhouse)
	w.SetTerrain(Point{14, 10}, Incubator)
	w.SetTerrain(Point{16, 10}, Chair)
	a, b := w.spawn(Colonist, Point{8, 8}), w.spawn(Colonist, Point{8, 9})
	a.practice, b.practice = [numSkills]uint32{}, [numSkills]uint32{}
	w.setRank(a, SkillCooking, 2)
	w.setRank(b, SkillCooking, 3)
	w.setRank(b, SkillMining, -1)
	w.tick = popFirstEvery
	w.samplePopulation()
	s := w.popHist[len(w.popHist)-1]
	if s.FixtureKinds[Scumhouse] != 2 || s.FixtureKinds[Incubator] != 1 || s.FixtureKinds[Chair] != 1 || s.FixtureKinds[Bed] != 0 {
		t.Fatalf("fixture kinds %v, want 2 scumhouses, 1 incubator, 1 chair, no beds", s.FixtureKinds)
	}
	if s.Fixtures != 3 {
		t.Fatalf("%d fixtures, want 3: a chair is furniture, not a fixture", s.Fixtures)
	}
	cook := s.SkillRanks[SkillCooking]
	if cook[0] != 0 || cook[2] != 1 || cook[3] != 1 {
		t.Fatalf("cooking ranks %v, want one rank 2 and one rank 3", cook)
	}
	if top := topRank(SkillMining); s.SkillRanks[SkillMining][top] != 1 || s.SkillRanks[SkillMining][0] != 1 {
		t.Fatalf("mining ranks %v, want one untrained and one at rank %d", s.SkillRanks[SkillMining], top)
	}
	if s.SkillRanks[SkillSmithing][0] != 2 {
		t.Fatalf("smithing ranks %v, want both untrained", s.SkillRanks[SkillSmithing])
	}
}

// Every skill's ranks fit the sample's array, no title is shared between
// skills (the Population tab names a series by title alone), and every
// tracked kind is placed structure.
func TestTrackedKindsFit(t *testing.T) {
	skillOf := map[string]SkillKind{}
	for _, k := range Skills() {
		labels := SkillRankLabels(k)
		if n := len(labels); n > maxSkillRanks {
			t.Errorf("%s has %d ranks; maxSkillRanks is %d", k, n, maxSkillRanks)
		}
		for _, l := range labels[1:] {
			if other, ok := skillOf[l]; ok && other != k {
				t.Errorf("%q is a title in both %s and %s", l, other, k)
			}
			skillOf[l] = k
		}
	}
	for _, f := range TrackedFixtures {
		if !isFixtureTerrain(f) && f != Chair {
			t.Errorf("%s is tracked but is not a fixture", f)
		}
	}
	for f := Terrain(0); f < numTerrains; f++ {
		if isFixtureTerrain(f) && !slices.Contains(TrackedFixtures, f) {
			t.Errorf("fixture %s is not in TrackedFixtures", f)
		}
	}
}

package sim

import "testing"

// foodWorld is a fresh landing with no creatures, so hunger is the only thing
// that can kill anyone.
func foodWorld(t *testing.T, meals int, infinite bool) *World {
	t.Helper()
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 36
	cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0
	cfg.CrashPodMeals = meals
	cfg.InfiniteFood = infinite
	return newTestWorld(t, cfg)
}

// ownedMeals is every meal e owns: in its pockets and on any ledger.
func ownedMeals(w *World, e *Entity) int {
	n := e.Inventory.Count(Meal)
	if e.Job == JobEat && e.eat == eatMeal {
		n++ // in hand
	}
	for _, c := range w.storageContainers {
		n += c.held(ColonistOwner(e.ID), Meal)
	}
	return n
}

// A colonist with food of its own eats it, and only falls back on the safety
// net's gruel once its meals are gone.
func TestColonistsEatTheirOwnMealsBeforeGruel(t *testing.T) {
	w := foodWorld(t, 3, true)
	start := map[EntityID]int{}
	for _, id := range w.entityIDsSorted() {
		start[id] = ownedMeals(w, w.entities[id])
	}
	ateGruel := false
	for i := 0; i < 4000; i++ {
		w.step()
		for _, id := range w.entityIDsSorted() {
			e := w.entities[id]
			if e.Kind != Colonist {
				continue
			}
			if e.Job == JobUse && e.Need == NeedFood && ownedMeals(w, e) > 0 {
				t.Fatalf("tick %d: %s went to the pod with %d meals of its own", w.tick, e.displayName(), ownedMeals(w, e))
			}
		}
	}
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e.Kind != Colonist {
			continue
		}
		if ownedMeals(w, e) >= start[id] {
			t.Errorf("%s never ate any of its %d meals", e.displayName(), start[id])
		}
		for _, m := range e.Memories {
			ateGruel = ateGruel || m.Kind == EvtAteGruel
		}
	}
	if !ateGruel {
		t.Error("nobody ate gruel once the meals ran out; the safety net is not being used")
	}
}

// With the safety net off and nothing produced, the colony lives exactly as
// long as the meals it landed with, and then starves. This is the scarcity
// itself under test: the manifest sets the timeline.
//
// At the baseline rise of 2/tick a meal is due every SeekAt/2 = 325 ticks and
// takes UseTicks = 18 to eat. From there, hunger climbs from 0 to Max in 500
// ticks and then drains HP at StarveDamage a tick. So with m meals nobody can
// die before (m-1)·(325+18) + 18 + 500 + HP ticks, and everyone should be dead
// by m·(325+18+travel) + 500 + HP, allowing some walking.
func TestWithoutTheSafetyNetTheColonyStarvesOnSchedule(t *testing.T) {
	const meals = 3
	w := foodWorld(t, meals, false)
	spec := w.cfg.Needs[NeedFood]
	rise := spec.Rise
	cycle := spec.SeekAt/rise + spec.UseTicks
	dying := (spec.Max)/rise + w.cfg.ColonistHP/w.cfg.StarveDamage
	earliest := (meals-1)*cycle + spec.UseTicks + dying
	const travel = 60
	latest := meals*(cycle+travel) + dying
	n := w.countKind(Colonist)

	firstDeath, lastDeath := -1, -1
	for w.tick < latest+200 && w.countKind(Colonist) > 0 {
		before := w.countKind(Colonist)
		w.step()
		if w.countKind(Colonist) < before {
			if firstDeath < 0 {
				firstDeath = w.tick
			}
			lastDeath = w.tick
		}
		for _, id := range w.entityIDsSorted() {
			if e := w.entities[id]; e.Kind == Colonist && e.Job == JobUse && e.Need == NeedFood {
				t.Fatalf("tick %d: %s is using a pod with infinite-food off", w.tick, e.displayName())
			}
		}
	}
	if w.countKind(Colonist) != 0 {
		t.Fatalf("%d of %d colonists still alive at tick %d; the colony should have starved by %d",
			w.countKind(Colonist), n, w.tick, latest)
	}
	if firstDeath < earliest {
		t.Fatalf("first starvation at tick %d, before the manifest runs out (%d)", firstDeath, earliest)
	}
	if lastDeath > latest {
		t.Fatalf("last starvation at tick %d, after the predicted %d", lastDeath, latest)
	}
	if w.countTerrain(NutrientPod) != 0 {
		t.Fatalf("the colony built %d nutrient pods that feed nobody", w.countTerrain(NutrientPod))
	}
}

// Eating that is interrupted never loses the meal: it goes back in the pocket.
func TestInterruptedMealGoesBackInThePocket(t *testing.T) {
	w := foodWorld(t, 2, true)
	var e *Entity
	for _, id := range w.entityIDsSorted() {
		if c := w.entities[id]; c.Kind == Colonist {
			e = c
			break
		}
	}
	e.Inventory.Add(Meal, 1)
	e.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt
	if !w.tryStartEating(e) || e.eat != eatMeal || e.Inventory.Count(Meal) != 0 {
		t.Fatalf("did not start eating the carried meal: job %v stage %v meals %d", e.Job, e.eat, e.Inventory.Count(Meal))
	}
	w.jobEat(e)
	w.clearJob(e) // an alien comes round the corner
	if e.Inventory.Count(Meal) != 1 {
		t.Fatalf("interrupted meal lost: %d in pocket", e.Inventory.Count(Meal))
	}
}

// A colonist may eat the colony's meals, but never another colonist's.
func TestColonistsEatOnlyMealsTheyMayTake(t *testing.T) {
	w, e, chest := storageBehaviorWorld(t, true)
	other := w.spawn(Colonist, Point{12, 12})
	c := w.storageContainers[chest]
	c.Inventory.Add(Meal, 2)
	c.credit(ColonistOwner(other.ID), Meal, 2)
	if _, ok := w.nearestMealDepot(e); ok {
		t.Fatal("found a depot holding only someone else's meals")
	}
	c.Inventory.Add(Meal, 1)
	c.credit(Community, Meal, 1)
	if got, ok := w.nearestMealDepot(e); !ok || got != chest {
		t.Fatal("did not find the colony's meal")
	}
	if !w.takeMeal(e, c) || c.held(Community, Meal) != 0 || c.held(ColonistOwner(other.ID), Meal) != 2 {
		t.Fatalf("took the wrong meal: ledger %+v", c.Ledger)
	}
	if w.takeMeal(e, c) {
		t.Fatal("took another colonist's meal")
	}
}

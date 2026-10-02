package sim

import "testing"

// foodWorld is a fresh landing with no creatures, so hunger is the only thing
// that can kill anyone.
func foodWorld(t *testing.T, meals int, infinite bool) *World {
	t.Helper()
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 36
	cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0
	cfg.CrashPodMeals = meals
	cfg.InfiniteFood = infinite
	return newTestWorld(t, cfg)
}

// ownedMeals is every meal e owns: in its pockets and on any ledger.
func ownedMeals(w *World, e *Entity) int {
	n := e.Inventory.Count(Meal)
	if e.Job == JobEat && (e.eat == eatMeal || e.eat == eatWalk) {
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
			if e.Job == JobUse && e.Drive == DriveFood && ownedMeals(w, e) > 0 {
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
			ateGruel = ateGruel || m.Rule == "ate-gruel"
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
// At the base food rate a meal is due every SeekAt/rate ticks and takes
// UseTicks = 18 to eat. From there, hunger climbs from 0 to Max in Max/rate
// ticks and then drains HP at StarveDamage a tick. So with m meals nobody can
// die before (m-1)·(325+18) + 18 + 500 + HP ticks, and everyone should be dead
// by m·(325+18+travel) + 500 + HP, allowing some walking — plus a night in
// bed for each day that takes, since hunger is paused while asleep.
func TestWithoutTheSafetyNetTheColonyStarvesOnSchedule(t *testing.T) {
	const meals = 3
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 36
	cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0
	cfg.CrashPodMeals = meals
	cfg.InfiniteFood = false
	cfg.ScumPercent = 0        // nothing to make food from: no scum, and no creatures to die
	cfg.IncubatorGrowTicks = 0 // and no incubator to seed: this colony produces nothing
	w := newTestWorld(t, cfg)
	spec := w.cfg.Drives[DriveFood]
	// testConfig grows every drive at its base rate whatever the colonist
	// is doing awake, and pauses hunger in bed.
	ticksTo := func(points int) int { return points * driveUnit / spec.Rate }
	cycle := ticksTo(spec.SeekAt) + spec.UseTicks
	dying := ticksTo(spec.Max) + w.cfg.ColonistHP/w.cfg.StarveDamage
	earliest := (meals-1)*cycle + spec.UseTicks + dying
	const travel = 60
	latest := meals*(cycle+travel) + dying
	latest += (latest/w.cfg.TicksPerDay() + 1) * w.cfg.Drives[DriveSleep].UseTicks
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
			if e := w.entities[id]; e.Kind == Colonist && e.Job == JobUse && e.Drive == DriveFood {
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
	w.setDrive(e, DriveFood, w.cfg.Drives[DriveFood].SeekAt)
	if !w.tryStartEating(e) || e.eat != eatMeal || e.Inventory.Count(Meal) != 0 {
		t.Fatalf("did not start eating the carried meal: job %v stage %v meals %d", e.Job, e.eat, e.Inventory.Count(Meal))
	}
	w.jobEat(e)
	w.clearJob(e) // an alien comes round the corner
	if e.Inventory.Count(Meal) != 1 {
		t.Fatalf("interrupted meal lost: %d in pocket", e.Inventory.Count(Meal))
	}
}

// A colonist eats only its own meals: never another colonist's, and not the
// colony's either — those are for sale (see TestCookingTurnsTheColonysScumIntoItsMeals).
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
	if _, ok := w.nearestMealDepot(e); ok {
		t.Fatal("found a depot holding only the colony's and someone else's meals")
	}
	if w.takeMeal(e, c) {
		t.Fatalf("took a meal that was not its own: ledger %+v", c.Ledger)
	}
	c.Inventory.Add(Meal, 1)
	c.credit(ColonistOwner(e.ID), Meal, 1)
	if got, ok := w.nearestMealDepot(e); !ok || got != chest {
		t.Fatal("did not find its own meal")
	}
	if !w.takeMeal(e, c) || c.held(ColonistOwner(e.ID), Meal) != 0 || c.held(Community, Meal) != 1 ||
		c.held(ColonistOwner(other.ID), Meal) != 2 {
		t.Fatalf("took the wrong meal: ledger %+v", c.Ledger)
	}
}

// A colonist whose hunger is pressing, with scum of its own in a scumhouse,
// drops whatever it was doing to cook it. Left to finish a dig, colonists starved
// with their own scum sitting in a free scumhouse.
func TestPressingHungerDropsWorkToCook(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.InfiniteFood = false
	house := Point{10, 6}
	w.SetTerrain(house, Scumhouse)
	w.SetTerrain(Point{20, 12}, Rock)
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{10, 7})
	me := ColonistOwner(e.ID)
	c := w.storageContainers[house]
	c.Inventory.Add(CaveScum, 2)
	c.credit(me, CaveScum, 2)
	w.assignMineTarget(e, Point{20, 12})

	// hungryWithoutFood only ever runs at pressing hunger: the eat focus
	// isn't eligible before that.
	e.drives[DriveFood].phase = DrivePressing
	w.hungryWithoutFood(e)
	if e.Job != JobCraft || e.craftFor != me {
		t.Fatalf("pressing hunger kept job %v (for %v); want cooking its own scum", e.Job, e.craftFor)
	}
}

// A colonist at pressing hunger with nothing of its own to cook, cooking the
// colony's scum, finishes the recipe: the meal goes on the colony's counter,
// where it can buy it, or be rationed it at critical hunger. hungryWithoutFood
// used to drop the job (it was not "feeding itself") and assignWorkJob handed
// the same job straight back, every turn, so the recipe never got past its
// first tick and the colony's last colonists starved at the stove beside its
// scum (seed 4 with -silo-bid-qty 0).
func TestPressingHungerFinishesTheColonysCooking(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.InfiniteFood, w.cfg.MealReserve = false, 100
	house := Point{10, 6}
	w.SetTerrain(house, Scumhouse)
	w.refreshSpatial()
	c := w.storageContainers[house]
	c.Inventory.Add(CaveScum, 2)
	c.credit(Community, CaveScum, 2)
	for p := range w.scum {
		w.clearScum(p) // nothing on the walls: the colony's scum is the only food to make
	}
	e := w.spawn(Colonist, Point{10, 7})
	e.drives[DriveFood].phase = DrivePressing

	for i := 0; i < 200 && c.held(Community, CaveScum) > 0; i++ {
		w.hungryWithoutFood(e)
	}
	if got := c.held(Community, CaveScum); got != 0 {
		t.Fatalf("the colony still holds %d scum after 200 turns: the hungry cook never finished a recipe", got)
	}
	if w.communityMeals()+w.openQty(Ask, Meal, w.outputDepot(house), Community) == 0 {
		t.Fatal("the recipe ran but the colony has no meal to sell")
	}
}

// A colonist at critical hunger that cannot afford a meal is given one of the
// colony's: only at critical hunger, and only one. The meal changes hands on
// the ledger, never leaving the shelf.
func TestTheColonyRationsTheStarving(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.InfiniteFood = false
	shelf := Point{10, 6}
	w.SetTerrain(shelf, Storage)
	w.refreshSpatial()
	c := w.storageContainers[shelf]
	c.Inventory.Add(Meal, 3)
	c.credit(Community, Meal, 3)
	w.offerColonyMeals(shelf) // on sale, in escrow
	e := w.spawn(Colonist, Point{12, 8})
	me := ColonistOwner(e.ID)
	w.transfer(me, Community, e.wallet) // broke

	e.drives[DriveFood].phase = DrivePressing
	if w.tryRation(e) {
		t.Fatal("rationed a colonist whose hunger is only pressing")
	}
	e.drives[DriveFood].phase = DriveCritical
	if !w.tryRation(e) {
		t.Fatal("no ration for a broke colonist at critical hunger")
	}
	if c.held(me, Meal) != 1 || c.Inventory.Count(Meal) != 3 || !c.ledgerBalanced() {
		t.Fatalf("after the ration: held %d, on the shelf %d, ledger %v", c.held(me, Meal), c.Inventory.Count(Meal), c.Ledger)
	}
	if got := c.held(Community, Meal) + w.openQty(Ask, Meal, shelf, Community); got != 2 {
		t.Fatalf("the colony has %d meals left, want 2", got)
	}
	assertMoneyConserved(t, w)
}

// A colony cook works a batch at the stove rather than walking across the
// colony for every twelve-tick recipe.
func TestAColonyCookWorksABatch(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.InfiniteFood, w.cfg.MealReserve = false, 100
	house := Point{10, 6}
	w.SetTerrain(house, Scumhouse)
	w.refreshSpatial()
	c := w.storageContainers[house]
	c.Inventory.Add(CaveScum, 20)
	c.credit(Community, CaveScum, 20)
	cook := w.spawn(Colonist, Point{10, 7})
	if !w.tryAssignCraft(cook) || cook.craftFor != Community {
		t.Fatal("no colony cooking job")
	}
	for i := 0; i < 1000 && cook.Job == JobCraft; i++ {
		w.jobCraft(cook)
	}
	if got := c.held(Community, CaveScum); got != 0 {
		t.Fatalf("the cook left %d scum uncooked; it should stay at the stove until it's gone", got)
	}
}

// setHunger puts e's food drive at level as of now.
func setHunger(w *World, e *Entity, level int) {
	w.setDrive(e, DriveFood, level)
	w.syncDrivePhase(e, DriveFood)
}

// Before it's hungry enough to eat, a colonist with no meal on it fetches
// one of its own to carry, and eats it the moment hunger presses: no walk.
func TestAColonistCarriesItsNextMeal(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.InfiniteFood = false
	shelf := Point{16, 10}
	w.SetTerrain(shelf, Storage)
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{8, 8})
	me := ColonistOwner(e.ID)
	c := w.storageContainers[shelf]
	c.Inventory.Add(Meal, 2)
	c.credit(me, Meal, 2)

	setHunger(w, e, w.cfg.PocketMealAt-1)
	if w.tryPocketMeal(e) {
		t.Fatal("fetched a pocket meal before pocket-meal-at")
	}
	setHunger(w, e, w.cfg.PocketMealAt)
	w.assignWorkJob(e)
	if e.Job != JobEat || !e.eatKeep {
		t.Fatalf("at pocket-meal-at: job %v (keep %v), want fetching a meal to carry", e.Job, e.eatKeep)
	}
	for i := 0; i < 200 && e.Job == JobEat; i++ {
		w.jobEat(e)
	}
	if e.ownCarried(Meal) != 1 || c.held(me, Meal) != 1 {
		t.Fatalf("after the fetch: %d carried, %d on the shelf; want 1 and 1", e.ownCarried(Meal), c.held(me, Meal))
	}
	if w.driveLevel(e, DriveFood) == 0 {
		t.Fatal("fetching a pocket meal fed the colonist")
	}
	w.assignWorkJob(e)
	if e.Job == JobEat {
		t.Fatal("a colonist with a meal on it fetched another")
	}

	setHunger(w, e, w.cfg.Drives[DriveFood].SeekAt)
	if !w.tryStartEating(e) || e.eat != eatMeal {
		t.Fatalf("pressing hunger with a pocket meal: job %v stage %v, want eating at once", e.Job, e.eat)
	}
}

// A pocket meal is only ever the colonist's own: it never buys one, so it
// never takes a meal off the shelf from a colonist hungrier than it is.
func TestAPocketMealIsNeverBought(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.InfiniteFood = false
	shelf := Point{16, 10}
	w.SetTerrain(shelf, Storage)
	w.refreshSpatial()
	c := w.storageContainers[shelf]
	c.Inventory.Add(Meal, 1)
	c.credit(Community, Meal, 1)
	w.offerColonyMeals(shelf)
	e := w.spawn(Colonist, Point{8, 8})
	me := ColonistOwner(e.ID)
	setHunger(w, e, w.cfg.PocketMealAt)
	if w.tryPocketMeal(e) || c.held(me, Meal) != 0 || w.hasOpenMealBid(me) {
		t.Fatalf("with only the colony's meal on sale: job %v, %d bought", e.Job, c.held(me, Meal))
	}
}

// Taking surplus meals to market never sells the pocket meal.
func TestSellingSurplusKeepsThePocketMeal(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.MealKeep = 0
	w.SetTerrain(Point{14, 10}, Storage)
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{8, 8})
	e.Inventory.Add(Meal, 1)
	silo, ok := w.marketDepot()
	if !ok {
		t.Fatal("no silo to sell at")
	}
	if w.surplusMeals(e, silo) != 0 {
		t.Fatalf("surplus with only a pocket meal = %d, want 0", w.surplusMeals(e, silo))
	}
	if w.tryAssignSellMeals(e) {
		t.Fatal("went to market to sell its pocket meal")
	}
}

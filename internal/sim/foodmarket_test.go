package sim

import "testing"

// The colony's meal price is the charter's while stores hold meal-reserve
// meals per colonist, and rises in step as they fall short, to meal-price-max
// percent with nothing stored. Every meal in storage counts, the colony's or
// not: at landing the colony has none, and the lockers are full.
func TestTheColonysMealPriceRisesAsStoresFall(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	w.cfg.MealReserve, w.cfg.MealPriceMax, w.cfg.PriceMeal = 2, 300, 5
	for i := 0; i < 5; i++ {
		w.spawn(Colonist, Point{12 + i, 10})
	}
	c := w.storageContainers[house]
	stock := func(n int) {
		c.Inventory.Remove(Meal, c.Inventory.Count(Meal))
		c.Inventory.Add(Meal, n)
		w.storedMealsTick = -1
	}
	for _, tc := range []struct {
		meals int
		want  Money
	}{{10, 5}, {20, 5}, {5, 10}, {0, 15}} {
		stock(tc.meals)
		if got := w.colonyMealPrice(); got != tc.want {
			t.Errorf("%d meals stored for 5 colonists: price %v, want %v", tc.meals, got, tc.want)
		}
	}
	w.cfg.MealPriceMax = 100
	if got := w.colonyMealPrice(); got != 5 {
		t.Errorf("meal-price-max 100 and nothing stored: price %v, want the charter's 5", got)
	}
}

// When the price moves, the colony's asks move with it at the next market
// upkeep; they are not left at the old price.
func TestTheColonyRepricesItsMeals(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	w.cfg.MealReserve, w.cfg.MealPriceMax, w.cfg.PriceMeal = 10, 300, 5
	w.spawn(Colonist, Point{12, 10})
	c := w.storageContainers[house]
	c.Inventory.Add(Meal, 10)
	c.credit(Community, Meal, 10)
	w.tick = marketInterval
	w.refreshColonyMealAsks()
	if ask, ok := w.bestAsk(Meal, house); !ok || ask.Price != 5 {
		t.Fatalf("with a full reserve: ask %+v, want $5", ask)
	}
	w.spawn(Colonist, Point{13, 10}) // a second mouth: 10 meals is now half the reserve
	w.refreshColonyMealAsks()
	if ask, ok := w.bestAsk(Meal, house); !ok || ask.Price != w.colonyMealPrice() || ask.Price <= 5 {
		t.Fatalf("with half the reserve: ask %+v, want the risen price %v", ask, w.colonyMealPrice())
	}
	if got := w.openQty(Ask, Meal, house, Community); got != 10 || !c.ledgerBalanced() {
		t.Fatalf("after repricing: %d meals on offer, ledger %+v", got, c.Ledger)
	}
}

// A colonist cooks for sale only when a meal sells for more than making one
// costs it, and then before the colony's food work.
func TestFoodOnItsOwnAccountWhenItPays(t *testing.T) {
	w, _ := scumhouseWorld(t, false)
	e := w.spawn(Colonist, Point{12, 10})
	w.cfg.PriceMeal, w.cfg.MealPriceMax = 2, 100
	if w.foodPays(e) {
		t.Fatal("a $2 meal pays for two units of scum and the work")
	}
	w.cfg.PriceMeal = 20
	if !w.foodPays(e) {
		t.Fatal("a $20 meal next to a scumhouse doesn't pay")
	}
}

// A colonist's own cooking goes on sale where it's made, beyond what it
// keeps, at the colonist's selling price.
func TestACookSellsItsSurplusWhereItCooks(t *testing.T) {
	w, house := scumhouseWorld(t, false)
	w.cfg.MealKeep = 0
	c := w.storageContainers[house]
	cook := w.spawn(Colonist, Point{12, 10})
	me := ColonistOwner(cook.ID)
	c.Inventory.Add(CaveScum, 2)
	c.credit(me, CaveScum, 2)
	if !w.tryAssignCraftFor(cook, []Owner{me}) {
		t.Fatal("no job cooking its own scum")
	}
	for i := 0; i < 200 && cook.Job == JobCraft; i++ {
		w.jobCraft(cook)
	}
	out := w.outputDepot(house)
	if got := w.openQty(Ask, Meal, out, me); got != 1 {
		t.Fatalf("the cook has %d meals on offer where it cooked, want 1", got)
	}
	if ask, _ := w.bestAsk(Meal, out); ask.Price != w.mealSellPrice() {
		t.Fatalf("offered at %v, want %v", ask.Price, w.mealSellPrice())
	}
}

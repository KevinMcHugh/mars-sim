package sim

import "testing"

// A colonist's unsold food ask comes down a step every ask-decay-ticks, to
// $1. Ore keeps its price, and so does the colony's ask.
func TestUnsoldFoodAsksComeDown(t *testing.T) {
	w, house, silo, cols := producerWorld(t, 1)
	me := ColonistOwner(cols[0].ID)
	w.cfg.AskDecayTicks, w.cfg.AskDecayPercent = 100, 10
	stock(w, house, me, CaveScum, 4)
	stock(w, silo, me, IronOre, 4)
	stock(w, house, Community, CaveScum, 4)
	w.post(Ask, CaveScum, 4, 20, me, house, 0)
	w.post(Ask, IronOre, 4, 20, me, silo, 0)
	w.post(Ask, CaveScum, 4, 20, Community, house, 0)
	price := func(actor Owner, item ItemKind) Money {
		os := w.sortedOrders(func(o *Order) bool { return o.Actor == actor && o.Item == item && o.Side == Ask })
		if len(os) != 1 {
			t.Fatalf("%v's asks for %v: %+v, want one", actor, item, os)
		}
		return os[0].Price
	}
	w.tick += 99
	w.decayAsks()
	if got := price(me, CaveScum); got != 20 {
		t.Fatalf("an ask came down before ask-decay-ticks: $%v", got)
	}
	w.tick++
	w.decayAsks()
	if got := price(me, CaveScum); got != 18 {
		t.Fatalf("after ask-decay-ticks the ask is $%v, want $18", got)
	}
	for i := 0; i < 40; i++ {
		w.tick += 100
		w.decayAsks()
	}
	if got := price(me, CaveScum); got != 1 {
		t.Fatalf("a long-unsold ask is $%v, want the $1 floor", got)
	}
	if price(me, IronOre) != 20 || price(Community, CaveScum) != 20 {
		t.Fatal("ore or the colony's ask came down")
	}
	assertMoneyConserved(t, w)
}

// Idle food a colonist owns at a kitchen goes up for sale at its value: scum
// beyond one recipe's worth, meals beyond what it keeps. A colonist at work
// on a plan keeps everything.
func TestIdleFoodIsOfferedForSale(t *testing.T) {
	w, house, _, cols := producerWorld(t, 2)
	idle, planner := ColonistOwner(cols[0].ID), ColonistOwner(cols[1].ID)
	stock(w, house, idle, CaveScum, 10)
	stock(w, house, idle, Meal, w.cfg.MealKeep+3)
	stock(w, house, planner, CaveScum, 10)
	cols[1].plan = 99
	w.relistIdleFood()
	if got := w.openQty(Ask, CaveScum, house, idle); got != 10-biomatterKeep {
		t.Errorf("%d of the idle colonist's scum on offer, want %d", got, 10-biomatterKeep)
	}
	if got := w.openQty(Ask, Meal, house, idle); got != 3 {
		t.Errorf("%d of its meals on offer, want the 3 beyond meal-keep", got)
	}
	if got := w.openQty(Ask, CaveScum, house, planner); got != 0 {
		t.Errorf("a colonist with a plan under way offered %d scum", got)
	}
	for _, o := range w.sortedOrders(func(o *Order) bool { return o.Side == Ask }) {
		if o.Price != w.valueOf(o.Item) {
			t.Errorf("%v offered at $%v, want its value $%v", o.Item, o.Price, w.valueOf(o.Item))
		}
	}
	w.relistIdleFood()
	if got := w.openQty(Ask, CaveScum, house, idle); got != 10-biomatterKeep {
		t.Errorf("offering again doubled it: %d on offer", got)
	}
}

// A hungry colonist's waiting meal bid starts below a meal's value and rises
// toward its limit while it goes unfilled; a plan serving it follows it.
func TestAWaitingMealBidRises(t *testing.T) {
	w, _, _, cols := producerWorld(t, 1)
	e := cols[0]
	e.wallet = 100
	w.cfg.BidRaiseTicks, w.cfg.BidRaisePercent, w.cfg.BidStartPercent = 50, 20, 80
	w.recordPrice(Meal, 10)
	w.setDrive(e, DriveFood, w.cfg.Drives[DriveFood].Max*6/10)
	limit := w.mealBidLimit(e)
	w.tryBuyMeal(e)
	bid := func() *Order {
		os := w.sortedOrders(func(o *Order) bool { return o.hunger })
		if len(os) != 1 {
			t.Fatalf("hunger bids: %+v, want one", os)
		}
		return os[0]
	}
	if got := bid().Price; got != 8 {
		t.Fatalf("the waiting bid starts at $%v, want $8 (80%% of $10)", got)
	}
	w.plans[1] = &plan{id: 1, actor: e.ID, target: bid().ID, expires: 1 << 30}
	w.tick += 50
	w.raiseMealBids()
	want := 8 + limit*20/100
	if got := bid().Price; got != want {
		t.Fatalf("after bid-raise-ticks the bid is $%v, want $%v", got, want)
	}
	if w.plans[1].target != bid().ID {
		t.Fatal("the plan serving the bid lost it when it was raised")
	}
	for i := 0; i < 20; i++ {
		w.tick += 50
		w.raiseMealBids()
	}
	if got := bid().Price; got != w.mealBidLimitWith(e, e.wallet+bid().escrow) {
		t.Fatalf("a long-unfilled bid is $%v, want its limit", got)
	}
	assertMoneyConserved(t, w)
}

// At critical hunger there is no haggling: the bid goes in at the limit.
func TestAStarvingColonistBidsItsLimitAtOnce(t *testing.T) {
	w, _, _, cols := producerWorld(t, 1)
	e := cols[0]
	e.wallet = 60
	w.setDrive(e, DriveFood, w.cfg.Drives[DriveFood].Max)
	w.tryBuyMeal(e)
	os := w.sortedOrders(func(o *Order) bool { return o.hunger })
	if len(os) != 1 || os[0].Price != 60 {
		t.Fatalf("a starving colonist with $60 bids %+v, want $60", os)
	}
}

// Meals sell at what they fetch: a cook asks a meal's market value, and
// foodPays reckons by it.
func TestCooksSellAtTheMarketPrice(t *testing.T) {
	w, _, _, _ := producerWorld(t, 0)
	w.recordPrice(Meal, 40)
	if got := w.mealSellPrice(); got != 40 {
		t.Fatalf("with meals trading at $40 a cook asks $%v", got)
	}
	w.cfg.MealSellAtMarket = false
	if got := w.mealSellPrice(); got != w.refPrice(Meal) {
		t.Fatalf("off, a cook asks $%v, want the charter's $%v", got, w.refPrice(Meal))
	}
}

// A hungry colonist about to cook its own scum takes it off sale first.
func TestAHungryColonistTakesItsScumOffSale(t *testing.T) {
	w, house, _, cols := producerWorld(t, 1)
	e := cols[0]
	me := ColonistOwner(e.ID)
	stock(w, house, me, CaveScum, 4)
	w.post(Ask, CaveScum, 4, 9, me, house, 0)
	if !w.tryAssignFoodWork(e, true) || e.Job != JobCraft {
		t.Fatalf("a hungry colonist with scum on sale took job %v, want to cook it", e.Job)
	}
	if got := w.openQty(Ask, CaveScum, house, me); got != 0 {
		t.Fatalf("%d of its scum still on sale", got)
	}
}

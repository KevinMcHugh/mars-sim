package sim

// ---- Price discovery -------------------------------------------------------------
//
// The book only finds a price if somebody moves one. Before this, nobody did:
// a cook asked the charter's $5 whatever meals fetched, a hungry colonist's
// waiting bid sat at its whole limit (the meal's value times its hunger),
// and an ask that didn't sell expired and was never offered again. A waiting
// bid filled at its own price, so every fill raised the remembered price and
// the next bid with it, to about half a wallet; and the colonists' idle
// scum, thousands of units, never reached the book to pull scum's price down.
//
// Three moves, each in the market's upkeep (movePrices):
//
//   - an unsold food ask comes down a step at a time (decayAsks);
//   - idle food a colonist owns at the kitchens and the silo goes up for sale
//     at its value (relistIdleFood);
//   - a hungry colonist's waiting bid starts below a meal's value and rises
//     while it goes unfilled (raiseMealBids, mealBidStart).
//
// See docs/pricing.md.

// relistInterval is how often idle food is offered again: often enough that
// a glut reaches the book within a meal or two, seldom enough that walking
// every kitchen's ledger costs nothing.
const relistInterval = 100

// movePrices is price discovery's share of the market's upkeep.
func (w *World) movePrices() {
	w.raiseMealBids()
	if !w.pricesFree() {
		return // nothing sold off cheap before the kitchens run (pricesFree)
	}
	w.decayAsks()
	if w.tick%relistInterval == 0 {
		w.relistIdleFood()
	}
}

// pricesFree reports whether prices float yet: once the colony has held
// free-prices-at meals per colonist, its kitchens are running, and from then
// on for good. Until then trades leave the remembered prices where they are
// (recordPrice), asks don't come down, and idle food isn't offered. Waiting
// bids still rise: a hungry colonist still pays what it takes.
//
// Floating from landing, the market sold the colonists' spare landing meals
// at the floor before the colony's kitchens had cooked anything. When the
// shelves emptied, the first few meals filled bids that had climbed to $40
// and more, a meal's value went from $4 to $62 in 250 ticks, and cooking for
// oneself suddenly paid for 95 colonists of 100 at once (foodPays). They left
// the colony's kitchens and incubators idle to scrape wild scum, and
// colonists with money starved with nothing on offer (docs/pricing.md).
func (w *World) pricesFree() bool {
	if w.pricesFreed || w.cfg.FreePricesAt <= 0 {
		return true
	}
	if n := w.countKind(Colonist); n > 0 && w.communityMeals() >= w.cfg.FreePricesAt*n {
		w.pricesFreed = true
	}
	return w.pricesFreed
}

// isFood reports whether k is a meal or what one is cooked from: the goods
// whose unsold asks come down in price. Ore isn't: the colony's standing bids
// at the charter's prices set ore's price, and a miner's ore resting at $1
// would only hand the next bid a windfall.
func isFood(k ItemKind) bool { return k == Meal || isBiomatter(k) }

// repost moves an open order to price: a cancel and a re-post, so it may trade
// at once, keeping what makes it what it is (its expiry, its plan, its
// flags, its fills). A plan serving the old order serves the new one. It
// returns the new order, or nil if it could not be re-posted.
func (w *World) repost(o *Order, price Money) *Order {
	side, item, qty, actor, depot := o.Side, o.Item, o.Qty, o.Actor, o.Depot
	ttl := 0
	if o.Expires > 0 {
		ttl = max(1, o.Expires-w.tick)
	}
	w.cancel(o)
	n, _ := w.post(side, item, qty, price, actor, depot, ttl)
	if n == nil {
		return nil
	}
	n.plan, n.depth, n.manual, n.wide, n.hunger = o.plan, o.depth, o.manual, o.wide, o.hunger
	n.inherit(o)
	for _, p := range w.plans {
		if p.target == o.ID {
			p.target = n.ID
		}
	}
	return n
}

// decayAsks lowers every colonist's food ask that has waited ask-decay-ticks
// at its price by ask-decay-percent, at least a dollar, to no less than its
// floor (askFloor).
// The colony's asks keep their price: the player or the charter set it.
func (w *World) decayAsks() {
	every, pct := w.cfg.AskDecayTicks, int64(w.cfg.AskDecayPercent)
	if every <= 0 || pct <= 0 {
		return
	}
	for _, o := range w.sortedOrders(func(o *Order) bool {
		return o.Side == Ask && o.Actor.Kind == OwnerColonist && isFood(o.Item) &&
			o.Price > w.askFloor(o.Item) && w.tick-o.priced >= every
	}) {
		cut := max(1, Money(int64(o.Price)*pct/100))
		w.repost(o, max(w.askFloor(o.Item), o.Price-cut))
	}
}

// askFloor is the least a colonist asks for item: for a meal, what its scum
// costs (mealInputCost), so a cook never sells a meal for less than its
// inputs would fetch; for anything else, $1.
//
// Without it, decay took meals to $1 in the glut after landing, when
// colonists sell their spare locker meals and nobody is hungry yet. The first
// $1 fills set a meal's value, cooks asked that, and cooking stopped paying
// just as the colony needed it to start (docs/pricing.md).
func (w *World) askFloor(item ItemKind) Money {
	if item == Meal {
		return w.mealInputCost()
	}
	return 1
}

// mealInputCost is what the scum for one meal is worth now: the scum recipe's
// inputs at their value, per meal it makes, at least $1.
func (w *World) mealInputCost() Money {
	r, ok := scumMealRecipe()
	if !ok {
		return 1
	}
	meals := 0
	for _, o := range r.Outputs {
		if o.Kind == Meal {
			meals += o.Count
		}
	}
	var cost Money
	for _, in := range r.Inputs {
		cost += Money(in.Count) * w.valueOf(in.Kind)
	}
	return max(1, cost/Money(max(1, meals)))
}

// mealBidStart is where e's waiting bid for a meal starts: bid-start-percent
// of a meal's value, or its limit if that's lower. At critical hunger, or with
// bid-raise-ticks 0, it bids its limit at once.
func (w *World) mealBidStart(e *Entity, limit Money) Money {
	if w.cfg.BidRaiseTicks <= 0 || w.foodCritical(e) {
		return limit
	}
	start := Money(max(1, int64(w.valueOf(Meal))*int64(w.cfg.BidStartPercent)/100))
	return min(limit, start)
}

// raiseMealBids raises each hungry colonist's waiting meal bid that has gone
// bid-raise-ticks unfilled by bid-raise-percent of its limit, at least a
// dollar, up to that limit (which rises with its hunger). A step of the limit,
// not of a meal's value, so a bid gets there inside its demand-ttl however low
// the last meal sold. At critical
// hunger it goes straight to the limit: there is no time left to haggle.
func (w *World) raiseMealBids() {
	every, pct := w.cfg.BidRaiseTicks, int64(w.cfg.BidRaisePercent)
	if every <= 0 {
		return
	}
	for _, o := range w.sortedOrders(func(o *Order) bool { return o.hunger && o.Side == Bid && o.Qty > 0 }) {
		e := w.entities[o.Actor.ID]
		if e == nil {
			continue
		}
		limit := w.mealBidLimitWith(e, e.wallet+o.escrow) // the escrow comes back to re-post
		target := o.Price
		switch {
		case w.foodCritical(e):
			target = limit
		case w.tick-o.priced >= every:
			step := max(1, Money(int64(limit)*pct/100))
			target = min(limit, o.Price+step)
		}
		if target > o.Price {
			w.repost(o, target)
		}
	}
}

// relistIdleFood offers the food colonists own and aren't using, at the
// kitchens and the silo, at its value; from there decayAsks brings it down
// until it sells. That is a meal beyond what its owner keeps (meal-keep, and
// its pocket meal: surplusMeals, counting every depot), and scum or other
// biomatter beyond one recipe's worth at each stove. A colonist with a plan
// under way, or cooking, scraping or eating, keeps everything for now: its
// stock may be that work's inputs or its supper.
//
// Before this, scum a colonist scraped for a bid somebody else filled first
// was offered once, at that bid's price, and when the ask expired it sat on
// the colonist's line for good: in a 100-colonist game, about 2,000 units by
// tick 30,000 while scum traded at $40, and nobody cooked it.
func (w *World) relistIdleFood() {
	if !w.cfg.RelistIdle {
		return
	}
	silo, _ := w.marketDepot()
	meals := map[EntityID]int{} // what each colonist may still list, worked out once
	for _, p := range w.mealDepots() {
		c := w.storageContainers[p]
		if c == nil || !w.communalFixture(p) {
			continue
		}
		for _, l := range append([]LedgerLine(nil), c.Ledger...) { // posting edits the ledger
			if l.Owner.Kind != OwnerColonist || l.Count <= 0 || !isFood(l.Item) {
				continue
			}
			e := w.entities[l.Owner.ID]
			if e == nil || e.plan != 0 || busyWithFood(e) {
				continue
			}
			n := l.Count
			if l.Item == Meal {
				left, ok := meals[e.ID]
				if !ok { // everything it owns, the silo too, less what it keeps
					left = w.surplusMeals(e, silo)
					if sc := w.storageContainers[silo]; sc != nil {
						left += sc.held(l.Owner, Meal)
					}
				}
				n = min(n, left)
				meals[e.ID] = left - max(0, n)
			} else {
				n -= biomatterKeep
			}
			price := max(w.valueOf(l.Item), w.askFloor(l.Item))
			if n <= 0 || price <= 0 {
				continue
			}
			w.post(Ask, l.Item, n, price, l.Owner, p, 0)
		}
	}
}

// biomatterKeep is how much of each kind of biomatter a colonist keeps at a
// stove when its idle stock goes on sale: one recipe's worth, its own supper.
const biomatterKeep = 2

// busyWithFood reports whether e is at work that may use the food it owns.
func busyWithFood(e *Entity) bool {
	switch e.Job {
	case JobCraft, JobScrape, JobEat, JobSell, JobCarry, JobTend:
		return true
	}
	return false
}

// withdrawOwnAsks takes owner's asks for the items keep picks off the book,
// returning the goods to its line: a hungry colonist about to cook its own
// scum takes it back off sale first.
func (w *World) withdrawOwnAsks(owner Owner, keep func(ItemKind) bool) {
	for _, o := range w.sortedOrders(func(o *Order) bool {
		return o.Side == Ask && o.Actor == owner && keep(o.Item)
	}) {
		w.cancel(o)
	}
}

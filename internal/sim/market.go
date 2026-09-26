package sim

import "sort"

// ---- The order book -------------------------------------------------------------
//
// Colonists and the colony trade goods through limit orders at a depot: a bid
// offers money for goods, an ask offers goods for money. A new order crosses
// the best opposite order in the same (item, depot) book — best price first,
// then the older order — and trades at the resting order's price; whatever is
// left rests in the book until it fills, is cancelled, or expires.
//
// Posting escrows what the order promises, so settlement can never fail: a
// bid's money moves out of the bidder's account into the order, and an ask's
// goods move off the seller's ledger line onto the order's own line at the
// depot. Both are ordinary transfers and ledger moves, so the money audit and
// every ledger still balance with orders open. Settlement is a ledger move and
// a transfer, both at one depot, in one step: no goods move physically.
//
// Nothing about which order matches is ever decided by ranging over a map.
// See docs/market.md.

// Side is which side of the book an order is on.
type Side uint8

const (
	Bid Side = iota // offers money for goods
	Ask             // offers goods for money
)

func (s Side) String() string {
	if s == Bid {
		return "bid"
	}
	return "ask"
}

// OrderID identifies an order. IDs come from a counter, so a lower ID is an
// older order: the time-priority tiebreak.
type OrderID uint64

// Order is one limit order in a book.
type Order struct {
	ID      OrderID
	Side    Side
	Item    ItemKind
	Qty     int   // still unfilled
	Price   Money // per unit, the limit
	Actor   Owner
	Depot   Point
	Posted  int
	Expires int // tick; 0 never

	// escrow is the money a bid is holding, Qty × Price at rest. An ask's
	// escrow is goods, on the order's own ledger line at the depot.
	escrow Money
	// plan is the production plan a derived bid was posted for, and depth how
	// many links below a finished-good bid it is (0 for a bid of its own).
	// See producer.go.
	plan  planID
	depth int
}

// owner is the order itself as a ledger or money holder: where its escrow
// lives.
func (o *Order) owner() Owner { return Owner{Kind: ownerOrder, ID: EntityID(o.ID)} }

// bookKey names one book: an item at a depot.
type bookKey struct {
	Item  ItemKind
	Depot Point
}

// book is the open orders for one item at one depot, each side kept sorted
// best-first: bids by price down, asks by price up, then by age.
type book struct {
	bids, asks []*Order
	last       Money // price of the most recent trade
	volume     int   // units traded, ever
	traded     bool
}

// Trade is one fill, for the market view and the log of what happened.
type Trade struct {
	Tick   int
	Item   ItemKind
	Depot  Point
	Qty    int
	Price  Money
	Buyer  Owner
	Seller Owner
}

// maxTrades is how many recent trades the world keeps for display.
const maxTrades = 64

// before reports whether a sits ahead of b on its side of a book.
func (a *Order) before(b *Order) bool {
	if a.Price != b.Price {
		if a.Side == Bid {
			return a.Price > b.Price
		}
		return a.Price < b.Price
	}
	return a.ID < b.ID
}

// crosses reports whether incoming would trade against resting.
func crosses(incoming, resting *Order) bool {
	if incoming.Side == Bid {
		return incoming.Price >= resting.Price
	}
	return incoming.Price <= resting.Price
}

// post places a limit order and matches it at once, returning the order (which
// may already be filled and closed) and how many units traded. It returns nil
// when the order cannot be placed: no depot there, nothing to trade, or the
// actor cannot fund it — a bid it cannot pay for in full, or an ask for goods
// it does not hold at that depot. ttl is how many ticks the rest of the order
// lives; 0 means it never expires.
func (w *World) post(side Side, item ItemKind, qty int, price Money, actor Owner, depot Point, ttl int) (*Order, int) {
	c := w.storageContainers[depot]
	if c == nil || qty <= 0 || price < 0 || item == ItemNone {
		return nil, 0
	}
	w.nextOrderID++
	o := &Order{ID: w.nextOrderID, Side: side, Item: item, Qty: qty, Price: price,
		Actor: actor, Depot: depot, Posted: w.tick}
	if ttl > 0 {
		o.Expires = w.tick + ttl
	}
	w.orders[o.ID] = o
	switch side {
	case Bid:
		if !w.transfer(actor, o.owner(), Money(qty)*price) {
			delete(w.orders, o.ID)
			return nil, 0
		}
	case Ask:
		if !c.moveLine(actor, o.owner(), item, qty) {
			delete(w.orders, o.ID)
			return nil, 0
		}
	}

	key := bookKey{item, depot}
	b := w.books[key]
	if b == nil {
		b = &book{}
		w.books[key] = b
	}
	filled := 0
	for o.Qty > 0 {
		opposite := &b.asks
		if side == Ask {
			opposite = &b.bids
		}
		if len(*opposite) == 0 || !crosses(o, (*opposite)[0]) {
			break
		}
		resting := (*opposite)[0]
		n := min(o.Qty, resting.Qty)
		if side == Bid {
			w.settle(b, c, o, resting, n, resting.Price)
		} else {
			w.settle(b, c, resting, o, n, resting.Price)
		}
		filled += n
		if resting.Qty == 0 {
			*opposite = (*opposite)[1:]
			w.closeOrder(resting)
		}
	}
	if o.Qty == 0 {
		w.closeOrder(o)
		return o, filled
	}
	side_ := &b.bids
	if side == Ask {
		side_ = &b.asks
	}
	i := sort.Search(len(*side_), func(i int) bool { return o.before((*side_)[i]) })
	*side_ = append(*side_, nil)
	copy((*side_)[i+1:], (*side_)[i:])
	(*side_)[i] = o
	return o, filled
}

// settle trades n units between a bid and an ask at price: the goods move from
// the ask's escrow line to the buyer, and the money from the bid's escrow to
// the seller. A bid that filled below its limit gets the difference back, so a
// bid's escrow is always exactly Qty × Price.
func (w *World) settle(b *book, c *StorageContainer, bid, ask *Order, n int, price Money) {
	c.moveLine(ask.owner(), bid.Actor, ask.Item, n)
	w.transfer(bid.owner(), ask.Actor, Money(n)*price)
	bid.Qty -= n
	ask.Qty -= n
	if over := w.balance(bid.owner()) - Money(bid.Qty)*bid.Price; over > 0 {
		w.transfer(bid.owner(), bid.Actor, over)
	}
	b.last, b.traded = price, true
	b.volume += n
	w.recordPrice(ask.Item, price)
	w.trades = append(w.trades, Trade{Tick: w.tick, Item: ask.Item, Depot: ask.Depot, Qty: n,
		Price: price, Buyer: bid.Actor, Seller: ask.Actor})
	if over := len(w.trades) - maxTrades; over > 0 {
		w.trades = w.trades[over:]
	}
}

// closeOrder returns whatever an order still escrows to its actor and forgets
// it. It does not take the order out of its book; callers do that.
func (w *World) closeOrder(o *Order) {
	switch o.Side {
	case Bid:
		if left := w.balance(o.owner()); left > 0 && !w.transfer(o.owner(), o.Actor, left) {
			// The bidder's account is gone (a dead colonist): the money
			// freezes with the dead, keeping the audit exact.
			w.moneyFrozen += left
			o.escrow = 0
		}
	case Ask:
		if c := w.storageContainers[o.Depot]; c != nil && o.Qty > 0 {
			c.moveLine(o.owner(), o.Actor, o.Item, o.Qty)
		}
	}
	o.Qty = 0
	delete(w.orders, o.ID)
}

// cancel takes an open order out of its book and returns its escrow.
func (w *World) cancel(o *Order) {
	if b := w.books[bookKey{o.Item, o.Depot}]; b != nil {
		side := &b.bids
		if o.Side == Ask {
			side = &b.asks
		}
		for i, x := range *side {
			if x == o {
				*side = append((*side)[:i], (*side)[i+1:]...)
				break
			}
		}
	}
	w.closeOrder(o)
}

// sortedOrders returns the open orders passing keep, oldest first.
func (w *World) sortedOrders(keep func(*Order) bool) []*Order {
	var out []*Order
	for _, o := range w.orders {
		if keep == nil || keep(o) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// cancelOrdersOf cancels every open order an owner has, oldest first. Death
// calls it before the dead colonist's wallet freezes, so a bid's escrow goes
// back to the wallet and freezes with it.
func (w *World) cancelOrdersOf(actor Owner) {
	for _, o := range w.sortedOrders(func(o *Order) bool { return o.Actor == actor }) {
		w.cancel(o)
	}
}

// expireOrders cancels every order whose time is up, oldest first.
func (w *World) expireOrders() {
	for _, o := range w.sortedOrders(func(o *Order) bool { return o.Expires > 0 && o.Expires <= w.tick }) {
		w.cancel(o)
	}
}

// moneyEscrowed is every dollar held in escrow: by open bids, and by open work
// orders (see workorder.go).
func (w *World) moneyEscrowed() Money {
	total := w.workEscrowed()
	for _, o := range w.orders {
		total += o.escrow
	}
	return total
}

// bestAsk and bestBid return the head of a book's side, if any.
func (w *World) bestAsk(item ItemKind, depot Point) (*Order, bool) {
	if b := w.books[bookKey{item, depot}]; b != nil && len(b.asks) > 0 {
		return b.asks[0], true
	}
	return nil, false
}

func (w *World) bestBid(item ItemKind, depot Point) (*Order, bool) {
	if b := w.books[bookKey{item, depot}]; b != nil && len(b.bids) > 0 {
		return b.bids[0], true
	}
	return nil, false
}

// openQty is how many units actor has open on one side of one book.
func (w *World) openQty(side Side, item ItemKind, depot Point, actor Owner) int {
	b := w.books[bookKey{item, depot}]
	if b == nil {
		return 0
	}
	orders := b.bids
	if side == Ask {
		orders = b.asks
	}
	n := 0
	for _, o := range orders {
		if o.Actor == actor {
			n += o.Qty
		}
	}
	return n
}

// ---- The silo -----------------------------------------------------------------

// marketDepot is where the colony trades: its communal chest nearest the map
// center, ties by position. It is recomputed only when a fixture has changed
// (fixtureRev), since a colony with a locker per settler has many containers.
func (w *World) marketDepot() (Point, bool) {
	if w.marketDepotRev == w.fixtureRev+1 {
		return w.marketDepotAt, w.marketDepotOK
	}
	center := Point{w.Width / 2, w.Height / 2}
	var best Point
	bestDist, found := 1<<30, false
	for p, c := range w.storageContainers {
		if c.Terrain != Storage || !w.communalFixture(p) {
			continue
		}
		d := center.Chebyshev(p)
		if !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	w.marketDepotAt, w.marketDepotOK, w.marketDepotRev = best, found, w.fixtureRev+1
	return best, found
}

// refPrice is the colony charter's reference price for an item: what the
// colony bids for ore and what a colonist asks for a meal. Zero means the item
// is not traded at a reference price.
func (w *World) refPrice(k ItemKind) Money {
	switch k {
	case Meal:
		return Money(w.cfg.PriceMeal)
	case RawRock:
		return Money(w.cfg.PriceRawRock)
	case IronOre:
		return Money(w.cfg.PriceIronOre)
	case WaterIce:
		return Money(w.cfg.PriceWaterIce)
	case UraniumOre:
		return Money(w.cfg.PriceUraniumOre)
	case Clay:
		return Money(w.cfg.PriceClay)
	default:
		return 0
	}
}

// prospectingGoods are what the colony keeps standing bids for at the silo.
var prospectingGoods = [...]ItemKind{RawRock, IronOre, WaterIce, UraniumOre, Clay}

// marketInterval is how often the market's upkeep runs: expiring orders and
// topping up the colony's standing bids.
const marketInterval = 10

// runMarket is the market's periodic upkeep.
func (w *World) runMarket() {
	if w.tick%marketInterval != 0 {
		return
	}
	w.expireOrders()
	w.prunePlans()
	w.refreshColonyBids()
	w.refreshColonyAsks()
	w.refreshBiomatterBids()
	w.refreshSiloStock()
	w.refreshColonyMealAsks()
}

// refreshColonyBids keeps the colony's standing bids for ore at the silo
// topped up to silo-bid-qty units each, at the reference price, as far as the
// treasury will stretch. This is paid prospecting: anyone who digs ore can
// sell it here.
func (w *World) refreshColonyBids() {
	silo, ok := w.marketDepot()
	if !ok {
		return
	}
	for _, k := range prospectingGoods {
		price := w.refPrice(k)
		if price <= 0 {
			continue
		}
		want := w.cfg.SiloBidQty - w.openQty(Bid, k, silo, Community)
		if afford := int(w.treasury / price); want > afford {
			want = afford
		}
		if want > 0 {
			w.post(Bid, k, want, price, Community, silo, 0)
		}
	}
}

// sellableStacks is what e carries that sells at the silo: general materials
// with a reference price.
func (w *World) sellableStacks(e *Entity) []ItemStack {
	var out []ItemStack
	for _, s := range e.Inventory.storableStacks() {
		if w.refPrice(s.Kind) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// sellAtMarket offers everything e holds of the sellable goods it just put in
// the depot at p, at reference prices. It trades at once against any bid (the
// colony's, for ore); the rest rests for order-ttl ticks.
func (w *World) sellAtMarket(e *Entity, p Point, kinds []ItemKind) {
	c := w.storageContainers[p]
	if c == nil {
		return
	}
	me := ColonistOwner(e.ID)
	for _, k := range kinds {
		price := w.refPrice(k)
		if n := c.held(me, k); n > 0 && price > 0 {
			w.post(Ask, k, n, price, me, p, w.cfg.OrderTTL)
		}
	}
}

// tryBuyMeal buys a hungry colonist one meal, if one is on offer at a price
// it will pay (mealBidLimit) at a depot it can reach and use: the silo, or a
// scumhouse selling what it cooks. Cheapest wins, then nearest, then
// position. The meal is then its own, where it bought it, and the ordinary
// eating job fetches it. If nothing fills, a bid rests for demand-ttl ticks —
// at most one per colonist — at the nearest scumhouse it can reach, where
// meals are made, or else at the silo. That makes it a queue: the colony
// offers each meal the moment it is cooked (offerColonyMeals), and a resting
// bid there takes it at once, in bid order, rather than whoever happens to
// ask next. It is also a standing sign that someone wants a meal, which is
// what a producer's planner answers (see producer.go). A later fill leaves
// the meal where the bid was, in its name.
func (w *World) tryBuyMeal(e *Entity) bool {
	me := ColonistOwner(e.ID)
	limit := w.mealBidLimit(e)
	if limit <= 0 {
		return false
	}
	room := w.roomOf(e.Pos)
	silo, hasSilo := w.marketDepot()
	depots := w.scumhousesSorted()
	if hasSilo {
		depots = append(depots, silo)
	}
	var best *Order
	for _, p := range depots {
		ask, ok := w.bestAsk(Meal, p)
		if !ok || ask.Actor == me || ask.Price > limit || !w.canUseFixture(e, p) || !w.taskReachable(p, room) {
			continue
		}
		if best == nil || ask.Price < best.Price || (ask.Price == best.Price &&
			(e.Pos.Chebyshev(p) < e.Pos.Chebyshev(best.Depot) ||
				(e.Pos.Chebyshev(p) == e.Pos.Chebyshev(best.Depot) && lessPoint(p, best.Depot)))) {
			best = ask
		}
	}
	if best != nil {
		price, at := best.Price, best.Depot
		o, filled := w.post(Bid, Meal, 1, price, me, at, 0)
		if o != nil && o.Qty > 0 {
			w.cancel(o) // buy now or not at all
		}
		if filled > 0 {
			w.remember(e, event(EvtBoughtMeal, "Bought a meal for %v.", price))
			return true
		}
	}
	if w.cfg.DemandTTL <= 0 || w.hasOpenMealBid(me) {
		return false
	}
	queue, ok := w.nearestScumhouse(e, nil)
	if !ok && hasSilo && w.canUseFixture(e, silo) && w.taskReachable(silo, room) {
		queue, ok = silo, true
	}
	if ok {
		w.post(Bid, Meal, 1, limit, me, queue, w.cfg.DemandTTL)
	}
	return false
}

// ---- Taking goods to market -------------------------------------------------

// sellStage is where a JobSell colonist is.
type sellStage uint8

const (
	sellFetch   sellStage = iota // taking the surplus out of its own depot at Target
	sellDeliver                  // carrying it to the silo at Target
)

// surplusMeals is how many of its own meals e holds beyond meal-keep, across
// every depot but the silo (meals already there are for sale or bought).
func (w *World) surplusMeals(e *Entity, silo Point) int {
	n := e.Inventory.Count(Meal)
	for p, c := range w.storageContainers {
		if p != silo {
			n += c.held(ColonistOwner(e.ID), Meal)
		}
	}
	return n - w.cfg.MealKeep
}

// tryAssignSellMeals sends e to take its surplus meals to the silo and offer
// them at the reference price: out of its locker, into its pockets, onto the
// silo's ledger in its own name, and into the book.
func (w *World) tryAssignSellMeals(e *Entity) bool {
	silo, ok := w.marketDepot()
	if !ok || w.refPrice(Meal) <= 0 || !w.canUseFixture(e, silo) {
		return false
	}
	room := w.roomOf(e.Pos)
	if !w.taskReachable(silo, room) || w.surplusMeals(e, silo) <= 0 {
		return false
	}
	if e.Inventory.Has(Meal) {
		e.Job, e.Target, e.sell, e.Progress = JobSell, silo, sellDeliver, 0
		return true
	}
	me := ColonistOwner(e.ID)
	var best Point
	bestDist, found := 1<<30, false
	for p, c := range w.storageContainers {
		if p == silo || c.held(me, Meal) == 0 || !w.canUseFixture(e, p) || !w.taskReachable(p, room) {
			continue
		}
		if d := e.Pos.Chebyshev(p); !found || d < bestDist || (d == bestDist && lessPoint(p, best)) {
			best, bestDist, found = p, d, true
		}
	}
	if !found {
		return false
	}
	e.Job, e.Target, e.sell, e.Progress = JobSell, best, sellFetch, 0
	return true
}

// jobSell runs one tick of taking meals to market.
func (w *World) jobSell(e *Entity) {
	silo, ok := w.marketDepot()
	if !ok {
		w.clearJob(e)
		return
	}
	arrived, reachable := w.travelTo(e, e.Target)
	if !reachable {
		w.clearJob(e)
		return
	}
	if !arrived {
		e.State = Hauling
		return
	}
	c := w.storageContainers[e.Target]
	me := ColonistOwner(e.ID)
	if c == nil {
		w.clearJob(e)
		return
	}
	if e.sell == sellFetch {
		n := min(w.surplusMeals(e, silo), c.held(me, Meal))
		for n > 0 && !e.Inventory.CanAdd(Meal, n) {
			n--
		}
		if n <= 0 || !c.debit(me, Meal, n) {
			w.clearJob(e)
			return
		}
		e.Inventory.Add(Meal, n)
		e.Target, e.sell = silo, sellDeliver
		return
	}
	n := e.Inventory.Count(Meal)
	if n == 0 || !c.Inventory.Add(Meal, n) {
		w.clearJob(e)
		return
	}
	c.credit(me, Meal, n)
	e.Inventory.RemoveAll(Meal)
	w.sellAtMarket(e, silo, []ItemKind{Meal})
	w.remember(e, event(EvtWentToMarket, "Took %d meals to market.", n))
	w.clearJob(e)
}

// hasOpenMealBid reports whether actor already has a meal bid resting
// anywhere: a hungry colonist queues at one depot at a time.
func (w *World) hasOpenMealBid(actor Owner) bool {
	for _, o := range w.orders {
		if o.Side == Bid && o.Item == Meal && o.Actor == actor {
			return true
		}
	}
	return false
}

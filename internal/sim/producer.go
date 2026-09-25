package sim

import (
	"fmt"
	"sort"
)

// ---- The producer planner ---------------------------------------------------------
//
// A colonist looking for work also looks at the market: is there a bid it
// could fill at a profit? The planner considers the few best open bids it
// could reach and, one recipe level deep, reckons
//
//	profit = bid price × units − input cost − labor ticks × its own labor price
//
// where an input it already owns costs what it is worth, one on offer at the
// workshop costs its ask, and one nobody offers is bid for: a derived bid, at
// the most the colonist can pay and still clear plan-min-profit. That bid is
// an ordinary bid, and the next producer's planner sees it. So demand reaches
// one link further down the recipe graph each round, and nobody plans a whole
// chain: a hungry colonist's bid for a meal becomes a cook's bid for scum,
// which a scraper fills by scraping it off the cave wall.
//
// A plan is tied to the bid it serves. Its derived bids are withdrawn when it
// is done, dropped, or out of time, so abandoned demand never outlives the
// plan that made it. See docs/valuation.md.

// planID identifies a production plan; IDs come from a counter.
type planID uint64

// planKind is what a plan does to fill its bid.
type planKind uint8

const (
	planGather planKind = iota // scrape scum and sell it into the bid
	planCraft                  // work a recipe and deliver the output to the bid
	planHaul                   // buy at one depot, carry, sell into the bid at another
)

func (k planKind) String() string {
	switch k {
	case planGather:
		return "gather"
	case planCraft:
		return "craft"
	default:
		return "haul"
	}
}

// plan is one colonist's scheme to fill one bid.
type plan struct {
	id       planID
	kind     planKind
	actor    EntityID
	target   OrderID  // the bid it serves
	item     ItemKind // what that bid wants
	qty      int      // units it means to deliver
	price    Money    // what it will ask for them: the bid's price
	depot    Point    // where the bid is
	workshop Point    // planCraft: where it cooks
	recipe   int      // planCraft: an index into recipes
	crafted  bool     // planCraft: the output exists
	derived  []OrderID
	depth    int // links below a finished-good bid: 1 serves one directly
	expires  int
}

// producible reports whether some plan can make k: it is scraped from the
// cave wall, or some recipe outputs it.
func producible(k ItemKind) bool {
	if k == CaveScum {
		return true
	}
	for _, r := range recipes {
		for _, out := range r.Outputs {
			if out.Kind == k {
				return true
			}
		}
	}
	return false
}

// candidateBids is every open bid, best price first, then oldest: a plan
// might make what it wants, or carry it in from a depot where it is cheaper. It is memoized for the tick: every colonist choosing
// work reads it, and the book changes far less often than that.
func (w *World) candidateBids() []*Order {
	if w.candidatesTick == w.tick {
		return w.candidatesCache
	}
	out := w.candidatesCache[:0]
	for _, o := range w.orders {
		if o.Side == Bid && o.Qty > 0 {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Price != out[j].Price {
			return out[i].Price > out[j].Price
		}
		return out[i].ID < out[j].ID
	})
	w.candidatesTick, w.candidatesCache = w.tick, out
	return out
}

// plannedQty is how many units of bid b open plans already mean to deliver.
func (w *World) plannedQty(b *Order) int {
	n := 0
	for _, p := range w.plans {
		if p.target == b.ID {
			n += p.qty
		}
	}
	return n
}

// tryAssignProduce advances e's plan, or looks for a bid worth a new one. It
// reports whether e has a job to do now; a plan waiting on its derived bids
// leaves e free for other work.
func (w *World) tryAssignProduce(e *Entity) bool {
	if e.plan != 0 {
		if p := w.plans[e.plan]; p != nil {
			return w.advancePlan(e, p)
		}
		e.plan = 0
	}
	if w.cfg.PlanCandidates <= 0 {
		return false
	}
	me := ColonistOwner(e.ID)
	room := w.roomOf(e.Pos)
	considered := 0
	for _, b := range w.candidateBids() {
		if considered >= w.cfg.PlanCandidates {
			break
		}
		if b.Actor == me || w.orders[b.ID] != b || b.Qty-w.plannedQty(b) <= 0 {
			continue
		}
		if !w.canUseFixture(e, b.Depot) || !w.taskReachable(b.Depot, room) {
			continue
		}
		ask, src, cheaper := w.cheapestAskElsewhere(e, b)
		if !cheaper && !producible(b.Item) {
			continue // nothing to make it from and nowhere cheaper to fetch it
		}
		considered++
		if cheaper && w.planArbitrage(e, b, ask, src) {
			return true
		}
		if b.Item == CaveScum {
			if w.planGather(e, b) {
				return true
			}
			continue
		}
		if started, planned := w.planCraft(e, b); planned {
			return started
		}
	}
	return false
}

// newPlan registers a plan serving bid b.
func (w *World) newPlan(e *Entity, kind planKind, b *Order, qty int) *plan {
	w.nextPlanID++
	p := &plan{id: w.nextPlanID, kind: kind, actor: e.ID, target: b.ID, item: b.Item,
		qty: qty, price: b.Price, depot: b.Depot, depth: b.depth + 1,
		expires: w.tick + max(1, w.cfg.PlanTTL)}
	w.plans[p.id] = p
	e.plan = p.id
	return p
}

// dropPlan withdraws a plan's derived bids and forgets it.
func (w *World) dropPlan(p *plan) {
	for _, id := range p.derived {
		if o := w.orders[id]; o != nil {
			w.cancel(o)
		}
	}
	delete(w.plans, p.id)
	if e := w.entities[p.actor]; e != nil && e.plan == p.id {
		e.plan = 0
	}
}

// prunePlans drops every plan whose colonist is gone or whose time is up, and
// every plan still waiting on inputs whose bid is gone — nobody wants the
// output any more, so nobody should be asked for the inputs. Oldest first.
func (w *World) prunePlans() {
	ids := make([]planID, 0, len(w.plans))
	for id := range w.plans {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		p := w.plans[id]
		e := w.entities[p.actor]
		waiting := p.kind == planCraft && !p.crafted
		if e == nil || !e.Alive() || w.tick >= p.expires || (waiting && w.orders[p.target] == nil) {
			w.dropPlan(p)
		}
	}
}

// chainDepth is the deepest open plan: how many links below a finished-good
// bid demand has reached.
func (w *World) chainDepth() int {
	d := 0
	for _, p := range w.plans {
		d = max(d, p.depth)
	}
	return d
}

// ---- Gathering -----------------------------------------------------------------------

// planGather takes on scraping scum on e's own account to sell into bid b at a
// scumhouse, if it pays.
func (w *World) planGather(e *Entity, b *Order) bool {
	c := w.storageContainers[b.Depot]
	if c == nil || c.Terrain != Scumhouse || e.Inventory.Has(CaveScum) {
		return false
	}
	qty := min(b.Qty-w.plannedQty(b), w.scrapeLoad())
	for qty > 0 && !e.Inventory.CanAdd(CaveScum, qty) {
		qty--
	}
	if qty <= 0 || !c.Inventory.CanAdd(CaveScum, qty) {
		return false
	}
	patch, ok := w.nearestScum(e)
	if !ok {
		return false
	}
	ticks := qty*w.cfg.ScrapeTicks + e.Pos.Chebyshev(patch) + patch.Chebyshev(b.Depot)
	if b.Price*Money(qty)-w.laborCost(ticks) < Money(w.cfg.PlanMinProfit) {
		return false
	}
	p := w.newPlan(e, planGather, b, qty)
	w.scumClaims[patch] = e.ID
	e.Job, e.Target, e.scrape, e.Progress = JobScrape, patch, scrapeGather, 0
	e.scrapeFor, e.scrapeQty = ColonistOwner(e.ID), p.qty
	return true
}

// sellGathered offers what a gather plan brought to its scumhouse into the
// bid it was for, and ends the plan.
func (w *World) sellGathered(e *Entity, p *plan) {
	me := ColonistOwner(e.ID)
	if n := min(p.qty, w.storageContainers[p.depot].held(me, p.item)); n > 0 {
		w.post(Ask, p.item, n, p.price, me, p.depot, w.cfg.OrderTTL)
		w.remember(e, event(EvtWentToMarket, "Scraped %d %s to sell for %v each.", n, p.item, p.price))
	}
	w.dropPlan(p)
}

// ---- Crafting ------------------------------------------------------------------------

// planCraft looks for a recipe whose output fills bid b at a profit. planned
// reports whether it took one on; started, whether e has a job now (false
// while the plan waits on its derived bids).
func (w *World) planCraft(e *Entity, b *Order) (started, planned bool) {
	me := ColonistOwner(e.ID)
	for ri, r := range recipes {
		out := 0
		for _, o := range r.Outputs {
			if o.Kind == b.Item {
				out += o.Count
			}
		}
		if out == 0 {
			continue
		}
		house, ok := w.nearestScumhouse(e, func(c *StorageContainer) bool {
			id := w.workshopClaims[c.Pos]
			return c.Terrain == r.Facility && (id == 0 || id == e.ID)
		})
		if !ok {
			continue
		}
		c := w.storageContainers[house]
		qty := min(out, b.Qty-w.plannedQty(b))
		revenue := b.Price * Money(qty)
		spent := w.laborCost(r.Ticks + e.Pos.Chebyshev(house) + house.Chebyshev(b.Depot))
		type buy struct {
			ask *Order
			n   int
		}
		var buys []buy
		var missing []ItemStack
		feasible := true
		for _, in := range r.Inputs {
			have := min(c.held(me, in.Kind), in.Count)
			spent += Money(have) * w.valueOf(in.Kind)
			need := in.Count - have
			if need == 0 {
				continue
			}
			if ask, ok := w.bestAsk(in.Kind, house); ok && ask.Actor != me && ask.Qty >= need {
				buys = append(buys, buy{ask, need})
				spent += Money(need) * ask.Price
				continue
			}
			if !producible(in.Kind) {
				feasible = false
				break
			}
			missing = append(missing, ItemStack{in.Kind, need})
		}
		if !feasible {
			continue
		}
		margin := revenue - spent - Money(w.cfg.PlanMinProfit)
		units := 0
		for _, m := range missing {
			units += m.Count
		}
		var unit Money
		if units > 0 {
			unit = margin / Money(units)
			if unit < 1 {
				continue
			}
		} else if margin < 0 {
			continue
		}
		var cash Money
		for _, x := range buys {
			cash += Money(x.n) * x.ask.Price
		}
		if e.wallet < cash+unit*Money(units) {
			continue
		}
		p := w.newPlan(e, planCraft, b, qty)
		p.workshop, p.recipe = house, ri
		for _, x := range buys {
			o, _ := w.post(Bid, x.ask.Item, x.n, x.ask.Price, me, house, 0)
			if o != nil && o.Qty > 0 {
				w.cancel(o) // somebody took the ask first; the derived bid covers it below
				missing = append(missing, ItemStack{x.ask.Item, o.Qty})
			}
		}
		for _, m := range missing {
			if unit < 1 {
				unit = max(1, w.valueOf(m.Kind))
			}
			if o, _ := w.post(Bid, m.Kind, m.Count, unit, me, house, max(1, w.cfg.PlanTTL)); o != nil && o.Qty > 0 {
				o.plan, o.depth = p.id, p.depth
				p.derived = append(p.derived, o.ID)
			}
		}
		if len(p.derived) > 0 {
			w.remember(e, event(EvtWentToMarket, "Bid for %s to make %s for a customer.", stackPhrase(missing), b.Item))
		}
		return w.advancePlan(e, p), true
	}
	return false, false
}

// advancePlan gives e the next job its plan needs, reporting whether there is
// one: deliver a gathered load, cook once the inputs are in, or carry the
// output to the bid.
func (w *World) advancePlan(e *Entity, p *plan) bool {
	me := ColonistOwner(e.ID)
	switch p.kind {
	case planGather:
		if e.Inventory.Count(CaveScum) > 0 {
			e.Job, e.Target, e.scrape, e.Progress = JobScrape, p.depot, scrapeHaul, 0
			e.scrapeFor, e.scrapeQty = me, p.qty
			e.cargo[CaveScum] = me
			return true
		}
		w.dropPlan(p) // the scrape was abandoned before it gathered anything
		return false
	case planCraft, planHaul:
		c := w.storageContainers[p.workshop]
		if c == nil {
			w.dropPlan(p)
			return false
		}
		if e.Inventory.Count(p.item) > 0 {
			w.assignCarry(e, p, p.depot, carryDeliver, min(p.qty, e.Inventory.Count(p.item)))
			return true
		}
		if p.crafted {
			n := min(p.qty, c.held(me, p.item))
			if n <= 0 {
				w.dropPlan(p)
				return false
			}
			w.assignCarry(e, p, p.workshop, carryFetch, n)
			return true
		}
		r := recipes[p.recipe]
		if id := w.workshopClaims[p.workshop]; (id != 0 && id != e.ID) || !craftable(c, r, me) {
			return false // waiting on inputs, or on the cook ahead of it
		}
		w.workshopClaims[p.workshop] = e.ID
		e.Job, e.Target, e.Progress = JobCraft, p.workshop, 0
		e.recipe, e.craftFor = p.recipe, me
		return true
	}
	return false
}

// ---- Carrying goods to a buyer -------------------------------------------------------

// carryStage is where a JobCarry colonist is.
type carryStage uint8

const (
	carryFetch   carryStage = iota // taking its goods out of the depot at Target
	carryDeliver                   // carrying them to the depot at Target
)

// assignCarry sends e to move qty of its plan's item, its own, to the plan's
// depot and ask the plan's price there.
func (w *World) assignCarry(e *Entity, p *plan, target Point, stage carryStage, qty int) {
	e.Job, e.Target, e.Progress = JobCarry, target, 0
	e.carry, e.carryItem, e.carryQty, e.carryPrice, e.carryTo = stage, p.item, qty, p.price, p.depot
	e.carryFor, e.carryWork = Owner{}, 0
}

// jobCarry runs one tick of carrying goods to a buyer: fetch them out of a
// depot on the colonist's own ledger line, walk them to the buyer's depot, put
// them in there in its own name, and ask the price.
func (w *World) jobCarry(e *Entity) {
	arrived, ok := w.travelTo(e, e.Target)
	if !ok {
		w.clearJob(e)
		return
	}
	if !arrived {
		e.State = Hauling
		return
	}
	c := w.storageContainers[e.Target]
	owner := ColonistOwner(e.ID)
	if e.carryFor.Kind != OwnerNone {
		owner = e.carryFor // hauling someone else's goods for hire
	}
	if c == nil {
		w.clearJob(e)
		return
	}
	if e.carry == carryFetch {
		n := min(e.carryQty, c.held(owner, e.carryItem))
		for n > 0 && !e.Inventory.CanAdd(e.carryItem, n) {
			n--
		}
		if n <= 0 || !c.debit(owner, e.carryItem, n) {
			w.clearJob(e)
			return
		}
		e.Inventory.Add(e.carryItem, n)
		e.cargo[e.carryItem] = e.carryFor
		e.carryQty, e.Target, e.carry = n, e.carryTo, carryDeliver
		return
	}
	n := min(e.carryQty, e.Inventory.Count(e.carryItem))
	if n <= 0 || !c.Inventory.Add(e.carryItem, n) {
		w.clearJob(e)
		return
	}
	e.Inventory.Remove(e.carryItem, n)
	e.cargo[e.carryItem] = Owner{}
	c.credit(owner, e.carryItem, n)
	if e.carryWork != 0 {
		w.finishHaul(e, n)
		return
	}
	me := owner
	_, filled := w.post(Ask, e.carryItem, n, e.carryPrice, me, e.Target, w.cfg.OrderTTL)
	w.remember(e, event(EvtWentToMarket, "Delivered %d %s to market (%d sold at once).", n, e.carryItem, filled))
	if p := w.plans[e.plan]; p != nil {
		w.dropPlan(p)
	}
	w.clearJob(e)
}

// planSummary renders an open plan for the market view.
func (p *plan) summary() string {
	return fmt.Sprintf("%s %d %s for %v at (%d, %d)", p.kind, p.qty, p.item, p.price, p.depot.X, p.depot.Y)
}

package sim

import (
	"fmt"
	"sort"
)

// ---- Colony-wide orders ----------------------------------------------------------
//
// A colony-wide order is a standing order the player sets for the colony by
// side and item, with no depot: "buy 100 meals at $10", "sell meals at $5".
// The market's upkeep places it, every marketInterval ticks, as ordinary
// orders on the books where the good changes hands, and keeps them topped
// up as they fill:
//
//   - a bid keeps Qty units bid for in all, split across the depots where the
//     good is delivered: a kitchen's pantry for meals, its stove for
//     biomatter, the silo for anything else;
//   - an ask keeps up to Qty units on offer in all, of whatever the colony
//     holds, wherever it holds it.
//
// It replaces the colony's own standing orders for that side and item while
// it is set. See docs/colony-orders.md.

// SetColonyWideOrder sets (or replaces) the colony's colony-wide order for a
// side and item. The orders it had for them come off the book and go back at
// the new terms at once.
type SetColonyWideOrder struct {
	Side  Side
	Item  ItemKind
	Qty   int
	Price Money
}

// ClearColonyWideOrder drops the colony's colony-wide order for a side and
// item and takes its orders off the book.
type ClearColonyWideOrder struct {
	Side Side
	Item ItemKind
}

func (SetColonyWideOrder) isCommand()   {}
func (ClearColonyWideOrder) isCommand() {}

// wideOrder is one colony-wide order's terms; Qty 0 means none is set.
type wideOrder struct {
	Qty   int
	Price Money
}

// wideSet reports whether a colony-wide order stands for side and item.
func (w *World) wideSet(side Side, item ItemKind) bool {
	return side <= Ask && item < numItemKinds && w.wide[side][item].Qty > 0
}

// setColonyWideOrder applies SetColonyWideOrder and reports whether it did.
func (w *World) setColonyWideOrder(c SetColonyWideOrder) bool {
	verb := "buy"
	if c.Side == Ask {
		verb = "sell"
	}
	refuse := func(why string) bool {
		w.logEvent(LogNote, fmt.Sprintf("The colony cannot %s %s colony-wide: %s.", verb, c.Item, why))
		return false
	}
	switch {
	case c.Side > Ask:
		return false
	case !c.Item.Tradable():
		return refuse("that is not something it trades")
	case c.Qty <= 0 || c.Qty > maxColonyOrderQty:
		return refuse(fmt.Sprintf("an order is 1 to %d units", maxColonyOrderQty))
	case c.Price <= 0:
		return refuse("the price must be at least $1")
	}
	w.withdrawWide(c.Side, c.Item, func(*Order) bool { return true })
	if !w.wideSet(c.Side, c.Item) {
		// The colony's own standing orders for it give way to the player's.
		for _, o := range w.sortedOrders(func(o *Order) bool {
			return o.Actor == Community && !o.manual && !o.wide && o.Side == c.Side && o.Item == c.Item
		}) {
			w.cancel(o)
		}
	}
	w.wide[c.Side][c.Item] = wideOrder{Qty: c.Qty, Price: c.Price}
	filled := w.refreshWide(c.Side, c.Item)
	what := fmt.Sprintf("keeps up to %d %s on offer at %v each, wherever it holds them", c.Qty, c.Item, c.Price)
	if c.Side == Bid {
		what = fmt.Sprintf("keeps a bid for %d %s at %v each where they are delivered", c.Qty, c.Item, c.Price)
	}
	note := ""
	if filled > 0 {
		note = fmt.Sprintf("; %d traded at once", filled)
	}
	w.logEvent(LogNote, fmt.Sprintf("The colony %s%s.", what, note))
	return true
}

// clearColonyWideOrder applies ClearColonyWideOrder and reports whether there
// was one.
func (w *World) clearColonyWideOrder(c ClearColonyWideOrder) bool {
	if !w.wideSet(c.Side, c.Item) {
		return false
	}
	w.wide[c.Side][c.Item] = wideOrder{}
	w.withdrawWide(c.Side, c.Item, func(*Order) bool { return true })
	verb := "buying"
	if c.Side == Ask {
		verb = "selling"
	}
	w.logEvent(LogNote, fmt.Sprintf("The colony stops %s %s colony-wide.", verb, c.Item))
	return true
}

// withdrawWide cancels the open orders a colony-wide order placed for side
// and item that drop says to.
func (w *World) withdrawWide(side Side, item ItemKind, drop func(*Order) bool) {
	for _, o := range w.sortedOrders(func(o *Order) bool {
		return o.wide && o.Side == side && o.Item == item && drop(o)
	}) {
		w.cancel(o)
	}
}

// refreshWideOrders is the upkeep for every colony-wide order, bids first,
// then in item order.
func (w *World) refreshWideOrders() {
	for _, side := range [...]Side{Bid, Ask} {
		for k := ItemNone + 1; k < numItemKinds; k++ {
			if w.wideSet(side, k) {
				w.refreshWide(side, k)
			}
		}
	}
}

// refreshWide tops up one colony-wide order and returns how many units
// traded as it did.
func (w *World) refreshWide(side Side, item ItemKind) int {
	if side == Bid {
		return w.refreshWideBid(item)
	}
	return w.refreshWideAsk(item)
}

// wideBidDepots is where a colony-wide bid for item stands: where the good is
// delivered, so the colonists who make or carry it find the bid there. A
// meal's is every colony kitchen's pantry, where a cook's meals come out and a
// cook carries meals for a bid; biomatter's is every colony kitchen's stove,
// the only bids a gather plan scrapes for; anything else's is the silo.
func (w *World) wideBidDepots(item ItemKind) []Point {
	var out []Point
	switch {
	case item == Meal:
		for _, h := range w.colonyKitchens() {
			out = append(out, w.outputDepot(h))
		}
	case isBiomatter(item):
		out = w.colonyKitchens()
	}
	if len(out) == 0 {
		if silo, ok := w.marketDepot(); ok {
			out = append(out, silo)
		}
	}
	return out
}

// refreshWideBid keeps a colony-wide bid's quantity bid for, split evenly
// across its depots (the first ones take the remainder), as far as the
// treasury stretches. Bids at a depot it no longer uses come off the book.
func (w *World) refreshWideBid(item ItemKind) int {
	terms := w.wide[Bid][item]
	depots := w.wideBidDepots(item)
	at := make(map[Point]int, len(depots))
	for i, p := range depots {
		at[p] = i
	}
	w.withdrawWide(Bid, item, func(o *Order) bool {
		_, ok := at[o.Depot]
		return !ok || o.Price != terms.Price
	})
	filled := 0
	for i, p := range depots {
		share := terms.Qty / len(depots)
		if i < terms.Qty%len(depots) {
			share++
		}
		want := share - w.wideOpen(Bid, item, p)
		if afford := int(w.treasury / terms.Price); want > afford {
			want = afford
		}
		if want <= 0 {
			continue
		}
		o, n := w.post(Bid, item, want, terms.Price, Community, p, 0)
		if o != nil {
			o.wide = true
		}
		filled += n
	}
	return filled
}

// refreshWideAsk offers what the colony holds of item, up to the order's
// quantity on offer in all, at every communal depot holding some, in
// position order. Not an incubator's scum: that is its seed and its crop,
// and harvesting carries it to a kitchen, where it is offered.
func (w *World) refreshWideAsk(item ItemKind) int {
	terms := w.wide[Ask][item]
	w.withdrawWide(Ask, item, func(o *Order) bool { return o.Price != terms.Price })
	left := terms.Qty
	for _, o := range w.orders {
		if o.wide && o.Side == Ask && o.Item == item {
			left -= o.Qty
		}
	}
	var depots []Point
	for p, c := range w.storageContainers {
		if c.Terrain != Incubator && c.held(Community, item) > 0 && w.communalFixture(p) {
			depots = append(depots, p)
		}
	}
	sort.Slice(depots, func(i, j int) bool { return lessPoint(depots[i], depots[j]) })
	filled := 0
	for _, p := range depots {
		if left <= 0 {
			break
		}
		n := min(left, w.storageContainers[p].held(Community, item))
		o, f := w.post(Ask, item, n, terms.Price, Community, p, 0)
		if o == nil {
			continue
		}
		o.wide = true
		left -= n
		filled += f
	}
	return filled
}

// wideOpen is how many units colony-wide orders have open on one side of one
// book.
func (w *World) wideOpen(side Side, item ItemKind, depot Point) int {
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
		if o.wide {
			n += o.Qty
		}
	}
	return n
}

// WideOrderView is one colony-wide order: its terms, and how many units its
// orders have open on the book now, at how many depots.
type WideOrderView struct {
	Side   Side
	Item   ItemKind
	Qty    int
	Price  Money
	Open   int
	Depots int
}

// wideOrders lists the colony-wide orders, bids first, then in item order.
func (w *World) wideOrders() []WideOrderView {
	var out []WideOrderView
	for _, side := range [...]Side{Bid, Ask} {
		for k := ItemNone + 1; k < numItemKinds; k++ {
			if !w.wideSet(side, k) {
				continue
			}
			v := WideOrderView{Side: side, Item: k, Qty: w.wide[side][k].Qty, Price: w.wide[side][k].Price}
			seen := map[Point]bool{}
			for _, o := range w.orders {
				if o.wide && o.Side == side && o.Item == k {
					v.Open += o.Qty
					seen[o.Depot] = true
				}
			}
			v.Depots = len(seen)
			out = append(out, v)
		}
	}
	return out
}

// isBiomatter reports whether k is one of the goods a kitchen's stove takes.
func isBiomatter(k ItemKind) bool {
	for _, b := range biomatterKinds {
		if b == k {
			return true
		}
	}
	return false
}

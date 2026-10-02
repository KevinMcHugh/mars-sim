package sim

import "fmt"

// ---- The player's colony orders ----------------------------------------------------
//
// A player can trade on the colony's behalf: post a bid or an ask in the
// colony's name at any communal depot, reprice one of the colony's open
// orders, or take one off the book. The orders are ordinary orders — escrowed
// from the treasury or the colony's ledger line, matched at once, resting
// until they fill — marked manual, so the colony's own upkeep leaves them
// alone: it never withdraws a manual ask to haul or reprice its meals, and
// never cancels one when the silo moves. They never expire. See
// docs/colony-orders.md.

// PlaceColonyOrder posts an order in the colony's name: Qty units of Item at
// Price each, at the communal depot at Depot. A bid is paid for from the
// treasury, an ask from what the colony holds at that depot; either must be
// covered in full. The outcome is logged, since a command has no reply.
type PlaceColonyOrder struct {
	Side  Side
	Item  ItemKind
	Qty   int
	Price Money
	Depot Point
}

// RepriceColonyOrder moves one of the colony's open orders to a new price. It
// is a cancel and a re-post, so the order loses its place in the queue and
// may trade at once if the new price crosses the book.
type RepriceColonyOrder struct {
	ID    OrderID
	Price Money
}

// CancelColonyOrder takes one of the colony's open orders off the book and
// returns its escrow. A standing order the colony keeps topped up (its bids
// for ore at the silo, its meal asks) is posted again by the next upkeep.
type CancelColonyOrder struct{ ID OrderID }

func (PlaceColonyOrder) isCommand()   {}
func (RepriceColonyOrder) isCommand() {}
func (CancelColonyOrder) isCommand()  {}

// maxColonyOrderQty caps one order, so a typo cannot escrow the treasury.
const maxColonyOrderQty = 10000

// Tradable reports whether the colony may be ordered to trade an item. A
// colonist's body is refuse to burn, never goods.
func (k ItemKind) Tradable() bool {
	return k > ItemNone && k < numItemKinds && k != ColonistCorpse
}

// TradableItems is every item a colony order may name, in item order.
func TradableItems() []ItemKind {
	var out []ItemKind
	for k := ItemNone + 1; k < numItemKinds; k++ {
		if k.Tradable() {
			out = append(out, k)
		}
	}
	return out
}

// ParseItemKind finds an item by its name (ItemKind.String).
func ParseItemKind(name string) (ItemKind, bool) {
	for k := ItemNone + 1; k < numItemKinds; k++ {
		if k.String() == name {
			return k, true
		}
	}
	return ItemNone, false
}

// placeColonyOrder posts a manual order for the colony and reports whether it
// did. It logs what happened, or why not.
func (w *World) placeColonyOrder(c PlaceColonyOrder) bool {
	verb := "bid for"
	if c.Side == Ask {
		verb = "offer"
	}
	refuse := func(why string) bool {
		w.logEvent(LogNote, fmt.Sprintf("The colony cannot %s %d %s at (%d, %d): %s.", verb, c.Qty, c.Item, c.Depot.X, c.Depot.Y, why))
		return false
	}
	switch {
	case !c.Item.Tradable():
		return refuse("that is not something it trades")
	case c.Qty <= 0 || c.Qty > maxColonyOrderQty:
		return refuse(fmt.Sprintf("an order is 1 to %d units", maxColonyOrderQty))
	case c.Price <= 0:
		return refuse("the price must be at least $1")
	}
	cont := w.storageContainers[c.Depot]
	if cont == nil || !w.communalFixture(c.Depot) {
		return refuse("there is no communal depot there")
	}
	switch c.Side {
	case Bid:
		if cost := Money(c.Qty) * c.Price; cost > w.treasury {
			return refuse(fmt.Sprintf("it would cost %v and the treasury holds %v", cost, w.treasury))
		}
	case Ask:
		if held := cont.held(Community, c.Item); held < c.Qty {
			return refuse(fmt.Sprintf("it holds only %d there", held))
		}
	}
	o, filled := w.post(c.Side, c.Item, c.Qty, c.Price, Community, c.Depot, 0)
	if o == nil {
		return refuse("the order could not be placed")
	}
	o.manual = true
	w.logEvent(LogNote, fmt.Sprintf("The colony posts an order to %s %d %s at %v each at (%d, %d)%s.",
		verb, c.Qty, c.Item, c.Price, c.Depot.X, c.Depot.Y, filledNote(filled, c.Qty)))
	return true
}

// repriceColonyOrder re-posts one of the colony's open orders at a new price
// and reports whether it did. A bid must be affordable at the new price from
// the treasury and what the order already holds; otherwise the order stays as
// it was.
func (w *World) repriceColonyOrder(c RepriceColonyOrder) bool {
	o := w.orders[c.ID]
	if o == nil || o.Actor != Community {
		w.logEvent(LogNote, "That order is no longer on the book.")
		return false
	}
	if c.Price <= 0 || c.Price == o.Price {
		return false
	}
	if o.Side == Bid {
		if cost := Money(o.Qty) * c.Price; cost > w.treasury+w.balance(o.owner()) {
			w.logEvent(LogNote, fmt.Sprintf("The treasury cannot cover %d %s at %v each.", o.Qty, o.Item, c.Price))
			return false
		}
	}
	side, item, qty, depot, was := o.Side, o.Item, o.Qty, o.Depot, o.Price
	w.cancel(o)
	n, filled := w.post(side, item, qty, c.Price, Community, depot, 0)
	if n == nil {
		// Unreachable: the escrow just came back. Say so rather than lose
		// the order silently.
		w.logEvent(LogNote, fmt.Sprintf("The colony's order for %d %s could not be re-posted.", qty, item))
		return false
	}
	n.manual = true
	w.logEvent(LogNote, fmt.Sprintf("The colony reprices its %s of %d %s at (%d, %d) from %v to %v%s.",
		side, qty, item, depot.X, depot.Y, was, c.Price, filledNote(filled, qty)))
	return true
}

// cancelColonyOrder takes one of the colony's open orders off the book and
// reports whether there was one.
func (w *World) cancelColonyOrder(id OrderID) bool {
	o := w.orders[id]
	if o == nil || o.Actor != Community {
		w.logEvent(LogNote, "That order is no longer on the book.")
		return false
	}
	w.logEvent(LogNote, fmt.Sprintf("The colony withdraws its %s of %d %s at %v at (%d, %d).",
		o.Side, o.Qty, o.Item, o.Price, o.Depot.X, o.Depot.Y))
	w.cancel(o)
	return true
}

// filledNote says how much of an order traded as it was posted.
func filledNote(filled, qty int) string {
	switch {
	case filled == 0:
		return ""
	case filled >= qty:
		return "; it filled at once"
	default:
		return fmt.Sprintf("; %d filled at once", filled)
	}
}

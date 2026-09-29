package sim

// ---- The foundry ------------------------------------------------------------------
//
// The colony's first supply chain that is not food. A forge smelts iron ore
// into steel ingots and a gun bench machines steel into assault rifles; the
// planner builds the two together in a foundry room. Nobody runs the foundry.
// The colony keeps a standing bid for rifles at its silo (the armory), and
// the producer planner does the rest: a gunsmith bids for steel at the gun
// bench, a smith bids for ore at the forge, and a hauler or a miner fills
// that bid from the silo's stock or its own. Every link is an ordinary bid in
// the book. See docs/foundry.md.

// foundryRoom is the forge and the gun bench, a tile apart, with an aisle. A
// smith carries its steel two tiles to the bench, not across the colony.
// Nothing in it is life support, so it may go up narrow in a cramped cavern:
// a smith and a gunsmith rarely want the same tile at once.
var foundryRoom = roomRecipe{
	name: "foundry", kinds: []Terrain{Forge, GunBench}, minFac: 2, maxFac: 2, aisle: true,
	planLog: "The colony marks out a foundry.",
}

// wantsFoundry reports whether the colony wants a foundry it has not planned:
// it wants rifles, and has no forge or no gun bench planned or built.
func (w *World) wantsFoundry() bool {
	if w.cfg.ArmoryRifles <= 0 || w.cfg.PriceAssaultRifle <= 0 {
		return false
	}
	return w.plannedFacilities(Forge) < 1 || w.plannedFacilities(GunBench) < 1
}

// mined reports whether k comes out of the rock: the goods the colony
// prospects for. Nothing makes them, but a planner can still bid for one,
// since a miner or a hauler can bring it.
func mined(k ItemKind) bool {
	for _, g := range prospectingGoods {
		if g == k {
			return true
		}
	}
	return false
}

// ---- The armory -------------------------------------------------------------------

// armoryStock is how many assault rifles the colony owns, in every depot.
func (w *World) armoryStock() int {
	n := 0
	for _, c := range w.storageContainers {
		n += c.held(Community, AssaultRifle)
	}
	return n
}

// refreshArmoryBids keeps the colony's standing bid for assault rifles at its
// silo topped up to what the armory is short of, at price-assault-rifle, as
// far as the treasury stretches. It bids only once a gun bench stands:
// before that the bid is demand nobody can meet, and money locked in escrow.
func (w *World) refreshArmoryBids() {
	price := Money(w.cfg.PriceAssaultRifle)
	if w.cfg.ArmoryRifles <= 0 || price <= 0 || w.countTerrain(GunBench) == 0 {
		return
	}
	silo, ok := w.marketDepot()
	if !ok {
		return
	}
	want := w.cfg.ArmoryRifles - w.armoryStock() - w.openQty(Bid, AssaultRifle, silo, Community)
	want = min(want, int(w.treasury/price))
	for want > 0 && !w.storageContainers[silo].Inventory.CanAdd(AssaultRifle, want) {
		want--
	}
	if want > 0 {
		w.post(Bid, AssaultRifle, want, price, Community, silo, 0)
	}
}

// ---- Supplying ore ------------------------------------------------------------------

// ownStock is a colonist's own units of a good it could take to a bid: in its
// pockets (carried), or on its line in the depot at from.
type ownStock struct {
	from    Point
	n       int
	carried bool
}

// suppliable reports whether a colonist takes its own stock of k to a bid
// for it: ore out of the rock, and what the foundry makes. Not food: a
// colonist's meals and scum have their own rules (meal-keep, cooking its own
// leftovers), and selling them from under it could starve it.
func suppliable(k ItemKind) bool {
	if mined(k) {
		return true
	}
	for _, r := range recipes {
		if r.Facility == Scumhouse {
			continue
		}
		for _, out := range r.Outputs {
			if out.Kind == k {
				return true
			}
		}
	}
	return false
}

// ownStockFor finds e's own units of bid b's item, if it is suppliable: what
// it carries first, else what it holds at b's own depot, else the nearest
// depot it can reach and use where it holds some. Ties break by position.
// Without this a smith's bid for ore was answered only by hauling from the
// silo, and a miner with ore in its pockets sold it to the colony for less.
// It is also how goods a plan left behind find a buyer: steel smelted for a
// gunsmith whose plan ran out of time sat in the forge for good.
func (w *World) ownStockFor(e *Entity, b *Order) (ownStock, bool) {
	if !suppliable(b.Item) {
		return ownStock{}, false
	}
	if n := e.ownCarried(b.Item); n > 0 {
		return ownStock{from: e.Pos, n: n, carried: true}, true
	}
	me := ColonistOwner(e.ID)
	if c := w.storageContainers[b.Depot]; c != nil {
		if n := c.held(me, b.Item); n > 0 {
			return ownStock{from: b.Depot, n: n}, true
		}
	}
	room := w.roomOf(e.Pos)
	var best ownStock
	bestDist, found := 1<<30, false
	for p, c := range w.storageContainers {
		n := c.held(me, b.Item)
		if n <= 0 || !w.canUseFixture(e, p) || !w.taskReachable(p, room) {
			continue
		}
		if d := e.Pos.Chebyshev(p); !found || d < bestDist || (d == bestDist && lessPoint(p, best.from)) {
			best, bestDist, found = ownStock{from: p, n: n}, d, true
		}
	}
	return best, found
}

// planSupply takes e's own stock to bid b, if the bid beats what the stock
// would fetch otherwise by enough to pay for the walk. It is arbitrage
// against itself. What ore would fetch otherwise is the colony's prospecting
// bid, its reference price, not its remembered trade price: the smith's first
// fill at $8 made iron "worth" $8, and nobody would carry their own ore to an
// $8 bid for no profit while the colony was paying $3 for it.
func (w *World) planSupply(e *Entity, b *Order, s ownStock, probe *planOffer) bool {
	qty := min(s.n, b.Qty)
	if !s.carried {
		for qty > 0 && !e.Inventory.CanAdd(b.Item, qty) {
			qty--
		}
	}
	if qty <= 0 {
		return false
	}
	if s.from == b.Depot && !s.carried {
		// Already where the buyer is: sell it on the spot. No job comes of
		// it, so e goes on looking for work.
		if best, ok := w.bestBid(b.Item, b.Depot); ok && best.Actor != ColonistOwner(e.ID) &&
			b.Price >= w.refPrice(b.Item)+Money(w.cfg.PlanMinProfit) {
			_, filled := w.post(Ask, b.Item, qty, b.Price, ColonistOwner(e.ID), b.Depot, w.cfg.OrderTTL)
			w.emitDone(e, ActionTrade, NounGoods, "Sold %d %s where it lay, for %v each.", filled, b.Item, b.Price)
		}
		return false
	}
	walk := e.Pos.Chebyshev(b.Depot)
	if !s.carried {
		walk = e.Pos.Chebyshev(s.from) + s.from.Chebyshev(b.Depot)
	}
	profit := Money(qty)*(b.Price-w.refPrice(b.Item)) - w.laborCostFor(e, walk)
	if profit < Money(w.cfg.PlanMinProfit) {
		return false
	}
	if probe != nil {
		*probe = planOffer{profit, walk}
		return true
	}
	p := w.newPlan(e, planHaul, b, qty)
	p.crafted, p.expect = true, profit
	w.emitDone(e, ActionTrade, NounGoods, "Took %d of its own %s to sell at (%d, %d) for %v.",
		qty, b.Item, b.Depot.X, b.Depot.Y, b.Price)
	if s.carried {
		p.workshop = b.Depot
		w.assignCarry(e, p, b.Depot, carryDeliver, qty)
		return true
	}
	p.workshop = s.from
	w.assignCarry(e, p, s.from, carryFetch, qty)
	return true
}

package sim

import "fmt"

// Wants: feelings that buy things. A drive that presses can want something
// it would buy, and a want posts an order on the book with a lifetime of
// want-ttl. When the order fills, lapses or is cancelled, the colonist decides
// again from where it stands: let it go (calm now), post again at today's
// willingness, or, after want-patience lapses on one rung, look past it to the
// next. Posting is how demand is shown: a colonist never checks whether
// anything could fill its order, so a bid for a rifle in a colony with no forge
// yet is the signal a smith reads. One drive's wants are a ladder of rungs,
// climbed one at a time; safety's are a gun, then a better gun (a bodyguard
// is S4). Hunger's meal bids predate this and still run on their own
// (mealBidLimit). See docs/safety.md.

// wantRung is one thing a drive can want.
type wantRung struct {
	// name is how the inspector says it: "a gun", "a better gun".
	name string
	// held reports whether e already has what this rung buys (counting a
	// purchase waiting at a depot), so the ladder moves past it.
	held func(w *World, e *Entity) bool
	// value is what it is worth to a calm colonist: what it would pay at
	// SeekAt. Zero means there is nothing to want (no market, nothing above
	// what it has).
	value func(w *World, e *Entity) Money
	// most is the percent of value it will pay at the drive's ceiling.
	most func(w *World) int
	// post puts an order for it at limit, with lifetime ttl: a bid on the
	// book, or a work order (a bodyguard's contract).
	post func(w *World, e *Entity, limit Money, ttl int) wantPosted
	// afford, if set, says whether e could buy it at all at limit, beyond
	// willingness being above zero (a bodyguard's budget rule).
	afford func(w *World, e *Entity, limit Money) bool
}

// wantPosted is what a want put up: a market order or a work order.
type wantPosted struct {
	order *Order
	work  *WorkOrder
}

// affordable reports whether e can buy r at limit.
func (r *wantRung) affordable(w *World, e *Entity, limit Money) bool {
	return limit > 0 && (r.afford == nil || r.afford(w, e, limit))
}

// wantSpec is one drive's ladder.
type wantSpec struct {
	drive DriveKind
	rungs []wantRung
}

// wantState is where one colonist is on one drive's ladder: the order it has
// open for a rung (nil for none), how many orders in a row have lapsed on that
// rung, and when it last took hold of a rung, so it gives a purchase time to
// work (want-settle-ticks) before climbing past it.
type wantState struct {
	order    *Order
	work     *WorkOrder // a bodyguard's contract, open
	rung     int
	lapses   int
	heldRung int  // how many rungs were held, from the bottom, when last looked
	looked   bool // heldRung is from an earlier look, not the zero value
	settle   int  // tick before which it does not climb past heldRung
	retry    int  // tick before which it does not try to post again after failing to
}

// wantRetryTicks is how long a colonist waits to try again when it could not
// post a want at all (no market depot, no money for even a token bid).
const wantRetryTicks = 90

// wantSpecs is every drive's ladder.
var wantSpecs = []wantSpec{{drive: DriveSafety, rungs: safetyRungs}}

// willingness is the most e will pay for something worth value to it when calm,
// with its drive d where it is now: value at SeekAt, rising linearly to most%
// of value at the ceiling. It spends only what it has beyond want-reserve-meals
// meals' worth, and at most half of that until the drive is critical, then all
// of it, as for a meal: a dollar matters more to someone with few of them,
// until nothing matters more. Food comes first: the reserve is there because
// colonists who spent their last dollars on guns starved.
func (w *World) willingness(e *Entity, d DriveKind, value Money, most int) Money {
	if value <= 0 {
		return 0
	}
	spendable := w.spendable(e)
	if spendable <= 0 {
		return 0
	}
	spec := w.cfg.Drives[d]
	span := max(1, spec.Max-spec.SeekAt)
	frac := clampInt(w.driveLevel(e, d)-spec.SeekAt, 0, span)
	limit := Money(int64(value) * int64(100*span+(most-100)*frac) / int64(100*span))
	if e.drives[d].phase < DriveCritical {
		limit = min(limit, spendable/2)
	}
	return max(0, min(limit, spendable))
}

// postWants runs a colonist's wants: it notes what became of an open order,
// and, while the drive presses and nothing is open, posts the next. step calls
// it after every colonist turn; with an order open, or the drive calm, it
// costs a comparison.
func (w *World) postWants(e *Entity) {
	if e.Kind != Colonist || !e.Alive() {
		return
	}
	for i := range wantSpecs {
		w.runWant(e, &wantSpecs[i])
	}
}

func (w *World) runWant(e *Entity, spec *wantSpec) {
	ws := &e.wants[spec.drive]
	if o := ws.order; o != nil {
		if w.orders[o.ID] == o {
			return // still on the book
		}
		ws.order = nil
		if o.Filled > 0 {
			ws.lapses = 0
			if o.Side == Bid && o.Item.isWeapon() {
				e.gunWaiting = max(e.gunWaiting, weaponRank(o.Item)) // bought: to fetch
			}
		} else {
			ws.lapses++
		}
	}
	if o := ws.work; o != nil {
		if w.workOrders[o.ID] == o && !w.lapseEscort(o) {
			return // still open, or being served
		}
		ws.work = nil
		if o.served > 0 {
			ws.lapses = 0
		} else {
			ws.lapses++
		}
	}
	if e.drives[spec.drive].phase < DrivePressing || e.drives[DriveFood].phase >= DrivePressing {
		return // calm, or hungry: food comes first
	}
	held := 0 // rungs held, from the bottom

	for held < len(spec.rungs) && spec.rungs[held].held(w, e) {
		held++
	}
	if !ws.looked {
		// What it arrived with (a gun from the ship) is not a purchase to
		// give time to work.
		ws.looked, ws.heldRung = true, held
	}
	if held != ws.heldRung {
		if held > ws.heldRung {

			ws.settle = w.tick + w.cfg.WantSettleTicks // give it a chance to work
			ws.lapses = 0
		}
		ws.heldRung = held
	}
	if held >= len(spec.rungs) || w.tick < ws.settle || w.tick < ws.retry {
		return
	}
	rung := held
	if rung != ws.rung {
		ws.rung, ws.lapses = rung, 0
	}
	// Waited long enough: a colonist that can afford the next rung up looks
	// past this one.
	if ws.lapses >= w.cfg.WantPatience && rung+1 < len(spec.rungs) {
		next := &spec.rungs[rung+1]
		if v := next.value(w, e); v > 0 && next.affordable(w, e, w.willingness(e, spec.drive, v, next.most(w))) {
			rung = rung + 1
		}
	}
	r := &spec.rungs[rung]
	v := r.value(w, e)
	limit := w.willingness(e, spec.drive, v, r.most(w))
	var p wantPosted
	if r.affordable(w, e, limit) {
		p = r.post(w, e, limit, w.cfg.WantTTL)
	}
	if p.order == nil && p.work == nil {
		ws.retry = w.tick + wantRetryTicks
		return
	}
	// Even an order filled on the spot is noted: the next look sees it gone
	// with Filled set, and the ladder moves on.
	ws.order, ws.work = p.order, p.work
}

// wantLine is the inspector's sentence for e's wants: what it wants, what it
// will pay, and what it has. Empty while nothing presses.
func (w *World) wantLine(e *Entity) string {
	for i := range wantSpecs {
		spec := &wantSpecs[i]
		if e.drives[spec.drive].phase < DrivePressing {
			continue
		}
		held := 0
		for held < len(spec.rungs) && spec.rungs[held].held(w, e) {
			held++
		}
		if held >= len(spec.rungs) {
			continue
		}
		r := &spec.rungs[held]
		v := r.value(w, e)
		if v <= 0 {
			return fmt.Sprintf("Wants %s; nothing to buy it with yet.", r.name)
		}
		limit := w.willingness(e, spec.drive, v, r.most(w))
		if !r.affordable(w, e, limit) {
			return fmt.Sprintf("Wants %s; can't afford it (has %v).", r.name, e.wallet)
		}
		return fmt.Sprintf("Wants %s: will pay up to %v; has %v.", r.name, limit, e.wallet)
	}
	return ""
}

// ---- Safety's ladder: a gun, a better gun --------------------------------------

var safetyRungs = []wantRung{
	{
		name:  "a gun",
		held:  func(w *World, e *Entity) bool { return w.bestGunOwned(e) > 0 },
		value: func(w *World, e *Entity) Money { return w.gunValue(Pistol) },
		most:  func(w *World) int { return w.cfg.GunWillingness },
		post: func(w *World, e *Entity, limit Money, ttl int) wantPosted {
			return wantPosted{order: w.bidForGun(e, 0, limit, ttl)}
		},
	},
	{
		name: "a better gun",
		held: func(w *World, e *Entity) bool { return w.bestGunOwned(e) >= weaponRank(AssaultRifle) },
		value: func(w *World, e *Entity) Money {
			// The next gun up, less what its own fetches: a trade-in.
			own := w.bestGunOwned(e)
			for _, k := range weaponKinds {
				if weaponRank(k) == own+1 {
					return max(1, w.gunValue(k)-w.gunValue(gunOfRank(own)))
				}
			}
			return 0
		},
		most: func(w *World) int { return w.cfg.GunWillingness },
		post: func(w *World, e *Entity, limit Money, ttl int) wantPosted {
			own := w.bestGunOwned(e)
			return wantPosted{order: w.bidForGun(e, own, limit+w.gunValue(gunOfRank(own)), ttl)}
		},
	},
	{
		// A bodyguard (escort.go): only the rich can afford one, by
		// escort-budget-percent, and only after a rifle, or after
		// want-patience days waiting for one.
		name:  "a bodyguard",
		held:  func(w *World, e *Entity) bool { return e.wants[DriveSafety].work != nil },
		value: func(w *World, e *Entity) Money { return Money(w.cfg.EscortWage) },
		most:  func(w *World) int { return w.cfg.EscortWillingness },
		post: func(w *World, e *Entity, limit Money, ttl int) wantPosted {
			return wantPosted{work: w.hireBodyguard(e, limit, ttl)}
		},
		afford: func(w *World, e *Entity, limit Money) bool {
			_, ok := w.escortRate(e, limit)
			return ok
		},
	},
}

// gunOfRank is the weapon kind of a weaponRank (ItemNone for 0).
func gunOfRank(rank int) ItemKind {
	for _, k := range weaponKinds {
		if weaponRank(k) == rank {
			return k
		}
	}
	return ItemNone
}

// gunValue is what a gun is worth: its smoothed trade price once guns of its
// kind have traded, its reference price (price-pistol, price-shotgun,
// price-assault-rifle) before. Kept apart from refPrice, which is what the
// colony pays at its silo and what the producer planner reckons a supplier's
// own stock at.
func (w *World) gunValue(k ItemKind) Money {
	if m := w.prices[k]; m.traded {
		return Money((m.milli + 500) / 1000)
	}
	switch k {
	case Pistol:
		return Money(w.cfg.PricePistol)
	case Shotgun:
		return Money(w.cfg.PriceShotgun)
	case AssaultRifle:
		return Money(w.cfg.PriceAssaultRifle)
	}
	return 0
}

// bestGunOwned is the rank of the best gun e carries or has bought and not
// yet fetched (gunWaiting). Remembering the purchase, rather than looking
// through every depot, keeps this a few comparisons: it runs every turn for
// an uneasy colonist, and for the inspector every frame.
func (w *World) bestGunOwned(e *Entity) int {
	return max(e.weaponRank, weaponRank(bestWeapon(e.Inventory)), e.gunWaiting)
}

// bidForGun posts e's bid for the best gun above rank it can pay limit for, at
// the market depot; for the cheapest one above rank, at limit, if it can pay
// for none. A bid below a gun's price is still demand, and shows it.
func (w *World) bidForGun(e *Entity, above int, limit Money, ttl int) *Order {
	depot, ok := w.marketDepot()
	if !ok || !w.canUseFixture(e, depot) {
		return nil
	}
	kind, price := ItemNone, Money(0)
	for _, k := range weaponKinds { // worst to best
		if weaponRank(k) <= above {
			continue
		}
		if v := w.gunValue(k); v > 0 && v <= limit {
			kind, price = k, v
		} else if kind == ItemNone {
			kind, price = k, limit // the cheapest above: bid what it can
		}
	}
	price = min(price, e.wallet)
	if kind == ItemNone || price <= 0 {
		return nil
	}
	o, filled := w.post(Bid, kind, 1, price, ColonistOwner(e.ID), depot, ttl)
	if o == nil {
		return nil
	}
	if filled > 0 {
		w.emitDone(e, ActionBuy, NounGoods, "Bought a %s for %v.", kind, price)
	}
	return o
}

// ---- Fetching a bought gun ------------------------------------------------------

// tryFetchGun sends e to collect a gun it owns in a depot that is better than
// the one it carries. assignWorkJob asks first: an unarmed colonist whose
// rifle sits at the silo should not go mining without it.
func (w *World) tryFetchGun(e *Entity) bool {
	me, carried := ColonistOwner(e.ID), weaponRank(bestWeapon(e.Inventory))
	if e.gunWaiting <= carried {
		e.gunWaiting = 0
		return false
	}
	room := w.roomOf(e.Pos)
	var at Point
	bestRank, bestDist, found := 0, 1<<30, false
	w.eachContainer(func(c *StorageContainer) {
		if !w.canUseFixture(e, c.Pos) || !w.taskReachable(c.Pos, room) {
			return
		}
		for _, k := range weaponKinds {
			r := weaponRank(k)
			if r <= carried || c.held(me, k) == 0 {
				continue
			}
			d := w.travelEstimate(e.Pos, c.Pos)
			if !found || r > bestRank || r == bestRank && (d < bestDist || d == bestDist && lessPoint(c.Pos, at)) {
				at, bestRank, bestDist, found = c.Pos, r, d, true
			}
		}
	})

	if !found {
		e.gunWaiting = 0 // it is not anywhere it can reach: stop looking
		return false
	}
	if !e.Inventory.CanAdd(gunOfRank(bestRank), 1) {
		return false
	}
	e.Job, e.Target, e.Progress = JobFetch, at, 0
	return true
}

// jobFetch walks to the depot at Target and takes the best gun e owns there.
// The gun it carried goes into the depot in its name and on sale at its
// value: a trade-in, which is how pistols and shotguns come to market.
func (w *World) jobFetch(e *Entity) {
	arrived, ok := w.travelTo(e, e.Target)
	if !ok {
		w.clearJob(e)
		return
	}
	if !arrived {
		e.State = Moving
		return
	}
	c := w.storageContainers[e.Target]
	me := ColonistOwner(e.ID)
	defer w.clearJob(e)
	if c == nil {
		return
	}
	for i := len(weaponKinds) - 1; i >= 0; i-- {
		k := weaponKinds[i]
		if weaponRank(k) <= weaponRank(bestWeapon(e.Inventory)) || c.held(me, k) == 0 || !c.debit(me, k, 1) {
			continue
		}
		old := bestWeapon(e.Inventory)
		e.Inventory.Add(k, 1)
		if weaponRank(k) >= e.gunWaiting {
			e.gunWaiting = 0
		}

		w.emitDone(e, ActionHaul, NounGoods, "Picked up %s %s.", e.possessive(), k)
		// The old gun goes into the depot, physically and on its line, and
		// only if the depot has room; otherwise it stays in its pocket.
		if old != ItemNone && c.Inventory.CanAdd(old, 1) && e.Inventory.Remove(old, 1) {
			c.Inventory.Add(old, 1)
			c.credit(me, old, 1)

			if o, _ := w.post(Ask, old, 1, w.gunValue(old), me, c.Pos, w.cfg.OrderTTL); o != nil {
				w.emitDone(e, ActionTrade, NounGoods, "Put %s old %s up for sale for %v.", e.possessive(), old, w.gunValue(old))
			}
		}
		return
	}
}

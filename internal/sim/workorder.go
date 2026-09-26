package sim

import (
	"fmt"
	"sort"
)

// ---- Labor orders --------------------------------------------------------------
//
// Most of what colonists do is work, not goods, so work has its own
// instrument. A WorkOrder is pay, escrowed from its issuer when it is posted,
// for a unit of work: raising one build task's tile, or hauling one unit of
// the issuer's goods between depots. Whoever does the work is paid when it is
// done.
//
// The colony is an issuer like anyone else. The room planner still decides
// what the colony needs, but it now buys the work: every task of a room it
// plans is a work order funded from the treasury, and a room the treasury
// cannot fund is not planned. A colonist can commission a room the same way,
// from its own wallet, and owns what gets built. See docs/labor.md.

// WorkKind is what a work order pays for.
type WorkKind uint8

const (
	WorkBuild WorkKind = iota // raise one build task's tile
	WorkHaul                  // move a unit of the issuer's goods from one depot to another
)

func (k WorkKind) String() string {
	if k == WorkBuild {
		return "build"
	}
	return "haul"
}

// WorkOrder is pay escrowed for work.
type WorkOrder struct {
	ID     OrderID
	Kind   WorkKind
	Issuer Owner
	Pay    Money // per unit of work
	Units  int   // units still to be paid for
	Pos    Point // WorkBuild: the task's tile. WorkHaul: where to.
	Posted int
	// WorkHaul only: the depot the goods come from, and what they are. The
	// goods stay the issuer's throughout; the hauler's cargo record says so.
	From Point
	Item ItemKind

	escrow Money // Pay × Units at rest
}

// owner is the work order as a money holder: where its escrow lives.
func (o *WorkOrder) owner() Owner { return Owner{Kind: ownerWork, ID: EntityID(o.ID)} }

// postWork escrows pay × units from issuer and opens a work order, or returns
// nil if the issuer cannot fund it.
func (w *World) postWork(kind WorkKind, issuer Owner, pay Money, units int, pos Point) *WorkOrder {
	if units <= 0 || pay < 0 {
		return nil
	}
	w.nextOrderID++
	o := &WorkOrder{ID: w.nextOrderID, Kind: kind, Issuer: issuer, Pay: pay, Units: units,
		Pos: pos, Posted: w.tick}
	w.workOrders[o.ID] = o
	if !w.transfer(issuer, o.owner(), pay*Money(units)) {
		delete(w.workOrders, o.ID)
		return nil
	}
	return o
}

// payWork pays worker for one unit of o, closing o when it has none left.
func (w *World) payWork(o *WorkOrder, worker *Entity) {
	if o == nil || o.Units <= 0 || w.workOrders[o.ID] != o {
		return
	}
	w.transfer(o.owner(), ColonistOwner(worker.ID), o.Pay)
	o.Units--
	if o.Units == 0 {
		w.closeWork(o)
	}
}

// closeWork returns whatever o still escrows to its issuer and forgets it. A
// dead issuer's refund freezes with its wallet, keeping the audit exact.
func (w *World) closeWork(o *WorkOrder) {
	if left := o.escrow; left > 0 && !w.transfer(o.owner(), o.Issuer, left) {
		w.moneyFrozen += left
		o.escrow = 0
	}
	o.Units = 0
	delete(w.workOrders, o.ID)
}

// workEscrowed is every dollar held by open work orders.
func (w *World) workEscrowed() Money {
	var total Money
	for _, o := range w.workOrders {
		total += o.escrow
	}
	return total
}

// sortedWork returns the open work orders passing keep, oldest first.
func (w *World) sortedWork(keep func(*WorkOrder) bool) []*WorkOrder {
	var out []*WorkOrder
	for _, o := range w.workOrders {
		if keep == nil || keep(o) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ---- Paying for construction --------------------------------------------------

// wageFor is what the colony (or a commissioner) pays to have a tile of t
// raised: digging rock out, a wall, or a fixture.
func (w *World) wageFor(t Terrain) Money {
	switch t {
	case Floor:
		return Money(w.cfg.WageDig)
	case Wall:
		return Money(w.cfg.WageWall)
	default:
		return Money(w.cfg.WageFixture)
	}
}

// projectCost is what funding every task of p would cost.
func (w *World) projectCost(p *project) Money {
	var total Money
	for _, t := range p.tasks {
		total += w.wageFor(t.terrain)
	}
	return total
}

// fundProject posts a work order for every task of p, paid by p.issuer, and
// reports whether it could. It is all or nothing: an issuer who cannot afford
// the whole room funds none of it.
//
// A project issued by nobody is unpaid community work: it posts no orders and
// always goes ahead. Only life support is planned that way (see planRooms).
func (w *World) fundProject(p *project) bool {
	if p.issuer.Kind == OwnerNone {
		return true
	}
	if w.balance(p.issuer) < w.projectCost(p) {
		return false
	}
	for _, t := range p.tasks {
		t.order = w.postWork(WorkBuild, p.issuer, w.wageFor(t.terrain), 1, t.pos)
	}
	return true
}

// cancelWorkOf cancels every work order an issuer has open, and drops the
// projects it was paying for: nobody is left to pay for the rest. What was
// already built stays built.
func (w *World) cancelWorkOf(issuer Owner) {
	for _, o := range w.sortedWork(func(o *WorkOrder) bool { return o.Issuer == issuer }) {
		w.closeWork(o)
	}
	kept := w.projects[:0]
	for _, p := range w.projects {
		if p.issuer == issuer && issuer != Community {
			for _, t := range p.tasks {
				if t.owner != 0 {
					if e := w.entities[t.owner]; e != nil && e.task == t {
						w.clearJob(e)
					}
				}
			}
			continue
		}
		kept = append(kept, p)
	}
	w.projects = kept
}

// ---- Commissions ------------------------------------------------------------------

// houseRoom is a colonist's own house: a bunk and a toilet behind its own
// walls. The bunk is private; the toilet is open to anyone who pays the fee,
// which is how renting out a spare toilet becomes a business.
var houseRoom = roomRecipe{
	name: "house", kinds: []Terrain{Bed, Toilet}, minFac: 2, maxFac: 2,
	planLog: "A colonist commissions a house.",
}

// fixtureAccess is how a commissioned room's fixtures are opened to others:
// a house's toilet takes paying customers; everything else is private.
func (w *World) fixtureAccess(p *project, t Terrain) (Access, Money) {
	if p.name == houseRoom.name && t == Toilet && w.cfg.ToiletFee > 0 {
		return AccessPaid, Money(w.cfg.ToiletFee)
	}
	return AccessPrivate, 0
}

// commissionHouses has the first colonist (by ID) who can afford a house and
// has none commission one, paid from its own wallet. One per planning cycle.
func (w *World) commissionHouses() {
	if w.cfg.HouseSavings <= 0 {
		return
	}
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e.Kind != Colonist || e.wallet < Money(w.cfg.HouseSavings) || e.commissioned {
			continue
		}
		if w.planRoomFor(houseRoom, ColonistOwner(e.ID)) {
			e.commissioned = true
			w.log.add(fmt.Sprintf("%s commissions a house.", e.displayName()))
		}
		return
	}
}

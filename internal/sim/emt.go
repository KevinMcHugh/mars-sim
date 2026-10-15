package sim

import "slices"

// Emergency services (aid phase A4). The player sets an EMT detail, how many
// colonists to keep on call, and the wage, in cents an hour. Being on call is
// not a job a colonist stands at but a status it carries while it works: it
// hears of every casualty in the colony, not just the ones it can see, and
// helping one scores aid-duty-bonus more, enough to drop any work and its own
// pressing (not critical) needs. Only an EMT drags a colonist down from blood
// loss to an infirmary cot.
//
// The wage is a market price, not a rule. Each colonist asks its own price
// for being on call (emtAsk): what the calls will take of its time, at what
// its time is worth to it (its reservation, as for any other work), plus a
// premium for being the one who has to drop everything. A colonist looking
// for work takes an open place only if the wage meets its ask, and quits on
// payday if its ask has risen above it. So a wage too low leaves places
// empty, and the colonists who take one are the ones whose time is cheapest
// or who care most. The treasury pays as far as it goes, in whole dollars
// as they accrue. A colonist keeps its place until it sleeps, goes down,
// becomes a casualty, quits, or the detail is cut below it; then the next
// willing colonist looking for work takes it. See
// docs/emergency-services.md.

// MaxEMTDetail is the most colonists the player may keep on call,
// MaxEMTWage the highest wage it may offer, in cents an hour, and MaxEMTFee
// the highest fee per call, in dollars.
const (
	MaxEMTDetail = 20
	MaxEMTWage   = 1000
	MaxEMTFee    = 50
)

// SetEMTDetail sets how many colonists the colony keeps on call as EMTs (0
// stands the detail down).
type SetEMTDetail struct {
	Detail int
}

func (SetEMTDetail) isCommand() {}

// SetEMTWage sets what the colony offers an on-call EMT, in cents an hour.
type SetEMTWage struct {
	Cents int
}

func (SetEMTWage) isCommand() {}

// SetEMTFee sets what the colony pays an on-call EMT for each casualty it
// sees to, in dollars (0: no fee).
type SetEMTFee struct {
	Dollars int
}

func (SetEMTFee) isCommand() {}

// setEMTFee carries out a SetEMTFee, and reports whether it did. Orders
// already posted keep the fee they were posted at.
func (w *World) setEMTFee(c SetEMTFee) bool {
	if c.Dollars < 0 || c.Dollars > MaxEMTFee {
		return false
	}
	w.emtFee = Money(c.Dollars)
	return true
}

// postRescue opens the colony's per-call order for casualty c, escrowing the
// fee from the treasury, when there is a detail and a fee and the treasury
// can fund it. It is an order like any other on the work book: the money
// sits in escrow until settleRescue pays it out or sends it home.
func (w *World) postRescue(c *Entity) {
	if w.emtDetail <= 0 || w.emtFee <= 0 || c.rescueOrder != 0 {
		return
	}
	if o := w.postWork(WorkRescue, Community, w.emtFee, 1, c.Pos); o != nil {
		c.rescueOrder = o.ID
	}
}

// settleRescue closes casualty c's per-call order: paid to helper if helper
// is an on-call EMT that did something for it, and otherwise back to the
// treasury (it came round on its own, saw to itself, or a bystander helped:
// a bystander's help is its own, and nobody is paid for it).
func (w *World) settleRescue(c, helper *Entity) {
	o := w.workOrders[c.rescueOrder]
	c.rescueOrder = 0
	if o == nil {
		return
	}
	if helper != nil && helper != c && helper.onCall && helper.aidDone != (aidDone{}) {
		w.emtFeesPaid += o.Pay
		w.payWork(o, helper)
		return
	}
	w.closeWork(o)
}

// setEMTDetail carries out a SetEMTDetail, and reports whether it did. A cut
// stands down the highest IDs first, so who stays never depends on map order.
func (w *World) setEMTDetail(c SetEMTDetail) bool {
	if c.Detail < 0 || c.Detail > MaxEMTDetail {
		return false
	}
	w.emtDetail = c.Detail
	for len(w.onCall) > w.emtDetail {
		if e := w.entities[w.onCall[len(w.onCall)-1]]; e != nil {
			w.releaseOnCall(e)
		} else {
			w.onCall = w.onCall[:len(w.onCall)-1]
		}
	}
	return true
}

// setEMTWage carries out a SetEMTWage, and reports whether it did. Nobody
// quits the moment a wage is cut: each EMT weighs it on its next payday.
func (w *World) setEMTWage(c SetEMTWage) bool {
	if c.Cents < 0 || c.Cents > MaxEMTWage {
		return false
	}
	w.emtWage = c.Cents
	return true
}

// emtAsk is what e wants, in cents an hour, to be on call: the share of an
// hour the calls take (emtShare, what the colony's EMTs have lately spent on
// calls) at what its time is worth to it (reservation, as for any other
// work), plus emt-premium percent of an hour's worth for being interruptible
// at all. A Caring colonist asks half the premium and a Callous one double:
// one would do it anyway, the other wants paying for its trouble. What it
// expects the calls themselves to pay (the per-call fee, at the calls an EMT
// has lately answered an hour) comes off, so a colony that pays by the call
// can offer a smaller retainer.
func (w *World) emtAsk(e *Entity) int {
	hour := int(w.reservation(e)) * w.cfg.TicksPerHour() / 1000 // cents an hour
	premium := hour * w.cfg.EMTPremiumPct / 100
	if p := e.Profile; p != nil {
		switch {
		case p.HasTrait(TraitCaring):
			premium /= 2
		case p.HasTrait(TraitCallous):
			premium *= 2
		}
	}
	ask := (hour*w.emtShare+999)/1000 + premium
	// What the calls themselves pay comes off: emtCallRate calls per
	// thousand hours on call, at the fee, in cents an hour, at what this
	// colonist reckons uncertain pay is worth (emtFeeTrust).
	ask -= w.emtFeeIncome() * w.emtFeeTrust(e) / 100
	return max(0, ask)
}

// emtFeeIncome is what the per-call fee pays an EMT, in cents an hour, at
// the calls an EMT has lately answered.
func (w *World) emtFeeIncome() int {
	return int(w.emtFee) * 100 * w.emtCallRate / 1000
}

// emtFeeTrust is how much of its expected fee income e counts on, in
// percent: pay that has to be earned call by call is less sure than a wage.
// emt-fee-trust for most; an Industrious colonist counts on more (it expects
// to earn it), a Lazy one on much less, an Optimist a little more and a
// Pessimist a little less. So a package heavy on fees draws the hard
// workers, and one heavy on wage draws whoever is cheapest, lazy or not.
func (w *World) emtFeeTrust(e *Entity) int {
	trust := w.cfg.EMTFeeTrustPct
	if p := e.Profile; p != nil {
		switch {
		case p.HasTrait(TraitIndustrious):
			trust += 40
		case p.HasTrait(TraitLazy):
			trust -= 30
		}
		switch {
		case p.HasTrait(TraitOptimist):
			trust += 20
		case p.HasTrait(TraitPessimist):
			trust -= 20
		}
	}
	return max(0, trust)
}

// emtDuty is the aid score an on-call EMT adds for a casualty: the duty a
// wage buys, which a Lazy EMT feels at half strength and an Industrious one
// at a quarter more, and the pull of the fee itself, emt-fee-incentive
// points a dollar up to emt-fee-incentive-max, which every EMT feels alike.
// A wage pays the same however hard it answers; a fee pays for answering.
func (w *World) emtDuty(e *Entity) int {
	duty := w.cfg.AidDutyBonus
	if p := e.Profile; p != nil {
		switch {
		case p.HasTrait(TraitLazy):
			duty /= 2
		case p.HasTrait(TraitIndustrious):
			duty = duty * 5 / 4
		}
	}
	return duty + min(int(w.emtFee)*w.cfg.EMTFeeIncentive, w.cfg.EMTFeeIncentiveMax)
}

// tryTakeOnCall puts e on the detail if a place is open and the wage meets
// its ask. assignWorkJob calls it for a colonist looking for work, which then
// goes on to choose work as usual: the place costs it nothing until a
// casualty comes up.
func (w *World) tryTakeOnCall(e *Entity) {
	if e.onCall || len(w.onCall) >= w.emtDetail || e.passedOut || e.Kind != Colonist || w.isCasualty(e) {
		return
	}
	if w.emtWage < w.emtAsk(e) {
		return
	}
	i, _ := slices.BinarySearch(w.onCall, e.ID)
	w.onCall = slices.Insert(w.onCall, i, e.ID)
	e.onCall, e.emtTicks = true, 0
	w.markMindDirty(e) // a casualty may already be waiting
}

// releaseOnCall takes e off the detail. Pay accrued and not yet a whole
// dollar goes with the place.
func (w *World) releaseOnCall(e *Entity) {
	if i, found := slices.BinarySearch(w.onCall, e.ID); found {
		w.onCall = slices.Delete(w.onCall, i, i+1)
	}
	e.onCall, e.emtTicks, e.emtOwed = false, 0, 0
}

// serveOnCall runs an on-call colonist's turn on the detail: it goes off
// call when it sleeps, goes down or is a casualty itself; otherwise its hour
// counts toward its pay and toward the colony's measure of how much of an
// EMT's time calls take. On payday it is paid what has accrued, in whole
// dollars, and quits if its ask has risen above the wage.
func (w *World) serveOnCall(e *Entity) {
	if !e.onCall {
		return
	}
	if e.passedOut || e.State == Sleeping || w.isCasualty(e) {
		w.releaseOnCall(e)
		return
	}
	w.emtOnTicks++
	if e.Job == JobAid {
		w.emtBusyTicks++
	}
	w.updateEMTShare()
	e.emtTicks++
	if e.emtTicks < w.cfg.TicksPerHour() {
		return
	}
	e.emtTicks = 0
	e.emtOwed += w.emtWage
	if dollars := Money(e.emtOwed / 100); dollars > 0 && w.transfer(Community, ColonistOwner(e.ID), dollars) {
		e.emtOwed -= int(dollars) * 100
		w.emtPaid += dollars // as far as the treasury goes
	}
	if w.emtWage < w.emtAsk(e) {
		w.releaseOnCall(e)
	}
}

// updateEMTShare folds what EMTs have spent on calls into emtShare, the
// per-mille of on-call time that calls take, once a day's worth of on-call
// ticks has gathered: a quarter new, three quarters what it was, so one bad
// day moves the price without setting it.
func (w *World) updateEMTShare() {
	if w.emtOnTicks < w.cfg.TicksPerDay() {
		return
	}
	now := w.emtBusyTicks * 1000 / w.emtOnTicks
	w.emtShare = (3*w.emtShare + now) / 4
	rate := w.emtCallsWindow * 1000 * w.cfg.TicksPerHour() / w.emtOnTicks
	w.emtCallRate = (3*w.emtCallRate + rate) / 4
	w.emtOnTicks, w.emtBusyTicks, w.emtCallsWindow = 0, 0, 0
}

// EMTView is the EMT detail as a frontend shows it: how many it asks for,
// who is on call now, the wage in cents an hour, what the colony has paid in
// all, how many cots stand, the share of on-call time calls have lately
// taken (per mille), and the market: the cheapest ask among the colonists
// not on call (-1 with none) and how many of them the wage would draw.
type EMTView struct {
	Detail, Max   int
	OnCall        []EntityID
	Wage, MaxWage int
	Paid          Money
	Cots          int
	Share         int
	CheapestAsk   int
	Willing       int
	// ResponseTicks is how long, on average, a casualty has waited for an
	// on-call EMT to set out to it (0 before any has), and Responses how
	// many it has answered.
	ResponseTicks, Responses int
	// Fee is the per-call fee in dollars and MaxFee the highest, FeesPaid
	// what the colony has paid in fees, and CallRate the calls an EMT has
	// lately answered per thousand hours on call.
	Fee, MaxFee Money
	FeesPaid    Money
	CallRate    int
	// Package is what the wage and the fee together pay an EMT that counts
	// on its fees at face value, in cents an hour.
	Package int
}

func (w *World) publishedEMT() EMTView {
	v := EMTView{
		Detail: w.emtDetail, Max: MaxEMTDetail,
		OnCall: slices.Clone(w.onCall),
		Wage:   w.emtWage, MaxWage: MaxEMTWage, Paid: w.emtPaid,
		Cots: w.countTerrain(Cot), Share: w.emtShare, CheapestAsk: -1,
		Responses: w.emtResponses,
		Fee:       w.emtFee, MaxFee: MaxEMTFee, FeesPaid: w.emtFeesPaid, CallRate: w.emtCallRate,
		Package: w.emtWage + w.emtFeeIncome(),
	}
	if w.emtResponses > 0 {
		v.ResponseTicks = w.emtResponseTicks / w.emtResponses
	}
	if w.emtDetail == 0 {
		return v // nobody is asked: the market is not worth reckoning
	}
	// Map order does not matter: a minimum and a count.
	for id := range w.kindEntities[Colonist] {
		e := w.entities[id]
		if e == nil || !e.Alive() || e.onCall {
			continue
		}
		ask := w.emtAsk(e)
		if v.CheapestAsk < 0 || ask < v.CheapestAsk {
			v.CheapestAsk = ask
		}
		if ask <= w.emtWage {
			v.Willing++
		}
	}
	return v
}

package sim

import "slices"

// Emergency services (aid phase A4). The player sets an EMT detail: how many
// colonists to keep on call. Being on call is not a job a colonist stands at
// but a status it carries while it works: a colonist looking for work takes
// an open place on the detail and goes on choosing work as usual. While on
// call it hears of every casualty in the colony, not just the ones it can
// see, and helping one scores aid-duty-bonus more, enough to drop any work
// and its own pressing (not critical) needs. Only an EMT drags a colonist
// down from blood loss to an infirmary cot. The treasury pays each on-call
// colonist wage-emt for every hour on call and awake, as far as it goes,
// as it pays a guard. It keeps the place until it sleeps, goes down, becomes
// a casualty itself, or the detail is cut below it; then the next colonist
// looking for work takes it, so the detail staffs itself in shifts. See
// docs/emergency-services.md.

// MaxEMTDetail is the most colonists the player may keep on call.
const MaxEMTDetail = 20

// SetEMTDetail sets how many colonists the colony keeps on call as EMTs (0
// stands the detail down).
type SetEMTDetail struct {
	Detail int
}

func (SetEMTDetail) isCommand() {}

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

// tryTakeOnCall puts e on the detail if a place is open. assignWorkJob calls
// it for a colonist looking for work, which then goes on to choose work as
// usual: the place costs it nothing until a casualty comes up.
func (w *World) tryTakeOnCall(e *Entity) {
	if e.onCall || len(w.onCall) >= w.emtDetail || e.passedOut || e.Kind != Colonist || w.isCasualty(e) {
		return
	}
	i, _ := slices.BinarySearch(w.onCall, e.ID)
	w.onCall = slices.Insert(w.onCall, i, e.ID)
	e.onCall, e.emtTicks = true, 0
	w.markMindDirty(e) // a casualty may already be waiting
}

// releaseOnCall takes e off the detail.
func (w *World) releaseOnCall(e *Entity) {
	if i, found := slices.BinarySearch(w.onCall, e.ID); found {
		w.onCall = slices.Delete(w.onCall, i, i+1)
	}
	e.onCall, e.emtTicks = false, 0
}

// serveOnCall runs an on-call colonist's turn on the detail: it goes off
// call when it sleeps, goes down or is a casualty itself, and otherwise
// counts toward its next hour's pay.
func (w *World) serveOnCall(e *Entity) {
	if !e.onCall {
		return
	}
	if e.passedOut || e.State == Sleeping || w.isCasualty(e) {
		w.releaseOnCall(e)
		return
	}
	e.emtTicks++
	if e.emtTicks < w.cfg.TicksPerHour() {
		return
	}
	e.emtTicks = 0
	if pay := Money(w.cfg.WageEMT); pay > 0 && w.transfer(Community, ColonistOwner(e.ID), pay) {
		w.emtPaid += pay // as far as the treasury goes
	}
}

// EMTView is the EMT detail as a frontend shows it: how many it asks for,
// who is on call now, what one is paid an hour, what the colony has paid in
// all, and how many cots stand.
type EMTView struct {
	Detail, Max int
	OnCall      []EntityID
	Wage, Paid  Money
	Cots        int
}

func (w *World) publishedEMT() EMTView {
	return EMTView{
		Detail: w.emtDetail, Max: MaxEMTDetail,
		OnCall: slices.Clone(w.onCall),
		Wage:   Money(w.cfg.WageEMT), Paid: w.emtPaid,
		Cots: w.countTerrain(Cot),
	}
}

package sim

import "sort"

// Bodyguards: the top rung of safety's ladder (wants.go). A colonist rich
// enough, and afraid enough, posts an escort contract: a WorkEscort work
// order, escrowed from its own wallet, for escort-hours hours at an hourly
// rate. An armed colonist who feels safe takes it the way it takes a defense
// post, walks with the client within escort-radius, shoots anything that
// threatens it, and is paid by the hour on station. A bodyguard reads only
// the contract (its rate, hours, escrow) and where the client is: never the
// client's drives or mood. See docs/safety.md.

// escortContracts is every open escort contract, best paid first, then
// oldest: what an armed colonist looking for work weighs.
func (w *World) escortContracts() []*WorkOrder {
	out := w.sortedWork(func(o *WorkOrder) bool { return o.Kind == WorkEscort })
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pay > out[j].Pay })
	return out
}

// calmEnoughToGuard reports whether e may take or keep a guard post or an
// escort contract: only while its own safety is calm. Whoever is on station
// is, by construction, not afraid, which is why guards never need guards.
func calmEnoughToGuard(e *Entity) bool {
	return e.drives[DriveSafety].phase < DrivePressing
}

// tryAssignEscort puts an armed, calm colonist looking for work on the best
// paid open escort contract whose client is out of its home and reachable.
// It comes before defense posts: a contract never pays less than escort-wage,
// above wage-guard.
func (w *World) tryAssignEscort(e *Entity) bool {
	if bestWeapon(e.Inventory) == ItemNone || !calmEnoughToGuard(e) {
		return false
	}
	room := w.roomOf(e.Pos)
	for _, o := range w.escortContracts() {
		if o.escort != 0 {
			continue
		}
		c := w.entities[o.Issuer.ID]
		if c == nil || !c.Alive() || c.ID == e.ID || c.place == PlaceHome || w.roomOf(c.Pos) != room {
			continue
		}
		o.escort, c.escortedBy = e.ID, e.ID
		e.Job, e.Target, e.Progress = JobEscort, c.Pos, 0
		e.escortWork, e.guardHour = o.ID, 0
		hired := w.occurrence(e, ActionGuard, c, e.Pos, "")
		hired.ActorText = "Took on " + c.displayName() + " as a bodyguard's client."
		hired.TargetText = "Hired " + e.displayName() + " as a bodyguard."
		w.emitOccurrence(hired)
		return true
	}
	return false
}

// releaseEscort takes e off its contract. clearJob calls it. The contract
// stays open for the next calm, armed colonist, until its hours are served.
func (w *World) releaseEscort(e *Entity) {
	if o := w.workOrders[e.escortWork]; o != nil && o.escort == e.ID {
		o.escort = 0
		if c := w.entities[o.Issuer.ID]; c != nil && c.escortedBy == e.ID {
			c.escortedBy = 0
		}
	}
	e.escortWork, e.guardHour = 0, 0
}

// jobEscort is one tick of a bodyguard's duty: shoot an alien threatening the
// client, else keep within escort-radius of it, else stand by. It gives the
// contract up when it is no longer calm, when the client goes home, and when
// the contract is over.
func (w *World) jobEscort(e *Entity) {
	o := w.workOrders[e.escortWork]
	weapon := bestWeapon(e.Inventory)
	if o == nil || weapon == ItemNone || !calmEnoughToGuard(e) {
		w.clearJob(e)
		return
	}
	c := w.entities[o.Issuer.ID]
	if c == nil || !c.Alive() || c.place == PlaceHome {
		w.clearJob(e)
		return
	}
	e.resting = false
	if a, ok := w.nearestAlien(e.Pos, w.cfg.GuardSight); ok && a.Pos.Level == c.Pos.Level &&
		a.Pos.Chebyshev(c.Pos) <= w.cfg.GuardSight {
		w.fightAlien(e, a, weapon)
		w.serveEscort(e, o, c)
		return
	}
	if !w.withEscort(e, c) {
		e.State = Moving
		if e.Target != c.Pos {
			e.Target = c.Pos
			e.clearPath()
		}
		if _, ok := w.travelTo(e, e.Target); !ok {
			w.clearJob(e)
			return
		}
		w.serveEscort(e, o, c)
		return
	}
	e.State = Guarding
	w.serveEscort(e, o, c)
}

// withEscort reports whether bodyguard g is close enough to client c to
// guard it: on its level, within escort-radius.
func (w *World) withEscort(g, c *Entity) bool {
	return g.Pos.Level == c.Pos.Level && g.Pos.Chebyshev(c.Pos) <= w.cfg.EscortRadius
}

// serveEscort counts a tick on station beside the client: practice at
// guarding, and an hour's pay from the contract's escrow for every hour of
// it. The last hour closes the contract.
func (w *World) serveEscort(e *Entity, o *WorkOrder, c *Entity) {
	if !w.withEscort(e, c) {
		return
	}
	w.practise(e, SkillGuarding, 1)
	e.guardHour++
	if e.guardHour < w.cfg.TicksPerHour() {
		return
	}
	e.guardHour = 0
	o.served++
	w.payWork(o, e)
}

// escortedNow reports whether e's bodyguard is on station beside it.
func (w *World) escortedNow(e *Entity) bool {
	if e.escortedBy == 0 {
		return false
	}
	g := w.entities[e.escortedBy]
	return g != nil && g.Alive() && g.Job == JobEscort && w.withEscort(g, e)
}

// hireBodyguard posts e's escort contract: escort-hours hours at the most it
// will pay an hour, which must be at least escort-wage, and a whole shift at
// most escort-budget-percent of what it has beyond its food money. Nil if it
// cannot afford one.
func (w *World) hireBodyguard(e *Entity, limit Money, ttl int) *WorkOrder {
	hourly, ok := w.escortRate(e, limit)
	if !ok {
		return nil
	}
	o := w.postWork(WorkEscort, ColonistOwner(e.ID), hourly, w.cfg.EscortHours, e.Pos)
	if o == nil {
		return nil
	}
	o.lapseAt = w.tick + ttl
	return o

}

// escortRate is the hourly rate e would post at, given the most it is
// willing to pay an hour, and whether it can afford a contract at all.
func (w *World) escortRate(e *Entity, limit Money) (Money, bool) {
	hours := Money(max(1, w.cfg.EscortHours))
	floor := Money(w.cfg.EscortWage)
	budget := w.spendable(e) * Money(w.cfg.EscortBudgetPercent) / 100
	hourly := min(limit, budget/hours)
	return hourly, hourly >= floor && floor > 0
}

// spendable is what e has beyond its food money (want-reserve-meals).
func (w *World) spendable(e *Entity) Money {
	return max(0, e.wallet-Money(w.cfg.WantReserveMeals)*w.valueOf(Meal))
}

// lapseEscort closes an escort contract nobody has taken by its lapse tick,
// so its escrow comes back and the client can decide again.
func (w *World) lapseEscort(o *WorkOrder) bool {
	if o.escort == 0 && o.served == 0 && o.lapseAt > 0 && w.tick >= o.lapseAt {
		w.closeWork(o)
		return true
	}
	return false
}

// nervePercent is how much of its fear a colonist feels for its guarding
// rank: guarding-nerve-percent less for each rank, up to four. Skill, unlike
// wear, does not roll off: a veteran who has been home a month is still one.
func (w *World) nervePercent(e *Entity) int {
	return max(0, 100-w.cfg.GuardingNervePercent*min(e.nerveRank, 4))
}

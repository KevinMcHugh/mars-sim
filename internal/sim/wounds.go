package sim

import "fmt"

// Wounds, healing, bleeding and the infirmary (aid phases A3 and A5). Every
// hit that lands on a colonist's body part leaves the part's wound open, and
// an open wound never heals: that keeps the old rule (injuries are permanent)
// for a colonist nobody sees to. Dressing it (first aid, by anyone, or by the
// colonist itself at half speed; see casualty.go) lets it heal, a point at a
// time, while the colonist lies down: asleep in a bunk, faster in an
// infirmary cot, slower on the floor. A part destroyed outright never grows
// back; dressing it only closes it. With bleeding on, a bad open wound drains
// HP until it is dressed, and a colonist that has lost too much collapses
// where it stands: a casualty nobody can rouse, who comes round only once it
// has healed, and whom an EMT can drag to a cot. See
// docs/emergency-services.md.

// Wound is the state of the wound on one body part.
type Wound uint8

const (
	WoundNone    Wound = iota // whole, or healed
	WoundOpen                 // hit, and nobody has dressed it: it does not heal, and may bleed
	WoundDressed              // dressed: heals while the colonist lies down
)

func (w Wound) String() string {
	switch w {
	case WoundOpen:
		return "open"
	case WoundDressed:
		return "dressed"
	default:
		return ""
	}
}

// openWounds is how many of e's parts carry an open wound.
func (e *Entity) openWounds() int {
	n := 0
	for _, wd := range e.wounds {
		if wd == WoundOpen {
			n++
		}
	}
	return n
}

// badWound reports whether an open wound has taken a part below half, or
// destroyed it.
func (e *Entity) badWound() bool {
	for p, wd := range e.wounds {
		if wd == WoundOpen && e.MaxParts[p] > 0 && e.Parts[p]*2 < e.MaxParts[p] {
			return true
		}
	}
	return false
}

// wound opens the wound on part, for a colonist hit there. applyDamage calls
// it, so every source of damage that lands on a part leaves one.
func (e *Entity) wound(part BodyPart) {
	if e.Kind == Colonist && part < numBodyParts {
		if e.wounds[part] == WoundDressed {
			e.dressed--
		}
		e.wounds[part] = WoundOpen
	}
}

// hurt is the world's half of a colonist being hit: it may be a casualty
// now. strike and a fall call it after damage that did not kill.
func (w *World) hurt(e *Entity) {
	if e.Kind == Colonist && e.Alive() {
		w.refreshCasualty(e)
	}
}

// dressWound dresses c's first open wound, as helper. A destroyed part is
// only closed: it will not grow back. The helper's medicine rank sets how
// fast what it dressed heals.
func (w *World) dressWound(helper, c *Entity) {
	for p := range c.wounds {
		if c.wounds[p] != WoundOpen {
			continue
		}
		if c.Parts[p] <= 0 || c.Parts[p] >= c.MaxParts[p] {
			c.wounds[p] = WoundNone
		} else {
			c.wounds[p] = WoundDressed
			c.dressed++
		}
		if pct := w.skillEffect(helper, SkillMedicine).YieldPct; pct > c.healPct {
			c.healPct = pct
		}
		if helper != c {
			helper.aidDone.dressed++
		}
		break
	}
	w.refreshCasualty(c)
}

// healWounds heals e's dressed wounds while it lies down: asleep in a bunk at
// heal-per-hour, in an infirmary cot at heal-cot of that, down on the floor at
// heal-floor of it. Points go one at a time to the dressed part furthest from
// whole (ties to the lower part), and with each the HP pool too; a part made
// whole is healed. Exact over time: an accumulator, not a roll.
func (w *World) healWounds(e *Entity) {
	if e.dressed == 0 || !(e.State == Sleeping || e.passedOut) {
		return
	}
	pct := 100
	switch {
	case e.inCot:
		pct = w.cfg.HealCotPct
	case e.passedOut:
		pct = w.cfg.HealFloorPct
	}
	if e.healPct > 0 {
		pct = pct * e.healPct / 100
	}
	// The accumulator counts hundredths of an HP times ticks an hour, so the
	// rate is exact however the hour divides.
	point := 100 * max(1, w.cfg.TicksPerHour())
	e.healAcc += w.cfg.HealPerHour * pct
	for e.healAcc >= point && e.dressed > 0 {
		e.healAcc -= point
		best := -1
		for p := range e.wounds {
			if e.wounds[p] != WoundDressed {
				continue
			}
			if best < 0 || e.Parts[p]*e.MaxParts[best] < e.Parts[best]*e.MaxParts[p] {
				best = p
			}
		}
		e.Parts[best]++
		e.HP = min(e.HP+1, e.MaxHP)
		if e.Parts[best] >= e.MaxParts[best] {
			e.wounds[best] = WoundNone
			e.dressed--
		}
	}
	if e.dressed == 0 {
		e.healAcc, e.healPct = 0, 0
	}
}

// bleedRate is how fast e's open wounds bleed, in hundredths of an HP an
// hour: every open wound below bleed-threshold of its part bleeds
// bleed-rate hundredths an hour for each point the part has lost. Nothing
// bleeds with bleeding off.
func (w *World) bleedRate(e *Entity) int {
	if !w.cfg.Bleeding || e.Kind != Colonist {
		return 0
	}
	rate := 0
	for p, wd := range e.wounds {
		if wd == WoundOpen && e.Parts[p]*100 < e.MaxParts[p]*w.cfg.BleedThresholdPct {
			rate += (e.MaxParts[p] - e.Parts[p]) * w.cfg.BleedPct
		}
	}
	return rate
}

// bleed drains e's HP for its open wounds, unless a helper beside it is
// holding them (aiding it this tick). It reports whether e bled, so a death
// this turn is put down to blood. Losing too much drops it where it stands.
func (w *World) bleed(e *Entity) bool {
	rate := w.bleedRate(e)
	if rate == 0 {
		e.bleedAcc = 0
		return false
	}
	if h := w.entities[e.aidedBy]; h != nil && h != e && h.State == Aiding && h.Pos.Adjacent(e.Pos) {
		return false
	}
	point := 100 * max(1, w.cfg.TicksPerHour())
	e.bleedAcc += rate
	for e.bleedAcc >= point && e.HP > 0 {
		e.bleedAcc -= point
		e.HP--
	}
	if e.Alive() && !e.passedOut && e.HP*100 < e.MaxHP*w.cfg.BleedCollapsePct {
		w.collapseFromBloodLoss(e)
	}
	return true
}

// collapseFromBloodLoss drops e where it stands, as passing out does, but
// nobody can rouse it: it comes round on its own once it has healed back
// above bleed-wake and stopped bleeding (stayPassedOut).
func (w *World) collapseFromBloodLoss(e *Entity) {
	w.clearJob(e)
	e.focus, e.focusSince = FocusIdle, w.tick
	e.resting = false
	e.clearPath()
	e.passedOut, e.downBled = true, true
	e.State = PassedOut
	e.driveActivity = DriveUnconscious
	w.refreshDrives(e)
	o := w.occurrence(e, ActionCollapse, nil, e.Pos, "Collapsed from blood loss.")
	w.emitOccurrence(o)
	w.logEvent(LogNote, fmt.Sprintf("%s collapsed from blood loss.", e.displayName()))
	w.markMindDirty(e)
	w.refreshCasualty(e)
}

// stillDownFromBloodLoss reports whether a colonist down from blood loss
// stays down: below bleed-wake of its HP, or still bleeding.
func (w *World) stillDownFromBloodLoss(e *Entity) bool {
	return e.HP*100 < e.MaxHP*w.cfg.BleedWakePct || w.bleedRate(e) > 0
}

// ---- The infirmary -----------------------------------------------------------

// infirmaryRoom is a row of cots. The colony wants one only while it keeps
// EMTs on call: a cot is where an EMT lays a colonist nobody can rouse.
var infirmaryRoom = roomRecipe{
	name: "infirmary", kinds: []Terrain{Cot}, minFac: 2, maxFac: roomFacilities,
	planLog: "The colony commissions an infirmary.",
}

// cotsShort is how many more cots the colony wants than it has planned: one
// for each colonists-per-cot colonists, never fewer than an infirmary's
// minimum, and none at all without an EMT detail.
func (w *World) cotsShort() int {
	per := w.cfg.ColonistsPerCot
	n := w.countKind(Colonist)
	if per <= 0 || w.emtDetail <= 0 || n < 2 {
		return 0
	}
	want := max((n+per-1)/per, infirmaryRoom.minFac)
	return max(0, want-w.plannedFacilities(Cot))
}

// freeCot is the nearest cot c could be laid beside: on its level, in its
// room, with nobody else laid there or on the way (one already claimed for c
// counts, or an EMT halfway there would find no cot and give up). Ties go to the lower point, so
// the answer does not depend on the order cots are visited in.
func (w *World) freeCot(c *Entity) (Point, bool) {
	var best Point
	bestDist, found := 0, false
	w.eachFacility(Cot, func(p Point) {
		if p.Level != c.Pos.Level || !w.sameRoomAdjacent(c.Pos, p) {
			return
		}
		if id, taken := w.cotClaims[p]; taken && id != c.ID {
			return // a cot claimed for c is still c's
		}
		d := w.travelEstimate(c.Pos, p)
		if !found || d < bestDist || d == bestDist && lessPoint(p, best) {
			best, bestDist, found = p, d, true
		}
	})
	return best, found
}

// sameRoomAdjacent reports whether a walkable tile beside p (a cot, which
// blocks movement) is in from's room.
func (w *World) sameRoomAdjacent(from, p Point) bool {
	for _, d := range neighbors8 {
		n := p.Add(d.X, d.Y)
		if w.Walkable(n) && w.sameRoom(from, n) {
			return true
		}
	}
	return false
}

// layInCot lays c beside cot at: its hunger held while it lies there, and
// its wounds healing at the cot's rate.
func (w *World) layInCot(c *Entity, at Point) {
	c.inCot, c.cotAt = true, at
	w.cotClaims[at] = c.ID
	w.refreshDrive(c, DriveFood, 0)
}

// leaveCot takes c out of its cot, if it is in one, and lets the cot go.
func (w *World) leaveCot(c *Entity) {
	if !c.inCot {
		return
	}
	if w.cotClaims[c.cotAt] == c.ID {
		delete(w.cotClaims, c.cotAt)
	}
	c.inCot, c.cotAt = false, Point{}
	w.refreshDrive(c, DriveFood, 0)
}

// dragStep moves an EMT's casualty, down from blood loss, one step toward a
// free cot, the EMT stepping into the tile it left so the two stay side by
// side. The casualty keeps its own tile in the occupancy index the whole
// way: nothing ever shares a tile. It goes at drag-speed, waits while the
// way is blocked, and gives up if the way is shut for good or leaves the
// level. It reports whether the job is over (laid in a cot, or given up).
func (w *World) dragStep(e, c *Entity) (over bool) {
	if e.dragPath == nil {
		cot, ok := w.freeCot(c)
		if !ok {
			return true
		}
		route, found := w.pathToAdjacent(c.Pos, cot)
		if !found {
			return true
		}
		w.cotClaims[cot] = c.ID
		e.dragCot, e.dragPath, e.dragAt, e.stuck = cot, route, 0, 0
	}
	if c.Pos.Adjacent(e.dragCot) {
		delete(w.cotClaims, e.dragCot)
		w.layInCot(c, e.dragCot)
		e.dragPath = nil
		e.aidDone.dragged = true
		return true
	}
	e.dragAcc += w.cfg.DragSpeedPct
	if e.dragAcc < 100 {
		return false
	}
	e.dragAcc -= 100
	for e.dragAt < len(e.dragPath) && e.dragPath[e.dragAt].Equal(c.Pos) {
		e.dragAt++
	}
	if e.dragAt >= len(e.dragPath) {
		w.releaseDrag(e)
		return true
	}
	next := e.dragPath[e.dragAt]
	if next.Level != c.Pos.Level || !w.Walkable(next) {
		w.releaseDrag(e)
		return true
	}
	from := c.Pos
	switch blocker := w.entityAt(next); {
	case blocker == e:
		w.swapEntities(e, c)
	case blocker == nil:
		w.moveEntity(c, next)
		if !e.Pos.Equal(from) {
			w.moveEntity(e, from)
		}
	default:
		e.stuck++
		if e.stuck > w.cfg.StuckLimit {
			w.releaseDrag(e)
			return true
		}
		return false
	}
	e.stuck = 0
	e.dragAt++
	w.practise(e, SkillMedicine, 1)
	return false
}

// releaseDrag drops an EMT's drag: its route, and the cot it had claimed for
// a casualty not yet laid there.
func (w *World) releaseDrag(e *Entity) {
	if e.dragPath != nil {
		if id, ok := w.cotClaims[e.dragCot]; ok && id == e.aidTarget {
			if c := w.entities[id]; c == nil || !c.inCot || c.cotAt != e.dragCot {
				delete(w.cotClaims, e.dragCot)
			}
		}
	}
	e.dragPath, e.dragAt, e.dragAcc, e.dragCot = nil, 0, 0, Point{}
}

// swapEntities exchanges the tiles of two entities on one level, keeping the
// occupancy and chunk indexes in step. moveEntity cannot: it needs its
// destination free.
func (w *World) swapEntities(a, b *Entity) {
	ap, bp := a.Pos, b.Pos
	l := w.lay(ap)
	l.occ.set(ap.X, ap.Y, b.ID)
	l.occ.set(bp.X, bp.Y, a.ID)
	if ac, bc := w.chunkIndexOf(ap), w.chunkIndexOf(bp); ac != bc {
		w.removeFromChunkIndex(l, ac, a.ID)
		w.removeFromChunkIndex(l, bc, b.ID)
		l.chunkEntities[bc] = append(l.chunkEntities[bc], a.ID)
		l.chunkEntities[ac] = append(l.chunkEntities[ac], b.ID)
	}
	a.Pos, b.Pos = bp, ap
}

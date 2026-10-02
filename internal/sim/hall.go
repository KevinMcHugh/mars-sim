package sim

// ---- The meeting hall -------------------------------------------------------------
//
// Colonists used to "socialize" wherever they happened to be: a social need
// that crossed its threshold made them look for anyone idle within talk-radius,
// and if nobody was, they stood there, need pinned, with nothing on the map to
// show whether they were chatting or only hoping to. The colony now
// commissions a meeting hall, a walled room of chairs (hallRoom), and the hall
// is where company is. A colonist who wants to socialize walks to a chair and
// waits beside it until a partner who is also in the hall is free; a colonist
// with a meal takes it there to eat. Nothing is a fixture or a claim: a hall
// is just its chairs, and "in the hall" is a distance from one. See
// docs/meeting-hall.md.

// hallRoom is a row of chairs along the back wall with open floor in front.
// Two chairs is the smallest hall worth the walls; four fill the usual bay.
// The planner asks for chairs, not rooms: see wantsHall.
var hallRoom = roomRecipe{
	name: "meeting hall", kinds: []Terrain{Chair}, minFac: 2, maxFac: roomFacilities,
	planLog: "The colony commissions a meeting hall.", structure: StructMeetingHall,
}

const (
	// hallReach is how near a chair a tile is part of the hall: the open floor
	// of a standard room, from the row beside the chairs to the one before the
	// front wall.
	hallReach = 2
	// chairShare is how many colonists crowd one chair. Chairs are not claimed
	// (nothing tracks who sits where, so nothing can leak a claim), but a
	// colonist looking for a seat passes over a chair that already has this
	// many beside it, which spreads a crowd along the row.
	chairShare = 2
)

// wantsHall reports whether the colony wants chairs it has not planned: one
// for each colonists-per-chair colonists, and never fewer than a hall's
// minimum.
func (w *World) wantsHall() bool {
	per := w.cfg.ColonistsPerChair
	n := w.countKind(Colonist)
	if per <= 0 || n < 2 {
		return false
	}
	want := max((n+per-1)/per, hallRoom.minFac)
	return w.plannedFacilities(Chair) < want
}

// hallOpen reports whether there is a hall to go to: the feature is on and a
// chair stands.
func (w *World) hallOpen() bool {
	return w.cfg.ColonistsPerChair > 0 && w.countTerrain(Chair) > 0
}

// nearChair reports whether p is within r tiles of a chair. The chair set is
// a handful of tiles, and the answer does not depend on their order.
func (w *World) nearChair(p Point, r int) bool {
	for c := range w.facilityTiles[Chair] {
		if p.Chebyshev(c) <= r {
			return true
		}
	}
	return false
}

// inHall reports whether p is in the meeting hall.
func (w *World) inHall(p Point) bool { return w.nearChair(p, hallReach) }

// seated reports whether p is right beside a chair.
func (w *World) seated(p Point) bool { return w.nearChair(p, 1) }

// chairCrowd counts the colonists standing beside the chair at c.
func (w *World) chairCrowd(c Point) int {
	n := 0
	for _, d := range neighbors8 {
		if o := w.entityAt(c.Add(d.X, d.Y)); o != nil && o.Kind == Colonist {
			n++
		}
	}
	return n
}

// nearestChair finds the chair e should walk to: the nearest one within
// hall-range that it can reach and that is not already crowded. Distance is
// straight-line, ties by position, so map order never decides it.
func (w *World) nearestChair(e *Entity) (Point, bool) {
	room := w.roomOf(e.Pos)
	var best Point
	bestDist, found := 1<<30, false
	for c := range w.facilityTiles[Chair] {
		d := e.Pos.Chebyshev(c)
		if d > w.cfg.HallRange || (found && d > bestDist) ||
			!w.taskReachable(c, room) || w.chairCrowd(c) >= chairShare {
			continue
		}
		if !found || d < bestDist || lessPoint(c, best) {
			best, bestDist, found = c, d, true
		}
	}
	return best, found
}

// socializeAtHall is a colonist's turn when it wants company and a hall
// stands, reporting whether it spent the turn there. False sends it back to
// the old way: any idle colonist within talk-radius, wherever that is. That
// is the answer when the hall is out of reach, full, or not built, and a
// colonist whose route to it is blocked.
//
// A colonist away from a seat walks to one. Seated, it takes the nearest
// free colonist in the hall; with none, it waits, in State Idle with its
// focus still set to socialize, so a newcomer finds it. Both halves matter:
// pairing only hall-goers is what makes the hall a place people meet rather
// than a place two people who were already together walked to.
func (w *World) socializeAtHall(e *Entity) bool {
	if e.Kind != Colonist || !w.hallOpen() {
		return false
	}
	if !w.seated(e.Pos) {
		if chair, ok := w.nearestChair(e); ok {
			if _, ok := w.travelTo(e, chair); !ok {
				return false
			}
			e.State = Moving
			return true
		}
		if !w.inHall(e.Pos) {
			return false // no seat to be had and not in the hall at all
		}
	}
	partner, ok := w.nearestMatch(e.Pos, w.cfg.TalkRadius, func(o *Entity) bool {
		return o.Kind == Colonist && o.ID != e.ID && w.inHall(o.Pos) && w.availableToTalk(o)
	})
	if !ok {
		e.State = Idle // in the hall, waiting for company
		return true
	}
	w.beginTalk(e, partner)
	w.runJob(e)
	return true
}

// hallTalkBonus is the quality a conversation gains from being held in the
// hall: both partners in it, since a chat one of them walked away from to take
// is not a meeting.
func (w *World) hallTalkBonus(a, b *Entity) int {
	if w.hallOpen() && w.inHall(a.Pos) && w.inHall(b.Pos) {
		return w.cfg.HallTalkBonus
	}
	return 0
}

// mealSeat is the chair e takes a meal in hand to, if it should: a colonist
// not yet critically hungry (one that is eats where it stands, since the
// walk is time it may not have), with a hall to go to. A meal eaten for later
// (eatKeep) never goes.
func (w *World) mealSeat(e *Entity) (Point, bool) {
	if e.Kind != Colonist || e.eatKeep || !w.hallOpen() || e.needPhase[NeedFood] >= NeedCritical {
		return Point{}, false
	}
	return w.nearestChair(e)
}

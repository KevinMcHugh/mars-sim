package sim

import "slices"

// Choosing which facility of a kind a colonist commits to. See docs/needs.md
// ("Spreading users across facilities") for what the choice is and why it
// avoids congested facilities.
//
// The answer is the nearest uncongested reachable facility by walking
// distance (ties to the lower lessPoint), else the nearest reachable facility
// at all, else the zero Point. It used to be computed by a BFS over everything
// the colonist could reach, followed by a scan of every facility. On a big
// colony that flood was a third of a real profile: it covered the whole
// reachable map on every need decision, even when the pod was three tiles
// away. Now there are up to three steps, cheapest first, and all of them
// produce that same answer (TestChooseFacilityMatchesReference checks it
// against the old version):
//
//  1. facilityByField reads the facility kind's shared flow field, which
//     already holds every tile's distance D to the nearest facility of that
//     kind. Walking only downhill from the colonist visits exactly the tiles
//     on shortest routes to the facilities at distance D, so it finds all the
//     nearest facilities without looking anywhere else. If one of them is
//     free, that is the answer.
//  2. If they're all busy, anyFreeFacility checks whether any reachable
//     facility is free. If none is, the answer is the nearest one, which step
//     1 already found.
//  3. Otherwise facilityBySearch runs the old BFS from the colonist, but layer
//     by layer, and stops at the first layer that reaches a free facility
//     rather than flooding everything.
//
// The shared field can't simply store which facility is nearest to each tile.
// The field is kept up to date by incremental repair (flowrepair.go), and a
// repair only follows distance changes. A new facility can change which
// facility is nearest across a whole region without changing any distance, so
// a stored label would silently go stale. Getting the nearest set from the
// distances themselves has no such problem.
//
// Reachability of a facility's other access tiles (for the occupancy half of
// "congested") is judged by room: a tile the colonist can walk to is in the
// colonist's room. Rooms, like the shared field, are refreshed between ticks,
// not mid-tick, so for the rest of a tick in which terrain changed, both can
// lag the old per-call BFS. That is deterministic, and it's the same view of
// the map the colonist navigates by.
//
// Ownership (see docs/property.md): a colonist never picks a fixture it may
// not use, and the shared field leads only to communal fixtures, so the field
// cannot see a colonist's own bunk — which may well be nearer than the nearest
// communal one. While any fixture of a kind is restricted, then, the field
// tiers are skipped and the bounded search runs, skipping what the colonist
// may not use. With crash pods that is every bunk and toilet choice; the
// search stops at the first usable free facility, usually the colonist's own
// a few tiles away, so it stays cheap.

// chooseFacility assigns a concrete facility to a need. The assignment is
// retained on the entity for the whole use job, so a user never ping-pongs
// between queues as their counts change.
func (w *World) chooseFacility(e *Entity, kind Terrain) Point {
	w.facilityCommitted = nil // counted on demand, once per call
	room := w.roomOf(e.Pos)
	if room == 0 || w.restrictedFixtures[kind] > 0 {
		return w.facilityBySearch(e, kind, room)
	}
	nearest, free := w.facilityByField(e, kind, room)
	if free {
		return nearest
	}
	// Every nearest facility is busy. If every other reachable one is too,
	// the answer is the nearest anyway, and the search below would flood the
	// whole reachable map looking for a free facility that doesn't exist.
	// In a colony short of facilities, which is exactly when many colonists
	// are choosing at once, that is the common case.
	if len(w.facilityFound) > 0 && !w.anyFreeFacility(e, kind, room) {
		return nearest
	}
	return w.facilityBySearch(e, kind, room)
}

// facilityCongested reports whether fac, a facility e can reach, is contended:
// something occupies one of its reachable access tiles, or another colonist is
// already committed to using it.
//
// This, not mere nearby foot traffic, is what "congested" means. An earlier
// version also flagged a facility whenever any other colonist stood within one
// tile of any of its access tiles, whatever that colonist was doing.
// Facilities are packed one tile apart in a room (see construction.md), so
// their access neighborhoods overlap. In a merely busy room (colonists
// resting, chatting, walking through, using the facility next door) that
// overbroad check could flag every facility in it as congested at once. The
// choice then fell back to "nearest for everyone," funneling a crowd onto one
// facility while others sat idle beside it.
//
// The commitment scan is over every entity, so callers evaluate it only for
// the few facilities that are actually in contention for the answer. The old
// version ran it once per access tile of every facility, which on a crowded
// colony cost more than its whole BFS.
func (w *World) facilityCongested(e *Entity, fac Point, reachable func(Point) bool) bool {
	for _, d := range neighbors8 {
		access := fac.Add(d.X, d.Y)
		if w.Walkable(access) && reachable(access) && w.entityAt(access) != nil {
			return true
		}
	}
	n := w.committedUsers()[fac]
	if committedTo(e, fac) {
		n-- // e's own commitment doesn't make it wait for itself
	}
	return n > 0
}

// committedUsers counts, per facility, the colonists committed to using it. It
// is built on first use in each chooseFacility call (one pass over the
// entities) and reused for every facility that call judges. The old version
// scanned every entity once per access tile of every facility, which on a
// crowded colony cost more than its whole BFS.
func (w *World) committedUsers() map[Point]int {
	if w.facilityCommitted != nil {
		return w.facilityCommitted
	}
	if w.facilityCommittedBuf == nil {
		w.facilityCommittedBuf = make(map[Point]int)
	}
	m := w.facilityCommittedBuf
	clear(m)
	for _, other := range w.entities {
		if committedTo(other, other.useFacility) {
			m[other.useFacility]++
		}
	}
	w.facilityCommitted = m
	return m
}

// committedTo reports whether e is a living colonist committed to using fac.
func committedTo(e *Entity, fac Point) bool {
	return e.Alive() && e.Kind == Colonist && e.Job == JobUse &&
		e.useFacilitySet && e.useFacility.Equal(fac)
}

// anyFreeFacility reports whether any facility of kind that e can reach (one
// with a walkable access tile in e's room) is uncongested.
func (w *World) anyFreeFacility(e *Entity, kind Terrain, room RoomID) bool {
	inRoom := func(p Point) bool { return w.roomOf(p) == room }
	for fac := range w.facilityTiles[kind] { // order-free: the result is a bool
		if !w.canUseFixture(e, fac) {
			continue
		}
		reachable := false
		for _, d := range neighbors8 {
			if a := fac.Add(d.X, d.Y); w.Walkable(a) && inRoom(a) {
				reachable = true
				break
			}
		}
		if reachable && !w.facilityCongested(e, fac, inRoom) {
			return true
		}
	}
	return false
}

// facilityByField is the first tier. It collects the facilities of kind at the
// nearest distance into w.facilityFound (empty if the field reaches none from
// here). If one of them is uncongested, it returns the most preferred such
// one and free=true. Otherwise it returns the most preferred of them, busy
// as it is, for chooseFacility's fallback.
func (w *World) facilityByField(e *Entity, kind Terrain, room RoomID) (fac Point, free bool) {
	w.facilityFound = w.facilityFound[:0]
	f := w.facilityField(kind)
	if f == nil || f.at(e.Pos) < 0 {
		return Point{}, false
	}
	w.facilityGen++
	gen := w.facilityGen
	cells := &w.facilityCells
	cells.set(e.Pos.X, e.Pos.Y, flowCell{gen: gen})
	queue := append(w.facilityQueue[:0], e.Pos)
	nearest := w.facilityFound[:0]
	// A tile at field distance d > 0 lies on a shortest route to a nearest
	// facility exactly when the route continues through a neighbor at d-1.
	// Following only those steps visits the union of all shortest routes
	// and nothing else, and every tile where they end (d == 0) is an access
	// tile of a facility at the colonist's nearest distance. No shortest
	// route passes through a goal tile before its end, since that tile would
	// be closer, so stopping at d == 0 loses nothing.
	for head := 0; head < len(queue); head++ {
		p := queue[head]
		d := f.at(p)
		if d == 0 {
			for _, n := range neighbors8 {
				if fac := p.Add(n.X, n.Y); w.TerrainAt(fac) == kind {
					nearest = append(nearest, foundFacility{fac: fac})
				}
			}
			continue
		}
		for _, n := range neighbors8 {
			q := p.Add(n.X, n.Y)
			if !w.Walkable(q) || f.at(q) != d-1 {
				continue
			}
			c := cells.ptr(q.X, q.Y)
			if c.gen == gen {
				continue
			}
			c.gen = gen
			queue = append(queue, q)
		}
	}
	w.facilityQueue = queue
	w.facilityFound = nearest

	if len(nearest) == 0 {
		return Point{}, false
	}
	// A facility is reached once per access tile at the nearest distance;
	// judge it once.
	slices.SortFunc(nearest, compareFound)
	nearest = slices.Compact(nearest)
	w.facilityFound = nearest
	inRoom := func(p Point) bool { return w.roomOf(p) == room }
	if fac, ok := w.firstFree(e, nearest, inRoom); ok {
		return fac, true
	}
	return nearest[0].fac, false
}

// foundFacility is a facility some search reached, and how far away it is.
type foundFacility struct {
	fac  Point
	dist int32
}

// compareFound orders by distance, then by lessPoint: the order in which
// chooseFacility prefers facilities.
func compareFound(a, b foundFacility) int {
	switch {
	case a.dist != b.dist:
		return int(a.dist - b.dist)
	case lessPoint(a.fac, b.fac):
		return -1
	case lessPoint(b.fac, a.fac):
		return 1
	}
	return 0
}

// facilityBySearch is the second tier: a BFS outward from the colonist that
// stops at the first distance at which it reaches an uncongested facility. If
// it never reaches one, it returns the nearest facility it reached at all:
// choosing the nearest even when it's busy still guarantees progress once its
// current users leave, rather than declaring the need unreachable and starving
// the colonist.
//
// room is the colonist's room, or 0 if it isn't standing on floor. Then there
// is no room to judge reachability by, so the search runs to exhaustion and
// uses what it reached instead, as the old version always did.
func (w *World) facilityBySearch(e *Entity, kind Terrain, room RoomID) Point {
	w.facilityGen++
	gen := w.facilityGen
	cells := &w.facilityCells
	stamped := func(p Point) bool { return cells.at(p.X, p.Y).gen == gen }
	reachable := func(p Point) bool { return w.roomOf(p) == room }
	exhaustive := room == 0
	if exhaustive {
		reachable = stamped
	}

	cells.set(e.Pos.X, e.Pos.Y, flowCell{gen: gen})
	queue := append(w.facilityQueue[:0], e.Pos)
	found := w.facilityFound[:0]
	// A facility's distance is that of its nearest reached access tile, and
	// an access tile is a walkable tile beside it. The BFS reads every
	// neighbor's terrain anyway, so it spots a facility while expanding the
	// access tile it first touches: the first layer that does so is the
	// facility's distance. Facility tiles are never walkable, so their own
	// cells are free to mark "already found" with the generation stamp.
	startWalkable := w.Walkable(e.Pos)
	pd, levelEnd, layerStart := int32(0), len(queue), 0
	for head := 0; head < len(queue); head++ {
		if head == levelEnd {
			if !exhaustive {
				if fac, ok := w.firstFree(e, found[layerStart:], reachable); ok {
					w.facilityQueue, w.facilityFound = queue, found
					return fac
				}
			}
			layerStart = len(found)
			pd++
			levelEnd = len(queue)
		}
		p := queue[head]
		isAccess := head > 0 || startWalkable
		page := cells.interiorPage(p.X, p.Y)
		for _, d := range neighbors8 {
			n := p.Add(d.X, d.Y)
			if !w.InBounds(n) {
				continue
			}
			t := w.tiles[w.index(n)].Terrain
			if t == kind && isAccess {
				if c := cells.ptr(n.X, n.Y); c.gen != gen {
					c.gen = gen
					if w.canUseFixture(e, n) { // never someone else's private fixture
						found = append(found, foundFacility{fac: n, dist: pd})
					}
				}
				continue
			}
			if !t.Walkable() {
				continue
			}
			np := page
			if np == nil {
				np = cells.pageAtAlloc(n.X, n.Y)
			}
			c := &np[offset(n.X, n.Y)]
			if c.gen == gen {
				continue
			}
			c.gen = gen
			queue = append(queue, n)
		}
	}
	w.facilityQueue, w.facilityFound = queue, found

	// found is in non-decreasing distance order, so the search either
	// returns above at the first layer with a free facility, or falls
	// through here with every reached facility congested (or, exhaustive,
	// not yet judged).
	if exhaustive {
		if fac, ok := w.firstFree(e, found, reachable); ok {
			return fac
		}
	} else if fac, ok := w.firstFree(e, found[layerStart:], reachable); ok {
		return fac
	}
	if len(found) == 0 {
		return Point{}
	}
	return slices.MinFunc(found, compareFound).fac
}

// firstFree returns the most preferred uncongested facility among found,
// sorting found in place into preference order.
func (w *World) firstFree(e *Entity, found []foundFacility, reachable func(Point) bool) (Point, bool) {
	slices.SortFunc(found, compareFound)
	for _, c := range found {
		if !w.facilityCongested(e, c.fac, reachable) {
			return c.fac, true
		}
	}
	return Point{}, false
}

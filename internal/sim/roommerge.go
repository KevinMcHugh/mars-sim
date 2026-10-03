package sim

import (
	"fmt"
	"slices"
)

// ---- Joining rooms ----------------------------------------------------------------
//
// Expansion grows a room into open ground, and stops at the first wall in its
// way. Very often that wall is a room of the same kind: the colony marked out
// a second incubator room while the first was still going up, or the planner
// sited a new one flush against the old because a shared wall is good
// backing. Two rooms then stand side by side, each with its own doorway, its
// own aisle and walls on every side, and neither can ever grow, since each is
// in the other's way.
//
// A merger joins them. It tears down the wall between them (both walls, when
// they stand back to back or with a lane between), takes in that lane, and
// fits new fixtures in the space where they now fit. The two records become
// one room, and so does their structure. See docs/room-expansion.md.

// maxMergeLane is the most columns of open ground between two rooms' side
// walls that a merger takes in.
const maxMergeLane = 3

// roomMerger is two rooms of one recipe side by side: l on the left in l's
// frame, r on its right, facing the same way with their back walls in one
// row. d is how far r's left wall is from l's right wall, in columns: 0 is
// one wall the two share, 1 two walls back to back, and more leaves d-1
// columns of ground between them.
type roomMerger struct {
	l, r *roomRecord
	d    int
}

// wallU are the frame columns (in l's frame) of the walls a merger tears
// down: l's right wall, and r's left wall when it is a different one.
func (m roomMerger) wallU() []int {
	lw := m.l.f.width
	if m.d == 0 {
		return []int{lw}
	}
	return []int{lw, lw + m.d}
}

// laneU are the columns of ground between the two walls.
func (m roomMerger) laneU() (u0, u1 int) { return m.l.f.width + 1, m.l.f.width + m.d - 1 }

// frame is the joined room's frame: l's, out to r's right wall.
func (m roomMerger) frame() roomFrame {
	f := m.l.f
	f.width = m.l.f.width + 1 + m.d + m.r.f.width
	return f
}

// fixtureU are the columns of the new fixtures the merger can fit, no more
// than most, in whole cycles of the recipe's kinds: from two past l's last
// fixture to two short of r's first, so every fixture keeps the floor either
// side of it that the bay spacing gives.
func (m roomMerger) fixtureU(most int) []int {
	off := m.l.recipe.bayOffset()
	last := m.l.f.width - 1 - off
	first := m.l.f.width + 1 + m.d + off
	step := len(m.l.recipe.kinds)
	n := (first-last)/2 - 1
	n = min(n, most) / step * step
	var us []int
	for i := 1; i <= n; i++ {
		us = append(us, last+2*i)
	}
	return us
}

// local is the frame coordinates of the map tile p: at's inverse.
func (f roomFrame) local(p Point) (u, v int) {
	switch f.face {
	case faceNorth:
		return p.X - f.o.X, f.o.Y - p.Y
	case faceEast:
		return p.Y - f.o.Y, p.X - f.o.X
	case faceWest:
		return p.Y - f.o.Y, f.o.X - p.X
	default:
		return p.X - f.o.X, p.Y - f.o.Y
	}
}

// besideRoom reports whether a and b stand side by side close enough to
// join, and if so how: facing the same way, back walls in one row, and no
// more than maxMergeLane columns of ground between their side walls.
func besideRoom(a, b *roomRecord) (roomMerger, bool) {
	if a.f.face != b.f.face {
		return roomMerger{}, false
	}
	u, v := a.f.local(b.f.o)
	if v != 0 {
		return roomMerger{}, false
	}
	m := roomMerger{l: a, r: b, d: u - a.f.width - 1}
	if u < 0 {
		u, _ = b.f.local(a.f.o)
		m = roomMerger{l: b, r: a, d: u - b.f.width - 1}
	}
	return m, m.d >= 0 && m.d <= maxMergeLane+1
}

// mergeRooms joins two of the colony's rooms of r that stand side by side,
// fitting up to want new fixtures between them, and reports whether it did.
// A merger that fits fewer than least is passed over: the planner asks for at
// least one when it wants fixtures, and none when it is only tidying up.
//
// Pairs are tried oldest room first. Both rooms must be the colony's, built
// alike (both with an aisle or both without, so the joined bay's two ends
// agree with the recipe), idle, whole cycles of the recipe's kinds, and no
// longer together than room-max-facilities.
func (w *World) mergeRooms(r roomRecipe, want, least int, st roomPlanState) bool {
	if !r.expands {
		return false
	}
	limit := w.cfg.RoomMaxFacilities
	step := len(r.kinds)
	var recs []*roomRecord
	for _, rec := range w.roomRecords {
		if rec.recipe.name == r.name && rec.issuer == Community && !st.busy[rec] && rec.n%step == 0 {
			recs = append(recs, rec)
		}
	}
	for i, a := range recs {
		for _, b := range recs[i+1:] {
			if a.recipe.aisle != b.recipe.aisle || a.n+b.n > limit {
				continue
			}
			m, ok := besideRoom(a, b)
			if !ok {
				continue
			}
			us := m.fixtureU(min(want, limit-a.n-b.n))
			if len(us) < least || !w.mergerClear(m, st) || !w.siteKeepsColonyWhole(m.frame(), st.designated) {
				continue
			}
			if w.designateMerger(m, us) {
				return true
			}
		}
	}
	return false
}

// mergerClear reports whether m can be built. Both walls coming down must be
// standing whole and no project's. The lane between them must be open,
// discovered floor or rock to dig, in no third room, on no reserved doorway,
// zoned for the room, and claimed by no project, except that a wall already
// standing where the joined room's back or front wall runs serves as it is.
// A new back or front wall tile is never raised against a standing or planned
// wall, as expansionClear never raises one.
func (w *World) mergerClear(m roomMerger, st roomPlanState) bool {
	f := m.l.f
	for _, u := range m.wallU() {
		for v := roomBackV; v <= roomFrontV; v++ {
			if p := f.at(u, v); w.TerrainAt(p) != Wall || st.designated[p] {
				return false
			}
		}
	}
	zone := m.l.recipe.structure.Zone()
	u0, u1 := m.laneU()
	for u := u0; u <= u1; u++ {
		for v := roomBackV; v <= roomFrontV; v++ {
			p := f.at(u, v)
			t := w.TerrainAt(p)
			wall := v == roomBackV || v == roomFrontV
			if wall && t == Wall {
				if st.designated[p] {
					return false
				}
				continue
			}
			if t != Floor && t != Rock {
				return false
			}
			if st.designated[p] || w.doorTiles[p] {
				return false
			}
			if zone != NoZone && !w.zoneAllows(p, zone) {
				return false
			}
			if t == Floor {
				if o := w.roomFloor[p]; !w.discovered(p) || (o != nil && o != m.l && o != m.r) {
					return false
				}
			}
			if !wall {
				continue
			}
			out := f.at(u, roomBackV-1)
			if v == roomFrontV {
				out = f.at(u, roomFrontV+1)
			}
			if w.TerrainAt(out) == Wall || st.walls[out] {
				return false
			}
		}
	}
	return true
}

// designateMerger marks out m, with new fixtures at columns us, paid for by
// the colony, and reports whether it could pay. Phases as an expansion's:
// tear down the walls between the rooms (their inside rows; the back and
// front wall tiles stay, as part of the joined room's), dig out the lane,
// wall it at the back and front, and fit the fixtures.
//
// The older record lives on as the joined room; the other is dropped, its
// floor and its structure taken in. Both doorways stay, and stay reserved.
func (w *World) designateMerger(m roomMerger, us []int) bool {
	r := m.l.recipe
	f := m.l.f
	keep, drop := m.l, m.r
	if slices.Index(w.roomRecords, m.r) < slices.Index(w.roomRecords, m.l) {
		keep, drop = m.r, m.l
	}
	p := &project{id: w.nextProjectID, name: r.name + " merger", queuedTick: w.tick, issuer: keep.issuer, room: keep}
	for _, u := range m.wallU() {
		for v := 0; v < roomFrontV; v++ {
			p.tasks = append(p.tasks, &buildTask{pos: f.at(u, v), terrain: Floor, clears: Wall, phase: roomDemolishPhase})
		}
	}
	u0, u1 := m.laneU()
	var lane []Point
	for u := u0; u <= u1; u++ {
		for v := roomBackV; v <= roomFrontV; v++ {
			pos := f.at(u, v)
			switch w.TerrainAt(pos) {
			case Rock:
				p.tasks = append(p.tasks, &buildTask{pos: pos, terrain: Floor, phase: roomDigPhase})
			case Wall:
				continue
			}
			lane = append(lane, pos)
		}
	}
	for u := u0; u <= u1; u++ {
		for _, v := range [2]int{roomBackV, roomFrontV} {
			if pos := f.at(u, v); w.TerrainAt(pos) != Wall {
				p.tasks = append(p.tasks, &buildTask{pos: pos, terrain: Wall, phase: roomWallPhase})
			}
		}
	}
	for i, u := range us {
		p.tasks = append(p.tasks, &buildTask{pos: f.at(u, 0), terrain: r.kinds[(m.l.n+i)%len(r.kinds)], phase: roomFitPhase})
	}
	for _, t := range p.tasks {
		t.proj = p
	}
	if !w.fundProject(p) {
		return false
	}
	w.nextProjectID++
	w.projects = append(w.projects, p)
	if r.name == scumhouseRoom.name {
		w.linkPantry(p) // fixtureU fits at most want, and a kitchen wants one pair
	}

	joined, lw, n := m.frame(), m.l.f.width, m.l.n+m.r.n+len(us)
	w.roomRecords = slices.DeleteFunc(w.roomRecords, func(rec *roomRecord) bool { return rec == drop })
	for q, rec := range w.roomFloor {
		if rec == drop {
			w.roomFloor[q] = keep // reassigning while ranging is safe, and order decides nothing
		}
	}
	keep.f, keep.n = joined, n
	w.indexRoomFloor(keep, lw, lw+m.d) // the walls coming down and the lane
	s := keep.structure
	if s == nil {
		s = drop.structure
	} else {
		w.absorbStructure(s, drop.structure)
	}
	keep.structure, p.structure = s, s
	w.growStructure(s, p, lane)
	if s != nil {
		s.room = keep
	}
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony takes down the wall between two rooms to make one larger %s.", r.name))
	return true
}

// absorbStructure folds o into s: o's tiles, area and doorways become s's,
// and o leaves the registry. A party wall the two shared is listed once.
func (w *World) absorbStructure(s, o *structure) {
	if s == nil || o == nil || s == o {
		return
	}
	for _, q := range o.tiles {
		w.unindexStructureTile(q, o.id)
		if !slices.Contains(w.structureAt[q], s.id) {
			s.tiles = append(s.tiles, q)
			w.structureAt[q] = append(w.structureAt[q], s.id)
		}
	}
	s.tiles = sortedPoints(s.tiles)
	s.area = append(s.area, o.area...)
	s.x0, s.y0 = min(s.x0, o.x0), min(s.y0, o.y0)
	s.x1, s.y1 = max(s.x1, o.x1), max(s.y1, o.y1)
	s.doors = append(s.doors, o.doors...)
	delete(w.structures, o.id)
	w.structureRev++
}

// tidyRooms joins rooms that stand side by side even when the colony wants
// no more of their fixtures: the planner's last call, when it has nothing
// else to build. One merger at a time, the rooms' kinds in a fixed order.
func (w *World) tidyRooms() bool {
	if !w.cfg.RoomMerge {
		return false
	}
	st := w.roomPlanState()
	for _, r := range [...]roomRecipe{scumhouseRoom, incubatorRoom, storageRoom, dormRoom, hallRoom} {
		if w.mergeRooms(r, r.fullBay(), 0, st) {
			return true
		}
	}
	return false
}

package sim

import "fmt"

// ---- Room expansion ----------------------------------------------------------------
//
// A colony short of bunks or storage used to mark out a new room every time:
// another shell of walls, another doorway, another site to find. A room that
// is only ever one bay of the same fixture can instead grow. Expansion tears
// down one of its side walls and raises a new one further out, extending the
// back and front walls to meet it, and the room gains fixtures where the old
// wall stood and beyond. The colony commissions the work like any public work,
// paying from the treasury, and tearing a wall down has a wage of its own
// (WageDemolish). See docs/room-expansion.md.

// roomRecord is a room the colony has marked out, as it stands now: an
// expansion widens f and adds to n.
type roomRecord struct {
	// recipe is the recipe the room was built from, as built: a narrow
	// fallback's has no aisle, and its expansions keep it that way.
	recipe roomRecipe
	f      roomFrame // the room's current extent
	n      int       // fixtures in its bay
	issuer Owner
	// structure is the room in the structure registry (structures.go),
	// which an expansion grows too.
	structure *structure
}

// stripU is the frame column of an expansion's column j, counted outward from
// the side wall it moves (j = 0) to the new side wall (j = 2k): to the right
// of the room (past u = width) or to its left (past u = -1).
func (r *roomRecord) stripU(j int, right bool) int {
	if right {
		return r.f.width + j
	}
	return -1 - j
}

// grown is the room's frame once it has gained k fixtures on one side. Each
// fixture adds two columns: the fixture and the gap before the next.
func (r *roomRecord) grown(k int, right bool) roomFrame {
	f := r.f
	if !right {
		f.o = f.at(-2*k, 0)
	}
	f.width += 2 * k
	return f
}

// growOrPlan has the colony enlarge one of its rooms of r by up to want
// fixtures, and marks out a new room only when none can grow. It reports
// whether it did either.
//
// Two rules keep it from scattering small rooms. A room of r that is still
// going up (or growing) is waited for rather than joined by another: once it
// stands it can grow, and a second room planned beside it only gets in its
// way. And a later room of r with an aisle waits for a site wide enough for
// one, as later kitchens always have (aisleRequired): the narrow fallback is
// for a colony's first, and a narrow room squeezed into the gap between two
// others can never grow. Storage is exempt from both: a full inventory can
// stall every project in flight, the storage room going up included, and a
// storage room anywhere is what breaks that.
func (w *World) growOrPlan(r roomRecipe, want int) bool {
	if w.expandRoom(r, want) {
		return true
	}
	if r.name != storageRoom.name {
		if w.roomGoingUp(r) {
			return false
		}
		r.aisleRequired = r.aisleRequired || (r.aisle && w.hasRoom(r))
	}
	return w.planRoomFor(r, Community)
}

// hasRoom reports whether the colony has marked out a room of r, anyone's.
func (w *World) hasRoom(r roomRecipe) bool {
	for _, rec := range w.roomRecords {
		if rec.recipe.name == r.name {
			return true
		}
	}
	return false
}

// roomGoingUp reports whether one of the colony's rooms of r is being built
// or enlarged and could grow once it stands: the planner waits for it rather
// than marking out another room of r beside it (see growOrPlan).
func (w *World) roomGoingUp(r roomRecipe) bool {
	if !w.cfg.RoomExpansion || !r.expands {
		return false
	}
	for _, p := range w.projects {
		if rec := p.room; rec != nil && rec.recipe.name == r.name && rec.issuer == Community && rec.n < w.cfg.RoomMaxFacilities {
			return true
		}
	}
	return false
}

// roomPlanState is what the room planner reads of the projects in flight:
// the rooms they are building or enlarging, every tile they will change, and
// the walls they will raise.
type roomPlanState struct {
	busy       map[*roomRecord]bool
	designated map[Point]bool
	walls      map[Point]bool
}

func (w *World) roomPlanState() roomPlanState {
	st := roomPlanState{busy: make(map[*roomRecord]bool), designated: make(map[Point]bool), walls: make(map[Point]bool)}
	for _, p := range w.projects {
		if p.room != nil {
			st.busy[p.room] = true
		}
		for _, t := range p.tasks {
			st.designated[t.pos] = true
			if t.terrain == Wall {
				st.walls[t.pos] = true
			}
		}
	}
	return st
}

// expandRoom enlarges one of the colony's rooms of r by as many of want
// fixtures as fit, and reports whether it did. Joining two rooms of r that
// stand side by side comes first (see roommerge.go): it fits fixtures where
// a wall stood and puts two rooms that block each other's growth into one.
// Then growing: the oldest room that can take more, by as many of want as
// fit (at most a full room's worth, roomFacilities, at a time, and never
// past RoomMaxFacilities), on whichever side has room: right first, then
// left.
//
// Only the colony's own rooms grow, one expansion at a time each, and only a
// recipe that expands. A room grows by whole cycles of its recipe's kinds, so
// the new end of the bay repeats the old: a kitchen by a stove and its pantry
// together, never a stove alone, and only a kitchen whose bay is whole pairs.
func (w *World) expandRoom(r roomRecipe, want int) bool {
	limit := w.cfg.RoomMaxFacilities
	if !r.expands || want < 1 {
		return false
	}
	st := w.roomPlanState()
	if w.cfg.RoomMerge && w.mergeRooms(r, want, 1, st) {
		return true
	}
	if !w.cfg.RoomExpansion {
		return false
	}
	step := len(r.kinds)
	for _, rec := range w.roomRecords {
		if rec.recipe.name != r.name || rec.issuer != Community || st.busy[rec] || rec.n >= limit || rec.n%step != 0 {
			continue
		}
		for k := min(want, roomFacilities, limit-rec.n) / step * step; k >= step; k -= step {
			for _, right := range [2]bool{true, false} {
				if !w.expansionClear(rec, k, right, st.designated, st.walls) ||
					!w.siteKeepsColonyWhole(rec.grown(k, right), st.designated) {
					continue
				}
				if w.designateExpansion(rec, k, right) {
					return true
				}
				// The treasury cannot pay for this much; a smaller one may do.
			}
		}
	}
	return false
}

// expansionClear reports whether rec can gain k fixtures on one side. The
// side wall that moves must be standing whole and no other project's. Past
// it, every tile the room takes in must be open, discovered floor or rock to
// dig, in no other room and on no reserved doorway, and claimed by no other
// project — except that where the new walls go, a wall already standing
// serves as it is (a party wall, as roomSiteClear shares one). A new wall is
// never raised against a standing or planned one: that is the double-thick
// wall rooms used to leave between them.
//
// Unlike a new room's site, nothing is asked of the ground outside: every
// tile of the expansion is reached from inside the room, through the gap the
// demolished wall leaves. Whether the bigger room cuts the colony in two is
// siteKeepsColonyWhole's question.
func (w *World) expansionClear(rec *roomRecord, k int, right bool, designated, walls map[Point]bool) bool {
	f := rec.f
	out := 2 * k
	if !w.InBounds(f.at(rec.stripU(out+1, right), roomBackV-1)) || !w.InBounds(f.at(rec.stripU(out+1, right), roomFrontV+1)) {
		return false
	}
	zone := rec.recipe.structure.Zone()
	for j := 0; j <= out; j++ {
		u := rec.stripU(j, right)
		for v := roomBackV; v <= roomFrontV; v++ {
			p := f.at(u, v)
			t := w.TerrainAt(p)
			// A room grows only onto ground zoned for it, as it was sited
			// (zoneAllows; a wall it would share is the neighbour's).
			if j > 0 && zone != NoZone && t != Wall && !w.zoneAllows(p, zone) {
				return false
			}
			if j == 0 {
				if t != Wall || designated[p] {
					return false
				}
				continue
			}
			wall := j == out || v == roomBackV || v == roomFrontV
			if wall && t == Wall {
				if designated[p] {
					return false
				}
				continue
			}
			if t != Floor && t != Rock {
				return false
			}
			if designated[p] || w.doorTiles[p] {
				return false
			}
			if t == Floor && (!w.discovered(p) || w.inOtherRoom(p, rec)) {
				return false
			}
			if !wall {
				continue
			}
			var outward [2]Point
			n := 0
			if v == roomBackV {
				outward[n], n = f.at(u, roomBackV-1), n+1
			}
			if v == roomFrontV {
				outward[n], n = f.at(u, roomFrontV+1), n+1
			}
			if j == out {
				outward[n], n = f.at(rec.stripU(out+1, right), v), n+1
			}
			for _, q := range outward[:n] {
				if w.TerrainAt(q) == Wall || walls[q] {
					return false
				}
			}
		}
	}
	return true
}

// inOtherRoom reports whether the floor tile p lies in a room the colony has
// marked out other than rec. A finished room's floor is ordinary discovered
// floor, so without this an expansion could take in a neighbor's aisle.
//
// It reads w.roomFloor rather than testing every room: the scan made a
// search that found nothing quadratic in the number of rooms, 4 ms with 800
// storage rooms packed wall to wall (BenchmarkExpandNoFit). The index holds
// only floor tiles, a room's inside and its doorway, which no two rooms
// share, so a lookup gives the same answer as the scan for any floor tile.
func (w *World) inOtherRoom(p Point, rec *roomRecord) bool {
	o := w.roomFloor[p]
	return o != nil && o != rec
}

// indexRoomFloor records the inside of rec's frame columns u0..u1 (rows 0 to
// just before the front wall) in w.roomFloor.
func (w *World) indexRoomFloor(rec *roomRecord, u0, u1 int) {
	for u := u0; u <= u1; u++ {
		for v := 0; v < roomFrontV; v++ {
			w.roomFloor[rec.f.at(u, v)] = rec
		}
	}
}

// designateExpansion marks out rec's growth by k fixtures on one side, paid
// for by its issuer, and reports whether the issuer could pay. Phases: tear
// down the moving side wall's inside rows (the back and front wall tiles in
// that column stay, as part of the longer back and front walls), dig out any
// rock, raise the new walls, then fit the fixtures.
//
// The wall comes down first so the work beyond it is reached from inside the
// room. Raising the new walls first would close off the space between them
// and the old wall, with no way in and possibly a builder inside.
func (w *World) designateExpansion(rec *roomRecord, k int, right bool) bool {
	r := rec.recipe
	f := rec.f
	out := 2 * k
	p := &project{id: w.nextProjectID, name: r.name + " expansion", queuedTick: w.tick, issuer: rec.issuer, room: rec}
	for v := 0; v < roomFrontV; v++ {
		p.tasks = append(p.tasks, &buildTask{pos: f.at(rec.stripU(0, right), v), terrain: Floor, clears: Wall, phase: roomDemolishPhase})
	}
	for j := 1; j <= out; j++ {
		for v := roomBackV; v <= roomFrontV; v++ {
			if pos := f.at(rec.stripU(j, right), v); w.TerrainAt(pos) == Rock {
				p.tasks = append(p.tasks, &buildTask{pos: pos, terrain: Floor, phase: roomDigPhase})
			}
		}
	}
	for j := 1; j <= out; j++ {
		for v := roomBackV; v <= roomFrontV; v++ {
			if j != out && v != roomBackV && v != roomFrontV {
				continue
			}
			if pos := f.at(rec.stripU(j, right), v); w.TerrainAt(pos) != Wall {
				p.tasks = append(p.tasks, &buildTask{pos: pos, terrain: Wall, phase: roomWallPhase})
			}
		}
	}
	// The bay carries on at the same spacing: the new fixtures sit where the
	// old wall stood (with an aisle) or just past it (without).
	for i := 1; i <= k; i++ {
		j := 2*i - 1 - r.bayOffset()
		p.tasks = append(p.tasks, &buildTask{pos: f.at(rec.stripU(j, right), 0),
			terrain: r.kinds[(rec.n+i-1)%len(r.kinds)], phase: roomFitPhase})
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
		w.linkPantry(p) // the new stove's pantry, as designateRoom links a new kitchen's
	}
	// The room's structure takes in the strip too: its new walls and
	// fixtures, and the ground that now has to lie in its zone.
	var strip []Point
	for j := 1; j <= out; j++ {
		for v := roomBackV; v <= roomFrontV; v++ {
			if pos := f.at(rec.stripU(j, right), v); w.TerrainAt(pos) != Wall {
				strip = append(strip, pos)
			}
		}
	}
	p.structure = rec.structure
	w.growStructure(rec.structure, p, strip)
	// The new inside, from where the old wall stood to the new wall, in the
	// frame as it was before growing.
	if right {
		w.indexRoomFloor(rec, rec.f.width, rec.f.width+out-1)
	} else {
		w.indexRoomFloor(rec, -out, -1)
	}
	rec.f = rec.grown(k, right)
	rec.n += k
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony moves a wall out to enlarge a %s.", r.name))
	return true
}

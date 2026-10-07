package sim

import (
	"fmt"
	"slices"
	"sort"
)

// ---- Rooms ----------------------------------------------------------------------------
//
// A room is a walled rectangle with doorways in its walls, holding fixtures
// of one zone (see fixtureZone). It used to be one recipe's bay along one
// back wall, a "kitchen" or an "incubator room", and nothing else could ever
// stand in it; a colony short of incubators beside a kitchen with floor to
// spare marked out another room. Now:
//
//   - A fixture goes into free floor in a room of its zone first (a fit-out),
//     wherever the layout rule (roomLayout.fits) lets it stand.
//   - Two rooms of a zone that stand side by side, back to back or facing
//     each other become one (a merger), the walls between them torn down.
//   - A room grows through any of its walls, a doorway in that wall moving
//     out with it.
//   - Only then is a new room marked out, from a recipe (project.go), which
//     is now just the layout of a new room's shell and first fixtures.
//
// Growth and mergers are one operation, a reshape: rooms become one new
// rectangle that covers them all. See docs/room-expansion.md. (rooms.go is
// something else: the walkable map's connected areas, for pathfinding.)

// roomRecord is a room the colony (or a colonist) has marked out, as it
// stands now.
type roomRecord struct {
	lo, hi Point   // the inside, inclusive; the walls stand one tile outside it
	doors  []Point // the doorways: gaps in the walls
	zone   ZoneKind
	issuer Owner
	// structure is the room in the structure registry (structures.go).
	structure *structure
}

// inBox reports whether p lies in the rectangle lo..hi, inclusive.
func inBox(lo, hi, p Point) bool { return p.X >= lo.X && p.X <= hi.X && p.Y >= lo.Y && p.Y <= hi.Y }

// onRing reports whether p is in the wall ring round the inside lo..hi.
func onRing(lo, hi, p Point) bool {
	return inBox(Point{lo.X - 1, lo.Y - 1, lo.Level}, Point{hi.X + 1, hi.Y + 1, hi.Level}, p) && !inBox(lo, hi, p)
}

// outward is the step from a doorway d in the walls round lo..hi out of the
// room: the inside tile next to it is d minus outward, the step outside d
// plus it. A corner has no outward and is never a doorway.
func outward(lo, hi, d Point) Point {
	switch {
	case d.Y == lo.Y-1 && d.X >= lo.X && d.X <= hi.X:
		return Point{0, -1, 0}
	case d.Y == hi.Y+1 && d.X >= lo.X && d.X <= hi.X:
		return Point{0, 1, 0}
	case d.X == lo.X-1 && d.Y >= lo.Y && d.Y <= hi.Y:
		return Point{-1, 0, 0}
	case d.X == hi.X+1 && d.Y >= lo.Y && d.Y <= hi.Y:
		return Point{1, 0, 0}
	}
	return Point{}
}

// doorStep is the tile just outside the doorway d, reserved in w.doorTiles.
func doorStep(lo, hi, d Point) Point { o := outward(lo, hi, d); return d.Add(o.X, o.Y) }

// doorInside is the tile just inside the doorway d, which no fixture takes.
func doorInside(lo, hi, d Point) Point { o := outward(lo, hi, d); return d.Add(-o.X, -o.Y) }

// newRoomRecord records the room designateRoom lays out in frame f.
func (w *World) newRoomRecord(f roomFrame, zone ZoneKind, issuer Owner) *roomRecord {
	lo, hi := f.box(0, 0, f.width-1, roomFrontV-1)
	rec := &roomRecord{lo: lo, hi: hi, doors: []Point{f.at(f.doorU(), roomFrontV)}, zone: zone, issuer: issuer}
	w.roomRecords = append(w.roomRecords, rec)
	w.indexRoom(rec)
	return rec
}

// indexRoom records rec's inside and its doorways in w.roomFloor.
func (w *World) indexRoom(rec *roomRecord) {
	for y := rec.lo.Y; y <= rec.hi.Y; y++ {
		for x := rec.lo.X; x <= rec.hi.X; x++ {
			w.roomFloor[Point{x, y, LandingLevel}] = rec
		}
	}
	for _, d := range rec.doors {
		w.roomFloor[d] = rec
	}
}

// ---- Where a fixture may stand ---------------------------------------------------------

// needsAisle reports whether a fixture of kind needs two tiles to be reached
// from: a workshop or a depot someone works at for long stretches while
// others fetch from it. With one, a cook at the stove starved the colonist
// queued behind it for a meal of its own (see roomRecipe.aisle).
func needsAisle(kind Terrain) bool {
	switch kind {
	case Scumhouse, Storage, Forge, GunBench, Incubator:
		return true
	}
	return false
}

// accessNeeded is how many free tiles beside a fixture of kind must stay
// reachable from the doorways.
func accessNeeded(kind Terrain) int {
	if needsAisle(kind) {
		return 2
	}
	return 1
}

// roomLayout is a room's inside as the layout rule sees it: its rectangle,
// the tiles inside its doorways, and its fixtures, standing or planned.
type roomLayout struct {
	lo, hi Point
	w, h   int
	inner  []Point
	occ    map[Point]Terrain
	cands  []Point // inside tiles in the order fixtures take them
}

// layoutFor is the layout of a room inside lo..hi with doorways doors, its
// fixtures what stands there now or a project plans there.
func (w *World) layoutFor(lo, hi Point, doors []Point, st roomPlanState) *roomLayout {
	l := &roomLayout{lo: lo, hi: hi, w: hi.X - lo.X + 1, h: hi.Y - lo.Y + 1, occ: make(map[Point]Terrain)}
	for _, d := range doors {
		l.inner = append(l.inner, doorInside(lo, hi, d))
	}
	for y := lo.Y; y <= hi.Y; y++ {
		for x := lo.X; x <= hi.X; x++ {
			p := Point{x, y, LandingLevel}
			if k, ok := st.planned[p]; ok {
				l.occ[p] = k
			} else if t := w.TerrainAt(p); FixtureZone(t) != NoZone {
				l.occ[p] = t
			}
		}
	}
	l.order(w)
	return l
}

// order sorts the inside into the order fixtures take it: farthest from the
// doorways first, so the floor by a doorway stays clear; then against a wall
// before out on the floor; then row-major. A new room's back row comes
// first, as its bay always has.
func (l *roomLayout) order(w *World) {
	type cand struct {
		p          Point
		dist, wall int
	}
	var cs []cand
	for y := l.lo.Y; y <= l.hi.Y; y++ {
		for x := l.lo.X; x <= l.hi.X; x++ {
			p := Point{x, y, LandingLevel}
			d := 1 << 30
			for _, in := range l.inner {
				d = min(d, p.Manhattan(in))
			}
			wall := 0
			if x == l.lo.X || x == l.hi.X || y == l.lo.Y || y == l.hi.Y {
				wall = 1
			}
			cs = append(cs, cand{p, d, wall})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.dist != b.dist {
			return a.dist > b.dist
		}
		if a.wall != b.wall {
			return a.wall > b.wall
		}
		return lessPoint(a.p, b.p)
	})
	l.cands = l.cands[:0]
	for _, c := range cs {
		l.cands = append(l.cands, c.p)
	}
}

func (l *roomLayout) idx(p Point) int { return (p.Y-l.lo.Y)*l.w + (p.X - l.lo.X) }

// reach floods the free inside from the first doorway's inside tile, and
// reports what it reached and whether it reached every doorway's.
func (l *roomLayout) reach() ([]bool, bool) {
	seen := make([]bool, l.w*l.h)
	var queue []Point
	for _, in := range l.inner {
		if _, ok := l.occ[in]; !ok && inBox(l.lo, l.hi, in) {
			seen[l.idx(in)] = true
			queue = append(queue, in)
			break
		}
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range neighbors4 {
			n := p.Add(d.X, d.Y)
			if !inBox(l.lo, l.hi, n) || seen[l.idx(n)] {
				continue
			}
			if _, ok := l.occ[n]; ok {
				continue
			}
			seen[l.idx(n)] = true
			queue = append(queue, n)
		}
	}
	for _, in := range l.inner {
		if _, ok := l.occ[in]; !ok && inBox(l.lo, l.hi, in) && !seen[l.idx(in)] {
			return seen, false
		}
	}
	return seen, true
}

// access counts the free tiles round f, diagonals included (a fixture is
// used from any tile next to it), that seen reached. The flood itself goes
// only orthogonally, the stricter test of whether the floor joins up.
func (l *roomLayout) access(seen []bool, f Point) int {
	n := 0
	for _, d := range neighbors8 {
		q := f.Add(d.X, d.Y)
		if inBox(l.lo, l.hi, q) && seen[l.idx(q)] {
			n++
		}
	}
	return n
}

// fits reports whether a fixture of kind may stand at p, the layout rule:
//
//   - p is inside, free, and not the tile just inside a doorway;
//   - no other fixture stands within a step of it, diagonals included, as a
//     bay has always spaced them (a fixture is used from the tiles round
//     it, and two side by side leave one of them unreachable);
//   - every doorway can still reach every other through the free floor;
//   - it has the free tiles beside it it needs (accessNeeded) reachable from
//     the doorways, and no fixture near it loses one it needs.
//
// So a fixture can stand anywhere in a room of any shape, a second row in a
// room two rooms deep, without walling anything off.
func (l *roomLayout) fits(p Point, kind Terrain) bool {
	if !inBox(l.lo, l.hi, p) || slices.Contains(l.inner, p) {
		return false
	}
	if _, ok := l.occ[p]; ok {
		return false
	}
	for q := range l.occ {
		if p.Chebyshev(q) <= 1 {
			return false
		}
	}
	before, _ := l.reach()
	l.occ[p] = kind
	after, whole := l.reach()
	defer delete(l.occ, p)
	if !whole || l.access(after, p) < accessNeeded(kind) {
		return false
	}
	for q, t := range l.occ {
		if q == p || p.Chebyshev(q) > 2 {
			continue
		}
		if l.access(after, q) < min(accessNeeded(t), l.access(before, q)) {
			return false
		}
	}
	return true
}

// fixtureUnit is what the planner places as one: a fixture, or two that go
// together two tiles apart in a line, a stove and its pantry or a forge and
// its gun bench.
type fixtureUnit []Terrain

// units is n units of one fixture.
func units(n int, kind Terrain) []fixtureUnit {
	out := make([]fixtureUnit, n)
	for i := range out {
		out[i] = fixtureUnit{kind}
	}
	return out
}

// placement is one fixture a project raises, and pair the fixture it goes
// with (the stove a pantry belongs to), if any.
type placement struct {
	pos  Point
	kind Terrain
	pair *Point
}

// place fits as many of us as it can, in order, without the room holding
// more than most fixtures, and returns where they go. It stops at the first
// that does not fit.
func (l *roomLayout) place(us []fixtureUnit, most int) []placement {
	var out []placement
	dirs := [...]Point{{1, 0, 0}, {0, 1, 0}, {-1, 0, 0}, {0, -1, 0}}
	for _, u := range us {
		if len(l.occ)+len(u) > most {
			break
		}
		placed := false
		for _, p := range l.cands {
			if !l.fits(p, u[0]) {
				continue
			}
			if len(u) == 1 {
				l.occ[p] = u[0]
				out = append(out, placement{pos: p, kind: u[0]})
				placed = true
				break
			}
			l.occ[p] = u[0]
			for _, d := range dirs {
				q := p.Add(2*d.X, 2*d.Y)
				if l.fits(q, u[1]) {
					l.occ[q] = u[1]
					first := p
					out = append(out, placement{pos: p, kind: u[0]}, placement{pos: q, kind: u[1], pair: &first})
					placed = true
					break
				}
			}
			if placed {
				break
			}
			delete(l.occ, p)
		}
		if !placed {
			break
		}
	}
	return out
}

// ---- Reshaping: growing and joining rooms ---------------------------------------------

// roomShape is rooms becoming one, inside lo..hi: one room growing, or two
// joining. The first room is the one that lives on.
type roomShape struct {
	recs   []*roomRecord
	lo, hi Point
}

// shapePlan is the work a roomShape takes, as shapeWork finds it.
type shapePlan struct {
	demolish, dig, walls []Point
	strip                []Point // ground the room takes in: its new inside and new walls
	doors                []Point // the joined room's doorways
	newSteps, oldSteps   []Point // door steps to reserve, and to let go
}

// shapeWork works out sh, and reports whether it can be built at all. hard
// is set when the refusal is something in the rectangle's inside that a
// bigger rectangle would take in too, so growing further that way is no use.
//
// The rules are an expansion's, for any rectangle covering the rooms:
//
//   - A wall of the rooms' that ends up inside comes down: it must stand
//     whole, be no project's, and be no third room's.
//   - Ground the room takes in must be discovered floor or rock to dig, in
//     no other room, on no other room's doorway step, zoned for the room,
//     and claimed by no project. Where its new walls go, a wall already
//     standing is shared; a new one is never raised against one standing or
//     planned (the double wall).
//   - A doorway in a wall that moves out moves out with it, if there is
//     open floor outside its new place; at least one doorway must survive.
func (w *World) shapeWork(sh roomShape, st roomPlanState) (plan *shapePlan, hard, ok bool) {
	plan = &shapePlan{}
	own := map[int]bool{}
	ownRec := map[*roomRecord]bool{}
	ownSteps := map[Point]bool{}
	oldDoor := map[Point]bool{}
	for _, rec := range sh.recs {
		ownRec[rec] = true
		if rec.structure != nil {
			own[rec.structure.id] = true
		}
		for _, d := range rec.doors {
			ownSteps[doorStep(rec.lo, rec.hi, d)] = true
			oldDoor[d] = true
		}
	}
	// Doorways first: which stay, which move out, which are lost.
	for _, rec := range sh.recs {
		for _, d := range rec.doors {
			if onRing(sh.lo, sh.hi, d) {
				plan.doors = append(plan.doors, d)
				continue
			}
			plan.oldSteps = append(plan.oldSteps, doorStep(rec.lo, rec.hi, d))
			o := outward(rec.lo, rec.hi, d)
			nd := d
			for !onRing(sh.lo, sh.hi, nd) && inBox(sh.lo, sh.hi, nd) {
				nd = nd.Add(o.X, o.Y)
			}
			ns := nd.Add(o.X, o.Y)
			if outward(sh.lo, sh.hi, nd) != o {
				continue // pushed into a corner
			}
			if t := w.TerrainAt(nd); (t != Floor && t != Rock) || st.designated[nd] {
				continue
			}
			if !w.Walkable(ns) || !w.discovered(ns) || st.designated[ns] || w.roomFloor[ns] != nil || w.doorTiles[ns] {
				continue
			}
			plan.doors = append(plan.doors, nd)
			plan.newSteps = append(plan.newSteps, ns)
		}
	}
	if len(plan.doors) == 0 {
		return nil, false, false
	}
	doorSet := map[Point]bool{}
	for _, d := range plan.doors {
		doorSet[d] = true
	}
	zone := sh.recs[0].zone
	for y := sh.lo.Y - 1; y <= sh.hi.Y+1; y++ {
		for x := sh.lo.X - 1; x <= sh.hi.X+1; x++ {
			p := Point{x, y, LandingLevel}
			ring := !inBox(sh.lo, sh.hi, p)
			t := w.TerrainAt(p)
			var oldIn, oldRing bool
			for _, rec := range sh.recs {
				oldIn = oldIn || inBox(rec.lo, rec.hi, p)
				oldRing = oldRing || onRing(rec.lo, rec.hi, p)
			}
			switch {
			case oldIn:
				continue
			case oldRing:
				if ring || oldDoor[p] || t == Floor {
					continue // a wall that stays, or a gap that is already open
				}
				if t != Wall || st.designated[p] {
					return nil, true, false
				}
				for _, id := range w.structureAt[p] {
					if !own[id] {
						return nil, true, false // a third room's wall too
					}
				}
				plan.demolish = append(plan.demolish, p)
			default:
				if ring && t == Wall && !doorSet[p] {
					if st.designated[p] {
						return nil, false, false
					}
					continue // a party wall
				}
				if (t != Floor && t != Rock) || st.designated[p] || (w.doorTiles[p] && !ownSteps[p]) {
					return nil, !ring, false
				}
				if t == Floor && !w.discovered(p) {
					return nil, !ring, false
				}
				if o := w.roomFloor[p]; t == Floor && o != nil && !ownRec[o] {
					return nil, !ring, false
				}
				if zone != NoZone && !w.zoneAllows(p, zone) {
					return nil, !ring, false
				}
				if t == Rock {
					plan.dig = append(plan.dig, p)
				}
				plan.strip = append(plan.strip, p)
				if !ring || doorSet[p] {
					continue
				}
				plan.walls = append(plan.walls, p)
				for _, d := range neighbors4 {
					q := p.Add(d.X, d.Y)
					if inBox(Point{sh.lo.X - 1, sh.lo.Y - 1, sh.lo.Level}, Point{sh.hi.X + 1, sh.hi.Y + 1, sh.hi.Level}, q) {
						continue
					}
					if w.TerrainAt(q) == Wall || st.walls[q] {
						return nil, false, false
					}
				}
			}
		}
	}
	return plan, false, true
}

// shapeKeepsColonyWhole is siteKeepsColonyWhole for sh's footprint. A
// footprint the rooms already cover changes nothing.
func (w *World) shapeKeepsColonyWhole(sh roomShape, st roomPlanState) bool {
	if len(sh.recs) == 1 && sh.lo == sh.recs[0].lo && sh.hi == sh.recs[0].hi {
		return true
	}
	return w.footprintKeepsColonyWhole(Point{sh.lo.X - 1, sh.lo.Y - 1, sh.lo.Level}, Point{sh.hi.X + 1, sh.hi.Y + 1, sh.hi.Level}, st.designated)
}

// designateShape marks out sh's work and fixtures us as one project named
// name, paid for by the room's issuer, and reports whether it could pay.
// Phases as a new room's, with the walls coming down first, so the ground
// beyond them is reached from inside the room: raising the new walls first
// would close off the space between them and the old, with no way in and
// perhaps a builder inside.
//
// The first room lives on as the reshaped one; the others are dropped, their
// floor and structure taken in. The record changes when the work is marked
// out, not when it is done, the way planned fixtures count as soon as they
// are designated, so demand converges.
func (w *World) designateShape(sh roomShape, plan *shapePlan, us []placement, name string) bool {
	keep := sh.recs[0]
	p := &project{id: w.nextProjectID, name: name, queuedTick: w.tick, issuer: keep.issuer, room: keep}
	for _, q := range plan.demolish {
		p.tasks = append(p.tasks, &buildTask{pos: q, terrain: Floor, clears: Wall, phase: roomDemolishPhase})
	}
	for _, q := range plan.dig {
		p.tasks = append(p.tasks, &buildTask{pos: q, terrain: Floor, phase: roomDigPhase})
	}
	for _, q := range plan.walls {
		p.tasks = append(p.tasks, &buildTask{pos: q, terrain: Wall, phase: roomWallPhase})
	}
	for _, f := range us {
		p.tasks = append(p.tasks, &buildTask{pos: f.pos, terrain: f.kind, phase: roomFitPhase})
	}
	for _, t := range p.tasks {
		t.proj = p
	}
	if !w.fundProject(p) {
		return false
	}
	w.nextProjectID++
	w.projects = append(w.projects, p)
	for _, f := range us {
		if f.pair != nil && f.kind == Storage {
			w.pantryOf[*f.pair], w.pantryHouse[f.pos] = f.pos, *f.pair
		}
	}

	drop := sh.recs[1:]
	if len(drop) > 0 {
		gone := map[*roomRecord]bool{}
		for _, rec := range drop {
			gone[rec] = true
		}
		w.roomRecords = slices.DeleteFunc(w.roomRecords, func(rec *roomRecord) bool { return gone[rec] })
		for q, rec := range w.roomFloor {
			if gone[rec] {
				delete(w.roomFloor, q) // deleting while ranging is safe, and order decides nothing
			}
		}
	}
	for _, q := range plan.oldSteps {
		delete(w.doorTiles, q)
		delete(w.roomFloor, q)
	}
	for _, q := range plan.newSteps {
		w.doorTiles[q] = true
	}
	for _, rec := range sh.recs {
		for _, d := range rec.doors {
			delete(w.roomFloor, d)
		}
	}
	keep.lo, keep.hi, keep.doors = sh.lo, sh.hi, plan.doors
	w.indexRoom(keep)

	s := keep.structure
	for _, rec := range drop {
		if s == nil {
			s = rec.structure
		} else {
			w.absorbStructure(s, rec.structure)
		}
	}
	keep.structure, p.structure = s, s
	if s != nil {
		w.growStructure(s, p, plan.strip)
		s.room = keep
		s.doors = s.doors[:0]
		for _, d := range keep.doors {
			s.doors = append(s.doors, doorStep(keep.lo, keep.hi, d))
		}
	}
	return true
}

// ---- The planner ----------------------------------------------------------------------

// roomPlanState is what the room planner reads of the projects in flight:
// the rooms they are building or reshaping, every tile they will change, the
// walls they will raise, and the fixtures they will put up.
type roomPlanState struct {
	busy       map[*roomRecord]bool
	designated map[Point]bool
	walls      map[Point]bool
	planned    map[Point]Terrain
}

func (w *World) roomPlanState() roomPlanState {
	st := roomPlanState{busy: make(map[*roomRecord]bool), designated: make(map[Point]bool),
		walls: make(map[Point]bool), planned: make(map[Point]Terrain)}
	for _, p := range w.projects {
		if p.room != nil {
			st.busy[p.room] = true
		}
		if p.from != nil {
			st.busy[p.from] = true
		}
		for _, t := range p.tasks {
			st.designated[t.pos] = true
			switch {
			case t.terrain == Wall:
				st.walls[t.pos] = true
			case FixtureZone(t.terrain) != NoZone:
				st.planned[t.pos] = t.terrain
			}
		}
	}
	return st
}

// maxMergeLane is the most columns of open ground between two rooms' walls
// that a merger takes in.
const maxMergeLane = 3

// improveRooms puts us into rooms the colony already has, and reports
// whether it marked out any of the work: a fit-out into free floor in a room
// of their zone, then two rooms of that zone joined, then one grown. Each
// places as many of us as fit, at least one; the planner asks again for the
// rest. Only the colony's own rooms are touched, never one already being
// built or reshaped, and never past room-max-facilities fixtures.
func (w *World) improveRooms(us []fixtureUnit) bool {
	if len(us) == 0 {
		return false
	}
	st := w.roomPlanState()
	zone := kindsZone(us[0])
	var recs []*roomRecord
	for _, rec := range w.roomRecords {
		if rec.zone == zone && rec.issuer == Community && !st.busy[rec] {
			recs = append(recs, rec)
		}
	}
	if w.cfg.RoomExpansion && w.fitOut(recs, us, st) {
		return true
	}
	if w.cfg.RoomMerge && w.mergeRooms(recs, us, 1, st) {
		return true
	}
	return w.cfg.RoomExpansion && w.growRoom(recs, us, st)
}

// roomFixtures counts the fixtures standing or planned inside rec: a cheap
// look before laying the room out, which a full room never needs.
func (w *World) roomFixtures(rec *roomRecord, st roomPlanState) int {
	n := 0
	for y := rec.lo.Y; y <= rec.hi.Y; y++ {
		for x := rec.lo.X; x <= rec.hi.X; x++ {
			p := Point{x, y, LandingLevel}
			if _, ok := st.planned[p]; ok || FixtureZone(w.TerrainAt(p)) != NoZone {
				n++
			}
		}
	}
	return n
}

// fitOut puts us into free floor in the oldest room of recs that has any.
func (w *World) fitOut(recs []*roomRecord, us []fixtureUnit, st roomPlanState) bool {
	for _, rec := range recs {
		if w.roomFixtures(rec, st)+len(us[0]) > w.cfg.RoomMaxFacilities {
			continue
		}
		l := w.layoutFor(rec.lo, rec.hi, rec.doors, st)
		pl := l.place(us, w.cfg.RoomMaxFacilities)
		if len(pl) == 0 {
			continue
		}
		sh := roomShape{recs: []*roomRecord{rec}, lo: rec.lo, hi: rec.hi}
		plan, _, ok := w.shapeWork(sh, st)
		if !ok {
			continue
		}
		name := w.roomTitle(rec) + " fit-out"
		if w.designateShape(sh, plan, pl, name) {
			w.logEvent(LogBuildStart, fmt.Sprintf("The colony fits out a %s with %s.", w.roomTitle(rec), fixturePhrase(pl)))
			return true
		}
	}
	return false
}

// mergeBox is the rectangle two rooms become when they join, if they stand
// close enough: side by side or back to back, at most maxMergeLane tiles of
// ground between their walls, overlapping along the other way, and no more
// ground to fill out to the rectangle than the larger of them already holds.
func mergeBox(a, b *roomRecord) (lo, hi Point, ok bool) {
	gap := func(alo, ahi, blo, bhi int) int {
		if blo > ahi {
			return blo - ahi - 1
		}
		return alo - bhi - 1
	}
	overlap := func(alo, ahi, blo, bhi int) bool { return alo <= bhi && blo <= ahi }
	gx, gy := gap(a.lo.X, a.hi.X, b.lo.X, b.hi.X), gap(a.lo.Y, a.hi.Y, b.lo.Y, b.hi.Y)
	switch {
	case overlap(a.lo.Y, a.hi.Y, b.lo.Y, b.hi.Y) && gx >= 1 && gx <= maxMergeLane+2:
	case overlap(a.lo.X, a.hi.X, b.lo.X, b.hi.X) && gy >= 1 && gy <= maxMergeLane+2:
	default:
		return lo, hi, false
	}
	lo = Point{min(a.lo.X, b.lo.X), min(a.lo.Y, b.lo.Y), a.lo.Level}
	hi = Point{max(a.hi.X, b.hi.X), max(a.hi.Y, b.hi.Y), a.hi.Level}
	area := func(lo, hi Point) int { return (hi.X - lo.X + 1) * (hi.Y - lo.Y + 1) }
	aa, ab := area(a.lo, a.hi), area(b.lo, b.hi)
	// The rectangle less both rooms and what lies between them along the gap.
	between := 0
	if gx >= 1 && overlap(a.lo.Y, a.hi.Y, b.lo.Y, b.hi.Y) {
		between = gx * (min(a.hi.Y, b.hi.Y) - max(a.lo.Y, b.lo.Y) + 1)
	} else {
		between = gy * (min(a.hi.X, b.hi.X) - max(a.lo.X, b.lo.X) + 1)
	}
	return lo, hi, area(lo, hi)-aa-ab-between <= max(aa, ab)
}

// mergePairs lists the pairs of recs that stand close enough to join
// (mergeBox's gap and overlap), oldest room first and then its partner, the
// order mergeRooms tries them in. It sweeps the rooms sorted by their west
// edge, so it looks at a room's near neighbours only: comparing every pair
// was 5.8 ms a planning pass with 800 rooms (BenchmarkTidyNoFit).
func mergePairs(recs []*roomRecord) [][2]int {
	order := make([]int, len(recs))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return recs[a].lo.X - recs[b].lo.X })
	var pairs [][2]int
	for k, i := range order {
		a := recs[i]
		for _, j := range order[k+1:] {
			b := recs[j]
			if b.lo.X > a.hi.X+maxMergeLane+3 {
				break
			}
			if _, _, ok := mergeBox(a, b); ok {
				pairs = append(pairs, [2]int{min(i, j), max(i, j)})
			}
		}
	}
	slices.SortFunc(pairs, func(p, q [2]int) int {
		if p[0] != q[0] {
			return p[0] - q[0]
		}
		return p[1] - q[1]
	})
	return pairs
}

// mergeRooms joins two rooms of recs into one, fitting at least least of us
// into the joined room, and reports whether it did. Pairs go oldest room
// first, and the older lives on.
func (w *World) mergeRooms(recs []*roomRecord, us []fixtureUnit, least int, st roomPlanState) bool {
	limit := w.cfg.RoomMaxFacilities
	counts := make([]int, len(recs))
	for i, rec := range recs {
		counts[i] = w.roomFixtures(rec, st)
	}
	for _, pr := range mergePairs(recs) {
		i, j := pr[0], pr[1]
		a, b := recs[i], recs[j]
		if counts[i]+counts[j] > limit {
			continue
		}
		lo, hi, _ := mergeBox(a, b)
		sh := roomShape{recs: []*roomRecord{a, b}, lo: lo, hi: hi}
		plan, _, ok := w.shapeWork(sh, st)
		if !ok {
			continue
		}
		l := w.layoutFor(lo, hi, plan.doors, st)
		pl := l.place(us, w.cfg.RoomMaxFacilities)
		if len(pl) < least || !w.shapeKeepsColonyWhole(sh, st) {
			continue
		}
		before := w.roomTitle(a)
		if w.designateShape(sh, plan, pl, before+" merger") {
			w.logEvent(LogBuildStart, fmt.Sprintf("The colony takes down the walls between two rooms to make one larger %s.", w.roomTitle(a)))
			return true
		}
	}
	return false
}

// growRoom grows the oldest room of recs that can take some of us, through
// whichever wall does it with the least new ground: a wall with no doorway
// before one with, and of those east, west, south, north. It grows by as few
// rows as fit everything it can (at most the most fixtures a room holds), or
// failing that, places what fits at the furthest it can grow.
func (w *World) growRoom(recs []*roomRecord, us []fixtureUnit, st roomPlanState) bool {
	limit := w.cfg.RoomMaxFacilities
	for _, rec := range recs {
		if w.roomFixtures(rec, st)+len(us[0]) > limit {
			continue
		}
		maxRows := min(2*len(us)+2, 2*roomFacilities+1)
		type option struct {
			sh   roomShape
			plan *shapePlan
			pl   []placement
		}
		var best *option
		for _, side := range w.growSides(rec) {
			for k := 1; k <= maxRows; k++ {
				sh := roomShape{recs: []*roomRecord{rec}, lo: rec.lo, hi: rec.hi}
				switch side {
				case Point{1, 0, 0}:
					sh.hi.X += k
				case Point{-1, 0, 0}:
					sh.lo.X -= k
				case Point{0, 1, 0}:
					sh.hi.Y += k
				default:
					sh.lo.Y -= k
				}
				plan, hard, ok := w.shapeWork(sh, st)
				if !ok {
					if hard {
						break
					}
					continue
				}
				l := w.layoutFor(sh.lo, sh.hi, plan.doors, st)
				pl := l.place(us, limit)
				if len(pl) == 0 {
					continue
				}
				o := &option{sh, plan, pl}
				full := len(pl) >= min(len(us), limit-len(l.occ)+len(pl))
				if best == nil || len(pl) > len(best.pl) {
					best = o
				}
				if full {
					break
				}
			}
		}
		if best == nil || !w.shapeKeepsColonyWhole(best.sh, st) {
			continue
		}
		name := w.roomTitle(rec) + " expansion"
		if w.designateShape(best.sh, best.plan, best.pl, name) {
			w.logEvent(LogBuildStart, fmt.Sprintf("The colony moves a wall out to enlarge a %s.", w.roomTitle(rec)))
			return true
		}
	}
	return false
}

// growSides is the order rec tries its walls in: those with no doorway
// first, each lot east, west, south, north.
func (w *World) growSides(rec *roomRecord) []Point {
	all := []Point{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}}
	var plain, doored []Point
	for _, s := range all {
		has := false
		for _, d := range rec.doors {
			if outward(rec.lo, rec.hi, d) == s {
				has = true
			}
		}
		if has {
			doored = append(doored, s)
		} else {
			plain = append(plain, s)
		}
	}
	return append(plain, doored...)
}

// roomTitle is what a room is called in a project or log line.
func (w *World) roomTitle(rec *roomRecord) string {
	if rec.structure != nil {
		return w.structureName(rec.structure)
	}
	return rec.zone.String() + " room"
}

// fixturePhrase names placements for the log: "2 bunks", "a scumhouse and a
// storage container".
func fixturePhrase(pl []placement) string {
	counts := map[Terrain]int{}
	var kinds []Terrain
	for _, f := range pl {
		if counts[f.kind] == 0 {
			kinds = append(kinds, f.kind)
		}
		counts[f.kind]++
	}
	var parts []string
	for _, k := range kinds {
		if n := counts[k]; n == 1 {
			parts = append(parts, withArticle(k.String()))
		} else {
			parts = append(parts, fmt.Sprintf("%d %s", n, pluralNoun(k.String())))
		}
	}
	switch len(parts) {
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	}
	return fmt.Sprintf("%d fixtures", len(pl))
}

// placeFixtures puts us into the colony's rooms (improveRooms), or marks out
// a new room from r when none can take any, and reports whether it did
// either.
//
// Two rules keep it from scattering small rooms. With wait, a room of the
// zone still going up (or being reshaped) is waited for rather than joined
// by another: once it stands it can take them. And a recipe with an aisle
// falls back to a narrow room only for the colony's first fixture of its
// kind: a narrow room squeezed into the gap between two others can never
// grow. Storage and life support do not wait: a full inventory can stall
// every project in flight, the storage room going up included, and a storage
// room anywhere is what breaks that; a toilet is not worth a queue. Storage
// keeps its narrow fallback too, for the same reason (and every settler's
// ship locker is a chest, so "the first" would never come).
func (w *World) placeFixtures(r roomRecipe, us []fixtureUnit, wait bool) bool {
	if w.improveRooms(us) {
		return true
	}
	if len(us) > 0 && wait && w.roomGoingUp(kindsZone(us[0])) {
		return false
	}
	if r.kinds[0] != Storage {
		r.aisleRequired = r.aisleRequired || (r.aisle && w.plannedFacilities(r.kinds[0]) > 0)
	}
	return w.planRoomFor(r, Community)
}

// roomGoingUp reports whether one of the colony's rooms of zone is being
// built or reshaped: the planner waits for it rather than marking out
// another room beside it (see placeFixtures).
func (w *World) roomGoingUp(zone ZoneKind) bool {
	if !w.cfg.RoomExpansion {
		return false
	}
	for _, p := range w.projects {
		if rec := p.room; rec != nil && rec.zone == zone && rec.issuer == Community {
			return true
		}
	}
	return false
}

// tidyRooms joins rooms that stand side by side even when the colony wants
// no more of their fixtures: the planner's last call, when it has nothing
// else to build. One merger at a time, zones in a fixed order.
func (w *World) tidyRooms() bool {
	if !w.cfg.RoomMerge {
		return false
	}
	st := w.roomPlanState()
	for _, z := range [...]ZoneKind{ZoneProduction, ZoneResidence, ZoneStorage} {
		var recs []*roomRecord
		for _, rec := range w.roomRecords {
			if rec.zone == z && rec.issuer == Community && !st.busy[rec] {
				recs = append(recs, rec)
			}
		}
		if w.mergeRooms(recs, nil, 0, st) {
			return true
		}
	}
	return w.consolidateRooms(st)
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

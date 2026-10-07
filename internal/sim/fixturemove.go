package sim

import "fmt"

// ---- Moving fixtures ------------------------------------------------------------------
//
// A fixture can move. The colony uses it to empty a small room into a bigger
// one of the same zone nearby, and then clears the empty shell away
// (consolidateRooms): rooms the colony marked out before it could fit
// fixtures into the rooms it had (see roomplan.go), or two lone ones left
// standing apart, end up as one.
//
// A move is a project of two phases. The fixture goes up in its new place
// first, costing no materials (buildTask.moved), since the old one is not
// salvaged. Then the old place comes down (buildTask.moveTo), and as it does
// everything that was the old fixture's goes to the new one
// (relocateFixture): its depot's goods and an incubator's growth, its owner
// and who may use it, a stove's pantry, a cook's claim, a keeper's trough.
// Building first means the colony is never a fixture short while one moves.
// See docs/room-expansion.md.

// relocateFixture moves what belongs to the fixture at from to the fixture of
// the same kind just built at to. The old fixture is still standing; its
// teardown follows (demolish), finding nothing left to empty or let go.
func (w *World) relocateFixture(from, to Point) {
	t := w.TerrainAt(from)
	if w.TerrainAt(to) != t {
		return // the new place is gone: the old comes down the usual way
	}
	if hasDepot(t) {
		w.closeDepotMarket(from)
		if c := w.storageContainers[from]; c != nil {
			delete(w.storageContainers, from)
			c.Pos = to
			w.storageContainers[to] = c
		}
	}
	if f := w.fixtures[from]; f != nil && w.fixtures[to] != nil {
		w.setFixtureOwner(to, f.Owner, f.Access)
	}
	if pantry, ok := w.pantryOf[from]; ok {
		delete(w.pantryOf, from)
		w.pantryOf[to], w.pantryHouse[pantry] = pantry, to
	}
	if house, ok := w.pantryHouse[from]; ok {
		delete(w.pantryHouse, from)
		w.pantryHouse[to], w.pantryOf[house] = house, to
	}
	if id, ok := w.workshopClaims[from]; ok {
		delete(w.workshopClaims, from)
		w.workshopClaims[to] = id
	}
	if id, ok := w.scumClaims[from]; ok {
		delete(w.scumClaims, from)
		w.scumClaims[to] = id
	}
	for _, e := range w.entities {
		if e.hasTrough && e.trough == from {
			e.trough = to
		}
		if e.pet != nil && e.pet.hasTrough && e.pet.trough == from {
			e.pet.trough = to
		}
		if e.hasKitchen && e.kitchen == from {
			e.kitchen = to
		}
	}
	w.fixtureRev++
}

// fixtureMove is one fixture moving: from where it stands to where it goes.
type fixtureMove struct {
	from, to Point
	kind     Terrain
}

// designateMove marks out moves from src into dst as one project, paid for by
// the colony, and reports whether it could pay.
func (w *World) designateMove(src, dst *roomRecord, moves []fixtureMove) bool {
	p := &project{id: w.nextProjectID, name: w.roomTitle(src) + " move", queuedTick: w.tick,
		issuer: Community, room: dst, from: src}
	for _, m := range moves {
		to := m.to
		p.tasks = append(p.tasks,
			&buildTask{pos: m.to, terrain: m.kind, phase: roomFitPhase, moved: true},
			&buildTask{pos: m.from, terrain: Floor, clears: m.kind, phase: roomMovePhase, moveTo: &to})
	}
	for _, t := range p.tasks {
		t.proj = p
	}
	if !w.fundProject(p) {
		return false
	}
	w.nextProjectID++
	w.projects = append(w.projects, p)
	if s := dst.structure; s != nil {
		p.structure = s
		w.growStructure(s, p, nil)
	}
	return true
}

// consolidateMost is the most fixtures a room may hold to be emptied into
// another: a room of one or two is what is worth the walk and the walls.
const consolidateMost = 2

// consolidateReach is how far apart, door to door, two rooms may stand for
// one to be emptied into the other.
const consolidateReach = 24

// consolidateRooms clears away an empty room of the colony's, or empties a
// small one (consolidateMost fixtures or fewer) into a room of its zone
// within consolidateReach that has floor for all of them and holds more,
// and reports whether it did either. One at a time, rooms oldest first, the
// planner's last call: it buys nothing the colony lacks.
//
// Only into a bigger room: two rooms of one fixture each would otherwise
// both be candidates for the other, and a colony of many (800 storage rooms
// in BenchmarkTidyNoFit's world) would lay out every pair within reach
// every planning cycle it had nothing else to do. Small rooms are rare, so
// the search is the small rooms against the bigger ones.
func (w *World) consolidateRooms(st roomPlanState) bool {
	limit := w.cfg.RoomMaxFacilities
	var recs []*roomRecord
	for _, rec := range w.roomRecords {
		if rec.issuer == Community && !st.busy[rec] {
			recs = append(recs, rec)
		}
	}
	counts := make(map[*roomRecord]int, len(recs))
	var dsts []*roomRecord // rooms of two or more: only a bigger room takes a small one's fixtures
	for _, rec := range recs {
		counts[rec] = w.roomFixtures(rec, st)
		if counts[rec] > 1 {
			dsts = append(dsts, rec)
		}
	}
	for _, src := range recs {
		n := counts[src]
		if src.structure == nil || n > consolidateMost {
			continue
		}
		if n == 0 {
			if w.clearShell(src) {
				return true
			}
			continue
		}
		us, from := w.roomUnits(src)
		for _, dst := range dsts {
			m := counts[dst]
			if dst == src || m <= n || dst.zone != src.zone || roomDistance(src, dst) > consolidateReach {
				continue
			}
			l := w.layoutFor(dst.lo, dst.hi, dst.doors, st)
			pl := l.place(us, limit)
			if len(pl) != len(from) {
				continue
			}
			moves := make([]fixtureMove, len(pl))
			for i, f := range pl {
				moves[i] = fixtureMove{from: from[i], to: f.pos, kind: f.kind}
			}
			if w.designateMove(src, dst, moves) {
				w.logEvent(LogBuildStart, fmt.Sprintf("The colony moves the fixtures of a small %s into a %s, to clear the room away.",
					w.roomTitle(src), w.roomTitle(dst)))
				return true
			}
		}
	}
	return false
}

// roomUnits is what stands in rec as the planner places it, with where each
// fixture stands, in the same order: a stove and its pantry together, every
// other fixture alone, row-major.
func (w *World) roomUnits(rec *roomRecord) ([]fixtureUnit, []Point) {
	var us []fixtureUnit
	var from []Point
	taken := map[Point]bool{}
	for y := rec.lo.Y; y <= rec.hi.Y; y++ {
		for x := rec.lo.X; x <= rec.hi.X; x++ {
			p := Point{x, y, LandingLevel}
			t := w.TerrainAt(p)
			if FixtureZone(t) == NoZone || taken[p] {
				continue
			}
			if pantry, ok := w.pantryOf[p]; ok && t == Scumhouse && inBox(rec.lo, rec.hi, pantry) {
				us = append(us, fixtureUnit{Scumhouse, Storage})
				from = append(from, p, pantry)
				taken[pantry] = true
				continue
			}
			us = append(us, fixtureUnit{t})
			from = append(from, p)
		}
	}
	return us, from
}

// roomDistance is how far apart two rooms' first doorways are.
func roomDistance(a, b *roomRecord) int {
	if len(a.doors) == 0 || len(b.doors) == 0 {
		return 1 << 30
	}
	return a.doors[0].Chebyshev(b.doors[0])
}

// clearShell orders an empty room's walls cleared away, a wall it shares
// with a room that stays excepted (tilesToClear), and reports whether it
// could pay. The room's record goes when its last wall does (forget).
func (w *World) clearShell(rec *roomRecord) bool {
	p := w.startClearing(w.tilesToClear([]*structure{rec.structure}))
	if p == nil {
		return false
	}
	p.from = rec
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony clears away an empty %s.", w.roomTitle(rec)))
	return true
}

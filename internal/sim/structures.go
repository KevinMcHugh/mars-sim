package sim

import (
	"fmt"
	"sort"
)

// ---- Structure types -------------------------------------------------------------
//
// A structure is something put up as one piece: a room the colony (or a
// colonist) built, with its walls and fixtures, a colony ship, or a lone
// fixture raised in an emergency. Its type says what it is for, and its type
// is what zoning reads: each type belongs to one zone kind, and with manual
// zoning a structure is only built, and may only stand, inside a zone of that
// kind. See zones.go and docs/zoning.md.
//
// The type, not the terrain, carries the tag, because a room mixes fixtures: a
// kitchen's pantry is a storage chest, but it stands in production with its
// stove, and a ship's lockers are chests in a home.

// StructureType is what a structure is for.
type StructureType uint8

const (
	StructNone StructureType = iota
	StructFacilityRoom
	StructDormitory
	StructHouse
	StructMeetingHall
	StructShip
	StructStorageRoom
	StructKitchen
	StructIncubatorRoom
	StructTrashRoom
	StructFoundry

	numStructureTypes // keep last
)

// structureSpec is a structure type's name and the zone it belongs in.
type structureSpec struct {
	name string
	zone ZoneKind
}

// structureSpecs tags every structure type with its zone. Moving a type to
// another zone ("scumhouses are storage") is one edit here; a new type is a
// row here and the recipe or arrival that builds it naming it.
var structureSpecs = [numStructureTypes]structureSpec{
	StructNone:          {name: "structure"},
	StructFacilityRoom:  {name: "facility room", zone: ZoneResidence},
	StructDormitory:     {name: "dormitory", zone: ZoneResidence},
	StructHouse:         {name: "house", zone: ZoneResidence},
	StructMeetingHall:   {name: "meeting hall", zone: ZoneResidence},
	StructShip:          {name: "colony ship", zone: ZoneResidence},
	StructStorageRoom:   {name: "storage room", zone: ZoneStorage},
	StructKitchen:       {name: "scumhouse", zone: ZoneProduction},
	StructIncubatorRoom: {name: "scum incubator", zone: ZoneProduction},
	StructTrashRoom:     {name: "trash room", zone: ZoneProduction},
	StructFoundry:       {name: "foundry", zone: ZoneProduction},
}

func (t StructureType) String() string {
	if t < numStructureTypes {
		return structureSpecs[t].name
	}
	return "unknown"
}

// Zone is the zone kind a structure of this type belongs in.
func (t StructureType) Zone() ZoneKind {
	if t < numStructureTypes {
		return structureSpecs[t].zone
	}
	return NoZone
}

// StructureTypes lists every structure type, in order (StructNone excluded).
func StructureTypes() []StructureType {
	out := make([]StructureType, 0, numStructureTypes-1)
	for t := StructNone + 1; t < numStructureTypes; t++ {
		out = append(out, t)
	}
	return out
}

// looseStructure is the type of a lone fixture raised outside any room (the
// emergency build): the room it would otherwise have stood in.
func looseStructure(t Terrain) StructureType {
	switch t {
	case NutrientPod, Toilet:
		return StructFacilityRoom
	case Bed:
		return StructDormitory
	case Storage:
		return StructStorageRoom
	case Scumhouse:
		return StructKitchen
	case Incubator:
		return StructIncubatorRoom
	case Incinerator:
		return StructTrashRoom
	case Forge, GunBench:
		return StructFoundry
	case Chair:
		return StructMeetingHall
	}
	return StructNone
}

// ---- The registry -------------------------------------------------------------------

// structure is one standing (or rising) structure.
type structure struct {
	id  int
	typ StructureType
	// tiles is every tile it builds on, sorted: walls, hull, fixtures —
	// including a party wall it borrowed from a neighbour, so clearing the
	// neighbour leaves that wall up. A room still going up lists its
	// unbuilt tiles too.
	tiles []Point
	// area is the ground that has to lie in a zone of its type: its whole
	// footprint, less a party wall it borrowed. x0..y1 bound it.
	area           []Point
	x0, y0, x1, y1 int
	// doors are the reserved tiles outside its doorways (see doorTiles),
	// released when the structure goes.
	doors []Point
	// building is the project raising it, while it rises; room is the
	// room's record (roomgrow.go), which goes with it.
	building *project
	room     *roomRecord
	// ship is a colony ship's, and lock the ground it holds as residence:
	// its shape and the walkway round it.
	ship *Ship
	lock []Point
}

// registerStructure records a structure and indexes its tiles.
func (w *World) registerStructure(typ StructureType, tiles, area []Point) *structure {
	w.nextStructureID++
	s := &structure{id: w.nextStructureID, typ: typ, tiles: sortedPoints(tiles), area: area}
	bound := area
	if len(bound) == 0 {
		bound = s.tiles
	}
	for i, p := range bound {
		if i == 0 {
			s.x0, s.y0, s.x1, s.y1 = p.X, p.Y, p.X, p.Y
			continue
		}
		s.x0, s.y0 = min(s.x0, p.X), min(s.y0, p.Y)
		s.x1, s.y1 = max(s.x1, p.X), max(s.y1, p.Y)
	}
	for _, p := range s.tiles {
		w.structureAt[p] = append(w.structureAt[p], s.id)
	}
	w.structures[s.id] = s
	w.structureRev++
	return s
}

func sortedPoints(ps []Point) []Point {
	out := append([]Point(nil), ps...)
	sort.Slice(out, func(i, j int) bool { return lessPoint(out[i], out[j]) })
	return out
}

// sortedStructures returns every structure by id: the registry is a map, and
// nothing may depend on its order.
func (w *World) sortedStructures() []*structure {
	out := make([]*structure, 0, len(w.structures))
	for _, s := range w.structures {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

// registerRoom records the room a project raises, in frame f. Its tiles are
// every wall and fixture task plus any wall it borrows; its area is the
// footprint zoneRoomTiles walks. With zoning-auto the colony zones that area
// for the room, where it is not zoned already.
func (w *World) registerRoom(r roomRecipe, p *project, f roomFrame) *structure {
	var tiles, area []Point
	for _, t := range p.tasks {
		if t.terrain != Floor {
			tiles = append(tiles, t.pos)
		}
	}
	for v := roomBackV; v <= roomFrontV; v++ {
		for u := -1; u <= f.width; u++ {
			if w.borrowedWall(f, u, v) {
				tiles = append(tiles, f.at(u, v))
			}
		}
	}
	w.zoneRoomTiles(f, func(q Point) { area = append(area, q) })
	s := w.registerStructure(r.structure, tiles, area)
	s.doors, s.building, s.room = []Point{f.doorStep()}, p, p.room
	p.structure = s
	if p.room != nil {
		p.room.structure = s
	}
	w.autoZone(area, r.structure.Zone())
	return s
}

// autoZone zones the unzoned, unheld tiles of area as k, with zoning-auto:
// the colony zoning what it builds.
func (w *World) autoZone(area []Point, k ZoneKind) {
	if !w.cfg.ZoningAuto {
		return
	}
	for _, q := range area {
		if w.zoneAt(q) == NoZone && !w.zoneLocked(q) {
			w.setZone(q, k)
		}
	}
}

// growStructure adds an expansion's tiles to its room's structure: the walls
// and fixtures p raises, and the strip it takes in (see designateExpansion).
func (w *World) growStructure(s *structure, p *project, strip []Point) {
	if s == nil {
		return
	}
	for _, t := range p.tasks {
		if t.terrain != Floor {
			s.tiles = append(s.tiles, t.pos)
			w.structureAt[t.pos] = append(w.structureAt[t.pos], s.id)
		}
	}
	s.tiles = sortedPoints(s.tiles)
	s.area = append(s.area, strip...)
	for _, q := range strip {
		s.x0, s.y0 = min(s.x0, q.X), min(s.y0, q.Y)
		s.x1, s.y1 = max(s.x1, q.X), max(s.y1, q.Y)
	}
	s.building = p
	w.autoZone(strip, s.typ.Zone())
	w.structureRev++
}

// registerShip records a colony ship that has just come down, and holds its
// shape and the walkway round it as residence for as long as it stands.
func (w *World) registerShip(sh *Ship) *structure {
	var tiles, area, lock []Point
	o := sh.Origin
	sh.layout.forEachTile(func(d Point, _ bool) {
		p := o.Add(d.X, d.Y)
		area = append(area, p)
		lock = append(lock, p)
		if isBuilt(w.TerrainAt(p)) {
			tiles = append(tiles, p)
		}
	})
	for _, d := range sh.layout.margin {
		lock = append(lock, o.Add(d.X, d.Y))
	}
	s := w.registerStructure(StructShip, tiles, area)
	for _, d := range sh.layout.doors {
		s.doors = append(s.doors, o.Add(d.X, d.Y))
	}
	s.ship, s.lock = sh, lock
	sh.structure = s
	for _, p := range lock {
		w.lockZone(p)
	}
	return s
}

// unregisterShip forgets a ship's structure without touching its tiles, for
// a ship lifted to land elsewhere (moveShip), which registers again where it
// comes down. The ground it held goes back to unzoned, unless another ship
// still holds it: it was residence only because the ship stood there, and a
// player trying sites before the first tick should not leave a trail of
// residence behind.
func (w *World) unregisterShip(sh *Ship) {
	s := sh.structure
	if s == nil || w.structures[s.id] != s {
		return
	}
	w.forget(s)
	for _, p := range s.lock {
		if !w.zoneLocked(p) {
			w.setZone(p, NoZone)
		}
	}
}

// registerLone records a lone fixture raised outside any room, and zones it
// with zoning-auto.
func (w *World) registerLone(p Point, t Terrain) {
	typ := looseStructure(t)
	w.registerStructure(typ, []Point{p}, []Point{p})
	if w.cfg.ZoningAuto && w.zoneAt(p) == NoZone && !w.zoneLocked(p) {
		w.setZone(p, typ.Zone())
	}
}

// hasProject reports whether p is still one of the colony's projects.
func (w *World) hasProject(p *project) bool {
	for _, q := range w.projects {
		if q == p {
			return true
		}
	}
	return false
}

// condemn readies a structure to come down: a room still going up is
// called off, its open orders refunded.
func (w *World) condemn(s *structure) {
	if s.building != nil && w.hasProject(s.building) {
		w.cancelProject(s.building)
	}
	s.building = nil
}

// maybeRetire forgets a structure once nothing of it stands and nothing is
// raising it.
func (w *World) maybeRetire(s *structure) {
	if w.structures[s.id] != s {
		return
	}
	if s.building != nil && w.hasProject(s.building) {
		return
	}
	for _, p := range s.tiles {
		if isBuilt(w.TerrainAt(p)) {
			return
		}
	}
	w.forget(s)
}

// forget drops a structure from the registry and lets go of what it held:
// its doorways, a ship's residence hold, and a room's record, so the room
// planner no longer tries to grow a room that is gone (see roomgrow.go).
func (w *World) forget(s *structure) {
	delete(w.structures, s.id)
	for _, p := range s.tiles {
		w.unindexStructureTile(p, s.id)
	}
	for _, d := range s.doors {
		delete(w.doorTiles, d)
	}
	for _, p := range s.lock {
		w.unlockZone(p)
	}
	if s.ship != nil && s.ship.structure == s {
		s.ship.structure = nil
	}
	if rec := s.room; rec != nil {
		for i, r := range w.roomRecords {
			if r == rec {
				w.roomRecords = append(w.roomRecords[:i], w.roomRecords[i+1:]...)
				break
			}
		}
		for p, r := range w.roomFloor {
			if r == rec {
				delete(w.roomFloor, p) // deleting while ranging is safe, and order decides nothing
			}
		}
	}
	w.structureRev++
}

func (w *World) unindexStructureTile(p Point, id int) {
	ids := w.structureAt[p]
	for i, x := range ids {
		if x == id {
			ids = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(ids) == 0 {
		delete(w.structureAt, p)
	} else {
		w.structureAt[p] = ids
	}
}

// cancelProject drops a project and refunds its open orders. Builders drop
// the job, so nobody is paid for a task that is gone. What was already built
// stays built.
func (w *World) cancelProject(p *project) {
	for _, t := range p.tasks {
		if t.owner != 0 {
			if e := w.entities[t.owner]; e != nil && e.task == t {
				w.clearJob(e)
			}
		}
		if t.order != nil && w.workOrders[t.order.ID] == t.order {
			w.closeWork(t.order)
		}
	}
	for i, q := range w.projects {
		if q == p {
			w.projects = append(w.projects[:i], w.projects[i+1:]...)
			break
		}
	}
	w.structureRev++
}

// ---- Tearing down ---------------------------------------------------------------------

// demolish clears one built tile back to floor: a clearing order's tile, a
// wall a growing room moves, or one a passage breaks through. A depot's
// goods go to the nearest chest that will take them first, still their
// owners'; a ship's trough and a chef's kitchen are let go by whoever kept
// them. A structure is forgotten once its last tile is down.
//
// SetTerrain does the rest, as it does for every change: the flow fields and
// the region graph hear of the new floor this tick. A clearing order also
// drops colonists' detours (clearTile).
func (w *World) demolish(p Point) {
	t := w.TerrainAt(p)
	if !isBuilt(t) {
		return
	}
	if hasDepot(t) {
		w.emptyDepot(p)
	}
	// Only a fixture is anything's by position. A wall must not unhook
	// anything: a growing kitchen's next stove is planned, and linked to its
	// pantry, on the very tile of the wall it tears down (see
	// designateExpansion), and letting go there dropped that link.
	if isFixtureTerrain(t) {
		w.letGoFixture(p)
	}
	w.SetTerrain(p, Floor)
	ids := append([]int(nil), w.structureAt[p]...)
	sort.Ints(ids)
	for _, id := range ids {
		if s := w.structures[id]; s != nil {
			w.maybeRetire(s)
		}
	}
	w.structureRev++
}

// clearTile takes down a clearing order's tile: demolish, and then, since
// the player cleared it to open the way, colonists already walking round it
// plan again (forgetDetours). Passages and growing rooms leave routes be, as
// they always have.
func (w *World) clearTile(p Point) {
	w.demolish(p)
	w.forgetDetours()
}

// forgetDetours drops every cached route that is longer than a straight
// walk to its end: one that may be going round what was just taken down. The
// shared flow fields repair themselves from SetTerrain; a route a colonist
// planned for itself is only replanned when it is blocked (travelTo), so
// without this a colonist already on its way would finish the long way
// round. A route that is already straight cannot get shorter and is kept.
func (w *World) forgetDetours() {
	for _, e := range w.entities {
		if e.pathAt >= len(e.path) {
			continue
		}
		if len(e.path)-e.pathAt > e.Pos.Chebyshev(e.path[len(e.path)-1]) {
			e.path, e.pathAt = e.path[:0], 0
		}
	}
}

// letGoFixture unhooks what refers to the fixture at p by position: a
// keeper's and its hens' trough, a chef's kitchen, a cook's claim, a
// kitchen's pantry link.
func (w *World) letGoFixture(p Point) {
	for _, e := range w.entities {
		if e.hasTrough && e.trough == p {
			e.hasTrough = false
		}
		if e.hasKitchen && e.kitchen == p {
			e.hasKitchen, e.kitchenCommissioned = false, false
		}
	}
	delete(w.workshopClaims, p)
	delete(w.scumClaims, p)
	if pantry, ok := w.pantryOf[p]; ok {
		delete(w.pantryHouse, pantry)
		delete(w.pantryOf, p)
	}
	if house, ok := w.pantryHouse[p]; ok {
		delete(w.pantryOf, house)
		delete(w.pantryHouse, p)
	}
}

// emptyDepot closes the market at a depot that is coming down and moves its
// goods out. Orders resting there are cancelled (an ask's goods go back on
// its seller's line, a bid's money back to its bidder), haul work to or from
// it is closed, and every ledger line moves, still its owner's, to the
// nearest chest that will take it: a communal one, or the owner's own. What
// fits nowhere is lost, and the log says so.
func (w *World) emptyDepot(p Point) {
	c := w.storageContainers[p]
	if c == nil {
		return
	}
	for _, o := range w.sortedOrders(func(o *Order) bool { return o.Depot == p }) {
		w.cancel(o)
	}
	for k := range w.books {
		if k.Depot == p {
			delete(w.books, k)
		}
	}
	for _, o := range w.sortedWork(func(o *WorkOrder) bool {
		return o.Kind == WorkHaul && (o.Pos == p || o.From == p)
	}) {
		delete(w.haulClaims, o.ID)
		w.closeWork(o)
	}
	moved, lost := 0, 0
	for _, l := range append([]LedgerLine(nil), c.Ledger...) {
		left := l.Count
		for left > 0 {
			dst := w.nearestChestFor(p, l.Owner, l.Item)
			if dst == nil {
				break
			}
			n := left
			for n > 1 && !dst.Inventory.CanAdd(l.Item, n) {
				n /= 2
			}
			if !c.debit(l.Owner, l.Item, n) || !dst.Inventory.Add(l.Item, n) {
				break
			}
			dst.credit(l.Owner, l.Item, n)
			left -= n
			moved += n
		}
		if left > 0 && c.debit(l.Owner, l.Item, left) {
			lost += left
		}
	}
	switch {
	case lost > 0:
		w.logEvent(LogBuildStart, fmt.Sprintf("Clearing the %s at (%d, %d): %d goods moved to other chests, %d lost with nowhere to go.",
			c.Terrain, p.X, p.Y, moved, lost))
	case moved > 0:
		w.logEvent(LogBuildStart, fmt.Sprintf("Clearing the %s at (%d, %d): %d goods moved to other chests.",
			c.Terrain, p.X, p.Y, moved))
	}
}

// nearestChestFor is the nearest chest other than from that can take one
// item of kind for owner: a communal chest, or one owner owns. Ties go to
// the lower row, then column.
func (w *World) nearestChestFor(from Point, owner Owner, kind ItemKind) *StorageContainer {
	var best *StorageContainer
	bestD := 0
	for q := range w.facilityTiles[Storage] {
		if q == from {
			continue
		}
		c := w.storageContainers[q]
		f := w.fixtures[q]
		if c == nil || f == nil || !(f.Access == AccessCommunal || f.Owner == owner) || !c.Inventory.CanAdd(kind, 1) {
			continue
		}
		if d := from.Chebyshev(q); best == nil || d < bestD || (d == bestD && lessPoint(q, best.Pos)) {
			best, bestD = c, d
		}
	}
	return best
}

// ---- Publishing -------------------------------------------------------------------------

// StructureView is a read-only copy of one structure, for the Zones tab: what
// it is, where it stands, and how much of it is built.
type StructureView struct {
	ID             int
	Type           StructureType
	X0, Y0, X1, Y1 int
	Built          int  // tiles standing
	Ship           bool // a colony ship (its ground is held as residence)
	Rising         bool // still going up
}

// publishedStructures copies the registry, reusing the last copy while
// nothing changed: structureRev moves when a structure is added or retired,
// when a project is dropped, and when any of a structure's tiles changes
// terrain (setTerrain).
func (w *World) publishedStructures() []StructureView {
	if w.snapStructures != nil && w.snapStructureRev == w.structureRev {
		return w.snapStructures
	}
	out := make([]StructureView, 0, len(w.structures))
	for _, s := range w.sortedStructures() {
		v := StructureView{ID: s.id, Type: s.typ, X0: s.x0, Y0: s.y0, X1: s.x1, Y1: s.y1, Ship: s.ship != nil,
			Rising: s.building != nil && w.hasProject(s.building)}
		for _, p := range s.tiles {
			if isBuilt(w.TerrainAt(p)) {
				v.Built++
			}
		}
		out = append(out, v)
	}
	w.snapStructures, w.snapStructureRev = out, w.structureRev
	return out
}

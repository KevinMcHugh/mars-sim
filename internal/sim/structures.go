package sim

import (
	"fmt"
	"sort"
)

// ---- Structure types -------------------------------------------------------------
//
// A structure is something put up as one piece: a room the colony (or a
// colonist) built, with its walls and fixtures, a crash pod, or a lone
// fixture raised in an emergency. Its type says what it is for, and its type
// is what zoning reads: each type belongs to one zone kind, and with manual
// zoning a structure is only built, and may only stand, inside a zone of that
// kind. See zones.go and docs/zoning.md.
//
// The type, not the terrain, carries the tag, because a room mixes fixtures: a
// kitchen's pantry is a storage chest, but it stands in production with its
// stove, and a crash pod's locker is a chest in a home.

// StructureType is what a structure is for.
type StructureType uint8

const (
	StructNone StructureType = iota
	StructFacilityRoom
	StructDormitory
	StructHouse
	StructMeetingHall
	StructCrashPod
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
	StructCrashPod:      {name: "crash pod", zone: ZoneResidence},
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
	// door is the reserved tile outside its doorway (see doorTiles),
	// released when the structure goes.
	door    Point
	hasDoor bool
	// building is the project raising it, while it rises.
	building *project
	// pod is a crash pod's origin; lock the ground it holds as residence.
	pod   Point
	isPod bool
	lock  []Point
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

// registerRoom records the room a project raises. Its tiles are every wall
// and fixture task plus a side wall it borrows; its area is the footprint
// zoneRoomTiles walks. With zoning-auto the colony zones that area for the
// room, where it is not zoned already.
func (w *World) registerRoom(r roomRecipe, p *project, o Point, width int, door Point) *structure {
	var tiles, area []Point
	for _, t := range p.tasks {
		if t.terrain != Floor {
			tiles = append(tiles, t.pos)
		}
	}
	backY, frontY := o.Y-1, roomFrontWallY(o.Y)
	for y := backY; y <= frontY; y++ {
		for _, x := range [2]int{o.X - 1, o.X + width} {
			if q := (Point{x, y}); w.TerrainAt(q) == Wall {
				tiles = append(tiles, q)
			}
		}
	}
	w.zoneRoomTiles(o, width, func(q Point) { area = append(area, q) })
	s := w.registerStructure(r.structure, tiles, area)
	s.door, s.hasDoor, s.building = door, true, p
	p.structure = s
	if w.cfg.ZoningAuto {
		for _, q := range area {
			if w.zoneAt(q) == NoZone && !w.zoneLocked(q) {
				w.setZone(q, r.structure.Zone())
			}
		}
	}
	return s
}

// registerPod records a crash pod that has just landed at o, and holds its
// footprint and margin as residence for as long as it stands.
func (w *World) registerPod(o Point, shareL, shareR bool) *structure {
	var tiles, area, lock []Point
	for dy := 0; dy < podHeight; dy++ {
		for dx := 0; dx < podWidth; dx++ {
			p := o.Add(dx, dy)
			lock = append(lock, p)
			if (dx == 0 && shareL) || (dx == podWidth-1 && shareR) {
				tiles = append(tiles, p) // the neighbour's hull, shared
				continue
			}
			area = append(area, p)
			if isBuilt(w.TerrainAt(p)) {
				tiles = append(tiles, p)
			}
		}
	}
	forEachPodMargin(o, shareL, shareR, func(p Point) { lock = append(lock, p) })
	s := w.registerStructure(StructCrashPod, tiles, area)
	s.door, s.hasDoor = o.Add(podApproach.X, podApproach.Y), true
	s.pod, s.isPod, s.lock = o, true, lock
	for _, p := range lock {
		w.lockZone(p)
	}
	return s
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
// raising it: it releases its door, and a pod its residence hold.
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
	delete(w.structures, s.id)
	for _, p := range s.tiles {
		w.unindexStructureTile(p, s.id)
	}
	if s.hasDoor && w.doorTiles[s.door] {
		delete(w.doorTiles, s.door)
	}
	if s.isPod {
		delete(w.pods, s.pod)
		for _, p := range s.lock {
			w.unlockZone(p)
		}
		for _, e := range w.entities {
			if e.hasPod && e.podOrigin == s.pod {
				e.hasPod = false
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

// demolish clears one built tile back to floor. A depot's goods go to the
// nearest chest that will take them first, still their owners'; a crash
// pod's trough and a chef's kitchen are let go by whoever kept them. A
// structure is forgotten once its last tile is down.
//
// SetTerrain does the rest, as it does for every change: the flow fields and
// the region graph hear of the new floor this tick, so nobody keeps routing
// round a wall that has gone.
func (w *World) demolish(p Point) {
	t := w.TerrainAt(p)
	if !isBuilt(t) {
		return
	}
	if hasDepot(t) {
		w.emptyDepot(p)
	}
	w.letGoFixture(p)
	w.SetTerrain(p, Floor)
	w.forgetDetours()
	ids := append([]int(nil), w.structureAt[p]...)
	sort.Ints(ids)
	for _, id := range ids {
		if s := w.structures[id]; s != nil {
			w.maybeRetire(s)
		}
	}
	w.structureRev++
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

// jobClear works a clearing task: walk beside the structure tile and take it
// down over demolish-ticks, then be paid. Someone else clearing it first
// ends the job.
func (w *World) jobClear(e *Entity) {
	if !isBuilt(w.TerrainAt(e.Target)) {
		w.clearJob(e)
		return
	}
	arrived, ok := w.travelTo(e, e.Target)
	if !ok {
		w.clearJob(e)
		return
	}
	if !arrived {
		e.State = Moving
		return
	}
	e.State = Building
	e.Progress++
	if e.Progress < w.workTicks(e, SkillConstruction, w.cfg.DemolishTicks) {
		return
	}
	what := w.TerrainAt(e.Target)
	w.demolish(e.Target)
	w.practise(e, SkillConstruction, w.cfg.DemolishTicks)
	if t := e.task; t != nil {
		w.payWork(t.order, e)
	}
	o := w.occurrence(e, ActionClear, nil, e.Target, "Cleared away a %s at (%d, %d).", what, e.Target.X, e.Target.Y)
	o.Object = FactRef{Noun: NounStructure, Label: what.String()}
	w.emitOccurrence(o)
	w.clearJob(e)
}

// ---- Publishing -------------------------------------------------------------------------

// StructureView is a read-only copy of one structure, for the Zones tab: what
// it is, where it stands, and how much of it is built.
type StructureView struct {
	ID             int
	Type           StructureType
	X0, Y0, X1, Y1 int
	Built          int  // tiles standing
	Pod            bool // a crash pod (its ground is held as residence)
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
		v := StructureView{ID: s.id, Type: s.typ, X0: s.x0, Y0: s.y0, X1: s.x1, Y1: s.y1, Pod: s.isPod,
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

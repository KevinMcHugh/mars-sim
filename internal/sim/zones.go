package sim

import (
	"fmt"
	"sort"
	"strings"
)

// ---- Zones -----------------------------------------------------------------
//
// A zone marks ground for one use: residence, storage, or production. Every
// structure type belongs to one zone kind (structures.go), and with manual
// zoning (the default) the colony builds a structure only inside a zone of
// its kind: a dormitory in residence, a silo in storage, a scumhouse in
// production. With no such zone it builds nothing of that kind.
//
// Zoning is per tile: a tile holds at most one zone kind, so zones never
// overlap. The player paints rectangles, and a rectangle painted over another
// kind replaces it where they meet; two rectangles of one kind side by side
// are one zone of any shape. Painting is free. What it costs is the work it
// implies, bought from the treasury like any public work:
//
//   - seen rock inside a new zone is dug out (an excavation order), and
//   - a structure left outside a zone of its kind — painted over with another
//     kind, or unzoned — is cleared (a clearing order).
//
// A paint whose work the treasury cannot cover changes nothing.
//
// The ground around colony ships is always residence: each ship holds its
// shape and the walkway round it as residence for as long as it stands
// (zoneCell.locks), and painting skips those tiles.
//
// With zoning-auto on, the colony sites rooms itself, as it always did:
// inside a zone of the room's kind when one has a site, otherwise on any
// ground not zoned for something else, which it then zones for the room.
// See docs/zoning.md.

// ZoneKind is what a zone is for. NoZone is unzoned ground.
type ZoneKind uint8

const (
	NoZone ZoneKind = iota
	ZoneResidence
	ZoneStorage
	ZoneProduction

	numZoneKinds // keep last
)

// zoneSpec is a zone kind's name, as commands and the wire spell it, and the
// colour a frontend tints it.
type zoneSpec struct {
	name  string
	color string // "#rrggbb"
}

// zoneSpecs is the one table to edit to add a zone kind: a row here, and
// structure types that point at it (structureSpecs).
var zoneSpecs = [numZoneKinds]zoneSpec{
	NoZone:         {name: "none"},
	ZoneResidence:  {name: "residence", color: "#4caf50"},
	ZoneStorage:    {name: "storage", color: "#3d8bfd"},
	ZoneProduction: {name: "production", color: "#9e9e9e"},
}

func (k ZoneKind) String() string {
	if k < numZoneKinds {
		return zoneSpecs[k].name
	}
	return "unknown"
}

// Color is the "#rrggbb" a frontend tints this zone kind with; "" for NoZone.
func (k ZoneKind) Color() string {
	if k < numZoneKinds {
		return zoneSpecs[k].color
	}
	return ""
}

// ZoneKinds lists every kind a tile can be zoned, in order (NoZone excluded).
func ZoneKinds() []ZoneKind {
	out := make([]ZoneKind, 0, numZoneKinds-1)
	for k := NoZone + 1; k < numZoneKinds; k++ {
		out = append(out, k)
	}
	return out
}

// ParseZoneKind reads a zone kind by name; "none" is NoZone.
func ParseZoneKind(s string) (ZoneKind, bool) {
	for k := NoZone; k < numZoneKinds; k++ {
		if zoneSpecs[k].name == s {
			return k, true
		}
	}
	return NoZone, false
}

// zoneCell is one tile's zoning: its kind, and how many standing colony ships
// hold it as residence (painting skips a held tile).
type zoneCell struct {
	kind  ZoneKind
	locks uint8
}

// zonable reports whether p can be zoned: on the map, on the landing level.
// Zones are the colony's plan for where it lives, and it lives on the landing
// level, so the zone grid covers that level only (see docs/layers.md); a
// deeper tile reads as unzoned and painting it does nothing.
func (w *World) zonable(p Point) bool {
	return p.Level == LandingLevel && w.InBounds(p)
}

// zoneAt is the zone kind of p; NoZone off the map or off the landing level.
func (w *World) zoneAt(p Point) ZoneKind {
	if !w.zonable(p) {
		return NoZone
	}
	return w.zones.at(p.X, p.Y).kind
}

// zoneLocked reports whether a colony ship holds p as residence.
func (w *World) zoneLocked(p Point) bool {
	return w.zonable(p) && w.zones.at(p.X, p.Y).locks > 0
}

// setZone zones p as k (NoZone unzones it), keeping the per-kind counts and
// the revision publishing reads. It does not look at locks: callers decide.
func (w *World) setZone(p Point, k ZoneKind) {
	if !w.zonable(p) {
		return
	}
	if old := w.zones.at(p.X, p.Y).kind; old == k {
		return
	} else {
		w.zoneTiles[old]--
	}
	w.zones.ptr(p.X, p.Y).kind = k
	w.zoneTiles[k]++
	w.zoneRev++
}

// lockZone holds p as residence for a colony ship (unlock releases one hold).
func (w *World) lockZone(p Point) {
	if !w.zonable(p) {
		return
	}
	w.setZone(p, ZoneResidence)
	if c := w.zones.ptr(p.X, p.Y); c.locks < 255 {
		c.locks++
	}
	w.zoneRev++
}

func (w *World) unlockZone(p Point) {
	if !w.zonable(p) {
		return
	}
	if c := w.zones.ptr(p.X, p.Y); c.locks > 0 {
		c.locks--
	}
	w.zoneRev++
}

// zoneAllows reports whether a structure zoned k may be built on p: inside a
// zone of k with manual zoning, and also on unzoned ground with zoning-auto.
func (w *World) zoneAllows(p Point, k ZoneKind) bool {
	z := w.zoneAt(p)
	return z == k || (w.cfg.ZoningAuto && z == NoZone)
}

// ---- Painting zones --------------------------------------------------------------

// PaintZone zones the rectangle with corners (X0, Y0) and (X1, Y1),
// inclusive, in either order, as Kind; NoZone removes zoning instead. Tiles
// a colony ship holds are skipped. Rock the colony has seen inside a new zone
// is dug out, and any structure the paint leaves outside a zone of its kind
// is cleared, both paid from the treasury; when it cannot pay for all of it
// the paint changes nothing. The outcome is logged.
type PaintZone struct {
	Kind           ZoneKind
	X0, Y0, X1, Y1 int
}

func (PaintZone) isCommand() {}

// zonePaint is what painting a rectangle would do, worked out before any of
// it is done.
type zonePaint struct {
	kind    ZoneKind
	tiles   []Point      // tiles whose zone changes, row-major
	locked  int          // tiles in the rectangle a colony ship holds
	evicted []*structure // structures left outside a zone of their kind, by id
	clear   []Point      // their built tiles to clear, row-major
	dig     []Point      // seen, unmarked rock to dig out, row-major
}

// cost is what the paint's work would escrow.
func (z *zonePaint) cost(w *World) Money {
	return Money(len(z.dig))*w.wageFor(Floor) + Money(len(z.clear))*Money(w.cfg.WageDemolish)
}

// clampRect orders a rectangle's corners and clips it to the map.
func (w *World) clampRect(x0, y0, x1, y1 int) (int, int, int, int) {
	x0, x1 = min(x0, x1), max(x0, x1)
	y0, y1 = min(y0, y1), max(y0, y1)
	return max(x0, 0), max(y0, 0), min(x1, w.Width-1), min(y1, w.Height-1)
}

// planZonePaint works out a paint without changing anything.
func (w *World) planZonePaint(c PaintZone) *zonePaint {
	x0, y0, x1, y1 := w.clampRect(c.X0, c.Y0, c.X1, c.Y1)
	z := &zonePaint{kind: c.Kind}
	if x0 > x1 || y0 > y1 {
		return z
	}
	changed := make(map[Point]bool)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			p := Point{x, y, LandingLevel}
			if w.zoneLocked(p) {
				z.locked++
				continue
			}
			if w.zoneAt(p) != c.Kind {
				z.tiles = append(z.tiles, p)
				changed[p] = true
			}
		}
	}
	// A structure whose ground is painted another kind, or unzoned, has to
	// go: it may stand only inside a zone of its own kind.
	for _, s := range w.sortedStructures() {
		if s.x1 < x0 || s.x0 > x1 || s.y1 < y0 || s.y0 > y1 {
			continue
		}
		for _, p := range s.area {
			if changed[p] && c.Kind != s.zone {
				z.evicted = append(z.evicted, s)
				break
			}
		}
	}
	z.clear = w.tilesToClear(z.evicted)
	if c.Kind != NoZone {
		z.dig = w.unmarkedRock(LandingLevel, x0, y0, x1, y1)
	}
	return z
}

// paintZone carries out a PaintZone, and reports whether it did.
func (w *World) paintZone(c PaintZone) bool {
	if c.Kind >= numZoneKinds {
		return false
	}
	z := w.planZonePaint(c)
	if len(z.tiles) == 0 && len(z.dig) == 0 {
		if z.locked > 0 {
			w.logEvent(LogBuildStart, "That ground is held as residence by the colony ship on it.")
		} else {
			w.logEvent(LogBuildStart, fmt.Sprintf("That area is already zoned %s.", c.Kind))
		}
		return false
	}
	if cost := z.cost(w); cost > 0 && w.balance(Community) < cost {
		w.logEvent(LogBuildStart, fmt.Sprintf("The treasury cannot pay %v for the work that zoning would take (%s).",
			cost, z.workPhrase(w)))
		return false
	}
	w.playerZoned = true
	for _, s := range z.evicted {
		w.condemn(s)
	}
	for _, p := range z.tiles {
		w.setZone(p, c.Kind)
	}
	var notes []string
	if len(z.clear) > 0 {
		if p := w.startClearing(z.clear); p != nil {
			notes = append(notes, fmt.Sprintf("%d tiles of %s to clear, %v escrowed",
				len(z.clear), w.structureList(z.evicted), w.projectCost(p)))
		}
	}
	if len(z.dig) > 0 {
		if p := w.startExcavation(z.dig); p != nil {
			notes = append(notes, fmt.Sprintf("%d tiles of rock to dig out, %v escrowed", len(z.dig), w.projectCost(p)))
		}
	}
	for _, s := range z.evicted {
		w.maybeRetire(s)
	}
	verb := fmt.Sprintf("zones %d tiles %s", len(z.tiles), c.Kind)
	if c.Kind == NoZone {
		verb = fmt.Sprintf("unzones %d tiles", len(z.tiles))
	}
	msg := "The colony " + verb
	if len(notes) > 0 {
		msg += ": " + strings.Join(notes, "; ")
	}
	if z.locked > 0 {
		msg += fmt.Sprintf(" (%d tiles around colony ships stay residence)", z.locked)
	}
	w.logEvent(LogBuildStart, msg+".")
	return true
}

// workPhrase describes a paint's work for a refusal.
func (z *zonePaint) workPhrase(w *World) string {
	var parts []string
	if n := len(z.dig); n > 0 {
		parts = append(parts, fmt.Sprintf("%d tiles of rock to dig", n))
	}
	if n := len(z.clear); n > 0 {
		parts = append(parts, fmt.Sprintf("%d tiles of %s to clear", n, w.structureList(z.evicted)))
	}
	return strings.Join(parts, ", ")
}

// structureList names structures for the log: "a dormitory and 2 storage
// rooms", in the order their names first come up.
func (w *World) structureList(ss []*structure) string {
	counts := map[string]int{}
	var names []string
	for _, s := range ss {
		n := w.structureName(s)
		if counts[n] == 0 {
			names = append(names, n)
		}
		counts[n]++
	}
	var parts []string
	for _, n := range names {
		if counts[n] == 1 {
			parts = append(parts, withArticle(n))
		} else {
			parts = append(parts, fmt.Sprintf("%d %s", counts[n], pluralNoun(n)))
		}
	}
	switch len(parts) {
	case 0:
		return "structures"
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// pluralNoun is a structure or fixture name's plural: "dormitories", "bunks".
func pluralNoun(n string) string {
	if strings.HasSuffix(n, "y") && !strings.HasSuffix(n, "ey") {
		return n[:len(n)-1] + "ies"
	}
	return n + "s"
}

// unmarkedRock is the rock in a rectangle that an excavation could take: seen
// by the colony, not a reserved door tile, and not already some project's
// unfinished task.
func (w *World) unmarkedRock(level Level, x0, y0, x1, y1 int) []Point {
	if w.layer(level) == nil {
		return nil // a level nobody has broken into has nothing seen
	}
	taken := w.markedTiles()
	var out []Point
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			p := Point{x, y, level}
			if w.TerrainAt(p) != Rock || !w.discovered(p) || w.doorTiles[p] || taken[p] {
				continue
			}
			out = append(out, p)
		}
	}
	return out
}

// markedTiles is every tile some project still has unfinished work on.
func (w *World) markedTiles() map[Point]bool {
	taken := make(map[Point]bool)
	for _, p := range w.projects {
		for _, t := range p.tasks {
			if !w.taskDone(t) {
				taken[t.pos] = true
			}
		}
	}
	return taken
}

// ---- Clearing orders ---------------------------------------------------------------

// ClearingName is the project a clearing order creates, as ExcavationName is
// an excavation's.
const ClearingName = "clearing"

// ClearArea orders every structure tile the colony has seen in the rectangle
// cleared back to floor: walls, ships' hulls, fixtures, but never a stair,
// shaft or hole (joinsLevels). It is a paid work order, all or nothing, and
// any room still going up in the area is called off. Nothing about zoning
// changes. The outcome is logged.
type ClearArea struct {
	X0, Y0, X1, Y1 int
	Level          Level // the zero value is the landing level (orderLevel)
}

func (ClearArea) isCommand() {}

// CancelClear closes a clearing order by its project ID: the orders still
// open are refunded to the treasury, and what was not yet cleared stands.
type CancelClear struct{ ID int }

func (CancelClear) isCommand() {}

// isBuilt reports whether terrain is a structure: anything but rock and floor.
func isBuilt(t Terrain) bool { return t != Rock && t != Floor }

// joinsLevels reports whether t is a stair, shaft or hole: clearing one end
// would leave the other dangling on a level the player may not be looking
// at, so a clearing leaves them be.
func joinsLevels(t Terrain) bool { return t >= StairDown && t <= Hole }

// orderLevel is the level a player's area order names. The zero value (the
// surface) means the landing level, so an order that predates levels, or a
// caller that never sets one, keeps meaning the map it always did; there is
// nothing on the surface to dig or clear until Z6 (see docs/z-levels.md).
func orderLevel(l Level) Level {
	if l == SurfaceLevel {
		return LandingLevel
	}
	return l
}

// clearArea carries out a ClearArea, and reports whether it did.
func (w *World) clearArea(c ClearArea) bool {
	x0, y0, x1, y1 := w.clampRect(c.X0, c.Y0, c.X1, c.Y1)
	level := orderLevel(c.Level)
	if w.layer(level) == nil {
		x0, x1 = 1, 0 // a level nobody has broken into: nothing to clear
	}
	taken := w.markedTiles()
	var tiles []Point
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			p := Point{x, y, level}
			if t := w.TerrainAt(p); isBuilt(t) && !joinsLevels(t) && w.discovered(p) && !taken[p] {
				tiles = append(tiles, p)
			}
		}
	}
	// Rooms going up in the area are called off: clearing part of one
	// would only have it rebuilt.
	var rooms []*project
	for _, p := range w.projects {
		if p.workKind != WorkBuild {
			continue
		}
		for _, t := range p.tasks {
			if t.pos.Level == level && t.pos.X >= x0 && t.pos.X <= x1 && t.pos.Y >= y0 && t.pos.Y <= y1 && !w.taskDone(t) {
				rooms = append(rooms, p)
				break
			}
		}
	}
	if len(tiles) == 0 && len(rooms) == 0 {
		w.logEvent(LogBuildStart, "There is nothing built the colony has seen in that area to clear.")
		return false
	}
	if cost := Money(len(tiles)) * Money(w.cfg.WageDemolish); w.balance(Community) < cost {
		w.logEvent(LogBuildStart, fmt.Sprintf("The treasury cannot pay %v to clear %d tiles.", cost, len(tiles)))
		return false
	}
	w.playerZoned = true
	for _, p := range rooms {
		w.cancelProject(p)
		if s := p.structure; s != nil {
			w.maybeRetire(s)
		}
	}
	msg := ""
	if len(tiles) > 0 {
		if p := w.startClearing(tiles); p != nil {
			msg = fmt.Sprintf("The colony orders %d tiles cleared, %v escrowed.", len(tiles), w.projectCost(p))
		}
	}
	if len(rooms) > 0 {
		msg = strings.TrimSpace(msg + fmt.Sprintf(" %s under construction there %s called off.",
			capitalizeFirst(plural(len(rooms), "room")), isAre(len(rooms))))
	}
	w.logEvent(LogBuildStart, msg)
	return true
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

// startClearing posts a clearing project for tiles, funded by the treasury,
// and returns it, or nil if there was nothing to do or no money for it.
func (w *World) startClearing(tiles []Point) *project {
	if len(tiles) == 0 {
		return nil
	}
	w.nextProjectID++
	p := &project{id: w.nextProjectID, name: ClearingName, queuedTick: w.tick,
		issuer: Community, workKind: WorkClear}
	for _, pos := range tiles {
		// A dig task that clears whatever stands there (see taskDone): the
		// same task a room's moved wall or a passage is.
		p.tasks = append(p.tasks, &buildTask{pos: pos, terrain: Floor, clears: w.TerrainAt(pos), proj: p})
	}
	if !w.fundProject(p) {
		w.nextProjectID--
		return nil
	}
	w.projects = append(w.projects, p)
	return p
}

// tilesToClear is every built tile of the structures being torn down that no
// structure staying up also stands on (a party wall shared with a neighbour
// that stays stays too), row-major. A tile some project already has
// unfinished work on is left to it.
func (w *World) tilesToClear(going []*structure) []Point {
	if len(going) == 0 {
		return nil
	}
	gone := make(map[int]bool, len(going))
	for _, s := range going {
		gone[s.id] = true
	}
	taken := w.markedTiles()
	seen := make(map[Point]bool)
	var out []Point
	for _, s := range going {
		for _, p := range s.tiles {
			if seen[p] || taken[p] || !isBuilt(w.TerrainAt(p)) {
				continue
			}
			seen[p] = true
			kept := false
			for _, id := range w.structureAt[p] {
				if !gone[id] {
					kept = true
					break
				}
			}
			if !kept {
				out = append(out, p)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessPoint(out[i], out[j]) })
	return out
}

// ---- Siting in zones ---------------------------------------------------------------

// siteZone says which ground a room may be built on. inZone requires every
// tile of its footprint to be zoned kind; without it the room may also use
// unzoned ground (zoning-auto's second search). The zero siteZone is
// anywhere, as before zoning. Whether the room needs rock behind it is the
// site search's business, not zoning's: inside a zone drawn on open floor,
// the free-standing search (findFreeStandingSiteIn) is the one that finds
// it a site.
type siteZone struct {
	kind   ZoneKind
	inZone bool
}

// allows reports whether a room's tile p may be built on under z.
func (z siteZone) allows(w *World, p Point) bool {
	if z.kind == NoZone {
		return true
	}
	k := w.zoneAt(p)
	return k == z.kind || (!z.inZone && k == NoZone)
}

// borrowedWall reports whether frame tile (u, v) of a room is a wall it
// borrows from a neighbour rather than builds: a side or back wall tile
// already standing (see roomSiteClear). That tile is the neighbour's, and
// lies in the neighbour's zone.
func (w *World) borrowedWall(f roomFrame, u, v int) bool {
	if u != -1 && u != f.width && v != roomBackV {
		return false
	}
	return w.TerrainAt(f.at(u, v)) == Wall
}

// zoneRoomTiles calls visit for every tile of a room in frame f that has to
// lie in its zone: the inside and the walls, less a wall it borrows.
func (w *World) zoneRoomTiles(f roomFrame, visit func(Point)) {
	for v := roomBackV; v <= roomFrontV; v++ {
		for u := -1; u <= f.width; u++ {
			if !w.borrowedWall(f, u, v) {
				visit(f.at(u, v))
			}
		}
	}
}

// roomZoned reports whether every tile zoneRoomTiles walks for f is ground z
// allows. It is zoneRoomTiles without the callback: siting asks it for every
// candidate, and a closure there allocated on each one.
func (w *World) roomZoned(f roomFrame, z siteZone) bool {
	for v := roomBackV; v <= roomFrontV; v++ {
		for u := -1; u <= f.width; u++ {
			if !w.borrowedWall(f, u, v) && !z.allows(w, f.at(u, v)) {
				return false
			}
		}
	}
	return true
}

// noteZoneWait records that the colony wanted a fixture of kind and no zone
// had room for it, for the Zones tab.
func (w *World) noteZoneWait(kind Terrain) {
	if !w.cfg.ZoningAuto {
		w.zoneWaits[kind] = w.tick + 1 // 0 means never
	}
}

// zoneWaitTicks is how long a fixture the colony could not site stays on the
// Zones tab's waiting list after it last asked.
const zoneWaitTicks = 256

// zoneWaiting lists the fixture kinds the colony has wanted recently and
// found no zone with room for, in terrain order.
func (w *World) zoneWaiting() []Terrain {
	var out []Terrain
	for t := Terrain(0); t < numTerrains; t++ {
		if at := w.zoneWaits[t]; at > 0 && w.tick+1-at <= zoneWaitTicks {
			out = append(out, t)
		}
	}
	return out
}

// ---- Publishing ----------------------------------------------------------------------

// ZoneRun is one horizontal run of a zone kind, for frontends: tiles X0..X1
// of row Y. Locked runs are colony ships' residence.
type ZoneRun struct {
	Y, X0, X1 int
	Kind      ZoneKind
	Locked    bool
}

// publishedZones is every zoned tile as row runs, sorted by row then column,
// reusing the last copy while nothing has been zoned since.
func (w *World) publishedZones() []ZoneRun {
	if w.snapZones != nil && w.snapZoneRev == w.zoneRev {
		return w.snapZones
	}
	runs := []ZoneRun{}
	for pi, page := range w.zones.pages {
		if page == nil {
			continue
		}
		px := (pi & (1<<w.zones.colShift - 1)) << gridPageBits
		py := (pi >> w.zones.colShift) << gridPageBits
		for oy := 0; oy < gridPageSide; oy++ {
			y := py + oy
			if y >= w.Height {
				break
			}
			var cur *ZoneRun
			for ox := 0; ox < gridPageSide; ox++ {
				x := px + ox
				if x >= w.Width {
					break
				}
				c := page[oy<<gridPageBits|ox]
				if c.kind == NoZone {
					cur = nil
					continue
				}
				if cur != nil && cur.Kind == c.kind && cur.Locked == (c.locks > 0) && cur.X1 == x-1 {
					cur.X1 = x
					continue
				}
				runs = append(runs, ZoneRun{Y: y, X0: x, X1: x, Kind: c.kind, Locked: c.locks > 0})
				cur = &runs[len(runs)-1]
			}
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].Y != runs[j].Y {
			return runs[i].Y < runs[j].Y
		}
		return runs[i].X0 < runs[j].X0
	})
	// Join the runs a page edge split.
	merged := runs[:0]
	for _, r := range runs {
		if n := len(merged); n > 0 {
			last := &merged[n-1]
			if last.Y == r.Y && last.X1 == r.X0-1 && last.Kind == r.Kind && last.Locked == r.Locked {
				last.X1 = r.X1
				continue
			}
		}
		merged = append(merged, r)
	}
	w.snapZones, w.snapZoneRev = merged, w.zoneRev
	return merged
}

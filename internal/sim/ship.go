package sim

import "fmt"

// ---- Colony ships ------------------------------------------------------------
//
// Every colonist arrives aboard a colony ship: a hulled prefab of up to
// ship-capacity settlers, stamped into the world where it lands. A ship is
// three rooms behind one metal hull — a bunkroom of communal bunks, a latrine
// of communal toilets, and a hold with a private locker for every passenger
// (and a private trough for every chicken keeper) — joined by a two-wide
// aisle. It comes down as a stick, a hub and spoke, or a knobby cluster (see
// ship_layout.go). Worldgen, the spawn command, and the director's arrival
// occurrence all go through land, so what a ship carries is decided in one
// place; in the browser the founders wait aloft until the player lands each
// one (LandShip). See docs/ships.md.
//
// The ship replaced one crash pod per colonist. Pods each carried a private
// bunk and toilet, so a colony of them never needed a dormitory, and they
// landed as rows of cubicles whose hulls the colony had to walk around. A
// ship packs the same settlers into one block with room for fewer than all
// of them to sleep at once, so the colony has to build.

const (
	// shipCrashSlack is how many rings beyond the first crash-through site
	// the search keeps looking for a clear one. An open site a little farther
	// out beats smashing a hole in the rock right by the colony, but not by
	// much: past this, the ship just comes down through the rock.
	shipCrashSlack = 8
	// shipRevealReach is how far past its footprint a landing ship reveals
	// the map: stamping discovers the footprint, and revealAround reveals the
	// ring around every tile it touches.
	shipRevealReach = 2
	// shipRingBacktrack is how many rings inside the last landing's the next
	// search starts (see findShipSite).
	shipRingBacktrack = 6
)

// Ship is one colony ship that has landed: where it is, its layout, and who
// came down in it.
type Ship struct {
	ID int // 1-based; Entity.ship holds it
	// Origin is the footprint's top-left.
	Origin Point
	layout shipLayout
	// Colonists are the passengers in boarding order: layout fixtures with
	// slot i are private to Colonists[i]. Pets are the chickens and cats that
	// came down with them, in the order they stepped out.
	Colonists []EntityID
	Pets      []EntityID
	// structure is the ship in the structure registry (structures.go),
	// which holds its ground as residence; nil once it has been cleared.
	structure *structure
}

// shipShare is pct percent of n, rounded up, but at least one while pct is
// positive: a ship for one still has a bunk and a toilet.
func shipShare(n, pct int) int {
	if pct <= 0 || n <= 0 {
		return 0
	}
	return max(1, (n*pct+99)/100)
}

// shipLoads splits n settlers into as few ships as ship-capacity allows,
// as evenly as it can: 30 at a capacity of 20 come down as two ships of 15,
// not 20 and 10.
func shipLoads(n, capacity int) []int {
	if n <= 0 {
		return nil
	}
	capacity = max(1, capacity)
	ships := (n + capacity - 1) / capacity
	loads := make([]int, ships)
	for i := range loads {
		loads[i] = n / ships
		if i < n%ships {
			loads[i]++
		}
	}
	return loads
}

// arriveWave brings n settlers down in as few ships as capacity allows and
// returns how many landed, in how many ships: fewer than n only on a map with
// no room left.
func (w *World) arriveWave(n int, announce bool) (landed, ships int) {
	for _, load := range shipLoads(n, w.cfg.ShipCapacity) {
		s := w.land(load, announce)
		if s == nil {
			break
		}
		landed += len(s.Colonists)
		ships++
	}
	return landed, ships
}

// shipsNoun is "a ship" or "N ships", for the log.
func shipsNoun(n int) string {
	if n == 1 {
		return "a ship"
	}
	return fmt.Sprintf("%d ships", n)
}

// land brings a ship of n settlers into the world, wherever findShipSite
// puts it, and returns it, or nil if no site could be found anywhere (a map
// with no room left at all). Every way a colonist enters the game comes
// through here or landAt. announce logs the landing; worldgen passes false so
// the opening log is not one line per ship.
func (w *World) land(n int, announce bool) *Ship {
	if n <= 0 {
		return nil
	}
	l := w.nextShipLayout(n)
	o, crashed, ok := w.findShipSite(l)
	if !ok && l.shape != shipStick {
		// No room anywhere for this shape: a stick is the slimmest ship and
		// fits where the others do not.
		l = w.planStick(w.shipKeepers(w.nextID, n))
		o, crashed, ok = w.findShipSite(l)
	}
	if !ok {
		return nil
	}
	return w.landShip(n, l, o, crashed, announce)
}

// landAt brings a ship of n settlers down with its top-left at o, where a
// player chose: nextShipLayout's ship, so the one a frontend was shown. It
// returns nil if the site is not allowed (see shipSiteAllowed). The ship
// crushes anything under it, as a moved ship does.
func (w *World) landAt(n int, o Point, announce bool) *Ship {
	if n <= 0 {
		return nil
	}
	l := w.nextShipLayout(n)
	if !w.shipSiteAllowed(nil, &l, o) {
		return nil
	}
	crashed := false
	l.forEachTile(func(d Point, _ bool) {
		p := o.Add(d.X, d.Y)
		crashed = crashed || w.TerrainAt(p) == Rock
		if e := w.entityAt(p); e != nil {
			w.remove(e.ID, "crushed by a landing ship")
		}
	})
	return w.landShip(n, l, o, crashed, announce)
}

// landShip lands a ship of n laid out as l with its top-left at o: it stamps
// the ship, steps the passengers and their pets out, and stocks the hold.
func (w *World) landShip(n int, l shipLayout, o Point, crashed, announce bool) *Ship {
	// The passengers take the next n IDs, in order (their pets come after),
	// so who keeps a chicken — and needs a trough in the hold — is known
	// before the ship is laid out.
	first := w.nextID
	s := &Ship{ID: len(w.ships) + 1, Origin: o, layout: l}
	w.ships = append(w.ships, s)
	// A ship a player put down beside a hidden cavern breaks into it. Its
	// nest is rolled once everyone aboard is out, so its aliens neither take
	// the passengers' IDs nor their tiles.
	w.holdNests = true
	w.stampShip(s)
	for i := 0; i < n; i++ {
		e := w.spawn(Colonist, o.Add(l.floor[i].X, l.floor[i].Y))
		if e.ID != first+EntityID(i) {
			panic("sim: a ship's passengers must take consecutive IDs")
		}
		e.ship = s.ID
		s.Colonists = append(s.Colonists, e.ID)
		w.rollBackground(e)
		w.rollEmployer(e)
	}
	w.furnishShip(s)
	w.registerShip(s)
	// The one rare item each passenger brought: a gun, a chicken, or a cat.
	next := n
	for _, id := range s.Colonists {
		e := w.entities[id]
		switch w.arrivalRareItem(id) {
		case rareGun:
			e.Inventory.Add(w.arrivalGun(id), 1)
		case rareChicken:
			p := l.floor[next]
			next++
			hen := w.spawn(Chicken, o.Add(p.X, p.Y))
			hen.pet = &PetBond{keeper: id, trough: e.trough, hasTrough: true}
			s.Pets = append(s.Pets, hen.ID)
		case rareCat:
			p := l.floor[next]
			next++
			cat := w.spawn(Cat, o.Add(p.X, p.Y))
			cat.pet = &PetBond{keeper: id}
			s.Pets = append(s.Pets, cat.ID)
		}
	}
	w.holdNests = false
	if len(w.nestCenters) > 0 {
		w.rollNests(w.nestCenters)
		w.nestCenters = w.nestCenters[:0]
	}

	if announce {
		how := "lands"
		if crashed {
			how = "smashes down through the rock"
		}
		who := w.entities[s.Colonists[0]].displayName()
		if n > 1 {
			who = fmt.Sprintf("%d settlers", n)
		}
		w.logEvent(LogArrival, fmt.Sprintf("A %s colony ship %s at (%d, %d): %s aboard.", l.shape, how, o.X, o.Y, who))
	}
	return s
}

// LandShip lands the next ship still waiting aloft with its top-left at
// (X, Y), before the game's first tick: with place-ships set (the browser
// sets it), the founders' ships wait for the player to land them one after
// another. Ship is the ship's ID, which must be the next to land, so a
// command sent twice cannot land the one after it by mistake.
type LandShip struct {
	Ship int
	X, Y int
}

func (LandShip) isCommand() {}

// landAloft carries out a LandShip, reporting whether a ship landed.
func (w *World) landAloft(c LandShip) bool {
	if w.tick != 0 || len(w.aloft) == 0 || c.Ship != len(w.ships)+1 {
		return false
	}
	if w.landAt(w.aloft[0], Point{c.X, c.Y}, true) == nil {
		return false
	}
	w.aloft = w.aloft[1:]
	w.refreshSpatial()
	return true
}

// landRestAloft brings down any founders' ships the player left aloft when
// the game started, wherever findShipSite puts them, so nobody is left in
// orbit.
func (w *World) landRestAloft() {
	for len(w.aloft) > 0 {
		w.land(w.aloft[0], true)
		w.aloft = w.aloft[1:]
	}
	w.refreshSpatial()
}

// stampShip writes a ship's hull, floor, and fixtures at its origin. The
// impact obliterates whatever was there: rock, ore, anything built, all of it
// simply gone. It blasts a one-tile crater of any rock round the hull too —
// the walkway that takes passengers from either doorway to whatever floor the
// margin touches — and reserves the approach outside each doorway, so no room
// or later ship seals the passengers in (see designateRoom).
func (w *World) stampShip(s *Ship) {
	l, o := &s.layout, s.Origin
	l.forEachTile(func(d Point, hull bool) {
		p := o.Add(d.X, d.Y)
		w.revealAround(p)
		if hull {
			w.SetTerrain(p, Hull)
		} else {
			w.SetTerrain(p, Floor)
		}
	})
	for _, d := range l.margin {
		if p := o.Add(d.X, d.Y); w.TerrainAt(p) == Rock {
			w.SetTerrain(p, Floor)
		}
	}
	for _, d := range l.doors {
		w.doorTiles[o.Add(d.X, d.Y)] = true
	}
	for _, f := range l.fixtures {
		w.SetTerrain(o.Add(f.at.X, f.at.Y), f.terrain)
	}
}

// furnishShip makes each passenger's locker and trough private to it and
// stocks them: the locker with its meals and the trough with feed, each
// credited to its owner. The bunks and toilets stay communal, as SetTerrain
// left them.
func (w *World) furnishShip(s *Ship) {
	for _, f := range s.layout.fixtures {
		if f.slot < 0 || f.slot >= len(s.Colonists) {
			continue
		}
		id := s.Colonists[f.slot]
		e := w.entities[id]
		if e == nil {
			continue
		}
		me := ColonistOwner(id)
		p := s.Origin.Add(f.at.X, f.at.Y)
		w.setFixtureOwner(p, me, AccessPrivate)
		c := w.storageContainers[p]
		switch f.terrain {
		case Storage:
			if n := w.arrivalMeals(id); n > 0 && c.Inventory.Add(Meal, n) {
				c.credit(me, Meal, n)
			}
		case Trough:
			e.trough, e.hasTrough = p, true
			// The trough lands full, so the hen eats while its keeper
			// settles in.
			if n := w.cfg.TroughFill; n > 0 && c.Inventory.Add(Feed, n) {
				c.credit(me, Feed, n)
			}
		}
	}
	for _, id := range s.Pets {
		if pet := w.entities[id]; pet != nil && pet.Kind == Chicken && pet.pet != nil {
			if keeper := w.entities[pet.pet.keeper]; keeper != nil && keeper.hasTrough {
				pet.pet.trough, pet.pet.hasTrough = keeper.trough, true
			}
		}
	}
}

// lockerOf returns where the locker of a colonist who came down in a ship
// is, if it has one.
func (w *World) lockerOf(e *Entity) (Point, bool) {
	s := w.shipByID(e.ship)
	if s == nil {
		return Point{}, false
	}
	for _, f := range s.layout.fixtures {
		if f.terrain == Storage && f.slot >= 0 && f.slot < len(s.Colonists) && s.Colonists[f.slot] == e.ID {
			return s.Origin.Add(f.at.X, f.at.Y), true
		}
	}
	return Point{}, false
}

// ShipView is a ship as a frontend sees it: its footprint, its shape, and
// how many came down in it. A ship still Aloft (see LandShip) has no
// position; only the next one to land has a shape yet, because where the
// ones before it land can change which IDs, and so which troughs, it gets.
type ShipView struct {
	ID            int
	X, Y          int // the footprint's top-left
	Width, Height int
	// Shape is the footprint row by row: '#' hull, '.' deck, ' ' not part
	// of the ship. Shared with the layout, so read-only.
	Shape     []string
	ShapeName string
	Colonists int
	Aloft     bool
}

// shipViews lists the ships for a snapshot, landed ones then any still
// aloft: a fresh copy, which is cheap — a colony has one ship per
// ship-capacity settlers.
func (w *World) shipViews() []ShipView {
	out := make([]ShipView, 0, len(w.ships)+len(w.aloft))
	for _, s := range w.ships {
		l := &s.layout
		out = append(out, ShipView{ID: s.ID, X: s.Origin.X, Y: s.Origin.Y, Width: l.width, Height: l.height,
			Shape: l.rows, ShapeName: l.shape.String(), Colonists: len(s.Colonists)})
	}
	for i, n := range w.aloft {
		v := ShipView{ID: len(w.ships) + 1 + i, Colonists: n, Aloft: true}
		if i == 0 {
			l := w.nextShipLayout(n)
			v.Width, v.Height, v.Shape, v.ShapeName = l.width, l.height, l.rows, l.shape.String()
		}
		out = append(out, v)
	}
	return out
}

// shipByID returns the ship with this ID, or nil.
func (w *World) shipByID(id int) *Ship {
	if id < 1 || id > len(w.ships) {
		return nil
	}
	return w.ships[id-1]
}

// MoveShip relands a ship with its top-left at (X, Y), before the game's
// first tick: the browser lets the player choose where the founders come
// down. The ship obliterates whatever it lands on — rock, ore, a rat — but
// never another ship or the walkway round one.
type MoveShip struct {
	Ship int
	X, Y int
}

func (MoveShip) isCommand() {}

// moveShip carries out a MoveShip, reporting whether the ship moved. The old
// site is left as bare floor; the passengers and pets step out at the new
// site exactly as they did at the old, and the lockers and troughs are
// restocked from the same pure functions, so a moved ship is the ship that
// would have landed there.
func (w *World) moveShip(c MoveShip) bool {
	s := w.shipByID(c.Ship)
	if w.tick != 0 || s == nil {
		return false
	}
	o := Point{c.X, c.Y}
	if !w.shipSiteAllowed(s, &s.layout, o) {
		return false
	}
	riders := make([]*Entity, 0, len(s.Colonists)+len(s.Pets))
	for _, id := range append(append([]EntityID(nil), s.Colonists...), s.Pets...) {
		if e := w.entities[id]; e != nil {
			riders = append(riders, e)
		}
	}
	aboard := make(map[EntityID]bool, len(riders))
	for _, e := range riders {
		aboard[e.ID] = true
		w.occ.set(e.Pos.X, e.Pos.Y, 0)
		w.removeFromChunkIndex(w.chunkIndexOf(e.Pos), e.ID)
	}
	old := s.Origin
	w.unregisterShip(s)
	s.layout.forEachTile(func(d Point, _ bool) {
		w.SetTerrain(old.Add(d.X, d.Y), Floor)
	})
	for _, d := range s.layout.doors {
		delete(w.doorTiles, old.Add(d.X, d.Y))
	}
	s.layout.forEachTile(func(d Point, _ bool) {
		if e := w.entityAt(o.Add(d.X, d.Y)); e != nil && !aboard[e.ID] {
			w.remove(e.ID, "crushed by a landing ship")
		}
	})
	s.Origin = o
	w.stampShip(s)
	for i, e := range riders {
		p := o.Add(s.layout.floor[i].X, s.layout.floor[i].Y)
		e.Pos = p
		w.occ.set(p.X, p.Y, e.ID)
		ci := w.chunkIndexOf(p)
		w.chunkEntities[ci] = append(w.chunkEntities[ci], e.ID)
	}
	w.furnishShip(s)
	w.registerShip(s)
	w.refreshSpatial()
	return true
}

// shipSiteAllowed reports whether a ship laid out as l may come down with
// its top-left at o: on the map with room for its crater, clear of every
// other ship and the walkway round it (which holds that ship's doorways), and
// not on top of anyone else's colonist. s is the ship being moved, which may
// overlap where it is now; nil for one still aloft.
func (w *World) shipSiteAllowed(s *Ship, l *shipLayout, o Point) bool {
	if o.X < 1 || o.Y < 1 || o.X+l.width >= w.Width || o.Y+l.height >= w.Height {
		return false
	}
	ok := true
	l.forEachTile(func(d Point, _ bool) {
		if !ok {
			return
		}
		p := o.Add(d.X, d.Y)
		for _, other := range w.ships {
			if other != s && other.near(p) {
				ok = false
				return
			}
		}
		if e := w.entityAt(p); e != nil && e.Kind == Colonist && (s == nil || e.ship != s.ID) {
			ok = false
		}
	})
	return ok
}

// near reports whether p is part of ship s or the one-tile walkway round it.
func (s *Ship) near(p Point) bool {
	dx, dy := p.X-s.Origin.X, p.Y-s.Origin.Y
	if dx < -1 || dy < -1 || dx > s.layout.width || dy > s.layout.height {
		return false
	}
	for y := dy - 1; y <= dy+1; y++ {
		for x := dx - 1; x <= dx+1; x++ {
			if s.layout.inShip(x, y) {
				return true
			}
		}
	}
	return false
}

// findShipSite picks where a ship with layout l comes down: the top-left of a
// footprint nearest the map center, preferring open floor to rock. A site
// whose footprint and margin are all floor lands cleanly; otherwise the ship
// crashes through, and any rock under the footprint is cleared. crashed
// reports which.
//
// The search walks rings of origins outward from the center and takes the
// first clear site; the first crash site it passes is held as a fallback and
// taken once the search is shipCrashSlack rings beyond it without finding
// open floor. Ring order and the fixed scan within each ring make the choice
// the same for a given world, and nothing here draws randomness.
//
// Ships fill the map from the middle out, so w.shipRingHint remembers roughly
// where the last one landed and the next search starts a few rings inside it
// rather than re-checking the full, already-packed middle every time. It can
// miss a clear site that opened up nearer the middle since, which only costs
// a slightly longer walk.
func (w *World) findShipSite(l shipLayout) (o Point, crashed, ok bool) {
	// Ships land in the lower half of the map only. The upper half of the
	// landing cavern is where the colony's first rooms go: rooms prefer rock
	// behind their back wall (see roomSiteClear), and the top rim is the open
	// stretch of it nothing lands on. When crash pods spread up into it, small
	// colonies had nowhere to site even one room.
	//
	// The top hull row starts one below the middle row, so the middle row
	// itself stays walkable margin: in the smallest cavern (see minCaveRy) it
	// is exactly the approach row of a room against the top rim.
	center := Point{w.Width/2 - l.width/2, w.Height/2 + 1}
	maxR := max(w.Width, w.Height)
	designated := make(map[Point]bool)
	for _, pr := range w.projects {
		for _, t := range pr.tasks {
			designated[t.pos] = true
		}
	}
	crashAt, crashRing, crashRock := Point{}, -1, 0
	for r := max(0, w.shipRingHint-shipRingBacktrack); r <= maxR; r++ {
		if crashRing >= 0 && r > crashRing+shipCrashSlack {
			break
		}
		found := false
		forEachRingPoint(center, r, func(p Point) bool {
			if p.Y < center.Y {
				return false // the upper half is for rooms
			}
			rock, marginRock, valid := w.shipSiteRock(p, &l, designated)
			if !valid {
				return false
			}
			if rock == 0 && marginRock == 0 {
				o, found = p, true
				return true
			}
			if crashRing < 0 {
				crashAt, crashRing, crashRock = p, r, rock
			}
			return false
		})
		if found {
			w.shipRingHint = r
			return o, false, true
		}
	}
	if crashRing >= 0 {
		w.shipRingHint = crashRing
		return crashAt, crashRock > 0, true
	}
	return Point{}, false, false
}

// forEachRingPoint visits the points on ring r around c, stopping early when
// visit returns true. A ring is stretched two-to-one across, like the landing
// cavern (see caveRadii): it holds the points whose max(ceil(|dx|/2), |dy|) is
// r. Within a ring, rows nearest c's come first, the row below before the row
// above.
//
// Both choices keep ships in the cavern's middle rows. Rooms are sited
// against the rock at a cavern's rim with several clear rows in front (see
// roomSiteClear); plain square rings from the center grew a block of crash
// pods that reached the top and bottom rims of a wide, short cavern and left
// the colony nowhere to build its first room.
func forEachRingPoint(c Point, r int, visit func(Point) bool) {
	if r == 0 {
		visit(c)
		return
	}
	for i := 0; i <= 2*r; i++ {
		dy := (i + 1) / 2 // 0, 1, 1, 2, 2, ...
		if i%2 == 0 {
			dy = -dy // below before above: 0, +1, -1, +2, -2, ...
		}
		y := c.Y + dy
		if dy == r || dy == -r {
			for x := c.X - 2*r; x <= c.X+2*r; x++ {
				if visit(Point{x, y}) {
					return
				}
			}
			continue
		}
		for _, x := range [4]int{c.X - 2*r, c.X - 2*r + 1, c.X + 2*r - 1, c.X + 2*r} {
			if visit(Point{x, y}) {
				return
			}
		}
	}
}

// shipSiteRock reports whether a ship width tiles wide may land with its
// top-left at o, how many of its footprint tiles are still rock, and how many
// of its margin tiles are.
//
// A clean landing needs both to be zero: open floor all round, out in the
// middle of a cavern. That is deliberate — a ship that settled against the
// cavern wall would take exactly the rock-backed edge the colony's rooms are
// sited on (see findRoomSite).
//
// The footprint must be rock or bare floor with nobody standing on it and no
// construction designated there, and must not cover a room's reserved door
// approach. The one-tile margin around it must hold no wall, hull, or
// fixture, so ships never land inside a built room or wedged against one or
// another ship. The margin is the walkway round the ship, and the landing
// clears any rock in it (see stampShip). That walkway is the only way out of
// the doorways, so it must lead somewhere: some of the margin must already be
// floor, or the ship would seal its passengers into the rock. Rock in the
// margin that a project means to dig is left alone: the ship may not land
// there.
//
// Only floor the colony has discovered counts. The floor of a natural cavern
// nobody has broken into (see caverns.md) is out of bounds for the
// footprint: a ship landing "cleanly" there would open the cavern around
// passengers with no way back to the colony. Nor does it count as a way out.
func (w *World) shipSiteRock(o Point, l *shipLayout, designated map[Point]bool) (rock, marginRock int, ok bool) {
	if o.X < 1 || o.Y < 1 || o.X+l.width >= w.Width || o.Y+l.height >= w.Height {
		return 0, 0, false // the margin must be on the map too
	}
	ok = true
	l.forEachTile(func(d Point, _ bool) {
		if !ok {
			return
		}
		p := o.Add(d.X, d.Y)
		switch w.TerrainAt(p) {
		case Rock:
			rock++
		case Floor:
			if !w.discovered(p) || w.occupied(p) {
				ok = false
			}
		default:
			ok = false
		}
		if w.doorTiles[p] || designated[p] || !shipZoneOK(w, p) {
			ok = false
		}
	})
	if !ok {
		return 0, 0, false
	}
	touchesFloor, blocked := false, false
	for _, d := range l.margin {
		p := o.Add(d.X, d.Y)
		if !shipZoneOK(w, p) {
			blocked = true
		}
		switch t := w.TerrainAt(p); {
		case t == Wall || t == Hull || isFixtureTerrain(t):
			blocked = true
		case t == Rock:
			marginRock++
			blocked = blocked || designated[p]
		case t == Floor && w.discovered(p):
			touchesFloor = true
		}
	}
	if blocked || !touchesFloor || w.hiddenFloorNear(o, l) {
		return 0, 0, false
	}
	return rock, marginRock, true
}

// shipZoneOK reports whether a ship may hold p as residence: it is unzoned,
// or residence already. A ship never lands on ground zoned for something
// else, which its landing would take over (see registerShip). A ship the
// player puts down by hand obliterates whatever is there, zones included.
func shipZoneOK(w *World, p Point) bool {
	z := w.zoneAt(p)
	return z == NoZone || z == ZoneResidence
}

// hiddenFloorNear reports whether any undiscovered floor lies within
// shipRevealReach of a footprint laid out as l at o. (It checks the
// footprint's whole box, which for a hub or cluster takes in a little more
// than the reveal reaches: erring on the side of not landing.) Landing there
// would break into a natural cavern nobody dug to — flooding it into view
// and rolling its nests (see caverns.md) — so an arrival wave could wake
// aliens with nobody digging: on a 120x70 map with 6 colonists, repeated
// crash-pod arrivals breached a cavern in 29 of 30 seeds.
func (w *World) hiddenFloorNear(o Point, l *shipLayout) bool {
	for y := o.Y - shipRevealReach; y < o.Y+l.height+shipRevealReach; y++ {
		for x := o.X - shipRevealReach; x < o.X+l.width+shipRevealReach; x++ {
			p := Point{x, y}
			if w.InBounds(p) && w.TerrainAt(p) == Floor && !w.discovered(p) {
				return true
			}
		}
	}
	return false
}

// shipReach is how far past the landing cavern a ship's footprint and crater
// can reach when it crashes through the rock at the cavern's rim: generate
// looks that far out for open floor.
func (w *World) shipReach() int {
	keepers := make([]bool, max(1, w.cfg.ShipCapacity))
	for i := range keepers {
		keepers[i] = true // the biggest ship: every passenger keeps a chicken
	}
	reach := 0
	for _, shape := range enabledShipShapes(w.cfg) {
		l := w.planShip(keepers, shape)
		reach = max(reach, l.width+l.height)
	}
	return reach + shipCrashSlack
}

// enabledShipShapes is every shape a ship may come down as under cfg: those
// with a positive weight, and always the stick, which any ship falls back
// to when its own shape will not fit.
func enabledShipShapes(cfg Config) []shipShape {
	out := []shipShape{shipStick}
	if cfg.ShipHubWeight > 0 {
		out = append(out, shipHub)
	}
	if cfg.ShipClusterWeight > 0 {
		out = append(out, shipCluster)
	}
	return out
}

// shipTilesPerColonist is the ground a full ship takes per passenger,
// crater included, rounded up — of the roomiest shape that may land: what
// the landing cavern sets aside for each settler's share of a ship (see
// caveRadii). A pure function of the config, so worldgen's chunk generator
// can size the cavern's clearance from it too.
func shipTilesPerColonist(cfg Config) int {
	n := max(1, cfg.ShipCapacity)
	if cfg.StartColonists > 0 {
		n = min(n, cfg.StartColonists)
	}
	w := &World{cfg: cfg}
	most := 0
	for _, shape := range enabledShipShapes(cfg) {
		l := w.planShip(make([]bool, n), shape)
		tiles := len(l.margin)
		l.forEachTile(func(Point, bool) { tiles++ })
		most = max(most, (tiles+n-1)/n)
	}
	return most
}

// arrivalMealSalt separates arrivalMeals' hash from anything else derived
// from the seed.
const arrivalMealSalt = 0x6D2B79F5A0761D65

// arrivalMeals is how many meals the locker of the colonist with this ID
// lands with: crash-pod-meals, give or take up to crash-pod-meal-spread,
// evenly.
//
// With every locker the same, every locker runs dry within a few hundred
// ticks of every other. A spread staggers that. It is off by default because,
// in the runs measured, it starved more colonists, not fewer (see
// docs/ships.md).
//
// It is a pure function of the seed and the ID, not a draw from a stream, so
// it shifts no other random draw and doesn't depend on the order colonists
// arrive in.
func (w *World) arrivalMeals(id EntityID) int {
	n, spread := w.cfg.CrashPodMeals, w.cfg.CrashPodMealSpread
	if n <= 0 || spread <= 0 {
		return max(0, n)
	}
	s := uint64(w.cfg.Seed) ^ uint64(id)*0x9E3779B97F4A7C15 ^ arrivalMealSalt
	d := int(splitmix64(&s)%uint64(2*spread+1)) - spread
	return max(0, n+d)
}

// rareItem is the one rare thing a colonist lands with.
type rareItem uint8

const (
	rareNone rareItem = iota // every weight is zero
	rareGun
	rareChicken
	rareCat
)

// arrivalRareSalt separates arrivalRareItem's hash from arrivalMeals' and
// anything else derived from the seed; arrivalGunSalt does the same for
// arrivalGun.
const (
	arrivalRareSalt = 0xA24BAED4963EE407
	arrivalGunSalt  = 0x4F1BBCDCBFA53E0B
)

// arrivalRareItem is the one rare item the colonist with this ID brings: a
// gun, a chicken, or a cat, by the crash-pod-*-weight odds. Everyone gets
// exactly one (unless every weight is zero), so the colony lands as a mix of
// the armed, the chicken keepers, and the cat owners rather than as
// identically equipped settlers.
//
// Like arrivalMeals it is a pure function of the seed and the ID, not a draw
// from a stream: it shifts no other random draw and doesn't depend on
// arrival order. That is also what lets land lay out a ship's troughs before
// its passengers exist.
func (w *World) arrivalRareItem(id EntityID) rareItem {
	gun, hen, cat := max(0, w.cfg.CrashPodGunWeight), max(0, w.cfg.CrashPodChickenWeight), max(0, w.cfg.CrashPodCatWeight)
	total := gun + hen + cat
	if total == 0 {
		return rareNone
	}
	s := uint64(w.cfg.Seed) ^ uint64(id)*0x9E3779B97F4A7C15 ^ arrivalRareSalt
	r := int(splitmix64(&s) % uint64(total))
	switch {
	case r < gun:
		return rareGun
	case r < gun+hen:
		return rareChicken
	default:
		return rareCat
	}
}

// arrivalGun is which gun a colonist whose rare item is a gun lands with: a
// shotgun crash-pod-shotgun-percent of the time, else a pistol. A pure
// function of the seed and the ID, on its own salt so it is independent of
// arrivalRareItem's roll.
func (w *World) arrivalGun(id EntityID) ItemKind {
	pct := w.cfg.CrashPodShotgunPercent
	if pct <= 0 {
		return Pistol
	}
	if pct >= 100 {
		return Shotgun
	}
	s := uint64(w.cfg.Seed) ^ uint64(id)*0x9E3779B97F4A7C15 ^ arrivalGunSalt
	if splitmix64(&s)%100 < uint64(pct) {
		return Shotgun
	}
	return Pistol
}

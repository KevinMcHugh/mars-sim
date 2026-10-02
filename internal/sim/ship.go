package sim

import "fmt"

// ---- Colony ships ------------------------------------------------------------
//
// Every colonist arrives aboard a colony ship: a hulled prefab of up to
// ship-capacity settlers, stamped into the world where it lands. A ship is
// three rooms in a row behind one metal hull — a bunkroom of communal bunks,
// a latrine of communal toilets, and a hold with a private locker for every
// passenger (and a private trough for every chicken keeper) — joined by a
// two-wide aisle that runs the ship's length and out of a doorway at each
// end. Worldgen, the spawn command, and the director's arrival occurrence
// all go through land, so what a ship carries is decided in one place. See
// docs/ships.md.
//
// A ship for six, with one chicken keeper:
//
//	H H H H H H H H H H H     H hull (metal wall)
//	H B B H T H L L L ~ H     B bunk, T toilet, L locker, ~ trough
//	. . . . . . . . . . .     the aisle: doorways at both ends and through
//	. . . . . . . . . . .     every partition; passengers step out here
//	H B . H T H L L L . H
//	H H H H H H H H H H H
//
// The ship replaced one crash pod per colonist. Pods each carried a private
// bunk and toilet, so a colony of them never needed a dormitory, and they
// landed as rows of cubicles whose hulls the colony had to walk around. A
// ship packs the same settlers into one block with room for fewer than all
// of them to sleep at once, so the colony has to build.

const (
	// shipHeight is every ship's footprint height: hull, fixture row, two
	// aisle rows, fixture row, hull.
	shipHeight = 6
	// shipAisle is the top aisle row; shipAisle+1 is the other.
	shipAisle = 2
	// shipCrashSlack is how many rings beyond the first crash-through site
	// the search keeps looking for a clear one. An open site a little farther
	// out beats smashing a hole in the rock right by the colony, but not by
	// much: past this, the ship just comes down through the rock.
	shipCrashSlack = 8
	// shipRevealReach is how far past its footprint a landing ship reveals
	// the map: stamping discovers the footprint, and revealAround reveals the
	// ring around every tile it touches.
	shipRevealReach = 2
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
}

// shipFixture is one fixture in a ship's layout, as an offset from its
// top-left. slot is the passenger it is private to, or -1 for a communal one.
type shipFixture struct {
	at      Point
	terrain Terrain
	slot    int
}

// shipLayout is a ship's floor plan, decided by how many it carries.
type shipLayout struct {
	width    int
	fixtures []shipFixture
	// floor is the interior aisle tiles in the order passengers, then pets,
	// step out onto them.
	floor []Point
	// hull marks the footprint's hull tiles, row-major.
	hull []bool
	// doors are the approach tiles just outside the two end doorways,
	// reserved like a room's.
	doors []Point
}

func (l *shipLayout) hullAt(dx, dy int) bool { return l.hull[dy*l.width+dx] }

// planShip lays out a ship for len(keepers) passengers, keepers[i] saying
// whether passenger i keeps a chicken (and so has a trough in the hold).
//
// Rooms are filled column by column, top row then bottom, each a column
// wider per two fixtures, so the ship grows along its length and every
// fixture faces the aisle. A room that would hold nothing is left out.
func (w *World) planShip(keepers []bool) shipLayout {
	n := len(keepers)
	type room struct {
		count int
		at    func(i int) (Terrain, int)
	}
	var troughs []int
	for i, k := range keepers {
		if k {
			troughs = append(troughs, i)
		}
	}
	communal := func(t Terrain) func(int) (Terrain, int) {
		return func(int) (Terrain, int) { return t, -1 }
	}
	rooms := []room{
		{shipShare(n, w.cfg.ShipBunkPercent), communal(Bed)},
		{shipShare(n, w.cfg.ShipToiletPercent), communal(Toilet)},
		{n + len(troughs), func(i int) (Terrain, int) {
			if i < n {
				return Storage, i
			}
			return Trough, troughs[i-n]
		}},
	}
	l := shipLayout{}
	var top, bottom []Point
	var partitions []int
	x := 1
	for _, r := range rooms {
		if r.count == 0 {
			continue
		}
		if x > 1 {
			partitions = append(partitions, x)
			x++
		}
		cols := (r.count + 1) / 2
		for i := 0; i < r.count; i++ {
			row := 1
			if i%2 == 1 {
				row = shipHeight - 2
			}
			t, slot := r.at(i)
			l.fixtures = append(l.fixtures, shipFixture{Point{x + i/2, row}, t, slot})
		}
		for c := 0; c < cols; c++ {
			top = append(top, Point{x + c, shipAisle})
			bottom = append(bottom, Point{x + c, shipAisle + 1})
		}
		x += cols
	}
	// Everyone aboard steps out onto a tile of their own: every passenger
	// and a pet each at most. Bunks and toilets alone leave room for that,
	// except in a few ships with many pets and few troughs; those get a
	// little spare floor at the end of the hold.
	for len(top)+len(partitions) < n {
		top = append(top, Point{x, shipAisle})
		bottom = append(bottom, Point{x, shipAisle + 1})
		x++
	}
	l.width = x + 1
	l.floor = append(top, bottom...)
	for _, px := range partitions {
		l.floor = append(l.floor, Point{px, shipAisle}, Point{px, shipAisle + 1})
	}
	l.hull = make([]bool, l.width*shipHeight)
	aisle := func(dy int) bool { return dy == shipAisle || dy == shipAisle+1 }
	for dy := 0; dy < shipHeight; dy++ {
		for dx := 0; dx < l.width; dx++ {
			edge := dy == 0 || dy == shipHeight-1 || ((dx == 0 || dx == l.width-1) && !aisle(dy))
			l.hull[dy*l.width+dx] = edge
		}
	}
	for _, px := range partitions {
		for dy := 0; dy < shipHeight; dy++ {
			if !aisle(dy) {
				l.hull[dy*l.width+px] = true
			}
		}
	}
	l.doors = []Point{
		{-1, shipAisle}, {-1, shipAisle + 1},
		{l.width, shipAisle}, {l.width, shipAisle + 1},
	}
	return l
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

// land brings a ship of n settlers into the world and returns it, or nil if
// no site could be found anywhere (a map with no room left at all). Every
// way a colonist enters the game comes through here. announce logs the
// landing; worldgen passes false so the opening log is not one line per
// ship.
func (w *World) land(n int, announce bool) *Ship {
	if n <= 0 {
		return nil
	}
	// The passengers take the next n IDs, in order (their pets come after),
	// so who keeps a chicken — and needs a trough in the hold — is known
	// before the ship is laid out.
	first := w.nextID
	keepers := make([]bool, n)
	for i := range keepers {
		keepers[i] = w.arrivalRareItem(first+EntityID(i)) == rareChicken
	}
	l := w.planShip(keepers)
	o, crashed, ok := w.findShipSite(l)
	if !ok {
		return nil
	}
	s := &Ship{ID: len(w.ships) + 1, Origin: o, layout: l}
	w.ships = append(w.ships, s)
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
			hen.keeper, hen.trough, hen.hasTrough = id, e.trough, true
			s.Pets = append(s.Pets, hen.ID)
		case rareCat:
			p := l.floor[next]
			next++
			cat := w.spawn(Cat, o.Add(p.X, p.Y))
			cat.keeper = id
			s.Pets = append(s.Pets, cat.ID)
		}
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
		w.logEvent(LogArrival, fmt.Sprintf("A colony ship %s at (%d, %d): %s aboard.", how, o.X, o.Y, who))
	}
	return s
}

// stampShip writes a ship's hull, floor, and fixtures at its origin. The
// impact obliterates whatever was there: rock, ore, anything built, all of it
// simply gone. It blasts a one-tile crater of any rock round the hull too —
// the walkway that takes passengers from either doorway to whatever floor the
// margin touches — and reserves the approach outside each doorway, so no room
// or later ship seals the passengers in (see designateRoom).
func (w *World) stampShip(s *Ship) {
	l, o := &s.layout, s.Origin
	for dy := 0; dy < shipHeight; dy++ {
		for dx := 0; dx < l.width; dx++ {
			p := o.Add(dx, dy)
			w.revealAround(p)
			if l.hullAt(dx, dy) {
				w.SetTerrain(p, Hull)
			} else {
				w.SetTerrain(p, Floor)
			}
		}
	}
	forEachShipMargin(o, l.width, func(p Point) {
		if w.TerrainAt(p) == Rock {
			w.SetTerrain(p, Floor)
		}
	})
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
		if pet := w.entities[id]; pet != nil && pet.Kind == Chicken {
			if keeper := w.entities[pet.keeper]; keeper != nil && keeper.hasTrough {
				pet.trough, pet.hasTrough = keeper.trough, true
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

// ShipView is a landed ship as a frontend sees it: its footprint and how many
// came down in it.
type ShipView struct {
	ID            int
	X, Y          int // the footprint's top-left
	Width, Height int
	Colonists     int
}

// shipViews lists the ships for a snapshot: a fresh copy, which is cheap —
// a colony has one ship per ship-capacity settlers.
func (w *World) shipViews() []ShipView {
	out := make([]ShipView, len(w.ships))
	for i, s := range w.ships {
		out[i] = ShipView{ID: s.ID, X: s.Origin.X, Y: s.Origin.Y, Width: s.layout.width, Height: shipHeight, Colonists: len(s.Colonists)}
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
	o, width := Point{c.X, c.Y}, s.layout.width
	if !w.shipSiteAllowed(s, o) {
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
	for dy := 0; dy < shipHeight; dy++ {
		for dx := 0; dx < width; dx++ {
			w.SetTerrain(old.Add(dx, dy), Floor)
		}
	}
	for _, d := range s.layout.doors {
		delete(w.doorTiles, old.Add(d.X, d.Y))
	}
	for dy := 0; dy < shipHeight; dy++ {
		for dx := 0; dx < width; dx++ {
			if e := w.entityAt(o.Add(dx, dy)); e != nil && !aboard[e.ID] {
				w.remove(e.ID, "crushed by a landing ship")
			}
		}
	}
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
	w.refreshSpatial()
	return true
}

// shipSiteAllowed reports whether ship s may be relanded with its top-left
// at o: on the map with room for its crater, clear of every other ship and
// the walkway round it (which holds that ship's doorways), and not on top of
// anyone else's colonist.
func (w *World) shipSiteAllowed(s *Ship, o Point) bool {
	width := s.layout.width
	if o.X < 1 || o.Y < 1 || o.X+width >= w.Width || o.Y+shipHeight >= w.Height {
		return false
	}
	for _, other := range w.ships {
		if other == s {
			continue
		}
		oo := other.Origin.Add(-1, -1)
		if o.X < oo.X+other.layout.width+2 && oo.X < o.X+width &&
			o.Y < oo.Y+shipHeight+2 && oo.Y < o.Y+shipHeight {
			return false
		}
	}
	for dy := 0; dy < shipHeight; dy++ {
		for dx := 0; dx < width; dx++ {
			if e := w.entityAt(o.Add(dx, dy)); e != nil && e.Kind == Colonist && e.ship != s.ID {
				return false
			}
		}
	}
	return true
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
	for r := max(0, w.shipRingHint-shipHeight); r <= maxR; r++ {
		if crashRing >= 0 && r > crashRing+shipCrashSlack {
			break
		}
		found := false
		forEachRingPoint(center, r, func(p Point) bool {
			if p.Y < center.Y {
				return false // the upper half is for rooms
			}
			rock, marginRock, valid := w.shipSiteRock(p, l.width, designated)
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
func (w *World) shipSiteRock(o Point, width int, designated map[Point]bool) (rock, marginRock int, ok bool) {
	if o.X < 1 || o.Y < 1 || o.X+width >= w.Width || o.Y+shipHeight >= w.Height {
		return 0, 0, false // the margin must be on the map too
	}
	for dy := 0; dy < shipHeight; dy++ {
		for dx := 0; dx < width; dx++ {
			p := o.Add(dx, dy)
			switch w.TerrainAt(p) {
			case Rock:
				rock++
			case Floor:
				if !w.discovered(p) || w.occupied(p) {
					return 0, 0, false
				}
			default:
				return 0, 0, false
			}
			if w.doorTiles[p] || designated[p] {
				return 0, 0, false
			}
		}
	}
	touchesFloor, blocked := false, false
	forEachShipMargin(o, width, func(p Point) {
		switch t := w.TerrainAt(p); {
		case t == Wall || t == Hull || isFixtureTerrain(t):
			blocked = true
		case t == Rock:
			marginRock++
			blocked = blocked || designated[p]
		case t == Floor && w.discovered(p):
			touchesFloor = true
		}
	})
	if blocked || !touchesFloor || w.hiddenFloorNear(o, width) {
		return 0, 0, false
	}
	return rock, marginRock, true
}

// hiddenFloorNear reports whether any undiscovered floor lies within
// shipRevealReach of a footprint width tiles wide at o. Landing there would
// break into a natural cavern nobody dug to — flooding it into view and
// rolling its nests (see caverns.md) — so an arrival wave could wake aliens
// with nobody digging: on a 120x70 map with 6 colonists, repeated crash-pod
// arrivals breached a cavern in 29 of 30 seeds.
func (w *World) hiddenFloorNear(o Point, width int) bool {
	for y := o.Y - shipRevealReach; y < o.Y+shipHeight+shipRevealReach; y++ {
		for x := o.X - shipRevealReach; x < o.X+width+shipRevealReach; x++ {
			p := Point{x, y}
			if w.InBounds(p) && w.TerrainAt(p) == Floor && !w.discovered(p) {
				return true
			}
		}
	}
	return false
}

// forEachShipMargin visits the one-tile ring around a ship width tiles wide
// whose top-left is o.
func forEachShipMargin(o Point, width int, visit func(Point)) {
	for dy := -1; dy <= shipHeight; dy++ {
		for dx := -1; dx <= width; dx++ {
			if dx >= 0 && dx < width && dy >= 0 && dy < shipHeight {
				continue
			}
			visit(o.Add(dx, dy))
		}
	}
}

// shipReach is how far past the landing cavern a ship's footprint and crater
// can reach when it crashes through the rock at the cavern's rim: generate
// looks that far out for open floor.
func (w *World) shipReach() int {
	keepers := make([]bool, max(1, w.cfg.ShipCapacity))
	for i := range keepers {
		keepers[i] = true // the widest ship: every passenger keeps a chicken
	}
	return w.planShip(keepers).width + shipHeight + shipCrashSlack
}

// shipTilesPerColonist is the ground a full ship takes per passenger,
// crater included, rounded up: what the landing cavern sets aside for each
// settler's share of a ship (see caveRadii). A pure function of the config,
// so worldgen's chunk generator can size the cavern's clearance from it too.
func shipTilesPerColonist(cfg Config) int {
	n := max(1, cfg.ShipCapacity)
	if cfg.StartColonists > 0 {
		n = min(n, cfg.StartColonists)
	}
	w := &World{cfg: cfg}
	l := w.planShip(make([]bool, n))
	return ((l.width+2)*(shipHeight+2) + n - 1) / n
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

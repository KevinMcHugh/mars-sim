package sim

import "strings"

// ---- Ship layouts ------------------------------------------------------------
//
// A ship's floor plan is a shape of hull and deck tiles, not a rectangle: the
// footprint is only its bounding box, and the tiles outside the shape are
// left as they were when it lands. Three shapes come down (see
// docs/ships.md):
//
// Ships for six, one of them a chicken keeper, as planShip draws them (#
// hull, B bunk, T toilet, L locker, ~ trough, . deck). A stick: the rooms in
// a row behind one hull, the aisle running through and out of both ends.
//
//	# # # # # # # # # # #
//	# B B # T # L L L ~ #
//	. . . . . . . . . . .
//	. . . . . . . . . . .
//	# B . # T # L L L . #
//	# # # # # # # # # # #
//
// A hub and spoke: an open concourse with a room down each spoke —
// bunkroom west, hold east, latrine south, and the back half of a long hold
// north. Every spoke's far end, and any side with no spoke, is a doorway.
//
//	# # # # # . . # # # # # # #
//	# B B # . . . . # L L L ~ #
//	. . . . . . . . . . . . . .
//	. . . . . . . . . . . . . .
//	# B . # . . . . # L L L . #
//	# # # # # . . # # # # # # #
//	      # T . . T #
//	      # # . . # #
//
// A knobby cluster: a spine corridor, open at both ends, with the rooms
// budding off it above and below as dead-end knobs of different depths.
//
//	            # # # # # #
//	            # L . . L #
//	  # # # # # # L . . L #
//	  # B . . B # L . . L #
//	  # B . . . # ~ . . . #
//	# # # . . # # # . . # # #
//	. . . . . . . . . . . . .
//	. . . . . . . . . . . . .
//	# # # # # . . # # # # # #
//	      # T . . T #
//	      # # # # # #
//
// Every room is the same segment underneath — a hull wall, a row of fixtures,
// a two-wide aisle, a row of fixtures, a hull wall, run along or across the
// map — so every fixture faces the aisle and passengers can pass each other
// in it whatever the shape.

// shipShape is which plan a ship is built to.
type shipShape uint8

const (
	shipStick shipShape = iota
	shipHub
	shipCluster
	numShipShapes
)

func (s shipShape) String() string {
	switch s {
	case shipHub:
		return "hub-and-spoke"
	case shipCluster:
		return "cluster"
	}
	return "stick"
}

// shipCell is one tile of a ship's footprint.
type shipCell uint8

const (
	shipVoid shipCell = iota // outside the shape: the landing leaves it be
	shipHull                 // metal wall (the Hull terrain)
	shipDeck                 // floor, or a fixture standing on it
)

// shipFixture is one fixture in a ship's layout, as an offset from its
// top-left. slot is the passenger it is private to, or -1 for a communal one.
type shipFixture struct {
	at      Point
	terrain Terrain
	slot    int
}

// shipLayout is a ship's floor plan, decided by who it carries and its shape.
type shipLayout struct {
	shape         shipShape
	width, height int
	// cells is the footprint, row-major.
	cells    []shipCell
	fixtures []shipFixture
	// floor is the interior aisle tiles in the order passengers, then pets,
	// step out onto them.
	floor []Point
	// doors are the approach tiles just outside the exterior doorways,
	// reserved like a room's.
	doors []Point
	// margin is the one-tile ring of void round the shape: the walkway, and
	// the crater the landing blasts out of any rock there.
	margin []Point
	// rows draws the shape for a frontend: '#' hull, '.' deck, ' ' void.
	rows []string
}

// cellAt is the footprint's cell at (dx, dy), void off the footprint.
func (l *shipLayout) cellAt(dx, dy int) shipCell {
	if dx < 0 || dy < 0 || dx >= l.width || dy >= l.height {
		return shipVoid
	}
	return l.cells[dy*l.width+dx]
}

func (l *shipLayout) hullAt(dx, dy int) bool { return l.cellAt(dx, dy) == shipHull }

// inShip reports whether (dx, dy) is part of the ship: hull or deck.
func (l *shipLayout) inShip(dx, dy int) bool { return l.cellAt(dx, dy) != shipVoid }

// forEachTile visits the ship's tiles, hull and deck, row-major.
func (l *shipLayout) forEachTile(visit func(d Point, hull bool)) {
	for dy := 0; dy < l.height; dy++ {
		for dx := 0; dx < l.width; dx++ {
			if c := l.cells[dy*l.width+dx]; c != shipVoid {
				visit(Point{dx, dy, 0}, c == shipHull)
			}
		}
	}
}

// shipRoom is one room's fixtures: count of them, the i-th being at(i).
type shipRoom struct {
	count int
	at    func(i int) (Terrain, int)
}

// slice is fixtures [from, from+n) of r as a room of their own.
func (r shipRoom) slice(from, n int) shipRoom {
	return shipRoom{n, func(i int) (Terrain, int) { return r.at(from + i) }}
}

// shipCanvas is a layout being drawn, in coordinates of the planner's
// choosing; layout normalizes them to a top-left of (0, 0).
type shipCanvas struct {
	cells    map[Point]shipCell
	fixtures []shipFixture
	floor    []Point
}

func newShipCanvas() *shipCanvas { return &shipCanvas{cells: make(map[Point]shipCell)} }

// hull walls p in unless it is already deck: a doorway punched through a
// shared wall stays open whichever room is drawn last.
func (c *shipCanvas) hull(p Point) {
	if c.cells[p] != shipDeck {
		c.cells[p] = shipHull
	}
}

func (c *shipCanvas) deck(p Point) { c.cells[p] = shipDeck }

// segment draws one room: cols columns of interior between two end walls,
// six tiles across — hull, fixture row, two aisle rows, fixture row, hull.
// It runs along the map from o when vertical is false and down it when true;
// local (u, v) is u along the room (0 and cols+1 the end walls) and v across
// it. An open end has a two-wide doorway in the aisle rows. Fixtures fill
// the room column by column from u = 1, one side then the other, so it is
// a column longer per two fixtures. It returns the aisle tiles on each side,
// in order along the room.
func (c *shipCanvas) segment(o Point, vertical bool, cols int, r shipRoom, openStart, openEnd bool) (near, far []Point) {
	at := func(u, v int) Point {
		if vertical {
			return o.Add(v, u)
		}
		return o.Add(u, v)
	}
	for u := 0; u <= cols+1; u++ {
		end := u == 0 || u == cols+1
		open := (u == 0 && openStart) || (u == cols+1 && openEnd)
		for v := 0; v < 6; v++ {
			aisle := v == 2 || v == 3
			switch {
			case v == 0 || v == 5 || (end && !(aisle && open)):
				c.hull(at(u, v))
			default:
				c.deck(at(u, v))
			}
		}
		if !end {
			near = append(near, at(u, 2))
			far = append(far, at(u, 3))
		}
	}
	for i := 0; i < r.count; i++ {
		v := 1
		if i%2 == 1 {
			v = 4
		}
		t, slot := r.at(i)
		c.fixtures = append(c.fixtures, shipFixture{at(1+i/2, v), t, slot})
	}
	return near, far
}

// shipCols is how many columns a room of count fixtures needs: two a column.
func shipCols(count int) int { return max(1, (count+1)/2) }

// layout finishes the drawing: the footprint's bounding box becomes the
// layout's, with every point moved so its top-left is (0, 0). The doors are
// worked out here, from the shape — every void tile beside a deck tile is
// outside an exterior doorway — so no planner has to list them.
//
// The canvas is a map, but nothing here depends on its order: the bounds
// are a min and a max, and every list is built by walking the box.
func (c *shipCanvas) layout(shape shipShape) shipLayout {
	first := true
	var lo, hi Point
	for p := range c.cells {
		if first {
			lo, hi, first = p, p, false
			continue
		}
		lo = Point{min(lo.X, p.X), min(lo.Y, p.Y), lo.Level}
		hi = Point{max(hi.X, p.X), max(hi.Y, p.Y), hi.Level}
	}
	l := shipLayout{shape: shape, width: hi.X - lo.X + 1, height: hi.Y - lo.Y + 1}
	l.cells = make([]shipCell, l.width*l.height)
	for p, cell := range c.cells {
		l.cells[(p.Y-lo.Y)*l.width+p.X-lo.X] = cell
	}
	shift := func(p Point) Point { return Point{p.X - lo.X, p.Y - lo.Y, p.Level} }
	for _, f := range c.fixtures {
		f.at = shift(f.at)
		l.fixtures = append(l.fixtures, f)
	}
	for _, p := range c.floor {
		l.floor = append(l.floor, shift(p))
	}
	for dy := -1; dy <= l.height; dy++ {
		for dx := -1; dx <= l.width; dx++ {
			if l.inShip(dx, dy) {
				continue
			}
			door, margin := false, false
			for _, d := range veinNeighbors {
				door = door || l.cellAt(dx+d.X, dy+d.Y) == shipDeck
			}
			for ny := dy - 1; ny <= dy+1; ny++ {
				for nx := dx - 1; nx <= dx+1; nx++ {
					margin = margin || l.inShip(nx, ny)
				}
			}
			if door {
				l.doors = append(l.doors, Point{dx, dy, 0})
			}
			if margin {
				l.margin = append(l.margin, Point{dx, dy, 0})
			}
		}
	}
	l.rows = make([]string, l.height)
	var b strings.Builder
	for dy := range l.rows {
		b.Reset()
		for dx := 0; dx < l.width; dx++ {
			b.WriteByte(" #."[l.cellAt(dx, dy)])
		}
		l.rows[dy] = b.String()
	}
	return l
}

// shipRooms is the three rooms a ship carries for len(keepers) passengers,
// keepers[i] saying whether passenger i keeps a chicken (and so has a trough
// in the hold): the bunkroom, the latrine, and the hold.
func (w *World) shipRooms(keepers []bool) (bunks, toilets, hold shipRoom) {
	n := len(keepers)
	var troughs []int
	for i, k := range keepers {
		if k {
			troughs = append(troughs, i)
		}
	}
	communal := func(t Terrain) func(int) (Terrain, int) {
		return func(int) (Terrain, int) { return t, -1 }
	}
	bunks = shipRoom{shipShare(n, w.cfg.ShipBunkPercent), communal(Bed)}
	toilets = shipRoom{shipShare(n, w.cfg.ShipToiletPercent), communal(Toilet)}
	hold = shipRoom{n + len(troughs), func(i int) (Terrain, int) {
		if i < n {
			return Storage, i
		}
		return Trough, troughs[i-n]
	}}
	return bunks, toilets, hold
}

// planShip lays out a ship of the given shape for len(keepers) passengers.
// Every shape leaves a step-out tile for each passenger and a pet each; one
// that somehow would not falls back to a stick, which always does.
func (w *World) planShip(keepers []bool, shape shipShape) shipLayout {
	n := len(keepers)
	var l shipLayout
	switch shape {
	case shipHub:
		l = w.planHub(keepers)
	case shipCluster:
		l = w.planCluster(keepers)
	default:
		return w.planStick(keepers)
	}
	if len(l.floor) < 2*n {
		return w.planStick(keepers)
	}
	return l
}

// planStick lays the rooms out in a row behind one hull, joined by the aisle
// running the ship's length and out of a doorway at each end. A room that
// would hold nothing is left out.
func (w *World) planStick(keepers []bool) shipLayout {
	n := len(keepers)
	bunks, toilets, hold := w.shipRooms(keepers)
	var rooms []shipRoom
	for _, r := range []shipRoom{bunks, toilets, hold} {
		if r.count > 0 {
			rooms = append(rooms, r)
		}
	}
	// Everyone aboard steps out onto a tile of their own: every passenger
	// and a pet each at most. Bunks and toilets alone leave room for that,
	// except in a few ships with many pets and few troughs; those get a
	// little spare floor at the end of the hold.
	cols := make([]int, len(rooms))
	total := len(rooms) - 1 // a partition's doorway is two tiles of floor too
	for i, r := range rooms {
		cols[i] = shipCols(r.count)
		total += cols[i]
	}
	cols[len(cols)-1] += max(0, n-total)
	c := newShipCanvas()
	var top, bottom, partitions []Point
	x := 0
	for i, r := range rooms {
		near, far := c.segment(Point{x, 0, 0}, false, cols[i], r, true, true)
		top = append(top, near...)
		bottom = append(bottom, far...)
		x += cols[i] + 1
		if i < len(rooms)-1 {
			partitions = append(partitions, Point{x, 2, 0}, Point{x, 3, 0})
		}
	}
	c.floor = append(append(top, bottom...), partitions...)
	return c.layout(shipStick)
}

// shipHubSide is the hub's square: hull round a 4×4 concourse, with a
// two-wide doorway in the middle of every side.
const shipHubSide = 6

// shipHubSplit is the most fixtures one hold spoke takes: a longer hold is
// split between the east and north spokes, so no spoke outgrows the rest.
const shipHubSplit = 8

// planHub lays the ship out as an open concourse with a room down each
// spoke: the bunkroom west, the hold east, the latrine south, and the back
// half of a long hold north. A side with no spoke is an exterior doorway,
// as is the far end of every spoke.
func (w *World) planHub(keepers []bool) shipLayout {
	bunks, toilets, hold := w.shipRooms(keepers)
	c := newShipCanvas()
	for y := 0; y < shipHubSide; y++ {
		for x := 0; x < shipHubSide; x++ {
			edge := x == 0 || y == 0 || x == shipHubSide-1 || y == shipHubSide-1
			door := (x == 2 || x == 3) && (y == 0 || y == shipHubSide-1) ||
				(y == 2 || y == 3) && (x == 0 || x == shipHubSide-1)
			if edge && !door {
				c.hull(Point{x, y, 0})
				continue
			}
			c.deck(Point{x, y, 0})
			if !edge {
				c.floor = append(c.floor, Point{x, y, 0})
			}
		}
	}
	north := shipRoom{}
	if hold.count > shipHubSplit {
		back := hold.count / 2
		north = hold.slice(hold.count-back, back)
		hold = hold.slice(0, hold.count-back)
	}
	edge := shipHubSide - 1
	spoke := func(r shipRoom, o func(cols int) Point, vertical bool) {
		if r.count == 0 {
			return
		}
		cols := shipCols(r.count)
		near, far := c.segment(o(cols), vertical, cols, r, true, true)
		c.floor = append(append(c.floor, near...), far...)
	}
	spoke(hold, func(int) Point { return Point{edge, 0, 0} }, false)
	spoke(bunks, func(cols int) Point { return Point{-(cols + 1), 0, 0} }, false)
	spoke(toilets, func(int) Point { return Point{0, edge, 0} }, true)
	spoke(north, func(cols int) Point { return Point{0, -(cols + 1), 0} }, true)
	return c.layout(shipHub)
}

// shipKnobDepths is how many fixtures each knob of a cluster takes in turn:
// two to a row, so knobs one to four rows deep. The mix is what makes the
// cluster knobby rather than a comb of equal teeth.
var shipKnobDepths = [...]int{6, 4, 8, 2, 6, 8, 4}

// planCluster lays the ship out as a spine corridor with its rooms budding
// off it as knobs: dead-end segments one to four rows deep, alternating
// above and below the spine, next to each other along it so neighbors share
// a wall. Each room is cut into knobs in turn (shipKnobDepths), and only the
// spine's ends open onto the outside.
func (w *World) planCluster(keepers []bool) shipLayout {
	bunks, toilets, hold := w.shipRooms(keepers)
	var knobs []shipRoom
	for _, r := range []shipRoom{bunks, toilets, hold} {
		for from := 0; from < r.count; {
			take := min(r.count-from, shipKnobDepths[len(knobs)%len(shipKnobDepths)])
			knobs = append(knobs, r.slice(from, take))
			from += take
		}
	}
	c := newShipCanvas()
	var top, bottom []Point
	right := 0
	for i, k := range knobs {
		cols := shipCols(k.count)
		var near, far []Point
		if i%2 == 0 {
			x := 1 + 5*(i/2)
			near, far = c.segment(Point{x, -(cols + 1), 0}, true, cols, k, false, true)
			top = append(append(top, near...), far...)
			right = max(right, x+5)
		} else {
			x := 3 + 5*(i/2)
			near, far = c.segment(Point{x, 3, 0}, true, cols, k, true, false)
			bottom = append(append(bottom, near...), far...)
			right = max(right, x+5)
		}
	}
	width := right + 2
	var spine [2][]Point
	for x := 0; x < width; x++ {
		c.hull(Point{x, 0, 0})
		c.hull(Point{x, 3, 0})
		c.deck(Point{x, 1, 0})
		c.deck(Point{x, 2, 0})
		if x > 0 && x < width-1 {
			spine[0] = append(spine[0], Point{x, 1, 0})
			spine[1] = append(spine[1], Point{x, 2, 0})
		}
	}
	c.floor = append(append(append(spine[0], spine[1]...), top...), bottom...)
	return c.layout(shipCluster)
}

// shipShapeSalt separates shipShapeFor's hash from the arrival rolls'.
const shipShapeSalt = 0x3C6EF372FE94F82B

// shipShapeFor is the shape of the ship whose first passenger has this ID,
// by the ship-*-weight odds: a pure function of the seed and the ID, like
// arrivalRareItem, so it shifts no random draw and a frontend can be shown
// the next ship's shape before it lands. All weights zero is a stick.
func (w *World) shipShapeFor(first EntityID) shipShape {
	weights := [numShipShapes]int{
		max(0, w.cfg.ShipStickWeight), max(0, w.cfg.ShipHubWeight), max(0, w.cfg.ShipClusterWeight),
	}
	total := 0
	for _, wt := range weights {
		total += wt
	}
	if total == 0 {
		return shipStick
	}
	s := uint64(w.cfg.Seed) ^ uint64(first)*0x9E3779B97F4A7C15 ^ shipShapeSalt
	r := int(splitmix64(&s) % uint64(total))
	for shape, wt := range weights {
		if r < wt {
			return shipShape(shape)
		}
		r -= wt
	}
	return shipStick
}

// shipKeepers is which of the n passengers taking the IDs from first keep a
// chicken, and so need a trough in the hold.
func (w *World) shipKeepers(first EntityID, n int) []bool {
	keepers := make([]bool, n)
	for i := range keepers {
		keepers[i] = w.arrivalRareItem(first+EntityID(i)) == rareChicken
	}
	return keepers
}

// nextShipLayout is the layout of a ship of n landing now: its passengers
// take the next n IDs, and their shape is shipShapeFor the first. A shape
// too big for the map, crater and all, comes down as a stick.
func (w *World) nextShipLayout(n int) shipLayout {
	first := w.nextID
	keepers := w.shipKeepers(first, n)
	l := w.planShip(keepers, w.shipShapeFor(first))
	if l.shape != shipStick && (l.width+2 > w.Width || l.height+2 > w.Height) {
		l = w.planStick(keepers)
	}
	return l
}

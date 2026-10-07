package sim

// Point is a tile: an integer grid coordinate on one level of the world. The
// origin is the top-left of every level; X grows east, Y grows south, and
// Level grows downward (see layer.go). Moving with Add stays on the same
// level; only a vertical link (a stair) changes it.
//
// Every position carries its level, so a map keyed by Point never confuses
// two levels, and a positional literal cannot be written without saying which
// level it is on. A keyed literal that leaves Level out lands on the surface,
// which nothing uses yet, so that mistake shows up in tests rather than
// passing as the landing level. See docs/layers.md.
type Point struct {
	X, Y  int
	Level Level
}

// Add returns p offset by dx, dy on the same level.
func (p Point) Add(dx, dy int) Point {
	return Point{p.X + dx, p.Y + dy, p.Level}
}

// Equal reports whether two points are the same tile.
func (p Point) Equal(o Point) bool {
	return p == o
}

// Chebyshev returns the king-move distance between two points: the number of
// 8-directional steps needed to travel from p to o. We use it everywhere
// because movement is 8-directional. Between levels it adds one step per
// level crossed, which keeps it a lower bound on any real route (a stair is
// one step and does not move a mover sideways), but says nothing about where
// the stairs are.
func (p Point) Chebyshev(o Point) int {
	d := max(abs(p.X-o.X), abs(p.Y-o.Y))
	if p.Level != o.Level {
		d += abs(int(p.Level) - int(o.Level))
	}
	return d
}

// Manhattan returns the taxicab distance between two points, counting one
// step per level crossed, as Chebyshev does.
func (p Point) Manhattan(o Point) int {
	return abs(p.X-o.X) + abs(p.Y-o.Y) + abs(int(p.Level)-int(o.Level))
}

// Adjacent reports whether o is within one 8-directional step of p on the
// same level (and not p itself). A tile on another level is never adjacent,
// even straight above or below: levels meet only at stairs.
func (p Point) Adjacent(o Point) bool {
	return p.Level == o.Level && p.Chebyshev(o) == 1
}

// Within reports whether o is on p's level and no more than r 8-directional
// steps from it: "close enough to touch" (r = 1) or "in range". Unlike
// comparing Chebyshev, it is never true across levels, however close a tile
// straight above or below is.
func (p Point) Within(o Point, r int) bool {
	return p.Level == o.Level && max(abs(p.X-o.X), abs(p.Y-o.Y)) <= r
}

// gridStep is a step on the grid: a dx, dy to Add to a Point.
type gridStep struct {
	X, Y int
}

// neighbors8 lists the eight steps around a cell, in no meaningful order.
var neighbors8 = [8]gridStep{
	{-1, -1}, {0, -1}, {1, -1},
	{-1, 0}, {1, 0},
	{-1, 1}, {0, 1}, {1, 1},
}

// neighbors4 lists the four orthogonal steps around a cell.
var neighbors4 = [4]gridStep{{0, -1}, {-1, 0}, {1, 0}, {0, 1}}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// sign returns -1, 0, or 1 matching the sign of x.
func sign(x int) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	default:
		return 0
	}
}

// stepToward returns the single 8-directional step from a that most reduces the
// distance to b. If a == b it returns a.
func stepToward(a, b Point) Point {
	return a.Add(sign(b.X-a.X), sign(b.Y-a.Y))
}

package sim

// Point is an integer grid coordinate. The origin is the top-left of the
// world; X grows east, Y grows south.
type Point struct {
	X, Y int
}

// Add returns p offset by dx, dy.
func (p Point) Add(dx, dy int) Point {
	return Point{p.X + dx, p.Y + dy}
}

// Equal reports whether two points are the same cell.
func (p Point) Equal(o Point) bool {
	return p.X == o.X && p.Y == o.Y
}

// Chebyshev returns the king-move distance between two points: the number of
// 8-directional steps needed to travel from p to o. We use it everywhere
// because movement is 8-directional.
func (p Point) Chebyshev(o Point) int {
	dx := abs(p.X - o.X)
	dy := abs(p.Y - o.Y)
	return max(dx, dy)
}

// Adjacent reports whether o is within one 8-directional step of p (and not p
// itself).
func (p Point) Adjacent(o Point) bool {
	d := p.Chebyshev(o)
	return d == 1
}

// neighbors8 lists the eight steps around a cell, in no meaningful order.
var neighbors8 = [8]Point{
	{-1, -1}, {0, -1}, {1, -1},
	{-1, 0}, {1, 0},
	{-1, 1}, {0, 1}, {1, 1},
}

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

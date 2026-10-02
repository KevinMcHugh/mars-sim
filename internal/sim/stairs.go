package sim

import "slices"

// Stairs: the one way between levels. A stair is a StairDown on one level and
// a StairUp straight below it (see linkFrom); searches cross it as a single
// step. This file keeps the list of stairs and the travel estimate built on
// it. Building one is in project.go. See docs/z-levels.md.

// trackStair keeps w.stairs, the upper end of every stair sorted by
// lessPoint, in step with a terrain change at p. setTerrain calls it.
func (w *World) trackStair(p Point, old, t Terrain) {
	if old == StairDown {
		if i, ok := slices.BinarySearchFunc(w.stairs, p, cmpPoint); ok {
			w.stairs = slices.Delete(w.stairs, i, i+1)
		}
	}
	if t == StairDown {
		if i, ok := slices.BinarySearchFunc(w.stairs, p, cmpPoint); !ok {
			w.stairs = slices.Insert(w.stairs, i, p)
		}
	}
}

// cmpPoint is lessPoint as a comparison.
func cmpPoint(a, b Point) int {
	switch {
	case lessPoint(a, b):
		return -1
	case lessPoint(b, a):
		return 1
	}
	return 0
}

// unreachableEstimate is travelEstimate's answer when no chain of stairs
// joins two levels: far enough to lose to anything reachable, small enough to
// add to without overflowing.
const unreachableEstimate = 1 << 24

// travelEstimate is a cheap guess at how many steps a walk from a to b takes,
// for choosing the nearest of several candidates. On one level it is the
// Chebyshev distance, as every such choice used before there were levels.
// Across levels it is the cheapest chain of straight lines through stairs:
// over to a stair, one step through it, and on from its other end, level by
// level. Like Chebyshev it ignores walls; unlike Chebyshev between levels, it
// knows where the stairs are, so a rock straight below a colonist is not
// "one step away" when the nearest stair is fifty tiles off.
func (w *World) travelEstimate(a, b Point) int {
	if a.Level == b.Level {
		return max(abs(a.X-b.X), abs(a.Y-b.Y))
	}
	return w.travelEstimateAcross(a, b)
}

// travelEstimateAcross is travelEstimate between levels: kept out of line so
// the same-level case, which is nearly every call, inlines.
func (w *World) travelEstimateAcross(a, b Point) int {
	step := Level(1)
	if b.Level < a.Level {
		step = -1
	}
	type reach struct {
		at   Point
		cost int
	}
	cur := []reach{{a, 0}}
	for l := a.Level; l != b.Level; l += step {
		// The stairs between l and l+step, by their upper end's level.
		upper := min(l, l+step)
		var next []reach
		for _, s := range w.stairs {
			if s.Level != upper {
				continue
			}
			best := unreachableEstimate
			for _, r := range cur {
				best = min(best, r.cost+r.at.Chebyshev(Point{s.X, s.Y, l}))
			}
			if _, ok := w.linkFrom(s); ok {
				next = append(next, reach{Point{s.X, s.Y, l + step}, best + 1})
			}
		}
		if len(next) == 0 {
			return unreachableEstimate
		}
		cur = next
	}
	best := unreachableEstimate
	for _, r := range cur {
		best = min(best, r.cost+r.at.Chebyshev(b))
	}
	return best
}

package sim

import (
	"fmt"
	"slices"
)

// Stairs: the one way between levels. A stair is a StairDown on one level and
// a StairUp straight below it (see links); searches cross it as a single
// step. This file keeps the list of stairs and the travel estimate built on
// it. Building one is in project.go. See docs/z-levels.md.

// trackLinks keeps w.stairs (the upper end of every stair), w.shafts (every
// shaft tile that can lead down) and w.holes, all sorted by lessPoint, in
// step with a terrain change at p. setTerrain calls it.
func (w *World) trackLinks(p Point, old, t Terrain) {
	leadsDown := func(t Terrain) bool { return t == ShaftTop || t == ShaftMid }
	w.stairs = trackSorted(w.stairs, p, old == StairDown, t == StairDown)
	w.shafts = trackSorted(w.shafts, p, leadsDown(old), leadsDown(t))
	w.holes = trackSorted(w.holes, p, old == Hole, t == Hole)
}

// trackSorted removes p from the sorted list if was, and inserts it if is.
func trackSorted(list []Point, p Point, was, is bool) []Point {
	if was == is {
		return list
	}
	i, ok := slices.BinarySearchFunc(list, p, cmpPoint)
	if was && ok {
		return slices.Delete(list, i, i+1)
	}
	if is && !ok {
		return slices.Insert(list, i, p)
	}
	return list
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
// Across levels it is the cheapest chain of straight lines through stairs
// and shafts: over to one, through it (a step for a stair, a climb for a
// shaft), and on from its other end, level by level. Like Chebyshev it ignores walls; unlike Chebyshev between levels, it
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
		// The stairs and shafts between l and l+step, by their upper end's
		// level.
		upper := min(l, l+step)
		var next []reach
		for _, list := range [2][]Point{w.stairs, w.shafts} {
			for _, s := range list {
				if s.Level != upper {
					continue
				}
				cost := int32(0)
				lk, n := w.links(s)
				for _, k := range lk[:n] {
					if k.to.Level == upper+1 {
						cost = k.cost
					}
				}
				if cost == 0 {
					continue // not linked down (yet)
				}
				best := unreachableEstimate
				for _, r := range cur {
					best = min(best, r.cost+r.at.Chebyshev(Point{s.X, s.Y, l}))
				}
				next = append(next, reach{Point{s.X, s.Y, l + step}, best + int(cost)})
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

// addLayer returns level l's layer, making it the first time anything breaks
// into the level. A new layer is all rock: its chunks are generated lazily,
// as the landing level's are, from the first tile anything reveals on it.
func (w *World) addLayer(l Level) *Layer {
	if layer := w.layer(l); layer != nil {
		return layer
	}
	for int(l) >= len(w.layers) {
		w.layers = append(w.layers, nil)
	}
	layer := newLayer(l, w.Width, w.Height)
	layer.board = newJobBoard(w)
	// The facility kinds the shared fields track, as trackFacility gave the
	// layers that already existed.
	for k, f := range w.fields {
		if f != nil {
			layer.facilityTiles[k] = make(map[Point]struct{})
		}
	}
	w.layers[l] = layer
	w.initGen(layer)
	return layer
}

// canDigStairAt reports whether a stair down can be dug at p: open, known
// floor, above a level the config allows digging to.
func (w *World) canDigStairAt(p Point) bool {
	return w.TerrainAt(p) == Floor && w.discovered(p) &&
		int(p.Level)+1 <= w.cfg.DeepestLevel && p.Level >= LandingLevel
}

// digStair finishes a stair at p: the tile straight below, on the next level,
// is broken into and becomes the StairUp, and p becomes the StairDown. The
// level below is made if this is the first way into it, and breaking in
// reveals what is around the bottom of the stair, natural caverns and the
// aliens nesting in them included (see revealAround), exactly as digging
// sideways into a cavern does. It reports false, changing nothing, when p
// cannot take a stair (see canDigStairAt).
//
// Whatever rock is dug out of the bottom is shaped into the stair: nobody
// gets its ore. A stair is a way down, not a mine.
func (w *World) digStair(p Point) bool {
	if !w.canDigStairAt(p) {
		return false
	}
	below := Point{p.X, p.Y, p.Level + 1}
	w.addLayer(below.Level)
	w.SetTerrain(below, StairUp)
	w.SetTerrain(p, StairDown)
	w.logEvent(LogBuildComplete, fmt.Sprintf("A stair is dug down to level %d.", below.Level))
	return true
}

// stairProjectName names a stair's project in the job board and the log.
const stairProjectName = "stair down"

// planStairs marks out a stair down when the colony wants one and Config
// allows it: when the player ordered one (OrderStair), or when there is no
// mining frontier left to work on any level. One stair is planned at a time,
// from the deepest level the colony has reached. It draws nothing random, and
// with DeepestLevel at 1 it does nothing at all, so a game that cannot dig
// down runs exactly as it did before levels.
func (w *World) planStairs() {
	from := w.deepestLevel()
	if int(from) >= w.cfg.DeepestLevel {
		w.manualStairs = 0 // nowhere further to dig
		return
	}
	if w.stairPlanned() {
		return
	}
	if w.manualStairs == 0 && w.unclaimedFrontier() > 0 {
		return
	}
	site, ok := w.findStairSite(from)
	if !ok {
		return // try again next planning round
	}
	if !w.designateStair(site, Community) {
		return
	}
	if w.manualStairs > 0 {
		w.manualStairs--
	}
}

// deepestLevel is the deepest level the colony has broken into.
func (w *World) deepestLevel() Level {
	deepest := LandingLevel
	for _, l := range w.layers {
		if l != nil {
			deepest = max(deepest, l.Level)
		}
	}
	return deepest
}

// stairPlanned reports whether a stair is already planned and unbuilt.
func (w *World) stairPlanned() bool {
	for _, p := range w.projects {
		for _, t := range p.tasks {
			if t.terrain == StairDown && !w.taskDone(t) {
				return true
			}
		}
	}
	return false
}

// designateStair plans a stair down at p, paid for by issuer, as a
// one-task project the colony's builders take up like any other.
func (w *World) designateStair(p Point, issuer Owner) bool {
	if !w.canDigStairAt(p) {
		return false
	}
	pr := &project{id: w.nextProjectID, name: stairProjectName, queuedTick: w.tick, issuer: issuer}
	pr.tasks = []*buildTask{{pos: p, terrain: StairDown, proj: pr}}
	if !w.fundProject(pr) {
		return false
	}
	w.nextProjectID++
	w.projects = append(w.projects, pr)
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony marks out a stair down to level %d.", p.Level+1))
	return true
}

// findStairSite picks where on level l the next stair down goes: open,
// known floor in the colony's own room (reachable from the main room, so
// someone can dig it), with every neighbour open floor too, so the stair
// neither plugs a corridor nor sits against a wall it would leave no room
// to step around. The search spreads from an anchor and takes the first
// site it meets, ring by ring and in a fixed order within a ring: on the
// landing level the middle of the map (where the colony lands), on a deeper
// level the foot of the stair (or, with no stair, the shaft) that leads into
// it. The search stops at the edge of the main room's extent on level l, the
// only place a site can be (see mainRoomBounds).
func (w *World) findStairSite(l Level) (Point, bool) {
	anchor := Point{w.Width / 2, w.Height / 2, l}
	if l != LandingLevel {
		found := false
		for _, s := range w.stairs {
			if s.Level == l-1 {
				anchor, found = Point{s.X, s.Y, l}, true
				break
			}
		}
		for _, s := range w.shafts {
			if !found && s.Level == l-1 {
				anchor, found = Point{s.X, s.Y, l}, true
			}
		}
	}
	x0, y0, x1, y1, reached := w.mainRoomBounds(l)
	if !reached {
		return Point{}, false
	}
	designated := make(map[Point]bool)
	for _, p := range w.projects {
		for _, t := range p.tasks {
			designated[t.pos] = true
		}
	}
	ok := func(p Point) bool {
		if !w.canDigStairAt(p) || designated[p] || w.doorTiles[p] || w.occupied(p) ||
			w.mainRoom == 0 || w.roomOf(p) != w.mainRoom {
			return false
		}
		for _, d := range neighbors8 {
			n := p.Add(d.X, d.Y)
			if w.TerrainAt(n) != Floor || designated[n] || w.doorTiles[n] {
				return false
			}
		}
		return true
	}
	if ok(anchor) {
		return anchor, true
	}
	var site Point
	found := false
	// The farthest ring that still touches the main room's bounds.
	radius := max(anchor.X-x0, x1-1-anchor.X, anchor.Y-y0, y1-1-anchor.Y)
	w.forEachInRadius(anchor, radius, func(p Point) bool {
		if ok(p) {
			site, found = p, true
			return true
		}
		return false
	})
	return site, found
}

// mainRoomBounds is the half-open tile bounds [x0,x1) x [y0,y1) of the
// chunks holding the main room's regions on level l; ok is false when the
// main room does not reach level l. findStairSite searches no further: a
// level the colony has just broken into is often only the foot of a stair or
// shaft, with no site anywhere on it, and searching the whole map for one
// cost seconds a planning round on a big map (minutes on the browser's
// 10000x10000), every round until the order could be met.
func (w *World) mainRoomBounds(l Level) (x0, y0, x1, y1 int, ok bool) {
	if w.mainRoom == 0 {
		return 0, 0, 0, 0, false
	}
	// Map order is fine here: only the min and max are kept.
	for _, r := range w.regions {
		if r.level != l || r.room != w.mainRoom {
			continue
		}
		cx0, cy0, cx1, cy1 := w.chunkBounds(r.chunk)
		if !ok {
			x0, y0, x1, y1, ok = cx0, cy0, cx1, cy1, true
			continue
		}
		x0, y0 = min(x0, cx0), min(y0, cy0)
		x1, y1 = max(x1, cx1), max(y1, cy1)
	}
	return x0, y0, x1, y1, ok
}

// finishStair is jobBuild's last step for a stair task: dig it, and pay and
// credit the digger as for any build.
func (w *World) finishStair(e *Entity) {
	if !w.digStair(e.Target) {
		w.clearJob(e)
		return
	}
	w.practise(e, SkillMining, w.buildTicks(StairDown))
	if t := e.task; t != nil {
		w.payWork(t.order, e)
	}
	o := w.occurrence(e, ActionConstruct, nil, e.Target, "Dug a stair down to level %d at (%d, %d).",
		e.Target.Level+1, e.Target.X, e.Target.Y)
	o.Object = FactRef{Noun: NounStructure, Label: StairDown.String()}
	w.emitOccurrence(o)
	w.clearJob(e)
}

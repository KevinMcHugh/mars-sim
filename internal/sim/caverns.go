package sim

import (
	"fmt"
	"math/rand"
	"slices"
)

// Natural caverns: pockets of open floor hollowed out of the rock at world
// generation, hidden under the fog until the colony digs into one, and
// sometimes joined to their nearest neighbor by a winding passage. See
// docs/caverns.md.

const (
	// cavernLandingClearance is how many tiles of rock, at least, separate
	// any natural cavern or passage from the landing cavern's bounding box.
	// Enough that the landing site's revealed rim never touches one, and that
	// finding the first cave takes a few tiles of deliberate digging.
	cavernLandingClearance = 4
	// cavernSpacing is how far (Chebyshev) a new cavern keeps from any
	// earlier one, so separate caverns stay separate and a passage between
	// them means something.
	cavernSpacing = 3
	// cavernEdgeMargin keeps caverns and passages off the outermost tiles, so
	// the world's edge is still solid rock.
	cavernEdgeMargin = 1
	// cavernPlanFailures is how many placements in a row may fail (the site
	// crowded the landing cavern or another cave) before generation gives up
	// on reaching CavernPercent. Only bites on maps too small to fit it.
	cavernPlanFailures = 50
	// cavernMaxLobes caps how many overlapping ellipses one cavern is built
	// from, so a tiny maximum size cannot spin forever.
	cavernMaxLobes = 12
	// passageMaxSpan is the furthest apart (Chebyshev, center to center) two
	// caverns may be and still get a passage. Past this, a one-tile tunnel is
	// less "a passage between caves" and more a second mining network.
	passageMaxSpan = 32
	// passageAttempts is how many random walks a passage gets to find a route
	// that stays clear of the landing site before it is abandoned.
	passageAttempts = 3
)

// cavern is one generated natural cavern.
type cavern struct {
	center Point // the seed tile the cavern was grown from; always inside it
	size   int
}

// generateCaverns hollows natural caverns out of the rock until roughly
// CavernPercent of the map is cavern floor, then joins some of them to their
// nearest neighbor with a passage. Everything is carved hidden (see
// carveHidden). landingLo/landingHi bound the landing cavern, which must
// already be carved: caverns keep cavernLandingClearance tiles away from that
// box.
func (w *World) generateCaverns(rng *rand.Rand, landingLo, landingHi Point) []cavern {
	lo := Point{landingLo.X - cavernLandingClearance, landingLo.Y - cavernLandingClearance}
	hi := Point{landingHi.X + cavernLandingClearance, landingHi.Y + cavernLandingClearance}
	nearLanding := func(p Point) bool {
		return p.X >= lo.X && p.X <= hi.X && p.Y >= lo.Y && p.Y <= hi.Y
	}

	minSize, maxSize := max(1, w.cfg.CavernMin), w.cfg.CavernMax
	if maxSize < minSize {
		maxSize = minSize
	}
	target := len(w.tiles) * w.cfg.CavernPercent / 100
	var caves []cavern
	placed, failures := 0, 0
	for placed < target && failures < cavernPlanFailures {
		size := minSize + rng.Intn(maxSize-minSize+1)
		center, tiles, ok := w.planCavern(rng, size, nearLanding)
		if !ok {
			failures++
			continue
		}
		failures = 0
		for _, p := range tiles {
			w.carveHidden(p)
		}
		placed += len(tiles)
		caves = append(caves, cavern{center: center, size: len(tiles)})
	}

	w.joinCaverns(rng, caves, nearLanding)
	return caves
}

// planCavern picks a random site and grows a cavern of about size tiles there
// as a cluster of overlapping, wider-than-tall ellipses (the map's own 2:1
// shape), each lobe centered on a tile of the cavern so far so the whole thing
// is one connected pocket. It returns the tiles to carve, in a deterministic
// order, or false if the site is unusable: too close to the landing cavern,
// another cavern, or the edge of the world. It carves nothing.
func (w *World) planCavern(rng *rand.Rand, size int, nearLanding func(Point) bool) (Point, []Point, bool) {
	m := cavernEdgeMargin + 1
	if w.Width <= 2*m || w.Height <= 2*m {
		return Point{}, nil, false
	}
	center := Point{m + rng.Intn(w.Width-2*m), m + rng.Intn(w.Height-2*m)}

	tiles := []Point{}
	in := map[Point]bool{}
	lobe := center
	for lobes := 0; len(tiles) < size && lobes < cavernMaxLobes; lobes++ {
		ry := 1 + rng.Intn(2)
		rx := ry + 1 + rng.Intn(ry+1)
		for y := -ry; y <= ry; y++ {
			for x := -rx; x <= rx; x++ {
				if float64(x*x)/float64(rx*rx)+float64(y*y)/float64(ry*ry) > 1.0 {
					continue
				}
				p := lobe.Add(x, y)
				if in[p] {
					continue
				}
				if !w.cavernTileOK(p, nearLanding) {
					return Point{}, nil, false
				}
				in[p] = true
				tiles = append(tiles, p)
			}
		}
		lobe = tiles[rng.Intn(len(tiles))]
	}
	return center, tiles, true
}

// cavernTileOK reports whether p may become cavern floor: inside the edge
// margin, clear of the landing cavern, and at least cavernSpacing from every
// cavern already carved.
func (w *World) cavernTileOK(p Point, nearLanding func(Point) bool) bool {
	if p.X < cavernEdgeMargin || p.Y < cavernEdgeMargin ||
		p.X >= w.Width-cavernEdgeMargin || p.Y >= w.Height-cavernEdgeMargin {
		return false
	}
	if nearLanding(p) {
		return false
	}
	for dy := -cavernSpacing; dy <= cavernSpacing; dy++ {
		for dx := -cavernSpacing; dx <= cavernSpacing; dx++ {
			if w.TerrainAt(p.Add(dx, dy)) != Rock {
				return false
			}
		}
	}
	return true
}

// joinCaverns gives each cavern a chance (CavernPassagePercent) of a passage to
// its nearest neighbor. A pair that are each other's nearest is only rolled
// once. Pairs further apart than passageMaxSpan are left alone.
func (w *World) joinCaverns(rng *rand.Rand, caves []cavern, nearLanding func(Point) bool) {
	if len(caves) < 2 {
		return
	}
	// Bucket centers into passageMaxSpan-sized cells, so a cavern only
	// compares against those in its own and the 8 surrounding cells: every
	// neighbor within passageMaxSpan is there, and one further away is
	// skipped anyway. Comparing against every cavern made this quadratic,
	// most of worldgen's time on a huge map. Candidates are checked in
	// ascending index, keeping the old lowest-index tie-break.
	cellOf := func(p Point) Point { return Point{p.X / passageMaxSpan, p.Y / passageMaxSpan} }
	buckets := map[Point][]int{}
	for i, c := range caves {
		k := cellOf(c.center)
		buckets[k] = append(buckets[k], i)
	}
	var near []int
	rolled := map[[2]int]bool{}
	for i, c := range caves {
		near = near[:0]
		k := cellOf(c.center)
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				near = append(near, buckets[k.Add(dx, dy)]...)
			}
		}
		slices.Sort(near)
		nearest, best := -1, passageMaxSpan+1
		for _, j := range near {
			if j == i {
				continue
			}
			if dist := c.center.Chebyshev(caves[j].center); dist < best {
				nearest, best = j, dist
			}
		}
		if nearest < 0 {
			continue // no cavern within passageMaxSpan
		}
		key := [2]int{min(i, nearest), max(i, nearest)}
		if rolled[key] {
			continue
		}
		rolled[key] = true
		if rng.Intn(100) >= w.cfg.CavernPassagePercent {
			continue
		}
		for attempt := 0; attempt < passageAttempts; attempt++ {
			if path, ok := w.planPassage(rng, c.center, caves[nearest].center, nearLanding); ok {
				for _, p := range path {
					w.carveHidden(p)
				}
				break
			}
		}
	}
}

// planPassage random-walks a one-tile-wide tunnel from one cavern center to
// another: mostly stepping toward the goal, sometimes wandering sideways, and
// only in orthogonal steps so the tunnel reads as a corridor rather than a
// diagonal staircase. It fails if the walk strays near the landing cavern or
// off the usable map, or runs too long.
func (w *World) planPassage(rng *rand.Rand, from, to Point, nearLanding func(Point) bool) ([]Point, bool) {
	cur := from
	limit := 4*from.Chebyshev(to) + 16
	var path []Point
	for steps := 0; cur != to; steps++ {
		if steps >= limit {
			return nil, false
		}
		dx, dy := sign(to.X-cur.X), sign(to.Y-cur.Y)
		var step Point
		switch {
		case rng.Intn(4) == 0:
			step = veinNeighbors[rng.Intn(len(veinNeighbors))]
		case dx != 0 && (dy == 0 || rng.Intn(2) == 0):
			step = Point{dx, 0}
		default:
			step = Point{0, dy}
		}
		next := cur.Add(step.X, step.Y)
		if next.X < cavernEdgeMargin || next.Y < cavernEdgeMargin ||
			next.X >= w.Width-cavernEdgeMargin || next.Y >= w.Height-cavernEdgeMargin {
			continue // bumped the edge; try another step
		}
		if nearLanding(next) {
			return nil, false
		}
		cur = next
		path = append(path, cur)
	}
	return path, true
}

// nestRadius is how far (Chebyshev) from its cavern's center a nest's aliens
// may be placed. Caverns are at least a few tiles across, so the nest lands in
// its own cavern rather than spread down a passage.
const nestRadius = 4

// trackCavernsForNests remembers each cavern's center so a breach can roll
// for its nest (see rollNests), and seeds the stream those rolls use. Only the
// centers are kept: nothing about a nest exists until its cavern is found.
func (w *World) trackCavernsForNests(caves []cavern) {
	w.nestRNG = rand.New(rand.NewSource(w.cfg.Seed ^ 0x0452821E638D0137))
	w.unfoundCaverns = make(map[Point]struct{}, len(caves))
	for _, c := range caves {
		w.unfoundCaverns[c.center] = struct{}{}
	}
}

// rollNests gives each cavern whose center a breach just discovered its one
// CavernNestPercent roll for a nest, in discovery order. Rolling at the
// breach rather than at worldgen means a huge map with thousands of caves
// never holds (or generates) thousands of aliens nobody has met.
func (w *World) rollNests(centers []Point) {
	for _, c := range centers {
		delete(w.unfoundCaverns, c)
		if w.nestRNG == nil || len(w.alienSpecies) == 0 {
			continue
		}
		if w.nestRNG.Intn(100) < w.cfg.CavernNestPercent {
			w.spawnNest(c)
		}
	}
}

// spawnNest places CavernNestMin–CavernNestMax aliens of one species on
// distinct free floor tiles within nestRadius of center. Everything -- the
// count, the species and the tiles -- comes from nestRNG, never the
// simulation stream, and members go through spawnAs so no species is drawn
// from it either.
func (w *World) spawnNest(center Point) {
	lo := max(1, w.cfg.CavernNestMin)
	hi := max(lo, w.cfg.CavernNestMax)
	want := lo + w.nestRNG.Intn(hi-lo+1)
	species := w.nestRNG.Intn(len(w.alienSpecies))
	var sites []Point
	for dy := -nestRadius; dy <= nestRadius; dy++ {
		for dx := -nestRadius; dx <= nestRadius; dx++ {
			if p := center.Add(dx, dy); w.Walkable(p) && w.discovered(p) && !w.occupied(p) {
				sites = append(sites, p)
			}
		}
	}
	placed := 0
	var first *Entity
	// A partial Fisher-Yates shuffle: each step draws one distinct tile.
	for i := 0; i < len(sites) && placed < want; i++ {
		j := i + w.nestRNG.Intn(len(sites)-i)
		sites[i], sites[j] = sites[j], sites[i]
		e := w.spawnAs(Alien, sites[i], species)
		if first == nil {
			first = e
		}
		placed++
	}
	if first != nil {
		w.log.add(fmt.Sprintf("The colony has broken into a nest of %s (%d)!", w.alienPluralFor(first), placed))
	}
}

// dormant reports whether e is an alien in a cave the colony has not found
// yet: one that spawned on hidden cavern floor (see alienSpawnSite).
// Aliens walk only on floor, and undiscovered floor is always a sealed
// cavern (see the invariant above), so a dormant alien cannot reach the
// colony anyway. It keeps to its cave and is invisible to the colony: nobody
// flees from, remembers, or fights an alien sealed behind rock they have
// never dug into.
func (w *World) dormant(e *Entity) bool {
	return e.Kind == Alien && !w.discovered(e.Pos)
}

// dormantTurn is a dormant alien's turn: now and then it shifts to a
// neighboring floor tile of its cave.
func (w *World) dormantTurn(e *Entity) {
	e.State, e.Quarry = Idle, 0
	if w.rng.Intn(4) != 0 {
		return
	}
	d := neighbors8[w.rng.Intn(len(neighbors8))]
	n := e.Pos.Add(d.X, d.Y)
	if w.Walkable(n) && !w.occupiedByOther(n, e.ID) {
		w.moveEntity(e, n)
	}
}

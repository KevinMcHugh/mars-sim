package sim

import "math"

// generate carves the starting situation into a fresh all-Rock world: a central
// landing cavern sized to the starting population, with the colonists inside it,
// and a handful of aliens lurking out in the surrounding rock, ready to burrow
// in.
func generate(w *World) {
	center := Point{w.Width / 2, w.Height / 2}

	// Carve an oval starting cavern large enough to hold the colonists with room
	// to move and a rock frontier to mine.
	rx, ry := w.caveRadii(w.cfg.StartColonists)
	for y := -ry; y <= ry; y++ {
		for x := -rx; x <= rx; x++ {
			if float64(x*x)/float64(rx*rx)+float64(y*y)/float64(ry*ry) <= 1.0 {
				w.SetTerrain(center.Add(x, y), Floor)
			}
		}
	}

	// Place colonists, then mice and cats, by drawing from one shuffled list of
	// open floor tiles, so every placement is a uniform draw without replacement
	// rather than rejection sampling, which could give up. Every Floor tile at
	// this point lies within the cavern just carved, so it is enough to collect
	// candidates from that small box — not the whole map, which on a huge map
	// would dwarf everything else generate() does for the sake of placing a
	// handful of colonists and critters.
	floors := w.freeFloorTilesIn(center.Add(-rx, -ry), center.Add(rx, ry))
	w.rng.Shuffle(len(floors), func(i, j int) { floors[i], floors[j] = floors[j], floors[i] })
	next := 0
	takeFloor := func() (Point, bool) {
		if next >= len(floors) {
			return Point{}, false
		}
		p := floors[next]
		next++
		return p, true
	}

	want := w.cfg.StartColonists
	if want > len(floors) {
		want = len(floors)
	}
	for i := 0; i < want; i++ {
		p, _ := takeFloor()
		w.spawn(Colonist, p)
	}

	// Place aliens out in the rock, away from the cavern, so they must burrow in.
	// Rock this far from a small starting cavern is the overwhelming majority of
	// even a huge map, so random guessing (randomTile's rejection-sampling fast
	// path) finds one immediately.
	minDist := rx + ry + 4
	for i := 0; i < w.cfg.StartAliens; i++ {
		if p, ok := w.randomRockFar(center, minDist); ok {
			w.spawn(Alien, p)
		}
	}

	// Mice and cats live on the floor with the colonists: mice raid the pods,
	// cats chase the mice. Place whatever the cavern has room for.
	for i := 0; i < w.cfg.StartMice; i++ {
		if p, ok := takeFloor(); ok {
			w.spawn(Mouse, p)
		}
	}
	for i := 0; i < w.cfg.StartCats; i++ {
		if p, ok := takeFloor(); ok {
			w.spawn(Cat, p)
		}
	}

	w.log.add("The colony ship settles onto the Martian crust. Something below stirs.")
	w.refreshSpatial()
}

// caveRadii returns the ellipse radii for a starting cavern big enough to hold n
// colonists with breathing room, clamped to something sane and to the world
// bounds. It keeps a 2:1 width:height shape to match the map.
func (w *World) caveRadii(n int) (rx, ry int) {
	const tilesPerColonist = 10
	// area = pi * rx * ry, with rx = 2*ry  =>  ry = sqrt(area / (2*pi)).
	area := float64(n * tilesPerColonist)
	ry = int(math.Ceil(math.Sqrt(area / (2 * math.Pi))))
	if ry < 4 {
		ry = 4
	}
	rx = 2 * ry
	if maxRx := w.Width/2 - 2; rx > maxRx {
		rx = maxRx
	}
	if maxRy := w.Height/2 - 2; ry > maxRy {
		ry = maxRy
	}
	if rx < 1 {
		rx = 1
	}
	if ry < 1 {
		ry = 1
	}
	return rx, ry
}

// freeFloorTiles returns every walkable, unoccupied tile. Used for bulk
// placement where we need a guaranteed, uniform draw.
func (w *World) freeFloorTiles() []Point {
	return w.freeFloorTilesIn(Point{0, 0}, Point{w.Width - 1, w.Height - 1})
}

// freeFloorTilesIn is freeFloorTiles restricted to the inclusive box between lo
// and hi (clamped to the map). For a caller that already knows every matching
// tile lies within some small region — generate's starting cavern, say —
// this avoids scanning the rest of a potentially huge map.
func (w *World) freeFloorTilesIn(lo, hi Point) []Point {
	x0, y0 := max(0, lo.X), max(0, lo.Y)
	x1, y1 := min(w.Width-1, hi.X), min(w.Height-1, hi.Y)
	out := make([]Point, 0, w.countTerrain(Floor))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			p := Point{x, y}
			if w.Walkable(p) && !w.occupied(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// randomTileRejectionAttempts caps how many uniform random guesses randomTile
// tries before falling back to a full scan. On a huge map with a sparsely
// carved colony, most guesses land on the vast majority-Rock area in one try;
// this bounds the cost even when pred matches almost nothing.
const randomTileRejectionAttempts = 4096

// randomTile samples one tile uniformly from those satisfying pred. It tries
// bounded random rejection sampling first — cheap as long as pred matches some
// non-tiny fraction of the map, e.g. randomRockFar's "any Rock tile far
// enough away," true of nearly the whole map outside a small starting
// cavern — and falls back to a full-grid reservoir scan, which always finds a
// match if one exists, only if that fails (as it will for a predicate matching
// only a sliver of a huge map, such as "any free Floor tile" once the colony
// has mined out just a small fraction of it).

func (w *World) randomTile(pred func(Point) bool) (Point, bool) {
	for i := 0; i < randomTileRejectionAttempts; i++ {
		p := Point{w.rng.Intn(w.Width), w.rng.Intn(w.Height)}
		if pred(p) {
			return p, true
		}
	}
	return w.randomTileScan(pred)
}

// randomTileScan reservoir-samples one tile satisfying pred in a single full
// pass. Unlike rejection sampling it always finds a match if one exists, and
// is uniform.
func (w *World) randomTileScan(pred func(Point) bool) (Point, bool) {
	var chosen Point
	found := false
	k := 0
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Point{x, y}
			if !pred(p) {
				continue
			}
			k++
			if w.rng.Intn(k) == 0 {
				chosen, found = p, true
			}
		}
	}
	return chosen, found
}

// randomFloor returns a random open, unoccupied floor tile.
func (w *World) randomFloor() (Point, bool) {
	return w.randomTile(func(p Point) bool { return w.Walkable(p) && !w.occupied(p) })
}

// randomRockFar returns a random unoccupied Rock tile at least minDist from
// origin.
func (w *World) randomRockFar(origin Point, minDist int) (Point, bool) {
	return w.randomTile(func(p Point) bool {
		return w.TerrainAt(p) == Rock && !w.occupied(p) && origin.Chebyshev(p) >= minDist
	})
}

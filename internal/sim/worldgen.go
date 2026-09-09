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

	// Place colonists by drawing from a shuffled list of open floor tiles, so
	// every requested colonist is placed as long as the cavern has room (rather
	// than rejection sampling, which could give up).
	floors := w.freeFloorTiles()
	w.rng.Shuffle(len(floors), func(i, j int) { floors[i], floors[j] = floors[j], floors[i] })
	want := w.cfg.StartColonists
	if want > len(floors) {
		want = len(floors)
	}
	for i := 0; i < want; i++ {
		w.spawn(Colonist, floors[i])
	}

	// Place aliens out in the rock, away from the cavern, so they must burrow in.
	minDist := rx + ry + 4
	for i := 0; i < w.cfg.StartAliens; i++ {
		if p, ok := w.randomRockFar(center, minDist); ok {
			w.spawn(Alien, p)
		}
	}

	w.log.add("The colony ship settles onto the Martian crust. Something below stirs.")
	w.refreshSpatial()
}

// caveRadii returns the ellipse radii for a starting cavern big enough to hold n
// colonists with breathing room, clamped to something sane and to the world
// bounds. It keeps a 2:1 width:height shape to match the map.
func (w *World) caveRadii(n int) (rx, ry int) {
	const tilesPerColonist = 4
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
	out := make([]Point, 0, w.countTerrain(Floor))
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			p := Point{x, y}
			if w.Walkable(p) && !w.occupied(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// randomTile reservoir-samples one tile satisfying pred in a single pass. Unlike
// rejection sampling it always finds a match if one exists, and is uniform.
func (w *World) randomTile(pred func(Point) bool) (Point, bool) {
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

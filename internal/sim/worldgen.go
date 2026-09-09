package sim

// generate carves the starting situation into a fresh all-Rock world: a central
// landing cavern with the colonists inside it, and a handful of aliens lurking
// out in the surrounding rock, ready to burrow in.
func generate(w *World) {
	center := Point{w.Width / 2, w.Height / 2}

	// Carve an oval starting cavern so colonists have room to move and a
	// frontier of Rock to mine.
	rx, ry := 6, 4
	for y := -ry; y <= ry; y++ {
		for x := -rx; x <= rx; x++ {
			// Points inside the ellipse become Floor.
			if float64(x*x)/float64(rx*rx)+float64(y*y)/float64(ry*ry) <= 1.0 {
				w.SetTerrain(center.Add(x, y), Floor)
			}
		}
	}

	// Place colonists on open floor near the center.
	for i := 0; i < w.cfg.StartColonists; i++ {
		if p, ok := w.randomFloorNear(center, 40); ok {
			w.spawn(Colonist, p)
		}
	}

	// Place aliens somewhere out in the rock, away from the cavern, so they
	// have to burrow in to reach anyone.
	for i := 0; i < w.cfg.StartAliens; i++ {
		if p, ok := w.randomRockFar(center, rx+ry+4); ok {
			w.spawn(Alien, p)
		}
	}

	w.log.add("The colony ship settles onto the Martian crust. Something below stirs.")
	w.refreshSpatial()
}

// randomFloorNear finds a random walkable tile within radius of origin. It gives
// up after a bounded number of tries and reports ok=false.
func (w *World) randomFloorNear(origin Point, radius int) (Point, bool) {
	for try := 0; try < 200; try++ {
		p := origin.Add(w.rng.Intn(2*radius+1)-radius, w.rng.Intn(2*radius+1)-radius)
		if w.Walkable(p) && !w.occupied(p) {
			return p, true
		}
	}
	return Point{}, false
}

// randomRockFar finds a random Rock tile at least minDist from origin.
func (w *World) randomRockFar(origin Point, minDist int) (Point, bool) {
	for try := 0; try < 400; try++ {
		p := Point{w.rng.Intn(w.Width), w.rng.Intn(w.Height)}
		if w.TerrainAt(p) == Rock && origin.Chebyshev(p) >= minDist && !w.occupied(p) {
			return p, true
		}
	}
	return Point{}, false
}

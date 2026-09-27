package sim

import "math"

// generate carves the starting situation into a fresh all-Rock world: a central
// landing cavern sized to the starting population, with the colonists inside it,
// and a handful of aliens lurking in the hidden caverns beyond it.
func generate(w *World) {
	center := Point{w.Width / 2, w.Height / 2}

	// Lay down ore veins and hidden natural caverns chunk by chunk. What a
	// chunk holds is a pure function of the config and its coordinates, on
	// worldgen's own streams, so it shifts nothing on the simulation stream.
	// See docs/worldgen-chunks.md.
	w.gen = newWorldGen(w.cfg)
	var caves []Point
	for cy := 0; cy < w.gen.chunkRows(); cy++ {
		for cx := 0; cx < w.gen.chunkCols(); cx++ {
			caves = append(caves, w.applyChunk(cx, cy)...)
		}
	}
	// Every chunk exists now, so no plan will be asked for again.
	w.gen.forget()

	// Carve an oval starting cavern large enough to hold the colonists with room
	// to move and a rock frontier to mine. Natural caverns keep
	// cavernLandingClearance tiles of rock from its bounding box.
	rx, ry := caveRadii(w.Width, w.Height, w.cfg.StartColonists)
	for y := -ry; y <= ry; y++ {
		for x := -rx; x <= rx; x++ {
			if x*x*ry*ry+y*y*rx*rx <= rx*rx*ry*ry {
				w.SetTerrain(center.Add(x, y), Floor)
			}
		}
	}

	// Place colonists, then mice and cats, by drawing from one shuffled list of
	// open floor tiles, so every placement is a uniform draw without replacement
	// rather than rejection sampling, which could give up. Every discovered
	// Floor tile at this point lies within the cavern just carved (natural
	// caverns are all still hidden), so it is enough to collect
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
	colonists := make([]*Entity, 0, want)
	for i := 0; i < want; i++ {
		p, _ := takeFloor()
		colonists = append(colonists, w.spawn(Colonist, p))
	}
	equipColonyShip(colonists, w.cfg)

	// Place aliens in the hidden caverns, where they lie dormant until the
	// colony digs in (see alienSpawnSite and docs/caverns.md).
	minDist := rx + ry + 4
	for i := 0; i < w.cfg.StartAliens; i++ {
		if p, ok := w.alienSpawnSite(center, minDist); ok {
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

	// Alien nests are not placed now: each cavern rolls for one when the
	// colony breaks into it (see rollNests).
	w.trackCavernsForNests(caves)

	w.log.add("The colony ship settles onto the Martian crust. Something below stirs.")
	w.refreshSpatial()
}

// veinNeighbors are the four orthogonal steps: veins and passages grow along
// them, so a deposit or tunnel is connected edge to edge, not by corners.
var veinNeighbors = [...]Point{
	{0, -1},
	{1, 0},
	{0, 1},
	{-1, 0},
}

// applyChunk writes one generated chunk into the tile grid and returns the
// centers of the natural caverns it owns. Composition and hidden cavern floor
// are written directly rather than through SetTerrain: nothing
// colony-facing can see undiscovered floor (see docs/caverns.md), so the
// TileChanged events carveHidden used to fire only did work for tiles nobody
// could reach. The counts, the published pages and the region chunks are
// kept in step here instead.
func (w *World) applyChunk(cx, cy int) []Point {
	c := w.gen.chunk(cx, cy)
	x0, y0 := cx*genChunkSize, cy*genChunkSize
	x1, y1 := min(x0+genChunkSize, w.Width), min(y0+genChunkSize, w.Height)
	page := w.tiles.pageAtAlloc(x0, y0) // one chunk is exactly one page
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			off := offset(x, y)
			cell := &page[off]
			cell.Composition = c.comp[off]
			// Hidden floor goes only on rock nobody has seen: a tile already
			// revealed (or changed) keeps what the colony knows of it, which is
			// what keeps "unexplored and not Rock" meaning undiscovered cave.
			if !c.isFloor(off) || cell.Terrain != Rock || cell.Explored {
				continue
			}
			if applyChunkViaCarveHidden {
				w.carveHidden(Point{x, y})
				continue
			}
			cell.Terrain = Floor
			w.terrainCounts[Rock]--
			w.terrainCounts[Floor]++
			w.hiddenFloor++
			w.dirtyChunks[w.chunkIndexOf(Point{x, y})] = struct{}{}
		}
	}
	w.markTilePageDirty(Point{x0, y0})
	return c.caverns
}

// applyChunkViaCarveHidden makes applyChunk carve hidden floor through
// carveHidden, firing TileChanged like the old generator did. Only
// TestChunkApplyNeedsNoTileEvents sets it, to prove the direct write is
// equivalent.
var applyChunkViaCarveHidden = false

// caveRadii returns the ellipse radii for a starting cavern big enough to hold n
// colonists with breathing room, clamped to something sane and to the world
// bounds. It keeps a 2:1 width:height shape to match the map.
func caveRadii(width, height, n int) (rx, ry int) {
	const tilesPerColonist = 10
	// area = pi * rx * ry, with rx = 2*ry  =>  ry = sqrt(area / (2*pi)).
	area := float64(n * tilesPerColonist)
	ry = int(math.Ceil(math.Sqrt(area / (2 * math.Pi))))
	if ry < 4 {
		ry = 4
	}
	rx = 2 * ry
	if maxRx := width/2 - 2; rx > maxRx {
		rx = maxRx
	}
	if maxRy := height/2 - 2; ry > maxRy {
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

// freeFloorTiles returns every walkable, unoccupied tile the colony has
// discovered — undiscovered natural caverns are no place to put anyone. Used
// for bulk placement where we need a guaranteed, uniform draw.
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
	out := make([]Point, 0, min(w.countTerrain(Floor)-w.hiddenFloor, (x1-x0+1)*(y1-y0+1)))
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			p := Point{x, y}
			if w.Walkable(p) && w.discovered(p) && !w.occupied(p) {
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
// non-tiny fraction of the map, e.g. alienSpawnSite's "any hidden cave
// floor," a few percent of the map by default — and falls back to a full-grid reservoir scan, which always finds a
// match if one exists, only if that fails (as it will for a predicate matching
// only a sliver of a huge map, such as "any free Floor tile" once the colony
// has mined out just a small fraction of it).

func (w *World) randomTile(pred func(Point) bool) (Point, bool) {
	for i := 0; i < randomTileRejectionAttempts; i++ {
		p := Point{w.rng.IntN(w.Width), w.rng.IntN(w.Height)}
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
			if w.rng.IntN(k) == 0 {
				chosen, found = p, true
			}
		}
	}
	return chosen, found
}

// randomFloor returns a random open, unoccupied floor tile the colony has
// discovered: newcomers and mouse plagues arrive in the colony, not in a
// natural cavern nobody has found.
func (w *World) randomFloor() (Point, bool) {
	return w.randomTile(func(p Point) bool { return w.Walkable(p) && w.discovered(p) && !w.occupied(p) })
}

// alienSpawnSite picks where a new alien appears. Aliens walk only on floor,
// so rock is out. The first choice is free floor in a natural cavern the
// colony has not found, where the alien lies dormant until a dig breaks in.
// With no free cave floor left (or caves turned off) it falls back to free
// discovered floor at least minDist from origin, and failing that to the
// free discovered floor farthest from origin.
func (w *World) alienSpawnSite(origin Point, minDist int) (Point, bool) {
	if w.hiddenFloor > 0 {
		if p, ok := w.randomTile(func(p Point) bool {
			return w.Walkable(p) && !w.discovered(p) && !w.occupied(p)
		}); ok {
			return p, true
		}
	}
	free := func(p Point) bool { return w.Walkable(p) && w.discovered(p) && !w.occupied(p) }
	if p, ok := w.randomTile(func(p Point) bool { return free(p) && origin.Chebyshev(p) >= minDist }); ok {
		return p, true
	}
	var best Point
	bestDist := -1
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if p := (Point{x, y}); free(p) {
				if d := origin.Chebyshev(p); d > bestDist {
					best, bestDist = p, d
				}
			}
		}
	}
	return best, bestDist >= 0
}

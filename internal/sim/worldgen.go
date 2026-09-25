package sim

import (
	"math"
	"math/rand"
)

// generate carves the starting situation into a fresh all-Rock world: a central
// landing cavern sized to the starting population, with the colonists inside it,
// and a handful of aliens lurking in the hidden caverns beyond it.
func generate(w *World) {
	center := Point{w.Width / 2, w.Height / 2}

	// Grow useful deposits into connected veins before carving. Use a dedicated
	// seed-derived stream: composition affects gameplay, but generating it must not
	// shift entity placement and every later decision on the main simulation
	// stream. Composition does not affect terrain indexes, so initializing the
	// dense tile data directly also avoids emitting TileChanged events.
	compositionRNG := rand.New(rand.NewSource(w.cfg.Seed ^ 0x243F6A8885A308D3))
	w.growRockVeins(compositionRNG, IronBearingRock, w.cfg.IronRockPercent)
	w.growRockVeins(compositionRNG, WaterIceBearingRock, w.cfg.IceRockPercent)
	// Uranium follows the older iron and ice compositions, preserving their
	// veins for established seeds.
	w.growRockVeins(compositionRNG, UraniumBearingRock, w.cfg.UraniumRockPercent)
	// Clay goes after all existing compositions so introducing it does not move
	// their veins for an established seed.
	w.growRockVeins(compositionRNG, ClayBearingRock, w.cfg.ClayRockPercent)

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

	// Hollow natural caverns out of the rest of the rock, hidden until the
	// colony digs into one. Their own stream, like the veins', so tuning
	// caverns does not reshuffle everything else about a seed — though aliens
	// still land on whatever rock is left. See docs/caverns.md.
	cavernRNG := rand.New(rand.NewSource(w.cfg.Seed ^ 0x13198A2E03707344))
	caves := w.generateCaverns(cavernRNG, center.Add(-rx, -ry), center.Add(rx, ry))

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

var veinNeighbors = [...]Point{
	{0, -1},
	{1, 0},
	{0, 1},
	{-1, 0},
}

// growRockVeins fills percent of the map with composition, grouped into
// orthogonally connected veins. Each vein takes a meandering random walk from
// one seed, occasionally branching from an earlier tile, producing long,
// irregular deposits instead of independent per-tile noise. Earlier
// compositions are never overwritten.
func (w *World) growRockVeins(rng *rand.Rand, composition RockComposition, percent int) {
	target := len(w.tiles) * percent / 100
	placed := 0
	for placed < target {
		remaining := target - placed
		size := w.nextVeinSize(rng, remaining)
		seed, ok := w.ordinaryRockSeed(rng)
		if !ok {
			return
		}

		w.tiles[seed].Composition = composition
		placed++
		vein := []int{seed}
		current := seed
		for len(vein) < size {
			neighbors := w.ordinaryNeighbors(current)
			// A vein mostly advances from its tip. Occasionally branch from an
			// earlier point; also do so whenever the current tip is boxed in.
			if len(neighbors) == 0 || (len(vein) > 2 && rng.Intn(6) == 0) {
				current, neighbors = w.branchableVeinTile(rng, vein)
				if len(neighbors) == 0 {
					break
				}
			}
			next := neighbors[rng.Intn(len(neighbors))]
			w.tiles[next].Composition = composition
			placed++
			vein = append(vein, next)
			current = next
		}
	}
}

// nextVeinSize chooses a configured vein size without leaving a final fragment
// smaller than RockVeinMin when the target has enough tiles to avoid one.
func (w *World) nextVeinSize(rng *rand.Rand, remaining int) int {
	if remaining <= w.cfg.RockVeinMax {
		return remaining
	}
	size := w.cfg.RockVeinMin + rng.Intn(w.cfg.RockVeinMax-w.cfg.RockVeinMin+1)
	if remaining-size < w.cfg.RockVeinMin {
		return remaining - w.cfg.RockVeinMin
	}
	return size
}

// ordinaryRockSeed deterministically chooses an unassigned tile, if one remains.
func (w *World) ordinaryRockSeed(rng *rand.Rand) (int, bool) {
	if len(w.tiles) == 0 {
		return 0, false
	}
	start := rng.Intn(len(w.tiles))
	for offset := 0; offset < len(w.tiles); offset++ {
		i := (start + offset) % len(w.tiles)
		if w.tiles[i].Composition == OrdinaryRock {
			return i, true
		}
	}
	return 0, false
}

func (w *World) ordinaryNeighbors(index int) []int {
	p := Point{X: index % w.Width, Y: index / w.Width}
	out := make([]int, 0, len(veinNeighbors))
	for _, d := range veinNeighbors {
		n := p.Add(d.X, d.Y)
		if !w.InBounds(n) {
			continue
		}
		i := w.index(n)
		if w.tiles[i].Composition == OrdinaryRock {
			out = append(out, i)
		}
	}
	return out
}

// branchableVeinTile picks an existing point that can still grow. Starting at a
// random offset prevents the earliest point from becoming the preferred hub.
func (w *World) branchableVeinTile(rng *rand.Rand, vein []int) (int, []int) {
	start := rng.Intn(len(vein))
	for offset := range vein {
		i := vein[(start+offset)%len(vein)]
		if neighbors := w.ordinaryNeighbors(i); len(neighbors) > 0 {
			return i, neighbors
		}
	}
	return 0, nil
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

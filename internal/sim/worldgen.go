package sim

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

// generate carves the starting situation into a fresh all-Rock world: a central
// landing cavern sized to the starting population, with the colonists inside it,
// and a handful of aliens lurking in the hidden caverns beyond it.
func generate(w *World) {
	center := Point{w.Width / 2, w.Height / 2}

	// Ore veins and hidden natural caverns are generated chunk by chunk,
	// lazily: carving the landing site generates the chunks under it, and
	// revealing tiles keeps generation WorldgenHalo chunks ahead of whatever
	// the colony has seen (see generateChunkAt). What a chunk holds is a pure
	// function of the config and its coordinates, on worldgen's own streams.
	// See docs/worldgen-chunks.md.
	w.gen = newWorldGen(w.cfg)
	w.genDone = make([]bool, len(w.tiles.pages))
	w.genSeen = make([]bool, len(w.tiles.pages))
	// Alien nests are not placed at generation: each cavern rolls for one
	// when the colony breaks into it (see rollNests). Chunks register their
	// caverns' centers as they are generated.
	w.trackCavernsForNests()

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

// generateChunkAt generates the chunk holding the in-bounds p, if it has not
// been already. Everything that reads or writes a tile the colony can reach
// calls it first (setTerrain, reveal, carveHidden), so no tile is used before
// its chunk exists. A world built without generate has no generator and
// treats every chunk as plain rock.
func (w *World) generateChunkAt(p Point) {
	if w.gen == nil || w.genDone[w.tiles.pageIndex(p.X, p.Y)] {
		return
	}
	w.generateChunk(p.X>>genChunkBits, p.Y>>genChunkBits)
}

// generateAround generates every chunk within WorldgenHalo of the one holding
// p. The order does not matter: each chunk's content is a pure function of its
// coordinates, and what generating one does to the World (counts, dirty
// pages and region chunks, cavern centers, genChunks) commutes.
func (w *World) generateAround(p Point) {
	h := max(1, w.cfg.WorldgenHalo)
	cx, cy := p.X>>genChunkBits, p.Y>>genChunkBits
	for y := max(0, cy-h); y <= min(w.gen.chunkRows()-1, cy+h); y++ {
		for x := max(0, cx-h); x <= min(w.gen.chunkCols()-1, cx+h); x++ {
			w.generateChunk(x, y)
		}
	}
}

// generateChunk generates chunk (cx, cy) unless it already exists.
func (w *World) generateChunk(cx, cy int) {
	pi := w.tiles.pageIndex(cx<<genChunkBits, cy<<genChunkBits)
	if w.genDone[pi] {
		return
	}
	w.genDone[pi] = true
	for _, c := range w.applyChunk(cx, cy) {
		w.unfoundCaverns[c] = struct{}{}
	}
	k := chunkKey{int32(cx), int32(cy)}
	i, _ := slices.BinarySearchFunc(w.genChunks, k, cmpChunkKey)
	w.genChunks = slices.Insert(w.genChunks, i, k)
}

// cmpChunkKey orders chunks by row, then column.
func cmpChunkKey(a, b chunkKey) int {
	if a.cy != b.cy {
		return cmp.Compare(a.cy, b.cy)
	}
	return cmp.Compare(a.cx, b.cx)
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
	// One chunk is exactly one page, and nothing may write a tile before its
	// chunk is generated (see generateChunkAt): a page that already exists
	// here would hold writes this is about to overwrite.
	if w.tiles.pageAt(x0, y0) != nil {
		panic(fmt.Sprintf("worldgen: chunk (%d, %d) was written before it was generated", cx, cy))
	}
	page := w.tiles.pageAtAlloc(x0, y0)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			off := offset(x, y)
			cell := &page[off]
			cell.Composition = c.comp[off]
			if !c.isFloor(off) {
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
// non-tiny fraction of the candidate tiles, e.g. alienSpawnSite's "any hidden
// cave floor," a few percent of them by default — and falls back to a
// reservoir scan, which always finds a match if one exists, only if that
// fails (as it will for a predicate matching only a sliver of a huge map,
// such as "any free Floor tile" once the colony has mined out just a small
// fraction of it).
//
// In a generated world the candidates are the tiles of generated chunks only.
// Every caller's predicate needs a tile that is not Rock, and an ungenerated
// chunk is all Rock, so this changes nothing about which tiles can be picked
// or how likely each is. It stops a huge map from spending thousands of
// guesses, and then a full-map scan, on chunks that do not exist.
func (w *World) randomTile(pred func(Point) bool) (Point, bool) {
	if w.gen != nil {
		return w.randomGeneratedTile(pred)
	}
	for i := 0; i < randomTileRejectionAttempts; i++ {
		p := Point{w.rng.IntN(w.Width), w.rng.IntN(w.Height)}
		if pred(p) {
			return p, true
		}
	}
	return w.randomTileScan(pred)
}

// randomGeneratedTile is randomTile over the generated chunks: a uniform chunk
// from genChunks, then a uniform tile in it. Guesses past the map's edge are
// misses, which keeps the draw uniform over tiles even though edge chunks are
// partial.
func (w *World) randomGeneratedTile(pred func(Point) bool) (Point, bool) {
	if len(w.genChunks) == 0 {
		return Point{}, false
	}
	for i := 0; i < randomTileRejectionAttempts; i++ {
		k := w.genChunks[w.rng.IntN(len(w.genChunks))]
		p := Point{int(k.cx)<<genChunkBits + w.rng.IntN(genChunkSize), int(k.cy)<<genChunkBits + w.rng.IntN(genChunkSize)}
		if w.InBounds(p) && pred(p) {
			return p, true
		}
	}
	var chosen Point
	found := false
	n := 0
	w.forGeneratedTiles(func(p Point) {
		if !pred(p) {
			return
		}
		n++
		if w.rng.IntN(n) == 0 {
			chosen, found = p, true
		}
	})
	return chosen, found
}

// forGeneratedTiles calls fn for every in-bounds tile of every generated
// chunk, chunk by chunk in genChunks order and row-major within each.
func (w *World) forGeneratedTiles(fn func(Point)) {
	for _, k := range w.genChunks {
		x0, y0 := int(k.cx)<<genChunkBits, int(k.cy)<<genChunkBits
		for y := y0; y < min(y0+genChunkSize, w.Height); y++ {
			for x := x0; x < min(x0+genChunkSize, w.Width); x++ {
				fn(Point{x, y})
			}
		}
	}
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
	// Farthest free floor: ties go to the lowest row, then column, whether
	// the scan covers the whole map or only the generated chunks (where all
	// floor is).
	var best Point
	bestDist := -1
	consider := func(p Point) {
		if !free(p) {
			return
		}
		if d := origin.Chebyshev(p); d > bestDist || (d == bestDist && (p.Y < best.Y || (p.Y == best.Y && p.X < best.X))) {
			best, bestDist = p, d
		}
	}
	if w.gen != nil {
		w.forGeneratedTiles(consider)
	} else {
		for y := 0; y < w.Height; y++ {
			for x := 0; x < w.Width; x++ {
				consider(Point{x, y})
			}
		}
	}
	return best, bestDist >= 0
}

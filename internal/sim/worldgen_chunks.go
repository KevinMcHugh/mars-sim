package sim

import (
	"math/rand/v2"
	"slices"
)

// Chunked world generation. The map is divided into genChunkSize-square
// chunks, and what a chunk holds (its ore veins and its hidden caverns and
// passages) is a pure function of (Config, cx, cy): it does not depend on
// which other chunks exist, or on the order anything was generated in. That
// is what lets a chunk be generated only when the simulation first needs it,
// and lets a frontend preview one without generating it. See
// docs/worldgen-chunks.md.
//
// Features may cross chunk boundaries. Each one is owned by the chunk its
// origin lies in, is rolled from that chunk's own stream, and has a bounded
// reach, so it can only touch nearby chunks. Generating a chunk therefore
// works in two steps:
//
//   - plan: roll a chunk's features from its own stream. A plan reads no
//     terrain and no other plan's tiles except where noted below, so plans
//     never cascade across the map;
//   - chunk: collect every planned feature from nearby chunks that reaches
//     this one, and keep the tiles that fall inside it.
//
// Plans are cached, but only as a cache: dropping one and recomputing it gives
// the same answer, so the cache size never changes a world.

const (
	genChunkBits = gridPageBits
	genChunkSize = 1 << genChunkBits // 64: one chunk is one pagedGrid page
	genChunkArea = genChunkSize * genChunkSize

	// veinReach bounds how far (Chebyshev) any tile of a vein lies from the
	// tile it grew from. A vein that fills its box stops short.
	veinReach = 16
	// cavernReach bounds how far any tile of a natural cavern lies from its
	// center. Lobes that would reach past it are clipped.
	cavernReach = 20
	// passageSlack is how far a passage may wander outside the box spanned
	// by the two cavern centers it joins.
	passageSlack = 12
	// scumRunMin and scumRunMax bound one run of cave scum patches: a short
	// meandering walk, so a run reaches at most scumRunMax-1 tiles from where
	// it started.
	scumRunMin, scumRunMax = 3, 8
	// veinOriginTries is how many sites a vein gets before it is skipped:
	// each must be ordinary rock with room to grow.
	veinOriginTries = 8
	// cavernSiteFailures is how many sites touching the landing box a chunk
	// may re-roll before it stops planning caverns short of its budget. Only
	// bites on a chunk that is mostly landing site.
	cavernSiteFailures = 16
	// genHorizon is how many chunks out from a chunk its plans reach (see
	// docs/worldgen-chunks.md, "planning horizon").
	genHorizon = 4
)

// A worldgen chunk is exactly one page of every pagedGrid, so a generated
// chunk and an allocated page are the same thing. Fails to compile if they
// diverge.
var _ = [1]struct{}{}[genChunkSize-gridPageSide]

// The reaches must keep every feature within one chunk of its owner, which is
// what the neighbourhood radii below assume. Fail to compile otherwise.
var (
	_ = [1]struct{}{}[(2*veinReach)/genChunkSize]
	_ = [1]struct{}{}[(2*cavernReach+cavernSpacing)/genChunkSize]
	_ = [1]struct{}{}[(passageMaxSpan+passageSlack)/genChunkSize]
	_ = [1]struct{}{}[scumRunMax/genChunkSize]
	// A salt run avoids scum from the chunks around its owner. A scum tile
	// that could touch a salt tile comes from a run starting within another
	// scumRunMax of the salt run's own reach, which must stay one chunk.
	_ = [1]struct{}{}[(2*scumRunMax)/genChunkSize]
)

// Stream identifiers, mixed into every feature's seed so the streams stay
// independent of each other and of the world-level streams.
const (
	genStreamVein    uint64 = 0x243F6A8885A308D3 // + composition level
	genStreamCavern  uint64 = 0x13198A2E03707344
	genStreamPassage uint64 = 0xA4093822299F31D0
	genStreamScum    uint64 = 0x5CA1AB1E
	genStreamSalt    uint64 = 0x5A17D0C5
)

// veinLevels lists the compositions in priority order. A vein avoids every
// tile of the compositions before its own, which is what keeps different
// compositions from overwriting (and so breaking up) each other's veins.
// New compositions go at the end, so adding one moves no existing vein.
var veinLevels = [...]RockComposition{IronBearingRock, WaterIceBearingRock, UraniumBearingRock, ClayBearingRock}

func veinPercent(cfg *Config, level int) int {
	switch veinLevels[level] {
	case IronBearingRock:
		return cfg.IronRockPercent
	case WaterIceBearingRock:
		return cfg.IceRockPercent
	case UraniumBearingRock:
		return cfg.UraniumRockPercent
	case ClayBearingRock:
		return cfg.ClayRockPercent
	}
	return 0
}

// chunkKey names a worldgen chunk.
type chunkKey struct{ cx, cy int32 }

// caveID names a planned cavern: its owner chunk and its index there.
type caveID struct {
	owner chunkKey
	idx   int32
}

// nearestRef is a cached nearestCavern answer; c is nil for "none in range".
type nearestRef struct{ c *genCavern }

// genCavern is one planned natural cavern.
type genCavern struct {
	owner  chunkKey
	idx    int32  // index within its owner's candidates
	key    uint64 // random priority: the lower key wins a conflict
	center Point
	tiles  []Point
	lo, hi Point // bounding box of tiles
}

// is reports whether o is the same cavern as c. Plans can be evicted from the
// cache and recomputed, so the same cavern can live at two addresses: compare
// identity, never pointers.
func (c *genCavern) is(o *genCavern) bool { return c.owner == o.owner && c.idx == o.idx }

// before is the total order caverns are ranked in: random key, then identity.
func (c *genCavern) before(o *genCavern) bool {
	if c.key != o.key {
		return c.key < o.key
	}
	if c.owner.cy != o.owner.cy {
		return c.owner.cy < o.owner.cy
	}
	if c.owner.cx != o.owner.cx {
		return c.owner.cx < o.owner.cx
	}
	return c.idx < o.idx
}

// chunkContent is what generation puts in one chunk, indexed by the tile's
// offset within it ((y&63)<<6 | x&63). Tiles outside the map are left zero.
type chunkContent struct {
	comp  [genChunkArea]RockComposition
	floor [genChunkArea / 64]uint64 // bit set: hidden cavern floor
	scum  [genChunkArea / 64]uint64 // bit set: a full patch of cave scum
	salt  [genChunkArea / 64]uint64 // bit set: a deposit of salt (never also scum)
	// caverns are the centers of the natural caverns this chunk owns, for
	// nest tracking.
	caverns []Point
}

func (c *chunkContent) isFloor(off int) bool { return c.floor[off>>6]&(1<<(off&63)) != 0 }
func (c *chunkContent) setFloor(off int)     { c.floor[off>>6] |= 1 << (off & 63) }
func (c *chunkContent) isScum(off int) bool  { return c.scum[off>>6]&(1<<(off&63)) != 0 }
func (c *chunkContent) setScum(off int)      { c.scum[off>>6] |= 1 << (off & 63) }
func (c *chunkContent) isSalt(off int) bool  { return c.salt[off>>6]&(1<<(off&63)) != 0 }
func (c *chunkContent) setSalt(off int)      { c.salt[off>>6] |= 1 << (off & 63) }

// worldGen generates chunks for one world. It holds only config and caches,
// so a frontend can own a second one to preview chunks without touching the
// World. It is not safe for concurrent use.
type worldGen struct {
	cfg           Config
	width, height int
	// level is the level this generator lays down. Every Point it makes is
	// on it, and it mixes into every feature's stream (see featureRand), so
	// each level holds its own rock while the landing level holds exactly
	// what it did before there were levels.
	level Level
	// landingLo/landingHi bound the landing cavern expanded by
	// cavernLandingClearance: no cavern or passage may touch this box.
	landingLo, landingHi Point

	veins    [len(veinLevels)]genCache[chunkKey, []Point]
	cands    genCache[chunkKey, []*genCavern]
	kept     genCache[chunkKey, []*genCavern]
	passages genCache[chunkKey, [][]Point]
	scum     genCache[chunkKey, []Point]
	salt     genCache[chunkKey, []Point]
	nearest  genCache[caveID, nearestRef]
	// cacheSize is each plan cache's capacity (see newWorldGenLanding).
	cacheSize int

	// Scratch, reused between plans.
	mark []bool // window bitmap for vein avoidance / cavern dilation
	own  []bool // a vein's or cavern's own tiles, over its reach box
	// runPlaced and runAvoid are runPlan's bitmaps over a runWindow: the
	// tiles a plan has placed, and (for salt) the scum it must step around.
	runPlaced []bool
	runAvoid  []bool
	nbrs      []Point
	nearBuf   []*genCavern
}

// newWorldGen returns the generator for one level. Only the landing level
// keeps caverns clear of the landing site; deeper levels have no landing site,
// and their exclusion box is empty.
func newWorldGen(cfg Config, level Level) *worldGen {
	if level != LandingLevel {
		return newWorldGenLanding(cfg, level, Point{1, 1, level}, Point{0, 0, level})
	}
	center := Point{cfg.Width / 2, cfg.Height / 2, level}
	rx, ry := caveRadii(cfg.Width, cfg.Height, cfg.StartColonists, shipTilesPerColonist(cfg))
	lo := center.Add(-rx-cavernLandingClearance, -ry-cavernLandingClearance)
	hi := center.Add(rx+cavernLandingClearance, ry+cavernLandingClearance)
	return newWorldGenLanding(cfg, level, lo, hi)
}

// newWorldGenLanding is newWorldGen with an explicit exclusion box, for tests
// that want caverns without a landing site in the way.
func newWorldGenLanding(cfg Config, level Level, lo, hi Point) *worldGen {
	// A deeper level generates from its depth-scaled settings (see
	// depthConfig); the landing level's are its own.
	g := &worldGen{cfg: depthConfig(cfg, level), width: cfg.Width, height: cfg.Height, level: level, landingLo: lo, landingHi: hi}
	// Generating the halo around one newly seen chunk plans a square of
	// 2*(halo+horizon)+1 chunks. Keep two of those per kind of plan, so
	// digging along an edge reuses the plans its last step made, and no more:
	// the cache lives as long as the game, and every entry is a few KB.
	side := 2*(max(1, cfg.WorldgenHalo)+genHorizon) + 1
	g.cacheSize = 2 * side * side
	g.forget()
	return g
}

// withCacheSize resizes the plan caches, for a caller that generates far more
// chunks at once than one halo (a test sweeping a whole map in raster order
// needs about 2*genHorizon+1 rows of chunks). It drops what is cached.
func (g *worldGen) withCacheSize(n int) *worldGen {
	g.cacheSize = n
	g.forget()
	return g
}

// forget drops every cached plan. Plans are pure, so this changes nothing but
// memory and the time to recompute one.
func (g *worldGen) forget() {
	for i := range g.veins {
		g.veins[i] = newGenCache[chunkKey, []Point](g.cacheSize)
	}
	g.cands = newGenCache[chunkKey, []*genCavern](g.cacheSize)
	g.kept = newGenCache[chunkKey, []*genCavern](g.cacheSize)
	g.passages = newGenCache[chunkKey, [][]Point](g.cacheSize)
	g.scum = newGenCache[chunkKey, []Point](g.cacheSize)
	g.salt = newGenCache[chunkKey, []Point](g.cacheSize)
	g.nearest = newGenCache[caveID, nearestRef](g.cacheSize * 8)
}

func (g *worldGen) chunkCols() int { return ceilDiv(g.width, genChunkSize) }
func (g *worldGen) chunkRows() int { return ceilDiv(g.height, genChunkSize) }

// chunkBounds returns the chunk's tiles that lie in the map, as an inclusive
// box, and false if none do.
func (g *worldGen) chunkBounds(k chunkKey) (lo, hi Point, ok bool) {
	x0, y0 := int(k.cx)*genChunkSize, int(k.cy)*genChunkSize
	x1, y1 := min(x0+genChunkSize, g.width)-1, min(y0+genChunkSize, g.height)-1
	if k.cx < 0 || k.cy < 0 || x0 >= g.width || y0 >= g.height {
		return Point{}, Point{}, false
	}
	return Point{x0, y0, g.level}, Point{x1, y1, g.level}, true
}

func (g *worldGen) inMap(p Point) bool {
	return p.X >= 0 && p.X < g.width && p.Y >= 0 && p.Y < g.height
}

func (g *worldGen) nearLanding(p Point) bool {
	return p.X >= g.landingLo.X && p.X <= g.landingHi.X && p.Y >= g.landingLo.Y && p.Y <= g.landingHi.Y
}

// featureRand returns the stream for one feature: the world seed and a stream
// constant, mixed with the coordinates that identify the feature. Each value
// goes through splitmix64, so neighbouring chunks get unrelated streams.
//
// A level other than the landing level mixes in its distance from it first.
// The landing level mixes in nothing, so it draws exactly the streams it did
// before there were levels, and every seed's landing level is unchanged.
func (g *worldGen) featureRand(stream uint64, ids ...int64) *rand.Rand {
	x := uint64(g.cfg.Seed) ^ stream
	if d := int64(g.level - LandingLevel); d != 0 {
		x ^= uint64(d) * 0x9E3779B97F4A7C15
		splitmix64(&x)
	}
	for _, v := range ids {
		x ^= uint64(v)
		splitmix64(&x)
	}
	return rand.New(rand.NewPCG(splitmix64(&x), splitmix64(&x)))
}

// stratified turns an expected count num/den into an integer: its floor, plus
// one with probability equal to the fraction left over. Unlike a Poisson draw
// this never rolls zero features where one or more are expected, which is what
// keeps a small map from missing a composition entirely.
func stratified(rng *rand.Rand, num, den int64) int {
	if den <= 0 || num <= 0 {
		return 0
	}
	n := num / den
	if rng.Int64N(den) < num%den {
		n++
	}
	return int(n)
}

// chunk generates one chunk.
func (g *worldGen) chunk(cx, cy int) *chunkContent {
	k := chunkKey{int32(cx), int32(cy)}
	lo, hi, ok := g.chunkBounds(k)
	out := &chunkContent{}
	if !ok {
		return out
	}
	in := func(p Point) (int, bool) {
		if p.X < lo.X || p.X > hi.X || p.Y < lo.Y || p.Y > hi.Y {
			return 0, false
		}
		return (p.Y&(genChunkSize-1))<<genChunkBits | p.X&(genChunkSize-1), true
	}

	// Veins: every vein that can reach this chunk has its origin within one
	// chunk of it. Levels never overlap (each avoids the ones before it),
	// but first-wins keeps that true even if a plan were ever wrong.
	for level := range veinLevels {
		comp := veinLevels[level]
		g.forNeighbours(k, 1, func(n chunkKey) {
			for _, p := range g.veinPlan(level, n) {
				if off, ok := in(p); ok && out.comp[off] == OrdinaryRock {
					out.comp[off] = comp
				}
			}
		})
	}

	// Cave scum rides on the rock, not in it: a patch can sit on what
	// becomes cavern or landing floor, as it always could.
	g.forNeighbours(k, 1, func(n chunkKey) {
		for _, p := range g.scumPlan(n) {
			if off, ok := in(p); ok {
				out.setScum(off)
			}
		}
	})

	// Salt rides on the rock too, on tiles scum does not hold: its plan
	// already steered around the scum, so no tile is ever both.
	g.forNeighbours(k, 1, func(n chunkKey) {
		for _, p := range g.saltPlan(n) {
			if off, ok := in(p); ok {
				out.setSalt(off)
			}
		}
	})

	// Caverns and passages.
	g.forNeighbours(k, 1, func(n chunkKey) {
		for _, c := range g.keptCaverns(n) {
			if c.hi.X < lo.X || c.lo.X > hi.X || c.hi.Y < lo.Y || c.lo.Y > hi.Y {
				continue
			}
			for _, p := range c.tiles {
				if off, ok := in(p); ok {
					out.setFloor(off)
				}
			}
		}
		for _, path := range g.passagePlan(n) {
			for _, p := range path {
				if off, ok := in(p); ok {
					out.setFloor(off)
				}
			}
		}
	})
	for _, c := range g.keptCaverns(k) {
		out.caverns = append(out.caverns, c.center)
	}
	return out
}

// forNeighbours calls fn for every chunk within r (Chebyshev) of k that has
// any tile in the map, in a fixed order.
func (g *worldGen) forNeighbours(k chunkKey, r int32, fn func(chunkKey)) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			n := chunkKey{k.cx + dx, k.cy + dy}
			if _, _, ok := g.chunkBounds(n); ok {
				fn(n)
			}
		}
	}
}

// --- Veins ---------------------------------------------------------------

// veinPlan returns every tile of the level's veins that grow from chunk k.
// A vein avoids the tiles of every earlier level, so planning level L reads
// the plans of levels < L within one chunk: a dependency four levels deep at
// most, and never on this level's own neighbours. Veins of the same level may
// overlap each other; they merge into one larger deposit, which keeps every
// tile of it connected to another.
func (g *worldGen) veinPlan(level int, k chunkKey) []Point {
	if v, ok := g.veins[level].get(k); ok {
		return v
	}
	var tiles []Point
	pct := veinPercent(&g.cfg, level)
	lo, hi, ok := g.chunkBounds(k)
	if ok && pct > 0 {
		tiles = g.growVeins(level, k, lo, hi, pct)
	}
	g.veins[level].put(k, tiles)
	return tiles
}

func (g *worldGen) growVeins(level int, k chunkKey, lo, hi Point, pct int) []Point {
	minSize := max(2, g.cfg.RockVeinMin) // a one-tile vein is an isolated deposit
	maxSize := max(minSize, g.cfg.RockVeinMax)

	// The window every vein of this chunk can reach, marked with the tiles of
	// earlier levels. Gather the dependencies first: they share the scratch
	// buffers this plan is about to use.
	wlo := Point{lo.X - veinReach, lo.Y - veinReach, g.level}
	ww, wh := hi.X-lo.X+1+2*veinReach, hi.Y-lo.Y+1+2*veinReach
	var earlier [][]Point
	for l := 0; l < level; l++ {
		g.forNeighbours(k, 1, func(n chunkKey) { earlier = append(earlier, g.veinPlan(l, n)) })
	}
	g.mark = resetBools(g.mark, ww*wh)
	taken := g.mark
	for _, plan := range earlier {
		for _, p := range plan {
			if x, y := p.X-wlo.X, p.Y-wlo.Y; x >= 0 && y >= 0 && x < ww && y < wh {
				taken[y*ww+x] = true
			}
		}
	}

	rng := g.featureRand(genStreamVein+uint64(level), int64(k.cx), int64(k.cy))
	area := int64(hi.X-lo.X+1) * int64(hi.Y-lo.Y+1)
	count := stratified(rng, area*int64(pct)*2, 100*int64(minSize+maxSize))

	const box = 2*veinReach + 1
	g.own = resetBools(g.own, box*box)
	var tiles []Point
	for v := 0; v < count; v++ {
		size := minSize + rng.IntN(maxSize-minSize+1)
		var origin Point
		found := false
		for try := 0; try < veinOriginTries && !found; try++ {
			origin = Point{lo.X + rng.IntN(hi.X-lo.X+1), lo.Y + rng.IntN(hi.Y-lo.Y+1), g.level}
			free := func(p Point) bool {
				x, y := p.X-wlo.X, p.Y-wlo.Y
				return g.inMap(p) && !taken[y*ww+x]
			}
			if !free(origin) {
				continue
			}
			for _, d := range veinNeighbors {
				if free(origin.Add(d.X, d.Y)) {
					found = true
					break
				}
			}
		}
		if !found {
			continue
		}
		vein := g.walkVein(rng, origin, size, func(p Point) bool {
			x, y := p.X-wlo.X, p.Y-wlo.Y
			return g.inMap(p) && !taken[y*ww+x]
		})
		// Later veins of this chunk steer around this one, so they do not
		// spend their tiles re-covering it. Veins from other chunks of the
		// same level can still overlap it; that costs a little abundance
		// (see docs/worldgen-chunks.md) but never connectivity.
		for _, p := range vein {
			taken[(p.Y-wlo.Y)*ww+(p.X-wlo.X)] = true
		}
		tiles = append(tiles, vein...)
	}
	return tiles
}

// walkVein grows one vein of up to size orthogonally connected tiles from
// origin: a meandering walk from its tip, now and then branching from an
// earlier tile, never leaving the veinReach box or stepping on a tile free
// rejects. The origin has at least one free neighbour, so the vein always has
// at least two tiles.
func (g *worldGen) walkVein(rng *rand.Rand, origin Point, size int, free func(Point) bool) []Point {
	const box = 2*veinReach + 1
	own := g.own
	slot := func(p Point) int { return (p.Y-origin.Y+veinReach)*box + (p.X - origin.X + veinReach) }
	inBox := func(p Point) bool { return origin.Chebyshev(p) <= veinReach }
	open := func(p Point) []Point {
		g.nbrs = g.nbrs[:0]
		for _, d := range veinNeighbors {
			n := p.Add(d.X, d.Y)
			if inBox(n) && !own[slot(n)] && free(n) {
				g.nbrs = append(g.nbrs, n)
			}
		}
		return g.nbrs
	}

	vein := []Point{origin}
	own[slot(origin)] = true
	current := origin
	for len(vein) < size {
		neighbors := open(current)
		// A vein mostly advances from its tip. Occasionally branch from an
		// earlier point; also do so whenever the tip is boxed in.
		if len(neighbors) == 0 || (len(vein) > 2 && rng.IntN(6) == 0) {
			neighbors = nil
			start := rng.IntN(len(vein))
			for off := range vein {
				c := vein[(start+off)%len(vein)]
				if n := open(c); len(n) > 0 {
					current, neighbors = c, n
					break
				}
			}
			if len(neighbors) == 0 {
				break
			}
		}
		next := neighbors[rng.IntN(len(neighbors))]
		own[slot(next)] = true
		vein = append(vein, next)
		current = next
	}
	for _, p := range vein {
		own[slot(p)] = false
	}
	return vein
}

// --- Cave scum ------------------------------------------------------------

// scumPlan returns the tiles of the cave scum runs that start in chunk k:
// ScumPercent of its area, in short meandering runs. It reads nothing, so
// runs from neighbouring chunks may overlap near a chunk edge; a tile is
// scummed or not, so an overlap only costs a little abundance. See
// docs/scumhouse.md.
func (g *worldGen) scumPlan(k chunkKey) []Point {
	if v, ok := g.scum.get(k); ok {
		return v
	}
	var tiles []Point
	if g.cfg.ScumPercent > 0 && g.cfg.ScumMax > 0 {
		tiles = g.runPlan(k, genStreamScum, g.cfg.ScumPercent, nil)
	}
	g.scum.put(k, tiles)
	return tiles
}

// --- Salt -------------------------------------------------------------------

// saltPlan returns the tiles of the salt runs that start in chunk k:
// SaltPercent of its area, laid down like scum but never on a tile scum
// holds. Scum is the fixed point (its plan reads nothing, and its abundance is
// what the food economy is tuned to), so salt is the one that steps around it:
// a run's walk skips a scum tile without placing there, and keeps going, so
// the chunk still lands on its share of distinct tiles. The scum it avoids is
// every scum plan of k's neighbours, which is all that can reach a salt tile
// (see the reach assertions above). See docs/salt.md.
func (g *worldGen) saltPlan(k chunkKey) []Point {
	if v, ok := g.salt.get(k); ok {
		return v
	}
	var tiles []Point
	if lo, hi, ok := g.chunkBounds(k); ok && g.cfg.SaltPercent > 0 {
		// Gather the scum plans first: they share the scratch buffers this
		// plan is about to use. Then mark the ones inside the window the salt
		// runs can reach. A bitmap, not a set: building a map of the 2,000-odd
		// scum tiles around every chunk nearly doubled whole-map generation.
		var scum [][]Point
		g.forNeighbours(k, 1, func(n chunkKey) { scum = append(scum, g.scumPlan(n)) })
		wlo, ww, wh := runWindow(lo, hi)
		g.runAvoid = resetBools(g.runAvoid, ww*wh)
		for _, plan := range scum {
			for _, p := range plan {
				if x, y := p.X-wlo.X, p.Y-wlo.Y; x >= 0 && y >= 0 && x < ww && y < wh {
					g.runAvoid[y*ww+x] = true
				}
			}
		}
		tiles = g.runPlan(k, genStreamSalt, g.cfg.SaltPercent, g.runAvoid)
	}
	g.salt.put(k, tiles)
	return tiles
}

// runReach is how far a run's tiles lie from the chunk it starts in: a run
// takes at most scumRunMax-1 steps before its last tile.
const runReach = scumRunMax - 1

// runWindow is the box every run of a chunk spanning lo..hi can reach: its
// top-left corner, width and height.
func runWindow(lo, hi Point) (wlo Point, ww, wh int) {
	return Point{lo.X - runReach, lo.Y - runReach, lo.Level}, hi.X - lo.X + 1 + 2*runReach, hi.Y - lo.Y + 1 + 2*runReach
}

// runPlan places percent of chunk k's area as distinct tiles, in short
// meandering runs of scumRunMin–scumRunMax steps, never on a tile marked in
// avoid (a runWindow bitmap, or nil). Both cave scum and salt are laid down
// this way.
func (g *worldGen) runPlan(k chunkKey, stream uint64, percent int, avoid []bool) []Point {
	lo, hi, ok := g.chunkBounds(k)
	if !ok {
		return nil
	}
	// Budget distinct tiles, as the old whole-map pass did: a short walk
	// often steps back onto itself, and counting steps instead came out
	// a fifth short. Runs keep going until the chunk's share is placed,
	// with a guard in case its map area is tiny.
	rng := g.featureRand(stream, int64(k.cx), int64(k.cy))
	area := int64(hi.X-lo.X+1) * int64(hi.Y-lo.Y+1)
	budget := stratified(rng, area*int64(percent), 100)
	if budget == 0 {
		return nil
	}
	// Which tiles this plan has placed, over the window its runs can reach:
	// a bitmap for the same reason as saltPlan's avoid.
	wlo, ww, wh := runWindow(lo, hi)
	g.runPlaced = resetBools(g.runPlaced, ww*wh)
	placed := g.runPlaced
	tiles := make([]Point, 0, budget)
	for guard := 0; len(tiles) < budget && guard < 8*budget; guard++ {
		p := Point{lo.X + rng.IntN(hi.X-lo.X+1), lo.Y + rng.IntN(hi.Y-lo.Y+1), g.level}
		for n := scumRunMin + rng.IntN(scumRunMax-scumRunMin+1); n > 0 && len(tiles) < budget; n-- {
			if g.inMap(p) {
				x, y := p.X-wlo.X, p.Y-wlo.Y
				if x < 0 || y < 0 || x >= ww || y >= wh {
					panic("worldgen: a run walked out of its window")
				}
				if i := y*ww + x; !placed[i] && (avoid == nil || !avoid[i]) {
					placed[i] = true
					tiles = append(tiles, p)
				}
			}
			d := veinNeighbors[rng.IntN(len(veinNeighbors))]
			p = p.Add(d.X, d.Y)
		}
	}
	return tiles
}

// --- Caverns --------------------------------------------------------------

// cavernCandidates plans the caverns rooted in chunk k, before any conflict
// with another chunk's caverns is resolved. It reads nothing but k's stream
// and the landing box.
func (g *worldGen) cavernCandidates(k chunkKey) []*genCavern {
	if v, ok := g.cands.get(k); ok {
		return v
	}
	var out []*genCavern
	lo, hi, ok := g.chunkBounds(k)
	if ok && g.cfg.CavernPercent > 0 {
		out = g.planCaverns(k, lo, hi)
	}
	g.cands.put(k, out)
	return out
}

func (g *worldGen) planCaverns(k chunkKey, lo, hi Point) []*genCavern {
	minSize := max(1, g.cfg.CavernMin)
	maxSize := max(minSize, g.cfg.CavernMax)
	// Centers stay off the edge margin, as the tiles do.
	m := cavernEdgeMargin + 1
	clo := Point{max(lo.X, m), max(lo.Y, m), g.level}
	chi := Point{min(hi.X, g.width-1-m), min(hi.Y, g.height-1-m), g.level}
	if clo.X > chi.X || clo.Y > chi.Y {
		return nil
	}

	// Plan caverns until their tiles reach this chunk's share of
	// CavernPercent. Budgeting tiles rather than a count keeps lobe overshoot
	// from inflating the total; the cavern that would cross the budget is kept
	// only if less than half of it lies past the line, so on average the
	// chunk lands on its share rather than over it. A site that touches the
	// landing box is re-rolled, so a small map still gets caves.
	rng := g.featureRand(genStreamCavern, int64(k.cx), int64(k.cy))
	area := int64(hi.X-lo.X+1) * int64(hi.Y-lo.Y+1)
	budget := stratified(rng, area*int64(g.cfg.CavernPercent), 100)

	const box = 2*cavernReach + 1
	g.own = resetBools(g.own, box*box)
	var out []*genCavern
	planned, failures := 0, 0
	for planned < budget && failures < cavernSiteFailures {
		c := &genCavern{owner: k, idx: int32(len(out)), key: rng.Uint64()}
		c.center = Point{clo.X + rng.IntN(chi.X-clo.X+1), clo.Y + rng.IntN(chi.Y-clo.Y+1), g.level}
		size := minSize + rng.IntN(maxSize-minSize+1)
		if !g.growCavern(rng, c, size) {
			failures++
			continue
		}
		if planned+len(c.tiles)/2 > budget {
			break
		}
		planned += len(c.tiles)
		out = append(out, c)
	}
	return out
}

// growCavern grows a cavern of about size tiles around c.center as a cluster
// of small overlapping, wider-than-tall ellipses (the map's own 2:1 shape),
// each lobe centered on a tile already in the cavern, so the cavern is one
// connected pocket. Lobes are clipped to cavernReach and the edge margin;
// clipping an ellipse's rows to a box keeps them contiguous, so the cavern
// stays connected. It reports false if the cavern would touch the landing
// box. The ellipse test is integer arithmetic so it rounds the same on every
// CPU (see docs/determinism.md on fused multiply-add).
func (g *worldGen) growCavern(rng *rand.Rand, c *genCavern, size int) bool {
	const box = 2*cavernReach + 1
	own := g.own
	slot := func(p Point) int { return (p.Y-c.center.Y+cavernReach)*box + (p.X - c.center.X + cavernReach) }
	usable := func(p Point) bool {
		return c.center.Chebyshev(p) <= cavernReach &&
			p.X >= cavernEdgeMargin && p.Y >= cavernEdgeMargin &&
			p.X < g.width-cavernEdgeMargin && p.Y < g.height-cavernEdgeMargin
	}
	ok := true
	lobe := c.center
	for lobes := 0; len(c.tiles) < size && lobes < cavernMaxLobes; lobes++ {
		ry := 1 + rng.IntN(2)
		rx := ry + 1 + rng.IntN(ry+1)
		for y := -ry; y <= ry; y++ {
			for x := -rx; x <= rx; x++ {
				if x*x*ry*ry+y*y*rx*rx > rx*rx*ry*ry {
					continue
				}
				p := lobe.Add(x, y)
				if !usable(p) || own[slot(p)] {
					continue
				}
				if g.nearLanding(p) {
					ok = false
				}
				own[slot(p)] = true
				c.tiles = append(c.tiles, p)
			}
		}
		lobe = c.tiles[rng.IntN(len(c.tiles))]
	}
	for _, p := range c.tiles {
		own[slot(p)] = false
	}
	if !ok {
		return false
	}
	c.lo, c.hi = c.tiles[0], c.tiles[0]
	for _, p := range c.tiles {
		c.lo.X, c.lo.Y = min(c.lo.X, p.X), min(c.lo.Y, p.Y)
		c.hi.X, c.hi.Y = max(c.hi.X, p.X), max(c.hi.Y, p.Y)
	}
	return true
}

// keptCaverns returns chunk k's candidates that survive conflict resolution:
// a candidate is dropped if any of its tiles is within cavernSpacing of a
// tile of any higher-ranked candidate. The rule looks at candidates, not at
// which of them survived, so it cannot chain: whether a cavern is kept
// depends only on the candidates within one chunk, never on a cascade of
// drops across the map. The cost is that a cavern dropped for crowding a
// neighbour that was itself dropped stays dropped, which slightly lowers
// density; see docs/worldgen-chunks.md.
func (g *worldGen) keptCaverns(k chunkKey) []*genCavern {
	if v, ok := g.kept.get(k); ok {
		return v
	}
	own := g.cavernCandidates(k)
	var near []*genCavern
	g.forNeighbours(k, 1, func(n chunkKey) { near = append(near, g.cavernCandidates(n)...) })
	var out []*genCavern
	for _, c := range own {
		if !g.crowded(c, near) {
			out = append(out, c)
		}
	}
	g.kept.put(k, out)
	return out
}

// crowded reports whether any higher-ranked candidate in near comes within
// cavernSpacing of c.
func (g *worldGen) crowded(c *genCavern, near []*genCavern) bool {
	// Mark c's tiles dilated by cavernSpacing over its bounding box, then
	// test each rival tile with one lookup.
	s := cavernSpacing
	wlo := Point{c.lo.X - s, c.lo.Y - s, g.level}
	ww, wh := c.hi.X-c.lo.X+1+2*s, c.hi.Y-c.lo.Y+1+2*s
	dilated := false
	for _, o := range near {
		if o.is(c) || !o.before(c) {
			continue
		}
		if o.hi.X < wlo.X || o.lo.X >= wlo.X+ww || o.hi.Y < wlo.Y || o.lo.Y >= wlo.Y+wh {
			continue
		}
		if !dilated {
			g.mark = resetBools(g.mark, ww*wh)
			for _, p := range c.tiles {
				for dy := -s; dy <= s; dy++ {
					for dx := -s; dx <= s; dx++ {
						g.mark[(p.Y+dy-wlo.Y)*ww+(p.X+dx-wlo.X)] = true
					}
				}
			}
			dilated = true
		}
		for _, p := range o.tiles {
			if x, y := p.X-wlo.X, p.Y-wlo.Y; x >= 0 && y >= 0 && x < ww && y < wh && g.mark[y*ww+x] {
				return true
			}
		}
	}
	return false
}

// --- Passages -------------------------------------------------------------

// passagePlan returns the passages owned by chunk k. Every kept cavern rolls
// once (CavernPassagePercent) for a passage to its nearest kept neighbour
// within passageMaxSpan; a pair that are each other's nearest is rolled once.
// A pair is owned by whichever of its caverns ranks first, and its roll and
// walk come from a stream seeded by the pair, so the passage is the same
// whichever chunk asks for it first.
func (g *worldGen) passagePlan(k chunkKey) [][]Point {
	if v, ok := g.passages.get(k); ok {
		return v
	}
	var out [][]Point
	if g.cfg.CavernPassagePercent > 0 {
		out = g.planPassages(k)
	}
	g.passages.put(k, out)
	return out
}

func (g *worldGen) planPassages(k chunkKey) [][]Point {
	// Both ends of a pair owned here lie within passageMaxSpan of a cavern
	// in k, so within one chunk; each end's nearest neighbour is within one
	// chunk of it.
	type pair struct{ a, b *genCavern }
	var pairs []pair
	g.forNeighbours(k, 1, func(n chunkKey) {
		for _, c := range g.keptCaverns(n) {
			o := g.nearestCavern(c)
			if o == nil {
				continue
			}
			a, b := c, o
			if b.before(a) {
				a, b = b, a
			}
			if a.owner != k {
				continue
			}
			if !slices.ContainsFunc(pairs, func(q pair) bool { return q.a.is(a) && q.b.is(b) }) {
				pairs = append(pairs, pair{a, b})
			}
		}
	})
	// Pairs are collected in chunk order, but nothing depends on the order:
	// each pair has its own stream.
	var out [][]Point
	for _, p := range pairs {
		rng := g.featureRand(genStreamPassage,
			int64(p.a.owner.cx), int64(p.a.owner.cy), int64(p.a.idx),
			int64(p.b.owner.cx), int64(p.b.owner.cy), int64(p.b.idx))
		if rng.IntN(100) >= g.cfg.CavernPassagePercent {
			continue
		}
		for attempt := 0; attempt < passageAttempts; attempt++ {
			if path, ok := g.walkPassage(rng, p.a.center, p.b.center); ok {
				out = append(out, path)
				break
			}
		}
	}
	return out
}

// nearestCavern returns the kept cavern nearest c (Chebyshev, center to
// center) within passageMaxSpan, ties going to the higher-ranked, or nil.
func (g *worldGen) nearestCavern(c *genCavern) *genCavern {
	// Every passage plan within one chunk asks about the same caverns, so
	// the answer is worth keeping.
	id := caveID{c.owner, c.idx}
	if r, ok := g.nearest.get(id); ok {
		return r.c
	}
	g.nearBuf = g.nearBuf[:0]
	g.forNeighbours(c.owner, 1, func(n chunkKey) { g.nearBuf = append(g.nearBuf, g.keptCaverns(n)...) })
	var best *genCavern
	bestDist := passageMaxSpan + 1
	for _, o := range g.nearBuf {
		if o.is(c) {
			continue
		}
		d := c.center.Chebyshev(o.center)
		if d < bestDist || (d == bestDist && best != nil && o.before(best)) {
			best, bestDist = o, d
		}
	}
	g.nearest.put(id, nearestRef{best})
	return best
}

// walkPassage random-walks a one-tile-wide tunnel from one cavern center to
// another: mostly stepping toward the goal, sometimes wandering sideways, in
// orthogonal steps so it reads as a corridor rather than a staircase. A step
// off the usable map or out of the pair's box (plus passageSlack) is skipped.
// The walk fails if it touches the landing box or runs too long.
func (g *worldGen) walkPassage(rng *rand.Rand, from, to Point) ([]Point, bool) {
	blo := Point{min(from.X, to.X) - passageSlack, min(from.Y, to.Y) - passageSlack, g.level}
	bhi := Point{max(from.X, to.X) + passageSlack, max(from.Y, to.Y) + passageSlack, g.level}
	cur := from
	limit := 4*from.Chebyshev(to) + 16
	var path []Point
	for steps := 0; cur != to; steps++ {
		if steps >= limit {
			return nil, false
		}
		dx, dy := sign(to.X-cur.X), sign(to.Y-cur.Y)
		var step gridStep
		switch {
		case rng.IntN(4) == 0:
			step = veinNeighbors[rng.IntN(len(veinNeighbors))]
		case dx != 0 && (dy == 0 || rng.IntN(2) == 0):
			step = gridStep{dx, 0}
		default:
			step = gridStep{0, dy}
		}
		next := cur.Add(step.X, step.Y)
		if next.X < cavernEdgeMargin || next.Y < cavernEdgeMargin ||
			next.X >= g.width-cavernEdgeMargin || next.Y >= g.height-cavernEdgeMargin ||
			next.X < blo.X || next.Y < blo.Y || next.X > bhi.X || next.Y > bhi.Y {
			continue
		}
		if g.nearLanding(next) {
			return nil, false
		}
		cur = next
		path = append(path, cur)
	}
	return path, true
}

// --- Plumbing -------------------------------------------------------------

// resetBools returns b with length n and every entry false, reusing its
// storage when it is big enough.
func resetBools(b []bool, n int) []bool {
	if cap(b) < n {
		return make([]bool, n)
	}
	b = b[:n]
	clear(b)
	return b
}

// genCache is a bounded memo: two generations of map, the older dropped
// whole when the newer fills. An entry used again is carried forward, so what
// generation is actively touching stays, and what it has moved past goes.
// Plans are pure, so a miss only costs the time to recompute one.
type genCache[K comparable, V any] struct {
	cur, prev map[K]V
	cap       int
}

func newGenCache[K comparable, V any](capacity int) genCache[K, V] {
	return genCache[K, V]{cur: make(map[K]V), cap: capacity}
}

func (c *genCache[K, V]) get(k K) (V, bool) {
	if v, ok := c.cur[k]; ok {
		return v, true
	}
	if v, ok := c.prev[k]; ok {
		c.put(k, v)
		return v, true
	}
	var zero V
	return zero, false
}

func (c *genCache[K, V]) put(k K, v V) {
	c.cur[k] = v
	if len(c.cur) >= c.cap {
		c.prev, c.cur = c.cur, make(map[K]V, c.cap)
	}
}

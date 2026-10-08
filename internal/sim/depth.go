package sim

import (
	"fmt"
	"slices"
)

// Depth: the deeper a level, the richer and the more dangerous it is. Every
// level below the landing level generates from depthConfig (more ore, much
// more uranium, bigger caverns), rolls nests more often and bigger, and
// weights its nests toward hostile species. Caverns the colony breaks into
// can turn out to hold a natural shaft down into a cavern below, or a
// sinkhole; and a hostile alien with nothing to hunt drops down a hole onto
// prey below. The landing level is depth 0, untouched, so a game that never
// digs down is exactly what it was. See docs/depth.md.

// depthOf is how many levels below the landing level l is (0 for the landing
// level, and for the surface).
func depthOf(l Level) int { return max(0, int(l-LandingLevel)) }

// depthConfig is cfg as level l generates and rolls nests: unchanged on the
// landing level, and scaled by the Depth* settings a level at a time below
// it. Percentages are capped at 100.
func depthConfig(cfg Config, l Level) Config {
	d := depthOf(l)
	if d == 0 {
		return cfg
	}
	scale := func(v, perLevel int) int { return v * (100 + perLevel*d) / 100 }
	pct := func(v int) int { return min(100, max(0, v)) }
	cfg.IronRockPercent = pct(scale(cfg.IronRockPercent, cfg.DepthOrePercent))
	cfg.IceRockPercent = pct(scale(cfg.IceRockPercent, cfg.DepthOrePercent))
	cfg.ClayRockPercent = pct(scale(cfg.ClayRockPercent, cfg.DepthOrePercent))
	cfg.UraniumRockPercent = pct(scale(cfg.UraniumRockPercent, cfg.DepthUraniumPercent))
	cfg.CavernMin = scale(cfg.CavernMin, cfg.DepthCavernPercent)
	cfg.CavernMax = scale(cfg.CavernMax, cfg.DepthCavernPercent)
	cfg.CavernNestPercent = pct(cfg.CavernNestPercent + cfg.DepthNestPercent*d)
	cfg.CavernNestMin += cfg.DepthNestSize * d
	cfg.CavernNestMax += cfg.DepthNestSize * d
	return cfg
}

// nestSpecies draws the species of a nest on level l from nestRNG: any
// species alike on the landing level (exactly the draw it always was), and
// below it each hostile species weighted DepthHostility percent heavier a
// level, so the deep is where the hunters live.
func (w *World) nestSpecies(l Level) int {
	d := depthOf(l)
	if d == 0 || w.cfg.DepthHostility == 0 {
		return w.nestRNG.IntN(len(w.alienSpecies))
	}
	weight := func(s AlienSpecies) int {
		if s.Temperament == TemperamentHostile {
			return 100 + w.cfg.DepthHostility*d
		}
		return 100
	}
	total := 0
	for _, s := range w.alienSpecies {
		total += weight(s)
	}
	r := w.nestRNG.IntN(total)
	for i, s := range w.alienSpecies {
		if r < weight(s) {
			return i
		}
		r -= weight(s)
	}
	return len(w.alienSpecies) - 1
}

// ---- Natural shafts and sinkholes -------------------------------------------

// Each a feature stream of the cavern's level's generator, keyed by the
// cavern's center: a pure function of the seed, like the cavern itself.
const (
	genStreamNaturalShaft uint64 = 0x5A4F7D0E
	genStreamSinkhole     uint64 = 0x51B4C0DE
)

// naturalFeatures rolls, for each cavern just discovered at centers, whether
// it holds a natural shaft down into a cavern on the level below
// (NaturalShaftPercent) and whether its floor has a sinkhole
// (SinkholePercent). Neither is rolled where the level below is deeper than
// Config allows, so a game that cannot dig down draws nothing. The rolls
// come from the level's generator (featureRand), not a stream, so they
// depend on nothing but the seed and the cavern.
func (w *World) naturalFeatures(centers []Point) {
	for _, c := range centers {
		g := w.lay(c).gen
		if g == nil || int(c.Level)+1 > w.cfg.DeepestLevel || c.Level < LandingLevel {
			continue
		}
		if g.featureRand(genStreamNaturalShaft, int64(c.X), int64(c.Y)).IntN(100) < w.cfg.NaturalShaftPercent {
			w.naturalShaft(c, g)
		}
		if g.featureRand(genStreamSinkhole, int64(c.X), int64(c.Y)).IntN(100) < w.cfg.SinkholePercent {
			w.sinkhole(c, g)
		}
	}
}

// cavernFloor lists the discovered open floor around a cavern's center that
// a feature could sit on, row-major: every tile within cavernReach whose
// eight neighbours are open floor too, so a feature never plugs a passage.
func (w *World) cavernFloor(c Point) []Point {
	var out []Point
	for y := c.Y - cavernReach; y <= c.Y+cavernReach; y++ {
		for x := c.X - cavernReach; x <= c.X+cavernReach; x++ {
			p := Point{x, y, c.Level}
			if w.TerrainAt(p) != Floor || !w.discovered(p) {
				continue
			}
			open := true
			for _, d := range neighbors8 {
				if w.TerrainAt(p.Add(d.X, d.Y)) != Floor {
					open = false
					break
				}
			}
			if open {
				out = append(out, p)
			}
		}
	}
	return out
}

// below is the terrain straight below p as it is, or as the level below
// will generate it when nothing there has been generated yet (a level
// nobody has broken into reads from a preview of its generator).
func (w *World) below(p Point) Terrain {
	q := Point{p.X, p.Y, p.Level + 1}
	if l := w.layer(q.Level); l != nil {
		if pi := l.tiles.pageIndex(q.X, q.Y); l.gen == nil || l.genDone[pi] {
			return w.TerrainAt(q)
		}
	}
	return w.peekGen(q.Level).At(q).Terrain
}

// peekGen is a preview of level l's generator, for asking what a level
// holds before it exists (see below). Cached; never saved.
func (w *World) peekGen(l Level) *ChunkPreview {
	if w.peeks == nil {
		w.peeks = make(map[Level]*ChunkPreview)
	}
	p, ok := w.peeks[l]
	if !ok {
		p = newChunkPreview(w.cfg, l)
		w.peeks[l] = p
	}
	return p
}

// naturalShaft joins the cavern at c to a cavern straight below it, if any
// part of the one lies over the other: a ladderless column of rock that
// erosion opened, now a one-level shaft. The lower cavern is broken into
// and revealed (its nests roll), and whatever lives there can climb up. A
// cavern with nothing beneath it gets no shaft.
func (w *World) naturalShaft(c Point, g *worldGen) {
	var sites []Point
	for _, p := range w.cavernFloor(c) {
		if w.below(p) == Floor {
			sites = append(sites, p)
		}
	}
	if len(sites) == 0 {
		return
	}
	p := sites[g.featureRand(genStreamNaturalShaft, int64(c.X), int64(c.Y), 1).IntN(len(sites))]
	foot := Point{p.X, p.Y, p.Level + 1}
	w.addLayer(foot.Level)
	w.SetTerrain(foot, ShaftBottom) // breaking in: the cavern below floods into view
	w.SetTerrain(p, ShaftTop)
	w.logEvent(LogCavern, fmt.Sprintf("The cavern holds a natural shaft down to a cavern on level %d.", foot.Level))
}

// sinkhole opens a hole in the cavern at c's floor onto the level below:
// the rubble lands on the tile underneath (rock becomes floor), which is
// broken into and revealed. A cavern with no room for one gets none.
func (w *World) sinkhole(c Point, g *worldGen) {
	var sites []Point
	for _, p := range w.cavernFloor(c) {
		if t := w.below(p); t == Rock || t == Floor {
			sites = append(sites, p)
		}
	}
	if len(sites) == 0 {
		return
	}
	p := sites[g.featureRand(genStreamSinkhole, int64(c.X), int64(c.Y), 1).IntN(len(sites))]
	foot := Point{p.X, p.Y, p.Level + 1}
	w.addLayer(foot.Level)
	w.SetTerrain(foot, Floor)
	w.SetTerrain(p, Hole)
	w.logEvent(LogCavern, fmt.Sprintf("A sinkhole in the cavern floor drops to level %d.", foot.Level))
}

// rollNestsAndFeatures is rollNests followed by naturalFeatures, on a copy
// of centers: a natural shaft breaks into the level below, which reveals,
// rolls nests and resets the shared nestCenters as it goes.
func (w *World) rollNestsAndFeatures(centers []Point) {
	centers = slices.Clone(centers)
	w.rollNests(centers)
	w.naturalFeatures(centers)
}

// ---- Raids -------------------------------------------------------------------

// raid is a hostile alien's rung after the hunt: with no prey it can walk
// to, it goes to an open hole on its level that lands in a room with a
// colonist in it, and drops in. It declines when there is no such hole.
type raid struct{}

func (raid) act(w *World, e *Entity) bool {
	h, ok := w.raidHole(e)
	if !ok {
		return false
	}
	e.State = Hunting
	if e.Pos.Adjacent(h) {
		w.leap(e, h, fmt.Sprintf("%s drops down a hole onto the level below!", capitalizeFirst(w.alienNounFor(e))))
		return true
	}
	if _, ok := w.travelTo(e, h); !ok {
		return false
	}
	return true
}

// raidHole is the open hole nearest e (by travelEstimate, ties to sorted
// order) that e can stand beside and that lands in a room holding a living
// colonist.
func (w *World) raidHole(e *Entity) (Point, bool) {
	if len(w.holes) == 0 {
		return Point{}, false
	}
	room := w.roomOf(e.Pos)
	var best Point
	bestD, found := 0, false
	for _, h := range w.holes {
		if h.Level != e.Pos.Level || !w.taskReachable(h, room) {
			continue
		}
		land, _, ok := w.fallTarget(h)
		if !ok || !w.colonistIn(w.roomOf(land)) {
			continue
		}
		if d := w.travelEstimate(e.Pos, h); !found || d < bestD {
			best, bestD, found = h, d, true
		}
	}
	return best, found
}

// colonistIn reports whether a living colonist is in room.
func (w *World) colonistIn(room RoomID) bool {
	if room == 0 {
		return false
	}
	for id := range w.kindEntities[Colonist] {
		if e := w.entities[id]; e != nil && e.Alive() && w.roomOf(e.Pos) == room {
			return true
		}
	}
	return false
}

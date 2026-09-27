package sim

import (
	"fmt"
	"math/rand/v2"
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
	// cavernSpacing is how far (Chebyshev) a cavern keeps from any
	// higher-ranked one (see keptCaverns), so separate caverns stay separate
	// and a passage between them means something.
	cavernSpacing = 3
	// cavernEdgeMargin keeps caverns and passages off the outermost tiles, so
	// the world's edge is still solid rock.
	cavernEdgeMargin = 1
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

// nestRadius is how far (Chebyshev) from its cavern's center a nest's aliens
// may be placed. Caverns are at least a few tiles across, so the nest lands in
// its own cavern rather than spread down a passage.
const nestRadius = 4

// trackCavernsForNests seeds the stream nest rolls use and starts the set of
// unfound cavern centers, which generateChunk fills as chunks are generated,
// so a breach can roll for each cavern's nest (see rollNests). Only the
// centers are kept: nothing about a nest exists until its cavern is found.
func (w *World) trackCavernsForNests() {
	w.rngSrc.nest = newPCG(w.cfg.Seed ^ 0x0452821E638D0137)
	w.nestRNG = rand.New(w.rngSrc.nest)
	w.unfoundCaverns = make(map[Point]struct{})
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
		if w.nestRNG.IntN(100) < w.cfg.CavernNestPercent {
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
	want := lo + w.nestRNG.IntN(hi-lo+1)
	species := w.nestRNG.IntN(len(w.alienSpecies))
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
		j := i + w.nestRNG.IntN(len(sites)-i)
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
	if w.rng.IntN(4) != 0 {
		return
	}
	d := neighbors8[w.rng.IntN(len(neighbors8))]
	n := e.Pos.Add(d.X, d.Y)
	if w.Walkable(n) && !w.occupiedByOther(n, e.ID) {
		w.moveEntity(e, n)
	}
}

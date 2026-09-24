package sim

// A flowField is a distance field toward a set of goal tiles: dist[cell] is the
// number of 8-connected steps from that cell to the nearest goal over walkable
// tiles (-1 = unreachable). It is the "everyone navigates the same" structure —
// computed once per change with a single multi-source BFS and then shared by
// every agent, each of which just steps to the downhill neighbor in O(1) per
// tick. That replaces N per-agent searches with one field.
//
// A field is defined by its seed function, which reports the goal (distance-0)
// tiles: walkable neighbors of facilities for a facility field, or of the
// unclaimed mining frontier for the frontier field.
// flowCell is one cell of a field: the distance, and the generation stamp that
// says whether that distance belongs to the current rebuild. They live in one
// struct because every read tests the stamp and then takes the distance, so
// splitting them across two grids meant two page lookups and two cache lines
// for what is one logical value.
type flowCell struct {
	gen  int32 // matches flowField.gen iff dist was written this rebuild
	dist int32 // steps to the nearest goal
}

type flowField struct {
	w    *World
	seed func(add func(Point)) // reports goal tiles (each passed to add)
	// goal reports whether a walkable tile is a goal. It must agree with seed
	// (a walkable tile is a goal iff seed passes it to add): seed builds the
	// field from nothing, goal answers for single tiles during a repair.
	goal func(Point) bool

	// cells is sparse: a field only ever writes a cell it reached, which means
	// walkable tiles, which means the colony. A page nothing reached reads as
	// the zero flowCell, whose gen can never match a live generation (rebuild
	// pre-increments, so gen >= 1), so an untouched page reads as unreachable
	// for free. See pagedgrid.go.
	cells pagedGrid[flowCell]
	gen   int32
	queue []int32 // reusable BFS frontier (cell indices)

	// full means the field must be rebuilt from scratch: it has never been
	// built, or more changed than a repair is worth. Otherwise touched lists
	// the places that changed since the last build, and ensureFresh repairs
	// just the part of the field they can reach (see repair).
	full      bool
	touched   []Point
	builtTick int // tick of the last build or repair (bounds them to once per tick)

	repairScratch // reusable buffers for repair
}

func newFlowField(w *World, seed func(add func(Point)), goal func(Point) bool) *flowField {
	return &flowField{
		w:         w,
		seed:      seed,
		goal:      goal,
		cells:     newPagedGrid[flowCell](w.Width, w.Height),
		full:      true,
		builtTick: -1,
	}
}

// maxTouched is how many changes a field accumulates before it gives up on
// repairing and rebuilds instead. Each touch costs a repair a few cells even
// when nothing moves, so past this a rebuild (one pass over the colony) is the
// cheaper and simpler answer; it is also what keeps world generation, which
// changes millions of tiles before any field is read, from queueing them all.
const maxTouched = 1024

// touch records that something at p changed: p's walkability, or whether any
// tile next to p is a goal (a facility built or removed beside it, a frontier
// rock appearing, vanishing, or being claimed). The field repairs the tiles
// around every touched point the next time it is read.
func (f *flowField) touch(p Point) {
	if f.full {
		return
	}
	if len(f.touched) >= maxTouched {
		f.invalidate()
		return
	}
	f.touched = append(f.touched, p)
}

// invalidate forces the next read to rebuild the field from scratch.
func (f *flowField) invalidate() {
	f.full = true
	f.touched = f.touched[:0]
}

// at returns the step distance from p to the nearest goal, or -1 if p is out of
// bounds or no goal is reachable.
func (f *flowField) at(p Point) int32 {
	w := f.w
	if !w.InBounds(p) {
		return -1
	}
	c := f.cells.at(p.X, p.Y)
	if c.gen != f.gen { // not reached in the current field => unreachable
		return -1
	}
	return c.dist
}

// rebuild recomputes the field from scratch with a multi-source BFS from its
// seed goals.
func (f *flowField) rebuild() {
	w := f.w
	f.gen++ // a new generation retires all prior distances without clearing them
	gen := f.gen
	q := f.queue[:0]
	add := func(p Point) {
		if !w.Walkable(p) {
			return
		}
		c := f.cells.ptr(p.X, p.Y)
		if c.gen == gen {
			return
		}
		c.gen, c.dist = gen, 0
		q = append(q, int32(w.index(p)))
	}
	f.seed(add)

	// Distance comes from the queue's layering rather than from reading the
	// cell back: every seed is at 0 and each expansion is one step further, so
	// the queue is in non-decreasing distance order and a layer ends where the
	// previous pass stopped appending. That is one paged read saved per node,
	// on the hottest loop in the simulation.
	cd, levelEnd := int32(0), len(q)
	for head := 0; head < len(q); head++ {
		if head == levelEnd {
			cd++
			levelEnd = len(q)
		}
		ci := int(q[head])
		cx, cy := ci%w.Width, ci/w.Width
		page := f.cells.interiorPage(cx, cy)
		for _, d := range neighbors8 {
			nx, ny := cx+d.X, cy+d.Y
			if nx < 0 || nx >= w.Width || ny < 0 || ny >= w.Height {
				continue
			}
			ni := ny*w.Width + nx
			cells := page
			if cells == nil {
				// On a page edge, so this neighbour may be on a page that does
				// not exist yet. Walkability has to be tested before asking for
				// it: rock never enters a field, and allocating for one would
				// give every field a border of pages around the reachable area.
				if !w.tiles[ni].Terrain.Walkable() {
					continue
				}
				cells = f.cells.pageAtAlloc(nx, ny)
			}
			// Stamp first, terrain second. Most neighbours in an open room are
			// already stamped this generation, and the stamp is a read of a
			// page this node is already holding, while the terrain read is a
			// scattered hit on the dense tile array a row-stride away. Testing
			// terrain first here cost 23% of the tick on a big colony.
			cell := &cells[offset(nx, ny)]
			if cell.gen == gen || !w.tiles[ni].Terrain.Walkable() {
				continue
			}
			cell.gen, cell.dist = gen, cd+1
			q = append(q, int32(ni))
		}
	}
	f.queue = q
}

// ensureFresh brings the field up to date, at most once per tick: the first
// reader of the tick pays for the shared field, the rest reuse it. A field
// that only has a few touched places is repaired around them; one that needs
// it (or has never been built) is rebuilt.
func (f *flowField) ensureFresh() {
	if f.builtTick == f.w.tick || (!f.full && len(f.touched) == 0) {
		return
	}
	if f.full || !f.repair() {
		f.rebuild()
	}
	f.full = false
	f.touched = f.touched[:0]
	f.builtTick = f.w.tick
}

// followField moves a colonist along the field toward the nearest goal. Any
// occupied tile can be traversed except an alien's, but the colonist only
// stops on a free tile. Returns whether it moved; false means it has arrived
// (distance 0), is boxed in, or the goal is unreachable from here.
//
// The breadth-first search expands through any occupied tile but an alien's —
// a cat or mouse parked in a narrow corridor must not wedge a colonist any
// more than another colonist would — and stops at the first depth with a free
// landing. Thus an open neighbor still costs one ordinary step, while a
// colonist can cross an arbitrarily crowded room or doorway in one turn
// without ever sharing a tile at rest. At that depth it prefers the lowest
// field distance, but may step uphill when every route toward the goal is
// occupied. That escape step is essential in a full room: otherwise a crowd
// with only uphill free space can remain gridlocked until its hungriest
// members starve.
func (w *World) followField(e *Entity, f *flowField) bool {
	cur := f.at(e.Pos)
	if cur <= 0 {
		return false
	}
	w.transitGen++
	gen := w.transitGen
	start := w.index(e.Pos)
	w.transitSeen.set(e.Pos.X, e.Pos.Y, gen)
	q := append(w.transitQ[:0], int32(start))
	var cand [8]Point
	var fallback [8]Point
	fallbackN := 0
	for head := 0; head < len(q); {
		levelEnd := len(q)
		n := 0
		best := int32(1<<31 - 1)
		fallbackLevelN := 0
		fallbackBest := int32(1<<31 - 1)
		for ; head < levelEnd; head++ {
			ci := int(q[head])
			from := Point{ci % w.Width, ci / w.Width}
			for _, d := range neighbors8 {
				p := from.Add(d.X, d.Y)
				if !w.Walkable(p) || w.buildTiles[p] {
					continue // never cross a tile a builder needs clear
				}
				nd := f.at(p)
				if nd < 0 {
					continue
				}
				pi := w.index(p)
				if pi == start {
					continue
				}
				blocker := w.entityAt(p)
				if blocker != nil && blocker.ID != e.ID {
					if blocker.Kind != Alien && w.transitSeen.at(p.X, p.Y) != gen {
						w.transitSeen.set(p.X, p.Y, gen)
						q = append(q, int32(pi))
					}
					continue
				}
				if nd > cur {
					if fallbackN == 0 {
						switch {
						case nd < fallbackBest:
							fallbackBest, fallback[0], fallbackLevelN = nd, p, 1
						case nd == fallbackBest && fallbackLevelN < len(fallback):
							fallback[fallbackLevelN] = p
							fallbackLevelN++
						}
					}
					continue
				}
				switch {
				case nd < best:
					best, cand[0], n = nd, p, 1
				case nd == best && n < len(cand):
					cand[n] = p
					n++
				}
			}
		}
		if best != int32(1<<31-1) {
			w.transitQ = q
			w.moveEntity(e, cand[w.rng.Intn(n)])
			return true
		}
		if fallbackN == 0 && fallbackBest != int32(1<<31-1) {
			fallbackN = fallbackLevelN
		}
	}
	if fallbackN > 0 {
		w.transitQ = q
		w.moveEntity(e, fallback[w.rng.Intn(fallbackN)])
		return true
	}
	w.transitQ = q
	return false
}

// facilityField returns the (lazily rebuilt) flow field for a facility terrain,
// or nil if that terrain is not a tracked facility.
func (w *World) facilityField(t Terrain) *flowField {
	if int(t) >= len(w.fields) || w.fields[t] == nil {
		return nil
	}
	f := w.fields[t]
	f.ensureFresh()
	return f
}

// frontierField returns the (lazily rebuilt) flow field toward the unclaimed
// mining frontier — the shared route every miner follows to reach diggable rock.
func (w *World) frontierField() *flowField {
	w.frontier.ensureFresh()
	return w.frontier
}

// adjacentFacility returns a facility tile of kind t next to p, if any.
func (w *World) adjacentFacility(p Point, t Terrain) (Point, bool) {
	for _, d := range neighbors8 {
		n := p.Add(d.X, d.Y)
		if w.TerrainAt(n) == t {
			return n, true
		}
	}
	return Point{}, false
}

// facilityGoal reports whether p is a goal of the facility field for kind:
// walkable and next to a tile of that kind that everyone may use. It is
// facilitySeed for one tile, and must agree with it, restricted fixtures and
// all, or a repaired field would differ from a rebuilt one.
func facilityGoal(w *World, kind Terrain) func(Point) bool {
	return func(p Point) bool {
		if !w.Walkable(p) {
			return false
		}
		restricted := w.restrictedFixtures[kind] > 0
		for _, d := range neighbors8 {
			fc := p.Add(d.X, d.Y)
			if w.TerrainAt(fc) == kind && (!restricted || w.communalFixture(fc)) {
				return true
			}
		}
		return false
	}
}

// facilitySeed builds the goal-seeding closure for a facility field: the walkable
// neighbors of every tile of the given terrain. Iterates w.facilityTiles[kind]
// (maintained incrementally by SetTerrain) rather than scanning the whole grid,
// so cost tracks the number of facilities, not the map's area.
func facilitySeed(w *World, kind Terrain) func(add func(Point)) {
	return func(add func(Point)) {
		restricted := w.restrictedFixtures[kind] > 0
		for fc := range w.facilityTiles[kind] {
			// The shared field is everyone's route, so it only leads to
			// fixtures everyone may use. A colonist headed for its own
			// private one routes there directly; see facilityReachable.
			if restricted && !w.communalFixture(fc) {
				continue
			}
			for _, d := range neighbors8 {
				add(fc.Add(d.X, d.Y))
			}
		}
	}
}

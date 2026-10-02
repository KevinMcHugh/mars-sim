package sim

// layered is a pagedGrid per level: per-tile scratch (A*'s cells, a flow
// field's distances, the facility search's stamps) for searches that can cross
// from one level to another at a stair. A level nothing has written to has no
// page table at all, so a grid that only ever touches the landing level costs
// what a single pagedGrid did.
//
// Hot loops that stay on one level fetch that level's grid once with grid and
// use the pagedGrid directly (pageAt, interiorPage), as they did before levels.
type layered[T any] struct {
	width, height int
	grids         []pagedGrid[T] // by Level; a zero pagedGrid until first written
}

func newLayered[T any](width, height int) layered[T] {
	return layered[T]{width: width, height: height}
}

// grid returns level l's grid, allocating its page table on first use.
func (g *layered[T]) grid(l Level) *pagedGrid[T] {
	for int(l) >= len(g.grids) {
		g.grids = append(g.grids, pagedGrid[T]{})
	}
	if g.grids[l].pages == nil {
		g.grids[l] = newPagedGrid[T](g.width, g.height)
	}
	return &g.grids[l]
}

// at reads p, which must be in bounds. A level never written reads as zero.
func (g *layered[T]) at(p Point) T {
	if int(p.Level) >= len(g.grids) || g.grids[p.Level].pages == nil {
		var zero T
		return zero
	}
	return g.grids[p.Level].at(p.X, p.Y)
}

// ptr returns a pointer to p's slot, allocating its page.
func (g *layered[T]) ptr(p Point) *T { return g.grid(p.Level).ptr(p.X, p.Y) }

// pagesAllocated counts the pages allocated on every level, for tests that
// check a grid tracks the colony rather than the map.
func (g *layered[T]) pagesAllocated() int {
	n := 0
	for i := range g.grids {
		if g.grids[i].pages != nil {
			n += g.grids[i].pagesAllocated()
		}
	}
	return n
}

// set writes p.
func (g *layered[T]) set(p Point, v T) { g.grid(p.Level).set(p.X, p.Y, v) }

// index is p's cell index: a dense integer naming one tile on one level, for
// queues and heaps that hold int32s rather than Points. Levels are stacked in
// order, so on one level the index orders tiles exactly as y*Width+x did, and
// tie-breaks on it are unchanged by levels. Callers must ensure p is in bounds.
func (w *World) index(p Point) int {
	return (int(p.Level)*w.Height+p.Y)*w.Width + p.X
}

// pointOf is the inverse of index.
func (w *World) pointOf(i int) Point {
	// Two divisions, not four: this decodes every node of every flow-field
	// BFS, the hottest loop in the simulation.
	l := i / (w.Width * w.Height)
	r := i - l*w.Width*w.Height
	y := r / w.Width
	return Point{r - y*w.Width, y, Level(l)}
}

// linkFrom returns the tile a stair at p leads to: the StairUp below a
// StairDown, or the StairDown above a StairUp. ok is false when p is not a
// stair, or its other end is not (yet, or any longer) the matching stair, so a
// half-built or broken stair links nothing. Every search that crosses levels
// (A*, the flow fields and their repair, the region graph) asks this, so a
// stair means the same thing to all of them.
func (w *World) linkFrom(p Point) (Point, bool) {
	var q Point
	var want Terrain
	switch w.TerrainAt(p) {
	case StairDown:
		q, want = Point{p.X, p.Y, p.Level + 1}, StairUp
	case StairUp:
		q, want = Point{p.X, p.Y, p.Level - 1}, StairDown
	default:
		return Point{}, false
	}
	if w.TerrainAt(q) != want {
		return Point{}, false
	}
	return q, true
}

// hasStairs reports whether any stair tile exists, so the hottest searches
// can skip asking linkFrom on a one-level world.
func (w *World) hasStairs() bool {
	return len(w.stairs) > 0 // a stair links only with its StairDown in place
}

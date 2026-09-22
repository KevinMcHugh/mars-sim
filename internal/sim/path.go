package sim

// Pathfinding: colonists navigate with A* over the walkable (Floor) grid instead
// of greedy step-toward, so they route around walls and concave rooms rather
// than wedging against them. A computed route is cached on the colonist and
// followed one step per tick, so A* runs once per job (not every tick), and the
// room system provides an O(1) reachability gate before any search runs.
//
// The search targets a tile *adjacent* to the work target (colonists mine,
// build, and use facilities from a neighboring floor tile), and uses 8-connected
// moves with uniform cost — matching the Chebyshev movement model used
// everywhere else.

// pfCell is A*'s per-cell state. The three fields are one struct because the
// search reads and writes them together on every relaxation, so keeping them in
// separate grids cost three page lookups and three cache lines for one logical
// record. int32 rather than int: a cell index has to fit one anyway to travel
// in flowField's queue, and halving the record is worth more here than the
// headroom on a map nothing can allocate.
type pfCell struct {
	gen  int32 // == pathfinder.gen means this cell was touched this search
	g    int32 // best known cost from start
	from int32 // predecessor cell index (-1 at the start cell)
}

// pathfinder holds reusable A* scratch, so repeated searches do not reallocate.
// A generation stamp (pfCell.gen vs gen) avoids clearing it between searches.
// One per World; used only on the engine goroutine.
type pathfinder struct {
	w     *World
	cells pagedGrid[pfCell]
	gen   int32
	open  pfHeap

	// corridorSeen marks cells inside the current HPA* corridor (== corridorGen),
	// painted once per search so the per-neighbor membership test is an O(1) array
	// read instead of a map lookup.
	corridorSeen pagedGrid[int32]
	corridorGen  int32
}

func newPathfinder(w *World) *pathfinder {
	return &pathfinder{
		w:            w,
		cells:        newPagedGrid[pfCell](w.Width, w.Height),
		corridorSeen: newPagedGrid[int32](w.Width, w.Height),
	}
}

// paintCorridor stamps every cell of the corridor's regions with a fresh
// generation, by scanning only those regions' chunks. After this, a cell is in
// the corridor iff corridorSeen[i] == corridorGen.
func (pf *pathfinder) paintCorridor(corridor map[RegionID]bool) {
	w := pf.w
	pf.corridorGen++
	gen := pf.corridorGen
	for rid := range corridor {
		reg := w.regions[rid]
		if reg == nil {
			continue
		}
		x0, y0, x1, y1 := w.chunkBounds(reg.chunk)
		// A chunk sits inside one page of each grid, so both lookups hoist out
		// of the sweep. Painting a corridor stamps every tile of every chunk it
		// crosses, which made this the single hottest paged read in the search.
		regions := w.regionOf.pageAt(x0, y0)
		if regions == nil {
			continue // no region in this chunk, so none of it is rid
		}
		seen := pf.corridorSeen.pageAtAlloc(x0, y0)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if o := offset(x, y); regions[o] == rid {
					seen[o] = gen
				}
			}
		}
	}
}

// toAdjacent returns a route (cells to step through, excluding the start) from
// start to some walkable tile adjacent to target, or ok=false if none exists.
// If useCorridor is set, the search only expands cells stamped by the most recent
// paintCorridor call (the HPA* abstract route); otherwise it is an unconstrained
// flat search.
func (pf *pathfinder) toAdjacent(start, target Point, useCorridor bool) ([]Point, bool) {
	w := pf.w
	if !w.Walkable(start) {
		return nil, false
	}
	pf.gen++
	si := w.index(start)
	pf.cells.set(start.X, start.Y, pfCell{gen: pf.gen, g: 0, from: -1})
	pf.open.reset()
	pf.open.push(pfNode{si, hAdjacent(start, target)})

	for pf.open.len() > 0 {
		ci := pf.open.pop().cell
		cp := Point{ci % w.Width, ci / w.Width}
		// Occupied tiles remain valid transit cells, but not destinations. The
		// start is the one exception: the caller already stands there.
		if cp.Chebyshev(target) == 1 && (ci == si || !w.occupied(cp)) {
			return pf.reconstruct(si, ci), true
		}
		cg := pf.cells.at(cp.X, cp.Y).g
		for _, d := range neighbors8 {
			np := cp.Add(d.X, d.Y)
			if !w.Walkable(np) {
				continue
			}
			if useCorridor && pf.corridorSeen.at(np.X, np.Y) != pf.corridorGen {
				continue // outside the abstract route
			}
			ng := cg + 1
			c := pf.cells.ptr(np.X, np.Y)
			if c.gen != pf.gen || ng < c.g {
				ni := w.index(np)
				c.gen, c.g, c.from = pf.gen, ng, int32(ci)
				pf.open.push(pfNode{ni, int(ng) + hAdjacent(np, target)})
			}
		}
	}
	return nil, false
}

// reconstruct walks predecessors from goal back to start, returning the forward
// route excluding the start cell.
func (pf *pathfinder) reconstruct(start, goal int) []Point {
	w := pf.w
	var rev []Point
	for ci := goal; ci != start; {
		p := Point{ci % w.Width, ci / w.Width}
		rev = append(rev, p)
		ci = int(pf.cells.at(p.X, p.Y).from)
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// hAdjacent is an admissible heuristic: the moves needed to reach the ring of
// tiles adjacent to target (Chebyshev distance minus the last step).
func hAdjacent(p, target Point) int {
	d := p.Chebyshev(target) - 1
	if d < 0 {
		return 0
	}
	return d
}

// pfNode is an open-set entry: a cell and its f = g + h score.
type pfNode struct {
	cell int
	f    int
}

// pfHeap is a hand-rolled binary min-heap of pfNode. It avoids the interface
// boxing of container/heap (A* pushes a lot), and reuses its backing array
// across searches. Ties break on cell index so paths are deterministic.
type pfHeap struct{ nodes []pfNode }

func (h *pfHeap) reset()   { h.nodes = h.nodes[:0] }
func (h *pfHeap) len() int { return len(h.nodes) }

func pfLess(a, b pfNode) bool {
	if a.f != b.f {
		return a.f < b.f
	}
	return a.cell < b.cell
}

func (h *pfHeap) push(n pfNode) {
	h.nodes = append(h.nodes, n)
	i := len(h.nodes) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if !pfLess(h.nodes[i], h.nodes[parent]) {
			break
		}
		h.nodes[i], h.nodes[parent] = h.nodes[parent], h.nodes[i]
		i = parent
	}
}

func (h *pfHeap) pop() pfNode {
	nodes := h.nodes
	top := nodes[0]
	last := len(nodes) - 1
	nodes[0] = nodes[last]
	h.nodes = nodes[:last]
	h.siftDown(0)
	return top
}

func (h *pfHeap) siftDown(i int) {
	n := len(h.nodes)
	for {
		l, r := 2*i+1, 2*i+2
		best := i
		if l < n && pfLess(h.nodes[l], h.nodes[best]) {
			best = l
		}
		if r < n && pfLess(h.nodes[r], h.nodes[best]) {
			best = r
		}
		if best == i {
			break
		}
		h.nodes[i], h.nodes[best] = h.nodes[best], h.nodes[i]
		i = best
	}
}

// pathToAdjacent finds a route to a tile adjacent to target, first rejecting
// obviously unreachable targets with an O(1) room check so A* is not run on a
// hopeless search.
func (w *World) pathToAdjacent(from, target Point) ([]Point, bool) {
	room := w.roomOf(from)
	reachable := false
	goalCell, goalDist := Point{}, 1<<30
	for _, d := range neighbors8 {
		n := target.Add(d.X, d.Y)
		if w.Walkable(n) && w.roomOf(n) == room && (n.Equal(from) || !w.occupied(n)) {
			reachable = true
			if dd := from.Chebyshev(n); dd < goalDist {
				goalCell, goalDist = n, dd
			}
		}
	}
	if !reachable {
		return nil, false
	}

	// Short or same-region trips: a flat tile search already explores little, so
	// skip the abstract routing overhead.
	startRegion := w.regionOf.at(from.X, from.Y)
	goalRegion := w.regionOf.at(goalCell.X, goalCell.Y)
	if startRegion == goalRegion || from.Chebyshev(target) <= 2*chunkSize {
		return w.pf.toAdjacent(from, target, false)
	}

	// Long cross-region trip: route over the region graph, paint that corridor,
	// then run the tile search constrained to it. Fall back to flat if either
	// step fails (e.g. the corridor cannot realize a tile path for some reason).
	if corridor, ok := w.abstractCorridor(startRegion, goalRegion); ok {
		w.pf.paintCorridor(corridor)
		if route, ok := w.pf.toAdjacent(from, target, true); ok {
			return route, true
		}
	}
	return w.pf.toAdjacent(from, target, false)
}

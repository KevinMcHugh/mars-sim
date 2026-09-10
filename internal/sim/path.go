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

// pathfinder holds reusable A* scratch sized to the grid, so repeated searches
// do not reallocate. A generation stamp (seen/gen) avoids clearing the arrays
// between searches. One per World; used only on the engine goroutine.
type pathfinder struct {
	w    *World
	g    []int // best known cost from start, per cell
	from []int // predecessor cell index, per cell
	seen []int // generation stamp per cell (== gen means touched this search)
	gen  int
	open pfHeap
}

func newPathfinder(w *World) *pathfinder {
	n := w.Width * w.Height
	return &pathfinder{
		w:    w,
		g:    make([]int, n),
		from: make([]int, n),
		seen: make([]int, n),
	}
}

// toAdjacent returns a route (cells to step through, excluding the start) from
// start to some walkable tile adjacent to target, or ok=false if none exists.
func (pf *pathfinder) toAdjacent(start, target Point) ([]Point, bool) {
	w := pf.w
	if !w.Walkable(start) {
		return nil, false
	}
	pf.gen++
	si := w.index(start)
	pf.g[si] = 0
	pf.from[si] = -1
	pf.seen[si] = pf.gen
	pf.open.reset()
	pf.open.push(pfNode{si, hAdjacent(start, target)})

	for pf.open.len() > 0 {
		ci := pf.open.pop().cell
		cp := Point{ci % w.Width, ci / w.Width}
		if cp.Chebyshev(target) == 1 { // adjacent to the work target: done
			return pf.reconstruct(si, ci), true
		}
		cg := pf.g[ci]
		for _, d := range neighbors8 {
			np := cp.Add(d.X, d.Y)
			if !w.Walkable(np) {
				continue
			}
			ni := w.index(np)
			ng := cg + 1
			if pf.seen[ni] != pf.gen || ng < pf.g[ni] {
				pf.seen[ni] = pf.gen
				pf.g[ni] = ng
				pf.from[ni] = ci
				pf.open.push(pfNode{ni, ng + hAdjacent(np, target)})
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
	for ci := goal; ci != start; ci = pf.from[ci] {
		rev = append(rev, Point{ci % w.Width, ci / w.Width})
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
	for _, d := range neighbors8 {
		n := target.Add(d.X, d.Y)
		if w.Walkable(n) && w.roomOf(n) == room {
			reachable = true
			break
		}
	}
	if !reachable {
		return nil, false
	}
	return w.pf.toAdjacent(from, target)
}

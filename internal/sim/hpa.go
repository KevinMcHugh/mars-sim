package sim

// Hierarchical pathfinding. For a long trip, searching every walkable tile is
// wasteful when the map already has a coarse structure. We reuse the region
// graph (regions + region.links, maintained by the room system) as an abstract
// graph: first route over regions ("which rooms to cross"), then run the real
// tile A* constrained to that corridor of regions. The corridor is the shared,
// broad-strokes part of the route; the tile search only fills in the detail.
//
// This bounds tile-level exploration to the regions on the abstract route,
// instead of the whole reachable area — the win grows with map size and with
// obstacles that would otherwise send a flat search exploring dead ends.

// abstractCorridor routes over the region graph from start to goal and returns
// the set of regions on that route (the corridor the tile search may use), or
// ok=false if the regions are not connected.
func (w *World) abstractCorridor(start, goal RegionID) (map[RegionID]bool, bool) {
	if start == 0 || goal == 0 || w.regions[start] == nil || w.regions[goal] == nil {
		return nil, false
	}
	if start == goal {
		return map[RegionID]bool{start: true}, true
	}
	goalRep := w.regions[goal].rep

	gscore := map[RegionID]int{start: 0}
	came := map[RegionID]RegionID{}
	closed := map[RegionID]bool{}
	open := regionHeap{}
	open.push(regionNode{start, w.regions[start].rep.Chebyshev(goalRep)})

	for open.len() > 0 {
		cur := open.pop().region
		if cur == goal {
			corridor := map[RegionID]bool{goal: true}
			for c := goal; c != start; c = came[c] {
				corridor[came[c]] = true
			}
			return corridor, true
		}
		if closed[cur] {
			continue
		}
		closed[cur] = true
		curRep := w.regions[cur].rep
		cg := gscore[cur]
		for nb := range w.regions[cur].links {
			nr := w.regions[nb]
			if nr == nil || closed[nb] {
				continue
			}
			ng := cg + curRep.Chebyshev(nr.rep)
			if old, ok := gscore[nb]; !ok || ng < old {
				gscore[nb] = ng
				came[nb] = cur
				open.push(regionNode{nb, ng + nr.rep.Chebyshev(goalRep)})
			}
		}
	}
	return nil, false
}

// regionNode is an abstract-search open-set entry.
type regionNode struct {
	region RegionID
	f      int
}

// regionHeap is a min-heap of regionNode by f, ties broken on region ID for
// determinism. Hand-rolled to avoid interface boxing, like the tile heap.
type regionHeap struct{ nodes []regionNode }

func (h *regionHeap) len() int { return len(h.nodes) }

func regionLess(a, b regionNode) bool {
	if a.f != b.f {
		return a.f < b.f
	}
	return a.region < b.region
}

func (h *regionHeap) push(n regionNode) {
	h.nodes = append(h.nodes, n)
	i := len(h.nodes) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if !regionLess(h.nodes[i], h.nodes[parent]) {
			break
		}
		h.nodes[i], h.nodes[parent] = h.nodes[parent], h.nodes[i]
		i = parent
	}
}

func (h *regionHeap) pop() regionNode {
	nodes := h.nodes
	top := nodes[0]
	last := len(nodes) - 1
	nodes[0] = nodes[last]
	h.nodes = nodes[:last]
	n := last
	i := 0
	for {
		l, r := 2*i+1, 2*i+2
		best := i
		if l < n && regionLess(nodes[l], nodes[best]) {
			best = l
		}
		if r < n && regionLess(nodes[r], nodes[best]) {
			best = r
		}
		if best == i {
			break
		}
		nodes[i], nodes[best] = nodes[best], nodes[i]
		i = best
	}
	return top
}

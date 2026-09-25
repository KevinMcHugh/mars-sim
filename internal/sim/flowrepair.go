package sim

// Incremental flow-field repair.
//
// A field used to be rebuilt from scratch — a BFS over the whole colony —
// whenever anything it depends on changed, and something always had: every
// mined tile is a terrain change, and every claim moves a frontier goal. With a
// hundred colonists digging that was several full-colony searches a tick, a
// quarter of a real profile. A change almost never moves more than a handful
// of distances, so repair recomputes only those.
//
// The result is exactly what rebuild would produce (TestFlowFieldRepairMatchesRebuild
// checks it tick by tick), so nothing that reads a field can tell the
// difference: it is the same simulation, just cheaper.
//
// A repair has two phases:
//
//  1. Find the cells whose distance may have gone *up*: a tile that stopped
//     being walkable or stopped being a goal, and every cell that routed
//     through one. A cell at distance d keeps its distance if it is still a
//     goal (d == 0) or still has a neighbour at d-1 that is itself unaffected.
//     Cells are checked in order of their old distance, so every possible
//     parent of a cell has been decided before the cell is. The affected cells
//     are forgotten (marked unreachable).
//  2. Recompute, Dijkstra-style over unit steps: seed every forgotten cell and
//     every changed tile with the best distance its current neighbours offer
//     (0 for a goal), then relax outward. This also carries the decreases a
//     new walkable tile or a new goal causes. Every distance that survived
//     phase 1 is still achievable, so relaxing only ever lowers a value
//     towards the truth, and a cell's value settles once nothing lowers it.

// maxRepairCells caps how many cells one repair may forget in phase 1. Past
// it the repair is covering a large part of the colony anyway, and a rebuild
// does the same work with less bookkeeping.
const maxRepairCells = 1 << 16

// repairScratch holds a field's reusable repair buffers, so a repair that
// runs every tick allocates nothing once they have grown.
type repairScratch struct {
	candidates []int32            // changed cells, deduplicated
	seen       map[int32]struct{} // dedup for candidates
	checked    map[int32]struct{} // phase 1: cells already decided
	affected   map[int32]struct{} // phase 1: cells that lost their distance
	affectedAt []int32            // affected, in the order found
	heap       distHeap           // phase 1 and phase 2 work queue
}

// repair brings the field up to date by fixing the cells around f.touched. It
// reports false, leaving the field for rebuild to redo, when the damage turns
// out to be too widespread to be worth repairing.
func (f *flowField) repair() bool {
	w := f.w
	if f.seen == nil {
		f.seen = make(map[int32]struct{})
		f.checked = make(map[int32]struct{})
		f.affected = make(map[int32]struct{})
	}
	clear(f.seen)
	clear(f.checked)
	clear(f.affected)
	f.candidates = f.candidates[:0]
	f.affectedAt = f.affectedAt[:0]

	// Every tile within one step of a touched point may have changed
	// walkability or goal status (see touch).
	for _, p := range f.touched {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				q := Point{p.X + dx, p.Y + dy}
				if !w.InBounds(q) {
					continue
				}
				i := int32(w.index(q))
				if _, dup := f.seen[i]; !dup {
					f.seen[i] = struct{}{}
					f.candidates = append(f.candidates, i)
				}
			}
		}
	}

	// Phase 1: find what lost its distance, in order of old distance.
	h := &f.heap
	h.reset()
	for _, i := range f.candidates {
		if d, ok := f.dist(i); ok {
			h.push(d, i)
		}
	}
	for h.len() > 0 {
		d, i := h.pop()
		if _, done := f.checked[i]; done {
			continue
		}
		f.checked[i] = struct{}{}
		if f.supported(i, d) {
			continue
		}
		f.affected[i] = struct{}{}
		f.affectedAt = append(f.affectedAt, i)
		if len(f.affectedAt) > maxRepairCells {
			return false
		}
		// Only a neighbour one step further out can have routed through i.
		f.forNeighbours(i, func(n int32) {
			if nd, ok := f.dist(n); ok && nd == d+1 {
				h.push(nd, n)
			}
		})
	}
	for _, i := range f.affectedAt {
		f.cellPtr(i).gen = 0 // forget it: reads as unreachable
	}

	// Phase 2: recompute, seeding from what is left.
	h.reset()
	seed := func(i int32) {
		p := Point{int(i) % w.Width, int(i) / w.Width}
		if !w.Walkable(p) {
			return
		}
		if f.goal(p) {
			h.push(0, i)
			return
		}
		best := int32(-1)
		f.forNeighbours(i, func(n int32) {
			if nd, ok := f.dist(n); ok && (best < 0 || nd+1 < best) {
				best = nd + 1
			}
		})
		if best >= 0 {
			h.push(best, i)
		}
	}
	for _, i := range f.affectedAt {
		seed(i)
	}
	for _, i := range f.candidates {
		seed(i)
	}
	for h.len() > 0 {
		d, i := h.pop()
		if cur, ok := f.dist(i); ok && cur <= d {
			continue
		}
		c := f.cellPtr(i)
		c.gen, c.dist = f.gen, d
		f.forNeighbours(i, func(n int32) {
			if !w.tiles[n].Terrain.Walkable() {
				return
			}
			if nd, ok := f.dist(n); !ok || nd > d+1 {
				h.push(d+1, n)
			}
		})
	}
	return true
}

// supported reports whether cell i, at distance d in the field as it stood,
// can keep that distance: it is still walkable and either still a goal or
// still next to an unaffected cell one step closer.
func (f *flowField) supported(i, d int32) bool {
	w := f.w
	p := Point{int(i) % w.Width, int(i) / w.Width}
	if !w.Walkable(p) {
		return false
	}
	if d == 0 {
		return f.goal(p)
	}
	ok := false
	f.forNeighbours(i, func(n int32) {
		if ok {
			return
		}
		if nd, valid := f.dist(n); valid && nd == d-1 {
			if _, gone := f.affected[n]; !gone {
				ok = true
			}
		}
	})
	return ok
}

// dist is at for a cell index: the cell's distance, and whether it has one.
func (f *flowField) dist(i int32) (int32, bool) {
	w := f.w
	c := f.cells.at(int(i)%w.Width, int(i)/w.Width)
	return c.dist, c.gen == f.gen
}

func (f *flowField) cellPtr(i int32) *flowCell {
	w := f.w
	return f.cells.ptr(int(i)%w.Width, int(i)/w.Width)
}

// forNeighbours calls fn with the index of each in-bounds 8-neighbour of i.
func (f *flowField) forNeighbours(i int32, fn func(int32)) {
	w := f.w
	x, y := int(i)%w.Width, int(i)/w.Width
	for _, d := range neighbors8 {
		nx, ny := x+d.X, y+d.Y
		if nx < 0 || nx >= w.Width || ny < 0 || ny >= w.Height {
			continue
		}
		fn(int32(ny*w.Width + nx))
	}
}

// distHeap is a binary min-heap of (distance, cell) pairs. Ties may pop in any
// order; nothing in a repair depends on which of two equal-distance cells goes
// first, only on nearer cells going before farther ones.
type distHeap struct{ items []distItem }

type distItem struct{ d, i int32 }

func (h *distHeap) reset()   { h.items = h.items[:0] }
func (h *distHeap) len() int { return len(h.items) }

func (h *distHeap) push(d, i int32) {
	h.items = append(h.items, distItem{d, i})
	for k := len(h.items) - 1; k > 0; {
		parent := (k - 1) / 2
		if h.items[parent].d <= h.items[k].d {
			break
		}
		h.items[parent], h.items[k] = h.items[k], h.items[parent]
		k = parent
	}
}

func (h *distHeap) pop() (d, i int32) {
	top := h.items[0]
	last := len(h.items) - 1
	h.items[0] = h.items[last]
	h.items = h.items[:last]
	for k := 0; ; {
		l, r, m := 2*k+1, 2*k+2, k
		if l < last && h.items[l].d < h.items[m].d {
			m = l
		}
		if r < last && h.items[r].d < h.items[m].d {
			m = r
		}
		if m == k {
			break
		}
		h.items[k], h.items[m] = h.items[m], h.items[k]
		k = m
	}
	return top.d, top.i
}

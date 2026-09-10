package sim

// A flowField is a distance field toward a set of goal tiles: dist[cell] is the
// number of 8-connected steps from that cell to the nearest goal over walkable
// tiles (-1 = unreachable). It is the "everyone navigates the same" structure —
// computed once per change with a single multi-source BFS and then shared by
// every agent, each of which just steps to the downhill neighbor in O(1) per
// tick. That replaces N per-agent searches (and, here, N full-grid facility
// scans) with one field.
//
// This field targets a facility Terrain: its goals are the walkable tiles
// adjacent to any tile of that kind, so following it to distance 0 leaves the
// agent standing next to a facility it can use.
type flowField struct {
	w    *World
	kind Terrain

	dist  []int32 // steps to nearest goal per cell; -1 = unreachable
	queue []int32 // reusable BFS frontier (cell indices)

	stale     bool // terrain changed since the last rebuild
	builtTick int  // tick of the last rebuild (bounds rebuilds to once per tick)
}

func newFlowField(w *World, kind Terrain) *flowField {
	return &flowField{
		w:         w,
		kind:      kind,
		dist:      make([]int32, w.Width*w.Height),
		stale:     true,
		builtTick: -1,
	}
}

// at returns the step distance from p to the nearest facility, or -1 if p is out
// of bounds or no facility is reachable.
func (f *flowField) at(p Point) int32 {
	w := f.w
	if !w.InBounds(p) {
		return -1
	}
	return f.dist[w.index(p)]
}

// rebuild recomputes the field from scratch with a multi-source BFS seeded from
// every walkable tile adjacent to a facility of this kind.
func (f *flowField) rebuild() {
	w := f.w
	for i := range f.dist {
		f.dist[i] = -1
	}
	q := f.queue[:0]

	// Seed: walkable neighbors of each facility tile are the goals (distance 0).
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if w.tiles[y*w.Width+x].Terrain != f.kind {
				continue
			}
			fc := Point{x, y}
			for _, d := range neighbors8 {
				n := fc.Add(d.X, d.Y)
				if w.Walkable(n) {
					ni := w.index(n)
					if f.dist[ni] != 0 {
						f.dist[ni] = 0
						q = append(q, int32(ni))
					}
				}
			}
		}
	}

	// BFS outward over walkable tiles.
	for head := 0; head < len(q); head++ {
		ci := int(q[head])
		cd := f.dist[ci]
		cx, cy := ci%w.Width, ci/w.Width
		for _, d := range neighbors8 {
			nx, ny := cx+d.X, cy+d.Y
			if nx < 0 || nx >= w.Width || ny < 0 || ny >= w.Height {
				continue
			}
			ni := ny*w.Width + nx
			if f.dist[ni] != -1 || !w.tiles[ni].Terrain.Walkable() {
				continue
			}
			f.dist[ni] = cd + 1
			q = append(q, int32(ni))
		}
	}
	f.queue = q
}

// followField moves a colonist one step down the field's gradient toward the
// nearest facility, avoiding tiles held by others. Returns whether it moved;
// false means it has arrived (distance 0), is stuck behind others, or the field
// is unreachable from here.
func (w *World) followField(e *Entity, f *flowField) bool {
	cur := f.at(e.Pos)
	if cur <= 0 {
		return false // already adjacent to a facility, or unreachable
	}
	best := e.Pos
	bestDist := cur
	for _, d := range neighbors8 {
		n := e.Pos.Add(d.X, d.Y)
		if !w.Walkable(n) || w.occupiedByOther(n, e.ID) {
			continue
		}
		if nd := f.at(n); nd >= 0 && nd < bestDist {
			best, bestDist = n, nd
		}
	}
	if best.Equal(e.Pos) {
		return false
	}
	w.moveEntity(e, best)
	return true
}

// facilityField returns the (lazily rebuilt) flow field for a facility terrain,
// or nil if that terrain is not a tracked facility. Rebuild happens at most once
// per tick: the first seeker of the tick pays for the shared field, the rest
// reuse it.
func (w *World) facilityField(t Terrain) *flowField {
	if int(t) >= len(w.fields) {
		return nil
	}
	f := w.fields[t]
	if f == nil {
		return nil
	}
	if f.stale && f.builtTick != w.tick {
		f.rebuild()
		f.stale = false
		f.builtTick = w.tick
	}
	return f
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

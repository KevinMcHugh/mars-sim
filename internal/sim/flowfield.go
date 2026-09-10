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
type flowField struct {
	w    *World
	seed func(add func(Point)) // reports goal tiles (each passed to add)

	dist  []int32 // steps to nearest goal per cell (valid only where seen==gen)
	seen  []int32 // generation stamp per cell; avoids an O(map) reset per rebuild
	gen   int32
	queue []int32 // reusable BFS frontier (cell indices)

	stale     bool // goals or terrain changed since the last rebuild
	builtTick int  // tick of the last rebuild (bounds rebuilds to once per tick)
}

func newFlowField(w *World, seed func(add func(Point))) *flowField {
	return &flowField{
		w:         w,
		seed:      seed,
		dist:      make([]int32, w.Width*w.Height),
		seen:      make([]int32, w.Width*w.Height),
		stale:     true,
		builtTick: -1,
	}
}

// at returns the step distance from p to the nearest goal, or -1 if p is out of
// bounds or no goal is reachable.
func (f *flowField) at(p Point) int32 {
	w := f.w
	if !w.InBounds(p) {
		return -1
	}
	i := w.index(p)
	if f.seen[i] != f.gen { // not reached in the current field => unreachable
		return -1
	}
	return f.dist[i]
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
		i := w.index(p)
		if f.seen[i] == gen {
			return
		}
		f.seen[i] = gen
		f.dist[i] = 0
		q = append(q, int32(i))
	}
	f.seed(add)

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
			if f.seen[ni] == gen || !w.tiles[ni].Terrain.Walkable() {
				continue
			}
			f.seen[ni] = gen
			f.dist[ni] = cd + 1
			q = append(q, int32(ni))
		}
	}
	f.queue = q
}

// ensureFresh rebuilds the field if it is stale, at most once per tick: the
// first reader of the tick pays for the shared field, the rest reuse it.
func (f *flowField) ensureFresh() {
	if f.stale && f.builtTick != f.w.tick {
		f.rebuild()
		f.stale = false
		f.builtTick = f.w.tick
	}
}

// followField moves a colonist one step down the field's gradient toward the
// nearest goal, avoiding tiles held by others. Returns whether it moved; false
// means it has arrived (distance 0), is stuck behind others, or the goal is
// unreachable from here.
func (w *World) followField(e *Entity, f *flowField) bool {
	cur := f.at(e.Pos)
	if cur <= 0 {
		return false
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

// facilitySeed builds the goal-seeding closure for a facility field: the walkable
// neighbors of every tile of the given terrain.
func facilitySeed(w *World, kind Terrain) func(add func(Point)) {
	return func(add func(Point)) {
		for y := 0; y < w.Height; y++ {
			for x := 0; x < w.Width; x++ {
				if w.tiles[y*w.Width+x].Terrain != kind {
					continue
				}
				fc := Point{x, y}
				for _, d := range neighbors8 {
					add(fc.Add(d.X, d.Y))
				}
			}
		}
	}
}

package sim

// Showing the shared flow fields to a frontend: which fields exist, and the
// distances of the one a frontend asked to see. It is a debugging view of the
// navigation layer (see docs/flow-field-view.md), so it costs nothing until a
// frontend opts in with ShowFlowField, and then only the one field it chose.

// FlowFieldRef names one shared flow field: the field toward a facility
// terrain (Frontier false), or the field toward the unclaimed mining frontier
// (Frontier true, Facility ignored).
type FlowFieldRef struct {
	Frontier bool
	Facility Terrain
}

// Name is what a frontend calls the field: the facility it leads to, or
// "mining frontier".
func (r FlowFieldRef) Name() string {
	if r.Frontier {
		return "mining frontier"
	}
	return r.Facility.String()
}

// ShowFlowField asks the engine to publish one flow field's distances on
// every Snapshot (as Snapshot.FlowField) until told otherwise. Show false
// stops it. The choice belongs to the engine, so with several frontends
// subscribed, all of them see the field the last one picked.
type ShowFlowField struct {
	Show  bool
	Field FlowFieldRef
}

func (ShowFlowField) isCommand() {}

// FlowFieldView is a published copy of one flow field. It is immutable and
// may be shared by consecutive snapshots while the field does not change.
type FlowFieldView struct {
	Field FlowFieldRef
	// Goals is how many tiles sit at distance 0, and Max is the largest
	// finite distance: the range a frontend scales its colour ramp to.
	Goals int
	Max   int32
	// dist holds distance+1, so an unwritten page (zero) reads as
	// unreachable without being allocated: the view is as sparse as the
	// field it copies.
	dist pagedGrid[int32]
	w, h int
}

// At returns the step distance from p to the field's nearest goal, or -1 if p
// is out of bounds, not walkable, or cannot reach a goal.
func (v *FlowFieldView) At(p Point) int32 {
	if v == nil || p.X < 0 || p.X >= v.w || p.Y < 0 || p.Y >= v.h {
		return -1
	}
	return v.dist.at(p.X, p.Y) - 1
}

// flowFieldRefs lists every shared field in a stable order: the facility
// fields in terrain order, then the frontier.
func (w *World) flowFieldRefs() []FlowFieldRef {
	var refs []FlowFieldRef
	for t, f := range w.fields {
		if f != nil {
			refs = append(refs, FlowFieldRef{Facility: Terrain(t)})
		}
	}
	return append(refs, FlowFieldRef{Frontier: true})
}

// flowFieldFor returns the field a ref names, freshened, or nil if there is
// no such field.
//
// Freshening here is safe for determinism: ensureFresh runs at most once a
// tick, and a repair or rebuild always produces the same distances whenever
// it runs (TestFlowFieldRepairMatchesRebuild), so reading a field for display
// changes when the work is done but never what a colonist reads.
// TestFlowFieldViewKeepsRunDeterministic holds that.
func (w *World) flowFieldFor(r FlowFieldRef) *flowField {
	if r.Frontier {
		return w.frontierField()
	}
	return w.facilityField(r.Facility)
}

// flowFieldView copies the field r names into a FlowFieldView, or reuses prev
// if it is a copy of the same field at the same version. It returns nil if r
// names no field.
func (w *World) flowFieldView(r FlowFieldRef, prev *FlowFieldView, prevVersion int) (*FlowFieldView, int) {
	f := w.flowFieldFor(r)
	if f == nil {
		return nil, 0
	}
	if prev != nil && prev.Field == r && prevVersion == f.version {
		return prev, prevVersion
	}
	v := &FlowFieldView{Field: r, dist: newPagedGrid[int32](w.Width, w.Height), w: w.Width, h: w.Height}
	for pi, src := range f.cells.pages {
		if src == nil {
			continue
		}
		var dst []int32
		for i, c := range src {
			if c.gen != f.gen {
				continue
			}
			if dst == nil {
				dst = make([]int32, gridPageLen)
			}
			dst[i] = c.dist + 1
			if c.dist == 0 {
				v.Goals++
			}
			v.Max = max(v.Max, c.dist)
		}
		v.dist.pages[pi] = dst
	}
	return v, f.version
}

// NewFlowFieldView builds a view from explicit distances, for a hand-built
// Snapshot (frontend tests). Every point not in dist reads as unreachable.
func NewFlowFieldView(field FlowFieldRef, width, height int, dist map[Point]int32) *FlowFieldView {
	v := &FlowFieldView{Field: field, dist: newPagedGrid[int32](width, height), w: width, h: height}
	for p, d := range dist {
		v.dist.set(p.X, p.Y, d+1)
		if d == 0 {
			v.Goals++
		}
		v.Max = max(v.Max, d)
	}
	return v
}

// Range calls fn for every tile in [x0,x1)×[y0,y1) the field reaches, in row
// order, with its distance. Stretches of the map the field never reached are
// skipped a page at a time, so a view the size of a huge map costs what the
// colony in it does, not its area.
func (v *FlowFieldView) Range(x0, y0, x1, y1 int, fn func(p Point, dist int32)) {
	if v == nil {
		return
	}
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = min(x1, v.w), min(y1, v.h)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; {
			page := v.dist.pageAt(x, y)
			end := min(x1, (x|gridPageMask)+1) // the end of this page's row
			if page == nil {
				x = end
				continue
			}
			for ; x < end; x++ {
				if d := page[offset(x, y)]; d > 0 {
					fn(Point{x, y}, d-1)
				}
			}
		}
	}
}

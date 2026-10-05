package sim

import (
	"container/heap"
	"fmt"
)

// ---- Passages: digging back through to a part of the colony cut off from it ----
//
// Room siting avoids cutting the colony in two (siteKeepsColonyWhole), but
// that is prevention, and something can always get through it. These are the
// two corrections. A colonist cut off from the colony makes its own way out
// (FocusEscape, see assignDemolish); a facility cut off with nobody beside it
// gets a passage project, dug from the colony's side (planPassage). Both follow
// the cheapest way through: round a structure through the rock where that is
// cheaper, through its wall where it is not. See docs/escape.md.

// PassageName is the project planPassage orders.
const PassageName = "passage"

// passageSearchLimit caps how many tiles passageRoute settles before giving up.
// A way round through rock to a colony far off is no rescue, and the search
// must not cost more than a pocket is worth.
const passageSearchLimit = 1 << 14

// passageEnterCost is what stepping onto p costs on a passage route, in ticks of
// work: one to walk onto open floor, or the work of digging out rock or
// breaking down a wall or hull first. Anything else — a facility, a chair —
// is never broken through, and neither is anything in avoid (tiles another
// project will build on, which a passage would only have closed again).
func (w *World) passageEnterCost(p Point, avoid map[Point]bool) (int, bool) {
	if !w.InBounds(p) || avoid[p] {
		return 0, false
	}
	switch t := w.TerrainAt(p); {
	case w.Walkable(p):
		return 1, true
	case t == Rock:
		return 1 + w.cfg.MineTicks, true
	case t == Wall || t == Hull:
		return 1 + w.cfg.DemolishTicks, true
	default:
		return 0, false
	}
}

// passageRoute returns the cheapest route (see passageEnterCost) from any of
// sources to an open tile in room goal, sources first and that tile last,
// or false if there is none within passageSearchLimit. A source costs what
// entering it does, so a source that is rock or wall is on the route to be
// cleared like any other. Ties break on the tile's row-major index, so the
// route is the same every run.
func (w *World) passageRoute(sources []Point, goal RoomID, avoid map[Point]bool) ([]Point, bool) {
	if goal == 0 {
		return nil, false
	}
	dist := make(map[Point]int)
	prev := make(map[Point]Point)
	h := &passageHeap{width: w.Width}
	for _, s := range sources {
		if c, ok := w.passageEnterCost(s, avoid); ok {
			if d, seen := dist[s]; !seen || c < d {
				dist[s] = c
				heap.Push(h, passageNode{s, c})
			}
		}
	}
	for settled := 0; h.Len() > 0 && settled < passageSearchLimit; settled++ {
		n := heap.Pop(h).(passageNode)
		if n.cost != dist[n.p] {
			continue // a cheaper way here was found after this was queued
		}
		if w.Walkable(n.p) && w.roomOf(n.p) == goal {
			route := []Point{n.p}
			for p := n.p; ; {
				q, ok := prev[p]
				if !ok {
					break
				}
				route = append(route, q)
				p = q
			}
			for i, j := 0, len(route)-1; i < j; i, j = i+1, j-1 {
				route[i], route[j] = route[j], route[i]
			}
			return route, true
		}
		for _, d := range neighbors8 {
			next := n.p.Add(d.X, d.Y)
			c, ok := w.passageEnterCost(next, avoid)
			if !ok {
				continue
			}
			if old, seen := dist[next]; !seen || n.cost+c < old {
				dist[next] = n.cost + c
				prev[next] = n.p
				heap.Push(h, passageNode{next, n.cost + c})
			}
		}
	}
	return nil, false
}

// escapeTarget is the first tile a colonist at from has to dig or break
// through on the cheapest way from its own pocket to the colony's main room.
// It borders the pocket, so the colonist can reach it.
func (w *World) escapeTarget(from Point) (Point, bool) {
	if w.roomOf(from) == 0 {
		return Point{}, false
	}
	route, ok := w.passageRoute([]Point{from}, w.mainRoom, nil)
	if !ok {
		return Point{}, false
	}
	for _, p := range route {
		if !w.Walkable(p) {
			return p, true
		}
	}
	return Point{}, false
}

// fixtureCutOff reports whether the fixture at p has no open neighbor in the
// colony's main room: nobody in the colony can stand beside it to use it.
func (w *World) fixtureCutOff(p Point) bool {
	for _, d := range neighbors8 {
		if n := p.Add(d.X, d.Y); w.Walkable(n) && w.roomOf(n) == w.mainRoom {
			return false
		}
	}
	return true
}

// planPassage orders a passage to the cut-off fixture with the row-major first
// position, if there is one and no passage is already under way, and reports
// whether it did. A passage is the cheapest route from the fixture's neighbors
// to the colony's main room, every rock tile on it to dig and every wall or
// hull tile to break down, all in one phase: they open from the colony's
// side inward as each cleared tile lets a builder reach the next, exactly as
// a room's interior is dug. It is unpaid, like the colony's first scumhouse:
// a facility walled off from everyone is not a public work to wait on the
// treasury for.
//
// A colonist who is cut off breaks out on its own (FocusEscape). This is for
// the facility with nobody beside it — a kitchen sealed shut, a pod whose
// only access tile a later room walled over — which no one would otherwise
// ever reach again.
func (w *World) planPassage() bool {
	if w.mainRoom == 0 {
		return false
	}
	for _, p := range w.projects {
		if p.name == PassageName {
			return false
		}
	}
	var target Point
	found := false
	for p := range w.fixtures {
		if w.discovered(p) && w.fixtureCutOff(p) && (!found || lessPoint(p, target)) {
			target, found = p, true
		}
	}
	if !found {
		return false
	}
	designated := make(map[Point]bool)
	for _, p := range w.projects {
		for _, t := range p.tasks {
			designated[t.pos] = true
		}
	}
	var sources []Point
	for _, d := range neighbors8 {
		sources = append(sources, target.Add(d.X, d.Y))
	}
	route, ok := w.passageRoute(sources, w.mainRoom, designated)
	if !ok {
		return false
	}
	p := &project{id: w.nextProjectID, name: PassageName, queuedTick: w.tick, issuer: Nobody, workKind: WorkDig}
	for _, pos := range route {
		if t := w.TerrainAt(pos); !w.Walkable(pos) {
			p.tasks = append(p.tasks, &buildTask{pos: pos, terrain: Floor, clears: t, phase: roomDigPhase, proj: p})
		}
	}
	if len(p.tasks) == 0 {
		return false // the region labels have not caught up with a change yet
	}
	w.nextProjectID++
	w.projects = append(w.projects, p)
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony orders a way dug through to the %s cut off at (%d, %d).",
		w.TerrainAt(target), target.X, target.Y))
	return true
}

// passageNode is a tile queued in passageRoute at a route cost.
type passageNode struct {
	p    Point
	cost int
}

// passageHeap orders passageNodes by cost, then row-major position.
type passageHeap struct {
	nodes []passageNode
	width int
}

func (h *passageHeap) Len() int { return len(h.nodes) }
func (h *passageHeap) Less(i, j int) bool {
	a, b := h.nodes[i], h.nodes[j]
	if a.cost != b.cost {
		return a.cost < b.cost
	}
	return a.p.Y*h.width+a.p.X < b.p.Y*h.width+b.p.X
}
func (h *passageHeap) Swap(i, j int) { h.nodes[i], h.nodes[j] = h.nodes[j], h.nodes[i] }
func (h *passageHeap) Push(x any)    { h.nodes = append(h.nodes, x.(passageNode)) }
func (h *passageHeap) Pop() any {
	n := h.nodes[len(h.nodes)-1]
	h.nodes = h.nodes[:len(h.nodes)-1]
	return n
}

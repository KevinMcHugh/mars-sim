package sim

import "fmt"

// ---- Excavation orders -----------------------------------------------------------
//
// A player can order an area mined out. The order is an ordinary public work,
// bought like a room: every rock tile in the area the colony has seen becomes a
// dig task of one project, and every task is a work order (WorkDig) on the
// order book, escrowed from the treasury. Whoever digs a tile is paid its wage
// (wage-dig) when it is cleared, and keeps the ore. It is all or nothing, as
// fundProject is for a room: a treasury that cannot cover the whole area buys
// none of it. See docs/excavation.md.

// excavationName is the project an excavation order creates. The Jobs tab
// lists it by this name, and it is how a dig is told from a room.
const excavationName = "excavation"

// maxExcavationTiles caps one order, so a stray drag across the whole map
// cannot escrow the treasury in one go. The browser reads it from the market
// topic and says so before sending.
const maxExcavationTiles = 400

// OrderExcavation asks the colony to have an area mined out: the rectangle with
// corners (X0, Y0) and (X1, Y1), inclusive, in either order. The outcome is
// logged, since a command has no reply.
type OrderExcavation struct{ X0, Y0, X1, Y1 int }

func (OrderExcavation) isCommand() {}

// orderExcavation marks out the rock the colony has seen inside the rectangle,
// funded by the treasury, and reports whether it did. It logs why not.
func (w *World) orderExcavation(c OrderExcavation) bool {
	x0, x1 := min(c.X0, c.X1), max(c.X0, c.X1)
	y0, y1 := min(c.Y0, c.Y1), max(c.Y0, c.Y1)
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = min(x1, w.Width-1), min(y1, w.Height-1)

	// Tiles another project already has a task on would be dug twice, and
	// paid twice.
	taken := make(map[Point]bool)
	for _, p := range w.projects {
		for _, t := range p.tasks {
			if !w.taskDone(t) {
				taken[t.pos] = true
			}
		}
	}
	var tasks []*buildTask
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			p := Point{x, y}
			if w.TerrainAt(p) != Rock || !w.discovered(p) || w.doorTiles[p] || taken[p] {
				continue
			}
			tasks = append(tasks, &buildTask{pos: p, terrain: Floor})
		}
	}
	if len(tasks) == 0 {
		w.logEvent(LogBuildStart, "There is no unmarked rock the colony has seen in that area to dig out.")
		return false
	}
	if len(tasks) > maxExcavationTiles {
		w.logEvent(LogBuildStart, fmt.Sprintf("That area is %d tiles of rock; an excavation order covers at most %d.", len(tasks), maxExcavationTiles))
		return false
	}

	w.nextProjectID++
	p := &project{id: w.nextProjectID, name: excavationName, queuedTick: w.tick, tasks: tasks,
		issuer: Community, workKind: WorkDig}
	for _, t := range tasks {
		t.proj = p
	}
	if !w.fundProject(p) {
		w.nextProjectID--
		w.logEvent(LogBuildStart, fmt.Sprintf("The treasury cannot pay %v to dig out %d tiles.", w.projectCost(p), len(tasks)))
		return false
	}
	w.projects = append(w.projects, p)
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony orders %d tiles of rock dug out, %v escrowed.", len(tasks), w.projectCost(p)))
	return true
}

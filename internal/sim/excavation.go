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

// ExcavationName is the project an excavation order creates. The Jobs tab
// lists it by this name, and it is how a dig is told from a room.
const ExcavationName = "excavation"

// maxExcavationTiles caps one order, so a stray drag across the whole map
// cannot escrow the treasury in one go. The browser reads it from the market
// topic and says so before sending.
const maxExcavationTiles = 400

// OrderExcavation asks the colony to have an area mined out: the rectangle with
// corners (X0, Y0) and (X1, Y1), inclusive, in either order, on Level. The outcome is
// logged, since a command has no reply.
type OrderExcavation struct {
	X0, Y0, X1, Y1 int
	Level          Level // the zero value is the landing level (orderLevel)
}

func (OrderExcavation) isCommand() {}

// CancelExcavation closes an excavation order by its project ID: the orders
// still open are closed and refunded to the treasury, and the rock not yet dug
// is left as it is. What was already dug, and paid for, stays so.
type CancelExcavation struct{ ID int }

func (CancelExcavation) isCommand() {}

// cancelExcavation drops the excavation project with this ID and refunds what
// its open orders still hold, and reports whether there was one. Only an
// excavation can be cancelled this way: a room is the planner's, not an order.
func (w *World) cancelExcavation(id int) bool {
	return w.cancelOrdered(id, ExcavationName, "an excavation", "dug")
}

// cancelOrdered drops the project with this ID if it is one the player
// ordered under name (an excavation or a clearing), refunding what its open
// orders still hold, and reports whether there was one.
func (w *World) cancelOrdered(id int, name, what, doneVerb string) bool {
	for _, p := range w.projects {
		if p.id != id || p.name != name {
			continue
		}
		done, refunded := 0, Money(0)
		for _, t := range p.tasks {
			if t.order != nil && w.workOrders[t.order.ID] == t.order {
				refunded += t.order.escrow
			}
			if w.taskDone(t) {
				done++
			}
		}
		// A worker on its way drops the job, so it is not paid for a task
		// that no longer exists.
		w.cancelProject(p)
		w.logEvent(LogBuildStart, fmt.Sprintf("The colony cancels %s of %d tiles, %d already %s, and takes back %v.",
			what, len(p.tasks), done, doneVerb, refunded))
		return true
	}
	return false
}

// cancelClearing closes a clearing order (see zones.go) as cancelExcavation
// closes a dig.
func (w *World) cancelClearing(id int) bool {
	return w.cancelOrdered(id, ClearingName, "a clearing", "cleared")
}

// orderExcavation marks out the rock the colony has seen inside the rectangle,
// funded by the treasury, and reports whether it did. It logs why not.
func (w *World) orderExcavation(c OrderExcavation) bool {
	x0, y0, x1, y1 := w.clampRect(c.X0, c.Y0, c.X1, c.Y1)

	// Tiles another project already has a task on would be dug twice, and
	// paid twice (unmarkedRock leaves them out).
	rock := w.unmarkedRock(orderLevel(c.Level), x0, y0, x1, y1)
	if len(rock) == 0 {
		w.logEvent(LogBuildStart, "There is no unmarked rock the colony has seen in that area to dig out.")
		return false
	}
	if len(rock) > maxExcavationTiles {
		w.logEvent(LogBuildStart, fmt.Sprintf("That area is %d tiles of rock; an excavation order covers at most %d.", len(rock), maxExcavationTiles))
		return false
	}
	p := w.startExcavation(rock)
	if p == nil {
		w.logEvent(LogBuildStart, fmt.Sprintf("The treasury cannot pay %v to dig out %d tiles.",
			Money(len(rock))*w.wageFor(Floor), len(rock)))
		return false
	}
	w.logEvent(LogBuildStart, fmt.Sprintf("The colony orders %d tiles of rock dug out, %v escrowed.", len(rock), w.projectCost(p)))
	return true
}

// startExcavation posts an excavation project digging out rock, funded by
// the treasury, and returns it, or nil if the treasury cannot pay for all of
// it. A zone painted over rock uses it too (see paintZone); only the dig
// tool's own orders are capped at maxExcavationTiles.
func (w *World) startExcavation(rock []Point) *project {
	if len(rock) == 0 {
		return nil
	}
	w.nextProjectID++
	p := &project{id: w.nextProjectID, name: ExcavationName, queuedTick: w.tick,
		issuer: Community, workKind: WorkDig}
	for _, pos := range rock {
		p.tasks = append(p.tasks, &buildTask{pos: pos, terrain: Floor, proj: p})
	}
	if !w.fundProject(p) {
		w.nextProjectID--
		return nil
	}
	w.projects = append(w.projects, p)
	return p
}

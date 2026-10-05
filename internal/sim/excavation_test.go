package sim

import "testing"

// frontierRect is a small rectangle starting at the first frontier rock (row
// major), so some of it can be reached from the colony at once.
func frontierRect(t *testing.T, w *World) OrderExcavation {
	t.Helper()
	var first Point
	found := false
	for p := range w.board.frontier {
		if w.discovered(p) && (!found || lessPoint(p, first)) {
			first, found = p, true
		}
	}
	if !found {
		t.Fatal("no explored frontier rock to dig")
	}
	return OrderExcavation{X0: first.X, Y0: first.Y, X1: first.X + 2, Y1: first.Y + 2}
}

func rockIn(w *World, c OrderExcavation) int {
	n := 0
	for y := c.Y0; y <= c.Y1; y++ {
		for x := c.X0; x <= c.X1; x++ {
			if p := (Point{x, y}); w.TerrainAt(p) == Rock && w.discovered(p) && !w.doorTiles[p] {
				n++
			}
		}
	}
	return n
}

// An excavation order is bought from the treasury as work orders on the book,
// and the colonists who dig the area out are paid for it.
func TestAnExcavationOrderIsPaidFromTheTreasuryAndDug(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	c := frontierRect(t, w)
	tiles := rockIn(w, c)
	before := w.treasury
	if !w.orderExcavation(c) {
		t.Fatalf("order refused; log: %v", w.log.tail(1))
	}
	p := w.projects[len(w.projects)-1]
	if p.name != ExcavationName || p.issuer != Community || len(p.tasks) != tiles {
		t.Fatalf("project %q issuer %v with %d tasks, want %d tiles", p.name, p.issuer, len(p.tasks), tiles)
	}
	cost := Money(tiles) * w.wageFor(Floor)
	if w.treasury != before-cost || w.workEscrowed() != cost {
		t.Fatalf("treasury %v (was %v), escrow %v, cost %v", w.treasury, before, w.workEscrowed(), cost)
	}
	digs := w.sortedWork(func(o *WorkOrder) bool { return o.Kind == WorkDig })
	if len(digs) != tiles {
		t.Fatalf("%d dig orders on the book, want %d", len(digs), tiles)
	}
	assertMoneyConserved(t, w)

	for i := 0; i < 6000 && w.workEscrowed() > 0; i++ {
		w.step()
	}
	if w.workEscrowed() > 0 {
		t.Fatalf("tick %d: %v still escrowed for digging", w.tick, w.workEscrowed())
	}
	for _, task := range p.tasks {
		if w.TerrainAt(task.pos) == Rock {
			t.Fatalf("tick %d: %v is still rock", w.tick, task.pos)
		}
	}
	if left := len(w.sortedWork(func(o *WorkOrder) bool { return o.Kind == WorkDig })); left != 0 {
		t.Fatalf("%d dig orders still open", left)
	}
	assertMoneyConserved(t, w)
}

// Ordering the same area twice does not buy it twice.
func TestAnExcavationOrderSkipsTilesAlreadyOrdered(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	c := frontierRect(t, w)
	if !w.orderExcavation(c) {
		t.Fatal("first order refused")
	}
	spent := w.workEscrowed()
	if w.orderExcavation(c) {
		t.Fatal("the same area was bought twice")
	}
	if w.workEscrowed() != spent {
		t.Fatalf("escrow %v after a refused order, was %v", w.workEscrowed(), spent)
	}
}

// A treasury that cannot cover the whole area buys none of it.
func TestAnExcavationOrderIsAllOrNothing(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	c := frontierRect(t, w)
	// Leave the treasury one tile's worth, moving the rest to a colonist so
	// the money audit still balances.
	w.transfer(Community, ColonistOwner(w.entityIDsSorted()[0]), w.treasury-w.wageFor(Floor))
	if rockIn(w, c) < 2 {
		t.Skip("rectangle has fewer than two rock tiles")
	}
	projects := len(w.projects)
	if w.orderExcavation(c) {
		t.Fatal("an empty treasury bought an excavation")
	}
	if len(w.projects) != projects || w.workEscrowed() != 0 {
		t.Fatalf("%d projects (was %d), escrow %v", len(w.projects), projects, w.workEscrowed())
	}
	assertMoneyConserved(t, w)
}

// One order covers a bounded area, and an area with nothing to dig is refused.
func TestAnExcavationOrderIsBounded(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	// Reveal a broad patch around the centre, so there is plenty of seen rock.
	cx, cy := w.Width/2, w.Height/2
	box := OrderExcavation{X0: cx - 40, Y0: cy - 40, X1: cx + 40, Y1: cy + 40}
	rock := 0
	for y := box.Y0; y <= box.Y1; y++ {
		for x := box.X0; x <= box.X1; x++ {
			w.reveal(Point{x, y})
		}
	}
	rock = rockIn(w, box)
	if rock <= maxExcavationTiles {
		t.Skipf("only %d rock tiles on the test map", rock)
	}
	if w.orderExcavation(box) {
		t.Fatalf("the whole map was ordered dug (%d tiles max)", maxExcavationTiles)
	}
	open := Point{}
	for y := 0; y < w.Height && open == (Point{}); y++ {
		for x := 0; x < w.Width; x++ {
			if w.TerrainAt(Point{x, y}) == Floor {
				open = Point{x, y}
				break
			}
		}
	}
	if w.orderExcavation(OrderExcavation{X0: open.X, Y0: open.Y, X1: open.X, Y1: open.Y}) {
		t.Fatal("an open floor tile was ordered dug")
	}
}

// Cancelling an excavation refunds what its open orders hold, drops the
// project, and leaves what was already dug and paid for alone.
func TestCancellingAnExcavationRefundsTheTreasury(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	c := frontierRect(t, w)
	before := w.treasury
	if !w.orderExcavation(c) {
		t.Fatal("order refused")
	}
	p := w.projects[len(w.projects)-1]
	// Only this project's escrow: the planner may mark out a room meanwhile,
	// since an excavation does not take a room's place (roomProjects).
	held := func() Money {
		var m Money
		for _, task := range p.tasks {
			if o := task.order; o != nil && w.workOrders[o.ID] == o {
				m += o.escrow
			}
		}
		return m
	}
	for i := 0; i < 300 && held() == w.projectCost(p); i++ {
		w.step() // until somebody has dug and been paid for a tile
	}
	paid := w.projectCost(p) - held()
	others := w.workEscrowed() - held()
	treasury := w.treasury
	if !w.cancelExcavation(p.id) {
		t.Fatal("cancel refused")
	}
	for _, q := range w.projects {
		if q == p {
			t.Fatal("the project is still open")
		}
	}
	if w.workEscrowed() != others || w.treasury != treasury+w.projectCost(p)-paid {
		t.Fatalf("escrow %v (others hold %v), treasury %v; want %v (paid out %v, ordered at %v)",
			w.workEscrowed(), others, w.treasury, treasury+w.projectCost(p)-paid, paid, before)
	}
	assertMoneyConserved(t, w)
	if w.cancelExcavation(p.id) {
		t.Fatal("cancelled twice")
	}
	for i := 0; i < 200; i++ {
		w.step() // nobody is left holding a task that is gone
	}
	assertMoneyConserved(t, w)
}

// Only an excavation is cancelled this way: a room is the planner's.
func TestCancelLeavesRoomsAlone(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)
	w.planRooms()
	if len(w.projects) == 0 {
		t.Fatal("nothing planned")
	}
	if w.cancelExcavation(w.projects[0].id) {
		t.Fatal("a room was cancelled as an excavation")
	}
}

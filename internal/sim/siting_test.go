package sim

import (
	"strings"
	"testing"
)

// lastLog is the newest log line's text.
func lastLog(w *World) string {
	if l := w.log.tail(1); len(l) == 1 {
		return l[0].Text
	}
	return ""
}

// Without siting-auto, an order with no tile is refused in the log and
// queues nothing; with it, the order waits for the planner as it always did.
func TestUnsitedOrdersNeedSitingAuto(t *testing.T) {
	orders := []Command{OrderStair{}, OrderShaft{Levels: 2}, OrderHole{}, OrderLadder{}}
	w := shaftColony(t, 1)
	for _, o := range orders {
		w.orderDigDown(o)
		if !strings.HasPrefix(lastLog(w), "Pick a tile for the ") {
			t.Errorf("%T without a tile: log %q", o, lastLog(w))
		}
	}
	if w.manualStairs+w.manualShaftLevels+w.manualHoles+w.manualLadders != 0 || len(w.projects) != 0 {
		t.Fatal("an unsited order was queued with siting-auto off")
	}
	w.cfg.SitingAuto = true
	for _, o := range orders {
		w.orderDigDown(o)
	}
	if w.manualStairs != 1 || w.manualShaftLevels != 2 || w.manualHoles != 1 || w.manualLadders != 1 {
		t.Fatalf("queued stairs %d, shaft levels %d, holes %d, ladders %d; want 1, 2, 1, 1",
			w.manualStairs, w.manualShaftLevels, w.manualHoles, w.manualLadders)
	}
}

// A picked tile is marked out at once for each kind, and refused, changing
// nothing, wherever that kind cannot go.
func TestPickedSitesAreChecked(t *testing.T) {
	w := shaftColony(t, 1)
	site := func() Point {
		t.Helper()
		p, ok := w.findStairSite(LandingLevel) // skips tiles already marked
		if !ok {
			t.Fatal("no open floor left to pick")
		}
		return p
	}
	stair, shaft, hole := site(), Point{}, Point{}
	w.orderDigDown(OrderStair{At: stair})
	shaft = site()
	w.orderDigDown(OrderShaft{At: shaft, Levels: 5}) // cut short at deepest-level
	hole = site()
	w.orderDigDown(OrderHole{At: hole})
	for _, c := range []struct {
		terrain Terrain
		at      Point
		depth   int
	}{{StairDown, stair, 0}, {ShaftTop, shaft, 2}, {Hole, hole, 0}} {
		found := false
		for _, p := range w.projects {
			for _, tk := range p.tasks {
				found = found || (tk.terrain == c.terrain && tk.pos == c.at && tk.depth == c.depth)
			}
		}
		if !found {
			t.Errorf("no %v task at %v (depth %d); log %q", c.terrain, c.at, c.depth, lastLog(w))
		}
	}

	rock := stair
	for w.TerrainAt(rock) != Rock {
		rock.X++
	}
	unseen := Point{0, 0, LandingLevel}
	if w.discovered(unseen) {
		t.Fatal("the map's corner is discovered: pick another unseen tile")
	}
	below := Point{stair.X, stair.Y, LandingLevel + 1}
	for _, c := range []struct {
		name  string
		order Command
	}{
		{"a stair on rock", OrderStair{At: rock}},
		{"a hole on an unseen tile", OrderHole{At: unseen}},
		{"a shaft off the map", OrderShaft{At: Point{-1, 0, LandingLevel}, Levels: 1}},
		{"a stair on a level not broken into", OrderStair{At: below}},
		{"a second order on a marked tile", OrderHole{At: stair}},
		{"a ladder into floor", OrderLadder{At: rock.Add(-1, 0)}},
		{"a ladder into a hole not dug yet", OrderLadder{At: hole}},
	} {
		before := len(w.projects)
		w.orderDigDown(c.order)
		if len(w.projects) != before || !strings.HasPrefix(lastLog(w), "No ") {
			t.Errorf("%s: %d projects, was %d; log %q", c.name, len(w.projects), before, lastLog(w))
		}
	}

	// A level deeper than deepest-level allows.
	cfg := w.cfg
	cfg.DeepestLevel = 1
	shallow := newTestWorld(t, cfg)
	shallow.orderDigDown(OrderStair{At: stair})
	if len(shallow.projects) != 0 || !strings.Contains(lastLog(shallow), "may not dig below level 1") {
		t.Errorf("a stair below deepest-level: log %q", lastLog(shallow))
	}
}

// Picking a shaft's top deepens it, and picking a dug hole fits it a ladder.
func TestPickedShaftDeepensAndLadderFits(t *testing.T) {
	w := shaftColony(t, 1)
	top, ok := w.findStairSite(LandingLevel)
	if !ok || !w.digShaft(top, LandingLevel+1) {
		t.Fatal("could not sink a shaft to deepen")
	}
	h, ok := w.findStairSite(LandingLevel)
	if !ok || !w.digHole(h) {
		t.Fatal("could not dig a hole to ladder")
	}
	w.refreshSpatial()
	w.orderDigDown(OrderShaft{At: top, Levels: 1})
	w.orderDigDown(OrderLadder{At: h})
	var deepen, ladder bool
	for _, p := range w.projects {
		for _, tk := range p.tasks {
			deepen = deepen || (tk.terrain == ShaftTop && tk.pos == top && shaftTaskBottom(tk) == LandingLevel+2)
			ladder = ladder || (tk.terrain == ShaftTop && tk.pos == h && tk.depth == 1)
		}
	}
	if !deepen || !ladder {
		t.Fatalf("deepening planned %v, ladder planned %v; log %v", deepen, ladder, w.log.tail(3))
	}
}

// Digging a stair down unasked, when there is no rock left to mine, is
// siting one: the colony does it only with siting-auto.
func TestColonyDigsDownUnaskedOnlyWithSitingAuto(t *testing.T) {
	for _, auto := range []bool{false, true} {
		cfg := testConfig()
		cfg.Width, cfg.Height = 40, 20
		cfg.DeepestLevel = 3
		cfg.SitingAuto = auto
		w := newWorld(cfg, newPCG(1))
		// A walled room: floor with no rock beside it, so nothing to mine.
		carveOn(w, LandingLevel, Point{4, 4, 0}, Point{16, 12, 0}, Wall)
		carveOn(w, LandingLevel, Point{5, 5, 0}, Point{15, 11, 0}, Floor)
		w.spawn(Colonist, Point{8, 6, LandingLevel})
		w.refreshSpatial()
		if w.unclaimedFrontier() != 0 {
			t.Fatalf("frontier %d: the test wants none", w.unclaimedFrontier())
		}
		w.planStairs()
		if got := w.taskPlanned(StairDown); got != auto {
			t.Errorf("siting-auto %v: stair planned %v", auto, got)
		}
	}
}

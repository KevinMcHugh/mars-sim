package sim

import (
	"bytes"
	"testing"
)

// holeWorld is an all-rock world three levels deep with no generation: a
// room on the landing level with a hole broken through at (10, 8).
//
//	level 1: floor x 3..20, y 4..12, hole at (10, 8)
//	level 2: the one open tile the hole lands on, (10, 8)
func holeWorld(t *testing.T) (w *World, hole Point) {
	t.Helper()
	cfg := testConfig()
	cfg.Width, cfg.Height = 40, 20
	cfg.DeepestLevel = 3
	w = newWorld(cfg, newPCG(1))
	carveOn(w, LandingLevel, Point{3, 4, 0}, Point{20, 12, 0}, Floor)
	hole = Point{10, 8, LandingLevel}
	if !w.digHole(hole) {
		t.Fatal("digHole refused open floor on the landing level")
	}
	w.refreshSpatial()
	return w, hole
}

func TestDigHoleOpensTheLevelBelow(t *testing.T) {
	w, hole := holeWorld(t)
	land := Point{10, 8, LandingLevel + 1}
	if w.TerrainAt(hole) != Hole || w.TerrainAt(land) != Floor {
		t.Fatalf("hole is %v over %v, want a hole over floor", w.TerrainAt(hole), w.TerrainAt(land))
	}
	if w.Walkable(hole) {
		t.Error("a hole is walkable")
	}
	if _, n := w.links(hole); n != 0 {
		t.Error("a hole is a link")
	}
	if _, n := w.links(land); n != 0 {
		t.Error("the foot of a hole is a link")
	}
	if w.sameRoom(Point{9, 8, LandingLevel}, land) {
		t.Error("a hole joins the rooms above and below: nothing comes back up it")
	}
	if got, levels, ok := w.fallTarget(hole); !ok || got != land || levels != 1 {
		t.Errorf("fallTarget = %v, %d, %v; want %v one level down", got, levels, ok, land)
	}
	if len(w.holes) != 1 || w.holes[0] != hole {
		t.Errorf("holes = %v, want [%v]", w.holes, hole)
	}
	if w.canDigHole(Point{3, 4, LandingLevel + 2}) {
		t.Error("a hole could be dug below deepest-level")
	}
}

// A fall lands on the floor below, legs first, a leg per level.
func TestFallHurtsLegsFirst(t *testing.T) {
	w, hole := holeWorld(t)
	// A second hole straight under the first: two levels to fall.
	if !w.digHole(Point{10, 8, LandingLevel + 1}) {
		t.Fatal("could not dig the second hole")
	}
	e := w.spawn(Colonist, Point{9, 8, LandingLevel})
	hp := e.HP
	w.moveEntity(e, hole) // as if the floor gave way under it
	if !w.fall(e) {
		t.Fatal("a two-level fall killed a healthy colonist")
	}
	if want := (Point{10, 8, LandingLevel + 2}); e.Pos != want {
		t.Fatalf("landed at %v, want %v", e.Pos, want)
	}
	d := w.cfg.FallDamage
	left, right := e.MaxParts[LeftLeg]-e.Parts[LeftLeg], e.MaxParts[RightLeg]-e.Parts[RightLeg]
	if left != min(d, e.MaxParts[LeftLeg]) || right != min(d, e.MaxParts[RightLeg]) || hp-e.HP != 2*d {
		t.Errorf("legs lost %d and %d, HP %d; want a level's damage to each leg and %d in all", left, right, hp-e.HP, 2*d)
	}
	if w.entityAt(hole) != nil || w.entityAt(e.Pos) != e {
		t.Error("occupancy lost track of the faller")
	}
}

// A fall can kill, leaving the body where it landed; a crowded landing puts
// the faller on the nearest free tile.
func TestFallKillsAndCrowds(t *testing.T) {
	w, hole := holeWorld(t)
	land := Point{10, 8, LandingLevel + 1}
	carveOn(w, LandingLevel+1, Point{9, 7, 0}, Point{11, 9, 0}, Floor)
	w.refreshSpatial()
	w.spawn(Rat, land)
	e := w.spawn(Colonist, Point{9, 8, LandingLevel})
	w.moveEntity(e, hole)
	if !w.fall(e) || e.Pos.Level != LandingLevel+1 || e.Pos == land || !e.Pos.Adjacent(land) {
		t.Fatalf("landed at %v beside a rat on %v", e.Pos, land)
	}
	w.cfg.FallDamage = 1000
	f := w.spawn(Colonist, Point{9, 8, LandingLevel})
	id := f.ID
	w.moveEntity(f, hole)
	if w.fall(f) || w.entities[id] != nil {
		t.Fatal("survived a fatal fall")
	}
	if w.corpsesAt(land)+w.corpsesAt(e.Pos)+refuseAround(w, land) == 0 {
		t.Error("no body where the faller landed")
	}
}

func refuseAround(w *World, p Point) int {
	n := 0
	for _, d := range neighbors8 {
		n += w.corpsesAt(p.Add(d.X, d.Y))
	}
	return n
}

// A colonist cornered by a threat leaps down an open hole beside it, unless
// the drop would kill it.
func TestCorneredColonistLeaps(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 30, 15
	cfg.DeepestLevel = 2
	w := newWorld(cfg, newPCG(1))
	carveOn(w, LandingLevel, Point{3, 5, 0}, Point{8, 5, 0}, Floor) // a dead-end corridor
	carveOn(w, LandingLevel, Point{3, 6, 0}, Point{3, 6, 0}, Floor)
	if !w.digHole(Point{3, 6, LandingLevel}) {
		t.Fatal("could not dig the hole")
	}
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{3, 5, LandingLevel})
	threat := Point{4, 5, LandingLevel}
	w.cfg.FallDamage = e.HP // would kill: stays put
	w.fleeStep(e, threat)
	if e.Pos.Level != LandingLevel {
		t.Fatal("leapt to its death")
	}
	w.cfg.FallDamage = 5
	w.fleeStep(e, threat)
	if e.Pos != (Point{3, 6, LandingLevel + 1}) {
		t.Fatalf("cornered colonist is at %v, want down the hole", e.Pos)
	}
}

// Refuse goes down a hole when it is nearer than any incinerator: bodies as
// bodies, viscera as gore, on the floor below.
func TestHoleIsAChute(t *testing.T) {
	w, hole := holeWorld(t)
	land := Point{10, 8, LandingLevel + 1}
	e := w.spawn(Colonist, Point{9, 8, LandingLevel})
	e.Inventory.Add(AnimalCorpse, 1)
	e.Inventory.Add(Viscera, 2)
	if burn, _ := w.refuseDestinations(e); !burn {
		t.Fatal("a hole does not count as somewhere to take refuse")
	}
	if p, ok := w.haulTarget(e); !ok || p != hole {
		t.Fatalf("haulTarget = %v, %v; want the hole", p, ok)
	}
	w.tipDown(e, hole)
	if carryingRefuse(e) {
		t.Error("still carrying refuse after tipping it down")
	}
	if w.corpsesOfAt(land, AnimalCorpse) != 1 || w.goreAt(land) != 2 {
		t.Errorf("below: %d bodies, %d gore; want 1 and 2", w.corpsesOfAt(land, AnimalCorpse), w.goreAt(land))
	}
	w.cfg.HoleChutes = false
	e.Inventory.Add(Viscera, 1)
	if burn, _ := w.refuseDestinations(e); burn {
		t.Error("with hole-chutes off, a hole still takes refuse")
	}
}

// Someone stranded below a hole is fetched back: the colony fits a ladder,
// and the hole becomes a shaft joining them to the colony.
func TestStrandedColonistGetsALadder(t *testing.T) {
	w, hole := holeWorld(t)
	for _, x := range []int{4, 6, 8} {
		w.spawn(Colonist, Point{x, 5, LandingLevel})
	}
	below := w.spawn(Colonist, Point{9, 8, LandingLevel})
	w.moveEntity(below, hole)
	if !w.fall(below) {
		t.Fatal("the fall killed")
	}
	w.refreshSpatial()
	if w.mainRoom == 0 || w.sameRoom(below.Pos, Point{4, 5, LandingLevel}) {
		t.Fatal("the faller is not cut off")
	}
	w.planLadders()
	if !w.taskPlanned(ShaftTop) {
		t.Fatal("no ladder planned for the stranded colonist")
	}
	if !w.digShaft(hole, hole.Level+1) {
		t.Fatal("could not fit the ladder")
	}
	w.refreshSpatial()
	if w.TerrainAt(hole) != ShaftTop || !w.sameRoom(below.Pos, Point{4, 5, LandingLevel}) {
		t.Errorf("after the ladder: %v at the hole, rejoined %v", w.TerrainAt(hole),
			w.sameRoom(below.Pos, Point{4, 5, LandingLevel}))
	}
	if len(w.holes) != 0 {
		t.Errorf("holes = %v after the ladder", w.holes)
	}
}

// The whole path a player sees: pick a tile and order a hole there, the
// colony digs it, order a ladder, the colony fits it; the run is
// deterministic and survives a save.
func TestOrderedHoleAndLadder(t *testing.T) {
	run := func() (*World, string) {
		w := shaftColony(t, 4)
		site, ok := w.findStairSite(LandingLevel) // somewhere a player might pick
		if !ok {
			t.Fatal("no open floor to order a hole on")
		}
		if !w.orderHole(site) {
			t.Fatalf("hole at %v refused: %s", site, w.holeRefusal(site))
		}
		runUntil(t, w, 3000, "a hole", func() bool { return len(w.holes) > 0 })
		h := w.holes[0]
		if h != site || w.TerrainAt(Point{h.X, h.Y, h.Level + 1}) != Floor {
			t.Fatalf("hole at %v, ordered at %v, below %v", h, site, w.TerrainAt(Point{h.X, h.Y, h.Level + 1}))
		}
		w.manualLadders = 1
		runUntil(t, w, 3000, "a ladder", func() bool { return w.TerrainAt(h) == ShaftTop })
		if w.TerrainAt(Point{h.X, h.Y, h.Level + 1}) != ShaftBottom {
			t.Fatal("the ladder has no foot")
		}
		checkRoomLabelsAllLevels(t, w)
		return w, levelsHash(w)
	}
	w, a := run()
	if _, b := run(); a != b {
		t.Fatalf("runs diverge:\n%s\n%s", a, b)
	}
	loaded := saveAndLoad(t, w)
	for i := 0; i < 300; i++ {
		w.step()
		loaded.step()
	}
	if !bytes.Equal(encodeWorld(t, w), encodeWorld(t, loaded)) {
		t.Fatalf("loaded game's state differs: %s", saveDiff(w, loaded))
	}
}

// After a one-level shaft the deepest level is only the shaft's foot, with
// no site on it. The stair and shaft planners' site search must stop at the
// main room's extent there, not sweep the whole map every planning round
// (which froze the browser's 10000x10000 game).
func TestSiteSearchStopsAtTheMainRoom(t *testing.T) {
	w := shaftColony(t, 1)
	w.manualShaftLevels = 1
	runUntil(t, w, 4000, "a shaft", func() bool { return len(w.shafts) > 0 })
	foot := Point{w.shafts[0].X, w.shafts[0].Y, LandingLevel + 1}
	x0, y0, x1, y1, ok := w.mainRoomBounds(foot.Level)
	if !ok || x1-x0 != chunkSize || y1-y0 != chunkSize || foot.X < x0 || foot.X >= x1 || foot.Y < y0 || foot.Y >= y1 {
		t.Fatalf("main room bounds on level %d = [%d,%d)x[%d,%d) %v, want the chunk holding %v",
			foot.Level, x0, x1, y0, y1, ok, foot)
	}
	if site, ok := w.findStairSite(foot.Level); ok {
		t.Fatalf("site %v on a level that is only a shaft's foot", site)
	}
	if _, _, _, _, ok := w.mainRoomBounds(LandingLevel + 2); ok {
		t.Error("the main room reaches a level nobody has broken into")
	}
}

// A hole goes where the player says, or nowhere: each tile that cannot take
// one is refused, changing nothing, and a good one is marked out at once.
func TestOrderHoleChecksTheTile(t *testing.T) {
	w := shaftColony(t, 1)
	good, ok := w.findStairSite(LandingLevel)
	if !ok {
		t.Fatal("no open floor to order a hole on")
	}
	rock := good
	for w.TerrainAt(rock) != Rock {
		rock.X++
	}
	unseen := Point{0, 0, LandingLevel}
	if w.discovered(unseen) {
		t.Fatal("the map's corner is discovered: pick another unseen tile")
	}
	shallow := w.cfg
	shallow.DeepestLevel = 1
	for _, c := range []struct {
		name string
		w    *World
		p    Point
	}{
		{"rock", w, rock},
		{"unseen", w, unseen},
		{"off the map", w, Point{-1, 0, LandingLevel}},
		{"a level not broken into", w, Point{good.X, good.Y, LandingLevel + 1}},
		{"too deep", newTestWorld(t, shallow), good},
	} {
		before := len(c.w.projects)
		if c.w.orderHole(c.p) || len(c.w.projects) != before {
			t.Errorf("%s (%v): a hole was marked out", c.name, c.p)
		}
	}
	if !w.orderHole(good) || !w.taskPlanned(Hole) {
		t.Fatalf("hole at %v refused: %s", good, w.holeRefusal(good))
	}
	if w.orderHole(good) {
		t.Error("the same tile took a second hole")
	}
}

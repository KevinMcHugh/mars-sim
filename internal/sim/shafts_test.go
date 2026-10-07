package sim

import (
	"bytes"
	"container/heap"
	"fmt"
	"testing"
)

// shaftWorld is an all-rock world three levels deep with no generation, and
// a shaft from the landing level to the bottom:
//
//	level 1: floor x 5..20, y 5..12, shaft top at (15, 8)
//	level 2: floor x 16..25, y 6..10, the shaft passing through at (15, 8)
//	level 3: floor x 16..35, y 6..10, the shaft's foot at (15, 8)
func shaftWorld(t *testing.T) (w *World, top Point) {
	t.Helper()
	cfg := testConfig()
	cfg.Width, cfg.Height = 60, 30
	cfg.DeepestLevel = 3
	w = newWorld(cfg, newPCG(1))
	carveOn(w, LandingLevel, Point{5, 5, 0}, Point{20, 12, 0}, Floor)
	top = Point{15, 8, LandingLevel}
	if !w.digShaft(top, LandingLevel+2) {
		t.Fatal("digShaft refused open floor on the landing level")
	}
	carveOn(w, LandingLevel+1, Point{16, 6, 0}, Point{25, 10, 0}, Floor)
	carveOn(w, LandingLevel+2, Point{16, 6, 0}, Point{35, 10, 0}, Floor)
	w.refreshSpatial()
	return w, top
}

func TestDigShaftMakesTheColumn(t *testing.T) {
	w, top := shaftWorld(t)
	mid, foot := Point{15, 8, LandingLevel + 1}, Point{15, 8, LandingLevel + 2}
	for p, want := range map[Point]Terrain{top: ShaftTop, mid: ShaftMid, foot: ShaftBottom} {
		if got := w.TerrainAt(p); got != want {
			t.Errorf("%v is %v, want %v", p, got, want)
		}
	}
	for p, want := range map[Point][]Point{top: {mid}, mid: {top, foot}, foot: {mid}} {
		lk, n := w.links(p)
		if n != len(want) {
			t.Fatalf("%v has %d links, want %d", p, n, len(want))
		}
		for i, q := range want {
			if lk[i].to != q || !lk[i].shaft || lk[i].cost != w.shaftCost() {
				t.Errorf("%v link %d = %+v, want a shaft to %v costing %d", p, i, lk[i], q, w.shaftCost())
			}
			if !linked(w, q, p) {
				t.Errorf("%v links to %v but not back", p, q)
			}
		}
	}
	if fmt.Sprint(w.shafts) != fmt.Sprint([]Point{top, mid}) {
		t.Errorf("shafts = %v, want the top and the middle", w.shafts)
	}
	if w.shaftBottom(top) != LandingLevel+2 {
		t.Errorf("shaftBottom = %d, want %d", w.shaftBottom(top), LandingLevel+2)
	}
	if !w.sameRoom(Point{6, 6, LandingLevel}, Point{34, 9, LandingLevel + 2}) {
		t.Error("the shaft does not join the landing level to the bottom")
	}
	if !w.sameRoom(Point{24, 9, LandingLevel + 1}, Point{34, 9, LandingLevel + 2}) {
		t.Error("the level the shaft passes through is not joined to it")
	}
	checkRoomLabelsAllLevels(t, w)
	if !w.discovered(foot.Add(1, 1)) {
		t.Error("breaking in at the foot did not reveal around it")
	}
}

// A shaft never cuts through a structure on a level it passes, and deepening
// one extends the column it already has.
func TestShaftDiggingRules(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 40, 20
	cfg.DeepestLevel = 4
	w := newWorld(cfg, newPCG(1))
	carveOn(w, LandingLevel, Point{5, 5, 0}, Point{15, 12, 0}, Floor)
	top := Point{10, 8, LandingLevel}
	if w.canDigShaft(top, LandingLevel) || w.canDigShaft(top, 5) {
		t.Error("a shaft no levels deep, or deeper than deepest-level, was allowed")
	}
	if !w.digShaft(top, LandingLevel+1) {
		t.Fatal("could not dig one level")
	}
	w.addLayer(LandingLevel + 3)
	w.SetTerrain(Point{10, 8, LandingLevel + 3}, Wall) // a structure in the way, two levels further down
	if w.canDigShaft(top, LandingLevel+3) {
		t.Error("the shaft would cut through a wall")
	}
	if !w.digShaft(top, LandingLevel+2) {
		t.Fatal("could not deepen the shaft by a level")
	}
	if got := w.TerrainAt(Point{10, 8, LandingLevel + 1}); got != ShaftMid {
		t.Errorf("the old foot is %v, want a shaft middle", got)
	}
	if got := w.TerrainAt(Point{10, 8, LandingLevel + 2}); got != ShaftBottom {
		t.Errorf("the new foot is %v, want a shaft bottom", got)
	}
	if w.canDigShaft(top, LandingLevel+2) {
		t.Error("deepening to where the shaft already reaches was allowed")
	}
}

// The fields weigh a climb: every tile's distance matches a weighted
// shortest-path search over the same tiles and links.
func TestFieldWeighsTheClimb(t *testing.T) {
	for _, climb := range []int{1, 8, 30} {
		t.Run(fmt.Sprint(climb), func(t *testing.T) {
			w, _ := shaftWorld(t)
			w.cfg.ShaftClimbTicks = climb
			w.SetTerrain(Point{5, 5, LandingLevel}, Toilet)
			w.refreshSpatial()
			f := w.facilityField(Toilet)
			want := referenceField(w, facilitySeed(w, Toilet))
			for _, l := range w.layers {
				if l == nil {
					continue
				}
				for y := 0; y < w.Height; y++ {
					for x := 0; x < w.Width; x++ {
						p := Point{x, y, l.Level}
						d, ok := want[p]
						if !ok {
							d = -1
						}
						if got := f.at(p); got != d {
							t.Fatalf("field at %v = %d, want %d", p, got, d)
						}
					}
				}
			}
			far := Point{35, 8, LandingLevel + 2}
			if f.at(far) < int32(2*climb) {
				t.Errorf("the far end of the bottom level is %d from the toilet, less than two climbs", f.at(far))
			}
		})
	}
}

// referenceField is a plain Dijkstra over walkable tiles, a step to each of
// eight neighbours and a link's cost through each link: what a flow field
// should hold.
func referenceField(w *World, seed func(func(Point))) map[Point]int32 {
	dist := map[Point]int32{}
	h := &refHeap{}
	seed(func(p Point) {
		if w.Walkable(p) {
			if _, ok := dist[p]; !ok {
				dist[p] = 0
				heap.Push(h, refItem{p, 0})
			}
		}
	})
	for h.Len() > 0 {
		it := heap.Pop(h).(refItem)
		if it.d != dist[it.p] {
			continue
		}
		relax := func(q Point, c int32) {
			if !w.Walkable(q) {
				return
			}
			if d, ok := dist[q]; !ok || it.d+c < d {
				dist[q] = it.d + c
				heap.Push(h, refItem{q, it.d + c})
			}
		}
		for _, d := range neighbors8 {
			relax(it.p.Add(d.X, d.Y), 1)
		}
		lk, n := w.links(it.p)
		for _, k := range lk[:n] {
			relax(k.to, k.cost)
		}
	}
	return dist
}

type refItem struct {
	p Point
	d int32
}
type refHeap []refItem

func (h refHeap) Len() int { return len(h) }
func (h refHeap) Less(i, j int) bool {
	return h[i].d < h[j].d || h[i].d == h[j].d && lessPoint(h[i].p, h[j].p)
}
func (h refHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *refHeap) Push(x any)   { *h = append(*h, x.(refItem)) }
func (h *refHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// A colonist sent to the bottom climbs down, and the climb takes its time:
// it spends its turns on the ladder, in the Climbing state.
func TestTravelClimbsTheShaft(t *testing.T) {
	w, top := shaftWorld(t)
	e := w.spawn(Colonist, Point{12, 8, LandingLevel})
	target := Point{30, 8, LandingLevel + 2}
	ticks, climbed := 0, 0
	for ; ticks < 500; ticks++ {
		w.tick++
		if w.climbing(e) {
			climbed++
			if e.State != Climbing {
				t.Fatalf("on the ladder in state %v", e.State)
			}
			continue
		}
		arrived, ok := w.travelTo(e, target)
		if !ok {
			t.Fatalf("gave up at %v", e.Pos)
		}
		if arrived {
			break
		}
	}
	if e.Pos.Level != target.Level || !e.Pos.Adjacent(target) {
		t.Fatalf("ended at %v, want beside %v", e.Pos, target)
	}
	if want := 2 * (w.cfg.ShaftClimbTicks - 1); climbed != want {
		t.Errorf("spent %d turns on the ladder, want %d (two levels)", climbed, want)
	}
	if w.entityAt(top) != nil || w.entityAt(e.Pos) != e {
		t.Error("occupancy lost track of the climber")
	}
}

// A laden climber is slower, and its route weighs the slower climb.
func TestLadenClimbIsSlower(t *testing.T) {
	w, _ := shaftWorld(t)
	e := w.spawn(Colonist, Point{12, 8, LandingLevel})
	if w.climbTicks(e) != w.cfg.ShaftClimbTicks {
		t.Fatalf("an empty-handed climb takes %d ticks, want %d", w.climbTicks(e), w.cfg.ShaftClimbTicks)
	}
	e.Inventory.Add(IronOre, w.cfg.ShaftCarry) // at the limit: still the usual pace
	if w.laden(e) {
		t.Fatal("a climber at shaft-carry counts as laden")
	}
	e.Inventory.Add(IronOre, 1)
	if !w.laden(e) || w.climbTicks(e) != w.cfg.ShaftLadenClimbTicks || w.climbCost(e) != int32(w.cfg.ShaftLadenClimbTicks) {
		t.Fatalf("over shaft-carry: laden %v, climb %d ticks, cost %d", w.laden(e), w.climbTicks(e), w.climbCost(e))
	}
}

// The route takes the shaft when the climb is cheap and walks round to the
// stair when it is dear.
func TestRouteWeighsShaftAgainstStair(t *testing.T) {
	for _, tc := range []struct {
		climb     int
		wantShaft bool
	}{{2, true}, {20, false}} {
		cfg := testConfig()
		cfg.Width, cfg.Height = 40, 20
		cfg.DeepestLevel = 2
		cfg.ShaftClimbTicks, cfg.ShaftLadenClimbTicks = tc.climb, tc.climb
		w := newWorld(cfg, newPCG(1))
		carveOn(w, LandingLevel, Point{5, 5, 0}, Point{20, 12, 0}, Floor)
		if !w.digStair(Point{10, 8, LandingLevel}) || !w.digShaft(Point{15, 8, LandingLevel}, LandingLevel+1) {
			t.Fatal("could not dig the stair and the shaft")
		}
		carveOn(w, LandingLevel+1, Point{11, 6, 0}, Point{14, 10, 0}, Floor)
		carveOn(w, LandingLevel+1, Point{16, 6, 0}, Point{20, 10, 0}, Floor)
		carveOn(w, LandingLevel+1, Point{14, 6, 0}, Point{16, 6, 0}, Floor) // join round the shaft's foot
		w.refreshSpatial()
		e := w.spawn(Colonist, Point{14, 8, LandingLevel})
		w.pf.climb = w.climbCost(e)
		route, ok := w.pathToAdjacent(e.Pos, Point{18, 8, LandingLevel + 1})
		w.pf.climb = 0
		if !ok {
			t.Fatalf("climb %d: no route", tc.climb)
		}
		viaShaft := false
		prev := e.Pos
		for _, p := range route {
			viaShaft = viaShaft || w.crossesShaft(prev, p)
			prev = p
		}
		if viaShaft != tc.wantShaft {
			t.Errorf("climb %d: route via the shaft = %v, want %v (%v)", tc.climb, viaShaft, tc.wantShaft, route)
		}
	}
}

// An alien climbs only if its build has arms; one without stays off shafts,
// so a level joined to its own only by a shaft is out of its reach.
func TestOnlyAliensWithArmsClimb(t *testing.T) {
	w, _ := shaftWorld(t)
	a := w.spawn(Alien, Point{8, 8, LandingLevel})
	sp := &w.alienSpecies[a.Species]
	sp.Arms = 0
	if w.canClimb(a) || w.climbCost(a) != -1 {
		t.Fatal("an armless alien can climb")
	}
	w.pf.climb = w.climbCost(a)
	_, ok := w.pathToAdjacent(a.Pos, Point{30, 8, LandingLevel + 2})
	w.pf.climb = 0
	if ok {
		t.Error("an armless alien found a route down the shaft")
	}
	prey := w.spawn(Colonist, Point{30, 8, LandingLevel + 2})
	if w.canReach(a, prey) {
		t.Error("an armless alien hunts prey only a shaft leads to")
	}
	sp.Arms = 2
	if !w.canClimb(a) {
		t.Fatal("an alien with arms cannot climb")
	}
	for _, k := range []Kind{Colonist, Cat, Rat} {
		if !w.canClimb(&Entity{Kind: k}) {
			t.Errorf("a %v cannot climb", k)
		}
	}
	if w.canClimb(&Entity{Kind: Chicken}) {
		t.Error("a chicken climbs")
	}
}

// Nobody passes through a crowd across a shaft: a mover whose route climbs
// waits while the far end is taken.
func TestNoPassingThroughAShaft(t *testing.T) {
	w, top := shaftWorld(t)
	e := w.spawn(Colonist, top)
	w.spawn(Colonist, Point{15, 8, LandingLevel + 1}) // standing on the shaft below
	w.tick++
	if _, ok := w.travelTo(e, Point{24, 8, LandingLevel + 1}); !ok {
		t.Fatal("gave up instead of waiting")
	}
	if e.Pos != top {
		t.Fatalf("moved to %v past the colonist on the ladder", e.Pos)
	}
}

// The whole path a player sees: order a shaft two levels deep, the colony
// marks it out, a digger sinks it, and the levels below exist.
func TestOrderedShaftIsDug(t *testing.T) {
	w := shaftColony(t, 3)
	w.manualShaftLevels = 2
	runUntil(t, w, 4000, "a shaft", func() bool { return len(w.shafts) > 0 })
	top := w.shafts[0]
	if w.TerrainAt(top) != ShaftTop || w.shaftBottom(top) != LandingLevel+2 {
		t.Fatalf("shaft at %v is %v reaching level %d, want a top reaching %d",
			top, w.TerrainAt(top), w.shaftBottom(top), LandingLevel+2)
	}
	if w.manualShaftLevels != 0 || w.shaftPlanned() {
		t.Error("the order is still pending after the shaft was dug")
	}
	for l := LandingLevel + 1; l <= LandingLevel+2; l++ {
		if w.layer(l) == nil {
			t.Errorf("level %d was not made", l)
		}
	}
	checkRoomLabelsAllLevels(t, w)
}

// shaftColony is a generated colony allowed two levels down.
func shaftColony(t *testing.T, seed int64) *World {
	t.Helper()
	cfg := testConfig()
	cfg.Seed = seed
	cfg.Width, cfg.Height = 120, 70
	cfg.StartColonists = 8
	cfg.DeepestLevel = 3
	return newTestWorld(t, cfg)
}

// A game with a shaft runs the same twice, and saved and loaded it plays on
// identically.
func TestShaftRunsAreDeterministicAndSave(t *testing.T) {
	run := func() (*World, []string) {
		w := shaftColony(t, 5)
		w.manualShaftLevels = 2
		var hashes []string
		for i := 0; i < 1500; i++ {
			w.step()
			if i%100 == 99 {
				hashes = append(hashes, levelsHash(w))
			}
		}
		if len(w.shafts) == 0 {
			t.Fatal("no shaft was dug: the test proves nothing about shafts")
		}
		return w, hashes
	}
	w, a := run()
	_, b := run()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("runs diverge by tick %d:\n%s\n%s", (i+1)*100, a[i], b[i])
		}
	}
	loaded := saveAndLoad(t, w)
	for i := 0; i < 400; i++ {
		w.step()
		loaded.step()
	}
	if !bytes.Equal(encodeWorld(t, w), encodeWorld(t, loaded)) {
		t.Fatalf("loaded game's state differs by tick %d: %s", w.tick, saveDiff(w, loaded))
	}
}

// A miner sent to rock at the foot of a shaft climbs down to it, spending
// turns on the ladder, and mines it.
func TestMinerClimbsDownToWork(t *testing.T) {
	w := shaftColony(t, 3)
	w.manualShaftLevels = 1
	runUntil(t, w, 3000, "a shaft", func() bool { return len(w.shafts) > 0 })
	w.step() // fold the shaft into the rooms
	below := w.layer(LandingLevel + 1)
	var rock Point
	found := false
	for p := range below.board.frontier {
		if !below.board.isClaimed(p) && (!found || lessPoint(p, rock)) {
			rock, found = p, true
		}
	}
	if !found {
		t.Fatal("no mineable rock around the foot of the shaft")
	}
	var miner *Entity
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Colonist && e.Pos.Level == LandingLevel {
			miner = e
			break
		}
	}
	w.clearJob(miner)
	below.board.claimMine(rock, miner.ID)
	w.assignMineTarget(miner, rock)
	climbed := false
	runUntil(t, w, 2000, "mining below", func() bool {
		climbed = climbed || miner.State == Climbing
		return w.TerrainAt(rock) != Rock
	})
	if !climbed {
		t.Fatalf("the rock at %v was mined without anyone climbing the shaft", rock)
	}
}

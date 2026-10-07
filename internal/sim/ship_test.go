package sim

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// shipInterior reports whether p is inside ship s: in its footprint and not
// hull.
func shipInterior(s *Ship, p Point) bool {
	dx, dy := p.X-s.Origin.X, p.Y-s.Origin.Y
	if dx < 0 || dy < 0 || dx >= s.layout.width || dy >= s.layout.height {
		return false
	}
	return !s.layout.hullAt(dx, dy)
}

// assertShipIntact checks that ship s stands as laid out — hull, communal
// bunks and toilets, a private locker per passenger stocked with its meals,
// reserved approaches — and that every passenger is aboard it.
func assertShipIntact(t *testing.T, w *World, s *Ship) {
	t.Helper()
	l := &s.layout
	for dy := 0; dy < l.height; dy++ {
		for dx := 0; dx < l.width; dx++ {
			p := s.Origin.Add(dx, dy)
			if l.inShip(dx, dy) && (w.TerrainAt(p) == Hull) != l.hullAt(dx, dy) {
				t.Fatalf("ship %d at %v: %v where the hull should be %v", s.ID, p, w.TerrainAt(p), l.hullAt(dx, dy))
			}
		}
	}
	for _, f := range l.fixtures {
		p := s.Origin.Add(f.at.X, f.at.Y)
		fx := w.fixtures[p]
		if w.TerrainAt(p) != f.terrain || fx == nil {
			t.Fatalf("ship %d's %v at %v is %v", s.ID, f.terrain, p, w.TerrainAt(p))
		}
		if f.slot < 0 {
			if fx.Owner != Community || fx.Access != AccessCommunal {
				t.Fatalf("ship %d's %v at %v is not communal: %+v", s.ID, f.terrain, p, fx)
			}
			continue
		}
		me := ColonistOwner(s.Colonists[f.slot])
		if fx.Owner != me || fx.Access != AccessPrivate {
			t.Fatalf("ship %d's %v at %v is not its passenger's own: %+v", s.ID, f.terrain, p, fx)
		}
	}
	for _, d := range l.doors {
		if p := s.Origin.Add(d.X, d.Y); !w.Walkable(p) || !w.doorTiles[p] {
			t.Fatalf("ship %d's approach %v is %v (reserved %v), want reserved floor", s.ID, p, w.TerrainAt(p), w.doorTiles[p])
		}
	}
	for _, id := range s.Colonists {
		e := w.entities[id]
		if e.ship != s.ID {
			t.Fatalf("%s came down in ship %d but records ship %d", e.displayName(), s.ID, e.ship)
		}
		p, ok := w.lockerOf(e)
		if !ok {
			t.Fatalf("%s has no locker", e.displayName())
		}
		locker := w.storageContainers[p]
		// A meal it has taken out to carry (a pocket meal) is still the manifest's.
		if got := locker.held(ColonistOwner(id), Meal) + e.ownCarried(Meal); got != w.cfg.CrashPodMeals {
			t.Fatalf("%s's locker and pockets hold %d meals of its own, want %d", e.displayName(), got, w.cfg.CrashPodMeals)
		}
		if !locker.ledgerBalanced() {
			t.Fatalf("%s's locker ledger does not match its contents", e.displayName())
		}
		if w.arrivalRareItem(id) == rareGun && e.Inventory.Count(Pistol)+e.Inventory.Count(Shotgun) != 1 {
			t.Fatalf("%s's rare item is a gun, but it carries %d pistols and %d shotguns", e.displayName(),
				e.Inventory.Count(Pistol), e.Inventory.Count(Shotgun))
		}
	}
}

// Every way in — worldgen, the spawn command, a director arrival — is a ship.
func TestEveryArrivalComesInAShip(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0
	cfg.Width, cfg.Height = 80, 50
	cfg.Schedules = []Schedule{{
		Name: "second wave", EarliestTick: 1, LatestTick: 1,
		Occurrences: []DirectorOccurrence{{Kind: OccArrival, Count: 3}},
	}}
	eng := NewEngine(cfg)
	w := eng.world
	if w.countKind(Colonist) != cfg.StartColonists || len(w.ships) != 1 {
		t.Fatalf("worldgen landed %d colonists in %d ships, want %d in 1", w.countKind(Colonist), len(w.ships), cfg.StartColonists)
	}
	w.step() // the director's wave
	eng.apply(Spawn{Kind: Colonist})
	if want := cfg.StartColonists + 4; w.countKind(Colonist) != want || len(w.ships) != 3 {
		t.Fatalf("%d colonists in %d ships after the wave and a spawn, want %d in 3", w.countKind(Colonist), len(w.ships), want)
	}
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Colonist && w.shipByID(e.ship) == nil {
			t.Fatalf("%s came down in no ship", e.displayName())
		}
	}
	for _, s := range w.ships {
		assertShipIntact(t, w, s)
	}
}

// A ship's layout, in every shape: bunks and toilets by their percents, a
// locker per passenger and a trough per keeper, every fixture facing open
// deck, room for everyone (pets included) to step out, and every bit of
// deck reachable from outside through a doorway.
func TestShipLayout(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.ShipBunkPercent, w.cfg.ShipToiletPercent = 50, 25
	for shape := shipStick; shape < numShipShapes; shape++ {
		for n := 1; n <= 20; n++ {
			for _, hens := range []bool{false, true} {
				keepers := make([]bool, n)
				k := 0
				for i := range keepers {
					if hens && i%3 == 0 {
						keepers[i] = true
						k++
					}
				}
				l := w.planShip(keepers, shape)
				if l.shape != shape {
					t.Fatalf("%v for %d came out a %v", shape, n, l.shape)
				}
				assertLayoutSound(t, fmt.Sprintf("%v for %d with %d keepers", shape, n, k), &l)
				count := map[Terrain]int{}
				for _, f := range l.fixtures {
					count[f.terrain]++
				}
				if count[Bed] != (n+1)/2 || count[Toilet] != (n+3)/4 || count[Storage] != n || count[Trough] != k {
					t.Fatalf("%v for %d with %d keepers: %v", shape, n, k, count)
				}
				if len(l.floor) < n+n {
					t.Fatalf("%v for %d: only %d aisle tiles to step out on", shape, n, len(l.floor))
				}
			}
		}
	}
	// A full stick is one compact block, not a row of cubicles.
	if l := w.planShip(make([]bool, 20), shipStick); l.width > 25 || l.height != 6 {
		t.Fatalf("a stick for 20 is %dx%d", l.width, l.height)
	}
}

// assertLayoutSound checks a layout's geometry: fixtures on deck, each
// beside open deck; step-out tiles distinct open deck; and every open deck
// tile reachable from the doors' approaches, so nobody is shut in.
func assertLayoutSound(t *testing.T, what string, l *shipLayout) {
	t.Helper()
	fixture := map[Point]bool{}
	for _, f := range l.fixtures {
		if l.cellAt(f.at.X, f.at.Y) != shipDeck || fixture[f.at] {
			t.Fatalf("%s: a %v at %v is not on a deck tile of its own", what, f.terrain, f.at)
		}
		fixture[f.at] = true
	}
	open := func(p Point) bool { return l.cellAt(p.X, p.Y) == shipDeck && !fixture[p] }
	for _, f := range l.fixtures {
		faces := false
		for _, d := range veinNeighbors {
			faces = faces || open(f.at.Add(d.X, d.Y))
		}
		if !faces {
			t.Fatalf("%s: the %v at %v faces no open deck", what, f.terrain, f.at)
		}
	}
	seen := map[Point]bool{}
	for _, p := range l.floor {
		if !open(p) || seen[p] {
			t.Fatalf("%s: step-out tile %v is not open deck of its own", what, p)
		}
		seen[p] = true
	}
	if len(l.doors) < 2 {
		t.Fatalf("%s: %d door approaches, want at least two ways out", what, len(l.doors))
	}
	reached := map[Point]bool{}
	queue := []Point{}
	for _, d := range l.doors {
		if l.inShip(d.X, d.Y) {
			t.Fatalf("%s: door approach %v is inside the ship", what, d)
		}
		queue = append(queue, d)
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range veinNeighbors {
			q := p.Add(d.X, d.Y)
			if open(q) && !reached[q] {
				reached[q] = true
				queue = append(queue, q)
			}
		}
	}
	for dy := 0; dy < l.height; dy++ {
		for dx := 0; dx < l.width; dx++ {
			if p := (Point{dx, dy, 0}); open(p) && !reached[p] {
				t.Fatalf("%s: open deck at %v cannot be reached from outside:\n%s", what, p, strings.Join(l.rows, "\n"))
			}
		}
	}
}

// Every shape comes down, at its weight's odds; a shape weighted 0 never
// does.
func TestShipShapesFollowTheirWeights(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.ShipStickWeight, w.cfg.ShipHubWeight, w.cfg.ShipClusterWeight = 1, 1, 1
	seen := map[shipShape]int{}
	for id := EntityID(1); id <= 3000; id++ {
		seen[w.shipShapeFor(id)]++
	}
	for shape := shipStick; shape < numShipShapes; shape++ {
		if seen[shape] < 800 || seen[shape] > 1200 {
			t.Errorf("equal weights: %v came up %d times of 3000", shape, seen[shape])
		}
	}
	w.cfg.ShipStickWeight, w.cfg.ShipHubWeight, w.cfg.ShipClusterWeight = 0, 0, 1
	for id := EntityID(1); id <= 100; id++ {
		if got := w.shipShapeFor(id); got != shipCluster {
			t.Fatalf("only clusters weighted, got a %v", got)
		}
	}
	w.cfg.ShipClusterWeight = 0
	if got := w.shipShapeFor(1); got != shipStick {
		t.Fatalf("no weights: got a %v, want a stick", got)
	}
}

// Waves come down in as few ships as capacity allows, loaded evenly.
func TestShipLoads(t *testing.T) {
	for _, c := range []struct {
		n, capacity int
		want        []int
	}{
		{0, 20, nil},
		{1, 20, []int{1}},
		{20, 20, []int{20}},
		{30, 20, []int{15, 15}},
		{41, 20, []int{14, 14, 13}},
		{5, 1, []int{1, 1, 1, 1, 1}},
	} {
		if got := shipLoads(c.n, c.capacity); !slices.Equal(got, c.want) {
			t.Errorf("shipLoads(%d, %d) = %v, want %v", c.n, c.capacity, got, c.want)
		}
	}
}

// Ships land in the lower half and leave room for the colony's first rooms,
// which are sited against rock above them. Every colony size must still have
// a room site the moment it lands — the bug this pins had crash pods fill the
// small landing cavern and stall all construction — and every passenger
// must be able to walk out of its ship to the rest of the colony.
func TestShipsLeaveRoomForTheFirstRooms(t *testing.T) {
	for _, weights := range [][3]int{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {1, 1, 1}} {
		for _, n := range []int{1, 3, 6, 10, 16, 20, 40} {
			t.Run(fmt.Sprintf("%v/%d", weights, n), func(t *testing.T) {
				assertShipsLeaveRoom(t, n, weights)
			})
		}
	}
}

func assertShipsLeaveRoom(t *testing.T, n int, weights [3]int) {
	{
		cfg := testConfig()
		cfg.StartColonists = n
		cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0
		cfg.ShipStickWeight, cfg.ShipHubWeight, cfg.ShipClusterWeight = weights[0], weights[1], weights[2]
		if n > 6 {
			cfg.Width, cfg.Height = 80, 50
		}
		w := newTestWorld(t, cfg)
		w.refreshSpatial()
		if _, ok := w.findRoomSite(bayWidth(lifeSupportRoom.minFac)); !ok {
			t.Errorf("%d colonists: no site for a facility room at landing", n)
		}
		for _, s := range w.ships {
			if s.Origin.Y <= w.Height/2 {
				t.Errorf("%d colonists: a ship landed at %v, in the upper half kept for rooms", n, s.Origin)
			}
			assertShipIntact(t, w, s)
		}
		for _, id := range w.entityIDsSorted() {
			if e := w.entities[id]; e.Kind == Colonist && w.roomOf(e.Pos) != w.mainRoom {
				t.Errorf("%d colonists: %s is shut in its ship at %v", n, e.displayName(), e.Pos)
			}
		}
	}
}

// With no open floor left, a ship smashes down through the rock, clears its
// footprint and crater, and still opens onto the colony.
func TestShipCrashesThroughRockWhenTheCavernIsFull(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.Width, cfg.Height = 40, 22
	w := newTestWorld(t, cfg)
	// Solid rock but for a single open strip across the middle: no ship fits
	// on floor with a clear margin, so every landing has to crash.
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y, LandingLevel}, Rock)
		}
	}
	for x := 5; x < 35; x++ {
		w.SetTerrain(Point{x, 12, LandingLevel}, Floor)
	}
	w.refreshSpatial()

	s := w.land(3, true)
	if s == nil {
		t.Fatal("no site found")
	}
	s.layout.forEachTile(func(d Point, _ bool) {
		if w.TerrainAt(s.Origin.Add(d.X, d.Y)) == Rock {
			t.Fatalf("rock left inside the ship's footprint at %v", s.Origin.Add(d.X, d.Y))
		}
	})
	for _, d := range s.layout.margin {
		if p := s.Origin.Add(d.X, d.Y); w.TerrainAt(p) == Rock {
			t.Fatalf("rock left in the crater around the ship at %v", p)
		}
	}
	assertShipIntact(t, w, s)
	w.refreshSpatial()
	for _, id := range s.Colonists {
		if w.roomOf(w.entities[id].Pos) != w.roomOf(Point{6, 12, LandingLevel}) {
			t.Fatal("the crashed ship does not open onto the strip")
		}
	}
	if !loggedContaining(w, "smashes down through the rock") {
		t.Fatal("the crash landing was not announced as one")
	}
}

// Ship placement is part of the simulation: same seed, same landing sites.
func TestShipLandingIsDeterministic(t *testing.T) {
	sites := func() []Point {
		cfg := testConfig()
		cfg.Width, cfg.Height = 120, 60
		cfg.StartColonists = 50
		w := newTestWorld(t, cfg)
		var out []Point
		for _, s := range w.ships {
			out = append(out, s.Origin)
		}
		return out
	}
	a, b := sites(), sites()
	if len(a) != 3 || !slices.Equal(a, b) {
		t.Fatalf("ships landed at %v, then %v", a, b)
	}
}

// A ship never lands on the floor of a natural cavern nobody has found: it
// would open the cavern around its passengers, cut off from the colony. Here
// the only clean landing is a hidden cavern right where the search starts;
// the ship must crash beside the colony's one known corridor instead.
func TestShipsNeverLandInAnUndiscoveredCavern(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.Width, cfg.Height = 60, 30
	w := newTestWorld(t, cfg)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y, LandingLevel}, Rock)
		}
	}
	for y := 15; y <= 26; y++ {
		for x := 20; x <= 40; x++ {
			cellAt(w, Point{x, y, LandingLevel}).Explored = false
			w.carveHidden(Point{x, y, LandingLevel})
		}
	}
	for y := 10; y < 29; y++ {
		w.SetTerrain(Point{5, y, LandingLevel}, Floor)
	}
	w.refreshSpatial()

	s := w.land(1, false)
	if s == nil {
		t.Fatal("no site found")
	}
	w.refreshSpatial()
	if w.roomOf(w.entities[s.Colonists[0]].Pos) != w.roomOf(Point{5, 10, LandingLevel}) {
		t.Fatalf("the ship at %v landed cut off from the colony's corridor", s.Origin)
	}
}

// A ship never lands close enough to a hidden cavern to reveal it: a landing
// that broke through would flood the cavern into view and roll its nests
// with nobody digging.
func TestShipsNeverRevealAHiddenCavern(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.Width, cfg.Height = 60, 30
	w := newTestWorld(t, cfg)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y, LandingLevel}, Rock)
		}
	}
	var cavern []Point
	for y := 5; y <= 25; y++ {
		for x := 30; x <= 45; x++ {
			p := Point{x, y, LandingLevel}
			cellAt(w, p).Explored = false
			w.carveHidden(p)
			cavern = append(cavern, p)
		}
	}
	// The colony's corridor runs right alongside the cavern wall.
	for y := 3; y < 28; y++ {
		w.SetTerrain(Point{27, y, LandingLevel}, Floor)
	}
	w.refreshSpatial()
	for i := 0; i < 4; i++ {
		if w.land(1, false) == nil {
			break
		}
		w.refreshSpatial()
	}
	for _, p := range cavern {
		if w.discovered(p) {
			t.Fatalf("a ship landing revealed the hidden cavern at %v", p)
		}
	}
}

// Before the first tick a ship can be moved anywhere: it obliterates what it
// lands on, its passengers and pets come with it, its lockers are restocked,
// and the old site is left bare. It may not land on another ship or the
// walkway round one, and once the game has started it stays put.
func TestMoveShipBeforeTheFirstTick(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 120, 60
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 30, 0, 0, 0
	cfg.CrashPodGunWeight, cfg.CrashPodChickenWeight, cfg.CrashPodCatWeight = 1, 1, 1
	w := newTestWorld(t, cfg)
	if len(w.ships) != 2 {
		t.Fatalf("30 settlers came down in %d ships, want 2", len(w.ships))
	}
	a, b := w.ships[0], w.ships[1]
	old := a.Origin
	dest := Point{10, 5, LandingLevel} // up in the rock, far from the cavern
	rat := w.spawn(Rat, Point{0, 0, LandingLevel})
	w.SetTerrain(dest.Add(3, 2), Floor)
	w.moveEntity(rat, dest.Add(3, 2))

	if w.moveShip(MoveShip{Ship: a.ID, X: b.Origin.X + 2, Y: b.Origin.Y}) {
		t.Fatal("a ship landed on top of another")
	}
	if w.moveShip(MoveShip{Ship: a.ID, X: b.Origin.X, Y: b.Origin.Y - a.layout.height}) {
		t.Fatal("a ship landed on the walkway round another")
	}
	if !w.moveShip(MoveShip{Ship: a.ID, X: dest.X, Y: dest.Y}) {
		t.Fatal("the ship would not move to open rock")
	}
	if a.Origin != dest {
		t.Fatalf("the ship is at %v, want %v", a.Origin, dest)
	}
	if w.entities[rat.ID] != nil {
		t.Fatal("the rat under the landing site survived")
	}
	assertShipIntact(t, w, a)
	assertShipIntact(t, w, b)
	for _, id := range append(append([]EntityID(nil), a.Colonists...), a.Pets...) {
		e := w.entities[id]
		if !shipInterior(a, e.Pos) || w.landing().occ.at(e.Pos.X, e.Pos.Y) != id {
			t.Fatalf("%s did not come with its ship: at %v", e.displayName(), e.Pos)
		}
	}
	a.layout.forEachTile(func(d Point, _ bool) {
		if p := old.Add(d.X, d.Y); !shipInterior(b, p) && w.TerrainAt(p) != Floor && !(p.X >= dest.X && p.X < dest.X+a.layout.width && p.Y >= dest.Y && p.Y < dest.Y+a.layout.height) {
			t.Fatalf("the old site still has %v at %v", w.TerrainAt(p), p)
		}
	})
	w.step()
	if w.moveShip(MoveShip{Ship: a.ID, X: old.X, Y: old.Y}) {
		t.Fatal("a ship moved after the game started")
	}
}

// Lockers carry crash-pod-meals give or take crash-pod-meal-spread, so they
// don't all run dry on the same tick. The count is a pure function of the
// seed and the colonist: the same every time, with no spread at all at 0.
func TestArrivalMealsSpreadAroundTheManifest(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.CrashPodMeals, w.cfg.CrashPodMealSpread = 10, 4
	seen := map[int]bool{}
	sum := 0
	for id := EntityID(1); id <= 400; id++ {
		n := w.arrivalMeals(id)
		if n < 6 || n > 14 {
			t.Fatalf("colonist %d's locker has %d meals, outside 10±4", id, n)
		}
		if n != w.arrivalMeals(id) {
			t.Fatalf("colonist %d's meals changed between calls", id)
		}
		seen[n] = true
		sum += n
	}
	if len(seen) < 7 {
		t.Fatalf("only %d distinct meal counts across 400 lockers: %v", len(seen), seen)
	}
	if mean := float64(sum) / 400; mean < 9.5 || mean > 10.5 {
		t.Fatalf("mean meals per locker %.2f, want about 10", mean)
	}
	w.cfg.CrashPodMealSpread = 0
	for id := EntityID(1); id <= 20; id++ {
		if n := w.arrivalMeals(id); n != 10 {
			t.Fatalf("with no spread, colonist %d's locker has %d meals", id, n)
		}
	}
}

// Every colonist lands with exactly one rare item, picked by the weights, and
// a gun is a shotgun crash-pod-shotgun-percent of the time. Both rolls are a
// pure function of the seed and the colonist.
func TestArrivalRareItemsFollowTheirWeights(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.CrashPodGunWeight, w.cfg.CrashPodChickenWeight, w.cfg.CrashPodCatWeight = 50, 25, 25
	w.cfg.CrashPodShotgunPercent = 25
	var got [4]int
	guns, shotguns := 0, 0
	const colonists = 2000
	for id := EntityID(1); id <= colonists; id++ {
		r := w.arrivalRareItem(id)
		if r != w.arrivalRareItem(id) || w.arrivalGun(id) != w.arrivalGun(id) {
			t.Fatalf("colonist %d's rare item changed between calls", id)
		}
		got[r]++
		if r == rareGun {
			guns++
			if w.arrivalGun(id) == Shotgun {
				shotguns++
			}
		}
	}
	if got[rareNone] != 0 {
		t.Fatalf("%d colonists landed with no rare item", got[rareNone])
	}
	if got[rareGun] < 920 || got[rareGun] > 1080 {
		t.Errorf("%d of %d colonists carried a gun, want about 1000", got[rareGun], colonists)
	}
	if got[rareChicken] < 430 || got[rareChicken] > 570 {
		t.Errorf("%d of %d colonists brought a chicken, want about 500", got[rareChicken], colonists)
	}
	if got[rareCat] < 430 || got[rareCat] > 570 {
		t.Errorf("%d of %d colonists brought a cat, want about 500", got[rareCat], colonists)
	}
	if shotguns < guns/4-60 || shotguns > guns/4+60 {
		t.Errorf("%d of %d guns were shotguns, want about a quarter", shotguns, guns)
	}

	w.cfg.CrashPodGunWeight, w.cfg.CrashPodChickenWeight, w.cfg.CrashPodCatWeight = 0, 0, 0
	if r := w.arrivalRareItem(1); r != rareNone {
		t.Fatalf("with every weight zero, a colonist landed with rare item %d", r)
	}
}

// With place-ships set the founders wait aloft: nobody is on the map until
// the player lands each ship in turn, where they choose, and the frontend is
// shown the next ship's shape before it lands. Whatever is still aloft when
// the game starts lands by itself.
func TestFoundersWaitAloftToBeLandedOneByOne(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 120, 60
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 50, 0, 0, 0
	cfg.CrashPodGunWeight, cfg.CrashPodChickenWeight, cfg.CrashPodCatWeight = 1, 1, 1
	cfg.ShipStickWeight, cfg.ShipHubWeight, cfg.ShipClusterWeight = 1, 1, 1
	cfg.PlaceShips = true
	w := newTestWorld(t, cfg)
	if w.countKind(Colonist) != 0 || len(w.ships) != 0 || !slices.Equal(w.aloft, []int{17, 17, 16}) {
		t.Fatalf("worldgen landed %d colonists in %d ships, %v aloft; want everyone aloft", w.countKind(Colonist), len(w.ships), w.aloft)
	}
	views := w.shipViews()
	if len(views) != 3 || !views[0].Aloft || views[0].Shape == nil || views[1].Shape != nil {
		t.Fatalf("aloft views: %+v", views)
	}
	next := views[0]
	if w.landAloft(LandShip{Ship: 2, X: 10, Y: 5}) {
		t.Fatal("ship 2 landed before ship 1")
	}
	if !w.landAloft(LandShip{Ship: 1, X: 10, Y: 5}) {
		t.Fatal("ship 1 would not land in open rock")
	}
	s := w.ships[0]
	if s.Origin != (Point{10, 5, LandingLevel}) || s.layout.width != next.Width || !slices.Equal(s.layout.rows, next.Shape) {
		t.Fatalf("ship 1 landed at %v as\n%s\nnot as shown:\n%s", s.Origin, strings.Join(s.layout.rows, "\n"), strings.Join(next.Shape, "\n"))
	}
	assertShipIntact(t, w, s)
	if w.landAloft(LandShip{Ship: 1, X: 60, Y: 40}) {
		t.Fatal("ship 1 landed twice")
	}
	if w.landAloft(LandShip{Ship: 2, X: 10 + s.layout.width - 2, Y: 5}) {
		t.Fatal("ship 2 landed on ship 1")
	}
	if !w.landAloft(LandShip{Ship: 2, X: 60, Y: 40}) {
		t.Fatal("ship 2 would not land")
	}
	assertShipIntact(t, w, w.ships[1])
	w.step() // the third is still aloft: it comes down by itself
	if len(w.aloft) != 0 || len(w.ships) != 3 || w.countKind(Colonist) != 50 {
		t.Fatalf("after the first tick: %d ships, %v aloft, %d colonists", len(w.ships), w.aloft, w.countKind(Colonist))
	}
	for _, s := range w.ships {
		for _, id := range s.Colonists {
			if w.entities[id].ship != s.ID {
				t.Fatalf("colonist %d is not aboard ship %d", id, s.ID)
			}
		}
	}
	if w.landAloft(LandShip{Ship: 4, X: 10, Y: 30}) {
		t.Fatal("a ship landed after the game started")
	}
}

// A ship the player lands beside a hidden cavern breaks into it, and the
// cavern's nest is rolled only once the passengers are out: its aliens never
// take the passengers' IDs (which would break the ship's manifest) or their
// tiles.
func TestPlayerLandingBreaksIntoACavernAfterThePassengersAreOut(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 80, 50
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 6, 0, 0, 0
	cfg.CavernNestPercent = 100
	cfg.PlaceShips = true
	w := newTestWorld(t, cfg)
	if len(w.alienSpecies) == 0 {
		t.Skip("no alien species to nest")
	}
	center := Point{40, 8, LandingLevel}
	for y := 4; y <= 12; y++ {
		for x := 34; x <= 46; x++ {
			p := Point{x, y, LandingLevel}
			w.SetTerrain(p, Rock)
			cellAt(w, p).Explored = false
			w.carveHidden(p)
		}
	}
	w.unfoundCaverns[center] = struct{}{}
	aliens, first := w.countKind(Alien), w.nextID
	if !w.landAloft(LandShip{Ship: 1, X: 26, Y: 6}) { // its hold end juts into the cavern
		t.Fatal("the ship would not land")
	}
	s := w.ships[0]
	assertShipIntact(t, w, s)
	if !w.discovered(center) {
		t.Fatal("the landing did not break into the cavern")
	}
	if w.countKind(Alien) == aliens {
		t.Fatal("the breach rolled no nest")
	}
	if s.Colonists[0] != first {
		t.Fatalf("the first passenger is %d, want %d", s.Colonists[0], first)
	}
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Alien && id >= first && id <= s.Colonists[len(s.Colonists)-1] {
			t.Fatalf("an alien took ID %d, among the passengers'", id)
		}
	}
}

package sim

import "testing"

// assertOwnsPod checks that e arrived in a crash pod that is fully its own: a
// private bunk, toilet, and locker, the locker stocked with its meals.
func assertOwnsPod(t *testing.T, w *World, e *Entity) {
	t.Helper()
	if !e.hasPod {
		t.Fatalf("%s has no crash pod", e.displayName())
	}
	me := ColonistOwner(e.ID)
	for _, f := range podFixtures {
		p := e.podOrigin.Add(f.dx, f.dy)
		fx := w.fixtures[p]
		if w.TerrainAt(p) != f.terrain || fx == nil || fx.Owner != me || fx.Access != AccessPrivate {
			t.Fatalf("%s's pod %v at %v: terrain %v fixture %+v", e.displayName(), f.terrain, p, w.TerrainAt(p), fx)
		}
	}
	locker := w.storageContainers[e.podOrigin.Add(podFixtures[2].dx, podFixtures[2].dy)]
	// A meal it has taken out to carry (a pocket meal) is still the manifest's.
	if got := locker.held(me, Meal) + e.ownCarried(Meal); got != w.cfg.CrashPodMeals {
		t.Fatalf("%s's locker and pockets hold %d meals of its own, want %d", e.displayName(), got, w.cfg.CrashPodMeals)
	}
	if !locker.ledgerBalanced() {
		t.Fatalf("%s's locker ledger does not match its contents", e.displayName())
	}
	for dy := 0; dy < podHeight; dy++ {
		for dx := 0; dx < podWidth; dx++ {
			p := e.podOrigin.Add(dx, dy)
			if (w.TerrainAt(p) == Hull) != podHullAt(dx, dy) {
				t.Fatalf("%s's pod at %v: %v where the hull should be %v", e.displayName(), p, w.TerrainAt(p), podHullAt(dx, dy))
			}
		}
	}
	if p := e.podOrigin.Add(podApproach.X, podApproach.Y); !w.Walkable(p) || !w.doorTiles[p] {
		t.Fatalf("%s's pod approach %v is %v (reserved %v), want reserved floor", e.displayName(), p, w.TerrainAt(p), w.doorTiles[p])
	}
	if w.podRareItem(e.ID) == rareGun && e.Inventory.Count(Pistol)+e.Inventory.Count(Shotgun) != 1 {
		t.Fatalf("%s's rare item is a gun, but it carries %d pistols and %d shotguns", e.displayName(),
			e.Inventory.Count(Pistol), e.Inventory.Count(Shotgun))
	}
}

// Every way in — worldgen, the spawn command, a director arrival — is a crash
// pod the colonist owns.
func TestEveryArrivalComesInACrashPod(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0
	cfg.Width, cfg.Height = 80, 50
	cfg.Schedules = []Schedule{{
		Name: "second wave", EarliestTick: 1, LatestTick: 1,
		Occurrences: []DirectorOccurrence{{Kind: OccArrival, Count: 3}},
	}}
	eng := NewEngine(cfg)
	w := eng.world
	if w.countKind(Colonist) != cfg.StartColonists {
		t.Fatalf("worldgen landed %d colonists, want %d", w.countKind(Colonist), cfg.StartColonists)
	}
	w.step() // the director's wave
	eng.apply(Spawn{Kind: Colonist})
	if want := cfg.StartColonists + 4; w.countKind(Colonist) != want {
		t.Fatalf("%d colonists after the wave and a spawn, want %d", w.countKind(Colonist), want)
	}
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Colonist {
			assertOwnsPod(t, w, e)
		}
	}
}

// Pods land in the lower half and leave room for the colony's first rooms,
// which are sited against rock above them. Every colony size must still have
// a room site the moment it lands — the bug this pins had pods fill the small
// landing cavern and stall all construction.
func TestPodsLeaveRoomForTheFirstRooms(t *testing.T) {
	for _, n := range []int{1, 3, 6, 10, 16, 40} {
		cfg := testConfig()
		cfg.StartColonists = n
		cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0
		if n > 6 {
			cfg.Width, cfg.Height = 80, 50
		}
		w := newTestWorld(t, cfg)
		if _, ok := w.findRoomSite(bayWidth(lifeSupportRoom.minFac)); !ok {
			t.Errorf("%d colonists: no site for a facility room at landing", n)
		}
		for _, id := range w.entityIDsSorted() {
			if e := w.entities[id]; e.Kind == Colonist && e.podOrigin.Y <= w.Height/2 {
				t.Errorf("%d colonists: a pod landed at %v, in the upper half kept for rooms", n, e.podOrigin)
			}
		}
	}
}

// With no open floor left, a pod smashes down through the rock, clears its
// footprint, and still opens onto the colony.
func TestPodCrashesThroughRockWhenTheCavernIsFull(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.Width, cfg.Height = 30, 20
	w := newTestWorld(t, cfg)
	// Solid rock but for a single open strip across the middle: no pod fits on
	// floor with a clear margin, so every landing has to crash.
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y}, Rock)
		}
	}
	for x := 5; x < 25; x++ {
		w.SetTerrain(Point{x, 12}, Floor)
	}
	w.refreshSpatial()

	e := w.arrive(true)
	if e == nil {
		t.Fatal("no site found")
	}
	for dy := 0; dy < podHeight; dy++ {
		for dx := 0; dx < podWidth; dx++ {
			if w.TerrainAt(e.podOrigin.Add(dx, dy)) == Rock {
				t.Fatalf("rock left inside the pod footprint at %v", e.podOrigin.Add(dx, dy))
			}
		}
	}
	forEachPodMargin(e.podOrigin, false, false, func(p Point) {
		if w.TerrainAt(p) == Rock {
			t.Fatalf("rock left in the crater around the pod at %v", p)
		}
	})
	assertOwnsPod(t, w, e)
	w.refreshSpatial()
	if w.roomOf(e.Pos) != w.roomOf(Point{6, 12}) {
		t.Fatal("the crashed pod does not open onto the strip")
	}
	if !loggedContaining(w, "smashes down through the rock") {
		t.Fatal("the crash landing was not announced as one")
	}
	assertOwnsPod(t, w, e)
}

// Pod placement is part of the simulation: same seed, same landing sites.
func TestPodLandingIsDeterministic(t *testing.T) {
	sites := func() []Point {
		cfg := testConfig()
		cfg.Width, cfg.Height = 80, 50
		cfg.StartColonists = 20
		w := newTestWorld(t, cfg)
		var out []Point
		for _, id := range w.entityIDsSorted() {
			if e := w.entities[id]; e.Kind == Colonist {
				out = append(out, e.podOrigin)
			}
		}
		return out
	}
	a, b := sites(), sites()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("pod %d landed at %v, then %v", i, a[i], b[i])
		}
	}
}

// A pod never lands on the floor of a natural cavern nobody has found: it would
// open the cavern around its colonist, cut off from the colony. Here the only
// clean landing is a hidden cavern right where the search starts; the pod must
// crash beside the colony's one known corridor instead.
func TestPodsNeverLandInAnUndiscoveredCavern(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.Width, cfg.Height = 60, 30
	w := newTestWorld(t, cfg)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y}, Rock)
		}
	}
	for y := 15; y <= 26; y++ {
		for x := 20; x <= 40; x++ {
			cellAt(w, Point{x, y}).Explored = false
			w.carveHidden(Point{x, y})
		}
	}
	for y := 10; y < 29; y++ {
		w.SetTerrain(Point{5, y}, Floor)
	}
	w.refreshSpatial()

	e := w.arrive(false)
	if e == nil {
		t.Fatal("no site found")
	}
	w.refreshSpatial()
	if w.roomOf(e.Pos) != w.roomOf(Point{5, 20}) {
		t.Fatalf("the pod at %v landed cut off from the colony's corridor", e.podOrigin)
	}
}

// Pods landing side by side share their side hull as a party wall, and every
// colonist can still walk out of its pod to the rest of the colony.
func TestPodsInARowSharePartyWalls(t *testing.T) {
	cfg := testConfig()
	cfg.Width, cfg.Height = 80, 40
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 10, 0, 0, 0
	w := newTestWorld(t, cfg)
	w.refreshSpatial()
	shared := 0
	for _, id := range w.entityIDsSorted() {
		e := w.entities[id]
		if e.Kind != Colonist {
			continue
		}
		if w.roomOf(e.Pos) != w.mainRoom {
			t.Fatalf("%s is shut in its pod at %v", e.displayName(), e.podOrigin)
		}
		if right := e.podOrigin.Add(podWidth-1, 0); w.pods[right] {
			shared++
			for dy := 0; dy < podHeight; dy++ {
				if p := right.Add(0, dy); w.TerrainAt(p) != Hull {
					t.Fatalf("party wall between pods at %v and %v is %v at %v", e.podOrigin, right, w.TerrainAt(p), p)
				}
			}
		}
	}
	if shared < 5 {
		t.Fatalf("only %d of 10 pods share a wall with the next; want them packed in rows", shared)
	}
}

// A pod never lands close enough to a hidden cavern to reveal it: a landing
// that broke through would flood the cavern into view and roll its nests
// with nobody digging.
func TestPodsNeverRevealAHiddenCavern(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	cfg.Width, cfg.Height = 60, 30
	w := newTestWorld(t, cfg)
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y}, Rock)
		}
	}
	var cavern []Point
	for y := 5; y <= 25; y++ {
		for x := 30; x <= 45; x++ {
			p := Point{x, y}
			cellAt(w, p).Explored = false
			w.carveHidden(p)
			cavern = append(cavern, p)
		}
	}
	// The colony's corridor runs right alongside the cavern wall.
	for y := 3; y < 28; y++ {
		w.SetTerrain(Point{27, y}, Floor)
	}
	w.refreshSpatial()
	for i := 0; i < 6; i++ {
		if w.arrive(false) == nil {
			break
		}
		w.refreshSpatial()
	}
	for _, p := range cavern {
		if w.discovered(p) {
			t.Fatalf("a pod landing revealed the hidden cavern at %v", p)
		}
	}
}

// Pods carry crash-pod-meals give or take crash-pod-meal-spread, so lockers
// don't all run dry on the same tick. The count is a pure function of the seed
// and the colonist: the same every time, with no spread at all at 0.
func TestPodMealsSpreadAroundTheManifest(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.CrashPodMeals, w.cfg.CrashPodMealSpread = 10, 4
	seen := map[int]bool{}
	sum := 0
	for id := EntityID(1); id <= 400; id++ {
		n := w.podMeals(id)
		if n < 6 || n > 14 {
			t.Fatalf("colonist %d's pod has %d meals, outside 10±4", id, n)
		}
		if n != w.podMeals(id) {
			t.Fatalf("colonist %d's pod meals changed between calls", id)
		}
		seen[n] = true
		sum += n
	}
	if len(seen) < 7 {
		t.Fatalf("only %d distinct meal counts across 400 pods: %v", len(seen), seen)
	}
	if mean := float64(sum) / 400; mean < 9.5 || mean > 10.5 {
		t.Fatalf("mean meals per pod %.2f, want about 10", mean)
	}
	w.cfg.CrashPodMealSpread = 0
	for id := EntityID(1); id <= 20; id++ {
		if n := w.podMeals(id); n != 10 {
			t.Fatalf("with no spread, colonist %d's pod has %d meals", id, n)
		}
	}
}

// Every colonist lands with exactly one rare item, picked by the weights, and
// a gun is a shotgun crash-pod-shotgun-percent of the time. Both rolls are a
// pure function of the seed and the colonist.
func TestPodRareItemsFollowTheirWeights(t *testing.T) {
	w := propertyWorld(t)
	w.cfg.CrashPodGunWeight, w.cfg.CrashPodChickenWeight, w.cfg.CrashPodCatWeight = 50, 25, 25
	w.cfg.CrashPodShotgunPercent = 25
	var got [4]int
	guns, shotguns := 0, 0
	const pods = 2000
	for id := EntityID(1); id <= pods; id++ {
		r := w.podRareItem(id)
		if r != w.podRareItem(id) || w.podGun(id) != w.podGun(id) {
			t.Fatalf("colonist %d's rare item changed between calls", id)
		}
		got[r]++
		if r == rareGun {
			guns++
			if w.podGun(id) == Shotgun {
				shotguns++
			}
		}
	}
	if got[rareNone] != 0 {
		t.Fatalf("%d colonists landed with no rare item", got[rareNone])
	}
	if got[rareGun] < 920 || got[rareGun] > 1080 {
		t.Errorf("%d of %d pods carried a gun, want about 1000", got[rareGun], pods)
	}
	if got[rareChicken] < 430 || got[rareChicken] > 570 {
		t.Errorf("%d of %d pods carried a chicken, want about 500", got[rareChicken], pods)
	}
	if got[rareCat] < 430 || got[rareCat] > 570 {
		t.Errorf("%d of %d pods carried a cat, want about 500", got[rareCat], pods)
	}
	if shotguns < guns/4-60 || shotguns > guns/4+60 {
		t.Errorf("%d of %d guns were shotguns, want about a quarter", shotguns, guns)
	}

	w.cfg.CrashPodGunWeight, w.cfg.CrashPodChickenWeight, w.cfg.CrashPodCatWeight = 0, 0, 0
	if r := w.podRareItem(1); r != rareNone {
		t.Fatalf("with every weight zero, a colonist landed with rare item %d", r)
	}
}

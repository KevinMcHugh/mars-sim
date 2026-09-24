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
	if got := locker.held(me, Meal); got != w.cfg.CrashPodMeals {
		t.Fatalf("%s's locker holds %d meals of its own, want %d", e.displayName(), got, w.cfg.CrashPodMeals)
	}
	if !locker.ledgerBalanced() {
		t.Fatalf("%s's locker ledger does not match its contents", e.displayName())
	}
	if e.Inventory.Count(Pistol) != w.cfg.CrashPodPistols {
		t.Fatalf("%s carries %d pistols, want %d", e.displayName(), e.Inventory.Count(Pistol), w.cfg.CrashPodPistols)
	}
}

// Every way in — worldgen, the spawn command, a director arrival — is a crash
// pod the colonist owns.
func TestEveryArrivalComesInACrashPod(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0
	cfg.Width, cfg.Height = 80, 50
	cfg.Schedules = []Schedule{{
		Name: "second wave", EarliestTick: 1, LatestTick: 1,
		Occurrences: []Occurrence{{Kind: OccArrival, Count: 3}},
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
		cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0
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
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
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

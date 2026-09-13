package sim

import (
	"context"
	"math/rand"
	"testing"
	"time"
)

// newTestWorld builds a deterministic world for assertions.
func newTestWorld(t *testing.T, cfg Config) *World {
	t.Helper()
	e := NewEngine(cfg)
	return e.world
}

func testConfig() Config {
	c := DefaultConfig()
	c.Seed = 42 // deterministic
	c.Width, c.Height = 40, 24
	c.TraitChance = 0 // mechanics tests want baseline colonists; trait tests opt in
	return c
}

// The starting world should contain the configured population and an open
// landing cavern to stand in.
func TestGenerateStartingState(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	if got := w.countKind(Colonist); got != cfg.StartColonists {
		t.Fatalf("colonists: got %d want %d", got, cfg.StartColonists)
	}
	if got := w.countKind(Alien); got != cfg.StartAliens {
		t.Fatalf("aliens: got %d want %d", got, cfg.StartAliens)
	}

	floor := 0
	for _, tile := range w.tiles {
		if tile.Terrain == Floor {
			floor++
		}
	}
	if floor == 0 {
		t.Fatal("expected a carved starting cavern, found no floor")
	}
}

// Over many ticks colonists should excavate rock. We measure remaining rock
// (which only ever decreases as they dig) rather than floor, since floor is also
// consumed when they build walls and facilities on top of it. Aliens are removed
// so nobody is eaten mid-dig.
func TestColonistsExcavate(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)

	start := w.countTerrain(Rock)
	for i := 0; i < 400; i++ {
		w.step()
	}
	end := w.countTerrain(Rock)

	if end >= start {
		t.Fatalf("colonists did not excavate: rock %d -> %d", start, end)
	}
}

// With no colonists to hunt, aliens must not crash and should still be around.
func TestAliensWanderWithoutPrey(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists = 0
	w := newTestWorld(t, cfg)
	for i := 0; i < 100; i++ {
		w.step()
	}
	if got := w.countKind(Alien); got != cfg.StartAliens {
		t.Fatalf("aliens vanished: got %d want %d", got, cfg.StartAliens)
	}
}

// A cornered colonist with aliens on top of it should eventually be eaten,
// exercising the bite/remove path.
func TestAliensEatColonists(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists = 0
	cfg.StartAliens = 0
	cfg.AlienSlowness = 1
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	victim := w.spawn(Colonist, center)
	// Surround with aliens so it cannot escape.
	for _, d := range neighbors8 {
		w.spawn(Alien, center.Add(d.X, d.Y))
	}

	for i := 0; i < 200 && w.entities[victim.ID] != nil; i++ {
		w.step()
	}
	if w.entities[victim.ID] != nil {
		t.Fatalf("surrounded colonist survived with HP %d", victim.HP)
	}
}

// Determinism: the same seed yields the same floor count after N ticks.
func TestDeterministicRun(t *testing.T) {
	run := func() int {
		w := newTestWorld(t, testConfig())
		for i := 0; i < 300; i++ {
			w.step()
		}
		return floorCount(w)
	}
	if a, b := run(), run(); a != b {
		t.Fatalf("nondeterministic run: %d != %d", a, b)
	}
}

// The engine goroutine should publish snapshots and honor pause without racing.
func TestEnginePublishesAndPauses(t *testing.T) {
	cfg := testConfig()
	cfg.TicksPerSecond = 60
	eng := NewEngine(cfg)
	snaps := eng.Subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Run(ctx)

	first := <-snaps // initial frame
	// Let it advance.
	time.Sleep(50 * time.Millisecond)
	eng.Send(TogglePause{})

	// Drain to the paused frame.
	var paused *Snapshot
	deadline := time.After(time.Second)
	for {
		select {
		case s := <-snaps:
			if s.Paused {
				paused = s
			}
		case <-deadline:
			t.Fatal("never observed a paused snapshot")
		}
		if paused != nil {
			break
		}
	}
	if paused.Tick <= first.Tick {
		t.Fatalf("expected time to advance before pause: first=%d paused=%d", first.Tick, paused.Tick)
	}
}

func floorCount(w *World) int {
	n := 0
	for _, tile := range w.tiles {
		if tile.Terrain == Floor {
			n++
		}
	}
	return n
}

// A hungry colonist standing by a nutrient pod should eat and reset its food
// need instead of starving.
func TestColonistUsesNutrientPod(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	stand := center.Add(1, 0)
	w.SetTerrain(center, NutrientPod)
	w.SetTerrain(stand, Floor)

	c := w.spawn(Colonist, stand)
	c.Needs[NeedFood] = cfg.Needs[NeedFood].Max // ravenous

	for i := 0; i < cfg.Needs[NeedFood].UseTicks+10; i++ {
		w.step()
	}
	if w.entities[c.ID] == nil {
		t.Fatal("colonist starved next to a working nutrient pod")
	}
	if c.Needs[NeedFood] >= cfg.Needs[NeedFood].SeekAt {
		t.Fatalf("food need not satisfied: %d", c.Needs[NeedFood])
	}
}

// Left to their own devices, colonists should build the colony's life-support:
// at least one nutrient pod and one toilet.
func TestColonyBuildsLifeSupport(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)

	for i := 0; i < 900; i++ {
		w.step()
	}
	if got := w.countTerrain(NutrientPod); got < 1 {
		t.Fatalf("colony built no nutrient pods after 900 ticks")
	}
	if got := w.countTerrain(Toilet); got < 1 {
		t.Fatalf("colony built no toilets after 900 ticks")
	}
}

// A colonist sealed away from any rock to mine or space to build cannot feed
// itself and must eventually starve, exercising the fatal-need path.
func TestColonistStarvesWhenTrapped(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(center, Floor)
	for _, d := range neighbors8 {
		w.SetTerrain(center.Add(d.X, d.Y), Wall) // sealed pocket: no rock, no room
	}
	c := w.spawn(Colonist, center)

	for i := 0; i < 1500 && w.entities[c.ID] != nil; i++ {
		w.step()
	}
	if w.entities[c.ID] != nil {
		t.Fatalf("trapped colonist survived with HP %d, food %d", c.HP, c.Needs[NeedFood])
	}
}

// The chunk-spiral nearestOfKind must return exactly what a brute-force scan
// would, including deterministic tie-breaking on ID. This guards the subtle
// ring-stopping condition.
func TestNearestMatchesBruteForce(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed, cfg.Width, cfg.Height = 1, 120, 90
	w := newWorld(cfg, rand.New(rand.NewSource(9)))

	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 300; i++ {
		p := Point{rng.Intn(w.Width), rng.Intn(w.Height)}
		if w.occupied(p) {
			continue
		}
		kind := Colonist
		if rng.Intn(2) == 0 {
			kind = Alien
		}
		w.spawn(kind, p)
	}

	for i := 0; i < 1000; i++ {
		from := Point{rng.Intn(w.Width), rng.Intn(w.Height)}
		within := rng.Intn(200) + 1
		for _, kind := range []Kind{Colonist, Alien} {
			got, gok := w.nearestOfKind(from, kind, within)
			want, wok := bruteNearestOfKind(w, from, kind, within)
			if gok != wok {
				t.Fatalf("presence mismatch from=%v kind=%v within=%d: got %v want %v", from, kind, within, gok, wok)
			}
			if gok && (got.ID != want.ID) {
				t.Fatalf("nearest mismatch from=%v kind=%v within=%d: got #%d @%v (d=%d) want #%d @%v (d=%d)",
					from, kind, within, got.ID, got.Pos, from.Chebyshev(got.Pos),
					want.ID, want.Pos, from.Chebyshev(want.Pos))
			}
		}
	}
}

func bruteNearestOfKind(w *World, from Point, kind Kind, within int) (*Entity, bool) {
	var best *Entity
	bestDist := within + 1
	for _, e := range w.entities {
		if e.Kind != kind || !e.Alive() {
			continue
		}
		d := from.Chebyshev(e.Pos)
		if d > within {
			continue
		}
		if best == nil || d < bestDist || (d == bestDist && e.ID < best.ID) {
			best, bestDist = e, d
		}
	}
	return best, best != nil
}

// Every requested colonist should be placed, at small and large populations —
// the cavern scales to fit and placement draws from a shuffled floor list rather
// than rejection sampling that could give up.
func TestAllRequestedColonistsSpawn(t *testing.T) {
	cases := []struct {
		n, w, h int
	}{
		{8, 80, 40},
		{30, 120, 80},
		{100, 120, 80},
	}
	for _, c := range cases {
		cfg := DefaultConfig()
		cfg.Seed = 3
		cfg.StartColonists, cfg.StartAliens = c.n, 0
		cfg.Width, cfg.Height = c.w, c.h
		eng := NewEngine(cfg)
		if got := eng.world.countKind(Colonist); got != c.n {
			t.Errorf("requested %d colonists on %dx%d, spawned %d", c.n, c.w, c.h, got)
		}
	}
}

// Regression: a colony left alone must feed itself over a long run, across seeds.
// This has repeatedly regressed as new behavior landed — a non-fatal need
// starving the fatal one, a synchronized-hunger stampede deadlocking the
// facilities, walls fragmenting the colony away from food, and (once facility
// rooms arrived) builders trapped or crowds blocking construction. A crowd of 20
// on one map exercises the facility-room planning, collaborative construction,
// and the crowd-flow rules that keep pods reachable.
func TestColonyDoesNotStarveOverTime(t *testing.T) {
	for _, seed := range []int64{5, 1, 2, 7, 42, 9, 100} {
		cfg := DefaultConfig()
		cfg.Seed, cfg.StartColonists, cfg.StartAliens = seed, 20, 0
		w := NewEngine(cfg).world
		for i := 0; i < 1500; i++ {
			w.step()
		}
		if got := w.countKind(Colonist); got != cfg.StartColonists {
			t.Fatalf("seed %d: colonists starved: %d of %d survived after 1500 ticks",
				seed, got, cfg.StartColonists)
		}
	}
}

// A facility room is carved against the cavern rock with its facilities spaced
// one tile apart. The spacing is the invariant that keeps every facility
// buildable even under a crowd: no colonist using one facility can stand on the
// tile of another, which would otherwise block that one from ever being built.
func TestFacilityRoomSpacedAgainstRock(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	// Carve a known pocket with a solid rock ceiling so a site is guaranteed.
	oy := w.Height / 2
	for y := oy; y <= oy+1+roomFrontClear; y++ {
		for x := 2; x < w.Width-2; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	for x := 2; x < w.Width-2; x++ {
		w.SetTerrain(Point{x, oy - 1}, Rock) // ceiling to back the room
	}
	w.refreshSpatial()

	site, ok := w.findRoomSite(bayWidth(roomFacilities))
	if !ok {
		t.Fatal("no rock-backed room site despite a carved pocket")
	}
	for dx := 0; dx < bayWidth(roomFacilities); dx++ {
		if w.TerrainAt(Point{site.X + dx, site.Y - 1}) != Rock {
			t.Fatalf("site not backed by rock at dx=%d", dx)
		}
	}

	w.designateRoom(site, roomFacilities)
	var facs []Point
	for _, p := range w.projects {
		for _, tk := range p.tasks {
			facs = append(facs, tk.pos)
		}
	}
	if len(facs) != roomFacilities {
		t.Fatalf("expected %d facility tasks, got %d", roomFacilities, len(facs))
	}
	for i := range facs {
		for j := i + 1; j < len(facs); j++ {
			if facs[i].Chebyshev(facs[j]) <= 1 {
				t.Fatalf("facilities %v and %v are adjacent; a user would block a build",
					facs[i], facs[j])
			}
		}
	}
}

// A construction project is collaborative: several colonists claim and build its
// tasks at once, and together they finish it faster than one could alone.
func TestColonistsCollaborateOnProject(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	// A clear pocket backed by rock, plus a crew of colonists in front of it.
	oy := w.Height / 2
	for y := oy; y <= oy+1+roomFrontClear; y++ {
		for x := 2; x < w.Width-2; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	for x := 2; x < w.Width-2; x++ {
		w.SetTerrain(Point{x, oy - 1}, Rock)
	}
	w.refreshSpatial()

	site, ok := w.findRoomSite(bayWidth(roomFacilities))
	if !ok {
		t.Fatal("no rock-backed room site despite a carved pocket")
	}
	w.designateRoom(site, roomFacilities)
	for i := 0; i < roomFacilities; i++ {
		w.spawn(Colonist, Point{site.X + i, oy + 1 + roomFrontClear})
	}

	maxConcurrent := 0
	done := false
	for i := 0; i < 400 && !done; i++ {
		w.step()
		builders := 0
		for _, e := range w.entities {
			if e.Kind == Colonist && e.task != nil {
				builders++
			}
		}
		if builders > maxConcurrent {
			maxConcurrent = builders
		}
		if len(w.projects) == 0 {
			done = true
		}
	}
	if !done {
		t.Fatal("project never completed")
	}
	if maxConcurrent < 2 {
		t.Fatalf("expected multiple colonists building at once, saw at most %d", maxConcurrent)
	}
	if got := w.countTerrain(NutrientPod); got < 1 {
		t.Fatal("collaboration finished but built no nutrient pod")
	}
}

// Facility construction is deterministic: the same seed lays out the same
// facilities in the same places, so runs stay reproducible.
func TestFacilityLayoutDeterministic(t *testing.T) {
	run := func() []Point {
		cfg := DefaultConfig()
		cfg.Seed, cfg.StartColonists, cfg.StartAliens = 7, 20, 0
		w := NewEngine(cfg).world
		for i := 0; i < 800; i++ {
			w.step()
		}
		var facs []Point
		for y := 0; y < w.Height; y++ {
			for x := 0; x < w.Width; x++ {
				if t := w.TerrainAt(Point{x, y}); t == NutrientPod || t == Toilet {
					facs = append(facs, Point{x, y})
				}
			}
		}
		return facs
	}
	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("nondeterministic facility count: %d != %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic facility layout at %d: %v != %v", i, a[i], b[i])
		}
	}
}

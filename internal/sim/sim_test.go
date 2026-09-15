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
	if got := w.countKind(Cat); got != cfg.StartCats {
		t.Fatalf("cats: got %d want %d", got, cfg.StartCats)
	}
	if got := w.countKind(Mouse); got != cfg.StartMice {
		t.Fatalf("mice: got %d want %d", got, cfg.StartMice)
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

// A tired colonist standing by a bed should sleep and reset its sleep need
// instead of staying exhausted, reusing the same JobUse machinery as the pod.
func TestColonistUsesBed(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	stand := center.Add(1, 0)
	w.SetTerrain(center, Bed)
	w.SetTerrain(stand, Floor)

	c := w.spawn(Colonist, stand)
	c.Needs[NeedSleep], c.needSince[NeedSleep] = cfg.Needs[NeedSleep].Max, w.tick // dead on its feet
	// Clear the other (staggered) needs so nothing fatal outranks sleep here.
	c.Needs[NeedFood], c.needSince[NeedFood] = 0, w.tick
	c.Needs[NeedBladder], c.needSince[NeedBladder] = 0, w.tick

	for i := 0; i < cfg.Needs[NeedSleep].UseTicks+10; i++ {
		w.step()
	}
	if c.Needs[NeedSleep] >= cfg.Needs[NeedSleep].SeekAt {
		t.Fatalf("sleep need not satisfied: %d", c.Needs[NeedSleep])
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

// Once life support is in, the colony should raise a dormitory so colonists have
// somewhere to sleep. Beds are planned only after pods and toilets, so this needs
// more runway than the life-support check.
func TestColonyBuildsDormitory(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)

	built := false
	for i := 0; i < 2500 && !built; i++ {
		w.step()
		built = w.countTerrain(Bed) >= 1
	}
	if !built {
		t.Fatalf("colony built no dormitory bunks after 2500 ticks")
	}
}

// Once an urgent colonist starts an emergency nutrient pod, crossing the need
// threshold again must not reset its build progress every tick.
func TestUrgentColonistFinishesEmergencyBuild(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)
	center := Point{w.Width / 2, w.Height / 2}
	c := w.spawn(Colonist, center)
	c.Needs[NeedFood] = cfg.Needs[NeedFood].SeekAt
	target, ok := w.findBuildSpot(c.Pos, 20)
	if !ok {
		t.Fatal("no emergency build spot")
	}
	w.assignBuild(c, NutrientPod, target)

	for i := 0; i < cfg.FacilityBuildTicks+10 && w.TerrainAt(target) != NutrientPod; i++ {
		w.tick++
		w.colonistTurn(c)
	}
	if got := w.TerrainAt(target); got != NutrientPod {
		t.Fatalf("emergency build never finished: target terrain %v, progress %d", got, c.Progress)
	}
}

// A non-fatal need (bladder, here) must trigger the same self-rescue as a
// fatal one: with no reachable toilet, no project task to help with, and none
// under construction, a colonist stuck on its own builds one rather than
// waiting indefinitely — the "stuck in a need loop" complaint a
// fatal-needs-only fallback left unaddressed. Driving colonistTurn directly
// (rather than step) keeps the normal room planner from ever running, so the
// only way a toilet appears is this fallback.
func TestUrgentNonFatalNeedTriggersEmergencyBuild(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)
	center := Point{w.Width / 2, w.Height / 2}
	c := w.spawn(Colonist, center)
	c.Needs[NeedBladder], c.needSince[NeedBladder] = cfg.Needs[NeedBladder].SeekAt, w.tick
	// Clear the other (staggered) needs so bladder is the one being addressed.
	c.Needs[NeedFood], c.needSince[NeedFood] = 0, w.tick
	c.Needs[NeedSleep], c.needSince[NeedSleep] = 0, w.tick

	w.tick++
	w.colonistTurn(c)
	if c.Job != JobBuild || c.BuildKind != Toilet {
		t.Fatalf("expected an emergency toilet build, got job=%v buildKind=%v", c.Job, c.BuildKind)
	}

	for i := 0; i < cfg.FacilityBuildTicks+40 && w.TerrainAt(c.Target) != Toilet; i++ {
		w.tick++
		w.colonistTurn(c)
	}
	if got := w.TerrainAt(c.Target); got != Toilet {
		t.Fatalf("emergency toilet build never finished: target terrain %v, progress %d", got, c.Progress)
	}
}

// Once a facility of a kind already exists, an urgent colonist must not just
// blindly queue at it forever: if the colony still wants more of that
// facility than it has, and there is a reachable task to help with, it helps
// build instead. Without this, once one facility exists no colonist ever
// helps build a second — exactly the gridlock a growing, undersupplied colony
// hits ("stuck in a need loop" even with unclaimed mining frontier and a
// buildable project sitting right there).
func TestUrgentColonistHelpsBuildWhenFacilityUndersupplied(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	cfg.ColonistsPerFacility = 1 // two colonists want two toilets, not one
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	// A short open corridor: an existing, reachable toilet plus an unclaimed,
	// reachable project task further along.
	for dx := -2; dx <= 3; dx++ {
		w.SetTerrain(Point{center.X + dx, center.Y}, Floor)
	}
	w.SetTerrain(Point{center.X - 2, center.Y}, Toilet)
	taskPos := Point{center.X + 3, center.Y}
	w.refreshSpatial()

	w.projects = append(w.projects, &project{
		id: 1, name: "test room",
		tasks: []*buildTask{{pos: taskPos, terrain: Wall, phase: 0}},
	})

	c := w.spawn(Colonist, center)
	w.spawn(Colonist, center) // just to raise desiredFacilities to 2
	c.Needs[NeedBladder], c.needSince[NeedBladder] = cfg.Needs[NeedBladder].SeekAt, w.tick
	c.Needs[NeedFood], c.needSince[NeedFood] = 0, w.tick
	c.Needs[NeedSleep], c.needSince[NeedSleep] = 0, w.tick

	w.tick++
	w.colonistTurn(c)
	if c.Job != JobBuild || c.task == nil {
		t.Fatalf("expected the colonist to help build more capacity instead of queueing at the existing toilet; got job=%v task=%v", c.Job, c.task)
	}
	if c.Target != taskPos {
		t.Fatalf("expected the colonist to claim the task at %v, got target %v", taskPos, c.Target)
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

// A hungry mouse standing by a nutrient pod should feed itself instead of
// starving, reusing the same JobUse machinery colonists use.
func TestMouseEatsFromPod(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	stand := center.Add(1, 0)
	w.SetTerrain(center, NutrientPod)
	w.SetTerrain(stand, Floor)

	m := w.spawn(Mouse, stand)
	m.Needs[NeedFood] = cfg.Needs[NeedFood].SeekAt // hungry enough to seek

	for i := 0; i < cfg.Needs[NeedFood].UseTicks+20; i++ {
		w.step()
	}
	if w.entities[m.ID] == nil {
		t.Fatal("mouse starved next to a working nutrient pod")
	}
	if w.needLevel(m, NeedFood) >= cfg.Needs[NeedFood].SeekAt {
		t.Fatalf("mouse food need not satisfied: %d", w.needLevel(m, NeedFood))
	}
}

// A mouse with no reachable food must eventually starve, exercising the fatal
// food need for mice.
func TestMouseStarvesWithoutFood(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(center, Floor)
	for _, d := range neighbors8 {
		w.SetTerrain(center.Add(d.X, d.Y), Wall) // sealed pocket: no pod within reach
	}
	m := w.spawn(Mouse, center)

	for i := 0; i < 1000 && w.entities[m.ID] != nil; i++ {
		w.step()
	}
	if w.entities[m.ID] != nil {
		t.Fatalf("walled-in mouse survived with HP %d, food %d", m.HP, w.needLevel(m, NeedFood))
	}
}

// A cat cornered against a mouse it cannot escape should catch and eat it,
// exercising the pounce/remove path.
func TestCatEatsMouse(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.CatSlowness = 1
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(center, Floor)
	// Wall the mouse in on every side but one, where the cat waits: the mouse
	// cannot flee, so the cat must catch it.
	catSpot := center.Add(1, 0)
	for _, d := range neighbors8 {
		n := center.Add(d.X, d.Y)
		if n.Equal(catSpot) {
			w.SetTerrain(n, Floor)
			continue
		}
		w.SetTerrain(n, Wall)
	}
	mouse := w.spawn(Mouse, center)
	w.spawn(Cat, catSpot)

	for i := 0; i < 50 && w.entities[mouse.ID] != nil; i++ {
		w.step()
	}
	if w.entities[mouse.ID] != nil {
		t.Fatal("cornered mouse was never caught by the adjacent cat")
	}
}

// A colonist with nothing pressing to do — no threat, no urgent need, no work —
// should crush a mouse it sees. Sealing a small floor pocket leaves the colonist
// idle (no rock to mine, no reachable construction), so it stomps the pest.
func TestIdleColonistStompsMouse(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	// Wall a 3x3 pocket so no mineable rock borders its floor.
	for y := -2; y <= 2; y++ {
		for x := -2; x <= 2; x++ {
			w.SetTerrain(center.Add(x, y), Wall)
		}
	}
	for y := -1; y <= 1; y++ {
		for x := -1; x <= 1; x++ {
			w.SetTerrain(center.Add(x, y), Floor)
		}
	}

	m := w.spawn(Mouse, center.Add(1, 0))
	c := w.spawn(Colonist, center)
	// Fully satisfied, so no need preempts the stomp.
	c.Needs[NeedFood], c.Needs[NeedBladder] = 0, 0
	c.needSince[NeedFood], c.needSince[NeedBladder] = w.tick, w.tick

	for i := 0; i < 10 && w.entities[m.ID] != nil; i++ {
		w.step()
	}
	if w.entities[m.ID] != nil {
		t.Fatal("idle colonist never stomped the nearby mouse")
	}
}

// Two adjacent mice of opposite sex should mate, and the female should carry a
// litter to term and give birth, growing the population.
func TestMiceBreedAndGiveBirth(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.MouseGestationTicks = 4
	cfg.MouseLitterMin, cfg.MouseLitterMax = 3, 3
	cfg.MouseBreedCooldown = 100
	cfg.MouseMaturityTicks = 100
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	for y := -2; y <= 2; y++ {
		for x := -2; x <= 2; x++ {
			w.SetTerrain(center.Add(x, y), Floor)
		}
	}
	male := w.spawn(Mouse, center)
	male.sex = SexMale
	female := w.spawn(Mouse, center.Add(1, 0))
	female.sex = SexFemale

	for i := 0; i < cfg.MouseGestationTicks+5; i++ {
		w.step()
	}
	if got, want := w.countKind(Mouse), 2+cfg.MouseLitterMin; got != want {
		t.Fatalf("mouse count after a litter: got %d want %d", got, want)
	}
}

// Two mice of the same sex must never breed, so the population stays put.
func TestSameSexMiceDoNotBreed(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice = 0, 0, 0, 0
	cfg.MouseGestationTicks = 4
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	for y := -2; y <= 2; y++ {
		for x := -2; x <= 2; x++ {
			w.SetTerrain(center.Add(x, y), Floor)
		}
	}
	a := w.spawn(Mouse, center)
	a.sex = SexMale
	b := w.spawn(Mouse, center.Add(1, 0))
	b.sex = SexMale

	for i := 0; i < cfg.MouseGestationTicks+5; i++ {
		w.step()
	}
	if got := w.countKind(Mouse); got != 2 {
		t.Fatalf("same-sex mice bred: mouse count %d, want 2", got)
	}
}

// With no mice to hunt, cats must not crash and should still be around.
func TestCatsWanderWithoutPrey(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartMice = 0, 0, 0
	cfg.StartCats = 3
	w := newTestWorld(t, cfg)
	for i := 0; i < 100; i++ {
		w.step()
	}
	if got := w.countKind(Cat); got != cfg.StartCats {
		t.Fatalf("cats vanished: got %d want %d", got, cfg.StartCats)
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

// Regression: on a large, mature map the facility rooms can fill completely.
// Hungry colonists must untangle crowded access, get a fair turn at vacancies,
// and have enough grace to finish a reachable food journey. With permanent ID
// priority and no journey grace, this seed fell from 20 colonists to 9.
func TestLargeColonyDoesNotGridlockAtFacilities(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 9
	cfg.StartColonists, cfg.StartAliens = 20, 0
	cfg.Width, cfg.Height = 200, 200
	w := NewEngine(cfg).world

	for i := 0; i < 10000; i++ {
		w.step()
	}
	if got := w.countKind(Colonist); got != cfg.StartColonists {
		t.Fatalf("facility crowd starved colonists: %d of %d survived after %d ticks",
			got, cfg.StartColonists, w.tick)
	}
}

// A room site can back onto another room's already-placed wall instead of
// requiring untouched rock, so rooms can sit flush against each other and
// share that boundary once a cave's easy rock-backed edges are used up.
func TestRoomSiteCanBackOntoAnotherRoomsWall(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	// Blank the procedurally generated cave to solid rock first, so the only
	// possible room site is the one this test carves — otherwise a genuinely
	// rock-backed site elsewhere on the map could satisfy a loose assertion.
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y}, Rock)
		}
	}

	width := bayWidth(roomFacilities)
	// Site everything near the map center — findRoomSite prefers the site
	// nearest center — and carve only the exact footprint roomSiteClear
	// requires (not a whole open row), so no other column could also qualify
	// and mask a regression in the assertion below.
	oy := w.Height / 2
	ox := w.Width / 2
	backY := oy - 1
	frontY := roomFrontWallY(oy)
	// Simulate an already-built neighboring room: a wall row with no rock
	// anywhere behind it (backY-1 lands here).
	for x := ox; x < ox+width; x++ {
		w.SetTerrain(Point{x, backY - 1}, Wall)
	}
	// The new room's own footprint plus its side lanes.
	for y := backY; y <= frontY; y++ {
		for x := ox - 2; x <= ox+width+1; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	// The front approach lane.
	for x := ox - 2; x <= ox+width+1; x++ {
		w.SetTerrain(Point{x, frontY + roomApproach}, Floor)
	}
	w.refreshSpatial()

	site, ok := w.findRoomSite(width)
	if !ok {
		t.Fatal("expected a room site backed by an existing wall")
	}
	if want := (Point{ox, oy}); site != want {
		t.Fatalf("site = %v, want %v (backed by the wall at y=%d)", site, want, backY-1)
	}
}

// A room can also sit flush against a neighbor side by side, sharing that
// neighbor's wall outright as its own party wall — no exterior lane needed on
// that side, and no redundant wall task of its own there.
func TestRoomSiteSharesSideWallWithNeighbor(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	// Blank the procedurally generated cave to solid rock first, so the only
	// possible room site is the one this test carves.
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			w.SetTerrain(Point{x, y}, Rock)
		}
	}

	width := 3
	oy := w.Height / 2
	ox := w.Width / 2
	backY := oy - 1
	frontY := roomFrontWallY(oy)

	// An existing neighbor's wall column immediately to the left — this
	// room's whole left side, shared outright.
	for y := backY; y <= frontY; y++ {
		w.SetTerrain(Point{ox - 1, y}, Wall)
	}
	// This room's own interior, plus its fresh right side wall column and
	// that side's exterior lane.
	for y := backY; y <= frontY; y++ {
		for x := ox; x <= ox+width+1; x++ { // interior, right wall, right lane
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	// The front approach lane: only from the shared (left) wall's column
	// rightward through the fresh (right) side's lane — nothing left of
	// ox-1, proving the shared side needed no exterior lane of its own.
	for x := ox - 1; x <= ox+width+1; x++ {
		w.SetTerrain(Point{x, frontY + roomApproach}, Floor)
	}
	w.refreshSpatial()

	site, ok := w.findRoomSite(width)
	if !ok {
		t.Fatal("expected a room site sharing a neighbor's side wall")
	}
	if want := (Point{ox, oy}); site != want {
		t.Fatalf("site = %v, want %v (sharing the wall at x=%d)", site, want, ox-1)
	}

	w.designateRoom(dormRoom, site, 2) // bayWidth(2) == 3, matching the site carved above
	for _, tk := range w.projects[0].tasks {
		if tk.pos == (Point{ox - 1, backY}) || tk.pos == (Point{ox - 1, frontY}) {
			t.Fatalf("designateRoom added a redundant task %v on the shared wall", tk.pos)
		}
	}
	sawRightWall := false
	for _, tk := range w.projects[0].tasks {
		if tk.pos == (Point{ox + width, backY}) {
			sawRightWall = true
		}
	}
	if !sawRightWall {
		t.Fatal("designateRoom should still build its own (fresh) right side wall")
	}
}

// A facility room has a complete placed-wall perimeter and centered doorway.
// Walls are phase zero so facilities cannot come online and attract users until
// the enclosure is complete.
func TestFacilityRoomHasCompleteWallsDoorAndBuildPhases(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	// Carve a known niche with rock beyond its future placed back wall.
	oy := w.Height / 2
	for y := oy - 1; y <= roomFrontWallY(oy)+roomApproach; y++ {
		for x := 1; x < w.Width-1; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	for x := 2; x < w.Width-2; x++ {
		w.SetTerrain(Point{x, oy - 2}, Rock)
	}
	w.refreshSpatial()

	site, ok := w.findRoomSite(bayWidth(roomFacilities))
	if !ok {
		t.Fatal("no room site despite a clear pocket")
	}

	w.designateRoom(lifeSupportRoom, site, roomFacilities)
	var facs []Point
	walls := make(map[Point]bool)
	for _, p := range w.projects {
		for _, tk := range p.tasks {
			switch tk.terrain {
			case Wall:
				walls[tk.pos] = true
				if tk.phase != roomWallPhase {
					t.Fatalf("wall %v has phase %d", tk.pos, tk.phase)
				}
			case NutrientPod, Toilet:
				facs = append(facs, tk.pos)
				if tk.phase != roomFitPhase {
					t.Fatalf("facility %v has phase %d", tk.pos, tk.phase)
				}
			}
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
	width := bayWidth(roomFacilities)
	backY := site.Y - 1
	frontY := roomFrontWallY(site.Y)
	door := Point{site.X + width/2, frontY}
	if walls[door] {
		t.Fatalf("doorway %v was designated as a wall", door)
	}
	for y := backY; y <= frontY; y++ {
		if !walls[Point{site.X - 1, y}] || !walls[Point{site.X + width, y}] {
			t.Fatalf("room is missing a side wall on row %d", y)
		}
	}
	for x := site.X; x < site.X+width; x++ {
		if p := (Point{x, backY}); !walls[p] {
			t.Fatalf("room is missing back wall %v", p)
		}
		if p := (Point{x, frontY}); p != door && !walls[p] {
			t.Fatalf("room is missing front wall %v", p)
		}
	}

	// A facility cannot be claimed while any phase-zero wall remains, and a wall
	// already occupied when the project is designated must be left for later.
	blockedWall := Point{site.X, backY}
	w.spawn(Colonist, blockedWall)
	if task, ok := w.claimNearestTask(Point{door.X, door.Y + 1}, 999); !ok ||
		task.terrain != Wall || task.pos == blockedWall {
		t.Fatalf("first claimed task = %#v, want a wall", task)
	}
}

// A construction project is collaborative: several colonists claim and build its
// tasks at once, and together they finish it faster than one could alone.
func TestColonistsCollaborateOnProject(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	// A clear rock-backed niche plus a crew of colonists in front of it.
	oy := w.Height / 2
	for y := oy - 1; y <= roomFrontWallY(oy)+roomApproach; y++ {
		for x := 1; x < w.Width-1; x++ {
			w.SetTerrain(Point{x, y}, Floor)
		}
	}
	for x := 2; x < w.Width-2; x++ {
		w.SetTerrain(Point{x, oy - 2}, Rock)
	}
	w.refreshSpatial()

	site, ok := w.findRoomSite(bayWidth(roomFacilities))
	if !ok {
		t.Fatal("no room site despite a clear pocket")
	}
	w.designateRoom(lifeSupportRoom, site, roomFacilities)
	for i := 0; i < roomFacilities; i++ {
		w.spawn(Colonist, Point{site.X + i, roomFrontWallY(oy) + roomApproach})
	}

	maxConcurrent := 0
	done := false
	for i := 0; i < 800 && !done; i++ {
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
	if got := w.countTerrain(Wall); got < 1 {
		t.Fatal("collaboration finished but built no walls")
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

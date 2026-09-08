package sim

import (
	"context"
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

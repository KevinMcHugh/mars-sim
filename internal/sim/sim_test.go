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

// Over many ticks colonists should excavate additional floor. We remove aliens
// first so nobody gets eaten mid-dig, isolating the mining behavior.
func TestColonistsExcavate(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens = 0
	w := newTestWorld(t, cfg)

	start := floorCount(w)
	for i := 0; i < 400; i++ {
		w.step()
	}
	end := floorCount(w)

	if end <= start {
		t.Fatalf("colonists did not excavate: floor %d -> %d", start, end)
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

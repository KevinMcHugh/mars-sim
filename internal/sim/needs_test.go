package sim

import "testing"

// Need levels are computed lazily from a base + elapsed ticks, clamp at Max, and
// reset to zero when satisfied.
func TestNeedLevelIsLazy(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	spec := w.cfg.Needs[NeedFood]
	// Spawn staggers starting levels; pin a known baseline for the lazy math.
	c.Needs[NeedFood], c.needSince[NeedFood] = 0, 0

	w.tick = 100
	if got, want := w.needLevel(c, NeedFood), spec.Rise*100; got != want {
		t.Fatalf("level at tick 100: got %d want %d", got, want)
	}

	// Clamps at Max.
	w.tick = 1 << 20
	if got := w.needLevel(c, NeedFood); got != spec.Max {
		t.Fatalf("level should clamp to Max %d, got %d", spec.Max, got)
	}

	// Reset drops to zero and starts rising again from now.
	w.tick = 200
	w.resetNeed(c, NeedFood)
	if got := w.needLevel(c, NeedFood); got != 0 {
		t.Fatalf("level right after reset: got %d want 0", got)
	}
	w.tick = 230
	if got, want := w.needLevel(c, NeedFood), spec.Rise*30; got != want {
		t.Fatalf("level 30 ticks after reset: got %d want %d", got, want)
	}
}

// A colonist with no reachable work rests: it stops re-searching and stays put
// until its rest interval elapses. Sealing a lone floor tile behind walls leaves
// nothing to mine (no rock borders floor) and nowhere to build (the only floor
// is occupied), so the colonist must idle.
func TestIdleColonistRests(t *testing.T) {
	w := roomsTestWorld(40, 24)
	c := Point{20, 12}
	w.SetTerrain(c, Floor)
	for _, d := range neighbors8 {
		w.SetTerrain(c.Add(d.X, d.Y), Wall)
	}
	w.refreshSpatial()
	col := w.spawn(Colonist, c)

	// A couple of ticks to settle into rest (well before any need gets urgent).
	w.step()
	w.step()
	if !col.resting {
		t.Fatalf("colonist with no available work should be resting")
	}

	// While resting it should not move.
	start := col.Pos
	for i := 0; i < w.cfg.RestTicks; i++ {
		w.step()
	}
	if col.Pos != start {
		t.Fatalf("resting colonist moved from %v to %v", start, col.Pos)
	}
}

// Resting must not prevent a colonist from tending an urgent need: once hunger
// crosses its threshold, a resting colonist heads for the pod and eats.
func TestRestingColonistStillEats(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)

	center := Point{w.Width / 2, w.Height / 2}
	stand := center.Add(1, 0)
	w.SetTerrain(center, NutrientPod)
	w.SetTerrain(stand, Floor)
	w.refreshSpatial()

	col := w.spawn(Colonist, stand)
	col.resting = true
	col.wakeTick = 1 << 30 // pretend it intends to rest "forever"
	// Make it hungry right now.
	col.Needs[NeedFood] = cfg.Needs[NeedFood].SeekAt

	ate := false
	for i := 0; i < cfg.Needs[NeedFood].UseTicks+5; i++ {
		w.step()
		if w.needLevel(col, NeedFood) == 0 {
			ate = true
			break
		}
	}
	if !ate {
		t.Fatalf("resting colonist did not eat despite urgent hunger (level %d)", w.needLevel(col, NeedFood))
	}
}

// A fatal need (food) must outrank a non-fatal one (bladder) that is more past
// its threshold — otherwise bladder, which rises faster and caps further over,
// permanently deferred food and starved the colonist.
func TestFatalNeedOutranksNonFatal(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	food, bladder := w.cfg.Needs[NeedFood], w.cfg.Needs[NeedBladder]
	if !food.Fatal || bladder.Fatal {
		t.Skip("assumes food fatal, bladder not")
	}
	// Food barely urgent; bladder maxed (further over its threshold).
	c.Needs[NeedFood], c.needSince[NeedFood] = food.SeekAt+1, w.tick
	c.Needs[NeedBladder], c.needSince[NeedBladder] = bladder.Max, w.tick

	need, urgent := w.mostUrgentNeed(c)
	if !urgent || need != NeedFood {
		t.Fatalf("fatal food need should win over maxed bladder: got need=%v urgent=%v", need, urgent)
	}
}

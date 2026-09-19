package sim

import "testing"

// Each need independently projects its lazy level into a phase and normalized
// actionable pressure.
func TestNeedPhaseTransitionsAndPressure(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	n := NeedBladder
	spec := w.cfg.Needs[n]

	cases := []struct {
		level    int
		phase    NeedPhase
		pressure int
	}{
		{0, NeedSatisfied, 0},
		{1, NeedGrowing, 0},
		{spec.SeekAt, NeedPressing, 1},
		{spec.CriticalAt - 1, NeedPressing, 74},
		{spec.CriticalAt, NeedCritical, 75},
		{spec.Max, NeedCritical, 100},
	}
	for _, tc := range cases {
		c.Needs[n], c.needSince[n] = tc.level, w.tick
		w.syncNeedPhase(c, n)
		if got := c.needPhase[n]; got != tc.phase {
			t.Errorf("level %d phase = %v, want %v", tc.level, got, tc.phase)
		}
		if got := needPressure(tc.level, spec); got != tc.pressure {
			t.Errorf("level %d pressure = %d, want %d", tc.level, got, tc.pressure)
		}
	}

	w.resetNeed(c, n)
	if c.needPhase[n] != NeedSatisfied {
		t.Fatalf("phase after reset = %v, want satisfied", c.needPhase[n])
	}
}

func TestNeedPhaseTracksLazyElapsedTimeAndNextBoundary(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	n := NeedBladder
	spec := w.cfg.Needs[n]
	c.Needs[n], c.needSince[n] = 0, w.tick

	w.syncNeedPhase(c, n)
	wantGrowing := w.tick + 1
	if c.nextNeedPhaseTick[n] != wantGrowing {
		t.Fatalf("next satisfied boundary = %d, want %d", c.nextNeedPhaseTick[n], wantGrowing)
	}
	w.tick = wantGrowing
	w.syncNeedPhase(c, n)
	if c.needPhase[n] != NeedGrowing {
		t.Fatalf("phase after lazy rise = %v, want growing", c.needPhase[n])
	}
	wantPressing := w.tick + (spec.SeekAt-w.needLevel(c, n)+c.needRise[n]-1)/c.needRise[n]
	if c.nextNeedPhaseTick[n] != wantPressing {
		t.Fatalf("next growing boundary = %d, want %d", c.nextNeedPhaseTick[n], wantPressing)
	}
	w.tick = wantPressing
	w.syncNeedPhase(c, n)
	if c.needPhase[n] != NeedPressing {
		t.Fatalf("phase at lazy seek crossing = %v, want pressing", c.needPhase[n])
	}
	wantCritical := w.tick + (spec.CriticalAt-w.needLevel(c, n)+c.needRise[n]-1)/c.needRise[n]
	if c.nextNeedPhaseTick[n] != wantCritical {
		t.Fatalf("next pressing boundary = %d, want %d", c.nextNeedPhaseTick[n], wantCritical)
	}
	w.tick = wantCritical
	w.syncNeedPhase(c, n)
	if c.needPhase[n] != NeedCritical || c.nextNeedPhaseTick[n] != 0 {
		t.Fatalf("critical phase=%v boundary=%d, want critical and unscheduled",
			c.needPhase[n], c.nextNeedPhaseTick[n])
	}
}

func TestNeedPressureDegenerateThresholds(t *testing.T) {
	spec := NeedSpec{SeekAt: 50, CriticalAt: 50, Max: 100}
	if got := needPressure(50, spec); got != 75 {
		t.Errorf("SeekAt == CriticalAt pressure = %d, want 75", got)
	}
	spec = NeedSpec{SeekAt: 50, CriticalAt: 100, Max: 100}
	if got := needPressure(100, spec); got != 100 {
		t.Errorf("CriticalAt == Max pressure = %d, want 100", got)
	}
}

func TestZeroRiseSocialNeedNeverBecomesPressing(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	n := NeedSocial
	c.Needs[n] = w.cfg.Needs[n].SeekAt - 1
	c.needSince[n], c.needRise[n] = w.tick, 0 // resolved Asocial behavior
	w.tick += 10_000
	w.syncNeedPhase(c, n)
	if c.needPhase[n] != NeedGrowing || c.nextNeedPhaseTick[n] != 0 {
		t.Fatalf("zero-rise social need phase=%v boundary=%d, want growing and unscheduled",
			c.needPhase[n], c.nextNeedPhaseTick[n])
	}
	c.Needs[n] = 0
	w.syncNeedPhase(c, n)
	if c.needPhase[n] != NeedSatisfied {
		t.Fatalf("zero-level social phase = %v, want satisfied", c.needPhase[n])
	}
}

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

func TestEatingRecoversOnlyStarvationDamage(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	c.HP -= 3 // an unrelated wound must remain after eating
	c.Needs[NeedFood], c.needSince[NeedFood] = w.cfg.Needs[NeedFood].Max, w.tick

	w.applyStarvation(c)
	if got, want := c.HP, c.MaxHP-3-w.cfg.StarveDamage; got != want {
		t.Fatalf("starvation HP: got %d want %d", got, want)
	}
	w.resetNeed(c, NeedFood)
	if got, want := c.HP, c.MaxHP-3; got != want {
		t.Fatalf("eating should recover deprivation but not wounds: got HP %d want %d", got, want)
	}
}

func TestEntityDoesNotStarveWhileSeekingReachableFood(t *testing.T) {
	w := roomsTestWorld(20, 20)
	pod := Point{10, 5}
	carve(w, Point{5, 5}, Point{9, 5}, Floor)
	w.SetTerrain(pod, NutrientPod)
	w.refreshSpatial()
	c := w.spawn(Colonist, Point{5, 5})
	c.Needs[NeedFood], c.needSince[NeedFood] = w.cfg.Needs[NeedFood].Max, w.tick
	c.Job, c.Need = JobUse, NeedFood

	hp := c.HP
	w.applyStarvation(c)
	if c.HP != hp {
		t.Fatalf("colonist seeking reachable food lost HP: %d -> %d", hp, c.HP)
	}
}

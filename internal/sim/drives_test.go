package sim

import "testing"

// Each need independently projects its lazy level into a phase and normalized
// actionable pressure.
func TestNeedPhaseTransitionsAndPressure(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	n := DriveBladder
	spec := w.cfg.Drives[n]

	cases := []struct {
		level    int
		phase    DrivePhase
		pressure int
	}{
		{0, DriveSatisfied, 0},
		{1, DriveGrowing, 0},
		{spec.SeekAt, DrivePressing, 1},
		{spec.CriticalAt - 1, DrivePressing, 74},
		{spec.CriticalAt, DriveCritical, 75},
		{spec.Max, DriveCritical, 100},
	}
	for _, tc := range cases {
		w.setDrive(c, n, tc.level)
		w.syncDrivePhase(c, n)
		if got := c.drives[n].phase; got != tc.phase {
			t.Errorf("level %d phase = %v, want %v", tc.level, got, tc.phase)
		}
		if got := drivePressure(tc.level, spec); got != tc.pressure {
			t.Errorf("level %d pressure = %d, want %d", tc.level, got, tc.pressure)
		}
	}

	w.resetDrive(c, n)
	if c.drives[n].phase != DriveSatisfied {
		t.Fatalf("phase after reset = %v, want satisfied", c.drives[n].phase)
	}
}

func TestNeedPhaseTracksLazyElapsedTimeAndNextBoundary(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	n := DriveBladder
	spec := w.cfg.Drives[n]
	w.setDrive(c, n, 0)

	w.syncDrivePhase(c, n)
	wantGrowing := w.tick + 1
	if c.drives[n].nextCrossing != wantGrowing {
		t.Fatalf("next satisfied boundary = %d, want %d", c.drives[n].nextCrossing, wantGrowing)
	}
	w.tick = wantGrowing
	w.syncDrivePhase(c, n)
	if c.drives[n].phase != DriveGrowing {
		t.Fatalf("phase after lazy rise = %v, want growing", c.drives[n].phase)
	}
	r := c.drives[n].rate
	wantPressing := w.tick + (spec.SeekAt*driveUnit-w.driveFelt(c, n)+r-1)/r
	if c.drives[n].nextCrossing != wantPressing {
		t.Fatalf("next growing boundary = %d, want %d", c.drives[n].nextCrossing, wantPressing)
	}
	w.tick = wantPressing
	w.syncDrivePhase(c, n)
	if c.drives[n].phase != DrivePressing {
		t.Fatalf("phase at lazy seek crossing = %v, want pressing", c.drives[n].phase)
	}
	wantCritical := w.tick + (spec.CriticalAt*driveUnit-w.driveFelt(c, n)+r-1)/r
	if c.drives[n].nextCrossing != wantCritical {
		t.Fatalf("next pressing boundary = %d, want %d", c.drives[n].nextCrossing, wantCritical)
	}
	w.tick = wantCritical
	w.syncDrivePhase(c, n)
	// The ceiling is a band of its own (bladder's consequence, soiling,
	// applies there), so critical still schedules one crossing: reaching it.
	wantCeiling := w.tick + (spec.Max*driveUnit-w.driveFelt(c, n)+r-1)/r
	if c.drives[n].phase != DriveCritical || c.drives[n].nextCrossing != wantCeiling {
		t.Fatalf("critical phase=%v boundary=%d, want critical and the ceiling at %d",
			c.drives[n].phase, c.drives[n].nextCrossing, wantCeiling)
	}
	w.tick = wantCeiling
	w.syncDrivePhase(c, n)
	if c.drives[n].phase != DriveCritical || c.drives[n].nextCrossing != 0 {
		t.Fatalf("at the ceiling phase=%v boundary=%d, want critical and unscheduled",
			c.drives[n].phase, c.drives[n].nextCrossing)
	}
}

func TestNeedPressureDegenerateThresholds(t *testing.T) {
	spec := DriveSpec{SeekAt: 50, CriticalAt: 50, Max: 100}
	if got := drivePressure(50, spec); got != 75 {
		t.Errorf("SeekAt == CriticalAt pressure = %d, want 75", got)
	}
	spec = DriveSpec{SeekAt: 50, CriticalAt: 100, Max: 100}
	if got := drivePressure(100, spec); got != 100 {
		t.Errorf("CriticalAt == Max pressure = %d, want 100", got)
	}
}

func TestZeroRiseSocialNeedNeverBecomesPressing(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	n := DriveSocial
	w.setDrive(c, n, w.cfg.Drives[n].SeekAt-1)
	setDriveRate(w, c, n, 0) // resolved Asocial behavior
	w.tick += 10_000
	w.syncDrivePhase(c, n)
	if c.drives[n].phase != DriveGrowing || c.drives[n].nextCrossing != 0 {
		t.Fatalf("zero-rise social need phase=%v boundary=%d, want growing and unscheduled",
			c.drives[n].phase, c.drives[n].nextCrossing)
	}
	w.setDrive(c, n, 0)
	w.syncDrivePhase(c, n)
	if c.drives[n].phase != DriveSatisfied {
		t.Fatalf("zero-level social phase = %v, want satisfied", c.drives[n].phase)
	}
}

// Drive levels are computed lazily from a base + elapsed ticks, clamp at Max, and
// reset to zero when satisfied.
func TestNeedLevelIsLazy(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	spec := w.cfg.Drives[DriveFood]
	// Spawn staggers starting levels; pin a known baseline for the lazy math.
	w.tick = 0
	w.setDrive(c, DriveFood, 0)

	w.tick = 100
	if got, want := w.driveLevel(c, DriveFood), c.drives[DriveFood].rate*100/driveUnit; got != want {
		t.Fatalf("level at tick 100: got %d want %d", got, want)
	}

	// Clamps at Max.
	w.tick = 1 << 20
	if got := w.driveLevel(c, DriveFood); got != spec.Max {
		t.Fatalf("level should clamp to Max %d, got %d", spec.Max, got)
	}

	// Reset drops to zero and starts rising again from now.
	w.tick = 200
	w.resetDrive(c, DriveFood)
	if got := w.driveLevel(c, DriveFood); got != 0 {
		t.Fatalf("level right after reset: got %d want 0", got)
	}
	w.tick = 230
	if got, want := w.driveLevel(c, DriveFood), c.drives[DriveFood].rate*30/driveUnit; got != want {
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

// Resting must not prevent a colonist from tending an urgent drive: once hunger
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
	w.setDrive(col, DriveFood, cfg.Drives[DriveFood].SeekAt)

	ate := false
	for i := 0; i < cfg.Drives[DriveFood].UseTicks+5; i++ {
		w.step()
		if w.driveLevel(col, DriveFood) == 0 {
			ate = true
			break
		}
	}
	if !ate {
		t.Fatalf("resting colonist did not eat despite urgent hunger (level %d)", w.driveLevel(col, DriveFood))
	}
}

// A fatal drive (food) must outrank a non-fatal one (bladder) that is more past
// its threshold — otherwise bladder, which rises faster and caps further over,
// permanently deferred food and starved the colonist.
func TestFatalNeedOutranksNonFatal(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	food, bladder := w.cfg.Drives[DriveFood], w.cfg.Drives[DriveBladder]
	if !food.Fatal() || bladder.Fatal() {
		t.Skip("assumes food fatal, bladder not")
	}
	// Food barely urgent; bladder maxed (further over its threshold).
	w.setDrive(c, DriveFood, food.SeekAt+1)
	w.setDrive(c, DriveBladder, bladder.Max)

	need, urgent := w.mostUrgentDrive(c)
	if !urgent || need != DriveFood {
		t.Fatalf("fatal food need should win over maxed bladder: got need=%v urgent=%v", need, urgent)
	}
}

func TestEatingRecoversOnlyStarvationDamage(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	c.HP -= 3 // an unrelated wound must remain after eating
	w.setDrive(c, DriveFood, w.cfg.Drives[DriveFood].Max)

	w.applyDriveConsequences(c)
	if got, want := c.HP, c.MaxHP-3-w.cfg.StarveDamage; got != want {
		t.Fatalf("starvation HP: got %d want %d", got, want)
	}
	w.resetDrive(c, DriveFood)
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
	w.setDrive(c, DriveFood, w.cfg.Drives[DriveFood].Max)
	c.Job, c.Drive = JobUse, DriveFood

	hp := c.HP
	w.applyDriveConsequences(c)
	if c.HP != hp {
		t.Fatalf("colonist seeking reachable food lost HP: %d -> %d", hp, c.HP)
	}
}

func TestTicksPerDayFollowsSleep(t *testing.T) {
	cfg := DefaultConfig()
	sleep := cfg.Drives[DriveSleep]
	want := (sleep.SeekAt*driveUnit+sleep.Rate-1)/sleep.Rate + sleep.UseTicks
	if got := cfg.TicksPerDay(); got != want {
		t.Fatalf("TicksPerDay = %d, want %d (awake to SeekAt, then a night in bed)", got, want)
	}

	// Retuning sleep retunes the day.
	cfg.Drives[DriveSleep].Rate = driveUnit
	cfg.Drives[DriveSleep].SeekAt = 300
	cfg.Drives[DriveSleep].UseTicks = 20
	if got := cfg.TicksPerDay(); got != 320 {
		t.Fatalf("TicksPerDay = %d, want 320", got)
	}
	// A sleep drive that never rises still gives a usable, positive day.
	cfg.Drives[DriveSleep].Rate = 0
	if got := cfg.TicksPerDay(); got != cfg.Drives[DriveSleep].Max+20 {
		t.Fatalf("TicksPerDay with zero rise = %d", got)
	}
}

func TestDayOfCountsLandingAsDayOne(t *testing.T) {
	// 740 ticks a day, landing at 06:00: a quarter day, 185 ticks, in. The
	// day turns over at midnight, 555 ticks after landing.
	for _, c := range []struct{ tick, day int }{{0, 1}, {554, 1}, {555, 2}, {1294, 2}, {1295, 3}} {
		if got := DayOf(c.tick, 740); got != c.day {
			t.Errorf("DayOf(%d, 740) = %d, want %d", c.tick, got, c.day)
		}
	}
	if got := DayOf(5, 0); got != 6 {
		t.Errorf("DayOf with a zero day length = %d, want 6", got)
	}
}

func TestMinuteOfDayStartsAtLandingHour(t *testing.T) {
	for _, c := range []struct{ tick, minute int }{
		{0, LandingHour * 60}, // landing
		{370, 18 * 60},        // half a day later
		{554, 1438},           // just before midnight
		{555, 0},              // midnight, the next day
		{740, LandingHour * 60},
	} {
		if got := MinuteOfDay(c.tick, 740); got != c.minute {
			t.Errorf("MinuteOfDay(%d, 740) = %d, want %d", c.tick, got, c.minute)
		}
	}
	if got := MinuteOfDay(5, 0); got != 0 {
		t.Errorf("MinuteOfDay with a zero day length = %d, want 0", got)
	}
}

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
		c.Drives[n], c.driveSince[n] = tc.level, w.tick
		w.syncDrivePhase(c, n)
		if got := c.drivePhase[n]; got != tc.phase {
			t.Errorf("level %d phase = %v, want %v", tc.level, got, tc.phase)
		}
		if got := drivePressure(tc.level, spec); got != tc.pressure {
			t.Errorf("level %d pressure = %d, want %d", tc.level, got, tc.pressure)
		}
	}

	w.resetDrive(c, n)
	if c.drivePhase[n] != DriveSatisfied {
		t.Fatalf("phase after reset = %v, want satisfied", c.drivePhase[n])
	}
}

func TestNeedPhaseTracksLazyElapsedTimeAndNextBoundary(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	n := DriveBladder
	spec := w.cfg.Drives[n]
	c.Drives[n], c.driveSince[n] = 0, w.tick

	w.syncDrivePhase(c, n)
	wantGrowing := w.tick + 1
	if c.nextDrivePhaseTick[n] != wantGrowing {
		t.Fatalf("next satisfied boundary = %d, want %d", c.nextDrivePhaseTick[n], wantGrowing)
	}
	w.tick = wantGrowing
	w.syncDrivePhase(c, n)
	if c.drivePhase[n] != DriveGrowing {
		t.Fatalf("phase after lazy rise = %v, want growing", c.drivePhase[n])
	}
	wantPressing := w.tick + (spec.SeekAt-w.driveLevel(c, n)+c.driveRise[n]-1)/c.driveRise[n]
	if c.nextDrivePhaseTick[n] != wantPressing {
		t.Fatalf("next growing boundary = %d, want %d", c.nextDrivePhaseTick[n], wantPressing)
	}
	w.tick = wantPressing
	w.syncDrivePhase(c, n)
	if c.drivePhase[n] != DrivePressing {
		t.Fatalf("phase at lazy seek crossing = %v, want pressing", c.drivePhase[n])
	}
	wantCritical := w.tick + (spec.CriticalAt-w.driveLevel(c, n)+c.driveRise[n]-1)/c.driveRise[n]
	if c.nextDrivePhaseTick[n] != wantCritical {
		t.Fatalf("next pressing boundary = %d, want %d", c.nextDrivePhaseTick[n], wantCritical)
	}
	w.tick = wantCritical
	w.syncDrivePhase(c, n)
	if c.drivePhase[n] != DriveCritical || c.nextDrivePhaseTick[n] != 0 {
		t.Fatalf("critical phase=%v boundary=%d, want critical and unscheduled",
			c.drivePhase[n], c.nextDrivePhaseTick[n])
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
	c.Drives[n] = w.cfg.Drives[n].SeekAt - 1
	c.driveSince[n], c.driveRise[n] = w.tick, 0 // resolved Asocial behavior
	w.tick += 10_000
	w.syncDrivePhase(c, n)
	if c.drivePhase[n] != DriveGrowing || c.nextDrivePhaseTick[n] != 0 {
		t.Fatalf("zero-rise social need phase=%v boundary=%d, want growing and unscheduled",
			c.drivePhase[n], c.nextDrivePhaseTick[n])
	}
	c.Drives[n] = 0
	w.syncDrivePhase(c, n)
	if c.drivePhase[n] != DriveSatisfied {
		t.Fatalf("zero-level social phase = %v, want satisfied", c.drivePhase[n])
	}
}

// Need levels are computed lazily from a base + elapsed ticks, clamp at Max, and
// reset to zero when satisfied.
func TestNeedLevelIsLazy(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	spec := w.cfg.Drives[DriveFood]
	// Spawn staggers starting levels; pin a known baseline for the lazy math.
	c.Drives[DriveFood], c.driveSince[DriveFood] = 0, 0

	w.tick = 100
	if got, want := w.driveLevel(c, DriveFood), spec.Rise*100; got != want {
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
	if got, want := w.driveLevel(c, DriveFood), spec.Rise*30; got != want {
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
	col.Drives[DriveFood] = cfg.Drives[DriveFood].SeekAt

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

// A fatal need (food) must outrank a non-fatal one (bladder) that is more past
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
	c.Drives[DriveFood], c.driveSince[DriveFood] = food.SeekAt+1, w.tick
	c.Drives[DriveBladder], c.driveSince[DriveBladder] = bladder.Max, w.tick

	need, urgent := w.mostUrgentDrive(c)
	if !urgent || need != DriveFood {
		t.Fatalf("fatal food need should win over maxed bladder: got need=%v urgent=%v", need, urgent)
	}
}

func TestEatingRecoversOnlyStarvationDamage(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	c.HP -= 3 // an unrelated wound must remain after eating
	c.Drives[DriveFood], c.driveSince[DriveFood] = w.cfg.Drives[DriveFood].Max, w.tick

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
	c.Drives[DriveFood], c.driveSince[DriveFood] = w.cfg.Drives[DriveFood].Max, w.tick
	c.Job, c.Drive = JobUse, DriveFood

	hp := c.HP
	w.applyDriveConsequences(c)
	if c.HP != hp {
		t.Fatalf("colonist seeking reachable food lost HP: %d -> %d", hp, c.HP)
	}
}

// memoriesOf counts the occurrences of one reaction in a colonist's memories,
// collapsed runs included.
func memoriesOf(e *Entity, rule RuleID) int {
	n := 0
	for _, m := range e.Memories {
		if m.Rule == rule {
			n += m.Count
		}
	}
	return n
}

// A social drive at its ceiling is felt as loneliness: once on arrival, again
// every ConsequenceEvery ticks while it stays, and afresh after it is met.
func TestLonelinessIsFeltAtTheCeiling(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	spec := w.cfg.Drives[DriveSocial]
	if spec.Consequence != ConsequenceLoneliness || spec.ConsequenceEvery <= 1 {
		t.Skip("assumes social's consequence is loneliness with a repeat interval")
	}
	c.driveRise[DriveSocial] = 0 // hold the level where the test puts it
	c.Drives[DriveSocial], c.driveSince[DriveSocial] = spec.Max-1, w.tick

	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 0 {
		t.Fatalf("felt lonely below the ceiling: %d", got)
	}

	c.Drives[DriveSocial] = spec.Max
	valence := c.affect.Valence
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 1 {
		t.Fatalf("felt-lonely at the ceiling = %d, want 1", got)
	}
	if c.affect.Valence >= valence {
		t.Fatalf("loneliness did not lower valence: %d -> %d", valence, c.affect.Valence)
	}

	w.tick += spec.ConsequenceEvery - 1
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 1 {
		t.Fatalf("felt lonely again before consequence-every: %d", got)
	}
	w.tick++
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 2 {
		t.Fatalf("felt-lonely after consequence-every = %d, want 2", got)
	}

	w.resetDrive(c, DriveSocial)
	c.Drives[DriveSocial] = spec.Max
	w.tick++
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-lonely"); got != 3 {
		t.Fatalf("a new stay at the ceiling should be felt at once: %d, want 3", got)
	}
}

// Only the side of a conversation that came to it wanting company socialized.
func TestConversationWhileLonelySocializes(t *testing.T) {
	w := roomsTestWorld(20, 20)
	carve(w, Point{5, 5}, Point{6, 5}, Floor)
	lonely := w.spawn(Colonist, Point{5, 5})
	content := w.spawn(Colonist, Point{6, 5})
	spec := w.cfg.Drives[DriveSocial]
	lonely.Drives[DriveSocial], lonely.driveSince[DriveSocial] = spec.SeekAt, w.tick
	content.Drives[DriveSocial], content.driveSince[DriveSocial] = spec.SeekAt-1, w.tick

	w.finishTalk(lonely, content)
	if got := memoriesOf(lonely, "socialized"); got != 1 {
		t.Errorf("lonely partner socialized = %d, want 1", got)
	}
	if got := memoriesOf(content, "socialized"); got != 0 {
		t.Errorf("content partner socialized = %d, want 0", got)
	}
}

// A sleep drive at its ceiling drops the colonist where it stands for
// PassOutTicks; it then comes to with the drive met and the memory of it.
func TestSleepDeprivedColonistPassesOut(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)
	at := Point{w.Width / 2, w.Height / 2}
	w.SetTerrain(at, Floor)
	c := w.spawn(Colonist, at)
	for n := DriveKind(0); n < numDrives; n++ {
		c.Drives[n], c.driveSince[n] = 0, w.tick
	}
	c.Drives[DriveSleep] = cfg.Drives[DriveSleep].Max

	w.step()
	if c.State != PassedOut || c.passedOutUntil == 0 {
		t.Fatalf("state = %v, want passed out", c.State)
	}
	if got := memoriesOf(c, "passed-out"); got != 1 {
		t.Fatalf("passed-out memories = %d, want 1", got)
	}
	for i := 1; i < cfg.PassOutTicks; i++ {
		w.step()
		if c.State != PassedOut || c.Pos != at || c.Job != JobNone {
			t.Fatalf("tick %d: state %v job %v at %v; want lying where it fell", i, c.State, c.Job, c.Pos)
		}
	}
	w.step()
	if c.passedOutUntil != 0 || c.State == PassedOut {
		t.Fatalf("did not come to after %d ticks", cfg.PassOutTicks)
	}
	if lvl := w.driveLevel(c, DriveSleep); lvl > 1 {
		t.Fatalf("sleep drive after passing out = %d, want met", lvl)
	}
	if got := memoriesOf(c, "passed-out"); got != 1 {
		t.Fatalf("passed out again: %d memories", got)
	}
}

// A colonist already asleep beside its bed when the drive reaches the ceiling
// finishes its sleep there rather than passing out.
func TestAsleepInBedDoesNotPassOut(t *testing.T) {
	w := roomsTestWorld(20, 20)
	bed, stand := Point{6, 5}, Point{5, 5}
	carve(w, stand, stand, Floor)
	w.SetTerrain(bed, Bed)
	c := w.spawn(Colonist, stand)
	c.Drives[DriveSleep], c.driveSince[DriveSleep] = w.cfg.Drives[DriveSleep].Max, w.tick
	c.Job, c.Drive, c.useFacility, c.useFacilitySet = JobUse, DriveSleep, bed, true

	w.applyDriveConsequences(c)
	if c.passedOutUntil != 0 {
		t.Fatal("passed out while asleep in bed")
	}
	c.Job = JobNone
	w.applyDriveConsequences(c)
	if c.passedOutUntil == 0 {
		t.Fatal("did not pass out once out of bed")
	}
}

// A bladder at its ceiling empties where the colonist stands: the drive resets
// and the colonist remembers the embarrassment. Anyone close enough sees it,
// and a Tidy witness minds more.
func TestFullBladderWetsSelfAndIsSeen(t *testing.T) {
	w := roomsTestWorld(30, 20)
	carve(w, Point{5, 5}, Point{20, 5}, Floor)
	c := w.spawn(Colonist, Point{5, 5})
	near := w.spawn(Colonist, Point{6, 5})
	tidy := w.spawn(Colonist, Point{7, 5})
	near.Profile.Traits, tidy.Profile.Traits = nil, []Trait{TraitTidy}
	far := w.spawn(Colonist, Point{5 + w.cfg.GoreSightRadius + 5, 5})
	spec := w.cfg.Drives[DriveBladder]
	if spec.Consequence != ConsequenceSoiling {
		t.Skip("assumes bladder's consequence is soiling")
	}
	c.Drives[DriveBladder], c.driveSince[DriveBladder] = spec.Max, w.tick
	grip := c.affect.Grip

	w.applyDriveConsequences(c)
	if lvl := w.driveLevel(c, DriveBladder); lvl != 0 {
		t.Fatalf("bladder after wetting self = %d, want 0", lvl)
	}
	if got := memoriesOf(c, "soiled-self"); got != 1 {
		t.Fatalf("soiled-self memories = %d, want 1", got)
	}
	if c.affect.Grip >= grip {
		t.Fatalf("embarrassment did not lower grip: %d -> %d", grip, c.affect.Grip)
	}
	if memoriesOf(near, "witnessed-soiling") != 1 || memoriesOf(tidy, "witnessed-soiling") != 1 {
		t.Fatalf("nearby colonists did not see it: %d, %d",
			memoriesOf(near, "witnessed-soiling"), memoriesOf(tidy, "witnessed-soiling"))
	}
	if memoriesOf(far, "witnessed-soiling") != 0 {
		t.Fatal("a colonist out of sight saw it")
	}
	if tidy.affect.Valence >= near.affect.Valence {
		t.Fatalf("Tidy witness valence %d not below plain witness %d", tidy.affect.Valence, near.affect.Valence)
	}

	// Once is once: the drive is met, so nothing more until it fills again.
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "soiled-self"); got != 1 {
		t.Fatalf("wet self again with an empty bladder: %d", got)
	}
}

// A colonist already at the toilet when the bladder tops out is using it.
func TestAtToiletDoesNotWetSelf(t *testing.T) {
	w := roomsTestWorld(20, 20)
	toilet, stand := Point{6, 5}, Point{5, 5}
	carve(w, stand, stand, Floor)
	w.SetTerrain(toilet, Toilet)
	c := w.spawn(Colonist, stand)
	c.Drives[DriveBladder], c.driveSince[DriveBladder] = w.cfg.Drives[DriveBladder].Max, w.tick
	c.Job, c.Drive, c.useFacility, c.useFacilitySet = JobUse, DriveBladder, toilet, true

	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "soiled-self"); got != 0 {
		t.Fatal("wet self while at the toilet")
	}
}

// quietDrives empties every drive of e as of now, for a test about something
// other than drives. Zeroing Drives alone is not enough: a level is the base
// plus its rise since driveSince, so a base of zero with an old driveSince
// still reads at the ceiling — and since drives have consequences there, a
// "fed" colonist would wet itself, pass out and feel lonely every tick.
func (w *World) quietDrives(e *Entity) {
	for k := range e.Drives {
		e.Drives[k], e.driveSince[k] = 0, w.tick
	}
}

// Soiling's memories and log line use the colonist's own pronouns.
func TestWetSelfUsesPronouns(t *testing.T) {
	for _, tc := range []struct {
		g    Gender
		want string
	}{{GenderMan, "himself"}, {GenderWoman, "herself"}, {GenderNonbinary, "themself"}} {
		w := roomsTestWorld(20, 20)
		carve(w, Point{5, 5}, Point{6, 5}, Floor)
		c := w.spawn(Colonist, Point{5, 5})
		seer := w.spawn(Colonist, Point{6, 5})
		c.Profile.Gender = tc.g
		w.wetSelf(c)
		if got, want := c.Memories[len(c.Memories)-1].Text, "Wet "+tc.want+"."; got != want {
			t.Errorf("%v: memory %q, want %q", tc.g, got, want)
		}
		if got, want := seer.Memories[len(seer.Memories)-1].Text, "Saw "+c.displayName()+" wet "+tc.want+"."; got != want {
			t.Errorf("%v: witness memory %q, want %q", tc.g, got, want)
		}
	}
}

// Dirty work raises the hygiene drive on top of its steady rise.
func TestDirtyWorkAddsGrime(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	c.Drives[DriveHygiene], c.driveSince[DriveHygiene] = 100, w.tick
	w.addGrime(c, w.cfg.GrimeMine)
	if got, want := w.driveLevel(c, DriveHygiene), 100+w.cfg.GrimeMine; got != want {
		t.Fatalf("hygiene after a dig = %d, want %d", got, want)
	}
	before := w.driveLevel(c, DriveHygiene)
	c.Drives[DriveBladder], c.driveSince[DriveBladder] = w.cfg.Drives[DriveBladder].Max, w.tick
	w.applyDriveConsequences(c)
	if got, want := w.driveLevel(c, DriveHygiene), min(before+w.cfg.GrimeSoil, w.cfg.Drives[DriveHygiene].Max); got != want {
		t.Fatalf("hygiene after wetting self = %d, want %d", got, want)
	}
	w.addGrime(c, 10*w.cfg.Drives[DriveHygiene].Max)
	if got := w.driveLevel(c, DriveHygiene); got != w.cfg.Drives[DriveHygiene].Max {
		t.Fatalf("grime pushed hygiene past its ceiling: %d", got)
	}
}

// A hygiene drive at its ceiling is felt as filth, like loneliness.
func TestFilthIsFeltAtTheCeiling(t *testing.T) {
	w := roomsTestWorld(20, 20)
	c := w.spawn(Colonist, Point{5, 5})
	spec := w.cfg.Drives[DriveHygiene]
	c.Drives[DriveHygiene], c.driveSince[DriveHygiene] = spec.Max, w.tick
	valence := c.affect.Valence
	w.applyDriveConsequences(c)
	if got := memoriesOf(c, "felt-filthy"); got != 1 {
		t.Fatalf("felt-filthy at the ceiling = %d, want 1", got)
	}
	if c.affect.Valence >= valence {
		t.Fatalf("filth did not lower valence: %d -> %d", valence, c.affect.Valence)
	}
}

// A dirty colonist walks to a shower, washes, and is clean.
func TestColonistWashesAtAShower(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens = 0, 0
	w := newTestWorld(t, cfg)
	center := Point{w.Width / 2, w.Height / 2}
	stand := center.Add(1, 0)
	w.SetTerrain(center, Shower)
	w.SetTerrain(stand, Floor)
	w.refreshSpatial()
	c := w.spawn(Colonist, stand)
	w.quietDrives(c)
	c.Drives[DriveHygiene] = cfg.Drives[DriveHygiene].CriticalAt

	for i := 0; i < cfg.Drives[DriveHygiene].UseTicks+10 && memoriesOf(c, "washed") == 0; i++ {
		w.step()
	}
	if memoriesOf(c, "washed") != 1 {
		t.Fatal("the colonist never washed")
	}
	if lvl := w.driveLevel(c, DriveHygiene); lvl >= cfg.Drives[DriveHygiene].SeekAt {
		t.Fatalf("hygiene after washing = %d", lvl)
	}
}

// With life support, bunks and a silo in place, the colony marks out a
// washroom for the hygiene drive.
func TestColonyPlansAWashroom(t *testing.T) {
	cfg := testConfig()
	cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0
	w := newTestWorld(t, cfg)
	center := Point{w.Width / 2, w.Height / 2}
	desired := w.desiredFacilities(w.countKind(Colonist))
	for i, kind := range []Terrain{NutrientPod, Toilet, Bed} {
		for n := 0; n < desired; n++ {
			w.SetTerrain(center.Add(-3+i, -3-n), kind)
		}
	}
	w.SetTerrain(center.Add(1, -3), Storage)
	w.refreshSpatial()

	w.planRooms()
	if !named(w.projects, washRoom.name) {
		t.Fatalf("no washroom planned; projects = %v", projectNames(w.projects))
	}
}

// A Tidy colonist's hygiene drive rises faster: it wants a wash sooner.
func TestTidyFeelsGrimySooner(t *testing.T) {
	w := roomsTestWorld(20, 20)
	plain := w.spawn(Colonist, Point{5, 5})
	tidy := w.spawn(Colonist, Point{6, 5})
	plain.Profile.Traits, tidy.Profile.Traits = nil, []Trait{TraitTidy}
	w.resolveTraitEffects(plain)
	w.resolveTraitEffects(tidy)
	if tidy.driveRise[DriveHygiene] <= plain.driveRise[DriveHygiene] {
		t.Fatalf("Tidy hygiene rise %d, plain %d: want Tidy faster",
			tidy.driveRise[DriveHygiene], plain.driveRise[DriveHygiene])
	}
	if tidy.driveRise[DriveFood] != plain.driveRise[DriveFood] {
		t.Fatal("Tidy changed a drive other than hygiene")
	}
}

package sim

import (
	"slices"
	"testing"
)

// driveWorld is a small DefaultConfig world (real activity percents, no
// traits) with cfg edited first, and one colonist standing in it.
func driveWorld(t *testing.T, edit func(*Config)) (*World, *Entity) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Width, cfg.Height = 20, 20
	cfg.TraitChance = 0
	if edit != nil {
		edit(&cfg)
	}
	if err := cfg.CheckDrives(); err != nil {
		t.Fatal(err)
	}
	w := newWorld(cfg, newPCG(1))
	carve(w, Point{2, 2}, Point{17, 17}, Floor)
	w.refreshSpatial()
	return w, w.spawn(Colonist, Point{5, 5})
}

// caffeine is the illustrative profile from docs/drives.md: it masks sleep
// and slows it, fades, then crashes.
func caffeine(stacks bool) DriveEffectProfile {
	stage := func(ticks, offset, change int) DriveEffectStage {
		var st DriveEffectStage
		st.Ticks = ticks
		st.Drive[DriveSleep] = DriveEffectTerms{Offset: offset, RateChange: change}
		return st
	}
	return DriveEffectProfile{Name: "caffeine", Stacks: stacks, Stages: []DriveEffectStage{
		stage(90, 250, -60), // kick-in: two hours
		stage(45, 120, -30), // fading: one hour
		stage(22, 0, +20),   // crash: half an hour
	}}
}

// The lazy level never gains or loses growth across rate changes: it always
// equals the sum of the rate that was in force on every tick, however the
// activity, traits and effects shift underneath.
func TestLazyDriveMatchesPerTickSum(t *testing.T) {
	w, e := driveWorld(t, func(c *Config) { c.DriveEffects = []DriveEffectProfile{caffeine(true)} })
	d := DriveSleep
	w.setDrive(e, d, 100)
	want := w.driveTrue(e, d)
	states := []State{Idle, Mining, Moving, Sleeping, Hauling, Idle}
	for i := 0; i < 400; i++ {
		rate := e.drives[d].rate
		w.tick++
		want = clampInt(want+rate, 0, w.cfg.Drives[d].Max*driveUnit)
		switch {
		case i%37 == 0:
			e.State = states[(i/37)%len(states)]
			w.syncDriveActivity(e)
		case i == 50:
			e.Profile.Traits = []Trait{TraitBigEater}
			w.resolveTraitEffects(e)
		case i == 80 || i == 200:
			w.applyEffect(e, 0)
		}
		w.advanceEffects(e)
		if got := w.driveTrue(e, d); got != want {
			t.Fatalf("tick %d: true sleep %d, want the per-tick sum %d", w.tick, got, want)
		}
	}
}

// Modifiers stack in a fixed order with integer arithmetic: base, then the
// activity's percent, the traits', and each effect's.
func TestDriveRateComposes(t *testing.T) {
	w, e := driveWorld(t, func(c *Config) {
		var st DriveEffectStage
		st.Ticks = 100
		st.Drive[DriveFood] = DriveEffectTerms{RateChange: -60}
		c.DriveEffects = []DriveEffectProfile{{Name: "appetite suppressant", Stages: []DriveEffectStage{st}}}
	})
	e.Profile.Traits = []Trait{TraitBigEater}
	w.resolveTraitEffects(e)
	e.State = Mining
	w.syncDriveActivity(e)
	w.applyEffect(e, 0)
	food := w.cfg.Drives[DriveFood]
	want := food.Rate * food.Activity[DriveLabor.index()] / 100
	want = want * 150 / 100 // Big Eater: +50%
	want = want * 40 / 100  // the effect: -60%
	if got := e.drives[DriveFood].rate; got != want {
		t.Fatalf("food rate %d, want %d", got, want)
	}
}

// Every player-facing activity is filed under a real drive activity, both
// while doing it and while walking to it.
func TestEveryActivityHasADriveActivity(t *testing.T) {
	for a := Activity(0); a < NumActivities; a++ {
		row := activityDrives[a]
		if row.Doing == driveActivityNone || row.Walking == driveActivityNone {
			t.Errorf("activity %v has no drive activity (%v / %v): add a row to activityDrives", a, row.Doing, row.Walking)
		}
	}
}

func TestDriveActivityOf(t *testing.T) {
	e := &Entity{Kind: Colonist}
	cases := []struct {
		state State
		job   JobKind
		want  DriveActivity
	}{
		{Sleeping, JobUse, DriveAsleep},
		{Mining, JobMine, DriveLabor},
		{Moving, JobMine, DriveWorking},
		{Moving, JobCarry, DriveLabor},
		{Idle, JobNone, DriveIdle},
		{Crafting, JobCraft, DriveWorking},
	}
	for _, c := range cases {
		e.State, e.Job = c.state, c.job
		if got := driveActivityOf(e); got != c.want {
			t.Errorf("%v on %v: drive activity %v, want %v", c.state, c.job, got, c.want)
		}
	}
	// Wanting to sleep is not sleeping: a colonist waiting for a free bed is idle.
	e.State, e.Job, e.focus = Idle, JobUse, FocusSleep
	if got := driveActivityOf(e); got != DriveIdle {
		t.Errorf("waiting for a bed: drive activity %v, want idle", got)
	}
}

// Hunger is slowest asleep and fastest at labor, and company is not missed in
// bed: the shipped activity percents reach the rates.
func TestActivityScalesDriveGrowth(t *testing.T) {
	w, e := driveWorld(t, nil)
	rateIn := func(s State) [numDrives]int {
		e.State = s
		w.syncDriveActivity(e)
		var r [numDrives]int
		for d := range r {
			r[d] = e.drives[d].rate
		}
		return r
	}
	asleep, idle, mining := rateIn(Sleeping), rateIn(Idle), rateIn(Mining)
	if !(asleep[DriveFood] < idle[DriveFood] && idle[DriveFood] < mining[DriveFood]) {
		t.Errorf("food rates asleep %d, idle %d, mining %d: want strictly rising", asleep[DriveFood], idle[DriveFood], mining[DriveFood])
	}
	if asleep[DriveFood] == 0 {
		t.Error("hunger stopped in bed; it should only slow")
	}
	if asleep[DriveSocial] != 0 {
		t.Errorf("social grows %d a tick asleep, want 0", asleep[DriveSocial])
	}
	// Sleep is what falls asleep; awake it builds at one rate whatever the
	// colonist does, because the calendar is derived from it.
	if asleep[DriveSleep] >= 0 {
		t.Errorf("sleep rate asleep %d, want it falling", asleep[DriveSleep])
	}
	if idle[DriveSleep] != mining[DriveSleep] {
		t.Errorf("sleep rate idle %d, mining %d: the calendar needs it activity-neutral awake", idle[DriveSleep], mining[DriveSleep])
	}
}

// Each drive activity is a config knob on every drive, named by the class.
func TestDriveActivityKnobs(t *testing.T) {
	cfg := DefaultConfig()
	keys := map[string]bool{}
	for _, k := range Knobs(&cfg) {
		keys[k.Key] = true
		if k.Key == "drives.bladder.activity.labor" && k.Name != "drive-bladder-activity-labor" {
			t.Errorf("flag for %s is %q", k.Key, k.Name)
		}
	}
	for d := DriveKind(0); d < numDrives; d++ {
		for _, name := range driveActivityNames {
			if key := "drives." + d.String() + ".activity." + name; !keys[key] {
				t.Errorf("no knob %s", key)
			}
		}
	}
	data := []byte("drives:\n  food:\n    activity:\n      asleep: 0\n")
	if _, err := ApplyConfigFile(&cfg, data, ConfigFileName); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Drives[DriveFood].Activity[DriveAsleep.index()]; got != 0 {
		t.Errorf("drives.food.activity.asleep = %d after setting it to 0", got)
	}
}

// Caffeine masks sleep and slows it while it lasts, and the crash brings the
// felt level back up to the true level, which kept growing underneath.
func TestCaffeineMasksThenCrashes(t *testing.T) {
	w, e := driveWorld(t, func(c *Config) { c.DriveEffects = []DriveEffectProfile{caffeine(true)} })
	plainW, plain := driveWorld(t, nil) // the same colonist without the coffee
	d := DriveSleep
	w.setDrive(e, d, 500)
	plainW.setDrive(plain, d, 500)
	base := e.drives[d].rate
	w.applyEffect(e, 0)
	if got := w.driveLevel(e, d); got != 250 {
		t.Fatalf("felt sleep %d right after the coffee, want 500 masked by 250", got)
	}
	if got := w.driveTrue(e, d); got != 500*driveUnit {
		t.Fatalf("true sleep %d: masking must not touch it", got)
	}
	if got := e.drives[d].rate; got != base*40/100 {
		t.Fatalf("sleep rate %d under caffeine, want %d", got, base*40/100)
	}
	feltBeforeCrash := 0
	for i := 0; i < 90+45+22+5; i++ {
		w.tick++
		plainW.tick++
		w.advanceEffects(e)
		if w.tick == 90+45 {
			feltBeforeCrash = w.driveLevel(e, d)
		}
		if w.tick == 90+45+1 && w.driveLevel(e, d) <= feltBeforeCrash {
			t.Fatalf("no crash: felt sleep %d after the masking ended, %d before", w.driveLevel(e, d), feltBeforeCrash)
		}
	}
	if len(e.effects) != 0 || e.nextEffectTick != 0 {
		t.Fatalf("caffeine still running after its last stage: %+v", e.effects)
	}
	if w.driveLevel(e, d) != floorDiv(w.driveTrue(e, d), driveUnit) {
		t.Fatal("felt and true sleep differ once the caffeine is gone")
	}
	if w.driveLevel(e, d) >= plainW.driveLevel(plain, d) {
		t.Fatalf("caffeinated sleep %d is not below the plain colonist's %d", w.driveLevel(e, d), plainW.driveLevel(plain, d))
	}
	if e.drives[d].rate != base {
		t.Fatalf("sleep rate %d after the caffeine, want %d again", e.drives[d].rate, base)
	}
}

// A profile that stacks runs a second instance alongside the first; one that
// does not starts over.
func TestEffectStacking(t *testing.T) {
	for _, stacks := range []bool{true, false} {
		w, e := driveWorld(t, func(c *Config) { c.DriveEffects = []DriveEffectProfile{caffeine(stacks)} })
		w.setDrive(e, DriveSleep, 600)
		w.applyEffect(e, 0)
		w.tick += 10
		w.applyEffect(e, 0)
		wantN, wantMask := 1, 250
		if stacks {
			wantN, wantMask = 2, 500
		}
		if len(e.effects) != wantN || e.drives[DriveSleep].offset != wantMask*driveUnit {
			t.Errorf("stacks=%v: %d instances masking %d, want %d masking %d",
				stacks, len(e.effects), e.drives[DriveSleep].offset/driveUnit, wantN, wantMask)
		}
		if !stacks && e.effects[0].ends != w.tick+90 {
			t.Errorf("a restarted effect ends at %d, want %d", e.effects[0].ends, w.tick+90)
		}
	}
}

// Instant changes are permanent and clamp to the drive's range; a timed Add
// (water filling the bladder) stops when its stage does.
func TestEffectInstantAndAdd(t *testing.T) {
	w, e := driveWorld(t, func(c *Config) {
		var meal, water DriveEffectStage
		meal.Ticks = 1
		meal.Drive[DriveFood] = DriveEffectTerms{Instant: -400}
		water.Ticks = 50
		water.Drive[DriveBladder] = DriveEffectTerms{Add: 4000}
		c.DriveEffects = []DriveEffectProfile{
			{Name: "big meal", Stages: []DriveEffectStage{meal}},
			{Name: "water", Stages: []DriveEffectStage{water}},
		}
	})
	w.setDrive(e, DriveFood, 300)
	w.applyEffect(e, 0)
	if got := w.driveLevel(e, DriveFood); got != 0 {
		t.Fatalf("food %d after a 400-point meal at 300, want clamped to 0", got)
	}
	before := e.drives[DriveBladder].rate
	w.applyEffect(e, 1)
	if e.drives[DriveBladder].rate <= before {
		t.Fatalf("water did not speed the bladder: %d -> %d", before, e.drives[DriveBladder].rate)
	}
	w.tick += 50
	w.advanceEffects(e)
	if e.drives[DriveBladder].rate != before {
		t.Fatalf("bladder rate %d after the water stage ended, want %d", e.drives[DriveBladder].rate, before)
	}
}

// A drive can shrink: a negative rate schedules the tick it falls out of its
// band, and it bottoms out at Min.
func TestNegativeRateDecaysAndSchedulesDownwardCrossing(t *testing.T) {
	w, e := driveWorld(t, nil)
	d := DriveSocial
	w.setDrive(e, d, 600)
	e.driveBase[d] = -2000
	w.refreshDrive(e, d, 0)
	if e.drives[d].phase != DrivePressing {
		t.Fatalf("phase %v at 600, want pressing", e.drives[d].phase)
	}
	at := e.drives[d].nextCrossing
	rate := e.drives[d].rate
	if at == 0 || rate >= 0 {
		t.Fatalf("decaying drive: rate %d, crossing %d", rate, at)
	}
	w.tick = at - 1
	if lvl := w.driveLevel(e, d); lvl < w.cfg.Drives[d].SeekAt {
		t.Fatalf("level %d a tick before the crossing is already below seek-at", lvl)
	}
	w.tick = at
	if lvl := w.driveLevel(e, d); lvl >= w.cfg.Drives[d].SeekAt {
		t.Fatalf("level %d at the crossing is still at or above seek-at", lvl)
	}
	w.tick += 10_000
	w.syncDrivePhase(e, d)
	if w.driveLevel(e, d) != 0 || e.drives[d].phase != DriveSatisfied || e.drives[d].nextCrossing != 0 {
		t.Fatalf("decayed drive at %d, phase %v, crossing %d: want 0, satisfied, none",
			w.driveLevel(e, d), e.drives[d].phase, e.drives[d].nextCrossing)
	}
}

// A ramp is a handful of even steps from its first value to its last, and the
// band table it compiles into has no duplicate edges.
func TestRampExpandsIntoSteps(t *testing.T) {
	r := DriveRamp{From: 700, To: 950, FromValue: 0, ToValue: -60, Steps: 5, Kind: ConsequenceRate, Target: DriveFood}
	got := r.expand(1000)
	want := []DriveConsequence{
		{From: 700, To: 749, Value: 0}, {From: 750, To: 799, Value: -12}, {From: 800, To: 849, Value: -24},
		{From: 850, To: 899, Value: -36}, {From: 900, To: 949, Value: -48}, {From: 950, To: 1000, Value: -60},
	}
	if len(got) != len(want) {
		t.Fatalf("%d steps, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].From != want[i].From || got[i].To != want[i].To || got[i].Value != want[i].Value {
			t.Errorf("step %d = %d..%d %d, want %d..%d %d", i, got[i].From, got[i].To, got[i].Value,
				want[i].From, want[i].To, want[i].Value)
		}
	}
	spec := DefaultConfig().Drives[DriveSleep]
	spec.Ramps = []DriveRamp{{From: spec.SeekAt, To: spec.Max, FromValue: 0, ToValue: 50, Steps: 4,
		Kind: ConsequenceRate, Target: DriveFood}}
	table, err := compileDrive(spec, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.IsSorted(table.edges) || len(slices.Compact(slices.Clone(table.edges))) != len(table.edges) {
		t.Fatalf("edges %v are not strictly increasing", table.edges)
	}
}

// Drives affect one another through rate consequences: while hunger presses,
// sleep builds faster, and it stops the moment hunger is met.
func TestCoupledDriveRateFollowsSourceBand(t *testing.T) {
	w, e := driveWorld(t, func(c *Config) {
		c.Drives[DriveFood].Consequences = []DriveConsequence{
			{From: 650, To: 1000, Kind: ConsequenceRate, Target: DriveSleep, Value: +30},
		}
	})
	base := e.drives[DriveSleep].rate
	w.setDrive(e, DriveFood, 640)
	at := e.drives[DriveFood].nextCrossing
	if at == 0 {
		t.Fatal("hunger schedules no crossing into its pressing band")
	}
	w.tick = at
	w.syncDrivePhase(e, DriveFood)
	if got := e.drives[DriveSleep].rate; got != base*130/100 {
		t.Fatalf("sleep rate %d with hunger pressing, want %d", got, base*130/100)
	}
	w.resetDrive(e, DriveFood)
	if got := e.drives[DriveSleep].rate; got != base {
		t.Fatalf("sleep rate %d once fed, want %d again", got, base)
	}
}

// Food's death consequence is an HP drain at the ceiling, worth StarveDamage.
func TestFatalCompilesToHPDrainAtMax(t *testing.T) {
	cfg := DefaultConfig()
	table := &newWorld(cfg, newPCG(1)).driveTables[DriveFood]
	if got := table.hpDrain[table.bandOf(cfg.Drives[DriveFood].Max)]; got != cfg.StarveDamage {
		t.Errorf("drain at max = %d, want %d", got, cfg.StarveDamage)
	}
	if got := table.hpDrain[table.bandOf(cfg.Drives[DriveFood].Max-1)]; got != 0 {
		t.Errorf("drain just below max = %d, want 0", got)
	}
}

func TestCheckDrivesRejectsMalformedTables(t *testing.T) {
	cases := map[string]func(*Config){
		"consequence ends before it starts": func(c *Config) {
			c.Drives[DriveFood].Consequences = []DriveConsequence{{From: 500, To: 400, Kind: ConsequenceDeath, Value: 1}}
		},
		"ramp with no steps": func(c *Config) {
			c.Drives[DriveFood].Ramps = []DriveRamp{{From: 500, To: 900, Kind: ConsequenceDeath}}
		},
		"effect stage with no length": func(c *Config) {
			c.DriveEffects = []DriveEffectProfile{{Name: "x", Stages: []DriveEffectStage{{}}}}
		},
		"rate change below -100%": func(c *Config) {
			c.Drives[DriveFood].Consequences = []DriveConsequence{{From: 1, To: 2, Kind: ConsequenceRate, Value: -150}}
		},
	}
	for name, edit := range cases {
		cfg := DefaultConfig()
		edit(&cfg)
		if err := cfg.CheckDrives(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	cfg := DefaultConfig()
	if err := cfg.CheckDrives(); err != nil {
		t.Errorf("the shipped drives do not compile: %v", err)
	}
}

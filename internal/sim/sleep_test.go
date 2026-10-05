package sim

import "testing"

// Sleep traits move a night by one clock hour either way, and the sleep drive
// falls in bed at the speed that fits the colonist's night.
func TestSleepTraitsMoveTheNightAnHour(t *testing.T) {
	w := propertyWorld(t)
	night, hour := w.cfg.NightTicks(), w.cfg.TicksPerHour()
	if want := w.cfg.TicksPerDay() / 3; night != want {
		t.Errorf("default night = %d ticks, want a third of the %d-tick day (eight hours)", night, w.cfg.TicksPerDay())
	}
	seek := w.cfg.Drives[DriveSleep].SeekAt
	for _, c := range []struct {
		traits []Trait
		want   int
	}{
		{nil, night},
		{[]Trait{TraitShortSleeper}, night - hour},
		{[]Trait{TraitLongSleeper}, night + hour},
	} {
		e := w.spawn(Colonist, Point{6, 6})
		e.Profile.Traits = c.traits
		w.resolveTraitEffects(e)
		if e.sleepTicks != c.want {
			t.Errorf("%v: sleepTicks = %d, want %d", c.traits, e.sleepTicks, c.want)
		}
		e.State = Sleeping
		w.syncDriveActivity(e)
		fall := -e.drives[DriveSleep].rate
		if fall <= 0 {
			t.Fatalf("%v: sleep rate %d in bed, want it falling", c.traits, e.drives[DriveSleep].rate)
		}
		// Clearing SeekAt takes the night, give or take the rounding of a
		// per-tick rate.
		if got := (seek*driveUnit + fall - 1) / fall; got < c.want || got > c.want+2 {
			t.Errorf("%v: a night from seek-at takes %d ticks, want about %d", c.traits, got, c.want)
		}
	}
}

// sleepyColonistBesideBed is a colonist ready for bed, standing next to one,
// with its food drive part way up.
func sleepyColonistBesideBed(t *testing.T) (*World, *Entity) {
	t.Helper()
	w := propertyWorld(t)
	w.SetTerrain(Point{10, 10}, Bed)
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{9, 10})
	w.setDrive(e, DriveSleep, w.cfg.Drives[DriveSleep].SeekAt+50)
	w.setDrive(e, DriveFood, 200)
	for i := 0; i < 50 && e.State != Sleeping; i++ {
		w.step()
	}
	if e.State != Sleeping {
		t.Fatalf("colonist never went to bed (state %v, focus %v)", e.State, e.focus)
	}
	return w, e
}

// In bed, the sleep drive falls and the night lasts until it reaches 0;
// food, bladder and social stop rising (under testConfig), so a night cannot
// be cut short by drives that come due every few hours, and start again once
// the colonist is up.
func TestNightLastsUntilSleepIsMet(t *testing.T) {
	w, e := sleepyColonistBesideBed(t)
	food := w.driveLevel(e, DriveFood)
	sleep := w.driveLevel(e, DriveSleep)
	for i := 0; i < 100; i++ {
		w.step()
	}
	if e.State != Sleeping {
		t.Fatalf("woke mid-night (state %v, sleep %d)", e.State, w.driveLevel(e, DriveSleep))
	}
	if got := w.driveLevel(e, DriveFood); got != food {
		t.Fatalf("food rose while asleep: %d -> %d", food, got)
	}
	if got := w.driveLevel(e, DriveSleep); got >= sleep {
		t.Fatalf("sleep did not fall in bed: %d -> %d", sleep, got)
	}
	// It stays in bed below seek-at: the night is not over until 0.
	for i := 0; i < 2*e.sleepTicks && w.driveLevel(e, DriveSleep) > 0; i++ {
		w.step()
		if lvl := w.driveLevel(e, DriveSleep); lvl > 0 && e.State != Sleeping {
			t.Fatalf("got up with sleep at %d (state %v, focus %v)", lvl, e.State, e.focus)
		}
	}
	for i := 0; i < 3 && e.State == Sleeping; i++ {
		w.step()
	}
	if e.State == Sleeping || w.driveLevel(e, DriveSleep) > 1 {
		t.Fatalf("night did not finish (state %v, sleep %d)", e.State, w.driveLevel(e, DriveSleep))
	}
	if got := memoriesOf(e, "slept"); got != 1 {
		t.Fatalf("slept memories = %d, want 1", got)
	}
	awake := w.driveLevel(e, DriveFood)
	for i := 0; i < 20; i++ {
		w.step()
	}
	if got := w.driveLevel(e, DriveFood); got <= awake {
		t.Fatalf("food did not rise again after waking: %d -> %d", awake, got)
	}
}

// A night that is interrupted keeps what was slept: the drive has fallen that
// far, and the colonist gets up that much less tired. Still above seek-at, it
// lies straight back down and the night carries on from where it was.
func TestInterruptedNightKeepsWhatWasSlept(t *testing.T) {
	w, e := sleepyColonistBesideBed(t)
	start := w.driveLevel(e, DriveSleep)
	for i := 0; i < 10; i++ {
		w.step()
	}
	slept := w.driveLevel(e, DriveSleep)
	if slept >= start-10 || slept < w.cfg.Drives[DriveSleep].SeekAt {
		t.Fatalf("sleep %d -> %d after 10 ticks in bed: want it fallen, still above seek-at", start, slept)
	}
	w.clearJob(e)
	e.State = Idle
	w.step()
	if got := w.driveLevel(e, DriveSleep); got > slept+2 {
		t.Fatalf("the interruption lost what was slept: %d -> %d", slept, got)
	}
	for i := 0; i < 5 && e.State != Sleeping; i++ {
		w.step()
	}
	if e.State != Sleeping {
		t.Fatalf("did not lie back down with sleep at %d", w.driveLevel(e, DriveSleep))
	}
	for i := 0; i < 2*e.sleepTicks && w.driveLevel(e, DriveSleep) > 0; i++ {
		w.step()
	}
	if w.driveLevel(e, DriveSleep) != 0 {
		t.Fatalf("night not finished: sleep %d", w.driveLevel(e, DriveSleep))
	}
}

// Got up below seek-at, a colonist is rested enough to stay up: its sleep
// rises again from where the night left it, not from 0.
func TestEarlyRiserCarriesTheRestOfTheNight(t *testing.T) {
	w, e := sleepyColonistBesideBed(t)
	for i := 0; i < 150; i++ {
		w.step()
	}
	left := w.driveLevel(e, DriveSleep)
	if left == 0 || left >= w.cfg.Drives[DriveSleep].SeekAt {
		t.Fatalf("sleep %d after 150 ticks in bed: want part of a night", left)
	}
	w.clearJob(e)
	e.State = Idle
	for i := 0; i < 20; i++ {
		w.step()
	}
	if e.State == Sleeping {
		t.Fatal("went back to bed below seek-at")
	}
	if got := w.driveLevel(e, DriveSleep); got <= left {
		t.Fatalf("sleep %d -> %d up and about: want it rising from where it was", left, got)
	}
}

// A colonist driven out of bed by a threat is awake again: its other drives
// resume rising, and its sleep stops falling.
func TestFleeingWakesTheSleeper(t *testing.T) {
	w, e := sleepyColonistBesideBed(t)
	for i := 0; i < 30; i++ {
		w.step()
	}
	w.spawn(Alien, e.Pos.Add(0, 1))
	for i := 0; i < 3; i++ {
		w.step()
	}
	if e.State == Sleeping {
		t.Fatalf("still asleep with an alien beside the bed (state %v, focus %v)", e.State, e.focus)
	}
	if e.drives[DriveFood].rate == 0 {
		t.Fatal("food still paused after waking")
	}
	if e.drives[DriveSleep].rate <= 0 {
		t.Fatalf("sleep rate %d after waking, want it rising again", e.drives[DriveSleep].rate)
	}
}

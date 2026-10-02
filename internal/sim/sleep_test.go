package sim

import "testing"

// Sleep traits move a night by one clock hour either way.
func TestSleepTraitsMoveTheNightAnHour(t *testing.T) {
	w := propertyWorld(t)
	night, hour := w.cfg.Needs[NeedSleep].UseTicks, w.cfg.TicksPerHour()
	if want := w.cfg.TicksPerDay() / 3; night != want {
		t.Errorf("default night = %d ticks, want a third of the %d-tick day (eight hours)", night, w.cfg.TicksPerDay())
	}
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
	}
}

// sleepyColonistBesideBed is a colonist ready for bed, standing next to one,
// with its food need part way up.
func sleepyColonistBesideBed(t *testing.T) (*World, *Entity) {
	t.Helper()
	w := propertyWorld(t)
	w.SetTerrain(Point{10, 10}, Bed)
	w.refreshSpatial()
	e := w.spawn(Colonist, Point{9, 10})
	e.Needs[NeedSleep] = w.cfg.Needs[NeedSleep].SeekAt + 50
	e.Needs[NeedFood] = 200
	w.syncNeedPhase(e, NeedSleep)
	w.syncNeedPhase(e, NeedFood)
	for i := 0; i < 50 && e.State != Sleeping; i++ {
		w.step()
	}
	if e.State != Sleeping {
		t.Fatalf("colonist never went to bed (state %v, focus %v)", e.State, e.focus)
	}
	return w, e
}

// Food, bladder and social stop rising while a colonist is in bed, so a
// night cannot be cut short by needs that come due every few hours, and
// start again once it is up.
func TestOtherNeedsPauseWhileAsleep(t *testing.T) {
	w, e := sleepyColonistBesideBed(t)
	food := w.needLevel(e, NeedFood)
	for i := 0; i < e.sleepTicks/2; i++ {
		w.step()
	}
	if e.State != Sleeping {
		t.Fatalf("woke mid-night (state %v)", e.State)
	}
	if got := w.needLevel(e, NeedFood); got != food {
		t.Fatalf("food rose while asleep: %d -> %d", food, got)
	}
	for i := 0; i < e.sleepTicks && e.State == Sleeping; i++ {
		w.step()
	}
	if e.State == Sleeping || w.needLevel(e, NeedSleep) > 5 {
		t.Fatalf("night did not finish (state %v, sleep %d)", e.State, w.needLevel(e, NeedSleep))
	}
	awake := w.needLevel(e, NeedFood)
	for i := 0; i < 20; i++ {
		w.step()
	}
	if got := w.needLevel(e, NeedFood); got <= awake {
		t.Fatalf("food did not rise again after waking: %d -> %d", awake, got)
	}
}

// A night that is interrupted keeps what was slept; going back to bed
// finishes it rather than starting over.
func TestInterruptedNightIsBanked(t *testing.T) {
	w, e := sleepyColonistBesideBed(t)
	for i := 0; i < 100; i++ {
		w.step()
	}
	banked := e.sleepBanked
	if banked < 90 {
		t.Fatalf("banked %d ticks after 100 in bed", banked)
	}
	// Pulled out of bed. Still sleepy and beside the bed, it lies straight
	// back down, and the night carries on from what it had banked.
	w.clearJob(e)
	e.State = Idle
	w.wakeUp(e)
	w.step()
	if e.sleepBanked < banked {
		t.Fatalf("interruption lost the banked sleep: %d -> %d", banked, e.sleepBanked)
	}
	for i := 0; i < e.sleepTicks && e.sleepBanked != 0; i++ {
		w.step()
	}
	if e.sleepBanked != 0 || w.needLevel(e, NeedSleep) > e.sleepTicks-banked+50 {
		t.Fatalf("night not finished from the bank: banked %d, sleep %d", e.sleepBanked, w.needLevel(e, NeedSleep))
	}
}

// A colonist driven out of bed by a threat is awake again: its other needs
// resume rising, and what it slept stays banked.
func TestFleeingWakesTheSleeper(t *testing.T) {
	w, e := sleepyColonistBesideBed(t)
	for i := 0; i < 30; i++ {
		w.step()
	}
	banked := e.sleepBanked
	w.spawn(Alien, e.Pos.Add(0, 1))
	for i := 0; i < 3; i++ {
		w.step()
	}
	if e.State == Sleeping || e.asleep {
		t.Fatalf("still asleep with an alien beside the bed (state %v, focus %v)", e.State, e.focus)
	}
	if e.needRise[NeedFood] == 0 {
		t.Fatal("food still paused after waking")
	}
	if e.sleepBanked != banked {
		t.Fatalf("banked sleep changed while awake: %d -> %d", banked, e.sleepBanked)
	}
}

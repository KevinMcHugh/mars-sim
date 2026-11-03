package sim

import "testing"

// A messy kill bloodies the colonists standing beside it and nobody further
// off; a wound and a bleed bloody the colonist who has them; blood is capped.
func TestBloodSplattersBystanders(t *testing.T) {
	w := newTestWorld(t, testConfig())
	at := Point{w.Width / 2, w.Height / 2, LandingLevel}
	near := w.spawn(Colonist, Point{at.X + 1, at.Y, at.Level})
	far := w.spawn(Colonist, Point{at.X + 3, at.Y, at.Level})

	w.addGore(at)
	if near.blood != 2 || far.blood != 0 {
		t.Fatalf("after a kill: beside %d, three tiles off %d", near.blood, far.blood)
	}
	for range 5 {
		w.takeGore(at)
		w.addGore(at)
	}
	if near.blood != maxBlood {
		t.Fatalf("blood %d, want the cap %d", near.blood, maxBlood)
	}

	far.wound(Torso)
	if far.blood != 1 {
		t.Fatalf("a wounded colonist's blood is %d", far.blood)
	}
	if v := w.entityView(far, nil, false); v.Blood != 1 {
		t.Fatalf("snapshot blood %d", v.Blood)
	}
}

// Blood fades a step every blood-fade-hours, and fresh blood restarts the clock.
func TestBloodFades(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)
	e := w.spawn(Colonist, Point{2, 2, LandingLevel})
	e.bloody(3)
	step := cfg.BloodFadeHours * w.cfg.TicksPerHour()
	for range step - 1 {
		w.fadeBlood(e)
	}
	if e.blood != 3 {
		t.Fatalf("faded early: %d", e.blood)
	}
	w.fadeBlood(e)
	if e.blood != 2 {
		t.Fatalf("after one step's time: %d", e.blood)
	}
	for range step - 1 {
		w.fadeBlood(e)
	}
	e.bloody(1) // fresh blood: the clock starts again
	for range step - 1 {
		w.fadeBlood(e)
	}
	if e.blood != 3 {
		t.Fatalf("fresh blood did not restart the fade: %d", e.blood)
	}
}

func TestBloodTiers(t *testing.T) {
	for blood, want := range map[int]int{0: 0, 1: 1, 2: 1, 3: 2, 4: 2, 5: 3, 6: 3} {
		if got := BloodTier(blood); got != want {
			t.Errorf("BloodTier(%d) = %d, want %d", blood, got, want)
		}
	}
}

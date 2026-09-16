package sim

import "testing"

// Sighting an alien should drop mood, and more than sighting a mouse does —
// aliens are the scarier thing to run into.
func TestSeeingAlienDropsMoodMoreThanMouse(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	sawAlien := w.spawn(Colonist, Point{0, 0})
	w.spawn(Alien, Point{1, 0})
	w.observeNearby(sawAlien)

	sawMouse := w.spawn(Colonist, Point{10, 10})
	w.spawn(Mouse, Point{11, 10})
	w.observeNearby(sawMouse)

	if sawAlien.mood >= 0 {
		t.Fatalf("mood after seeing an alien = %d, want negative", sawAlien.mood)
	}
	if sawMouse.mood >= 0 {
		t.Fatalf("mood after seeing a mouse = %d, want negative", sawMouse.mood)
	}
	if sawAlien.mood >= sawMouse.mood {
		t.Errorf("alien sighting mood %d should drop more than mouse sighting mood %d", sawAlien.mood, sawMouse.mood)
	}
}

// Sighting the same alien again on a later tick, still in view, should not
// pile on another mood hit — it's the same encounter, edge-triggered on
// e.seen exactly like the memory itself.
func TestRepeatedAlienSightingDoesNotRepeatMoodHit(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{0, 0})
	w.spawn(Alien, Point{1, 0})
	w.observeNearby(colonist)
	afterFirst := colonist.mood

	w.observeNearby(colonist)
	if colonist.mood != afterFirst {
		t.Errorf("mood changed on repeated sighting: %d -> %d", afterFirst, colonist.mood)
	}
}

// Killing an alien should raise a colonist's mood, and a bystander who
// merely watches it happen should also feel better, but less than the
// colonist who did the killing.
func TestKillingAlienRaisesMoodMoreThanWitnessing(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	killer := w.spawn(Colonist, Point{0, 0})
	witness := w.spawn(Colonist, Point{1, 1})
	alien := w.spawn(Alien, Point{1, 0})
	alien.HP = 1
	alien.Parts = [numBodyParts]int{1, 1, 1, 1, 1, 1}

	w.shoot(killer, alien, Shotgun, weaponStats(Shotgun, cfg))

	if killer.mood <= 0 {
		t.Fatalf("killer mood = %d, want positive", killer.mood)
	}
	if witness.mood <= 0 {
		t.Fatalf("witness mood = %d, want positive", witness.mood)
	}
	if witness.mood >= killer.mood {
		t.Errorf("witness mood %d should be less than killer mood %d", witness.mood, killer.mood)
	}
}

// Coming within sight of gore should drop mood.
func TestSeeingGoreDropsMood(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{5, 5})
	w.addGore(colonist.Pos)

	w.observeGore(colonist)
	if colonist.mood >= 0 {
		t.Fatalf("mood after seeing gore = %d, want negative", colonist.mood)
	}
}

// A Tidy colonist should take a bigger mood hit from the same gore than a
// colonist without the trait.
func TestTidyTraitAmplifiesGoreMoodDrop(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	tidy := w.spawn(Colonist, Point{5, 5})
	tidy.Profile = &Profile{Traits: []Trait{TraitTidy}}
	plain := w.spawn(Colonist, Point{20, 20})
	plain.Profile = &Profile{}

	w.addGore(tidy.Pos)
	w.addGore(plain.Pos)
	w.observeGore(tidy)
	w.observeGore(plain)

	if tidy.mood >= plain.mood {
		t.Errorf("Tidy colonist mood %d should drop more than plain colonist mood %d", tidy.mood, plain.mood)
	}
}

// Gore sightings are edge-triggered like entity sightings: staying in view of
// the same mess across ticks should record one memory and one mood hit, not
// one per tick.
func TestGoreSightIsEdgeTriggered(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{5, 5})
	w.addGore(colonist.Pos)

	w.observeGore(colonist)
	afterFirst := colonist.mood
	w.observeGore(colonist)

	if colonist.mood != afterFirst {
		t.Errorf("mood changed on repeated gore sighting while still in view: %d -> %d", afterFirst, colonist.mood)
	}
	if len(colonist.Memories) != 1 {
		t.Fatalf("expected exactly one gore memory, got %d", len(colonist.Memories))
	}

	// Moving away and the gore falling out of sight, then coming back, should
	// let it fire again — a fresh encounter with the scene.
	w.observeGore(colonist) // still in view; no-op, sanity check above already covers it
	colonist.seeingGore = false
	w.observeGore(colonist)
	if len(colonist.Memories) != 2 {
		t.Fatalf("expected a second gore memory after the sighting reset, got %d", len(colonist.Memories))
	}
}

// remember should tag the stored Memory with the LifeEventKind that produced
// it, not just the rendered text.
func TestMemoryRecordsLifeEventKind(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{0, 0})
	w.remember(colonist, event(EvtAte, "Had a meal."))

	last := colonist.Memories[len(colonist.Memories)-1]
	if last.Kind != EvtAte {
		t.Errorf("memory kind = %v, want EvtAte", last.Kind)
	}
	if last.Text != "Had a meal." {
		t.Errorf("memory text = %q, want %q", last.Text, "Had a meal.")
	}
}

// Being bitten and surviving should drop mood.
func TestBittenDropsMood(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	alien := w.spawn(Alien, Point{0, 0})
	victim := w.spawn(Colonist, Point{1, 0})
	// Give every part (and the aggregate pool) plenty of HP so the bite is
	// non-fatal regardless of which part rollHit lands on.
	victim.HP = 100
	victim.Parts = [numBodyParts]int{100, 100, 100, 100, 100, 100}

	w.bite(alien, victim)

	if victim.mood >= 0 {
		t.Fatalf("mood after being bitten = %d, want negative", victim.mood)
	}
}

// Finishing a work job should raise mood a little, and more for an
// Industrious colonist than a plain one.
func TestFinishingJobRaisesMoodMoreForIndustrious(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	plain := w.spawn(Colonist, Point{0, 0})
	plain.Profile = &Profile{}
	industrious := w.spawn(Colonist, Point{5, 5})
	industrious.Profile = &Profile{Traits: []Trait{TraitIndustrious}}

	w.remember(plain, event(EvtFinishedMining, "Finished mining at (%d, %d).", 1, 1))
	w.remember(industrious, event(EvtFinishedMining, "Finished mining at (%d, %d).", 1, 1))

	if plain.mood <= 0 {
		t.Fatalf("mood after finishing a job = %d, want positive", plain.mood)
	}
	if industrious.mood <= plain.mood {
		t.Errorf("industrious colonist mood %d should rise more than plain colonist mood %d", industrious.mood, plain.mood)
	}
}

// Every "finished a work job" kind should share the same mood bump, not just
// mining — a regression guard for jobFinishedMood's table wiring.
func TestAllJobCompletionKindsRaiseMood(t *testing.T) {
	cfg := testConfig()
	for _, kind := range []LifeEventKind{EvtFinishedMining, EvtClearedRock, EvtFinishedConstruction} {
		w := newTestWorld(t, cfg)
		c := w.spawn(Colonist, Point{0, 0})
		c.Profile = &Profile{}
		w.remember(c, event(kind, "did a job"))
		if c.mood <= 0 {
			t.Errorf("kind %v: mood = %d, want positive", kind, c.mood)
		}
	}
}

// A finished conversation should now go entirely through remember/LifeEvent:
// one call records the memory and moves mood together, rather than two
// separate mechanisms that could drift apart.
func TestConversationRecordsMemoryAndMood(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	a := w.spawn(Colonist, Point{0, 0})
	b := w.spawn(Colonist, Point{1, 0})
	// Max out their existing affinity so the roll's quality (which leans
	// toward existing affinity's valence) is guaranteed positive, and with it
	// the mood delta — this test is about the mechanism (one call records
	// both the memory and the mood), not the mood formula itself (see
	// TestTalkMoodRules for that).
	w.addAffinity(a.ID, b.ID, cfg.AffinityMax)

	w.finishTalk(a, b)

	if len(a.Memories) != 1 || a.Memories[0].Kind != EvtConversation {
		t.Fatalf("expected one conversation memory on a, got %+v", a.Memories)
	}
	if len(b.Memories) != 1 || b.Memories[0].Kind != EvtConversation {
		t.Fatalf("expected one conversation memory on b, got %+v", b.Memories)
	}
	if a.mood <= 0 || b.mood <= 0 {
		t.Fatalf("expected finishTalk to raise both participants' mood, got a=%d b=%d", a.mood, b.mood)
	}
}

package sim

import "testing"

// Sighting an alien should activate a colonist and cost more grip than a mouse.
func TestSeeingAlienAffectsChargeAndGripMoreThanMouse(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	sawAlien := w.spawn(Colonist, Point{0, 0})
	w.spawn(Alien, Point{1, 0})
	w.observeNearby(sawAlien)

	sawMouse := w.spawn(Colonist, Point{10, 10})
	w.spawn(Mouse, Point{11, 10})
	w.observeNearby(sawMouse)

	if sawAlien.affect.Charge <= sawMouse.affect.Charge {
		t.Errorf("alien charge %d should exceed mouse charge %d", sawAlien.affect.Charge, sawMouse.affect.Charge)
	}
	if sawAlien.affect.Grip >= sawMouse.affect.Grip {
		t.Errorf("alien grip %d should drop more than mouse grip %d", sawAlien.affect.Grip, sawMouse.affect.Grip)
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
	afterFirst := colonist.affect

	w.observeNearby(colonist)
	if colonist.affect != afterFirst {
		t.Errorf("affect changed on repeated sighting: %+v -> %+v", afterFirst, colonist.affect)
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

	if killer.affect.Charge <= witness.affect.Charge || killer.affect.Grip <= witness.affect.Grip {
		t.Fatalf("killer affect %+v should exceed witness affect %+v on both axes", killer.affect, witness.affect)
	}
	if witness.affect.Charge <= 0 || witness.affect.Grip <= 0 {
		t.Fatalf("witness affect = %+v, want both axes positive", witness.affect)
	}
}

// Coming within sight of gore should lower both affect axes.
func TestSeeingGoreDropsMood(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	colonist := w.spawn(Colonist, Point{5, 5})
	w.addGore(colonist.Pos)

	w.observeGore(colonist)
	if colonist.affect.Charge >= 0 || colonist.affect.Grip >= 0 {
		t.Fatalf("affect after seeing gore = %+v, want both axes negative", colonist.affect)
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

	if tidy.affect.Charge >= plain.affect.Charge || tidy.affect.Grip >= plain.affect.Grip {
		t.Errorf("Tidy affect %+v should drop more than plain affect %+v", tidy.affect, plain.affect)
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
	afterFirst := colonist.affect
	w.observeGore(colonist)

	if colonist.affect != afterFirst {
		t.Errorf("affect changed on repeated gore sighting while still in view: %+v -> %+v", afterFirst, colonist.affect)
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

// Being bitten and surviving should raise charge and lower grip.
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

	if victim.affect.Charge <= 0 || victim.affect.Grip >= 0 {
		t.Fatalf("affect after being bitten = %+v, want positive charge and negative grip", victim.affect)
	}
}

// Finishing work should restore grip, doubled for an Industrious colonist.
func TestFinishingJobRaisesMoodMoreForIndustrious(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	plain := w.spawn(Colonist, Point{0, 0})
	plain.Profile = &Profile{}
	industrious := w.spawn(Colonist, Point{5, 5})
	industrious.Profile = &Profile{Traits: []Trait{TraitIndustrious}}

	w.remember(plain, event(EvtFinishedMining, "Finished mining at (%d, %d).", 1, 1))
	w.remember(industrious, event(EvtFinishedMining, "Finished mining at (%d, %d).", 1, 1))

	if plain.affect.Grip <= 0 {
		t.Fatalf("affect after finishing a job = %+v, want positive grip", plain.affect)
	}
	if industrious.affect.Grip != 2*plain.affect.Grip || industrious.affect.Charge != 2*plain.affect.Charge {
		t.Errorf("industrious affect %+v should double plain affect %+v", industrious.affect, plain.affect)
	}
}

// Every finished-work kind should restore grip, not just mining.
func TestAllJobCompletionKindsRaiseMood(t *testing.T) {
	cfg := testConfig()
	for _, kind := range []LifeEventKind{EvtFinishedMining, EvtClearedRock, EvtFinishedConstruction, EvtCleanedRefuse, EvtIncineratedRefuse} {
		w := newTestWorld(t, cfg)
		c := w.spawn(Colonist, Point{0, 0})
		c.Profile = &Profile{}
		w.remember(c, event(kind, "did a job"))
		if c.affect.Grip <= 0 {
			t.Errorf("kind %v: affect = %+v, want positive grip", kind, c.affect)
		}
	}
}

// A finished conversation goes entirely through remember/LifeEvent: one call
// records memory and moves affect together.
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
	if a.affect.Grip <= 0 || b.affect.Grip <= 0 {
		t.Fatalf("expected finishTalk to restore both participants' grip, got a=%+v b=%+v", a.affect, b.affect)
	}
}

package sim

import "testing"

func TestStimulusCoalescesByRuleAndSource(t *testing.T) {
	w, c := focusTestColonist(t)
	w.tick = 10
	rememberTestFrom(w, c, "saw-alien", 7, "Saw alien #7.")
	w.tick = 11
	rememberTestFrom(w, c, "saw-alien", 7, "Saw alien #7 again.")
	if c.stimulusCount != 1 {
		t.Fatalf("stimulus count = %d, want 1", c.stimulusCount)
	}
	if got, want := c.stimuli[0].ExpiresAt, 23; got != want {
		t.Fatalf("expiry = %d, want refreshed %d", got, want)
	}

	rememberTestFrom(w, c, "saw-alien", 8, "Saw alien #8.")
	if c.stimulusCount != 2 || c.stimuli[1].Source != 8 {
		t.Fatalf("different source did not remain distinct: %+v", c.stimuli[:c.stimulusCount])
	}
}

func TestStimulusExpiresExactlyAtBoundaryWithoutErasingAffectOrMemory(t *testing.T) {
	w, c := focusTestColonist(t)
	rememberTestFrom(w, c, "saw-alien", 7, "Saw alien #7.")
	affect, memories := c.affect, len(c.Memories)
	expires := c.stimuli[0].ExpiresAt

	w.tick = expires - 1
	w.expireStimuli(c)
	if c.stimulusCount != 1 {
		t.Fatal("stimulus expired before its boundary")
	}
	w.tick = expires
	w.expireStimuli(c)
	if c.stimulusCount != 0 {
		t.Fatal("stimulus remained active at its boundary")
	}
	if c.affect != affect || len(c.Memories) != memories {
		t.Fatalf("expiry changed durable products: affect %+v/%+v memories %d/%d", c.affect, affect, len(c.Memories), memories)
	}
}

func TestStimulusEvictionOrderAndIncomingWeakEvent(t *testing.T) {
	w, c := focusTestColonist(t)
	w.cfg.ActiveStimulusLimit = 2
	c.stimuli[0] = Stimulus{Rule: "saw-alien", Source: 9, Salience: 100, ExpiresAt: 20}
	c.stimuli[1] = Stimulus{Rule: "saw-gore", Source: 4, Salience: 35, ExpiresAt: 15}
	c.stimulusCount = 2

	// A weaker incoming event is itself the eviction candidate.
	if addTestStimulus(w, c, "finished-mining", 0) {
		t.Fatal("weaker incoming stimulus displaced a live entry")
	}
	if c.stimuli[0].Rule != "saw-alien" {
		t.Fatal("low-salience event hid live alien")
	}

	if !addTestStimulus(w, c, "witnessed-colonist-attacked", 8) {
		t.Fatal("stronger stimulus was not inserted")
	}
	if c.stimuli[1].Rule != "witnessed-colonist-attacked" {
		t.Fatalf("wrong entry evicted: %+v", c.stimuli[:c.stimulusCount])
	}
}

func TestStimulusWeakOrdering(t *testing.T) {
	if !stimulusWeaker(
		Stimulus{Salience: 34, ExpiresAt: 99, Source: 99},
		Stimulus{Salience: 35, ExpiresAt: 1, Source: 1},
	) {
		t.Fatal("lower salience was not weaker")
	}
	if !stimulusWeaker(
		Stimulus{Salience: 35, ExpiresAt: 9, Source: 99},
		Stimulus{Salience: 35, ExpiresAt: 10, Source: 1},
	) {
		t.Fatal("earlier expiry was not weaker at equal salience")
	}
	if !stimulusWeaker(
		Stimulus{Salience: 35, ExpiresAt: 10, Source: 1},
		Stimulus{Salience: 35, ExpiresAt: 10, Source: 2},
	) {
		t.Fatal("lower source ID was not weaker at equal salience and expiry")
	}
}

func TestZeroSpecEventRecordsMemoryWithoutStimulus(t *testing.T) {
	w, c := focusTestColonist(t)
	rememberTest(w, c, "saw-mouse", "Saw a mouse.")
	if c.stimulusCount != 0 {
		t.Fatalf("zero-spec event created %d stimuli", c.stimulusCount)
	}
	if len(c.Memories) != 1 || c.Memories[0].Rule != "saw-mouse" {
		t.Fatal("zero-spec event did not record memory")
	}
}

func TestVisibleThreatRefreshesWithoutMemorySpamAndRemovalEndsEligibility(t *testing.T) {
	w, c := focusTestColonist(t)
	alien := w.spawn(Alien, c.Pos.Add(1, 0))
	w.observeNearby(c)
	if len(c.Memories) != 1 || c.stimulusCount != 1 {
		t.Fatalf("first sighting products = %d memories, %d stimuli", len(c.Memories), c.stimulusCount)
	}
	firstExpiry := c.stimuli[0].ExpiresAt
	w.tick++
	w.observeNearby(c)
	if len(c.Memories) != 1 {
		t.Fatalf("ongoing sighting recorded %d memories, want 1", len(c.Memories))
	}
	if c.stimuli[0].ExpiresAt <= firstExpiry {
		t.Fatal("ongoing threat did not refresh expiry")
	}

	var candidates [numFocusKinds]FocusCandidate
	w.focusCandidates(c, &candidates)
	if !candidates[FocusFlee].Eligible || candidates[FocusFlee].Score.Stimulus != 500 {
		t.Fatalf("visible threat candidate = %+v", candidates[FocusFlee])
	}
	w.remove(alien.ID, "test")
	w.focusCandidates(c, &candidates)
	if candidates[FocusFlee].Eligible || candidates[FocusFight].Eligible {
		t.Fatal("removed alien remained directly eligible")
	}
	if len(c.Memories) != 1 || c.stimulusCount != 1 {
		t.Fatal("threat removal erased lingering memory or stimulus")
	}
}

func BenchmarkStimulusUpdate(b *testing.B) {
	cfg := DefaultConfig()
	w := newWorld(cfg, nil)
	c := newEntity(1, Colonist, Point{1, 1}, cfg)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		w.tick = i
		addTestStimulus(w, c, "saw-alien", EntityID(i%32+1))
	}
}

func BenchmarkStimulusBias(b *testing.B) {
	cfg := DefaultConfig()
	w := newWorld(cfg, nil)
	c := newEntity(1, Colonist, Point{1, 1}, cfg)
	for i := 0; i < cfg.ActiveStimulusLimit; i++ {
		c.stimuli[i] = Stimulus{Rule: "saw-alien", Source: EntityID(i + 1), Salience: 100, ExpiresAt: 100}
	}
	c.stimulusCount = cfg.ActiveStimulusLimit
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = w.stimulusBias(c, FocusFlee)
	}
}

package sim

import "testing"

// moodName indexes moodAttractors by MoodKind rather than searching it, which
// is only safe while the table is declared in MoodKind order.
func TestMoodAttractorsIndexedByKind(t *testing.T) {
	for i, a := range moodAttractors {
		if a.Kind != MoodKind(i) {
			t.Fatalf("attractor %d is %v: table is no longer indexable by MoodKind", i, a.Kind)
		}
	}
	if int(MoodSettling) != len(moodAttractors) {
		t.Fatalf("MoodSettling = %d, want %d so the bounds check catches it", MoodSettling, len(moodAttractors))
	}
}

func TestReactionAppraisals(t *testing.T) {
	cfg := DefaultConfig()
	for _, reaction := range cfg.Cognition.Reactions {
		if reaction.Impact < 0 || reaction.Impact > 100 {
			t.Errorf("reaction %s impact %d outside [0, 100]", reaction.ID, reaction.Impact)
		}
		if reaction.Target != (MoodVector{}) && reaction.Impact == 0 {
			t.Errorf("reaction %s moves affect but has no impact", reaction.ID)
		}
		for name, v := range map[string]int{"charge": reaction.Target.Charge, "grip": reaction.Target.Grip, "valence": reaction.Target.Valence} {
			if v < -cfg.MoodMax || v > cfg.MoodMax {
				t.Errorf("reaction %s %s target %d outside the plane", reaction.ID, name, v)
			}
		}
	}
	// Conversation is contextual, so its configured target is neutral.
	conversation, _ := cfg.Cognition.reaction("conversation")
	if got := conversation.Target; got != (MoodVector{}) {
		t.Errorf("conversation declares a non-neutral fallback target %+v", got)
	}
}

// The events that relocate rather than nudge have to be written at the scale
// of the plane: relocating to a nudge-sized point would leave a colonist who
// just watched someone die almost exactly neutral.
func TestRelocatingEventsAreWrittenAtPlaneScale(t *testing.T) {
	cfg := DefaultConfig()
	for _, reaction := range cfg.Cognition.Reactions {
		if reaction.Impact < cfg.MoodPullImpact {
			continue
		}
		if _, claim := bestMoodAttractor(reaction.Target.Charge, reaction.Target.Grip); claim < 0 {
			t.Errorf("reaction %s relocates to %+v, which lands in unnamed space", reaction.ID, reaction.Target)
		}
	}
}

func TestAffectBlendClampsEveryAxis(t *testing.T) {
	w, c := focusTestColonist(t)
	c.affect.Charge, c.affect.Grip, c.affect.Valence = 95, -95, 95
	w.blendAffect(c, MoodVector{Charge: 20, Grip: -20, Valence: 20}, 0)
	if c.affect.Charge != w.cfg.MoodMax || c.affect.Grip != -w.cfg.MoodMax || c.affect.Valence != w.cfg.MoodMax {
		t.Fatalf("clamped affect = %+v", c.affect)
	}
}

func TestMoodPullEndpoints(t *testing.T) {
	w, _ := focusTestColonist(t)
	for _, tc := range []struct{ impact, want int }{
		{0, 0},
		{w.cfg.MoodPushImpact, 0},
		{w.cfg.MoodPushImpact + 10, 25},
		{w.cfg.MoodPullImpact, 100},
		{100, 100},
	} {
		if got := w.moodPull(tc.impact); got != tc.want {
			t.Errorf("moodPull(%d) = %d, want %d", tc.impact, got, tc.want)
		}
	}
}

// The reason the blend exists: under plain addition a good enough run-up would
// soften a killing. It cannot, because the killing relocates rather than adds.
func TestAGoodDayCannotSoftenAKilling(t *testing.T) {
	w, fed := focusTestColonist(t)
	bare := w.spawn(Colonist, fed.Pos.Add(10, 0))
	for i := 0; i < 10; i++ {
		rememberTest(w, fed, "ate", "ate")
		rememberTest(w, fed, "finished-mining", "mined")
	}
	if fed.affect == (AffectState{Label: MoodSteady}) {
		t.Fatal("a day of meals and work left affect untouched")
	}
	for _, c := range []*Entity{fed, bare} {
		rememberTest(w, c, "witnessed-colonist-killed", "saw a killing")
	}
	if fed.affect.Charge != bare.affect.Charge ||
		fed.affect.Grip != bare.affect.Grip ||
		fed.affect.Valence != bare.affect.Valence {
		t.Fatalf("the day before changed where the killing left them: %+v versus %+v", fed.affect, bare.affect)
	}
	if fed.affect.Grip >= 0 || fed.affect.Valence >= 0 {
		t.Fatalf("witnessing a killing left affect at %+v", fed.affect)
	}
}

// Routine life stays additive: below the push threshold nothing is relocated,
// so a second dig is worth exactly as much as the first.
func TestRoutineEventsStillAccumulate(t *testing.T) {
	w, once := focusTestColonist(t)
	thrice := w.spawn(Colonist, once.Pos.Add(10, 0))
	rememberTest(w, once, "finished-mining", "mined")
	for i := 0; i < 3; i++ {
		rememberTest(w, thrice, "finished-mining", "mined")
	}
	if thrice.affect.Grip != 3*once.affect.Grip || thrice.affect.Charge != 3*once.affect.Charge {
		t.Fatalf("three digs = %+v, want three times one dig %+v", thrice.affect, once.affect)
	}
}

func TestConversationOutcomeVectors(t *testing.T) {
	for _, tc := range []struct {
		outcome int
		want    MoodVector
	}{
		{7, MoodVector{3, 7, 2}},
		{0, MoodVector{}},
		{-6, MoodVector{4, -6, -2}},
		{1, MoodVector{1, 1, 0}},
		{-1, MoodVector{1, -1, 0}},
	} {
		if got := conversationMoodVector(tc.outcome); got != tc.want {
			t.Errorf("outcome %d = %+v, want %+v", tc.outcome, got, tc.want)
		}
	}
}

func TestTraitMoodTransforms(t *testing.T) {
	cases := []struct {
		name  string
		trait Trait
		rule  RuleID
		in    MoodVector
		want  MoodVector
	}{
		{"tidy gore", TraitTidy, "saw-gore", MoodVector{-3, -7, -6}, MoodVector{-6, -15, -13}},
		{"tidy incineration", TraitTidy, "incinerated-refuse", MoodVector{-1, 7, 1}, MoodVector{-1, 14, 1}},
		{"industrious work", TraitIndustrious, "finished-construction", MoodVector{-1, 6, 3}, MoodVector{-2, 12, 6}},
		{"mutant lover", TraitMutantLover, "mutated", MoodVector{18, -70, -35}, MoodVector{18, 70, 35}},
		{"introvert", TraitIntrovert, "conversation", MoodVector{3, 7, 3}, MoodVector{-3, 7, 3}},
	}
	cfg := DefaultCognitionConfig()
	w := &World{cognition: cfg}
	for _, tc := range cases {
		e := &Entity{Profile: &Profile{Traits: []Trait{tc.trait}}}
		reaction, _ := w.cognition.reaction(tc.rule)
		got, _ := w.transformAppraisal(e, reaction, nil, tc.in, reaction.Impact)
		if got != tc.want {
			t.Errorf("%s = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestTraitAppraisalThroughPerceptFunnel(t *testing.T) {
	w, plain := focusTestColonist(t)
	tidy := w.spawn(Colonist, plain.Pos.Add(10, 0))
	tidy.Profile = &Profile{Traits: []Trait{TraitTidy}}
	introvert := w.spawn(Colonist, plain.Pos.Add(20, 0))
	introvert.Profile = &Profile{Traits: []Trait{TraitIntrovert}}

	rememberTest(w, plain, "incinerated-refuse", "burned refuse")
	rememberTest(w, tidy, "incinerated-refuse", "burned refuse")
	if tidy.affect.Grip != 2*plain.affect.Grip {
		t.Fatalf("Tidy incineration grip %d, want twice plain %d", tidy.affect.Grip, plain.affect.Grip)
	}
	contextual := conversationMoodVector(7)
	rememberTestReaction(w, introvert, "conversation", 0, "talked", &contextual)
	if introvert.affect.Charge >= 0 || introvert.affect.Grip <= 0 {
		t.Fatalf("Introvert conversation affect = %+v, want fatigue and restored grip", introvert.affect)
	}
	if introvert.affect.Valence <= 0 {
		t.Fatalf("Introvert conversation valence = %d, want the chat to still have done them good", introvert.affect.Valence)
	}
}

func TestTraitTransformsUseDeclarationOrder(t *testing.T) {
	cfg := DefaultCognitionConfig()
	w := &World{cognition: cfg}
	reaction, _ := w.cognition.reaction("incinerated-refuse")
	in := reaction.Target
	a := &Entity{Profile: &Profile{Traits: []Trait{TraitTidy, TraitIndustrious}}}
	b := &Entity{Profile: &Profile{Traits: []Trait{TraitIndustrious, TraitTidy}}}
	gotA, _ := w.transformAppraisal(a, reaction, nil, in, reaction.Impact)
	gotB, _ := w.transformAppraisal(b, reaction, nil, in, reaction.Impact)
	if gotA != gotB || gotA != (MoodVector{-2, 28, 2}) {
		t.Fatalf("stored trait order changed transform: %+v versus %+v", gotA, gotB)
	}
}

func TestAffectDecayDefaultsAndNoOvershoot(t *testing.T) {
	w, c := focusTestColonist(t)
	c.affect.Charge, c.affect.Grip = 10, -10
	w.decayAffect(c)
	if c.affect.Charge != 8 || c.affect.Grip != -9 {
		t.Fatalf("default decay = %+v, want charge 8 grip -9", c.affect)
	}
	c.affect.Charge, c.affect.Grip = 1, -1
	w.decayAffect(c)
	if c.affect.Charge != 0 || c.affect.Grip != 0 {
		t.Fatalf("decay overshot home: %+v", c.affect)
	}
}

// Valence answers to hours, not minutes: it gives up a point every few turns
// where grip gives up one every turn.
func TestValenceDecaysSlowerThanGrip(t *testing.T) {
	w, c := focusTestColonist(t)
	c.affect.Grip, c.affect.Valence = 50, 50
	period := w.cfg.MoodValenceDecayTicks
	for i := 0; i < period; i++ {
		w.tick = i
		w.decayAffect(c)
	}
	gripLost, valenceLost := 50-c.affect.Grip, 50-c.affect.Valence
	if valenceLost != 1 {
		t.Fatalf("valence lost %d points over %d ticks, want 1", valenceLost, period)
	}
	if gripLost <= valenceLost {
		t.Fatalf("grip lost %d points and valence %d: grip should be the faster axis", gripLost, valenceLost)
	}
	w.tick = period
	w.decayAffect(c)
	if got := 50 - c.affect.Valence; got != 2 {
		t.Fatalf("valence lost %d points across the period boundary, want 2", got)
	}
}

// Nothing scored reads valence, so a colonist whose mood is merely settling
// has no reason to reconsider what they are doing.
func TestValenceOnlyChangeDoesNotDisturbCognition(t *testing.T) {
	w, c := focusTestColonist(t)
	w.tick = w.cfg.MoodValenceDecayTicks
	c.affect.Charge, c.affect.Grip, c.affect.Valence = 0, 0, 50
	c.mindDirty = false
	thinkAt := w.tick + 50
	c.nextThinkTick = thinkAt
	w.decayAffect(c)
	if c.affect.Valence != 49 {
		t.Fatalf("valence = %d, want one point of decay", c.affect.Valence)
	}
	if c.mindDirty || c.nextThinkTick != thinkAt {
		t.Fatalf("valence decay dirtied cognition: dirty=%v nextThink=%d, want %d",
			c.mindDirty, c.nextThinkTick, thinkAt)
	}
}

func TestAttractorTieUsesDeclarationOrder(t *testing.T) {
	specs := []moodAttractor{
		{Kind: MoodDriven, Charge: 0, Grip: 0, Radius: 10},
		{Kind: MoodElated, Charge: 0, Grip: 0, Radius: 10},
	}
	kind, _ := bestMoodAttractorIn(specs, 0, 0)
	if kind != MoodDriven {
		t.Fatalf("tie chose %v, want first declared attractor", kind)
	}
}

func TestMoodLabelHysteresisPreventsBoundaryFlicker(t *testing.T) {
	w, c := focusTestColonist(t)
	c.affect.Label = MoodSteady
	for _, p := range []MoodVector{{20, 20, 0}, {20, 19, 0}, {20, 20, 0}, {20, 19, 0}} {
		c.affect.Charge, c.affect.Grip = p.Charge, p.Grip
		w.refreshMoodAttractor(c)
		if c.affect.Label != MoodSteady {
			t.Fatalf("small boundary oscillation switched label to %v at %+v", c.affect.Label, p)
		}
	}
	c.affect.Charge, c.affect.Grip = 21, 20
	w.refreshMoodAttractor(c)
	if c.affect.Label != MoodSettling {
		t.Fatalf("decisive boundary crossing retained %v", c.affect.Label)
	}
}

func TestSnapshotExposesAffectCoordinatesAndLabel(t *testing.T) {
	w, c := focusTestColonist(t)
	c.affect = AffectState{Charge: 70, Grip: 60, Valence: 20, Label: MoodDriven}
	snap := w.snapshot(false, 8)
	for _, view := range snap.Entities {
		if view.ID != c.ID {
			continue
		}
		if view.Charge != 70 || view.Grip != 60 || view.Valence != 20 || view.MoodLabel != "driven" {
			t.Fatalf("snapshot affect = charge %d grip %d valence %d label %q",
				view.Charge, view.Grip, view.Valence, view.MoodLabel)
		}
		return
	}
	t.Fatal("colonist missing from snapshot")
}

func TestSnapshotUsesConfiguredAttractorNames(t *testing.T) {
	w, c := focusTestColonist(t)
	w.cognition.Attractors[MoodDriven].GoodName = "purposeful"
	w.cognition.Attractors[MoodDriven].BadName = "seething"
	c.affect = AffectState{Charge: 70, Grip: 60, Valence: -20, Label: MoodDriven}
	snap := w.snapshot(false, 8)
	for _, view := range snap.Entities {
		if view.ID == c.ID && view.MoodLabel != "seething" {
			t.Fatalf("configured mood label = %q, want seething", view.MoodLabel)
		}
	}
}

// Valence picks which of an attractor's two readings applies without moving
// the colonist or renaming the region they are in.
func TestValenceChangesMoodWordOnly(t *testing.T) {
	w, c := focusTestColonist(t)
	c.affect = AffectState{Charge: 70, Grip: 60, Valence: 30, Label: MoodDriven}
	if got := c.affect.MoodName(); got != "driven" {
		t.Fatalf("good-valence name = %q", got)
	}
	before := c.affect
	c.affect.Valence = -30
	w.refreshMoodAttractor(c)
	if got := c.affect.MoodName(); got != "furious" {
		t.Fatalf("bad-valence name = %q", got)
	}
	if c.affect.Charge != before.Charge || c.affect.Grip != before.Grip || c.affect.Label != before.Label {
		t.Fatalf("valence changed coordinates/kind: %+v -> %+v", before, c.affect)
	}
}

// The dead end this axis exists to close: a colonist who has just watched
// someone die reads badly even while well fed, unhurt and in no danger.
func TestGriefOutlastsGoodCircumstances(t *testing.T) {
	w, c := focusTestColonist(t)
	rememberTest(w, c, "witnessed-colonist-killed", "saw a killing")
	for n := NeedKind(0); n < numNeeds; n++ {
		c.Needs[n], c.needSince[n] = 0, w.tick
	}
	c.HP = c.MaxHP
	if c.affect.Valence >= 0 {
		t.Fatalf("valence after a killing = %d, want negative", c.affect.Valence)
	}
	if got := c.affect.MoodName(); got != moodName(c.affect.Label, true) {
		t.Fatalf("mood name %q read as good while grieving", got)
	}
}

func TestValenceDoesNotAffectFocusScores(t *testing.T) {
	w, glad := focusTestColonist(t)
	grim := w.spawn(Colonist, glad.Pos.Add(10, 0))
	grim.Profile = &Profile{}
	w.resolveTraitEffects(grim)
	glad.affect = AffectState{Charge: 30, Grip: -20, Valence: w.cfg.MoodMax, Label: MoodGiddy}
	grim.affect = glad.affect
	grim.affect.Valence = -w.cfg.MoodMax
	var a, b [numFocusKinds]FocusCandidate
	w.focusCandidates(glad, &a)
	w.focusCandidates(grim, &b)
	for f := FocusKind(0); f < numFocusKinds; f++ {
		if a[f].Score.Affect != b[f].Score.Affect {
			t.Fatalf("%v affect score differs by valence: %d vs %d", f, a[f].Score.Affect, b[f].Score.Affect)
		}
	}
}

func TestGripOrdersFleeAndFight(t *testing.T) {
	w, c := focusTestColonist(t)
	c.Inventory.Add(Pistol, 1)
	w.spawn(Alien, c.Pos.Add(1, 0))
	var candidates [numFocusKinds]FocusCandidate
	for _, grip := range []int{-w.cfg.MoodMax, w.cfg.MoodMax} {
		c.affect.Grip = grip
		w.focusCandidates(c, &candidates)
		flee, fight := candidates[FocusFlee].Score.Total(), candidates[FocusFight].Score.Total()
		if grip < 0 && flee <= fight || grip > 0 && fight <= flee {
			t.Fatalf("grip %d produced flee=%d fight=%d", grip, flee, fight)
		}
	}
}

func TestChargeOrdersSleepAndWork(t *testing.T) {
	w, c := focusTestColonist(t)
	c.Needs[NeedSleep], c.needSince[NeedSleep] = w.cfg.Needs[NeedSleep].SeekAt, w.tick
	var candidates [numFocusKinds]FocusCandidate
	for _, charge := range []int{-w.cfg.MoodMax, w.cfg.MoodMax} {
		c.affect.Charge = charge
		w.focusCandidates(c, &candidates)
		sleep, work := candidates[FocusSleep].Score.Total(), candidates[FocusWork].Score.Total()
		if charge < 0 && sleep <= work || charge > 0 && work <= sleep {
			t.Fatalf("charge %d produced sleep=%d work=%d", charge, sleep, work)
		}
	}
}

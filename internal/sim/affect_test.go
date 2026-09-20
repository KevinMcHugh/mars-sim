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

func TestLifeEventAppraisalTable(t *testing.T) {
	cfg := DefaultConfig()
	for kind := LifeEventKind(0); kind < numLifeEventKinds; kind++ {
		a := lifeEventAppraisals[kind]
		if a.Impact < 0 || a.Impact > 100 {
			t.Errorf("kind %d impact %d outside [0, 100]", kind, a.Impact)
		}
		if a.Fresh != (MoodVector{}) && a.Impact == 0 {
			t.Errorf("kind %d moves affect but has no impact, so it can only ever nudge", kind)
		}
		for col, vec := range map[string]MoodVector{"fresh": a.Fresh, "worn": a.Worn} {
			for name, v := range map[string]int{"charge": vec.Charge, "grip": vec.Grip, "valence": vec.Valence} {
				if v < -cfg.MoodMax || v > cfg.MoodMax {
					t.Errorf("kind %d %s %s %d outside the plane", kind, col, name, v)
				}
			}
		}
	}
	// A conversation is the one kind whose target cannot be declared: it is
	// computed per occurrence from how the chat actually went.
	if got := lifeEventAppraisals[EvtConversation].Fresh; got != (MoodVector{}) {
		t.Errorf("EvtConversation declares a target %+v that applyAffect always discards", got)
	}
}

// The events that relocate rather than nudge have to be written at the scale
// of the plane: relocating to a nudge-sized point would leave a colonist who
// just watched someone die almost exactly neutral.
func TestRelocatingEventsAreWrittenAtPlaneScale(t *testing.T) {
	cfg := DefaultConfig()
	for kind := LifeEventKind(0); kind < numLifeEventKinds; kind++ {
		a := lifeEventAppraisals[kind]
		if a.Impact < cfg.MoodPullImpact {
			continue
		}
		for col, vec := range map[string]MoodVector{"fresh": a.Fresh, "worn": a.Worn} {
			if _, claim := bestMoodAttractor(vec.Charge, vec.Grip); claim < 0 {
				t.Errorf("kind %d relocates to %s %+v, which lands in unnamed space", kind, col, vec)
			}
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
		w.remember(fed, event(EvtAte, "ate"))
		w.remember(fed, event(EvtFinishedMining, "mined"))
	}
	if fed.affect == (AffectState{Label: MoodSteady}) {
		t.Fatal("a day of meals and work left affect untouched")
	}
	for _, c := range []*Entity{fed, bare} {
		w.remember(c, event(EvtWitnessedColonistKilled, "saw a killing"))
	}
	if fed.affect.Charge != bare.affect.Charge ||
		fed.affect.Grip != bare.affect.Grip ||
		fed.affect.Valence != bare.affect.Valence {
		t.Fatalf("the day before changed where the killing left them: %+v versus %+v", fed.affect, bare.affect)
	}
	// Grip is deliberately *up* on a first killing -- that is the fresh
	// reading, the rallying cry that wear later turns into collapse. Valence is
	// what says it was a bad thing to have happened.
	if fed.affect.Valence >= 0 || fed.affect.Charge <= 0 {
		t.Fatalf("witnessing a killing left affect at %+v", fed.affect)
	}
}

// Routine life stays additive: below the push threshold nothing is relocated,
// so digs keep adding up rather than overwriting each other -- just by less
// each time, as the job stops being novel.
func TestRoutineEventsStillAccumulate(t *testing.T) {
	w, once := focusTestColonist(t)
	thrice := w.spawn(Colonist, once.Pos.Add(10, 0))
	w.remember(once, event(EvtFinishedMining, "mined"))
	for i := 0; i < 3; i++ {
		w.remember(thrice, event(EvtFinishedMining, "mined"))
	}
	if thrice.affect.Grip <= once.affect.Grip {
		t.Fatalf("three digs = %+v, want more than one dig %+v", thrice.affect, once.affect)
	}
	if thrice.affect.Grip >= 3*once.affect.Grip {
		t.Fatalf("three digs = %+v, want wear to have taken something off %+v x3", thrice.affect, once.affect)
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

// The four transforms that predate the rule table have to survive the move to
// it unchanged, for the events they already applied to.
func TestTraitMoodTransforms(t *testing.T) {
	w, _ := focusTestColonist(t)
	cases := []struct {
		name  string
		trait Trait
		kind  LifeEventKind
		in    MoodVector
		want  MoodVector
	}{
		{"tidy gore", TraitTidy, EvtSawGore, MoodVector{-3, -7, -6}, MoodVector{-6, -15, -13}},
		{"tidy incineration", TraitTidy, EvtIncineratedRefuse, MoodVector{-1, 7, 1}, MoodVector{-1, 14, 1}},
		{"industrious work", TraitIndustrious, EvtFinishedConstruction, MoodVector{-1, 6, 3}, MoodVector{-2, 12, 6}},
		{"mutant lover", TraitMutantLover, EvtMutated, MoodVector{18, -70, -35}, MoodVector{18, 70, 35}},
		{"introvert", TraitIntrovert, EvtConversation, MoodVector{3, 7, 3}, MoodVector{-3, 7, 3}},
	}
	for _, tc := range cases {
		e := &Entity{Profile: &Profile{Traits: []Trait{tc.trait}}}
		tags := lifeEventAppraisals[tc.kind].Tags
		if got := w.transformMoodVector(e, tags, tc.in); got != tc.want {
			t.Errorf("%s = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestTraitAppraisalThroughLifeEventFunnel(t *testing.T) {
	w, plain := focusTestColonist(t)
	tidy := w.spawn(Colonist, plain.Pos.Add(10, 0))
	tidy.Profile = &Profile{Traits: []Trait{TraitTidy}}
	introvert := w.spawn(Colonist, plain.Pos.Add(20, 0))
	introvert.Profile = &Profile{Traits: []Trait{TraitIntrovert}}

	w.remember(plain, event(EvtIncineratedRefuse, "burned refuse"))
	w.remember(tidy, event(EvtIncineratedRefuse, "burned refuse"))
	if tidy.affect.Grip != 2*plain.affect.Grip {
		t.Fatalf("Tidy incineration grip %d, want twice plain %d", tidy.affect.Grip, plain.affect.Grip)
	}
	w.remember(introvert, eventOutcome(EvtConversation, 7, "talked"))
	if introvert.affect.Charge >= 0 || introvert.affect.Grip <= 0 {
		t.Fatalf("Introvert conversation affect = %+v, want fatigue and restored grip", introvert.affect)
	}
	if introvert.affect.Valence <= 0 {
		t.Fatalf("Introvert conversation valence = %d, want the chat to still have done them good", introvert.affect.Valence)
	}
}

func TestTraitTransformsUseDeclarationOrder(t *testing.T) {
	w, _ := focusTestColonist(t)
	in := lifeEventAppraisals[EvtIncineratedRefuse].Fresh
	tags := lifeEventAppraisals[EvtIncineratedRefuse].Tags
	a := &Entity{Profile: &Profile{Traits: []Trait{TraitTidy, TraitIndustrious}}}
	b := &Entity{Profile: &Profile{Traits: []Trait{TraitIndustrious, TraitTidy}}}
	gotA := w.transformMoodVector(a, tags, in)
	gotB := w.transformMoodVector(b, tags, in)
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
	w.remember(c, event(EvtWitnessedColonistKilled, "saw a killing"))
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

// The headline behavior: the same event leaves a colonist somewhere different
// once they have stopped being new to it.
func TestFirstKillingAndTenthDifferInKind(t *testing.T) {
	w, c := focusTestColonist(t)
	w.remember(c, event(EvtWitnessedColonistKilled, "saw a killing"))
	first := c.affect
	for i := 0; i < 10; i++ {
		w.remember(c, event(EvtWitnessedColonistKilled, "saw another killing"))
	}
	tenth := c.affect

	if first.Grip <= 0 {
		t.Fatalf("a first killing left grip at %d, want the fresh reading to hold them together", first.Grip)
	}
	if tenth.Grip >= 0 {
		t.Fatalf("a tenth killing left grip at %d, want the worn reading to have broken it", tenth.Grip)
	}
	if tenth.Valence >= first.Valence {
		t.Fatalf("valence went %d -> %d, want the tenth to read worse than the first", first.Valence, tenth.Valence)
	}
	if tenth.Charge >= first.Charge {
		t.Fatalf("charge went %d -> %d, want the tenth to read number than the first", first.Charge, tenth.Charge)
	}
}

// Wear is not a ratchet. Occasions live in the bounded memory log, so a
// colonist who has not seen a thing in a long time meets it fresh again.
func TestWearRecoversAsMemoriesRollOff(t *testing.T) {
	w, c := focusTestColonist(t)
	for i := 0; i < 6; i++ {
		w.remember(c, event(EvtSawGore, "gore"))
	}
	worn := w.moodWear(c, EvtSawGore)
	if worn <= 0 {
		t.Fatalf("six sightings produced wear %d, want some", worn)
	}
	// A long stretch of anything else pushes those occasions out of the log.
	for i := 0; i < maxColonistMemories; i++ {
		w.remember(c, eventFrom(EvtWitnessedCatCatch, EntityID(i), "the cat got one"))
	}
	if got := w.moodWear(c, EvtSawGore); got != 0 {
		t.Fatalf("wear after the memories rolled off = %d, want 0", got)
	}
}

// Counting occurrences rather than occasions would let one uninterrupted shift
// peg a colonist forever; collapsing has already called that run one thing.
func TestCollapsedRunCountsAsOneOccasion(t *testing.T) {
	w, c := focusTestColonist(t)
	for i := 0; i < 20; i++ {
		w.remember(c, event(EvtFinishedMining, "mined"))
	}
	if got, want := w.moodWear(c, EvtFinishedMining), w.cfg.MoodWearPerOccasion; got != want {
		t.Fatalf("wear after one uninterrupted shift = %d, want %d for a single occasion", got, want)
	}
}

func TestWearTargetInterpolatesAndCaps(t *testing.T) {
	a := moodAppraisal{Fresh: MoodVector{10, 20, 30}, Worn: MoodVector{-10, 0, -10}}
	for _, tc := range []struct {
		wear int
		want MoodVector
	}{
		{0, MoodVector{10, 20, 30}},
		{50, MoodVector{0, 10, 10}},
		{100, MoodVector{-10, 0, -10}},
	} {
		if got := wearTarget(a, tc.wear); got != tc.want {
			t.Errorf("wearTarget at %d%% = %+v, want %+v", tc.wear, got, tc.want)
		}
	}
}

// A full memory log must not push wear past its worn reading and out the far
// side of the plane.
func TestWearCapsAtFullyWorn(t *testing.T) {
	w, c := focusTestColonist(t)
	for i := 0; i < maxColonistMemories; i++ {
		w.remember(c, eventFrom(EvtSawGore, EntityID(i), "gore"))
	}
	if got := w.moodWear(c, EvtSawGore); got != 100 {
		t.Fatalf("wear with a log full of one kind = %d, want it capped at 100", got)
	}
	a := lifeEventAppraisals[EvtSawGore]
	if got := wearTarget(a, w.moodWear(c, EvtSawGore)); got != a.Worn {
		t.Fatalf("fully worn target = %+v, want exactly the worn reading %+v", got, a.Worn)
	}
}

// A conversation carries its own per-occurrence appraisal and its own fatigue
// window; wearing it too would charge a talkative colonist twice.
func TestConversationIsExemptFromWear(t *testing.T) {
	w, c := focusTestColonist(t)
	for i := 0; i < 8; i++ {
		w.remember(c, eventOutcome(EvtConversation, 20, "talked"))
	}
	if w.moodWear(c, EvtConversation) == 0 {
		t.Fatal("test no longer exercises a worn colonist")
	}
	// Both start from neutral, so this compares how the chat was appraised
	// rather than where the earlier ones happened to leave them.
	c.affect = AffectState{}
	w.remember(c, eventOutcome(EvtConversation, 20, "talked"))

	fw, fresh := focusTestColonist(t)
	fresh.affect = AffectState{}
	fw.remember(fresh, eventOutcome(EvtConversation, 20, "talked"))

	if c.affect.Grip != fresh.affect.Grip || c.affect.Valence != fresh.affect.Valence {
		t.Fatalf("a worn colonist read a chat as %+v and a fresh one as %+v -- wear leaked into conversations",
			c.affect, fresh.affect)
	}
}

// The point of tags: a trait reacts to a flavor of occurrence, so an event it
// was never written against still gets the right reaction. Tidy was declared
// against gore, and a colonist being eaten is gore.
func TestTraitRulesReachEventsTheyWereNotWrittenAgainst(t *testing.T) {
	w, plain := focusTestColonist(t)
	tidy := w.spawn(Colonist, plain.Pos.Add(10, 0))
	tidy.Profile = &Profile{Traits: []Trait{TraitTidy}}
	plain.Profile = &Profile{}

	for _, c := range []*Entity{plain, tidy} {
		w.remember(c, event(EvtWitnessedColonistKilled, "saw a killing"))
	}
	if tidy.affect.Valence >= plain.affect.Valence {
		t.Fatalf("tidy colonist read a killing as valence %d, plain as %d -- the gore rule did not reach it",
			tidy.affect.Valence, plain.affect.Valence)
	}
}

func TestTraitRuleMatching(t *testing.T) {
	both := TagGore | TagDeath
	for _, tc := range []struct {
		name string
		rule traitRule
		tags EventTag
		want bool
	}{
		{"no tags matches anything", traitRule{}, TagRest, true},
		{"any hits", traitRule{Any: TagGore}, both, true},
		{"any misses", traitRule{Any: TagRest}, both, false},
		{"all present", traitRule{All: both}, both, true},
		{"all partially present", traitRule{All: both}, TagGore, false},
		{"none excludes", traitRule{Any: TagGore, None: TagDeath}, both, false},
		{"none allows", traitRule{Any: TagGore, None: TagRest}, both, true},
	} {
		if got := tc.rule.matches(tc.tags); got != tc.want {
			t.Errorf("%s: matches = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A dynamic tag carries what no table could: who it happened to.
func TestFriendTagStampedFromAffinity(t *testing.T) {
	w, mourner := focusTestColonist(t)
	stranger := w.spawn(Colonist, mourner.Pos.Add(5, 0))
	friend := w.spawn(Colonist, mourner.Pos.Add(6, 0))
	w.addAffinity(mourner.ID, friend.ID, w.cfg.MoodFriendAffinity+10)

	killed := func(subject EntityID) EventTag {
		return w.eventTags(mourner, eventAbout(EvtWitnessedColonistKilled, 99, subject, "died"))
	}
	if killed(friend.ID).none(TagFriend) {
		t.Error("a death in the family was not tagged as one")
	}
	if !killed(stranger.ID).none(TagFriend) {
		t.Error("a stranger's death was tagged as a friend's")
	}
	if !killed(0).none(TagFriend) {
		t.Error("an event with no subject was tagged as a friend's")
	}
	if !killed(mourner.ID).none(TagFriend) {
		t.Error("a colonist was counted as their own friend")
	}
}

// Wear rules change how fast experience stops being new, which is the thing a
// vector scale could never say.
func TestNerveTraitsBendWearRate(t *testing.T) {
	w, steady := focusTestColonist(t)
	tough := w.spawn(Colonist, steady.Pos.Add(10, 0))
	tough.Profile = &Profile{Traits: []Trait{TraitResilient}}
	fragile := w.spawn(Colonist, steady.Pos.Add(20, 0))
	fragile.Profile = &Profile{Traits: []Trait{TraitCowardly}}
	steady.Profile = &Profile{}

	for _, c := range []*Entity{steady, tough, fragile} {
		for i := 0; i < 3; i++ {
			w.remember(c, eventFrom(EvtSawGore, EntityID(i), "gore"))
		}
	}
	base, resilient, cowardly := w.moodWear(steady, EvtSawGore), w.moodWear(tough, EvtSawGore), w.moodWear(fragile, EvtSawGore)
	if !(resilient < base && base < cowardly) {
		t.Fatalf("wear after three sightings: resilient %d, plain %d, cowardly %d -- want them ordered",
			resilient, base, cowardly)
	}
}

// And an impact rule changes how big a deal something is, which decides
// whether it nudges a colonist or moves them.
func TestCowardiceRaisesThreatImpact(t *testing.T) {
	w, steady := focusTestColonist(t)
	fragile := w.spawn(Colonist, steady.Pos.Add(10, 0))
	fragile.Profile = &Profile{Traits: []Trait{TraitCowardly}}
	steady.Profile = &Profile{}
	for _, c := range []*Entity{steady, fragile} {
		c.affect = AffectState{Charge: 60, Grip: 60}
		w.remember(c, eventFrom(EvtSawAlien, 99, "an alien"))
	}
	// Both saw the same alien from the same mood. The coward is pulled further
	// from where they were, because for them it was a bigger deal.
	if fragile.affect.Grip >= steady.affect.Grip {
		t.Fatalf("cowardly grip %d, steady grip %d -- want the coward relocated further",
			fragile.affect.Grip, steady.affect.Grip)
	}
}

// Adding a trait is one table edit: no event kind mentions any trait.
func TestTraitRulesNameNoEventKinds(t *testing.T) {
	for _, r := range traitRules {
		if r.Trait >= numTraits {
			t.Errorf("rule for trait %d is not a declared trait", r.Trait)
		}
		if r.Any == 0 && r.All == 0 && r.None == 0 &&
			r.Charge == 0 && r.Grip == 0 && r.Valence == 0 && r.Impact == 0 && r.WearRate == 0 {
			t.Errorf("rule for %v does nothing", r.Trait)
		}
	}
	for kind := LifeEventKind(0); kind < numLifeEventKinds; kind++ {
		if kind != EvtConversation && lifeEventAppraisals[kind].Tags == 0 {
			t.Errorf("kind %d has no tags, so no trait can ever react to it", kind)
		}
	}
}

// The dynamic tag has to reach a rule, or stamping it is theatre.
func TestLosingAFriendHitsAnExtrovertHarder(t *testing.T) {
	w, c := focusTestColonist(t)
	c.Profile = &Profile{Traits: []Trait{TraitExtrovert}}
	friend := w.spawn(Colonist, c.Pos.Add(5, 0))
	stranger := w.spawn(Colonist, c.Pos.Add(6, 0))
	w.addAffinity(c.ID, friend.ID, w.cfg.MoodFriendAffinity+10)

	died := func(subject EntityID) AffectState {
		c.affect = AffectState{}
		c.Memories = nil
		w.remember(c, eventAbout(EvtWitnessedColonistKilled, 99, subject, "died"))
		return c.affect
	}
	byStranger, byFriend := died(stranger.ID), died(friend.ID)
	if byFriend.Valence >= byStranger.Valence {
		t.Fatalf("a friend's death read as valence %d and a stranger's as %d",
			byFriend.Valence, byStranger.Valence)
	}
	if byFriend.Grip != byStranger.Grip {
		t.Fatalf("the rule moved grip (%d vs %d); it is meant to leave that to the event",
			byFriend.Grip, byStranger.Grip)
	}
}

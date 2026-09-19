package sim

import "testing"

func TestLifeEventMoodVectorTable(t *testing.T) {
	want := map[LifeEventKind]MoodVector{
		EvtSawAlien: {8, -10}, EvtSawMouse: {2, -3}, EvtSawGore: {-3, -7},
		EvtBitten: {10, -8}, EvtWitnessedColonistKilled: {6, -16},
		EvtWitnessedColonistAttacked: {5, -9}, EvtCrushedMouse: {-1, 2},
		EvtWitnessedMouseCrushed: {-1, -2}, EvtWitnessedCatCatch: {1, 1},
		EvtKilledAlien: {12, 14}, EvtWitnessedAlienKilled: {5, 6},
		EvtWoundedAlien: {4, 5}, EvtWitnessedGunfight: {7, -5},
		EvtConversation: {3, 7}, EvtAte: {4, 2}, EvtUsedToilet: {1, 2},
		EvtSlept: {15, 2}, EvtNeedSatisfied: {2, 2}, EvtFinishedMining: {-1, 5},
		EvtClearedRock: {-1, 5}, EvtFinishedConstruction: {-1, 6},
		EvtCleanedRefuse: {-1, 5}, EvtIncineratedRefuse: {-1, 7},
		EvtMutated: {4, -18}, EvtWitnessedMutation: {2, -8},
	}
	for kind := LifeEventKind(0); kind < numLifeEventKinds; kind++ {
		got := lifeEventMoodVectors[kind]
		if expected, ok := want[kind]; !ok {
			if got != (MoodVector{}) {
				t.Errorf("kind %d unexpectedly has vector %+v", kind, got)
			}
		} else if got != expected {
			t.Errorf("kind %d vector = %+v, want %+v", kind, got, expected)
		}
	}
}

func TestAffectAdditionClampsBothAxes(t *testing.T) {
	w, c := focusTestColonist(t)
	c.affect.Charge, c.affect.Grip = 95, -95
	w.addAffect(c, MoodVector{Charge: 20, Grip: -20})
	if c.affect.Charge != w.cfg.MoodMax || c.affect.Grip != -w.cfg.MoodMax {
		t.Fatalf("clamped affect = %+v", c.affect)
	}
}

func TestConversationOutcomeVectors(t *testing.T) {
	for _, tc := range []struct {
		outcome int
		want    MoodVector
	}{{7, MoodVector{3, 7}}, {0, MoodVector{}}, {-6, MoodVector{4, -6}}, {1, MoodVector{1, 1}}, {-1, MoodVector{1, -1}}} {
		if got := conversationMoodVector(tc.outcome); got != tc.want {
			t.Errorf("outcome %d = %+v, want %+v", tc.outcome, got, tc.want)
		}
	}
}

func TestTraitMoodTransforms(t *testing.T) {
	cases := []struct {
		name  string
		trait Trait
		kind  LifeEventKind
		in    MoodVector
		want  MoodVector
	}{
		{"tidy gore", TraitTidy, EvtSawGore, MoodVector{-3, -7}, MoodVector{-6, -15}},
		{"tidy incineration", TraitTidy, EvtIncineratedRefuse, MoodVector{-1, 7}, MoodVector{-1, 14}},
		{"industrious work", TraitIndustrious, EvtFinishedConstruction, MoodVector{-1, 6}, MoodVector{-2, 12}},
		{"mutant lover", TraitMutantLover, EvtMutated, MoodVector{4, -18}, MoodVector{4, 18}},
		{"introvert", TraitIntrovert, EvtConversation, MoodVector{3, 7}, MoodVector{-3, 7}},
	}
	for _, tc := range cases {
		e := &Entity{Profile: &Profile{Traits: []Trait{tc.trait}}}
		if got := transformMoodVector(e, tc.kind, tc.in); got != tc.want {
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
}

func TestTraitTransformsUseDeclarationOrder(t *testing.T) {
	in := lifeEventMoodVectors[EvtIncineratedRefuse]
	a := &Entity{Profile: &Profile{Traits: []Trait{TraitTidy, TraitIndustrious}}}
	b := &Entity{Profile: &Profile{Traits: []Trait{TraitIndustrious, TraitTidy}}}
	gotA := transformMoodVector(a, EvtIncineratedRefuse, in)
	gotB := transformMoodVector(b, EvtIncineratedRefuse, in)
	if gotA != gotB || gotA != (MoodVector{-2, 28}) {
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
	for _, p := range []MoodVector{{20, 20}, {20, 19}, {20, 20}, {20, 19}} {
		c.affect.Charge, c.affect.Grip = p.Charge, p.Grip
		w.refreshMoodLabel(c)
		if c.affect.Label != MoodSteady {
			t.Fatalf("small boundary oscillation switched label to %v at %+v", c.affect.Label, p)
		}
	}
	c.affect.Charge, c.affect.Grip = 21, 20
	w.refreshMoodLabel(c)
	if c.affect.Label != MoodSettling {
		t.Fatalf("decisive boundary crossing retained %v", c.affect.Label)
	}
}

func TestSnapshotExposesAffectCoordinatesAndLabel(t *testing.T) {
	w, c := focusTestColonist(t)
	c.affect = AffectState{Charge: 70, Grip: 60, Label: MoodDriven, labelName: "driven"}
	snap := w.snapshot(false, 8)
	for _, view := range snap.Entities {
		if view.ID != c.ID {
			continue
		}
		if view.Charge != 70 || view.Grip != 60 || view.MoodLabel != "driven" {
			t.Fatalf("snapshot affect = charge %d grip %d label %q", view.Charge, view.Grip, view.MoodLabel)
		}
		return
	}
	t.Fatal("colonist missing from snapshot")
}

func TestContextChangesMoodWordOnly(t *testing.T) {
	w, c := focusTestColonist(t)
	c.affect = AffectState{Charge: 70, Grip: 60, Label: MoodDriven}
	w.refreshMoodLabel(c)
	if got := c.affect.MoodName(); got != "driven" {
		t.Fatalf("good-context name = %q", got)
	}
	before := c.affect
	c.HP = 1
	w.refreshMoodLabel(c)
	if got := c.affect.MoodName(); got != "furious" {
		t.Fatalf("bad-context name = %q", got)
	}
	if c.affect.Charge != before.Charge || c.affect.Grip != before.Grip || c.affect.Label != before.Label {
		t.Fatalf("context changed coordinates/kind: %+v -> %+v", before, c.affect)
	}
}

func TestContextualValenceDoesNotAffectFocusScores(t *testing.T) {
	w, healthy := focusTestColonist(t)
	injured := w.spawn(Colonist, healthy.Pos.Add(10, 0))
	injured.Profile = &Profile{}
	w.resolveTraitEffects(injured)
	healthy.affect = AffectState{Charge: 30, Grip: -20, Label: MoodGiddy}
	injured.affect = healthy.affect
	injured.HP = 1
	w.refreshMoodLabel(healthy)
	w.refreshMoodLabel(injured)
	var a, b [numFocusKinds]FocusCandidate
	w.focusCandidates(healthy, &a)
	w.focusCandidates(injured, &b)
	for f := FocusKind(0); f < numFocusKinds; f++ {
		if a[f].Score.Affect != b[f].Score.Affect {
			t.Fatalf("%v affect score differs by context: %d vs %d", f, a[f].Score.Affect, b[f].Score.Affect)
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

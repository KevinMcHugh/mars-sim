package sim

import "testing"

func labCognitionFrom(cfg Config) LabCognition {
	cog := LabCognition{
		Focuses: map[string]LabFocus{},
		Arbitration: LabArbitration{
			CurrentBonus:  cfg.FocusCurrentBonus,
			SwitchMargin:  cfg.FocusSwitchMargin,
			CriticalBonus: cfg.FocusCriticalBonus,
			FatalBonus:    cfg.FocusFatalBonus,
		},
	}
	for f := FocusKind(0); f < numFocusKinds; f++ {
		spec := cfg.Focuses[f]
		cog.Focuses[f.String()] = LabFocus{
			Base: spec.Base, NeedWeight: spec.NeedWeight,
			ChargeWeight: spec.ChargeWeight, GripWeight: spec.GripWeight,
			DistanceWeight: spec.DistanceWeight,
		}
	}
	for _, a := range cfg.Cognition.Attractors {
		cog.Attractors = append(cog.Attractors, LabAttractor{
			Kind: a.Kind.String(), Good: a.GoodName, Bad: a.BadName,
			Charge: a.Charge, Grip: a.Grip, Radius: a.Radius,
		})
	}
	return cog
}

func labSituationFrom(w *World, e *Entity) LabSituation {
	sit := LabSituation{
		Charge: e.affect.Charge, Grip: e.affect.Grip, Valence: e.affect.Valence,
		MoodLabel: e.affect.Label.String(),
		Drives:    map[string]int{},
		Current:   e.focus.String(),
		CanWork:   workJob(e.Job) || !e.resting || w.tick >= e.wakeTick,
		Pod:       true, Toilet: true, Bed: true, Company: true,
		Alien:  false,
		Armed:  bestWeapon(e.Inventory) != ItemNone,
		Sealed: e.disconnectedTicks >= w.cfg.EscapeGraceTicks,
	}
	for n := DriveKind(0); n < numDrives; n++ {
		sit.Drives[n.String()] = w.driveLevel(e, n)
	}
	if e.Profile != nil {
		for _, trait := range e.Profile.Traits {
			sit.Traits = append(sit.Traits, canonicalID(trait.Name()))
		}
	}
	return sit
}

func TestLabEvaluateMatchesChooseFocus(t *testing.T) {
	w, c := focusTestColonist(t)
	c.focus = FocusWork
	c.Job = JobMine
	c.Drives[DriveFood] = w.cfg.Drives[DriveFood].SeekAt
	c.driveSince[DriveFood] = w.tick

	var candidates [numFocusKinds]FocusCandidate
	want := w.chooseFocus(c, &candidates)
	got, err := LabEvaluate(w.cfg.Drives, w.cfg.MoodMax, w.cfg.MoodLabelSwitchMargin, labCognitionFrom(w.cfg), labSituationFrom(w, c))
	if err != nil {
		t.Fatal(err)
	}
	if got.Winner.ID != want.Kind.String() {
		t.Fatalf("winner %s, chooseFocus %s", got.Winner.ID, want.Kind)
	}
	for f := FocusKind(0); f < numFocusKinds; f++ {
		lab := got.Candidates[f]
		world := candidates[f]
		if lab.ID != f.String() {
			t.Fatalf("candidate %d id %s", f, lab.ID)
		}
		if lab.Total != world.Score.Total() || lab.Eligible != world.Eligible {
			t.Fatalf("%s lab total=%d eligible=%v, world total=%d eligible=%v",
				f, lab.Total, lab.Eligible, world.Score.Total(), world.Eligible)
		}
	}
}

func TestLabBenchReachDropsEligibilityOnly(t *testing.T) {
	w, c := focusTestColonist(t)
	c.Drives[DriveFood] = w.cfg.Drives[DriveFood].SeekAt
	c.driveSince[DriveFood] = w.tick
	sit := labSituationFrom(w, c)
	sit.Pod = false
	sit.Current = FocusWork.String()
	got, err := LabEvaluate(w.cfg.Drives, w.cfg.MoodMax, w.cfg.MoodLabelSwitchMargin, labCognitionFrom(w.cfg), sit)
	if err != nil {
		t.Fatal(err)
	}
	eat := got.Candidates[FocusEat]
	if eat.Eligible {
		t.Fatal("eat stayed eligible with no pod")
	}
	if eat.Need == 0 {
		t.Fatal("bench gate cleared the need score")
	}
	if got.Winner.ID == FocusEat.String() {
		t.Fatal("chooser kept an out-of-reach eat")
	}
}

func TestLabRollIsStable(t *testing.T) {
	a := LabRoll(7, 30)
	b := LabRoll(7, 30)
	if a.Name != b.Name || a.Age != b.Age || a.HeightCM != b.HeightCM {
		t.Fatalf("same seed diverged: %+v vs %+v", a, b)
	}
	if a.Name == "" || a.Age < 18 {
		t.Fatalf("empty roll: %+v", a)
	}
	other := LabRoll(8, 30)
	if other.Name == a.Name && other.Age == a.Age && other.HeightCM == a.HeightCM {
		t.Fatal("different seeds produced the same colonist")
	}
}

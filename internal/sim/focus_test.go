package sim

import (
	"strings"
	"testing"
)

func focusTestColonist(t *testing.T) (*World, *Entity) {
	t.Helper()
	cfg := testConfig()
	cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartRats = 0, 0, 0, 0
	w := newTestWorld(t, cfg)
	c := w.spawn(Colonist, Point{w.Width / 2, w.Height / 2})
	return w, c
}

func TestFocusScoreTotalAndFormat(t *testing.T) {
	c := FocusCandidate{
		Kind:     FocusWork,
		Eligible: true,
		Score: FocusScore{
			Base: 1, Need: 2, Affect: 3, Stimulus: 4,
			Personality: 5, Commitment: 6, Distance: -7,
		},
	}
	if got := c.Score.Total(); got != 14 {
		t.Fatalf("Total = %d, want 14", got)
	}
	text := c.String()
	for _, want := range []string{"work", "base=1", "distance=-7", "total=14"} {
		if !strings.Contains(text, want) {
			t.Errorf("format %q does not contain %q", text, want)
		}
	}
}

func TestFocusCommitmentRequiresEligibility(t *testing.T) {
	w, c := focusTestColonist(t)
	c.focus = FocusEat
	var candidates [numFocusKinds]FocusCandidate
	w.focusCandidates(c, &candidates)
	if candidates[FocusEat].Eligible || candidates[FocusEat].Score.Commitment != 0 {
		t.Fatalf("satisfied eat candidate = %+v, want ineligible without commitment", candidates[FocusEat])
	}

	c.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt
	w.focusCandidates(c, &candidates)
	if !candidates[FocusEat].Eligible ||
		candidates[FocusEat].Score.Commitment != w.cfg.FocusCurrentBonus {
		t.Fatalf("pressing eat candidate = %+v, want eligible with commitment", candidates[FocusEat])
	}
}

func TestFocusSwitchRequiresStrictMargin(t *testing.T) {
	w, c := focusTestColonist(t)
	c.focus = FocusIdle
	currentTotal := w.cfg.Focuses[FocusIdle].Base + w.cfg.FocusCurrentBonus
	w.cfg.Focuses[FocusWork].Base = currentTotal + w.cfg.FocusSwitchMargin

	var candidates [numFocusKinds]FocusCandidate
	if got := w.chooseFocus(c, &candidates).Kind; got != FocusIdle {
		t.Fatalf("equal-to-margin challenger selected %v, want idle", got)
	}
	w.cfg.Focuses[FocusWork].Base++
	if got := w.chooseFocus(c, &candidates).Kind; got != FocusWork {
		t.Fatalf("challenger over margin selected %v, want work", got)
	}
}

func TestFocusTieOrderingIsDeterministic(t *testing.T) {
	w, c := focusTestColonist(t)
	c.focus = FocusEat // ineligible, so it cannot receive tie preference
	w.cfg.FocusCurrentBonus = 0
	w.cfg.FocusSwitchMargin = 0
	for f := FocusKind(0); f < numFocusKinds; f++ {
		w.cfg.Focuses[f].Base = 0
	}
	var candidates [numFocusKinds]FocusCandidate
	if got := w.chooseFocus(c, &candidates).Kind; got != FocusIdle {
		t.Fatalf("equal candidates selected %v, want lower FocusIdle", got)
	}
	if candidates[FocusFight].Eligible {
		t.Fatal("fight candidate eligible without a live threat and weapon")
	}
}

func TestFatalPressingNeedSuppressesNonFatalNeeds(t *testing.T) {
	w, c := focusTestColonist(t)
	c.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt
	c.Needs[NeedBladder] = w.cfg.Needs[NeedBladder].Max
	var candidates [numFocusKinds]FocusCandidate
	w.focusCandidates(c, &candidates)
	if !candidates[FocusEat].Eligible {
		t.Fatal("fatal food candidate is not eligible")
	}
	if candidates[FocusRelieve].Eligible {
		t.Fatal("non-fatal bladder candidate remained eligible beside fatal food")
	}
}

func TestCriticalFatalNeedBeatsWorkCommitment(t *testing.T) {
	w, c := focusTestColonist(t)
	c.focus = FocusWork
	c.Job = JobMine
	c.Needs[NeedFood], c.needSince[NeedFood] = w.cfg.Needs[NeedFood].Max, w.tick
	var candidates [numFocusKinds]FocusCandidate
	if got := w.chooseFocus(c, &candidates).Kind; got != FocusEat {
		t.Fatalf("focus = %v, want critical food to preempt committed work", got)
	}
}

func TestVisibleThreatBeatsCriticalHunger(t *testing.T) {
	w, c := focusTestColonist(t)
	c.Needs[NeedFood] = w.cfg.Needs[NeedFood].Max
	w.spawn(Alien, c.Pos.Add(1, 0))
	w.observeNearby(c)
	var candidates [numFocusKinds]FocusCandidate
	if got := w.chooseFocus(c, &candidates).Kind; got != FocusFlee {
		t.Fatalf("focus = %v, want flee from visible threat", got)
	}
}

func TestArmedThreatCanFightAndUnarmedCannot(t *testing.T) {
	w, c := focusTestColonist(t)
	w.spawn(Alien, c.Pos.Add(1, 0))
	w.observeNearby(c)
	var candidates [numFocusKinds]FocusCandidate
	w.focusCandidates(c, &candidates)
	if candidates[FocusFight].Eligible {
		t.Fatal("unarmed fight candidate is eligible")
	}

	c.Inventory.Add(Pistol, 1)
	c.affect.Grip = w.cfg.MoodMax
	if got := w.chooseFocus(c, &candidates).Kind; got != FocusFight {
		t.Fatalf("armed focus = %v, want fight", got)
	}
}

func TestFocusTransitionReleasesMineClaim(t *testing.T) {
	w, c := focusTestColonist(t)
	target := c.Pos.Add(1, 0)
	w.SetTerrain(target, Rock)
	w.board.claimMine(target, c.ID)
	c.Job, c.Target, c.mineClaimed = JobMine, target, true
	c.focus = FocusWork
	c.Needs[NeedFood] = w.cfg.Needs[NeedFood].SeekAt

	w.colonistTurn(c)
	if w.board.isClaimed(target) {
		t.Fatal("mine claim survived focus transition")
	}
	if c.focus != FocusEat {
		t.Fatalf("focus = %v, want eat", c.focus)
	}
}

func TestSnapshotExposesFocus(t *testing.T) {
	w, c := focusTestColonist(t)
	c.focus = FocusSleep
	view := w.entityView(c, nil, true)
	if view.Focus != FocusSleep {
		t.Fatalf("snapshot focus = %v, want sleep", view.Focus)
	}
}

func BenchmarkFocusCandidates(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Seed = 1
	w := newWorld(cfg, nil)
	c := newEntity(1, Colonist, Point{1, 1}, cfg)
	c.Needs[NeedFood] = cfg.Needs[NeedFood].SeekAt
	w.entities[c.ID] = c
	var candidates [numFocusKinds]FocusCandidate
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.focusCandidates(c, &candidates)
	}
}

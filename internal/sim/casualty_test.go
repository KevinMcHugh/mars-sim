package sim

import (
	"slices"
	"testing"
)

// aidWorld is a small open floor with a colonist about to be a casualty and a
// bystander three tiles off, both fed, rested, without traits and unrelated.
// Family is off because driveWorld's seed is the clock's: on some runs the
// bystander would otherwise arrive as the casualty's cousin, already warm
// toward it.
func aidWorld(t *testing.T, edit func(*Config)) (w *World, casualty, bystander *Entity) {
	t.Helper()
	w, casualty = driveWorld(t, func(c *Config) {
		c.FamilyChance = 0
		if edit != nil {
			edit(c)
		}
	})
	bystander = w.spawn(Colonist, casualty.Pos.Add(3, 0))
	for _, e := range []*Entity{casualty, bystander} {
		e.Profile.Traits = nil
		for d := DriveKind(0); d < numDrives; d++ {
			w.setDrive(e, d, 0)
		}
	}
	return w, casualty, bystander
}

// knockOut passes c out from exhaustion where it stands, as the sleep drive's
// ceiling would.
func knockOut(w *World, c *Entity) {
	w.setDrive(c, DriveSleep, w.cfg.Drives[DriveSleep].Max)
	w.applyDriveConsequences(c)
}

// stepUntil steps w until done or limit ticks pass, and reports the ticks it
// took (limit+1 if it never happened).
func stepUntil(w *World, limit int, done func() bool) int {
	for i := 1; i <= limit; i++ {
		w.step()
		if done() {
			return i
		}
	}
	return limit + 1
}

// Passing out puts a colonist in the casualty index; how much danger it is in
// follows its hunger; coming to on its own takes it out again.
func TestPassingOutMakesACasualty(t *testing.T) {
	w, c := driveWorld(t, func(c *Config) { c.Aid = false })
	food := w.cfg.Drives[DriveFood]
	knockOut(w, c)
	if !c.passedOut || !w.isCasualty(c) {
		t.Fatal("a passed-out colonist is not a casualty")
	}
	for _, tc := range []struct {
		food int
		want Peril
	}{{0, PerilStable}, {food.SeekAt, PerilSerious}, {food.Max, PerilCritical}} {
		w.setDrive(c, DriveFood, tc.food)
		if got := w.perilOf(c); got != tc.want {
			t.Errorf("hunger %d: peril %v, want %v", tc.food, got, tc.want)
		}
	}
	w.setDrive(c, DriveFood, 0)
	c.HP = c.MaxHP * (w.cfg.PerilCriticalHPPct - 1) / 100
	if got := w.perilOf(c); got != PerilCritical {
		t.Errorf("badly hurt: peril %v, want critical", got)
	}
	c.HP = c.MaxHP

	w.setDrive(c, DriveSleep, w.cfg.Drives[DriveSleep].CriticalAt-1)
	w.step()
	if c.passedOut || w.isCasualty(c) || len(w.casualties) != 0 {
		t.Fatalf("came to but still a casualty: %v", w.casualties)
	}
	if got := w.perilOf(c); got != PerilNone {
		t.Fatalf("peril on its feet = %v", got)
	}
}

// A bystander who sees a colonist collapse goes over and brings it round long
// before it would have come to on its own. Both remember it, the rescued one
// is grateful, and the roused effect keeps it from dropping again at once.
func TestBystanderRousesACasualty(t *testing.T) {
	w, c, b := aidWorld(t, nil)
	knockOut(w, c)
	if took := stepUntil(w, 40, func() bool { return !c.passedOut }); took > 40 {
		t.Fatalf("still out after 40 ticks; bystander focus %v job %v", b.focus, b.Job)
	}
	if got := memoriesOf(b, "helped"); got != 1 {
		t.Errorf("helper's helped memories = %d, want 1", got)
	}
	if got := memoriesOf(c, "was-helped"); got != 1 {
		t.Errorf("casualty's was-helped memories = %d, want 1", got)
	}
	if got, want := w.affinityBetween(c.ID, b.ID), w.cfg.AidGratitude; got < want {
		t.Errorf("gratitude toward the rescuer = %d, want at least %d", got, want)
	}
	if w.isCasualty(c) || c.aidedBy != 0 || b.aidTarget != 0 || b.Job == JobAid {
		t.Fatalf("claim left behind: casualty %v aidedBy %d aidTarget %d job %v", w.isCasualty(c), c.aidedBy, b.aidTarget, b.Job)
	}
	if !slices.ContainsFunc(c.effects, func(fx activeEffect) bool {
		return w.cfg.DriveEffects[fx.kind].Name == rousedEffectName
	}) {
		t.Fatal("roused colonist has no roused effect")
	}
	if lvl := w.driveLevel(c, DriveSleep); lvl >= w.cfg.Drives[DriveSleep].Max {
		t.Fatalf("roused at the sleep ceiling (%d): it would collapse again at once", lvl)
	}
	if got := memoriesOf(c, "passed-out"); got != 1 {
		t.Fatalf("passed out %d times; the roused effect should hold it up", got)
	}
}

// A hungry casualty gets the helper's own meal: a gift.
func TestHelperSharesAMealWithAHungryCasualty(t *testing.T) {
	w, c, b := aidWorld(t, nil)
	b.Inventory.Add(Meal, 1)
	w.setDrive(c, DriveFood, w.cfg.Drives[DriveFood].SeekAt+10)
	knockOut(w, c)
	if took := stepUntil(w, 40, func() bool { return !c.passedOut }); took > 40 {
		t.Fatal("never brought round")
	}
	if b.ownCarried(Meal) != 0 {
		t.Fatal("helper kept its meal")
	}
	if c.ownCarried(Meal) != 1 && c.Job != JobEat {
		t.Fatalf("casualty has no meal (carried %d, job %v)", c.ownCarried(Meal), c.Job)
	}
}

// With aid off nobody stops, and a casualty lies there until it comes to.
func TestNobodyHelpsWithAidOff(t *testing.T) {
	w, c, b := aidWorld(t, func(c *Config) { c.Aid = false })
	knockOut(w, c)
	for range 40 {
		w.step()
	}
	if !c.passedOut || memoriesOf(b, "helped") != 0 {
		t.Fatal("someone helped with aid off")
	}
}

// How a helper's ties and nature weigh: affinity either way, Caring for
// anyone, Callous against strangers, a coward while an alien is in sight.
func TestAidRegard(t *testing.T) {
	w, c, b := aidWorld(t, nil)
	if got := w.aidRegard(b, c, false); got != 0 {
		t.Fatalf("stranger, no traits: %d, want 0", got)
	}
	w.bumpAffinity(b.ID, c.ID, -80)
	if got, want := w.aidRegard(b, c, false), w.cfg.AidAffinityWeight*-80/w.cfg.AffinityMax; got != want {
		t.Fatalf("rival: %d, want %d", got, want)
	}
	w.bumpAffinity(b.ID, c.ID, 80)
	for _, tc := range []struct {
		trait  Trait
		threat bool
		want   int
	}{
		{TraitCaring, false, w.cfg.AidTraitBonus},
		{TraitCallous, false, -w.cfg.AidTraitBonus},
		{TraitCowardly, false, 0},
		{TraitCowardly, true, -w.cfg.AidCowardPenalty},
	} {
		b.Profile.Traits = []Trait{tc.trait}
		if got := w.aidRegard(b, c, tc.threat); got != tc.want {
			t.Errorf("%s (threat %v): %d, want %d", traitSpecs[tc.trait].Name, tc.threat, got, tc.want)
		}
	}
}

// A colonist whose arbitration is offered the rescue and picks something else
// is remembered on the casualty, and feels it if the casualty dies: guilt for
// no reason, lighter guilt with one, none for running from a threat, none for
// a Callous stranger. The helper on its way grieves instead.
func TestGuiltForPassingADyingCasualty(t *testing.T) {
	w, c, b := aidWorld(t, nil)
	hungry := w.spawn(Colonist, c.Pos.Add(-2, 0))
	fled := w.spawn(Colonist, c.Pos.Add(0, 2))
	callous := w.spawn(Colonist, c.Pos.Add(0, -2))
	helper := w.spawn(Colonist, c.Pos.Add(2, 2))
	for _, e := range []*Entity{hungry, fled, callous, helper} {
		e.Profile.Traits = nil
	}
	callous.Profile.Traits = []Trait{TraitCallous}
	w.setDrive(hungry, DriveFood, w.cfg.Drives[DriveFood].SeekAt+10)
	w.syncDrivePhase(hungry, DriveFood)
	knockOut(w, c)

	w.notePass(b, c.ID, FocusCandidate{Kind: FocusWork})
	w.notePass(hungry, c.ID, FocusCandidate{Kind: FocusEat, Drive: DriveFood})
	w.notePass(fled, c.ID, FocusCandidate{Kind: FocusFlee})
	w.notePass(callous, c.ID, FocusCandidate{Kind: FocusIdle})
	if got := len(c.aidPassedBy); got != 3 {
		t.Fatalf("passed-by record holds %d, want 3 (fleeing is no failing)", got)
	}
	c.aidedBy, helper.aidTarget = helper.ID, c.ID

	w.remove(c.ID, "test")
	if memoriesOf(b, "guilt") != 1 {
		t.Error("walked past for work: no guilt")
	}
	if memoriesOf(hungry, "guilt-light") != 1 || memoriesOf(hungry, "guilt") != 0 {
		t.Error("passed it to eat: want light guilt only")
	}
	if memoriesOf(fled, "guilt")+memoriesOf(fled, "guilt-light") != 0 {
		t.Error("fled a threat and felt guilty")
	}
	if memoriesOf(callous, "guilt")+memoriesOf(callous, "guilt-light") != 0 {
		t.Error("a Callous stranger felt guilty")
	}
	if memoriesOf(helper, "couldnt-save") != 1 {
		t.Error("the helper on its way did not grieve")
	}
	if len(w.casualties) != 0 || w.aidDeaths != 1 {
		t.Fatalf("casualties %v, aid deaths %d", w.casualties, w.aidDeaths)
	}
}

// Arbitration records the pass itself: a bystander offered the rescue that
// chooses something else (here, aid made worthless) is on the casualty's
// record, and helping takes it off.
func TestArbitrationRecordsAPass(t *testing.T) {
	w, c, b := aidWorld(t, func(c *Config) { c.AidPerilStable = -1000 })
	knockOut(w, c)
	for range 5 {
		w.step()
	}
	if b.focus == FocusAid {
		t.Fatal("helped although aid scored -1000")
	}
	if !slices.ContainsFunc(c.aidPassedBy, func(p aidPass) bool { return p.id == b.ID }) {
		t.Fatalf("bystander not on the passed-by record: %v", c.aidPassedBy)
	}
}

// A hostile hunter goes for a passed-out colonist a little further off before
// a standing one nearer to it.
func TestHunterPrefersHelplessPrey(t *testing.T) {
	for _, tc := range []struct {
		discount int
		helpless bool
	}{{6, true}, {0, false}} {
		w, c, b := aidWorld(t, func(c *Config) { c.HelplessPreyDiscount = tc.discount })
		alien := w.spawn(Alien, b.Pos.Add(3, 0)) // standing colonist 3 off, casualty 6
		knockOut(w, c)
		prey, ok := preyInRoom(hostilePrey)(w, alien)
		if !ok {
			t.Fatal("no prey found")
		}
		if got := prey == c; got != tc.helpless {
			t.Errorf("discount %d: chose %d (casualty %d, standing %d)", tc.discount, prey.ID, c.ID, b.ID)
		}
	}
}

// The compassion group is rolled on its own stream: reseeding that stream
// changes who is Caring or Callous and nothing else any colonist rolls.
func TestCompassionMovesNoOtherTrait(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TraitChance = 100
	a, b := newWorld(cfg, newPCG(7)), newWorld(cfg, newPCG(7))
	b.compassionRNG = newRand(999)
	others := func(e *Entity) []Trait {
		return slices.DeleteFunc(slices.Clone(e.Profile.Traits), func(t Trait) bool {
			return traitSpecs[t].group == groupCompassion
		})
	}
	compassionate, differ := 0, false
	for i := range 20 {
		at := Point{2 + i, 2, LandingLevel}
		x, y := a.spawn(Colonist, at), b.spawn(Colonist, at)
		if x.Profile.Name != y.Profile.Name || !slices.Equal(others(x), others(y)) {
			t.Fatalf("colonist %d: %s %v vs %s %v", i, x.Profile.Name, others(x), y.Profile.Name, others(y))
		}
		if len(others(x)) != len(x.Profile.Traits) {
			compassionate++
		}
		differ = differ || !slices.Equal(x.Profile.Traits, y.Profile.Traits)
	}
	if compassionate == 0 || !differ {
		t.Fatalf("compassion never rolled (%d) or never moved with its stream (%v)", compassionate, differ)
	}
}

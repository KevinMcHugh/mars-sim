package sim

import (
	"math/rand"
	"testing"
)

// personalityWorld builds a colonist-free world with a given trait chance.
func personalityWorld(traitChance int) *World {
	cfg := DefaultConfig()
	cfg.Width, cfg.Height = 30, 20
	cfg.TraitChance = traitChance
	return newWorld(cfg, rand.New(rand.NewSource(1)))
}

// Every colonist gets a populated profile: a name and plausible attributes.
func TestColonistProfilePopulated(t *testing.T) {
	w := personalityWorld(30)
	for i := 0; i < 200; i++ {
		e := w.spawn(Colonist, Point{i % w.Width, 0})
		p := e.Profile
		if p == nil {
			t.Fatal("colonist spawned without a profile")
		}
		if p.Name == "" {
			t.Fatal("colonist has an empty name")
		}
		if p.HeightCM < 140 || p.HeightCM > 210 {
			t.Fatalf("implausible height %d cm", p.HeightCM)
		}
		if p.WeightKG < 30 || p.WeightKG > 250 {
			t.Fatalf("implausible weight %d kg", p.WeightKG)
		}
		if p.Sex > SexIntersex || p.Gender > GenderNonbinary || p.Orientation > Asexual {
			t.Fatalf("attribute out of range: %+v", p)
		}
	}
}

// Aliens have no profile.
func TestAlienHasNoProfile(t *testing.T) {
	w := personalityWorld(30)
	if e := w.spawn(Alien, Point{1, 1}); e.Profile != nil {
		t.Fatal("alien should not have a profile")
	}
}

// TraitChance gates trait assignment: none at 0, one per group at 100. Attributes
// are always populated regardless.
func TestTraitChanceGatesTraits(t *testing.T) {
	none := personalityWorld(0)
	for i := 0; i < 50; i++ {
		if e := none.spawn(Colonist, Point{i % none.Width, 0}); len(e.Profile.Traits) != 0 {
			t.Fatalf("TraitChance=0 should give no traits, got %v", e.Profile.Traits)
		}
	}
	all := personalityWorld(100)
	for i := 0; i < 50; i++ {
		e := all.spawn(Colonist, Point{i % all.Width, 0})
		if len(e.Profile.Traits) != int(numTraitGroups) {
			t.Fatalf("TraitChance=100 should give one trait per group (%d), got %v",
				numTraitGroups, e.Profile.Traits)
		}
	}
}

// Traits change the colonist's effective parameters: appetite scales food rise,
// work ethic scales work speed and rest.
func TestTraitsScaleEffectiveParams(t *testing.T) {
	w := personalityWorld(0)
	base := w.spawn(Colonist, Point{1, 1})
	baseRise, baseRest := base.needRise[NeedFood], base.restTicks

	withTrait := func(tr Trait) *Entity {
		e := w.spawn(Colonist, Point{2, 2})
		e.Profile.Traits = []Trait{tr}
		w.resolveTraitEffects(e)
		return e
	}

	if big := withTrait(TraitBigEater); big.needRise[NeedFood] <= baseRise {
		t.Fatalf("big eater food rise %d should exceed baseline %d", big.needRise[NeedFood], baseRise)
	}
	if light := withTrait(TraitLightEater); light.needRise[NeedFood] >= baseRise {
		t.Fatalf("light eater food rise %d should be below baseline %d", light.needRise[NeedFood], baseRise)
	}

	ind, lazy := withTrait(TraitIndustrious), withTrait(TraitLazy)
	if ind.restTicks >= baseRest {
		t.Fatalf("industrious rest %d should be below baseline %d", ind.restTicks, baseRest)
	}
	if lazy.restTicks <= baseRest {
		t.Fatalf("lazy rest %d should exceed baseline %d", lazy.restTicks, baseRest)
	}
	if scaleTicks(w.cfg.MineTicks, ind.workScale) >= scaleTicks(w.cfg.MineTicks, lazy.workScale) {
		t.Fatalf("industrious should mine faster than lazy (%.2f vs %.2f)", ind.workScale, lazy.workScale)
	}
}

func TestSocialTraitsScaleSocialNeed(t *testing.T) {
	w := personalityWorld(0)
	base := w.spawn(Colonist, Point{1, 1})
	baseRise := base.needRise[NeedSocial]

	withTrait := func(tr Trait) *Entity {
		e := w.spawn(Colonist, Point{2 + int(tr), 2})
		e.Profile.Traits = []Trait{tr}
		w.resolveTraitEffects(e)
		return e
	}

	asocial := withTrait(TraitAsocial)
	if asocial.needRise[NeedSocial] != 0 {
		t.Fatalf("asocial social rise: got %d want 0", asocial.needRise[NeedSocial])
	}
	introvert := withTrait(TraitIntrovert)
	if introvert.needRise[NeedSocial] >= baseRise {
		t.Fatalf("introvert social rise %d should be below baseline %d",
			introvert.needRise[NeedSocial], baseRise)
	}
	extrovert := withTrait(TraitExtrovert)
	if extrovert.needRise[NeedSocial] <= baseRise {
		t.Fatalf("extrovert social rise %d should exceed baseline %d",
			extrovert.needRise[NeedSocial], baseRise)
	}
}

func TestIntrovertConversationFatigue(t *testing.T) {
	w := personalityWorld(0)
	e := w.spawn(Colonist, Point{1, 1})
	e.Profile.Traits = []Trait{TraitIntrovert}
	w.resolveTraitEffects(e)
	w.tick = 1

	if got := w.noteConversation(e); got != 0 {
		t.Fatalf("first introvert conversation should not cause fatigue: %d", got)
	}
	if got := w.noteConversation(e); got != 0 {
		t.Fatalf("second introvert conversation should not cause fatigue: %d", got)
	}
	if got := w.noteConversation(e); got >= 0 {
		t.Fatalf("third introvert conversation should reduce mood, got %d", got)
	}

	w.tick += w.cfg.SocialWindowTicks
	if got := w.noteConversation(e); got != 0 {
		t.Fatalf("conversation after the social window should not inherit fatigue: %d", got)
	}
}

// A big eater's hunger outpaces a baseline colonist's over the same elapsed time.
func TestBigEaterHungersFaster(t *testing.T) {
	w := personalityWorld(0)
	base := w.spawn(Colonist, Point{1, 1})
	big := w.spawn(Colonist, Point{2, 2})
	big.Profile.Traits = []Trait{TraitBigEater}
	w.resolveTraitEffects(big)

	for _, e := range []*Entity{base, big} {
		e.Needs[NeedFood], e.needSince[NeedFood] = 0, 0
	}
	w.tick = 100
	if w.needLevel(big, NeedFood) <= w.needLevel(base, NeedFood) {
		t.Fatalf("big eater hunger %d should exceed baseline %d after 100 ticks",
			w.needLevel(big, NeedFood), w.needLevel(base, NeedFood))
	}
}

// Personality generation is deterministic: the same seed produces the same
// colonists (names and traits) in the same order.
func TestPersonalityDeterministic(t *testing.T) {
	profiles := func() []Profile {
		cfg := DefaultConfig()
		cfg.Seed, cfg.StartColonists, cfg.StartAliens = 99, 12, 0
		w := NewEngine(cfg).world
		var out []Profile
		for _, id := range w.entityIDsSorted() {
			if e := w.entities[id]; e.Kind == Colonist {
				out = append(out, *e.Profile)
			}
		}
		return out
	}
	a, b := profiles(), profiles()
	if len(a) != len(b) {
		t.Fatalf("colonist count differs: %d != %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Sex != b[i].Sex || len(a[i].Traits) != len(b[i].Traits) {
			t.Fatalf("personality nondeterministic at %d: %+v != %+v", i, a[i], b[i])
		}
	}
}

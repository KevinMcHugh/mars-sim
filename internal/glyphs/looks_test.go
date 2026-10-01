package glyphs

import (
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The page draws the first candidate its font fuses, so the list must end in
// something every font has: the plain figure from All, which the TUI vets.
func TestEveryLookEndsInItsPlainFigure(t *testing.T) {
	seen := map[string]bool{}
	for i, l := range Looks {
		if len(l) == 0 {
			t.Fatalf("look %d is empty", i)
		}
		if !Known(l[len(l)-1]) {
			t.Errorf("look %d ends in %+q, which is not in All", i, l[len(l)-1])
		}
		for _, c := range l[:len(l)-1] {
			if Known(c) {
				t.Errorf("look %d offers %+q before its last candidate, but it is a plain glyph", i, c)
			}
		}
		if seen[l[0]] {
			t.Errorf("look %d repeats %+q", i, l[0])
		}
		seen[l[0]] = true
	}
	if len(All)+len(Looks) > 1<<16 {
		t.Fatal("looks overflow the frame's uint16 glyph index")
	}
}

func TestColonistLooks(t *testing.T) {
	cases := []struct {
		p    sim.Profile
		want Look
	}{
		{sim.Profile{Gender: sim.GenderMan, Age: 30, SkinTone: sim.SkinDark, HairColor: sim.HairRed},
			Look{"👨🏿‍🦰", "👨🏿", "👨‍🦰", "👨"}},
		{sim.Profile{Gender: sim.GenderWoman, Age: 30, SkinTone: sim.SkinLight, HairColor: sim.HairBrown},
			Look{"👩🏻", "👩"}},
		{sim.Profile{Gender: sim.GenderWoman, Age: 30, SkinTone: sim.SkinMedium, HairColor: sim.HairBlonde},
			Look{"👱🏽‍♀️", "👩🏽", "👱‍♀️", "👩"}},
		{sim.Profile{Gender: sim.Gender(99), Age: 30, SkinTone: sim.SkinMediumDark, HairColor: sim.HairBald},
			Look{"🧑🏾‍🦲", "🧑🏾", "🧑‍🦲", "🧑"}},
		// Seniors have no hair components: tone only.
		{sim.Profile{Gender: sim.GenderMan, Age: 70, SkinTone: sim.SkinMediumLight, HairColor: sim.HairWhite},
			Look{"👴🏼", "👴"}},
	}
	for _, c := range cases {
		got := ForColonistLook(&c.p)
		if len(got) != len(c.want) {
			t.Errorf("%+v: got %+q, want %+q", c.p, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%+v: got %+q, want %+q", c.p, got, c.want)
				break
			}
		}
	}
}

// A look replaces only the resting figure: a state glyph, a mutant and a
// creature keep theirs.
func TestLooksOnlyReplaceTheRestingFigure(t *testing.T) {
	p := &sim.Profile{Gender: sim.GenderMan, Age: 30, SkinTone: sim.SkinDark, HairColor: sim.HairRed}
	resting := sim.EntityView{Kind: sim.Colonist, State: sim.Idle, Profile: p}
	i, ok := LookIndex(resting)
	if !ok || i < len(All) || Looks[i-len(All)][0] != "👨🏿‍🦰" {
		t.Errorf("resting colonist: index %d, ok %v", i, ok)
	}
	if ForEntityLook(resting) == nil {
		t.Error("resting colonist has no look")
	}

	mutant := &sim.Profile{Gender: sim.GenderMan, Age: 30, Traits: []sim.Trait{sim.TraitMutant}}
	for _, e := range []sim.EntityView{
		{Kind: sim.Colonist, State: sim.Fleeing, Profile: p},
		{Kind: sim.Colonist, Profile: mutant},
		{Kind: sim.Colonist},
		{Kind: sim.Cat},
	} {
		if _, ok := LookIndex(e); ok {
			t.Errorf("%+v has a look index", e)
		}
		if ForEntityLook(e) != nil {
			t.Errorf("%+v has a look", e)
		}
	}
}

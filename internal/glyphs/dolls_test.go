package glyphs

import (
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// Every key names one combination of digits, and unpacking gives them back:
// the page reads a key with the same layout (docs/body-sprites.md).
func TestDollKeysAreDenseAndUnique(t *testing.T) {
	seen := make([]bool, NumDolls)
	parts := 1 << len(DollParts)
	for f := range DollFigures {
		for tone := range DollTones {
			for h := range DollHairs {
				for bits := range parts {
					k := dollKey(f, sim.SkinTone(tone), sim.HairColor(h), bits)
					if k < 0 || k >= NumDolls || seen[k] {
						t.Fatalf("key %d for %d/%d/%d/%d out of range or repeated", k, f, tone, h, bits)
					}
					seen[k] = true
					if k%parts != bits || k/parts%DollHairs != h || k/parts/DollHairs%DollTones != tone || k/parts/DollHairs/DollTones != f {
						t.Fatalf("key %d does not unpack to %d/%d/%d/%d", k, f, tone, h, bits)
					}
				}
			}
		}
	}
	if len(All)+len(Looks)+NumDolls+1000 > 0xFFFF {
		t.Fatal("doll keys no longer fit a frame's uint16 beside the looks and a pack")
	}
}

// A mutant at rest draws as their own doll, with the parts they grew; a
// mutant drawn by their state, and anyone who is not a mutant, has none.
func TestDollKeyIsAMutantAtRest(t *testing.T) {
	mutant := &sim.Profile{Gender: sim.GenderWoman, Age: 70, SkinTone: sim.SkinMedium, HairColor: sim.HairRed,
		Traits: []sim.Trait{sim.TraitMutant}}
	e := sim.EntityView{Kind: sim.Colonist, State: sim.Idle, Profile: mutant}
	e.MaxParts[sim.Head] = 10
	e.MaxParts[sim.Tail] = 3
	k, ok := DollKey(e)
	if !ok {
		t.Fatal("a mutant at rest has no doll")
	}
	want := dollKey(4, sim.SkinMedium, sim.HairRed, 1<<2) // old woman, tail only
	if k != want {
		t.Errorf("key %d, want %d", k, want)
	}
	if r, ok := RestingDollKey(sim.EntityView{Kind: sim.Colonist, State: sim.Fleeing, Profile: mutant, MaxParts: e.MaxParts}); !ok || r != k {
		t.Errorf("resting doll of a fleeing mutant %d %v, want %d", r, ok, k)
	}

	plain := &sim.Profile{Gender: sim.GenderMan, Age: 30}
	for _, e := range []sim.EntityView{
		{Kind: sim.Colonist, State: sim.Fleeing, Profile: mutant},
		{Kind: sim.Colonist, Profile: plain},
		{Kind: sim.Colonist},
		{Kind: sim.Alien},
	} {
		if _, ok := DollKey(e); ok {
			t.Errorf("%+v has a doll", e)
		}
	}
}

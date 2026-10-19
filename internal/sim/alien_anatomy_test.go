package sim

import (
	"strings"
	"testing"
)

// Tusks and mouthparts roll on their own stream: the rest of a species'
// anatomy is what rollAnatomy alone gives, so adding them moved no horn,
// shell or claw.
func TestHeadFeaturesDoNotShiftOtherAnatomy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 10
	for seed := int64(1); seed <= 20; seed++ {
		cfg.Seed = seed
		roster := rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg)
		rng := newRand(cfg.Seed ^ alienAnatomySeed)
		for _, sp := range roster {
			want := rollAnatomy(rng, sp)
			got := sp.Anatomy
			got.Tusks, got.Mouth = 0, MouthPlain
			if got != want {
				t.Fatalf("seed %d %s: anatomy %+v, want %+v plus head features", seed, sp.Singular, sp.Anatomy, want)
			}
		}
	}
}

// Across many rolls every tusk grade and every mouthpart turns up, each is
// in the description, and the scientific names draw on their word parts.
func TestHeadFeaturesAreRolledAndNamed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AlienSpeciesCount = 10
	tusks := map[int]int{}
	mouths := map[Mouthpart]int{}
	billNamed, total := 0, 0
	for seed := int64(1); seed <= 100; seed++ {
		cfg.Seed = seed
		for _, sp := range rollAlienSpeciesRoster(newRand(cfg.Seed^alienLoreSeed), cfg) {
			total++
			tusks[sp.Anatomy.Tusks]++
			mouths[sp.Anatomy.Mouth]++
			desc := sp.Description()
			if sp.Anatomy.Tusks > 0 && !strings.Contains(desc, "tusks") {
				t.Errorf("%s has tusks but its description does not say so: %s", sp.Singular, desc)
			}
			if m := sp.Anatomy.Mouth; m != MouthPlain && !strings.Contains(desc, m.String()) {
				t.Errorf("%s has a %v but its description does not say so: %s", sp.Singular, m, desc)
			}
			if sp.Anatomy.Mouth == MouthBill {
				for _, part := range []string{"rhynch", "platy", "anas", "anatin", "latirostr", "rostrat"} {
					if strings.Contains(strings.ToLower(sp.ScientificName), part) {
						billNamed++
						break
					}
				}
			}
		}
	}
	for g := 1; g <= 3; g++ {
		if tusks[g] == 0 {
			t.Errorf("no species rolled tusks of grade %d in %d", g, total)
		}
	}
	for _, m := range mouthparts {
		if mouths[m] == 0 {
			t.Errorf("no species rolled a %v in %d", m, total)
		}
	}
	if mouths[MouthPlain] < total/2 {
		t.Errorf("only %d of %d species have a plain mouth: mouthparts should be a find", mouths[MouthPlain], total)
	}
	if billNamed == 0 {
		t.Errorf("no billed species' scientific name uses a bill word part (%d billed)", mouths[MouthBill])
	}
}

// A young form has its species' mouth but lesser tusks; a big caste pushes
// the tusks further.
func TestHeadFeaturesGrowWithTheBody(t *testing.T) {
	adult := AlienAnatomy{Tusks: 2, Mouth: MouthBeak}
	young := scaledAnatomy(adult, 1, 3)
	if young.Tusks != 0 || young.Mouth != MouthBeak {
		t.Errorf("a third-grown form: %+v, want no tusks and a beak", young)
	}
	if got := scaledAnatomy(adult, 2, 3); got.Tusks != 1 {
		t.Errorf("a two-thirds-grown form has %d tusks, want 1", got.Tusks)
	}
	if got := pushedAnatomy(adult, 150); got.Tusks != 3 || got.Mouth != MouthBeak {
		t.Errorf("a big caste: %+v, want great tusks and a beak", got)
	}
}

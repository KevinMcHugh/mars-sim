package sim

import (
	"strings"
	"testing"
)

func TestEmbeddedTraitRulesLoaded(t *testing.T) {
	if len(traitRules) == 0 {
		t.Fatal("traits.yaml produced no rules")
	}
	// Order is application order, so the file's order has to survive loading.
	if traitRules[0].Trait != TraitTidy || traitRules[0].Any != TagGore {
		t.Errorf("first rule = %+v, want Tidy reacting to gore as traits.yaml declares", traitRules[0])
	}
	var sawWear, sawAll bool
	for _, r := range traitRules {
		if r.Trait == TraitResilient && r.WearRate == 40 {
			sawWear = true
		}
		if r.Trait == TraitExtrovert && r.All == TagSocialLoss|TagFriend {
			sawAll = true
		}
	}
	if !sawWear {
		t.Error("resilient's wear rate did not survive the move to data")
	}
	if !sawAll {
		t.Error("the extrovert rule's all-tags match did not survive the move to data")
	}
}

func TestTraitRulesRejectBadInput(t *testing.T) {
	for _, tc := range []struct{ name, yaml, want string }{
		{"unknown trait", "rules:\n  - trait: brooding\n    grip: 50\n", "not a known trait"},
		{"unknown tag in any", "rules:\n  - trait: tidy\n    any: [vibes]\n    grip: 50\n", "not a known tag"},
		{"unknown tag in none", "rules:\n  - trait: tidy\n    none: [vibes]\n    grip: 50\n", "not a known tag"},
		{"rule with no effect", "rules:\n  - trait: tidy\n    any: [gore]\n", "does nothing"},
		{"unknown field", "rules:\n  - trait: tidy\n    mood: 5\n", "field mood not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := loadTraitRules([]byte(tc.yaml))
			if err == nil {
				t.Fatal("accepted input that should not load")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestFailedTraitLoadLeavesRulesIntact(t *testing.T) {
	before := len(traitRules)
	if err := loadTraitRules([]byte("rules:\n  - trait: brooding\n    grip: 50\n")); err == nil {
		t.Fatal("accepted an unknown trait")
	}
	if len(traitRules) != before {
		t.Fatalf("a failed load left %d rules, want the live %d untouched", len(traitRules), before)
	}
}

func TestEveryTraitHasAUniqueName(t *testing.T) {
	seen := map[string]Trait{}
	for tr := Trait(0); tr < numTraits; tr++ {
		name := traitNames[tr]
		if name == "" {
			t.Errorf("trait %d has no name, so traits.yaml cannot address it", tr)
		}
		if prev, dup := seen[name]; dup {
			t.Errorf("traits %d and %d are both called %q", prev, tr, name)
		}
		seen[name] = tr
	}
}

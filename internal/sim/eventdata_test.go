package sim

import (
	"strings"
	"testing"
)

// The embedded file is the real source of these numbers now, so a spot check
// that it actually reached the tables is worth more than it looks.
func TestEmbeddedEventDefinitionsLoaded(t *testing.T) {
	killed := lifeEventAppraisals[EvtWitnessedColonistKilled]
	if killed.Impact != 95 || killed.Fresh.Grip != 40 || killed.Worn.Grip != -85 {
		t.Errorf("witnessed-colonist-killed = %+v, want the pair from events.yaml", killed)
	}
	if killed.Tags.none(TagDeath | TagGore | TagSocialLoss | TagViolence) {
		t.Errorf("witnessed-colonist-killed tags = %b", killed.Tags)
	}
	if got := stimulusSpecs[EvtSawAlien]; got.Salience != 100 || got.Contribution[FocusFlee] != 500 {
		t.Errorf("saw-alien stimulus = %+v", got)
	}
	if got := lifeEventCollapseText[EvtFinishedMining]; got != "Finished mining." {
		t.Errorf("finished-mining collapses to %q", got)
	}
	if got := lifeEventCollapseText[EvtWitnessedColonistKilled]; got != "" {
		t.Errorf("a killing collapses to %q; runs of them should stay separate memories", got)
	}
}

// Every kind needs a row, and only known names may appear. The array literal
// this replaced got both of those from the compiler.
func TestEventDefinitionsRejectBadInput(t *testing.T) {
	// A file naming every kind, so "missing" is not what fails these cases.
	var complete strings.Builder
	complete.WriteString("events:\n")
	for k := LifeEventKind(0); k < numLifeEventKinds; k++ {
		complete.WriteString("  " + k.String() + ":\n    impact: 1\n")
	}

	for _, tc := range []struct{ name, yaml, want string }{
		{"unknown kind", complete.String() + "  saw-a-ghost:\n    impact: 1\n", "not a known event kind"},
		{"missing kind", "events:\n  ate:\n    impact: 1\n", "no definition for"},
		{"impact out of range", "events:\n  ate:\n    impact: 900\n", "outside [0, 100]"},
		{"unknown tag", "events:\n  ate:\n    tags: [vibes]\n", "not a known tag"},
		{"unknown focus", "events:\n  ate:\n    stimulus:\n      focus: {brooding: 5}\n", "not a known focus"},
		{"unknown field", "events:\n  ate:\n    mood: 5\n", "field mood not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := loadEventDefinitions([]byte(tc.yaml))
			if err == nil {
				t.Fatal("accepted input that should not load")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// A load that fails must not leave the tables half-written.
func TestFailedLoadLeavesTablesIntact(t *testing.T) {
	before := lifeEventAppraisals
	if err := loadEventDefinitions([]byte("events:\n  saw-a-ghost:\n    impact: 1\n")); err == nil {
		t.Fatal("accepted an unknown kind")
	}
	if lifeEventAppraisals != before {
		t.Fatal("a failed load modified the live appraisal table")
	}
}

func TestEveryKindHasAUniqueName(t *testing.T) {
	seen := map[string]LifeEventKind{}
	for k := LifeEventKind(0); k < numLifeEventKinds; k++ {
		name := k.String()
		if name == "" || name == "unknown" {
			t.Errorf("kind %d has no name, so events.yaml cannot address it", k)
		}
		if prev, dup := seen[name]; dup {
			t.Errorf("kinds %d and %d are both called %q", prev, k, name)
		}
		seen[name] = k
	}
}

package sim

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDefaultCognitionRulesCompileAndCoverShippedContent(t *testing.T) {
	def := DefaultCognitionConfig()
	if len(def.Attractors) != len(moodAttractors) {
		t.Fatalf("attractors len = %d, want %d", len(def.Attractors), len(moodAttractors))
	}
	for i, a := range moodAttractors {
		got := def.Attractors[i]
		if got.Kind != a.Kind || got.GoodName != a.GoodName || got.BadName != a.BadName ||
			got.Charge != a.Charge || got.Grip != a.Grip || got.Radius != a.Radius {
			t.Errorf("attractor %d mismatch: got %+v, want %+v", i, got, a)
		}
	}
	if got, want := len(def.Reactions), 25; got != want {
		t.Fatalf("reaction count = %d, want %d migrated reactions", got, want)
	}
	for _, id := range []RuleID{
		"saw-alien", "saw-mouse", "saw-gore", "bitten",
		"witnessed-colonist-killed", "witnessed-colonist-attacked",
		"crushed-mouse", "witnessed-mouse-crushed", "witnessed-cat-catch",
		"killed-alien", "witnessed-alien-killed", "wounded-alien",
		"witnessed-gunfight", "conversation", "ate", "used-toilet", "slept",
		"need-satisfied", "finished-mining", "cleared-rock",
		"finished-construction", "cleaned-refuse", "incinerated-refuse",
		"mutated", "witnessed-mutation",
	} {
		if _, ok := def.reaction(id); !ok {
			t.Errorf("missing shipped reaction %q", id)
		}
	}
	if err := compileCognition(&def); err != nil {
		t.Fatalf("default config did not compile: %v", err)
	}
}

func TestCognitionYAMLRoundTrip(t *testing.T) {
	tmpl := CognitionConfigTemplate()
	if len(tmpl) == 0 {
		t.Fatal("CognitionConfigTemplate returned empty bytes")
	}

	var parsed CognitionConfig
	if err := ApplyCognitionYAML(&parsed, tmpl); err != nil {
		t.Fatalf("ApplyCognitionYAML error: %v", err)
	}
	def := DefaultCognitionConfig()
	if len(parsed.Perceptions) != len(def.Perceptions) ||
		len(parsed.Reactions) != len(def.Reactions) ||
		len(parsed.Modifiers) != len(def.Modifiers) {
		t.Fatalf("round trip rule counts = %d/%d/%d, want %d/%d/%d",
			len(parsed.Perceptions), len(parsed.Reactions), len(parsed.Modifiers),
			len(def.Perceptions), len(def.Reactions), len(def.Modifiers))
	}
	for _, want := range def.Reactions {
		got, ok := parsed.reaction(want.ID)
		if !ok {
			t.Fatalf("round trip lost reaction %s", want.ID)
		}
		if got.Impact != want.Impact || got.Target != want.Target ||
			(got.Stimulus == nil) != (want.Stimulus == nil) {
			t.Errorf("reaction %s mismatch: got %+v, want %+v", want.ID, got, want)
		}
	}
}

func TestApplyCognitionYAMLPartialOverrideAndExtension(t *testing.T) {
	cfg := DefaultCognitionConfig()
	override := []byte(`
schema_version: 1
vocabulary:
  nouns: [toy]
  actions: [play]
  tags: [comfort]
arbitration:
  current_bonus: 42
focuses:
  flee:
    base: 10
    charge_weight: 35
perceptions:
  - id: see-toy-play
    match: { actor_noun: colonist, action: play, object_noun: toy }
    sense: { channel: sight, role: witness, distance: 5, cadence: instant }
reactions:
  - id: saw-toy-play
    match: { actor_noun: colonist, action: play, object_noun: toy, channel: sight, role: witness, phase: instant }
    tags: [comfort]
    memory: { record: true, text: "Watched {actor} play." }
    affect: { impact: 12, target: { charge: 1, grip: 3, valence: 2 } }
`)
	if err := ApplyCognitionYAML(&cfg, override); err != nil {
		t.Fatalf("ApplyCognitionYAML failed: %v", err)
	}
	if cfg.Arbitration.CurrentBonus != 42 || cfg.Arbitration.SwitchMargin != 10 {
		t.Fatalf("arbitration overlay = %+v", cfg.Arbitration)
	}
	if cfg.Focuses[FocusFlee].Base != 10 || cfg.Focuses[FocusFlee].ChargeWeight != 35 ||
		cfg.Focuses[FocusFlee].GripWeight != -40 {
		t.Fatalf("focus field-wise overlay = %+v", cfg.Focuses[FocusFlee])
	}
	if !containsNoun(cfg.Nouns, "toy") || !containsAction(cfg.Actions, "play") {
		t.Fatal("vocabulary extension was not retained")
	}
	if r, ok := cfg.reaction("saw-toy-play"); !ok || r.Target.Grip != 3 {
		t.Fatalf("custom reaction = %+v, present=%v", r, ok)
	}
	var exported struct {
		Vocabulary struct {
			Nouns []string `json:"nouns"`
		} `json:"vocabulary"`
	}
	if err := json.Unmarshal(CognitionVocabularyJSON(cfg), &exported); err != nil {
		t.Fatalf("custom vocabulary JSON: %v", err)
	}
	foundToy := false
	for _, noun := range exported.Vocabulary.Nouns {
		foundToy = foundToy || noun == "toy"
	}
	if !foundToy {
		t.Fatal("loaded vocabulary export omitted custom noun toy")
	}
}

func TestCognitionYAMLRejectsUnknownAndAmbiguousRules(t *testing.T) {
	cfg := DefaultCognitionConfig()
	if err := ApplyCognitionYAML(&cfg, []byte("mystery: true\n")); err == nil {
		t.Fatal("unknown top-level key was accepted")
	}

	cfg = DefaultCognitionConfig()
	err := ApplyCognitionYAML(&cfg, []byte(`
reactions:
  - id: ambiguous-sighting
    match: { actor_noun: alien, action: present, channel: sight, role: witness, phase: enter }
    memory: { record: true }
    affect: { impact: 1, target: { charge: 0, grip: 0, valence: 0 } }
`))
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous rule error = %v", err)
	}
}

func TestCognitionVocabularyJSON(t *testing.T) {
	var decoded struct {
		SchemaVersion int `json:"schema_version"`
		Vocabulary    struct {
			Nouns   []string `json:"nouns"`
			Actions []string `json:"actions"`
		} `json:"vocabulary"`
		Schema map[string][]string `json:"schema"`
	}
	if err := json.Unmarshal(CognitionVocabularyJSON(), &decoded); err != nil {
		t.Fatalf("vocabulary JSON: %v", err)
	}
	if decoded.SchemaVersion != 1 || len(decoded.Vocabulary.Nouns) == 0 ||
		len(decoded.Vocabulary.Actions) == 0 || len(decoded.Schema["reaction"]) == 0 {
		t.Fatalf("incomplete vocabulary export: %+v", decoded)
	}
}

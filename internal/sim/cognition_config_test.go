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

func TestDefaultReactionFreshWornParity(t *testing.T) {
	want := map[RuleID][2]MoodVector{
		"saw-alien":                   {{8, -10, -15}, {5, -22, -22}},
		"saw-mouse":                   {{2, -3, 0}, {1, -1, 0}},
		"saw-gore":                    {{-3, -7, -6}, {-6, -14, -10}},
		"bitten":                      {{55, -44, -30}, {35, -80, -55}},
		"witnessed-colonist-killed":   {{70, 40, -60}, {20, -85, -85}},
		"witnessed-colonist-attacked": {{45, 25, -40}, {25, -70, -60}},
		"crushed-mouse":               {{-1, 2, 0}, {-2, 0, 0}},
		"witnessed-mouse-crushed":     {{-1, -2, -1}, {-1, -1, 0}},
		"witnessed-cat-catch":         {{1, 1, 0}, {}},
		"killed-alien":                {{45, 52, 35}, {25, 20, 8}},
		"witnessed-alien-killed":      {{5, 6, 4}, {2, 2, 0}},
		"wounded-alien":               {{4, 5, 2}, {2, 2, 0}},
		"witnessed-gunfight":          {{35, -25, -20}, {18, -45, -35}},
		"conversation":                {{}, {}},
		"ate":                         {{4, 2, 1}, {2, -1, 0}},
		"used-toilet":                 {{1, 2, 0}, {}},
		"slept":                       {{15, 2, 2}, {12, -2, 0}},
		"need-satisfied":              {{2, 2, 0}, {1, 0, 0}},
		"finished-mining":             {{-1, 5, 0}, {-5, -3, 0}},
		"cleared-rock":                {{-1, 5, 0}, {-5, -3, 0}},
		"finished-construction":       {{-1, 6, 3}, {-4, 0, 0}},
		"cleaned-refuse":              {{-1, 5, 0}, {-5, -4, 0}},
		"incinerated-refuse":          {{-1, 7, 1}, {-4, 0, 0}},
		"mutated":                     {{18, -70, -35}, {10, -90, -70}},
		"witnessed-mutation":          {{2, -8, -10}, {1, -16, -18}},
	}
	cfg := DefaultCognitionConfig()
	for id, vectors := range want {
		reaction, ok := cfg.reaction(id)
		if !ok {
			t.Errorf("missing reaction %s", id)
			continue
		}
		if reaction.Fresh != vectors[0] || reaction.Worn != vectors[1] {
			t.Errorf("%s vectors = %+v/%+v, want %+v/%+v", id,
				reaction.Fresh, reaction.Worn, vectors[0], vectors[1])
		}
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
		len(parsed.TraitRules) != len(def.TraitRules) {
		t.Fatalf("round trip rule counts = %d/%d/%d, want %d/%d/%d",
			len(parsed.Perceptions), len(parsed.Reactions), len(parsed.TraitRules),
			len(def.Perceptions), len(def.Reactions), len(def.TraitRules))
	}
	for _, want := range def.Reactions {
		got, ok := parsed.reaction(want.ID)
		if !ok {
			t.Fatalf("round trip lost reaction %s", want.ID)
		}
		if got.Impact != want.Impact || got.Fresh != want.Fresh || got.Worn != want.Worn ||
			got.WearPolicy != want.WearPolicy ||
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
    memory: { record: true, text: "Watched {actor} play." }
    affect: { impact: 12, fresh: { charge: 1, grip: 3, valence: 2 }, worn: { charge: 0, grip: 1, valence: 0 } }
    wear_policy: memory-occasions
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
	if r, ok := cfg.reaction("saw-toy-play"); !ok || r.Fresh.Grip != 3 {
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
    affect: { impact: 1, fresh: { charge: 0, grip: 0, valence: 0 }, worn: { charge: 0, grip: 0, valence: 0 } }
    wear_policy: memory-occasions
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

func TestCognitionYAMLStrictWearTraitsAndGrammar(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"unknown wear policy", `
reactions:
  - id: conversation
    wear_policy: forever
`, "unknown wear policy"},
		{"removed tags field", `
reactions:
  - id: conversation
    tags: [social]
`, "field tags not found"},
		{"no-op trait rule", `
trait_rules:
  - id: no-op
    trait: tidy
    scales: {}
`, "no effect"},
		{"unknown relation", `
trait_rules:
  - id: strange-relation
    trait: tidy
    match: { object_relation: enemy }
    scales: { charge: 120 }
`, "unknown object relation"},
		{"missing trait", `
trait_rules:
  - id: missing-trait
    scales: { charge: 120 }
`, "trait is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultCognitionConfig()
			before := len(cfg.TraitRules)
			err := ApplyCognitionYAML(&cfg, []byte(tc.yaml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want containing %q", err, tc.want)
			}
			if len(cfg.TraitRules) != before {
				t.Fatal("failed load mutated live configuration")
			}
		})
	}
}

func TestCognitionIncludesEscapeAndNoTagsInVocabulary(t *testing.T) {
	cfg := DefaultCognitionConfig()
	if cfg.Focuses[FocusEscape].Name != "escape" {
		t.Fatalf("FocusEscape config = %+v", cfg.Focuses[FocusEscape])
	}
	data := string(CognitionVocabularyJSON(cfg))
	for _, removed := range []string{`"tags"`, "any_tags", "all_tags", "not_tags"} {
		if strings.Contains(data, removed) {
			t.Fatalf("vocabulary still exposes removed tag grammar %q", removed)
		}
	}
	if !strings.Contains(data, "object_relations") || !strings.Contains(data, "memory-occasions") {
		t.Fatalf("vocabulary omitted relation or wear policy: %s", data)
	}
}

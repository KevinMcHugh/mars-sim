package wire

import (
	"encoding/json"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The config topic lists every knob once, grouped under mars-sim.yaml's
// sections, with the game's value beside the default.
func TestConfigTopicListsEveryKnob(t *testing.T) {
	cfg := sim.DefaultConfig()
	cfg.StartColonists = 42
	cfg.FogOfWar = !cfg.FogOfWar
	snap := &sim.Snapshot{Seed: 7, Config: &cfg}

	b, err := json.Marshal(configTopic(snap))
	if err != nil {
		t.Fatal(err)
	}
	var got ConfigTopic
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Seed != 7 {
		t.Errorf("seed = %d, want 7", got.Seed)
	}
	if got.Sections[0].Title != "World" {
		t.Errorf("first section = %q, want World", got.Sections[0].Title)
	}
	seen := map[string]ConfigSetting{}
	for _, sec := range got.Sections {
		if sec.Title == "" {
			t.Errorf("untitled section before %q", sec.Settings[0].Key)
		}
		for _, s := range sec.Settings {
			if _, dup := seen[s.Key]; dup {
				t.Errorf("%q listed twice", s.Key)
			}
			seen[s.Key] = s
		}
	}
	def := sim.DefaultConfig()
	if n := len(sim.Knobs(&def)); len(seen) != n {
		t.Errorf("listed %d settings, want every knob (%d)", len(seen), n)
	}
	// Through JSON, numbers come back as float64.
	if c := seen["colonists"]; c.Value != float64(42) || c.Default != float64(def.StartColonists) {
		t.Errorf("colonists = %v (default %v), want 42 (default %d)", c.Value, c.Default, def.StartColonists)
	}
	if f := seen["fog-of-war"]; f.Value != !def.FogOfWar || f.Default != def.FogOfWar {
		t.Errorf("fog-of-war = %v (default %v)", f.Value, f.Default)
	}
}

// A snapshot without a Config (a test fixture) sends an empty list, not a
// panic.
func TestConfigTopicWithoutConfig(t *testing.T) {
	got := configTopic(&sim.Snapshot{Seed: 3}).(ConfigTopic)
	if got.Seed != 3 || len(got.Sections) != 0 {
		t.Errorf("got %+v", got)
	}
}

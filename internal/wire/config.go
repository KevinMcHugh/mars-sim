package wire

import (
	"sync"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// ConfigTopic is the Game tab's settings list: every tunable the running game
// was started with, grouped by the sections mars-sim.yaml uses, each beside
// its compiled default. It is read-only: the page can set only the few the
// new-game form offers (docs/game-settings.md).
type ConfigTopic struct {
	Seed     int64           `json:"seed"`
	Sections []ConfigSection `json:"sections"`
}

// ConfigSection is one group of settings, titled as in mars-sim.yaml.
type ConfigSection struct {
	Title    string          `json:"title"`
	Settings []ConfigSetting `json:"settings"`
}

// ConfigSetting is one knob: its mars-sim.yaml key, its description, and its
// value in this game and by default (a number or a bool).
type ConfigSetting struct {
	Key     string `json:"key"`
	Doc     string `json:"doc"`
	Value   any    `json:"value"`
	Default any    `json:"default"`
}

// defaultKnobs is DefaultConfig's knob values by key, built once: the
// defaults are compiled in, so they never change while the program runs.
var defaultKnobs = sync.OnceValue(func() map[string]any {
	def := sim.DefaultConfig()
	m := map[string]any{}
	for _, k := range sim.Knobs(&def) {
		m[k.Key] = knobValue(k)
	}
	return m
})

func configTopic(s *sim.Snapshot) any {
	t := ConfigTopic{Seed: s.Seed, Sections: []ConfigSection{}}
	if s.Config == nil {
		return t
	}
	// Knobs wants a pointer it may hand out; give it a copy so nothing here
	// can reach the World's own Config.
	cfg := *s.Config
	defs := defaultKnobs()
	for _, k := range sim.Knobs(&cfg) {
		if k.Section != "" || len(t.Sections) == 0 {
			t.Sections = append(t.Sections, ConfigSection{Title: k.Section, Settings: []ConfigSetting{}})
		}
		sec := &t.Sections[len(t.Sections)-1]
		sec.Settings = append(sec.Settings, ConfigSetting{Key: k.Key, Doc: k.Doc, Value: knobValue(k), Default: defs[k.Key]})
	}
	return t
}

// knobValue reads the scalar a Knob points at.
func knobValue(k sim.Knob) any {
	switch p := k.Ptr.(type) {
	case *int:
		return *p
	case *int64:
		return *p
	case *bool:
		return *p
	}
	return nil
}

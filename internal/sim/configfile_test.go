package sim

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// settingLine matches a commented-out setting in the template ("# width: 80",
// "#     rise: 2") but not the prose above it ("## world width in tiles") or a
// section rule.
var settingLine = regexp.MustCompile(`^# (\s*[a-z][a-z0-9-]*:.*)$`)

// uncommentTemplate switches on every setting in a generated template, which is
// what a player does by hand to one line at a time.
func uncommentTemplate(t *testing.T, data []byte) []byte {
	t.Helper()
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if m := settingLine.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}

// TestTemplateRoundTrip is the guarantee the whole scheme rests on: the
// generated template holds every knob, at its default, spelled the way the
// loader reads it. Perturbing every field first means a knob missing from the
// template shows up as a difference rather than passing by luck.
func TestTemplateRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	for _, k := range Knobs(&cfg) {
		switch p := k.Ptr.(type) {
		case *int:
			*p += 7
		case *int64:
			*p += 7
		case *bool:
			*p = !*p
		default:
			t.Fatalf("knob %q has unsupported type %T", k.Name, k.Ptr)
		}
	}

	set, err := ApplyConfigFile(&cfg, uncommentTemplate(t, ConfigTemplate()), "template")
	if err != nil {
		t.Fatalf("applying the uncommented template: %v", err)
	}

	want := DefaultConfig()
	want.Seed = cfg.Seed // the template's seed is 0, meaning "keep the random one"
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("uncommented template did not reproduce DefaultConfig\n got %+v\nwant %+v", cfg, want)
	}
	if len(set) != len(ConfigKeys()) {
		t.Errorf("template set %d settings, want all %d", len(set), len(ConfigKeys()))
	}
}

// TestTemplateChangesNothing: the file as generated is inert. That is what
// makes it safe to commit and regenerate.
func TestTemplateChangesNothing(t *testing.T) {
	cfg := DefaultConfig()
	want := cfg
	set, err := ApplyConfigFile(&cfg, ConfigTemplate(), ConfigFileName)
	if err != nil {
		t.Fatalf("applying the commented template: %v", err)
	}
	if len(set) != 0 {
		t.Errorf("commented template set %v, want nothing", set)
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Error("commented template changed the config")
	}
}

func TestApplyConfigFile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 1
	data := []byte(`
seed: 99
colonists: 12
needs:
  food:
    rise: 5
    fatal: false
`)
	set, err := ApplyConfigFile(&cfg, data, ConfigFileName)
	if err != nil {
		t.Fatalf("ApplyConfigFile: %v", err)
	}
	if cfg.Seed != 99 {
		t.Errorf("Seed = %d, want 99", cfg.Seed)
	}
	if cfg.StartColonists != 12 {
		t.Errorf("StartColonists = %d, want 12", cfg.StartColonists)
	}
	if cfg.Needs[NeedFood].Rise != 5 || cfg.Needs[NeedFood].Fatal {
		t.Errorf("food need = %+v, want rise 5 and not fatal", cfg.Needs[NeedFood])
	}
	if cfg.Needs[NeedFood].SeekAt != DefaultConfig().Needs[NeedFood].SeekAt {
		t.Error("an untouched field inside a need was overwritten")
	}
	want := []string{"seed", "colonists", "needs.food.rise", "needs.food.fatal"}
	if !reflect.DeepEqual(set, want) {
		t.Errorf("set = %v, want %v", set, want)
	}
}

// A zero seed in the file means the same as -seed 0: keep rolling a fresh one.
func TestApplyConfigFileZeroSeedKeepsRandom(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed = 4242
	if _, err := ApplyConfigFile(&cfg, []byte("seed: 0\n"), ConfigFileName); err != nil {
		t.Fatalf("ApplyConfigFile: %v", err)
	}
	if cfg.Seed != 4242 {
		t.Errorf("Seed = %d, want the default 4242 left alone", cfg.Seed)
	}
}

func TestApplyConfigFileErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string // substring the message must carry
	}{
		{"unknown key", "colonits: 3\n", `unknown setting "colonits"`},
		{"unknown nested key", "needs:\n  food:\n    risé: 3\n", `unknown setting "needs.food.risé"`},
		{"unknown section", "creatures:\n  goat: 3\n", `unknown setting "creatures"`},
		{"not a number", "colonists: many\n", "want a whole number"},
		{"not a bool", "needs:\n  food:\n    fatal: 3\n", "want true or false"},
		{"duplicate", "colonists: 3\ncolonists: 4\n", "set twice"},
		{"list at top level", "- colonists\n", "want a mapping"},
		{"scalar where a spec belongs", "needs: 3\n", `unknown setting "needs"`},
		{"line number", "\n\ncolonits: 3\n", ":3:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			_, err := ApplyConfigFile(&cfg, []byte(tc.yaml), ConfigFileName)
			if err == nil {
				t.Fatalf("no error for %q", tc.yaml)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// Every knob needs a unique name on both surfaces and a line of documentation:
// the template and -h both print the doc, and a duplicate name would make
// flag.Parse panic at startup.
func TestKnobsAreWellFormed(t *testing.T) {
	cfg := DefaultConfig()
	names := map[string]bool{}
	keys := map[string]bool{}
	for _, k := range Knobs(&cfg) {
		if k.Doc == "" {
			t.Errorf("knob %q has no doc tag", k.Name)
		}
		if names[k.Name] {
			t.Errorf("duplicate flag name %q", k.Name)
		}
		if keys[k.Key] {
			t.Errorf("duplicate config key %q", k.Key)
		}
		names[k.Name], keys[k.Key] = true, true
		if k.Name == SeedKey || k.Key == SeedKey {
			t.Errorf("knob %q collides with the special seed setting", k.Name)
		}
	}
	if !names["colonists"] || !names["need-food-rise"] {
		t.Error("expected knobs are missing; did the cfg tags move?")
	}
}

// Knobs must point into the Config they were asked about, not a copy.
func TestKnobsPointAtTheConfig(t *testing.T) {
	cfg := DefaultConfig()
	for _, k := range Knobs(&cfg) {
		if k.Key != "needs.food.rise" {
			continue
		}
		*(k.Ptr.(*int)) = 123
		if cfg.Needs[NeedFood].Rise != 123 {
			t.Fatal("writing through a knob did not reach the config")
		}
		return
	}
	t.Fatal("needs.food.rise knob not found")
}

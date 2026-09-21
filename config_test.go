package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// settingLine matches a setting in the committed file whether or not it is
// still commented out.
var settingLine = regexp.MustCompile(`^# (\s*[a-z][a-z0-9-]*:.*)$`)

// liveSettings switches on every commented-out setting in the committed file,
// so the checks below see the whole set regardless of which lines a player has
// actually turned on.
func liveSettings(t *testing.T, data []byte) []byte {
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

func readCommittedSettings(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(sim.ConfigFileName)
	if err != nil {
		t.Fatalf("reading the committed settings file: %v", err)
	}
	return data
}

// The committed settings file is the one place a player looks for the full set
// of knobs, so adding a tunable without adding it here is a bug — the same
// bug the flag list used to have. The fix is one command, and the failure says
// so.
func TestCommittedSettingsFileCoversEverySetting(t *testing.T) {
	cfg := sim.DefaultConfig()
	present, err := sim.ApplyConfigFile(&cfg, liveSettings(t, readCommittedSettings(t)), sim.ConfigFileName)
	if err != nil {
		t.Fatalf("%s does not load: %v", sim.ConfigFileName, err)
	}
	sort.Strings(present)

	want := sim.ConfigKeys()
	if reflect.DeepEqual(present, want) {
		return
	}
	var missing, extra []string
	have := map[string]bool{}
	for _, k := range present {
		have[k] = true
	}
	known := map[string]bool{}
	for _, k := range want {
		known[k] = true
		if !have[k] {
			missing = append(missing, k)
		}
	}
	for _, k := range present {
		if !known[k] {
			extra = append(extra, k)
		}
	}
	t.Errorf("%s is out of date: missing %v, no longer a setting %v\n"+
		"regenerate it with: go run . -print-config > %s   (then re-apply any values you had set)",
		sim.ConfigFileName, missing, extra, sim.ConfigFileName)
}

// Committed as generated, the file must be inert: it documents the defaults
// until someone deliberately uncomments something.
func TestCommittedSettingsFileLoads(t *testing.T) {
	cfg := sim.DefaultConfig()
	if _, err := sim.ApplyConfigFile(&cfg, readCommittedSettings(t), sim.ConfigFileName); err != nil {
		t.Fatalf("%s does not load: %v", sim.ConfigFileName, err)
	}
	if err := validateConfig(cfg); err != nil {
		t.Errorf("%s produces a config the game rejects: %v", sim.ConfigFileName, err)
	}
}

func TestValidateFocusConfig(t *testing.T) {
	cfg := sim.DefaultConfig()
	cfg.FocusSwitchMargin = -1
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "focus") {
		t.Fatalf("negative focus margin error = %v, want focus validation", err)
	}

	cfg = sim.DefaultConfig()
	cfg.Focuses[sim.FocusWork].DistanceWeight = -1
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "focus-work-distance-weight") {
		t.Fatalf("negative work distance error = %v, want focus-work validation", err)
	}
}

func TestValidateAffectConfig(t *testing.T) {
	for _, alter := range []func(*sim.Config){
		func(cfg *sim.Config) { cfg.MoodChargeDecayPerTick = -1 },
		func(cfg *sim.Config) { cfg.MoodGripDecayPerTick = -1 },
		func(cfg *sim.Config) { cfg.MoodLabelSwitchMargin = -1 },
	} {
		cfg := sim.DefaultConfig()
		alter(&cfg)
		if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "affect") {
			t.Fatalf("invalid affect config error = %v, want affect validation", err)
		}
	}
}

func TestValidateActiveStimulusLimit(t *testing.T) {
	for _, limit := range []int{0, sim.MaxActiveStimuli + 1} {
		cfg := sim.DefaultConfig()
		cfg.ActiveStimulusLimit = limit
		if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "active-stimulus-limit") {
			t.Fatalf("limit %d error = %v, want active-stimulus-limit validation", limit, err)
		}
	}
}

func TestValidateNeedCriticalThreshold(t *testing.T) {
	for _, alter := range []func(*sim.NeedSpec){
		func(spec *sim.NeedSpec) { spec.CriticalAt = spec.SeekAt - 1 },
		func(spec *sim.NeedSpec) { spec.CriticalAt = spec.Max + 1 },
	} {
		cfg := sim.DefaultConfig()
		alter(&cfg.Needs[sim.NeedFood])
		if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "critical-at") {
			t.Fatalf("invalid critical threshold error = %v, want critical-at validation", err)
		}
	}
}

// The layering the whole feature exists for: file over defaults, flags over
// file.
func TestFlagsOverrideTheSettingsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, sim.ConfigFileName)
	settings := "colonists: 12\nmice: 3\nneeds:\n  food:\n    rise: 9\n"
	if err := os.WriteFile(path, []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := sim.DefaultConfig()
	if err := loadConfigFile(&cfg, path, true); err != nil {
		t.Fatalf("loadConfigFile: %v", err)
	}
	if cfg.StartColonists != 12 || cfg.StartMice != 3 || cfg.Needs[sim.NeedFood].Rise != 9 {
		t.Fatalf("settings file not applied: %d colonists, %d mice, food rise %d",
			cfg.StartColonists, cfg.StartMice, cfg.Needs[sim.NeedFood].Rise)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	bindConfigFlags(fs, &cfg)
	if err := fs.Parse([]string{"-colonists", "20", "-need-food-rise", "1"}); err != nil {
		t.Fatalf("parsing flags: %v", err)
	}
	if cfg.StartColonists != 20 {
		t.Errorf("colonists = %d, want the flag's 20", cfg.StartColonists)
	}
	if cfg.Needs[sim.NeedFood].Rise != 1 {
		t.Errorf("food rise = %d, want the flag's 1", cfg.Needs[sim.NeedFood].Rise)
	}
	if cfg.StartMice != 3 {
		t.Errorf("mice = %d, want the file's 3 (no flag passed)", cfg.StartMice)
	}
	if cfg.StartAliens != sim.DefaultConfig().StartAliens {
		t.Errorf("aliens = %d, want the compiled-in default", cfg.StartAliens)
	}
}

// A settings file is optional, but one the player named and misspelled is not
// something to shrug off: they would get a game that ignores their settings.
func TestLoadConfigFileMissing(t *testing.T) {
	cfg := sim.DefaultConfig()
	missing := filepath.Join(t.TempDir(), "nope.yaml")

	if err := loadConfigFile(&cfg, missing, false); err != nil {
		t.Errorf("an absent default file should be fine, got %v", err)
	}
	if err := loadConfigFile(&cfg, missing, true); err == nil {
		t.Error("an absent -config file should be an error")
	}
	if err := loadConfigFile(&cfg, "", false); err != nil {
		t.Errorf(`-config "" should read nothing, got %v`, err)
	}
}

func TestConfigPathFromArgs(t *testing.T) {
	cases := []struct {
		args     []string
		path     string
		explicit bool
	}{
		{nil, sim.ConfigFileName, false},
		{[]string{"-colonists", "9"}, sim.ConfigFileName, false},
		{[]string{"-config", "balance.yaml"}, "balance.yaml", true},
		{[]string{"-config=balance.yaml"}, "balance.yaml", true},
		{[]string{"--config", "balance.yaml"}, "balance.yaml", true},
		{[]string{"--config=balance.yaml"}, "balance.yaml", true},
		{[]string{"-colonists", "9", "-config", "b.yaml", "-mice", "2"}, "b.yaml", true},
		{[]string{"-config", ""}, "", true},
		{[]string{"-config"}, "", true},
	}
	for _, tc := range cases {
		path, explicit := configPathFromArgs(tc.args)
		if path != tc.path || explicit != tc.explicit {
			t.Errorf("configPathFromArgs(%q) = %q, %v; want %q, %v", tc.args, path, explicit, tc.path, tc.explicit)
		}
	}
}

// Binding every knob must not collide with an application flag: flag.FlagSet
// panics on a duplicate, which would be a startup crash rather than a test
// failure if it ever happened.
func TestFlagNamesDoNotCollide(t *testing.T) {
	cfg := sim.DefaultConfig()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for _, name := range []string{"duration", "headless", "seed", "glyphs", "config", "director", "print-config"} {
		fs.String(name, "", "application flag")
	}
	bindConfigFlags(fs, &cfg) // panics on a duplicate name
	if fs.Lookup("colonists") == nil || fs.Lookup("need-food-seek-at") == nil {
		t.Error("expected config flags were not registered")
	}
}

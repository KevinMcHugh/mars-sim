// Command mars-sim runs the Mars colony simulation with a Bubble Tea terminal UI.
//
// Architecture: the simulation runs in its own goroutine (the Engine) and never
// shares mutable state with the UI. Frontends subscribe for immutable Snapshots
// and send Commands back. The Bubble Tea TUI here is one such frontend; another
// (web, GUI, headless test harness) could attach to the same Engine unchanged.
//
// Settings arrive in three layers, each overriding the one before it:
// sim.DefaultConfig, then a mars-sim.yaml settings file in the working
// directory (a committed set of options; see docs/config-file.md), then
// command-line flags. Every tunable in sim.Config carries a `cfg` tag that
// names it once and drives all three, so a new knob gets its flag, its file
// key, and its documentation without a second edit. Run with -h or ? to list
// them, or -print-config to write a fresh settings file.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfg := sim.DefaultConfig()

	// Support "?" as a help alias alongside the flag package's built-in -h/-help.
	for _, a := range os.Args[1:] {
		if a == "?" || a == "-?" || a == "--?" {
			usage()
			return
		}
	}

	// Cognition and settings files are read before any flag is registered, so
	// that the values they set become the flag defaults: flags then override
	// the files for free, and -h prints the defaults this run will actually use.
	cogPath, cogPathGiven := cognitionPathFromArgs(os.Args[1:])
	if err := loadCognitionConfigFile(&cfg, cogPath, cogPathGiven); err != nil {
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		os.Exit(2)
	}

	cfgPath, cfgPathGiven := configPathFromArgs(os.Args[1:])
	if err := loadConfigFile(&cfg, cfgPath, cfgPathGiven); err != nil {
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		os.Exit(2)
	}

	// The director's schedule file loads the same way, before flags: found by
	// hand for the same reason -config is (see below).
	directorPath, directorPathGiven := directorPathFromArgs(os.Args[1:])
	if err := loadDirectorFile(&cfg, directorPath, directorPathGiven); err != nil {
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		os.Exit(2)
	}

	// The alien name pool loads the same way, before flags: found by hand
	// for the same reason -config is (see below). An absent default file
	// leaves cfg.AlienNames empty, which falls back to the built-in pool
	// compiled into the sim package (see lore.go/alien_names.go).
	alienNamesPath, alienNamesPathGiven := alienNamesPathFromArgs(os.Args[1:])
	if err := loadAlienNamesFile(&cfg, alienNamesPath, alienNamesPathGiven); err != nil {
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		os.Exit(2)
	}

	// Application flags (not part of the simulation config).
	var (
		duration             time.Duration
		headless             bool
		seed                 int64
		glyphs               string
		printConfig          bool
		printCognitionConfig bool
		printCognitionVocab  bool
		cpuProfile           string
	)
	flag.DurationVar(&duration, "duration", 0, "auto-exit after this long (0 = run until quit); handy for smoke tests")
	flag.BoolVar(&headless, "headless", false, "run without the TUI, printing periodic stats")
	flag.Int64Var(&seed, "seed", 0, "world seed (0 = random each run)")
	flag.StringVar(&glyphs, "glyphs", glyphModeAuto, "map glyphs: auto (measure the terminal), emoji (trust the width table), or ascii")
	flag.String("config", cfgPath, "settings file to read before the flags (\"\" to ignore any file)")
	flag.String("director", directorPath, "director schedule file to read (\"\" to run with no scheduled occurrences)")
	flag.String("alien-names", alienNamesPath, "alien name pool file to read (\"\" to use the built-in pool)")
	flag.String("cognition", cogPath, "cognition balance file to read before flags (\"\" to ignore any file)")
	flag.BoolVar(&printConfig, "print-config", false, "write a commented settings file with every setting at its default, then exit")
	flag.BoolVar(&printCognitionConfig, "print-cognition-config", false, "write a cognition settings file with every setting at its default, then exit")
	flag.BoolVar(&printCognitionVocab, "print-cognition-vocab", false, "write cognition vocabulary and editor schema as JSON, then exit")
	flag.StringVar(&cpuProfile, "cpuprofile", "", "write a CPU profile of the whole run to this file (read it with go tool pprof)")

	// Simulation config flags, each defaulting to the value the settings file
	// left in place.
	bindConfigFlags(flag.CommandLine, &cfg)

	flag.Usage = usage
	flag.Parse()
	if err := checkNoArgs(flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		fmt.Fprintln(os.Stderr, "run with -h for options")
		os.Exit(2)
	}

	if printConfig {
		os.Stdout.Write(sim.ConfigTemplate())
		return
	}
	if printCognitionConfig {
		os.Stdout.Write(sim.CognitionConfigTemplate())
		return
	}
	if printCognitionVocab {
		os.Stdout.Write(sim.CognitionVocabularyJSON(cfg.Cognition))
		return
	}
	cfg.SyncToCognition()

	if seed != 0 {
		cfg.Seed = seed // otherwise keep DefaultConfig's random, time-based seed
	}
	if glyphs != glyphModeAuto && glyphs != glyphModeEmoji && glyphs != glyphModeASCII {
		fmt.Fprintf(os.Stderr, "mars-sim: -glyphs must be %s, %s or %s (got %q)\n",
			glyphModeAuto, glyphModeEmoji, glyphModeASCII, glyphs)
		os.Exit(2)
	}
	if err := validateConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		fmt.Fprintln(os.Stderr, "run with -h for options")
		os.Exit(2)
	}

	// Profiling starts before NewEngine so world generation is in the profile
	// too; on a big map it is a real share of a short run.
	stopProfile, err := startCPUProfile(cpuProfile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		os.Exit(2)
	}
	defer stopProfile()

	eng := sim.NewEngine(cfg)

	// Subscribe before starting the engine so the very first frame is not missed.
	snaps := eng.Subscribe()

	// Ctrl+C cancels the context rather than killing the process, so a
	// headless run returns through the deferred calls above and a -cpuprofile
	// is flushed rather than left empty. (The TUI reads Ctrl+C as a key and
	// quits on its own.)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go eng.Run(ctx)

	if headless {
		runHeadless(ctx, snaps, cfg, duration)
		return
	}

	setUpGlyphs(glyphs)

	if err := runTUI(eng, snaps, duration); err != nil {
		cancel()
		stopProfile() // os.Exit skips deferred calls
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		os.Exit(1)
	}
}

// startCPUProfile begins writing a CPU profile to path, or does nothing if
// path is empty. The returned stop function flushes and closes the file; it
// is safe to call more than once.
func startCPUProfile(path string) (stop func(), err error) {
	if path == "" {
		return func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("creating CPU profile: %w", err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("starting CPU profile: %w", err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			pprof.StopCPUProfile()
			f.Close()
			fmt.Fprintf(os.Stderr, "mars-sim: CPU profile written to %s (go tool pprof -http=: %s)\n", path, path)
		})
	}, nil
}

// bindConfigFlags registers a flag for every tunable in the simulation config,
// using the passed-in values as the flag defaults.
//
// The flags are derived from the `cfg` struct tags rather than written out by
// hand: the tag already has to exist for the settings file, and a hand-written
// list of ~80 flags beside it is a list that drifts. sim.Knobs hands back a
// pointer into this cfg, so binding is a type switch over the three scalar
// kinds a tunable can be.
func bindConfigFlags(fs *flag.FlagSet, cfg *sim.Config) {
	for _, k := range sim.Knobs(cfg) {
		switch p := k.Ptr.(type) {
		case *int:
			fs.IntVar(p, k.Name, *p, k.Doc)
		case *int64:
			fs.Int64Var(p, k.Name, *p, k.Doc)
		case *bool:
			fs.BoolVar(p, k.Name, *p, k.Doc)
		default:
			// A new field kind needs a case here and in sim.assign; failing
			// loudly at startup beats a tunable that silently has no flag.
			panic(fmt.Sprintf("mars-sim: setting %q has unsupported type %T", k.Name, k.Ptr))
		}
	}
}

// cognitionPathFromArgs finds the -cognition value before the flag package runs.
func cognitionPathFromArgs(args []string) (path string, given bool) {
	for i, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		if name != "-cognition" && name != "--cognition" {
			continue
		}
		if hasValue {
			return value, true
		}
		if i+1 < len(args) {
			return args[i+1], true
		}
		return "", true
	}
	return sim.DefaultCognitionConfigFileName, false
}

// loadCognitionConfigFile applies a cognition balance file to cfg.
func loadCognitionConfigFile(cfg *sim.Config, path string, given bool) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && !given:
		return nil
	case err != nil:
		return fmt.Errorf("reading cognition settings: %w", err)
	}
	if err := sim.ApplyCognitionYAML(&cfg.Cognition, data); err != nil {
		return err
	}
	cfg.SyncWithCognition()
	return nil
}

// configPathFromArgs finds the -config value before the flag package runs.
// Reported separately from its value so an explicitly named file that is
// missing can be an error while the default one simply may not exist.
func configPathFromArgs(args []string) (path string, given bool) {
	for i, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		if name != "-config" && name != "--config" {
			continue
		}
		if hasValue {
			return value, true
		}
		if i+1 < len(args) {
			return args[i+1], true
		}
		return "", true // "-config" with nothing after it: read no file
	}
	return sim.ConfigFileName, false
}

// loadConfigFile applies a settings file to cfg. An absent default file is
// normal — most runs have none — but an absent file the player named, or one
// that does not parse, stops the run rather than quietly playing something
// other than what the file says.
func loadConfigFile(cfg *sim.Config, path string, given bool) error {
	if path == "" {
		return nil // -config "" opts out
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && !given:
		return nil
	case err != nil:
		return fmt.Errorf("reading settings: %w", err)
	}
	if _, err := sim.ApplyConfigFile(cfg, data, path); err != nil {
		return err
	}
	return nil
}

// directorPathFromArgs is configPathFromArgs's counterpart for -director,
// found the same way and for the same reason: the director's schedule file
// must be loaded before flag.Parse runs.
func directorPathFromArgs(args []string) (path string, given bool) {
	for i, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		if name != "-director" && name != "--director" {
			continue
		}
		if hasValue {
			return value, true
		}
		if i+1 < len(args) {
			return args[i+1], true
		}
		return "", true // "-director" with nothing after it: read no file
	}
	return sim.DirectorFileName, false
}

// loadDirectorFile applies a director.yaml schedule file to cfg. An absent
// default file is normal — most runs have no scripted occurrences — but a
// file the player named explicitly, or one that does not parse, stops the
// run rather than quietly playing without it.
func loadDirectorFile(cfg *sim.Config, path string, given bool) error {
	if path == "" {
		return nil // -director "" opts out
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && !given:
		return nil
	case err != nil:
		return fmt.Errorf("reading director schedule: %w", err)
	}
	schedules, err := sim.LoadSchedules(data, path)
	if err != nil {
		return err
	}
	cfg.Schedules = schedules
	return nil
}

// alienNamesPathFromArgs is configPathFromArgs's counterpart for
// -alien-names, found the same way and for the same reason: the alien name
// pool must be loaded before flag.Parse runs.
func alienNamesPathFromArgs(args []string) (path string, given bool) {
	for i, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		if name != "-alien-names" && name != "--alien-names" {
			continue
		}
		if hasValue {
			return value, true
		}
		if i+1 < len(args) {
			return args[i+1], true
		}
		return "", true // "-alien-names" with nothing after it: use the built-in pool
	}
	return sim.AlienNameFileName, false
}

// loadAlienNamesFile applies an alien-names.yaml file to cfg. An absent
// default file is normal — cfg.AlienNames then stays empty, and world
// generation falls back to the built-in pool compiled into the sim package
// (see lore.go's defaultAlienNames) — but a file the player named
// explicitly, or one that does not parse, stops the run rather than quietly
// playing without it.
func loadAlienNamesFile(cfg *sim.Config, path string, given bool) error {
	if path == "" {
		return nil // -alien-names "" opts out
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && !given:
		return nil
	case err != nil:
		return fmt.Errorf("reading alien name pool: %w", err)
	}
	names, err := sim.LoadAlienNames(data, path)
	if err != nil {
		return err
	}
	cfg.AlienNames = names
	return nil
}

// checkNoArgs rejects anything left over after the flags. Go's flag package
// stops at the first argument that is not a flag and leaves the rest
// unparsed, so "mars-sim -- -tps 100" or "mars-sim x -tps 100" used to run at
// the settings file's rate with no hint that -tps had been dropped. mars-sim
// takes no positional arguments, so any leftover is a mistake worth stopping
// for.
func checkNoArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("unexpected argument %q: mars-sim takes only flags, and every flag after it was ignored", args[0])
}

// validateConfig rejects settings that would break world generation or the
// renderer, with a message a player can act on.
func validateConfig(cfg sim.Config) error {
	switch {
	case cfg.Width < 10 || cfg.Height < 10:
		return fmt.Errorf("world must be at least 10x10 (got %dx%d)", cfg.Width, cfg.Height)
	case cfg.IronRockPercent < 0 || cfg.IceRockPercent < 0 || cfg.UraniumRockPercent < 0 || cfg.ClayRockPercent < 0 ||
		cfg.IronRockPercent+cfg.IceRockPercent+cfg.UraniumRockPercent+cfg.ClayRockPercent > 100:
		return fmt.Errorf("rock composition percentages must be non-negative and total at most 100 (got iron %d + ice %d + uranium %d + clay %d)",
			cfg.IronRockPercent, cfg.IceRockPercent, cfg.UraniumRockPercent, cfg.ClayRockPercent)
	case cfg.RockVeinMin < 1 || cfg.RockVeinMax < cfg.RockVeinMin:
		return fmt.Errorf("rock vein range is invalid: min %d, max %d", cfg.RockVeinMin, cfg.RockVeinMax)
	case cfg.CavernPercent < 0 || cfg.CavernPercent > 100:
		return fmt.Errorf("cavern-percent must be between 0 and 100 (got %d)", cfg.CavernPercent)
	case cfg.CavernMin < 1 || cfg.CavernMax < cfg.CavernMin:
		return fmt.Errorf("cavern size range is invalid: min %d, max %d", cfg.CavernMin, cfg.CavernMax)
	case cfg.CavernPassagePercent < 0 || cfg.CavernPassagePercent > 100:
		return fmt.Errorf("cavern-passage-percent must be between 0 and 100 (got %d)", cfg.CavernPassagePercent)
	case cfg.StartColonists < 0 || cfg.StartAliens < 0 || cfg.StartCats < 0 || cfg.StartMice < 0:
		return fmt.Errorf("population counts cannot be negative")
	case cfg.StartPistols < 0 || cfg.StartShotguns < 0:
		return fmt.Errorf("starting weapon counts cannot be negative")
	case cfg.GraveyardSize < 0:
		return fmt.Errorf("graveyard-size cannot be negative")
	case cfg.TicksPerSecond < 1:
		return fmt.Errorf("tps must be at least 1 (got %d)", cfg.TicksPerSecond)
	case cfg.ColonistsPerFacility < 1:
		return fmt.Errorf("per-facility must be at least 1 (got %d)", cfg.ColonistsPerFacility)
	case cfg.MaxConcurrentProjects < 1:
		return fmt.Errorf("max-concurrent-projects must be at least 1 (got %d)", cfg.MaxConcurrentProjects)
	case cfg.ActiveStimulusLimit < 1 || cfg.ActiveStimulusLimit > sim.MaxActiveStimuli:
		return fmt.Errorf("active-stimulus-limit must be between 1 and %d (got %d)", sim.MaxActiveStimuli, cfg.ActiveStimulusLimit)
	case cfg.RestTicks < 1:
		return fmt.Errorf("rest-ticks must be at least 1 (got %d)", cfg.RestTicks)
	case cfg.StuckLimit < 1:
		// Movement takes this modulo, so 0 would panic on the first blocked step.
		return fmt.Errorf("stuck-limit must be at least 1 (got %d)", cfg.StuckLimit)
	case cfg.AlienSlowness < 1 || cfg.CatSlowness < 1:
		return fmt.Errorf("alien-slowness and cat-slowness must be at least 1 (got %d and %d)", cfg.AlienSlowness, cfg.CatSlowness)
	case cfg.TraitChance < 0 || cfg.TraitChance > 100:
		return fmt.Errorf("trait-chance must be between 0 and 100 (got %d)", cfg.TraitChance)
	case cfg.UraniumExposureTicks < 1:
		return fmt.Errorf("uranium-exposure-ticks must be at least 1 (got %d)", cfg.UraniumExposureTicks)
	case cfg.MutationChance < 0 || cfg.MutationChance > 100:
		return fmt.Errorf("mutation-chance must be between 0 and 100 (got %d)", cfg.MutationChance)
	case cfg.MutantLoverAffinityBonus < 0:
		return fmt.Errorf("mutant-lover-affinity cannot be negative (got %d)", cfg.MutantLoverAffinityBonus)
	case cfg.MutationStaturePercent < 0 || cfg.MutationStaturePercent > 99:
		return fmt.Errorf("mutation-stature-percent must be between 0 and 99 (got %d)", cfg.MutationStaturePercent)
	case cfg.StatureMinCM < 1:
		return fmt.Errorf("stature-min-cm must be at least 1 (got %d)", cfg.StatureMinCM)
	case cfg.StatureMaxCM < cfg.StatureMinCM:
		return fmt.Errorf("stature-max-cm (%d) cannot be below stature-min-cm (%d)", cfg.StatureMaxCM, cfg.StatureMinCM)
	case cfg.FamilyChance < 0 || cfg.FamilyChance > 100:
		return fmt.Errorf("family-chance must be between 0 and 100 (got %d)", cfg.FamilyChance)
	case cfg.AppearanceInheritChance < 0 || cfg.AppearanceInheritChance > 100:
		return fmt.Errorf("appearance-inherit-chance must be between 0 and 100 (got %d)", cfg.AppearanceInheritChance)
	case cfg.SpouseSurnameChance < 0 || cfg.SpouseSurnameChance > 100:
		return fmt.Errorf("spouse-surname-chance must be between 0 and 100 (got %d)", cfg.SpouseSurnameChance)
	case cfg.FamilyAffinity < 0 || cfg.FamilyAffinity > 100:
		return fmt.Errorf("family-affinity must be between 0 and 100 (got %d)", cfg.FamilyAffinity)
	case cfg.FamilyAffinitySpread < 0:
		return fmt.Errorf("family-affinity-spread must not be negative (got %d)", cfg.FamilyAffinitySpread)
	case cfg.TalkChance < 0 || cfg.TalkChance > 100:
		return fmt.Errorf("talk-chance must be between 0 and 100 (got %d)", cfg.TalkChance)
	case cfg.TalkChance > 0 && (cfg.TalkRadius < 1 || cfg.TalkTicks < 1):
		return fmt.Errorf("talk-radius and talk-ticks must be at least 1 when talking is enabled")
	case cfg.AffinityMax < 1:
		return fmt.Errorf("affinity-max must be at least 1 (got %d)", cfg.AffinityMax)
	case cfg.MoodMax < 1:
		return fmt.Errorf("mood-max must be at least 1 (got %d)", cfg.MoodMax)
	case cfg.SocialWindowTicks < 1:
		return fmt.Errorf("social-window-ticks must be at least 1 (got %d)", cfg.SocialWindowTicks)
	case cfg.MoodChargeDecayPerTick < 0 || cfg.MoodGripDecayPerTick < 0 || cfg.MoodLabelSwitchMargin < 0:
		return fmt.Errorf("affect decay and label switch margin cannot be negative")
	case cfg.MouseLitterMin < 0 || cfg.MouseLitterMax < cfg.MouseLitterMin:
		return fmt.Errorf("mouse litter range is invalid: min %d, max %d", cfg.MouseLitterMin, cfg.MouseLitterMax)
	}
	// Need specs are only reachable from the settings file and the -need-*
	// flags, but a bad one breaks the colonists quietly (a need that never
	// fires, or a fatal one pinned at its ceiling), so check them here too.
	for _, spec := range cfg.Needs {
		switch {
		case spec.Max < 1:
			return fmt.Errorf("need-%s-max must be at least 1 (got %d)", spec.Name, spec.Max)
		case spec.Rise < 0:
			return fmt.Errorf("need-%s-rise cannot be negative (got %d)", spec.Name, spec.Rise)
		case spec.SeekAt < 0 || spec.SeekAt > spec.CriticalAt || spec.CriticalAt > spec.Max:
			return fmt.Errorf("need-%s thresholds must satisfy 0 <= seek-at <= critical-at <= max (got %d, %d, %d)",
				spec.Name, spec.SeekAt, spec.CriticalAt, spec.Max)
		case spec.UseTicks < 0 || spec.GrabTicks < 0:
			return fmt.Errorf("need-%s use and grab ticks cannot be negative (got %d and %d)", spec.Name, spec.UseTicks, spec.GrabTicks)
		}
	}
	if cfg.FocusCurrentBonus < 0 || cfg.FocusSwitchMargin < 0 ||
		cfg.FocusCriticalBonus < 0 || cfg.FocusFatalBonus < 0 {
		return fmt.Errorf("focus bonuses and switch margin cannot be negative")
	}
	for _, spec := range cfg.Focuses {
		switch {
		case spec.Name == "":
			return fmt.Errorf("focus name cannot be empty")
		case spec.NeedWeight < 0:
			return fmt.Errorf("focus-%s-need-weight cannot be negative (got %d)", spec.Name, spec.NeedWeight)
		case spec.DistanceWeight < 0:
			return fmt.Errorf("focus-%s-distance-weight cannot be negative (got %d)", spec.Name, spec.DistanceWeight)
		}
	}
	return nil
}

func usage() {
	out := flag.CommandLine.Output()
	name := filepath.Base(os.Args[0])
	fmt.Fprintf(out, "mars-sim — a Mars colony simulation (Dwarf-Fortress-like)\n\n")
	fmt.Fprintf(out, "Usage:\n  %s [options]\n\n", name)
	fmt.Fprintf(out, "Examples:\n")
	fmt.Fprintf(out, "  %s -colonists 20 -aliens 5\n", name)
	fmt.Fprintf(out, "  %s -mice 20 -cats 4\n", name)
	fmt.Fprintf(out, "  %s -width 120 -height 60 -tps 12\n", name)
	fmt.Fprintf(out, "  %s -headless -duration 10s -seed 42\n", name)
	fmt.Fprintf(out, "  %s -print-config > %s   # a settings file you can edit and commit\n", name, sim.ConfigFileName)
	fmt.Fprintf(out, "  %s -director %s        # script scheduled occurrences (mouse plagues, alien swarms, supply drops)\n", name, sim.DirectorFileName)
	fmt.Fprintf(out, "  %s -alien-names %s   # customize what a seed's aliens can be named\n\n", name, sim.AlienNameFileName)
	fmt.Fprintf(out, "Options:\n")
	flag.PrintDefaults()
}

// Glyph modes for the -glyphs flag.
const (
	// glyphModeAuto measures each glyph against the real terminal at startup
	// and falls back to ASCII if any of them is painted at an unexpected
	// width. This is the default because no static width table is
	// authoritative: the terminal is.
	glyphModeAuto = "auto"
	// glyphModeEmoji skips the probe and trusts the width table. Useful when
	// the probe cannot run — inside a multiplexer that swallows the cursor
	// position report, say — but the emoji do in fact line up.
	glyphModeEmoji = "emoji"
	// glyphModeASCII forces the fallback set, for a terminal whose font has no
	// glyph for an emoji. That case is invisible to the probe: a missing glyph
	// still advances the expected number of cells, it just looks wrong.
	glyphModeASCII = "ascii"
)

// setUpGlyphs applies the -glyphs choice before the TUI starts. The probe needs
// raw mode and sole use of stdin, which it can only have before Bubble Tea
// takes the terminal.
//
// A probe that cannot run is not an error. Failing to get an answer out of the
// terminal leaves us exactly where a build without the probe would be — using
// the static registry — so it is not worth interrupting a run over.
func setUpGlyphs(mode string) {
	switch mode {
	case glyphModeASCII:
		tui.UseASCIIGlyphs()
	case glyphModeAuto:
		check, err := tui.VerifyGlyphWidths()
		if err != nil {
			// stderr, not stdout: the alt screen is about to cover stdout, and
			// a redirected stderr is where you look when the grid misbehaves.
			// The probe writes its own escape sequences to /dev/tty, so a
			// redirected stdout stays clean.
			fmt.Fprintf(os.Stderr, "mars-sim: could not measure glyph widths (%v); using the built-in width table\n", err)
			return
		}
		if check.Downgraded {
			fmt.Fprintln(os.Stderr, "mars-sim:", check.Detail)
		}
	}
}

// tuiFPS caps terminal redraws independently of the simulation tick rate. The
// latest snapshot still replaces stale frames, so lowering this cap affects
// presentation work without changing simulation timing or input handling.
const tuiFPS = 30

// runTUI drives the Bubble Tea frontend. If duration > 0 the program quits
// itself after that long, so a non-interactive run cannot hang.
func runTUI(eng *sim.Engine, snaps <-chan *sim.Snapshot, duration time.Duration) error {
	prog := tea.NewProgram(tui.New(eng, snaps), tea.WithAltScreen(), tea.WithFPS(tuiFPS))
	if duration > 0 {
		go func() {
			time.Sleep(duration)
			prog.Quit()
		}()
	}
	_, err := prog.Run()
	return err
}

// runHeadless consumes snapshots and logs a stats line roughly once a second,
// with no terminal UI. Useful for CI, profiling, and eyeballing balance without
// a TTY. Stops after duration (0 means run until interrupted).
func runHeadless(ctx context.Context, snaps <-chan *sim.Snapshot, cfg sim.Config, duration time.Duration) {
	var deadline <-chan time.Time
	if duration > 0 {
		deadline = time.After(duration)
	}
	report := time.NewTicker(time.Second)
	defer report.Stop()

	var latest *sim.Snapshot
	fmt.Printf("mars-sim headless: seed %d, %d colonists, %d aliens, %d cats, %d mice (Ctrl+C to stop)\n",
		cfg.Seed, cfg.StartColonists, cfg.StartAliens, cfg.StartCats, cfg.StartMice)
	for {
		select {
		case s, ok := <-snaps:
			if !ok {
				return
			}
			latest = s
		case <-report.C:
			if latest != nil {
				fmt.Printf("tick %5d | colonists %2d | aliens %2d | cats %2d | mice %2d | pods %d | toilets %d | beds %d | burners %d | refuse %d | rooms %d | excavated %5d\n",
					latest.Tick, latest.Stats.Colonists, latest.Stats.Aliens,
					latest.Stats.Cats, latest.Stats.Mice,
					latest.Stats.Pods, latest.Stats.Toilets, latest.Stats.Beds,
					latest.Stats.Incinerators, latest.Stats.Refuse,
					latest.Stats.Rooms, latest.Stats.FloorDug)
			}
		case <-ctx.Done():
			return
		case <-deadline:
			if latest != nil {
				fmt.Printf("done at tick %d: colonists %d, aliens %d, cats %d, mice %d, pods %d, toilets %d, beds %d, incinerators %d, refuse %d, excavated %d tiles\n",
					latest.Tick, latest.Stats.Colonists, latest.Stats.Aliens,
					latest.Stats.Cats, latest.Stats.Mice,
					latest.Stats.Pods, latest.Stats.Toilets, latest.Stats.Beds,
					latest.Stats.Incinerators, latest.Stats.Refuse, latest.Stats.FloorDug)
			}
			return
		}
	}
}

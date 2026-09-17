// Command mars-sim runs the Mars colony simulation with a Bubble Tea terminal UI.
//
// Architecture: the simulation runs in its own goroutine (the Engine) and never
// shares mutable state with the UI. Frontends subscribe for immutable Snapshots
// and send Commands back. The Bubble Tea TUI here is one such frontend; another
// (web, GUI, headless test harness) could attach to the same Engine unchanged.
//
// Every tunable in sim.Config is exposed as a command-line flag whose default is
// the value from sim.DefaultConfig, so DefaultConfig stays the single source of
// truth. Run with -h or ? to list them.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cfg := sim.DefaultConfig()

	// Application flags (not part of the simulation config).
	var (
		duration time.Duration
		headless bool
		seed     int64
		glyphs   string
	)
	flag.DurationVar(&duration, "duration", 0, "auto-exit after this long (0 = run until quit); handy for smoke tests")
	flag.BoolVar(&headless, "headless", false, "run without the TUI, printing periodic stats")
	flag.Int64Var(&seed, "seed", 0, "world seed (0 = random each run)")
	flag.StringVar(&glyphs, "glyphs", glyphModeAuto, "map glyphs: auto (measure the terminal), emoji (trust the width table), or ascii")

	// Simulation config flags, each defaulting to the DefaultConfig value.
	bindConfigFlags(&cfg)

	flag.Usage = usage
	// Support "?" as a help alias alongside the flag package's built-in -h/-help.
	for _, a := range os.Args[1:] {
		if a == "?" || a == "-?" || a == "--?" {
			usage()
			return
		}
	}
	flag.Parse()

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

	eng := sim.NewEngine(cfg)

	// Subscribe before starting the engine so the very first frame is not missed.
	snaps := eng.Subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Run(ctx)

	if headless {
		runHeadless(snaps, cfg, duration)
		return
	}

	setUpGlyphs(glyphs)

	if err := runTUI(eng, snaps, duration); err != nil {
		cancel()
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		os.Exit(1)
	}
}

// bindConfigFlags registers a flag for every tunable in the simulation config,
// using the passed-in (default) values as the flag defaults.
func bindConfigFlags(cfg *sim.Config) {
	// World.
	flag.IntVar(&cfg.Width, "width", cfg.Width, "world width in tiles")
	flag.IntVar(&cfg.Height, "height", cfg.Height, "world height in tiles")
	flag.IntVar(&cfg.IronRockPercent, "iron-rock-percent", cfg.IronRockPercent, "percent of rock tiles bearing iron")
	flag.IntVar(&cfg.IceRockPercent, "ice-rock-percent", cfg.IceRockPercent, "percent of rock tiles bearing water ice")
	flag.IntVar(&cfg.UraniumRockPercent, "uranium-rock-percent", cfg.UraniumRockPercent, "percent of rock tiles bearing uranium")
	flag.IntVar(&cfg.ClayRockPercent, "clay-rock-percent", cfg.ClayRockPercent, "percent of rock tiles bearing clay")
	flag.IntVar(&cfg.RockVeinMin, "rock-vein-min", cfg.RockVeinMin, "minimum tiles in a generated rock deposit vein")
	flag.IntVar(&cfg.RockVeinMax, "rock-vein-max", cfg.RockVeinMax, "maximum tiles in a generated rock deposit vein")

	// Starting population.
	flag.IntVar(&cfg.StartColonists, "colonists", cfg.StartColonists, "starting number of colonists")
	flag.IntVar(&cfg.StartAliens, "aliens", cfg.StartAliens, "starting number of aliens")
	flag.IntVar(&cfg.StartCats, "cats", cfg.StartCats, "starting number of cats")
	flag.IntVar(&cfg.StartMice, "mice", cfg.StartMice, "starting number of mice")
	flag.IntVar(&cfg.StartPistols, "pistols", cfg.StartPistols, "pistols the colony ship arrives with")
	flag.IntVar(&cfg.StartShotguns, "shotguns", cfg.StartShotguns, "shotguns the colony ship arrives with")
	flag.IntVar(&cfg.GraveyardSize, "graveyard-size", cfg.GraveyardSize, "recent deaths kept for the roster's dead filter (0 disables)")

	// Timing.
	flag.IntVar(&cfg.TicksPerSecond, "tps", cfg.TicksPerSecond, "simulation ticks per second")
	flag.IntVar(&cfg.LogSize, "log-size", cfg.LogSize, "number of recent events retained")

	// Colonists.
	flag.IntVar(&cfg.ColonistHP, "colonist-hp", cfg.ColonistHP, "colonist hit points")
	flag.IntVar(&cfg.MineTicks, "mine-ticks", cfg.MineTicks, "ticks of work to excavate one rock tile")
	flag.IntVar(&cfg.BuildTicks, "build-ticks", cfg.BuildTicks, "ticks of work to raise one wall")
	flag.IntVar(&cfg.FacilityBuildTicks, "facility-ticks", cfg.FacilityBuildTicks, "ticks of work to build a pod or toilet")
	flag.IntVar(&cfg.FleeRadius, "flee-radius", cfg.FleeRadius, "colonist flees when an alien is within this many tiles")
	flag.IntVar(&cfg.ColonistStompRadius, "stomp-radius", cfg.ColonistStompRadius, "an idle colonist chases and crushes a mouse within this many tiles")
	flag.IntVar(&cfg.GoreSightRadius, "gore-sight-radius", cfg.GoreSightRadius, "a colonist notices gore on the ground within this many tiles")
	flag.IntVar(&cfg.CleanRadius, "clean-radius", cfg.CleanRadius, "how far a colonist looks for refuse to clean up (a Tidy colonist looks twice as far)")
	flag.IntVar(&cfg.CleanTicks, "clean-ticks", cfg.CleanTicks, "ticks of work to scrub one tile of refuse clean")
	flag.IntVar(&cfg.IncinerateTicks, "incinerate-ticks", cfg.IncinerateTicks, "ticks spent feeding a load of refuse into an incinerator")
	flag.IntVar(&cfg.IncineratorBuildTicks, "incinerator-ticks", cfg.IncineratorBuildTicks, "ticks of work to build an incinerator")
	flag.IntVar(&cfg.StarveDamage, "starve-damage", cfg.StarveDamage, "HP lost per tick while starving")
	flag.IntVar(&cfg.ColonistsPerFacility, "per-facility", cfg.ColonistsPerFacility, "colonists served by each life-support facility")
	flag.IntVar(&cfg.MaxConcurrentProjects, "max-concurrent-projects", cfg.MaxConcurrentProjects, "rooms that can be under construction at once")
	flag.IntVar(&cfg.RestTicks, "rest-ticks", cfg.RestTicks, "ticks an idle colonist rests before re-checking for work")
	flag.IntVar(&cfg.TraitChance, "trait-chance", cfg.TraitChance, "percent chance a colonist gets a trait from each trait group (0 disables)")
	flag.IntVar(&cfg.UraniumExposureTicks, "uranium-exposure-ticks", cfg.UraniumExposureTicks, "ticks of uranium exposure per mutation roll")
	flag.IntVar(&cfg.MutationChance, "mutation-chance", cfg.MutationChance, "percent chance each full uranium dose mutates a colonist (0 disables mutation)")
	flag.IntVar(&cfg.MutantLoverAffinityBonus, "mutant-lover-affinity", cfg.MutantLoverAffinityBonus, "extra affinity a mutant-lover gains toward a mutant per conversation")
	flag.IntVar(&cfg.FamilyChance, "family-chance", cfg.FamilyChance, "percent chance a new colonist is tied to an existing one by family (0 disables)")
	flag.IntVar(&cfg.AppearanceInheritChance, "appearance-inherit-chance", cfg.AppearanceInheritChance, "percent chance each of a colonist's features is inherited from a close relative (0 disables)")
	flag.IntVar(&cfg.SpouseSurnameChance, "spouse-surname-chance", cfg.SpouseSurnameChance, "percent chance a colonist marrying in takes their spouse's surname")
	flag.IntVar(&cfg.FamilyAffinity, "family-affinity", cfg.FamilyAffinity, "starting affinity between close relatives, as a percent of affinity-max (0 disables)")
	flag.IntVar(&cfg.FamilyAffinitySpread, "family-affinity-spread", cfg.FamilyAffinitySpread, "random swing around the starting family affinity, in the same units")
	flag.IntVar(&cfg.TalkChance, "talk-chance", cfg.TalkChance, "percent chance an idle colonist starts a conversation (0 disables talking)")
	flag.IntVar(&cfg.TalkRadius, "talk-radius", cfg.TalkRadius, "how far a colonist looks for a conversation partner")
	flag.IntVar(&cfg.TalkTicks, "talk-ticks", cfg.TalkTicks, "ticks a conversation lasts before affinity is credited")
	flag.IntVar(&cfg.TalkAffinityGain, "talk-affinity-gain", cfg.TalkAffinityGain, "base affinity step per conversation (scaled by outcome and diminishing returns)")
	flag.IntVar(&cfg.AffinityMax, "affinity-max", cfg.AffinityMax, "affinity runs in [-affinity-max, affinity-max]; talking alone saturates at half")
	flag.IntVar(&cfg.TalkQualityBias, "talk-quality-bias", cfg.TalkQualityBias, "baseline lean of conversation quality (-100..100)")
	flag.IntVar(&cfg.TalkQualityValence, "talk-quality-valence", cfg.TalkQualityValence, "how strongly existing affinity biases conversation quality")
	flag.IntVar(&cfg.TalkQualitySpread, "talk-quality-spread", cfg.TalkQualitySpread, "random swing around a conversation's mean quality")
	flag.IntVar(&cfg.MoodMax, "mood-max", cfg.MoodMax, "colonist mood runs in [-mood-max, mood-max]")
	flag.IntVar(&cfg.MoodCompanyWeight, "mood-company-weight", cfg.MoodCompanyWeight, "mood shift per conversation from how one feels about the other")
	flag.IntVar(&cfg.MoodConversationWeight, "mood-conversation-weight", cfg.MoodConversationWeight, "mood shift per conversation from how the chat itself went")
	flag.IntVar(&cfg.SocialWindowTicks, "social-window-ticks", cfg.SocialWindowTicks, "ticks in the rolling window for social conversation fatigue")
	flag.IntVar(&cfg.FrontierFieldMinColonists, "frontier-field-colonists", cfg.FrontierFieldMinColonists, "colony size at/above which miners use the shared frontier flow field")
	flag.IntVar(&cfg.FrontierFieldMinArea, "frontier-field-area", cfg.FrontierFieldMinArea, "map area (tiles) at/above which miners use the shared frontier flow field")

	// Aliens.
	flag.IntVar(&cfg.AlienHP, "alien-hp", cfg.AlienHP, "alien hit points")
	flag.IntVar(&cfg.AlienDamage, "alien-damage", cfg.AlienDamage, "HP removed per alien bite")
	flag.IntVar(&cfg.AlienBiteRest, "alien-bite-rest", cfg.AlienBiteRest, "cooldown ticks between alien bites")
	flag.IntVar(&cfg.AlienSlowness, "alien-slowness", cfg.AlienSlowness, "alien acts once every N ticks (higher = slower)")

	// Weapons.
	flag.IntVar(&cfg.PistolDamage, "pistol-damage", cfg.PistolDamage, "HP removed per pistol shot")
	flag.IntVar(&cfg.PistolRange, "pistol-range", cfg.PistolRange, "max tiles a pistol can fire from")
	flag.IntVar(&cfg.PistolFireRest, "pistol-fire-rest", cfg.PistolFireRest, "cooldown ticks between pistol shots")
	flag.IntVar(&cfg.ShotgunDamage, "shotgun-damage", cfg.ShotgunDamage, "HP removed per shotgun blast")
	flag.IntVar(&cfg.ShotgunRange, "shotgun-range", cfg.ShotgunRange, "max tiles a shotgun can fire from")
	flag.IntVar(&cfg.ShotgunFireRest, "shotgun-fire-rest", cfg.ShotgunFireRest, "cooldown ticks between shotgun blasts")

	// Cats.
	flag.IntVar(&cfg.CatHP, "cat-hp", cfg.CatHP, "cat hit points")
	flag.IntVar(&cfg.CatSlowness, "cat-slowness", cfg.CatSlowness, "cat acts once every N ticks (higher = slower)")
	flag.IntVar(&cfg.CatPounceRest, "cat-pounce-rest", cfg.CatPounceRest, "cooldown ticks after a cat catches a mouse")

	// Mice.
	flag.IntVar(&cfg.MouseHP, "mouse-hp", cfg.MouseHP, "mouse hit points")
	flag.IntVar(&cfg.MouseHungerRise, "mouse-hunger-rise", cfg.MouseHungerRise, "food need a mouse gains per tick (mice eat frequently)")
	flag.IntVar(&cfg.MouseFleeRadius, "mouse-flee-radius", cfg.MouseFleeRadius, "mouse flees when a cat is within this many tiles")
	flag.IntVar(&cfg.MouseGestationTicks, "mouse-gestation", cfg.MouseGestationTicks, "ticks a pregnant mouse carries a litter before giving birth")
	flag.IntVar(&cfg.MouseLitterMin, "mouse-litter-min", cfg.MouseLitterMin, "smallest mouse litter size")
	flag.IntVar(&cfg.MouseLitterMax, "mouse-litter-max", cfg.MouseLitterMax, "largest mouse litter size")
	flag.IntVar(&cfg.MouseBreedCooldown, "mouse-breed-cooldown", cfg.MouseBreedCooldown, "ticks a mouse waits before it can mate again")
	flag.IntVar(&cfg.MouseMaturityTicks, "mouse-maturity", cfg.MouseMaturityTicks, "ticks a newborn mouse takes to mature enough to breed")
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
	case cfg.RestTicks < 1:
		return fmt.Errorf("rest-ticks must be at least 1 (got %d)", cfg.RestTicks)
	case cfg.TraitChance < 0 || cfg.TraitChance > 100:
		return fmt.Errorf("trait-chance must be between 0 and 100 (got %d)", cfg.TraitChance)
	case cfg.UraniumExposureTicks < 1:
		return fmt.Errorf("uranium-exposure-ticks must be at least 1 (got %d)", cfg.UraniumExposureTicks)
	case cfg.MutationChance < 0 || cfg.MutationChance > 100:
		return fmt.Errorf("mutation-chance must be between 0 and 100 (got %d)", cfg.MutationChance)
	case cfg.MutantLoverAffinityBonus < 0:
		return fmt.Errorf("mutant-lover-affinity cannot be negative (got %d)", cfg.MutantLoverAffinityBonus)
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
	case cfg.MouseLitterMin < 0 || cfg.MouseLitterMax < cfg.MouseLitterMin:
		return fmt.Errorf("mouse litter range is invalid: min %d, max %d", cfg.MouseLitterMin, cfg.MouseLitterMax)
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
	fmt.Fprintf(out, "  %s -headless -duration 10s -seed 42\n\n", name)
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
func runHeadless(snaps <-chan *sim.Snapshot, cfg sim.Config, duration time.Duration) {
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

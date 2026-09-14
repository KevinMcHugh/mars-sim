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
	)
	flag.DurationVar(&duration, "duration", 0, "auto-exit after this long (0 = run until quit); handy for smoke tests")
	flag.BoolVar(&headless, "headless", false, "run without the TUI, printing periodic stats")
	flag.Int64Var(&seed, "seed", 0, "world seed (0 = random each run)")

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

	// Starting population.
	flag.IntVar(&cfg.StartColonists, "colonists", cfg.StartColonists, "starting number of colonists")
	flag.IntVar(&cfg.StartAliens, "aliens", cfg.StartAliens, "starting number of aliens")
	flag.IntVar(&cfg.StartCats, "cats", cfg.StartCats, "starting number of cats")
	flag.IntVar(&cfg.StartMice, "mice", cfg.StartMice, "starting number of mice")

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
	flag.IntVar(&cfg.StarveDamage, "starve-damage", cfg.StarveDamage, "HP lost per tick while starving")
	flag.IntVar(&cfg.ColonistsPerFacility, "per-facility", cfg.ColonistsPerFacility, "colonists served by each life-support facility")
	flag.IntVar(&cfg.RestTicks, "rest-ticks", cfg.RestTicks, "ticks an idle colonist rests before re-checking for work")
	flag.IntVar(&cfg.TraitChance, "trait-chance", cfg.TraitChance, "percent chance a colonist gets a trait from each trait group (0 disables)")
	flag.IntVar(&cfg.FamilyChance, "family-chance", cfg.FamilyChance, "percent chance a new colonist is tied to an existing one by family (0 disables)")
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
	flag.IntVar(&cfg.FrontierFieldMinColonists, "frontier-field-colonists", cfg.FrontierFieldMinColonists, "colony size at/above which miners use the shared frontier flow field")
	flag.IntVar(&cfg.FrontierFieldMinArea, "frontier-field-area", cfg.FrontierFieldMinArea, "map area (tiles) at/above which miners use the shared frontier flow field")

	// Aliens.
	flag.IntVar(&cfg.AlienHP, "alien-hp", cfg.AlienHP, "alien hit points")
	flag.IntVar(&cfg.AlienDamage, "alien-damage", cfg.AlienDamage, "HP removed per alien bite")
	flag.IntVar(&cfg.AlienBiteRest, "alien-bite-rest", cfg.AlienBiteRest, "cooldown ticks between alien bites")
	flag.IntVar(&cfg.AlienSlowness, "alien-slowness", cfg.AlienSlowness, "alien acts once every N ticks (higher = slower)")

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
	case cfg.StartColonists < 0 || cfg.StartAliens < 0 || cfg.StartCats < 0 || cfg.StartMice < 0:
		return fmt.Errorf("population counts cannot be negative")
	case cfg.TicksPerSecond < 1:
		return fmt.Errorf("tps must be at least 1 (got %d)", cfg.TicksPerSecond)
	case cfg.ColonistsPerFacility < 1:
		return fmt.Errorf("per-facility must be at least 1 (got %d)", cfg.ColonistsPerFacility)
	case cfg.RestTicks < 1:
		return fmt.Errorf("rest-ticks must be at least 1 (got %d)", cfg.RestTicks)
	case cfg.TraitChance < 0 || cfg.TraitChance > 100:
		return fmt.Errorf("trait-chance must be between 0 and 100 (got %d)", cfg.TraitChance)
	case cfg.FamilyChance < 0 || cfg.FamilyChance > 100:
		return fmt.Errorf("family-chance must be between 0 and 100 (got %d)", cfg.FamilyChance)
	case cfg.TalkChance < 0 || cfg.TalkChance > 100:
		return fmt.Errorf("talk-chance must be between 0 and 100 (got %d)", cfg.TalkChance)
	case cfg.TalkChance > 0 && (cfg.TalkRadius < 1 || cfg.TalkTicks < 1):
		return fmt.Errorf("talk-radius and talk-ticks must be at least 1 when talking is enabled")
	case cfg.AffinityMax < 1:
		return fmt.Errorf("affinity-max must be at least 1 (got %d)", cfg.AffinityMax)
	case cfg.MoodMax < 1:
		return fmt.Errorf("mood-max must be at least 1 (got %d)", cfg.MoodMax)
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

// runTUI drives the Bubble Tea frontend. If duration > 0 the program quits
// itself after that long, so a non-interactive run cannot hang.
func runTUI(eng *sim.Engine, snaps <-chan *sim.Snapshot, duration time.Duration) error {
	prog := tea.NewProgram(tui.New(eng, snaps), tea.WithAltScreen())
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
				fmt.Printf("tick %5d | colonists %2d | aliens %2d | cats %2d | mice %2d | pods %d | toilets %d | beds %d | rooms %d | excavated %5d\n",
					latest.Tick, latest.Stats.Colonists, latest.Stats.Aliens,
					latest.Stats.Cats, latest.Stats.Mice,
					latest.Stats.Pods, latest.Stats.Toilets, latest.Stats.Beds, latest.Stats.Rooms, latest.Stats.FloorDug)
			}
		case <-deadline:
			if latest != nil {
				fmt.Printf("done at tick %d: colonists %d, aliens %d, cats %d, mice %d, pods %d, toilets %d, beds %d, excavated %d tiles\n",
					latest.Tick, latest.Stats.Colonists, latest.Stats.Aliens,
					latest.Stats.Cats, latest.Stats.Mice,
					latest.Stats.Pods, latest.Stats.Toilets, latest.Stats.Beds, latest.Stats.FloorDug)
			}
			return
		}
	}
}

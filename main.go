// Command mars-sim runs the Mars colony simulation with a Bubble Tea terminal UI.
//
// Architecture: the simulation runs in its own goroutine (the Engine) and never
// shares mutable state with the UI. Frontends subscribe for immutable Snapshots
// and send Commands back. The Bubble Tea TUI here is one such frontend; another
// (web, GUI, headless test harness) could attach to the same Engine unchanged.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	var (
		duration = flag.Duration("duration", 0, "auto-exit after this long (0 = run until quit); handy for smoke tests")
		headless = flag.Bool("headless", false, "run the simulation without the TUI, printing periodic stats")
		seed     = flag.Int64("seed", 0, "world seed (0 = random)")
	)
	flag.Parse()

	cfg := sim.DefaultConfig()
	if *seed != 0 {
		cfg.Seed = *seed
	}
	eng := sim.NewEngine(cfg)

	// Subscribe before starting the engine so the very first frame is not
	// missed.
	snaps := eng.Subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Run(ctx)

	if *headless {
		runHeadless(snaps, *duration)
		return
	}

	if err := runTUI(eng, snaps, *duration); err != nil {
		cancel()
		fmt.Fprintln(os.Stderr, "mars-sim:", err)
		os.Exit(1)
	}
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
func runHeadless(snaps <-chan *sim.Snapshot, duration time.Duration) {
	var deadline <-chan time.Time
	if duration > 0 {
		deadline = time.After(duration)
	}
	report := time.NewTicker(time.Second)
	defer report.Stop()

	var latest *sim.Snapshot
	fmt.Println("mars-sim headless: simulating (Ctrl+C to stop)")
	for {
		select {
		case s, ok := <-snaps:
			if !ok {
				return
			}
			latest = s
		case <-report.C:
			if latest != nil {
				fmt.Printf("tick %5d | colonists %2d | aliens %2d | pods %d | toilets %d | excavated %5d\n",
					latest.Tick, latest.Stats.Colonists, latest.Stats.Aliens,
					latest.Stats.Pods, latest.Stats.Toilets, latest.Stats.FloorDug)
			}
		case <-deadline:
			if latest != nil {
				fmt.Printf("done at tick %d: colonists %d, aliens %d, pods %d, toilets %d, excavated %d tiles\n",
					latest.Tick, latest.Stats.Colonists, latest.Stats.Aliens,
					latest.Stats.Pods, latest.Stats.Toilets, latest.Stats.FloorDug)
			}
			return
		}
	}
}

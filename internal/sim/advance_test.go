package sim

import (
	"context"
	"testing"
	"time"
)

// The first Advance publishes the initial world before any tick, and a slow
// engine then reports how long until its first tick instead of running it.
func TestAdvancePublishesTheInitialFrame(t *testing.T) {
	cfg := testConfig()
	cfg.TicksPerSecond = 1
	eng := NewEngine(cfg)

	snap, wait := eng.Advance(time.Millisecond)
	if snap == nil || snap.Tick != 0 {
		t.Fatalf("first Advance returned %v, want the tick-0 frame", snap)
	}
	if wait <= 0 || wait > time.Second {
		t.Errorf("wait = %v, want the time until the first tick (0, 1s]", wait)
	}
	if snap, _ := eng.Advance(time.Millisecond); snap != nil {
		t.Errorf("an Advance with nothing due returned frame %d, want nil", snap.Tick)
	}
}

// A fast engine runs ticks until the budget is spent, then returns with the
// next tick still due (wait 0) and the newest frame.
func TestAdvanceRunsDueTicksWithinBudget(t *testing.T) {
	cfg := testConfig()
	cfg.TicksPerSecond = 1_000_000 // every tick is overdue by the time it is checked
	eng := NewEngine(cfg)
	eng.Advance(0) // the initial frame

	start := time.Now()
	snap, wait := eng.Advance(20 * time.Millisecond)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Advance(20ms) took %v; it should stop once the budget is spent", elapsed)
	}
	if wait != 0 {
		t.Errorf("wait = %v, want 0 with ticks still due", wait)
	}
	if snap == nil || snap.Tick < 2 {
		t.Fatalf("Advance returned %v, want a frame several ticks in", snap)
	}
	if got := eng.world.tick; snap.Tick != got {
		t.Errorf("returned frame is tick %d, world is at %d: want the newest", snap.Tick, got)
	}
}

// However small the budget, a due tick runs, so a tick slower than the
// budget cannot stall the game.
func TestAdvanceRunsOneTickOnAZeroBudget(t *testing.T) {
	cfg := testConfig()
	cfg.TicksPerSecond = 1_000_000
	eng := NewEngine(cfg)
	eng.Advance(0)

	before := eng.world.tick
	eng.Advance(0)
	if got := eng.world.tick - before; got != 1 {
		t.Errorf("Advance(0) ran %d ticks, want exactly 1", got)
	}
}

// Commands sent between calls apply at the next Advance. Paused, Advance
// reports a negative wait and runs no ticks; resuming waits one interval.
func TestAdvanceAppliesCommandsAndPauses(t *testing.T) {
	cfg := testConfig()
	cfg.TicksPerSecond = 1_000_000
	eng := NewEngine(cfg)
	eng.Advance(0)
	eng.Advance(0)

	eng.Send(TogglePause{})
	snap, wait := eng.Advance(time.Millisecond)
	if wait >= 0 {
		t.Errorf("paused wait = %v, want negative", wait)
	}
	if snap == nil || !snap.Paused {
		t.Fatalf("pausing returned %v, want a frame showing the pause", snap)
	}
	paused := eng.world.tick
	if _, wait := eng.Advance(time.Millisecond); wait >= 0 || eng.world.tick != paused {
		t.Errorf("a paused Advance ran to tick %d (wait %v), want %d and a negative wait", eng.world.tick, wait, paused)
	}

	eng.Send(SetTicksPerSecond{Rate: 1})
	eng.Send(TogglePause{})
	if _, wait := eng.Advance(time.Millisecond); wait <= 0 || eng.world.tick != paused {
		t.Errorf("resuming at 1 tps: tick %d, wait %v; want no tick yet and a positive wait", eng.world.tick, wait)
	}
}

// Advance and Run each own the schedule, so an engine takes one or the other;
// and once Advance has started, the world is no longer safe to reconfigure.
func TestAdvanceExcludesRunAndLateLiveTiles(t *testing.T) {
	cfg := testConfig()
	mustPanic := func(name string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic", name)
			}
		}()
		f()
	}

	eng := NewEngine(cfg)
	eng.Advance(0)
	mustPanic("Run after Advance", func() { eng.Run(context.Background()) })
	mustPanic("ShareLiveTiles after Advance", eng.ShareLiveTiles)

	eng = NewEngine(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { eng.Run(ctx); close(done) }()
	for {
		eng.mu.Lock()
		running := eng.running
		eng.mu.Unlock()
		if running {
			break
		}
		time.Sleep(time.Millisecond)
	}
	mustPanic("Advance after Run", func() { eng.Advance(0) })
	cancel()
	<-done
}

// The browser worker's setup: live tiles, driven by Advance. Every frame the
// engine publishes is returned, so TileChanges.Frame never skips and an
// incremental consumer can trust the deltas — including when a command that
// wants a fresh frame (a spawn) lands in the same call as ticks that would
// publish one too.
func TestAdvanceWithLiveTilesSeesEveryFrame(t *testing.T) {
	cfg := testConfig()
	cfg.TicksPerSecond = 1_000_000
	eng := NewEngine(cfg)
	eng.ShareLiveTiles()

	var last uint64
	deadline := time.Now().Add(5 * time.Second)
	for last < 10 && time.Now().Before(deadline) {
		eng.Send(Spawn{Kind: Cat})
		snap, _ := eng.Advance(5 * time.Millisecond)
		if snap == nil {
			continue
		}
		if f := snap.TileChanges.Frame; f != last+1 {
			t.Fatalf("frame %d followed %d; a published frame never reached the host", f, last)
		}
		last = snap.TileChanges.Frame
	}
	if last < 10 {
		t.Errorf("saw %d frames in 5s, want 10", last)
	}
}

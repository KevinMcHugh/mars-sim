package sim

import (
	"context"
	"testing"
	"time"
)

// A late wakeup must not cost a tick: the next tick stays due on the original
// schedule, so it runs immediately and the average rate holds.
func TestNextDueCatchesUpAfterALateWakeup(t *testing.T) {
	t0 := time.Unix(1000, 0)
	interval := 10 * time.Millisecond

	// Woken 7ms late for the tick due at t0 (as a coalescing OS timer might).
	now := t0.Add(7 * time.Millisecond)
	if got, want := nextDue(t0, now, interval), t0.Add(interval); !got.Equal(want) {
		t.Errorf("next due %v, want %v: the schedule drifted by the late wakeup", got.Sub(t0), want.Sub(t0))
	}

	// Woken 25ms late: two ticks are already overdue, so the next one is due
	// in the past and runs straight away.
	now = t0.Add(25 * time.Millisecond)
	if got := nextDue(t0, now, interval); !got.Before(now) {
		t.Errorf("next due %v is not overdue at %v; the missed tick would be dropped", got.Sub(t0), now.Sub(t0))
	}
}

// Far behind (the machine cannot keep up), the schedule restarts from now
// instead of bursting through an ever-growing backlog.
func TestNextDueGivesUpOnAHopelessBacklog(t *testing.T) {
	t0 := time.Unix(1000, 0)
	now := t0.Add(time.Second)
	if got := nextDue(t0, now, 10*time.Millisecond); !got.Equal(now) {
		t.Errorf("next due %v, want now (%v)", got.Sub(t0), now.Sub(t0))
	}
}

func TestShouldPublish(t *testing.T) {
	frame := time.Second / maxPublishRate
	cases := []struct {
		tps       int
		sinceLast time.Duration
		want      bool
	}{
		{8, 0, true},                           // slow games publish every tick
		{maxPublishRate, frame / 2, true},      // even if timer jitter lands a tick early
		{maxPublishRate + 1, frame / 2, false}, // fast games skip until a frame is due
		{5000, frame - time.Microsecond, false},
		{5000, frame, true},
	}
	for _, c := range cases {
		if got := shouldPublish(c.tps, c.sinceLast); got != c.want {
			t.Errorf("shouldPublish(%d, %v) = %v, want %v", c.tps, c.sinceLast, got, c.want)
		}
	}
}

// A fast engine publishes about maxPublishRate frames a second, not one per
// tick, and pausing still publishes the latest state.
func TestFastEnginePublishesAtFrameRate(t *testing.T) {
	cfg := testConfig()
	cfg.TicksPerSecond = 2000
	eng := NewEngine(cfg)
	snaps := eng.Subscribe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Run(ctx)

	<-snaps // the initial frame
	frames := 0
	window := time.After(500 * time.Millisecond)
collect:
	for {
		select {
		case <-snaps:
			frames++
		case <-window:
			break collect
		}
	}
	eng.Send(TogglePause{})
	var paused *Snapshot
	for paused == nil {
		if s := <-snaps; s.Paused {
			paused = s
		}
	}
	if paused.Tick < 300 {
		t.Skipf("only %d ticks in 500ms; too slow a machine to tell frames from ticks", paused.Tick)
	}
	// 500ms at 60 frames a second is 30; allow generous slack for scheduling.
	if frames > 60 {
		t.Errorf("received %d frames over %d ticks in 500ms; want about %d", frames, paused.Tick, maxPublishRate/2)
	}
	if paused.Tick != eng.world.tick {
		t.Errorf("paused frame shows tick %d, but the world is at %d", paused.Tick, eng.world.tick)
	}
}

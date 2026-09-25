package sim

import (
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

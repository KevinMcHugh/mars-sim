package sim

// eventLog is a fixed-size ring buffer of human-readable event strings (a
// colonist died, a wall went up, ...). The UI shows the most recent entries.
type eventLog struct {
	entries []string
	max     int
}

func newEventLog(max int) *eventLog {
	if max < 1 {
		max = 1
	}
	return &eventLog{max: max}
}

// add appends a message, trimming the oldest entries past the cap.
func (l *eventLog) add(msg string) {
	l.entries = append(l.entries, msg)
	if len(l.entries) > l.max {
		l.entries = l.entries[len(l.entries)-l.max:]
	}
}

// tail returns up to n most recent messages, oldest first, as a fresh slice
// (safe to hand to another goroutine).
func (l *eventLog) tail(n int) []string {
	if n > len(l.entries) {
		n = len(l.entries)
	}
	out := make([]string, n)
	copy(out, l.entries[len(l.entries)-n:])
	return out
}

package sim

// LogKind is the type column on the colony log. The sentence stays in
// LogEntry.Text; the kind is how a reader scans for a death or a finished
// room without reading every line. A new log line picks one of these rather
// than leaving the zero value, which is only "note".
type LogKind int

const (
	LogNote LogKind = iota
	LogDeath
	LogCombat
	LogBuildStart
	LogBuildComplete
	LogArrival
	LogMutation
	LogHaul
	LogBurn
	LogMate
	LogBirth
	LogEscape
	LogCavern
	LogNest
	LogKindCount
)

// String is the column label. The widest is "build complete"; the log tab
// sizes the column from that and rejects a longer label in a test.
func (k LogKind) String() string {
	switch k {
	case LogDeath:
		return "death"
	case LogCombat:
		return "combat"
	case LogBuildStart:
		return "build start"
	case LogBuildComplete:
		return "build complete"
	case LogArrival:
		return "arrival"
	case LogMutation:
		return "mutation"
	case LogHaul:
		return "haul"
	case LogBurn:
		return "burn"
	case LogMate:
		return "mate"
	case LogBirth:
		return "birth"
	case LogEscape:
		return "escape"
	case LogCavern:
		return "cavern"
	case LogNest:
		return "nest"
	default:
		return "note"
	}
}

// LogEntry is one retained colony-log line: a type and the sentence.
type LogEntry struct {
	Kind LogKind
	Text string
}

// eventLog is a fixed-size ring buffer of colony-log lines. Snapshots
// publish the whole buffer. The map sidebar shows the tail that fits; the
// log tab shows all of it, with Kind as its own column.
type eventLog struct {
	entries []LogEntry
	max     int
}

func newEventLog(max int) *eventLog {
	if max < 1 {
		max = 1
	}
	return &eventLog{max: max}
}

// add appends a line, trimming the oldest entries past the cap.
func (l *eventLog) add(kind LogKind, msg string) {
	l.entries = append(l.entries, LogEntry{Kind: kind, Text: msg})
	if len(l.entries) > l.max {
		l.entries = l.entries[len(l.entries)-l.max:]
	}
}

// tail returns up to n most recent lines, oldest first, as a fresh slice
// (safe to hand to another goroutine).
func (l *eventLog) tail(n int) []LogEntry {
	if n > len(l.entries) {
		n = len(l.entries)
	}
	out := make([]LogEntry, n)
	copy(out, l.entries[len(l.entries)-n:])
	return out
}

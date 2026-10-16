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

// LogEntry is one retained colony-log line: a type and the sentence, when
// it happened, and where it falls in the whole game's log.
type LogEntry struct {
	Kind LogKind
	Text string
	// Tick is the tick the line was logged on.
	Tick int
	// Seq numbers every line the game has ever logged, from 0, so a reader
	// holding older lines can tell which in a snapshot are new: the ring
	// drops lines from the front and says nothing about it.
	Seq int
	// At is the tile the line is about, when HasAt: where a reader clicking
	// the line is taken. A line about the colony as a whole (a recruiter, an
	// order book change, a refused command) has none.
	At    Point
	HasAt bool
}

// eventLog is a fixed-size ring buffer of colony-log lines. Snapshots
// publish the whole buffer. The map sidebar shows the tail that fits; the
// log tab shows all of it, with Kind as its own column.
type eventLog struct {
	entries []LogEntry
	max     int
	next    int // the Seq of the next line
}

func newEventLog(max int) *eventLog {
	if max < 1 {
		max = 1
	}
	return &eventLog{max: max}
}

// add appends a line logged at tick, trimming the oldest entries past the cap.
func (l *eventLog) add(tick int, kind LogKind, msg string, at Point, hasAt bool) {
	l.entries = append(l.entries, LogEntry{Kind: kind, Text: msg, Tick: tick, Seq: l.next, At: at, HasAt: hasAt})
	l.next++
	if len(l.entries) > l.max {
		l.entries = l.entries[len(l.entries)-l.max:]
	}
}

// logEvent adds a line to the colony log, stamped with the current tick,
// about nowhere in particular. A line about something that happened at a
// place uses logEventAt, so the log can take the reader there.
func (w *World) logEvent(kind LogKind, msg string) { w.log.add(w.tick, kind, msg, Point{}, false) }

// logEventAt adds a line to the colony log about the tile at.
func (w *World) logEventAt(kind LogKind, at Point, msg string) {
	w.log.add(w.tick, kind, msg, at, true)
}

// rectSite is the middle of the rectangle x0,y0..x1,y1 on level: where a log
// line about an area points.
func rectSite(x0, y0, x1, y1 int, level Level) Point {
	return Point{(x0 + x1) / 2, (y0 + y1) / 2, level}
}

// site is the middle of what p builds on, on its first task's level, for a
// log line about it. It reports false for a project with no tasks.
func (p *project) site() (Point, bool) {
	if len(p.tasks) == 0 {
		return Point{}, false
	}
	first := p.tasks[0].pos
	lo, hi := first, first
	for _, t := range p.tasks {
		if t.pos.Level != first.Level {
			continue
		}
		lo.X, lo.Y = min(lo.X, t.pos.X), min(lo.Y, t.pos.Y)
		hi.X, hi.Y = max(hi.X, t.pos.X), max(hi.Y, t.pos.Y)
	}
	return rectSite(lo.X, lo.Y, hi.X, hi.Y, first.Level), true
}

// logEventProject adds a line about p, at its site when it has one.
func (w *World) logEventProject(kind LogKind, p *project, msg string) {
	if p == nil {
		w.logEvent(kind, msg)
		return
	}
	at, ok := p.site()
	w.log.add(w.tick, kind, msg, at, ok)
}

// roomSite is the middle of a room's inside.
func (r *roomRecord) site() Point { return rectSite(r.lo.X, r.lo.Y, r.hi.X, r.hi.Y, r.lo.Level) }

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

// lastProject is the project most recently queued, or nil: the room a
// successful planRoomFor just laid out.
func (w *World) lastProject() *project {
	if len(w.projects) == 0 {
		return nil
	}
	return w.projects[len(w.projects)-1]
}

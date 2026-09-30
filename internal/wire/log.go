package wire

import (
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The log topic: the colony log as a stream. The engine keeps only the last
// -log-size lines (64 by default); the page keeps more, so the topic sends
// just the lines it has not sent yet, told apart by LogEntry.Seq. The first
// send after subscribing, and after the log starts over (a new game), is the
// whole ring with Reset set, and the page replaces what it held.

// logEvery is how often the log is checked for new lines: often enough that
// the map's ticker reads as live.
const logEvery = 250 * time.Millisecond

// LogTopic is the lines since the last send. With Reset, it is every line
// the engine still holds, and the page drops what it had.
type LogTopic struct {
	Reset   bool       `json:"reset"`
	Entries []LogEntry `json:"entries"`
}

// LogEntry is one colony-log line.
type LogEntry struct {
	Seq  int    `json:"seq"`
	Tick int    `json:"tick"`
	Kind string `json:"kind"` // sim.LogKind's label: "death", "build complete", ...
	Text string `json:"text"`
}

// newLogTopic makes one subscription's log topic. Unlike the others it has
// state, what it last sent, so each Subscribe (and each Topics.Restart, on a
// new game) makes a fresh one that starts with the whole ring again.
func newLogTopic() topic {
	next := -1 // the Seq the page expects next; -1 until the first send
	return topic{every: logEvery, build: func(s *sim.Snapshot) any {
		log := s.Log
		t := LogTopic{Entries: []LogEntry{}}
		from := 0
		switch {
		case next < 0:
			t.Reset = true
		case len(log) > 0 && log[len(log)-1].Seq < next-1:
			// The newest line is older than one already sent: the log
			// started over without a Restart. Belt and braces.
			t.Reset = true
		default:
			for from < len(log) && log[from].Seq < next {
				from++
			}
		}
		for _, e := range log[from:] {
			t.Entries = append(t.Entries, LogEntry{Seq: e.Seq, Tick: e.Tick, Kind: e.Kind.String(), Text: e.Text})
		}
		if len(log) > 0 {
			next = log[len(log)-1].Seq + 1
		} else {
			next = 0
		}
		return t
	}}
}

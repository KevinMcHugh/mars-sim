package wire

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

func logLines(from, to int) []sim.LogEntry {
	var out []sim.LogEntry
	for s := from; s < to; s++ {
		out = append(out, sim.LogEntry{Seq: s, Tick: s * 10, Kind: sim.LogDeath, Text: "line"})
	}
	return out
}

func seqs(t *testing.T, raw json.RawMessage) (bool, []int) {
	t.Helper()
	var lt LogTopic
	if err := json.Unmarshal(raw, &lt); err != nil {
		t.Fatal(err)
	}
	var out []int
	for _, e := range lt.Entries {
		out = append(out, e.Seq)
	}
	return lt.Reset, out
}

// The log goes out whole once, then only what is new, even across the ring
// dropping lines; a new game (Restart) starts it over.
func TestLogTopicStreamsNewLines(t *testing.T) {
	snap := fixture(true)
	tp := NewTopics()
	if err := tp.Subscribe("log"); err != nil {
		t.Fatal(err)
	}
	at := time.Unix(0, 0)
	step := func(log []sim.LogEntry) (bool, []int) {
		t.Helper()
		snap.Log = log
		at = at.Add(time.Second)
		raw, ok := tp.Due(snap, at)["log"]
		if !ok {
			return false, nil
		}
		return seqs(t, raw)
	}

	if reset, got := step(logLines(0, 3)); !reset || len(got) != 3 || got[0] != 0 {
		t.Fatalf("first = %v %v, want the whole ring with reset", reset, got)
	}
	if reset, got := step(logLines(0, 5)); reset || len(got) != 2 || got[0] != 3 {
		t.Fatalf("second = %v %v, want 3 and 4", reset, got)
	}
	// The ring (of 4) dropped the front and took three more.
	if reset, got := step(logLines(4, 8)); reset || len(got) != 3 || got[0] != 5 || got[2] != 7 {
		t.Fatalf("after a drop = %v %v, want 5 to 7", reset, got)
	}

	var lt LogTopic
	snap.Log = logLines(7, 8)
	snap.Log[0].Kind = sim.LogBuildComplete
	tp.Restart()
	if err := json.Unmarshal(tp.Due(snap, at.Add(time.Millisecond))["log"], &lt); err != nil {
		t.Fatal(err)
	}
	if !lt.Reset || len(lt.Entries) != 1 || lt.Entries[0].Kind != "build complete" || lt.Entries[0].Tick != 70 {
		t.Errorf("after restart = %+v", lt)
	}
}

// A real game stamps lines with their tick and numbers them from 0.
func TestLogLinesCarryTickAndSeq(t *testing.T) {
	cfg := sim.DefaultConfig()
	cfg.Seed = 7
	cfg.Width, cfg.Height = 200, 200
	eng := sim.NewEngine(cfg)
	var snap *sim.Snapshot
	for i := 0; i < 400; i++ {
		if s, _ := eng.Advance(time.Millisecond); s != nil {
			snap = s
		}
	}
	if snap == nil || len(snap.Log) == 0 {
		t.Fatal("nothing logged")
	}
	for i, e := range snap.Log {
		if i > 0 && (e.Seq != snap.Log[i-1].Seq+1 || e.Tick < snap.Log[i-1].Tick) {
			t.Fatalf("line %d = %+v after %+v", i, e, snap.Log[i-1])
		}
	}
	if first := snap.Log[0]; len(snap.Log) < cfg.LogSize && first.Seq != 0 {
		t.Errorf("first line's seq = %d", first.Seq)
	}
}

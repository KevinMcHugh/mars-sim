package sim

import (
	"testing"
	"time"
)

func TestPerfRecorderBucketsTicks(t *testing.T) {
	var p perfRecorder
	t0 := time.Unix(1000, 0)
	p.advance(t0)
	p.record(time.Millisecond, time.Millisecond)
	p.record(3*time.Millisecond, time.Millisecond)
	p.advance(t0.Add(PerfBucket))

	got := p.samples()
	if len(got) != 1 {
		t.Fatalf("closed %d buckets, want 1", len(got))
	}
	s := got[0]
	if s.Ticks != 2 || s.MeanTick() != 3*time.Millisecond || s.MaxTick != 4*time.Millisecond || !s.Start.Equal(t0) {
		t.Errorf("sample = %+v, want 2 ticks, mean 3ms, max 4ms from t0", s)
	}
}

// A pause shows up as empty buckets, not as one bucket stretched across it.
func TestPerfRecorderKeepsIdleBuckets(t *testing.T) {
	var p perfRecorder
	t0 := time.Unix(1000, 0)
	p.advance(t0)
	p.record(time.Millisecond, 0)
	p.advance(t0.Add(4*PerfBucket + PerfBucket/2))

	got := p.samples()
	if len(got) != 4 {
		t.Fatalf("closed %d buckets, want 4", len(got))
	}
	for i, s := range got[1:] {
		if s.Ticks != 0 || !s.Start.Equal(t0.Add(time.Duration(i+1)*PerfBucket)) {
			t.Errorf("idle bucket %d = %+v", i+1, s)
		}
	}
}

// Published snapshots share the history, so closing a bucket must never
// write into an array a snapshot already holds.
func TestPerfHistoryIsNeverWrittenInPlace(t *testing.T) {
	var p perfRecorder
	t0 := time.Unix(1000, 0)
	p.advance(t0)
	for i := 1; i <= perfHistory+5; i++ {
		p.record(time.Duration(i), 0)
		p.advance(t0.Add(time.Duration(i) * PerfBucket))
		if i == 3 || i == perfHistory {
			held := p.samples()
			first, last := held[0], held[len(held)-1]
			p.advance(t0.Add(time.Duration(i+1) * PerfBucket))
			i++
			if held[0] != first || held[len(held)-1] != last {
				t.Fatalf("history shared with a snapshot changed underneath it at %d", i)
			}
		}
	}
	if n := len(p.samples()); n != perfHistory {
		t.Errorf("history holds %d samples, want it capped at %d", n, perfHistory)
	}
}

// A very long pause is capped rather than closing millions of empty buckets.
func TestPerfRecorderCapsLongGaps(t *testing.T) {
	var p perfRecorder
	t0 := time.Unix(1000, 0)
	p.advance(t0)
	p.advance(t0.Add(24 * time.Hour))
	if n := len(p.samples()); n > perfHistory {
		t.Errorf("history holds %d samples, want at most %d", n, perfHistory)
	}
	last := p.samples()[len(p.samples())-1]
	if want := t0.Add(24 * time.Hour); last.Start.Add(PerfBucket).After(want) || want.Sub(last.Start) > 2*PerfBucket {
		t.Errorf("last idle bucket starts %v, want just before %v", last.Start, want)
	}
}

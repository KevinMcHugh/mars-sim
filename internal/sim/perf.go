package sim

import "time"

// PerfBucket is the wall-clock width of one PerfSample. A quarter second is
// fine enough for a graph to show a stall as a stall, and coarse enough that a
// slow tick rate still lands a couple of ticks in most buckets.
const PerfBucket = 250 * time.Millisecond

// perfHistory is how many closed buckets the engine keeps: five minutes at
// PerfBucket. A frontend draws only the tail that fits on screen.
const perfHistory = 1200

// PerfSample is what the engine measured over one PerfBucket of wall-clock
// time. Samples are wall-clock measurements of the engine, not simulation
// state: they never feed back into the World, so they cannot disturb
// determinism (see docs/determinism.md).
type PerfSample struct {
	Start   time.Time     // when the bucket began
	Ticks   int           // ticks completed in the bucket
	Step    time.Duration // total time spent in World.step
	Publish time.Duration // total time spent building and posting snapshots
	MaxTick time.Duration // slowest single tick (step + publish)
}

// Busy is the bucket's total tick work: stepping plus publishing.
func (s PerfSample) Busy() time.Duration { return s.Step + s.Publish }

// MeanTick is the average cost of one tick in the bucket, or 0 if no tick ran.
func (s PerfSample) MeanTick() time.Duration {
	if s.Ticks == 0 {
		return 0
	}
	return s.Busy() / time.Duration(s.Ticks)
}

// perfRecorder accumulates tick timings into PerfBucket-wide samples. It is
// owned by the engine goroutine; only its closed history leaves it, via
// Snapshot.Perf.
type perfRecorder struct {
	cur  PerfSample
	hist []PerfSample
}

// advance closes every bucket that ended by now. Buckets with no ticks in them
// (the engine was paused, or a single tick ran longer than a bucket) are kept
// as zero-tick samples so the graph shows the gap rather than hiding it.
func (p *perfRecorder) advance(now time.Time) {
	if p.cur.Start.IsZero() {
		p.cur.Start = now
		return
	}
	end := p.cur.Start.Add(PerfBucket)
	if now.Before(end) {
		return
	}
	p.close(p.cur)
	// Skip straight past a long idle stretch instead of closing one empty
	// bucket at a time: at most perfHistory of them can be shown anyway.
	gap := int(now.Sub(end) / PerfBucket)
	if gap > perfHistory {
		end = end.Add(time.Duration(gap-perfHistory) * PerfBucket)
		gap = perfHistory
	}
	for i := 0; i < gap; i++ {
		p.close(PerfSample{Start: end})
		end = end.Add(PerfBucket)
	}
	p.cur = PerfSample{Start: end}
}

// close appends a finished sample to the history.
//
// The history is shared with every Snapshot published since the last close, so
// it must never be written in place. Slicing it with a capacity equal to its
// length forces append to copy into a fresh array: published snapshots keep the
// old one untouched, and the engine pays one short copy four times a second
// rather than one per published frame.
func (p *perfRecorder) close(s PerfSample) {
	h := p.hist
	if len(h) >= perfHistory {
		h = h[len(h)-perfHistory+1:]
	}
	p.hist = append(h[:len(h):len(h)], s)
}

// record adds one tick's timings to the open bucket.
func (p *perfRecorder) record(step, publish time.Duration) {
	p.cur.Ticks++
	p.cur.Step += step
	p.cur.Publish += publish
	p.cur.MaxTick = max(p.cur.MaxTick, step+publish)
}

// samples returns the closed history, oldest first. Callers must not modify
// it (see close).
func (p *perfRecorder) samples() []PerfSample { return p.hist }

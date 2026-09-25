package tui

import (
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
	"github.com/kevinmchugh/mars-sim/internal/ui/tui/cells"
)

func TestBrailleDotsLandInTheRightCell(t *testing.T) {
	b := newBraille(2, 1)
	b.set(0, 0) // top-left dot of the first cell: dot 1
	b.set(3, 3) // bottom-right dot of the second cell: dot 8
	if got, want := b.row(0), "⠁⢀"; got != want {
		t.Errorf("row = %q, want %q", got, want)
	}
	if w := cells.Width(b.row(0)); w != 2 {
		t.Errorf("two braille cells measured %d wide, want 2", w)
	}
}

func TestTPSSeriesAveragesOverOneSecond(t *testing.T) {
	var samples []sim.PerfSample
	for i := 0; i < 8; i++ {
		samples = append(samples, sim.PerfSample{Ticks: 2}) // 2 ticks per 250ms = 8 tps
	}
	for i, v := range tpsSeries(samples) {
		if v != 8 {
			t.Errorf("point %d = %v tps, want 8", i, v)
		}
	}
}

func TestMsSeriesLeavesIdleBucketsAsGaps(t *testing.T) {
	got := msSeries([]sim.PerfSample{
		{Ticks: 2, Step: 3 * time.Millisecond, Publish: time.Millisecond},
		{}, // paused
	})
	if got[0] != 2 {
		t.Errorf("mean tick = %vms, want 2", got[0])
	}
	if !math.IsNaN(got[1]) {
		t.Errorf("idle bucket = %v, want a gap (NaN)", got[1])
	}
}

func TestPerfScreenWithoutSamples(t *testing.T) {
	m := New(nil, nil)
	m.termW, m.termH = 100, 30
	m.latest = busySnapshot()
	m.latest.Perf = nil
	m.mode = modePerf
	if frame := m.renderFrame(); !strings.Contains(frame, "collecting samples") {
		t.Errorf("expected a placeholder before the first sample, got:\n%s", frame)
	}
}

func TestPerfScreenDrawsBothCharts(t *testing.T) {
	m := New(nil, nil)
	m.termW, m.termH = 120, 40
	m.latest = busySnapshot()
	m.mode = modePerf
	frame := m.renderFrame()
	for _, want := range []string{"TICKS/SEC", "MS/TICK", "target 8", "budget 125.0ms", "└"} {
		if !strings.Contains(frame, want) {
			t.Errorf("perf screen is missing %q:\n%s", want, frame)
		}
	}
	// The window shows the newest samples, so the axis ends one window after
	// the first sample it shows.
	if !regexp.MustCompile(`\d\d:\d\d:\d\d`).MatchString(frame) {
		t.Error("perf screen drew no time axis labels")
	}
	if !strings.ContainsFunc(frame, func(r rune) bool { return r > 0x2800 && r <= 0x28FF }) {
		t.Error("perf screen drew no braille dots")
	}
}

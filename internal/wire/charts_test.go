package wire

import (
	"testing"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The chart topics carry the snapshot's histories column by column.
func TestChartTopics(t *testing.T) {
	snap := colony(t, 2000)

	var perf PerfTopic
	due(t, snap, "perf", &perf)
	if len(perf.Ticks) != len(snap.Perf) || len(perf.BusyMs) != len(perf.Ticks) || len(perf.MaxMs) != len(perf.Ticks) {
		t.Fatalf("perf columns %d/%d/%d, %d samples", len(perf.Ticks), len(perf.BusyMs), len(perf.MaxMs), len(snap.Perf))
	}
	if perf.BucketMs != int(sim.PerfBucket/time.Millisecond) || perf.TPS != snap.TicksPerSecond {
		t.Errorf("perf = bucket %d, tps %d", perf.BucketMs, perf.TPS)
	}
	ticks := 0
	for _, n := range perf.Ticks {
		ticks += n
	}
	if len(snap.Perf) > 0 && (ticks == 0 || perf.Start != snap.Perf[0].Start.UnixMilli()) {
		t.Errorf("perf: %d ticks, start %d", ticks, perf.Start)
	}

	var pop PopulationTopic
	due(t, snap, "population", &pop)
	n := len(snap.Population)
	if n == 0 || len(pop.Tick) != n || len(pop.Activities) != int(sim.NumActivities) {
		t.Fatalf("population: %d points of %d, %d activities", len(pop.Tick), n, len(pop.Activities))
	}
	last := snap.Population[n-1]
	if pop.Tick[n-1] != last.Tick || pop.Colonists[n-1] != last.Colonists || pop.ColonySize[n-1] != last.ColonySize {
		t.Errorf("last point = tick %d, %d colonists", pop.Tick[n-1], pop.Colonists[n-1])
	}
	for a := range sim.NumActivities {
		if pop.Activities[a] != a.String() || pop.Activity[a][n-1] != last.Activity[a] || pop.Walking[a][n-1] != last.Walking[a] {
			t.Errorf("activity %s mismatched", a)
		}
	}
}

// The chart builder's catalog lists every metric and every series with
// readings, and a series topic carries one series' readings with its ticks
// and clock hours, aligned.
func TestMetricsAndSeriesTopics(t *testing.T) {
	snap := colony(t, 2000)

	var cat MetricsTopic
	due(t, snap, "metrics", &cat)
	if len(cat.Metrics) != len(sim.MetricDefs()) || len(cat.Series) == 0 {
		t.Fatalf("catalog: %d metrics, %d series", len(cat.Metrics), len(cat.Series))
	}
	keys := map[string]bool{}
	for _, s := range cat.Series {
		keys[s.Key] = true
		if s.Metric < 0 || s.Metric >= len(cat.Metrics) {
			t.Fatalf("series %s names metric %d", s.Key, s.Metric)
		}
	}
	for _, k := range []string{"colonists/", "balance/treasury", "money-moved/", "open-bids/all"} {
		if !keys[k] {
			t.Errorf("catalog has no %q series", k)
		}
	}

	var col SeriesTopic
	due(t, snap, "series:colonists/", &col)
	if !col.Found || len(col.Values) == 0 || len(col.Tick) != len(col.Values) || len(col.Hour) != len(col.Values) || col.Every < 1 {
		t.Fatalf("colonists series: found %v, %d values, %d ticks, %d hours, every %d",
			col.Found, len(col.Values), len(col.Tick), len(col.Hour), col.Every)
	}
	if last := col.Values[len(col.Values)-1]; last <= 0 {
		t.Errorf("colonists series ends at %d", last)
	}

	var none SeriesTopic
	due(t, snap, "series:price/no-such-good", &none)
	if none.Found || none.Values == nil {
		t.Errorf("an unknown series: %+v, want not found with empty columns", none)
	}
}

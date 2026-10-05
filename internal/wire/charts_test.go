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
	if len(pop.FixtureKinds) != len(sim.TrackedFixtures) || len(pop.FixtureCounts) != len(pop.FixtureKinds) {
		t.Fatalf("%d fixture kinds, %d series", len(pop.FixtureKinds), len(pop.FixtureCounts))
	}
	for f, kind := range sim.TrackedFixtures {
		if pop.FixtureKinds[f] != kind.String() || pop.FixtureCounts[f][n-1] != last.FixtureKinds[kind] {
			t.Errorf("fixture kind %s mismatched", kind)
		}
	}
	if len(pop.Skills) != len(sim.Skills()) {
		t.Fatalf("%d skills, want %d", len(pop.Skills), len(sim.Skills()))
	}
	for j, k := range sim.Skills() {
		sr := pop.Skills[j]
		total := 0
		for r := range sr.Ranks {
			if sr.Ranks[r][n-1] != last.SkillRanks[k][r] {
				t.Errorf("%s rank %d mismatched", k, r)
			}
			total += sr.Ranks[r][n-1]
		}
		if sr.Skill != k.String() || len(sr.Labels) != len(sr.Ranks) || total != last.Colonists {
			t.Errorf("%s: %d labels, %d ranks, %d colonists of %d", sr.Skill, len(sr.Labels), len(sr.Ranks), total, last.Colonists)
		}
	}
}

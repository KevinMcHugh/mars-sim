package wire

import (
	"math"
	"time"

	"github.com/kevinmchugh/mars-sim/internal/sim"
)

// The Charts tab's topics: "perf" (the engine's recent timing, as the TUI's
// Perf screen draws it; docs/perf-screen.md) and "population" (the colony's
// vital signs and activity over the whole game, as the Population and
// Activity screens draw them). Both are columns of numbers, one array per
// series, since that is what a chart library plots and it keeps the JSON a
// list of numbers rather than of objects.
const chartEvery = time.Second

// PerfTopic is the engine's timing history, oldest first, one point per
// sim.PerfBucket of wall-clock time. A bucket with no ticks (paused) is
// kept, so the chart shows the gap.
type PerfTopic struct {
	BucketMs int   `json:"bucketMs"`
	Start    int64 `json:"start"` // the first bucket's start, Unix milliseconds
	// TPS is the speed the engine is set to, for the chart's reference line.
	TPS    int       `json:"tps"`
	Ticks  []int     `json:"ticks"`  // ticks completed in each bucket
	BusyMs []float64 `json:"busyMs"` // time spent stepping and publishing
	MaxMs  []float64 `json:"maxMs"`  // the slowest single tick
}

func perfTopic(s *sim.Snapshot) PerfTopic {
	t := PerfTopic{BucketMs: int(sim.PerfBucket / time.Millisecond), TPS: s.TicksPerSecond,
		Ticks: make([]int, len(s.Perf)), BusyMs: make([]float64, len(s.Perf)), MaxMs: make([]float64, len(s.Perf))}
	if len(s.Perf) > 0 {
		t.Start = s.Perf[0].Start.UnixMilli()
	}
	for i, p := range s.Perf {
		t.Ticks[i] = p.Ticks
		t.BusyMs[i] = ms3(p.Busy())
		t.MaxMs[i] = ms3(p.MaxTick)
	}
	return t
}

// ms3 is d in milliseconds to the microsecond: enough for a chart, and it
// keeps the JSON short.
func ms3(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Microsecond)) / 1000
}

// PopulationTopic is the colony over the whole game, oldest first: one point
// per sim.PopulationSample. The engine halves the resolution as the game
// grows (docs/population-screen.md), so it is at most a few hundred points.
type PopulationTopic struct {
	Tick       []int `json:"tick"`
	Colonists  []int `json:"colonists"`
	Meals      []int `json:"meals"`
	ColonySize []int `json:"colonySize"`
	Fixtures   []int `json:"fixtures"`
	// Activities names each activity (sim.Activity's String), indexing
	// Activity and Walking: Activity[a][i] is the colonist-ticks spent on
	// activity a since the sample before i, and Walking[a][i] the part of
	// them spent walking there. See docs/activity-screen.md.
	Activities []string `json:"activities"`
	Activity   [][]int  `json:"activity"`
	Walking    [][]int  `json:"walking"`
	// FixtureKinds names each kind of fixture tracked (sim.TrackedFixtures),
	// indexing FixtureCounts: FixtureCounts[f][i] is how many stood at
	// sample i.
	FixtureKinds  []string `json:"fixtureKinds"`
	FixtureCounts [][]int  `json:"fixtureCounts"`
	// Skills is every skill's colonists by rank.
	Skills []SkillRanks `json:"skills"`
}

// SkillRanks is one skill's colonists by rank over the game: Ranks[r][i] is
// how many living colonists stood at exactly rank r at sample i. Labels has
// one title per rank, "" (untrained) first; a title may cover more than one
// rank.
type SkillRanks struct {
	Skill  string   `json:"skill"`
	Labels []string `json:"labels"`
	Ranks  [][]int  `json:"ranks"`
}

func populationTopic(s *sim.Snapshot) PopulationTopic {
	n := len(s.Population)
	t := PopulationTopic{
		Tick: make([]int, n), Colonists: make([]int, n), Meals: make([]int, n),
		ColonySize: make([]int, n), Fixtures: make([]int, n),
		Activities: make([]string, sim.NumActivities),
		Activity:   make([][]int, sim.NumActivities),
		Walking:    make([][]int, sim.NumActivities),
	}
	for a := range sim.NumActivities {
		t.Activities[a] = a.String()
		t.Activity[a] = make([]int, n)
		t.Walking[a] = make([]int, n)
	}
	t.FixtureKinds = make([]string, len(sim.TrackedFixtures))
	t.FixtureCounts = make([][]int, len(sim.TrackedFixtures))
	for f, kind := range sim.TrackedFixtures {
		t.FixtureKinds[f] = kind.String()
		t.FixtureCounts[f] = make([]int, n)
		for i, p := range s.Population {
			t.FixtureCounts[f][i] = p.FixtureKinds[kind]
		}
	}
	for _, k := range sim.Skills() {
		sr := SkillRanks{Skill: k.String(), Labels: sim.SkillRankLabels(k)}
		sr.Ranks = make([][]int, len(sr.Labels))
		for r := range sr.Ranks {
			sr.Ranks[r] = make([]int, n)
			for i, p := range s.Population {
				sr.Ranks[r][i] = p.SkillRanks[k][r]
			}
		}
		t.Skills = append(t.Skills, sr)
	}
	for i, p := range s.Population {
		t.Tick[i], t.Colonists[i], t.Meals[i], t.ColonySize[i], t.Fixtures[i] =
			p.Tick, p.Colonists, p.Meals, p.ColonySize, p.Fixtures
		for a := range sim.NumActivities {
			t.Activity[a][i] = p.Activity[a]
			t.Walking[a][i] = p.Walking[a]
		}
	}
	return t
}

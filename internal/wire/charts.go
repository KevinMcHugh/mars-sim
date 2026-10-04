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

// ---- the chart builder ------------------------------------------------------
//
// Everything the sim measures (sim.MetricsView; docs/charts.md) goes out in
// two tiers, so a chart costs what it plots, not what could be plotted: the
// "metrics" topic is the catalog the picker lists (every metric and every
// series, no readings), and "series:<key>" is one series' readings. The
// catalog changes only when a series starts or ends, so after its first send
// it is rarely sent again; a colony of hundreds has hundreds of wallets, and
// only the ones on a chart are ever sent.

// MetricsTopic is the catalog: what can be charted.
type MetricsTopic struct {
	Metrics []MetricLine `json:"metrics"`
	Series  []SeriesLine `json:"series"`
}

// MetricLine is one metric (sim.MetricDef).
type MetricLine struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Group string `json:"group"`
	Doc   string `json:"doc"`
	Kind  string `json:"kind"` // "level" or "total" (a running total since the landing)
	Unit  string `json:"unit"` // "count" or "dollars"
	Per   string `json:"per"`  // "colony", "item" or "account"
}

// SeriesLine is one series the picker can offer: Metric indexes Metrics.
type SeriesLine struct {
	Key     string `json:"key"`
	Metric  int    `json:"metric"`
	Subject string `json:"subject,omitempty"`
	Ended   bool   `json:"ended,omitempty"`
}

func metricsTopic(s *sim.Snapshot) MetricsTopic {
	defs := sim.MetricDefs()
	t := MetricsTopic{Metrics: make([]MetricLine, len(defs))}
	for i, d := range defs {
		t.Metrics[i] = MetricLine{Key: d.Key, Label: d.Label, Group: d.Group, Doc: d.Doc,
			Kind: d.Kind.String(), Unit: d.Unit.String(), Per: d.Per.String()}
	}
	if s.Metrics == nil {
		t.Series = []SeriesLine{}
		return t
	}
	t.Series = make([]SeriesLine, 0, len(s.Metrics.Series))
	for _, x := range s.Metrics.Series {
		if len(x.Values) > 0 {
			t.Series = append(t.Series, SeriesLine{Key: x.Key, Metric: x.Metric, Subject: x.Subject, Ended: x.Ended})
		}
	}
	return t
}

// SeriesTopic is one series' readings, oldest first, with the tick and the
// clock hour of each (hours since the landing day's midnight: day
// hour/24+1). Every is the hours between samples, which a frontend's
// buckets cannot be finer than. Found is false for a key the sim has no
// readings for (yet): a saved chart from another game, or a good nobody has
// traded.
type SeriesTopic struct {
	Key    string  `json:"key"`
	Found  bool    `json:"found"`
	Every  int     `json:"every"`
	Tick   []int   `json:"tick"`
	Hour   []int   `json:"hour"`
	Values []int64 `json:"values"`
}

func seriesTopic(s *sim.Snapshot, key string) SeriesTopic {
	t := SeriesTopic{Key: key, Tick: []int{}, Hour: []int{}, Values: []int64{}}
	m := s.Metrics
	if m == nil {
		return t
	}
	t.Every = m.Every
	for _, x := range m.Series {
		if x.Key != key || len(x.Values) == 0 {
			continue
		}
		end := x.Start + len(x.Values)
		t.Found, t.Values = true, x.Values
		t.Tick, t.Hour = m.Ticks[x.Start:end], m.Hours[x.Start:end]
		break
	}
	return t
}

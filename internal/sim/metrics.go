package sim

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// ---- Metrics: the chart system's history ---------------------------------------
//
// Everything a player might want to chart (an account's balance, a good's
// price, how much money changed hands, how many bids are open) is a metric: a
// row in metricDefs saying what it is and how to read it. Once an hour on the
// colony clock the world reads every metric for every subject it covers (each
// item kind, each account) and appends the readings to that series' history.
// A frontend picks any of them to chart. See docs/charts.md.
//
// Like the Population history it is read-only bookkeeping: nothing in the
// simulation reads it back, so it cannot disturb determinism. Unlike it, the
// set of series is open-ended (a series per colonist's wallet), so it is kept
// per series, each starting at the sample its subject first had a value.
//
// Two kinds of reading cover everything asked for so far:
//   - a Gauge is a value at a moment: a balance, a price, open bids;
//   - a Total is a running total since the landing: dollars that changed
//     hands, bids posted. The amount in any span of time is the difference
//     between two readings, so a frontend can bucket it by hour, half-day or
//     day without the sim choosing the buckets, and halving the history
//     (dropping every other sample) loses no money: the totals still add up.
//
// Totals are read from plain running counters on World (moneyMoved,
// posted, traded…) that the hot paths bump with one add, so counting costs
// nothing beyond the increment; the metric code only runs once an hour.

// MetricKind says how to read a series.
type MetricKind uint8

const (
	// Gauge is a reading at the sample's moment. (Not "Level": that is a
	// depth in the world; see layer.go.)
	Gauge MetricKind = iota
	// Total is a running total since the landing: what happened in a span is
	// the difference between the readings at its ends. Before its first
	// reading a Total was zero.
	Total
)

func (k MetricKind) String() string {
	if k == Total {
		return "total"
	}
	return "level"
}

// MetricUnit is what a series counts.
type MetricUnit uint8

const (
	UnitCount MetricUnit = iota
	UnitDollars
)

func (u MetricUnit) String() string {
	if u == UnitDollars {
		return "dollars"
	}
	return "count"
}

// MetricPer is the set of subjects a metric is read for: once for the
// colony, once per item kind (plus all goods together), once per account
// (the treasury and each living colonist), once per kind of fixture
// (metricFixtures), or once per rank of each skill (metricSkillRanks).
type MetricPer uint8

const (
	PerColony MetricPer = iota
	PerItem
	PerAccount
	PerFixture
	PerSkillRank
)

func (p MetricPer) String() string {
	switch p {
	case PerItem:
		return "item"
	case PerAccount:
		return "account"
	case PerFixture:
		return "fixture"
	case PerSkillRank:
		return "skill-rank"
	default:
		return "colony"
	}
}

// MetricDef is one measurable thing.
type MetricDef struct {
	Key   string // stable, for series keys and saved charts: "price"
	Label string // "Price"
	Group string // what the picker files it under: "Market"
	Doc   string // one line for the picker
	Kind  MetricKind
	Unit  MetricUnit
	Per   MetricPer
	// read is the metric's value for one subject (an ItemKind, an account's
	// EntityID with 0 for the treasury, or 0 for a colony metric), and
	// whether it has one. A subject with no value gets no series yet; a
	// series whose subject loses its value (a colonist dies) ends there.
	read func(c *metricCtx, subject int64) (int64, bool)
}

// metricDefs is every metric, in the order the picker lists them. Adding a
// metric is a row here (and, for a Total, a counter the hot path bumps):
// series, history, snapshot and wire all follow from it. Keys are saved in
// players' charts, so rename one only with a reason.
var metricDefs = []MetricDef{
	// Colony
	{Key: "colonists", Label: "Colonists", Group: "Colony", Doc: "living colonists",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(c.colonists), true }},
	{Key: "colony-size", Label: "Colony size", Group: "Colony", Doc: "floor tiles dug or discovered",
		read: func(c *metricCtx, _ int64) (int64, bool) {
			return int64(c.w.countTerrain(Floor) - c.w.hiddenFloor()), true
		}},
	{Key: "fixtures", Label: "Fixtures", Group: "Colony", Doc: "placed fixtures: bunks, toilets, chests, workshops…",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(len(c.w.fixtures)), true }},
	{Key: "deaths", Label: "Deaths", Group: "Colony", Kind: Total, Doc: "colonists who have died",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(len(c.w.deceasedColonists)), true }},
	{Key: "starved", Label: "Starved", Group: "Colony", Kind: Total, Doc: "colonists who starved to death",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(c.w.starved), true }},
	{Key: "fixture-kind", Label: "Fixtures of a kind", Group: "Colony", Per: PerFixture,
		Doc:  "placed fixtures of one kind: scumhouses, scum incubators, beds…",
		read: func(c *metricCtx, t int64) (int64, bool) { return int64(c.w.countTerrain(Terrain(t))), true }},
	{Key: "stock", Label: "In storage", Group: "Colony", Per: PerItem,
		Doc:  "units in any depot (chests, lockers, scumhouses, the silo), whoever owns them",
		read: func(c *metricCtx, k int64) (int64, bool) { return c.stock[k], true }},

	// Skills
	{Key: "skill-rank", Label: "Colonists at a skill rank", Group: "Skills", Per: PerSkillRank,
		Doc:  "living colonists at exactly one rank of a skill (rank 0: untrained)",
		read: func(c *metricCtx, sr int64) (int64, bool) { k, r := skillRankOf(sr); return c.ranks[k][r], true }},
	{Key: "skill-rank-up", Label: "Colonists at a skill rank or better", Group: "Skills", Per: PerSkillRank,
		Doc: "living colonists at one rank of a skill or higher: a chef or better",
		read: func(c *metricCtx, sr int64) (int64, bool) {
			k, r := skillRankOf(sr)
			if r == 0 {
				return 0, false // every colonist: the Colonists metric
			}
			n := int64(0)
			for _, x := range c.ranks[k][r:] {
				n += x
			}
			return n, true
		}},

	// Money
	{Key: "balance", Label: "Balance", Group: "Money", Unit: UnitDollars, Per: PerAccount,
		Doc: "what an account holds: the treasury or a colonist's wallet",
		read: func(c *metricCtx, id int64) (int64, bool) {
			if id == 0 {
				return int64(c.w.treasury), true
			}
			if e := c.w.entities[EntityID(id)]; e != nil && e.Kind == Colonist {
				return int64(e.wallet), true
			}
			return 0, false
		}},
	{Key: "circulating", Label: "In circulation", Group: "Money", Unit: UnitDollars,
		Doc:  "the treasury plus every living colonist's wallet",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(c.w.moneyInCirculation()), true }},
	{Key: "escrowed", Label: "In escrow", Group: "Money", Unit: UnitDollars,
		Doc:  "held by open bids and work orders",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(c.w.moneyEscrowed()), true }},
	{Key: "money-moved", Label: "Money changed hands", Group: "Money", Kind: Total, Unit: UnitDollars,
		Doc:  "dollars paid from one party to another: trades, wages, taxes (not escrow or refunds)",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(c.w.moneyMoved), true }},
	{Key: "payments", Label: "Payments", Group: "Money", Kind: Total,
		Doc:  "payments from one party to another, however large",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(c.w.payments), true }},
	{Key: "tax", Label: "Tax collected", Group: "Money", Kind: Total, Unit: UnitDollars,
		Doc:  "the wealth levy paid to the treasury",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(c.w.taxCollected), true }},
	{Key: "exported", Label: "Paid off-world", Group: "Money", Kind: Total, Unit: UnitDollars,
		Doc:  "recruiter fees and passage",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(c.w.moneyExported), true }},

	// Market
	{Key: "price", Label: "Price", Group: "Market", Unit: UnitDollars, Per: PerItem,
		Doc: "the smoothed trade price, once the good has traded",
		read: func(c *metricCtx, k int64) (int64, bool) {
			if k == int64(ItemNone) || !c.w.prices[k].traded {
				return 0, false
			}
			return int64(c.w.valueOf(ItemKind(k))), true
		}},
	{Key: "open-bids", Label: "Open bids", Group: "Market", Per: PerItem, Doc: "bids on the book",
		read: func(c *metricCtx, k int64) (int64, bool) { return c.open[Bid][k], true }},
	{Key: "open-asks", Label: "Open asks", Group: "Market", Per: PerItem, Doc: "asks on the book",
		read: func(c *metricCtx, k int64) (int64, bool) { return c.open[Ask][k], true }},
	{Key: "bids-posted", Label: "Bids posted", Group: "Market", Kind: Total, Per: PerItem,
		Doc:  "bids placed, filled at once or not",
		read: func(c *metricCtx, k int64) (int64, bool) { return c.w.posted[Bid][k], true }},
	{Key: "asks-posted", Label: "Asks posted", Group: "Market", Kind: Total, Per: PerItem,
		Doc:  "asks placed, filled at once or not",
		read: func(c *metricCtx, k int64) (int64, bool) { return c.w.posted[Ask][k], true }},
	{Key: "traded", Label: "Units traded", Group: "Market", Kind: Total, Per: PerItem,
		Doc:  "units that changed hands on the book",
		read: func(c *metricCtx, k int64) (int64, bool) { return c.w.tradedUnits[k], true }},
	{Key: "turnover", Label: "Trade value", Group: "Market", Kind: Total, Unit: UnitDollars, Per: PerItem,
		Doc:  "dollars paid for goods on the book",
		read: func(c *metricCtx, k int64) (int64, bool) { return c.w.tradedValue[k], true }},
	{Key: "work-orders", Label: "Open work orders", Group: "Market", Doc: "paid jobs waiting for a worker",
		read: func(c *metricCtx, _ int64) (int64, bool) { return int64(len(c.w.workOrders)), true }},
}

// MetricDefs is every metric, for a frontend's picker. It is shared: do not
// modify it.
func MetricDefs() []MetricDef { return metricDefs }

// metricCtx is what several metrics read, worked out once per sample rather
// than once per series. Item-indexed arrays hold the all-goods sum at
// ItemNone.
type metricCtx struct {
	w         *World
	colonists int
	accounts  []int64 // 0 (the treasury), then living colonists by ID
	stock     [numItemKinds]int64
	open      [2][numItemKinds]int64 // open orders per side
	// ranks counts living colonists by their rank in each skill.
	ranks [numSkills][maxSkillRanks]int64
}

// newMetricCtx fills the store's scratch context for this sample. It is
// reused, not allocated, so a sample allocates only what it keeps.
func (w *World) newMetricCtx() *metricCtx {
	c := &w.metrics.ctx
	*c = metricCtx{w: w, accounts: append(c.accounts[:0], 0)}
	for id, e := range w.entities {
		if e.Kind == Colonist {
			c.colonists++
			c.accounts = append(c.accounts, int64(id))
			for k := SkillKind(1); k < numSkills; k++ {
				c.ranks[k][e.rank(k)]++
			}
		}
	}
	slices.Sort(c.accounts)
	for _, s := range w.storageContainers {
		for _, st := range s.Inventory {
			if st.Kind != ItemNone && st.Count > 0 {
				c.stock[st.Kind] += int64(st.Count)
				c.stock[ItemNone] += int64(st.Count)
			}
		}
	}
	for _, o := range w.orders {
		c.open[o.Side][o.Item]++
		c.open[o.Side][ItemNone]++
	}
	return c
}

// metricColony and metricItems are the subjects of a colony metric and of a
// per-item one (ItemNone standing for all goods).
var (
	metricColony = [1]int64{0}
	metricItems  = func() (ks [numItemKinds]int64) {
		for k := range ks {
			ks[k] = int64(k)
		}
		return ks
	}()
)

// metricFixtures is every kind of placed structure counted per kind, in the
// order the picker lists them: each fixture kind (isFixtureTerrain) and the
// meeting hall's chairs, which are furniture, so not in the "fixtures" count.
var metricFixtures = func() []int64 {
	var out []int64
	for _, t := range []Terrain{Bed, Toilet, NutrientPod, Storage, Scumhouse, Incubator, Trough,
		Forge, GunBench, Incinerator, Chair} {
		out = append(out, int64(t))
	}
	return out
}()

// maxSkillRanks bounds every skill's ranks, untrained included: the length
// of the longest skillSpec.Labels (mining's).
const maxSkillRanks = 9

// metricSkillRanks is every rank of every skill as a subject (skillRankSubject),
// in skill then rank order.
var metricSkillRanks = func() []int64 {
	var out []int64
	for k := SkillKind(1); k < numSkills; k++ {
		for r := range skillSpecs[k].Labels {
			out = append(out, skillRankSubject(k, r))
		}
	}
	return out
}()

// skillRankSubject packs a skill and a rank into one subject, and
// skillRankOf unpacks it.
func skillRankSubject(k SkillKind, r int) int64 { return int64(k)<<8 | int64(r) }
func skillRankOf(sr int64) (SkillKind, int)     { return SkillKind(sr >> 8), int(sr & 0xff) }

// metricHistory is the most samples kept before the resolution halves, as
// with the Population history: the chart always spans the whole game.
const metricHistory = 512

// metricKey names a series: a metric and a subject.
type metricKey struct {
	def     uint16
	subject int64
}

// metricSeries is one series' history: values from sample start onward.
type metricSeries struct {
	metricKey
	key   string // "price/meal": the metric's key and the subject's
	label string // the subject's name: "meal", "Treasury", a colonist's name
	start int
	vals  []int64
	ended bool
}

// metricStore is the whole history. ticks and hours are the shared time
// axis; every series indexes into it from its start.
type metricStore struct {
	every  int   // clock hours between samples: 1, then 2, 4, 12, 24, 48… as the history halves
	ticks  []int // the tick of each sample
	hours  []int // the clock hour of each sample, counted from the landing day's midnight
	series []metricSeries
	index  map[metricKey]int32
	// view is what snapshots publish, rebuilt once per sample and shared
	// by every snapshot until the next. It aliases the slices above at
	// shorter lengths, which the save codec refuses, and is cheap to
	// rebuild, so it is not saved.
	view *MetricsView `save:"-"`
	// ctx is the per-sample scratch (newMetricCtx): nothing in it outlives
	// a sample.
	ctx metricCtx `save:"-"`
}

// clockHour is how many clock hours tick is past the landing day's
// midnight: the hour MinuteOfDay is in, counted across days.
func clockHour(tick, ticksPerDay int) int {
	ticksPerDay = max(ticksPerDay, 1)
	return (tick + landingOffset(ticksPerDay)) * 24 / ticksPerDay
}

// sampleMetrics takes a sample on the first tick of each sampling interval's
// clock hour. Sampling on the clock, not every N ticks, keeps every sample on
// an hour boundary, so a frontend's half-day and day buckets line up exactly
// with the day the top bar shows.
func (w *World) sampleMetrics() {
	m := &w.metrics
	if m.every == 0 {
		m.every = 1
	}
	tpd := w.cfg.TicksPerDay()
	h, prev := clockHour(w.tick, tpd), clockHour(w.tick-1, tpd)
	if w.tick <= 0 || h/m.every == prev/m.every {
		return
	}
	h -= h % m.every // the grid line crossed (a day under 24 ticks can skip one)
	if len(m.ticks) >= metricHistory {
		m.halve()
		if h%m.every != 0 {
			m.view = nil
			return
		}
	}
	if m.index == nil {
		m.index = map[metricKey]int32{}
	}
	n := len(m.ticks) // this sample's index
	m.ticks = append(m.ticks, w.tick)
	m.hours = append(m.hours, h)
	c := w.newMetricCtx()
	for d := range metricDefs {
		def := &metricDefs[d]
		subjects := metricColony[:]
		switch def.Per {
		case PerItem:
			subjects = metricItems[:]
		case PerAccount:
			subjects = c.accounts
		case PerFixture:
			subjects = metricFixtures
		case PerSkillRank:
			subjects = metricSkillRanks
		}
		for _, subj := range subjects {
			v, ok := def.read(c, subj)
			key := metricKey{uint16(d), subj}
			i, have := m.index[key]
			if !have {
				// A per-item, per-fixture or per-rank series waits for its
				// first non-zero reading, so goods nobody has touched,
				// fixtures never built and ranks nobody has reached add
				// nothing to the picker.
				if !ok || (v == 0 && waitsForReading(def.Per, subj)) {
					continue
				}
				i = int32(len(m.series))
				m.index[key] = i
				m.series = append(m.series, metricSeries{metricKey: key,
					key: def.Key + "/" + w.metricSubjectKey(def.Per, subj), start: n})
			}
			s := &m.series[i]
			if s.ended || !ok {
				continue
			}
			s.label = w.metricSubjectLabel(def.Per, subj)
			// Snapshots hold these slices at shorter lengths; appending in
			// place writes only past every published length.
			s.vals = append(s.vals, v)
		}
	}
	// A series that got no reading has lost its subject: it ends.
	for i := range m.series {
		if s := &m.series[i]; !s.ended && s.start+len(s.vals) <= n {
			s.ended = true
		}
	}
	m.view = nil
}

// waitsForReading reports whether a subject's series starts only at its
// first non-zero reading. All goods together starts at once, like a colony
// metric.
func waitsForReading(per MetricPer, subj int64) bool {
	switch per {
	case PerItem:
		return subj != int64(ItemNone)
	case PerFixture, PerSkillRank:
		return true
	}
	return false
}

// nextMetricEvery is the interval after every: doubling, except that 4
// hours goes to 12. Every interval up to a day divides a day, so a sample
// always falls on midnight and noon and a frontend's half-day and day
// buckets stay exact (doubling on to 8 hours would split half-days from day
// 85 on; this way they hold to day 256, and days for good).
func nextMetricEvery(every int) int {
	if every == 4 {
		return 12
	}
	return every * 2
}

// halve drops every sample off the next interval (nextMetricEvery): half of
// them, or two in three going from 4 hours to 12. Levels lose resolution;
// Totals lose nothing, since each reading is a running total.
func (m *metricStore) halve() {
	m.every = nextMetricEvery(m.every)
	keep := make([]int, len(m.ticks)) // a sample's new index, or -1
	ticks, hours := make([]int, 0, metricHistory), make([]int, 0, metricHistory)
	for i, h := range m.hours {
		keep[i] = -1
		if h%m.every == 0 {
			keep[i] = len(ticks)
			ticks, hours = append(ticks, m.ticks[i]), append(hours, h)
		}
	}
	m.ticks, m.hours = ticks, hours
	for i := range m.series {
		s := &m.series[i]
		start, n := len(ticks), 0
		for j := range s.vals {
			if k := keep[s.start+j]; k >= 0 {
				start = min(start, k)
				n++
			}
		}
		// A live series will grow to the end of the history, so it gets the
		// room now; an ended one never grows. Every reading dropped (a
		// colonist who lived a few hours) leaves it known, with nothing to
		// show.
		room := n
		if !s.ended {
			room = metricHistory - start
		}
		vals := make([]int64, 0, room)
		for j, v := range s.vals {
			if keep[s.start+j] >= 0 {
				vals = append(vals, v)
			}
		}
		s.start, s.vals = start, vals
	}
}

// metricSubjectKey is a subject's part of a series key: stable across saves
// and versions for items ("iron-ore"), per game for colonists ("c12").
func (w *World) metricSubjectKey(per MetricPer, subj int64) string {
	switch per {
	case PerItem:
		if subj == int64(ItemNone) {
			return "all"
		}
		return strings.ReplaceAll(ItemKind(subj).String(), " ", "-")
	case PerAccount:
		if subj == 0 {
			return "treasury"
		}
		return "c" + strconv.FormatInt(subj, 10)
	case PerFixture:
		return strings.ReplaceAll(Terrain(subj).String(), " ", "-")
	case PerSkillRank:
		k, r := skillRankOf(subj)
		return k.String() + "-" + strconv.Itoa(r)
	}
	return ""
}

// metricSubjectLabel is a subject's name for the picker and the legend.
func (w *World) metricSubjectLabel(per MetricPer, subj int64) string {
	switch per {
	case PerItem:
		if subj == int64(ItemNone) {
			return "all goods"
		}
		return ItemKind(subj).String()
	case PerAccount:
		if subj == 0 {
			return "Treasury"
		}
		if e := w.entities[EntityID(subj)]; e != nil {
			return e.displayName()
		}
	case PerFixture:
		return Terrain(subj).String()
	case PerSkillRank:
		k, r := skillRankOf(subj)
		title := skillSpecs[k].Labels[r]
		if r == 0 {
			title = "untrained"
		}
		return fmt.Sprintf("%s %d: %s", k, r, title)
	}
	return ""
}

// MetricsView is the metrics history as a frontend reads it: a shared time
// axis and every series, each a run of readings from its Start. It is shared
// between snapshots and never written.
type MetricsView struct {
	Every  int   // clock hours between samples
	Ticks  []int // each sample's tick
	Hours  []int // each sample's clock hour since the landing day's midnight: day Hours/24+1, hour Hours%24
	Series []SeriesView
}

// SeriesView is one series: Values[i] is the reading at sample Start+i.
type SeriesView struct {
	Metric  int    // index into MetricDefs()
	Key     string // "price/meal"
	Subject string // "meal", "Treasury", a colonist's name; "" for a colony metric
	Start   int
	Values  []int64
	Ended   bool // its subject is gone (a colonist died); no more readings
}

// metricsView returns the published view, building it on the first
// snapshot after a sample.
func (w *World) metricsView() *MetricsView {
	m := &w.metrics
	if m.view != nil {
		return m.view
	}
	v := &MetricsView{Every: max(m.every, 1), Ticks: m.ticks[:len(m.ticks):len(m.ticks)],
		Hours: m.hours[:len(m.hours):len(m.hours)], Series: make([]SeriesView, len(m.series))}
	for i, s := range m.series {
		v.Series[i] = SeriesView{Metric: int(s.def), Key: s.key, Subject: s.label, Start: s.start,
			Values: s.vals[:len(s.vals):len(s.vals)], Ended: s.ended}
	}
	m.view = v
	return v
}

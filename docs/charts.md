# Charts: the general-purpose chart system

> Part of the [mars-sim documentation](./README.md).

## What it is

Anything the simulation can count or measure is a **metric**: an account's
balance, a good's price, how many bids are open, how much money changed
hands. Once a colony hour the world reads every metric for every subject it
covers (each good, each account) and keeps the readings for the whole game.
In the browser, the Charts tab's **Custom** view lets the player build charts
from any of them. A chart holds up to eight series, plots their readings or
buckets them by hour, half-day, day or week ("D43 H1: $1,394 changed hands"),
and is remembered in that browser.

## Source

- [`internal/sim/metrics.go`](../internal/sim/metrics.go): `metricDefs` (the
  table of metrics), `sampleMetrics` (the hourly sample), `halve`,
  `MetricsView` (what a snapshot publishes).
- [`internal/sim/money.go`](../internal/sim/money.go): `transfer` counts
  money changing hands (`moneyMoved`, `payments`); `payer` tells a refund from
  a payment.
- [`internal/sim/market.go`](../internal/sim/market.go): `post` counts orders
  posted and `settle` units and dollars traded, per item.
- [`internal/wire/charts.go`](../internal/wire/charts.go): the `metrics`
  catalog topic and the `series:<key>` topic.
- [`web/src/ui/charts/builder.ts`](../web/src/ui/charts/builder.ts): bucketing,
  aligning series onto one x axis, labels, the starter charts, and checking
  saved charts. Tested by `builder.test.mjs`.
- [`web/src/ui/charts/CustomCharts.svelte`](../web/src/ui/charts/CustomCharts.svelte),
  `CustomChart.svelte`, `SeriesPicker.svelte`, `Subscribe.svelte`: the view,
  one chart, the picker, and one topic subscription per series.
- [`internal/sim/metrics_test.go`](../internal/sim/metrics_test.go),
  [`internal/wire/charts_test.go`](../internal/wire/charts_test.go),
  `BenchmarkSampleMetrics2000` in
  [`internal/sim/bench_test.go`](../internal/sim/bench_test.go).

## How it works

### Metrics

A `MetricDef` row says what a metric is (key, label, group, a line of doc for
the picker), its unit (count or dollars), what it is read for (`PerColony`,
`PerItem` with "all goods" as subject 0, or `PerAccount`: the treasury and
each living colonist), and how to read it. There are two kinds of reading:

| Kind | Meaning | Examples |
| --- | --- | --- |
| `Level` | a value at the sample's moment | colonists, balance, price, open bids, units in storage |
| `Total` | a running total since the landing | money changed hands, payments, bids posted, units traded, trade value, deaths |

A Total's amount over any span is the difference between the readings at its
ends. The sim never chooses buckets: the page sums an hour, a half-day or a
week out of the same series, and the running total itself answers "total
bids over time".

The Totals come from plain counters on `World` (`moneyMoved`, `payments`,
`posted[side][item]`, `tradedUnits`, `tradedValue`, plus counters that already
existed: `taxCollected`, `moneyExported`, `starved`). The hot path bumps them
with one add. The metric code reads them once an hour.

**Money changing hands** is any `transfer` of a positive amount into a real
account (the treasury or a wallet) from someone else's money. Escrow is not a
real account: a bid's escrow going in is not a payment, and escrow coming back
to its own poster (a refund, a bid filled under its limit) is not either.
Escrow paid out to a seller or a worker is. So a $5 meal bought with a $7 bid
counts $5 once.

### Sampling on the colony clock

`sampleMetrics` runs every tick but does work only on the first tick of a
clock hour (`clockHour`, the hour `MinuteOfDay` is in, counted from the
landing day's midnight). Each sample records its tick and its hour. Day 43's
first half is hours 1008 to 1019, so the page can name buckets the way the top
bar names days.

For each metric and subject the sample reads `(value, ok)`:

- A subject with no series yet gets one at its first reading. A per-item
  series waits for a non-zero reading, so goods nobody has touched add
  nothing to the picker. A price waits for the good's first trade.
- A series whose subject stops reading (`ok` false, or a colonist no longer
  living) **ends**. It keeps its history and is listed as "gone".

Each series is a `start` index into the shared time axis plus its values.

### A history that spans the game

As with the [Population history](./population-screen.md), the axis is capped
(`metricHistory`, 512 samples). When it fills, the interval grows and the
samples off the new grid are dropped. The intervals run **1, 2, 4, 12, 24, 48…
hours**, not plain doubling. Every interval up to a day divides a day, so
samples always land on midnight and noon, and half-day and day buckets stay
exact: half-days to about day 256, days for good. Plain doubling to 8 hours
split half-days from day 85. The page widens a bucket that no longer divides
evenly (`effectiveBucket`) and says so under the chart.

Halving loses nothing from a Total, since each kept reading is still the
running total at its tick (`TestMetricsSampleHourlyAndSpanTheGame` pins
this). Levels lose resolution, as on the Population tab.

### Publishing without copying

Snapshots share the history. A new reading is appended in place, into spare
capacity past every length a snapshot holds, so no snapshot ever sees it
change. Halving builds new slices. `MetricsView` (the axis and a header per
series) is rebuilt once per sample, on the first snapshot after it, and every
snapshot until the next sample shares it. Publishing at 60 frames a second
costs nothing extra. The cached view aliases the store's slices at shorter
lengths, which the [save codec](./save-load.md) refuses, so it is tagged
`save:"-"` and rebuilt on demand.

### The wire

Two tiers, so a chart costs what it plots, not what could be plotted (see
[wire-format.md](./wire-format.md)):

- `metrics`: the catalog, every metric and every series with readings
  (key, metric, subject name, ended), with no values. It changes only when a
  series starts or ends, so after the first send it rarely goes again.
- `series:<key>`: one series' readings, with the tick and the hour of each,
  and the history's interval. A key with no readings answers `found: false`.
  This covers a saved chart from another game or a good nobody has traded.

A colony of hundreds has hundreds of wallets. Only the ones on a chart are
ever encoded.

### The Custom view

`ui.charts` holds the player's charts (`{id, title, series, bucket}`), kept in
`localStorage` and checked on load (`parseCharts`). A new browser starts with
the charts the system was asked for: money changing hands per half-day, open
bids, bids posted (a running total), the treasury, and the meal price.

`columns` (in `builder.ts`) turns a chart into uPlot's columns, with the x
axis in clock hours:

- **readings** (`bucket` 0): the union of the series' hours. A level is a gap
  before its first reading. A Total is zero there, since it has none because
  nothing had happened yet.
- **buckets**: a sample at hour h covers the hour before it, so it falls in
  bucket `⌊(h−1)/B⌋`. A Total's bucket sums its differences and draws as bars
  (a custom path builder, grouped when a chart has several Totals). A level's
  bucket is its last reading, drawn as a line through the bucket's middle.

Dollars and counts each get an axis, so a price and a bid count can share a
chart. The chart rebuilds only when its shape changes (series, units,
bucket). A new reading is a `setData`.

## Why it is this way

- **One table, everything else follows.** "Any new thing countable and
  measurable" means adding a metric must be cheap. A row in `metricDefs` gets
  its series, history, snapshot, catalog entry and picker entry for free.
  Nothing per metric lives in the wire or the page.
- **Running totals, not per-interval tallies.** The Activity tally folds
  dropped samples forward on every halving and pushes a trailing remainder
  back into the accumulator ([activity-screen.md](./activity-screen.md)).
  Storing a running total makes halving a plain drop. Any bucket width is then
  a subtraction, and the sim never needs to know what buckets a player wants.
- **Sampled hourly on the clock, not every N ticks.** The issue's own example
  is a half-day. A 50-tick grid never lines up with a 540-tick half-day, so
  every bucket edge would be approximate. Sampling at clock hours costs the
  same (one sample per 45 ticks at defaults) and makes day and half-day
  buckets exact.
- **Per series, not per sample.** The Population history stores a struct per
  sample. That works for a fixed set of fields but not for one wallet per
  colonist, a set that grows and shrinks. Series also let a short-lived
  colonist's history cost only its own lifetime.
- **Counters in the hot path, reads at sample time.** Reading a Level (a
  balance, the stock in depots) once an hour costs nothing during play.
  Counting events can't be done by reading, so those are single integer
  adds where they happen. Neither path allocates.
- **A catalog topic and a topic per series.** One topic with every series'
  values would send every wallet's history every second, megabytes at a few
  hundred colonists. The catalog carries no values, so it is stable, and a
  series costs bandwidth only while it is on a chart.
- **Charts live in the browser, not the save.** A chart is a way of looking,
  not colony state. Keeping it per browser means the same charts greet every
  game, and the sim's state stays free of UI.
- **Read-only bookkeeping.** Nothing in the simulation reads the metrics, and
  no reading draws randomness or changes a cache, so determinism and the
  save/load lockstep test are untouched. The full suite passes unchanged.

### What it costs

`BenchmarkSampleMetrics2000` (2,000 colonists, so 2,000-odd wallet series and
a few dozen others) takes about 115 µs per sample on an M-series
laptop, including halvings and the view rebuild. It makes four allocations,
all for the published view; the per-sample scratch is reused. Profiled, the sample itself
is about 50 µs, and sorting the accounts is the largest single part. At one
sample per 45 ticks that is under 3 µs a tick, far below what 2,000 colonists
cost to step. Between samples a tick pays two divisions. `BenchmarkStep500`
measured 405.6 µs a tick before and 408.6 µs after (six runs each, within
run-to-run noise). A full history holds
8 bytes per series per sample: 4 MB for 1,000 colonists' wallets at 512
samples.

## Extending it

- **A new Level:** add a row to `metricDefs` with a `read` returning the
  value. If several metrics need the same scan (a walk over containers or
  orders), compute it once in `newMetricCtx`.
- **A new Total:** add a counter field on `World`, bump it where the thing
  happens (with the all-goods sum at `ItemNone` for a per-item one), and add a
  row reading it. Never decrement a Total: the page reads differences.
- **A new family of subjects** (per species, per workshop): a `MetricPer`
  value, its subject list in `sampleMetrics`, and its key and label in
  `metricSubjectKey` and `metricSubjectLabel`. Keys are saved in players'
  charts, so keep them stable (items use their name, not their enum value).
- **Keys are an interface.** Renaming a metric's `Key` silently empties every
  saved chart that used it.
- **Not built:** the TUI has no chart builder. Its Population and Activity
  screens predate this and still read the Population history. Moving them
  onto metrics would remove one of the two histories.

## Related

- [population-screen.md](./population-screen.md): the fixed-field history this
  generalises, and the halving it borrows.
- [money.md](./money.md), [market.md](./market.md): what the money and market
  metrics count.
- [days.md](./days.md): the colony clock the samples follow.
- [wire-format.md](./wire-format.md): the topics.
- [frontend-web.md](./frontend-web.md): the Charts tab.

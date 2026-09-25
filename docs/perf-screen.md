# Perf screen: tick rate and tick cost over time

> Part of the [mars-sim documentation](./README.md).

## What it is

A TUI tab (`tab` to the last one, **Perf**) that graphs how the engine is keeping
up, in the style of [gping](https://github.com/orf/gping): a braille line chart
of **ticks per second** actually achieved, and below it one of **milliseconds
per tick**. With no upper limit on `-tps` any more (see [cli.md](./cli.md)), this
is how you find out what rate a given colony can actually sustain.

## Source

- [`internal/sim/perf.go`](../internal/sim/perf.go) — `PerfSample`, `PerfBucket`, and the engine's `perfRecorder`.
- [`internal/sim/engine.go`](../internal/sim/engine.go) — `Engine.tick` times each step and publish; `publish` attaches the history to the snapshot.
- [`internal/ui/tui/render_perf.go`](../internal/ui/tui/render_perf.go) — the series, stats lines, and the braille chart.
- [`internal/sim/perf_test.go`](../internal/sim/perf_test.go), [`internal/ui/tui/render_perf_test.go`](../internal/ui/tui/render_perf_test.go) — bucketing, idle gaps, sharing safety, and chart rendering.

## How it works

### Measuring (engine)

`Engine.tick` wraps each tick in two wall-clock timings: `World.step` (the
simulation) and `publish` (building the snapshot and posting it). Both count as
the tick's cost — publishing is real work, and on big maps it has been the
expensive half before (see [snapshot-tile-grid.md](./snapshot-tile-grid.md)).
Above 60 tps most ticks skip publishing (see `maxPublishRate` in
[architecture.md](./architecture.md)) and record zero publish time, so
publishing's cost shows up spread across all the ticks.

`perfRecorder` sums those into fixed **`PerfBucket` (250 ms)** samples: ticks
completed, total step time, total publish time, and the slowest single tick.
`advance` closes every bucket that has ended; a bucket in which nothing ticked
(paused, or one tick slower than a whole bucket) is kept as a zero-tick sample so
the gap shows. The last `perfHistory` (1200 — five minutes) closed samples ride
along on every snapshot as `Snapshot.Perf`.

### Drawing (TUI)

- **Ticks/sec** for each sample is ticks over the trailing one second of
  buckets (`tpsSeries`), not the sample's own count ×4.
- **ms/tick** is each bucket's mean tick cost (`msSeries`); a zero-tick bucket
  is a gap (NaN), not a zero.
- Each chart shows the newest `2 × plot width` samples — one per braille dot
  column — so a wider terminal shows more history. Consecutive points are
  joined by a vertical run of dots in the new point's column, which is what
  gives gping its continuous line.
- The y range is the visible data's min/max padded 5%; labels get just enough
  decimals (`axisLabel`) that neighbouring labels differ. The time axis shows
  the start, middle, and end of the window.
- The stats lines: tps `last/min/max/avg` and the `target` rate; ms/tick
  `last`, `avg` (total work ÷ total ticks), `p95` of bucket means, `worst`
  single tick, `budget` (1000 ÷ target: the most a tick can cost before the sim
  falls behind), `min`, and `step %` (the simulation's share of tick cost; the
  rest is publishing).

## Why it is this way

- **The engine measures; the TUI does not infer.** The obvious shortcut is to
  derive tps from the tick numbers on the snapshots the TUI receives. But the
  engine's one-slot subscription drops stale frames and the TUI redraws at
  30 fps, so at high rates it never sees most ticks — and it has no way to
  know how long any of them took.
- **Buckets, not per-tick samples.** At thousands of tps, a per-tick history
  would be large and would have to be copied into every published frame. A
  quarter-second bucket bounds the history at 1200 small structs whatever the
  rate.
- **The history is shared, never copied per frame.** `perfRecorder.close`
  appends through a slice whose capacity equals its length, so each close
  allocates a fresh array and every snapshot already holding the old one keeps
  it unchanged. That costs one short copy four times a second, instead of one
  per published frame. `TestPerfHistoryIsNeverWrittenInPlace` pins this down:
  mutating in place would be a data race with the render goroutine.
- **Wall-clock timing stays out of the World.** It lives on the `Engine`, never
  feeds back into the simulation, and so cannot affect determinism (see
  [determinism.md](./determinism.md)).
- **Ticks/sec is averaged over a second.** At 8 tps a quarter-second bucket
  holds 1–3 ticks, and plotting counts×4 draws noise between 4 and 12.

- **It caught a real bug on day one.** On macOS, `-tps 100` measured about
  60 here, while `+` pushed it higher. The engine ran on a `time.Ticker`,
  which drops any tick whose wakeup came late, and macOS coalesces timers.
  The loop now paces against deadlines and makes late ticks up (see
  [architecture.md](./architecture.md)). If ticks/sec sits below target while
  ms/tick is well under budget, suspect the scheduler, not the simulation.

The Perf tab tells you *how much* a tick costs. To see *where* the time goes,
take a CPU profile with `-cpuprofile` (see [cli.md](./cli.md#profiling)).

## Extending it

- A new measure (say, time spent in pathfinding) is a field on `PerfSample`,
  filled in `perfRecorder.record`, and a series function plus a `perfChart` in
  `renderPerf`. Keep `PerfSample` small: 1200 of them are live at once.
- `braille` is a general dot canvas; any other chart can reuse it.
- Do not write into `Snapshot.Perf` or the recorder's history in place (see
  above).

## Related

- [frontend-tui.md](./frontend-tui.md) — the tab rotation and the other screens.
- [architecture.md](./architecture.md) — the engine loop and snapshot contract.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the performance work this screen helps measure.
- [cli.md](./cli.md) — `-tps` and the speed controls.

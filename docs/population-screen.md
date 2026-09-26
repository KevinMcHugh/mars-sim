# Population screen

> Part of the [mars-sim documentation](./README.md).

## What it is

A TUI tab, **Population** (between Lore and Perf), that charts the colony's
vital signs over the whole game: how many colonists there are, how many meals
are in storage, how big the colony is, and how many fixtures it has. It uses
the same braille line charts as the [Perf screen](./perf-screen.md), on the
simulation clock instead of the wall clock.

## Source

- [`internal/sim/population.go`](../internal/sim/population.go) —
  `PopulationSample`, `samplePopulation`, and the halving history.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) —
  `Snapshot.Population`.
- [`internal/ui/tui/render_population.go`](../internal/ui/tui/render_population.go)
  — the four charts, `stretch`, and `tickAxis`.
- [`internal/ui/tui/render_perf.go`](../internal/ui/tui/render_perf.go) —
  `perfChart`, shared with the Perf screen; `xAxis` and `counts` exist for
  this screen.
- [`internal/sim/population_test.go`](../internal/sim/population_test.go),
  [`internal/ui/tui/render_population_test.go`](../internal/ui/tui/render_population_test.go).

## How it works

### What is sampled

At the end of every `step`, `samplePopulation` records a `PopulationSample`
when the tick is a multiple of the current interval:

| Field | Meaning |
| --- | --- |
| `Colonists` | living colonists |
| `Meals` | meals physically in any depot (lockers, chests, scumhouses, the silo), whoever owns them and whether or not they're on offer. Meals in someone's pockets aren't counted. |
| `ColonySize` | floor tiles the colony has dug or discovered (the header's "excavated") |
| `Fixtures` | placed fixtures: bunks, toilets, pods, lockers, chests, scumhouses, incinerators |

### A history that always spans the game

A game has no set length, so the history is capped at `popHistory` (512)
samples. It starts at one sample every 50 ticks. When it fills, it keeps only
the samples on double the interval and carries on at that interval. Every
sample is always on the current interval, so the history is evenly spaced and
runs from near the start of the game to now. Resolution is finer early and
coarser later, which suits a line chart of a whole game.

Like the Perf history, the slice is shared with every published snapshot and
never written in place: appending to a full-capacity slice copies it, and
halving builds a new one.

### The charts

`renderPopulation` lays the four charts out two by two. `popChart` builds a
`perfChart` for each, with:
- `stretch` resampling the whole history to exactly one value per braille
  dot column (a short history fills the plot as steps; a long one is thinned);
- stats of now, min and max;
- `tickAxis` labelling the first, middle and last sample's tick.

`counts` gives the y axis whole-number labels, each shown once, instead of the
Perf screen's fractional ones.

## Why it is this way

- **Simulation clock, not wall clock.** The Perf screen measures the engine,
  so wall time is its natural axis. These are facts about the colony: a
  history should read the same for a seed however fast it was played, paused,
  or sped up.
- **Sampled in the sim, not the frontend.** A frontend only sees the frames
  it receives, and the engine drops frames a slow renderer can't keep up
  with (see [frontend-tui.md](./frontend-tui.md)). Sampling in `step` misses
  nothing, and it's there for any frontend.
- **Halving, not a ring buffer.** A ring buffer of recent samples would show
  only the last stretch of a long game, and "how has the colony grown since
  landing?" is the question this screen is for.
- **Read-only bookkeeping.** Nothing in the simulation reads the history, so
  it can't affect determinism. The meal count walks every container once per
  sample (every 50+ ticks), which is negligible.
- **Reusing the Perf chart.** One braille renderer, with two small options
  (`xAxis`, `counts`), rather than a second chart implementation to keep in
  step.

## Extending it

- **A new vital sign**: a field on `PopulationSample`, set in
  `samplePopulation`, and a `popSeries` entry. Four charts fill the 2×2 grid;
  a fifth needs a layout change in `renderPopulation`.
- **Finer history**: raise `popHistory` or lower `popFirstEvery`. Each sample
  is five ints, so even thousands are cheap.

## Related

- [perf-screen.md](./perf-screen.md) — the chart this screen reuses.
- [frontend-tui.md](./frontend-tui.md) — tabs and screens.
- [food.md](./food.md), [crash-pods.md](./crash-pods.md) — where the meals and fixtures come from.

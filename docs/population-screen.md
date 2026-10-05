# Population screen

> Part of the [mars-sim documentation](./README.md).

## What it is

A TUI tab, **Population** (between Lore and Perf), that charts the colony's
vital signs over the whole game: how many colonists there are, how many meals
are in storage, how big the colony is, and one **tracked** series the player
picks: how many fixtures it has, how many of one kind (scumhouses, scum
incubators, beds...), or how many colonists stand at a skill title or better
("chef or better", "master smith"). It uses
the same braille line charts as the [Perf screen](./perf-screen.md), on the
simulation clock instead of the wall clock.

## Source

- [`internal/sim/population.go`](../internal/sim/population.go) —
  `PopulationSample`, `samplePopulation`, and the halving history.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) —
  `Snapshot.Population`.
- [`internal/ui/tui/render_population.go`](../internal/ui/tui/render_population.go)
  — the four charts, `trackedSeries` and its `[`/`]` keys, `stretch`, and
  `tickAxis`.
- [`internal/wire/charts.go`](../internal/wire/charts.go) — the browser's
  `population` topic, with `fixtureKinds`/`fixtureCounts` and `skills`;
  [`web/src/ui/charts/population.ts`](../web/src/ui/charts/population.ts)
  (`trackedSeries`) and
  [`PopulationCharts.svelte`](../web/src/ui/charts/PopulationCharts.svelte)
  — the same list as a picker on the fourth chart.
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
| `FixtureKinds` | placed tiles of each kind in `TrackedFixtures`, indexed by `Terrain`: every fixture kind, plus the meeting hall's chairs (furniture, so not in `Fixtures`). Read from `terrainCounts`, so it costs nothing to take. |
| `SkillRanks` | living colonists at exactly each rank of each skill, rank 0 (untrained) included, so a skill's row sums to `Colonists`. `SkillRankLabels` gives each rank's title. |
| `Activity`, `Walking` | colonist-ticks per activity since the previous sample, and the part of them spent walking there, for the [Activity tab](./activity-screen.md). Halving folds a dropped sample's tally into the next kept one. |

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

`renderPopulation` lays the four charts out two by two: the three fixed
`popCharts`, and bottom right the tracked series, `trackedSeries[popTrack]`.
`[`/`]` (or `←`/`→`, `h`/`l`) step through `trackedSeries`, wrapping:

1. all fixtures (the default, and what the fourth chart always showed);
2. each kind in `TrackedFixtures`, titled by its name ("SCUMHOUSE");
3. per skill, each title **or better**: "CHEF OR BETTER" counts chefs and
   master chefs, and the top title stands alone ("MASTER CHEF"). A title
   that covers several ranks (mining's "digger" is ranks 2 and 3) is one
   series from its lowest rank.

The browser builds the same list in `population.ts` and puts it in a
`<select>` as the fourth chart's title, grouped by fixtures and by skill.

`popChart` builds a `perfChart` for each chart, with:
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
- **One tracked chart with a picker, not a chart per kind.** Eleven fixture
  kinds and over twenty skill titles would be thirty small charts; most of a
  game they would be flat lines at 0 or 1. A player asking "how many
  incubators do we have?" wants one answer at a time, at full size, and the
  three vital signs keep their place.
- **Ranks stored exactly, counted "or better".** The sample keeps the raw
  count per rank, so any view (exactly a cook, cook or better, a histogram)
  can be computed later without changing the sim. The charts count "or
  better" because a colony asking for cooks is glad of a chef, and because a
  count of exact ranks falls whenever someone is promoted, which reads as a
  loss.
- **Series titled by skill title alone.** The titles are unique across
  skills (`TestTrackedKindsFit` pins it), and the short title leaves a
  half-width TUI chart room for its now/min/max.
- **Reusing the Perf chart.** One braille renderer, with two small options
  (`xAxis`, `counts`), rather than a second chart implementation to keep in
  step.

## Extending it

- **A new vital sign**: a field on `PopulationSample`, set in
  `samplePopulation`, and a `popSeries` entry. Four charts fill the 2×2 grid;
  a fifth needs a layout change in `renderPopulation`. A count the player
  wants only now and then belongs in `trackedSeries` instead (in both the
  TUI and `population.ts`), not in a chart of its own.
- **A new fixture kind**: add it to `TrackedFixtures` (`TestTrackedKindsFit`
  fails until you do). Both frontends pick it up from there.
- **A new skill or rank**: nothing to do unless a skill outgrows
  `maxSkillRanks` (the test says so). A new title must not repeat another
  skill's.
- **Finer history**: raise `popHistory` or lower `popFirstEvery`. Each sample
  is about a hundred ints (most of them the fixture kinds and skill ranks),
  under 1 KB, so even thousands are cheap.

## Related

- [perf-screen.md](./perf-screen.md) — the chart this screen reuses.
- [frontend-tui.md](./frontend-tui.md) — tabs and screens.
- [food.md](./food.md), [ships.md](./ships.md) — where the meals and fixtures come from.
- [skills.md](./skills.md) — skills, ranks and their titles.

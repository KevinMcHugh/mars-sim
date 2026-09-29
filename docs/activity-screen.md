# Activity screen

> Part of the [mars-sim documentation](./README.md).

## What it is

A TUI tab, **Activity** (between Population and Log), with a stacked area chart
of what the colonists spend their time doing over the whole game: sleeping,
eating, cooking, mining, building, hauling, fighting, fleeing, and so on. A
legend beside it gives each activity's share over the latest sample and over
the whole game. `c` switches the chart between **shares** of colonist time
(the stack always fills to 100%) and **average colonists** (the stack's height
is the population, so a death or a new arrival shows).

## Source

- [`internal/sim/activity.go`](../internal/sim/activity.go): `Activity`,
  `activityOf` (the classifier), and `tallyActivity`.
- [`internal/sim/population.go`](../internal/sim/population.go):
  `PopulationSample.Activity`, and how halving folds dropped tallies forward.
- [`internal/sim/systems.go`](../internal/sim/systems.go): `step` tallies each
  colonist right after its turn.
- [`internal/ui/tui/render_activity.go`](../internal/ui/tui/render_activity.go):
  the band order and colours, column bucketing, the half-block stack, and the
  legend.
- [`internal/sim/activity_test.go`](../internal/sim/activity_test.go),
  [`internal/ui/tui/render_activity_test.go`](../internal/ui/tui/render_activity_test.go).

## How it works

### What is counted

Every tick, right after a colonist's turn, `tallyActivity` adds one to
`World.actTally[activityOf(e)]`. The next [Population](./population-screen.md)
sample copies the tally into `PopulationSample.Activity` and resets it. A
sample's tally is therefore **colonist-ticks since the previous sample**. Divided
by the ticks between the two samples, it gives the average number of colonists
doing each thing over that stretch. Divided by the tally's own total, it gives
the share of colonist time.

`activityOf` works in three steps:

1. **State**, when it names the work: `Eating`, `Sleeping`, `Crafting`
   (cooking; the scumhouse is the only workshop), `Mining`, `Fighting`, and so
   on. Some States share a band: `Hauling` and `Storing` are both hauling;
   `Cleaning` and `Scraping` are both cleaning; `Stomping` counts as fighting.
2. Otherwise (the colonist is `Moving` or `Idle`), the **job** it is walking
   for: `JobMine` is mining, `JobCraft` is cooking, `JobStore`/`JobSell`/
   `JobCarry` are hauling, and so on.
3. For `JobUse` or no job, the **focus**: a colonist walking to a bed is
   sleeping, one running with `FocusFlee` is fleeing. Anything left is idle.

### Halving keeps every tick

The Population history halves its resolution when it fills: every other
sample is dropped. A dropped sample's tally is folded into the next kept
sample. A trailing dropped sample goes back into `actTally` for the next
sample to be taken. So the tallies across the history always add up to every
colonist-tick up to the last sample. `TestActivityTallyCoversEveryTick` pins
this across several halvings.

### The chart

`activityColumns` buckets the history into one slice per plot column by
summing the samples in each column's range. A short history is repeated
across the columns and a long one is summed down. Each column sums its own
ticks as well, so count mode divides by the right span.

`stackPlot` stacks `activityBands` bottom to top. Each cell is two "dots" tall:
a `▀` in the upper dot's band colour on the lower dot's band as background.
This doubles the vertical resolution of plain blocks and needs no special
font support. `halfBlockRow` styles each run of identical cells once, instead
of once per cell, because a full-screen chart would otherwise cost thousands
of lipgloss renders a frame.

The y axis runs from 0 to 100% for shares. For counts it runs to the tallest
column, rounded up to a whole colonist. The x axis reuses the Population tab's
`tickAxis`.

## Why it is this way

- **Tallied every tick, not sampled.** A reading of State every 50+ ticks would
  alias badly: a fight lasts a handful of ticks and would usually be missed
  entirely. Counting every tick costs one array increment per colonist.
- **Riding on the Population history.** The sim already has a whole-game
  history on the simulation clock with the right halving behaviour. Adding a
  fixed-size array to its sample reuses that whole mechanism: no second
  cadence, cap, or snapshot field to keep in step. Thirteen ints per sample,
  512 samples, is nothing.
- **An Activity, not the raw State.** `Moving` is the single most common
  State, and it tells a player nothing. Crediting a walk to its purpose is the
  difference between "40% moving" and "the colony spends a third of its day
  fetching meals". The classification lives in the sim, beside the fields it
  reads (`focus` is unexported), and not in the frontend.
- **Stacking order.** Needs sit at the floor, work in the middle, danger
  (escaping, fleeing, fighting) above that, and idle on top as a grey lid. A
  raid shows up as red and magenta swelling just under the lid, where the eye
  lands, and not buried in the middle of the stack.
- **Read-only bookkeeping.** Nothing reads the tally back, so it cannot
  affect determinism.

## Extending it

- **A new activity**: add it to `Activity` before `NumActivities` (with a
  `String` case), classify it in `activityOf`, and give it a band and colour in
  `activityBands`. The legend grows by a row. Every band must appear in
  `activityBands`, or its time silently vanishes from the stack.
- **A new State or JobKind**: decide which activity it belongs to in
  `activityOf`. Unclassified work falls through to the focus and then to idle,
  which is quiet but wrong.

## Related

- [population-screen.md](./population-screen.md): the history this rides on.
- [entities-and-ai.md](./entities-and-ai.md): State, focus, and job.
- [frontend-tui.md](./frontend-tui.md): tabs and screens.

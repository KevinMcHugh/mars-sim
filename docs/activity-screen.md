# Activity screen

> Part of the [mars-sim documentation](./README.md).

## What it is

A TUI tab, **Activity** (between Population and Log), with a stacked area chart
of what the colonists spend their time doing over the whole game: sleeping,
eating, cooking, mining, building, hauling, fighting, fleeing, and so on. Each
band is split in two: the activity itself in its colour, and **walking to it**
(fetching a meal, heading to the scumhouse to cook) in a darker shade just
above. A legend beside it gives each activity's share over the latest sample
and over the whole game, and how much of its time was spent walking. `c` switches the chart between **shares** of colonist time
(the stack always fills to 100%) and **average colonists** (the stack's height
is the population, so a death or a new arrival shows).

## Source

- [`internal/sim/activity.go`](../internal/sim/activity.go): `Activity`,
  `activityOf` (the classifier, and whether the colonist is walking), and
  `tallyActivity`.
- [`internal/sim/population.go`](../internal/sim/population.go):
  `PopulationSample.Activity` and `.Walking`, and how halving folds dropped
  tallies forward.
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
`World.actTally` for the colonist's activity, and to `World.walkTally` as well
if it was walking there. The next [Population](./population-screen.md) sample
copies both into `PopulationSample.Activity` and `.Walking` and resets them.
`Walking` is a part of `Activity`, not an addition to it. A
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
   sleeping. Anything left is idle.

Only steps 2 and 3 can count as walking, and only when the State is `Moving`.
A colonist whose State already names the work is doing it, even when that work
moves it: running is what `Fleeing` is, and carrying refuse to the incinerator
(`Hauling`) is the hauling. An `Idle` colonist holding a job (waiting at the
rock face for a claim) counts as doing the job, not walking to it.

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

`stackPlot` stacks `activityBands` bottom to top, each as two segments: the
activity's time less its walking in the band's colour, then its walking in the
band's `walk` shade. Each cell is two "dots" tall:
a `▀` in the upper dot's segment colour on the lower dot's as background.
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
- **Walking as a shade of the band, not a band of its own.** A single
  "moving" band says the colony walks a lot but not why; a separate
  "walking to X" band per activity doubles the legend. A darker shade directly
  above each activity keeps the two together, so "cooking is a third walking"
  reads off the chart, and the legend's `walk` column gives the number.
- **Stacking order.** Needs sit at the floor, work in the middle, danger
  (escaping, fleeing, fighting) above that, and idle on top as a grey lid. A
  raid shows up as red and magenta swelling just under the lid, where the eye
  lands, and not buried in the middle of the stack.
- **Read-only bookkeeping.** Nothing reads the tally back, so it cannot
  affect determinism.

## Extending it

- **A new activity**: add it to `Activity` before `NumActivities` (with a
  `String` case), classify it in `activityOfState` and/or
  `activityOfPurpose`, and give it a band, colour, and walking shade in
  `activityBands`. The legend grows by a row. Every band must appear in
  `activityBands`, or its time silently vanishes from the stack.
- **A new State or JobKind**: decide which activity it belongs to in
  `activityOfState` or `activityOfPurpose`. A State that names work is never
  counted as walking; a travel State other than `Moving` needs `activityOf`
  taught about it. Unclassified work falls through to the focus and then to idle,
  which is quiet but wrong.

## Related

- [population-screen.md](./population-screen.md): the history this rides on.
- [entities-and-ai.md](./entities-and-ai.md): State, focus, and job.
- [frontend-tui.md](./frontend-tui.md): tabs and screens.

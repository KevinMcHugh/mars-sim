# Colony days and the night's sleep

> Part of the [mars-sim documentation](./README.md).

## What it is

A calendar for the player: the top bar (web) and header (TUI) show **day N**
since landing and the **time of day** (`day 3 07:05`) beside the raw tick. A day's length is not its own knob; it is
derived from the sleep need, so a day means "one waking stretch and one night".
Everything inside the simulation, and the performance charts, still count in
ticks.

Because the day is built from sleep, sleep is tuned to the day: a night in
bed is **eight clock hours**, and the Short Sleeper and Long Sleeper traits
move it an hour either way. Hours-long nights needed two new rules (other
needs pause while asleep, and an interrupted night is banked), covered below.

## Source

- [`internal/sim/needs.go`](../internal/sim/needs.go) — `Config.TicksPerDay`, `LandingHour`, `DayOf`, and `MinuteOfDay`; the sleep `NeedSpec`.
- [`internal/sim/sleep.go`](../internal/sim/sleep.go) — `Config.TicksPerHour`, `sleepTick`, `fallAsleep` / `wakeUp` (pausing the other needs), and the banked night.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — the Short Sleeper / Long Sleeper traits (`sleepHours`), resolved into `Entity.sleepTicks`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `Stats.Day` and `Stats.MinuteOfDay`, filled in by `snapshot`.
- [`internal/ui/tui/view.go`](../internal/ui/tui/view.go) — `renderHeader` shows `day N  HH:MM` on the title line.
- [`web/src/ui/TopBar.svelte`](../web/src/ui/TopBar.svelte) — the web top bar's `day N HH:MM` readout.
- [`internal/sim/needs_test.go`](../internal/sim/needs_test.go) — `TestTicksPerDayFollowsSleep`, `TestDayOfCountsLandingAsDayOne`, `TestMinuteOfDayStartsAtLandingHour`.
- [`internal/sim/sleep_test.go`](../internal/sim/sleep_test.go) — trait night lengths, needs pausing in bed, banking, and waking to flee.
- [`internal/ui/tui/render_test.go`](../internal/ui/tui/render_test.go) — `TestHeaderShowsDayAndTimeOfDay`.

## How it works

```
TicksPerDay = ceil(sleep.SeekAt / sleep.Rise) + sleep.UseTicks
offset      = TicksPerDay * LandingHour / 24       // landing is 06:00
Day         = (tick + offset) / TicksPerDay + 1    // the landing is day 1
MinuteOfDay = (tick + offset) % TicksPerDay * 1440 / TicksPerDay
```

With the default sleep need (rise 1, seek-at 720, use-ticks 360) a day is
**1080 ticks**. A colonist starts with no sleep need, stays up until it reaches
`SeekAt` (when they drop work to go to bed), then spends `UseTicks` in a bunk:
that cycle is the day.

The clock stretches those ticks over 24 hours, so at defaults one clock hour
(`TicksPerHour`) is exactly **45 ticks**, 720 waking ticks are 16 hours, and the
360-tick night is 8. The colony lands at **06:00**, so a colonist on the base
rhythm goes to bed at 22:00 and gets up at 06:00. The day number turns over at
**midnight**, 810 ticks after landing, not at the landing's anniversary.

### The night's sleep

- **Length.** `Entity.sleepTicks` is the sleep need's `UseTicks` plus
  `sleepHours × TicksPerHour` from its traits: Short Sleeper is −1 (315 ticks,
  seven hours) and Long Sleeper +1 (405, nine). They share the `sleep` trait
  group, so a colonist has at most one. The calendar still uses the base night.
- **Other needs pause.** `sleepTick` calls `fallAsleep`, which folds each
  non-sleep need's lazy level into its base and sets its rise to 0, keeping the
  rate in `wakeRise`. `wakeUp` does the reverse. A finished night wakes the
  colonist directly; any other way out of bed (fleeing, a fatal need, the bed
  going away) is caught at the top of `colonistTurn`, which wakes a colonist
  whose `asleep` flag is set but whose state is no longer `Sleeping`.
  `resolveTraitEffects` re-applies the pause if it runs mid-night (mutation).
- **Banked nights.** Sleep progress lives in `Entity.sleepBanked`, not the
  shared `Progress` job timer, so leaving bed early keeps what was slept and the
  next lie-down finishes the night instead of restarting it. It resets to 0
  only when a night completes.

`Day` and `MinuteOfDay` are `int` fields of `sim.Stats`, so they reach the
browser through the frame's stats section with no wire change (see
[wire-format.md](./wire-format.md)); the page reads `ui.stats.Day` and
`ui.stats.MinuteOfDay` and formats `HH:MM` itself. They are the last fields so
existing stat indices stay put. The TUI puts the clock on the header's title
line because the counts line below is already close to 120 columns; putting
it there truncated that line, which `TestLogTabShowsTheFullEntryWrapped` caught.

## Why it is this way

- **Derived, not configured.** A separate `ticks-per-day` tunable would drift
  from how colonists actually live the moment someone retunes sleep, and "day 3"
  would stop meaning "they have slept about twice". Change the sleep need and the
  calendar follows.
- **Base rates only.** Traits scale each colonist's rise rate, and the walk to a
  bed varies; neither is counted. A calendar has to be one length for everyone,
  so it uses the configured `NeedSpec`, not any individual.
- **Seek-at, not max.** Colonists go to bed at `SeekAt`, so the waking stretch
  ends there. Using `Max` would make days longer than anyone's real cycle.
- **Charts stay in ticks.** The performance and population charts measure the
  engine; a day there would only blur the resolution. Days are for the player's
  sense of elapsed colony time.
- **A clock in Go, not in the page.** The sim sends minutes since midnight
  rather than the day length, so the TUI and the page cannot disagree about
  where the landing falls or when midnight is.
- **Landing at 06:00, midnight rollover.** Colonists land with no sleep need,
  so landing is their morning. A day number that turned over at landing would
  make 05:59 and 06:00 different days, which reads wrong next to a clock.
  Colonists do not sleep in sync (they go to bed when their own need says so),
  so the clock is a label, not a schedule.
- **Sleep used to be 40 ticks.** That made a 740-tick day with a 1.3-hour
  night, which looked absurd on a clock. The fix was to make the night eight
  hours (`use-ticks` 360) rather than to bend the clock; `seek-at` moved from
  700 to 720 so the day is 1080 ticks and an hour a whole 45.
- **Why other needs pause in bed.** Lengthening the night alone broke the
  colony. Food comes due every ~325 ticks awake, so hunger pulled almost
  every colonist out of bed before a 360-tick night finished, the night
  restarted from zero, and sleep sat pinned near its ceiling. Measured over
  six seeds × 10,800 ticks: ~150 interrupted nights to ~8 finished per run,
  colonists in bed ~60% of the time, digging down ~80%, and one colony wiped
  out. Slowing rather than stopping the other needs does not work with integer
  rise rates: food rises 2/tick, so "slower" is either 1 (half, still enough
  to starve someone who went to bed hungry) or 0. With the pause: ~55 finished
  nights per run, a handful interrupted (only by fleeing), ~30% of time in bed
  (eight hours is 33%), and colony survival about where the 40-tick night left
  it (aliens do most of the killing either way). Digging per tick falls by
  about a quarter, which is the cost of colonists sleeping a third of the day.
- **Why banking.** A night is now long enough that something will
  occasionally interrupt it (an alien, mostly). Restarting from zero turned
  each interruption into a lost night; banking makes it a short break.
- **Zero rise.** If sleep is configured never to rise, the waking stretch falls
  back to `Max` ticks so `TicksPerDay` is still positive and `DayOf` never
  divides by zero.

## Extending it

- Showing days elsewhere (log lines, memories, the roster's "dead (tick N)"):
  call `DayOf(tick, cfg.TicksPerDay())` in Go. `Snapshot.TicksPerDay` carries
  the day length for code that has only a snapshot (the wire): the
  `order:<id>` topic turns an order's posted tick into a day and clock this
  way, and sends durations in colony minutes so the page never needs the day
  length (see [order-detail.md](./order-detail.md)). The frame's stats still
  carry only the current day.
- Moving the landing time is `LandingHour`. It shifts both the clock and when
  the day number turns over; the tests in `needs_test.go` pin the 06:00 values.
- Another sleep trait is a `traitSpec` with `sleepHours` in the `sleep` group.
  Anything else that should not tick while a colonist sleeps belongs in
  `fallAsleep` / `wakeUp`, which must stay paired: every way out of bed has to
  end in `wakeUp`, or the colonist's needs stay frozen.
- Nothing in the simulation reads the clock. A day/night mechanic (lighting,
  shift schedules) would want `MinuteOfDay` from Go, not the stat.

## Related

- [needs.md](./needs.md) — the sleep need the day is derived from.
- [personality.md](./personality.md) — the trait table the sleep traits join.
- [wire-format.md](./wire-format.md) — how a new stat reaches the page.

# Colony days and the night's sleep

> Part of the [mars-sim documentation](./README.md).

## What it is

A calendar for the player: the top bar (web) and header (TUI) show **day N**
since landing and the **time of day** (`day 3 07:05`) beside the raw tick. A day's length is not its own knob; it is
derived from the sleep drive, so a day means "one waking stretch and one night".
Everything inside the simulation, and the performance charts, still count in
ticks.

Because the day is built from sleep, sleep is tuned to the day: a night in
bed is **eight clock hours**, and the Short Sleeper and Long Sleeper traits
move it an hour either way. Hours-long nights needed two new rules (other
drives all but stop while asleep, and an interrupted night is banked), covered
below.

## Source

- [`internal/sim/drives.go`](../internal/sim/drives.go) — `Config.TicksPerDay`, `LandingHour`, `DayOf`, and `MinuteOfDay`; the sleep `DriveSpec`.
- [`internal/sim/sleep.go`](../internal/sim/sleep.go) — `Config.TicksPerHour`, `sleepTick`, and the banked night.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — the Short Sleeper / Long Sleeper traits (`sleepHours`), resolved into `Entity.sleepTicks`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `Stats.Day` and `Stats.MinuteOfDay`, filled in by `snapshot`.
- [`internal/ui/tui/view.go`](../internal/ui/tui/view.go) — `renderHeader` shows `day N  HH:MM` on the title line.
- [`web/src/ui/TopBar.svelte`](../web/src/ui/TopBar.svelte) — the web top bar's `day N HH:MM` readout.
- [`internal/sim/drives_test.go`](../internal/sim/drives_test.go) — `TestTicksPerDayFollowsSleep`, `TestDayOfCountsLandingAsDayOne`, `TestMinuteOfDayStartsAtLandingHour`.
- [`internal/sim/sleep_test.go`](../internal/sim/sleep_test.go) — trait night lengths, drives pausing in bed (under `testConfig`), banking, and waking to flee.
- [`internal/ui/tui/render_test.go`](../internal/ui/tui/render_test.go) — `TestHeaderShowsDayAndTimeOfDay`.

## How it works

```
TicksPerDay = ceil(sleep.SeekAt × 1000 / sleep.Rate) + sleep.UseTicks
offset      = TicksPerDay * LandingHour / 24       // landing is 06:00
Day         = (tick + offset) / TicksPerDay + 1    // the landing is day 1
MinuteOfDay = (tick + offset) % TicksPerDay * 1440 / TicksPerDay
```

With the default sleep drive (rate 1000, one point a tick; seek-at 720;
use-ticks 360) a day is **1080 ticks**. A colonist starts with no sleep drive, stays up until it reaches
`SeekAt` (when they drop work to go to bed), then spends `UseTicks` in a bunk:
that cycle is the day.

The clock stretches those ticks over 24 hours, so at defaults one clock hour
(`TicksPerHour`) is exactly **45 ticks**, 720 waking ticks are 16 hours, and the
360-tick night is 8. The colony lands at **06:00**, so a colonist on the base
rhythm goes to bed at 22:00 and gets up at 06:00. The day number turns over at
**midnight**, 810 ticks after landing, not at the landing's anniversary.

### The night's sleep

- **Length.** `Entity.sleepTicks` is the sleep drive's `UseTicks` plus
  `sleepHours × TicksPerHour` from its traits: Short Sleeper is −1 (315 ticks,
  seven hours) and Long Sleeper +1 (405, nine). They share the `sleep` trait
  group, so a colonist has at most one. The calendar still uses the base night.
- **Other drives all but stop.** A colonist in bed (`State == Sleeping`) is in
  the asleep drive activity, and each drive says how fast it grows there
  (`drives.<name>.activity.asleep`): social 0%, food and bladder 10%, sleep
  itself 100%. Getting into or out of bed is a change of drive activity, which
  `syncDriveActivity` notices after the turn and refreshes the rates for, so
  there is nothing to pair up and nothing to forget on an unusual way out of
  bed. See [drives.md](./drives.md).
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
  would stop meaning "they have slept about twice". Change the sleep drive and the
  calendar follows.
- **Base rates only.** Traits scale each colonist's drive rates, and the walk to a
  bed varies; neither is counted. A calendar has to be one length for everyone,
  so it uses the configured `DriveSpec`, not any individual.
- **Seek-at, not max.** Colonists go to bed at `SeekAt`, so the waking stretch
  ends there. Using `Max` would make days longer than anyone's real cycle.
- **Charts stay in ticks.** The performance and population charts measure the
  engine; a day there would only blur the resolution. Days are for the player's
  sense of elapsed colony time.
- **A clock in Go, not in the page.** The sim sends minutes since midnight
  rather than the day length, so the TUI and the page cannot disagree about
  where the landing falls or when midnight is.
- **Landing at 06:00, midnight rollover.** Colonists land with no sleep drive,
  so landing is their morning. A day number that turned over at landing would
  make 05:59 and 06:00 different days, which reads wrong next to a clock.
  Colonists do not sleep in sync (they go to bed when their own drive says so),
  so the clock is a label, not a schedule.
- **Sleep used to be 40 ticks.** That made a 740-tick day with a 1.3-hour
  night, which looked absurd on a clock. The fix was to make the night eight
  hours (`use-ticks` 360) rather than to bend the clock; `seek-at` moved from
  700 to 720 so the day is 1080 ticks and an hour a whole 45.
- **Why other drives (almost) pause in bed.** Lengthening the night alone broke the
  colony. Food comes due every ~325 ticks awake, so hunger pulled almost
  every colonist out of bed before a 360-tick night finished, the night
  restarted from zero, and sleep sat pinned near its ceiling. Measured over
  six seeds × 10,800 ticks: ~150 interrupted nights to ~8 finished per run,
  colonists in bed ~60% of the time, digging down ~80%, and one colony wiped
  out. Slowing rather than stopping the other needs did not work with integer
  rise rates: food rose 2/tick, so "slower" was either 1 (half, still enough
  to starve someone who went to bed hungry) or 0. With the pause: ~55 finished
  nights per run, a handful interrupted (only by fleeing), ~30% of time in bed
  (eight hours is 33%), and colony survival about where the 40-tick night left
  it (aliens do most of the killing either way). Digging per tick falls by
  about a quarter, which is the cost of colonists sleeping a third of the day.
  Drives made "slower" possible: rates are in thousandths, so food and bladder
  now grow at 10% in bed instead of stopping. That costs some nights, because a
  colonist who goes to bed just short of hungry wakes to eat. Over the same six
  seeds: 263 finished nights and 21 interrupted (13 by hunger, the rest by
  aliens), against 225 and 6 with the full pause; time in bed is unchanged at
  about 30%. Banking keeps each of those a short break. The tuning is in
  [drives.md](./drives.md).
- **Why banking.** A night is now long enough that something will
  occasionally interrupt it (an alien, mostly). Restarting from zero turned
  each interruption into a lost night; banking makes it a short break.
- **Zero rate.** If sleep is configured never to grow, the waking stretch falls
  back to `Max` ticks so `TicksPerDay` is still positive and `DayOf` never
  divides by zero.

## Extending it

- Showing days elsewhere (log lines, memories, the roster's "dead (tick N)"):
  call `DayOf(tick, cfg.TicksPerDay())` in Go, or carry the day length to the
  page if it needs to convert arbitrary ticks. Only the current day is on the
  wire today.
- Moving the landing time is `LandingHour`. It shifts both the clock and when
  the day number turns over; the tests in `drives_test.go` pin the 06:00 values.
- Another sleep trait is a `traitSpec` with `sleepHours` in the `sleep` group.
  Anything else that should grow differently while a colonist sleeps is its
  drive's `activity.asleep` percent.
- Nothing in the simulation reads the clock. A day/night mechanic (lighting,
  shift schedules) would want `MinuteOfDay` from Go, not the stat.

## Related

- [drives.md](./drives.md) — the sleep drive the day is derived from, and the asleep drive activity.
- [personality.md](./personality.md) — the trait table the sleep traits join.
- [wire-format.md](./wire-format.md) — how a new stat reaches the page.

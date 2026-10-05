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
move it an hour either way. A night is the sleep drive falling: in bed it runs
backwards until it reaches 0, and the night is over. Everything below follows
from that.

## Source

- [`internal/sim/drives.go`](../internal/sim/drives.go) — `Config.TicksPerDay`, `Config.NightTicks`, `LandingHour`, `DayOf`, and `MinuteOfDay`; the sleep `DriveSpec`; the sleep traits' night in `driveRate`.
- [`internal/sim/sleep.go`](../internal/sim/sleep.go) — `Config.TicksPerHour`, and `sleepTick`, which ends the night when the drive reaches 0.
- [`internal/sim/focus.go`](../internal/sim/focus.go) — `focusDrive`, which keeps a sleeper in bed until the night is over.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — the Short Sleeper / Long Sleeper traits (`sleepHours`), resolved into `Entity.sleepTicks`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `Stats.Day` and `Stats.MinuteOfDay`, filled in by `snapshot`.
- [`internal/ui/tui/view.go`](../internal/ui/tui/view.go) — `renderHeader` shows `day N  HH:MM` on the title line.
- [`web/src/ui/TopBar.svelte`](../web/src/ui/TopBar.svelte) — the web top bar's `day N HH:MM` readout.
- [`internal/sim/drives_test.go`](../internal/sim/drives_test.go) — `TestTicksPerDayFollowsSleep`, `TestDayOfCountsLandingAsDayOne`, `TestMinuteOfDayStartsAtLandingHour`.
- [`internal/sim/sleep_test.go`](../internal/sim/sleep_test.go) — trait night lengths, the night lasting until sleep is met, interrupted nights keeping what was slept, and waking to flee.
- [`internal/ui/tui/render_test.go`](../internal/ui/tui/render_test.go) — `TestHeaderShowsDayAndTimeOfDay`.

## How it works

```
NightTicks  = ceil(sleep.SeekAt × 1000 / (sleep.Rate × −asleep% / 100))
TicksPerDay = ceil(sleep.SeekAt × 1000 / sleep.Rate) + NightTicks
offset      = TicksPerDay * LandingHour / 24       // landing is 06:00
Day         = (tick + offset) / TicksPerDay + 1    // the landing is day 1
MinuteOfDay = (tick + offset) % TicksPerDay * 1440 / TicksPerDay
```

With the default sleep drive (rate 1000, one point a tick; seek-at 720; asleep
−200%, so it falls two points a tick in bed) a night is 360 ticks and a day is
**1080 ticks**. A colonist starts with no sleep drive, stays up until it reaches
`SeekAt` (when they drop work to go to bed), then sleeps it off: that cycle is
the day. A sleep drive that does not fall in bed has no night of its own, and
the bed's `use-ticks` stands in (it is 0 for sleep, meaning "until the drive is
met").

The clock stretches those ticks over 24 hours, so at defaults one clock hour
(`TicksPerHour`) is exactly **45 ticks**, 720 waking ticks are 16 hours, and the
360-tick night is 8. The colony lands at **06:00**, so a colonist on the base
rhythm goes to bed at 22:00 and gets up at 06:00. The day number turns over at
**midnight**, 810 ticks after landing, not at the landing's anniversary.

### The night's sleep

- **Sleep falls in bed.** A colonist in bed (`State == Sleeping`) is in the
  asleep drive activity, where each drive has its own percent
  (`drives.<name>.activity.asleep`): sleep −200%, so it falls; food and
  bladder 10%; social 0%. Getting into or out of bed is a change of drive
  activity, which `syncDriveActivity` notices after the turn and refreshes the
  rates for, so there is nothing to pair up and nothing to forget on an
  unusual way out of bed. See [drives.md](./drives.md).
- **A night lasts until the drive is met.** `sleepTick` ends the night (the
  "Slept in a bed." memory, the bed's fee) on the tick the sleep drive reaches
  0, not after a fixed number of ticks. A colonist that stayed up later sleeps
  longer: going to bed at 900 is a 450-tick night.
- **Staying in bed.** Below `SeekAt` the sleep drive no longer presses, so on
  its own a sleeper would get up halfway through the night. `focusDrive` holds a
  drive being satisfied in place at just short of critical while the colonist is
  using the facility, so only a critical drive, a fatal one (hunger), or a
  threat gets it up. Holding it at `SeekAt` was tried first: a sleeper then got
  up for any drive that was barely pressing, and interrupted nights went from 13
  to 71, 41 of them a mildly full bladder.
- **Length by trait.** `Entity.sleepTicks` is `NightTicks` plus
  `sleepHours × TicksPerHour` from its traits: Short Sleeper is −1 (315 ticks,
  seven hours) and Long Sleeper +1 (405, nine). They share the `sleep` trait
  group, so a colonist has at most one. `driveRate` scales the colonist's
  asleep rate by `NightTicks / sleepTicks`, so clearing `SeekAt` takes its own
  night. The calendar still uses the base night.
- **Interrupted nights keep what was slept.** There is nothing to bank: the
  drive has fallen as far as it has, and the colonist gets up that much less
  tired. Still above `SeekAt`, it goes back to bed and finishes from there;
  below it, it is rested enough to stay up, and the drive rises again from where
  the night left it.
- **No night at all.** A colonist whose sleep drive reaches its ceiling passes
  out where it stands, into the unconscious drive activity, where sleep falls
  at half the bed's rate (−100%). It comes to once the drive is below
  `CriticalAt`, still tired, about 100 ticks later, and goes to find a bed. See
  [drives.md](./drives.md).

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
  hours (then `use-ticks` 360, now an asleep rate of −200%) rather than to bend
  the clock; `seek-at` moved from 700 to 720 so the day is 1080 ticks and an
  hour a whole 45.
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
  colonist who goes to bed just short of hungry wakes to eat (hunger is fatal,
  so once it presses it outranks sleep). Over the same six seeds, with nights
  that last until sleep is met: 234 finished nights and 49 interrupted (32 by
  hunger, 11 by a critical bladder, the rest aliens), against 225 and 6 with the
  full pause; time in bed 33%. An interrupted night is now cheap, because what
  was slept is kept. The tuning is in [drives.md](./drives.md).
- **Why the drive falls instead of a timer.** The night used to be a fixed
  `use-ticks` counted in `sleepBanked`, while the sleep drive kept rising in
  bed and only reset when the count was done. Three things went wrong once
  drives had consequences:
  - Everyone reached the ceiling in bed. A colonist that lay down at 720 hit
    1000 after 280 ticks of a 360-tick night, and only an "already in bed"
    exemption kept it from passing out there. One woken late in the night got
    up already at the ceiling and collapsed in the corridor.
  - Passing out (60 ticks, then a full reset) was a far cheaper night than a
    bed (360 ticks).
  - Passing out ignored the bank, so a half-slept night carried on after it.

  With the drive falling, the level is the bank, a night is as long as the
  colonist is tired, and passing out is the same thing on the floor at half the
  rate, so how long it lasts comes out of the drive too.
- **Zero rate.** If sleep is configured never to grow, the waking stretch falls
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
  the day number turns over; the tests in `drives_test.go` pin the 06:00 values.
- Another sleep trait is a `traitSpec` with `sleepHours` in the `sleep` group.
  Anything else that should grow differently while a colonist sleeps is its
  drive's `activity.asleep` percent; a slower night (a worse bed) is a less
  negative one for sleep.
- Nothing in the simulation reads the clock. A day/night mechanic (lighting,
  shift schedules) would want `MinuteOfDay` from Go, not the stat.

## Related

- [drives.md](./drives.md) — the sleep drive the day is derived from, and the asleep drive activity.
- [personality.md](./personality.md) — the trait table the sleep traits join.
- [wire-format.md](./wire-format.md) — how a new stat reaches the page.

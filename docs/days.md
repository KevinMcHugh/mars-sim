# Colony days

> Part of the [mars-sim documentation](./README.md).

## What it is

A calendar for the player: the top bar (web) and header (TUI) show **day N**
since landing and the **time of day** (`day 3 07:05`) beside the raw tick. A day's length is not its own knob; it is
derived from the sleep need, so a day means "one waking stretch and one night".
Everything inside the simulation, and the performance charts, still count in
ticks.

## Source

- [`internal/sim/needs.go`](../internal/sim/needs.go) — `Config.TicksPerDay`, `LandingHour`, `DayOf`, and `MinuteOfDay`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `Stats.Day` and `Stats.MinuteOfDay`, filled in by `snapshot`.
- [`internal/ui/tui/view.go`](../internal/ui/tui/view.go) — `renderHeader` shows `day N  HH:MM` on the title line.
- [`web/src/ui/TopBar.svelte`](../web/src/ui/TopBar.svelte) — the web top bar's `day N HH:MM` readout.
- [`internal/sim/needs_test.go`](../internal/sim/needs_test.go) — `TestTicksPerDayFollowsSleep`, `TestDayOfCountsLandingAsDayOne`, `TestMinuteOfDayStartsAtLandingHour`.
- [`internal/ui/tui/render_test.go`](../internal/ui/tui/render_test.go) — `TestHeaderShowsDayAndTimeOfDay`.

## How it works

```
TicksPerDay = ceil(sleep.SeekAt / sleep.Rise) + sleep.UseTicks
offset      = TicksPerDay * LandingHour / 24       // landing is 06:00
Day         = (tick + offset) / TicksPerDay + 1    // the landing is day 1
MinuteOfDay = (tick + offset) % TicksPerDay * 1440 / TicksPerDay
```

With the default sleep need (rise 1, seek-at 700, use-ticks 40) a day is
**740 ticks**. A colonist starts with no sleep need, stays up until it reaches
`SeekAt` (when they drop work to go to bed), then spends `UseTicks` in a bunk:
that cycle is the day.

The clock stretches those ticks over 24 hours, so at defaults one clock hour is
about 31 ticks. The colony lands at **06:00**, and the day number turns over
at **midnight**, 555 ticks after landing, not at the landing's anniversary.

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
  The cost: sleep (the last `UseTicks` of the cycle) falls at about
  04:42-06:00, so "night" on the clock is short. That is the sleep need's
  real proportion (40 of 740 ticks), not a bug; colonists also do not sleep in
  sync, so the clock is a label, not a schedule.
- **Zero rise.** If sleep is configured never to rise, the waking stretch falls
  back to `Max` ticks so `TicksPerDay` is still positive and `DayOf` never
  divides by zero.

## Extending it

- Showing days elsewhere (log lines, memories, the roster's "dead (tick N)"):
  call `DayOf(tick, cfg.TicksPerDay())` in Go, or carry the day length to the
  page if it needs to convert arbitrary ticks. Only the current day is on the
  wire today.
- Moving the landing time is `LandingHour`. It shifts both the clock and when
  the day number turns over; the tests in `needs_test.go` pin the 06:00 values.
- Nothing in the simulation reads the clock. A day/night mechanic (lighting,
  shift schedules) would want `MinuteOfDay` from Go, not the stat.

## Related

- [needs.md](./needs.md) — the sleep need the day is derived from.
- [wire-format.md](./wire-format.md) — how a new stat reaches the page.

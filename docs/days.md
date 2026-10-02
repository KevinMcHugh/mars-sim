# Colony days

> Part of the [mars-sim documentation](./README.md).

## What it is

A calendar for the player: the top bar (web) and header (TUI) show **day N**
since landing beside the raw tick. A day's length is not its own knob; it is
derived from the sleep need, so a day means "one waking stretch and one night".
Everything inside the simulation, and the performance charts, still count in
ticks.

## Source

- [`internal/sim/needs.go`](../internal/sim/needs.go) — `Config.TicksPerDay` and `DayOf`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `Stats.Day`, filled in by `snapshot`.
- [`internal/ui/tui/view.go`](../internal/ui/tui/view.go) — `renderHeader` shows `day N | tick T | R tps`.
- [`web/src/ui/TopBar.svelte`](../web/src/ui/TopBar.svelte) — the web top bar's `day N` readout.
- [`internal/sim/needs_test.go`](../internal/sim/needs_test.go) — `TestTicksPerDayFollowsSleep`, `TestDayOfCountsLandingAsDayOne`.

## How it works

```
TicksPerDay = ceil(sleep.SeekAt / sleep.Rise) + sleep.UseTicks
Day         = tick / TicksPerDay + 1        // the landing, tick 0, is day 1
```

With the default sleep need (rise 1, seek-at 700, use-ticks 40) a day is
**740 ticks**. A colonist starts with no sleep need, stays up until it reaches
`SeekAt` (when they drop work to go to bed), then spends `UseTicks` in a bunk:
that cycle is the day.

`Day` is an `int` field of `sim.Stats`, so it reaches the browser through the
frame's stats section with no wire change (see
[wire-format.md](./wire-format.md)); the page reads `ui.stats.Day`. It is the
last field so existing stat indices stay put.

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
- **Zero rise.** If sleep is configured never to rise, the waking stretch falls
  back to `Max` ticks so `TicksPerDay` is still positive and `DayOf` never
  divides by zero.

## Extending it

- Showing days elsewhere (log lines, memories, the roster's "dead (tick N)"):
  call `DayOf(tick, cfg.TicksPerDay())` in Go, or carry the day length to the
  page if it needs to convert arbitrary ticks. Only the current day is on the
  wire today.
- A time-of-day (day/night) would be `tick % TicksPerDay`; nothing uses it yet,
  and colonists do not sleep in sync, so it would be a label, not a schedule.

## Related

- [needs.md](./needs.md) — the sleep need the day is derived from.
- [wire-format.md](./wire-format.md) — how a new stat reaches the page.

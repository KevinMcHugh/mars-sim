# Charge/grip affect

> Part of the [mars-sim documentation](./README.md).

## What it is

Affect is a colonist's bounded `(charge, grip)` response to accumulated life
events. The numeric axes bias focus arbitration; a cached contextual mood word
projects them for display without becoming behavioral state.

## Source

- [`internal/sim/affect.go`](../internal/sim/affect.go) — vectors, trait appraisal, decay, attractors, contextual valence, and labels.
- [`internal/sim/lifeevents.go`](../internal/sim/lifeevents.go) — event kinds and the temporary per-conversation `Outcome`.
- [`internal/sim/world.go`](../internal/sim/world.go) — `remember`, the single affect/stimulus/memory ingestion funnel.
- [`internal/sim/focus.go`](../internal/sim/focus.go) — additive charge/grip focus contributions.
- [`internal/sim/affect_test.go`](../internal/sim/affect_test.go) — semantic tables, transforms, decay, labels, and behavioral ordering.

## How it works

### Coordinates and events

Charge is energy behind the next action; grip is felt control. Both are clamped
to `[-MoodMax, MoodMax]`. Each `LifeEventKind` has one semantic `MoodVector` in
`lifeEventMoodVectors`. `remember` applies the vector once per occurrence before
updating stimulus and memory state, so collapsed memories still affect the
colonist each time.

Conversations preserve their existing per-occurrence calculation. `talkMoodDelta`
and `noteConversation` produce a temporary signed `LifeEvent.Outcome`; the
funnel converts a positive outcome mostly into grip, and a negative outcome into
lower grip plus raised charge. No scalar outcome remains on the entity.

Traits transform vectors in `Trait` declaration order:

| Trait | Appraisal |
| --- | --- |
| Tidy | scales gore by 2.2 and doubles incineration grip relief |
| Industrious | doubles every finished-work vector |
| Mutant-Lover | reflects mutation grip |
| Introvert | reflects conversation charge |

These event-time checks are intentionally rare-path `HasTrait` calls. Per-tick
need/work effects remain resolved at spawn.

### Decay and focus

At the start of each colonist turn, `approach` moves charge and grip independently
toward home `(0, 0)` without overshoot. Defaults decay charge by 2 and grip by 1,
so activation settles before felt control. Focus scoring normalizes each axis by
`MoodMax`, multiplies it by the existing focus `ChargeWeight`/`GripWeight`, and
adds the result to `FocusScore.Affect`. It performs no string work and never reads
the displayed mood kind.

### Display projection

Nine declared attractors claim nearby points with integer squared-distance math.
Declaration order breaks equal claims. `MoodLabelSwitchMargin` keeps the incumbent
kind until a challenger has a real advantage, preventing boundary flicker.
Unclaimed space is `MoodSettling`.

Good/bad wording is contextual: maximum need pressure, injury, and a live visible
threat determine valence. Thus one `MoodDriven` point can display as `driven` or
`furious`. Context changes only the cached word choice, never coordinates or
`MoodKind`, and neither is fed back into focus scoring. Snapshots expose charge,
grip, and the cached word; the roster shows all three on the old single mood line.

## Why it is this way

- A scalar good/bad value duplicated needs, health, and threats but could not
  distinguish fatigue from fear. Charge and grip directly describe useful action
  tendencies.
- Cartesian integer decay avoids floats, angular edge cases, and another
  scheduler. Polar decay was deliberately deferred until a behavioral need
  justifies it.
- Labels are projections rather than states. Branching on `"panicked"` would
  hide numeric thresholds in UI vocabulary and double-count the context used to
  choose that word.
- Event vectors are semantic code tables, not CLI knobs. Decay and hysteresis are
  user-facing pacing choices and therefore belong in `Config`.

## Extending it

Add a mood-bearing event by adding one row to `lifeEventMoodVectors` and emitting
it only through `remember`. Add an event-specific trait transform to
`transformMoodVector`; preserve declaration-order iteration. Add or tune an
attractor in `moodAttractors`, remembering that order is the tie-break. Never use
`MoodName` or `MoodKind` in focus scoring or an executor.

## Related

- [memories.md](./memories.md) — the single event ingestion funnel and memory collapse.
- [personality.md](./personality.md) — trait ordering and the personality RNG invariant.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — fixed design and score model.

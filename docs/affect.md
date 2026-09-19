# Charge/grip/valence affect

> Part of the [mars-sim documentation](./README.md).

## What it is

Affect is a colonist's bounded `(charge, grip, valence)` response to their life
so far. Charge and grip bias focus arbitration; valence records how that life
has been going and picks which word the other two display as, without ever
becoming behavioral state itself.

How hard an event lands is not fixed. Each kind carries an **impact**, and
impact alone decides whether the event nudges affect or relocates it outright.

## Source

- [`internal/sim/affect.go`](../internal/sim/affect.go) — appraisals, the push/pull blend, trait transforms, decay, attractors, and labels.
- [`internal/sim/lifeevents.go`](../internal/sim/lifeevents.go) — event kinds and the temporary per-conversation `Outcome`.
- [`internal/sim/world.go`](../internal/sim/world.go) — `remember`, the single affect/stimulus/memory ingestion funnel.
- [`internal/sim/focus.go`](../internal/sim/focus.go) — additive charge/grip focus contributions.
- [`internal/sim/affect_test.go`](../internal/sim/affect_test.go) — semantic tables, transforms, decay, labels, and behavioral ordering.

## How it works

### Coordinates and events

Charge is energy behind the next action, grip is felt control, and valence is
whether life has been going well. All three are clamped to
`[-MoodMax, MoodMax]`. Each `LifeEventKind` has one `moodAppraisal` in
`lifeEventAppraisals`: an `Impact` and a `Target` vector. `remember` applies it
once per occurrence before updating stimulus and memory state, so collapsed
memories still reach the colonist each time.

A `Target` is both a displacement and a destination — see the blend below —
which is why the handful of high-impact rows are written at the scale of the
plane rather than the scale of a nudge.

Valence is mostly zero for routine upkeep, and deliberately so. Charge and grip
shed points every turn, so a steady drip of small numbers goes nowhere; valence
settles a point at a time, and a colonist digs far more often than that. Paying
valence for ordinary work pegged every colonist at the maximum within a few
hundred ticks, which is the "everyone always reads as fine" failure the axis
exists to fix. Only what a colonist would count as a good or bad day moves it.

Conversations preserve their existing per-occurrence calculation. `talkMoodDelta`
and `noteConversation` produce a temporary signed `LifeEvent.Outcome`; the
funnel converts a positive outcome mostly into grip, a negative outcome into
lower grip plus raised charge, and either into a quarter as much valence. No
scalar outcome remains on the entity, and `EvtConversation` is the one row that
declares an impact but no target, because its target cannot be a table lookup.

### Push, pull, and impact

`blendAffect` composes one operation out of two. `moodPull` turns an impact into
a percentage: zero at or below `MoodPushImpact`, 100 at or above
`MoodPullImpact`, linear between them. The colonist then keeps that much less of
where they were before the target is added:

```text
keep  = 100 - pull
axis' = axis*keep/100 + target
```

At pull 0 this is plain addition, and routine life accumulates exactly as it
always did. At pull 100 the colonist ends at the target no matter where they
started. That asymmetry is the point: under pure addition a good enough day
would soften a killing, and it no longer can, because the killing moves the
colonist rather than adding to them.

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

At the start of each colonist turn, `approach` moves each axis independently
toward home `(0, 0, 0)` without overshoot. Defaults decay charge by 2 and grip
by 1 per turn, so activation settles before felt control. Valence answers to
hours rather than minutes, so it gives up one point every `MoodValenceDecayTicks`
ticks instead of points every turn; the period counts world ticks rather than
per-colonist turns, which keeps a seeded run reproducible.

Focus scoring normalizes charge and grip by `MoodMax`, multiplies each by the
focus's `ChargeWeight`/`GripWeight`, and adds the result to `FocusScore.Affect`.
It performs no string work, never reads the displayed mood kind, and never reads
valence. A valence-only change therefore does not mark cognition dirty: a
colonist whose mood is merely settling has no reason to reconsider what they
are doing.

### Display projection

Nine declared attractors claim nearby points with integer squared-distance math.
Declaration order breaks equal claims. `MoodLabelSwitchMargin` keeps the incumbent
kind until a challenger has a real advantage, preventing boundary flicker.
Unclaimed space is `MoodSettling`.

Good/bad wording reads the stored valence: one `MoodDriven` point displays as
`driven` while valence is at or above zero and `furious` below it. Valence
changes only the word, never coordinates or `MoodKind`, and the word is never
fed back into focus scoring. `moodName` indexes `moodAttractors` by `MoodKind`,
which only works while the table stays in declaration order — a test guards
that. Snapshots expose all three axes plus the word; the roster shows them on
the old single mood line.

## Why it is this way

- A scalar good/bad value could not distinguish fatigue from fear. Charge and
  grip directly describe useful action tendencies.
- Valence is stored rather than derived from needs and HP, which is how it
  worked first. Deriving it does not survive a counterexample: a well-fed,
  unhurt colonist standing nowhere near an alien reads as *good* however recently
  they watched someone die. Two colonists at identical coordinates now read
  differently because they have had different lives, which is the entire reason
  the axis exists. Its cost is one int and one comparison; the derived version
  cost a need loop and a threat search on the display path.
- Valence stays out of focus scoring. Needs, HP and visible threats already
  reach the score directly, so letting the axis that summarizes them back in
  would count them twice.
- Cartesian integer decay avoids floats, angular edge cases, and another
  scheduler. Polar decay was deliberately deferred until a behavioral need
  justifies it.
- Labels are projections rather than states. Branching on `"panicked"` would
  hide numeric thresholds in UI vocabulary and double-count the context used to
  choose that word.
- Event vectors are semantic code tables, not CLI knobs. Decay and hysteresis are
  user-facing pacing choices and therefore belong in `Config`.

## Extending it

Add a mood-bearing event by adding one row to `lifeEventAppraisals` — an impact
and a target — and emitting it only through `remember`. Keep the target
nudge-sized below `MoodPushImpact` and plane-sized above `MoodPullImpact`; a
test checks that anything which relocates lands in named space, because
relocating to a nudge-sized point would leave a colonist almost exactly neutral
after something terrible. Add an event-specific trait transform to
`transformMoodVector`; preserve declaration-order iteration. Add or tune an
attractor in `moodAttractors`, remembering that order is the tie-break. Never use
`MoodName` or `MoodKind` in focus scoring or an executor.

## Related

- [memories.md](./memories.md) — the single event ingestion funnel and memory collapse.
- [personality.md](./personality.md) — trait ordering and the personality RNG invariant.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — fixed design and score model.
- [mood-space.md](./mood-space.md) — what is still proposed on top of this: habituation and tag-based trait rules.

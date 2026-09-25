# Charge/grip/valence affect

> Part of the [mars-sim documentation](./README.md).

## What it is

Affect is a colonist's bounded `(charge, grip, valence)` response to their life
so far. Charge and grip bias focus arbitration; valence records how that life
has been going and picks which word the other two display as, without ever
becoming behavioral state itself.

How hard an event lands is not fixed. Each reaction in
[`cognition.yaml`](../cognition.yaml) carries an **impact**, which decides
whether the percept nudges affect or relocates it outright, and a pair of
readings — **fresh** and **worn** — between which a colonist's history with
that reaction picks.

## Source

- [`internal/sim/affect.go`](../internal/sim/affect.go) — the push/pull blend,
  wear-policy registry, grammar-matched trait scales, decay toward
  `affectHome`, attractors, and labels.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go) —
  reaction `Fresh`/`Worn` pairs, wear-policy IDs, and trait rules.
- [`internal/sim/world.go`](../internal/sim/world.go) — `rememberPercept`, the
  single affect/stimulus/memory ingestion funnel.
- [`internal/sim/focus.go`](../internal/sim/focus.go) — additive charge/grip
  focus contributions and `affectSettled()`.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — per-colonist
  `affectHome` resolved from outlook traits.
- [`internal/sim/affect_test.go`](../internal/sim/affect_test.go) — semantic
  tables, wear, baselines, transforms, decay, labels, and behavioral ordering.

## How it works

### Coordinates and reactions

Charge is energy behind the next action, grip is felt control, and valence is
whether life has been going well. All three are clamped to
`[-MoodMax, MoodMax]`. Each reaction has one appraisal: an `Impact` and a
`Fresh`/`Worn` pair of targets. `rememberPercept` applies it once per
occurrence before updating stimulus and memory state, so collapsed memories
still reach the colonist each time.

A target is both a displacement and a destination — see the blend below —
which is why the handful of high-impact rows are written at the scale of the
plane rather than the scale of a nudge.

Valence is mostly zero for routine upkeep, and deliberately so. Charge and grip
shed points every turn, so a steady drip of small numbers goes nowhere; valence
settles a point at a time, and a colonist digs far more often than that. Paying
valence for ordinary work pegged every colonist at the maximum within a few
hundred ticks, which is the "everyone always reads as fine" failure the axis
exists to fix. Only what a colonist would count as a good or bad day moves it.

Conversations preserve their existing per-occurrence calculation.
`talkMoodDelta` and `noteConversation` produce a per-observer contextual
target on the shared occurrence. The conversation reaction declares an impact
and uses wear policy `none`, because its target cannot be a table lookup and
its repetition is already social fatigue.

### Wear: fresh, worn, and policy

`memory-occasions` counts how many remembered **occasions** a colonist has of
a reaction (`Memory.Rule == reaction.ID`), multiplies by a trait-scaled
`MoodWearPerOccasion`, and caps at 100. `wearTarget` then moves that
reaction's appraisal that far from `Fresh` toward `Worn` with integer
`roundedDiv` interpolation.

Two details carry the design:

- **Occasions, not occurrences.** A run of digs collapses into one memory, and
  collapsing has already decided that run was one memorable thing. Counting
  `Memory.Count` instead would let a single long shift peg a colonist
  permanently, which is the ratchet this has to avoid.
- **The count comes from the bounded memory log**, so occasions roll off with
  the memories holding them and wear falls again once something stops
  happening. Recovery is most of what keeps this feeling like a person rather
  than a counter.

Note which way round the pair runs for the traumatic kinds: `Fresh` is the
*stronger* reading. A first witnessed killing lands near `furious` — grip up,
valence down, a rallying cry — and the tenth lands near `despairing`. Wear
turns a reaction rather than merely quieting it, which a per-trait scale factor
could not express.

An activity a colonist does constantly does reach fully worn and stay there.
That is the intended reading: they really are habituated to their job, and what
it costs them is the lift the work used to give. The arc matters where it
should, on the rare and terrible things, whose occasions accumulate slowly and
roll off in between.

Conversation does not hard-code an exemption in `applyAffect`. It selects wear
policy `none`, which returns the contextual target when present and otherwise
`Fresh`. The registry is the seam for a later relationship-aware conversation
policy; that policy is not implemented here.

Resilient scales global wear rate to 40. Cowardly scales it to 180. Those
rules are data in `cognition.yaml`, not a switch in Go.

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

Traits scale impact and the resolved target through grammar-matched
`trait_rules` in YAML declaration order. The shipped rules preserve the old
transforms:

| Trait | Appraisal |
| --- | --- |
| Tidy | scales visible gore, witnessed colonist death, witnessed mouse crush, and cleaning refuse by 2.2; doubles incineration grip relief |
| Industrious | doubles the actor's finished mine/clear/construct/clean/incinerate vectors |
| Mutant-Lover | reflects mutation grip for doing it or watching it |
| Introvert | reflects conversation charge |
| Cowardly | 1.5× impact on visible alien, being bitten, and witnessed gunfight |
| Extrovert | 1.3× charge and 1.5× valence when the object relation is `friend` and an alien kills or bites that colonist |

These event-time checks are intentionally rare-path `HasTrait` calls plus a
short rule walk. Per-tick need/work effects remain resolved at spawn.

### Baselines

A colonist decays toward `affectHome`, not the origin. Outlook traits set
that home at spawn: Optimist is `{grip: 8, valence: 25}`, Pessimist is
`{grip: -8, valence: -25}`. Re-resolving traits later updates home without
overwriting current affect. A new colonist starts at home.

`affectSettled()` is true when charge and grip have reached home. Focus
caching uses that instead of "are we at the origin?", so an Optimist resting
at a non-zero home does not re-arbitrate every tick.

Valence still decays toward 0 on its own slower clock. The home valence is a
resting expectation after events, not a second valence decay target.

### Decay and focus

At the start of each colonist turn, `approach` moves charge and grip
independently toward `affectHome` without overshoot. Defaults decay charge by
2 and grip by 1 per turn, so activation settles before felt control. Valence
answers to hours rather than minutes, so it gives up one point every
`MoodValenceDecayTicks` ticks instead of points every turn; the period counts
world ticks rather than per-colonist turns, which keeps a seeded run
reproducible.

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
fed back into focus scoring. Attractors now live in `cognition.yaml` and are
indexed by `MoodKind` in declaration order — a test still guards that.
Snapshots expose all three axes plus the word; the roster shows them on the
old single mood line.

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
- Event vectors are semantic data in `cognition.yaml`, not CLI knobs. Decay,
  hysteresis, wear-per-occasion, and the friend-affinity threshold are
  user-facing pacing choices and therefore belong in `Config`.
- Decay-to-home rather than decay-to-zero is how two colonists with the same
  day still read differently at rest. `affectSettled` exists so that difference
  does not fight the cognition cache.

## Extending it

Add a mood-bearing reaction in `cognition.yaml` — an impact, a fresh/worn pair,
and a wear policy — and emit the occurrence only through `emitOccurrence` /
`rememberPercept`. Keep the target nudge-sized below `MoodPushImpact` and
plane-sized above `MoodPullImpact`; a test checks that anything which
relocates lands in named space, because relocating to a nudge-sized point
would leave a colonist almost exactly neutral after something terrible. Add a
grammar-matched trait rule rather than a Go switch; rules compose in file
order. Add or tune an attractor in `cognition.yaml`, remembering that order is
the tie-break. Never use `MoodName` or `MoodKind` in focus scoring or an
executor.

## Related

- [compositional-perception-and-events.md](./compositional-perception-and-events.md)
  — occurrence/percept/reaction grammar and wear-policy seam.
- [cognition-config-and-lab.md](./cognition-config-and-lab.md) — authoring
  schema and the Cognition Lab.
- [memories.md](./memories.md) — the single event ingestion funnel and memory collapse.
- [personality.md](./personality.md) — trait groups, affect homes, and the
  personality RNG invariant.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — fixed design and score model.
- [mood-space.md](./mood-space.md) — what is still open after wear, trait
  rules, and baselines shipped.

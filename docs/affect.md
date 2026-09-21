# Charge/grip/valence affect

> Part of the [mars-sim documentation](./README.md).

## What it is

Affect is a colonist's bounded `(charge, grip, valence)` response to their life
so far. Charge and grip bias focus arbitration; valence records how that life
has been going and picks which word the other two display as, without ever
becoming behavioral state itself.

How hard an event lands is not fixed. Each kind carries an **impact**, which
decides whether the event nudges affect or relocates it outright, and a pair of
readings — **fresh** and **worn** — between which a colonist's history with that
kind of thing picks.

## Source

- [`internal/sim/affect.go`](../internal/sim/affect.go) — the push/pull blend, wear, trait rules, decay, attractors, and labels.
- [`internal/sim/events.yaml`](../internal/sim/events.yaml) — the appraisal, stimulus and collapse tables, as data.
- [`internal/sim/lifeevents.go`](../internal/sim/lifeevents.go) — event kinds, `Subject` versus `Source`, and the temporary per-conversation `Outcome`.
- [`internal/sim/world.go`](../internal/sim/world.go) — `remember`, the single affect/stimulus/memory ingestion funnel.
- [`internal/sim/focus.go`](../internal/sim/focus.go) — additive charge/grip focus contributions.
- [`internal/sim/affect_test.go`](../internal/sim/affect_test.go) — semantic tables, transforms, decay, labels, and behavioral ordering.

## How it works

### Coordinates and events

Charge is energy behind the next action, grip is felt control, and valence is
whether life has been going well. All three are clamped to
`[-MoodMax, MoodMax]`. Each `LifeEventKind` has one `moodAppraisal` in
`lifeEventAppraisals`, loaded from [`events.yaml`](../internal/sim/events.yaml):
an `Impact` and a `Fresh`/`Worn` pair of targets. `remember` applies it
once per occurrence before updating stimulus and memory state, so collapsed
memories still reach the colonist each time.

A target is both a displacement and a destination — see the blend below —
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

### Wear: fresh and worn

`moodWear` counts how many remembered **occasions** a colonist has of a kind,
multiplies by `MoodWearPerOccasion`, and caps at 100. `wearTarget` then moves
that kind's appraisal that far from `Fresh` toward `Worn`.

Two details carry the design:

- **Occasions, not occurrences.** A run of digs collapses into one memory, and
  collapsing has already decided that run was one memorable thing. Counting
  each occurrence instead would let a single long shift peg a colonist
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

Conversations are exempt. Their vector is already computed per occurrence, and
`noteConversation`'s social fatigue window is wearing repetition down by the
time appraisal happens; wearing it again would charge a talkative colonist
twice for the same talkativeness.

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

### Tags and trait rules

Traits do not react to event kinds. Each appraisal declares a set of `EventTag`
flavors — a bitmask, so matching is a couple of ANDs and allocates nothing —
and each entry in `traitRules` names the tags it cares about plus what it does
about them:

| Trait | Reacts to | Does |
| --- | --- | --- |
| Tidy | `gore` | scales the whole vector by 2.2 |
| Tidy | `incineration` | doubles grip relief |
| Industrious | `finished-work` | doubles the whole vector |
| Introvert | `social` | reflects charge |
| Mutant-Lover | `mutation` | reflects grip and valence |
| Extrovert | `social-loss` **and** `friend` | 1.3x charge, 1.5x valence |
| Resilient | anything | wears at 0.4x |
| Cowardly | anything | wears at 1.8x |
| Cowardly | `threat` | 1.5x impact |

Every factor is a percentage where 100 — or an unset 0 — means no change,
matching the convention `traitSpecs` already uses; a reflection is simply
`-100`. Percentages rather than floats because appraisal is integer throughout,
so a seeded run reproduces. `pct` truncates toward zero exactly as the
per-trait arithmetic it replaced did, so the four transforms that predate the
table still land on the same numbers.

Rules apply in **rule declaration order**, so a colonist carrying two of them
composes them the same way regardless of what order their traits are stored in.

This is what keeps adding a trait from being a decision against every event and
vice versa: a new event picks its tags, a new trait picks the tags it cares
about, and neither has to know the other exists. It also generalizes in ways
the old `switch` could not — Tidy was written against `EvtSawGore`, and now
reacts to a colonist being eaten too, because that is also gore.

Three of the factors do something no vector scale could say. `WearRate` changes
how fast a colonist stops being new to a thing rather than how hard it hits;
`Impact` changes how big a deal it is, and so whether it nudges them or moves
them.

**Dynamic tags.** `TagFriend` is the one tag no table can declare: `eventTags`
stamps it per occurrence when the colonist was close enough (`MoodFriendAffinity`)
to whoever the event happened *to*. That is why `LifeEvent` distinguishes
`Source` (what caused it — what stimulus tracking keys on) from `Subject` (who
it happened to). Relationship-, health- and location-dependent reactions all
fall out of the same mechanism without the rule table learning about any of it.

These event-time checks are intentionally rare-path `HasTrait` calls. Per-tick
need/work effects remain resolved at spawn.

### Decay and focus

At the start of each colonist turn, `approach` moves each axis independently
toward that colonist's own resting point without overshoot. Defaults decay charge by 2 and grip
by 1 per turn, so activation settles before felt control. Valence answers to
hours rather than minutes, so it gives up one point every `MoodValenceDecayTicks`
ticks instead of points every turn; the period counts world ticks rather than
per-colonist turns, which keeps a seeded run reproducible.

### Baselines: where a colonist settles

Home is `(0, 0, 0)` for most colonists and is not for anyone with a
temperament. `traitSpecs.affectHome` displaces it, summed across traits and
resolved once at spawn by `resolveTraitEffects` — the same pay-once path as
need rates and work speed, so the per-turn cost stays a subtraction rather than
a trait scan. A colonist is also *born* at their resting point rather than at
everyone's origin: a pessimist has been one since before the sim began.

| Trait | Home |
| --- | --- |
| Optimist | grip +8, valence +25 |
| Pessimist | grip −8, valence −25 |

Valence carries the outlook, which is why the difference is legible at rest: a
pessimist with nothing wrong reads `flat` where everyone else reads `steady`.
The small grip offset is the part that reaches behavior, since flee and fight
read grip — kept small deliberately, because grip feeding back into choices
that produce more events is the doom-spiral risk the proposal flagged.

Charge is deliberately left alone. It describes the energy behind the *next
action*, so a permanent offset would misdescribe what the axis is for, and it
is the fastest-decaying axis by design.

`resolveTraitEffects` never touches current affect, only the home it will
settle toward: it runs again when a trait is acquired in play, and a colonist
who has just mutated should not have the mood that produced wiped.

**`affectSettled` is not the same as neutral.** Cognition caching asks whether
a mood is still moving, and a colonist with a temperament rests somewhere other
than the origin. Comparing against zero would leave them re-arbitrating every
tick for the rest of the run.

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

Add a mood-bearing event by adding one row to `events.yaml` — tags, an impact
and a fresh/worn pair — and emitting it only through `remember`. The numbers
live in data; what fires the event does not, and deliberately so: see the
header of `eventdata.go`. Keep the target
nudge-sized below `MoodPushImpact` and plane-sized above `MoodPullImpact`; a
test checks that anything which relocates lands in named space, because
relocating to a nudge-sized point would leave a colonist almost exactly neutral
after something terrible. Give a trait a reaction by adding a row to `traitRules` against tags that
already exist; no event definition changes. Give an event a new flavor by
adding a tag to its appraisal; no trait changes. Only reach for a new
`EventTag` when no existing one describes the thing a trait would want to
react to. Add or tune an
attractor in `moodAttractors`, remembering that order is the tie-break. Never use
`MoodName` or `MoodKind` in focus scoring or an executor.

## Related

- [memories.md](./memories.md) — the single event ingestion funnel and memory collapse.
- [personality.md](./personality.md) — trait ordering and the personality RNG invariant.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — fixed design and score model.
- [mood-space.md](./mood-space.md) — the design record behind all of this: the dead ends, the open questions, and the tuning sandbox.

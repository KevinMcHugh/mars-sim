# Memories & life events

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists retain a short history of notable experiences — eating, mining,
conversing, sighting an alien, watching a fight — recorded as `Memory`
entries. Every one of these is built from a `LifeEvent`: a `LifeEventKind`
(what kind of thing happened) paired with the player-facing text describing
this particular occurrence. A `LifeEventKind` also carries a table of mood
effects, so remembering something and it affecting a colonist's mood are the
same act, not two systems that have to be kept in sync by hand.

## Source

- [`internal/sim/lifeevents.go`](../internal/sim/lifeevents.go) — `LifeEventKind`,
  `LifeEvent`, `event`, `MoodEffect`, `lifeEventMoodEffects`, `applyMoodEffects`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Memory` (now carries
  `Kind`), the colonist's `Memories`/`seen`/`seeingGore` fields.
- [`internal/sim/world.go`](../internal/sim/world.go) — `remember`, the one
  funnel every memory and mood effect goes through.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `observeNearby`
  (creature sightings) and `observeGore` (gore sightings); every other
  `remember` call site records an experience at its completion.
- [`internal/sim/relationships.go`](../internal/sim/relationships.go) —
  `adjustMood`, the mood field's clamp primitive, called only from
  `applyMoodEffects` now.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — `TraitTidy`
  and `TraitIndustrious`, the traits a mood effect is conditioned on today
  (see [personality.md](./personality.md)).
- [`internal/sim/lifeevents_test.go`](../internal/sim/lifeevents_test.go),
  [`internal/sim/memories_test.go`](../internal/sim/memories_test.go) — the
  tests that pin this behavior.

## How it works

### LifeEvent: text and mood effect, built together

```go
type LifeEvent struct {
    Kind LifeEventKind
    Text string
    Mood int // extra delta on top of Kind's table effects; 0 for a table-only event
}
```

Almost every call site constructs one with `event(kind, format, args...)`,
which is `fmt.Sprintf` plus the `Kind` tag and leaves `Mood` at 0:

```go
w.remember(prey, event(EvtBitten, "Bitten in the %s by an alien!", part))
```

`remember` (in `world.go`) is the single funnel: it appends the `Memory`
(bounded at 64 per colonist, oldest evicted first) *and* calls
`applyMoodEffects(e, evt)` — recording something and it moving mood can never
drift apart, because they happen in the same call, the same way
`World.remove` is the one funnel for a death and its graveyard record (see
[combat.md](./combat.md)). There is exactly one path a mood change can take
in the whole simulation: through `remember`.

### Mood effects: a fixed table, plus room for a computed delta

```go
type MoodEffect struct {
    Conditional bool
    Trait       Trait // meaningful only when Conditional
    Delta       int
}

var lifeEventMoodEffects = [numLifeEventKinds][]MoodEffect{
    EvtSawAlien: {{Delta: -6}},
    EvtSawMouse: {{Delta: -2}},
    EvtSawGore: {
        {Delta: -4},
        {Conditional: true, Trait: TraitTidy, Delta: -6},
    },
    EvtBitten:               {{Delta: -5}},
    EvtKilledAlien:          {{Delta: 15}},
    EvtWitnessedAlienKilled: {{Delta: 6}},
    EvtFinishedMining:       jobFinishedMood, // shared: {{Delta: 2}, {Conditional: true, Trait: TraitIndustrious, Delta: 2}}
    EvtClearedRock:          jobFinishedMood,
    EvtFinishedConstruction: jobFinishedMood,
}
```

Each `LifeEventKind` maps to a slice of `MoodEffect`. An effect with
`Conditional: false` applies to every colonist; one with `Conditional: true`
applies only if `Profile.HasTrait(Trait)` — and effects stack, which is how
gore gets a small universal dip *plus* an extra dip that only lands on a Tidy
colonist, and finishing a job gets a small universal lift that doubles for an
Industrious one. `EvtFinishedMining`/`EvtClearedRock`/`EvtFinishedConstruction`
all point at the same `jobFinishedMood` slice rather than repeating it three
times — safe because nothing ever mutates it, only reads it.

Most `LifeEventKind`s still have no table entry at all (the zero value, a nil
slice): they're memory-worthy but mood-neutral, which is deliberate — see Why
it is this way.

Not every mood effect fits "a fixed number, maybe trait-gated," though — a
conversation's impact depends on the pair's affinity and a quality roll made
fresh each time, so it can't be a table lookup at all. That's what
`LifeEvent.Mood` is for: `eventMood(kind, mood, format, args...)` is `event`
plus a caller-computed delta that `applyMoodEffects` adds on top of whatever
the table says for that kind (nothing, for `EvtConversation` — the entire
effect is the computed one):

```go
w.remember(a, eventMood(EvtConversation, mood+w.noteConversation(a), "Had a conversation with %s.", b.displayName()))
```

`applyMoodEffects` sums the table effects and `evt.Mood` into a single
number and makes one call to `adjustMood` — one clamp, one place mood
actually changes, whether the delta came from a static table or a live
computation.

### Sightings, including gore, stay edge-triggered

Creature sightings were already edge-triggered on `e.seen` (a colonist
fleeing an alien for fifty ticks gets one memory, not fifty) — see
`observeNearby`. Gore sightings work the same way but as a single flag,
`e.seeingGore`, rather than a per-tile map: "in sight of gore" is one
memory-worthy (and mood-worthy) fact, whether it's one stained tile or a
whole battlefield, so `observeGore` doesn't fire once per tile.
`GoreSightRadius` (`Config`) is deliberately smaller than the creature-
sighting radii — a bloodstain doesn't call attention to itself the way a
moving alien does.

### The full `LifeEventKind` roster

| Kind | Fires when | Mood effect |
| --- | --- | --- |
| `EvtSawAlien` | first sighting an alien within `FleeRadius` | universal -6 |
| `EvtSawMouse` | first sighting a mouse within `ColonistStompRadius` | universal -2 |
| `EvtSawGore` | first sighting a gored tile within `GoreSightRadius` | universal -4, +Tidy -6 |
| `EvtBitten` | bitten by an alien, survived | universal -5 |
| `EvtWitnessedColonistKilled` | watched an alien's bite kill another colonist | none yet |
| `EvtWitnessedColonistAttacked` | watched an alien bite another colonist, non-fatally | none yet |
| `EvtCrushedMouse` | stomped a mouse | none yet |
| `EvtWitnessedMouseCrushed` | watched another colonist stomp a mouse | none yet |
| `EvtWitnessedCatCatch` | watched a cat catch a mouse | none yet |
| `EvtKilledAlien` | killed an alien with a weapon | universal +15 |
| `EvtWitnessedAlienKilled` | watched another colonist kill an alien | universal +6 |
| `EvtWoundedAlien` | shot an alien, non-fatally | none yet |
| `EvtWitnessedGunfight` | watched another colonist shoot at an alien, non-fatally | none yet |
| `EvtConversation` | finished a conversation | computed per-occurrence (`LifeEvent.Mood`, from affinity + quality + social fatigue) — see below |
| `EvtAte` / `EvtUsedToilet` / `EvtSlept` / `EvtNeedSatisfied` | finished using a facility | none yet |
| `EvtFinishedMining` / `EvtClearedRock` / `EvtFinishedConstruction` / `EvtCleanedRefuse` | finished a dig, build, or cleaning job | universal +2, +Industrious +2 |
| `EvtIncineratedRefuse` | burned a load of refuse in the incinerator | universal +2, +Industrious +2, +Tidy +6 |

"None yet" is a table entry away from having one — see Extending it.

### Conversations: the one event with a computed delta

`finishTalk` still rolls a conversation's quality and computes its mood
impact from affinity, quality, and each participant's social-fatigue state
(`noteConversation` — which must run exactly once per participant, since it
also advances their rolling conversation-count window as a side effect) —
none of that changed. What changed is where the result goes: instead of
calling `adjustMood` itself, `finishTalk` hands the computed total to
`eventMood` and calls `remember`, the same as every other event:

```go
func (w *World) finishTalk(a, b *Entity) {
    existing := w.affinityBetween(a.ID, b.ID)
    quality := w.rollTalkQuality(existing)
    w.addAffinity(a.ID, b.ID, w.talkAffinityDelta(existing, quality))
    mood := w.talkMoodDelta(quality, existing)
    w.remember(a, eventMood(EvtConversation, mood+w.noteConversation(a), "Had a conversation with %s.", b.displayName()))
    w.remember(b, eventMood(EvtConversation, mood+w.noteConversation(b), "Had a conversation with %s.", a.displayName()))
}
```

This used to be two separate mechanisms: `finishTalk` moved mood directly,
and a separate pair of `remember` calls (back in `jobTalk`) recorded the
memory with no mood effect at all. Now there is exactly one: `remember`.

## Why it is this way

- **One funnel (`remember`)** for memory *and* mood means there is no way to
  add a mood-worthy event and forget to wire the mood, or vice versa — the
  same discipline `World.remove` already applies to deaths.
- **A trait check at event time (`HasTrait`), not a resolved-at-spawn
  field**: `docs/personality.md`'s "pay once, not per tick" principle is
  about the hot path — every tick, every colonist. A `LifeEvent` fires far
  less often than that (an alien sighting, a kill, a gore encounter), so
  reading `Profile.HasTrait` at the moment it matters costs nothing
  meaningful, and it avoids pre-computing a per-Entity field for every
  trait/event combination that might someday have an effect — most of which,
  per the table above, don't yet.
- **Most kinds are still mood-neutral on purpose.** Each round of this system
  named a handful of specific effects (sightings, a kill, witnessing a kill,
  gore, being bitten, finishing a job, conversations) rather than assigning
  plausible-sounding numbers to every one of two dozen events at once.
  Witnessing a bite or a stomp, for instance, has no effect yet even though
  the direct experience does — the table (data, not a code change per event)
  is what makes adding one later cheap when it's actually wanted.
- **`Mood int` on `LifeEvent`, not a second parameter to `remember`**: keeping
  the computed delta *on* the event (rather than, say,
  `remember(e, evt, extraMood)`) means `remember`'s signature never needed to
  change to support it, and a future caller with the same need (a per-
  occurrence delta) reaches for the same `eventMood` constructor rather than
  inventing another way to pass one in.
- **`finishTalk` still owns the conversation-specific math** (affinity,
  quality, social fatigue) — only where the *result* goes changed. Folding
  `rollTalkQuality`/`talkMoodDelta`/`noteConversation` themselves into the
  generic `LifeEvent` machinery would have meant a mood-effect vocabulary
  general enough to express "scales with a live-rolled quality and the
  pair's affinity," which is a lot more machinery for a computation that
  happens in exactly one place.
- **`Memory.Kind` is carried even though nothing reads it yet** (only `Text`
  is displayed). It costs one field and means a future "what kind of thing
  keeps happening to this colonist" view, or a different mood formula per
  kind, doesn't require re-deriving the kind from memory text.

## Extending it

- **Give an existing kind a fixed (optionally trait-gated) mood effect, or
  add a new mood-bearing kind**: a table edit in `lifeEventMoodEffects`
  (`lifeevents.go`). No call site changes.
- **An event whose delta can't be a fixed number** (it depends on something
  rolled or accumulated at the moment it happens, the way a conversation's
  does): build it with `eventMood(kind, mood, format, args...)` instead of
  `event`, computing `mood` at the call site. It adds to, rather than
  replaces, whatever the table declares for that kind.
- **A new perception** (something a colonist should notice near it, like
  gore): add it to `observeNearby` (per-entity) or write an `observeGore`-like
  sibling (for anything not entity-shaped), and keep it edge-triggered —
  reuse the `e.seen`-style pattern for something identifiable per-instance,
  or the `e.seeingGore`-style single flag for an environmental condition
  where "how many" shouldn't matter.
- **A new witnessable action**: call `World.colonistsWithin` at the same
  radius `observeNearby` uses for that creature kind, so witnessing lines up
  with noticing (see [combat.md](./combat.md) for how `bite`/`stomp`/`pounce`/
  `shoot` already do this).
- **A trait that blunts or amplifies many events at once** (e.g. a
  "Hardened" trait that halves every combat-related mood penalty): today
  that means adding a `Conditional` entry to each affected kind's slice
  individually — sharing one slice across several kinds (as
  `jobFinishedMood` does for the three job-completion kinds) works as long
  as they should all move together. If a trait needed to scale a whole
  category by a *factor* rather than an additive bonus, a per-kind
  "category" (combat/social/environmental) would be worth adding — not
  needed yet for the two traits here, each with a fixed additive bonus.

## Related

- [combat.md](./combat.md) — the bite/stomp/pounce/shoot events, gore, and
  `World.remove`'s parallel one-funnel pattern for deaths.
- [personality.md](./personality.md) — traits, `TraitTidy`/`TraitIndustrious`,
  and why trait *effects* are usually resolved at spawn while these mood
  effects are checked live.
- [entities-and-ai.md](./entities-and-ai.md) — `observeNearby`'s place in
  `colonistTurn`.
- [architecture.md](./architecture.md) — how memories reach a `Snapshot`.

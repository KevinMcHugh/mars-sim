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
  `adjustMood`, the mood field's clamp primitive; conversations are still the
  other mood source, via `finishTalk`.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — `TraitTidy`,
  the one trait a mood effect is conditioned on today (see
  [personality.md](./personality.md)).
- [`internal/sim/lifeevents_test.go`](../internal/sim/lifeevents_test.go),
  [`internal/sim/memories_test.go`](../internal/sim/memories_test.go) — the
  tests that pin this behavior.

## How it works

### LifeEvent: text and mood effect, built together

```go
type LifeEvent struct {
    Kind LifeEventKind
    Text string
}
```

Call sites never construct one by hand; they call `event(kind, format, args...)`,
which is `fmt.Sprintf` plus the `Kind` tag:

```go
w.remember(prey, event(EvtBitten, "Bitten in the %s by an alien!", part))
```

`remember` (in `world.go`) is the single funnel: it appends the `Memory`
(bounded at 64 per colonist, oldest evicted first) *and* calls
`applyMoodEffects(e, evt.Kind)` — recording something and it moving mood can
never drift apart, because they happen in the same call, the same way
`World.remove` is the one funnel for a death and its graveyard record (see
[combat.md](./combat.md)).

### Mood effects: universal, or gated on a trait

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
    EvtKilledAlien:          {{Delta: 15}},
    EvtWitnessedAlienKilled: {{Delta: 6}},
}
```

Each `LifeEventKind` maps to a slice of `MoodEffect`. An effect with
`Conditional: false` applies to every colonist; one with `Conditional: true`
applies only if `Profile.HasTrait(Trait)` — and effects stack, which is how
gore gets a small universal dip *plus* an extra dip that only lands on a Tidy
colonist. Most `LifeEventKind`s have no entry at all (the zero value, a nil
slice): they're memory-worthy but mood-neutral, which is deliberate — see Why
it is this way. `applyMoodEffects` walks the slice and calls the existing
`adjustMood(e, delta)` (in `relationships.go`) for every effect that applies,
which is what actually clamps mood to `[-MoodMax, MoodMax]`.

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
| `EvtBitten` | bitten by an alien, survived | none yet |
| `EvtWitnessedColonistKilled` | watched an alien's bite kill another colonist | none yet |
| `EvtWitnessedColonistAttacked` | watched an alien bite another colonist, non-fatally | none yet |
| `EvtCrushedMouse` | stomped a mouse | none yet |
| `EvtWitnessedMouseCrushed` | watched another colonist stomp a mouse | none yet |
| `EvtWitnessedCatCatch` | watched a cat catch a mouse | none yet |
| `EvtKilledAlien` | killed an alien with a weapon | universal +15 |
| `EvtWitnessedAlienKilled` | watched another colonist kill an alien | universal +6 |
| `EvtWoundedAlien` | shot an alien, non-fatally | none yet |
| `EvtWitnessedGunfight` | watched another colonist shoot at an alien, non-fatally | none yet |
| `EvtConversation` | finished a conversation | none yet (`finishTalk` moves mood separately — see below) |
| `EvtAte` / `EvtUsedToilet` / `EvtSlept` / `EvtNeedSatisfied` | finished using a facility | none yet |
| `EvtFinishedMining` / `EvtClearedRock` / `EvtFinishedConstruction` | finished a dig or build job | none yet |

"None yet" is a table entry away from having one — see Extending it.

### Mood has two independent sources today

Conversations still move mood the way they did before this system existed:
`finishTalk` (`relationships.go`) computes a delta from affinity and
conversation quality and calls `adjustMood` directly, bypassing
`lifeEventMoodEffects`. `EvtConversation` only records the memory. The two
sources share the same clamp
(`adjustMood`), so they compose safely — a bad chat right after a bitten
encounter has both penalties land — but a conversation's mood math has its
own inputs (affinity, quality, introvert fatigue) that don't fit the
"universal delta, maybe trait-gated" shape `MoodEffect` covers. Folding it in
would have meant either weakening `MoodEffect` to carry a function instead of
a plain delta, or leaving conversations as the one exception either way; it
stays its own thing rather than a false unification.

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
- **Most kinds are mood-neutral on purpose.** The ask that built this system
  named five specific effects (two sightings, a kill, witnessing a kill, and
  gore). Everything else — a finished meal, a mined tile, a non-fatal bite —
  is memory-worthy today but was left mood-neutral rather than guessing at
  plausible-sounding numbers for two dozen events at once. The table (data,
  not a code change per event) is what makes tuning or adding one later
  cheap.
- **`Memory.Kind` is carried even though nothing reads it yet** (only `Text`
  is displayed). It costs one field and means a future "what kind of thing
  keeps happening to this colonist" view, or a different mood formula per
  kind, doesn't require re-deriving the kind from memory text.

## Extending it

- **Give an existing kind a mood effect, or add a new mood-bearing kind**: a
  table edit in `lifeEventMoodEffects` (`lifeevents.go`). No call site
  changes.
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
  individually. If that pattern shows up more than once, a per-kind
  "category" (combat/social/environmental) that a trait could scale in one
  place would be worth adding — not needed yet for a single trait
  (`TraitTidy`) affecting a single kind.

## Related

- [combat.md](./combat.md) — the bite/stomp/pounce/shoot events, gore, and
  `World.remove`'s parallel one-funnel pattern for deaths.
- [personality.md](./personality.md) — traits, `TraitTidy`, and why trait
  *effects* are usually resolved at spawn while this one is checked live.
- [entities-and-ai.md](./entities-and-ai.md) — `observeNearby`'s place in
  `colonistTurn`.
- [architecture.md](./architecture.md) — how memories reach a `Snapshot`.

# Memories & life events

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists retain a short history of notable experiences — eating, mining,
conversing, sighting an alien, watching a fight — recorded as `Memory`
entries. Every one of these is built from a `LifeEvent`: a `LifeEventKind`
(what kind of thing happened) paired with the player-facing text describing
this particular occurrence. A `LifeEventKind` also carries a table of mood
effects and a transient-stimulus table, so remembering something, changing a
colonist's mood, and influencing immediate focus are one ingestion act rather
than systems call sites must keep in sync by hand.

Minor, repetitive events — a mining shift, a string of meals — collapse into a
single `Memory` covering the whole run, so a colonist who mines for a thousand
ticks has a history, not a mining log.

## Source

- [`internal/sim/lifeevents.go`](../internal/sim/lifeevents.go) — `LifeEventKind`,
  `LifeEvent`, `event`, `MoodEffect`, `lifeEventMoodEffects`, `applyMoodEffects`,
  `lifeEventCollapseText`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Memory` (carries
  `Kind`, and `LastTick`/`Count` for a collapsed run), the colonist's fixed
  stimulus storage, and `Memories`/`seen`/`seeingGore` fields.
- [`internal/sim/stimulus.go`](../internal/sim/stimulus.go) — `Stimulus`, the
  per-event specs, bounded coalescing/eviction, expiry, and focus bias.
- [`internal/sim/world.go`](../internal/sim/world.go) — `remember`, the one
  funnel every memory, mood effect, and new stimulus goes through, and
  `collapseRepeat`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `observeNearby`
  (creature sightings) and `observeGore` (gore sightings); every other
  `remember` call site records an experience at its completion.
- [`internal/sim/relationships.go`](../internal/sim/relationships.go) —
  `adjustMood`, the mood field's clamp primitive, called only from
  `applyMoodEffects` now.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — `TraitTidy`
  and `TraitIndustrious`, the traits a mood effect is conditioned on today
  (see [personality.md](./personality.md)).
- [`internal/ui/tui/render_roster.go`](../internal/ui/tui/render_roster.go) —
  `memoryLine`, which renders a collapsed run as a span plus a count.
- [`internal/sim/lifeevents_test.go`](../internal/sim/lifeevents_test.go),
  [`internal/sim/memories_test.go`](../internal/sim/memories_test.go) — the
  tests that pin this behavior.

## How it works

### LifeEvent: text and mood effect, built together

```go
type LifeEvent struct {
    Kind   LifeEventKind
    Source EntityID // zero when the occurrence is not tied to an entity
    Text   string
    Mood   int // extra delta on top of Kind's table effects; 0 for a table-only event
}
```

Most call sites construct one with `event(kind, format, args...)`, which is
`fmt.Sprintf` plus the `Kind` tag and leaves `Source` and `Mood` at 0. Events
whose current context is a particular entity use `eventFrom` instead:

```go
w.remember(prey, eventFrom(EvtBitten, alien.ID,
    "Bitten in the %s by an alien!", part))
```

`remember` (in `world.go`) is the single funnel: it applies mood, inserts or
coalesces a configured active stimulus, and records the `Memory` (bounded at 64
per colonist, oldest evicted first — or folds it into the previous one, see
Collapsing runs of a minor event). A zero stimulus-table entry still records
mood and memory normally. There is exactly one path for a new occurrence;
ongoing alien perception may refresh the expiry of an existing context without
recording another memory.

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

### Active stimuli are bounded current context

Stimuli retain actionable aftereffects without pretending they are either mood
or history. Each colonist reserves fixed storage for at most
`ActiveStimulusLimit` entries (default 8, hard cap `MaxActiveStimuli`), so event
ingestion does not allocate a stimulus slice. The same `(Kind, Source)` refreshes
in place. At capacity, the weakest entry is selected by salience, then earliest
expiry, then lowest source ID; an incoming event weaker than every retained
entry is discarded rather than hiding stronger context.

Expiry is exact: an entry is inactive when `tick >= ExpiresAt`. Focus scoring
sums each active entry's per-focus contribution scaled by salience. Current
flee/fight eligibility still comes from a live visible alien, not from lingering
stimulus state, so removing an alien ends direct threat behavior immediately
without erasing mood, stimulus aftereffects, or memory.

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

### Collapsing runs of a minor event

A colonist assigned to the mining frontier finishes a dig every seven or eight
ticks, forever. Recorded one memory per dig, that colonist's whole remembered
history is a mining log: the 64-memory buffer holds about eight minutes of
digging, and the conversation, the alien sighting, and the mutation that
actually characterize them have all been evicted by tile coordinates.

So `remember` folds consecutive occurrences of a *minor* event into one
`Memory`:

```go
type Memory struct {
    Tick     int // when it happened; the first occurrence of a collapsed run
    LastTick int // the most recent occurrence; == Tick unless collapsed
    Count    int // occurrences folded into this memory; 1 when uncollapsed
    Text     string
    Kind     LifeEventKind
}
```

Which kinds are minor is a table, in the same spirit as the mood table —
`lifeEventCollapseText`, where a non-empty entry both *marks* the kind
collapsible and supplies the text a run reads as:

```go
var lifeEventCollapseText = [numLifeEventKinds]string{
    EvtFinishedMining:       "Finished mining.",
    EvtClearedRock:          "Cleared rock for a room.",
    EvtFinishedConstruction: "Finished construction.",
    EvtCleanedRefuse:        "Cleaned up refuse.",
    EvtIncineratedRefuse:    "Burned refuse in the incinerator.",
    EvtAte:                  "Had a meal.",
    EvtUsedToilet:           "Used the toilet.",
    EvtSlept:                "Slept in a bed.",
    EvtNeedSatisfied:        "Satisfied a need.",
}
```

Routine work and bodily upkeep collapse; everything with any narrative weight
— kills, bites, sightings, mutations, conversations — does not. `remember`
tries `collapseRepeat` first and only appends (and only then evicts) if it
comes back false:

```go
func (w *World) collapseRepeat(e *Entity, evt LifeEvent) bool {
    if lifeEventCollapseText[evt.Kind] == "" || len(e.Memories) == 0 {
        return false
    }
    last := &e.Memories[len(e.Memories)-1]
    if last.Kind != evt.Kind {
        return false
    }
    last.Text = lifeEventCollapseText[evt.Kind]
    last.LastTick = w.tick
    last.Count++
    return true
}
```

Three decisions are worth spelling out:

- **Only the newest memory is a candidate.** A run is *consecutive*
  occurrences, so a meal in the middle of a mining shift ends the run and the
  next dig starts a fresh memory. Folding into any older matching memory would
  give one line per kind for a colonist's entire life and destroy the order
  things happened in — which is most of what the log is for. The screenshot
  case (mine ×6, eat, mine ×4) becomes exactly three lines.
- **A run loses its per-occurrence text.** A single dig keeps
  `"Finished mining at (514, 501)."`; the second one rewrites the memory to
  `"Finished mining."`, because once the line stands for twelve digs, which
  tile each was on is not what it is about. That is also why the collapse
  table holds the text rather than a bare `bool` — "is this kind minor" and
  "what does a run of it read as" are the same question, answered in one
  place.
- **Mood still applies per occurrence.** `applyMoodEffects` runs on every
  call, collapsed or not: the twelfth completed job lifts an Industrious
  colonist exactly as much as the first. Collapsing is about what the log
  reads like, not about the simulation deciding repetition stops counting.

`Tick`/`LastTick`/`Count` are what a frontend needs to render a run without
pretending it was one event — the TUI writes
`t1607-1630: Finished mining. (x12)` (`memoryLine`), and an uncollapsed memory
(`Count` 1, `LastTick == Tick`) goes through the same code as a plain
`t1586: Had a meal.` See [frontend-tui.md](./frontend-tui.md).

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
| `EvtMutated` | uranium exposure grew a new body part | universal -14, +Mutant-Lover +28 (net +14) |
| `EvtWitnessedMutation` | watched another colonist mutate | universal -6, +Mutant-Lover +12 (net +6) |

"None yet" is a table entry away from having one — see Extending it. The
routine kinds in the bottom half of the table (`EvtAte`, `EvtUsedToilet`,
`EvtSlept`, `EvtNeedSatisfied`, and the four job-completion kinds plus
`EvtIncineratedRefuse`) are also the collapsible ones; everything above them
records one memory per occurrence.

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
    existing := w.mutualAffinity(a.ID, b.ID)
    quality := w.rollTalkQuality(existing)
    step := w.talkAffinityDelta(existing, quality)
    w.bumpAffinity(a.ID, b.ID, step+w.mutantAffinityBonus(a, b))
    w.bumpAffinity(b.ID, a.ID, step+w.mutantAffinityBonus(b, a))
    mood := w.talkMoodDelta(quality, existing)
    w.remember(a, eventMood(EvtConversation, mood+w.noteConversation(a), "Had a conversation with %s.", b.displayName()))
    w.remember(b, eventMood(EvtConversation, mood+w.noteConversation(b), "Had a conversation with %s.", a.displayName()))
}
```

This used to be two separate mechanisms: `finishTalk` moved mood directly,
and a separate pair of `remember` calls (back in `jobTalk`) recorded the
memory with no mood effect at all. Now there is exactly one: `remember`.

The affinity credit is per direction rather than one symmetric `addAffinity`
because a trait-driven bonus can be one-sided — a Mutant-Lover's warmth toward
a mutant is not returned in kind (see [mutation.md](./mutation.md)). The
conversation's own step is still identical for both sides, so a pair with no
such trait between them stays exactly as symmetric as before.

`EvtMutated` is also the clearest demonstration of what conditional
`MoodEffect`s are for: the same event is body horror to most colonists (-14)
and the best day of a Mutant-Lover's life (+14 net), declared as two rows of
data rather than a branch in `mutate()`.

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
- **Collapsing lives in `remember`, not in the frontend.** A renderer that
  folded repeated lines at draw time would be reading a history that had
  already thrown away everything a mining shift pushed out — the memory
  buffer is 64 entries, and the point of collapsing is that a run costs one
  of them instead of twelve. Doing it at write time also means every frontend
  and any future save format gets it for free.
- **Which kinds collapse is data, like their mood effects.** The alternative
  — a `Minor` bit on `LifeEvent`, set at each call site — would put the same
  fact in 25 places and let two mining call sites disagree about it. It also
  keeps the two per-kind tables side by side in `lifeevents.go`, so adding a
  kind means looking at both.
- **No time window on a run.** Two digs a thousand ticks apart with nothing
  between them still collapse. A gap threshold was considered and dropped: it
  is another tunable to justify, and "nothing else happened to this colonist
  in between" is already the honest summary of that stretch — if something
  had, it would have broken the run.
- **`Memory.Kind` is what collapsing matches on.** It was carried
  speculatively before anything read it ("a future view, or a different mood
  formula per kind, shouldn't have to re-derive the kind from memory text");
  run-collapsing is the first thing to actually use it, and it is the reason
  a run of digs at different coordinates can be recognized as the same thing
  happening again.

## Extending it

- **Give an existing kind a fixed (optionally trait-gated) mood effect, or
  add a new mood-bearing kind**: a table edit in `lifeEventMoodEffects`
  (`lifeevents.go`). No call site changes.
- **An event whose delta can't be a fixed number** (it depends on something
  rolled or accumulated at the moment it happens, the way a conversation's
  does): build it with `eventMood(kind, mood, format, args...)` instead of
  `event`, computing `mood` at the call site. It adds to, rather than
  replaces, whatever the table declares for that kind.
- **Make an existing kind collapsible (or stop it collapsing)**: add or remove
  its entry in `lifeEventCollapseText` (`lifeevents.go`). The entry's text is
  what a run of it reads as; no call site changes. `EvtCrushedMouse` is the
  most likely next candidate if stomping ever becomes routine.
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
- [frontend-tui.md](./frontend-tui.md) — the roster inspector, which now shows
  a colonist's whole remembered history (up to `maxColonistMemories`) in a
  scrolling panel rather than the last five.

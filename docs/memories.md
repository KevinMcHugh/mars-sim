# Memories & life events

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists retain a short history of notable experiences — eating, mining,
conversing, sighting an alien, watching a fight — recorded as `Memory`
entries. Every one of these is built from a `LifeEvent`: a `LifeEventKind`
(what kind of thing happened) paired with the player-facing text describing
this particular occurrence. A `LifeEventKind` also indexes an affect-vector
table and a transient-stimulus table, so remembering something, changing a
colonist's affect, and influencing immediate focus are one ingestion act rather
than systems call sites must keep in sync by hand.

Minor, repetitive events — a mining shift, a string of meals — collapse into a
single `Memory` covering the whole run, so a colonist who mines for a thousand
ticks has a history, not a mining log.

## Source

- [`internal/sim/lifeevents.go`](../internal/sim/lifeevents.go) — `LifeEventKind`,
  `LifeEvent`, event constructors, and `lifeEventCollapseText`.
- [`internal/sim/affect.go`](../internal/sim/affect.go) — `MoodVector`,
  `lifeEventMoodVectors`, trait transforms, and affect application.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Memory` (carries
  `Kind`, and `LastTick`/`Count` for a collapsed run), the colonist's fixed
  stimulus storage, and `Memories`/`seen`/`seeingGore` fields.
- [`internal/sim/stimulus.go`](../internal/sim/stimulus.go) — `Stimulus`, the
  per-event specs, bounded coalescing/eviction, expiry, and focus bias.
- [`internal/sim/world.go`](../internal/sim/world.go) — `remember`, the one
  funnel every memory, affect vector, and new stimulus goes through, and
  `collapseRepeat`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `observeNearby`
  (creature sightings) and `observeGore` (gore sightings); every other
  `remember` call site records an experience at its completion.
- [`internal/sim/relationships.go`](../internal/sim/relationships.go) — the
  computed conversation outcome.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — traits that
  transform event appraisal (see [personality.md](./personality.md)).
- [`internal/ui/tui/render_roster.go`](../internal/ui/tui/render_roster.go) —
  `memoryLine`, which renders a collapsed run as a span plus a count.
- [`internal/sim/lifeevents_test.go`](../internal/sim/lifeevents_test.go),
  [`internal/sim/memories_test.go`](../internal/sim/memories_test.go) — the
  tests that pin this behavior.

## How it works

### LifeEvent: text and affect input, built together

```go
type LifeEvent struct {
    Kind    LifeEventKind
    Source  EntityID // zero when the occurrence is not tied to an entity
    Text    string
    Outcome int // temporary signed input for EvtConversation only
}
```

Most call sites construct one with `event(kind, format, args...)`, which is
`fmt.Sprintf` plus the `Kind` tag and leaves `Source` and `Outcome` at 0. Events
whose current context is a particular entity use `eventFrom` instead:

```go
w.remember(prey, eventFrom(EvtBitten, alien.ID,
    "Bitten in the %s by an alien!", part))
```

`remember` (in `world.go`) is the single funnel: it applies charge/grip affect,
inserts or coalesces a configured active stimulus, and records the `Memory`
(bounded at 64 per colonist, oldest evicted first — or folds it into the previous
one, see Collapsing runs of a minor event). A zero stimulus-table entry still
records affect and memory normally. There is exactly one path for a new occurrence;
ongoing alien perception may refresh the expiry of an existing context without
recording another memory.

### Affect vectors: one complete table plus a computed conversation outcome

`lifeEventMoodVectors` has one `MoodVector{Charge, Grip}` slot per event kind.
Every non-zero row is semantic data in `affect.go`; adding an ordinary
affect-bearing event is one table edit. `applyAffect` then applies trait
transforms in declaration order and clamps both stored axes once.

Conversation quality remains computed per occurrence because affinity, the live
quality roll, and social fatigue cannot be a fixed row. `eventOutcome` carries
that temporary signed result on the `LifeEvent`; ingestion converts it to a
vector and applies the Introvert transform when relevant. The outcome is never
stored as scalar mood. See [affect.md](./affect.md) for the complete vector and
transform rules.

### Active stimuli are bounded current context

Stimuli retain actionable aftereffects without pretending they are either affect
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
without erasing affect, stimulus aftereffects, or memory.

### Sightings, including gore, stay edge-triggered

Creature sightings were already edge-triggered on `e.seen` (a colonist
fleeing an alien for fifty ticks gets one memory, not fifty) — see
`observeNearby`. Gore sightings work the same way but as a single flag,
`e.seeingGore`, rather than a per-tile map: "in sight of gore" is one
memory-worthy (and affect-bearing) fact, whether it's one stained tile or a
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

Which kinds are minor is a table, in the same spirit as the affect table —
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
- **Affect still applies per occurrence.** `applyAffect` runs on every call,
  collapsed or not: the twelfth completed job affects an Industrious colonist
  exactly as much as the first. Collapsing changes the log, not simulation.

`Tick`/`LastTick`/`Count` are what a frontend needs to render a run without
pretending it was one event — the TUI writes
`t1607-1630: Finished mining. (x12)` (`memoryLine`), and an uncollapsed memory
(`Count` 1, `LastTick == Tick`) goes through the same code as a plain
`t1586: Had a meal.` See [frontend-tui.md](./frontend-tui.md).

### The full `LifeEventKind` roster

| Kind | Fires when | Affect vector |
| --- | --- | --- |
| `EvtSawAlien` / `EvtSawMouse` / `EvtSawGore` | first nearby sighting | `(8,-10)` / `(2,-3)` / `(-3,-7)` |
| `EvtBitten` / witnessed colonist harm | alien attack experience | `(10,-8)`; killed `(6,-16)`; attacked `(5,-9)` |
| mouse/cat events | stomp or catch | crusher `(-1,2)`; witness `(-1,-2)`; cat catch `(1,1)` |
| alien combat events | kill, wound, or witness | kill `(12,14)`; witnessed kill `(5,6)`; wound `(4,5)`; gunfight `(7,-5)` |
| `EvtConversation` | finished a conversation | computed per occurrence, then converted to a vector |
| need completions | ate, toilet, slept, generic satisfaction | `(4,2)`, `(1,2)`, `(15,2)`, `(2,2)` |
| finished work | mining, clearing, construction, cleaning, incineration | `(-1,5)`, `(-1,5)`, `(-1,6)`, `(-1,5)`, `(-1,7)` |
| mutation events | mutated or witnessed mutation | `(4,-18)` / `(2,-8)` |

The routine need and work kinds are collapsible; the narrative-weight events
above them record one memory per occurrence. Trait transforms are documented in
[affect.md](./affect.md).

### Conversations: the one event with a computed outcome

`finishTalk` still rolls a conversation's quality and computes its signed
outcome from affinity, quality, and each participant's social-fatigue state
(`noteConversation` — which must run exactly once per participant, since it
also advances their rolling conversation-count window as a side effect) —
none of that changed. It hands the temporary result to `eventOutcome` and calls
`remember`, the same as every other event:

```go
func (w *World) finishTalk(a, b *Entity) {
    existing := w.mutualAffinity(a.ID, b.ID)
    quality := w.rollTalkQuality(existing)
    step := w.talkAffinityDelta(existing, quality)
    w.bumpAffinity(a.ID, b.ID, step+w.mutantAffinityBonus(a, b))
    w.bumpAffinity(b.ID, a.ID, step+w.mutantAffinityBonus(b, a))
    outcome := w.talkMoodDelta(quality, existing)
    w.remember(a, eventOutcome(EvtConversation, outcome+w.noteConversation(a), "Had a conversation with %s.", b.displayName()))
    w.remember(b, eventOutcome(EvtConversation, outcome+w.noteConversation(b), "Had a conversation with %s.", a.displayName()))
}
```

The funnel converts that signed value deterministically and applies any
Introvert reflection. There is no parallel scalar mood update.

The affinity credit is per direction rather than one symmetric `addAffinity`
because a trait-driven bonus can be one-sided — a Mutant-Lover's warmth toward
a mutant is not returned in kind (see [mutation.md](./mutation.md)). The
conversation's own step is still identical for both sides, so a pair with no
such trait between them stays exactly as symmetric as before.

`EvtMutated` demonstrates trait appraisal: the base vector lowers grip, while a
Mutant-Lover reflects grip positive, without a branch in `mutate()`.

## Why it is this way

- **One funnel (`remember`)** for memory, affect, and stimuli means there is no
  way to add an affect-bearing event and forget one durable product — the same
  discipline `World.remove` already applies to deaths.
- **A trait check at event time (`HasTrait`), not a resolved-at-spawn
  field**: `docs/personality.md`'s "pay once, not per tick" principle is
  about the hot path — every tick, every colonist. A `LifeEvent` fires far
  less often than that (an alien sighting, a kill, a gore encounter), so
  reading `Profile.HasTrait` at the moment it matters costs nothing
  meaningful, and it avoids pre-computing a per-Entity field for every
  trait/event combination that might someday have an effect — most of which,
  per the table above, don't yet.
- **The complete vector table is semantic data.** Event meanings remain together
  and table-driven rather than being scattered as executor branches.
- **`Outcome int` on `LifeEvent`, not a second parameter to `remember`** keeps
  the exceptional computed conversation input on the event while avoiding a
  hidden scalar mood source of truth.
- **`finishTalk` still owns the conversation-specific math** (affinity,
  quality, social fatigue) — only where the *result* goes changed. Folding
  `rollTalkQuality`/`talkMoodDelta`/`noteConversation` themselves into the
  generic `LifeEvent` machinery would have meant an appraisal vocabulary
  general enough to express "scales with a live-rolled quality and the
  pair's affinity," which is a lot more machinery for a computation that
  happens in exactly one place.
- **Collapsing lives in `remember`, not in the frontend.** A renderer that
  folded repeated lines at draw time would be reading a history that had
  already thrown away everything a mining shift pushed out — the memory
  buffer is 64 entries, and the point of collapsing is that a run costs one
  of them instead of twelve. Doing it at write time also means every frontend
  and any future save format gets it for free.
- **Which kinds collapse is data, like their affect vectors.** The alternative
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
  speculatively before anything read it ("a future view, or a different affect
  formula per kind, shouldn't have to re-derive the kind from memory text");
  run-collapsing is the first thing to actually use it, and it is the reason
  a run of digs at different coordinates can be recognized as the same thing
  happening again.

## Extending it

- **Give an existing kind affect, or add a new affect-bearing kind**: edit its
  `MoodVector` row in `lifeEventMoodVectors` (`affect.go`). No call-site change.
- **A computed conversation outcome** uses `eventOutcome`; keep that temporary
  input inside the one ingestion funnel rather than updating affect directly.
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
- **A trait that transforms many events** belongs in `transformMoodVector`.
  Keep event membership explicit and preserve `Trait` declaration order so
  profile slice order cannot change appraisal.

## Related

- [combat.md](./combat.md) — the bite/stomp/pounce/shoot events, gore, and
  `World.remove`'s parallel one-funnel pattern for deaths.
- [personality.md](./personality.md) — traits, `TraitTidy`/`TraitIndustrious`,
  and why trait *effects* are usually resolved at spawn while event appraisal
  transforms are checked live.
- [entities-and-ai.md](./entities-and-ai.md) — `observeNearby`'s place in
  `colonistTurn`.
- [architecture.md](./architecture.md) — how memories reach a `Snapshot`.
- [frontend-tui.md](./frontend-tui.md) — the roster inspector, which now shows
  a colonist's whole remembered history (up to `maxColonistMemories`) in a
  scrolling panel rather than the last five.

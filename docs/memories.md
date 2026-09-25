# Memories & compositional events

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists retain a short history of notable experiences — eating, mining,
conversing, sighting an alien, watching a fight — recorded as `Memory`
entries. Every one of these is built from a `Percept`: an observer-relative
view of an `Occurrence`, matched to one reaction in
[`cognition.yaml`](../cognition.yaml). That reaction ID indexes affect
appraisal, wear, and transient stimulus, so remembering something, changing a
colonist's affect, and influencing immediate focus are one ingestion act
rather than systems call sites must keep in sync by hand.

Minor, repetitive events — a mining shift, a string of meals — collapse into a
single `Memory` covering the whole run, so a colonist who mines for a thousand
ticks has a history, not a mining log.

## Source

- [`internal/sim/perception.go`](../internal/sim/perception.go) — `Occurrence`,
  `Percept`, perception fan-out, and persistent enter/stay/exit.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go) —
  reaction identity, memory templates, collapse text, and stimulus specs.
- [`internal/sim/affect.go`](../internal/sim/affect.go) — `MoodVector`, wear,
  trait scales, and affect application.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Memory` (carries
  `Rule`, and `LastTick`/`Count` for a collapsed run), stimulus storage, and
  the `perceiving` cache.
- [`internal/sim/stimulus.go`](../internal/sim/stimulus.go) — `Stimulus`,
  bounded coalescing/eviction, expiry, and focus bias.
- [`internal/sim/world.go`](../internal/sim/world.go) — `rememberPercept`, the
  one funnel every memory, affect vector, and new stimulus goes through, and
  `collapseRepeat`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `observeNearby`
  (persistent perception) and the `emitOccurrence` call sites.
- [`internal/sim/relationships.go`](../internal/sim/relationships.go) — the
  computed conversation outcome.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — traits
  whose appraisal rules live in `cognition.yaml`.
- [`internal/ui/tui/render_roster.go`](../internal/ui/tui/render_roster.go) —
  `memoryLine`, which renders a collapsed run as a span plus a count.
- [`internal/sim/lifeevents_test.go`](../internal/sim/lifeevents_test.go),
  [`internal/sim/memories_test.go`](../internal/sim/memories_test.go) — the
  tests that pin this behavior.

## How it works

### Occurrence, percept, reaction

```go
type Occurrence struct {
    Actor, Object FactRef
    Action        ActionID
    Location      Point
    Text          string
    Appraisals    []ObserverAppraisal
}

type Percept struct {
    Observer, Channel, Role, Phase, ObjectRelation
    Occurrence Occurrence
}

type Memory struct {
    Tick, LastTick, Count int
    Text                  string
    Rule                  RuleID
}
```

Feature code emits one `Occurrence`. Perception rules decide who learns about
it and in what role. The winning reaction ID is what `rememberPercept`
records. See
[compositional-perception-and-events.md](./compositional-perception-and-events.md)
for the grammar and matching rules.

Most call sites construct the occurrence at the mechanic's point of truth and
hand it to `emitOccurrence`, which fans out direct and witness percepts:

```go
w.emitOccurrence(Occurrence{
    Actor: w.factRef(alien), Action: ActionBite, Object: w.factRef(prey),
    Location: prey.Pos,
    TargetText: fmt.Sprintf("Bitten in the %s by %s!", part, w.alienNounFor(alien)),
})
```

Fatal bite emits that occurrence *before* `remove(prey.ID)`, so witnesses can
still read affinity and compute `object_relation: friend`.

`rememberPercept` is the single funnel: it applies the reaction's affect
appraisal, inserts or coalesces a configured active stimulus, and records the
`Memory` (bounded at 64 per colonist, oldest evicted first — or folds it into
the previous one, see Collapsing runs of a minor event). A reaction with no
stimulus still records affect and memory normally. There is exactly one path
for a new occurrence; ongoing alien perception may refresh the expiry of an
existing context without recording another memory.

### Affect appraisals and contextual conversation

Each reaction carries `Impact`, `Fresh`, `Worn`, and a `wear_policy`. Wear and
the push/pull blend are documented in [affect.md](./affect.md).

Conversation is the one reaction whose target is not a table lookup.
`finishTalk` still rolls quality and computes a signed outcome from affinity,
quality, and each participant's social-fatigue state (`noteConversation` —
which must run exactly once per participant, since it also advances their
rolling conversation-count window as a side effect). It attaches that vector
as an `ObserverAppraisal` on the shared occurrence and emits once. The
conversation reaction uses wear policy `none`, so the funnel applies the
contextual target without habituation on top of social fatigue.

The affinity credit is per direction rather than one symmetric `addAffinity`
because a trait-driven bonus can be one-sided — a Mutant-Lover's warmth toward
a mutant is not returned in kind (see [mutation.md](./mutation.md)). The
conversation's own step is still identical for both sides, so a pair with no
such trait between them stays exactly as symmetric as before.

### Collapsing runs of a minor event

A reaction with `memory.collapse` set will fold into the previous memory when
the last entry has the same `Rule`. The collapsed text replaces the specific
wording; `Count` and `LastTick` advance. Wear still sees one occasion.

```go
func (w *World) collapseRepeat(e *Entity, reaction *ReactionSpec, text string) bool {
    if reaction.Memory == nil || reaction.Memory.Collapse == "" || len(e.Memories) == 0 {
        return false
    }
    last := &e.Memories[len(e.Memories)-1]
    if last.Rule != reaction.ID {
        return false
    }
    last.Text = reaction.Memory.Collapse
    last.LastTick = w.tick
    last.Count++
    return true
}
```

Collapsing changes the log, not simulation: the twelfth completed job still
appraises, and an Industrious colonist still gets the doubled vector. Wear is
the system that cares that the run was one occasion.

`Tick`/`LastTick`/`Count` are what a frontend needs to render a run without
pretending it was one event — the TUI writes
`t1607-1630: Finished mining. (x12)` (`memoryLine`), and an uncollapsed memory
(`Count` 1, `LastTick == Tick`) goes through the same code as a plain
`t1586: Had a meal.` See [frontend-tui.md](./frontend-tui.md).

### The shipped reaction roster

| Reaction | Fires when | Fresh charge/grip (valence) |
| --- | --- | --- |
| `saw-alien` / `saw-mouse` / `saw-gore` | first nearby sighting | `(8,-10,-15)` / `(2,-3,0)` / `(-3,-7,-6)` |
| `bitten` / witnessed colonist harm | alien attack experience | bitten `(55,-44,-30)`; killed `(70,40,-60)`; attacked `(45,25,-40)` |
| mouse/cat events | stomp or catch | crusher `(-1,2,0)`; witness `(-1,-2,-1)`; cat catch `(1,1,0)` |
| alien combat events | kill, wound, or witness | kill `(45,52,35)`; witnessed kill `(5,6,4)`; wound `(4,5,2)`; gunfight `(35,-25,-20)` |
| `conversation` | finished a conversation | computed per occurrence |
| need completions | ate, toilet, slept, generic satisfaction | `(4,2,1)`, `(1,2,0)`, `(15,2,2)`, `(2,2,0)` |
| finished work | mining, clearing, construction, cleaning, incineration | `(-1,5,0)` / `(-1,5,0)` / `(-1,6,3)` / `(-1,5,0)` / `(-1,7,1)` |
| mutation events | mutated or witnessed mutation | `(18,-70,-35)` / `(2,-8,-10)` |

The routine need and work reactions are collapsible; the narrative-weight
events above them record one memory per occurrence. Trait scales and worn
readings live next to these rows in `cognition.yaml`.

## Why it is this way

- **One funnel (`rememberPercept`)** for memory, affect, and stimuli means
  there is no way to add an affect-bearing event and forget one durable
  product — the same discipline `World.remove` already applies to deaths.
- **A trait check at event time (`HasTrait` + grammar rules), not a
  resolved-at-spawn field**: [personality.md](./personality.md)'s "pay once,
  not per tick" principle is about the hot path — every tick, every colonist.
  A percept fires far less often than that, so walking matching trait rules at
  the moment it matters costs nothing meaningful, and it avoids pre-computing
  a per-Entity field for every trait/reaction combination.
- **The complete vector table is semantic data.** Event meanings remain
  together in `cognition.yaml` rather than being scattered as executor
  branches or a Go enum.
- **Contextual conversation stays on the occurrence**, not as a second
  parameter to `rememberPercept`. That keeps the exceptional computed input on
  the fact while avoiding a hidden scalar mood source of truth.
- **`finishTalk` still owns the conversation-specific math** (affinity,
  quality, social fatigue) — only where the *result* goes changed. Folding
  `rollTalkQuality`/`talkMoodDelta`/`noteConversation` themselves into generic
  reaction data would have meant an appraisal vocabulary general enough to
  express "scales with a live-rolled quality and the pair's affinity."
- **Collapsing lives in `rememberPercept`, not in the frontend.** A renderer
  that folded repeated lines at draw time would be reading a history that had
  already thrown away everything a mining shift pushed out — the memory
  buffer is 64 entries, and the point of collapsing is that a run costs one
  of them instead of twelve. Doing it at write time also means every frontend
  and any future save format gets it for free.
- **Which reactions collapse is data, like their affect vectors.** The
  alternative — a `Minor` bit set at each call site — would put the same fact
  in 25 places and let two mining call sites disagree about it.
- **No time window on a run.** Two digs a thousand ticks apart with nothing
  between them still collapse. A gap threshold was considered and dropped: it
  is another tunable to justify, and "nothing else happened to this colonist
  in between" is already the honest summary of that stretch — if something
  had, it would have broken the run.
- **`Memory.Rule` is what collapsing and wear match on.** Stable reaction IDs
  replaced the closed `LifeEventKind` enum so a run of digs at different
  coordinates is still the same thing happening again, and so config can add
  a reaction without a Go constant.

## Extending it

- **Give an existing reaction affect, or add a new one**: edit
  `cognition.yaml`. No call-site change if the occurrence is already emitted.
- **A computed conversation-like outcome** attaches an `ObserverAppraisal` and
  selects wear policy `none`.
- **Make an existing reaction collapsible (or stop it collapsing)**: add or
  remove `memory.collapse`. The text is what a run of it reads as; no call
  site changes. `crushed-mouse` is the most likely next candidate if stomping
  ever becomes routine.
- **A new perception** (something a colonist should notice near it, like
  gore): add a persistent perception rule. Do not add a special-case observer
  in `colonistTurn`. Custom persistent rules automatically disable the
  resting fast path.
- **A new witnessable action**: emit one occurrence; add instant witness
  perception at the same radius the persistent rule uses for that noun.
- **A trait that transforms many percepts** is a `trait_rules` row against
  the grammar. Keep declaration order so file order, not map iteration,
  decides composition.

## Related

- [compositional-perception-and-events.md](./compositional-perception-and-events.md)
  — the grammar, perception engine, and wear-policy seam.
- [cognition-config-and-lab.md](./cognition-config-and-lab.md) — authoring
  schema and the Cognition Lab.
- [combat.md](./combat.md) — the bite/stomp/pounce/shoot events, gore, and
  `World.remove`'s parallel one-funnel pattern for deaths.
- [personality.md](./personality.md) — traits, and why trait *effects* are
  usually resolved at spawn while event appraisal is checked live.
- [entities-and-ai.md](./entities-and-ai.md) — `observeNearby`'s place in
  `colonistTurn`.
- [architecture.md](./architecture.md) — how memories reach a `Snapshot`.
- [frontend-tui.md](./frontend-tui.md) — the roster inspector, which now shows
  a colonist's whole remembered history (up to `maxColonistMemories`) in a
  scrolling panel rather than the last five.

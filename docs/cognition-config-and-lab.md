# Compositional perception, cognition config, and lab

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonist cognition consumes compositional facts instead of a closed enum such
as `SawAlien` or `WitnessedMouseCrushed`. Feature code says what happened;
`cognition.yaml` says who can perceive it and how that perception changes
affect, attention, and memory. The Cognition Lab edits and visualizes the same
typed vocabulary and rule schema.

`cognition.yaml` is the sole event and trait-appraisal data surface. There is
no `events.yaml`, `traits.yaml`, or tag vocabulary.

## Source

- [`internal/sim/perception.go`](../internal/sim/perception.go) — occurrences,
  percepts, matching, persistent edge detection, sensory range, LOS, and
  friend-relation context.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go) —
  vocabulary, perception/reaction/trait rules, wear-policy IDs, strict YAML
  loading, defaults, template generation, and the JSON editor export.
- [`internal/sim/world.go`](../internal/sim/world.go) — `rememberPercept`, the
  one ingestion funnel for affect, stimulus, and memory.
- [`cognition.yaml`](../cognition.yaml) — the committed authoring file.
- [`tools/cognition_lab.html`](../tools/cognition_lab.html) — standalone
  visualization and typed rule editor.
- [`internal/sim/cognition_config_test.go`](../internal/sim/cognition_config_test.go)
  and [`internal/sim/lifeevents_test.go`](../internal/sim/lifeevents_test.go) —
  schema, matching, and behavioral parity tests.

## How it works

### World fact versus perception

An `Occurrence` is objective:

```go
Occurrence{Actor, Action, Object, Location, Text, Appraisals}
```

A `Percept` adds the colonist-relative dimensions:

```go
Percept{Observer, Channel, Role, Phase, ObjectRelation, Occurrence}
```

`Channel` is `direct`, `sight`, `hearing`, or `proximity`. `Role` is `actor`,
`target`, or `witness`. `Phase` is `instant`, `enter`, `ongoing`, or `exit`.
`ObjectRelation` is computed per observer (`friend` when affinity is at least
`MoodFriendAffinity`). This is why playing with a cat and watching someone play
with a cat do not need two Go event kinds: both are `colonist / play / cat`;
channel and role differ.

Feature systems still own physical truth. Combat decides that a shot killed an
alien, work decides that a tile was mined, and conversation computes its
per-participant outcome. They emit one occurrence. Configuration does not
script combat, pathfinding, claims, or job eligibility.

### Perception rules

`perceptions` matches actor/action/object and defines the sensory channel,
observer role, range, optional line-of-sight test, and cadence:

```yaml
perceptions:
  - id: visible-alien
    match: { actor_noun: alien, action: present }
    sense:
      { channel: sight, role: witness, radius: flee-radius,
        cadence: enter-and-ongoing }
```

Instant actions fan out when emitted. Persistent facts are not written to a
global fact database every tick: each colonist keeps only the bounded set of
perception keys currently in range. Nearby entity IDs come from the chunk
index in sorted order. Enter creates a durable appraisal; ongoing refreshes
its stimulus; leaving emits an `exit` percept and clears the key so re-entry
can fire again.

Named radii refer to existing simulation knobs (`flee-radius`, `stomp-radius`,
and `gore-sight-radius`). A rule may instead specify a fixed positive
`distance`. Direct rules must be instant and may not set a radius or LOS.

### Reaction rules

`reactions` deterministically select one interpretation for a percept:

```yaml
reactions:
  - id: witnessed-alien-killed
    match:
      { actor_noun: colonist, action: kill, object_noun: alien,
        channel: sight, role: witness, phase: instant }
    memory: { record: true }
    affect:
      { impact: 35,
        fresh: { charge: 5, grip: 6, valence: 4 },
        worn: { charge: 2, grip: 2, valence: 0 } }
    wear_policy: memory-occasions
```

The immutable `id` is memory-collapse and stimulus-coalescing identity.
Priority wins first, then pattern specificity. Equal-priority, equally specific
overlapping patterns are rejected at load time rather than depending on YAML or
map order.

Memory text may be a configured template (`{actor}`, `{object}`, `{action}`),
or feature code may supply occurrence-specific detail such as a body part or
weapon. `collapse` supplies generic wording after consecutive repetitions.
Stimuli select `source: actor`, `object`, or `none` for deterministic
coalescing identity.

`wear_policy` is a registered ID, not an expression. `memory-occasions` is
current-main habituation. `none` is for reactions that already own repetition,
conversation being the shipped example. Unknown IDs fail config loading.

### Trait rules

`trait_rules` match the same grammar. Empty fields are wildcards; rules apply
in file order and all matching rules compose:

```yaml
trait_rules:
  - id: tidy-saw-gore
    trait: tidy
    match: { actor_noun: gore, action: present, channel: sight, role: witness }
    scales: { charge: 220, grip: 220, valence: 220 }
```

A trait's spawn-time mechanics (need rates, work scale, social capacity, affect
home) still live in `personality.go`. Appraisal is the part that became data.

### Loading and authoring

Cognition settings layer separately from `mars-sim.yaml`:

1. `DefaultCognitionConfig()` compiled into the binary.
2. `cognition.yaml` (or `-cognition PATH`).
3. mirrored focus/arbitration fields in `mars-sim.yaml`.
4. command-line flags for those mirrored fields.

The loader uses strict field decoding, rejects duplicate/unknown vocabulary and
rule IDs, unknown wear policies, unknown traits and relations, no-op trait
rules, and ambiguous base reactions, then compiles exact lookup indexes. Run:

```sh
go run . -cognition "" -print-cognition-config > cognition.yaml
go run . -print-cognition-vocab
```

The second command is the authoritative vocabulary/schema feed for editor
dropdowns: nouns, actions, channels, roles, phases, cadences, radii, traits,
object relations, wear-policy IDs, and wear scales. The standalone Lab can
import it, edit stable reaction IDs with typed selectors, preview wear and
affect-home, and import/export the runtime YAML.

Open the Lab from a local static server (a `file://` open is enough for the
defaults; vocabulary import needs the JSON from `-print-cognition-vocab`):

```sh
ruby -run -e httpd . -p 8765 -b 127.0.0.1
# then open http://127.0.0.1:8765/tools/cognition_lab.html
```

It is a top-to-bottom situation simulator: set charge/grip/valence or click
the affect plane, trigger a reaction, choose the current focus, tune needs,
and watch arbitration. Below that, edit reactions (fresh/worn targets and wear
policy) and perceptions, then trait rules. Active-trait, wear-occasion, and
affect-home controls preview how the same percept lands on different
colonists.

## Why it is this way

- **Why two rule layers?** What happened, whether John could see it, and how
  John felt about it are independent facts. One flat event name entangled all
  three and forced every feature to add another enum/table/parser branch.
- **Why not a universal fact stream?** Enumerating every relation at every tick
  would create allocation and matching work for facts nobody consumes.
  Occurrence fan-out plus enter/stay caches gives the authoring benefit without
  turning cognition into a world database.
- **Why not tags?** The grammar already names the actor, action, object,
  channel, role, phase, and observer-relative relation. A second vocabulary
  would have to stay aligned with that grammar and still could not express
  "this object is *my* friend" without a dynamic stamp.
- **Why a wear-policy registry?** Conversation already wears through social
  fatigue. Habituation through remembered occasions is a different rule. A
  future relationship-aware conversation policy should plug in here without
  changing `rememberPercept`.
- **Why stable string IDs?** Designers need durable YAML and memory identity;
  runtime map lookup is exact and maps are never iterated for simulation order.
- **Why no expression language?** Typed matching, contextual targets, and
  integer effect tables cover the current content. A DSL would add parsing,
  evaluation, and error-reporting complexity before a concrete rule needs it.

## Extending it

For a new mechanic:

1. Add its reusable noun/action IDs to the vocabulary.
2. Emit one `Occurrence` from the feature's point of truth.
3. Add perception rules for the channels that can notice it.
4. Add reaction rules for actor, target, and witness interpretations that
   should affect cognition, including `fresh`/`worn` and a wear policy.
5. Add trait rules if a trait should scale that percept.
6. Export/import through the Lab, then run `go test ./...`.

Adding a new entity kind still requires simulation code. Adding a new
actor/action/object combination and its cognitive tuning does not require a
new Go event enum. Adding a specialist wear subsystem means registering a
policy ID and selecting it on the reactions that own it.

Preserve sorted entity iteration, integer appraisal math, immutable rule IDs,
and the one-funnel invariant.

## Related

- [compositional-perception-and-events.md](./compositional-perception-and-events.md)
  — feature description, architecture, and design decisions.
- [memories.md](./memories.md) — durable history and consecutive collapse.
- [affect.md](./affect.md) — push/pull appraisal, wear, and baselines.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — how
  stimuli and affect feed focus arbitration.
- [configuration.md](./configuration.md) — ordinary simulation knobs.
- [cli.md](./cli.md) — `-cognition`, `-print-cognition-config`, and
  `-print-cognition-vocab`.

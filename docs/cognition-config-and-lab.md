# Compositional perception, cognition config, and lab

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonist cognition consumes compositional facts instead of a closed enum such
as `SawAlien` or `WitnessedMouseCrushed`. Feature code says what happened;
`cognition.yaml` says who can perceive it and how that perception changes
affect, attention, and memory. The Cognition Lab edits and visualizes the same
typed vocabulary and rule schema.

## Source

- [`internal/sim/perception.go`](../internal/sim/perception.go) — occurrences,
  percepts, matching, persistent edge detection, sensory range, and fan-out.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go) —
  vocabulary, perception/reaction/modifier rules, strict YAML loading, defaults,
  template generation, and the JSON editor export.
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
Occurrence{Actor, Action, Object, Location, Tags}
```

A `Percept` adds the colonist-relative dimensions:

```go
Percept{Observer, Channel, Role, Phase, Occurrence}
```

`Channel` is `direct`, `sight`, `hearing`, or `proximity`. `Role` is `actor`,
`target`, or `witness`. `Phase` is `instant`, `enter`, `ongoing`, or `exit`.
This is why playing with a cat and watching someone play with a cat do not need
two Go event kinds: both are `colonist / play / cat`; channel and role differ.

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
perception keys currently in range. Enter creates a durable appraisal; ongoing
refreshes its stimulus; leaving emits an `exit` percept and clears the key so
re-entry can fire again.

Named radii refer to existing simulation knobs (`flee-radius`, `stomp-radius`,
and `gore-sight-radius`). A rule may instead specify a fixed positive
`distance`.

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
      { impact: 35, target: { charge: 5, grip: 6, valence: 4 } }
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

### Tags and trait modifiers

Reactions carry tags; traits react to tags. This replaces trait × event
switches:

```yaml
modifiers:
  - id: tidy-gore
    trait: tidy
    any_tags: [gore]
    scales: { impact: 100, charge: 220, grip: 220, valence: 220 }
```

Scales are integer percentages and are applied in trait declaration order, so
seeded simulation remains deterministic. Occurrences can also add dynamic tags
for contextual facts without creating a new reaction ID.

### Loading and authoring

Precedence remains:

1. `DefaultCognitionConfig()` compiled into the binary.
2. `cognition.yaml`.
3. mirrored settings in `mars-sim.yaml`.
4. command-line flags for the mirrored focus/arbitration fields.

The loader uses strict field decoding, rejects duplicate/unknown vocabulary and
rule IDs, validates every reference, and compiles exact lookup indexes. Run:

```sh
go run . -cognition "" -print-cognition-config > cognition.yaml
go run . -print-cognition-vocab > cognition-vocab.json
```

The second command is the authoritative vocabulary/schema feed for editor
dropdowns, including extensions from the loaded cognition file. The standalone
Lab can import it, edit stable reaction IDs with typed selectors, validate
references, and import/export the runtime YAML.

## Why it is this way

- **Why two rule layers?** What happened, whether John could see it, and how
  John felt about it are independent facts. One flat event name entangled all
  three and forced every feature to add another enum/table/parser branch.
- **Why not a universal fact stream?** Enumerating every relation at every tick
  would create allocation and matching work for facts nobody consumes.
  Occurrence fan-out plus enter/stay caches gives the authoring benefit without
  turning cognition into a world database.
- **Why stable string IDs?** Designers need durable YAML and memory identity;
  runtime map lookup is exact and maps are never iterated for simulation order.
- **Why no expression language?** Typed matching, dynamic tags, and integer
  effect tables cover the current content. A DSL would add parsing, evaluation,
  and error-reporting complexity before a concrete rule needs it.

## Extending it

For a new mechanic:

1. Add its reusable noun/action/tag IDs to the vocabulary.
2. Emit one `Occurrence` from the feature's point of truth.
3. Add perception rules for the channels that can notice it.
4. Add reaction rules for actor, target, and witness interpretations that
   should affect cognition.
5. Export/import through the Lab, then run `go test ./...`.

Adding a new entity kind still requires simulation code. Adding a new
actor/action/object combination and its cognitive tuning does not require a
new Go event enum.

Preserve sorted entity iteration, integer appraisal math, immutable rule IDs,
and the one-funnel invariant.

## Related

- [compositional-perception-and-events.md](./compositional-perception-and-events.md)
  — feature description, architecture, and design decisions.
- [memories.md](./memories.md) — durable history and consecutive collapse.
- [affect.md](./affect.md) — push/pull appraisal and trait/tag modifiers.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — how
  stimuli and affect feed focus arbitration.
- [configuration.md](./configuration.md) — ordinary simulation knobs.

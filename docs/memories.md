# Memories and percept ingestion

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists retain a bounded history of notable percepts: things they did,
experienced directly, saw, heard, or were near. A single compositional
ingestion path applies affect, updates transient attention, and records or
collapses memory, so feature code cannot update one cognitive product and
forget the others.

## Source

- [`internal/sim/perception.go`](../internal/sim/perception.go) —
  `Occurrence`, `Percept`, sensory fan-out, and persistent edge caches.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go) —
  reaction IDs, matching, memory policy, appraisal, stimuli, and trait tags.
- [`internal/sim/world.go`](../internal/sim/world.go) — `rememberPercept` and
  `collapseRepeat`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Memory`, active
  perception keys, fixed stimulus storage, and the bounded memory slice.
- [`internal/sim/stimulus.go`](../internal/sim/stimulus.go) — coalescing,
  eviction, expiry, and cached focus bias.
- [`internal/ui/tui/render_roster.go`](../internal/ui/tui/render_roster.go) —
  rendering individual and collapsed memories.
- [`internal/sim/memories_test.go`](../internal/sim/memories_test.go) and
  [`internal/sim/stimulus_test.go`](../internal/sim/stimulus_test.go) — the
  durable-history and transient-attention contracts.

## How it works

### From occurrence to reaction

A feature emits one objective occurrence:

```go
Occurrence{
    Actor: actor,
    Action: ActionKill,
    Object: alien,
    Location: actor.Pos,
}
```

Configured perception rules produce observer-relative `Percept` values. A
reaction then matches actor/action/object plus channel, observer role, and
phase. Its stable `RuleID` replaces the old need for one enum constant per
combination such as “performed kill alien” and “saw colonist kill alien.”

`rememberPercept` resolves exactly one reaction, then:

1. applies its affect target and trait/tag modifiers;
2. inserts or refreshes its configured stimulus;
3. records its memory, or folds it into the previous run.

Ongoing persistent perception is the one deliberate partial path: it reuses
the enter reaction only to refresh stimulus expiry. It does not replay affect
or append another memory.

### Memory shape and identity

```go
type Memory struct {
    Tick     int
    LastTick int
    Count    int
    Text     string
    Rule     RuleID
}
```

`Rule` is an immutable config ID such as `finished-mining`. Text may come from
a reaction template (`{actor}`, `{object}`, `{action}`) or occurrence-specific
detail supplied by the feature, such as a weapon, body part, or tile.

Each colonist keeps at most 64 entries. Snapshots copy the slice before
frontends see it.

### Consecutive collapse

Routine reactions opt into collapse with configured generic text:

```yaml
- id: finished-mining
  memory:
    record: true
    collapse: "Finished mining."
```

Only the newest memory is eligible. If it has the same `RuleID`, the funnel
keeps its first tick, advances `LastTick`, increments `Count`, and replaces the
specific occurrence text with `collapse`. A meal or conversation between two
digs breaks the run, preserving story order.

Affect and stimuli still apply for every occurrence. Collapse changes storage
and display, not simulation. The TUI renders a run as:

```text
t1607-1630: Finished mining. (x12)
```

### Active stimuli

Stimuli are bounded current context, not durable history:

```go
type Stimulus struct {
    Rule      RuleID
    Source    EntityID
    Salience  int
    ExpiresAt int
}
```

The same `(Rule, Source)` refreshes in place. Each stimulus explicitly selects
`source: actor`, `object`, or `none`; this preserves alien identity while
allowing routine work to coalesce. At capacity, eviction uses salience, then
expiry, then source ID; this total order is deterministic. Focus contributions
come from the reaction's config and are cached on the colonist.
An expired threat stimulus may linger as an emotional aftereffect, but live
flee/fight eligibility still queries the world, so removing an alien ends
direct threat behavior immediately.

### Persistent sightings

Colonists keep a set of `perceptionKey{Rule, Source, Noun}` values currently in
range. `enter` creates the durable reaction, `ongoing` can refresh attention,
and absence emits `exit` before removing the key. Returning later enters again.

Environmental facts can use source zero to aggregate “any gore nearby” into one
perception rather than one memory per tile. Entity facts use entity ID, so two
aliens retain independent stimulus identity.

### Contextual appraisals

Most reactions use the configured target. Conversation is contextual: the
feature still computes quality, affinity, and each participant's social
fatigue once, then attaches a per-observer `MoodVector` to the shared
occurrence. The generic funnel consumes that target and applies the normal
conversation tags/modifiers. There is no parallel scalar mood update.

## Why it is this way

- **One funnel** prevents memory, mood, and attention from drifting apart.
- **Reaction IDs are durable data identity.** Rendered text can change and
  compositional fields can grow without breaking collapse or stimulus refresh.
- **Collapse happens on write.** Doing it in the frontend would not recover
  memories already evicted from the 64-entry buffer.
- **Only consecutive repetitions collapse.** Folding into any older matching
  entry would destroy chronology.
- **Persistent facts are edge-triggered.** A global every-tick fact stream
  would spend time and allocation on relations nobody consumes.
- **Feature code owns contextual math.** A generic expression language is not
  justified merely to move conversation quality out of its one point of truth.

## Extending it

1. Add reusable noun/action/tag IDs to `cognition.yaml`.
2. Emit one occurrence from the feature.
3. Configure direct and sensory perception rules.
4. Configure reactions for the participant/witness roles that matter.
5. Set `memory.record`, optional template, and optional collapse text.
6. Keep entity iteration sorted and all appraisal math integer.

Do not append `Memory` directly or call affect/stimulus helpers independently.

## Related

- [cognition-config-and-lab.md](./cognition-config-and-lab.md) — schema,
  matching, authoring, and the Lab.
- [affect.md](./affect.md) — reaction targets and trait/tag transforms.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — how
  stimuli feed focus arbitration.
- [frontend-tui.md](./frontend-tui.md) — memory display.

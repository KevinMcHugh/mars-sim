# Charge/grip/valence affect

> Part of the [mars-sim documentation](./README.md).

## What it is

Affect is a colonist's bounded `(charge, grip, valence)` response to their life
so far. Charge and grip bias focus arbitration. Valence records whether life
has been going well and chooses the good/bad wording of a mood attractor without
becoming behavioral state itself.

Each configured reaction has an impact and target. Impact alone decides whether
the reaction nudges current affect or relocates it.

## Source

- [`internal/sim/affect.go`](../internal/sim/affect.go) — the push/pull blend,
  trait/tag modifiers, decay, attractors, and labels.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go) —
  configured reaction targets, impact, tags, and modifier rules.
- [`internal/sim/world.go`](../internal/sim/world.go) — `rememberPercept`, the
  affect/stimulus/memory ingestion funnel.
- [`internal/sim/focus.go`](../internal/sim/focus.go) — additive charge/grip
  focus contributions.
- [`internal/sim/affect_test.go`](../internal/sim/affect_test.go) — blending,
  modifiers, decay, labels, and behavioral ordering.

## How it works

### Coordinates and reaction appraisal

Charge is energy behind the next action, grip is felt control, and valence is
whether life has been going well. All three are clamped to
`[-MoodMax, MoodMax]`.

A reaction in `cognition.yaml` supplies:

```yaml
affect:
  impact: 35
  target: { charge: 5, grip: 6, valence: 4 }
```

The target is both a displacement and a destination. Routine reactions are
nudge-sized; high-impact reactions are written at the scale of the plane.
`rememberPercept` applies appraisal for every occurrence even when memory
collapse folds the history entry.

Valence is mostly zero for routine upkeep. It decays much more slowly than
charge or grip, and paying it for every dig once pegged every colonist at the
maximum. Only events someone would count as evidence of a good or bad life
should move it.

Conversation is contextual. The social system converts live quality, affinity,
and personal fatigue into a per-observer target on the occurrence. That target
overrides the reaction's zero default and then follows the same modifier/blend
path as every other percept.

### Push, pull, and impact

`moodPull` converts impact to a percentage: zero at or below
`MoodPushImpact`, 100 at or above `MoodPullImpact`, linear between them.

```text
keep  = 100 - pull
axis' = axis*keep/100 + target
```

At pull zero, routine life adds. At pull 100, the colonist ends at the target
regardless of where they started. A good morning therefore cannot soften
watching someone die; the death relocates instead of adding.

### Configured trait/tag modifiers

Reaction tags compose personality with content:

```yaml
modifiers:
  - id: tidy-gore
    trait: tidy
    any_tags: [gore]
    scales: { impact: 100, charge: 220, grip: 220, valence: 220 }
```

Shipped rules preserve the previous behavior:

| Trait | Tagged appraisal |
| --- | --- |
| Tidy | scales `gore` by 2.2 and doubles `incineration` grip relief |
| Industrious | doubles `finished-work` vectors |
| Mutant-Lover | reflects `mutation` grip and valence |
| Introvert | reflects `conversation` charge |

Scales are integer percentages. Rules run in `Trait` declaration order,
independent of profile slice order. These rare event-time `HasTrait` checks are
appropriate; per-tick need/work effects remain resolved at spawn.

### Decay and focus

At the start of a colonist turn, `approach` moves each axis toward
`(0, 0, 0)` without overshoot. Defaults decay charge by 2 and grip by 1 per
turn. Valence gives up one point every `MoodValenceDecayTicks` world ticks.

Focus scoring normalizes charge and grip by `MoodMax`, applies each focus's
weights, and adds the result to `FocusScore.Affect`. It never reads valence or
the displayed mood label. A valence-only change therefore does not dirty
cognition.

### Display projection

Nine configured attractors claim nearby charge/grip points using integer
squared-distance math. Declaration order breaks equal claims.
`MoodLabelSwitchMargin` prevents boundary flicker; unclaimed space is
`MoodSettling`.

Valence selects the attractor's good or bad name: the same `MoodDriven` point
reads `driven` at non-negative valence and `furious` below zero. Labels are a
projection, never executor state.

## Why it is this way

- A scalar good/bad value cannot distinguish fatigue from fear.
- Stored valence remembers history that current needs/HP cannot reconstruct.
- Valence stays out of focus scoring because needs, HP, and threats already
  contribute directly; feeding the summary back would count them twice.
- Integer Cartesian decay and percentage modifiers preserve seeded
  reproducibility.
- Reaction meanings are config data so content combinations can be tuned
  without another Go enum/table branch.
- Labels remain projections so UI vocabulary cannot hide behavior thresholds.

## Extending it

Add or tune appraisal in a reaction rule. Keep low-impact targets nudge-sized
and high-impact targets plane-sized; tests should ensure relocating reactions
land in named space. Add cross-cutting personality behavior as a tagged
modifier, not a switch over every action/noun combination. Add or tune
attractors in `cognition.yaml`, remembering declaration order is the tie-break.

Never use `MoodName` or `MoodKind` in focus scoring or an executor.

## Related

- [cognition-config-and-lab.md](./cognition-config-and-lab.md) — reaction and
  modifier authoring.
- [memories.md](./memories.md) — the one ingestion funnel and collapse.
- [personality.md](./personality.md) — trait ordering.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — score
  model.
- [mood-space.md](./mood-space.md) — remaining habituation and baseline ideas.

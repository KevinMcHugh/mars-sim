# Personality & traits

> Part of the [mars-sim documentation](./README.md).

## What it is

Every colonist has a `Profile`: a name, attributes (sex, gender, orientation,
height, weight), and any traits. Attributes are flavor for now; **traits change
how a colonist plays** by scaling need rates and work behavior. All of it is
generated from a dedicated RNG stream so flavor never perturbs the simulation.

## Source

- [`internal/sim/personality.go`](../internal/sim/personality.go) — `Profile`, `Trait`, `traitSpecs`, generation, trait resolution.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — the trait-resolved effective params (`needRise`, `restTicks`, `workScale`).
- [`internal/sim/config.go`](../internal/sim/config.go) — `TraitChance`.

## How it works

### The separate RNG stream

Personality is generated from `World.prng`, seeded from the run seed XORed with a
constant — **separate from `World.rng`, the simulation stream**. This is the whole
trick: adding or changing flavor generation never shifts the sim's own random
draws, so with traits disabled (`TraitChance = 0`) a run plays bit-for-bit as it
did before personalities existed. Preserving this separation is a hard invariant
(see [`AGENTS.md`](../AGENTS.md)).

### Attributes (flavor)

`assignPersonality` rolls sex, gender (usually but not always aligned with sex),
orientation, a correlated height/weight (via a BMI draw), and a name drawn from
gender-appropriate pools. Nothing simulates against these yet — they exist for
flavor and future systems.

### Traits (mechanical)

Traits are drawn from **mutually exclusive groups**; a colonist gets at most one
per group, each taken with `TraitChance` probability:

| Group | Traits | Effect |
| --- | --- | --- |
| appetite | Big Eater / Light Eater | food need rises 1.5x / 0.7x |
| work ethic | Industrious / Lazy | work 0.75x time + rest 0.5x / work 1.4x + rest 2.0x |
| social | Asocial / Introvert / Extrovert | no social need / social need 0.5x plus conversation fatigue / social need 1.5x |

Each trait is a `traitSpec` with multiplier effects (`needRiseScale`, `restScale`,
`workScale`; 1.0 or unset means no change). Social traits additionally resolve
social capacity and conversation-fatigue effects onto the entity. An Asocial
colonist's social need rises at zero, so it never becomes an urgent reason to
seek a conversation. An Introvert's need rises more slowly, but conversations
past its per-window capacity reduce mood. An Extrovert's need rises faster, so it
seeks social contact more often.

### Trait resolution: pay once, not per tick

The critical design point: **traits are resolved into per-colonist effective
parameters at spawn**, not scanned on the hot path. `resolveTraitEffects` folds a
colonist's traits into three fields on the `Entity`:

- `needRise[i]` — per-need rise per tick (used directly by `needLevel`),
- `restTicks` — idle rest duration,
- `workScale` — a mine/build time multiplier (via `scaleTicks`).
- social need rise and conversation-fatigue capacity/penalty, used by the
  social-need and completed-conversation paths.

So the per-tick systems just read these numbers; a colonist's traits are never
re-scanned during simulation. `newEntity` sets the config baselines, and
`assignPersonality` scales them by whatever traits it rolled.

## Why it is this way

- **Separate RNG** makes flavor free of gameplay consequences and keeps runs
  reproducible and comparable — you can add names and body types without changing
  a single AI decision.
- **Resolve-at-spawn** keeps traits off the hot path entirely: no per-tick trait
  loop, no branching on trait membership in `needLevel` or the job executors.
- **Mutually exclusive groups** model "you can't be both a big and a light eater"
  cleanly and make adding an axis a matter of adding a group.

## Extending it

- **A new trait**: add a `Trait` constant before `numTraits` and a `traitSpec`
  entry with its group and effect multipliers. If it is a new axis, add a
  `traitGroup`. The systems read effects generically via `resolveTraitEffects`,
  so nothing else changes. New needs and systems are expected to bring traits
  that suit them.
- **Making an attribute mechanical**: give it an effect and fold it into
  `resolveTraitEffects` (or an equivalent resolve step) so it stays off the hot
  path.

## Related

- [needs.md](./needs.md) — the need-rise rates traits scale.
- [entities-and-ai.md](./entities-and-ai.md) — how `workScale`/`restTicks` feed behavior.
- [configuration.md](./configuration.md) — `TraitChance`.

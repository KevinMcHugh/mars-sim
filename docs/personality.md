# Personality & traits

> Part of the [mars-sim documentation](./README.md).

## What it is

Every colonist has a `Profile`: a name, attributes (age, gender, orientation,
height, weight, skin tone, hair color), and any traits. Attributes are flavor for now; **traits change
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

`assignPersonality` rolls age (18–80), gender, orientation, a correlated
height/weight (via a BMI draw), a skin tone (uniform
across the five emoji tone points), a hair color (white and bald weighted
upward with age), and a name drawn from gender-appropriate pools. Nothing
simulates against these yet — they exist for flavor and future systems.

Skin tone and hair color are text-only (shown in the roster detail pane) —
not composed into a colonist's map glyph. Both a skin tone modifier and a
ZWJ-joined hair component were tried in [`internal/ui/tui/glyphs.go`](../internal/ui/tui/glyphs.go),
but plenty of terminals don't fuse a modifier or a ZWJ sequence onto the
preceding glyph — they print it as its own separate character (a skin tone
modifier alone renders as a plain colored square), which throws off the
column count the renderer assumes and corrupts the roster layout. The map
glyph sticks to age and gender only, which composes cleanly everywhere.

### Traits (mechanical)

Traits are drawn from **mutually exclusive groups**; a colonist gets at most one
per group, each taken with `TraitChance` probability:

| Group | Traits | Effect |
| --- | --- | --- |
| appetite | Big Eater / Light Eater | food need rises 1.5x / 0.7x |
| work ethic | Industrious / Lazy | work 0.75x time + rest 0.5x, plus a doubled mood lift on finishing a job / work 1.4x + rest 2.0x |
| social | Asocial / Introvert / Extrovert | no social need / social need 0.5x plus conversation fatigue / social need 1.5x |
| temperament | Tidy | an extra mood penalty on seeing gore (see below) |
| mutant attitude | Mutant-Lover | extra affinity toward mutants per conversation, and the opposite mood reaction to a mutation |
| mutation | Mutant | *acquired in play only* — the marker for a colonist uranium has changed |

`temperament` is a group of one today — unlike the others, Tidy isn't paired
with a mutually-exclusive opposite yet (a "Slob", numbed to gore, would be
the natural one to add). It still needed its own group rather than joining
an existing one: it isn't mutually exclusive with anything already there — a
colonist can be both an Extrovert and Tidy. `mutant attitude` is the same
shape, waiting on its own opposite (a purist who recoils from mutants).

`mutation` is different in kind: `TraitMutant` is marked `acquired`, so
`rollTraits` never draws it and no colonist is ever *generated* a mutant —
`mutate()` (see [mutation.md](./mutation.md)) hands it out during play via
`giveTrait`, which appends the trait and re-runs `resolveTraitEffects` so an
acquired trait behaves exactly like a rolled one. A group with nothing
rollable in it is skipped **before** any number is drawn from the personality
stream, so adding it left every existing seed's colonists byte-for-byte
unchanged.

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

### The exception: traits checked live, at event time

Not every trait fits that mold. `TraitTidy` has no need-rise/rest/work/social
effect to resolve — its only effect is an extra mood penalty when a colonist
sees gore, and that's read directly off `Profile.HasTrait(TraitTidy)` at the
moment the sighting happens, in `lifeevents.go`'s `applyMoodEffects` (see
[memories.md](./memories.md)). This doesn't violate "pay once, not per tick":
that principle is about the hot path, and a life event fires far less often
than every tick for every colonist. Resolving Tidy into a spawn-time field
would mean inventing an `Entity` field for a value `HasTrait` already answers
in one slice scan. The pattern to follow depends on how often the effect is
read: a per-tick or per-job cost belongs in `resolveTraitEffects`; a
per-event cost is fine read live.

`TraitIndustrious` shows a trait can use both mechanisms at once: its
work/rest multipliers are resolved at spawn as always, but its extra mood
lift on finishing a job (see [memories.md](./memories.md)) is a second,
independent effect checked live via `HasTrait`, exactly like Tidy's. Nothing
about having a `traitSpec` entry requires a trait to pick one mechanism
exclusively.

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
- **A trait gained during play**, not at spawn: mark its `traitSpec`
  `acquired: true` so `rollTraits` skips it, and hand it out with
  `giveTrait(e, trait)` from whatever system earns it. `TraitMutant` is the
  worked example.
- **Making an attribute mechanical**: give it an effect and fold it into
  `resolveTraitEffects` (or an equivalent resolve step) so it stays off the hot
  path.
- **A trait that affects a life event's mood impact** (like Tidy): no
  `traitSpec` field needed — add a `Conditional` `MoodEffect` entry naming the
  trait to the relevant `LifeEventKind`(s) in `lifeevents.go`. See
  [memories.md](./memories.md).

## Related

- [needs.md](./needs.md) — the need-rise rates traits scale.
- [entities-and-ai.md](./entities-and-ai.md) — how `workScale`/`restTicks` feed behavior.
- [configuration.md](./configuration.md) — `TraitChance`.
- [mutation.md](./mutation.md) — `TraitMutant` and `TraitMutantLover`, and how a trait is acquired in play.
- [memories.md](./memories.md) — life events, mood effects, and how `TraitTidy`
  hooks into them.

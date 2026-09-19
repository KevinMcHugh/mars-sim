# Personality & traits

> Part of the [mars-sim documentation](./README.md).

## What it is

Every colonist has a `Profile`: a name, attributes (age, gender, orientation,
height, weight, skin tone, hair color), and any traits. Most attributes are
flavor; **traits change how a colonist plays** by scaling need rates and work
behavior, and **height and weight are live state** once uranium is involved —
mutation resizes a colonist and scales their body with them (see
[mutation.md](./mutation.md)). All of it is *generated* from a dedicated RNG
stream so flavor never perturbs the simulation.

## Source

- [`internal/sim/personality.go`](../internal/sim/personality.go) — `Profile`, `Trait`, `traitSpecs`, generation, trait resolution.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — the trait-resolved effective params (`needRise`, `restTicks`, `workScale`).
- [`internal/sim/config.go`](../internal/sim/config.go) — `TraitChance`.
- [`internal/sim/heredity.go`](../internal/sim/heredity.go) — the family pass that overwrites part of a generated profile.

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
height/weight (via a BMI draw), a skin tone (uniform across the five emoji tone
points), a hair color (white and bald weighted upward with age), and a name
drawn from gender-appropriate pools. Nothing simulates against these at spawn —
they exist for flavor and future systems.

The one exception arrives later: `HeightCM`/`WeightKG` are what a mutation
rescales, and the body scales with them, so they stop being flavor the moment a
colonist takes a uranium dose. `BornHeightCM`/`BornWeightKG` keep the body the
colonist was generated with, because weight is always recomputed from that
rather than from the last rescaled value (see [mutation.md](./mutation.md)).
Every path that *generates* a body ends in `rememberBornBody` — the original
roll and heredity's `setHeightZ` re-framing alike — since inheriting a
relative's frame changes who the colonist always was, while a mutation changes
what became of them. Generated heights stay in the ordinary human 145–205 cm;
the configured stature limits bound what mutation does, and a colonist
generated outside a narrower range walks back into it on their first dose. The
resize itself runs on the *simulation* stream, not this one — it changes how
combat resolves.

It rolls a **complete, standalone person**, and part of that person is then
overwritten by [heredity.md](./heredity.md)'s family pass, which runs after kin
assignment because it needs to know who the colonist is related to. That is why
generation rolls everything up front: a colonist who inherits only their
mother's hair still has a skin tone and a height of their own to fall back on.

Three attributes are stored twice, because the value shown is not the value that
passes down: `hairBase` is the natural hair color under any age-driven white or
bald, `heightZ` is height as standard deviations from the colonist's own gender
mean, and `given`/`surname` are the halves of `Name` so a colonist can move onto
a family's name without re-parsing it. All three are unexported — nothing
outside generation reads them. `rollHair` returns the natural and shown colors
together, applying the age roll on top of a natural color rather than replacing
it in the same draw.

Full names are kept unique: `uniquifyName` re-draws the given name until no
other colonist answers to the same name, checking `World.colonistNames` (an
index of living colonists' names) rather than scanning the roster. The pools are
small enough that a colony of thirty collided about half the time without it,
and shared surnames make that likelier still.

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
| work ethic | Industrious / Lazy | work 0.75x time + rest 0.5x, plus doubled finished-work affect vectors / work 1.4x + rest 2.0x |
| social | Asocial / Introvert / Extrovert | no social need / social need 0.5x plus conversation fatigue / social need 1.5x |
| temperament | Tidy | 2.2x gore appraisal and doubled grip relief from incineration |
| mutant attitude | Mutant-Lover | extra affinity toward mutants and reflected mutation grip |
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
past its per-window capacity worsen conversation outcome. An Extrovert's need rises faster, so it
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
effect to resolve — it transforms gore and incineration vectors, read directly
off `Profile.HasTrait(TraitTidy)` during event appraisal in `affect.go` (see
[affect.md](./affect.md)). This doesn't violate "pay once, not per tick":
that principle is about the hot path, and a life event fires far less often
than every tick for every colonist. Resolving Tidy into a spawn-time field
would mean inventing an `Entity` field for a value `HasTrait` already answers
in one slice scan. The pattern to follow depends on how often the effect is
read: a per-tick or per-job cost belongs in `resolveTraitEffects`; a
per-event cost is fine read live.

`TraitIndustrious` shows a trait can use both mechanisms at once: its
work/rest multipliers are resolved at spawn as always, but its finished-work
vector amplification is a second, independent effect checked live via
`HasTrait`, exactly like Tidy's. Nothing
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
- **A trait that transforms life-event affect** (like Tidy): no `traitSpec`
  field is needed — add its case to `transformMoodVector` in `affect.go` and
  pin declaration-order behavior. See [affect.md](./affect.md).

## Related

- [needs.md](./needs.md) — the need-rise rates traits scale.
- [entities-and-ai.md](./entities-and-ai.md) — how `workScale`/`restTicks` feed behavior.
- [configuration.md](./configuration.md) — `TraitChance`.
- [mutation.md](./mutation.md) — `TraitMutant` and `TraitMutantLover`, and how a trait is acquired in play.
- [affect.md](./affect.md) — life-event vectors and how appraisal traits hook in.
- [memories.md](./memories.md) — the one life-event ingestion funnel.
- [heredity.md](./heredity.md) — the family pass that rewrites a generated
  profile's surname and appearance.

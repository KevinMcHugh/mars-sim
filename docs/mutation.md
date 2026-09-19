# Uranium & mutation

> Part of the [mars-sim documentation](./README.md).

## What it is

Uranium is a fourth rock composition, mined like iron or water ice — and the
only one that acts back on the colonist who handles it. A colonist standing
beside an unexcavated uranium deposit, or carrying uranium ore in their pack,
accumulates a **dose**. Every `UraniumExposureTicks` (2000) of accumulated dose
is one roll at `MutationChance` (1%) to **mutate**: grow a body part nobody is
born with (a third arm, an extra eye, a tail, a vestigial twin) and pick up the
**Mutant** trait for good.

Those two numbers are deliberately harsh. Mutation is a rarity the colony talks
about, not a career stage: with uranium at `UraniumRockPercent` (1% of rock) and
a dose this long, about **1% of colonists** are mutants after a typical run,
creeping toward 3% in a very long one. See [Tuning the rate](#tuning-the-rate).

Mutants are then a thing other colonists have opinions about. The
**Mutant-Lover** trait, rolled at spawn like any other, makes a colonist warm to
mutants much faster than to anyone else, and inverts how they feel about
watching a mutation happen — or undergoing one.

## Source

- [`internal/sim/mutation.go`](../internal/sim/mutation.go) — exposure, the
  mutation roll, `growPart`, `giveTrait`, `mutantAffinityBonus`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — the mutant half of the
  `BodyPart` enum, `numBaseBodyParts`, `Entity.MaxParts`, `Entity.hasPart`,
  `Entity.uraniumExposure`.
- [`internal/sim/world.go`](../internal/sim/world.go) — `UraniumBearingRock`.
- [`internal/sim/inventory.go`](../internal/sim/inventory.go) — `UraniumOre`,
  `Inventory.Has`, `miningYield`.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go) — uranium veins.
- [`internal/sim/personality.go`](../internal/sim/personality.go) —
  `TraitMutant` (acquired), `TraitMutantLover`, `traitSpec.acquired`.
- [`internal/sim/combat.go`](../internal/sim/combat.go) — `rollHit` weighting
  over the target's own anatomy.
- [`internal/sim/lifeevents.go`](../internal/sim/lifeevents.go) — `EvtMutated`,
  `EvtWitnessedMutation` and their trait-transformed affect vectors.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `colonistTurn`'s
  exposure step, `finishTalk`'s per-direction affinity credit.
- [`internal/sim/mutation_test.go`](../internal/sim/mutation_test.go) — the
  tests that pin this behavior.
- [`internal/ui/tui/glyphs.go`](../internal/ui/tui/glyphs.go) — 🟩 uranium rock,
  🧟 mutant colonist.

## How it works

### Exposure

`colonistTurn` calls `applyUraniumExposure(e)` right after the sighting pass,
*before* any branch that can return — uranium does not care whether the
colonist is fleeing, eating, or digging.

`uraniumExposed(e)` is true when either:

1. the colonist carries `UraniumOre` (`Inventory.Has`), or
2. any of the eight neighboring tiles is unexcavated `UraniumBearingRock`.

Both are bounded, allocation-free checks (eight slots, eight neighbors), which
matters because this runs for every colonist every tick.

Exposure is **cumulative and never decays**. "Prolonged exposure" is meant as a
record of how much uranium this colonist has handled in total, not a level they
can cool off from by working elsewhere for a while. The counter is spent, not
latched: crossing the threshold rolls once and resets to zero, so a colonist who
keeps working a vein keeps rolling, and can mutate more than once over a career.

### Mutating

`mutate(e)` draws uniformly (on `w.rng`, the simulation stream — this is
gameplay, not flavor) among the mutant parts the colonist has *not* grown,
calls `growPart`, and `giveTrait(e, TraitMutant)`. It records an `EvtMutated`
memory for the colonist, an `EvtWitnessedMutation` memory for everyone close
enough to see, and a colony log line. A colonist who already has all four parts
keeps the trait and grows nothing further.

### Body parts, before and after

`BodyPart` is split by the `numBaseBodyParts` marker: the six parts everyone has
below it, the four mutant ones above. **Which parts an individual has is
per-entity data**, not a property of the enum — `Entity.MaxParts[p] > 0`
(`hasPart`) is the test, which also keeps "never grown" distinct from
"destroyed" (`Parts` zero, `MaxParts` intact).

`growPart` sizes the new part from the colonist's *base* body (`baseBodyHP`,
the sum of the base parts' maxima) by the part's `bodyPartWeight`, and **adds**
it to `HP`/`MaxHP`:

| Part | Weight | On a 40 HP colonist |
| --- | --- | --- |
| third arm | 12 | +4 HP |
| extra eye | 5 | +2 HP |
| tail | 8 | +3 HP |
| vestigial twin | 15 | +6 HP |

`rollHit(target)` then weights over the parts that target actually has, so a
grown limb is one more place to be wounded — and, because the weights are over
a bigger total, a slightly *smaller* chance that any single hit finds the head
or torso. A mutant is therefore a little harder to kill. That is deliberate: the
cost of being a mutant is social, not physical.

### Being a mutant

- **Affect.** `EvtMutated` adds `(4,-18)` and witnessing adds `(2,-8)`.
  Mutant-Lover appraisal reflects grip positive. The base rows and transform live
  in `affect.go` (see [affect.md](./affect.md)), not branches in `mutate()`.
- **Affinity.** `finishTalk` credits affinity **per direction** instead of
  through the symmetric `addAffinity`: both sides get the conversation's own
  step, and a Mutant-Lover additionally gets `MutantLoverAffinityBonus` (3) of
  one-way warmth toward a mutant partner. That bonus rides on top of the talk
  step and is *not* subject to its half-of-`AffinityMax` saturation, so a
  mutant-lover's regard for a mutant can climb into the outer range plain
  conversation can never reach.
- **Display.** A mutant draws as 🧟 on the map instead of their age/gender
  figure, and the roster's BODY line lists the parts they actually have.

## Why it is this way

- **Exposure is proximity *or* carrying, not just mining.** Tying it to the
  moment of excavation would make it a single coin flip per uranium tile. Tying
  it to the ore in the pack makes uranium a liability you carry around, which
  is the interesting version: a colonist keeps its dose until it hauls the ore
  into a chest (`jobStore`, see [storage.md](./storage.md)), so how long a miner
  stays dosed is a consequence of how the colony handles what it digs up.
- **A mutant part adds HP instead of redistributing it.** Sizing all parts from
  one `MaxHP` would mean growing a third arm quietly weakened every limb the
  colonist already had — a mutation that makes you worse everywhere is not what
  "grew an extra arm" should mean, and it would have made `MaxParts` a pure
  function of `MaxHP` in name only.
- **`MaxParts` is stored on the `Entity`, not recomputed.** It used to be
  derived from `MaxHP` in `entityView`. Once mutation exists the two genuinely
  diverge, and a derived value would have had to re-derive *which* parts this
  body has as well as their sizes.
- **`TraitMutant` is an `acquired` trait in the ordinary trait table** rather
  than a bool on `Profile`. It then gets trait display and `HasTrait` checks for
  free. The `acquired` flag keeps
  `rollTraits` from ever generating a pre-mutated colonist, and a group with
  nothing rollable is skipped before drawing from the personality stream, so
  adding all of this left existing seeds' colonists unchanged.
- **Uranium veins are grown after iron and ice in `generate`**, so introducing
  uranium left those older deposits exactly where they were for existing seeds.
  Clay follows uranium for the same compatibility reason.
- **Affinity became directional.** Storage was always per-direction
  (`affinity[a][b]`); `addAffinity` just kept the two in step. The Mutant-Lover
  bonus is a fact about the admirer, not about the pair, and nothing but a
  per-direction credit can express that. Conversation quality and the affect
  outcome's company term read `mutualAffinity` (the mean of both directions) so a
  *pair* property cannot depend on which colonist was passed first.

## Tuning the rate

The knob that decides how many colonists become mutants is **not**
`MutationChance` — it is `UraniumExposureTicks`, because exposure never decays
and a full dose is spent rather than latched. A colonist working near uranium
accumulates dose indefinitely, so it is the *number* of rolls that converges,
and the per-roll chance is close to saturated however low it is set.

Measured over 8 seeds of a 120x60 map with 40 colonists and 10,000 ticks
(colonists carrying the Mutant trait at the end of the run):

| uranium % | `MutationChance` | `UraniumExposureTicks` | mutants |
| --- | --- | --- | --- |
| 3% | 25% | 100 | 70% |
| 1% | 25% | 100 | 43% |
| 1% | 5% | 100 | 40% |
| 1% | 1% | 100 | 20% |
| 1% | 1% | 500 | 3.8% |
| 1% | 1% | 1000 | 3.1% |
| **1%** | **1%** | **2000** | **0.9%** |

Two lessons for anyone re-balancing this: cutting `MutationChance` alone barely
moves the outcome (25% → 1% only took 43% of colonists down to 20%), and the
curve is steep past a dose of ~1000 ticks. Lengthening the dose is the lever;
the chance is the fine adjustment on top of it.

### The rate drifts with game length

Because the dose is permanent, the share of mutants is not a fixed rate — it
keeps climbing for as long as a run goes on. At the defaults, over 20 seeds:

| run | mutants |
| --- | --- |
| 80x40, 6 colonists, 10,000 ticks (~20 min at 8 tps) | 0.9% |
| 80x40, 6 colonists, 30,000 ticks (~1 hr) | 3.6% |
| 120x60, 40 colonists, 30,000 ticks | 2.9% |

That drift is a property of the mechanic, not of the numbers: with a permanent
dose and unbounded rolls, every colonist who keeps working uranium mutates
eventually. Making the rate genuinely independent of game length would take a
change to the mechanic — a decaying dose, or one roll per colonist per career —
not a smaller `MutationChance`.

## Extending it

- **A new mutant part**: add the constant between `numBaseBodyParts` and
  `numBodyParts`, its `String()`/`Short()` names, its `bodyPartWeight`, and an
  entry in `mutantParts`. Keep it non-`Vital` unless a mutation really should
  add a new way to die outright.
- **A mutant-hater ("Purist")**: a `traitSpec` in `groupMutantAttitude` plus a
  negative `mutantAffinityBonus` branch, and a grip transform for
  `EvtMutated`/`EvtWitnessedMutation` in `transformMoodVector`. The group was made its own axis for
  exactly this.
- **Another source of mutation** (an alien bite, a lab accident): call
  `w.mutate(e)`. Nothing in it is uranium-specific past the memory text.
- **Making mutation cost something physical** (a work or need penalty scaled by
  part count) is the obvious balance lever if mutants prove strictly better;
  `resolveTraitEffects` is where a scalar effect for `TraitMutant` would go.
- **Dropping ore into a stockpile** would turn permanent exposure back into a
  choice, and is the natural follow-up once hauling exists (see
  [inventory.md](./inventory.md)).

## Related

- [world.md](./world.md) — rock composition and how veins are generated.
- [inventory.md](./inventory.md) — `UraniumOre` and the no-drop rule that makes
  carrying it a permanent dose.
- [combat.md](./combat.md) — body parts, `rollHit`, and damage.
- [personality.md](./personality.md) — traits, trait groups, and acquired traits.
- [affect.md](./affect.md) — life-event vectors and trait transforms.
- [memories.md](./memories.md) — the life-event ingestion funnel.
- [configuration.md](./configuration.md) — the tunables and their flags.

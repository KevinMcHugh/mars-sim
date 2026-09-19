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

A mutation also **resizes** the colonist. Every one of them moves their height
a `MutationStaturePercent` (15%) step up or down, and the body goes with it —
weight, HP, and every body part scale together, between `StatureMinCM` (61 cm,
two feet) and `StatureMaxCM` (305 cm, ten feet). Since each mutation is one
step of a random walk, the ends of that range belong to the rare colonist who
mutates over and over — a ten-foot one stooping through the dormitory, or a
two-foot one who goes down to a single alien bite. At the default rate that is
a whole colony's story, not a career; [Tuning the rate](#tuning-the-rate) says
which knob to turn for more of it.

Mutants are then a thing other colonists have opinions about. The
**Mutant-Lover** trait, rolled at spawn like any other, makes a colonist warm to
mutants much faster than to anyone else, and inverts how they feel about
watching a mutation happen — or undergoing one.

## Source

- [`internal/sim/mutation.go`](../internal/sim/mutation.go) — exposure, the
  mutation roll, `growPart`, `resize`/`rollStature`/`setStature`/`scaleBody`,
  `FormatHeight`, `giveTrait`, `mutantAffinityBonus`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — the mutant half of the
  `BodyPart` enum, `numBaseBodyParts`, `Entity.MaxParts`, `Entity.hasPart`,
  `Entity.uraniumExposure`.
- [`internal/sim/world.go`](../internal/sim/world.go) — `UraniumBearingRock`.
- [`internal/sim/inventory.go`](../internal/sim/inventory.go) — `UraniumOre`,
  `Inventory.Has`, `miningYield`.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go) — uranium veins.
- [`internal/sim/personality.go`](../internal/sim/personality.go) —
  `TraitMutant` (acquired), `TraitMutantLover`, `traitSpec.acquired`,
  `Profile.HeightCM`/`WeightKG` and the `BornHeightCM`/`BornWeightKG` they are
  rescaled from, `rollBody`.
- [`internal/sim/config.go`](../internal/sim/config.go) —
  `UraniumExposureTicks`, `MutationChance`, `MutationStaturePercent`,
  `StatureMinCM`, `StatureMaxCM`.
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
- [`internal/ui/tui/render_roster.go`](../internal/ui/tui/render_roster.go) —
  the attributes line, which leads with feet and inches.

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

`mutate(e)` attempts **both** changes uranium can make, and each can decline:

1. It draws uniformly (on `w.rng`, the simulation stream — this is gameplay,
   not flavor) among the mutant parts the colonist has *not* grown and calls
   `growPart`. A colonist who already has all four grows nothing further.
2. It calls `resize(e)`, which moves the colonist one stature step. A colonist
   pinned at a limit can still move — the other way (see below).

Whatever actually happened is joined into one phrase ("grew a tail and
stretched from 5'10" to 6'9""), which becomes the colonist's `EvtMutated`
memory, an `EvtWitnessedMutation` memory for everyone close enough to see, and
a colony log line. `giveTrait(e, TraitMutant)` runs on any change. If *neither*
half found anything to do — every part grown, and resizing disabled — nothing
happened at all: no trait, no memory, no log line.

### Stature

`resize` is a fixed-size step in a drawn direction, not a drawn size:
`rollStature` builds the candidates (one step up, one step down), drops
whichever the configured limits forbid, and picks among what is left on
`w.rng`. Two consequences fall out of that shape:

- A colonist's height is their starting height times `1.15ⁿ`, where n is lucky
  doses minus unlucky ones. Growing multiplies by 1.15 and shrinking *divides*
  by it, rather than taking 15% off: the two must be exact inverses, or a walk
  that is supposed to be a fair coin flip drifts steadily downward (0.85 × 1.15
  is 0.98, and that missing 2% compounds over a career). From an average
  178 cm it is two straight growths to clear seven feet, four to touch the
  ten-foot ceiling, and eight shrinks to reach the two-foot floor.
- A colonist **at** a limit is not wasting the dose: with one candidate
  forbidden, the other one is certain, so the ten-foot colonist's next
  mutation shrinks them. The extremes are sticky walls, not absorbing ones.

`setStature` then moves the rest of the body:

| | scales as | 178 cm → 305 cm | 178 cm → 61 cm |
| --- | --- | --- | --- |
| weight | height² (constant BMI) | 70 kg → 206 kg | 70 kg → 8 kg |
| `MaxHP`, every part | height | 40 → ~69 | 40 → ~14 |

Weight is recomputed from `BornHeightCM`/`BornWeightKG` — the body the colonist
arrived with — rather than from the weight they are now. Rescaling a rounded
integer over and over is a ratchet: an early version scaled from the current
weight and ground an 80 kg colonist down to 1 kg over a career of stretching
and shrinking back.

`scaleBody` scales current values alongside the maxima, so a resize neither
heals a wound nor opens one — a colonist half dead before is half dead after.
A part that gets small never rounds away to nothing (`MaxParts` at zero means
an anatomy that never had the part, which would be a silent amputation), while
a *destroyed* part (current zero) stays destroyed. Unlike weight, the parts are
scaled step by step from their current size, so a colonist resized dozens of
times carries a few HP of rounding noise; it is bounded and unbiased, and the
body still tracks the height within a few percent.

Heights are displayed in feet and inches (`FormatHeight`), which is the only
unit in which "ten foot tall" is a thing to say; the roster's attributes line
leads with it and keeps the centimetres in parentheses.

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
- **Mutation resizes as well as grows.** Growing a part is a strict
  improvement — more HP, a bigger target pool — so mutation with parts alone is
  a buff a colony would farm deliberately. Stature makes every mutation a coin
  flip on the thing that matters most in a fight, which is what turns "go stand
  by the uranium" from an exploit into a gamble. It is drawn on `w.rng` for the
  same reason the part is: it changes how combat resolves.
- **The step is fixed and only the direction is drawn.** Drawing a magnitude
  too would make each mutation a little more surprising and the *history*
  unreadable — with a fixed step, a colonist's height is a running count of
  their luck, and the extremes are a story about a career at the vein rather
  than one jackpot roll.
- **Weight scales as height², HP as height¹.** Squaring the height keeps the
  colonist's BMI — the build `rollBody` gave them — exactly as it was, so a
  two-foot colonist reads as a small person rather than as something that could
  blow away; a true cube law is the physical answer for a scaled statue, not
  for a person. HP deliberately scales *slower* than mass: a ten-foot colonist
  who came out of the uranium four times as hard to kill would end the alien
  problem by standing in the wrong tunnel often enough.
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

### Stature needs repeat mutations, so it is rarer still

A mutation is one 15% step, and the extremes are several steps from an ordinary
178 cm: four growths to the ten-foot ceiling, eight shrinks to the two-foot
floor. At the default rate — where mutating *once* already puts a colonist in
the 1% — a colonist reaching either end is a whole colony's story rather than
something a run reliably produces.

Two knobs change that, and they do different things:

| want | turn | effect |
| --- | --- | --- |
| more mutants, same drama each | `UraniumExposureTicks` down | more colonists mutate, each still a 15% step |
| the same rarity, more drama | `MutationStaturePercent` up | one mutation is a bigger jump |

At `MutationStaturePercent` 40, a single mutation takes an average colonist to
8'2" or down to 4'2", and two put them at a limit — so the first colonist to
mutate is visibly a giant or a dwarf, without mutation itself becoming common.
That is the knob to reach for if the extremes are the point; the default of 15
is tuned for a legible history (a colonist's height reads as a running count of
their luck) over immediate spectacle.

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
- **Making mutation cost something physical** beyond the stature gamble (a work
  or need penalty scaled by part count) is the obvious balance lever if mutants
  prove strictly better; `resolveTraitEffects` is where a scalar effect for
  `TraitMutant` would go.
- **Making stature do more than HP**: a giant that mines faster and eats more,
  a two-foot colonist that squeezes through gaps or is passed over by aliens
  hunting. `Profile.HeightCM` is live state any system can read; a scalar
  derived from it would slot into `resolveTraitEffects` next to the trait
  multipliers, which is already the one place `workScale` is set.
- **A distinct glyph for the extremes** (the roster already reads 8'4"): the
  mutant 🧟 could give way to something taller or smaller past a threshold. See
  the tone/hair caveats in [personality.md](./personality.md) before reaching
  for a composed emoji.
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

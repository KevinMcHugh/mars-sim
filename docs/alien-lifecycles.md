# Alien lifecycles

> Part of the [mars-sim documentation](./README.md).

## What it is

Some alien species change over a life: an egg that hatches into a grub, a
joey that pupates, a nymph that becomes a queen. Each seed rolls them with
its roster. Most species have a single form, and every species, single-form
or not, rolls **graded anatomy**: horns, antlers, quills, a shell, claws, a
stinger, a tail club, in degrees a life can grow along. A species with a
lifecycle also **breeds**: its adult (or its queen, betty, jill or matriarch)
lays the first form of its life, an egg or live young, until its nest or its
numbers are full. The tone is the platypus: plausible parts assembled
slightly wrong.

This is phase 4b of [species-and-behaviors.md](./species-and-behaviors.md):
each form of a life is its own species value with its own behavior ladder.

## Source

- [`internal/sim/alien_anatomy.go`](../internal/sim/alien_anatomy.go) — `AlienAnatomy`, `TailTip`, `rollAnatomy`, `scaledAnatomy` (a young form's lesser features), `featurePhrases`.
- [`internal/sim/alien_lifecycle.go`](../internal/sim/alien_lifecycle.go) — `AlienForm`, the stage vocabulary (`stageWords`) and caste sets (`casteSets`), `rollLifecycle`, `pushedAnatomy` (a caste's), and the individual side: `beginLife`, `growUp`, `layBrood` (with `broodCrowded`, `speciesPopulation`, `startYoung`), `resizeAlien`, `alienDamage`, `inert`, `alienFormNoun`, and the lore tab's `lifePhrase` and `broodPhrase`.
- [`internal/sim/components.go`](../internal/sim/components.go) — `LifeStage`, the component an alien of a multi-form species carries.
- [`internal/sim/species.go`](../internal/sim/species.go) — `newAlienSpeciesTable`: one species per form, each with its ladder.
- [`internal/sim/lore.go`](../internal/sim/lore.go) — the `Anatomy`, `Forms` and `FormCount` fields on `AlienSpecies`, the two extra passes in `rollAlienSpeciesRoster`, and the sentences `Description` adds.
- [`internal/sim/config.go`](../internal/sim/config.go) — `alien-one-form-weight` … `alien-four-form-weight`, `alien-caste-percent`, `alien-stage-ticks`; and for broods `alien-lay-ticks`, `alien-brood-cap`, `alien-brood-radius`, `alien-species-cap`, `alien-nest-roam`; and `alien-apex-percent`.
- [`internal/sim/alien_lifecycle_test.go`](../internal/sim/alien_lifecycle_test.go) — the rules below, as tests.

## How it works

### A life, rolled once per seed

`rollLifecycle` gives a species its `Forms`, in stage order, or leaves
`FormCount` at 0 for a single form. The steps:

1. **How many stages.** One to four, by the four weights (60 / 25 / 12 / 3 by
   default). Across 48 rolled species in a sample of seeds, 56% were
   single-form.
2. **Which family of words.** The hide suggests one: fur and hair suggest
   mammal words (joey, puggle, kit, cub, yearling), chitin and armor insect
   words (grub, larva, nymph, instar, pupa, cocoon, chrysalis), feathers bird
   words (hatchling, eyas, squab, fledgling), scales, slime, smooth skin and
   jelly aquatic words (spawn, polliwog, elver, eft), bark and moss plant
   words (bud, sporeling, husk). One species in five draws from every family
   instead: the furred thing that pupates. Any line may begin as an **egg**
   (40%), whatever its family.
3. **Sizes.** The first stage is 8–30% of adult size, each next stage closes
   30–70% of the remaining gap, and the adult is 100%.
4. **The young.** Each stage before the adult gets a word fit for its place
   (`early` or `middle`), never one the line already used. A middle stage is a
   casing (cocoon, pupa, chrysalis, husk; or an egg) 35% of the time when the
   stage before it was not. A mobile young form has part of the adult's body:
   fewer limbs, a tail only past halfway through the life, no wings yet, and
   `scaledAnatomy`'s lesser features. A casing shows nothing.
5. **The adult, or its castes.** 30% of multi-stage lines end in castes: a
   caste set that fits the family (queen/worker/drone, bull/betty, jack/jill,
   or matriarch/drudge/sire), each caste with odds and a size of its own. A
   caste bigger than the adult pushes each of the species' features further
   (`pushedAnatomy`); a smaller one carries less.

Each stage lasts about `alien-stage-ticks` (two colony days), give or take a
quarter, rolled per stage.

A real roll (seed 4 with eight species, `DefaultConfig` otherwise), from the lore tab:

> Oranges stand 0.5-1.2 m … They can be recognized by their orange armor
> plates, 6 eyes, 3 arms, 2 legs, and a tail. … Adults bear a coat of quills.
> Their life has 4 stages: a nymph, about a third of adult size, with 2 legs;
> a larva, about half adult size, with 2 legs, 1 arm, a tail, and a few
> quills; a pupa; then the adult castes. Adults come as queens, workers or
> drones; a queen, the largest, bears a coat of quills.

### Graded anatomy

`rollAnatomy` gives every species its adult features, each rare on its own
(horns 22%, claws 25%, a shell 20%, spines 18%, a stinger 12%, antlers 10%, and
a tail ornament for 30% of tailed species), so most species carry one or two
and some none. Each feature has degrees: 1 to 12 horns ("a single nub of a
horn" … "a crown of 12 horns"), 2 to 14 antler tines, quills from "a few" to
"a coat", a shell from "a patch" to "a full carapace", claws from "blunt
nubs" to "scythe-like", a stinger or a barbed one, and a tail ending in a
club, a spiked club or a stinger. Most of them fight (below); spines do
not, yet.

### Features that fight

`alien_weapons.go` turns the body into combat, always reading the alien's
**current form** (`anatomyOf`): a young form's lesser features, a queen's
pushed ones. A species grows into its weapons.

| Feature | In a fight |
| --- | --- |
| horns or antlers | grant **gore**: +8% damage per horn and +5% per antler tine, at most double |
| a stinger, or a tail ending in one | grant **sting**: half damage (three quarters if barbed), but always to the torso, a vital part |
| claws | the claw mode rakes 25% harder per grade |
| a clubbed tail | the tail mode hits 50% harder (75% for a spiked club) |
| a shell | every hit on it, gunshot or strike, is blunted 10% / 20% / 30% (patch, plates, carapace); a hit that does damage still does at least 1 |

Gore and sting are **granted, not rolled**: every species with horns gores,
every species with a stinger stings (`featureAttacks`, applied in the anatomy
pass). They live in their own list, `featureAttackModes`, after the rolled
`attackModes`, because `rollAttackModes` walks `attackModes` drawing from the
lore stream and appending to it would have re-rolled every later species'
attacks. At the moment of a blow, `attacksNow` drops a feature mode the body
cannot use yet (a hornless grub of a horned species bites). A body without
any feature fights exactly as before: same modes, same damage, same draws.
Descriptions say "goring with their horns" and "stinging"; narration says
"gores" and "stings".

### Apex species

A few species are **very deadly on purpose**. `rollApex` marks
`alien-apex-percent` (8%) of the species that both fight (not Friendly) and
have a feature; their features hit by `apexWeapons` instead of
`ordinaryWeapons`:

| | ordinary | apex |
| --- | --- | --- |
| gore | +8% per horn, +5% per tine, at most 2x | +15% per horn, +10% per tine, at most 3x |
| claws | +25% per grade | +50% per grade |
| club / spiked club tail | 1.5x / 1.75x | 2x / 2.5x |
| sting (to the torso) | 0.5x / 0.75x barbed | 1x / 1.5x barbed |
| shell blocks | 10% / 20% / 30% | 25% / 45% / 60% |

Across 1,000 rolled species, 55 were apex: with the shipped one species per
seed, about one game in twenty meets one. The lore tab warns: "Colonists who
have seen what those can do speak of them in whispers; nothing else in the
caves is as deadly." The roll draws from its own stream (`alienApexSeed`),
once per species whatever its anatomy or temperament, so it moves no other
draw and one species never shifts another's roll.

What it does to a colony, over 20,000 ticks with 20 colonists, the first six
apex seeds run with the apex roll off and on: a Hostile goring species took
seed 11 from 19 survivors to 3 and seed 12 from 10 to 0; a Hostile stinger
(seed 74) and the Cautious apex species (seeds 16, 36, 66) changed nothing,
because a Cautious species only strikes a colonist who comes close.

### An individual's life

An alien of a multi-form species carries a `LifeStage` component: which form
it is, and the tick it grows into the next stage.

- **Spawning** (`beginLife`): at a random stage (a caste by its odds), part
  of the way through it, so a nest holds a spread of ages. These are
  `World.rng` draws, and only species with a lifecycle make them.
- **Growing** (`growUp`, a pre-step of `animalTurn`): when its time comes it
  takes the next stage's form (a caste by its odds), grows to that size
  keeping its share of health (`resizeAlien`: a wounded grub is a wounded
  nymph), and the log records "A grelk egg hatches into a grelk grub." An egg
  hatches, a casing's occupant emerges, anything else molts. Growth in a
  cave the colony has not found happens unlogged.
- **Behavior** (`newAlienSpeciesTable`): an egg or casing's ladder is
  `inert` (it lies there); a mobile young form grazes, whatever its species'
  temperament (`dormant`, `grazeScum`, `wander`); the adult and its castes
  run the temperament's ladder.
- **Strength**: hit points and strike damage scale with the form's size
  (`resizeAlien`, `alienDamage`), never below `minAlienFormHP` (7).
- **Naming**: narration says "a grelk grub", "a grelk queen"; the plain adult
  is just "a grelk". The inspector and roster show the form after the
  species label.
- **Threat**: an egg or casing is no threat. Colonists do not perceive it,
  flee it or fight it (`nearestAlien` and `observePersistent` skip it as they skip
  a dormant alien). Hostile aliens of another species can still eat one.

### Broods

Each life has exactly one **laying form** (`AlienForm.Lays`): the plain adult
of a line without castes, or its laying caste: the queen (not the worker or
drone), the betty (not the bull), the jill (not the jack), the matriarch (not
the drudge or sire). Single-form species do not breed; their numbers are
still only what worldgen's nests and the director put down.

A laying alien carries a brood timer (`LifeStage.layAt`): spawned as a
layer, part of the way through an interval so a nest's layers do not lay on
the same tick; grown into one, a full `alien-lay-ticks` (four colony days)
away. When it comes due, `layBrood` (an `animalTurn` pre-step that does not
use the layer's turn):

1. resets the timer, whatever happens next;
2. does nothing if `alien-brood-cap` (8) of its species already live within
   `alien-brood-radius` (6) of it, itself included, or `alien-species-cap`
   (16) of its species live anywhere;
3. does nothing if no floor tile beside it is free;
4. otherwise lays the species' first form there, at the very start of that
   stage and at full health (`startYoung`), and logs it: "A grelk queen lays
   a grelk egg." when the first form is an egg, "A grelk betty bears a grelk
   joey." when it is live young. A brood in an undiscovered cave is laid
   unlogged, as growth is, so a nest found late can be a big one.

**A laying caste keeps to her nest.** A queen, betty, jill or matriarch
remembers where she was spawned as one, or where she grew into one
(`LifeStage.nest`). Her ladder is her temperament's with a `stayNear` rung
right after dormancy: beyond `alien-nest-roam` (4) tiles of her nest she
walks home before anything else, and within it she does what her
temperament does, so a Hostile queen still strikes what comes close but is
tethered to her brood. The roam is under the brood radius (6), so what she
lays near home counts toward her nest's cap. The plain adult of a line
without castes roams like any adult: if it kept to a nest too, such a
species could never spread past where it started. `stayNear` is the
chicken's rung: the trough and the nest are two anchors for one behavior.

The lore tab's life paragraph ends with who breeds and how: "Only the
queens lay eggs.", "Adults bear young."

## Why it is this way

- **Generated, mostly absent, and additive.** Authored stages would cap the
  variety at what someone wrote. Most species being single-form keeps a
  lifecycle a discovery. Each stage being a superset of the last is what
  lets a colonist recognize a grub as the young of the thing that killed
  someone, which matters more than biological accuracy.
- **Own streams.** Anatomy and lifecycles roll on their own seeded streams
  (`alienAnatomySeed`, `alienLifecycleSeed`) after the roster and its
  taxonomy, so adding them changed no existing species' build, name or
  temperament. `TestScientificNamesDoNotShiftTheRoster` now guards all three
  later passes.
- **A fixed array of forms.** `AlienSpecies.Forms` is a `[7]AlienForm` with a
  count, not a slice: the roster is compared with `==` (the snapshot tests
  do) and copied into every snapshot, and an array keeps both working and
  keeps a frontend's copy from aliasing the world's.
- **The egg that never hatched.** The first version sized an egg's hit points
  as its share of the adult's: 8% of 30 is 2. At 2 HP the head's share of the
  body rounds to 0, `Entity.Alive` reads a body with an empty vital part as
  dead, and `step` skips the dead, so the egg sat in its cave forever: never
  removed (nothing killed it) and never given a turn to hatch. Every form now
  has at least 7 HP, and rescaling a wounded body never rounds a living part
  down to 0. `TestEveryFormSpawnsAlive` holds it.
- **Tests opt in.** `testConfig` gives every species a single form, as it
  turns off pets: mechanics tests spawn an alien and expect an adult that
  hunts or grazes by temperament, never an egg. Lifecycle tests use
  `lifecycleConfig`.
- **Young that do not hunt.** A hostile species' young hunting colonists
  would make nests far deadlier than their size suggests, and "an egg does not
  hunt" was the proposal's rule. Grazing keeps the young a competitor for
  scum, like a peaceful species.

- **Two caps, because one ran away.** The first version capped broods only
  by local crowding (`alien-brood-cap` within `alien-brood-radius`). In a long
  run with an alien-heavy setup (seed 9, six species, 30 starting aliens) the
  population went 30, 33, 46, 79, 108, 159, 243 over 30,000 ticks and was
  still climbing: the young wander out of the layer's radius, so the nest
  never looks full. The species-wide cap (`alien-species-cap`) is what bounds
  it; the same run levels off at 92 (three breeding species at the cap, plus
  the rest).
- **The default cap is gentle on purpose.** At the shipped alien settings,
  seeds whose one species has a lifecycle were run for 30,000 ticks with a
  species cap of 30 and a brood every three days, and with 16 and four days.
  At 30 a Hostile, three-stage species (seed 9) reached its cap and wiped out
  a 20-colonist colony; at 16 the same seed's brood never passed 4 and was
  cleared, though the colony ended with 7 of its 20 (seed 5: the brood grew to 6 before colonists cleared it). The
  harsher numbers are a config change away.

- **Features were cut back after a balance run.** The first numbers (a
  gore up to 2.5x, a sting at 75% / 100% to the torso, a shell blocking
  20% / 35% / 50%) wiped out three of sixteen 20-colonist colonies at the
  shipped alien settings over 20,000 ticks, against one before. Two of
  the three were a full carapace on a breeding species: at half damage
  from every pistol shot a brood was all but impossible to clear. With the
  numbers in the table, forty seeds lost four colonies against six before,
  and ended with 641 colonists against 625: no harsher overall. Single
  seeds flip either way (the wiped seed 8 was wiped again with stings
  rolling their hit like any blow), because any change to which modes a
  species rolls reshuffles every later draw; judge balance across many
  seeds, not one.

- **Deadly is fine; common is not.** After the balance run cut the
  ordinary numbers back, the stronger ones came back as the apex table:
  the game can have species that wipe a colony, as long as they are rare.
  Rarity is the knob (`alien-apex-percent`); the apex numbers are not meant
  to be fair.
- **A flaky test that was a lifecycle assumption.** Adding apex turned up a
  test that failed about one run in fifteen, on `main` too:
  `TestBystanderRemembersAlienAttack` uses `DefaultConfig`, whose seed is the
  clock, and set a victim's HP to its species' full bite damage. Since
  lifecycles, an alien can spawn as a young form that bites for less, so the
  "fatal" bite sometimes was not. It now uses `alienDamage`, what this alien
  actually deals. A test on `DefaultConfig` gets a different roster every
  run; it must not assume an adult, a single form, or no features.

## Extending it

- **Spines that fight back.** Quills are the one feature with no combat
  effect yet. A natural one: an attacker striking a quilled alien takes
  damage back (only another alien strikes one in melee today).
- **Venom.** A sting is a torso blow today; lingering damage would need a
  status component on the victim and a death path for it outside the turn
  that dealt it.
- **Breeding for single-form species.** They do not breed; giving them a
  laying form would need a first form to lay (a hatchling the size of the
  adult reads wrong), or a separate rule.
- **Eggs as targets.** Colonists could smash eggs and casings (a work order,
  or a fight target that does not trigger flight).
- **Stage words in names.** Feature names exist ("unicorn", "urchin",
  "horror"; see [lore.md](./lore.md#naming-a-condition-gated-pool-not-a-flat-table)),
  but nothing names a species for its lifecycle yet (a "broodmother" for a
  line with queens, say). It would go in the same second pass.
- Keep each young form a lesser version of the next (`gainsOnly` in the
  tests), keep sizes rising, and keep every new draw on the lifecycle or
  anatomy stream, or on `World.rng` when it is a gameplay decision.

## Related

- [species-and-behaviors.md](./species-and-behaviors.md) — the species model each form is a value of.
- [lore.md](./lore.md) — the roster these passes extend, and the description they add to.
- [caverns.md](./caverns.md) — nests and dormancy; a young form's ladder opens with `dormant` like the adult's.
- [combat.md](./combat.md) — body parts, and why a body with an empty vital part is dead.
- [alien-taxonomy.md](./alien-taxonomy.md) — the stream pattern these passes follow.

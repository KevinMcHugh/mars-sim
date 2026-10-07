# Species and behaviors (toward an ECS)

> Part of the [mars-sim documentation](./README.md).

## What it is

**In progress: phases 1–4 of 7 are built** (see [Migration](#migration)).
A plan to stop hand-writing one turn function per creature and
instead define each creature (cat, rat, chicken, each alien species) as a
**species**: a set of attributes plus an ordered list of reusable
**behaviors** drawn from a shared library. It borrows the useful idea from an
entity-component system — behavior comes from what an entity *has*, not from a
type switch — without adopting an ECS library, an archetype store, or a
modeling language for definitions. The phases below are written so each one
can land on its own without changing behavior. Cats, rats and chickens already
run on behavior ladders, rat breeding and pets are components, and every
rolled alien species is a species value with a ladder its temperament picks.
Only colonists still have a turn of their own.

## Source

Built so far:

- [`internal/sim/species.go`](../internal/sim/species.go) — `Species`, `kindIdentity` (the Config-free half: name, noun, body, spawn site), `newSpeciesTable` (stats and ladders from `Config`), `World.speciesOf`.
- [`internal/sim/behaviors.go`](../internal/sim/behaviors.go) — the `behavior` interface, `animalTurn`, and the rungs: `hunt` with its `preyFinder`s, `flee`, `forage` with its `foodSource`s, `breed`, `stayNearTrough`, `dormant`, `grazeScum`, `wander`.
- `newAlienSpeciesTable` and `World.buildAlienSpecies` (in `species.go`) — one species per rolled alien species.
- [`internal/sim/components.go`](../internal/sim/components.go) — `Breeding` and `PetBond`, with the `keeperOf` / `petTrough` accessors.
- [`internal/sim/species_test.go`](../internal/sim/species_test.go) — the table's and the components' invariants.

Still to move (phase 5):

- [`internal/sim/systems.go`](../internal/sim/systems.go) — the kind-keyed queries (`nearestOfKind*`, `nearestReachablePrey`) and the prey finders built on them.
- [`internal/sim/lore.go`](../internal/sim/lore.go) — `AlienSpecies`, the per-seed roster whose temperament already picks between hunting and grazing.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — the per-kind counters in the frame's stats.

## How it works

### What an ECS is, and the part we want

In an entity-component system an **entity** is just an ID, a **component** is
a plain data struct attached to it (`Position`, `Hunger`, `Breeds`), and a
**system** is a function that runs over every entity carrying a given set of
components. A "rat" stops being a type and becomes whatever has `Hunger +
Scavenges + FleesCats + Breeds + Wanders`. Engines like Bevy and flecs also
store components in packed per-archetype arrays for cache speed.

[entities-and-ai.md](./entities-and-ai.md) records that mars-sim chose "one
struct, not an ECS" on purpose, with the plan that fields would graduate into
components "as behavior multiplies." This doc is that graduation plan. We want
**composition**: a creature is a list of behaviors and the data those
behaviors need. We do not want the storage model (see
[Why it is this way](#why-it-is-this-way)).

### Where we started

Before phase 2 each non-colonist kind was a hand-written priority ladder
(`catTurn`, `ratTurn`, `chickenTurn`, `alienTurn`). Laid side by side, they
were mostly the same rungs in different orders:

| Rung | Cat | Rat | Chicken | Alien (Hostile / Friendly, Cautious) |
| --- | --- | --- | --- | --- |
| starve and die | — | ✓ | ✓ | — (hunger rises, never fatal) |
| give birth when due | — | ✓ | — | — |
| pace by cooldown | `CatSlowness` | — | `ChickenSlowness` | `AlienSlowness` |
| flee a kind | — | cats within `RatFleeRadius` | — | — (Cautious reacts inside a radius) |
| hunt a kind | rats, anywhere | — | — | colonists, rats, other species, same room |
| eat when hungry | — | scavenge, then pod | trough, then graze scum | graze scum |
| breed | — | adjacent opposite sex | — | — |
| stay near home | — | — | trough within `ChickenRoam` | — |
| wander | ✓ | ✓ | ✓ | ✓ |

The kind identity leaked into roughly two dozen `switch e.Kind` /
`e.Kind == X` sites: stats in `newEntity`, nouns and labels in `perception.go`,
spawning in `engine.go`, prey names, and checks like `canBreed`'s
`e.Kind == Rat`. Phase 1 folded the identity and stat switches into the
species table; the behavior checks go in phases 3 and 5.

The `Entity` struct carries every kind's fields at once: colonist economy and
mind state, chicken `keeper`/`trough`, rat `sex`/`pregnant`/`dueTick`, alien
`Species`. Each kind leaves the others' fields zero, and nothing but comments
says which fields belong to whom.

### The model: three layers

| Layer | What it is | Where it comes from | Example |
| --- | --- | --- | --- |
| **Behavior** | a parameterized piece of logic | Go, always | `hunt`, `flee`, `forage`, `breed`, `wander` |
| **Species** | attributes + an ordered behavior list | a Go table, a generator (the alien roster), later maybe a data file | "rat: HP 3, hunger rise 4, [flee(cat), forage(…), breed(…), wander]" |
| **Entity** | one individual's state | the simulation, at runtime | rat #412, female, pregnant, due tick 9001 |

**Behaviors are the only place logic lives.** A behavior is a Go function
with a fixed signature and its own parameters, which reports whether it took
this entity's tick:

```go
// A behavior either acts for e this tick and reports true (the ladder stops),
// or declines and reports false (the next rung runs).
type behavior interface {
	act(w *World, e *Entity) bool
}

type hunt struct{ find preyFinder; catch func(w *World, hunter, prey *Entity); rest int }
type flee struct{ from Kind; radius int }
type forage struct{ sources []foodSource }  // tried in order once hungry
type breed struct{}                         // tryMate; litter size etc. still in Config
type stayNearTrough struct{ roam int }      // a chicken
type dormant struct{}                       // an alien in an undiscovered cave shuffles about
type grazeScum struct{}                     // a peaceful alien eats cave scum (alienGraze)
type wander struct{}                        // always acts: every ladder ends here
```

`hunt` is shared by the cat and the hunting aliens: what differs is the
**finder** (`preyAnywhere(Rat)` for a cat; `nearestReachablePrey` for a
Hostile alien; `colonistWithin(alien-cautious-radius)` for a Cautious one)
and the **catch** (`pounce`, `strike`). The proposal wrote it as
`hunt{prey: tags, scope}`; a finder function is what the three hunters
actually needed, and phase 5's tags will be a way to build finders. `flee`
still takes a `Kind`.
`forage` first lets a foraging job already under way (`JobScavenge`, `JobUse`)
run on, hungry or not, exactly as `ratTurn` did; only then does it check the
food drive and try its sources. A `foodSource` is a plain
`func(*World, *Entity) bool`, so the chicken's existing `chickenFeed` and
`chickenGraze` plug in as method expressions (`(*World).chickenFeed`)
unchanged.

**A species is a value, not a type.** It holds the display identity and
stats that used to be spread across `switch` statements, and its ladder:

```go
type Species struct {
	Kind       Kind
	Name       string    // "rat": Kind.String, and "rat #12" labels
	Noun       NounID    // perception grammar noun; "" for chickens
	Body       bool      // per-part HP (hasParts)
	Spawn      spawnSite // floor, ship, or cavern (Engine.spawn)
	HP         int
	HungerRate int       // food drive base rate; 0: no hunger
	Starves    bool      // a full food drive kills it, leaving Corpse
	Corpse     ItemKind
	Paced      bool      // acts every Slowness ticks, counting down Cooldown
	Slowness   int
	ladder     []behavior
}
```

The table is indexed by `Kind`. `kindIdentity` is the Config-free half
(name, noun, body, spawn site), readable without a World, so `Kind.String` and
`Entity.hasParts` use it. `newSpeciesTable(cfg)` copies it and fills in stats
and ladders from `Config`; `newWorld` keeps the result in `World.species`.
The creatures, as built (every number is an existing `Config` tunable, so
nothing moved out of `mars-sim.yaml`):

```go
cat.ladder     = {hunt{prey: Rat, rest: CatPounceRest}, wander{}}
rat.ladder     = {flee{from: Cat, radius: RatFleeRadius},
                  forage{forageScavenge, foragePod}, breed{}, wander{}}
chicken.ladder = {forage{(*World).chickenFeed, (*World).chickenGraze},
                  stayNearTrough{roam: ChickenRoam}, wander{}}
```

**One turn function runs every species.** The steps every animal shares
(starvation, gestation, pacing) run first; then the ladder runs top to bottom
until a rung acts:

```go
func (w *World) animalTurn(e *Entity) {
	sp := w.speciesOf(e)
	if sp.Starves { /* apply drives; if dead: corpse, log, remove, return */ }
	if e.pregnant && w.tick >= e.dueTick { w.giveBirth(e) }
	if sp.Paced {
		if e.Cooldown > 0 { e.Cooldown--; return }
		e.Cooldown = sp.Slowness - 1 // a rung may lengthen it (hunt's rest)
	}
	for _, b := range sp.ladder {
		if b.act(w, e) { return }
	}
}
```

`Paced` is its own flag, not "`Slowness > 0`": rats were never paced, and a
cat configured with `cat-slowness: 0` must still honor its pounce rest. The
Cooldown is set *before* the ladder so a rung can override it, which is what
the old cat and chicken turns did between them. The `switch e.Kind` in `step`
is now colonist or `animalTurn`.

**Aliens.** `newAlienSpeciesTable` turns each rolled `AlienSpecies` into a
`Species`: the Alien kind's identity and HP, the rolled species' pace, and a
ladder by temperament:

```go
Hostile:  {dormant{}, hunt{find: nearestReachablePrey, catch: strike, rest: BiteRest}, wander{}}
Cautious: {dormant{}, hunt{find: colonistWithin(radius), catch: strike, rest: BiteRest},
           grazeScum{}, wander{}}
Friendly: {dormant{}, grazeScum{}, wander{}}
```

`World.alienKinds` holds them, `speciesOf` routes an alien there by
`Entity.Species` (its roster index, as before), and like `World.species` it
is derived and not saved: `newWorld` and `afterLoad` rebuild it from the
roster. Values the roster holds and combat reads when it happens (damage,
attack modes, a graze's bite rest) are still read from the roster then;
only pace and the ladder are fixed into the species. Anything that edits the
roster after the world is built must call `buildAlienSpecies` (the tests
that force a temperament do, through `setAlienTemperament`).

### Components: optional data, attached when needed

A behavior that needs per-entity state gets it from a **component**: a small
struct held by a nil-able pointer on `Entity`. Nil means absent, and "does
this entity have X" replaces "is this entity kind K":

```go
type Entity struct {
	// ... identity, position, HP, drives, job, path: shared by everyone ...
	breeding *Breeding // sex, pregnant, dueTick, mateReadyTick
	pet      *PetBond  // keeper, and a chicken's trough
	// later: mind *Mind for the colonist-only bulk, if it is ever worth it
}
```

`canBreed` is `e.breeding != nil && …`, not `e.Kind == Rat && …`. A species
with `Breeds` set attaches a `Breeding` (and rolls its sex, at the same point
in the spawn as before) whenever one is spawned, a newborn pup included; a
test holds `Breeds` and the `breed` rung together. Ship landing attaches a
`PetBond` to the cat or hen a colonist brings; a stray has none. Events can
attach them later too: a stray cat adopted by a colonist would gain a
`PetBond`; a mutation already changes anatomy per individual through
`MaxParts`.

**The trough is held on both sides.** The proposal first put the trough
wholly in `PetBond`, but the keeper colonist needs it too: it is the trough
the keeper fills (`jobTend`), and the keeper has it before its hen is wired
up at landing. So the colonist keeps `Entity.trough`/`hasTrough` (its side:
where to deliver feed) and the hen's `PetBond` holds its own copy (where to
eat). A trough that moves (`fixturemove.go`) or is torn down
(`letGoFixture`) updates both, exactly as the shared fields were updated
before.

Components live on the entity, not in side tables keyed by `EntityID`. A
`map[EntityID]*Breeding` is a map iteration waiting to decide a tie (see
[determinism.md](./determinism.md)); a field never is.

### Relationships: rules versus edges

Most game logic is relationships, and they come in two kinds that this design
keeps apart.

**Between categories, relationships are rules.** Cat hunts rat; rat flees
cat; a Hostile alien hunts colonists, rats, and other species; cats and
chickens ignore each other. These belong in behavior parameters, expressed
over **tags** rather than kinds: `hunt{prey: rat}` matches any species tagged
`rat`, and "chickens are not prey to cats" is simply the absence of a `hunt`
that matches them. The one-line `chickens.md` rule ("cats and chickens ignore
each other") stops being an invariant the code has to keep and becomes
something you can read off the table. The query helpers `nearestOfKind` and
`nearestReachablePrey` take a tag set instead of a `Kind` and keep their
distance-then-ID tie break.

**Within or across individuals, relationships are edges.** A pet's keeper, a
hunter's `Quarry`, a talker's `partner`, a colonist's family tree and
affinities, a rat's mate. These are created and broken at runtime and cannot
be declared anywhere ahead of time; a species declares only the *rule* that
creates them ("breeds with same species, opposite sex, adjacent"). They stay
what they are today: typed `EntityID` fields, inside the component that owns
them (`PetBond.keeper`, `Breeding`). flecs-style relationship pairs (`(Eats,
Rat)`, `(ChildOf, e42)`) are the place to go if a generic edge store is ever
needed, but nothing yet asks for queries across arbitrary edge types, and
the colonist family tree already has its own structure in `relationships.go`
(see [ages-and-family.md](./ages-and-family.md)).

### Entities created at runtime

Species are fixed once the world is built; entities come and go. That makes
runtime creation simple:

- **Spawn** takes a species and builds the entity from it: stats, components,
  nothing per-kind in `newEntity`.
- **Litters** inherit the mother's species.
- **Ship arrivals** (a colonist's cat or chicken) spawn the species and attach
  a `PetBond` to the owner.
- **Alien species are generated, not declared.** Each `AlienSpecies` the
  roster rolls produces a `Species` whose ladder its temperament picks (see
  **Aliens** above; built in phase 4). This is why a species must be a Go
  value and not only a file: the most varied creatures in the game are made
  by a generator at worldgen.
- **Individual drift** (a mutated limb) stays on the entity. A species is the
  default; the entity is the truth.

### Alien lifecycles (proposed)

*Not built; recorded here because it is the strongest reason for the species
model.* Today every alien species has one body. The idea is that some species
**change over a life**: an egg that hatches into a larva, a pupa, an imago; a
queen with workers and drones; a cocoon; a joey that grows into a bull or a
betty. The tone is the platypus: plausible parts assembled slightly wrong.
Mammal words on things that hatch, insect words on things with fur, a stage
that should not exist between two that should.

**Rules for generating a lifecycle** (all rolled at worldgen with the roster,
never authored per species):

- **Most species have one form.** A lifecycle is a notable thing a seed rolls
  for one or two species, not the norm. Something like 60% single form, 25%
  two, 12% three, 3% four or more is the starting point, weighted in `Config`.
- **Stages are not Earth's.** The vocabulary borrows Earth words (egg, grub,
  nymph, pupa, cocoon, imago, joey, puggle, bull, betty, queen, drone) but a
  species picks and orders them freely: a joey can pupate; an egg can be the
  *middle* stage; a cocoon can hatch something smaller than went in, as long
  as the life as a whole grows (below). Stage names come from a
  condition-gated pool like `alien-names.yaml`, so a furred species gets
  mammal words and a shelled one gets insect words, and occasionally the
  opposite, which is where the unsettling part comes from.
- **They grow, and stay recognizable.** Each stage is a fraction of the
  species' adult size range (the existing `HeightMin/MaxCM`, `WeightMin/MaxKG`
  become the final stage's), rising through the life. Color, hide, pattern
  and eye arrangement carry through every stage, so a colonist who has seen
  the adult recognizes the grub.
- **Change is additive and pushes further.** A later stage keeps everything
  an earlier one had and may gain or intensify a feature: more limbs or eyes,
  a tail, a shell, claws. "More extreme" means *more of the species' own
  idea*, not more aggressive: a species whose larva has one nub of a horn has
  an adult with a crown of them; a faintly spotted nymph becomes a densely
  spotted imago. Temperament is the species', not the stage's (but see
  behaviors below: an egg does not hunt). Think evolution lines in Pokémon,
  generated once per seed instead of designed.
- **Castes and sexes are branches, not stages.** A lifecycle is a sequence
  that may end in a fork: the last stage splits into weighted forms (queen /
  worker / drone, bull / betty). Branches share the line's appearance and
  differ in size and features: the queen is the most extreme form, the drone
  may be the least.

**New anatomy worth adding now,** because lifecycles need features with
degrees to grow along, and Earth has good ones: **stinger**, **spines or
quills**, **horns** (a count), **antlers** (tines), a **tail ornament**
(none, club, spiked club, stinger), **shell or carapace** (patch, plates,
full), and **claws**. Each is a small level (0 = absent) so a line can go
"one little horn → three horns → nine," "tail → clubbed tail → spiked club."
Some imply an attack mode beside today's bite, claw, tail and strangle (a
sting, a gore); some are only description until something uses them.
Single-form species roll these too, so the features are not lifecycle-only.

**How it fits the model.** Each stage (and each caste) is its own `Species`
value, generated from the `AlienSpecies` line: its own size, features, attack
modes, damage, pace, and **ladder**. An egg or cocoon has no ladder at all
(it lies there, and can be found, guarded, or smashed); a larva might only
`forage{scum}` and `flee`; the adult runs the temperament's ladder. An
individual carries a small `*Lifecycle` component (which line, which stage,
the tick it advances); growing up is swapping which species entry it points
at. Aliens are spawned in nests at worldgen today and never reproduce, so the
first version needs only aging and a mix of stages in a nest; a queen laying
eggs is a later step.

**Determinism.** Lifecycles roll on their own seeded stream, the way the
taxonomy does (`alienTaxonomySeed`), so adding them re-rolls no existing
roster's names, builds or temperaments. Which stage an individual spawns at,
and its exact advance tick, are gameplay draws from `World.rng` at spawn,
like which species it is. Stage features that change combat (an attack mode,
damage from size) are simulation, never `World.prng` flavor.

### Migration

Each phase is behavior-preserving and checkable against the lockstep test in
[determinism.md](./determinism.md): same seed, same `worldFingerprint`, before
and after. That means **every RNG draw happens in the same order as today** —
the ladder is a refactor of the existing ladders, not a redesign of them.

1. **Species table for identity.** *Done.* A `[numKinds]Species` table
   holding stats, nouns, body and spawn site; `newEntity`, `Kind.String`,
   `nounForKind`, `hasParts`, `factRef`, `preyName`, and `Engine.spawn` read
   it. No behavior moved. One visible fix fell out: `displayName` gave an
   unnamed cat or rat the fallback "colonist #12"; it now says "rat #12".
2. **Behaviors for cat, rat, chicken.** *Done.* Each ladder rung lifted out
   of `catTurn`/`ratTurn`/`chickenTurn` into a behavior; all three run
   through `animalTurn`, and the three turn functions are gone.
3. **Components.** *Done.* Rat breeding is a `*Breeding` and a pet's keeper
   (and a hen's trough) a `*PetBond`; `canBreed` checks the component. Two
   kind checks stay on purpose: landing asks whether a pet is a chicken to
   wire its trough, and the frame's per-kind counters are the wire format's.
   Save files from before phase 3 no longer load (the layout fingerprint
   changed with `Entity`'s fields), as with any field change.
4. **Aliens.** *Done.* One `Species` per rolled `AlienSpecies`
   (`World.alienKinds`), `alienTurn` replaced by temperament ladders run
   through `animalTurn`, dormancy as the first rung. `Entity.Species` keeps
   its meaning (an index into the roster) rather than becoming an index
   into one combined table; `speciesOf` does the routing, which left
   `alienSpeciesFor` and everything reading the roster untouched.
4b. **Alien lifecycles.** Roll lifecycle lines and the new anatomy features
   with the roster (on their own stream), generate a `Species` per stage and
   caste, add the `*Lifecycle` component and aging, and teach the lore tab
   and narration to describe a line. This changes the game, so it is checked
   by its own tests, not by fingerprint. See
   [Alien lifecycles](#alien-lifecycles-proposed).
5. **Tags for prey and threat.** Replace kind arguments to the nearest-entity
   queries with tag sets. After this, `Kind` is only what the wire format and
   UI use to pick a sprite.
6. *(Optional, changes behavior.)* Run animals through the colonist focus
   arbitration (scored candidates, commitment, switch margin; see
   [cascading_wsts_architecture.md](./cascading_wsts_architecture.md)) instead
   of a strict ladder, so a starving rat can out-weigh a distant cat. This
   needs tuning and will change fingerprints, so it is its own decision.
7. *(Optional.)* Load species ladders from a data file. Only once phases 2–5
   have settled the behavior vocabulary, and only as composition: the file
   names behaviors and their parameters; it never carries conditions.

Colonists are out of scope. They are already one large, well-factored mind
(focus arbitration, jobs, the economy), and the gain from splitting them is
small. In this model they are simply a species whose one behavior is
`colonistTurn`.

## Why it is this way

- **The table is indexed by `Kind`, not pointed to from `Entity`.** A
  `*Species` on each entity was the obvious design, but tests and
  `recruit.go` build `&Entity{Kind: Colonist, ...}` literals that would carry
  a nil pointer. `Kind` is always set. Phase 4 can route aliens through
  `speciesOf` to a per-species entry without touching `Entity`.
- **The species table is not saved.** It is derived from the Config, and its
  ladders are interface values the save codec has no names for. It is tagged
  `save:"-"` and `newWorld` rebuilds it from the loaded Config (see
  [save-load.md](./save-load.md)). That also keeps it out of the save layout
  fingerprint, so files from before phase 1 still load.
- **How phases 1–4 were checked.** The lockstep test runs two worlds in one
  process, so it cannot see a refactor that changes behavior the same way in
  both. Instead, a throwaway test hashed the full `worldFingerprint` (plus each
  animal's cooldown, quarry, food drive, pregnancy and keeper) every tick
  over ten seeded runs, on `origin/main` and on the branch; the hashes matched.
  Lesson for the next phase: the `testConfig` runs alone proved nothing about
  rats, because their cats ate every rat within a few hundred ticks, so no rat
  ever foraged or raided a pod. Cat-free rat scenarios (on both `testConfig`
  and `DefaultConfig`) were needed, and the check was only trusted once
  swapping the rat's `breed` and `forage` rungs visibly changed the hashes
  (rat populations doubled). Phase 3 moved fields, so the test read
  breeding and pet state through a small helper per version (old fields on
  `HEAD`, components on the branch). A first mismatch turned out to be the
  helper, not the sim: it printed `Entity.trough` for hens, which the old
  layout set and the new one leaves in `PetBond`. Compare like with like
  before believing a diff. Phase 4 added alien-heavy runs (five species,
  60% nests), and, because no seed tried rolled a Friendly species, runs that
  force temperaments onto the roster through a per-version helper; the
  check was trusted once moving the Cautious `grazeScum` rung above its
  `hunt` changed the hashes.

- **No modeling language.** We considered YAML growth, CUE, TypeScript as a
  definition language, and embedded scripting (Starlark, Lua). The
  interesting problem is which behaviors exist and how they attach to
  entities; the format a species is written in is secondary. Go already gives
  types, refactoring, and a debugger, and the creature count is small. If
  data files come later (phase 7), they compose Go behaviors and stay free of
  conditions — the moment one needs an `if`, that is a new behavior in Go.
  `alien-names.yaml`'s `when` trees show how quickly YAML starts growing
  logic once it is allowed to.
- **No ECS library or archetype storage.** Entities are few next to tiles,
  and the cost centers are pathfinding and colonist decisions
  (see [spatial-index-and-performance.md](./spatial-index-and-performance.md)),
  not iterating entity fields. A library would move iteration order into code
  we do not control, and archetype stores reorder entities when components
  are added or removed — exactly the kind of order that must never decide a
  tie. It would also grow the WASM build ([wasm.md](./wasm.md)).
- **A ladder before scoring.** The current creatures *are* ladders, so a
  ladder lets phases 2–4 be pure refactors verified by fingerprint. Scored
  arbitration for animals is a gameplay change and is kept separate (phase 6)
  so a regression is never ambiguous between "the refactor broke it" and "the
  new AI chose differently."
- **Pointer components, not side tables.** Fields keep determinism trivial and
  keep an entity's state in one place for snapshots and save/load.
- **Tags, not kinds, in relationships.** Matching on kind is what made
  "cats ignore chickens" a rule someone had to remember. Matching on tags
  makes a new species fit into the food web by declaring what it is.
- **Lifecycles are generated, mostly absent, and additive.** Authoring
  stages per species would cap the variety at what someone wrote and make
  every seed's aliens familiar. Making most species single-form keeps a
  lifecycle a discovery. Keeping each stage a superset of the last is what
  lets a colonist (and a player) recognize a grub as the young of the thing
  that killed someone, which matters more than biological plausibility.
- **Species as values.** The alien roster already generates species from a
  seed. A design where species come only from files would leave the aliens
  out.

## Extending it

After phase 2, adding a creature is a species entry plus, at most, one new
behavior. A goat that grazes scum, flees aliens, and stays near its pen is
`[flee{from: alien}, forage{scum}, stayNear{pen}, wander]` — all existing
rungs.

Invariants any change must keep:

- A behavior's queries tie-break by distance then ID; none may iterate a map
  to pick a target.
- Ladder order is part of a species' definition: reordering rungs is a
  gameplay change, and in phases 1–5 a fingerprint change means a bug.
- Flavor randomness stays on `World.prng`, simulation randomness on
  `World.rng` ([personality.md](./personality.md)); moving code into a
  behavior must not move its draws between streams.
- New tunables a behavior needs go in `sim.Config` like any other
  ([configuration.md](./configuration.md)).

## Related

- [entities-and-ai.md](./entities-and-ai.md) — the current entity model and each creature's ladder, which this proposal refactors.
- [chickens.md](./chickens.md) — the trough, keeper, and the cat/chicken rule that tags replace.
- [lore.md](./lore.md) — the alien species roster that becomes a species generator.
- [determinism.md](./determinism.md) — the lockstep test each migration phase is checked against.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — the focus arbitration animals could adopt in phase 6.

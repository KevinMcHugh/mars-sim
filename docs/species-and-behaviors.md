# Species and behaviors (toward an ECS)

> Part of the [mars-sim documentation](./README.md).

## What it is

**Proposal.** A plan to stop hand-writing one turn function per creature and
instead define each creature (cat, rat, chicken, each alien species) as a
**species**: a set of attributes plus an ordered list of reusable
**behaviors** drawn from a shared library. It borrows the useful idea from an
entity-component system — behavior comes from what an entity *has*, not from a
type switch — without adopting an ECS library, an archetype store, or a
modeling language for definitions. Nothing here is built yet; the phases
below are written so each one can land on its own without changing behavior.

## Source

Where the things this proposal replaces live today:

- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Kind`, the one `Entity` struct every kind shares, and `newEntity`'s per-kind stats switch.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — the `switch e.Kind` turn dispatch in `step`, `catTurn`, `ratTurn`, `tryMate`, and the shared primitives (`travelTo`, `fleeStep`, `wanderStep`, `nearestOfKind*`, `nearestReachablePrey`).
- [`internal/sim/chickens.go`](../internal/sim/chickens.go) — `chickenTurn`, `chickenFeed`, `chickenGraze`.
- [`internal/sim/lore.go`](../internal/sim/lore.go) — `AlienSpecies`, the per-seed roster whose temperament already picks between hunting and grazing.
- [`internal/sim/perception.go`](../internal/sim/perception.go), [`snapshot.go`](../internal/sim/snapshot.go), [`engine.go`](../internal/sim/engine.go) — the other `switch e.Kind` sites (nouns, labels, glyphs, spawning).

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

### Where we are

Each non-colonist kind is a hand-written priority ladder. Laid side by side,
they are mostly the same rungs in different orders:

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

The kind identity then leaks into roughly two dozen `switch e.Kind` /
`e.Kind == X` sites: stats in `newEntity`, nouns and labels in `perception.go`,
glyphs in `snapshot.go`, spawning in `engine.go`, prey names, and checks like
`canBreed`'s `e.Kind == Rat`. Adding a goat today means touching all of them.

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

type flee struct{ from Tags; radius int }
type hunt struct{ prey Tags; scope huntScope; rest int } // scope: anywhere, same room
type forage struct{ sources []foodSource }               // tried in order
type breed struct{ litter, gestation, mature int }
type stayNear struct{ anchor anchorKind; roam int }      // chicken → its trough
type wander struct{}
```

**A species is a value, not a type.** It holds the stats that are now spread
across `newEntity` and `Config`, its display identity, its tags, and its
ladder:

```go
type Species struct {
	Name, Noun string
	Tags       Tags      // what others' behaviors match on: prey, pest, pet, alien...
	HP         int
	HungerRise int       // 0: no food need
	Starves    bool
	Slowness   int       // turn pacing; 0 acts every tick
	Body       bool      // per-part HP (hasParts)
	Ladder     []behavior
}
```

Expressed this way, today's creatures are (parameters elided; every value
comes from the existing `Config` tunables, so nothing moves out of
`mars-sim.yaml`):

```go
cat     = Species{Tags: pet,      Ladder: {hunt{prey: rat, scope: anywhere}, wander{}}}
rat     = Species{Tags: pest|prey, Starves: true,
                  Ladder: {flee{from: cat}, forage{scavenge, pod}, breed{…}, wander{}}}
chicken = Species{Tags: pet,      Starves: true,
                  Ladder: {forage{trough, scum}, stayNear{trough}, wander{}}}
```

**One turn function runs every species.** The steps every animal shares
(starvation, gestation, cooldown) run first; then the ladder runs top to
bottom until a rung acts:

```go
func (w *World) animalTurn(e *Entity) {
	sp := w.species[e.Species]
	if sp.Starves && w.starved(e) { return }   // corpse, log, remove
	if b := e.breeding; b != nil && b.due(w.tick) { w.giveBirth(e) }
	if e.Cooldown > 0 { e.Cooldown--; return }
	for _, b := range sp.Ladder {
		if b.act(w, e) { break }
	}
	e.Cooldown = sp.Slowness - 1
}
```

The `switch e.Kind` in `step` collapses to "colonist, or animal."

### Components: optional data, attached when needed

A behavior that needs per-entity state gets it from a **component**: a small
struct held by a nil-able pointer on `Entity`. Nil means absent, and "does
this entity have X" replaces "is this entity kind K":

```go
type Entity struct {
	// ... identity, position, HP, needs, job, path: shared by everyone ...
	breeding *Breeding // sex, pregnant, dueTick, mateReadyTick
	pet      *PetBond  // keeper, trough, hasTrough
	// later: mind *Mind for the colonist-only bulk, if it is ever worth it
}
```

`canBreed` becomes `e.breeding != nil && …`, not `e.Kind == Rat && …`. A
species attaches its components at spawn (`breed` in the ladder implies a
`Breeding`). Events can attach them later too: a stray cat adopted by a
colonist gains a `PetBond`; a mutation already changes anatomy per individual
through `MaxParts`.

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
  roster rolls produces a `Species` whose ladder its temperament picks:
  Hostile `[hunt{prey: colonist|rat|alien-not-mine, scope: room}, wander]`,
  Friendly `[forage{scum}, wander]`, Cautious `[react{radius}, forage{scum},
  wander]`. This is why a species must be a Go value and not only a file:
  the most varied creatures in the game are made by a generator at worldgen.
- **Individual drift** (a mutated limb) stays on the entity. A species is the
  default; the entity is the truth.

### Migration

Each phase is behavior-preserving and checkable against the lockstep test in
[determinism.md](./determinism.md): same seed, same `worldFingerprint`, before
and after. That means **every RNG draw happens in the same order as today** —
the ladder is a refactor of the existing ladders, not a redesign of them.

1. **Species table for identity.** A `[numKinds]Species` table holding stats,
   nouns, labels, and glyph keys; `newEntity`, `Kind.String`, `nounForKind`,
   `factRef`, snapshot glyphs, and `Engine.spawn` read it. No behavior moves.
2. **Behaviors for cat, rat, chicken.** Lift each ladder rung out of
   `catTurn`/`ratTurn`/`chickenTurn` into a behavior and run all three through
   `animalTurn`. Delete the three turn functions.
3. **Components.** Move rat breeding into `*Breeding` and the pet fields into
   `*PetBond`; replace the remaining `Kind == Rat/Chicken` checks with
   component checks.
4. **Aliens.** Give `Entity.Species` the meaning "index into `World.species`"
   for every animal, generate one `Species` per rolled `AlienSpecies`, and
   move `alienTurn` onto the ladder (strike, graze, the Cautious reaction,
   dormancy as a pre-step).
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

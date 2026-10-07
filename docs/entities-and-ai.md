# Entities & AI

> Part of the [mars-sim documentation](./README.md).

## What it is

Every actor in the world — colonist, alien, cat, rat, chicken — is one `Entity` struct
interpreted by its `Kind`. Each tick, `World.step()` runs a per-kind behavior
function for every living entity. This doc covers the entity model, the tick and
turn order, the movement primitives, and each creature's behavior.

## Source

- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Kind`, `State`, `JobKind`, the `Entity` struct, `newEntity`.
- [`internal/sim/species.go`](../internal/sim/species.go) — the species table: each kind's name, noun, body, spawn site, stats, and (cats, rats, chickens) behavior ladder.
- [`internal/sim/behaviors.go`](../internal/sim/behaviors.go) — `animalTurn` and the behavior rungs ladders are built from (`hunt`, `flee`, `forage`, `breed`, `stayNearTrough`, `wander`).
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `step`, turn order, the colonist and alien turns, movement primitives, queries.
- [`internal/sim/config.go`](../internal/sim/config.go) — the per-creature stat tunables.

## How it works

### One struct, not an ECS

Rather than a strict entity-component system, mars-sim uses a single `Entity`
struct whose fields the systems interpret according to `Kind`. This is a
deliberate scaffold choice: it keeps the code readable while systems are few, and
fields can graduate into real components later as behavior multiplies. Colonists
use the most fields (drives, personality, inventory, a job, a cached path); other
kinds leave the irrelevant ones zero.

That graduation has started: each kind is now a species value (`species.go`),
and cats, rats and chickens are built from shared behavior rungs rather than
hand-written turns. See [species-and-behaviors.md](./species-and-behaviors.md)
for the plan and how far it has got.

The five kinds:

| Kind | Moves on | Eats | Flees | Notes |
| --- | --- | --- | --- | --- |
| **Colonist** | Floor | (food drive) | aliens | mines, builds, tends drives; has personality + inventory |
| **Alien** | Floor | colonists, rats, other species' aliens (Hostile); cave scum (Friendly, Cautious) | — | the antagonist; a Hostile species hunts the nearest prey it can reach |
| **Cat** | Floor | rats | — | no drives; hunts by instinct |
| **Rat** | Floor | (food drive) | cats (not aliens, which also eat them) | reuses the colonist food drive; scavenges bodies, gore, and scum, else raids pods; never builds |
| **Chicken** | Floor | feed from its keeper's trough, else cave scum | — | lands in a keeper's ship; cats ignore it; see [chickens.md](./chickens.md) |

`State` (idle, moving, mining, building, eating, relieving, fleeing, hunting,
feeding, fighting, cleaning, hauling, storing, demolishing) is a **display projection**
derived from behavior each tick and surfaced in the UI. A colonist's
`FocusKind` is its weighted, persistent goal (work, a particular drive, fight,
flee, or idle), while `JobKind` is the concrete execution step beneath that
goal. A hungry colonist building a nutrient pod therefore remains focused on
eating while its job is building.

> The top-level README predates cats and rats; this doc is the current reference
> for the creature roster.

### The tick: `step()`

`World.step()` increments the tick, then for each entity in turn order runs
`colonistTurn`, or, for every other creature, `animalTurn`, which runs its
species' behavior ladder (see
[species-and-behaviors.md](./species-and-behaviors.md)). The dead are removed the
moment they are eaten or starve, so liveness is re-checked as the loop proceeds.
After all entities act, `step` folds in the tick's terrain changes:
`refreshSpatial` (regions/rooms), `pruneProjects`, `planFacilities` (on a
cadence), and `rebuildBuildTiles`.

### Turn order is deterministic — and hunger-first for colonists

`entityTurnOrder` sorts entities so that **colonists act before other kinds**, and
among colonists the **hungriest acts first**; ties and all non-colonists fall back
to ascending ID. This is a scheduling fix, not just cosmetics: in a full facility
room, acting first gives the hungriest colonist first claim on the access tile a
neighbor vacated last tick, so a fixed low-ID colonist cannot repeatedly win
reopened facility access and let others starve. Sorting is stable and fully
ordered, so runs stay deterministic.

### Colonist behavior (`colonistTurn`)

Each turn first applies starvation, observations, and uranium exposure. Focus
arbitration generates a fixed set of cheap candidates and scores each from
configured base, drive pressure, visible stimuli, commitment, and distance
contributions. The current eligible focus receives a commitment bonus, and a
challenger must beat it by the configured switch margin. Ties are deterministic.
Candidate scoring never claims a target or runs A*; only the winning focus
invokes the existing job executors.

Arbitration is cached while the current focus remains eligible and no score can
change. Need phase crossings, stimulus insertion/expiry, threat edges, affect
changes, job teardown, and focus transitions invalidate the cache immediately;
a bounded deadline is the backstop. Need pressure or affect that can drift still
reconsiders every tick, rather than relying on an unsafe estimated horizon. The
selected job executor continues every tick even when arbitration is skipped.
Resting idle colonists and in-place sleepers additionally bypass the full
observation/executor machinery when no nearby threat, rat, or gore exists,
while retaining per-tick starvation, uranium, fatal-drive, and facility
checks. A custom persistent perception wakes them only when it sets
`interrupt_rest`. Threat presence is still alien-only: `seesThreat` and
`nearestAlien` answer the same question, so fight/flee cannot target a
rat or a configured non-alien sighting.

Execution order and invariants:

1. **Drive consequences** — `applyDriveConsequences` (starvation, and passing out, soiling and loneliness); if it just died, release its job
   claims and remove it. A colonist that has passed out (`stayPassedOut`)
   spends the rest of its turn unconscious.
1a. **Uranium dose** — `applyUraniumExposure` (right after the sighting pass,
   before anything below can return): a colonist beside a uranium deposit or
   carrying uranium ore accumulates exposure whatever else it is doing, and a
   full dose rolls for a mutation. See [mutation.md](./mutation.md).
2. **Weighted focus arbitration** — visible aliens enable flee and, when armed,
   fight; pressing drives enable their matching focus; valid work and idle
   provide the ordinary alternatives. A visible alien's starting stimulus
   weight dominates even critical hunger. A colonist already fleeing keeps
   flee eligible until the alien is past `FleeRadius+FleeReleaseMargin`
   (`focusThreat`), so it doesn't flicker flee/relieve at the radius edge.
   Fight still needs the alien inside `FleeRadius`. See
   [cascading_wsts_architecture.md](./cascading_wsts_architecture.md#flee-hysteresis).
3. **Drive-focus execution** — a selected drive focus may interrupt the current task,
   unless the task already serves that drive: a live conversation (social) or a
   matching `JobUse`/`JobBuild` runs on rather than restarting. If a facility of
   the right kind is reachable, switch to `JobUse` and follow its flow field. Otherwise help with **reachable** facility construction; only
   a *fatal* drive with no reachable life-support under construction justifies a
   lone emergency build. If all reachable project tasks are claimed, wait (step
   aside if idling would block) rather than wandering off and losing your place.
4. **Continue the focus's current job** if one is set (`runJob`).
5. **Look for work** (`assignWorkJob`) only after work or idle execution wins:
   a material load that blocks mining goes
   to reachable storage first, or helps build storage if none is usable.
   Otherwise claim the nearest reachable construction task, clean up refuse,
   or mine the frontier. See [storage.md](./storage.md) and
   [sanitation.md](./sanitation.md).
6. **Rest** — if there was no work and no pressing drive, an idle colonist rests
   (skips arbitration, observation with no nearby event, and the work search)
   until `wakeTick`, so an established colony with nothing to do stops rescanning
   the entity set and map every tick. A colonist never rests where it
   would block others (`idleWouldBlock`): parked on a facility access tile or a
   pending build tile, it `stepAside`s instead.

Work jobs:

- **`jobMine`** — two strategies (see [pathfinding.md](./pathfinding.md) and
  [construction.md](./construction.md)): big colonies/maps follow the shared
  frontier flow field and claim a rock on arrival; small ones claim a specific
  rock up front and A\* to it. Mining awards `RawRock` to the inventory *before*
  changing terrain, so a full inventory can never make mined material vanish; a
  colonist that cannot carry more will not start mining.
- **`jobBuild`** — travel to a floor tile, then raise the structure over
  `buildTicks` (scaled by the colonist's `workScale` trait). Yields to whoever is
  standing on the build tile for a few ticks before giving up.
- **`jobUse`** — follow the facility flow field, stand adjacent, use it for
  `UseTicks`, then reset the drive. This covers eating, relieving, and sleeping;
  sleep is non-fatal, so no bunk means waiting rather than emergency building.
- **`jobClean`** — scrub gore and bodies off a tile (`cleanGather`), then carry
  the load to an incinerator and burn it (`cleanHaul`). Only offered when an
  incinerator is reachable, so refuse is never picked up with nowhere to put it.
  See [sanitation.md](./sanitation.md).
- **`jobStore`** — carry a work-blocking load of raw rock, iron ore, water ice,
  uranium ore, and clay to the nearest reachable chest that can fit it all, then
  transfer atomically. Weapons remain equipped and refuse remains on its
  incinerator route. See [storage.md](./storage.md).

`FocusEscape`/`jobDemolish` sit outside the drive/work/threat groupings above: a
colonist whose room has been cut off from the colony's main network for long
enough breaks the nearest wall back down to Floor, regardless of what else it
was doing — a detect-and-correct backstop for a room sealed shut by
construction elsewhere, excluded whenever a threat is visible so it never
competes with fleeing or fighting. See [escape.md](./escape.md).

### Alien behavior (ladder by temperament: `dormant`, `hunt`, `grazeScum`, `wander`)

Each rolled alien species is its own species value (`newAlienSpeciesTable`),
whose ladder its temperament picks; an alien runs its own species' ladder
through `animalTurn`. A species with a lifecycle has a species value per
form: its eggs and cocoons lie inert, its young graze, and only its adults
hunt (see [alien-lifecycles.md](./alien-lifecycles.md)). Aliens are paced by a `Cooldown` (their species'
slowness, scaled from `AlienSlowness`). Each active turn: find
the nearest prey in the alien's own room (`preyInRoom(hostilePrey)`: a colonist,
a rat, or an alien of another species); if adjacent, `strike` (one of its species' attack modes — bite, claws, tail,
or strangle — lands `AlienDamage` on a random body part, or half of it on the head for a strangle — see [combat.md](./combat.md) — eating
the prey if the wound is fatal, then rests `AlienBiteRest`); otherwise `travelTo` it over the floor with cached A\*,
exactly as a cat or colonist would. That is a Hostile species; a Friendly one
never hunts and a Cautious one only reacts inside `alien-cautious-radius` (see
[lore.md](./lore.md#temperament-whether-a-species-fights-at-all)). A Friendly
or Cautious alien with no colonist to react to grazes cave scum when hungry
(`alienGraze`). With nothing else to do, aliens wander. An
alien hunts the same way whether or not its target is armed; the only
difference a weapon makes is whether the colonist stands and shoots back
instead of running.

Aliens follow the same movement rules as everyone else: floor only, never
through rock, walls or facilities. A wall is a real defense, and a colonist
sealed in another room is safe from them. Aliens used to burrow through any
terrain; that was dropped so they play by the colony's rules.

Most aliens spawn on hidden cavern floor (`alienSpawnSite`). An alien on
undiscovered floor is **dormant**: it shuffles around its cave, unseen by
colonists, until a dig breaks in. See [caverns.md](./caverns.md#aliens-in-the-caves).

### Cat behavior (ladder: `hunt` rats, `wander`)

Cats have no drives — they hunt rats by instinct, paced by `CatSlowness`. They
travel the floor with cached A\* and `pounce`
when adjacent (a single pounce is fatal to a rat), then rest `CatPounceRest`. If
a rat is walled off or the cat is wedged, it prowls (`wanderStep`) instead of
freezing.

Cats arrive as one of a colonist's three possible rare items (see
[ships.md](./ships.md)): a cat steps out of its owner's ship with a
`PetBond` naming the owner, which so far is only shown, never acted on. The
`cats` setting adds strays at worldgen and is 0 by default. A cat hunts only
rats: chickens are not prey, and a chicken does not flee a cat.

### Rat behavior (ladder: `flee` cats, `forage`, `breed`, `wander`)

Rats reuse the colonists' `DriveFood`, but hunger far faster (`RatHungerRate`)
and **never build**. They eat what the scumhouse eats: a hungry rat
(`nearestScavenge`, `scavenge.go`) heads for the nearest tile within
`rat-scavenge-radius` holding a body (any body, a colonist's included), gore,
or exposed cave scum, and eats a unit there (`JobScavenge`). Only with nothing
in range does it raid a nutrient pod through the generic `JobUse` machinery,
and only while pods feed anyone. Rats claim nothing, so they race cleaners and
scrapers for the same biomatter; every unit a rat eats is a unit the colony
cannot turn into slurry, and cats are what keep them down. Order: starve check
and any due litter (`animalTurn`), then the ladder: flee nearby cats
(`RatFleeRadius`), scavenge or raid when hungry (a foraging job under way runs
on), breed, otherwise scurry.

Rats used to be mice, which only ate at pods and so starved as soon as the
safety net was off. The rename came with the diet.

### Movement primitives

- **`fleeStep`** (colonists, rats) — the walkable step that maximizes Chebyshev
  distance from the threat.
- **`wanderStep`** — a small random step (often stays put so idlers don't jitter);
  floor only, and colonists keep off pending build tiles.
- **`stepAside`** — a BFS off a facility-access or pending-build tile that may pass
  *through* a packed crowd to reach the nearest genuinely clear landing. A plain
  random wander is insufficient in a full room, where there may be no adjacent
  vacancy and a blocked builder or food queue would stall indefinitely.
- **`travelTo` / `followField`** — the two ways to move toward a goal: cached A\*
  route vs. shared flow field. Both let an entity pass through any other entity
  mid-route except an alien — a real, dangerous obstacle, unlike a colonist,
  cat, or rat just standing in the way — but require it to end the tick on a
  free tile. Covered in [pathfinding.md](./pathfinding.md).

### Neighbor queries

Queries ask for **tags**, not kinds (see
[species-and-behaviors.md](./species-and-behaviors.md#relationships-rules-versus-edges)):
`nearestTagged` (and the `nearestAlien` / `nearestVermin` wrappers) expands in
**chunk rings** around the query point and
stops once the next ring cannot beat the best candidate found — so "nearest prey"
is cheap even on a crowded map. Ties break toward the lower ID for determinism.
`forEachInRadius` visits tiles in a square ring, nearest-first, allocation-free,
so it is safe to call per entity per tick. An unbounded search
(`nearestTaggedWhere`, behind a cat's `preyAnywhere` and a Hostile alien's
`preyInRoom`) scans `kindEntities` for each kind whose species can carry the
tags instead, since chunk rings would cross the whole map to prove a miss.

## Why it is this way

- **`Kind`-dispatched single struct** is the right amount of structure for a
  scaffold: an ECS would be premature complexity while there are four kinds and a
  handful of systems.
- **`FocusKind` vs. `JobKind` vs. `State`** separates weighted intent, concrete
  execution, and display. The AI arbitrates focus, executors own jobs and exact
  targets, and the UI reads state plus the snapshot's current focus.
- **Hunger-first turn order** was learned from colonies starving in full rooms;
  fairness of scheduling turned out to matter as much as job selection.
- **Reachability-gated fallbacks** (build your own life support only when nothing
  reachable is already being built) stop the colony from either mobbing one site
  or letting a disconnected colonist starve next to an unreachable project.
- **Cooldown-paced predators** make aliens/cats readable and tunable without a
  separate scheduler.

## Extending it

- **A new creature**: add a `Kind` before `numKinds`, give it stats in `Config`,
  an entry in `kindIdentity` (name, noun, body, spawn site), its stats and
  behavior ladder in `newSpeciesTable`, and a glyph. `step` runs it through
  `animalTurn`. Build the ladder from the existing rungs in `behaviors.go`, and
  add a rung only for behavior none of them has. See
  [species-and-behaviors.md](./species-and-behaviors.md#extending-it).
- **A new job**: add a `JobKind`, an `assign*`/`clearJob` pair that keeps the job
  board's bookkeeping exact, a `job*` executor, and a case in `runJob`.
- **A new display state**: add a `State`, set it in the relevant turn, and map it
  to a glyph in the TUI.

## Related

- [combat.md](./combat.md) — body-part HP, weapons, and how an armed colonist's
  survival priority differs from an unarmed one's.
- [drives.md](./drives.md) — the drives that preempt colonist and rat work.
- [personality.md](./personality.md) — trait-scaled colonist parameters.
- [construction.md](./construction.md) — how build jobs become rooms.
- [escape.md](./escape.md) — breaking out of a room cut off from the colony.
- [pathfinding.md](./pathfinding.md) — how entities actually move.
- [inventory.md](./inventory.md) — what colonists carry.

# Entities & AI

> Part of the [mars-sim documentation](./README.md).

## What it is

Every actor in the world — colonist, alien, cat, mouse — is one `Entity` struct
interpreted by its `Kind`. Each tick, `World.step()` runs a per-kind behavior
function for every living entity. This doc covers the entity model, the tick and
turn order, the movement primitives, and each creature's behavior.

## Source

- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Kind`, `State`, `JobKind`, the `Entity` struct, `newEntity`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `step`, turn order, per-kind turns, movement primitives, queries.
- [`internal/sim/config.go`](../internal/sim/config.go) — the per-creature stat tunables.

## How it works

### One struct, not an ECS

Rather than a strict entity-component system, mars-sim uses a single `Entity`
struct whose fields the systems interpret according to `Kind`. This is a
deliberate scaffold choice: it keeps the code readable while systems are few, and
fields can graduate into real components later as behavior multiplies. Colonists
use the most fields (needs, personality, inventory, a job, a cached path); other
kinds leave the irrelevant ones zero.

The four kinds:

| Kind | Moves on | Eats | Flees | Notes |
| --- | --- | --- | --- | --- |
| **Colonist** | Floor | (needs food) | aliens | mines, builds, tends needs; has personality + inventory |
| **Alien** | anything (burrows) | colonists | — | the antagonist; hunts the nearest colonist |
| **Cat** | Floor | mice | — | no needs; hunts by instinct; cannot burrow |
| **Mouse** | Floor | (needs food) | cats | reuses the colonist food need; raids pods; never builds |

`State` (idle, moving, mining, building, eating, relieving, fleeing, hunting,
feeding, fighting) is a **display projection** derived from behavior each tick
and surfaced in the UI. `JobKind` (none, mine, build, use) is the colonist's committed task and
the single source of truth for its behavior.

> The top-level README predates cats and mice; this doc is the current reference
> for the creature roster.

### The tick: `step()`

`World.step()` increments the tick, then for each entity in turn order runs
`colonistTurn` / `alienTurn` / `catTurn` / `mouseTurn`. The dead are removed the
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

Priority order each tick:

1. **Starvation check** — `applyStarvation`; if it just died, release its job
   claims and remove it.
1a. **Uranium dose** — `applyUraniumExposure` (right after the sighting pass,
   before anything below can return): a colonist beside a uranium deposit or
   carrying uranium ore accumulates exposure whatever else it is doing, and a
   full dose rolls for a mutation. See [mutation.md](./mutation.md).
2. **Survival** — if an alien is within `FleeRadius`: a colonist carrying a
   pistol or shotgun stands its ground and fights (`fightAlien`) instead;
   an unarmed one drops everything and flees, same as always. See
   [combat.md](./combat.md).
3. **Urgent need preemption** — `mostUrgentNeed` may interrupt the current task,
   unless the task already serves that need: a live conversation (social) or a
   matching `JobUse`/`JobBuild` runs on rather than restarting. If a facility of
   the right kind is reachable, switch to `JobUse` and follow its flow field. Otherwise help with **reachable** facility construction; only
   a *fatal* need with no reachable life-support under construction justifies a
   lone emergency build. If all reachable project tasks are claimed, wait (step
   aside if idling would block) rather than wandering off and losing your place.
4. **Continue the current job** if one is set (`runJob`).
5. **Look for work** (`assignWorkJob`): claim the nearest reachable construction
   task first, else mine the frontier.
6. **Rest** — if there was no work and no pressing need, an idle colonist rests
   (skips the work search) until `wakeTick`, so an established colony with nothing
   to do stops rescanning the map every tick. A colonist never rests where it
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
  `UseTicks`, then reset the need. This covers eating, relieving, and sleeping;
  sleep is non-fatal, so no bunk means waiting rather than emergency building.

### Alien behavior (`alienTurn`)

Aliens are paced by a `Cooldown` (from `AlienSlowness`). Each active turn: find
the nearest colonist anywhere on the map; if adjacent, `bite` (lands
`AlienDamage` on a random body part — see [combat.md](./combat.md) — eating
the colonist if the wound is fatal, then rests `AlienBiteRest`); otherwise
`burrowStep` toward it through any terrain. With no colonists left, they
wander. An alien hunts the same way whether or not its target is armed; the
only difference a weapon makes is whether the colonist stands and shoots back
instead of running.

### Cat behavior (`catTurn`)

Cats have no needs — they hunt mice by instinct, paced by `CatSlowness`. Unlike
aliens they **cannot burrow**: they travel the floor with cached A\* and `pounce`
when adjacent (a single pounce is fatal to a mouse), then rest `CatPounceRest`. If
a mouse is walled off or the cat is wedged, it prowls (`wanderStep`) instead of
freezing.

### Mouse behavior (`mouseTurn`)

Mice reuse the colonists' `NeedFood` and the generic `JobUse` machinery, but
hunger far faster (`MouseHungerRise`) and **never build** — they depend on pods the
colony has already raised and starve if none is reachable. Order: starve check,
flee nearby cats (`MouseFleeRadius`), head to a nutrient pod when hungry,
otherwise scurry.

### Movement primitives

- **`burrowStep`** (aliens) — greedy step toward a destination through any terrain,
  avoiding occupied tiles.
- **`fleeStep`** (colonists, mice) — the walkable step that maximizes Chebyshev
  distance from the threat.
- **`wanderStep`** — a small random step (often stays put so idlers don't jitter);
  only aliens may enter non-floor, and colonists keep off pending build tiles.
- **`stepAside`** — a BFS off a facility-access or pending-build tile that may pass
  *through* a packed crowd to reach the nearest genuinely clear landing. A plain
  random wander is insufficient in a full room, where there may be no adjacent
  vacancy and a blocked builder or food queue would stall indefinitely.
- **`travelTo` / `followField`** — the two ways to move toward a goal: cached A\*
  route vs. shared flow field. Both let a colonist pass through other colonists
  mid-route but require it to end the tick on a free tile. Covered in
  [pathfinding.md](./pathfinding.md).

### Neighbor queries

`nearestOfKind` (and its `nearestColonist`/`nearestAlien`/`nearestCat`/
`nearestMouse` wrappers) expands in **chunk rings** around the query point and
stops once the next ring cannot beat the best candidate found — so "nearest prey"
is cheap even on a crowded map. Ties break toward the lower ID for determinism.
`forEachInRadius` visits tiles in a square ring, nearest-first, allocation-free,
so it is safe to call per entity per tick.

## Why it is this way

- **`Kind`-dispatched single struct** is the right amount of structure for a
  scaffold: an ECS would be premature complexity while there are four kinds and a
  handful of systems.
- **`State` vs. `JobKind`** keeps display concerns from leaking into behavior;
  the UI reads a label, the AI reads the job.
- **Hunger-first turn order** was learned from colonies starving in full rooms;
  fairness of scheduling turned out to matter as much as job selection.
- **Reachability-gated fallbacks** (build your own life support only when nothing
  reachable is already being built) stop the colony from either mobbing one site
  or letting a disconnected colonist starve next to an unreachable project.
- **Cooldown-paced predators** make aliens/cats readable and tunable without a
  separate scheduler.

## Extending it

- **A new creature**: add a `Kind` before `numKinds`, give it stats in `Config`,
  a spawn case in `Engine.spawn` and `worldgen`, a `String()` and glyph, and a
  `<kind>Turn` in `systems.go` dispatched from `step`. Reuse `nearestOfKind`,
  the movement primitives, and (if it has drives) the needs machinery.
- **A new job**: add a `JobKind`, an `assign*`/`clearJob` pair that keeps the job
  board's bookkeeping exact, a `job*` executor, and a case in `runJob`.
- **A new display state**: add a `State`, set it in the relevant turn, and map it
  to a glyph in the TUI.

## Related

- [combat.md](./combat.md) — body-part HP, weapons, and how an armed colonist's
  survival priority differs from an unarmed one's.
- [needs.md](./needs.md) — the drives that preempt colonist and mouse work.
- [personality.md](./personality.md) — trait-scaled colonist parameters.
- [construction.md](./construction.md) — how build jobs become rooms.
- [pathfinding.md](./pathfinding.md) — how entities actually move.
- [inventory.md](./inventory.md) — what colonists carry.

# Sanitation: cleaning, corpses, and the trash room

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonies make a mess. A violent death splatters gore across a tile, and a death
nothing eats leaves a body lying where it fell. Sanitation is the loop that
deals with both: a colonist with nothing urgent to do **scrubs the refuse up
and hauls it away** — **biomatter** (viscera, and every body but a
colonist's) to a scumhouse to become food, and a **colonist's body** to an
incinerator to be burned. The colony builds itself a **trash room** to hold
that incinerator once there is a mess to burn. See
[scumhouse.md](./scumhouse.md) for what the scumhouse does with its share.

## Source

- [`internal/sim/cleaning.go`](../internal/sim/cleaning.go) — the whole cleaning
  job: `tryAssignClean`, `nearestRefuse`, `jobCleanGather`, `gatherRefuse`,
  `jobCleanHaul`, `incinerate`.
- [`internal/sim/world.go`](../internal/sim/world.go) — the `Incinerator`
  terrain, `refuseCell` (bodies counted by kind), `addCorpse`/`takeCorpse`/
  `corpsesOfAt`/`takeGore`/`clearRefuse`, `refuseAt`/`refuseTotal`, and
  `trackFacility`.
- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) —
  `deliverBiomatter`, `biomatterStacks`, `carriedOwner`.
- [`internal/sim/project.go`](../internal/sim/project.go) — the `trashRoom`
  recipe and its slot in `planRooms`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `JobClean`,
  `cleanStage`, and the `Cleaning`/`Hauling` display states.
- [`internal/sim/inventory.go`](../internal/sim/inventory.go) — the `Viscera`,
  `ColonistCorpse`, `AlienCorpse` and `AnimalCorpse` item kinds, `isRefuse`,
  `isBiomatter`, `RemoveAll`, `Count`.
- [`internal/sim/jobboard.go`](../internal/sim/jobboard.go) — `claimClean` /
  `releaseClean`, so two colonists never walk at the same splatter.
- [`internal/sim/cleaning_test.go`](../internal/sim/cleaning_test.go) — the
  tests that pin all of the above, including the end-to-end
  `TestColonyBuildsATrashRoomAndBurnsItsRefuse`.

## How it works

### Refuse is tile state

Two fields on `Tile` hold everything there is to clean:

| Field | Left by | Glyph |
| --- | --- | --- |
| `Gore` | any messy kill: an alien's bite, a gunshot, a boot (capped at `maxGore`) | 🩸 |
| `Corpses` | a death nothing ate: starvation, a gunned-down alien, a stomped mouse | 🦴 |

Behind `Tile.Corpses` (a total, for display) the refuse index counts bodies
**by kind** — `ColonistCorpse`, `AlienCorpse`, `AnimalCorpse` — because the
kind decides where one goes. Every `addCorpse` call names it: starvation leaves
a colonist's body (or, for a mouse, an animal's), `shoot` an alien's, `stomp` an
animal's. Gore is one undifferentiated count: viscera is viscera, whoever it
came from.

`refuseAt(p)` is their sum, and `refuseTotal()` is the map-wide running total,
maintained incrementally by `addGore`/`addCorpse`/`takeGore`/`takeCorpse` so the
planner can ask "is the colony dirty?" without walking the grid.

Which deaths leave a body is a decision at each call site, not a rule derived
from the cause string: `bite` (an alien devouring a colonist) and `pounce` (a
cat swallowing a mouse) leave only gore, because the remains were eaten. `shoot`,
`stomp`, and starvation call `addCorpse` as well.

### The cleaning job

`JobClean` is one job with two stages (`cleanStage`):

1. **`cleanGather`** — walk to the claimed refuse tile (standing on it or beside
   it both count) and scrub for `CleanTicks`. `gatherRefuse` then moves as much
   of the tile's refuse as fits into the colonist's inventory, bodies (as their
   own item kinds) before `Viscera`. Biomatter is gathered as community work,
   so its **cargo record** marks it the colony's (see
   [property.md](./property.md)).
2. **`cleanHaul`** — carry the load where it goes (`haulTarget`): to a
   scumhouse that can take all the biomatter carried, if any is carried, and
   there `deliverBiomatter` puts it in the depot on the colony's account; then,
   with whatever is left (a colonist's body, or biomatter no scumhouse had room
   for), to an incinerator, where `IncinerateTicks` later `incinerate` destroys
   the whole load at once.

Where refuse can go decides what gets picked up (`refuseDestinations`):

| Reachable | What a cleaner gathers |
| --- | --- |
| an incinerator | everything, as before |
| only a scumhouse with room | biomatter only; a colonist's body stays where it lies |
| neither | nothing |

Both legs end in `clearJob`, which releases the tile claim if the colonist was
still on its way to gather.

`assignWorkJob` offers cleaning **after construction and before mining**. That
ordering is the crux of the feature: see *Why it is this way*.

### The trash room

`trashRoom` is an ordinary `roomRecipe` — the same wall-and-doorway shell as a
facility room or a dormitory — whose single facility kind is `Incinerator`.
`planRooms` gets to it last, after life support and bunks, and only when
`refuseTotal() > 0` and the colony has fewer than one incinerator planned or
built. `b` then `t` in the TUI queues one by hand (`OrderTrashRoom`).

The incinerator backs no need, so the loop in `newWorld` that gives each need's
facility a tracked tile set and a shared flow field never reaches it; sanitation
calls `trackFacility(Incinerator)` explicitly. From there haulers find and route
to one exactly the way an eater finds a nutrient pod, `chooseFacility` and all.

## Why it is this way

- **Cleaning is work, not an idle whim.** Stomping a mouse happens in
  `colonistTurn`'s idle branch, and cleaning started there too. It never fired:
  the mining frontier is effectively infinite, so a colonist is never actually
  out of work and the idle branch is only reached when nothing is reachable.
  Refuse would have piled up forever in exactly the colony that generates it.
  Cleaning therefore sits in `assignWorkJob` — but *behind* project
  construction, so it can never delay life support.
- **Nowhere to take it, no cleaning.** A colonist that picked up a body with
  nowhere to take it would just move the mess into an inventory slot, where it
  is invisible, occupies carrying capacity, and is never destroyed.
  `tryAssignClean` gates on a reachable destination for what it would pick up,
  so refuse waits on the floor — visibly, and as the planner's signal to build
  the trash room.
- **A colonist's body is never food.** It goes to the incinerator and nowhere
  else; `isBiomatter` leaves `ColonistCorpse` out, and a scumhouse depot never
  receives one. Viscera, though, is viscera — the colony does not sort a stain
  by whose it was.
- **Biomatter is not burned while a scumhouse can take it.** `haulTarget` tries
  the scumhouse first; only a load no scumhouse has room for goes to the fire.
- **A load in hand is never stranded.** The one exception to that gate: a
  colonist already carrying refuse (its haul was interrupted by an alien, a
  meal, or a route that closed) takes the haul again ahead of any other work,
  the moment an incinerator is reachable.
- **Corpses are tile state, not entities.** [combat.md](./combat.md) predicted a
  real corpse would look "more like a new, non-acting `Kind`". It cannot be: the
  occupancy index (`World.occ`) allows one entity per tile, so a corpse-entity
  would wall off the tile where anything died, break predator targeting, and add
  a kind to every dispatch switch — all to model something that never takes a
  turn. `Tile.Corpses` mirrors `Tile.Gore`, which already worked.
- **Building over refuse clears it; digging does not.** `SetTerrain` wipes a
  tile's refuse when it becomes a structure. Without that, a wall raised over a
  bloodstain leaves refuse that no colonist can ever reach, which both reports a
  mess the player cannot act on and keeps the "colony is dirty" signal stuck on.
  Digging deliberately does *not* clear it, so mining through to an alien shot
  dead inside a rock vein exposes the stain rather than erasing it.
- **One incinerator, not one per N colonists.** Pods and bunks scale with
  headcount because appetite and sleep do. Refuse does not: it comes from
  deaths, which are episodic. A second incinerator would only split the haulers
  and cost a room's worth of digging.
- **Claims, so cleaners spread out.** `jobBoard.cleaning` keys a refuse tile by
  its cleaner, exactly like the mining claim set. Unlike mining there is no
  maintained frontier set — refuse is rare and cleaners only search a radius —
  so the board tracks the claims and nothing else.

## Extending it

- **A new kind of trash** (spoiled rations, alien viscera worth studying) is an
  `ItemKind` plus a line in `isRefuse`, and a source that calls `addCorpse` or
  `addGore` — or a new `Tile` counter alongside them, folded into `refuseAt`.
- **Burning could yield something** — ash, heat, power. `incinerate` is the one
  place a load is destroyed; it already knows the exact counts.
- **Burial instead of burning** would be another room recipe and another
  facility terrain: the haul leg only cares that its target is a terrain kind
  reachable through a tracked facility field.
- **A cleaning skill or a janitor role**: `cleanRadius` already varies per
  colonist (Tidy searches twice as far) and `CleanTicks` runs through
  `scaleTicks(..., e.workScale)`, so both hooks are in place.
- Invariant to preserve: **refuse is never held with nowhere to put it**. Any
  new way to pick refuse up has to keep the "reachable destination first" gate
  and the carrying-colonist fallback, or a load can be stranded in an inventory
  for the rest of the run.

## Related

- [scumhouse.md](./scumhouse.md) — where biomatter goes, and what it becomes.
- [combat.md](./combat.md) — where gore and most corpses come from.
- [construction.md](./construction.md) — the project/recipe machinery the trash
  room is built with.
- [entities-and-ai.md](./entities-and-ai.md) — where `JobClean` sits in the
  colonist's priority order.
- [inventory.md](./inventory.md) — the stacks refuse is carried in.
- [memories.md](./memories.md) — `EvtCleanedRefuse` / `EvtIncineratedRefuse` and
  the Tidy colonist's relief at a clean colony.
- [configuration.md](./configuration.md) — `CleanRadius`, `CleanTicks`,
  `IncinerateTicks`, `IncineratorBuildTicks` and their flags.

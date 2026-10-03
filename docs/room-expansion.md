# Rooms: fit-outs, mergers and growth

> Part of the [mars-sim documentation](./README.md).

## What it is

A room is a walled rectangle with doorways in its walls, holding fixtures of
one zone: residence, storage or production (see [zoning.md](./zoning.md); a
fixture carries the zone, and a chest goes by its role). When the colony
wants more fixtures, it puts them into the rooms it already has before it
marks out a new one:

1. **Fit-out.** Free floor in a room of the fixture's zone, wherever the
   layout rule lets it stand. An incubator goes into a kitchen; a toilet into
   a dormitory.
2. **Merger.** Two rooms of the zone that stand side by side, back to back or
   facing each other become one. The walls between them come down, and every
   doorway stays.
3. **Growth.** A room moves one of its walls out, any of the four. A doorway
   in that wall moves out with it.
4. **A new room**, from a recipe, only when none of these places anything.
   The planner won't mark one out while a room of the zone is still going up,
   and the narrow fallback is for the colony's first room of a kind only.

The colony commissions all of it like any public work, from the treasury, and
tearing a wall down has its own wage. Fixtures can also be moved: see
*Moving fixtures* below.

## Source

- [`internal/sim/roomplan.go`](../internal/sim/roomplan.go): the room record,
  the layout rule (`roomLayout.fits`, `place`), reshaping (`roomShape`,
  `shapeWork`, `designateShape`), and the planner (`improveRooms`, `fitOut`,
  `mergeBox`/`mergeRooms`, `growRoom`, `placeFixtures`, `roomGoingUp`,
  `tidyRooms`).
- [`internal/sim/project.go`](../internal/sim/project.go): recipes (now only a
  new room's layout), `recipeUnits`, `facilityUnits`, `designateRoom`
  (`newRoomRecord`), and `planRooms`, which asks `placeFixtures` for
  everything.
- [`internal/sim/roomframe.go`](../internal/sim/roomframe.go):
  `footprintKeepsColonyWhole`, the split check for any rectangle.
- [`internal/sim/structures.go`](../internal/sim/structures.go):
  `growStructure`, `absorbStructure` (in roomplan.go), `structureName`.
- [`internal/sim/systems.go`](../internal/sim/systems.go): `jobBuild`'s
  demolition branch, shared with passages and clearing.
- [`internal/sim/roomplan_test.go`](../internal/sim/roomplan_test.go): the
  layout rule, fitting an incubator into a kitchen, mergers back to back and
  facing opposite ways, growing through a doorway, a kitchen's pair, staying
  out of other rooms, the planner using what it has, and colonists building
  a merger.
- [`internal/sim/bench_test.go`](../internal/sim/bench_test.go):
  `BenchmarkExpandNoFit`, the search that finds nothing.

## How it works

### The record

`designateRoom` lays a new room out from a recipe in a `roomFrame` (see
[construction.md](./construction.md)) and records it as a `roomRecord`: its
inside (`lo`..`hi`), its doorways (the gaps in its walls), its zone, its
issuer and its structure. After that nothing about the room is a recipe or a
frame: no back wall, no bay, no facing. A fit-out, a merger or growth
changes the rectangle and the doorways, and that is all. `w.roomFloor`
indexes every inside tile and doorway, so "whose floor is this?" is one map
lookup.

### The layout rule

`roomLayout.fits` decides where a fixture may stand in a room of any shape:

- inside, free, and not the tile just inside a doorway;
- no other fixture, standing or planned, within a step of it, diagonals
  included, the spacing a bay always had (a fixture is used from the tiles
  round it, and two side by side leave one unreachable);
- every doorway still reaches every other across the free floor, flooded
  orthogonally (the stricter test);
- it keeps the free tiles it needs round it reachable from the doorways, one,
  or two for a stove, a chest, a forge, a gun bench or an incubator (things
  worked at for long stretches: see *Aisles* in
  [construction.md](./construction.md)), and no fixture near it drops below
  what it needs.

`place` tries tiles farthest from the doorways first, then against a wall,
then row-major, so a new room's back row fills first, the floor by a doorway
stays clear, and a room two rooms deep gets a second row. A pair (a stove and
its pantry, a forge and its gun bench: `roomRecipe.paired`) takes two tiles
two apart in a line, and the pantry is linked to its stove as it is marked
out.

### Reshaping

Growing a room and joining two are one operation: rooms become one new
rectangle that covers them all (`roomShape`). `shapeWork` walks the new
footprint, inside and walls:

- **A wall of the old rooms' that ends up inside comes down.** It must
  stand whole, be no project's, and be no third room's.
- **Ground the room takes in** must be discovered floor or rock to dig, in no
  other room, on no other room's doorway step, zoned for the room, and no
  project's. Where a new wall goes, a wall already standing is shared (a party
  wall). A new wall is never raised against one standing or planned, the
  double wall (see [construction.md](./construction.md)).
- **Doorways.** A doorway still on the new walls stays. One whose wall moved
  out moves out with it, if there is open floor outside its new place. Its
  old step is let go from `w.doorTiles` and the new one reserved. At least one
  must survive.

Then `footprintKeepsColonyWhole` runs on the new footprint, unless it is the
old one (a fit-out). `designateShape` builds one project in phases:

| Phase | Work |
| --- | --- |
| `roomDemolishPhase` (-2) | tear down the walls that end up inside: dig tasks with `clears: Wall`, paid `wage-demolish` |
| `roomDigPhase` (-1) | dig out rock in the new ground |
| `roomWallPhase` (0) | raise the new walls |
| `roomFitPhase` (1) | build the fixtures |

The oldest room lives on. The others are dropped from `w.roomRecords`, their
floor re-indexed, their structures folded into its (`absorbStructure`, so a
party wall the two shared is listed once). The record changes when the work
is marked out, not when it is done, the way planned fixtures count as soon
as they are designated, so demand converges instead of re-triggering every
planning cycle.

### The planner

Every demand in `planRooms` goes through `placeFixtures(recipe, units,
wait)`: bunks, pods and toilets, chests, a later stove with its pantry,
incubators, chairs, the incinerator, a forge with its gun bench, and every
player order (a recipe's `fullBay` worth). `improveRooms` tries, among the
colony's own rooms of the units' zone that nothing is building:

1. `fitOut`: the oldest room with free floor for at least one unit.
2. `mergeRooms`: two rooms whose rectangles lie side by side or back to back
   (`mergeBox`), overlapping across the gap, at most `maxMergeLane` (3) tiles
   of ground between their walls, and no more ground to fill out to the
   rectangle than the larger room already holds. The joined room must take
   at least one unit.
3. `growRoom`: the oldest room that can grow. It tries walls with no doorway
   first, each lot east, west, south, north, and grows by as few rows as fit
   everything it can, or else places what fits at the furthest it can grow.

Each places as many units as fit, never more than `room-max-facilities`
fixtures in a room, and the planner asks again next cycle for the rest. A
room a colonist commissioned (a house, a chef's kitchen) is never touched.
When none of these places anything, `placeFixtures` falls back to a new room
from the recipe, with two rules:

- **Wait for a room going up** (`roomGoingUp`). While one of the colony's
  rooms of the zone is being built or reshaped, no new one is marked out:
  once it stands, it can take them. Storage, life support and player orders
  don't wait.
- **The narrow fallback is for the colony's first.** A recipe with an aisle
  falls back to a narrow room only while no fixture of its first kind is
  planned; later ones set `aisleRequired`. Storage keeps the fallback (every
  ship locker is a chest, so its "first" never comes, and a full inventory
  needs a chest anywhere).

The colony's first kitchen is always a new room, unpaid if it must be, as
life support has always been. With nothing else to build, `planRooms` ends
with `tidyRooms`, which joins rooms even when they gain no fixture.

`room-expansion` turns fit-outs and growth off, and `room-merge` mergers.

### Moving fixtures

*(Phase 3 of issue #128; see below for when it lands.)*

## Why it is this way

- **The honeycomb.** A colony walled itself into a stack of one-tile-wide
  rooms: incubators and kitchens, each one fixture behind its own doorway,
  side walls shared in a column (issue #128). Instrumenting every moment a
  new room was marked out showed two causes. The room that could have grown
  was still going up, and later aisled rooms took the narrow fallback into
  the gap between two walls, where they could never grow either. Waiting and
  keeping the narrow fallback for the first took 24 seeds (20 colonists,
  200×200, 20,000 ticks, aliens off) from 38 narrow kitchens and incubators
  to none, with the same survival.
- **Fixtures, not recipes, decide what goes where.** That first fix added a
  merger for two rooms of a kind side by side, facing the same way. It never
  fired: the same-kind neighbours that do occur are back to back across a
  shared back wall, or side by side facing opposite ways, and a room record
  that was one bay along one back wall could not describe their union. Nor
  could anything put an incubator into a kitchen with floor to spare: both are
  production, but each room was its recipe. So the record became a rectangle
  with doorways, the zone moved to the fixtures, and the recipes shrank to the
  layout of a new room.
- **One reshape.** Growth used to mean moving the side wall along the bay;
  joining was a second, narrower routine. Covering rooms with a new rectangle
  does both, in any direction, with one set of rules, and growing through a
  wall with a doorway in it is just a doorway that moves.
- **Demolition first.** The old walls come down before anything else, so new
  ground is reached from inside the room. Raising the new walls first would
  enclose the space between them and the old, with no way in and possibly a
  builder inside, which is what [escape.md](./escape.md) exists to clean up.
  Digging first would need the strip to be reachable from outside, which a
  room in a rock niche never is.
- **Records, not rediscovery.** Inferring rooms from terrain (find a wall
  rectangle) would be fragile around party walls and ships' hulls. Recording
  them is a slice append and keeps everything deterministic (records are
  iterated in order, never as a map).
- **A layout rule, not a bay.** A bay's spacing worked for one row along one
  wall. Any shape needed a rule a fixture can be checked against wherever it
  goes: spacing, clear doorways, the floor joining up, and access. Two floods
  of a room's inside per candidate is cheap at room sizes, and a room that is
  already at `room-max-facilities` is turned down by counting its fixtures,
  before any layout is built.
- **Partial placement.** A fit-out places what fits and the planner comes
  back for the rest. Holding out for a room with space for the whole demand
  is what marked out new rooms beside old ones with floor to spare.
- **No lanes or approach required for a reshape.** A new room needs them so
  its outer wall tasks can be reached before its inside exists. A reshaped
  room's new ground is reached from inside from the start.
- **Player orders use the rooms too.** Ordering "a dormitory" asks for a
  dormitory's worth of bunks. The orders never controlled placement anyway:
  the planner has always chosen the site.

### Measured

24 seeds, 20 colonists, 200×200, 20,000 ticks, aliens off, against the first
fix (PR #130) on the same `main`:

| | first fix | rooms by fixture |
| --- | --- | --- |
| rooms | 412 | 362 |
| fixtures | 1928 | 1931 |
| fit-outs / expansions / new rooms | – / 126 / 412 | 92 / 72 / 362 |
| alive / starved | 480 / 0 | 480 / 0 |
| ms per tick | 0.151 | 0.158 |

Mergers: none. What the colony now marks out new is, per seed, its first room
of each kind (the first kitchen, the hall, the silo, the first incubators)
and a kitchen once every production room holds `room-max-facilities`
fixtures. Over 6 seeds, 51 of 83 new rooms were colonists' houses, which are
private homes by design. Production rooms hold stoves with their pantries,
incubators, and the forge and gun bench together, in two rows along their
walls.

`BenchmarkExpandNoFit` (800 full storage rooms wall to wall, merging off)
is 0.10 ms, as before. Counting a room's fixtures before laying it out is
what keeps it there: building every room's layout first cost 3.9 ms.

### History

- **Kitchens, incubators and halls grew first as their own kind.** On a
  30-colonist colony (seed 1790962337151000000, 10000×10000) the colony
  wanted ten kitchens, four incubator rooms and three halls, each a new room,
  until those kinds could grow; then it had 11 rooms where it had 22, for the
  same fixtures.
- **It does not cost food.** Over 48 seeds, as many colonists survived with
  expansion as without. More starved with it only in runs with aliens, where
  any change to the rooms reshuffles who is eaten and who starves.
  `TestTheTreasuryOutlastsALongRun` runs without aliens for that reason.
- **A search that finds nothing was quadratic** while "is this tile another
  room's?" tested every room: 4 ms with 800 rooms. `w.roomFloor` made it one
  lookup.

## Extending it

- **A new fixture** needs a zone in `fixtureZones` and, if it is worked at
  for long stretches, `needsAisle`. A fixture that goes with another (as a
  pantry with its stove) is a recipe with `paired`, and its link is made in
  `designateShape`.
- **Shrinking a room**, or splitting one, is not a reshape: `shapeWork`
  assumes the new rectangle covers the old ones.
- **Mergers of three rooms** would need `mergeBox` over more than a pair; the
  reshape itself takes any number.
- **Commissioned rooms** are skipped. Letting a colonist grow its own house
  would mean `improveRooms` taking an issuer, and fixtures inheriting
  ownership as in `jobBuild`.
- **Invariants**: a room's record changes when its work is marked out;
  nothing iterates `roomRecords` other than in order; `roomFloor` holds only
  inside tiles and doorways, which no two rooms share; every room keeps at
  least one doorway.

## Related

- [construction.md](./construction.md): new rooms, siting, sharing walls, and
  the double-wall rule a reshape shares.
- [zoning.md](./zoning.md): fixture zones, and why a room's zone is its
  fixtures'.
- [labor.md](./labor.md): work orders, the treasury, and `wage-demolish`.
- [escape.md](./escape.md): passages, the other place walls get torn down.
- [incubator.md](./incubator.md), [scumhouse.md](./scumhouse.md),
  [meeting-hall.md](./meeting-hall.md), [foundry.md](./foundry.md),
  [storage.md](./storage.md): the fixtures that share rooms.

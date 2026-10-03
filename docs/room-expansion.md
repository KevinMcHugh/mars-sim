# Room expansion and merging

> Part of the [mars-sim documentation](./README.md).

## What it is

When the colony wants more of a room's fixtures (bunks, storage containers,
kitchens, incubators, meeting-hall chairs), it first tries to **grow a room it
already has**: it tears down one of the room's side walls,
raises a new one further out, and fits the new fixtures in the space between.
Two rooms of the same kind standing side by side are **joined** instead:
the wall between them comes down and new fixtures go where it stood. Only
when no room of that kind can grow or join does it mark out a new one, and
not while one is still going up. The colony commissions the work like any
public work, from the treasury, and tearing a wall down has its own wage.

## Source

- [`internal/sim/roomgrow.go`](../internal/sim/roomgrow.go) — `roomRecord`, `growOrPlan` (and its `roomGoingUp` wait), `roomPlanState`, `expandRoom`, `expansionClear`, `designateExpansion`.
- [`internal/sim/roommerge.go`](../internal/sim/roommerge.go) — joining two rooms: `besideRoom`, `mergeRooms`, `mergerClear`, `designateMerger`, `absorbStructure`, and `tidyRooms`.
- [`internal/sim/project.go`](../internal/sim/project.go) — `roomRecipe.expands` and `fullBay`, `roomDemolishPhase`, where `designateRoom` records each room, and the `planRooms` call sites.
- [`internal/sim/hall.go`](../internal/sim/hall.go) — `chairsShort`, the hall's shortfall in chairs.
- [`internal/sim/workorder.go`](../internal/sim/workorder.go) — `taskWage`: a wall torn down pays `wage-demolish`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `jobBuild`'s demolition branch, shared with passages.
- [`internal/sim/bench_test.go`](../internal/sim/bench_test.go) — `BenchmarkExpandNoFit`, the worst case for a search that finds nothing.
- [`internal/sim/roomgrow_test.go`](../internal/sim/roomgrow_test.go) — growing right and left, a storage room's aisle, a kitchen's stove-and-pantry pair, no double walls, staying out of other rooms, the planner's preference and orders for every kind, and colonists building one.
- [`internal/sim/roommerge_test.go`](../internal/sim/roommerge_test.go) — joining flush rooms and rooms across a lane, a kitchen's pair, what does not line up, tidying, colonists building a merger, and the planner waiting for a room going up.

## How it works

### Which rooms grow

A recipe with `expands` set grows: the **dormitory**, **storage room**,
**kitchen** (scumhouse), **incubator** and **meeting hall**. A room grows by
whole cycles of its recipe's `kinds`, so the new end of the bay repeats the
old. For every one but the kitchen that is one fixture at a time. A kitchen's
bay is a stove and its pantry, so it grows a pair at a time: the stove where
the old side wall stood, its pantry two tiles on, linked by `linkPantry` as a
new kitchen's are. A kitchen whose bay is not whole pairs (a narrow one-stove
kitchen from a cramped cavern) does not grow, since its next fixture would be a
pantry for nobody. Life-support rooms (pods and toilets), the trash room and
the foundry do not expand: the colony wants one incinerator and one foundry,
and a facility room's alternation is its own demand.

Expansion is tried before a new room (`growOrPlan`) wherever the colony wants
more of those fixtures:

| Trigger | Wants |
| --- | --- |
| The planner short of bunks (`plannedFacilities(Bed) < desired`) | the shortfall |
| A colonist with a full inventory and nowhere to unload (`colonyNeedsStorage`), including the over-cap exception | 1 container |
| The planner short of kitchens, after the first (`plannedColonyKitchens() < desiredScumhouses()`) | one kitchen: a stove and pantry |
| The planner short of incubators (`wantsIncubator`) | the shortfall |
| The planner short of chairs (`chairsShort`) | the shortfall |
| A player's order for any of these rooms (`b` then `d`, `r`, `h`, `i`, `m`) | a new room's worth, `fullBay` (4 bunks or chairs, 1 container, 1 kitchen, 2 incubators) |

Two rooms are always new: the colony's first storage room (its silo, when
there is no market depot) and its first kitchen, which is life support and may
be built unpaid when the treasury can't fund it. Either way there is nothing
to grow yet.

### Not scattering small rooms

Expansion only helps if there is a room that can grow. Two rules in
`growOrPlan` (and the later-kitchen branch of `planRooms`) keep the colony
from marking out rooms that never will:

- **Wait for a room going up.** While one of the colony's rooms of a kind is
  being built or enlarged (`roomGoingUp`), the planner marks out no other room
  of that kind. Once it stands, it grows. Before this, a colony short of three
  incubators marked out one room, then another the next planning cycle
  because the first was busy, then a third.
- **The narrow fallback is for a colony's first.** A recipe with an aisle
  (kitchen, incubator) may fall back to a narrow room in a cramped cavern
  (see [construction.md](./construction.md), *Aisles*), but only for the
  colony's first room of that kind. Later ones set `aisleRequired` and wait
  for a site wide enough, as later kitchens already did. A chef's commissioned
  kitchen always does.

Storage is exempt from both. A full inventory can stall the dig phase of every
project in flight, the storage room going up included, and a storage room
anywhere is what breaks that (see [construction.md](./construction.md)).

### Rooms are recorded

`designateRoom` appends a `roomRecord` to `w.roomRecords`: the recipe the room
was actually built from (a narrow fallback's has no aisle), its `roomFrame`,
its fixture count `n`, and its issuer. Before this, a room was forgotten as
soon as its project was pruned, and nothing could be asked of it afterwards. A
record's frame is the room's **current** extent: an expansion widens it,
growing left moves its origin, and a merger stretches it over both rooms (so
a joined room has two doorways). The doorway does not move, so the record does
not recompute it from the frame (`doorU` is only meaningful when a room is
first laid out); `w.doorTiles` keeps the door step reserved as before.

### Laying out the strip

`expandRoom` walks the records oldest first, skipping rooms of another recipe,
rooms a colonist commissioned (only the colony's own grow), rooms already being
expanded, and rooms at `room-max-facilities`. For each it tries the largest
growth first, `k = min(want, roomFacilities, room-max-facilities - n)` down to
1, on the right and then the left.

Growing by `k` fixtures adds `2k` columns. In frame terms, columns `j` count
outward from the side wall being moved (`j = 0`) to the new side wall
(`j = 2k`):

```
before (dorm, n = 2)    after (k = 2, grown right)
#####                   #########
#B.B#                   #B.B.B.B#
#...#                   #.......#
#...#                   #.......#
##.##                   ##.######
                            01234  <- j
```

- `j = 0`: the old side wall. Its inside rows are torn down; its back and front
  wall tiles stay, as part of the longer back and front walls.
- `j = 1 .. 2k-1`: the new interior, with back and front wall tiles.
- `j = 2k`: the new side wall.
- The new fixtures are at `j = 2i - 1 - bayOffset` for `i = 1..k`, the same
  spacing as the old bay. Without an aisle that puts the first one just past
  where the wall stood; a storage room has an aisle, so its next container
  stands **on** the old wall's tile and the aisle moves out past it.

`expansionClear` checks that strip. The wall being moved must be standing whole
and not another project's. Every tile the room takes in must be discovered
floor or rock to dig, claimed by no project, on no reserved doorway, and (for
floor) inside no other recorded room: a finished room's aisle is ordinary
floor, and nothing else would stop an expansion from taking it. Where a new
wall goes, a wall already standing is shared outright (a party wall). A new
wall tile is never raised against a standing or planned wall, the same rule
new rooms follow (see [construction.md](./construction.md), *No double walls*).
Then `siteKeepsColonyWhole` runs on the grown frame, so a bigger room can't cut
the colony in two any more than a new one can.

Nothing is asked of the ground outside the strip (no lanes, no approach row),
unlike a new room's site: every tile of an expansion is reached from inside
the room, through the gap the old wall leaves.

### Phases and pay

`designateExpansion` builds one project, named "dormitory expansion" or
"storage room expansion", with `room` pointing at the record:

| Phase | Work |
| --- | --- |
| `roomDemolishPhase` (-2) | tear down the old wall's inside rows: dig tasks with `clears: Wall` |
| `roomDigPhase` (-1) | dig out any rock in the strip |
| `roomWallPhase` (0) | raise the new back, front and side walls |
| `roomFitPhase` (1) | build the new fixtures |

It is funded all or nothing by `fundProject`. `taskWage` pays `wage-demolish`
(3, against 2 to raise a wall) for each wall torn down, because the rock is not
salvaged. If the treasury can't pay for `k` fixtures, `expandRoom` tries a
smaller `k`, then the next room, then gives up, and `growOrPlan` falls back to
a new room (which an empty treasury can't fund either). The record is updated
when the expansion is marked out, not when it finishes, the same way
`plannedFacilities` counts designated fixtures, so demand converges instead of
re-triggering every planning cycle.

Builders tear the wall down with the same `jobBuild` branch passages use
(`DemolishTicks` of work, nothing to carry away). The log line says which:
"tears down a wall ... to enlarge the dormitory" when the task belongs to a
room's project.

### Joining two rooms

`expandRoom` first tries `mergeRooms`: two of the colony's rooms of the kind,
standing side by side, become one. `besideRoom` accepts a pair that faces the
same way with their back walls in one row, and at most `maxMergeLane` (3)
columns of ground between their side walls. `d` counts that distance: 0 is one
wall the two share, 1 two walls back to back, more leaves a lane.

```
before (two dorms, a lane)   after (joined, d = 2)
#####.#####                  ###########
#B.B#.#B.B#                  #B.B.B.B.B#
#...#.#...#                  #.........#
#...#.#...#                  #.........#
##.##.##.##                  ##.#####.##
```

Both walls' inside rows are torn down (their back and front tiles stay, as
part of the joined room's walls), the lane is dug and walled at the back and
front, and fixtures go between the two bays at the bay spacing, two past the
left room's last fixture to two short of the right room's first
(`fixtureU`), in whole cycles of the recipe's kinds. Two aisled storage rooms
sharing a wall gain a container on the wall's tile; two narrow dormitories
sharing a wall gain nothing, since their bunks flank it.

`mergerClear` holds the lane to an expansion strip's rules: open discovered
floor or rock, in no third room, on no doorway step, zoned for the room, no
project's, and no new wall raised against an old one. The joined frame must
pass `siteKeepsColonyWhole`, which matters here: the lane may be a route.

Both rooms must be the colony's, idle, whole cycles of the kinds, no more
than `room-max-facilities` together, and built alike, both with an aisle or
both narrow. A room's two ends are laid out from the recipe's `bayOffset`, and
a joined room whose ends disagreed would put its next fixture against the old
one.

The older record survives as the joined room; the other is dropped, its floor
re-indexed and its structure folded into the survivor's (`absorbStructure`),
so a party wall the two shared is listed once. Both doorways stay, and stay
reserved. The project is "dormitory merger" and so on, with an expansion's
phases.

When the colony wants fixtures, a merger has to fit at least one. With nothing
else to build, `planRooms` ends with `tidyRooms`, which joins rooms that fit
none. `room-merge` (on by default) turns both off.

## Why it is this way

- **Demolition first.** The old wall comes down before anything else, so the
  strip is reached from inside the room. Raising the new walls first would
  enclose the space between them and the old wall, with no way in and possibly
  a builder inside, which is what [escape.md](./escape.md) exists to clean up.
  Digging first would need the strip to be reachable from outside, which a room
  in a rock niche never is.
- **Records, not rediscovery.** Inferring rooms from terrain (find a wall
  rectangle with a bay of bunks) would be fragile around party walls and
  ships' hulls. Recording them at designation is one slice append and keeps
  everything deterministic (records are iterated in order, never as a map).
- **Whole cycles of the bay.** Growing a room means its new end repeats its
  old one. A kitchen grows a stove and its pantry together, so every stove
  keeps a pantry of its own (see [scumhouse.md](./scumhouse.md)).
- **Kitchens, incubators and halls too.** The first version grew only
  dormitories and storage rooms. On a 30-colonist colony (seed
  1790962337151000000, 10000×10000) the colony never wanted either, since every
  settler then landed in a crash pod with its own bunk and locker. What it did
  want was ten kitchens, four incubator rooms and three halls, each a new room.
  Back-wall sharing (see [construction.md](./construction.md)) made both side
  walls of a room good backing, so they went up as triptychs: a room with a
  kitchen backed onto each side wall, facing away. With those three kinds
  growing too, and since colonists arrive by ship (see [ships.md](./ships.md)),
  the same run at tick 10,000 has 11 rooms with expansion and 22 without: 5
  kitchen rooms against 11 for the same 22 stoves and pantries, 2 incubator
  rooms against 4 for the same 8 incubators, and one hall of 8 chairs against
  three of 10 chairs between them. All 30 colonists are alive either way. It
  still builds no dormitory: its 17 bunks are well over the 6 it wants.
- **It does not cost food.** Over 48 seeds (20 colonists, 200×200, 30,000
  ticks), as many colonists survived with expansion as without (789 and 788).
  More starved with it (19 against 8), but only in runs with aliens, where any
  change to the rooms reshuffles who is eaten and who starves: one run with
  expansion off lost all 20 colonists to a grelk swarm and so counted none
  starved, where with expansion on 11 lived and 5 starved. With aliens off,
  neither starved any of its 960 colonists. `TestTheTreasuryOutlastsALongRun`
  now runs without aliens for that reason.
- **No lanes or approach required.** A new room needs those so its outer wall
  tasks can be reached before its interior exists. An expansion's interior is
  reachable from the start, so requiring them would only refuse sites, mostly
  the rock niches where growing is most useful.
- **The honeycomb.** A colony would wall itself into a stack of one-tile-wide
  rooms: incubators and kitchens, each one fixture behind its own doorway,
  side walls shared in a column. Over 12 seeds (20 colonists, 200×200,
  20,000 ticks), the moments a new expanding room was marked out showed why.
  The room that could have grown was still going up, or the new room was
  narrow and squeezed between two walls, where it could never grow either.
  Waiting and keeping the narrow fallback for the first took those seeds from
  21 narrow kitchens and incubators to none, 22 one-wide rooms to 11 (the
  trash rooms, one-wide by design) and 145 rooms to 131, with the same
  fixtures. With aliens off, over 24 seeds: 38 narrow kitchens and incubators
  to none, 333 rooms to 259, and 480 colonists alive with none starved,
  either way.
- **Mergers rarely fire, and that is fine.** Once rooms stop being planned
  beside a busy one, two rooms of a kind facing the same way, side by side,
  almost never happen: no merger in those 12 seeds, nor in 40-colonist runs to
  30,000 ticks. The same-kind neighbors that do occur are kitchens back to
  back across a shared back wall, or facing opposite ways, and a joined room
  of those would not be one bay along one back wall, which is all a
  `roomRecord` can describe. Mergers are for the pairs that do line up: a
  player ordering rooms, a ship's rooms, rooms planned before a rule changed.
- **Player orders expand too.** The colony rarely wants a dormitory on its own:
  its ships sleep half their passengers, and `desiredFacilities` wants only one
  bunk per `per-facility` (5) colonists. Many dormitories and storage rooms
  come from the player's `d` and `r` orders, so those had to grow rooms for the
  feature to matter. The orders have no placement control anyway: the planner
  has always chosen the site.

## Performance

Measured on an M1 Pro against `main` at the merge of colony ships (#114, #115).

**Per tick: no consistent change.** The step benchmarks (`Step500`,
`Step2000`, `StepMixed500`, `StepBigMap`, `StepSmallColonyOnHugeMap10000`)
are within noise of `main` (benchstat, 6 runs each, interleaved). Real runs
(30 colonists, 10000×10000, 10,000 ticks) vary by seed in both directions:
on the motivating seed 1790962337151000000 the branch is about 4% slower
(0.168 against 0.162 ms/tick, steady over repeated runs), and on seeds 7, 11,
23 and 42 it is +3%, +4%, −1% and −4%. Room planning is under 2% of a run on
either branch (about 30 ms of 10,000 ticks, 20 ms of it expansion, mostly
`siteKeepsColonyWhole`). The per-seed differences sit in colonist decisions
(threat checks, facility choice): the colony's rooms differ, so its history
differs.

`BenchmarkStepBigColonyOnHugeMap` showed −37%, but that is an artifact: it
steps one world b.N times, so each side was measured at a different stage of
its colony. Over a fixed 300 ticks both take 3.70 ms/tick.

**Site search: +3%.** `BenchmarkFindRoomSiteNoFit` went from 19.4 to 20.0 µs,
the stricter back-wall check in `roomSiteClear` (see
[construction.md](./construction.md), *No double walls*). Running that check
after the interior loop, with a cheap backed-only pre-check first, is what
keeps it that small; checked first, it cost 36%.

**A search that finds nothing: linear in rooms.** `inOtherRoom` first tested
every recorded room for each floor tile in a strip, so when no room could grow
the search was quadratic: 24 µs with 50 rooms, 273 µs with 200, 4.0 ms with
800 storage rooms packed wall to wall, where every strip runs into a
neighbor's aisle. Planning repeats a failed search every `planInterval` (16)
ticks, so a big colony wanting storage it could not place would have paid
about 250 µs a tick for it. `w.roomFloor` indexes each room's floor tiles (its
inside and its doorway, which no two rooms share) instead, and the same
search is 10, 43 and 175 µs (`BenchmarkExpandNoFit`, 800 rooms).

## Extending it

- **Another expandable recipe** needs only `expands: true`, provided its bay
  repeats in whole cycles of `kinds` (`designateExpansion` continues the cycle
  from `n`). If its fixtures are linked to each other, as a kitchen's stove and
  pantry are, link the new ones in `designateExpansion` as it does
  `linkPantry`.
- **Growing deeper rather than wider** (more rows in front of the bay) would
  move the front wall and its doorway, so `w.doorTiles` would need an entry
  removed, and nothing removes entries today. Keep that invariant in mind.
- **Joining rooms that face each other or stand back to back** would need a
  record that is not one bay: two fixture rows, or a doorway in the back wall.
  Expansion and merging both read a room as one bay with a doorway in front,
  so that is a record change first.
- **Joining an aisled room to a narrow one** needs per-end bay offsets on the
  record, in place of the recipe's one `bayOffset`.
- **Commissioned rooms** (houses, chefs' kitchens) are skipped by the issuer
  check. Letting a colonist grow its own house would mean `expandRoom` taking
  an issuer, and fixtures inheriting ownership as in `jobBuild`.

## Related

- [construction.md](./construction.md) — room shells, siting, sharing walls, and the double-wall rule expansion shares.
- [labor.md](./labor.md) — work orders, the treasury, and `wage-demolish`.
- [escape.md](./escape.md) — passages, the other place walls get torn down.
- [incubator.md](./incubator.md), [scumhouse.md](./scumhouse.md) — the rooms whose narrow fallback is now the first one's alone.
- [storage.md](./storage.md) — what a storage room's containers hold.

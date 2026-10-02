# Room expansion

> Part of the [mars-sim documentation](./README.md).

## What it is

When the colony wants more of a room's fixtures (bunks, storage containers,
kitchens, incubators, meeting-hall chairs), it first tries to **grow a room it
already has**: it tears down one of the room's side walls,
raises a new one further out, and fits the new fixtures in the space between.
Only when no room of that kind can grow does it mark out a new one. The colony
commissions the work like any public work, from the treasury, and tearing a
wall down has its own wage.

## Source

- [`internal/sim/roomgrow.go`](../internal/sim/roomgrow.go) — `roomRecord`, `growOrPlan`, `expandRoom`, `expansionClear`, `designateExpansion`.
- [`internal/sim/project.go`](../internal/sim/project.go) — `roomRecipe.expands` and `fullBay`, `roomDemolishPhase`, where `designateRoom` records each room, and the `planRooms` call sites.
- [`internal/sim/hall.go`](../internal/sim/hall.go) — `chairsShort`, the hall's shortfall in chairs.
- [`internal/sim/workorder.go`](../internal/sim/workorder.go) — `taskWage`: a wall torn down pays `wage-demolish`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `jobBuild`'s demolition branch, shared with passages.
- [`internal/sim/roomgrow_test.go`](../internal/sim/roomgrow_test.go) — growing right and left, a storage room's aisle, a kitchen's stove-and-pantry pair, no double walls, staying out of other rooms, the planner's preference and orders for every kind, and colonists building one.

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

### Rooms are recorded

`designateRoom` appends a `roomRecord` to `w.roomRecords`: the recipe the room
was actually built from (a narrow fallback's has no aisle), its `roomFrame`,
its fixture count `n`, and its issuer. Before this, a room was forgotten as
soon as its project was pruned, and nothing could be asked of it afterwards. A
record's frame is the room's **current** extent: an expansion widens it, and
growing left moves its origin. The doorway does not move, so the record does
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

## Why it is this way

- **Demolition first.** The old wall comes down before anything else, so the
  strip is reached from inside the room. Raising the new walls first would
  enclose the space between them and the old wall, with no way in and possibly
  a builder inside, which is what [escape.md](./escape.md) exists to clean up.
  Digging first would need the strip to be reachable from outside, which a room
  in a rock niche never is.
- **Records, not rediscovery.** Inferring rooms from terrain (find a wall
  rectangle with a bay of bunks) would be fragile around party walls and
  crash pods. Recording them at designation is one slice append and keeps
  everything deterministic (records are iterated in order, never as a map).
- **Whole cycles of the bay.** Growing a room means its new end repeats its
  old one. A kitchen grows a stove and its pantry together, so every stove
  keeps a pantry of its own (see [scumhouse.md](./scumhouse.md)).
- **Kitchens, incubators and halls too.** The first version grew only
  dormitories and storage rooms. On a 30-colonist colony (seed
  1790962337151000000, 10000×10000) the colony never wanted either: crash pods
  bring bunks and lockers. What it did want was ten kitchens, four incubator
  rooms and three halls, each a new room. Back-wall sharing (see
  [construction.md](./construction.md)) made both side walls of a room good
  backing, so they went up as triptychs: a room with a kitchen backed onto each
  side wall, facing away. With those three kinds growing too, the same run at
  tick 10,000 had 11 rooms instead of 20: two kitchens of four stoves each, one
  incubator room of six, two full halls.
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
- **Player orders expand too.** The colony rarely wants a dormitory on its own:
  each crash pod brings a bunk, and `desiredFacilities` is a headcount ratio
  well below one per colonist. Most dormitories and storage rooms come from the
  player's `d` and `r` orders, so those had to grow rooms for the feature to
  matter. The orders have no placement control anyway: the planner has always
  chosen the site.

## Extending it

- **Another expandable recipe** needs only `expands: true`, provided its bay
  repeats in whole cycles of `kinds` (`designateExpansion` continues the cycle
  from `n`). If its fixtures are linked to each other, as a kitchen's stove and
  pantry are, link the new ones in `designateExpansion` as it does
  `linkPantry`.
- **Growing deeper rather than wider** (more rows in front of the bay) would
  move the front wall and its doorway, so `w.doorTiles` would need an entry
  removed, and nothing removes entries today. Keep that invariant in mind.
- **Commissioned rooms** (houses, chefs' kitchens) are skipped by the issuer
  check. Letting a colonist grow its own house would mean `expandRoom` taking
  an issuer, and fixtures inheriting ownership as in `jobBuild`.

## Related

- [construction.md](./construction.md) — room shells, siting, sharing walls, and the double-wall rule expansion shares.
- [labor.md](./labor.md) — work orders, the treasury, and `wage-demolish`.
- [escape.md](./escape.md) — passages, the other place walls get torn down.
- [storage.md](./storage.md) — what a storage room's containers hold.

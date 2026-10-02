# Zoning and structure types

> Part of the [mars-sim documentation](./README.md).

## What it is

The player decides where the colony builds. A **zone** marks ground for one use
(residence, storage, or production), and every **structure type** (a dormitory, a
silo, a scumhouse, a crash pod) is tagged with the zone it belongs in. With manual
zoning, the game's default, colonists build a structure only inside a zone of its
kind, and build nothing they have no zone for. The player paints zones as
rectangles on the map, can remove them, and can order any area's structures
cleared. Painting is free. The work it implies is bought from the treasury: rock
inside a new zone is dug out, and a structure left outside a zone of its kind is
cleared. With `zoning-auto` on, the colony sites its rooms itself, as it always
did, and zones each one as it marks it out.

## Source

- [`internal/sim/zones.go`](../internal/sim/zones.go): `ZoneKind` and
  `zoneSpecs` (the zone table), the per-tile zone grid, `PaintZone` /
  `ClearArea` / `CancelClear` (the commands), `planZonePaint` / `paintZone`,
  `clearArea`, `siteRule` (how planning asks where a room may go), the
  waiting list, and `publishedZones` (row runs for frontends).
- [`internal/sim/structures.go`](../internal/sim/structures.go):
  `StructureType` and `structureSpecs` (the tag table), the structure
  registry (`registerRoom`, `registerPod`, `registerLone`, `maybeRetire`),
  `cancelProject`, and tearing down: `demolish`, `emptyDepot`,
  `forgetDetours`, `jobClear`.
- [`internal/sim/project.go`](../internal/sim/project.go): `roomRecipe.structure`,
  `planRoomFor` / `planRoomWith` (zone first, then free ground with
  zoning-auto), `roomSiteClearRule`, `roomProjects`, and demolish tasks
  (`buildTask.demolish`).
- [`internal/sim/crashpod.go`](../internal/sim/crashpod.go): `registerPod` on
  landing, and `podZoneOK` (a pod never lands on another kind's zone).
- [`internal/sim/excavation.go`](../internal/sim/excavation.go):
  `startExcavation` (shared with zone digs) and `cancelOrdered` (shared with
  clearing orders).
- [`internal/sim/workorder.go`](../internal/sim/workorder.go): `WorkClear` and
  `taskWage`.
- [`internal/wire/zones.go`](../internal/wire/zones.go): the `zones` and
  `zoning` topics.
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go): the `zone`,
  `clear` and `clear-cancel` commands (`hostAPI` 13).
- [`web/src/ui/ZonesPanel.svelte`](../web/src/ui/ZonesPanel.svelte), `showZone`
  and `showZones` in [`web/src/main.ts`](../web/src/main.ts), `setZones` in
  [`web/src/map/renderer.ts`](../web/src/map/renderer.ts), the zone tints in
  [`web/src/map/palette.ts`](../web/src/map/palette.ts).
- [`internal/sim/zones_test.go`](../internal/sim/zones_test.go),
  [`internal/wire/zones_test.go`](../internal/wire/zones_test.go).

## How it works

### Zones are per tile

Every tile holds at most one zone kind (`zoneCell` in a sparse `pagedGrid`), so
zones cannot overlap by construction. A paint sets every tile of its rectangle to
its kind. Where it covers another kind it replaces it, which is how a player
moves the edge between a residence and a storage zone. Two rectangles of one
kind that touch make one zone of any shape, an L for example. Nothing groups
tiles into numbered "zones": what matters to a room is that every tile it builds
on is zoned for its kind. `PaintZone{Kind: NoZone}` unzones.

### Structure types carry the tags

| Zone | Structure types |
| --- | --- |
| residence (green) | facility room (pods and toilets), dormitory, house, meeting hall, crash pod |
| storage (blue) | storage room (silos) |
| production (gray) | scumhouse (kitchen), scum incubator, trash room (incinerator), foundry |

Every `roomRecipe` names its `structure`, and so does every way a structure
appears: a crash pod (`registerPod`), and a lone emergency fixture
(`registerLone`, typed by `looseStructure`). The tag belongs to the structure
type, not to the terrain, because rooms mix fixtures: a kitchen's pantry is a
`Storage` chest, but it stands in production with its stove.

The structure registry (`World.structures`) records each structure: its type,
the tiles it builds on (`tiles`, including a party wall it borrows), the
footprint that has to lie in its zone (`area`, less a borrowed party wall), its
reserved door tile, and, while it rises, its project. `structureAt` indexes the
built tiles, so a party wall shared by two rooms is known to be both rooms'.

### Where a room goes

`planRoomFor` asks `findRoomSiteRule` with a `siteRule`:

1. **Inside a zone of the room's kind** (`inZone`). Every tile of the room's
   footprint must be zoned for it. The back wall may stand against open floor
   as well as rock or a wall: a zone drawn in the middle of a cavern has no rock
   to back onto, and the old rule would never site anything there.
2. **With zoning-auto only, on free ground.** Unzoned or same-kind ground, under
   the old rule (backed by rock or a wall). `registerRoom` then zones the room's
   footprint for it, so the colony's zones grow with what it builds.

With manual zoning a room that finds no site in step 1 waits, and
`noteZoneWait` records that the colony wanted it. The Zones tab lists it ("the
colony wants a scumhouse: no production zone has room for one"). Planning order
is unchanged, so a manual colony with no production zone holds everything
behind its first scumhouse, exactly as it holds everything behind life support
today.

Emergency builds (`findBuildSpot`) obey the same rule tile by tile
(`zoneAllows`).

### Crash pods hold their ground

Each pod holds its footprint and its one-tile margin as residence while it
stands (`zoneCell.locks`, counted, since neighbouring pods' margins overlap).
Painting skips held tiles and says so in the log, so the cluster of pods is
always residence. `findPodSite` will not land a pod on ground zoned for anything
else (`podZoneOK`), because a landing would take that ground over. Pods can be
cleared like anything else. When the last tile of one comes down,
`maybeRetire` releases its hold (the ground stays residence, unheld), its door
tile, its `World.pods` entry, and its colonist's `hasPod`.

### What a paint costs

`planZonePaint` works a paint out before anything changes:

- **Tiles**: the rectangle less held tiles, and less tiles already of that kind.
- **Evictions**: every structure with any `area` tile the paint changes to a
  kind it does not belong in, or unzones, is evicted. The whole structure is
  evicted, even when the paint covers only its corner, because half a room is
  not a room.
- **Clearing**: the evicted structures' built tiles, less any tile a structure
  that stays also stands on (`tilesToClear`), at `wage-clear` each.
- **Digging**: seen, unmarked rock in a new zone (`unmarkedRock`), at
  `wage-dig` each, as an ordinary excavation project (`startExcavation`) that
  shows and cancels on the Dig tab. Unlike the Dig tool it is not capped at
  `maxExcavationTiles`: the page prices the paint before it is sent.

It is all or nothing: if the treasury cannot pay for the digging and the
clearing together, nothing is painted (`TestARezoneTheTreasuryCannotPayForChangesNothing`).
Otherwise an evicted structure that is still rising has its project called off
and refunded (`condemn`), the zones change, and the clearing and dig orders go
up.

### Clearing

A clearing order is a project named `clearing` (`ClearingName`) whose tasks have
`demolish` set and terrain `Floor`, funded as `WorkClear` orders. Claiming is
unchanged (`claimNearestTask`); `jobBuild` hands a demolish task to `jobClear`,
which walks beside the tile and takes it down over `demolish-ticks`. Then
`demolish`:

- **Empties a depot** (`emptyDepot`): cancels every order resting at it (an ask's
  goods go back to the seller's line, a bid's money to the bidder), closes haul
  work to or from it, and moves every ledger line, still its owner's, to the
  nearest chest that will take it: a communal one, or the owner's own. What fits
  nowhere is lost, and the log says how much.
- **Unhooks** what points at the fixture by position (`letGoFixture`): a
  keeper's and its hens' trough, a chef's kitchen, cook and scum claims, a
  pantry link.
- **Sets the tile to floor.** `SetTerrain` touches every flow field and dirties
  the region graph, as for any terrain change, so the shared fields route
  through the gap on their next read.
- **Forgets detours** (`forgetDetours`): a colonist's own A\* route is only
  replanned when it is blocked, so a colonist already walking round the old wall
  would otherwise finish the long way. Every cached route longer than a straight
  walk to its end is dropped and replanned
  (`TestClearingAWallReroutesColonists`).
- **Retires** each structure that stood there once nothing of it stands.

`ClearArea` (the Zones tab's "Clear area" tool) is the same order for every built
tile the colony has seen in a rectangle, with no zone involved. A room still
going up in the area is called off and refunded. It is not a zone and leaves
nothing behind. `CancelClear` refunds what an open clearing order still holds,
like an excavation's cancel.

### The map and the tab

The `zones` topic (`[y, x0, x1, kind, locked]` runs plus each kind's name and
colour) is held for the page's life, as `names` is, and drawn by the renderer as
the lowest tint layer over the terrain, under filth, so a stain still shows in a
zone. The `zoning` topic carries the Zones tab: the type table, tile counts,
every structure (type, zone, bounds, tiles built, pod, rising), open clearing
orders, and the waiting list.

The tab's tools are one per zone kind, "Remove zone", and "Clear area". A drag
marks the area (the Dig tool's `areaTool` hook, shared). `showZone` estimates
the outcome from the topics and the tile pages the page holds, the same way the
Dig tab counts rock: tiles changed, held tiles skipped, rock to dig, structures
that would be evicted (tinted red, with the warning "a dormitory in this area
will have to be cleared: an additional paid work order of up to $X"), and the
price against the treasury. The engine has the last word and logs the outcome.

The TUI shows no zones and has no tool for them.

### Auto mode, and where each mode runs

`zoning-auto` defaults off: the game, as the browser starts it, is zoned by the
player. The TUI and headless runs cannot draw a zone, so the committed
`mars-sim.yaml` turns it on for them. Without that file (for example
`-config ""`) a terminal colony builds nothing but its pods. The new-game form has
a "Colonists zone for themselves" box, and the page takes
`?zoning-auto=true`.

In auto mode the golden hashes did not move: zoning the rooms it builds, and
preferring the player's zones, changes nothing a seed produces when the player
draws nothing.

## Why it is this way

- **Per-tile kinds, not zone objects.** A list of rectangles makes "partly
  replace a zone" a rectangle subtraction (up to four pieces per cut), and makes
  "is this one zone?" a graph problem. A tile grid makes the non-overlap rule
  impossible to break, a replacement just a paint, and an L-shaped zone free.
  Nothing needed zone identity: siting asks per tile.
- **Tags on structure types, not terrain.** Per-terrain tags put a kitchen's
  pantry chest in storage and evicted it from its own kitchen.
- **Whole structures are evicted.** Clearing only the tiles a paint covered
  leaves a room with a wall missing, which the planner cannot finish and the
  colony cannot use.
- **All-or-nothing paints.** A rezone over a dormitory that could be painted but
  not cleared would leave a structure standing in the wrong zone with nothing
  ever coming to clear it.
- **Open-floor backing inside zones only.** Rooms are normally backed by rock so
  they never stand free across a route (see
  [construction.md](./construction.md)). Inside a zone that is the player's
  call; outside, the old rule protects auto-sited rooms.
- **Excavations and clearings do not count against the room cap.**
  `planRooms` caps rooms under way (`roomProjects`), not projects. With the old
  count, a big zone dug out of the rock held up every room behind it.
- **Pods hold their ground with a count, not a flag.** Neighbouring pods'
  margins overlap; clearing one pod must not release ground the other still
  holds.
- **The page estimates; the engine decides.** A command has no reply. The page
  cannot see which tiles another order has marked, or which walls two rooms
  share, so its clearing figure is an upper bound.

## Extending it

- **Move a structure type to another zone** ("scumhouses are storage"): change
  its `zone` in `structureSpecs`. Rooms already standing in the old zone stay
  until somebody repaints their ground.
- **A new zone kind**: a constant before `numZoneKinds` and a row in
  `zoneSpecs` (name and colour). The wire, the overlay and the tab's tools all
  read the table; nothing else changes.
- **A new structure type**: a constant and a row in `structureSpecs`, and the
  recipe (or arrival) that builds it naming it in `structure`.
  `TestEveryStructureTypeHasAZone` fails until it has a zone.
- **Invariants**: a structure's `area` lies in a zone of its type, in manual
  mode and in auto mode alike; nothing may iterate `structures`, `structureAt`
  or the zone grid in map order to decide anything (use `sortedStructures`); a
  paint is all or nothing.
- **Next**: zones the TUI can show, salvage (a cleared wall giving back its
  rock), and a preview the engine computes for the page.

## Related

- [construction.md](./construction.md): rooms, recipes, and siting.
- [excavation.md](./excavation.md): the dig orders a zone over rock posts, and
  the area tool the Zones tab shares.
- [crash-pods.md](./crash-pods.md): the pods whose ground is always residence.
- [labor.md](./labor.md): work orders, which clearing and digging are.
- [pathfinding.md](./pathfinding.md): the flow fields and routes a cleared wall
  opens.
- [frontend-web.md](./frontend-web.md): the Zones tab and the map's layers.
- [wire-format.md](./wire-format.md): the `zones` and `zoning` topics.

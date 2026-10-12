# Stairs

> Part of the [mars-sim documentation](./README.md).

## What it is

A stair is the way between two levels: a `StairDown` on one level and a
`StairUp` straight below it, at the same (x, y). Anything that walks can step
from one end to the other as a single move. The colony digs one when the
player orders it, or when it runs out of rock to mine; digging one makes the
level below (generated on its own random streams) and reveals what is around
the stair's foot. This doc covers the stair, how every search and system
crosses it, and how the colony digs one. The data model it rests on (levels,
`Point.Level`, layers) is [layers.md](./layers.md).

## Source

- [`internal/sim/layered.go`](../internal/sim/layered.go): `links` (what
  a stair or shaft leads to, and its cost), `hasLinks`.
- [`internal/sim/stairs.go`](../internal/sim/stairs.go): the stair list
  (`trackLinks`), `travelEstimate`, `addLayer`, `canDigStairAt`, `digStair`,
  and the planner: `planStairs`, `findStairSite`, `designateStair`,
  `finishStair`.
- [`internal/sim/rooms.go`](../internal/sim/rooms.go): regions flood
  walkable tiles; `linkChunkRegions` links across a stair.
- [`internal/sim/path.go`](../internal/sim/path.go),
  [`flowfield.go`](../internal/sim/flowfield.go),
  [`flowrepair.go`](../internal/sim/flowrepair.go),
  [`facilitychoice.go`](../internal/sim/facilitychoice.go): the searches,
  each with the stair as one more neighbour.
- [`internal/sim/worldgen_chunks.go`](../internal/sim/worldgen_chunks.go):
  `worldGen.level` and `featureRand`'s level mixing.
- [`internal/ui/tui/model.go`](../internal/ui/tui/model.go): `<`/`>`
  (`changeLevel`), the build menu's "stair down".
- [`internal/sim/stairs_test.go`](../internal/sim/stairs_test.go): both ends
  and their links, rooms joining and splitting, A\* and flow fields (with
  repair) crossing, a colonist climbing by field, travel estimates, nothing
  reaching through a floor, a stair dug in a real colony, a miner working
  the level below, and lockstep determinism with two levels.
- [`internal/ui/tui/levels_test.go`](../internal/ui/tui/levels_test.go): the
  map switching levels.

## How it works

### What a stair links

`w.links(p)` is the single definition every system uses: a `StairDown` at
`p` leads to the tile below if that is a `StairUp`, and a `StairUp` leads to
the tile above if that is a `StairDown`, each at a cost of one step. A stair
with only one end in place links nothing. Shafts are links too, with a
higher cost; see [shafts.md](./shafts.md). Both stair terrains are walkable, so a mover stands on either
end like floor.

`w.stairs` lists the upper end of every stair, sorted by `lessPoint`, kept in
step by `setTerrain`. While it and the shaft list are empty, no search asks
`links` at all (`hasLinks`), so a one-level game pays nothing for stairs in
its hot loops.

### Every search treats a stair as one more neighbour

- **Rooms.** A region is a connected group of walkable tiles (floor and both
  stair ends) within a chunk of one level. `linkChunkRegions` links regions
  that touch across a chunk border, and the regions at the two ends of every
  stair. Rooms are connected groups of linked regions, so a stair makes two
  levels one room, and `sameRoom`, the reachability gate every job checks,
  answers across levels in O(1). Links are symmetric, so walling over either
  end re-floods that chunk, drops the link, and splits the room.
  `refreshSpatial` re-floods dirty chunks level by level, shallowest first,
  and links only after every level's regions have their final IDs.
- **A\*.** After the eight neighbours, a node on a stair tries the other end.
  Its heuristic stays admissible because a level change counts one step and a
  stair is exactly one step. The goal test uses `Adjacent`, so a target is
  never "reached" from straight above it.
- **HPA\*.** Unchanged: it routes over the region graph, which now has stair
  links in it. A corridor can cross levels; `paintCorridor` paints each
  region on its own level.
- **Flow fields.** A field spans every level: a toilet on the landing level
  is a goal for a colonist two levels down. Rebuild's BFS adds the stair
  neighbour; repair's `forNeighbours` does too, so a repaired field still
  matches a rebuilt one. A change at a point also repairs the tiles straight
  above and below it, because the point may have been one end of a stair.
- **`followField`**, the facility searches and `stepAside` add the same
  neighbour where it means something (`stepAside` keeps a colonist on its
  level).
- **Moving.** `moveEntity` updates occupancy and the chunk index on both
  levels when an entity changes level.

### "Nearest" goes by the stairs

Choosing the nearest of several candidates (chests, rock to mine, scum, a
meal depot, a workshop, a building task, prey) used straight-line distance.
Across levels that is wrong in both directions: a rock straight below is not
one step away when the nearest stair is fifty tiles off. `w.travelEstimate`
is Chebyshev on one level, exactly as before, so a one-level game chooses the
same things. Across levels it takes the cheapest chain of straight lines
through the stairs that exist: over to a stair, one step through it, on from
its other end, level by level. With no stair between two levels it answers
`unreachableEstimate`, which loses to anything reachable.

### Digging a stair

- **The setting.** `deepest-level` (default 1) is how far down the colony
  may dig. At 1 it never plans a stair, so a game that does not ask for
  levels is unchanged. `stair-ticks` is the work one takes.
- **A picked tile.** `OrderStair{At}` with a tile (the browser's Dig tab:
  the Stair tool, then a click on the map) is checked and marked out at
  once, or refused in the log, in either siting mode. See
  [siting.md](./siting.md).
- **When the colony sites it.** Only with `siting-auto`: `planStairs` runs
  with the room planner and marks out one stair at a time, from the deepest
  level reached, when the player has ordered one without a tile
  (`OrderStair{}`, `b` then `v` in the terminal) or when there is no
  unclaimed mining frontier on any level. It draws nothing random.
- **Where.** `findStairSite` spreads outward from an anchor (the middle of
  the map on the landing level, the foot of the stair in on a deeper one, or
  of the shaft in when there is no stair) and takes the first open, known
  floor tile in the main room whose eight neighbours are open floor too, so
  the stair plugs no corridor and has room around it. It stops at the edge
  of the main room's extent on that level (`mainRoomBounds`, the chunks its
  regions are in); see "Why".
- **Building it.** A stair is a one-task project, paid for and claimed like
  any other build. It is mining work (`buildSkill`), takes `stair-ticks`, and
  costs no materials. On completion `finishStair` calls `digStair`: the tile
  below becomes the `StairUp` (making the level below if this is the first
  way in, and revealing around the foot), then the tile itself becomes the
  `StairDown`. The rock dug out of the bottom is shaped into the stair, so
  nobody gets its ore.
- **What is down there.** Revealing around the foot generates the level's
  chunks around it, and breaking into a hidden cavern floods it into view and
  rolls its nests, exactly as digging sideways into one does (see
  [caverns.md](./caverns.md)). Miners then find the rock around the foot
  through the shared frontier.

### Each level has its own rock

Each level has its own `worldGen`, and `featureRand` mixes the level's
distance from the landing level into every feature's stream: veins, caverns,
passages, scum and salt are all different on each level. The landing level
mixes in nothing, so it generates exactly what it did before there were
levels, and the golden hashes did not move. Scum growth runs per level on the
same principle (`growScumOn`). Depth does not yet make a level richer or more
dangerous; that is phase Z4 of [z-levels.md](./z-levels.md).

### What players see

In the terminal the map shows one level, starting on the landing level. `<`
and `>` move up and down through the levels the colony has reached, keeping
the view's (x, y), so the view lands straight above or below where it was.
The header names the level and the legend shows 🔽/🔼 once there is more
than one level; a one-level game looks as it always did.

The browser does the same since Z5: a level picker in the top bar and `<`
and `>` change the level the map shows, and the Dig tab orders stairs,
shafts, holes and ladders (see [frontend-web.md](./frontend-web.md),
"Levels"). Every frame is one level's (see
[wire-format.md](./wire-format.md), "Levels").

## Why it is this way

- **The site search is bounded by the main room.** A level the colony has
  just broken into is often only the foot of a stair or shaft, with no site
  on it, and the planners ask for a site on the deepest level every planning
  round until the order can be met. The search used to run out to the map's
  full width before giving up: seconds a round on a 1000x1000 map, and on the
  browser's 10000x10000 the worker never got through a tick (a shaft and then
  a hole froze the game). A site must be in the main room, so nothing past
  the main room's bounds can be one, and the first site found is the same.
- **One definition of a link.** A stair is defined by two terrains, not a
  link table: a half-built or walled-over stair links nothing, and every
  system (rooms, A\*, fields, repair) agrees about it because they all ask
  `links`. The cost is that a stair can only join two neighbouring levels
  at one (x, y), which is all a stair should do. Shafts span many levels
  and are still terrain-defined: a middle tile on each level links up and
  down (see [shafts.md](./shafts.md)).
- **A stair is mining, and free.** Making it a fixture build with materials
  would put a supply chain between the colony and its second level before
  there is any reason to go down. Depth's rewards (Z4) will justify a price.
- **Planned only when wanted.** The doc's trigger, "no mining frontier
  left", rarely fires on a big map, where there is always more rock. That is
  deliberate for now: until depth pays (Z4), digging down is the player's
  choice, and the automatic trigger is the colony's last resort.
- **Rooms join across stairs; nothing else does.** Perception, talk, combat
  range, the hall and witnesses all ask `Within` or a spatial query that
  searches one level's chunk index (`entityIDsNearSorted`, `nearestMatch`),
  and line of sight is false between levels, so a colonist cannot see, hear
  or shoot through a floor even when the room graph says the tile below is
  reachable.

## Extending it

- A new search that expands neighbours must also try `links` (behind
  `hasLinks`), adding each link's cost, or it will not see the other levels.
- A new "nearest" choice should rank by `travelEstimate`.
- A new kind of vertical link belongs in `links`' terrain
  switch if it is two-way. A one-way link must not be a region link: rooms
  are symmetric. See [z-levels.md](./z-levels.md).
- To let the colony build on a deeper level, start from the `landing()` calls
  listed in [layers.md](./layers.md).

## Related

- [layers.md](./layers.md): levels, `Point.Level`, and layers.
- [z-levels.md](./z-levels.md): the plan; this is phase Z1.
- [pathfinding.md](./pathfinding.md): regions, rooms, A\*, HPA\* and flow
  fields.
- [caverns.md](./caverns.md): what breaking in reveals.
- [frontend-tui.md](./frontend-tui.md): the terminal's controls.

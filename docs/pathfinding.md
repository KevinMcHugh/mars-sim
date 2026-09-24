# Pathfinding & navigation

> Part of the [mars-sim documentation](./README.md).

## What it is

How entities move toward goals. There are two complementary systems: **cached A\***
for an individual's trip to a specific tile, and **shared flow fields** for the
hot destinations many agents head to at once (facilities, the mining frontier).
Both sit on top of a two-level **region/room** structure that answers "can I even
get there?" in O(1) and bounds long searches.

## Source

- [`internal/sim/rooms.go`](../internal/sim/rooms.go) — regions, rooms, incremental maintenance.
- [`internal/sim/path.go`](../internal/sim/path.go) — tile A\*, the reachability gate, corridor-constrained search.
- [`internal/sim/hpa.go`](../internal/sim/hpa.go) — hierarchical routing over the region graph.
- [`internal/sim/flowfield.go`](../internal/sim/flowfield.go) — shared multi-source BFS distance fields.
- [`internal/sim/flowrepair.go`](../internal/sim/flowrepair.go) — repairing a field around what changed instead of rebuilding it.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `travelTo` (follow a route) and how jobs invoke navigation.

## How it works

### Regions and rooms (the reachability layer)

Walkable tiles are grouped in two levels so updates stay cheap:

- A **region** is a connected component of `Floor` cells **within one chunk**
  (chunks are 16x16; see
  [spatial-index-and-performance.md](./spatial-index-and-performance.md)).
- A **room** is a connected component of the **region graph** — regions are linked
  when their floor cells touch across a chunk border. A room's ID is the smallest
  `RegionID` it contains, so it does not depend on the order the component is
  walked.

When a tile changes, `SetTerrain` marks its chunk dirty; `refreshSpatial`
(end of each tick) re-floods only the dirty chunks' regions, re-links across
borders, and relabels rooms. Relabeling is incremental too: only components that
touch a region just created, or that neighbored one just deleted, are re-walked
(`relabelSeeds`), the rooms they used to belong to are dropped (`staleRooms`),
and every other room keeps its label. So excavating or walling a tile costs
**O(one chunk)** to re-flood plus O(the rooms it touches) to relabel, not a
global O(map) flood fill — nor O(all regions), which is what relabeling used to
cost and what became milliseconds a tick once undiscovered natural caverns put
thousands of static rooms on a big map (see [caverns.md](./caverns.md)).
`w.rooms` keeps every room's floor-tile size, so a room nobody touched never
needs re-measuring.

`refreshSpatial` walks the dirty chunks **in sorted order**, and that sort is
load-bearing: a region's ID is the next value of a counter, so re-flooding in
`map` order handed the same world different region IDs on different runs — and
room IDs and the abstract search's tie-break are both built on those IDs. See
[determinism.md](./determinism.md).

`roomOf(p)` and `sameRoom(a, b)` are then O(1): two floor tiles are mutually
reachable on foot iff they are in the same room. Every job-assignment routine uses
this as a cheap gate before running any search.

### Tile A\* (`path.go`)

Colonists navigate with A\* over walkable tiles (8-connected, uniform cost, so the
heuristic is Chebyshev). The search targets a tile **adjacent** to the work target
(colonists mine/build/use facilities from a neighbor). The `pathfinder` holds
reusable scratch, with a generation stamp so it need not be cleared between
searches, and a hand-rolled binary heap to avoid `container/heap` interface
boxing. Ties break on cell index for determinism.

That scratch is a `pagedGrid` rather than an array sized to the map, as are the
flow fields below and the region labels above: all of them only ever hold a
value on a walkable tile. Together they were 124 bytes per tile of permanently
zero memory — most of a 16 GB process. See
[sparse-grids.md](./sparse-grids.md), which also covers the two fast paths
(`interiorPage`, `pageAt`) these searches use to get their speed back.

A computed route is **cached on the colonist and followed one step per tick**
(`travelTo` in `systems.go`), so A\* runs once per job, not every tick. Occupied
tiles are valid transit cells but not valid destinations; an entity may pass
*through* any other entity on its route but must end the tick on a free tile —
except an alien, which still blocks outright (a real, dangerous obstacle, not
clutter to walk past; a colonist, cat, or mouse parked in a narrow corridor
used to wedge a whole queue behind it before this). It replans when the route
is missing, was for a different goal, ran out, or the terrain changed under it.

`pathToAdjacent` is the entry point. It first rejects unreachable targets with the
O(1) room check, then chooses a strategy:

- **Short or same-region trips** run a flat (optimal) tile search directly.
- **Long cross-region trips** use HPA\* (below).

### Hierarchical A\* (`hpa.go`)

For a long trip, `abstractCorridor` first routes over the **region graph**
(`region.links`, using region representatives for the heuristic, expanded in
ascending region ID via `sortedLinks` so equal-cost corridors resolve the same
way every run) to get a corridor of regions — the broad "which rooms to cross" answer. `paintCorridor` stamps every
cell of those regions with a fresh generation, and the tile A\* then runs
**constrained to the corridor** (an O(1) array membership test per neighbor). This
bounds tile exploration to the abstract route instead of the whole reachable area.
If the abstract step or the constrained search fails, it falls back to the flat
search. On a map where a wall forces a long detour, this cut a cross-fort search
from ~1.0 ms to ~0.28 ms (~3.6x, ~4.5x fewer cells explored).

`abstractCorridor`'s region-graph search sorts a region's neighbor IDs before
expanding them, even though `regionHeap` itself already breaks ties
deterministically by region ID. Without the sort, two neighbors that reach a
third region at *exactly* equal cost raced: `gscore`'s strict less-than only
records whichever is visited first, and `region.links` is a map, whose
iteration order Go deliberately randomizes — so the same seed could pick a
different (still equally short) corridor on every run, sending a colonist
down a visibly different but equally valid route each time. That is a real,
user-visible break of "same seed => same game" (see AGENTS.md), not a
cosmetic one — it's what a long-distance job's route looked like changing
between runs. If you touch this search again, keep the sort (or replace it
with an equally order-independent reduction) — see the comment inline in
`hpa.go`.

### Flow fields (`flowfield.go`)

When *many* agents head to the *same* destinations, per-agent A\* is wasteful. A
`flowField` is a distance field: `dist[cell]` is the number of 8-connected steps
to the nearest goal over walkable tiles (-1 = unreachable), computed once per
change with a **single multi-source BFS** from a seed function and then shared by
every agent. Each agent just steps to a downhill neighbor — O(1) per tick —
replacing N searches with one field.

Fields are **lazy and generation-stamped**: `ensureFresh` updates at most once
per tick (the first reader of the tick pays, the rest reuse), and a new generation
retires all prior distances without an O(map) clear, so a rebuild is O(reachable),
not O(map).

Fields are **repaired, not rebuilt**, when something changes. A terrain change
`touch`es every field at that tile; a frontier rock appearing, vanishing, being
claimed or released touches the frontier field there (the job board does it).
The next read repairs the tiles around every touched point (`repair` in
`flowrepair.go`):

1. **Forget what went up.** Cells whose distance can no longer be supported (a
   tile that became a wall or stopped being a goal, and every cell whose only
   route ran through one) are found in order of their old distance and marked
   unreachable. A cell at distance d survives if it is still a goal (d = 0) or
   still has an unaffected neighbour at d - 1.
2. **Relax outward.** Each forgotten cell and each changed tile is seeded with
   the best its neighbours now offer (0 for a goal), and a unit-step Dijkstra
   lowers distances until nothing changes. This also carries decreases from a
   newly dug tile or a new goal.

The result is identical to a rebuild — `TestFlowFieldRepairMatchesRebuild`
checks every cell of every field on every tick of busy worlds — so readers
cannot tell the difference. A field touched more than `maxTouched` times
between reads, or whose repair would forget more than `maxRepairCells` cells,
rebuilds instead; so does a field's first read. A field needs both a seed
function (all goals, for a rebuild) and a goal predicate (one tile, for a
repair), and the two must agree (`TestFlowFieldGoalAgreesWithSeed`).

There is one field per **facility terrain** (nutrient pods, toilets) and one
**frontier** field toward the nearest *unclaimed* diggable rock. A facility
field is everyone's route, so it only leads to **communal** fixtures; a
colonist headed for its own private one routes there with A\* instead (see
[property.md](./property.md)). `followField`
moves an agent along a field: it BFSes through any occupied tile but an
alien's to the first depth with a free landing, preferring the lowest field
distance but able to step **uphill** when every downhill route is occupied —
an escape valve that is essential in a full room, where a crowd with only
uphill free space would otherwise gridlock until its hungriest members starve.
It never crosses a `buildTiles` tile a builder needs clear.

A facility field also answers *which* facilities are nearest, not just how
far: walking only downhill from a tile visits exactly the shortest routes to
the facilities at its distance. `chooseFacility` relies on that instead of
running its own BFS (see [needs.md](./needs.md)).

### Two mining strategies

Mining picks a strategy by threshold (`useFrontierMining`, checked dynamically as
the colony grows):

- **Small colonies/maps**: each miner claims a specific nearest rock up front
  (`claimNearestMine`) and A\*s to it — cheaper when few miners share the sweep.
- **Big colonies/maps** (`FrontierFieldMinColonists` / `FrontierFieldMinArea`):
  miners follow the shared frontier flow field to the digging edge and claim a
  rock **on arrival** — one BFS serves everyone. A 300x300 map with 3000 colonists
  runs ~10 ms/tick, about the same as 2000 colonists on a quarter of the map.

## Why it is this way

- **Two-level regions/rooms** turn reachability into an O(1) lookup and make
  terrain edits cheap to reflect — the foundation both A\* and HPA\* stand on.
- **Cache-and-follow A\*** amortizes the search over the whole trip instead of
  re-searching every tick, and lets colonists route around walls instead of
  wedging (the original greedy step-toward could box them in).
- **Flow fields** are the "everyone navigates the same" answer: they scale to
  thousands of agents converging on a handful of destinations, which per-agent
  search cannot.
- **Repair instead of rebuild**, because the fields change constantly. Every
  mined tile used to mark every field stale and every claim the frontier field,
  so a digging colony rebuilt several fields with a BFS over its whole area
  nearly every tick; on a 100-colonist game that was a quarter of the CPU. A
  dug tile almost never moves more than a handful of distances. The repair
  handles increases as well as decreases (claims and walls remove goals and
  routes), which is what makes it exact rather than a heuristic; the cap on
  forgotten cells keeps a change that really does reroute half the colony from
  costing more than the rebuild it replaces.
- **HPA\* corridors** keep long trips from exploring dead ends; the win grows with
  map size and obstacle density.
- **The uphill escape step** and **transit-through-crowds** were both learned from
  full-room gridlock and starvation.

## Extending it

- **A new shared destination** (another facility kind, a stockpile): allocate a
  flow field with a seed function reporting its goal tiles and a goal predicate
  that agrees with it, and `touch` it wherever a goal or walkability changes —
  a missed touch leaves a wrong distance behind, which
  `TestFlowFieldRepairMatchesRebuild` will catch if the new field is in it.
  Facility-terrain fields are auto-allocated in `newWorld`.
- **Reusing corridors**: HPA\* corridors are a natural thing to cache and share
  across agents — a noted future step.
- **Z-levels**: the region, flow-field, and HPA\* machinery were built to extend
  into a multi-floor world (not yet implemented).

## Related

- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — chunks, the job board/frontier, and the performance story.
- [entities-and-ai.md](./entities-and-ai.md) — the movement primitives that call this.
- [construction.md](./construction.md) — room reachability and routing around build tiles.
- [world.md](./world.md) — the grid and `SetTerrain`'s dirty-chunk marking.
- [determinism.md](./determinism.md) — why the region/link ordering here is sorted.

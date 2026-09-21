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
borders, and relabels rooms from the small region graph. So excavating or walling
a tile costs **O(one chunk)** to re-flood plus O(regions) to relabel, not a global
O(map) flood fill.

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
reusable scratch sized to the grid, with a generation stamp so arrays need not be
cleared between searches, and a hand-rolled binary heap to avoid `container/heap`
interface boxing. Ties break on cell index for determinism.

A computed route is **cached on the colonist and followed one step per tick**
(`travelTo` in `systems.go`), so A\* runs once per job, not every tick. Occupied
tiles are valid transit cells but not valid destinations; a colonist may pass
*through* a crowd of other colonists on its route but must end the tick on a free
tile (non-colonists still block). It replans when the route is missing, was for a
different goal, ran out, or the terrain changed under it.

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

### Flow fields (`flowfield.go`)

When *many* agents head to the *same* destinations, per-agent A\* is wasteful. A
`flowField` is a distance field: `dist[cell]` is the number of 8-connected steps
to the nearest goal over walkable tiles (-1 = unreachable), computed once per
change with a **single multi-source BFS** from a seed function and then shared by
every agent. Each agent just steps to a downhill neighbor — O(1) per tick —
replacing N searches with one field.

Fields are **lazy and generation-stamped**: `ensureFresh` rebuilds at most once
per tick (the first reader of the tick pays, the rest reuse), and a new generation
retires all prior distances without an O(map) clear, so a rebuild is O(reachable),
not O(map). Terrain changes mark every field stale; claiming/unclaiming a mine
tile marks the frontier field stale.

There is one field per **facility terrain** (nutrient pods, toilets) and one
**frontier** field toward the nearest *unclaimed* diggable rock. `followField`
moves an agent along a field: it BFSes through occupied *colonist* tiles to the
first depth with a free landing, preferring the lowest field distance but able to
step **uphill** when every downhill route is occupied — an escape valve that is
essential in a full room, where a crowd with only uphill free space would
otherwise gridlock until its hungriest members starve. It never crosses a
`buildTiles` tile a builder needs clear.

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
- **HPA\* corridors** keep long trips from exploring dead ends; the win grows with
  map size and obstacle density.
- **The uphill escape step** and **transit-through-crowds** were both learned from
  full-room gridlock and starvation.

## Extending it

- **A new shared destination** (another facility kind, a stockpile): allocate a
  flow field with a seed function reporting its goal tiles, and mark it stale on
  the relevant events. Facility-terrain fields are auto-allocated in `newWorld`.
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

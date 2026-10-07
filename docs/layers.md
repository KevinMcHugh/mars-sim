# Layers and levels

> Part of the [mars-sim documentation](./README.md).

## What it is

The world is a stack of levels, each its own 2D grid. A `Level` numbers
them: 0 is the surface (nothing is there yet), 1 is the landing level where
colony ships come down, and deeper levels count up. Every `Point` carries the
level it is on. Everything indexed by a tile's (x, y) on one grid, and every
per-level count (tiles, occupancy, region labels, refuse, scum and salt, the
job board, facility indexes, worldgen state) lives in that level's `Layer`.
The `World` keeps what spans levels: entities, the economy, the flow fields,
the region graph and rooms, the list of stairs, scratch buffers, and the
sparse maps keyed by `Point` (storage, fixtures, claims, door tiles,
structures), because their keys already say which level they mean.

This is the data model. How levels connect (stairs), and how everything that
moves or searches crosses between them, is in [stairs.md](./stairs.md). The
plan behind both is [z-levels.md](./z-levels.md); this is its phases Z0 and
Z1.

## Source

- [`internal/sim/geom.go`](../internal/sim/geom.go): `Point` with its
  `Level`, `Add`, `Chebyshev`, `Adjacent`, `Within`, and `gridStep` (an
  offset, for the neighbour tables).
- [`internal/sim/layer.go`](../internal/sim/layer.go): `Level`,
  `SurfaceLevel`, `LandingLevel`, `MaxLevel`, `Layer`, `newLayer`, and the
  lookups: `lay`, `layerIn`, `layer`, `landing`, `levelExists`, `noLayer`,
  `eachLayer`, `eachContainer`, `eachFacility`, `containerAt`.
- [`internal/sim/layered.go`](../internal/sim/layered.go): `layered[T]` (a
  `pagedGrid` per level), `World.index` / `pointOf` (cell indices that
  include the level), `linkFrom`, `hasStairs`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go): `LevelTiles`,
  `Levels`, and the snapshot's tile readers choosing a level by the `Point`.
- [`internal/sim/layer_test.go`](../internal/sim/layer_test.go): the
  one-layer shape, nothing on the surface after a long run, level-first
  ordering, neighbours staying on their level, and the cell index round trip.

## How it works

### A position names its level

```go
type Point struct {
	X, Y  int
	Level Level
}
```

There is one position type. An entity's `Pos`, a job's `Target`, a cached
route, an order book's depot and every map keyed by tile are all `Point`s, so
all of them say which level they mean. Consequences that matter:

- `p.Add(dx, dy)` stays on `p`'s level. Neighbour loops (`neighbors8`, a
  table of `gridStep` offsets) can never wander onto another level.
- `Adjacent` and `Within(o, r)` are false across levels however close the
  tiles are: nothing is next to, in reach of, in sight of, or able to talk to
  anything through a floor. Use them for "close enough to touch" and "in
  range".
- `Chebyshev` counts one step per level crossed. That keeps it a lower bound
  on any real walk (a stair is one step that moves nobody sideways), which is
  what A\*'s heuristic needs, but it knows nothing about where stairs are. To
  choose the *nearest* of several candidates, use `w.travelEstimate`, which
  goes through the stairs that exist (see [stairs.md](./stairs.md)).
- `lessPoint`, the tie-break everything sorts by, orders by level first, then
  row-major as it always did.
- A positional literal must state a level (`Point{x, y, l}`); the compiler
  rejects `Point{x, y}`. A keyed literal that leaves it out (`Point{X: x, Y:
  y}`) is on level 0, the surface, where nothing exists. That is deliberate:
  a missing level reads as nothing at all, not as the landing level, so it
  fails visibly. `TestNothingIsOnTheSurface` runs a trading colony and
  checks every entity, depot and container is on a real level.

### Finding a layer

| Call | Use |
| --- | --- |
| `w.lay(p)` | The layer `p` is on. For a level with no layer (an unset target like `Point{-1, -1}`) it returns `noLayer`, an empty layer whose maps read as empty, so a lookup there finds nothing. Writing to `noLayer` is a bug: its maps are nil and panic. |
| `w.layerIn(p)` | The layer if `p` is on the map and its level exists, else nil: `InBounds` and `lay` in one check, for the tile reads every search makes every step. |
| `w.layer(l)` | Level `l`'s layer, or nil. |
| `w.landing()` | The landing level. Calling it means "landing level by design". |
| `w.eachLayer`, `eachContainer`, `eachFacility` | Every level, shallowest first. |

Code that has a point asks that point's layer. The few places that use
`landing()` do so on purpose, and are the list to revisit when the colony
starts settling deeper levels:

- colony ships land on the landing level (`LandShip`, `MoveShip`, the ship
  siting in ship.go);
- the zone grid covers the landing level only (`zonable`): the colony zones
  where it lives, and a deeper tile reads as unzoned;
- the room planner sites rooms there (`findRoomSiteAllowingRock`); the
  colony builds where it lives, and a deeper level is where it digs;
- the market's silo is chosen there (`marketDepot`), and a browser's order
  or tile point is read as one there (`parsePoint` in internal/wire, the
  wasm host's order depot);
- random placement for arrivals, rat plagues and the starting aliens
  (`randomTile`, `freeFloorTiles`, `alienSpawnSite`) and the director's
  occurrences.

### Layers come into being

`World.layers` is indexed by `Level`, nil for a level nobody has broken into.
`newWorld` makes the landing level. `addLayer` makes a deeper one the first
time a stair reaches it: all rock, with its own job board, facility indexes
and, if the world generates lazily, its own chunk generator. A level the
colony has not reached costs a nil pointer.

### Per-tile scratch across levels

Searches that can cross a stair (A\*, the flow fields, the facility search,
`followField`'s look through a crowd) keep their per-tile state in a
`layered[T]`: one `pagedGrid` per level, allocated on the first write to that
level. Hot loops fetch the current level's grid once and keep it until a
stair takes them elsewhere, so a search that stays on one level (nearly all
of them) pays no more than it did. Queues and heaps hold `int32` cell indices;
`World.index` folds the level in as `(level*Height + y)*Width + x` and
`pointOf` undoes it. On a single level that adds the same constant to every
index, so every tie broken on an index breaks the same way it did.

### Saving and loading

The save codec writes `World.layers` like any other slice of pointers: a nil
entry (a level never broken into) stays nil, and each layer's fields are
saved except those tagged `save:"-"` (the published grid and the chunk
preview). `afterLoad` gives each generated layer a fresh preview. A game
saved with two levels loads with both and plays on identically
(`TestSaveLoadKeepsLevels`).

### What a frontend sees

`Snapshot.LevelTiles` holds every level's published grid, by level;
`Snapshot.Tiles` is the landing level's, for consumers that only know one.
`TileAt`, `TerrainAt` and `ExploredAt` read the grid of the level the `Point`
is on, and `Levels` lists the levels that exist. Entities, scum, salt, storage
and fixtures are published for every level; their positions say where. The
terminal shows one level at a time; the browser shows the landing level only
for now (see [stairs.md](./stairs.md)).

## Why it is this way

- **`Point.Level`, not a separate `Loc`.** Phase Z0 added a `Loc` (a level and
  a point) for the places that cross levels, kept `Point` for grid code, and
  kept the landing level as `w.home` so that removing it would list every
  single-level assumption. Z1 found that nearly everything crosses levels: a
  miner's rock is below while it stands above, a colonist two levels down
  walks to a toilet on the landing level, ore comes up to storage. Converting
  each of those to `Loc` meant hundreds of conversions between two position
  types. Giving `Point` its level made every position, route and map key
  carry one for free, made neighbour arithmetic stay on its level without
  thinking, and turned every positional literal into a compile error until it
  said which level it meant. `w.home` was then removed as Z0 planned, and
  each of its 685 uses resolved to the layer of the point at hand, an
  aggregate over every layer, or `landing()` by design.
- **`Level` is an `int`, not an `int8`.** With an `int8`, `Point` carried
  seven bytes of padding, and Go cannot hash a struct with padding as one
  block of memory: profiles of every map keyed by `Point` (claims, refuse,
  scum, storage, the frontier) showed a field-by-field hash. As an `int` the
  24-byte key has no padding and hashes in one pass. Measured interleaved the
  difference was small (about 3% across the tick and pathfinding
  benchmarks, within this machine's noise), but it costs nothing.
- **What one-level games pay.** A `Point` is 24 bytes, not 16, so map keys
  hash slower and routes and queues are bigger. Against the commit before
  levels, interleaved, six runs each: about 6% slower by geometric mean over
  the tick, room-refresh, need-seeking and pathfinding benchmarks, with tile
  A\* the most affected (around 15% by median) and no single benchmark
  significant at this machine's noise. A first cut was nearly twice as slow;
  three things got it back: hoisting the layer lookup out of the searches'
  inner loops (`layerIn`, caching the current level's grids), asking for
  stair links only while a stair exists (`hasStairs` is
  `len(w.stairs) > 0`), and skipping the refuse lookup in `TileAt` when a
  level has none.
- **`noLayer` instead of nil.** Code that looks up a target before checking
  it (cleaning's unset `Point{-1, -1}`, which is on the surface) used to read
  an empty map entry and move on. A nil layer would have turned each of those
  into a panic; an empty one keeps them reading nothing.
- **Scratch is shared, not per layer.** The A\* cells, the facility search's
  stamps and the transit buffers live inside one call, so one set serves
  every level, indexed by `layered[T]`, rather than one set per layer.

## Extending it

- New grid state (indexed by (x, y)) and per-level counts go on `Layer`
  (initialised in `newLayer`); a sparse map keyed by `Point` goes on `World`,
  since its key already names the level; state that spans levels goes on
  `World`.
- Reach a layer's state through the point you have: `w.lay(p).thing`. An
  iteration over every level goes through `eachLayer` (or `eachContainer`,
  `eachFacility`), in level order.
- Use `Within`/`Adjacent` for "touching" and "in range", `travelEstimate` for
  "nearest", and `Chebyshev` only for a heuristic that must stay a lower
  bound.
- Every new `w.landing()` is a decision that something only happens on the
  landing level. Say why in a comment.
- If a new identity holds a `Point`, add it to `TestNothingIsOnTheSurface`.

## Related

- [stairs.md](./stairs.md): how levels connect, and how navigation crosses
  them.
- [z-levels.md](./z-levels.md): the plan this is phases Z0 and Z1 of.
- [determinism.md](./determinism.md): level-first ordering and per-level
  iteration.
- [sparse-grids.md](./sparse-grids.md): why a level's grids cost nothing
  until something is written to them.

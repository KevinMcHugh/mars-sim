# World & world generation

> Part of the [mars-sim documentation](./README.md).

## What it is

The world is a single underground level: a dense, row-major grid of `Tile`s that
starts as solid rock with ordinary, iron-bearing, or water ice-bearing
composition. World generation carves a landing cavern, drops the colonists inside
it, and seeds aliens out in the surrounding rock and cats/mice on the floor.

## Source

- [`internal/sim/world.go`](../internal/sim/world.go) — `Terrain`, `Tile`, `World`, tile/occupancy/entity accessors.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go) — `generate` and the placement helpers.
- [`internal/sim/geom.go`](../internal/sim/geom.go) — `Point`, Chebyshev distance, the 8-neighbor table.

## How it works

### Terrain and tiles

`Terrain` is an enum: `Rock`, `Floor`, `Wall`, `NutrientPod`, `Toilet`, `Bed`.
Only
`Floor` is `Walkable()` — colonists, cats, and mice stay on floor; **aliens ignore
walkability and burrow through anything**. Beds are dormitory bunks used from an
adjacent floor tile.

A `Tile` stores both `Terrain` and `RockComposition`. Composition is meaningful
only while the terrain is `Rock`: ordinary rock yields one `RawRock`, while
iron-bearing and water ice-bearing rock also yield one `IronOre` or `WaterIce`.
Keeping composition separate from terrain means all three deposits share the
same blocking, frontier, pathfinding, and excavation rules instead of multiplying
terrain cases throughout the simulation.

The grid is stored as a flat `[]Tile` of length `Width*Height`, indexed row-major
via `World.index(p)`. `TerrainAt` returns `Rock` for out-of-bounds cells so the
edge of the world reads as solid.

### Coordinates

`Point` is an integer grid coordinate; origin is top-left, X grows east, Y grows
south. Movement is 8-directional, so distances everywhere use **Chebyshev**
(king-move) distance, and `neighbors8` is the canonical 8-step table. `stepToward`
gives the single greedy step that most reduces Chebyshev distance (used by alien
burrowing and simple movement).

### World state

`World` is the mutable game state owned by the engine goroutine (see
[architecture.md](./architecture.md)). Alongside the tile grid it holds several
incrementally-maintained indexes so systems never rescan the map — the occupancy
index, terrain/kind counts, the chunk entity index, the region/room graph, the
job board, flow fields, and the projects list. Those are documented in
[spatial-index-and-performance.md](./spatial-index-and-performance.md),
[pathfinding.md](./pathfinding.md), and [construction.md](./construction.md).

The critical invariant lives in `SetTerrain`: it updates `terrainCounts`, marks
the tile's chunk dirty (for region recompute), and **emits a `TileChanged` event**
— which is how the job board and flow fields stay current. Always change terrain
through `SetTerrain`, never by writing `tiles` directly, or those derived systems
go stale.

### World generation

`generate` (called once by `NewEngine`):

1. Assigns rock composition using a dedicated RNG derived from the simulation
   seed. The configurable iron and ice percentages default to 10% and 5%; the
   remainder is ordinary rock.
2. Carves an **oval cavern** at the map center. `caveRadii` sizes it to the
   starting colonist count (~10 tiles per colonist) at a 2:1 width:height ratio,
   clamped to the map.
3. Places colonists by shuffling the list of free floor tiles and drawing from
   it, so every requested colonist is placed if the cavern has room (this beats
   rejection sampling, which can give up).
4. Places aliens on random rock tiles **far** from the cavern (`randomRockFar`),
   so they must burrow in.
5. Places mice and cats on random floor tiles inside the cavern.
6. Runs `refreshSpatial` once so regions/rooms exist before the first tick.

`randomTile` reservoir-samples a tile satisfying a predicate in one pass — uniform,
and it always finds a match if one exists.

## Why it is this way

- **Flat row-major grid** keeps tile access to one multiply-add and makes copying
  a snapshot a single `copy`.
- **Out-of-bounds reads as `Rock`** removes bounds-checking special cases from
  the many callers that ask "what's next to me?" — the world edge just behaves
  like solid wall.
- **Terrain changes funnel through `SetTerrain`** so the event-driven systems can
  be trusted; this is the linchpin of the reactive performance design.
- **Composition is tile data, not terrain** because deposits do not differ in
  walkability or mining cost. New terrain kinds would complicate every rock
  predicate and derived index for no gameplay benefit.
- **Composition has a separate seed-derived RNG stream** so generating deposits
  remains reproducible without shifting colonist placement, alien placement, or
  every later decision on the main simulation stream.
- **Shuffle-and-draw placement** guarantees the requested population actually
  spawns, which matters for reproducible, comparable runs.

## Extending it

- **A new terrain**: add a `Terrain` constant before `numTerrains`, decide its
  `Walkable()` result, give it a glyph in the TUI, and (if it is a facility)
  wire it into the needs table. Flow fields are allocated per facility terrain
  in `newWorld`.
- **A new rock composition**: add a `RockComposition`, its world-generation
  weighting, mining yield, and TUI glyph. Leave it out of `Terrain` unless it
  actually changes movement or construction rules.
- **Multiple levels (z-layers)** are the big planned extension; the region and
  flow-field machinery were built to extend into it. This is not implemented yet.

## Related

- [entities-and-ai.md](./entities-and-ai.md) — who lives on the grid and how they act.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the indexes over the grid.
- [construction.md](./construction.md) — how floor becomes walls and facilities.

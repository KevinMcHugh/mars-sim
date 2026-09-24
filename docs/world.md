# World & world generation

> Part of the [mars-sim documentation](./README.md).

## What it is

The world is a single underground level: a dense, row-major grid of `Tile`s that
starts as solid rock with ordinary, iron-bearing, water ice-bearing,
uranium-bearing, or clay-bearing composition. World generation carves a landing cavern,
hollows hidden natural caverns (some joined by passages) out of the rock
beyond, lands the colonists' crash pods in the landing cavern, and seeds aliens
out in the surrounding rock and cats/rats on the floor.

## Source

- [`internal/sim/world.go`](../internal/sim/world.go) — `Terrain`, `Tile`, `World`, tile/occupancy/entity accessors.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go) — `generate` and the placement helpers.
- [`internal/sim/caverns.go`](../internal/sim/caverns.go) — natural caverns and passages (see [caverns.md](./caverns.md)).
- [`internal/sim/geom.go`](../internal/sim/geom.go) — `Point`, Chebyshev distance, the 8-neighbor table.

## How it works

### Terrain and tiles

`Terrain` is an enum: `Rock`, `Floor`, `Wall`, `NutrientPod`, `Toilet`, `Bed`,
`Incinerator`, `Storage`. Only
`Floor` is `Walkable()`, and every creature, aliens included, stays on floor. Beds are dormitory bunks used from an
adjacent floor tile; the incinerator is the machine refuse is burned in, used the
same way (see [sanitation.md](./sanitation.md)). Storage is a blocking trunk used
from beside it; its large contents live in sparse world state rather than
`Tile` (see [storage.md](./storage.md)).

A `Tile` stores both `Terrain` and `RockComposition`. Composition is meaningful
only while the terrain is `Rock`: ordinary rock yields one `RawRock`, while
iron-bearing, water ice-bearing, uranium-bearing, and clay-bearing rock also
yield one `IronOre`, `WaterIce`, `UraniumOre`, or `Clay`. Keeping composition separate from terrain
means every deposit shares the same blocking, frontier, pathfinding, and
excavation rules instead of multiplying terrain cases throughout the simulation.

Uranium is the one composition that does something beyond its yield: standing
next to an unexcavated uranium deposit (or carrying the ore away from it) puts a
colonist under a dose that can eventually mutate them. It is also the scarcest
deposit, at 1% of rock against iron's 10%, because that dose is the point of it:
uranium is meant to be a hazard a colony stumbles onto, not a routine yield.
That lives entirely in `mutation.go` and reads the tile — the tile itself
behaves like any other rock. See [mutation.md](./mutation.md).

Two further fields hold what is *lying on* a tile rather than what it is made
of: `Gore` (a violent death's stains, see [combat.md](./combat.md)) and
`Corpses` (bodies waiting to be hauled off, see
[sanitation.md](./sanitation.md)). Raising a structure on a tile clears both;
digging one out does not.

Those two are **not stored per tile**. They describe the few hundred tiles
anything has ever died on, so they live in `World.refuse`, a sparse
`map[Point]refuseCell` — the same treatment placed storage containers get.
`Tile` is an assembled view: `World.tile` and `TileGrid.At` put it together
from the stored record plus the refuse index, so every reader still just sees a
`Tile`. See [sparse-grids.md](./sparse-grids.md).

`Explored` is the one field that is about the *player* rather than the tile: it
records that the colony has dug or built its way to within one tile of here, and
a frontend draws nothing on a tile that has not. It only ever goes from false to
true, and it is set only by `SetTerrain` — see
[fog-of-war.md](./fog-of-war.md).

The grid is stored as a flat `[]tileCell` of length `Width*Height`, indexed
row-major via `World.index(p)`. `tileCell` is terrain, composition and the
explored flag — **three bytes**, and deliberately so: this is the one structure
in the simulation that genuinely needs an entry per tile, since composition is
ore and worldgen threads veins through a fifth of the map. Every byte added
here is 95 MB on a 10000x10000 map, and twice that once the published grid
mirrors it, so `TestTileRecordStaysNarrow` fails if it grows. Everything else
that was once per-tile is sparse instead — see
[sparse-grids.md](./sparse-grids.md).

`TerrainAt` returns `Rock` for out-of-bounds cells so the edge of the world
reads as solid.

### Coordinates

`Point` is an integer grid coordinate; origin is top-left, X grows east, Y grows
south. Movement is 8-directional, so distances everywhere use **Chebyshev**
(king-move) distance, and `neighbors8` is the canonical 8-step table. `stepToward`
gives the single greedy step that most reduces Chebyshev distance (used for
simple movement).

### World state

`World` is the mutable game state owned by the engine goroutine (see
[architecture.md](./architecture.md)). Alongside the tile grid it holds several
incrementally-maintained indexes so systems never rescan the map — the occupancy
index, terrain/kind counts, the chunk entity index, the region/room graph, the
job board, flow fields, and the projects list. Those are documented in
[spatial-index-and-performance.md](./spatial-index-and-performance.md),
[pathfinding.md](./pathfinding.md), and [construction.md](./construction.md).

The critical invariant lives in `SetTerrain`: it updates `terrainCounts`, marks
the tile's chunk dirty (for region recompute), lifts the fog of war over the
tile and its eight neighbors (`revealAround`), and **emits a `TileChanged`
event** (worldgen's `carveHidden` is the same path minus the fog lift) — which is how the job board and flow fields stay current. Always change
terrain through `SetTerrain`, never by writing `tiles` directly, or those
derived systems go stale (and the map the player sees never grows).

### World generation

`generate` (called once by `NewEngine`):

1. Grows iron, water-ice, uranium, and clay deposits as meandering, occasionally branching veins using a
   dedicated RNG derived from the simulation seed. The configurable iron, ice, and
   uranium, and clay percentages default to 10%, 5%, 1%, and 5%; the remainder
   is ordinary rock. New compositions are appended to the generation order, so
   clay was grown last and did not move the iron, ice, or uranium veins of
   established seeds. Veins
   default to 8–24 orthogonally connected tiles. Then it seeds **cave scum**
   patches across `scum-percent` of the map's rock, on a stream of its own so
   that adding scum moved no vein (see [scumhouse.md](./scumhouse.md)).
2. Carves an **oval cavern** at the map center. `caveRadii` sizes it to the
   starting colonist count — ten tiles of elbow room per colonist plus the
   ground its crash pod takes, at a 2:1 width:height ratio, never less than
   `minCaveRy` (6) rows above and below the middle — clamped to the map. The
   pods' share and the taller minimum both exist so the colony can still site
   its first room once the pods are down; see [crash-pods.md](./crash-pods.md).
3. Hollows **natural caverns** out of the remaining rock with `carveHidden`, on
   their own seed-derived RNG stream, and joins some to their nearest neighbor
   with a passage. They stay under the fog, and out of every colony-facing
   system, until a dig breaks into one. See [caverns.md](./caverns.md).
4. Lands each colonist in a crash pod (`arrive`), in the lower half of the
   cavern, crashing through the rock once the open floor runs out.
5. Places aliens with `alienSpawnSite`: on hidden cavern floor, where they lie
   dormant until the colony digs in, or, with no cave room, on colony floor far
   from the landing site. See [caverns.md](./caverns.md#aliens-in-the-caves).
6. Places rats and cats by shuffling the free floor tiles in and around the
   cavern and drawing from the list (this beats rejection sampling, which can
   give up).
7. Records each natural cavern's center (`trackCavernsForNests`), so that
   breaking into it later can roll for an **alien nest**. No nest aliens exist
   before then. See [caverns.md](./caverns.md#alien-nests).
8. Runs `refreshSpatial` once so regions/rooms exist before the first tick.

`randomTile` reservoir-samples a tile satisfying a predicate in one pass — uniform,
and it always finds a match if one exists.

## Why it is this way

- **Flat row-major grid** keeps tile access to one multiply-add on the hot path
  the simulation walks constantly. Frontends see the same grid through a paged,
  page-shared copy instead, so publishing a frame does not re-copy the map — see
  [snapshot-tile-grid.md](./snapshot-tile-grid.md).
- **Out-of-bounds reads as `Rock`** removes bounds-checking special cases from
  the many callers that ask "what's next to me?" — the world edge just behaves
  like solid wall.
- **Terrain changes funnel through `SetTerrain`** so the event-driven systems can
  be trusted; this is the linchpin of the reactive performance design. Fog of war
  hangs off the same funnel, which is what makes revealing the map cost nothing
  per tick (see [fog-of-war.md](./fog-of-war.md)).
- **Composition is tile data, not terrain** because deposits do not differ in
  walkability or mining cost. New terrain kinds would complicate every rock
  predicate and derived index for no gameplay benefit.
- **Composition has a separate seed-derived RNG stream** so generating deposits
  remains reproducible without shifting colonist placement, alien placement, or
  every later decision on the main simulation stream.
- **Deposits grow as branching random walks** rather than rolling each tile
  independently. This produces narrow, irregular veins, keeps each generated
  vein connected, and makes finding one deposit useful information about nearby
  tiles, while still meeting the configured map-wide abundance target exactly.
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
- **Different deposit shapes**: adjust `RockVeinMin` / `RockVeinMax` for coarse
  clustering. Change `growRockVeins` only when the topology itself should change;
  doing so intentionally changes generated maps for existing seeds.
- **Multiple levels (z-layers)** are the big planned extension; the region and
  flow-field machinery were built to extend into it. This is not implemented yet.

## Related

- [entities-and-ai.md](./entities-and-ai.md) — who lives on the grid and how they act.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the indexes over the grid.
- [construction.md](./construction.md) — how floor becomes walls and facilities.
- [lore.md](./lore.md) — the alien species roster this seed's worldgen
  rolls, layered on top of the aliens `generate` places here.

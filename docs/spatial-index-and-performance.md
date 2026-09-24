# Spatial index & performance

> Part of the [mars-sim documentation](./README.md).

## What it is

The engine is built to stay cheap as the colony scales. The unifying idea is
**never rescan the world**: every "how many / who / where / what's diggable"
question is answered by an index that is maintained *incrementally* as things
change, driven by the event bus. This doc collects those indexes and the
performance results they produced.

## Source

- [`internal/sim/world.go`](../internal/sim/world.go) — the occupancy index (`occ`), terrain/kind counts.
- [`internal/sim/chunks.go`](../internal/sim/chunks.go) — the chunk entity index.
- [`internal/sim/jobboard.go`](../internal/sim/jobboard.go) — the mining frontier and in-progress build counts.
- [`internal/sim/events.go`](../internal/sim/events.go) — the bus these systems react to.
- [`internal/sim/tilegrid.go`](../internal/sim/tilegrid.go) — the page-shared grid published in each `Snapshot`.
- [`internal/sim/rooms.go`](../internal/sim/rooms.go), [`flowfield.go`](../internal/sim/flowfield.go), [`path.go`](../internal/sim/path.go) — covered in [pathfinding.md](./pathfinding.md).
- [`internal/sim/bench_test.go`](../internal/sim/bench_test.go) — `BenchmarkStep`.

## How it works

### Occupancy index

`World.occ` holds the `EntityID` standing on a tile (0 = empty; IDs start at 1).
It makes "who is here?" an O(1) lookup instead of an O(entities) scan, and it is
the mechanism that enforces **one entity per tile**. `moveEntity`, `spawn`, and
`remove` keep it in step.

It is a `pagedGrid`, not a dense slice: entities stand on a vanishing fraction
of a big map, and 0 — what an unwritten page reads as — already means empty. See
[sparse-grids.md](./sparse-grids.md).

### Incremental counts

`terrainCounts[numTerrains]` and `kindCounts[numKinds]` answer "how many floor
tiles? how many colonists?" in O(1). `SetTerrain` adjusts terrain counts on every
change; `spawn`/`remove` adjust kind counts. Nothing ever rescans the grid or the
entity map to count.

### Chunk entity index

The map is divided into 16x16 **chunks** (`chunkSize`). `chunkEntities[ci]` buckets
entity IDs by chunk, so neighbor queries scan only nearby chunks. `nearestOfKind`
(see [entities-and-ai.md](./entities-and-ai.md)) expands in chunk rings and stops
as soon as the next ring cannot beat the best candidate. Chunks also bound region
recomputation (see [pathfinding.md](./pathfinding.md)). Buckets use swap-delete
since order within a bucket does not matter (queries tie-break on ID).

`entityIDsNearSorted` is the other chunk query: every entity within a square
radius, in ascending ID order. `observeNearby` (a colonist noticing aliens and
rats) uses it. It used to copy and sort *every* entity ID once per colonist per
tick, so a 500-colonist tick did 500 full sorts; see the table below. Sorting
only the few nearby hits keeps the same visiting order, and so the same
memories and stimuli in the same order, which determinism needs (see
[determinism.md](./determinism.md)).

### The job board (mining frontier)

The `jobBoard` tracks the **mineable frontier** — rock tiles bordering floor — so
mining needs no per-colonist map scan. It is maintained incrementally from
`TileChanged` events: when a tile changes, only it and its 8 neighbors are
re-evaluated for frontier membership. Claims are **owner-keyed** and made on
arrival, keeping two colonists off one rock and making release safe. The board
also keeps O(1) counts of builds in progress per terrain (`startBuild`/`endBuild`/
`inProgress`), which is how `plannedFacilities` avoids scanning colonists.

Claiming or releasing a frontier tile, or a tile joining or leaving the
frontier, touches the frontier flow field there, so other miners route around a
claimed rock (the field repairs around touched tiles; see
[pathfinding.md](./pathfinding.md)).

The board also holds the **cleaning claims** (`claimClean`/`releaseClean`): the
refuse tile each cleaner is walking to, so the colony does not converge on one
splatter. There is no maintained refuse *set* to go with them — refuse is rare
and scattered and cleaners only search a radius, so the frontier's incremental
bookkeeping would cost more than it saved. See
[sanitation.md](./sanitation.md).

### Publishing a frame

The same rule governs the engine's output: a `Snapshot` does not re-copy the map.
`Snapshot.Tiles` is an immutable page-shared grid that re-copies only the pages
a tick actually changed, and the terrain totals in `Stats` are read off
`terrainCounts` rather than counted by walking the grid. Before that, publishing
was the *entire* per-tick cost of a large map — 79 ms and 49 MB a frame at
7000x7000, against 16 µs for the tick itself — and it scaled with the map's area
rather than the colony. See [snapshot-tile-grid.md](./snapshot-tile-grid.md).

### The reactive backbone

All of this hangs off the synchronous event bus (see
[architecture.md](./architecture.md)). `newWorld` subscribes the job board and the
flow fields (each `touch`ed at the changed tile) to `TileChanged`. Producers emit only on real changes
(`SetTerrain` no-ops on unchanged terrain), because boxing a `WorldEvent` allocates.

## Why it is this way

The naive scaffold rescanned the world for everything, which was fine at a handful
of colonists and catastrophic at scale. Each index removed a class of scans, and
the effects compound. Measured on stress runs:

| Colonists | Naive scans | With occupancy index & counts |
| --- | --- | --- |
| 500 | ~2.5 s/tick | ~8 ms/tick |
| 2000 | ~42 s/tick | ~144 ms/tick |

Cumulative effect of the reactive work on a 2000-colonist stress tick:

| Stage | ms/tick |
| --- | --- |
| Baseline (naive scans) | ~42000 |
| + occupancy index & counts | ~144 |
| + chunk index, rooms, board | ~43 |
| + lazy needs & resting AI | ~13 |

Per-colonist full-entity sorts in `observeNearby`, replaced by
`entityIDsNearSorted` (same seed, same simulation, before → after):

| Benchmark | ms/tick before | after |
| --- | --- | --- |
| `BenchmarkStep500` | 34.6 | 15.6 |
| `BenchmarkStepMixed500` | 23.0 | 9.0 |
| `BenchmarkStepSmallColonyOnHugeMap10000` | 1.49 | 1.21 |

Map size is the other axis, and it used to be the one that bit: with the colony
held fixed, a fresh 6-colonist game published one frame per tick and paid for the
whole grid every time.

| Map | Per published frame, before | After |
| --- | --- | --- |
| 5000x5000 | 41 ms, 25 MB | 33 µs, 19 KB |
| 7000x7000 | 79 ms, 49 MB | 34 µs, 28 KB |

Lazy needs and the resting AI (see [needs.md](./needs.md)) matter here too: a
colonist's needs are computed on read, so an idle colonist rests instead of
re-scanning the map every tick. Flow fields and HPA\* (see
[pathfinding.md](./pathfinding.md)) carry the win into the thousands of agents.

## Extending it

- **A new aggregate query**: prefer an incremental counter updated at the mutation
  site over a per-tick scan. Add it to `World`, update it in `SetTerrain` /
  `spawn` / `remove` / the relevant job transition.
- **A new derived system**: subscribe to the event bus in `newWorld` and maintain
  your own incremental state from events (the job board is the model to copy).
- **Measure it**: `go test ./internal/sim/ -run '^$' -bench BenchmarkStep -benchmem`.

## Related

- [architecture.md](./architecture.md) — the event bus and the ownership model.
- [snapshot-tile-grid.md](./snapshot-tile-grid.md) — why publishing a frame no longer copies the map.
- [pathfinding.md](./pathfinding.md) — chunks, regions/rooms, and flow fields.
- [needs.md](./needs.md) — lazy needs and the resting AI.
- [world.md](./world.md) — `SetTerrain` and the grid these indexes shadow.

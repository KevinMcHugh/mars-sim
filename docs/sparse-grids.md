# Sparse grids

> Part of the [mars-sim documentation](./README.md).

## What it is

`pagedGrid[T]` is a map-sized 2D array whose storage is allocated one 64x64
**page** at a time, on first write. A page nothing has written reads as the zero
value of `T` and costs one nil slice header.

Every navigation structure in the simulation is one of these. It is why a
10000x10000 world costs ~590 MB instead of ~16.4 GB.

## Source

- [`internal/sim/pagedgrid.go`](../internal/sim/pagedgrid.go) — `pagedGrid[T]`, `at` / `set` / `ptr`, `pageAt`, `interiorPage`.
- [`internal/sim/pagedgrid_test.go`](../internal/sim/pagedgrid_test.go) — the addressing, and the sparsity guard that fails if a grid starts allocating with map area.
- [`internal/sim/flowfield.go`](../internal/sim/flowfield.go), [`path.go`](../internal/sim/path.go), [`rooms.go`](../internal/sim/rooms.go), [`world.go`](../internal/sim/world.go) — the callers.

## Why it is this way

A big colony on a big map, left running, used **16.59 GB**. Almost none of it
was leaked, and almost none of it was tick-driven: it was all allocated before
the first tick ran, and `make()` hands back untouched zero pages, so residency
climbed over ~150k ticks as the colony paged it in. The size was there from
`newWorld`.

It came from 172 bytes of per-tile side structures, on a map of 100M tiles:

| structure | B/tile | MiB |
| --- | ---: | ---: |
| 6 x `flowField` — `dist` + `seen` + `transitSeen` | 72 | 6,866 |
| `pathfinder` `g` / `from` / `seen` / `corridorSeen` (`[]int`) | 32 | 3,052 |
| `World.tiles []Tile` | 24 | 2,289 |
| published `TileGrid` pages | 24 | 2,289 |
| `occ []EntityID` | 8 | 763 |
| `facilityDist` + `facilityDistGen` | 8 | 763 |
| `regionOf []RegionID` | 4 | 381 |
| **total** | **172** | **16,403** |

The colony that prompted this had excavated 174,403 tiles — **0.17%** of the
map. Every one of those navigation structures only ever holds a meaningful
value on a walkable tile, so more than 99% of all that memory was permanently
zero. That is the whole observation: they are dense arrays over the world, but
what they describe is the colony.

### Why zero has to mean "nothing known"

Paging only works because an unwritten page reads as useful. Each grid was
already built that way, which is why this was a storage change and not a
redesign:

- Flow fields and the facility BFS stamp a **generation** per cell and compare
  it to the current one. `rebuild` pre-increments, so a live generation is
  always >= 1 and an all-zero page reads as unreachable for free.
- `regionOf` uses 0 for "not floor / no region".
- `occ` uses 0 for "empty" — entity IDs start at 1.
- `corridorSeen` is another generation stamp.

Nowhere did a zero page have to be made to mean something it did not already.

### Square pages, not row-major runs

This is the one place the paged grids differ from the published tile grid (see
[snapshot-tile-grid.md](./snapshot-tile-grid.md)), which pages row-major. That
grid memcpys whole pages, so contiguity is what it wants. These are written a
tile at a time by things that spread outward from the colony, so what matters
is how much of a page a blob-shaped colony actually uses.

On a 10000-wide map a 4096-entry row-major page is a strip two fifths of a row
long. A 500x500 colony would touch one or two per row it spans — ~750 pages for
250k tiles. The same colony fits in **64** square pages.

### What is left, and why

`World.tiles` stays dense. It is the one structure here that genuinely is:
`Composition` is ore, and worldgen threads veins through 21% of the map, so
every page has something in it. There is no sparsity to exploit, only width to
cut — which is what the `tileCell` split did (see below). The remaining ~590 MB
is 3 bytes of terrain/ore/fog per tile, mirrored once so frontends can read a
frame lock-free.

Going lower means making composition lazy, and it cannot be: `growRockVeins` is
a sequential random walk over the whole map, so generating a page on demand
would change the layout every existing seed produces. That is a gameplay
change, not a refactor.

## How it works

### Addressing

```
page  = (y >> 6) << colShift | (x >> 6)
slot  = (y & 63) << 6 | (x & 63)
```

The page table is rounded up to a power-of-two width so the row index is a
shift rather than an `IMUL`. The slack is empty slice headers — 965 KB instead
of 592 KB on a 10000x10000 map — against a multiply on the hottest read in the
simulation.

### The fast paths

Naively, paging cost **+27% on the tick** and **+83% on pathfinding**. Most of
that came back. Measured against the same commit's parent, 10000x10000,
medians of paired runs:

| Benchmark | Before | After |
| --- | ---: | ---: |
| `StepBigColonyOnHugeMap` (160k tiles open — the real case) | 18.73 ms | 18.96 ms (+1%) |
| `StepSmallColonyOnHugeMap10000` | 1.134 ms | 1.149 ms (+1%) |
| `Pathfind` | 16.5 us | 17.4 us (+5%) |
| `RoomRefresh` | 35.2 us | 39.2 us (+11%) |
| `PublishSmallColonyOnHugeMap10000` | 4.59 ms | 1.41 ms (**-69%**) |
| peak heap, fresh world + 3000 ticks | 16,413 MB | 594 MB (**-96%**) |

`RoomRefresh` is the one that did not fully come back, and it is ~3% of a tick,
so ~0.3% of one. Publishing got *faster* for an unrelated reason: the tile
record is a quarter the width, so a dirty page copy is a quarter the bytes.

Three things got the time back, and none of them were micro-tuning:

- **`interiorPage(x, y)`** returns the page holding `(x, y)` when all eight of
  its neighbours are in it too, and nil on a page edge. The 8-connected
  searches — `flowField.rebuild`, `chooseFacility` — do one lookup per *node*
  instead of nine for the ~94% of nodes that are not on an edge.
- **`pageAt(x, y)`** hoists the lookup out of a whole-chunk sweep. A page holds
  a whole number of chunks and chunks are aligned, so every tile of a chunk is
  in the page its origin is (`pagedgrid.go` fails to compile if `chunkSize`
  stops dividing `gridPageSide`). `paintCorridor` and the region re-flood use
  it, and both get a "nil page, skip the chunk" fast path out of it.

- **Taking the BFS depth from the queue's layering** instead of reading the
  current cell's distance back. A uniform-cost BFS visits in non-decreasing
  distance order, so the depth is the layer. One fewer read per node, and
  nothing to do with paging.

### The mistake that cost the most

None of the above was the big one. Moving to paged storage, `rebuild`'s inner
test got reordered so walkability was checked before the generation stamp — to
stop a rock neighbour allocating a page for it. That looks harmless and it cost
**23% of the tick on a big colony**, more than paging itself ever did.

The reason is that in an open room most neighbours are *already stamped this
generation*, so the original order short-circuits on the stamp and never reads
the tile. The stamp is a read of a page the node is already holding; the terrain
read is a scattered hit on the dense tile array a row-stride away. Reordering
turned a skipped read into eight of them per node.

The fix is to keep the original order wherever the page is already in hand
(`interiorPage` returned it, so no allocation can happen) and only check terrain
first on the page-edge path, where a page genuinely might have to be allocated.
The lesson generalizes: when a condition guards an allocation, check *where the
allocation can actually happen*, not everywhere.

### An earlier attempt that did not work

Memoizing the last page lookup (`lastIdx`/`lastPage` on the grid) looked
obviously right — the callers are all spatial — and measured as noise, then
slightly negative. The hit path is a compare plus a slice-header load, which is
barely cheaper than the lookup it replaces, and every miss pays 32 bytes of
stores into the grid struct. Hoisting the lookup to where the *caller* knows the
page cannot change beat it outright. It was removed.

### The tile record

Separately, `Tile` was 24 bytes for what is really three:

- `Gore` and `Corpses` were word-sized `int`s, which padded the struct. They
  describe the few hundred tiles anything has ever died on, so they moved to
  `World.refuse`, a sparse `map[Point]refuseCell` — the same treatment
  `storageContainers` already had. `refuseCell` is a `uint8` and a `uint16`.
- What is left is stored as `tileCell`: terrain, composition, explored. Three
  bytes.

`Tile` still exists with all five fields and is what every caller reads; it is
assembled from `tileCell` plus the refuse index (`World.tile`, `TileGrid.At`).
Nothing outside `world.go` and `tilegrid.go` noticed.

## Extending it

- **A new per-tile array is a `pagedGrid`** unless you can say why every tile
  needs one. Give it a `T` whose zero value already means "nothing here"; if you
  find yourself wanting a sentinel like -1, use a generation stamp instead.
- **Sweeping a chunk?** Take `pageAt` once outside the loop and index with
  `offset(x, y)`. **Expanding 8-connected?** Take `interiorPage` per node and
  fall back to `ptr` on an edge.
- **`ptr` allocates**; `at` and `pageAt` do not. Order your conditions so the
  cheap dense test comes first — `flowField.rebuild` checks walkability before
  the stamp for exactly this reason, which is what keeps a field's pages to the
  reachable area instead of a one-tile border around it.
- **Pages are never freed.** Terrain only ever opens up, so a page written is a
  page the colony reached. If something ever makes terrain close back up, that
  assumption is the one to revisit.
- **The sparsity is a test, not a hope.** `TestPagedGridsStaySparse` builds a
  small colony on a huge map and fails if any grid has allocated more pages than
  the colony can account for. A change that reintroduces a full-map walk over
  one of these grids will trip it.

## Related

- [snapshot-tile-grid.md](./snapshot-tile-grid.md) — the published tile grid, which pages row-major for the opposite reason.
- [pathfinding.md](./pathfinding.md) — the flow fields, A\* scratch and region labels that live in these grids.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the occupancy index and the rest of the "never rescan the world" story.
- [world.md](./world.md) — `Tile`, `tileCell`, and the refuse index.

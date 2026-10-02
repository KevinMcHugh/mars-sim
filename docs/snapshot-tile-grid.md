# Published tile grid

> Part of the [mars-sim documentation](./README.md).

## What it is

The terrain a frontend reads out of a `Snapshot`. It is **not** a copy of the
map: it is an immutable grid stored as fixed-size pages, and each published
frame re-copies only the pages whose tiles changed since the last one, sharing
every other page with the frames already in frontends' hands. That is what keeps
the cost of publishing a frame proportional to what the tick *touched* rather
than to how big the map *is*.

## Source

- [`internal/sim/tilegrid.go`](../internal/sim/tilegrid.go) — `TileGrid`, the page table, `World.publishedTiles`, and the `TileSharing` / `TileChanges` types.
- [`internal/sim/engine.go`](../internal/sim/engine.go) — `Engine.ShareLiveTiles`, the opt-in for the live mode below.
- [`internal/sim/world.go`](../internal/sim/world.go) — `SetTerrain` (and `reveal`) mark the changed tile's page dirty.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `Snapshot.Tiles` / `Snapshot.TerrainAt`.
- [`internal/sim/tilegrid_test.go`](../internal/sim/tilegrid_test.go) — stability across later edits, page sharing, a concurrent-reader run for `-race`, `TileChanges` in both modes, live aliasing, switching modes, and the `ShareLiveTiles` guards.
- [`internal/sim/bench_test.go`](../internal/sim/bench_test.go) — `BenchmarkPublishSmallColonyOnHugeMap2500` / `...10000`, and `BenchmarkFirstPublishHugeMapCopyOnWrite` / `...Live` for the memory the first frame retains.

## How it works

`Layer.tiles` is a `pagedGrid[tileCell]`: 64×64 square pages, the same as
every other per-tile grid (see [sparse-grids.md](./sparse-grids.md)), and one
page per worldgen chunk (see [worldgen-chunks.md](./worldgen-chunks.md)). The
published grid mirrors that page table exactly:

| Piece | Role |
| --- | --- |
| `TileGrid.pages` | `[]*tilePage`, laid out like the world's page table; each entry a copy of one world page, or nil where the world has never written one (reads as unexplored Rock) |
| `TileGrid.refuse` | the published copy of the sparse gore/corpse index, shared between frames until it changes |
| `Layer.snapGrid` | the grid handed to the most recent `Snapshot` |
| `Layer.pageDirty` / `dirtyPages` | pages whose tiles changed since the last publish |
| `Snapshot.TileChanges` | those pages (and whether refuse changed), handed to the frontend |

`SetTerrain` — the only writer of a tile's terrain — calls `markTilePageDirty`,
which is an array write and (first time per page) an append. (`World.reveal` is
the one other writer of `tiles`, for the fog-of-war flag, and does the same.) `publishedTiles` then has
three cases:

1. **No grid yet** (the first frame, after worldgen): copy every page the world
   has, and leave the rest nil.
2. **Nothing changed**: return `snapGrid` unchanged. The new `Snapshot` shares
   the previous frame's grid outright and publishing costs nothing. This is the
   common case — digging one rock takes `-mine-ticks` ticks, so most ticks
   change no terrain at all.
3. **Something changed**: copy the page *table* (pointers only), replace the
   dirty entries with fresh copies of those pages, and keep the rest shared.

Case 3 is the copy-on-write step, and the order matters: a page is copied
*before* the grid that published it could ever observe the change, so no
`TileGrid` mutates under a reader. Grids already published keep the old table,
and with it the pre-change pages.

Every snapshot also carries `TileChanges`: `All` on the first frame, otherwise
the page table indexes that changed (`TileGrid.PageOrigin` turns one into the
top-left tile of its `TilePageSide`-square page) and whether refuse did. A newly
generated chunk counts as a changed page. A frontend that redraws from scratch
can ignore it; one that forwards terrain incrementally (the browser's wire
encoder) sends exactly those pages.

The catch is that the delta is against the previous snapshot the world *built*,
not the one the consumer *saw*: publishing clears the dirty list every time. A
`Subscribe` channel drops stale frames, so a channel reader applying deltas
would silently lose the pages changed in a frame it never received. That is why
`TileChanges.Frame` numbers the snapshots: a consumer that last applied frame F
and gets anything other than F+1 missed something and must reread every page it
cares about, as if `All` were set. An incremental consumer should be one that
sees every frame, like the same-goroutine encoder below.

### Live sharing, for a same-goroutine frontend

The copy-on-write grid exists because the TUI reads a frame on its own goroutine
while the engine keeps ticking. A frontend that reads frames on the *engine's*
goroutine, between ticks, gets nothing from it, and it costs a second copy of
every generated chunk. The browser worker is that frontend: its wire encoder
runs in the same thread as the engine (see the browser frontend proposal,
`docs/browser-frontend.md`).

`Engine.ShareLiveTiles()` (or `World.SetTileSharing(TilesLive)`) switches the
world to `TilesLive`. `publishedTiles` then builds one grid whose page entries
point at the world's own pages and whose `refuse` is `Layer.refuse` itself, and
hands that same grid out on every frame. When a chunk is generated later, its
page arrives dirty and publishing points the grid's empty slot at it; pages are
never reallocated, so that is the only upkeep. Nothing is copied; the dirty list
is only reported, through `TileChanges`. Readers keep using `TerrainAt` /
`Tiles.At` unchanged. The price is that the frame is not immutable: its terrain
is only correct until the next step. Two things keep that from leaking into the
TUI:

- `ShareLiveTiles` and `Subscribe` panic if combined, in either order, because a
  subscription channel is how a frame reaches another goroutine. `ShareLiveTiles`
  also panics once `Run` has started.
- `TilesCopyOnWrite` stays the zero value, so every existing caller is unchanged.

**What it saves is now small.** This mode was written against eager worldgen,
where the second copy was the whole map: 287 MiB at 10000x10000, and 1032 MiB of
heap against 746 MiB with live sharing. Chunked lazy worldgen
([worldgen-chunks.md](./worldgen-chunks.md)) then made the published copy track
generated chunks instead of map area. Measured after that change, 10000x10000,
seed 7, default config:

| | Copy-on-write | Live |
| --- | --- | --- |
| Heap after first publish (30 chunks generated) | 28 MiB | 27 MiB |
| Heap after 3000 ticks (36 chunks) | 29 MiB | 28-29 MiB |
| Retained by the first frame (`BenchmarkFirstPublishHugeMap*`) | 419 KiB | 371 KiB |

The difference is 12 KiB per generated chunk (one 64x64 page of 3-byte cells);
the rest is the page table, which both modes hold. It grows with how much of the
world the colony has explored, never with the map's area. Keep the mode for a
consumer that needs every frame anyway, and because it costs nothing, but it is
not a memory fix any more.

Page identity cannot tell a live consumer what changed (live pages never move),
which is why `TileChanges` exists rather than asking consumers to compare page
pointers.

Frontends read through `Snapshot.TerrainAt(p)` / `TileAt(p)` (or `Tiles.At(p)`);
all return `Rock` out of bounds so a camera can walk off the edge of the world.
A page that is nil because its chunk has not been generated reads as
unexplored Rock through `Tiles`. With fog off, `Snapshot.TileAt`/`TerrainAt`
read it from a preview instead (see
[worldgen-chunks.md](./worldgen-chunks.md#previewing)). Tests and
alternative frontends can build a standalone grid with `NewTileGrid`.

Terrain totals in `Stats` (`FloorDug`, `Pods`, `Toilets`, `Beds`) come from the
incremental `terrainCounts` (`FloorDug` less `hiddenFloor`, the undiscovered
cavern floor — see [caverns.md](./caverns.md)) (see
[spatial-index-and-performance.md](./spatial-index-and-performance.md)), not
from walking the published grid — counting by walking would put the map's whole
area straight back onto every tick.

## Why it is this way

Publishing used to be `make([]Tile, len(w.tiles))` plus a `copy`, once per tick.
That is invisible on an 80x40 map and ruinous on a big one: **the entire per-tick
cost of a large map was the snapshot**, and it scaled with area, not with the
colony. Measured on a fresh 6-colonist game (`-tps 60`, one published frame per
tick):

| Map | Before | After |
| --- | --- | --- |
| 5000x5000 | 41 ms/frame, 25 MB/frame | 33 µs/frame, 19 KB/frame |
| 7000x7000 | 79 ms/frame, 49 MB/frame | 34 µs/frame, 28 KB/frame |

The `step()` half of the tick was already flat at ~16 µs for both sizes, so the
2x area from 5000 to 7000 was showing up as a 2x tick rate drop entirely through
`snapshot()`: 45 ticks/s against 22 ticks/s in a real headless run, both now
pinned at the 60/s cap. Peak RSS at 7000x7000 fell from 4.9 GB to 88 MB, because
the per-tick copy was touching (and handing the GC) 49 MB of fresh pages every
tick, while the world's own large arrays stay mostly untouched zero pages.

Approaches that were considered and dropped:

- **Page the live `Layer.tiles` too.** First rejected: the simulation reads
  terrain far more often than it publishes, and every read would pay a shift
  and an indirect load to save a copy that only happens once per frame. Then
  adopted, because lazy world generation needs a tile grid that costs nothing
  where no chunk has been generated. The read cost turned out small, and the
  searches that hoist the page lookup (A\*, flow-field rebuilds, the region
  flood) got faster, because a node's eight neighbours now share its page:

  | Benchmark | Dense tiles | Paged tiles |
  | --- | ---: | ---: |
  | `Pathfind` | 13.9 µs | 12.6 µs |
  | `StepBigColonyOnHugeMap` | 186 µs | 189 µs |
  | `StepSmallColonyOnHugeMap10000` | 201 µs | 209 µs |
  | `RoomRefresh` | 34.9 µs | 35.1 µs |
  | `PublishSmallColonyOnHugeMap10000` | 288 µs, 650 KB | 286 µs, 422 KB |

  The small-colony tick is the one that pays: its terrain reads are scattered
  one at a time through `at`, about 5% of the tick.
- **Ship only the camera's viewport.** Cheapest possible frame, but it puts the
  camera in the engine (a new command, a round trip per pan) and quietly breaks
  any frontend that wants the whole map — a minimap, a zoomed-out view, a test
  that reads a far corner. The snapshot stops being a picture of the world.
- **Reuse one tile buffer between frames.** Needs the engine to know when a
  frontend is done with a frame; with drop-stale-frames publishing and a
  renderer on another goroutine, that is exactly the ownership question the
  immutable-snapshot design exists to avoid.

The page is 64×64 (`gridPageBits`), shared with every paged grid and with
worldgen chunks, so it is no longer a knob of its own. The page table *is*
copied on every frame that changed anything, which is why it holds 8-byte
array pointers instead of 24-byte slice headers: a 10000x10000 map's table is
~40k entries (rounded up to a power-of-two width, as in `pagedGrid`), about
320 KB a frame, against the 300 MB it replaced. Row-major 4096-tile pages,
the first design, had a smaller table (~24k entries) but a colony-shaped
change dirtied a strip per row instead of one square.

## Extending it

- **Adding a field to the tile record** needs nothing here: pages are
  `[]tileCell`, so they grow with the struct. But think hard before you do.
  `tileCell` is three bytes and `TestTileRecordStaysNarrow` fails if it grows,
  because a byte here is 95 MB on a 10000x10000 map and then 95 MB again in
  these pages. If the new field is only true of *some* tiles — refuse was the
  worked example, at two of the original 24 bytes for something true of a few
  hundred — it belongs in a sparse index instead, like `Layer.refuse` or
  `storageContainers`. See [sparse-grids.md](./sparse-grids.md).
- **A new mutator of `tiles`** must call `markTilePageDirty`, or frontends will
  render stale terrain. `SetTerrain` is the only writer of a tile's *terrain* and
  the only place that emits `TileChanged`; keep it that way. `World.reveal`
  (fog of war — see [fog-of-war.md](./fog-of-war.md)) writes the explored flag
  and dirties the page without an event, since nothing derived from terrain
  reads that flag.
- **A new sparse index** published alongside the pages follows `Layer.refuse`:
  bump a revision on every write, and hand the previous frame's copy back when
  the revision has not moved (`publishedRefuse`). That is what lets refuse reach
  frontends without dirtying a whole page every time something dies.
  `TestPublishedRefuseReachesFrontendsWithoutTerrainChange` is the guard.
- **A new aggregate in `Stats`** should come from an incremental count, not from
  a walk of the grid. The scan this doc replaced is the cautionary tale.
- The invariant to preserve: **a page that has been published is never written
  again**. Copy first, then change the copy. The one exception is `TilesLive`,
  which publishes the live pages on purpose and is only allowed where no frame
  crosses a goroutine; don't loosen the `Subscribe` guard.
- **A new same-goroutine frontend** (a WebSocket server driving the browser UI
  from a native engine would be one, if it encodes on the engine goroutine)
  opts in with `ShareLiveTiles` and reads `TileChanges`. A frontend that reads
  frames anywhere else must not.

## Related

- [fog-of-war.md](./fog-of-war.md) — `Tile.Explored`, which rides these pages to frontends.
- [architecture.md](./architecture.md) — the snapshot/command contract this fits into.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the incremental counts and the rest of the "never rescan the world" story.
- [world.md](./world.md) — the tile grid itself and `SetTerrain`.

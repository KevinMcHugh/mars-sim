# Browser frontend (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan for running mars-sim in a browser tab. The Go engine is compiled to
WebAssembly and runs in a **Web Worker**. A new TypeScript/Svelte UI runs on the
main thread, and a WebGL canvas draws the map. It is the same contract as every
other frontend ([architecture.md](./architecture.md)): the engine publishes
snapshots, the frontend sends commands. The difference is that a
`postMessage` boundary now sits between the two, where a Go channel used to be.

Nothing here is built yet. It is a sketch to argue with before any code is
written. The numbers in it come from a spike run against the current tree, not
from guesses.

## Source

Nothing yet. The proposed layout:

- `cmd/mars-sim-wasm/` — the `GOOS=js GOARCH=wasm` entry point. It owns the
  worker's message loop and never imports the TUI or Bubble Tea.
- `internal/wire/` — turns a `*sim.Snapshot` into wire messages and decodes
  commands. It is plain Go, so it is tested natively. It knows nothing about JS.
- `web/` — Vite + Svelte 5 + TypeScript.
  - `web/src/sim/` — the worker bootstrap, `SimClient`, and the frame decoder.
  - `web/src/map/` — the WebGL map renderer. It is framework-free.
  - `web/src/views/` — the Svelte panels: roster, job board, storage, lore, perf.

## How it works

```
 main thread                                   worker
┌──────────────────────────────────────┐      ┌─────────────────────────────┐
│ Svelte shell (tabs, panels, dialogs) │      │ JS driver loop              │
│        ▲  topic stores, ~4-10 Hz     │      │   │ advance(now, budget)    │
│        │                             │      │   ▼                         │
│ SimClient ── decode ──▶ FrameStore ──┼──┐   │ Go/WASM: sim.Engine         │
│    │                       │         │  │   │   │ *Snapshot               │
│    │ commands, interest,   ▼ rAF     │  │   │   ▼                         │
│    │ subscriptions    MapRenderer    │  │   │ internal/wire encoder       │
│    │                  (WebGL2)       │  │   │   (only subscribed topics)  │
└────┼─────────────────────────────────┘  │   └──────────────┬──────────────┘
     │         postMessage (JSON)         │                  │
     └────────────────────────────────────┼─────────────────▶│
                                          └──────────────────┘
                     postMessage (binary frame, ArrayBuffer transferred)
```

### 1. The engine in a worker

`internal/sim` already builds for `js/wasm` with no changes: it does no file or
terminal I/O, and `main.go` does all the file reading. Two things still need
doing.

**The tick loop has to give control back to JS.** `Engine.Run` never returns,
and when it falls behind schedule it runs overdue ticks back to back without
blocking ([architecture.md](./architecture.md)). Go's WASM runtime only returns
to the JS event loop when every goroutine is blocked. So a sim running flat out
would starve `onmessage`, and a pause command would never arrive. Parking on a
Go timer every tick doesn't fix this either: that becomes a nested `setTimeout`,
which browsers clamp to at least 4 ms, so the sim tops out near 250 ticks/s.

The fix is to turn the loop inside out. Pull the scheduling logic (`nextDue`,
`shouldPublish`, `maxTickLag`) out into a steppable call, roughly:

```go
// Advance runs every tick due by now, stopping early once budget is spent,
// and reports the snapshot to publish (nil if none is due yet).
func (e *Engine) Advance(now time.Time, budget time.Duration) *Snapshot
```

`Run` becomes a thin native wrapper around `Advance`, so there is still only one
scheduler and the TUI doesn't notice the change. In the worker, a JS driver calls
`advance` once per slice (a budget of about 8 ms), posts any frame it gets back,
and yields. It yields through a `MessageChannel` ping rather than `setTimeout`,
so it doesn't hit the clamp. Commands arrive between slices, just as they are
applied between ticks today.

**The toolchain is standard Go, not TinyGo.** `yaml.v3` and the config struct tags
rely on `reflect`, and TinyGo's GC is weaker. The alternative is
`GOOS=wasip1` with `//go:wasmexport`: JS could read a frame buffer straight out of
linear memory, but it needs a WASI shim. It is worth measuring during the spike.
The default is `js/wasm` with `wasm_exec.js`, which crosses into JS once per
frame (`js.CopyBytesToJS`).

### 2. The wire: topics, not snapshots

In-process, a frontend gets the whole `Snapshot`, and that is cheap because
nothing gets copied twice. Across `postMessage`, structured-cloning an object
graph with thousands of `EntityView`s, profiles and memories 60 times a second
would cost more than the sim does. So the worker sends only what the open views
need, in two tiers:

| Tier | Contents | Rate | Encoding |
| --- | --- | --- | --- |
| **Frame** | tick, paused, tps, `Stats`; entities as struct-of-arrays (id, x, y, kind, state, focus, species, hp); tile pages that changed and are in the interest set; refuse deltas; log lines since the last sequence number | ≤ 60 Hz, as published | Hand-rolled little-endian binary in one `ArrayBuffer`, **transferred** (zero-copy) |
| **Topic** | whatever one open view needs: `roster` rows, `entity:<id>` (the full inspector), `jobs`, `storage`, `lore`, `perf` | Only while subscribed, throttled to ~4–10 Hz (lore only on change) | JSON. It is small and rare enough that parse cost doesn't matter, and it is easy to debug |

On the main thread, the frame decoder produces typed-array *views* over the
buffer. It doesn't parse anything, and it doesn't create one JS object per
entity. At 2000 entities, a frame is roughly 30 KB, or about 2 MB/s at 60 Hz,
which is trivial for a transfer.

Main → worker messages are the existing `Command`s plus two that exist only on
the wire:

- `setInterest{rect, zoom}` — the map region the renderer wants tiles for.
- `subscribe` / `unsubscribe{topic}` — which panels are open.

**Tiles ride the existing page scheme.** `TileGrid` pages are already
copy-on-write ([snapshot-tile-grid.md](./snapshot-tile-grid.md)). A page whose
slice pointer is the same as the one sent last time hasn't changed. The encoder
keeps the last `*TileGrid` it sent and sends a page only when both of these hold:

- the page is in the interest set, and
- its identity has changed since the client last received it.

This matters most on the first frame: a 10000×10000 map is 300 MB of
`tileCell`s. Sending the map lazily, by viewport, is required, not an
optimization. A page is a 4096-tile stretch of one row, which lines up well with
the renderer's `texSubImage2D` uploads.

**Enums travel as a catalog, not as duplicated TS tables.** On startup the worker
sends a `hello` message with:

- the name of every `Terrain` / `Kind` / `State` / `FocusKind` / body part /
  need, plus the glyph hints from the TUI's registry;
- the map's dimensions and the seed;
- the `sim.Config` schema (name, doc, default, and type, from the same
  `cfg:"…" doc:"…"` tags that generate the flags and `mars-sim.yaml`).

The UI then draws a new-game form from that schema, and adding a terrain type in
Go needs no TS change ([design-principles.md](./design-principles.md): enums
over ints).

The wire isn't tied to a Worker. The same encoder behind a WebSocket gives a
`mars-sim -web` mode that drives the browser UI from a native engine. That is
useful for debugging, and for comparing native and WASM speed on the same UI.

### 3. The map: a canvas, and WebGL2 from the start

It has to be a canvas. The open question is whether it's 2D or WebGL. Canvas 2D
can draw a viewport of about 100×60 tiles with `drawImage`. But zooming out to a
minimap or a whole-colony view is exactly where per-tile draw calls fall over,
and the docs already design for 10000×10000 maps. So:

- **Terrain is a data texture.** Tile bytes (terrain, composition, explored) go
  into `R8UI`/`RG8UI` textures, chunked to about 2048² because WebGL's maximum
  texture size can be as low as 8192. One quad per chunk, and the fragment shader
  looks up each tile's atlas cell and applies the fog. A changed page becomes one
  `texSubImage2D`. Drawing cost doesn't depend on zoom.
- **Entities are instanced sprites**, drawn straight from the frame's typed arrays
  (`x`, `y`, `kind`, …) as instance attributes, with no per-entity JS object.
- **The atlas starts as emoji** rendered with `fillText` into an offscreen canvas
  from the catalog's glyph hints, so the browser can match the TUI on day one.
  Real art replaces it cell by cell later.
- **Rendering runs on `requestAnimationFrame`** from the newest decoded frame, and
  never once per frame received. The same rule as the TUI: keep the latest frame,
  draw at the display rate.

The renderer is a plain TS class that owns its `<canvas>`. Svelte mounts it and
passes it the camera, and that's all. Keeping it framework-free means it can move
into its own worker with `OffscreenCanvas` later, if main-thread DOM work (a big
roster re-render, say) ever shows up as map jank.

### 4. The other views: Svelte 5

Svelte suits the panels. It compiles to direct DOM updates with no virtual-DOM
diff, runes give fine-grained reactivity, and the runtime is small. What matters
more than which framework you pick is **where the framework stops**:

- The map, the decoder and the frame store never go through Svelte state.
- A panel reads a topic store that updates at most about 10 times a second. Use
  `$state.raw` and replace the whole value, rather than deep `$state`: deep
  proxies over thousands of rows cost a lot, and the data is immutable anyway,
  just like a `Snapshot`.
- Long lists (the roster, graveyard, job tasks, memories) are virtualized.
- A closed panel unsubscribes, and the worker stops encoding its topic.

The panels map one-to-one onto the TUI's tabs (map, roster, job board, storage,
lore, perf). There is no "economy" system in the sim yet. The storage/inventory
data is the closest thing, and an economy view would start as a topic built from
it.

**Alternatives considered:**

- **React.** The virtual-DOM diff on 10 Hz data is avoidable overhead, and the
  map would bypass it anyway.
- **Solid.** About as good a fit as Svelte. Pick whichever is more pleasant to
  write.
- **Vanilla TS everywhere.** Fine for the map, tedious for forms and lists.

### 5. Big worlds: 10K×10K now, unbounded later

The target is at least 10000×10000, and eventually as big as a colony needs.
Measured on today's engine (seed 7, default config, 10000×10000):

| | Native | WASM (Node 22) |
| --- | --- | --- |
| `NewEngine` (worldgen) | 13.2 s | 37.7 s |
| Heap after worldgen | 748 MB | 748 MB |
| Heap after the first publish | 1.03 GB | 1.03 GB (1.39 GB reserved from the OS) |
| 300 ticks, 6 colonists | 0.05 s | 0.18 s |

The good news: ticking a big map is already cheap. The sparse grids and the
page-shared tile grid did their job. The bad news is everything that happens
before the first tick. A profile of `NewEngine` + first publish shows why:

- **Worldgen is eager over the whole map.** Rock veins (`growRockVeins`, 35% of
  CPU) and hidden caverns (`generateCaverns`, 32%) are laid out across all
  100M tiles up front. Their scratch allocations (`ordinaryNeighbors`,
  `planCavern`) add up to 1.2 GB churned. Carving caverns through `setTerrain`
  also fires `TileChanged` into the job board for tiles nobody can reach yet
  (15%).
- **The map is stored twice.** `World.tiles` is 300 MB (3 bytes × 100M). The
  first publish clones every page into the `TileGrid`, which is another
  300 MB and 1.7 s natively.

So there are three limits, and they arrive in this order:

1. **Load time.** 38 s to start a new game in the browser. It can be tolerated
   behind a progress bar, but not for long.
2. **Memory.** About 1–1.4 GB per 100M tiles. Desktop browsers can handle
   that. Phones can't: iOS kills tabs well below it.
3. **The wasm32 address space.** Go's WASM target is 32-bit, so memory stops
   at 4 GB, and there is no Go wasm64 target. At the current density, that
   caps an eagerly generated map at about 18000×18000. "Unbounded" is not
   reachable by tuning.

**Two fixes, in order of payoff:**

- **Skip the published copy in the worker.** In the browser, the wire encoder
  runs on the same thread as the engine, *between* ticks. Nothing reads a
  frame while the world mutates, so the copy-on-write grid buys nothing
  there. A same-thread consumer can read `World.tiles` directly plus the
  existing dirty-page list, and the 300 MB copy disappears. The native TUI
  keeps the copy, because it really does read on another goroutine. This is
  a small, contained change.
- **Chunked, lazy worldgen.** This is the real answer, and it is an engine
  change, not a browser one. Divide the world into chunks (for example
  256×256). Generate each one deterministically from `(seed, chunk x,
  chunk y)` the first time anything needs it: a dig reaching its border,
  pathfinding asking about it, or the camera looking at it (it would render
  as fog anyway). Unvisited chunks cost nothing, so world size stops being a
  memory question and becomes a coordinate-range question. Features that
  cross chunk boundaries (veins, caverns, the passages between caverns) need
  care. The usual approach: each feature is owned by the chunk that contains
  its origin and is rolled from that chunk's seed, and a chunk is generated
  after the neighbors whose features could reach into it. Caverns are
  already hidden until a dig breaks in ([caverns.md](./caverns.md)), which
  fits lazy generation well. The live grid, the regions and the flow fields
  would then be paged by chunk, the way `pagedGrid` already is
  ([sparse-grids.md](./sparse-grids.md)).

Chunking is also what keeps save files small (see the next section) and new
games instant. It should be designed before the browser frontend depends on
world size. The browser can ship on eager worldgen at 10K in the meantime.

### 6. Save and load

Saves are the one feature here the engine can't support today, for two
reasons:

- **The RNG streams can't be saved.** `World.rng`, `prng`, `agePRNG` and
  `nestRNG` are `math/rand` v1 sources, whose state is private. Moving them to
  `math/rand/v2`'s PCG, which implements `MarshalBinary`, fixes that. It
  changes what every seed produces, so it is a one-time break for shared
  seeds, and it should happen before the browser gives anyone a seed worth
  sharing.
- **There is no serializer.** The authoritative state needs one: tiles,
  entities (profiles, affect, memories, inventory), relationships, projects,
  storage, the director's schedule, the graveyard and deceased archive, the
  tick, and the RNG states. Derived state (regions, rooms, flow fields, the
  job board, the spatial index and every cache) is *not* saved. It is rebuilt
  on load.

Rebuilding derived state is where save/load can break determinism. If a
cache's contents, or the order it was filled in, ever decides a tie, a loaded
game will drift away from the one that was saved. The guard is a test in the
style of [determinism.md](./determinism.md): run N ticks, save, load, run M
more, and require the fingerprint to match a straight N+M run on every tick.

**The format** is one versioned binary file, compressed, and identical on
every platform. Since the cross-platform check shows native and WASM agree bit
for bit, a save from the browser loads in the TUI and the other way round.
Tiles are the only big part:

- With chunked worldgen, save only the chunks that have been modified. Any
  other chunk regenerates from the seed. A save's size then tracks what the
  colony has done, not the size of the map.
- Until then, save the tile planes compressed, since they are mostly unbroken
  rock. Saving a diff against regenerated worldgen would be tiny, but loading
  it would cost the full 38 s of worldgen, and any change to worldgen code
  would break old saves. Chunking fixes both problems, so don't build the
  diff format now.

The file records its format version and a worldgen version. A load that
doesn't match fails with a clear message rather than loading a subtly
different world.

**Where saves live in the browser:**

- **Not `localStorage`.** It caps at about 5 MB of strings and is synchronous
  on the main thread.
- **The Origin Private File System (OPFS).** This is the browser's private
  file store for a site. The worker can write to it directly, through a sync
  access handle, without copying the save through the main thread. Its quota
  is a share of free disk, in gigabytes. Call `navigator.storage.persist()`
  so the browser doesn't evict saves under storage pressure. Autosave goes
  here as a rolling slot.
- **Export/import for portability.** A save is a download of the same bytes
  (`.marssave`, via a `Blob`) and an import through a file picker or drag and
  drop. This is also the backup story, and it matters most on Safari: unless
  the site is installed to the home screen, Safari deletes script-written
  storage after 7 days without a visit. Treat OPFS as a cache of your saves
  and exported files as the durable copy.

Sharing a *world*, as opposed to a game in progress, needs no file at all. A
URL like `?seed=…&w=…&h=…` regenerates it.

### 7. Hosting and mobile

**Static hosting is fine.** Everything above is files: `index.html`, the JS
bundle, `sim.wasm` and `wasm_exec.js`. The simulation runs on the player's
machine and saves stay in their browser or files, so there's no server to run.
The earlier caveat was narrower than it sounded. Only three things would push
past a plain static host, and none of them is planned:

- **`SharedArrayBuffer` / WASM threads.** These need two HTTP response headers
  (`Cross-Origin-Opener-Policy` and `Cross-Origin-Embedder-Policy`). GitHub
  Pages can't set custom headers. Cloudflare Pages and Netlify can (via a
  `_headers` file) and are still static hosts. Go's WASM target is
  single-threaded anyway, so this is unlikely to come up. If it does, it
  means changing host, not adding a server.
- **Anything that stores data for players**: cloud saves, share-by-link
  saves, leaderboards, multiplayer. These need a backend, or at least a
  storage service.
- **Serving the `.wasm` file correctly.** It must be served as
  `application/wasm` for streaming compilation, and compressed. GitHub Pages
  does both (gzip). Brotli would shave a bit more, which Cloudflare does.

Cloudflare Pages is the suggested host: free, static, custom headers if they
are ever needed, and brotli. GitHub Pages works too, with the headers caveat
above.

**Mobile, later.** Nothing above rules it out: WebGL2, workers, OPFS and WASM
all work on current iOS and Android. What to do now, so it stays possible:

- Handle input as pointer events rather than mouse events. That gives touch
  for free.
- Keep panels responsive, not fixed-width.
- Don't assume desktop memory. A 10K eager world won't fit on a phone, so
  mobile realistically waits on chunked worldgen (or ships with a smaller
  default map).

## Why it is this way

**Measured: WASM is about 2.3–3.3× slower than native.** `internal/sim`
benchmarks, native (`go test -bench`) against the same test binary built
`GOOS=js GOARCH=wasm` and run under Node 22 (V8, the same engine Chrome uses):

| Benchmark | Native | WASM | Ratio |
| --- | --- | --- | --- |
| `BenchmarkStep500` (160², 500 colonists) | 10.6 ms/tick | 24.7 ms/tick | 2.3× |
| `BenchmarkStep2000` | 44.0 ms/tick | 102.5 ms/tick | 2.3× |
| `BenchmarkStepMixed500` | 6.9 ms/tick | 19.8 ms/tick | 2.9× |
| `BenchmarkPathfind` | 21 µs | 68 µs | 3.2× |
| `BenchmarkPublishSmallColonyOnHugeMap2500` | 0.37 ms | 1.21 ms | 3.3× |

The test binary is 8.3 MB uncompressed. What this means:

- **Sim throughput is the main risk, not rendering.** A 500-colonist colony that
  ticks at about 95/s natively ticks at about 40/s in a browser.
- WASM Go has a single-threaded, non-concurrent GC and no goroutine parallelism.
  Allocation-reduction work pays off *more* there than it does natively.
- That argues for keeping the TUI and native builds first-class, and for keeping
  the engine, not the browser, the place where perf work happens.

**Measured: WASM stays deterministic, bit for bit.** The determinism tests pass
under WASM. Beyond that, a 3000-tick fingerprint hash (seed 99, the
`TestDeterministicRunAgreesEveryTick` world) came out **identical** natively and
in WASM. So a seed shared from the browser reproduces natively and vice versa,
and a command log is enough to replay a run on either. That should be a CI test
(see Extending it), not a one-off check.

**Why a worker, not the main thread.** A 25 ms tick on the main thread drops
every frame it overlaps. In a worker, a slow tick only delays the next snapshot,
and the map keeps drawing the last one. That is the same drop-stale-frames
decoupling the engine already provides between goroutines.

**Why not structured-clone the `Snapshot`.** It holds deep copies of every
profile, memory and relation. They are needed in-process because the frontend
shares an address space with the engine, but most are never looked at on a given
frame. Topics move that cost to the moment someone opens a panel.

**Why not `SharedArrayBuffer`.** Transferring `ArrayBuffer`s is already
zero-copy, and shared memory would only pay off with more than one thread
reading the same buffer. It also needs cross-origin isolation headers, which
rules out GitHub Pages (see Hosting). Revisit only if a profile shows the
transfer mattering.

**Why not ship only the viewport's entities.** We rejected viewport-only
snapshots in-process ([snapshot-tile-grid.md](./snapshot-tile-grid.md)), and
entities are cheap on the wire. Tiles are where the interest set earns its keep.

## Extending it

A suggested order. Each step is worth landing on its own:

1. **Spike.** Add `Engine.Advance` and move `Run` onto it, with native tests
   unchanged. Then add `cmd/mars-sim-wasm` running headless in a worker, posting
   `Stats` only. Measure ticks/s in Chrome, Firefox and Safari against the table
   above, and try `wasip1` + `wasmexport`. Include a 10K world to measure load
   time and peak memory in a real browser tab, not just Node.
2. **Cross-platform determinism in CI.** Build the lockstep fingerprint test for
   `js/wasm` and compare its hash against native. Note:
   `go_js_wasm_exec` passes the whole environment to Node and trips
   *"total length of command line and environment variables exceeds limit"*, so
   run it under `env -i PATH=… HOME=…`.
3. **`internal/wire`.** Add the frame encoder, the topics, and the
   `hello` catalog, with golden-byte tests natively. Keep a TS decoder test fed
   by the same golden files, so the two sides can't drift apart.
4. **The map renderer.** Terrain textures, sprites, camera, fog, and click-to-pick
   (resolved on the main thread from the frame's positions).
5. **Svelte shell.** The header stats, controls (pause, speed, spawn, the
   build orders) and the roster with its inspector. Then the job board, storage,
   lore and perf.
6. **New game / config.** A form generated from the `hello` schema.
   `director.yaml` and `alien-names.yaml` become fetched or uploaded bytes that
   the WASM entry point parses, instead of files `main.go` reads.

Engine work that runs alongside, and benefits the TUI too:

- **Same-thread consumers skip the published tile copy.** This removes 300 MB
  and 1.7 s at 10K.
- **RNG streams move to `math/rand/v2` PCG**, so their state can be saved.
  This is a one-time seed break, so do it early.
- **Save/load**: the serializer, rebuilding derived state on load, and the
  save → load → lockstep test. Then the browser adds OPFS slots, autosave, and
  export/import.
- **Chunked lazy worldgen.** It needs its own design doc before code. It
  unlocks unbounded worlds, instant new games, small saves and mobile.

Invariants to preserve:

- The engine stays unaware of the browser. Anything web-specific lives in
  `internal/wire` or `cmd/mars-sim-wasm`.
- UI-only messages (`setInterest`, `subscribe`) never reach `sim` and never
  affect the simulation. Only `sim.Command`s do, which is what keeps
  seed + command log a complete replay.
- Tile pages are sent by identity, never by rescanning, the same
  "never walk the map per frame" rule the engine follows.

Decided so far:

- **Scale:** at least 10000×10000 from the start, so viewport tile interest and
  texture chunking are required on day one. Unbounded later, via chunked
  worldgen.
- **Saves:** required. OPFS slots plus portable exported files, in one format
  shared with the native build.
- **Mobile:** wanted, not day one. Use pointer events and a responsive layout
  from the start.
- **Hosting:** static (Cloudflare Pages suggested).

Still open:

- **How much of the 38 s load to accept** before chunked worldgen lands: ship
  eager worldgen at 10K behind a progress bar, or make chunking a prerequisite?
- **Whether a replay log is also kept.** It is nearly free (seed plus the
  command log) and good for bug reports, but it is not a substitute for saves.

## Related

- [architecture.md](./architecture.md) — the snapshot/command contract this keeps.
- [snapshot-tile-grid.md](./snapshot-tile-grid.md) — the page-shared grid the wire's tile deltas reuse.
- [frontend-tui.md](./frontend-tui.md) — the reference frontend, and the panels to reach parity with.
- [determinism.md](./determinism.md) — why the cross-platform fingerprint test matters.
- [perf-screen.md](./perf-screen.md) — the timing samples the browser's perf panel would show.
- [configuration.md](./configuration.md) — the struct tags the new-game schema comes from.

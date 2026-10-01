# Browser frontend (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan for running mars-sim in a browser tab. The Go engine is compiled to
WebAssembly and runs in a **Web Worker**. A new TypeScript/Svelte UI runs on the
main thread, and a WebGL canvas draws the map. It is the same contract as every
other frontend ([architecture.md](./architecture.md)): the engine publishes
snapshots, the frontend sends commands. The difference is that a
`postMessage` boundary now sits between the two, where a Go channel used to be.

Most of it is still a proposal. What is built: the engine-side prerequisites
(the steppable loop, live tiles, chunked worldgen, saveable RNG) and a
**spike**, a headless worker plus a measurement page, whose results are in
[The spike](#the-spike-what-a-real-browser-measured) below. The wire, the map
and the Svelte UI are not started. The numbers here come from runs against
the tree, not from guesses.

## Source

Built so far:

- [`internal/sim/engine.go`](../internal/sim/engine.go) — `Engine.Advance`, the
  steppable tick loop, sharing its schedule with `Run`.
- [`internal/sim/advance_test.go`](../internal/sim/advance_test.go) — budgets,
  pausing, the Run/Advance exclusion, and every frame reaching the host.
- [`cmd/mars-sim-wasm/`](../cmd/mars-sim-wasm/main.go) — the `GOOS=js
  GOARCH=wasm` entry point. It exports `start`, `advance`, `interest`, `send`
  and `memory` on `globalThis.marssim`; `advance` returns a binary wire frame
  ([wire-format.md](./wire-format.md)), the rest JSON. It never imports the TUI.
- [`web/public/worker.js`](../web/public/worker.js) — the host loop, shared by
  the game and the spike. [`web/build-wasm.sh`](../web/build-wasm.sh) builds the
  WASM next to it.
- [`web/spike/index.html`](../web/spike/index.html) — the measurement page. It
  runs under `npm run dev` in `web/` at `/spike/`; add
  `?auto=1&width=…&tps=…&viewW=…` to start straight away.
- The game page and the map renderer, in `web/`: see
  [frontend-web.md](./frontend-web.md).

- [`internal/wire/`](../internal/wire/wire.go) and
  [`web/wire/decode.js`](../web/wire/decode.js) — the frame tier of the wire:
  the Hello, the binary frame, and its decoder. See
  [wire-format.md](./wire-format.md).

Still proposed:
- `web/` — Vite + Svelte 5 + TypeScript.
  - `web/src/sim/` — the worker bootstrap, `SimClient`, and the frame decoder.
  - `web/src/map/` — the WebGL map renderer. It is framework-free.
  - `web/src/views/` — the Svelte panels: roster, job board, storage, lore, perf.

## How it works

```
 main thread                                   worker
┌──────────────────────────────────────┐      ┌─────────────────────────────┐
│ Svelte shell (tabs, panels, dialogs) │      │ JS driver loop              │
│        ▲  topic stores, ~4-10 Hz     │      │   │ advance(budget)         │
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

**The tick loop has to give control back to JS. Done:** `Engine.Advance`. `Engine.Run` never returns,
and when it falls behind schedule it runs overdue ticks back to back without
blocking ([architecture.md](./architecture.md)). Go's WASM runtime only returns
to the JS event loop when every goroutine is blocked. So a sim running flat out
would starve `onmessage`, and a pause command would never arrive. Parking on a
Go timer every tick doesn't fix this either: that becomes a nested `setTimeout`,
which browsers clamp to at least 4 ms, so the sim tops out near 250 ticks/s.

The loop is turned inside out:

```go
// Advance applies queued commands, runs the ticks due, stops once budget has
// elapsed (after at least one due tick), and returns the frame it published,
// if any, and how long to wait before calling again (0: a tick is due now;
// negative: paused, only a command changes anything).
func (e *Engine) Advance(budget time.Duration) (*Snapshot, time.Duration)
```

`Run` and `Advance` share one schedule (`restartSchedule`, `handle`,
`runDueTick`, over the same `nextDue` and `maxTickLag`); `Run` only adds the
blocking. An engine takes one or the other (mixing them panics), and
`ShareLiveTiles` is refused once either has started.

Three decisions in it that were not obvious up front:

- **One publish per call, at its end.** Under `Run`, a tick or a command
  publishes on the spot. Under `Advance` that lost frames: a spawn published,
  then a tick in the same call published again, and the host only ever saw the
  second. With live tiles that is a correctness bug, not a waste, because
  `TileChanges` is a delta against the previous frame *built*, so the first
  frame's changed pages were gone. So in stepping mode `publish` becomes a
  request (`requestPublish`) that the end of the call honours, along with the
  usual 60-a-second cap; unpublished ticks' changes accumulate into the next
  frame. `TestAdvanceWithLiveTilesSeesEveryFrame` pins it.
- **At least one tick per call.** A budget smaller than one tick (a 200-colonist
  tick is 5 ms in the browser) would otherwise never tick.
- **The host decides how to wait.** `worker.js` re-queues through a
  `MessageChannel` ping only when a tick is due *now* (wait 0). Any positive
  wait goes to `setTimeout`, 4 ms clamp and all. Pinging for sub-millisecond
  waits was the first version, and it spun the worker at 33,000 empty slices a
  second at 1000 tps. The clamp costs nothing because the schedule is fixed
  deadlines: a slice that wakes 4 ms late runs the four ticks it owes, and
  1000 tps held at 999 with 180 slices a second.

**The toolchain is standard Go, not TinyGo.** `yaml.v3` and the config struct tags
rely on `reflect`, and TinyGo's GC is weaker. The alternative is
`GOOS=wasip1` with `//go:wasmexport`: JS could read a frame buffer straight out of
linear memory, but it needs a WASI shim. The spike uses `js/wasm` with
`wasm_exec.js` and crosses into Go once per slice; it did not try `wasip1`,
because the spike's frames are tiny and the question only matters once the
wire's binary frames exist. Measure it then. The release build is 7.1 MB,
1.9 MB gzipped.

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

**Tiles ride the existing page scheme.** Every `Snapshot` carries
`TileChanges`: the pages that changed since the previous one, including newly
generated chunks, plus a frame number to detect a gap
([snapshot-tile-grid.md](./snapshot-tile-grid.md)). The encoder runs with
`Engine.ShareLiveTiles`, sees every frame, and sends a page only when both of
these hold:

- the page is in the interest set, and
- it is in `TileChanges.Pages` (or `All` is set) since the client last received it.

Page identity is not the signal: live pages never move. Sending lazily, by
viewport, still matters on the first frame and as the colony explores. A page is
a 64×64 square (`TilePageSide`, located with `TileGrid.PageOrigin`), which lines
up well with the renderer's `texSubImage2D` uploads.

**The frame tier is built** ([wire-format.md](./wire-format.md)), with a few
differences from the sketch above. Entities carry id, position, kind, state
and focus so far (no species or hp yet). Refuse is a whole list sent when it
changes, not deltas, and the log isn't carried yet. The view is a tile
rectangle (`interest`), with no zoom. A frame carries at most 64 pages, and
the rest are "owed" and follow straight away, nearest the middle of the view
first. Topics are not built.

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

**Built, with flat colors** ([frontend-web.md](./frontend-web.md)). Terrain
is `RG8UI` chunk textures filled straight from the wire's page bytes. Entities
and refuse are instanced quads. Occupants of unseen tiles are filtered out.
Zoomed in, glyphs from an emoji atlas replace the flat colors, and each
entity's glyph is picked in Go (`internal/glyphs`, shared with the TUI).

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

**Built** ([frontend-web.md](./frontend-web.md)). The map stays a canvas, and
the chrome around it is Svelte 5:

- **A top bar:** the clock, the speed selector, and the TUI header's counts.
- **A side panel:** one tab open at a time, beside the map, so the map stays
  visible.

Frame data reaches Svelte at most 10 times a second, through
`web/src/game.svelte.ts`. Topic payloads are `$state.raw`, replaced whole.

**Alternatives considered:**

- **React.** The virtual-DOM diff on 10 Hz data is avoidable overhead, and the
  map would bypass it anyway.
- **Solid.** About as good a fit as Svelte. Pick whichever is more pleasant to
  write.
- **Vanilla TS everywhere.** Fine for the map, tedious for forms and lists.

### Parity with the TUI

What the browser still lacks against the terminal UI, in the order it is being
built. No step needs engine work: every panel's data is already in a
`Snapshot`. Each is a topic on the wire (see [wire-format.md](./wire-format.md),
"Topics") plus a panel.

| Step | Contents | Status |
| --- | --- | --- |
| A. Shell | Svelte, side panel, top bar with the TUI's counts, speed selector, topics, Lore | **Done** |
| B. Inspector | Click an entity or tile. Colonist inspector: need, HP-per-part, mood (charge, grip, valence) and affinity bars; inventory, traits, family, memories. Tile inspector: fixture owner, storage contents. | **Done** |
| C. Roster, Log | Virtualized roster with the TUI's filters (non-humans, the dead) and B's inspector. Log tab, and a log ticker on the map. | **Done** |
| D. List tabs | Jobs (and a project's tiles highlighted on the map), Storage, Market | **Done** |
| E. Charts (uPlot) | Perf (tps, ms per tick), Population (four series over the game), Activity (stacked shares with walk-to bands; share or average toggle) | **Done** |

Decisions so far:

- **Layout:** the map is always visible, and tabs open in a side panel, which
  is a bottom sheet on a phone. Windows (several panels open at once) may come
  later; a panel is a self-contained component that subscribes to its own
  topic, so moving panels into windows would not change them.
- **Svelte 5** for the chrome, and **uPlot** for the charts.
- **Speed:** a Pause / Normal (8 tps) / Fast (32) / Faster (128) / Max selector. `+` and `-`
  step through it, and space toggles pause. The TUI's spawn (`s`) and build
  (`b`) menus are left out for now.

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

- **Skip the published copy in the worker. Done;** see
  `Engine.ShareLiveTiles` in [snapshot-tile-grid.md](./snapshot-tile-grid.md).
  The encoder runs on the same thread as the engine, *between* ticks, so the
  copy-on-write grid buys nothing there; live sharing points the published
  grid at the world's own pages and reports changes through `TileChanges`.
  The native TUI keeps the copy. Chunked worldgen landed first and shrank the
  copy from the whole map to the generated chunks (12 KiB each), so this now
  saves kilobytes, not 300 MB.
- **Chunked, lazy worldgen. Done;** see
  [worldgen-chunks.md](./worldgen-chunks.md). The world is 64×64 chunks,
  each a pure function of `(config, cx, cy)`: every feature is owned by the
  chunk its origin is in, rolled from that chunk's stream, and planned
  without reading tiles, so a chunk can be generated alone. Two things
  differ from the sketch above:
  - **Only the simulation triggers generation, never the camera.** A
    camera-driven trigger would make the set of generated chunks, and with
    it spawn sampling and dormant aliens, differ between machines. Chunks
    are generated when the colony first sees ground in or next to them
    (`worldgen-halo`). A fog-off view previews the rest without generating
    it.
  - **Chunks are 64×64, not 256×256,** so a chunk is exactly one page of
    every `pagedGrid`, and the tile grid itself is paged too.

  A new 10000×10000 game now starts in about 27 ms and 28 MB natively
  (118 ms under wasm), against 11.4 s and 1.35 GB (34 s under wasm) for
  eager generation once the economy landed. Unvisited chunks cost nothing,
  so world size is a coordinate-range question now: flow fields and
  `flowrepair` still use `int32` row-major indices, and the page tables are
  map-sized, which are the next limits past about 46000×46000.

Chunking is also what keeps save files small (see the next section): a
chunk nobody has changed can be regenerated from the seed instead of saved.

### 6. Save and load

Saves are the one feature here the engine can't support today. The RNG half
is done: every stream is a `math/rand/v2` PCG whose state `World.saveRNG` /
`loadRNG` round-trip (see [rng-streams.md](./rng-streams.md)). That move was
the one-time break for shared seeds. What remains:

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

**It is on GitHub Pages** (see [frontend-web.md](./frontend-web.md),
"Hosting"): the repo already lives on GitHub, and nothing above needs custom
headers. Cloudflare Pages was the first suggestion, for brotli and a
`_headers` file. It is still the place to move if `SharedArrayBuffer` ever
becomes necessary.

**Mobile, later.** Nothing above rules it out: WebGL2, workers, OPFS and WASM
all work on current iOS and Android. What to do now, so it stays possible:

- Handle input as pointer events rather than mouse events. That gives touch
  for free.
- Keep panels responsive, not fixed-width.
- Don't assume desktop memory. A 10K eager world won't fit on a phone, so
  mobile realistically waits on chunked worldgen (or ships with a smaller
  default map).

## The spike: what a real browser measured

Headless Chromium (Playwright's build, in a cloud container, so absolute
numbers are a slow machine's), driving the spike page. Seed 7, default
config otherwise, `tps` 0 meaning flat out:

| Scenario | New game | Ticks/s | Go heap | Main thread worst frame gap |
| --- | --- | --- | --- | --- |
| 10000×10000, 6 colonists, flat out | 348 ms | ~10,000 | 43 MB | 16.8 ms |
| 10000×10000, 200 colonists, flat out | 695 ms | 194 | 52 MB | 16.8 ms |
| 200×200, 6 colonists, 60 tps | 115 ms | 60.1 | 3 MB | 16.8 ms |
| 200×200, 6 colonists, 1000 tps | 129 ms | 999 | 4 MB | 16.8 ms |

What it settles:

- **Big worlds are no longer a browser problem.** Before chunked worldgen, a
  10K new game was 38 s and 1.4 GB in WASM (see Big worlds). Now it is well
  under a second and tens of megabytes, because only the landing site is
  generated. The wasm32 4 GB ceiling is now about how much a colony
  *explores*, not how big the map is.
- **The worker keeps the page smooth.** The worst gap between animation
  frames stayed at one 60 Hz frame (16.8 ms) in every run, including the
  flat-out ones where the worker never idles.
- **Commands are prompt.** Pausing a flat-out 10K game showed up as a paused
  frame 14 ms after the click.
- **Pacing is exact** at 60 and 1000 tps (see the `setTimeout` note above).
- **The slowdown against native is 3–8×, and larger on small ticks.** Native,
  same scenarios: the 200-colonist 10K world ticks at 1.6 ms (browser 5.2 ms,
  3.3×) and generates in 115 ms (browser 695 ms, 6×); a 6-colonist 200×200
  game's ticks cost 0.04 ms late in the game (browser ~0.2 ms, 5×). The Node
  benchmarks below said 2.3–3.3×, on bigger ticks.

What it leaves open:

- **Low tick rates read slow per tick.** At 60 tps the browser reported
  1.3 ms per early-game tick, where native spends 0.15 ms (8×), and the
  1000 tps run's ticks were cheaper than that despite being later in the game.
  The likeliest cause is waking from idle every 16 ms (CPU frequency, cold
  caches, the Go scheduler resuming), which a headless container exaggerates.
  It does not affect the tick rate, which held exactly, but it would affect a
  big colony at normal speed. Check it on real hardware.
- **Per-tick timings are coarse in a browser.** Go's clock there is
  `performance.now()`, which Chrome rounds to 100 µs without cross-origin
  isolation (Firefox and Safari to 1 ms or so). The Perf screen's per-tick
  numbers will be noisy for sub-millisecond ticks; tick *rates* are fine.
- **Firefox and Safari are unmeasured.** Only Chromium was available. Run
  `npm run wasm && npm run dev` in `web/` and open `/spike/` in each; the numbers above
  are the comparison. Safari matters most, for memory limits.
- **Real hardware.** All of the above is a shared cloud CPU.

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

1. **Spike. Done** for Chromium; see [The spike](#the-spike-what-a-real-browser-measured).
   Firefox, Safari and real hardware are still to measure, with the same page.
2. **Cross-platform determinism. Done as a test** (`golden_test.go`, run
   natively and under `js/wasm` by `tools/determinism-check.sh`). The repo has no
   CI, so nothing runs it automatically yet.
3. **`internal/wire`. Frame tier done** ([wire-format.md](./wire-format.md)):
   the Hello (map, enum names, stat names), the binary frame, golden frames,
   and a JS decoder tested against them. The spike page decodes real frames.
   Topics and the config schema come with the panels that need them.
4. **The map renderer. Done with flat colors** ([frontend-web.md](./frontend-web.md)):
   terrain textures, sprites, camera, fog, pan and zoom, and hover. Next:
   glyphs (done; see frontend-web.md), then click-to-select and page eviction.
5. **Svelte shell. Done**, with the Lore tab. The rest of the TUI's tabs
   follow the plan in "Parity with the TUI".
6. **New game / config.** A form generated from the `hello` schema.
   `director.yaml` and `alien-names.yaml` become fetched or uploaded bytes that
   the WASM entry point parses, instead of files `main.go` reads.

Engine work that runs alongside, and benefits the TUI too:

- **Same-thread consumers skip the published tile copy. Done.** After chunked
  worldgen it saves 12 KiB per generated chunk rather than 300 MB.
- **RNG streams move to `math/rand/v2` PCG**, so their state can be saved.
  Done; see [rng-streams.md](./rng-streams.md).
- **Save/load**: the serializer, rebuilding derived state on load, and the
  save → load → lockstep test. Then the browser adds OPFS slots, autosave, and
  export/import.

Invariants to preserve:

- The engine stays unaware of the browser. Anything web-specific lives in
  `internal/wire` or `cmd/mars-sim-wasm`.
- UI-only messages (`setInterest`, `subscribe`) never reach `sim` and never
  affect the simulation. Only `sim.Command`s do, which is what keeps
  seed + command log a complete replay.
- Tile pages are sent from `TileChanges`, never by rescanning, the same
  "never walk the map per frame" rule the engine follows. The host must see
  every frame `Advance` returns, or `TileChanges.Frame` will skip and it must
  resend everything.

Decided so far:

- **Scale:** at least 10000×10000 from the start, so viewport tile interest and
  texture chunking are required on day one. Unbounded later, via chunked
  worldgen.
- **Saves:** required. OPFS slots plus portable exported files, in one format
  shared with the native build.
- **Mobile:** wanted, not day one. Use pointer events and a responsive layout
  from the start.
- **Hosting:** static, on GitHub Pages
  (<https://kevinmchugh.github.io/mars-sim/>).

Still open:

- **Whether a replay log is also kept.** It is nearly free (seed plus the
  command log) and good for bug reports, but it is not a substitute for saves.

## Related

- [architecture.md](./architecture.md) — the snapshot/command contract this keeps.
- [snapshot-tile-grid.md](./snapshot-tile-grid.md) — the page-shared grid the wire's tile deltas reuse.
- [frontend-tui.md](./frontend-tui.md) — the reference frontend, and the panels to reach parity with.
- [determinism.md](./determinism.md) — why the cross-platform fingerprint test matters.
- [perf-screen.md](./perf-screen.md) — the timing samples the browser's perf panel would show.
- [configuration.md](./configuration.md) — the struct tags the new-game schema comes from.

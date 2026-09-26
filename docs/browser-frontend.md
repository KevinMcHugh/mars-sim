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

**Why not `SharedArrayBuffer`.** It needs cross-origin isolation (COOP/COEP
headers), and static hosts like GitHub Pages can't set those without a
service-worker workaround. Transferring `ArrayBuffer`s is already zero-copy.
Revisit only if a profile shows the transfer mattering.

**Why not ship only the viewport's entities.** We rejected viewport-only
snapshots in-process ([snapshot-tile-grid.md](./snapshot-tile-grid.md)), and
entities are cheap on the wire. Tiles are where the interest set earns its keep.

## Extending it

A suggested order. Each step is worth landing on its own:

1. **Spike.** Add `Engine.Advance` and move `Run` onto it, with native tests
   unchanged. Then add `cmd/mars-sim-wasm` running headless in a worker, posting
   `Stats` only. Measure ticks/s in Chrome, Firefox and Safari against the table
   above, and try `wasip1` + `wasmexport`.
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

Invariants to preserve:

- The engine stays unaware of the browser. Anything web-specific lives in
  `internal/wire` or `cmd/mars-sim-wasm`.
- UI-only messages (`setInterest`, `subscribe`) never reach `sim` and never
  affect the simulation. Only `sim.Command`s do, which is what keeps
  seed + command log a complete replay.
- Tile pages are sent by identity, never by rescanning, the same
  "never walk the map per frame" rule the engine follows.

Open questions for the next pass:

- **Target scale for the browser build.** How many colonists, and what map size?
  That decides whether tile interest management and texture chunking are needed
  from day one or can wait.
- **Save/load.** A deterministic replay (seed plus the command log, in
  IndexedDB) is nearly free. Full-state saves don't exist in the engine today.
- **Touch and mobile support.**
- **Hosting.** A static site fits everything above, as long as we stay off
  `SharedArrayBuffer`.

## Related

- [architecture.md](./architecture.md) — the snapshot/command contract this keeps.
- [snapshot-tile-grid.md](./snapshot-tile-grid.md) — the page-shared grid the wire's tile deltas reuse.
- [frontend-tui.md](./frontend-tui.md) — the reference frontend, and the panels to reach parity with.
- [determinism.md](./determinism.md) — why the cross-platform fingerprint test matters.
- [perf-screen.md](./perf-screen.md) — the timing samples the browser's perf panel would show.
- [configuration.md](./configuration.md) — the struct tags the new-game schema comes from.

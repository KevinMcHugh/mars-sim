# Wire format

> Part of the [mars-sim documentation](./README.md).

## What it is

The messages the browser build's worker sends the page: a JSON **Hello** once
per game, then a binary **frame** per published snapshot. A frame carries the
tick, the `Stats`, every entity's position and kind, the tile pages in view that
the page lacks, and gore and corpses when they changed. It is the frame tier of
the plan in [browser-frontend.md](./browser-frontend.md). The panel "topics"
(roster, inspector, job board) are not built yet.

## Source

- [`internal/wire/wire.go`](../internal/wire/wire.go) — `Hello`, `Version`, and
  the `Stats` field list.
- [`internal/wire/encoder.go`](../internal/wire/encoder.go) — `Encoder`: the
  frame layout, page selection, and the held-page bookkeeping.
- [`internal/wire/encoder_test.go`](../internal/wire/encoder_test.go) — a Go
  decoder, and the behavior: deltas, gaps, the view, the per-frame cap, fog.
- [`internal/wire/golden_test.go`](../internal/wire/golden_test.go) and
  `testdata/` — golden frames (`NAME.bin`) and what they decode to (`NAME.json`).
- [`web/wire/decode.js`](../web/wire/decode.js) — the page's decoder;
  [`decode.test.mjs`](../web/wire/decode.test.mjs) checks it against the same
  golden files (`node --test web/wire/decode.test.mjs`).
- [`internal/sim/pageread.go`](../internal/sim/pageread.go) — `ReadPage`,
  `PageKnown`, `PageIndex` and `RefuseTiles`: bulk reads of the published
  terrain.
- [`internal/sim/catalog.go`](../internal/sim/catalog.go) — `EnumNames`, the
  names Hello carries.
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go) — the host: when
  to encode, and the `interest` export.

## How it works

### Hello

Sent once, as JSON, in `start`'s result: the map size, the seed, the page side
(64), whether the fog is on, the **enum names** (terrains, rock compositions,
kinds, states, focuses, each indexed by value), and the **names of the stats**,
in frame order. Stats are every `int` field of `sim.Stats`, found by
reflection, so a new stat reaches the page with no change here. Frames carry
enum *values*; the page never keeps its own copy of the tables.

### A frame

Little-endian. Every section starts on a 4-byte boundary, so the page views each
one in place as a typed array (`decode.js` allocates a few views and parses
nothing).

| Offset | Size | Field |
| --- | --- | --- |
| 0 | 4 | magic `MSFR` |
| 4 | 2 | `Version` (1) |
| 6 | 2 | flags: 1 paused, 2 fog of war, 4 **tiles reset**, 8 **refuse frame** |
| 8 | 8 | tick |
| 16 | 8 | `TileChanges.Frame` |
| 24 | 4 | ticks per second |
| 28 | 4 | S, the number of stats |
| 32 | 4 | N, entities |
| 36 | 4 | P, tile pages |
| 40 | 4 | R, refuse tiles |
| 44 | 4 | pages owed (see below) |

Then the sections, in order:

- **stats**: `int32 × S`, named by `Hello.stats`.
- **entities**, struct-of-arrays: `uint32 id × N`, `int32 x × N`,
  `int32 y × N`, then `uint8 kind × N`, `uint8 state × N`, `uint8 focus × N`,
  padded to 4.
- **pages**: `int32 px × P`, `int32 py × P` (page coordinates, so tile
  `px*64`), then P × 4096 tiles of 2 bytes, row by row: **terrain**, then
  **flags** (rock composition in the low 4 bits, bit 4 *visible* = explored,
  or the fog is off). An edge page is sent whole; tiles past the map read as
  rock.
- **refuse**: `int32 x × R`, `int32 y × R`, `uint16 corpses × R` (padded),
  `uint8 gore × R` (padded). R is 0 unless the refuse-frame flag is set; then
  it is the *whole* list, and the page replaces its last one.

### Which pages go

The encoder keeps the set of pages the page **holds** with current contents,
and the view (`SetInterest`, a tile rectangle). Each frame:

1. Apply the snapshot's `TileChanges`: a changed page is no longer held.
   `All`, or a skipped `Frame` number, empties the set and sets **tiles
   reset**: the page drops everything and starts over.
2. Send the pages in view that are not held, **nearest the middle of the view
   first**, up to `MaxPages` (64, about 512 KB). The rest are **owed**: the
   header says how many, and the host encodes again straight away (even while
   paused) until none are.
3. Skip a page with nothing to show: fog on and its chunk not generated
   (`PageKnown`). It is not held, so it goes out when generation lands it in
   `TileChanges`, if still in view. With fog off, an ungenerated page is sent
   from the preview (`ReadPage` reads it the way `TileAt` does).

A page that changes while out of view is dropped from the held set and sent
again when it comes back into view. The page may keep drawing the stale copy
meanwhile (a minimap would); it is never told it is stale.

### When the host encodes

`cmd/mars-sim-wasm`'s `advance` encodes when `Engine.Advance` published a
snapshot, when the view moved, or when pages are still owed. The last two
re-encode the **newest snapshot again**, with nothing new to apply (same
`TileChanges.Frame`). With live tiles its terrain is the live map, which may
have moved on by a few unpublished ticks. That is harmless: those ticks'
dirty pages arrive in the next snapshot's `TileChanges` and are sent again.
Entities in a re-encode are from the newest snapshot, not newer ticks.

## Why it is this way

- **One buffer, typed-array sections, transferred.** Structured-cloning objects
  would cost one JS object per entity per frame. A transferred `ArrayBuffer` is
  zero-copy, and the typed views make decoding a dozen allocations. The only
  copy is Go's linear memory into a fresh `Uint8Array` (`js.CopyBytesToJS`),
  once per frame.
- **Refuse as a sparse list, not a tile bit.** `TileChanges.Refuse` says the
  list changed, not *where*, so a per-tile bit would mean resending every page
  in view on every death. The list is a few hundred entries in a long game.
- **A per-frame page cap.** A first frame, a zoom-out or a fast pan can put
  hundreds of pages in view. Without a cap, one frame would be megabytes and
  the worker would stall building it; with it, the view fills in from the
  middle over a few frames.
- **Re-encoding the last snapshot** instead of asking the engine to publish: a
  paused engine publishes nothing, and panning a paused map still has to show
  new ground. Publishing just to pan would also build the whole snapshot again.
- **Hello in JSON.** It is sent once, and being able to read it in devtools is
  worth more than the bytes.
- **Measured** (headless Chromium, the spike page): a flat-out 10000×10000 game
  with 6 colonists sends about 11 KB a frame, 655 KB/s at 58 frames a second;
  most of that is the page the colony is digging in, resent as it changes. A
  400×250-tile view at 60 tps holds 30 pages and averages 232 KB/s.
- **Considered and left for later**: `wasip1` with `//go:wasmexport`, reading
  the frame straight out of linear memory (it would save the one copy); and
  compressing pages (most are unbroken rock, but the copy is not the
  bottleneck yet).

## Extending it

- **A field on an entity** (hp, species): add an array to the entities section in
  both `Encoder.Encode` and `decode.js`, bump `Version`, regenerate the golden
  files (`go test ./internal/wire -run Golden -update`), and extend
  `decode.test.mjs`'s reshaping. Keep sections 4-byte aligned: put 4-byte arrays
  before 2-byte before 1-byte, and pad.
- **A new stat** needs nothing here, but it changes the golden frames: rerun
  `-update`. So does renumbering an enum the fixture uses.
- **A new enum** a frame indexes: add it to `sim.Enums` so Hello names it.
- **Tile flags** have 3 bits spare. `TestHelloNamesEverythingAFrameIndexes`
  fails if rock compositions outgrow their 4 bits or an enum outgrows its byte.
- **A second transport** (a WebSocket from a native engine) reuses `Encoder`
  unchanged, as long as it encodes on the engine goroutine with live tiles, or
  from a copy-on-write snapshot anywhere else.
- Invariant: the encoder must see **every** snapshot the engine publishes, or
  it falls back to a full resend. `Engine.Advance` guarantees it; a `Subscribe`
  channel does not.

## Related

- [browser-frontend.md](./browser-frontend.md) — the plan this is the frame tier of.
- [snapshot-tile-grid.md](./snapshot-tile-grid.md) — pages, `TileChanges`, and live tiles.
- [fog-of-war.md](./fog-of-war.md) — what *visible* means, and the preview.
- [sanitation.md](./sanitation.md) — the refuse the refuse section carries.

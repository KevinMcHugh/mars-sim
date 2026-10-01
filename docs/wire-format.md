# Wire format

> Part of the [mars-sim documentation](./README.md).

## What it is

The messages the browser build's worker sends the page: a JSON **Hello** once
per game, then a binary **frame** per published snapshot. A frame carries the
tick, the `Stats`, every entity's position and kind, the tile pages in view that
the page lacks, gore and corpses when they changed, and the shown flow field's
distances in view when it or the view changed. It is the frame tier of
the plan in [browser-frontend.md](./browser-frontend.md). Beside frames go
**topics**: JSON for one open panel at a time (see Topics).

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

Since version 2, Hello also carries the **glyphs** (`internal/glyphs`):
`symbols` (every emoji, `glyphs.All`; an entity's glyph indexes it), `terrain`
(each terrain's glyph index, or -1 for a swatch such as rock, floor or hull,
which the page draws as a color), `kinds` (each creature kind's generic glyph,
`glyphs.ForKind`, for counts), and the `gore` and `corpse` glyphs. Since
version 3 it carries `goreMax` (`sim.MaxGore`) and `scumMax`
(`Snapshot.ScumMax`, from `-scum-max`), so the page can shade a tile by how
much is on it. Version 4 adds the **salt** section (see below); the header's
reserved word became the salt count. `glyphs.looks` (no version bump: the
frame layout is unchanged) lists colonist looks, each a candidate list; a
frame's glyph index past the end of `symbols` names one (see
[colonist-looks.md](./colonist-looks.md)). Version 5 adds the **flow** section and
four header words for it, and Hello's `flowFields`: the shared flow fields'
names (`FlowFieldRef.Name`), in the engine's order, which the flow section's
field index and the page's `flow` command both index. The fields are made with
the world, so the list never changes during a game.

### A frame

Little-endian. Every section starts on a 4-byte boundary, so the page views each
one in place as a typed array (`decode.js` allocates a few views and parses
nothing).

| Offset | Size | Field |
| --- | --- | --- |
| 0 | 4 | magic `MSFR` |
| 4 | 2 | `Version` (5) |
| 6 | 2 | flags: 1 paused, 2 fog of war, 4 **tiles reset**, 8 **refuse frame**, 16 **scum frame**, 32 **salt frame**, 64 **flow frame** |
| 8 | 8 | tick |
| 16 | 8 | `TileChanges.Frame` |
| 24 | 4 | ticks per second |
| 28 | 4 | S, the number of stats |
| 32 | 4 | N, entities |
| 36 | 4 | P, tile pages |
| 40 | 4 | R, refuse tiles |
| 44 | 4 | pages owed (see below) |
| 48 | 4 | C, scum tiles |
| 52 | 4 | D, salt tiles |
| 56 | 4 | F, flow tiles |
| 60 | 4 | flow field: an index into `Hello.flowFields`, or -1 for none (int32) |
| 64 | 4 | the flow field's largest distance, over the whole map |
| 68 | 4 | the flow field's goal tiles (distance 0), over the whole map |

Then the sections, in order:

- **stats**: `int32 × S`, named by `Hello.stats`.
- **entities**, struct-of-arrays: `uint32 id × N`, `int32 x × N`,
  `int32 y × N`, `uint16 glyph × N` (padded to 4), then `uint8 kind × N`,
  `uint8 state × N`, `uint8 focus × N`, padded to 4. The glyph is picked in Go
  by `glyphs.ForEntity`, from things the page never sees (a colonist's gender,
  age and traits, an alien's species), so the TUI and the browser always
  agree — except that a resting colonist's index may point past `symbols`
  into `looks`, the same figure in their own skin and hair.
- **pages**: `int32 px × P`, `int32 py × P` (page coordinates, so tile
  `px*64`), then P × 4096 tiles of 2 bytes, row by row: **terrain**, then
  **flags** (rock composition in the low 4 bits, bit 4 *visible* = explored,
  or the fog is off). An edge page is sent whole; tiles past the map read as
  rock.
- **refuse**: `int32 x × R`, `int32 y × R`, `uint16 corpses × R` (padded),
  `uint8 gore × R` (padded). R is 0 unless the refuse-frame flag is set; then
  it is the *whole* list, and the page replaces its last one.
- **scum**: `int32 x × C`, `int32 y × C`, `uint8 amount × C` (padded), in row
  order. Amounts run 1 to `Hello.scumMax`. C is 0 unless the scum-frame flag
  is set; then, like refuse, it is the whole list. The encoder sends it when
  `Snapshot.Scum` is a different map from the last one sent. The engine hands
  out the same map until scum is scraped or grows (`publishedScum`), so map
  identity is an exact, free change signal. It is also sent on a tiles reset.
- **salt**: `int32 x × D`, `int32 y × D`, in row order. A deposit has no
  amount, so there is nothing else to send. D is 0 unless the salt-frame flag
  is set; then, like scum, it is the whole list of exposed deposits
  (`Snapshot.Salt`, see [salt.md](./salt.md)). It is sent when `Snapshot.Salt`
  is a different map from the last one sent (`publishedSalt` hands out the
  same map until a deposit is exposed or built over), and on a tiles reset.
- **flow**: `int32 x × F`, `int32 y × F`, `uint16 distance × F` (padded, and
  saturating at 65535), in row order: every tile **in the view** (the
  interest rectangle) that the shown flow field reaches. Unlike scum and
  salt, only the view goes: a field covers the whole colony, and the page
  only draws what is on screen. F is 0 and the header's flow words mean
  nothing unless the flow-frame flag is set; then this is the whole list for
  the view, and the page replaces its last one. A frame with the flag and
  field -1 says no field is shown any more: clear the overlay. It is sent
  when `Snapshot.FlowField` is a different view from the last one sent (the
  engine hands out the same `FlowFieldView` until the field changes, and nil
  until a page asks with the `flow` command), when the interest moved while
  a field is shown, and on a tiles reset. See
  [flow-field-view.md](./flow-field-view.md).

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

### Topics

The second tier: one panel's data, as JSON, sent only while a panel subscribes
to it (`internal/wire/topics.go`). The page sends `subscribe` or `unsubscribe`
with a topic name. `advance` then returns, next to the frame, a `topics` object
of the payloads due, by name:

- **On subscribe:** a topic is sent at once, paused or not.
- **After that:** it is rebuilt at most once per its interval, and sent only
  if its JSON changed. A panel over a paused game gets nothing further.
- **Unknown names** fail the subscribe.
- **New games:** subscriptions outlive them, so an open panel keeps its data.
  The host calls `Topics.Restart` on `start`, so each is sent again at once.

| Topic | Every | Payload |
| --- | --- | --- |
| `lore` | 1 s | `world` (size, fog, explored tiles, chunks generated, seed) and `species` (each rolled species' roster label, map glyph, build, temperament, bite and pace, and field notes) |
| `names` | 1 s | Every living colonist's name, by id (as a string key). The page holds it open for the hover readout, since frames carry ids, not names. It changes only on an arrival or a death. |
| `roster`, `roster:<filters>` | 500 ms | One row per creature the TUI's roster lists (`RosterRow`): glyph, name, an info line (pronouns and age, an alien's species, or the kind), a state line (state and mood, or `dead — <cause>`), and health. By ID. Filters, comma-separated: `dead` (every dead colonist from `Deceased`, and the graveyard's other kinds the other filter admits) and `nonhuman` (aliens, cats, rats). |
| `log` | 250 ms | The colony log as a stream (`LogTopic`): only the lines newer than the last send, each with `seq`, `tick`, `kind` (the `LogKind` label) and `text`. The first send, and the first after a new game, has `reset: true` and the whole ring. |
| `perf` | 1 s | The engine's timing (`PerfTopic`), the last five minutes in quarter-second buckets: ticks, busy ms and the slowest tick per bucket, as columns, plus the set tps. Paused buckets stay, with zero ticks. |
| `population` | 1 s | The whole game's vital signs and activity (`PopulationTopic`), one point per `PopulationSample`, as columns: tick, colonists, meals, colony size, fixtures, and per activity the colonist-ticks spent on it and walking to it. |
| `jobs` | 500 ms | The job board (`JobsTopic`): each queued project with its tasks (tile, terrain, phase, done, and who is building it) and assignees, and the manual orders still waiting for a build site. |
| `storage` | 500 ms | Every container (`StorageRow`): position, label (chest, pantry, someone's locker, a workshop), slots and items used, and what it holds most of. Contents and ledger are the tile topic's. |
| `market` | 500 ms | The market tab (`MarketTopic`): accounts (the treasury, then colonists richest first), the money supply, books, prices, plans, work orders totalled by issuer and kind (`work`; kind `build`, `dig` or `haul`), the dig tool's `dig` terms (the wage a tile pays and the most tiles an order may cover), `digs` (each open excavation: its id, bounds, tiles dug and money held), and the last 20 trades, newest first. |
| `account:<key>` | 500 ms | One account's page (`AccountTopic`), `colony` or a colonist's id: balance, share of circulating money, holdings summed across every storage ledger, fixtures owned, open orders, and a colonist's plans. `found: false` once the colonist is gone. |
| `entity:<id>` | 250 ms | One creature (`EntityTopic`): name, glyph, position, state, focus, health, body parts, and death if dead; an alien's species; a colonist's profile, wallet, affect, needs, inventory, traits, skills (rank, top rank, label, practice) and profession, family, affinities and memories (newest first). Looked up among the living, then `Deceased`, then `Graveyard`; `found: false` once it is in none of them. |
| `tile:<x>,<y>` | 250 ms | One tile (`TileTopic`): terrain (a rock's composition), glyph, the fixture's owner and access, a container's contents and ledger, and the creatures on it. Under fog, only `explored: false`. |

The inspector's two topics take a parameter, so they are not in `topicTable`:
`paramTopic` (`internal/wire/inspect.go`) parses them on subscribe, and a
malformed one (`entity:x`, `tile:3`) fails like an unknown name. Each
selection is its own topic, so the dedupe and the interval work per creature
with no extra state. Filth is not in the tile topic: the page already has it
from the frames.

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
  Scum follows the same pattern for the same reason. It is a longer list (every
  scummy patch of cave wall, about 850 in a 150,000-tick game), but it changes
  only when a patch is scraped or grows (spawns or spreads). Salt follows
  the same pattern again, and changes even less: it is only ever exposed by
  digging or lost to a build, never grown.
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

- **A topic with state** (the log remembers what it sent): a constructor
  like `newLogTopic`, called from `Subscribe`, so each subscription (and each
  `Restart`) gets its own. The JSON dedupe still applies on top.
- **A new topic with a parameter** (`project:<id>`): a case in `paramTopic`.
  The roster's filters work this way: each filter set is its own topic, so
  toggling one is an unsubscribe and a subscribe, with no per-page state in Go.
- **A new topic**: a `topicTable` entry in `topics.go` (an interval, and a
  function from the snapshot to a JSON-able value, with its own
  `json`-tagged types), a test, and a row in the Topics table above. Topics
  are JSON, not part of the frame layout, so they need no `Version` bump.

- **A field on an entity** (hp): add an array to the entities section in
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

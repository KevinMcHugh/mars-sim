# Save and load

> Part of the [mars-sim documentation](./README.md).

## What it is

A save file is the whole game: every tile, entity, project, order, cache and
RNG stream, written by a reflection codec that walks the `World`. A loaded game
plays on exactly as the saved one would have, tick for tick. Each file says
which commit wrote it. Saves load only into a build whose `World` has the same
shape; there is deliberately no compatibility with older layouts (issue #129).

## Source

- [`internal/sim/savecodec.go`](../internal/sim/savecodec.go): the codec
  (`saveEncoder`, `saveDecoder`), object ids, the overlap check,
  `saveInterfaceTypes`, and `saveLayout`.
- [`internal/sim/save.go`](../internal/sim/save.go): the file format,
  `SaveInfo`, `BuildCommit`, `Engine.Save` / `SaveBytes`, the `SaveGame`
  command, `LoadEngine`, `ReadSaveInfo`, and `World.afterLoad`.
- [`internal/sim/save_test.go`](../internal/sim/save_test.go): the round
  trips, including the run-on-identically test, and `saveDiff`.
- [`main.go`](../main.go): `-load` and `-save`.
- [`internal/ui/tui/model.go`](../internal/ui/tui/model.go): `ctrl+s`
  (`saveGame`).
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go): the `save` and
  `load` exports. [`web/public/worker.js`](../web/public/worker.js),
  [`web/src/sim/client.ts`](../web/src/sim/client.ts) and
  [`web/src/main.ts`](../web/src/main.ts) (`saveGame`, `loadGame`) carry
  them to the Game tab ([`NewGamePanel.svelte`](../web/src/ui/NewGamePanel.svelte)).

## How it works

### The file

```
mars-sim save
{"format":1,"commit":"acbc79e…","layout":"39202b72291dadc7","seed":7,"tick":376345,"width":512,"height":512,"tps":8,"paused":true}
<gzip: uvarint lengths, the Config, the World>
```

The first two lines are text, so `head -2 game.marssave` says which build wrote a
save and where in the game it was taken without loading anything.
`ReadSaveInfo` reads just that.

- **`commit`** is `BuildCommit()`: the `vcs.revision` Go stamps into a
  binary built with `go build` in a git checkout, with `-dirty` added for
  uncommitted changes. That includes the WASM from `web/build-wasm.sh`.
  `go run` and `go test` stamp nothing, so they write `unknown`. A build made
  outside a checkout can set it with `-ldflags "-X
  github.com/kevinmchugh/mars-sim/internal/sim.buildCommit=<sha>"`.
- **`layout`** fingerprints the type graph reachable from `Config` and
  `World`: every type's kind and each saved field's name and type, hashed.
  `LoadEngine` refuses a file whose layout differs and names both commits.
- **`format`** versions the framing: the header and the codec's grammar.
- **`tps`** and **`paused`** are the engine's. A loaded engine starts at the
  same speed and in the same paused state.

The Config is written before the World, and separately, because the loader
needs it to build the World it reads into.

### The codec

`saveEncoder.value` switches on `reflect.Kind`:

| Kind | Written as |
| --- | --- |
| bool, ints, uints, floats | a byte or varints (floats as IEEE bits) |
| string | length, bytes |
| array, struct | each element, or each field not tagged `save:"-"`, in order |
| pointer, map, slice | `0` for nil, else an **object id**. The first time an id appears, the contents follow. A repeat is just the id. |
| interface | a name from `saveInterfaceTypes`, then the value |
| func | nothing |

**Object ids keep sharing.** The same `*Ship` reached from `w.ships` and from a
structure, the same `*WorkOrder` from a task and from `w.workOrders`, and the
same map or slice in two places all load as one object. Pointers back to the
World (`flowField.w`, `jobBoard.w`) load as the World, because the root is
object 1. Ids are given out on first sight, and the decoder registers each
object before reading its contents, so cycles work.

**Map entries are written in key order** (by their encoded bytes), so equal
worlds always encode to equal bytes, whichever order Go's maps happen to keep.
That is what makes the strongest test possible: see below.

**The decoder writes private fields.** `settable` uses
`reflect.NewAt(…, UnsafeAddr)` to make unexported fields writable. The codec is
the World's own serializer, so reading and writing its private state is the
point.

### Loading

`LoadEngine` checks the header, unzips the body, decodes the Config, and builds
a fresh `newWorld(cfg, …)`. It does not call `generate`. Then it decodes the
World *into* that fresh world. Every saved field is overwritten. Fields the
codec skips keep the fresh world's values:

| Not saved | Why | Restored by |
| --- | --- | --- |
| `World.subscribers` | closures over the world | `newWorld` registers them again on the new world |
| `flowField.seed` / `goal` | closures (funcs are never written) | `afterLoad` rebinds them (`facilitySeed`/`Goal`, `frontierSeed`/`Goal`) |
| `World.snapGrid` | under `TilesLive` its pages alias `tiles`' pages, which the codec cannot keep | the first publish after the load builds it from scratch and reports `TileChanges.All` |
| `World.tileSharing` | it belongs to the host. The browser calls `ShareLiveTiles` on a loaded engine as on a new one | — |
| `World.preview` | it holds a mutex that frontends' goroutines take, and it is only a cache | `afterLoad` makes a new one when fog is off |

Not saved because it is not in the World: the engine's perf history, which flow
field a frontend is showing, and everything a frontend holds (camera, open
panels, the browser's 2,000-line log history). The engine's own log ring *is*
saved.

### Frontends

- **CLI:** `-load PATH` plays a save instead of generating a world, and the
  save's own settings replace the settings file and the flags. `-save PATH`
  writes the game when the run ends, whether by quitting, `-duration` or
  Ctrl+C: `finish` waits for `Run` to return and then saves on the main
  goroutine. A headless `-load` of a game saved paused is unpaused, since
  nothing headless could resume it. See [cli.md](./cli.md).
- **TUI:** `ctrl+s` writes `mars-sim-<seed>-<time>.marssave` in the working
  directory. The save runs on the engine goroutine through the `SaveGame`
  command, and the footer reports the result for a few seconds.
- **Browser:** the Game tab's **Save game** button, or Ctrl/⌘+S, downloads
  `mars-sim-<seed>-t<tick>.marssave` (the tick is read from the file's own
  header). **Load game…** reads one through a file picker. The worker calls
  the `save` / `load` exports between `advance` slices, when the world is at
  rest. `load` returns what `start` does, so the page reuses the new-game path
  (`resetGame`, `began`). A file that fails to load leaves the running game
  as it was and shows the reason.

### What it costs

Measured natively on an M-series Mac:

| World | Ticks | File | Save | Load |
| --- | --- | --- | --- | --- |
| 80×40 (the default), 6 colonists | 3,000 | 26 KB | 4 ms | 5 ms |
| 2048×2048, 60 colonists | 3,000 | 0.8 MB | 53 ms | 44 ms |
| 512×512 in the browser | 376,345 | 628 KB | — | — |

Files track the colony, not the map. Every per-tile grid is a
[`pagedGrid`](./sparse-grids.md), and only generated chunks have pages, so a
10000×10000 map saves the few chunks the colony has touched.

## Why it is this way

- **The whole graph, caches included, not the authoritative state
  alone.** The plan in [browser-frontend.md](./browser-frontend.md) was to save
  tiles, entities and so on, and rebuild derived state (regions, rooms, flow
  fields, the job board, the spatial index) on load. It also named the risk:
  if a rebuilt cache's contents or fill order ever decides a tie, the loaded
  game drifts. Saving the caches too removes that risk instead of guarding
  against it. It also removes a large hand-written rebuild path that every new
  cache would have to join. Flow fields mid-repair, generation stamps and
  memoised per-tick values all load as they were. The cost is a few hundred
  KB of scratch that compresses well.
- **Reflection, not a hand-written serializer.** `World` has around 150
  fields and reaches dozens of types, and it grows every week. A hand-written
  encoder is a second copy of all of that, and it silently drops whatever
  field someone forgets to add. With reflection, a new field is saved with no
  edit at all. The handful of cases reflection cannot handle (funcs,
  interfaces, sharing it cannot reproduce) fail loudly in tests, as described
  next.
- **Not `encoding/gob`.** Gob writes only exported fields, and almost all of
  the World is unexported. It also flattens pointers, so two references to one
  `*Ship` would load as two ships.
- **Not a replay of the commands.** The sim is deterministic, so a save could
  be the seed, the config and the player's commands, replayed on load. That
  file would be tiny. But loading would take as long as the game had run, and
  it would not capture the state, which is what the issue asked for. Any
  change to the sim would also make every old save replay into a different
  game without any error.
- **Refuse what cannot be kept.** Ids preserve sharing of whole objects. They
  cannot preserve a pointer *into* something, such as `&slice[i]` or two
  different slices over one backing array: the loader would give each its own
  copy. The encoder records the memory every pointee and slice covers, and
  `checkOverlaps` refuses a graph where two ranges overlap. The first run
  caught real sharing this way: `w.cognition` is a struct copy of
  `w.cfg.Cognition` and shares its slices. Giving slices ids fixed that.
  Identical slice headers now load as one slice. Only partial overlap is an
  error.
- **Empty slices get no shared id.** Go gives every zero-length allocation the
  same address. Distinct empty slices in the saved world all load at that one
  address, so on the next save they looked like one shared slice and encoded
  differently. That made two equal worlds encode to different bytes. The
  value-by-value `saveDiff` saw no difference at all, which is how it was
  found.
- **A layout hash, not field names in the stream.** Writing names, as gob
  does, would let a save survive an added field. But a save that loads into a
  World it was not written for is the worst outcome: it plays on, subtly
  wrong. The issue explicitly accepts that changes will break old saves. A
  hash fails up front and names both commits. If compatibility is ever
  wanted, put field names in the stream and match by name. The codec is the
  only place that would change.
- **The test is the whole state.** `TestSaveLoadPlaysOnIdentically` runs N
  ticks, saves and loads, then steps the straight run and the loaded run side
  by side, comparing their *encoded bytes* every 25 ticks. Because map order
  is fixed, equal bytes mean equal state: every cache, every claim, every RNG
  stream. A cache that loads slightly wrong is caught before it ever decides
  anything. When the bytes differ, the failure names the lockstep fingerprint
  field ([determinism.md](./determinism.md)) or, failing that, the first path
  `saveDiff` finds. It runs on the mechanics config and on the game's
  scarcity defaults.

## Extending it

- **A new field or type:** nothing to do. It is saved, and
  `TestSaveLoadPlaysOnIdentically` proves it loads.
- **A new func field** (a closure on a new struct):
  `TestSaveFuncFieldsAreRebound` fails. Rebind it in `World.afterLoad`, and
  add it to the test's list.
- **A new interface field:** the encoder refuses any concrete type that is not
  in `saveInterfaceTypes`. Add it there.
- **State that must not be saved** (aliasing the codec cannot keep, a mutex
  another goroutine takes, something the host owns): tag it `save:"-"`, and
  either let `newWorld` provide it or restore it in `afterLoad`.
- **Map keys must be pointer-free.** Entries are sorted by key bytes, and a
  key holding a pointer would get an id. The encoder refuses such a map.
- **Any layout change breaks existing saves.** That is intended. Bump
  `saveFormat` only when the framing or the codec's grammar changes.
- **Not built yet:** autosave and saves kept in the browser (the
  [OPFS](./browser-frontend.md#6-save-and-load) plan). The browser only
  downloads files today, so a save survives only as the file the player kept.

## Related

- [rng-streams.md](./rng-streams.md): the streams whose PCG state a save
  carries.
- [determinism.md](./determinism.md): the invariant a load must preserve, and
  the fingerprint the round-trip test reports through.
- [browser-frontend.md](./browser-frontend.md): the original save/load plan,
  and where saves live in the browser.
- [cli.md](./cli.md), [frontend-tui.md](./frontend-tui.md),
  [frontend-web.md](./frontend-web.md): the controls.

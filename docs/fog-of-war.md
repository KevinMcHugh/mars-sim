# Fog of war

> Part of the [mars-sim documentation](./README.md).

## What it is

The colony only sees the rock it has dug up to. A tile is **explored** once
something has changed the terrain within one tile of it, and until then a
frontend draws nothing there — no ore, no cavern shape, and no alien lurking in
it. The starting frame is the landing cavern plus the rim of rock around
it, floating in an otherwise blank map; mining peels that rim outward one tile at
a time.

It costs the simulation next to nothing and is off the per-frame path entirely.
The flag has one simulation meaning: an unexplored *floor* tile is a natural
cavern the colony has not found yet, which colony-facing systems ignore, and
where aliens stay dormant (see [caverns.md](./caverns.md)).

## Source

- [`internal/sim/world.go`](../internal/sim/world.go) — `Tile.Explored`, `World.revealAround` / `reveal` / `Explored`, and the `SetTerrain` call that lifts the fog.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `Snapshot.FogOfWar`, `Snapshot.ExploredAt`, and `Stats.ExploredTiles`, the surface a frontend reads.
- [`internal/sim/config.go`](../internal/sim/config.go) — the `fog-of-war` knob.
- [`internal/ui/tui/view.go`](../internal/ui/tui/view.go) — `renderMap`'s fog runs, `fogStyle`/`fogCells`, `terrainLabel`, and the legend swatch.
- [`internal/sim/fog_test.go`](../internal/sim/fog_test.go) — what worldgen reveals, what a dig reveals, that reveals reach the published grid, and that exploration never goes backwards.
- [`internal/sim/bench_test.go`](../internal/sim/bench_test.go) — `BenchmarkPublishSmallColonyOnHugeMap*`, where the reveal's page cost shows up.
- [`internal/sim/caverns.go`](../internal/sim/caverns.go) — natural caverns, the one thing generated under the fog; `revealAround`'s cavern flood lives in world.go. See [caverns.md](./caverns.md).
- [`internal/ui/tui/fog_test.go`](../internal/ui/tui/fog_test.go) — hidden rock, hidden entities, exact row widths with fog, and the inspector's "unexplored".

## How it works

### One flag, one writer

`Tile.Explored` is a bool on the tile, and `SetTerrain` — already the only writer
of the tile grid, and the funnel every derived system hangs off (see
[world.md](./world.md)) — is the only thing that sets it:

```go
w.revealAround(p)   // p and its eight neighbors are no longer unknown
...
w.tiles[i].Terrain = t
```

(The reveal comes *before* the write, so the changed tile is revealed as
whatever it was. That keeps "unexplored and not Rock" meaning exactly
"undiscovered cavern floor". The one other terrain writer, worldgen's
`carveHidden`, goes through the same `setTerrain` with the reveal switched off —
that is how natural caverns start out under the fog.)

`revealAround` marks `p` and its eight neighbors; `reveal` marks one tile and
dirties its page so the next `Snapshot` carries it. That is the whole mechanism.
There is no visibility pass, no per-colonist line of sight, and no per-tick work
at all: fog is lifted by the same call that changes the map, and only ever on the
handful of tiles around a change.

The rule "a terrain change reveals its neighborhood" is why worldgen needs no
special case. Carving the cavern is `SetTerrain(..., Floor)` per cell, so the
cavern and the ring of rock touching it come out explored and everything past
them dark. Mining one frontier rock into floor reveals the next ring behind it.

The one reveal that goes further is breaking into a natural cavern: if the ring
`revealAround` uncovers holds undiscovered cavern floor, it keeps flooding
through that floor until the whole connected cave system and its rock rim are
revealed. See [caverns.md](./caverns.md).

Exploration only ever goes from false to true. Nothing un-reveals a tile, so
walling off a corridor or building over the floor that revealed a rock face does
not put it back in the dark — the colony has been there.

### Reading it

Frontends do not read `Tile.Explored`; they ask the snapshot:

```go
func (s *Snapshot) ExploredAt(p Point) bool
```

which reports `true` for every in-bounds tile when `Snapshot.FogOfWar` is off.
That indirection is the point: with fog off the flag is still maintained
exactly as with it on (the simulation needs it to tell undiscovered caverns
apart), but *only* around what the colony has touched, so a huge map's tile
array stays the mostly-untouched zero pages that make publishing a frame cheap
(see [snapshot-tile-grid.md](./snapshot-tile-grid.md)). Marking 49 million
tiles explored to say "there is no fog" would have cost more than the feature
saves. (Before natural caverns, fog off marked nothing at all; the colony-sized
cost of marking anyway is the same one fog-on always paid.) It also means a hand-built `Snapshot` — a test fixture, another
frontend's scratch frame — renders the whole map rather than a blank screen,
because its zero value is "no fog".

`World.Explored(p)` is the same question against live state, for the engine
side, and likewise says yes to everything with fog off. Simulation code that
means "has the colony found this?" calls `World.discovered(p)`, which reads the
flag whatever the fog setting.

### Counting it

`World.exploredCount` tracks how many tiles have ever been revealed,
incremented the one time each tile's `Explored` flips in `reveal` — the
same "maintained incrementally, never rescan the grid" pattern
`terrainCounts` already uses for excavation progress (see
[world.md](./world.md)). `Snapshot.Stats.ExploredTiles` publishes a copy of
it every frame. With fog of war off the counter still moves, but the stat is
published as zero (`exploredTilesStat`) — a caller wanting "how much is
explored" checks `Snapshot.FogOfWar` first, the same way `ExploredAt` does,
since with fog off every tile already reads as explored. The TUI's lore tab (see [lore.md](./lore.md) and
[frontend-tui.md](./frontend-tui.md)) is what actually shows this, as a
percentage of the map's area.

### What the TUI does with it

`renderMap` asks `ExploredAt` per tile and, for an unexplored one, draws neither
terrain nor occupant:

- **Terrain** is replaced by a blank. Blank, not a glyph: the point is that there
  is nothing to see. Open floor is blank too, so fog gets a faintly shaded
  background (`fogStyle`, an `AdaptiveColor` so it reads on light and dark
  terminals) to tell "unexplored" from "dug out and empty". A background colour
  paints no character, so a fogged row is still exactly `cols*tileWidth` cells —
  the invariant the whole map depends on (see
  [terminal-cell-widths.md](./terminal-cell-widths.md)) — and fog is deliberately
  *not* in the glyph registry, since there is no glyph to measure.
- **Entities** on unexplored tiles are skipped. An alien dormant in an
  undiscovered cave is exactly what the fog is for; drawing it on an otherwise blank tile
  would also just look like a bug. Colonists and pets live on floor, which is
  always explored, so nothing the player owns can hide from them.
- **Runs, not tiles.** Consecutive fog tiles are emitted as one styled run. Early
  on most of the screen is fog, and a pair of escape sequences per tile is the
  kind of ANSI bill that already made redraws sluggish once (see *Redraw cost* in
  [frontend-tui.md](./frontend-tui.md)); a run costs the same for eighty tiles as
  for one.
- **The inspector** (`terrainLabel`) reports an unexplored tile as
  `unexplored` rather than naming the rock underneath, which would hand the map
  straight back. The sidebar legend gains an `unexplored` swatch, but only when
  the frame says fog is on.

### The knob

`fog-of-war` (default **on**) turns the display off: `ExploredAt` says yes to
everything and the map draws exactly as it did before this feature. The
simulation still tracks what the colony has discovered underneath, because
undiscovered natural caverns depend on it (see [caverns.md](./caverns.md)). See [configuration.md](./configuration.md) and
[config-file.md](./config-file.md).

## Why it is this way

- **Reveal on terrain change, not per-tick visibility.** A colonist's sight
  radius would mean a per-entity scan every tick and a map that dims behind
  whoever walked away — neither of which the player wants for a mining colony,
  where "have we dug to here?" is the actual question. Hanging the reveal off
  `SetTerrain` makes the feature free and makes it impossible for a new system to
  change the map without lifting the fog over it.
- **A tile field, not a set.** `map[Point]struct{}` of explored tiles would grow
  to the size of the excavated colony and be copied or shared per frame, while
  the bool rides the page-shared grid a frontend already holds — the tilegrid
  doc's own advice for a new tile field (see
  [snapshot-tile-grid.md](./snapshot-tile-grid.md)).
- **It is world state, not a display setting.** What the colony has seen is a
  fact about the colony, the way its room graph is; the UI's job is to draw it.
  A frontend that re-derived "rock bordering floor" for itself would be
  reinventing a game rule, and every other frontend would have to reinvent it
  identically. The filter menu is the counter-example of a genuine display
  setting (see [frontend-tui.md](./frontend-tui.md)) — it changes nothing about
  the world.
- **`Explored` sits beside `Terrain` and `Composition`, not after the counters.**
  `Tile` is two enum bytes and two word-aligned ints. A bool declared after the
  ints grows the struct from 24 bytes to 32; in the padding after the enums it
  costs nothing. A 7000x7000 map holds 49M tiles and pages of them are what
  publishing a frame copies, so the eight bytes were worth the field ordering.
- **What it costs.** A dig used to dirty one published page; the reveal around it
  touches up to three (the neighborhood spans three rows, and on a wide map
  consecutive rows are more than a page apart), so publishing a frame that
  contains a dig copies more. `BenchmarkPublishSmallColonyOnHugeMap2500` — 60
  colonists mining flat out, which is the worst case for reveals — went from
  ~916 µs and ~245 KB per published frame to ~968 µs and ~295 KB: about 6% more
  time and 20% more bytes. Most of those nine `reveal` calls are no-ops that
  dirty nothing, which is why it is not 3x. Finer-grained paging would claw the
  rest back and is not worth its complexity; if it ever is, `tilePageBits` is the
  knob (see [snapshot-tile-grid.md](./snapshot-tile-grid.md)).
- **No `TileChanged` for a reveal.** The event bus exists so the job board and
  flow fields can stay current, and neither cares who has seen what. Emitting
  reveals would put eight events on the bus per mined tile for no subscriber.
  The one reveal the job board does care about — a cavern breach, which turns
  the cave's walls into mining frontier — is rare and handled directly by
  `discoverCavernTile` rather than by eventing every reveal.
- **Fog is blank rather than a dark glyph.** A solid emoji square for the unknown
  would make an unexplored map look exactly as busy as an explored one, just in
  a different colour — the screen would read as a wall of tiles rather than as
  the absence of information.
- **Approaches dropped:** a `[]bool` parallel to `tiles` (needs its own paging
  and its own copy-on-write to be publishable — all of tilegrid.go again, for one
  bit); computing fog in the renderer from "does this rock border a non-rock
  tile?" (eight grid lookups per tile per frame, a game rule living in the
  frontend, and no memory — a corridor walled off behind the colony would go dark
  again); and marking every tile explored when fog is off (touches the whole tile
  array, undoing what makes large maps cheap).

## Extending it

- **A wider reveal** (a colonist's lantern, a scanner) is a change to
  `revealAround` — call `reveal` over a larger neighborhood — plus whatever
  triggers it. Keep reveals monotonic; the TUI, and the player's mental map,
  assume a tile once seen stays seen.
- **Revealing without digging** (a probe, a radar ping, a mutant's sense) should
  call `reveal`/`revealAround` directly rather than growing a second flag.
  Anything that reveals must dirty the page, which `reveal` already does — a
  reveal that skips `markTilePageDirty` leaves frontends rendering fog over rock
  the colony dug up frames ago (`TestRevealsReachThePublishedSnapshot`).
- **Hiding more from the player.** `Stats` in the header still counts the whole
  map (aliens alive, tiles excavated). Anything the fog should hide has to be
  computed over explored tiles instead, in `snapshot()`.
- **A new frontend** gets fog for free by asking `Snapshot.ExploredAt(p)` before
  it draws a tile or an occupant. Reading `Tile.Explored` directly works only
  with fog on, which is why `ExploredAt` exists.

## Related

- [world.md](./world.md) — the tile grid and `SetTerrain`, where reveals happen.
- [snapshot-tile-grid.md](./snapshot-tile-grid.md) — the page-shared grid the flag rides to frontends.
- [frontend-tui.md](./frontend-tui.md) — how the map draws the fog.
- [terminal-cell-widths.md](./terminal-cell-widths.md) — the row-width invariant fog has to respect.
- [configuration.md](./configuration.md) — the `fog-of-war` knob.
- [lore.md](./lore.md) — the lore tab's "how much is explored" line, read
  from `Stats.ExploredTiles`.

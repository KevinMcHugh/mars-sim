# Frontend: the terminal UI

> Part of the [mars-sim documentation](./README.md).

## What it is

The reference frontend: a [Bubble Tea](https://github.com/charmbracelet/bubbletea)
terminal UI. It is a **pure consumer** of the engine — it draws the latest snapshot
and forwards key presses as commands, holding no game state of its own beyond the
camera, the current screen, and the last frame. A web or GUI frontend would
implement the same consumer contract.

## Source

- [`internal/ui/tui/model.go`](../internal/ui/tui/model.go) — the Bubble Tea `Model`, update loop, input handling.
- [`internal/ui/tui/view.go`](../internal/ui/tui/view.go) — map, header, sidebar, footer rendering and layout.
- [`internal/ui/tui/render_roster.go`](../internal/ui/tui/render_roster.go) — the colonist roster and inspector.
- [`internal/ui/tui/glyphs.go`](../internal/ui/tui/glyphs.go) — terrain and entity glyphs.
- [`main.go`](../main.go) — `runTUI` (and `runHeadless`, the no-UI alternative).

## How it works

### The consumer loop

`Model` holds the `*sim.Engine` (to `Send` commands) and the snapshot channel.
`Init` issues `waitSnap`, a Bubble Tea command that blocks on the channel and
turns the next frame into a `snapshotMsg`; `Update` stores the frame and re-issues
`waitSnap`, so the UI keeps following the sim. A closed channel (engine shut down)
quits the program. The model never mutates or reads live world state — only
snapshots (see [architecture.md](./architecture.md)).

### Two screens

`viewMode` switches between the **map** (default) and the **roster**. `tab`
toggles them. Global keys (`handleKey`) work on both screens; the rest dispatch to
`handleMapKey` or `handleRosterKey`.

- **Map** (`renderMap`): draws a camera-windowed view of the tile grid, two
  terminal cells per tile, overlaying entity glyphs (aliens win position ties).
  A sidebar shows a legend and the tail of the event log; the header shows tick,
  speed, pause state, and `Stats` counts, including built dormitory beds.
- **Roster** (`renderRoster`): a scrolling, ID-sorted colonist list plus a
  detail pane for the selected colonist — name, attributes, HP, needs, the
  eight-slot inventory, and traits.

### Controls

| Key | Action |
| --- | --- |
| `space` | pause / resume (`TogglePause`) |
| `+` / `-` | faster / slower (`SetTicksPerSecond`, ±2) |
| `c` / `a` / `x` / `m` | spawn colonist / alien / cat / mouse (`Spawn`) |
| arrows or `hjkl` | pan the camera (map) / move selection (roster) |
| `tab` | toggle map ↔ roster |
| `q` / `esc` | quit |

Every key that changes the simulation becomes a `Command`; the UI never touches
the world directly.

### Glyphs

Each glyph is chosen to render **two terminal cells wide** so the grid stays
aligned (open floor is two spaces). If the map ever looks sheared, the terminal is
sizing emoji as one cell instead of two. Colonists show distinct "fleeing",
"talking", and "stomping" glyphs for those activities; dormitory beds render as
🛏️.

### Headless mode

`runHeadless` (in `main.go`) is a second, non-graphical consumer of the same
snapshot channel: it prints a stats line about once a second. It is the tool for
CI, profiling, and eyeballing balance without a TTY — proof that the frontend
contract is genuinely frontend-agnostic.

## Why it is this way

- **Pure consumer** keeps the sim/render split honest: the UI cannot corrupt game
  state or slow the sim, and swapping frontends is a matter of implementing the
  same two-method contract.
- **Blocking `waitSnap` re-issued each frame** is the idiomatic Bubble Tea way to
  turn a channel into a message stream, and it naturally paces the UI to the
  engine's drop-stale-frames publishing.
- **Two-cell glyphs** are the pragmatic fix for terminal grid alignment with
  emoji.

## Extending it

- **Show more data**: add a field to `Snapshot`/`EntityView`/`Stats` in `sim` and
  render it; do not reach into engine internals.
- **A new control**: add a key case that `Send`s a `Command` (add the command
  type in `sim` if needed — see [architecture.md](./architecture.md)).
- **A new glyph**: add it in `glyphs.go` and map the terrain/kind/state to it.
- **A different frontend entirely**: implement a consumer of `Subscribe()` frames
  that `Send`s commands; `runHeadless` is the minimal example.

## Related

- [architecture.md](./architecture.md) — the snapshot/command contract this implements.
- [inventory.md](./inventory.md) — what the roster's inventory view shows.
- [personality.md](./personality.md) — the attributes and traits the inspector shows.

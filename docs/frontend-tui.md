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
- [`internal/ui/tui/render_jobboard.go`](../internal/ui/tui/render_jobboard.go) — the job board: queued projects and their tasks.
- [`internal/ui/tui/glyphs.go`](../internal/ui/tui/glyphs.go) — terrain and entity glyphs.
- [`main.go`](../main.go) — `runTUI` (and `runHeadless`, the no-UI alternative).

## How it works

### The consumer loop

`Model` holds the `*sim.Engine` (to `Send` commands) and the snapshot channel.
`Init` issues `waitSnap`, a Bubble Tea command that blocks on the channel and
turns the next frame into a `snapshotMsg`; `Update` stores the frame and re-issues
`waitSnap`, so the UI keeps following the sim. A closed channel (engine shut down)
quits the program. The Bubble Tea renderer is capped at 30 redraws per second
(`tuiFPS` in `main.go`), independently of the simulation's tick rate. Since the
engine's one-slot subscription replaces stale frames, the renderer displays the
newest available snapshot rather than building a terminal view for every tick.
The model never mutates or reads live world state — only snapshots (see
[architecture.md](./architecture.md)).

### Three screens

`viewMode` cycles between the **map** (default), the **roster**, and the
**job board**. `tab` advances map → roster → job board → map; `esc` returns
straight to the map from either. Global keys (`handleKey`) work on every
screen; the rest dispatch to `handleMapKey`, `handleRosterKey`, or
`handleJobsKey`.

- **Map** (`renderMap`): draws a camera-windowed view of the tile grid, two
  terminal cells per tile, overlaying entity glyphs (aliens win position ties).
  A sidebar shows a legend and the tail of the event log; the header shows tick,
  speed, pause state, and `Stats` counts, including built dormitory beds.
- **Roster** (`renderRoster`): a scrolling, ID-sorted colonist list with each
  colonist's name, pronouns, and current status, plus a detail pane for the
  selected colonist — name, attributes, HP, needs, the eight-slot inventory,
  recent memories, and traits.
- **Job board** (`renderJobs`): a scrolling list of queued construction
  projects (facility rooms, dormitories), each with its tick-queued time,
  build progress, and assigned colonist count; a detail pane for the selected
  project lists every task tile with its terrain, build status, and builder
  (if any is currently assigned). With nothing queued, it instead reports any
  manual spawn/build orders still waiting for a build site. See
  [architecture.md](./architecture.md) for how projects and tasks work. (This
  screen is unrelated to the engine's internal `jobBoard`, which tracks the
  mining frontier — see `internal/sim/jobboard.go`.)

### Spawn and build menus

`s` and `b` each open a one-key picker (`menuKind` in `model.go`) rather than
sending a command directly: pressing `s` then `c`/`a`/`x`/`m` spawns a
colonist/alien/cat/mouse, and `b` then `f`/`d` queues a facility room or
dormitory. While a menu is open its prompt takes over the footer (styled
distinctly via `menuStyle`) on whichever screen it was opened from, every
other key is ignored except `esc` (cancel, no command sent) and `q`/`ctrl+c`
(quit); a recognized selection sends the `Command` and closes the menu. This
keeps the top-level key surface small as more spawnable/buildable kinds are
added — new options are new cases in `handleMenuKey` and `menuPrompt`, not new
top-level keys.

### Controls

| Key | Action |
| --- | --- |
| `space` | pause / resume (`TogglePause`) |
| `+` / `-` | faster / slower (`SetTicksPerSecond`, ±2) |
| `s` | open the spawn menu, then `c`/`a`/`x`/`m` for colonist/alien/cat/mouse (`Spawn`) |
| `b` | open the build menu, then `f`/`d` for facility room/dormitory (`OrderFacilityRoom`, `OrderDormitory`) |
| arrows or `hjkl` | pan the camera (map) / move selection (roster, job board) |
| `tab` | cycle map → roster → job board → map |
| `q` / `esc` | quit (`esc` returns to the map from roster/job board, or cancels an open menu) |

`s` and `b` work from every screen. Every key that changes the simulation
becomes a `Command`; the UI never touches the world directly.

### Glyphs

Each tile is allocated **two terminal cells** (open floor is two spaces). Emoji
are retained for readability, and `fitGlyph` pads or replaces glyphs according
to the renderer's width table. The map deliberately avoids per-tile cursor
positioning: emitting thousands of ANSI cursor sequences made redraws much less
responsive.

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
- **A 30 FPS renderer cap** keeps terminal formatting and I/O from scaling with
  high simulation speeds or large colonies. This is a renderer throttle, not a
  second input loop: Bubble Tea continues reading keyboard input independently,
  while `Update` applies keys as soon as they arrive.
- **Two-cell tile slots plus cursor pinning** preserve the readable emoji while
  handling terminals whose painted emoji width differs from the width table.

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

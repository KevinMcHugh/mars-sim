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
  An empty tile with refuse on it (`tileGlyph`) draws a body 🦴 or, failing
  that, a splatter 🩸 instead of its bare terrain — see
  [combat.md](./combat.md) and [sanitation.md](./sanitation.md). A sidebar shows
  a legend and the tail of the event log; the header shows tick, speed, pause
  state, and `Stats` counts, including built dormitory beds, incinerators, and
  refuse still on the floor.
- **Roster** (`renderRoster`): a scrolling, ID-sorted entity list — living
  colonists by default, plus aliens/cats/mice and/or graveyard entries once
  the filter menu (`f`) turns those on — with each row's name, pronouns (or
  kind, for anything without a `Profile`), and current status (or cause of
  death). The detail pane for the selection is the full colonist inspector —
  name, attributes, HP, a compact per-body-part wound summary, needs, the
  eight-slot inventory, recent memories, and traits — for a colonist, or a
  shorter identity/status/body-part view (`renderNonColonistDetail`) for
  anything else. See [combat.md](./combat.md) for the wound and graveyard
  data this displays.
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

`s` and `b` each open a picker (`menuKind` in `model.go`, options listed in
`spawnMenuItems`/`buildMenuItems`) rather than sending a command directly.
Within an open menu: `up`/`down` (or `k`/`j`) move the highlighted option,
`enter` submits whichever is highlighted, a shortcut letter (`c`/`a`/`x`/`m`
for spawn, `f`/`d` for build) jumps to and submits that option immediately,
`esc` cancels with no command sent, and `q`/`ctrl+c` still quits. Every other
key is ignored so the prompt stays open until answered.

Each menu remembers its own highlighted option (`spawnCursor`/`buildCursor` on
`Model`) across opens *and* across submits — moving the highlight and
submitting both update it — so repeating the same choice is just
reopen-and-confirm: `s` → navigate to mouse → `enter` once, then `s` → `enter`,
`s` → `enter` for two more mice, with no renavigating. While a menu is open its
prompt (current options, with the highlighted one bracketed) takes over the
footer (styled distinctly via `menuStyle`) on whichever screen it was opened
from. This keeps the top-level key surface small as more spawnable/buildable
kinds are added — new options are new entries in `spawnMenuItems`/
`buildMenuItems` plus a case in `submitMenuItem`, not new top-level keys.

### Controls

| Key | Action |
| --- | --- |
| `space` | pause / resume (`TogglePause`) |
| `+` / `-` | faster / slower (`SetTicksPerSecond`, ±2) |
| `s` | open the spawn menu — `↑↓`/`enter` to pick, or `c`/`a`/`x`/`m` for colonist/alien/cat/mouse directly (`Spawn`) |
| `b` | open the build menu — `↑↓`/`enter` to pick, or `f`/`d`/`t` for facility room/dormitory/trash room directly (`OrderFacilityRoom`, `OrderDormitory`, `OrderTrashRoom`) |
| `f` (roster only) | open the roster's filter menu — `↑↓`/`enter`/`space` to toggle the highlighted checkbox, or `d`/`n` for dead/non-human directly; no command sent, this only changes what the roster shows |
| arrows or `hjkl` | pan the camera (map) / move selection (roster, job board) |
| `tab` | cycle map → roster → job board → map |
| `q` / `esc` | quit (`esc` returns to the map from roster/job board, or cancels an open menu) |

`s` and `b` work from every screen; `f` only does anything on the roster
screen (`handleRosterKey`), since filtering only means something there. Every
key that changes the simulation becomes a `Command`; the UI never touches the
world directly — `f` is the one menu that *doesn't* send a `Command` at all,
since it only changes what `Model` itself displays.

### The filter menu

Unlike the spawn/build pickers, the filter menu (`menuFilter`) is a set of
checkboxes, not a one-shot choice, so it gets its own key handler
(`handleFilterMenuKey`) instead of the generic submit-and-close flow
(`handleMenuKey`): toggling a filter never closes the menu, since setting
both in one visit is the normal case. `Model.showDead`/`showNonHuman` hold
the two filters (both default off, so the roster's out-of-the-box view is
unchanged); `rosterEntries()` (`render_roster.go`) is what every roster
render calls instead of walking `Snapshot.Entities` directly — colonists are
always eligible, `showNonHuman` admits aliens/cats/mice, and `showDead`
additionally merges in `Snapshot.Graveyard` (subject to the same kind
filter, so a dead mouse needs both filters on). The title above the list
(`rosterTitle`) names which filters are active so a longer-than-expected list
is never a mystery.

### Glyphs

Each tile is allocated **two terminal cells** (open floor is two spaces), and
every glyph reaches the terminal through `fitGlyph`, which renders it in exactly
that many cells. Glyphs are not loose constants: they live in a registry with a
declared width and an ASCII fallback, and a startup probe measures them against
the real terminal. This is the part that used to break the grid — the whole
story, and the rules a new glyph has to follow, are in
[terminal-cell-widths.md](./terminal-cell-widths.md).

The map deliberately avoids per-tile cursor positioning: emitting thousands of
ANSI cursor sequences made redraws much less responsive. Correctness comes from
fitting each line to a known cell count instead.

### Panel widths

Panel width constants (`sidebarWidth`, `rosterListWidth`, `jobListWidth`) are
**total footprints including the border**. `lipgloss`'s `Style.Width` sets the
content box — padding in, border out — so renderers pass
`Width(total - borderCells)`.

When the terminal is too narrow for two panels side by side, the second one is
dropped (`sidebarFits`, `splitPanels`) rather than squeezed. Flooring it at a
minimum instead is what used to push the sidebar's border off the right edge.

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
- **Two-cell tile slots, a vetted glyph registry, and a startup width probe**
  preserve the readable emoji without trusting any width table to be right about
  a terminal we have never run on. See
  [terminal-cell-widths.md](./terminal-cell-widths.md).
- **The filter menu is a display setting, not a `Command`**: what the roster
  shows is purely a `Model` concern (like the camera or the current screen),
  so there is nothing for the engine to know or validate. Routing it through
  `Send` anyway would have meant inventing a command whose only effect is on
  the UI's own state.
- **Filters default off**: the roster's behavior before this feature — living
  colonists only — stays the default so nothing about the existing view
  changes unless a player opens the filter menu and asks for more.

## Extending it

- **Show more data**: add a field to `Snapshot`/`EntityView`/`Stats` in `sim` and
  render it; do not reach into engine internals.
- **A new control**: add a key case that `Send`s a `Command` (add the command
  type in `sim` if needed — see [architecture.md](./architecture.md)).
- **A new glyph**: add the constant *and* a `glyphRegistry` entry in
  `glyphs.go`, then map the terrain/kind/state to it. The tests reject a glyph
  whose width terminals would disagree about — see
  [terminal-cell-widths.md](./terminal-cell-widths.md).
- **A new roster filter**: add a field to `Model`, an entry to
  `filterMenuItems`, a case in `toggleFilter`/`filterOn`, and a clause in
  `rosterEntries`'s `include` closure — the list, count, and title all update
  automatically since they're all derived from `rosterEntries()`.
- **A different frontend entirely**: implement a consumer of `Subscribe()` frames
  that `Send`s commands; `runHeadless` is the minimal example.

## Related

- [architecture.md](./architecture.md) — the snapshot/command contract this implements.
- [combat.md](./combat.md) — body parts, weapons, and the gore glyph.
- [inventory.md](./inventory.md) — what the roster's inventory view shows.
- [personality.md](./personality.md) — the attributes and traits the inspector shows.
- [terminal-cell-widths.md](./terminal-cell-widths.md) — how glyph widths are measured and kept honest.

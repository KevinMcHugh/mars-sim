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

### Map and details panels

`viewMode` cycles between the **map** (default) and three **details panels**:
the **roster**, **job board**, and **storage**. `tab` advances map → roster →
job board → storage → map; `esc` returns straight to the map from any details
panel. Global keys (`handleKey`) work everywhere; the rest dispatch to the
active panel's handler.

- **Map** (`renderMap`): draws a camera-windowed view of the tile grid, two
  terminal cells per tile, overlaying entity glyphs (aliens win position ties).
  An empty tile with refuse on it (`tileGlyph`) draws a body 🦴 or, failing
  that, a splatter 🩸 instead of its bare terrain — see
  [combat.md](./combat.md) and [sanitation.md](./sanitation.md). A tile the
  colony has not dug up to yet draws neither terrain nor occupant, just a
  faintly shaded blank — see [fog-of-war.md](./fog-of-war.md). A sidebar shows
  a legend and the tail of the event log; the header shows tick, speed, pause
  state, and `Stats` counts, including built dormitory beds, incinerators, and
  refuse still on the floor.
- **Roster** (`renderRoster`): a scrolling, ID-sorted entity list — living
  colonists by default, plus aliens/cats/mice and/or graveyard entries once
  the filter menu (`f`) turns those on — with each row's name, pronouns (or
  kind, for anything without a `Profile`), and current status (or cause of
  death). The detail pane for the selection is the full colonist inspector —
  name, attributes, HP, a compact per-body-part wound summary (`bodyPartLines`,
  which lists only the parts that entity actually has, so a mutant's grown
  limbs appear and nobody else shows empty ones — see
  [mutation.md](./mutation.md)), needs, the
  eight-slot inventory, traits, family, affinities, and the colonist's whole
  remembered history — for a colonist, or a shorter identity/status/body-part
  view (`nonColonistDetailLines`) for anything else. It is taller than the
  panel and scrolls (see The scrolling inspector). See
  [combat.md](./combat.md) for the wound and graveyard data this displays.
- **Job board** (`renderJobs`): a scrolling list of queued construction
  projects (facility rooms, dormitories), each with its tick-queued time,
  build progress, and assigned colonist count; a detail pane for the selected
  project lists every task tile with its terrain, build status, and builder
  (if any is currently assigned). With nothing queued, it instead reports any
  manual spawn/build orders still waiting for a build site. See
  [architecture.md](./architecture.md) for how projects and tasks work. (This
  screen is unrelated to the engine's internal `jobBoard`, which tracks the
  mining frontier — see `internal/sim/jobboard.go`.)
- **Storage** (`renderStorage`): a position-sorted list of built chests with
  arrow navigation. Its detail pane shows the selected chest's occupied slots,
  total item count, and 48-slot capacity.

### The scrolling inspector

A colonist's inspector does not fit in the panel, and the fix for that used to
be to show less: memories were capped at the last five, and everything past
the panel's last row was simply clipped by `lipgloss`. Both were invisible to
the player — a clipped panel looks exactly like a panel that ends there — so
"what does this colonist actually remember?" was a question the UI could not
answer.

`Model.detailScroll` is the first content line the detail panel shows.
`shift+↑`/`shift+↓` move it a line and `pgup`/`pgdn` a screenful; the plain
arrows stay on the list, since moving between colonists is what the roster is
mostly for. It resets to the top whenever the selection or a roster filter
changes — the offset belongs to the colonist being read, not to the panel.

The renderer splits into two halves for this: `detailLines` builds the content
as one string per terminal line (`nonColonistDetailLines` for anything without
a `Profile`), and `scrollDetail` windows that to the panel's height, fitting
only the lines it actually shows, so the per-frame width work stays
proportional to the visible rows rather than to a 64-memory history. When the
content overflows, the panel's bottom row becomes a position line
(`scrollStatusLine`: `↑↓  18-35 of 50  shift+↑↓ scroll`), with arrows only for
the directions that have more to show. Content that fits is drawn untouched,
with no position line.

A run of the same routine event is already one `Memory` by the time the
renderer sees it (see [memories.md](./memories.md)), so the panel does not
need its own de-duplication pass: `memoryLine` just renders a memory with
`Count > 1` as a span plus an occurrence count —
`t1607-1630: Finished mining. (x12)` — and everything else as the plain
`t1586: Had a meal.`. Collapsing happens in the simulation rather than here
because the memory *is* one memory (it fills one of the colonist's 64 slots,
not twelve); a frontend that folded lines at render time would still be
reading a history where a mining shift had pushed out everything else.

`scrollDetail` clamps the offset as well as `Update` does. Update-side
clamping (`clampDetailScroll`, via `detailExtent`, which re-derives the
content length and panel height the way the renderer does) is what makes one
`shift+↑` after a run of `pgdn` move the panel instead of unwinding a
runaway offset; render-side clamping is for the content shrinking on its own
between keypresses, as a colonist's state changes.

One layout trap is worth knowing: `Style.Height` sizes the content box and
`Style.MaxHeight` trims the *finished* block, border included, so they are two
cells apart. Both roster panels (and both job-board panels) passed the content
height to each, which quietly ate their last row and bottom border —
`TestListScreensFillTerminalHeight` pins the fix, and it is the row the
position line lives on.

### Spawn and build menus

`s` and `b` each open a picker (`menuKind` in `model.go`, options listed in
`spawnMenuItems`/`buildMenuItems`) rather than sending a command directly.
Within an open menu: `up`/`down` (or `k`/`j`) move the highlighted option,
`enter` submits whichever is highlighted, a shortcut letter (`c`/`a`/`x`/`m`
for spawn, `f`/`d`/`t`/`r` for build) jumps to and submits that option immediately,
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

### Map inspection and details panels

Pressing `i` on the map enters inspection mode. A reverse-video cursor starts
near the center of the viewport; arrows or `hjkl` move it one world tile at a
time and pan the camera when it reaches an edge. The sidebar shows the cursor
coordinate and terrain — or `unexplored` (`terrainLabel`) for a tile still under
fog, since naming the rock there would hand back the map the fog is hiding. For a storage container it also shows occupied stacks,
and `enter` jumps directly to that container in the storage details panel.
`i` or `esc` closes inspection without quitting.

`tab` cycles **map → roster → jobs → storage → map**. Roster, jobs, and storage
are collectively the details panels. In storage, `up`/`down` or `j`/`k` selects
a chest from the position-sorted snapshot list; the inspector shows its occupied
slots and total capacity.

### Controls

| Key | Action |
| --- | --- |
| `space` | pause / resume (`TogglePause`) |
| `+` / `-` | faster / slower (`SetTicksPerSecond`, ±2) |
| `s` | open the spawn menu — `↑↓`/`enter` to pick, or `c`/`a`/`x`/`m` for colonist/alien/cat/mouse directly (`Spawn`) |
| `b` | open the build menu — `↑↓`/`enter` to pick, or `f`/`d`/`t`/`r` for facility room/dormitory/trash room/storage container directly |
| `i` (map only) | enter map inspection; arrows/`hjkl` move the cursor, `enter` opens a storage chest's details, and `i`/`esc` closes |
| `f` (roster only) | open the roster's filter menu — `↑↓`/`enter`/`space` to toggle the highlighted checkbox, or `d`/`n` for dead/non-human directly; no command sent, this only changes what the roster shows |
| arrows or `hjkl` | pan the camera (map) / move selection (roster, job board) |
| `shift+↑↓`, `pgup`/`pgdn` (roster only) | scroll the selected colonist's inspector a line / a screenful |
| `tab` | cycle map → roster → job board → storage → map |
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
additionally merges in every dead colonist from `Snapshot.Deceased` (the
permanent, by-ID archive — see docs/combat.md) plus any non-colonist entries
from `Snapshot.Graveyard` (bounded, subject to the same kind filter, so a
dead mouse needs both filters on); a colonist's own `Graveyard` entry, if
any, is skipped there so one death is never listed twice. `colonistNames()`
merges `Deceased` in too, so a living colonist's FAMILY section can name a
dead relative instead of leaving their slot blank. The title above the list
(`rosterTitle`) names which filters are active so a longer-than-expected list
is never a mystery.

### Glyphs

Fog is the one thing the map draws that is **not** a glyph: an unexplored tile is
two spaces with a shaded background (`fogStyle`/`fogCells`), so there is no
symbol for the registry to measure and nothing for the probe to check. See
[fog-of-war.md](./fog-of-war.md).

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

### Redraw cost

The map screen is a full grid of emoji, and most of the cost of drawing one is
measuring it: every `ansi.StringWidth` call runs a grapheme segmentation, which
is at its slowest on emoji. Profiling found three places doing it for every line
of every frame, plus a large per-frame garbage bill, and each is now avoided
without dropping the guarantee it provided:

- **Map and sidebar join.** `lipgloss.JoinHorizontal` cannot know its blocks'
  widths, so it measured every line twice. `joinColumns` produces the same bytes
  from widths the layout already knows (`TestJoinColumnsMatchesLipgloss`).
- **Map rows.** Rows are exact by construction rather than re-fitted; see
  [terminal-cell-widths.md](./terminal-cell-widths.md).
- **`clampFrame`.** Reuses widths of lines unchanged since the previous frame.
- **Sidebar.** Memoized: lipgloss's border, padding and wrapping dominate its
  cost, and it only changes with the event log, the panel height, the glyph
  set, or whether fog of war is on — which adds a legend row
  (`TestSidebarCacheInvalidates`).
- **Fog runs.** Unexplored tiles are emitted as one styled run per stretch
  rather than one per tile. Early on most of the screen is fog, and a pair of
  escape sequences per tile would put tens of kilobytes of ANSI on every frame
  — the same bill that made per-tile cursor positioning too slow to keep.
- **Occupancy index.** `renderMap` indexes entities by position; it now stores
  each occupant's glyph rather than copying its whole `EntityView` (profile,
  inventory, needs, relations, memories) into a map every frame.

The caches live in `renderCache`, behind a pointer shared by every copy of the
by-value `Model`. Bubble Tea calls `Update` and `View` on one goroutine, so they
need no lock, and a `Model` without a cache renders the same frames uncached
(`TestCachedAndUncachedFramesMatch`). Together this took a 200x60 frame with
entities moving every tick from about 1.7 ms to about 0.26 ms, with identical
output.

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
- [fog-of-war.md](./fog-of-war.md) — what the colony has seen, and how the map draws the rest.
- [combat.md](./combat.md) — body parts, weapons, and the gore glyph.
- [mutation.md](./mutation.md) — the uranium-rock and mutant-colonist glyphs, and the per-entity body parts the inspector lists.
- [inventory.md](./inventory.md) — what the roster's inventory view shows.
- [personality.md](./personality.md) — the attributes and traits the inspector shows.
- [terminal-cell-widths.md](./terminal-cell-widths.md) — how glyph widths are measured and kept honest.

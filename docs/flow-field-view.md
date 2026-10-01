# Flow-field view

> Part of the [mars-sim documentation](./README.md).

## What it is

A debugging overlay on the terminal map that shows one of the shared flow
fields (see [pathfinding.md](./pathfinding.md)): every tile the field reached is
tinted by its distance to the field's nearest goal, goal tiles are bright green,
and bare floor has the distance written on it. `f` on the map cycles through the
fields (one per tracked facility terrain — nutrient pods, toilets, beds,
incinerators, storage, scumhouses, forges, gun benches — then the mining
frontier) and back to off; `F` turns it off. It answers "why is everyone walking
*there*?" without a debugger.

## Source

- `internal/sim/flowview.go` — `FlowFieldRef`, the `ShowFlowField` command, the published `FlowFieldView`, and copying a field into one.
- `internal/sim/engine.go` — the engine remembers the requested field and attaches it in `publish`, reusing the last copy while the field is unchanged.
- `internal/sim/flowfield.go` — `flowField.version`, bumped by every build or repair.
- `internal/sim/flowview_test.go` — the view matches the field cell for cell; publishing every field every tick keeps a run in lockstep with an unwatched one; the engine starts and stops publishing on command.
- `internal/ui/tui/render_flow.go` — the colour ramp, cycling, the sidebar legend, and the inspect-cursor readout.
- `internal/ui/tui/render_flow_test.go` — cycling, drawing, the stale-frame guard, and the footer distance.

## How it works

**Opt-in, one field.** `Snapshot.FlowFields` always lists the fields (cheap: a
handful of refs, facility fields in terrain order, frontier last), but no
distances are copied until a frontend sends `ShowFlowField{Show: true, Field: r}`.
From then on every published frame carries `Snapshot.FlowField`, a
`FlowFieldView` of that one field, until `ShowFlowField{}` turns it off. The
choice lives on the `Engine`, not the `World`: it is presentation state, not
simulation state, so it is not saved and not part of determinism.

**Copying.** `World.flowFieldView` freshens the field (`ensureFresh`) and copies
it page by page into a `pagedGrid[int32]` holding `distance+1`, so an unwritten
page reads as unreachable for free and the copy is as sparse as the field
itself. It also records `Goals` (tiles at distance 0) and `Max` (largest finite
distance), which the TUI scales its ramp to. Each field has a `version` that
`ensureFresh` bumps whenever it actually builds or repairs; the engine keeps the
last view and its version and hands the same pointer to the next frame when
nothing changed. A view is never written after publication, so sharing it
across frames and goroutines is safe.

**Drawing (TUI).** `renderMap` decides a tile's glyph as before, notes whether
it is *bare* floor (no refuse, scum, salt or occupant), and then `flowTile`
paints it: goal style for 0, one of eight ramp bands for 1..Max, and the
distance as two right-aligned digits on bare floor (blank past 99 — the tint
still says how far). Tiles the field never reached are drawn untouched. While
the overlay is on, the sidebar's legend is replaced by the field's name, goal
count, farthest distance, and the step range of each ramp band in use; with
inspection open, the footer adds the cursor tile's distance.

**Stale frames.** The command and the frames are asynchronous, so after `f` the
next frame may still carry the previous field (or none). `shownFlowField` only
returns the snapshot's view when it is the field the player last picked; until
then the map is drawn plain and the sidebar says it is waiting for the engine,
rather than labelling one field's distances with another's name.

## Why it is this way

- **Freshening a field to show it is safe for determinism, but only because of
  two existing invariants.** `ensureFresh` runs at most once per tick, and a
  repair always lands on exactly the distances a rebuild would
  (`TestFlowFieldRepairMatchesRebuild`); nothing in a build or repair touches an
  RNG. So publishing can change *when* the work happens, never *what* any
  colonist reads. `TestFlowFieldViewKeepsRunDeterministic` pins this: a world
  whose every field is copied every tick stays in lockstep with one nobody
  watches. If either invariant ever weakens, that test is the one that breaks.
- **Not showing the field as of its last build instead.** That would have
  avoided calling `ensureFresh`, but a field nobody has read yet (a toilet field
  before anyone needs a toilet) would show as empty, and the overlay would lag
  the colony by an unbounded amount — the opposite of a debugging tool.
- **The whole field, not the viewport.** Sending a camera rectangle with the
  command would copy less, but every pan would become a command and a round
  trip, and the overlay would flash blank at the edges while scrolling. The
  field is already sparse and only copied when it changes, so the whole-field
  copy is cheap enough for something that is off by default.
- **Engine-wide choice.** With several frontends subscribed, all see the field
  the last one picked. Only the TUI shows fields today; a per-subscriber choice
  can come when a second frontend wants it.

## Extending it

- **The browser frontend** needs the view on the wire: a new panel topic or
  frame section carrying the visible pages of the field (see
  [wire-format.md](./wire-format.md)) and a WebGL tint pass. `FlowFieldView`
  already has the page layout the tile grid uses.
- **A new shared field** (a new facility, or something other than a facility)
  shows up automatically if it is in `World.fields` or handled by
  `flowFieldRefs` / `flowFieldFor`; add it to both if it lives elsewhere.
- **Arrows instead of numbers** (the downhill direction) would show flow more
  directly, but arrow glyphs are ambiguous-width in many terminals; vet them
  through the glyph registry first ([terminal-cell-widths.md](./terminal-cell-widths.md)).

## Related

- [pathfinding.md](./pathfinding.md) — what the flow fields are and how they are repaired.
- [sparse-grids.md](./sparse-grids.md) — the paged grid the field and its view share.
- [determinism.md](./determinism.md) — the lockstep test this feature leans on.
- [frontend-tui.md](./frontend-tui.md) — the map, sidebar, and controls this extends.

# Flow-field view

> Part of the [mars-sim documentation](./README.md).

## What it is

A debugging overlay on the map, in both the terminal and the browser, that
shows one of the shared flow fields (see [pathfinding.md](./pathfinding.md)):
every tile the field reached is tinted by its distance to the field's nearest
goal, from bright green on the goals through eight bands to dark red. `f`
cycles through the fields (one per tracked facility terrain — nutrient pods,
toilets, beds, incinerators, storage, scumhouses, forges, gun benches — then the
mining frontier) and back to off; `F` turns it off. The TUI writes the distance
on bare floor; the browser has a *Flow* picker in the top bar and puts the
distance in the hover readout. It answers "why is everyone walking *there*?"
without a debugger.

## Source

- `internal/sim/flowview.go` — `FlowFieldRef`, the `ShowFlowField` command, the published `FlowFieldView`, and copying a field into one.
- `internal/sim/engine.go` — the engine remembers the requested field and attaches it in `publish`, reusing the last copy while the field is unchanged.
- `internal/sim/flowfield.go` — `flowField.version`, bumped by every build or repair.
- `internal/sim/flowview_test.go` — the view matches the field cell for cell; publishing every field every tick keeps a run in lockstep with an unwatched one; the engine starts and stops publishing on command.
- `internal/ui/tui/render_flow.go` — the colour ramp, cycling, the sidebar legend, and the inspect-cursor readout.
- `internal/ui/tui/render_flow_test.go` — cycling, drawing, the stale-frame guard, and the footer distance.
- `internal/wire/encoder.go` — the frame's flow section: the shown field's tiles in view (`flowTiles`, via `FlowFieldView.Range`), and when it is sent. `wire.go` names the fields in Hello.
- `internal/wire/encoder_test.go` (`TestEncodeFlowField`) and the `changed` golden frame — the section's layout, its resend rules, and clearing.
- `cmd/mars-sim-wasm/main.go` — the `flow` command (`{"type":"flow","field":i}`, -1 for off).
- `web/wire/decode.js` — the flow section, as typed-array views.
- `web/src/map/renderer.ts` — the overlay's tint quads (`rebuildFlow`), `flowAt` for the hover readout.
- `web/src/map/palette.ts` — the ramp (`flowTint`, `flowBand`), the TUI's colors in hex.
- `web/src/ui/FlowControl.svelte` — the top bar's picker and the legend; `game.svelte.ts` holds `flowPick` (asked for) and `flowShown` (drawn).

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

**The browser.** The engine runs in a worker, so the field reaches the page
over the wire (see [wire-format.md](./wire-format.md)): Hello names the fields,
the page asks for one with the `flow` command, and frames carry a **flow
section** — the field's tiles inside the page's interest rectangle, with their
distances, plus the field's index, goal count and farthest distance in the
header. The encoder sends it only when the engine hands over a different
`FlowFieldView` (the field changed, or was switched on or off) or the interest
moved; otherwise the page keeps the last one. The renderer turns it into one
more layer of instanced tint quads, over filth and under creatures, filtered to
visible tiles and refiltered as pages arrive, exactly like filth. The legend
(`FlowControl.svelte`) reads `ui.flowShown`, which `main.ts` takes from the
renderer whenever a frame carries a flow section; the picker reads
`ui.flowPick`, what was asked for.

**Stale frames.** The command and the frames are asynchronous, so after `f` the
next frame may still carry the previous field (or none). `shownFlowField` only
returns the snapshot's view when it is the field the player last picked; until
then the map is drawn plain and the sidebar says it is waiting for the engine,
rather than labelling one field's distances with another's name. The browser
gets the same guarantee differently: the flow section names its own field, and
the legend is built from that, not from the picker.

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
  the last one picked. The browser has an engine of its own, so this only
  matters for two TUIs on one engine, which nothing does yet.
- **The browser gets only the view, unlike scum and salt.** Scum and salt are
  short lists that change rarely; a field covers every walkable tile of the
  colony and the frontier field changes on most ticks (every claim moves its
  goals). Sending only the interest rectangle bounds the section by the
  screen, and resending on a move is cheap because the interest only moves a
  page at a time. `FlowFieldView.Range` skips pages the field never reached,
  so a zoomed-out view of a huge map costs what the colony in it does. A
  field spans every level (see [stairs.md](./stairs.md)); `Range` takes the
  level to draw, and the browser asks for the landing level.
- **Flagged clearing, not an empty list.** An empty flow section is also what a
  field with no tiles in view looks like, so "off" is field -1, not F = 0.
- **The legend follows the frame, not the throttle.** Frame-driven UI
  refreshes at `UI_HZ`; a flow section is rare and, on a paused game, may be the
  only frame for a long time, so it updates the legend as it arrives (the
  first version waited for the throttle and showed nothing until unpaused).
- **No distances on the browser map.** Text in WebGL would mean a glyph atlas
  for digits and one more instanced layer; the hover readout gives the exact
  number, and the tint carries the shape.

## Extending it

- **Distances on the browser map** when zoomed in: a digits atlas and a
  sprite layer reading `renderer.flowDist`.
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
- [frontend-web.md](./frontend-web.md) — the browser map, top bar, and hover readout this extends.
- [wire-format.md](./wire-format.md) — the flow section and Hello's `flowFields`.

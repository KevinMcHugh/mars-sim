# Excavation orders

> Part of the [mars-sim documentation](./README.md).

## What it is

The player can order an area mined out. In the browser, the **Dig** tab arms a
tool, a drag on the map marks a rectangle, and **Order** buys the digging: every
rock tile in it that the colony has seen becomes one `WorkDig` order on the order
book, escrowed from the treasury. Whoever digs a tile is paid its wage
(`wage-dig`) when it is cleared and keeps the ore. It is the player's hand on
the same machinery the room planner uses ([labor.md](./labor.md)), not a new
kind of job.

## Source

- [`internal/sim/excavation.go`](../internal/sim/excavation.go) —
  `OrderExcavation` and `CancelExcavation` (the commands), `orderExcavation`,
  `cancelExcavation`, `maxExcavationTiles`.
- [`internal/sim/workorder.go`](../internal/sim/workorder.go) — `WorkDig`, and
  `fundProject` posting each task with its project's `workKind`.
- [`internal/sim/project.go`](../internal/sim/project.go) — `project.workKind`;
  the excavation is an ordinary project, claimed by `claimNearestTask`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) and
  [`internal/wire/boards.go`](../internal/wire/boards.go) — `DigWage` and
  `DigMax`, carried to the page as the market topic's `dig`.
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go) — the `dig`
  command (`hostAPI` 10, with `dig-cancel`).
- [`web/src/ui/DigPanel.svelte`](../web/src/ui/DigPanel.svelte), `dragArea` and
  `showDig` in [`web/src/main.ts`](../web/src/main.ts), the `areaTool` hooks in
  [`web/src/map/input.ts`](../web/src/map/input.ts), and `ui.dig` in
  `game.svelte.ts`.
- [`internal/sim/excavation_test.go`](../internal/sim/excavation_test.go).

## How it works

`orderExcavation` clamps the rectangle to the map, then collects every tile that
is `Rock`, `discovered` (the colony has seen it; fog is not something an order
can see through), not a reserved door tile, and not already a task of another
project. Those become the tasks of one project named `excavation`, issued by
`Community`, with `workKind: WorkDig`, and `fundProject` posts a work order for
each at `wageFor(Floor)`.

From there nothing is special. Builders claim dig tasks with
`claimNearestTask` like a room's, `jobBuild` clears the rock and `payWork`s the
task's order, the Jobs tab shows the project's progress, and `pruneProjects`
retires it and refunds any order a miner left unpaid. Tiles in the middle of a
large area become claimable as the tiles around them are cleared, because
`taskReachable` only asks for a walkable neighbour.

A command has no reply, so every outcome goes to the colony log: what was
ordered and what is escrowed, or why it was refused.

The page prices an order before it is sent, from the tile pages it holds and the
market topic's `dig` terms, and disables **Order** when the area has no rock,
is over `maxExcavationTiles`, or costs more than the treasury holds.

## Why it is this way

- **A project, not a new job type.** A dig task already is a `buildTask` with
  terrain `Floor`, and everything about claiming, paying and finishing it
  already worked. The only new thing is a name and a work kind.
- **`WorkDig` rather than `WorkBuild`.** The order book groups work by issuer and
  kind. Lumping an excavation in with room-building made the player's order
  impossible to find on the Market tab.
- **All or nothing.** The same rule as a room (see
  [labor.md](./labor.md)): half an area dug is not what was ordered, and a
  partly funded order would leave tiles nobody can claim.
- **A cap.** One stray drag across a map this size could escrow the whole
  treasury. `maxExcavationTiles` bounds an order, and the page says so before
  sending rather than the engine truncating a rectangle in some arbitrary
  order.
- **Seen rock only.** Ordering a dig through unexplored rock would reveal the
  map for the price of a wage. The page and the engine both ignore unseen tiles.
- **Skipping tiles already in a project.** Otherwise two orders for one area
  would pay twice for the same tile, and the second's tasks would be done
  before anyone held them. The page doesn't know which tiles are taken, so its
  count can be higher than what the engine buys.

## Cancelling

`CancelExcavation{ID}` names the excavation's project id, which the market
topic lists in `digs` beside how many tiles are dug and what is still held.
`cancelExcavation` releases any digger's claim on one of its tasks (so it is
not paid for a task that is gone), closes every order still open, refunding the
treasury, and drops the project. Rock not yet dug stays rock, and tiles already
dug and paid for stay so. Only a project named `excavation` can be cancelled:
rooms belong to the planner. The Dig tab lists open orders with a **Cancel**
button, which is also how to recover the money held for an area walled off from
every walkable tile.

## Known gaps

- **Idle miners ignore the wage.** Frontier mining is not chosen by pay (see
  [work-market.md](./work-market.md)), so a miner who passes a marked tile may
  clear it without holding the task. The order's leftover escrow is refunded
  when the project finishes.
- **The TUI has no dig tool.** `OrderExcavation` is an ordinary command, but
  only the browser frontend can mark an area.

## Extending it

- **A colonist ordering a dig** from its own wallet: `issuer` is already a field,
  and `fundProject` pays from whoever it names.
- **Other areas**: a `kind` on the command (clear refuse, say) would reuse the
  rectangle tool; the page's tool is already generic over what it marks.

## Related

- [labor.md](./labor.md) — work orders and public works, which this reuses.
- [market.md](./market.md) — the order book the orders show up on.
- [construction.md](./construction.md) — projects and the room planner.
- [frontend-web.md](./frontend-web.md) — the Dig tab and the map's input.
- [fog-of-war.md](./fog-of-war.md) — why only seen rock counts.

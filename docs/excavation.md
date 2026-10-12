# Excavation orders

> Part of the [mars-sim documentation](./README.md).

## What it is

The player can order an area mined out. In the browser, the **Dig** tab arms a
tool, a drag on the map marks a rectangle, and **Order** buys the digging: every
rock tile in it that the colony has seen, and every tile it has not seen at all,
becomes one `WorkDig` order on the order book, escrowed from the treasury. So an
order can clear a wide area in one go, not just the revealed rim of rock
around the colony. Whoever digs a tile is paid its wage
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

`orderExcavation` clamps the rectangle to the map, then collects
(`unmarkedRock(..., unseen=true)`) every tile that is either seen `Rock` or not
`Explored` at all, and is not a reserved door tile or already a task of another
project. An unseen tile is taken whatever it really is: usually rock, but it
can be a hidden cavern's floor (see below). Those become the tasks of one project named `excavation`, issued by
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
- **Unseen tiles, bought blind.** Orders used to take only rock the colony had
  seen, which in practice meant the one-tile rim of revealed rock around the
  colony: clearing a large area took an order per ring. They now take every
  unseen tile too. The danger was that an order through the fog maps it, so
  the rule is that nothing about an order may depend on what is under the fog:
  - An unseen tile is bought, counted and priced the same whatever it holds.
    Taking only the unseen tiles that are really rock would make the tile
    count a cavern detector. The page counts the same way (`showDig`).
  - A dig task on a tile not yet `discovered` is never `taskDone`, even when
    the tile is a hidden cavern's floor and so already "not rock". Without
    that, the order's `done` count (and the project finishing) would show
    the hidden floor before anyone found it. Once digging beside it reveals
    it, the task is done, unpaid, and its escrow is refunded with the
    project's by `pruneProjects`.
  - Fog off (`fog-of-war: false`) makes every tile `Explored`, so only rock
    counts, matching what the page shows.
  Breaking into a cavern this way is the same as frontier mining doing it:
  whatever is nesting there wakes up. The order makes that easy to do by
  accident over a big area; it is the player's call.
- **Skipping tiles already in a project.** Otherwise two orders for one area
  would pay twice for the same tile, and the second's tasks would be done
  before anyone held them. The page doesn't know which tiles are taken, so its
  count can be higher than what the engine buys. (It also counts a tile on a
  page it doesn't hold as unseen.)

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

## Zones over rock

Painting a zone over rock the colony has seen digs it out the same way
(but only seen rock: `unmarkedRock(..., unseen=false)`, since a zone painted
over fog is a reservation, not an order to tunnel through it)
(`startExcavation`, shared with `orderExcavation`): an ordinary `excavation`
project that shows on the Dig tab and cancels like any other. A zone's dig is
not capped at `maxExcavationTiles`, because the Zones tab prices the whole paint
before it is sent. `cancelExcavation` is now `cancelOrdered`, which clearing
orders share. An open excavation no longer holds up the room planner: its
concurrency cap counts rooms only. See [zoning.md](./zoning.md).

## Known gaps

- **Idle miners ignore the wage.** Frontier mining is not chosen by pay (see
  [work-market.md](./work-market.md)), so a miner who passes a marked tile may
  clear it without holding the task. The order's leftover escrow is refunded
  when the project finishes.
- **An order over a sealed cavern can sit forever.** Hidden floor tasks are
  not done until revealed, and if nothing in the order is rock the colony
  can reach, nothing reveals them. It reads exactly like an order on
  unreachable rock (no progress), which is the point; **Cancel** refunds it.
- **The cap is still `maxExcavationTiles`** (400, so 20×20). A larger area
  is several orders.
- **The TUI has no dig tool.** `OrderExcavation` is an ordinary command, but
  only the browser frontend can mark an area.

An order is on one level: `OrderExcavation.Level`, the level the browser
shows (Z5 of [z-levels.md](./z-levels.md)). The zero value means the landing
level (`orderLevel`), so a caller that never names a level digs where it
always did; a level nobody has broken into has no rock the colony has seen,
so an order there is refused. `ClearArea` names its level the same way, and
never clears a stair, shaft or hole (`joinsLevels`): clearing one end would
leave the other dangling on a level the player is not looking at.

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
- [zoning.md](./zoning.md) — zones over rock, which post excavations, and the
  clearing orders that share the cancel.

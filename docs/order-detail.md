# Order detail

> Part of the [mars-sim documentation](./README.md).

## What it is

In the browser's Market tab, any open bid or ask opens a detail view where it
is listed. The view shows who posted the order, which side it is on, the item,
the unit price, how much has filled out of how much, the total at the limit,
what it still holds in escrow, the depot, when it opened on the colony clock
and how long it has been open ("Day 12 12:43 — 3 days, 2 hrs open"), and who
it has traded with: how many units and how many dollars for each. A row in
**Books** opens in place to list that book's orders, so any order on the
market can be reached in two clicks. Built for issue #125.

## Source

- [`internal/sim/market.go`](../internal/sim/market.go) — `Order.Filled` and
  `Order.Fills`, `Fill`, `recordFill` (called from `settle` for both sides),
  `addFill`, and `inherit`.
- [`internal/sim/colonyorders.go`](../internal/sim/colonyorders.go) —
  `repriceColonyOrder` calls `inherit` so a repriced order keeps its history.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `OrderView`'s
  `Expires`, `Escrow`, `Filled` and `Fills` (a copy), and
  `Snapshot.TicksPerDay`.
- [`internal/wire/orders.go`](../internal/wire/orders.go) — the
  `book:<x>,<y>:<item>` topic (`BookTopic`) and the `order:<id>` topic
  (`OrderTopic`), plus `ownerKey`, `depotLabel` and `parsePoint`.
- [`internal/wire/boards.go`](../internal/wire/boards.go) — `OrderRow.ID`, so
  an account's open orders can open their detail.
- [`web/src/ui/OrderDetail.svelte`](../web/src/ui/OrderDetail.svelte) — the
  detail view. [`BookOrders.svelte`](../web/src/ui/BookOrders.svelte) — one
  book's orders. `MarketPanel`, `AccountDetail` and `ColonyOrders` open them.
- [`web/src/ui/format.ts`](../web/src/ui/format.ts) — `clock`, `dayClock` and
  `span`, the colony-time formats (the top bar uses `clock` too).
- Tests: `TestAnOrderRecordsItsFills` and `TestARepricedOrderKeepsItsHistory`
  in `internal/sim/market_test.go`; `TestBookTopic`, `TestOrderTopic` and
  `TestOrderTopicNames` in `internal/wire/orders_test.go`, plus a check in
  `TestBoardTopics` that every live order resolves and is in its book;
  `web/src/ui/format.test.mjs`.

## How it works

**The sim records fills.** Before this change an order kept only `Qty`, what
was still unfilled. The original size and the counterparties were gone. Now
`settle` calls `recordFill` on both orders in a trade. `Filled` counts units
traded, and `Fills` has one line per counterparty (`With`, `Qty`, `Total`
dollars), in the order each first traded. Posted size is `Qty + Filled`. The
fields are for display only: no matching, pricing or planning reads them.

**A reprice keeps the history.** Repricing is a cancel and a re-post (see
[colony-orders.md](./colony-orders.md)), so the order gets a new ID.
`inherit` copies the old order's `Posted`, `Filled` and `Fills` onto the new
one, and merges whatever the re-post filled at once after them. `Posted` is
display-only too: time priority is the ID, so carrying it over changes no
matching.

**Two topics.** `order:<id>` is one order's page. It derives everything the
page shows: posted size, open, filled, total at the limit, a bid's escrow,
the depot's label (silo, chest, a locker), and the times. `PostedDay` and
`PostedMinute` come from `sim.DayOf` and `sim.MinuteOfDay` on the snapshot's
`TicksPerDay`. `OpenMinutes` and `ExpiresMinutes` are in **colony minutes**
(1440 a day, however many ticks a day is), so the page formats them without
knowing the day length. Once the order leaves the book (filled, withdrawn,
expired, repriced) the topic sends `found: false`. `book:<x>,<y>:<item>`
lists one book's open orders in matching order (bids dearest first, asks
cheapest first, then the older), each with its ID for `order:<id>`. Both
are rebuilt at most every 500 ms, like the other market topics, and only
while a view has them open.

**Where an order opens.** The detail opens in place under the row that
lists it, as an account does in the Accounts list:

- **Books**: a row expands into its book's bids and asks, and each of those
  expands into its detail.
- **Accounts**: an account's open orders.
- **Colony orders**: the order text on each row of the desk.

## Why it is this way

- **Fills per counterparty, not per trade.** The issue asks who traded, how
  many units, and the total, which is a per-counterparty question. A list per
  trade would also grow without limit: the colony's standing iron bid is filled
  one or two units at a time by the same miners for hundreds of days. One line
  per counterparty is bounded by the colony's population.
- **Recorded on the order, not rebuilt from `w.trades`.** The world keeps only
  the last 64 trades, and a `Trade` does not say which order it filled. A
  long-lived order's early fills would be gone. Recording on the order costs a
  short slice per order, and the order is freed when it closes.
- **A new topic, not more fields on `market`.** The market topic is already
  the heaviest panel payload; a colony can have hundreds of open orders (the
  standing bids top up with a new order each time). Fills for all of them in
  every send would be wasted on orders nobody opened. The `account:<key>`
  pattern, one topic per open view, already solved this.
- **Colony minutes on the wire, not ticks.** The page knows the current day
  and clock from the frame's stats, but not the day length, so it could not
  turn a tick difference into days. Sending minutes keeps `TicksPerDay` on the
  Go side. Adding it to `Stats` would also have changed the frame's stat
  indices (see [days.md](./days.md)); it is on `Snapshot` instead.
- **Inherit on reprice.** Without it, repricing a half-filled order from the
  desk showed "0 of 5 filled, opened just now", which is wrong from the
  player's point of view: it is the same order to them.

## Extending it

- **The TUI** shows open orders on its market tab but has no way to select one.
  It can call `orderTopic` directly (it takes a snapshot and an ID) when it
  gets a cursor there.
- **Closed orders** vanish: `found: false`. Keeping a short ring of closed
  orders with their fills (like `w.trades`) would let the page show "filled
  on Day 14" instead. It would need its own cap and should stay out of the
  snapshot unless a view asks for it.
- **Work orders** (`WorkOrder`, see [labor.md](./labor.md)) have the same
  shape of question (who took the work, how much was paid). They would
  follow the same pattern: a fill record on the work order and a
  `work:<id>` topic.
- Anything new on `Order` that a view shows must be copied into `OrderView`
  as a copy (`slices.Clone`), never aliased: the snapshot is read on another
  goroutine while the live order keeps changing.

## Related

- [market.md](./market.md) — orders, books, settlement and escrow.
- [colony-orders.md](./colony-orders.md) — the colony's desk, and repricing.
- [wire-format.md](./wire-format.md) — the topic tier and the topic table.
- [frontend-web.md](./frontend-web.md) — the Market tab.
- [days.md](./days.md) — the colony day and clock the times are on.

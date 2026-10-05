# Colony orders

> Part of the [mars-sim documentation](./README.md).

## What it is

The player trading on the colony's behalf. From the browser's Market tab a
player posts a bid or an ask in the colony's name at any communal depot,
reprices one of the colony's open orders, takes one off the book, or
suspends one of the colony's standing orders so it stops coming back. They are
ordinary orders on the [order book](./market.md), escrowed from the treasury
or the colony's stock, so nothing about matching, settlement or the money
audit changes.

## Source

- [`internal/sim/colonyorders.go`](../internal/sim/colonyorders.go) — the
  `PlaceColonyOrder`, `RepriceColonyOrder`, `CancelColonyOrder`,
  `SuspendColonyOrders` and `ResumeColonyOrders` commands, their validation,
  `postStanding` (the gate every standing order goes through),
  `ItemKind.Tradable`, `TradableItems` and `ParseItemKind`.
- `World.suspended` in [`internal/sim/world.go`](../internal/sim/world.go), and
  the standing-order upkeep that posts through `postStanding`:
  `refreshColonyBids` (market.go), `refreshArmoryBids` (foundry.go),
  `refreshColonyAsks` (hauling.go), the colony's scumhouse bids and
  `offerColonyMeals` (scumhouse.go). `standingAllowed` is the
  `standing-orders-build-only` filter in the same gate.
- [`internal/sim/market.go`](../internal/sim/market.go) — `Order.manual`, and
  `retireOldSilo` skipping manual orders.
- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) —
  `withdrawColonyAsks` skipping manual orders.
- [`internal/sim/engine.go`](../internal/sim/engine.go) — `apply` routes the
  commands.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `OrderView.Posted`,
  `OrderView.Manual`, and `EconomyView.Suspended`.
- [`internal/wire/boards.go`](../internal/wire/boards.go) — the market topic's
  `colony` desk: the colony's orders, its communal depots with what it holds
  at each, the goods an order may name, and the suspended standing orders.
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go) — the
  `order-place`, `order-reprice`, `order-cancel`, `order-suspend` and
  `order-resume` page commands (host API 12).
- [`web/src/ui/ColonyOrders.svelte`](../web/src/ui/ColonyOrders.svelte) — the
  order desk, in `MarketPanel`; the actions are in
  [`web/src/game.svelte.ts`](../web/src/game.svelte.ts).
- [`internal/sim/colonyorders_test.go`](../internal/sim/colonyorders_test.go).

## How it works

**Placing.** `PlaceColonyOrder` names a side, an item, a quantity, a price and
a depot. It is refused, with a log line saying why, unless:

- the item is tradable (anything but a colonist's body);
- the quantity is 1 to 10,000 and the price at least $1;
- the depot is a communal storage container (a chest, a pantry, a workshop's
  store; not a locker);
- a bid's `Qty × Price` is in the treasury, or an ask's goods are on the
  colony's ledger line at that depot.

Then it is `post`ed with no expiry and marked `manual`. It matches at once like
any order, so a bid at or over the best ask buys straight away; the log line
says how much filled.

A bid fills a delivery at a time; the goods don't have to arrive at once.
Colonists aren't assigned to it. Any number of them can plan to fill it, and
the first to deliver is paid (see [valuation.md](./valuation.md)). What
answers it depends on the good and the depot. A bid for **cave scum** gets
scraped for only at a scumhouse (one load, `scum-max`, per trip, with or
without incubators). At the silo, nobody scrapes for it.

A **meal** bid at the silo doesn't make the colony more food just by being
there. Meals bought from colonists only move between depots, which leaves
`storedMeals` the same. Only a cook's craft plan to fill the bid makes new
meals. With `standing-orders-build-only` off, the colony also sells its own
meals at the silo, and the book doesn't stop it buying from itself, so a bid
at or over that price trades with the colony's own ask.

**Repricing** is a cancel and a re-post at the new price: the escrow comes
back, then goes out again. The order gets a new ID, so it joins the back of
the queue at its price, and it may trade at once if the new price crosses. A
bid is checked first against the treasury *plus* what the order already
holds, so a reprice that cannot be paid for leaves the order as it was rather
than cancelling it. The re-posted order is manual, even if it was one of the
colony's own. It keeps the old order's opening tick and its fills (`inherit`),
so its detail still reads as the same order (see
[order-detail.md](./order-detail.md)).

**Removing** cancels the order and returns its escrow. Any colony order can be
removed, but the colony's standing orders (its bids for ore at the silo, its
meal asks, its biomatter and armory bids) are topped up again by the next
market upkeep, ten ticks later. The desk tags those rows *auto* and says so.

**Suspending** is how to keep a standing order off the book. An *auto* row
has a **Suspend** button, which sends `SuspendColonyOrders` for its side and
item: "stop buying iron ore". That sets `World.suspended[side][item]`,
withdraws every open standing order for it (at every depot), and from then on
`postStanding` posts nothing for it, whichever upkeep asks. The player's own
orders for that good are left open, and the player can still place new ones.
The desk lists what is suspended, each with **Resume** (`ResumeColonyOrders`),
after which the next upkeep round posts the standing orders again.

**What the colony starts with.** With `standing-orders-build-only` (on by
default), the only standing orders the colony posts are its silo bids for
the goods rooms are built from (`buildGoods`: rock, iron ore, clay).
`postStanding` asks `standingAllowed` first and posts nothing else: no ore
resale asks, no water or uranium bids, no biomatter bids at the kitchens, no
rifle bid, no meal asks. They are not suspended and do not appear on the
desk; they are simply never created. The building bids can still be
suspended like any standing order.

Food is then the colonists' own business: a hungry colonist bids for a meal
and buys from whoever sells one (a chef, a neighbour), and `tryRation` still
hands a colonist at critical hunger one of the colony's meals. Whether the
colony sells its meals, or buys scum to cook more, is the player's call: a
food subsidy is a manual order on the desk (an ask for the colony's meals,
a bid for scum at a kitchen) at a chosen price and quantity.

**What upkeep does with a manual order.** It leaves it alone:

- `withdrawColonyAsks`, which pulls the colony's meal asks off the book to
  haul meals, to ration one to a starving colonist, or to re-post them when
  the meal price moves, skips manual asks.
- `retireOldSilo`, which cancels the colony's orders at a silo that moved,
  skips manual orders. The player chose that depot and can see the order.

It does still *count* them: `openQty` sums every colony order, so the upkeep
that tops a standing bid up to `silo-bid-qty` sees a manual bid for the same
good at the same depot as part of that quantity.

**The desk** (`ColonyOrders.svelte`) reads the market topic's `colony` field.
It defaults to the silo, prefills the price with the item's market value (in the field itself, not a
placeholder, and again on each change of side or item, but not when the value
merely moves under a price the player typed),
shows the book at that depot, what the bid escrows or what the colony holds
there, and warns when the order will trade at once. On the sell side it lists
only the depots where the colony holds something and, at the chosen depot, only
the items it holds there: an ask for stock the colony lacks is always refused,
so the full lists (a dozen items at "0 held") were just noise to scroll past.
A choice the filter hides falls back to the first one left. It checks what the engine
checks, to grey out the button; the engine checks again, and its log line is
the answer, since a command has no reply.

## Why it is this way

- **Ordinary orders, not a new mechanism.** Escrow on posting means a player's
  order can never fail at settlement, and the money audit, the ledgers and the
  determinism fingerprint already cover orders.
- **The `manual` flag.** Without it, upkeep fought the player. A player's meal
  ask was withdrawn the next time the colony hauled meals or repriced its own,
  and a player's order at the silo was cancelled when the silo moved. A flag on
  the order is the one fact upkeep needs; tracking player orders in a side
  table would have meant keeping it in step with every fill and cancel.
- **Counting manual orders toward top-ups.** The alternative, upkeep ignoring
  them, made repricing a standing bid pointless: reprice the 64 iron bids from
  $3 to $5 and upkeep posted a fresh 64 at $3 beside them. Counted, a repriced
  lot replaces the standing one until it fills.
- **Repricing as cancel and re-post.** Changing the price in place would have
  to re-sort the book, true up the escrow and re-run matching against the
  other side, which is exactly what `cancel` then `post` already do. Losing
  time priority is the standard exchange rule for a price change.
- **Removing a standing order does not stop it; suspending does.** Stopping
  the colony's standing bids is a policy (stop buying ore), not an order, so
  it is its own command rather than a sticky cancel. A cancel that also
  suppressed re-posting would have needed to remember which order it was, and
  the standing orders are re-posted under new IDs, split across fills.
- **Suspended by side and item, colony-wide, not per depot.** The silo moves,
  and the colony's bids for scum stand at every scumhouse. Keyed by depot, a
  suspended iron bid would come back the moment the silo moved, and "stop
  buying cave scum" would take one click per kitchen. What a player means is
  the good.
- **Only the building bids by default.** Measured over 20,000 ticks on six
  seeds of the default game, the colony's ore resale asks filled 0 to 4 units
  while 100 to 220 sat on the book, and the carcass and viscera bids barely
  filled; together they buried the book. The ore bids for rock, iron and clay
  are what rooms are built from, so they stay. Survival without the rest was
  about the same (24 colonists alive across eight seeds against 26), with
  the colony's open orders down from roughly 45–105 to 0–13.
- **Not created, rather than created suspended.** A first cut started them
  suspended, which put thirteen rows on the desk's Suspended list before the
  player had done anything. Filtered in `postStanding`, they never exist. The
  upkeep that posts them is kept behind the setting rather than deleted, so
  it stays tested (`testConfig` turns the setting off) and a scenario can
  turn it back on.
- **One gate, `postStanding`.** The standing orders are posted by five
  different upkeep functions. Checking the suspension in each would leave the
  next one added to forget it; routing them all through one call makes a new
  standing order suspendable for free.
- **An array, not a map.** `suspended` is `[2][numItemKinds]bool`, so listing
  it never ranges over a map (see [determinism.md](./determinism.md)).

## Extending it

- **The TUI**: the commands are frontend-neutral; a market-tab form would send
  the same three.
- **A new standing order** must post through `postStanding`, not `post`, or
  Suspend will not hold it, and `standing-orders-build-only` will not keep it
  off the book.
- **Saving a game** would need `World.suspended` saved with the orders.
- **An expiry**: `PlaceColonyOrder` posts with ttl 0; a `TTL` field would pass
  straight through to `post`.
- Keep `manual` checks in any new upkeep that withdraws or cancels colony
  orders, or it will undo the player's.

## Related

- [market.md](./market.md) — the order book, escrow, the silo and the colony's
  standing orders.
- [hauling.md](./hauling.md) — the colony's asks and meal hauling.
- [excavation.md](./excavation.md) — the other player order paid from the
  treasury.
- [frontend-web.md](./frontend-web.md) — the Market tab.

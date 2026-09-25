# The market

> Part of the [mars-sim documentation](./README.md).

## What it is

The order book: colonists and the colony trade goods by posting limit orders at
a depot — a **bid** offers money for goods, an **ask** offers goods for money —
and orders that cross trade at once. The colony keeps standing bids for ore at
its **silo**, so digging ore pays; colonists take surplus meals there to sell,
and a hungry colonist with nothing to eat buys one before it settles for gruel.
This is phase **E4** of the [economy plan](./economy.md). Prices are still the
colony charter's reference prices until they trade; after that, trades move
them (see [valuation.md](./valuation.md)).

## Source

- [`internal/sim/market.go`](../internal/sim/market.go) — `Order`, `book`,
  `post`, `settle`, `cancel`, `expireOrders`, `cancelOrdersOf`; the silo
  (`marketDepot`), reference prices (`refPrice`), the colony's standing bids
  (`refreshColonyBids`), `sellAtMarket`, `tryBuyMeal`, and taking meals to
  market (`JobSell`).
- [`internal/sim/owner.go`](../internal/sim/owner.go) — `ownerOrder`, the
  owner an order's escrow is held under.
- [`internal/sim/property.go`](../internal/sim/property.go) —
  `StorageContainer.moveLine`, the ledger-only move a sale is.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `tryAssignStore`
  unloads at the silo, `jobStore` sells there; `runMarket` in `step`.
- [`internal/sim/food.go`](../internal/sim/food.go) — buying a meal in
  `runFoodFocus`.
- [`internal/sim/project.go`](../internal/sim/project.go) — the planner builds
  a silo if the colony has none.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `EconomyView`'s
  `Orders`, `Books`, `Trades`, `Escrowed`, `Silo`.
- [`internal/ui/tui/render_market.go`](../internal/ui/tui/render_market.go) —
  books, trades, and open orders on the market tab.
- [`internal/sim/market_test.go`](../internal/sim/market_test.go).

## How it works

### Orders and books

An `Order` is a side, an item, a quantity, a limit price, an actor (a colonist
or the colony), a depot, and an optional expiry tick. Its ID comes from a
counter, so a lower ID is an older order. Each `(item, depot)` has a `book`
whose two sides stay sorted best-first — bids by price down, asks by price up,
then oldest first.

`post` places an order and **matches it at once**: while it crosses the head of
the opposite side, they trade the smaller quantity **at the resting order's
price**. Whatever is left rests; `ttl` (or never) decides when it expires.
Trades are recorded (`w.trades`, the last 64) and set the book's last price and
volume.

### Escrow

Posting escrows what the order promises, so a trade can never fail at
settlement:

- a **bid** moves `Qty × Price` from its actor's account into the order's own
  account (`ownerOrder`, see [money.md](./money.md));
- an **ask** moves its goods off the seller's ledger line onto the order's own
  line at the depot (`moveLine`, see [property.md](./property.md)).

Settling is a ledger move from the ask's line to the buyer and a transfer from
the bid's escrow to the seller. A bid that fills below its limit gets the
difference back at once, so a bid's escrow is always exactly `Qty × Price`.
Cancelling or expiring returns whatever is left. Nothing moves physically: the
goods were already in the depot, and taking them out is an ordinary withdrawal.

Because escrow is just an owner, the audits keep working with orders open:
money is `circulating + frozen + escrowed == issued` (escrowed counts work
orders too, see [labor.md](./labor.md)), and every ledger still
sums to its container's contents.

Death cancels a colonist's orders (`cancelOrdersOf`, in `remove`) before its
wallet freezes, so a bid's escrow goes back to the wallet and freezes with it.

### The silo

`marketDepot` is where the colony trades: its **communal** chest nearest the map
centre (a crash-pod locker is private, so it never counts). It is cached on
`fixtureRev`, because a colony with a locker per settler has a container per
settler. Crash pods mean nothing else ever calls for a shared chest, so the
planner builds one when there is no silo (after life support, before bunks).

Every `marketInterval` ticks, `runMarket` expires stale orders and has the
colony top up a standing bid of `silo-bid-qty` units for each ore it buys, at
the reference price, as far as the treasury stretches.

### Who trades what

- **Miners sell ore.** A colonist whose load stops it mining takes what sells
  to the silo (`tryAssignStore`, `sellableStacks`) and offers it there at
  reference prices (`sellAtMarket`). The colony's standing bid takes it at
  once: **prospecting pays**. Raw rock is not bought by default
  (`price-raw-rock` 0) — there is always more, and buying it would drain the
  treasury on nothing — so it goes to an ordinary chest instead, and the silo
  holds only what the market trades. (Before that rule, a colony's silo filled
  with 1500 units of unsold rock.)
- **Colonists sell surplus meals.** One holding more than `meal-keep` of its
  own meals outside the silo takes the rest there (`JobSell`: out of its
  locker, into its pockets, onto the silo's ledger in its own name) and asks
  the reference price.
- **Hungry colonists buy.** One with no meal of its own or the colony's in
  reach buys the cheapest meal at the silo at up to its `mealBidLimit` (hunger
  times the meal's value, capped by its money), before it eats gruel. If
  nothing fills, the bid rests for `demand-ttl` ticks as demand the producer
  planner can answer (see [valuation.md](./valuation.md)).

| Setting | Default |
| --- | --- |
| `price-meal` | 5 |
| `price-raw-rock` | 0 (not bought) |
| `price-iron-ore` / `price-water-ice` / `price-uranium-ore` / `price-clay` | 3 / 2 / 6 / 2 |
| `silo-bid-qty` | 64 |
| `order-ttl` | 2000 |
| `meal-keep` | 5 |
| `meal-willingness` | 3 |

In a default colony over 6000 ticks, miners sold the silo a few hundred units of
ore, wallets grew from 100 to between roughly 160 and 280, and the treasury
spent about 1500 of its 5000.

## Why it is this way

- **Escrow on posting.** It removes a whole class of settlement failures (the
  buyer went broke, the goods are gone) at the cost of locking money and goods
  in open orders. That cost is deliberate: it discourages spamming the book.
- **The order as an owner.** Holding escrow under `ownerOrder` rather than in
  side tables means every existing audit — the money identity, the ledger
  balance, the determinism fingerprint's ledger lines — covers orders with no
  special cases.
- **Trade at the resting price.** The standard exchange rule; it rewards the
  order that was there first, and makes an incoming bid's fill price
  predictable from the book.
- **No map order anywhere.** Books are sorted slices; expiry and death walk
  orders sorted by ID. `TestRandomTradingIsDeterministicAndConserved` replays
  400 random orders twice and requires identical trades.
- **One silo.** Matching only happens within a depot, so a single market depot
  keeps the thin early market from splitting into books nobody else is at.
  Arbitrage between depots closes the gaps (see [hauling.md](./hauling.md)).

## Extending it

- **Prices that move** (E6, see [valuation.md](./valuation.md)): `valueOf`
  is an item's smoothed trade price, and `tryBuyMeal` bids by hunger. The
  colony's standing bids and a miner's asks are still at reference prices.
- **A new tradable good**: give it a reference price (a `price-*` setting and a
  case in `refPrice`); add it to `prospectingGoods` if the colony should buy it.
- **The colony selling** (E7, see [hauling.md](./hauling.md)):
  `refreshColonyAsks` offers what it bought beyond a reserve, at a markup.

## Related

- [economy.md](./economy.md) — the plan this is phase E4 of.
- [money.md](./money.md) — accounts, `transfer`, and the audit escrow joins.
- [property.md](./property.md) — ledgers, and `moveLine`.
- [food.md](./food.md) — where buying a meal sits in eating.
- [storage.md](./storage.md) — where miners unload.
- [determinism.md](./determinism.md) — why nothing ranges over a map.

# Price discovery

> Part of the [mars-sim documentation](./README.md).

## What it is

How prices in the order book move toward what goods are worth. Sellers lower
food asks that don't sell, and offer idle food they own. A hungry colonist's
waiting bid starts low and rises until it fills. Cooks price meals, and decide
whether cooking pays, at what meals actually fetch. Before this nobody moved a
price, so the meal price climbed to about half a wallet and stayed there, and
scum's price followed it however much scum lay idle.

## Source

- [`internal/sim/pricing.go`](../internal/sim/pricing.go) — `movePrices` (in
  `runMarket`): `decayAsks`, `relistIdleFood`, `raiseMealBids`; `askFloor` and
  `mealInputCost`, the price floor; `mealBidStart`,
  `repost` (an order moved to a new price), `withdrawOwnAsks`, `isFood`.
- [`internal/sim/valuation.go`](../internal/sim/valuation.go) —
  `mealBidLimitWith` and `foodCritical`: what a hungry colonist will pay.
- [`internal/sim/market.go`](../internal/sim/market.go) — `Order.priced` and
  `Order.hunger`; `tryBuyMeal` posts the waiting bid at `mealBidStart`.
- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) —
  `mealSellPrice` (and so `foodPays`) at a meal's value; `tryAssignFoodWork`
  takes a hungry colonist's own scum off sale before it cooks.
- [`internal/sim/pricing_test.go`](../internal/sim/pricing_test.go), and
  `TestAHungryColonistsLimitIsItsOwn` in
  [`valuation_test.go`](../internal/sim/valuation_test.go).

## How it works

All three run in the market's upkeep (`movePrices`, every `marketInterval`
ticks), before the colony's standing orders are topped up.

**Sellers.** A colonist's ask for food (meals, scum, other biomatter) that has
waited `ask-decay-ticks` at its price comes down `ask-decay-percent`, at least
a dollar, to no less than its floor (`decayAsks`, `askFloor`). A meal's floor
is what its scum costs: the scum recipe's inputs at their value, per meal
(`mealInputCost`; two scum at $3 make a $6 floor). Anything else's is $1. `Order.priced` is when it was last
priced. A reprice is a cancel and a re-post (`repost`), so it can trade at once
and loses its place in the queue. Ore asks don't move, and nor do the colony's
asks: the player or the charter priced those.

Every `relistInterval` (100) ticks, `relistIdleFood` puts idle food colonists
own at the kitchens and the silo (`mealDepots`) on sale at its value, or its
floor if that's higher:

- meals beyond what the owner keeps (`surplusMeals` plus what it holds at the
  silo: `meal-keep` and its pocket meal);
- scum and other biomatter beyond one recipe's worth (`biomatterKeep`, 2) at
  each stove.

These asks never expire; decay brings them down until they sell. A colonist
with a plan under way, or cooking, scraping, eating, selling, carrying or
tending, lists nothing that round, since its stock may be that work's input
or its supper. A hungry colonist about to cook its own scum takes it off sale
first (`withdrawOwnAsks` in `tryAssignFoodWork`).

**Buyers.** What a hungry colonist will pay (`mealBidLimit`) is its own: the
old value-based ceiling (a meal's value, rising with hunger to
`meal-willingness` times it), or up to half its money as hunger grows if that
is more, and everything it has at critical hunger. As before, it pays at most
half its money until critical.

It still buys at once from the cheapest ask within the limit. If nothing
fills, its waiting bid (`Order.hunger`) starts at `bid-start-percent` of a
meal's value, or the limit if that's lower (`mealBidStart`). Every
`bid-raise-ticks` that it goes unfilled, it rises `bid-raise-percent` of the
limit, at least a dollar, up to the limit (`raiseMealBids`). At critical
hunger the bid goes in, or jumps, straight to the limit. A plan serving a bid
follows it when it is re-posted (`repost` re-targets `plan.target`), or
`prunePlans` would drop a cook waiting on scum.

**Cooks.** With `meal-sell-at-market`, `mealSellPrice` is a meal's value
(`valueOf`), unless the colony's scarcity price (`meal-price-max`) is higher,
and never below what its scum costs (`mealInputCost`).
Cooks ask that for their meals (`offerOwnMeals`, `sellAtMarket`), and `foodPays`
uses it to decide whether cooking for oneself pays.

| Setting | Default |
| --- | --- |
| `ask-decay-ticks` | 150 (0: asks never move) |
| `ask-decay-percent` | 10 |
| `relist-idle` | true |
| `bid-start-percent` | 80 |
| `bid-raise-ticks` | 50 (0: bid the limit at once, the old way) |
| `bid-raise-percent` | 20 |
| `meal-sell-at-market` | true |

### What it does

100 colonists on a 300×150 map, seeds 1–4, to tick 30,000, colonists starved
(a: asks come down and idle food is offered, with the floor; b: bids start
low and rise; c: cooks sell at market). With the sleep drive as shipped:

| | Starved (seeds 1, 2, 3, 4) | Meal price at the end | Scum price at the end |
| --- | --- | --- | --- |
| None (before) | 41, 37, 26, 39 (143) | $87–108 | $38–56 |
| c | 33, 29, 30, 33 (125) | $88–114 | $36–50 |
| b + c | 30, 32, 24, 37 (123) | $97–152 | $34–51 |
| a + b + c (the default) | 39, 44, 31, 36 (150) | $8, $104, $120, $9 | $1–23 |

**Starvation there isn't the market's.** In every run, before and after, the
colonists who starved had **passed out** first: 289 of 289 deaths in the
first round of runs. A colonist too hungry to stop for sleep collapses at the
sleep ceiling, and hunger keeps climbing while it is unconscious. Many die
carrying a meal, or owning one in a reachable pantry, or with $100. That is a
drives problem (see [drives.md](./drives.md)), and it swamps whatever the
market does to survival.

With sleep switched off (the sleep drive's growth set to 0 in every activity
where it grows), nobody passes out, and the market's effect shows:

| | Starved (seeds 1, 2, 3, 4) | Meal price at the end | Scum price at the end |
| --- | --- | --- | --- |
| None (before) | 1, 0, 13, 3 (17) | $123–138 | $52–71 |
| a, floored at $1 (the first version) | 1, 0, 18, 39 (58) | $1–46 | $2–8 |
| a, floored at the scum's cost | 1, 0, 3, 3 (7) | $8, $13, $112, $28 | $2–10 |
| b | 0, 0, 2, 3 (5) | $106–269 | $57–97 |
| c | 1, 0, 9, 2 (12) | $86–130 | $34–54 |
| b + c | 0, 0, 7, 0 (7) | $195–278 | $47–83 |
| a + b + c, floored at the scum's cost (the default) | 3, 0, 6, 8 (17) | $5, $5, $218, $5 | $1–8 |

Prices now move both ways. Where meals stay dear, scum doesn't follow them up
any more: the old price tied scum to the meal's (a cook's bid for scum offers
its whole margin), so scum stayed at a third of a meal however much lay idle.
Rising bids alone (b) keep the most colonists alive but drive meals to $200
and more, and leave thousands of units of scum idle. Falling asks with the
floor (a) are about as good for survival with meals at $8–28 on three seeds
of four. Most deaths in every variant happen before tick 10,000, as the
colony goes from its landing meals to cooked ones.

## Why it is this way

- **Willingness is the colonist's, not the last price's.** The first version
  kept `mealBidLimit` as a multiple of a meal's value. At landing colonists
  sell their spare locker meals, nobody is hungry yet, and the asks decayed
  to $1. The first fills set a meal's value to $1, which capped every bid at
  $3 for good. No cook could profit, and on every seed meals stopped (20 to
  47 traded in 30,000 ticks), with 48 and 52 starved. A ceiling tied to the
  last price can only ratchet one way. Before, that was up; with decay, it
  was down.
- **A meal's floor is its scum's cost.** Floored at $1, asks fell to $1 in
  the glut after landing, and the first $1 fills set a meal's value. Cooks
  asked that, cooking stopped paying just as the colony needed it to start,
  and with sleep off, falling asks alone starved 58 colonists against 17
  with none of this. Floored at the scum's cost, 7.
- **Raise by a share of the limit, not of the value.** A waiting bid lives
  `demand-ttl` (300) ticks and then starts over. Steps of 10% of a $1 value
  never got anywhere in that time.
- **Bids start low, but not at critical.** A bid that has to climb wastes the
  last ticks a starving colonist has. `TestHungryBidRestsAsDemand` caught it:
  the colony's $5 meal no longer filled a starving colonist's $4 opening bid.
- **Food only.** Ore's price is the colony's standing bids at charter prices.
  A miner's ore decaying to $1 would hand the next colony bid a windfall at
  the miner's expense, since a fill is at the resting price.
- **Keep a recipe's worth.** Listing every unit sold the supper of a colonist
  that scraped to feed itself. One recipe (2 scum) is its next meal.
- **Asks that never expire.** An expiring ask re-listed at full value would
  saw-tooth the price back up every `order-ttl`.

## Extending it

- **Pass-outs first.** Survival can't measure a market change until hunger
  stops killing the unconscious.
- **A floor per seller.** The meal floor is the scum's market value, the
  same for everyone. A seller could instead hold at what it paid, or add its
  labor; that would be a floor kept on the order.
- **Shrewder derived bids.** A cook's bid for scum still offers its whole
  margin (see [valuation.md](./valuation.md)). With sellers competing, it
  could start low and rise the same way.
- **New tunables** go on `Config` with the rest of the Market section's.

## Related

- [market.md](./market.md) — the book, matching at the resting price.
- [valuation.md](./valuation.md) — remembered prices and the planner.
- [scumhouse.md](./scumhouse.md) — `foodPays` and cooking for oneself.
- [colony-orders.md](./colony-orders.md) — the colony's standing orders,
  which keep their prices.
- [drives.md](./drives.md) — passing out.

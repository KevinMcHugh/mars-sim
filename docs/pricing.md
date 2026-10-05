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
  `runMarket`): `decayAsks`, `relistIdleFood`, `raiseMealBids`; `mealBidStart`,
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
a dollar, to no less than $1 (`decayAsks`). `Order.priced` is when it was last
priced. A reprice is a cancel and a re-post (`repost`), so it can trade at once
and loses its place in the queue. Ore asks don't move, and nor do the colony's
asks: the player or the charter priced those.

Every `relistInterval` (100) ticks, `relistIdleFood` puts idle food colonists
own at the kitchens and the silo (`mealDepots`) on sale at its value:

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
(`valueOf`), unless the colony's scarcity price (`meal-price-max`) is higher.
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

100 colonists on a 300×150 map, seeds 1–4, to tick 30,000:

| | Meal price at tick 30,000 | Scum price at tick 30,000 | Meals traded | Starved |
| --- | --- | --- | --- | --- |
| Before (all off) | $87–108 | $38–56 | 658–797 | 41, 37, 26, 39 (143) |
| Cooks at market only | $91–108 | $35–50 | 558–676 | 33, 32, 17, 33 (115) |
| All on (the default) | $6, $119, $154, $44 | $2, $10, $19, $8 | 547–958 | 30, 47, 37, 32 (146) |

Prices now move both ways. On seed 1 a meal went from $107 at tick 7,500 to
$6, and scum from $13 to $2; on seed 4, from $187 to $44. Where meals stay
dear (seeds 2 and 3), scum doesn't follow them up any more: $10–19 against
$119–154. The old price tied scum to the meal's (a cook's bid for scum offers
its whole margin), so scum stayed at a third of a meal however much lay idle.
Now the book says the shortage is cooking, not scum.

**Starvation didn't fall, and the market isn't why.** In every one of those
runs, before and after, all the colonists who starved had **passed out**
first (289 of 289 deaths). A colonist too hungry to stop for sleep collapses
at the sleep ceiling, and hunger keeps climbing while it is unconscious. Many
die carrying a meal, or owning one in a reachable pantry, or with $100. That
is a drives problem (see [drives.md](./drives.md)), not a pricing one, and
it hides any effect the market has on survival. Cooks at market price on its
own did better on all four seeds (115 against 143). The full set did worse
than that on seeds 2 and 3. Treat the survival column as noise until
pass-outs are fixed.

## Why it is this way

- **Willingness is the colonist's, not the last price's.** The first version
  kept `mealBidLimit` as a multiple of a meal's value. At landing colonists
  sell their spare locker meals, nobody is hungry yet, and the asks decayed
  to $1. The first fills set a meal's value to $1, which capped every bid at
  $3 for good. No cook could profit, and on every seed meals stopped (20 to
  47 traded in 30,000 ticks), with 48 and 52 starved. A ceiling tied to the
  last price can only ratchet one way. Before, that was up; with decay, it
  was down.
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
- **Decay toward cost, not $1.** A seller could hold at what it paid, or at
  its labor, rather than give food away. That is a floor per seller, kept on
  the order.
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

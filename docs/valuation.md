# Valuation and the producer planner

> Part of the [mars-sim documentation](./README.md).

## What it is

How colonists put prices on things, and act on them. Every good has a
remembered price that trades move. A hungry colonist bids for a meal by how
hungry it is. A colonist looking for work checks the book for a bid it could
fill at a profit, and if it lacks the inputs, it bids for them itself. That
derived bid is what the next producer sees, so a demand for a meal becomes a
demand for scum, and someone goes and scrapes the cave wall. Nobody plans the
whole chain. This is phase **E6** of the [economy plan](./economy.md).

## Source

- [`internal/sim/valuation.go`](../internal/sim/valuation.go) — `priceMemory`,
  `recordPrice` (called from `settle`), `referenceValue`, `valueOf`,
  `laborCost`, `mealBidLimit`.
- [`internal/sim/producer.go`](../internal/sim/producer.go) — `plan`,
  `candidateBids`, `tryAssignProduce`, `planGather`, `planCraft`,
  `advancePlan`, `prunePlans`, `chainDepth`, and `JobCarry` (`jobCarry`).
- [`internal/sim/market.go`](../internal/sim/market.go) — `tryBuyMeal`, whose
  unfilled bid now rests as demand; `Order.plan` and `Order.depth`.
- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) — scraping on
  one's own account (`scrapeFor`, `scrapeQty`) and selling the load
  (`sellGathered`); `jobCraft` marks a plan's output made.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `assignWorkJob`
  asks the planner after food, cleaning and selling surplus meals, before
  mining.
- [`internal/sim/trace.go`](../internal/sim/trace.go) — `TraceEconomy`, the
  tuning harness; [`main.go`](../main.go) — `-econ-trace`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) —
  `EconomyView.Prices`, `Plans`, `ChainDepth`, `Starved`;
  [`render_market.go`](../internal/ui/tui/render_market.go) shows them.
- [`internal/sim/valuation_test.go`](../internal/sim/valuation_test.go).

## How it works

### Prices

`valueOf(item)` is an item's **smoothed trade price** once it has traded, and its
reference value before that: the charter's `price-*` setting, or
`price-cave-scum` for scum, which the colony doesn't buy. Every fill moves the
remembered price one eighth of the way toward the fill (`priceSmoothing`),
stored in thousandths of a dollar so small moves aren't rounded away. The first
fill sets it outright.

### A hungry colonist's bid

`mealBidLimit` starts at a meal's value and rises linearly with hunger, to
`meal-willingness` times the value when starving. Until hunger is critical
(90%), a colonist spends at most half its money on one meal, then everything it
has. A dollar matters more to someone who has few.

`tryBuyMeal` (see [food.md](./food.md) for when it runs) buys at once if an ask
is within the limit. If nothing fills, the bid **rests** for `demand-ttl`
ticks, one per colonist, as standing demand. A later fill leaves the meal at
the silo in its name, and the ordinary eating job fetches it.

### The planner

When a colonist looks for work, `assignWorkJob` asks `tryAssignProduce` just
before mining. The planner reads `candidateBids`, every open bid for a
**producible** good (scum, or any recipe's output), best price first. The list
is memoized per tick. The planner considers the first `plan-candidates` it can
reach and use, skipping its own bids and bids that other plans already cover
in full. For each one it reckons, one recipe level deep:

```
profit = bid price × units − input cost − labor ticks × labor-price / 100
```

- **Gather** (a bid for scum at a scumhouse): scrape the nearest patch on its
  own account (`scrapeFor`), deliver to that scumhouse in its own name, and
  ask the bid's price, which fills at once. Labor is the scraping plus both
  walks.
- **Craft** (a bid for a recipe's output): cook at the nearest usable
  workshop, then carry the output to the bid's depot (`JobCarry`) and ask the
  bid's price. Each input is costed as follows:
  - An input the colonist already owns at the workshop costs its value.
  - One on offer there costs its ask, and the planner buys it now.
  - One nobody offers is **missing**.

  The margin left over, less `plan-min-profit`, divided by the missing units,
  is the most the colonist can pay per missing unit. If that's at least a
  dollar and the colonist can fund it, it posts a **derived bid** for them at
  the workshop, at that price.

A plan with derived bids leaves the colonist free for other work until the
inputs arrive. Later calls pick it up (`advancePlan`): cook once the inputs are
in, then carry the output.

### Plans and their bids

A plan is tied to the bid it serves (`plan.target`), and its derived bids to it
(`Order.plan`). `prunePlans` drops a plan, cancelling its derived bids, when:
- its colonist is gone;
- `plan-ttl` has passed;
- or it's still waiting on inputs and its target bid is gone.

A plan also ends when it delivers. `Order.depth` counts links below a
finished-good bid: a colonist's own bid is 0, a plan serving it is depth 1, the
bid that plan derives is depth 1, and a plan filling that is depth 2.
`chainDepth` is the deepest open plan.

`TestAMealBidReachesTheCaveWall` is the phase gate. A customer bids $15 for a
meal nobody has. A cook bids for two units of scum at the scumhouse, a scraper
fills that bid off the wall, and the cook cooks and sells the customer its
meal. Nothing is scripted but the first bid.

### The tuning harness

`mars-sim -econ-trace TICKS [-econ-every N] [-econ-seeds 1,2,3]` runs each
seed synchronously for TICKS ticks, without a clock or a UI, and prints CSV.
Each row has:
- colonists, starved, treasury, escrow, and the colonists' total wallets;
- open plans and chain depth;
- each traded good's value and volume.

Since nothing depends on the wall clock, a trace is a pure function of its
settings, and two runs compare row for row. Every other setting flag applies,
e.g. `-infinite-food=false`.

| Setting | Default |
| --- | --- |
| `labor-price` | 2 dollars per 100 ticks |
| `plan-min-profit` | 1 |
| `plan-candidates` | 4 |
| `plan-ttl` | 1500 |
| `demand-ttl` | 300 |
| `price-cave-scum` | 1 |

## Why it is this way

- **One level deep, with derived bids.** A planner that searched the whole
  recipe graph would cost a search per colonist per decision, which is what the
  focus design forbids ([cascading_wsts_architecture.md](./cascading_wsts_architecture.md)).
  Pushing the next level out as an ordinary bid gets chains for free, and
  every link is visible in the book.
- **Derived bids die with their plan.** Otherwise the book fills with demand
  for meals nobody wants any more. `TestDerivedBidsDieWithTheirPlan` pins it.
- **A derived bid offers the planner's whole margin.** Bidding lower would be
  shrewder. But with a handful of colonists there is one seller at best, and
  a derived bid that a scraper finds not worth the walk breaks the chain.
  Paying up is what keeps the thin early market moving. It can get shrewder
  once there's competition.
- **The planner runs just before mining, after community work.** Building,
  cooking and cleaning for the colony still come first: they are what keep
  everyone alive, and they don't depend on anyone's wallet. Mining pays only
  at the colony's fixed prospecting bids, so a better-paying bid should beat
  it.
- **Hungry bids rest.** In E4 an unfilled hungry bid was cancelled at once, so
  the book never showed any demand and no producer could see it. A resting bid
  locks up at most one meal's worth of money per colonist.
- **Prices in thousandths.** Money is whole dollars
  ([money.md](./money.md)), but smoothing by eighths in whole dollars rounds
  every move under $8 to nothing.
- **What the trace shows today.** With the safety net on (the default), food
  hardly trades: gruel is free. Even with `-infinite-food=false`, the colony's
  own scumhouse feeds everyone on the reference seeds, and there's little
  private food demand. The planner does its work when bids exist; the colony
  selling its stock and arbitrage (E7) and scarcity on (E8) are what create
  more of them.

## Extending it

- **A new producible good**: a recipe row outputting it. Both `producible` and
  `planCraft` read the recipe table. If it's gathered rather than made, it
  needs its own gather plan, like `planGather`.
- **Shrewder bidding**: change the derived bid's price in `planCraft`. Keep it
  at or below the margin, or the planner loses money by design.
- **Wanting more than food**: a willingness-to-pay rule like `mealBidLimit`
  for the need, and a caller that posts the bid.
- **Skills** (see [economy.md](./economy.md)): a per-colonist `laborCost`,
  cheaper for the practiced. Plans then fall to whoever is best at the work.

## Related

- [economy.md](./economy.md) — the plan this is phase E6 of.
- [market.md](./market.md) — the order book the planner reads and posts to.
- [scumhouse.md](./scumhouse.md) — recipes, scraping, and cooking.
- [food.md](./food.md) — when a hungry colonist bids.
- [labor.md](./labor.md) — work orders, which are paid work the planner does not yet weigh.

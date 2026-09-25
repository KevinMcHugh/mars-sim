# Hauling and arbitrage

> Part of the [mars-sim documentation](./README.md).

## What it is

Moving goods between depots, and the colony selling back what it bought.
Orders only match within one depot, so a cheap ask at one and a dear bid at
another never cross by themselves. A colonist closes that gap by buying,
carrying and selling (**arbitrage**, at its own risk), or by being paid to
carry someone else's goods (**hauling for hire**). The colony uses haulers to
keep meals at its silo. It offers its bought stock back at a markup and builds
public works from that stock, so the founding grant works as capital rather
than running down. This is phase **E7** of the [economy plan](./economy.md).

## Source

- [`internal/sim/hauling.go`](../internal/sim/hauling.go):
  - arbitrage: `cheapestAskElsewhere`, `planArbitrage`;
  - hauling for hire: `tryAssignHaul`, `startHaul`, `finishHaul`;
  - the colony's silo stock: `refreshSiloStock`;
  - the colony selling: `colonyAskPrice`, `refreshColonyAsks`.
- [`internal/sim/producer.go`](../internal/sim/producer.go) — `planHaul`
  among the producer planner's plans, and `jobCarry`, which both kinds of
  hauling run.
- [`internal/sim/workorder.go`](../internal/sim/workorder.go) — `WorkHaul`,
  with `WorkOrder.From` and `Item`.
- [`internal/sim/construction.go`](../internal/sim/construction.go) —
  `materialPayers`, `taskIssuer`: whoever pays for the work pays for its
  materials first.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `assignWorkJob`
  tries hauling for hire, then the planner; `jobStore` credits carried goods
  to their owner.
- [`internal/ui/tui/render_market.go`](../internal/ui/tui/render_market.go) —
  each good's quotes at every depot.
- [`internal/sim/hauling_test.go`](../internal/sim/hauling_test.go).

## How it works

### Arbitrage

The producer planner ([valuation.md](./valuation.md)) now considers every open
bid, not just those for goods it can make. For each one it checks
`cheapestAskElsewhere`: the best ask for the item at another depot the colonist
can reach and use, from anyone but itself or the bidder, below the bid. Moving
the goods is one more recipe:

```
profit = units × (bid − ask) − (walk to the ask + walk to the bid) × labor-price / 100
```

If that clears `plan-min-profit` and the colonist can pay, it buys at once
(the bid never rests), and a `planHaul` plan carries the goods (`JobCarry`) to
the bid's depot and asks the bid's price there. In transit, the goods are the
hauler's. If the bid is gone when it arrives, its ask rests like any seller's.
A bid can't be reserved in advance: that would let one colonist lock up a
market by claiming to be on its way.

### Hauling for hire

A `WorkHaul` work order pays `Pay` per unit to move `Units` of the issuer's
`Item` from `From` to `Pos` ([labor.md](./labor.md)).

- `tryAssignHaul` takes the oldest open order a colonist can do:
  - the goods are at the source;
  - both depots are reachable and usable;
  - nobody else has claimed it (`haulClaims`, released by `clearJob`).
- A colonist carrying goods for an order delivers them first.
- The goods stay the issuer's throughout. The fetch debits the issuer's
  ledger line, the cargo record marks them in the hauler's pockets, and the
  delivery credits the issuer.
- `finishHaul` pays for each unit delivered.

**The colony keeps meals at its silo.** `refreshSiloStock`, in the market's
upkeep, posts haul orders at `haul-pay` a unit. They bring the colony's meals
in from its scumhouses, nearest first, until `silo-meal-stock` are there or on
their way. The colony sells its meals at both ends (see
[scumhouse.md](./scumhouse.md)), so the haul first withdraws the scumhouse's
asks to free the meals it needs from escrow.

### The colony sells

`refreshColonyAsks` offers every good the colony bids for, at the silo:
- the quantity is whatever it holds beyond `colony-stock-reserve` units;
- the price is `colonyAskPrice`, the reference price plus `colony-markup`
  percent, and always at least a dollar over it.

The colony never sells for what it pays. Its standing bid at the reference
price and its ask above it never cross.

### Public works use the colony's stock

With `construction-costs` on, a build's materials come first from **whoever
pays for the work** (`materialPayers`): the colony for a public work, the
commissioner for a commission, then the builder. The reserve kept back from
sale is what public works draw on. Materials fetched from someone else's line
carry a cargo record until they're built with, so a job dropped halfway can't
turn the colony's iron into the builder's. `jobStore` credits whatever it
unloads to the cargo record's owner.

### Seeing it

The treasury's page on the market tab lists each good's value followed by its
quotes at every depot with open orders: `iron ore $3 (ref) · (3,2) —/$4 ·
(9,7) $12/—` reads "no bid and a $4 ask at the silo; a $12 bid and no ask over
there".

| Setting | Default |
| --- | --- |
| `colony-sells` | true |
| `colony-markup` | 50 (%) |
| `colony-stock-reserve` | 8 |
| `silo-meal-stock` | 6 |
| `haul-pay` | 1 |

### The gate

- `TestArbitrageClosesAPriceGap`: the colony holds iron at the silo and asks
  $4; a colonist bids $12 for 8 at another chest. Another colonist buys the
  colony's iron, carries it across, and fills the bid, and no bid that high is
  left open.
- `TestColonySellingPaysOverALongRun`: the same world over 4000 ticks. The
  colony's treasury ends at $4094 selling and $4062 only buying (the 8 iron at
  $4 each).

## Why it is this way

- **Arbitrage is a plan, not a system.** Moving goods is one more recipe to
  the planner (inputs at A, outputs at B, labor the walk), so it needs no
  machinery beyond cargo records and `JobCarry`. It competes with making
  things on the same profit terms.
- **Buy first, carry, then sell.** Reserving the far bid before setting off
  would let a colonist lock a market up by claiming to be on the way. The
  hauler carries the risk instead, which is what the margin pays for.
- **The planner skips bids it can do nothing about.** Once every bid was a
  candidate, the colony's standing ore bids at the silo filled the planner's
  few candidate slots and crowded out bids it could act on. A bid counts
  toward `plan-candidates` only if its good is producible or on offer
  somewhere cheaper.
- **Whoever pays for the work pays for the materials.** Before E7 the builder
  donated its own rock and ore to public works. The colony now builds with
  what it bought. The builder's own stock is still the last resort, which is
  what keeps an empty storeroom from halting construction.
- **In ordinary play the colony's asks rarely fill.** Nothing yet consumes ore
  privately: no recipe uses it, and with construction costs off (the default)
  nobody builds with it. Traces of the reference seeds show the colony
  buying a few hundred units of ore over 10000 ticks and selling none. Its
  resales pay when there's demand: a colonist's bid, or construction costs on.
  E8 turns scarcity on; ore-consuming recipes are what will make the resale
  loop carry the treasury.

## Extending it

- **A new thing to haul for hire**: any issuer can post a `WorkHaul` with
  `From` and `Item` set; `tryAssignHaul` and `jobCarry` do the rest.
- **The colony selling something new**: add it to `prospectingGoods` (it will
  also be bought), or give `refreshColonyAsks` another source.
- **Smarter routes**: `cheapestAskElsewhere` takes the cheapest ask. Weighing
  the walk from the colonist into that choice is the natural next step.

## Related

- [economy.md](./economy.md) — the plan this is phase E7 of.
- [valuation.md](./valuation.md) — the producer planner arbitrage is part of.
- [market.md](./market.md) — books, escrow, the silo.
- [labor.md](./labor.md) — work orders, of which hauling is one kind.
- [construction.md](./construction.md) — construction costs.
- [property.md](./property.md) — ledgers and cargo records.

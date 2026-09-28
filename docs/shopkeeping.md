# Shopkeeping (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan for **shopkeepers**: colonists who buy goods, hold them in a shop they
own, and sell them at a premium. The premium pays for something real.
Today a buyer waits for production. A hungry colonist's meal bid rests for up
to 300 ticks, and a smith's bid for ore waits for someone to go and dig it. A
shop that holds stock turns those waits into instant fills, and it takes on
the risk of paying for goods before anyone has asked for them.

This is the market-making role the colony plays now, reselling ore at
`colony-markup` and keeping meals on its counters. That role moves to
colonists as the colony becomes a minor player ([work-market.md](./work-market.md),
W6).

Nothing here is built.

| Phase | What ships | Status |
| --- | --- | --- |
| K1 | The shop: a depot a colonist owns that anyone may trade at. | Proposed |
| K2 | Stocking: a plan to buy goods to hold, sized by what sells. | Proposed |
| K3 | Pricing: the spread, set against competing shops and the colonist's own price memory. | Proposed |
| K4 | Trading as a skill, and shopkeeper as a profession. | Proposed |

## Source

Nothing exists yet. The expected footprint:

- `internal/sim/property.go` — an `AccessShop` policy.
- `internal/sim/shop.go` — the shop record (stock targets, volume memory),
  `planStock`, and shop pricing.
- `internal/sim/producer.go` — stocking as one more candidate in the chooser
  ([work-market.md](./work-market.md)).
- `internal/sim/snapshot.go` and the market tab — shops and their prices.

## Decisions already made

| Topic | Decision |
| --- | --- |
| Shopkeeping | A profession: someone should be willing to buy stock and hold it to sell at a premium. |
| The colony | A minor player. Holding stock to resell is a colonist's business, not the treasury's ([work-market.md](./work-market.md)). |

## How it works (proposed)

### The shop (K1)

A shop is a storage container the shopkeeper owns, with a new access policy:

| Policy | Who may put in | Who may take out |
| --- | --- | --- |
| `AccessPrivate` (today) | owner | owner |
| `AccessCommunal` (today) | anyone | anyone, their own ledger lines |
| `AccessShop` (new) | owner | anyone, their own ledger lines |

Anyone can reach the shop's book and take out what they've bought, but only the
owner stocks it. Today `canUseFixture` lets nobody but the owner near a
private chest, so a private chest can't be a shop.

Trades at a shop settle like any other depot: the ledger line moves and money
moves, instantly ([market.md](./market.md)). The shopkeeper doesn't have to be
there. That's what makes the job mostly capital and hauling, not standing at a
counter.

Building a shop is an investment like building a workshop
([skills.md](./skills.md), S6): a chest where buyers already are. By the
scumhouse's pantry for meals, by the forge for ore, at the colony center for
everything.

### Stocking (K2)

A stocking plan is the existing arbitrage plan ([hauling.md](./hauling.md))
with one change. It sells into the shop's own stock, at the shop's ask, not
into a bid that is already there:

```
buy:   Q × item, from an ask elsewhere, or from a bid the shop posts itself
carry: to the shop (or not at all, if bought at the shop)
sell:  over time, at the shop's ask
```

How much to hold comes from what sells there. The shop remembers, per item, a
smoothed sale volume per 1,000 ticks (integer, like `priceMemory`). The target
stock is enough for one turnover window, capped by the shopkeeper's money
(escrow) and the chest's space. A shop that isn't selling an item stops
buying it. A shop that sells out raises its target.

A shopkeeper can also stock without walking. It posts **bids at its own
shop**, and a miner or a cook sells into them as it would into any bid. The
shop is then the buyer the colony's ore bids used to be, except that it's
paying with its own money, for goods it expects to sell.

### Pricing (K3)

A shop quotes both sides:

```
bid (buy)  ≤ expected sale price − margin
ask (sell) ≥ what it paid       + margin
```

- The **expected sale price** comes from the shopkeeper's own price memory, not
  a global one ([skills.md](./skills.md), S5). A shopkeeper that knows prices
  better than its customers is exactly how shops make money.
- The **margin** covers the shopkeeper's time and the money its stock ties up,
  at its reservation rate (skills.md, S3).
- **Competition** caps both sides. A shop's ask goes no higher than just under
  the nearest competing ask plus what a buyer would spend walking to it. Its
  bid goes no lower than a seller would accept for not walking further. The
  premium a shop can charge is the convenience it offers, and no more.

Two consequences:

- **Shops sell before production does.** A hungry colonist's bid fills at
  once if a shop has a meal at or below it, so fewer bids rest, and fewer
  derived bids wait on a chain. That's the premium's justification, and it
  should be measurable in `-econ-trace` as shorter waits and fewer resting
  bids.
- **Famine is profitable.** Hungry colonists bid up to `meal-willingness` ×
  a meal's value, and a shop with meals sells to them at those prices. That's
  a profiteer, and a good story (affect, memories, relationships). It's only a
  problem if a shop can starve someone who could otherwise have eaten. A shop
  can buy up the colony's meals before a hungry colonist gets to them, and
  then price them beyond a broke colonist's wallet. The colony's **rations**
  for the critically hungry ([food.md](./food.md)) come from the colony's own
  meals, so they only stay a lifeline if the colony keeps some meals back from
  sale. Keep a ration reserve that no shop can buy.

### The profession (K4)

Speed doesn't help a shopkeeper. Skill and ranks
([skills.md](./skills.md)) would have to mean something else. The best
candidate is **what it knows**:

- A Trading skill widens the colonist's price memory: which depots' trades it
  hears about, and how quickly. A novice knows its own trades. A master knows
  the colony's.
- Practice is counted in trade volume (units bought for stock and sold), the
  way crafting skills count work ticks.

Until K4, shopkeeper is a profession by capital: a colonist who owns a shop.
That's enough for the roster label and for the identity system to hang on.

## Why it is this way

- **A shop is a depot with a policy, not a new kind of thing.** Ledgers,
  escrow, settlement and hauling already exist. What a shop adds is someone
  choosing to hold stock, and an access rule that lets customers take what
  they bought.
- **Stocking reuses arbitrage.** Buying low in one place and selling high in
  another is already a plan. A shop only changes where and when it sells.
- **Volume memory, not forecasting.** A shopkeeper that tried to predict
  demand would need a model of every buyer. Stocking what sold recently is
  cheap, explainable (principle 6), and self-correcting.
- **Private price memory is what makes a spread.** With one global price, every
  colonist knows exactly what a shop's goods are worth, and the only premium
  left is walking.

## Open questions

- **Spoilage.** Meals don't spoil, so holding costs are only money and space.
  If food starts to rot, shops need to account for it, and meal shops get
  riskier.
- **The colony's counters.** The colony keeps meals on its scumhouse counters
  (`refreshColonyMealAsks`). When shops exist, does the colony stop, or
  compete with them?
- **Credit.** A shopkeeper with more customers than money can't stock up.
  Lending is a planned future goal ([economy.md](./economy.md)), and shops
  are its first natural borrowers.
- **Theft.** A shop full of goods is a target. Enforcement is "later" in
  economy.md; shops make it matter.

## Related

- [work-market.md](./work-market.md) — the colony stepping back, and the
  chooser a stocking plan competes in.
- [skills.md](./skills.md) — professions, reservation rates, per-colonist
  price memory, and workshop investment.
- [hauling.md](./hauling.md) — arbitrage, which stocking extends.
- [market.md](./market.md) — the book, escrow and settlement.
- [property.md](./property.md) — owners and access policies.

# Money

> Part of the [mars-sim documentation](./README.md).

## What it is

The colony's currency: dollars, held in a wallet on every colonist and in the
community's treasury. Money has no physical presence and takes no inventory
slot; every exchange is a digital transfer. This is phase **E0** of the
[economy plan](./economy.md). Nothing *spends* money yet — the order book and
labor orders that give it a use come in later phases — so today it is a
supply, a set of accounts, and the rules that keep them honest.

## Source

- [`internal/sim/money.go`](../internal/sim/money.go) — `Money`, `transfer`,
  `mint`, `export`, `freezeWallet`, `balance`, `moneyInCirculation`.
- [`internal/sim/owner.go`](../internal/sim/owner.go) — `Owner`/`OwnerKind`,
  the name of whoever holds money or property.
- [`internal/sim/world.go`](../internal/sim/world.go) — `treasury`,
  `moneyIssued`, `moneyFrozen`; the founding grant (in `newWorld`), the purse
  (in `spawn`), and the freeze (in `remove`).
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Entity.wallet`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `EntityView.Wallet`
  and `Snapshot.Economy` (`EconomyView`).
- [`internal/ui/tui/render_market.go`](../internal/ui/tui/render_market.go) — the
  market tab.
- [`internal/sim/money_test.go`](../internal/sim/money_test.go) — minting,
  transfer rules, freezing on death, and conservation over a live colony.

## How it works

### Accounts and owners

Every holder of money is an `Owner`: the community (`Community`), one colonist
(`ColonistOwner(id)`), or nobody (`Nobody`). The type is shared with
[property](./property.md), so a wallet, a ledger line, and a bunk all name
their holder the same way. `OwnerNone` can own goods (abandoned property) but
never money. An organization owner kind is a commented-out TODO.

`account(owner)` resolves an owner to the balance its money lives in: the
treasury, a **living** colonist's `wallet`, or an open market order's escrow
(an unexported owner kind, see [market.md](./market.md)). Anyone else has no
account.

### The supply

Money is created in exactly two places, both through `mint`:

| Where | When | Setting |
| --- | --- | --- |
| the treasury | once, in `newWorld` | `founding-grant` (default 5000) |
| a colonist's wallet | when it spawns — worldgen, the spawn command, anything else | `crash-pod-purse` (default 100) |

A recruit's wallet is the second row too: it mints the recruit's savings
instead of the purse (see [recruiting.md](./recruiting.md)).

Every minted dollar is added to `moneyIssued`. Money leaves the colony in
exactly one way, through `export`: paying someone off-world, which today is
only the recruiter's fee and a recruit's passage. Every exported dollar is
added to `moneyExported`. When a colonist dies, `freezeWallet` moves its
balance into `moneyFrozen`: the dollars stay on its permanent record (`EntityView.Wallet` in `Snapshot.Deceased`) but
leave circulation, because nobody can spend them. So the books always balance:

```
treasury + Σ living wallets (= moneyInCirculation) + moneyFrozen
    + moneyEscrowed (held by open bids and work orders, see market.md and
      labor.md) + moneyExported (paid off-world) == moneyIssued
```

The wealth levy (below) moves money from wallets to the treasury through
`transfer`, so it changes nobody's total but its payers': the supply only
changes when someone arrives or the colony pays someone off-world.

`TestMoneyIsConserved` checks that on every tick of a colony that is gaining
arrivals, losing colonists to aliens, and shuffling random payments between
random accounts.

### The wealth levy

The treasury pays wages, bounties, hauls, and its standing bids, and its only
income was what it sold. That was not enough. A colonist who cooks its own
scum rarely buys anything, so money pooled in wallets. With 20 colonists the
treasury hit $0 somewhere between ticks 17,500 and 27,500 across seeds. The
colony then stopped buying biomatter, which is where its food came from, and
by tick 40,000 three-quarters of the colony had starved while wallets held
about $5,000.

So every `tax-interval` ticks (`levyWealthTax`), each living colonist pays
`wealth-tax` percent of whatever it holds above `tax-floor` to the treasury:
at least a dollar if it holds any excess, nothing otherwise.
`taxCollected` counts it.

| Setting | Default |
| --- | --- |
| `wealth-tax` | 2 (percent; 0 disables) |
| `tax-floor` | 200 |
| `tax-interval` | 100 ticks |

- **A levy on holdings, not income.** An income tax only shrinks wages, and
  the colony would still pay out more than it takes back. A levy on money
  that sits idle bites when money stops moving, which is exactly when the
  treasury starves. It leaves the poor alone.
- **The floor is below `house-savings`.** At a $300 floor the levy barely
  touched a colony whose wallets averaged about $300, and its treasury ran
  near empty for 30,000 ticks. At $200 the same seeds hold $1,100 to $2,500.
  Saving for a house just takes a little longer.
- Across 9 seeds of 20 colonists at 40,000 ticks, with rations and batch
  cooking (see [food.md](./food.md) and [scumhouse.md](./scumhouse.md)),
  nobody starved and no treasury ran dry.
  `TestTheTreasuryOutlastsALongRun` keeps one of those runs.

### Transfers

`transfer(from, to, amount)` is the only way money changes hands. It refuses:

- a negative amount;
- an owner with no account (nobody, or a colonist who is dead or never
  existed);
- anything that would take `from` below zero.

It is all-or-nothing and reports whether it happened. Paying yourself is a
no-op that still requires the funds.

It also keeps the running totals the chart system reads (see
[charts.md](./charts.md)): `moneyMoved` and `payments`, the dollars and the
number of payments that changed hands. A transfer into escrow, or escrow
going back to its own poster (`payer`), is not a payment, so a bid that fills
under its limit counts its price once and its refund not at all.

### Seeing it

`Snapshot.Economy` carries the treasury, circulating, frozen, escrowed,
exported, and issued totals; each colonist's balance is on its `EntityView.Wallet`. The TUI's
**market** tab (between storage and lore) lists the treasury and every living
colonist richest first, with the money supply beside the selected account.
Both it and the browser's Market tab show the exported total as
**Off-world** once there is any.

## Why it is this way

- **Integers, not floats.** Settlement has to be exactly reproducible for a
  seed ([determinism.md](./determinism.md)); a float total depends on the order
  its terms were summed in. `Money` is whole dollars; if prices ever need
  fractions it becomes cents.
- **One funnel.** Every payment through `transfer` means one place to log, one
  place a tax hooks in (the levy does), and a supply that can be audited at all. A second
  way to write a wallet is a leak waiting to happen.
- **No negative balances — for now.** That is a v1 limit, not a principle.
  Debt and lending are a planned goal (see *Deliberately not doing* in
  [economy.md](./economy.md)), and the `*src < amount` check in `transfer` is
  the line they will relax.
- **Freeze, don't inherit.** Inheritance needs families to be owners, which
  needs organizations and law; the economy plan deliberately leaves it as a
  big TODO. Freezing keeps the conservation invariant exact without deciding
  who a dead colonist's money belongs to.
- **Config fields are `int64`, not `Money`.** The flag binder only accepts
  `int`, `int64`, and `bool` (`bindConfigFlags` in `main.go`), and a named
  type would fall through its type switch.
- **Minting draws no randomness.** The purse and grant are constants, so adding
  money left every simulation fingerprint unchanged for the same seed. A
  recruit's savings are random, but drawn on the recruit stream (see
  [recruiting.md](./recruiting.md)).
- **Exporting is counted, not hidden.** Money paid off-world could have been
  subtracted from `moneyIssued` instead. Keeping it as its own total keeps
  `moneyIssued` a record of everything minted, and lets the market tab say
  how much the colony has spent outside itself.

## Extending it

- **Paying for something** is a `transfer` call; check its result.
- **Paying someone off-world** is an `export` call; check its result.
- **A new source or sink of money** (a tax, a fee that burns money, lending)
  must go through a funnel like `mint` or `export` that keeps an audit total,
  and `assertMoneyConserved` in `money_test.go` must learn about it.
- **A new owner kind** needs a case in `account`; everything else takes an
  `Owner` and shouldn't care.

## Related

- [economy.md](./economy.md) — the full plan this is phase E0 of.
- [property.md](./property.md) — the other half of `Owner`: who owns goods and fixtures.
- [frontend-tui.md](./frontend-tui.md) — the market tab.
- [configuration.md](./configuration.md) — the `founding-grant` and `crash-pod-purse` settings.
- [recruiting.md](./recruiting.md) — the first money paid off-world, and savings minted on arrival.

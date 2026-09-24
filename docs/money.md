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
  `mint`, `freezeWallet`, `balance`, `moneyInCirculation`.
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

Every minted dollar is added to `moneyIssued`. Nothing destroys money. When a
colonist dies, `freezeWallet` moves its balance into `moneyFrozen`: the dollars
stay on its permanent record (`EntityView.Wallet` in `Snapshot.Deceased`) but
leave circulation, because nobody can spend them. So the books always balance:

```
treasury + Σ living wallets (= moneyInCirculation) + moneyFrozen
    + moneyEscrowed (held by open bids, see market.md) == moneyIssued
```

`TestMoneyIsConserved` checks that on every tick of a colony that is gaining
arrivals, losing colonists to aliens, and shuffling random payments between
random accounts.

### Transfers

`transfer(from, to, amount)` is the only way money changes hands. It refuses:

- a negative amount;
- an owner with no account (nobody, or a colonist who is dead or never
  existed);
- anything that would take `from` below zero.

It is all-or-nothing and reports whether it happened. Paying yourself is a
no-op that still requires the funds.

### Seeing it

`Snapshot.Economy` carries the treasury, circulating, frozen, and issued
totals; each colonist's balance is on its `EntityView.Wallet`. The TUI's
**market** tab (between storage and lore) lists the treasury and every living
colonist richest first, with the money supply beside the selected account.

## Why it is this way

- **Integers, not floats.** Settlement has to be exactly reproducible for a
  seed ([determinism.md](./determinism.md)); a float total depends on the order
  its terms were summed in. `Money` is whole dollars; if prices ever need
  fractions it becomes cents.
- **One funnel.** Every payment through `transfer` means one place to log, one
  place a tax will hook in, and a supply that can be audited at all. A second
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
  money left every simulation fingerprint unchanged for the same seed.

## Extending it

- **Paying for something** is a `transfer` call; check its result.
- **A new source or sink of money** (a tax, a fee that burns money, lending)
  must go through a funnel like `mint` that keeps an audit total, and
  `assertMoneyConserved` in `money_test.go` must learn about it.
- **A new owner kind** needs a case in `account`; everything else takes an
  `Owner` and shouldn't care.

## Related

- [economy.md](./economy.md) — the full plan this is phase E0 of.
- [property.md](./property.md) — the other half of `Owner`: who owns goods and fixtures.
- [frontend-tui.md](./frontend-tui.md) — the market tab.
- [configuration.md](./configuration.md) — the `founding-grant` and `crash-pod-purse` settings.

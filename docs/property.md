# Property

> Part of the [mars-sim documentation](./README.md).

## What it is

Who owns what. Every placed fixture (pod, toilet, bed, incinerator, storage
container) has an ownership record saying whose it is and who may use it, and
every storage container keeps a **ledger** of whose goods are inside it. This is
phase **E1** of the [economy plan](./economy.md). It changes nothing about how
the colony plays today — everything the colony builds is owned by the community
and open to all — but it is the ground the crash pods (a colonist's own bunk
and locker), the order book, and paid fixtures stand on.

## Source

- [`internal/sim/property.go`](../internal/sim/property.go) — `Access`,
  `Fixture`, `placeFixture`/`dropFixture`/`setFixtureOwner`, `canUseFixture`,
  `facilityReachable`, `LedgerLine` and the container's `credit`/`held`/
  `ledgerBalanced`, and `publishedFixtures`.
- [`internal/sim/owner.go`](../internal/sim/owner.go) — `Owner`, shared with
  [money](./money.md).
- [`internal/sim/world.go`](../internal/sim/world.go) — `fixtures`,
  `restrictedFixtures`, and the hook in `SetTerrain`.
- [`internal/sim/flowfield.go`](../internal/sim/flowfield.go) — `facilitySeed`
  and `facilityGoal` skip restricted fixtures.
- [`internal/sim/facilitychoice.go`](../internal/sim/facilitychoice.go) —
  `chooseFacility` skips fixtures a colonist may not use.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `runNeedFocus`,
  `jobUse`, `chooseStorage`, and `jobStore` (which credits the depositor).
- [`internal/sim/needs.go`](../internal/sim/needs.go) — the starvation grace
  uses `facilityReachable`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `StorageView.Ledger`,
  `Snapshot.Fixtures`, `Snapshot.FixtureAt`.
- [`internal/sim/property_test.go`](../internal/sim/property_test.go) — access,
  routing, ledgers, and publishing.

## How it works

### Where ownership lives

Stacks are `{Kind, Count}` and stay that way; ownership is recorded next to
things rather than on them, by the kind of thing:

| Thing | Where its owner is recorded |
| --- | --- |
| A placed fixture | `World.fixtures[pos]`: owner plus `Access` |
| Goods in a storage container | the container's `Ledger`: one line per (owner, item) |
| Goods a colonist is carrying, weapons included | nowhere: **what you carry is yours** |

The carrier rule is the default. The exception is a **cargo record**
(`Entity.cargo`, one owner per item kind): a job that carries someone else's
goods marks them. Scraping scum and cleaning up biomatter are community work,
so what they gather is recorded as the colony's, and `deliverBiomatter` credits
it to the colony when it reaches the scumhouse (see
[scumhouse.md](./scumhouse.md)). `carriedOwner` reads the record, falling back
to the carrier.

### Fixtures

`SetTerrain` is the only terrain writer, so it is where fixtures are born and
die: placing a fixture terrain calls `placeFixture`, which records it as owned
by the community and `AccessCommunal`; replacing one calls `dropFixture`.
Anything that wants a different owner calls `setFixtureOwner(pos, owner,
access)` afterwards: crash pods (see [crash-pods.md](./crash-pods.md)) and
commissioned rooms (see [labor.md](./labor.md)) do.

`Access` is `AccessCommunal` (anyone), `AccessPrivate` (the owner only), or
`AccessPaid` (the owner free, anyone else who can pay `Fixture.Price`, charged
by `chargeForUse` when the use finishes — see [labor.md](./labor.md)).
`canUseFixture(e, pos)` answers "may e use this?"; a rat never owns anything or
pays, so it may use only communal fixtures.

### Routing around what isn't yours

The expensive part of ownership is that one **shared flow field** per facility
terrain routes every seeker to the nearest one (see
[pathfinding.md](./pathfinding.md)). A private fixture can't be a goal of that
field, or it would lead everyone else to a bed they may not use. So:

- `facilitySeed` and `facilityGoal` (its one-tile form, which the field's
  incremental repair uses) leave restricted fixtures out: the field leads only
  to communal ones. The two must agree, or a repaired field would differ from
  a rebuilt one.
- `facilityReachable(e, kind)` replaces the old "does the field reach me?"
  check in the need focus, `jobUse`, and the starvation grace. It is true if the
  field reaches `e`, **or** `e` may use a restricted fixture of that kind in its
  own room.
- `chooseFacility` and `chooseStorage` skip fixtures `e` may not use, and
  `jobUse` re-chooses if the fixture it committed to stops being usable.
  `chooseFacility`'s fast tiers read the shared field, which can't see a
  colonist's own bunk, so while any fixture of a kind is restricted it goes
  straight to its bounded search (see [needs.md](./needs.md)). That search
  stops at the first usable free facility, usually the colonist's own.
- `jobUse`'s shortcut for a lone facility — just follow the field — applies only
  while no fixture of that kind is restricted, since the field would walk an
  owner past its own bunk to a shared one.

`restrictedFixtures[kind]` counts non-communal fixtures per terrain. While it is
zero every one of these checks takes its old path, which is why a colony with
no private property plays exactly as before (below). `setFixtureOwner`
`touch`es the kind's field at the fixture whenever it changes between communal
and not. Like a terrain change, that reaches the field on its next read, as an
incremental repair around the tile (see [pathfinding.md](./pathfinding.md)),
at most once per tick (`ensureFresh`).

### Ledgers

A container's `Ledger` says whose its contents are: lines of `{Owner, Item,
Count}`, kept sorted by owner then item, so a ledger reads the same whatever
order deposits arrived in. The one invariant is that, per item kind, the lines
sum to exactly what the container physically holds (`ledgerBalanced`).
`credit` must be called in the same step as the physical add; `jobStore` does
both, crediting the depositing colonist for everything it unloads — so a chest
is shared, but the ore in it stays the miner's. `moveLine` reassigns units from one owner's line to another's with nothing
moving physically — a sale, or an ask setting its goods aside on the order's
own line (see [market.md](./market.md)). `debit` is the other half:
it takes items out on one owner's account, the physical items and the ledger
line together, and refuses to take more than that owner's line holds — so one
owner can never withdraw another's goods. Eating a meal from a locker is its
first user (see [food.md](./food.md)).

### Seeing it

`Snapshot.Fixtures` lists every fixture's owner and access sorted by position
(`FixtureAt` finds one). Publishing rebuilds that list only when `fixtureRev`
moved, and reuses the last published slice otherwise — fixtures change rarely
next to how often frames are published, and a published slice is never written
again. `StorageView.Ledger` is copied per frame with its container.

In the TUI, the map inspector (`i`) shows a fixture's owner and access, storage
details list each chest's ledger under **OWNED BY**, and the market tab totals
an account's holdings across every chest.

### Death

A dead colonist keeps owning what it owned: its ledger lines stay, and a
private fixture of its stays private to nobody who can use it. That is
deliberate for now (see *Deliberately not doing* in
[economy.md](./economy.md)): inheritance needs families to be owners, and
abandoned-property rules need law.

## Why it is this way

- **Ledgers, not owners on stacks.** An owner per stack would split every stack
  by owner, change the capacity rules of every inventory, and still not give
  instant settlement. A ledger per depot gives fungible ownership and a trade
  that is just a line moving from one owner to another, with nothing hauled.
- **Sparse fixture records.** Same reason as `storageContainers`: ownership on
  every tile would inflate the page-shared grid for the handful of tiles that
  are fixtures.
- **The shared field stays communal-only.** The alternative — a field per owner
  — multiplies the most expensive structure in the pathfinder by the
  population. A private fixture is used by one colonist, so routing to it with
  A\* costs one search, only when that colonist needs it.
- **Behavior was proven unchanged, not assumed.** Before this change, five
  seeds (80×50, 16 colonists, 3000 ticks) were fingerprinted every 50 ticks;
  after it, every fingerprint matched exactly. Keep that bar for anything that
  is supposed to be pure plumbing.
- **Ownership is in the lockstep test.** `worldFingerprint` now includes a
  `property` field (wallets, fixtures, ledgers), so a future economy decision
  made in map order fails `TestDeterministicRunAgreesEveryTick` by name. See
  [determinism.md](./determinism.md).

## Extending it

- **Giving a colonist a fixture**: `setFixtureOwner(pos, ColonistOwner(id),
  AccessPrivate)` after placing it. Everything that routes already respects it.
- **A new fixture terrain**: add it to `isFixtureTerrain`, and make sure its
  users go through `canUseFixture`. One existing user does not yet: cleaners
  find an incinerator through its shared field alone (`cleaning.go`). That is
  correct only while nothing makes an incinerator private — teach the haul
  route `canUseFixture` before anything does.
- **Charging for a new kind of use**: `AccessPaid` charges in `finishUse`;
  any other executor that finishes a use of a paid fixture must call
  `chargeForUse` too.
- **Taking goods out of storage**: `debit(owner, kind, n)`. Never call
  `StorageInventory.Remove` directly on a container that has a ledger; that
  unbalances it.

## Related

- [economy.md](./economy.md) — the plan this is phase E1 of.
- [money.md](./money.md) — the other half of `Owner`: wallets and the treasury.
- [storage.md](./storage.md) — containers, and how colonists come to store.
- [needs.md](./needs.md) — facility use, which private fixtures now gate.
- [pathfinding.md](./pathfinding.md) — the shared flow fields.

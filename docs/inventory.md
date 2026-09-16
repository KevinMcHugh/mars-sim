# Inventory

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists carry items in a fixed set of slots, each holding one homogeneous stack.
`RawRock` (from mining) and the `Pistol`/`Shotgun` weapons (from the colony
ship's starting equipment; see [combat.md](./combat.md)) are the only items
today, but the machinery is generic.

## Source

- [`internal/sim/inventory.go`](../internal/sim/inventory.go) — `ItemKind`, `ItemStack`, `Inventory`, `CanAdd`/`Add`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — the `Inventory` field on `Entity`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — mining awards `RawRock` (`jobMine`).

## How it works

An `Inventory` is a fixed array of `InventorySlotCount` (8) `ItemStack`s. Each
stack is one `ItemKind` and a count up to `MaxStackSize` (64). Empty slots have
count 0 and `ItemNone`.

- `CanAdd(kind, n)` reports whether `n` items fit **without changing anything** —
  it sums remaining capacity across partial matching stacks and empty slots.
- `Add(kind, n)` stores all `n` items, filling existing matching stacks before
  empty slots, and is **all-or-nothing**: it returns false and leaves the
  inventory untouched if they do not all fit.

Mining is the one producer today. `jobMine` awards the `RawRock` **before**
changing the terrain, so a full inventory can never make mined material disappear;
and `assignWorkJob` refuses to start a mine job when the colonist cannot carry the
result (`CanAdd(RawRock, 1)`), so colonists don't begin work they can't complete.

Weapons are the other producer, though a one-time one: `equipColonyShip`
(`combat.go`) hands a `Pistol` or `Shotgun` to a colonist's inventory once, at
worldgen. There is no equip/unequip step — `bestWeapon` (`inventory.go`) just
scans the stacks for the best weapon kind present, so carrying one *is*
wielding it. Nothing removes a weapon from inventory today (no drop, no
ammo, no loss on death), so once armed, always armed.

Inventory is copied by value into the snapshot's `EntityView`, so the frontend can
render it without touching live state.

## Why it is this way

- **Fixed slots + homogeneous stacks** are a simple, bounded model that renders
  cleanly (the roster shows eight slots) and is enough for a scaffold.
- **All-or-nothing `Add`** keeps callers simple: a partial add would force every
  caller to handle leftovers.
- **Award-before-terrain-change** and the pre-mine capacity check are the
  invariant that keeps resources conserved.

## Extending it

- **A new item**: add an `ItemKind` constant and its `String()` case; the stacking
  logic is generic. Add a producer/consumer where it makes sense.
- **Stockpiles / hauling** would build on this: a haul job would move stacks
  between an inventory and a storage structure (a future project kind — see
  [construction.md](./construction.md)).
- **Picking up or dropping a weapon mid-game** would need a floor-item concept
  the world doesn't have yet — today the only way an inventory changes is
  mining or the one-time worldgen equip. See [combat.md](./combat.md)'s
  Extending it for more.

## Related

- [entities-and-ai.md](./entities-and-ai.md) — mining, the current producer.
- [combat.md](./combat.md) — the pistol/shotgun weapons carried in inventory.
- [frontend-tui.md](./frontend-tui.md) — how the roster renders inventory.

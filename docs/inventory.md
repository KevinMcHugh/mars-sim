# Inventory

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists carry items in a fixed set of slots, each holding one homogeneous stack.
Mining produces `RawRock` and may also produce `IronOre`, `WaterIce`,
`UraniumOre`, or `Clay`, depending on the excavated tile's rock composition. Colonists may also carry `Pistol` or
`Shotgun` weapons from the colony ship's starting equipment; see
[combat.md](./combat.md). Cleaning up after the colony's dead fills slots too,
with the `Viscera`/`Corpse` refuse a cleaner carries to the incinerator; see
[sanitation.md](./sanitation.md).

## Source

- [`internal/sim/inventory.go`](../internal/sim/inventory.go) — `ItemKind`, `ItemStack`, `Inventory`, the atomic add/yield helpers, and `RemoveAll`/`Count` (how a load is burned).
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
- `AddAll(stacks...)` applies the same rule to a heterogeneous group of stacks.
  It works against an inventory copy and commits only if the complete group fits.
- `CanAddAll(stacks...)` performs that heterogeneous capacity check without
  changing the inventory, so job selection can skip deposits a colonist cannot
  carry completely.

Mining is the one producer today. `miningYield` always returns one `RawRock` and
adds one `IronOre`, `WaterIce`, `UraniumOre`, or `Clay` for a bearing tile. `jobMine` and construction
dig tasks add that complete yield with `AddAll` **before** changing terrain, so
limited inventory can never make one part of a deposit disappear.

`UraniumOre` is the one item that acts on its carrier.
`Inventory.Has(UraniumOre)` is one of the two things that puts a colonist under
a mutation-causing dose every tick — see [mutation.md](./mutation.md). The dose
ends after a full colonist unloads its ore into storage.

Weapons are the other producer, though a one-time one: `equipColonyShip`
(`combat.go`) hands a `Pistol` or `Shotgun` to a colonist's inventory once, at
worldgen. There is no equip/unequip step — `bestWeapon` (`inventory.go`) just
scans the stacks for the best weapon kind present, so carrying one *is*
wielding it. Nothing removes a weapon from inventory today (no drop, no
ammo, no loss on death), so once armed, always armed.

Whatever a colonist carries is its own property — there is no owner on a
stack. Once deposited, whose it is lives in the chest's ledger instead; see
[property.md](./property.md).

Inventory is copied by value into the snapshot's `EntityView`, so the frontend can
render it without touching live state.

## Why it is this way

- **Fixed slots + homogeneous stacks** are a simple, bounded model that renders
  cleanly (the roster shows eight slots) and is enough for a scaffold.
- **All-or-nothing `Add` and `AddAll`** keep callers simple: a partial
  composition yield would force mining to handle leftovers or silently destroy
  a resource.
- **Award-before-terrain-change** and the pre-mine capacity check are the
  invariant that keeps resources conserved.

## Extending it

- **A new item**: add an `ItemKind` constant and its `String()` case; the stacking
  logic is generic. Add a producer/consumer where it makes sense.
- **Stockpile withdrawal** would move requested stacks from an existing
  [storage container](./storage.md) back into an `Inventory`. Full colonists
  already deposit general materials automatically.
- **Picking up or dropping a weapon mid-game** would need a floor-item concept
  the world doesn't have yet. See [combat.md](./combat.md)'s
  Extending it for more.

## Related

- [entities-and-ai.md](./entities-and-ai.md) — mining, the current producer.
- [mutation.md](./mutation.md) — what carrying uranium ore does to the carrier.
- [combat.md](./combat.md) — the pistol/shotgun weapons carried in inventory.
- [frontend-tui.md](./frontend-tui.md) — how the roster renders inventory.

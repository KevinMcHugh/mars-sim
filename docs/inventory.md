# Inventory

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists carry items in a fixed set of slots, each holding one homogeneous stack.
Mining produces `RawRock` and may also produce `IronOre` or `WaterIce`, depending
on the excavated tile's rock composition.

## Source

- [`internal/sim/inventory.go`](../internal/sim/inventory.go) — `ItemKind`, `ItemStack`, `Inventory`, and atomic add/yield helpers.
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
adds one `IronOre` or `WaterIce` for a bearing tile. `jobMine` and construction
dig tasks add that complete yield with `AddAll` **before** changing terrain, so
limited inventory can never make one part of a deposit disappear.

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
- **Stockpiles / hauling** would build on this: a haul job would move stacks
  between an inventory and a storage structure (a future project kind — see
  [construction.md](./construction.md)).

## Related

- [entities-and-ai.md](./entities-and-ai.md) — mining, the current producer.
- [frontend-tui.md](./frontend-tui.md) — how the roster renders inventory.

# Inventory

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists carry items in a fixed set of slots, each holding one homogeneous stack.
Today the only item is `RawRock`, produced by mining, but the machinery is generic.

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

## Related

- [entities-and-ai.md](./entities-and-ai.md) — mining, the current producer.
- [frontend-tui.md](./frontend-tui.md) — how the roster renders inventory.

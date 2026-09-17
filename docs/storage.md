# Storage containers

> Part of the [mars-sim documentation](./README.md).

## What it is

Storage containers are placeable trunks for colony materials. Each container
holds 48 homogeneous item stacks: exactly six times a colonist's eight-slot
inventory.

## Source

- [`internal/sim/inventory.go`](../internal/sim/inventory.go) — container state,
  capacity, and atomic stack operations.
- [`internal/sim/world.go`](../internal/sim/world.go) — `Storage` terrain and the
  sparse position-to-container state.
- [`internal/sim/project.go`](../internal/sim/project.go) — the one-container
  storage-room construction recipe.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — immutable
  `StorageView` copies for frontends.
- [`internal/ui/tui/model.go`](../internal/ui/tui/model.go) — the build-menu
  order (`b`, then `r`).

## How it works

`StorageInventorySlotCount` is defined as `6 * InventorySlotCount`, so changing
colonist carrying capacity keeps the six-inventory requirement true. A
`StorageInventory` uses the same `ItemStack` and `MaxStackSize` rules as a
colonist. `Add`, `AddAll`, `CanAdd`, `CanAddAll`, `Count`, and `RemoveAll` reuse
the same internal stack operations; additions are all-or-nothing.

The `Storage` terrain is solid and used from an adjacent floor tile, like other
facilities. Building one creates a `StorageContainer` in
`World.storageContainers`; replacing that terrain removes its state. Containers
are sparse rather than embedded in `Tile`, because adding 48 stacks to every
rock tile would make large maps prohibitively expensive.

The build menu queues an `OrderStorageRoom`. The normal project planner finds a
site and builds a small walled room containing one trunk. The planner also
creates a storage room automatically when a colonist's material load prevents
it from accepting the raw rock included in every mining yield and no reachable
container can accept that complete load. Storage outranks non-fatal dormitory
demand and may exceed the normal concurrent-project cap by one: otherwise full
builders can deadlock every active project's excavation phase while the project
cap prevents the one structure that would unblock them.

During work selection, a blocked colonist seeks the nearest reachable chest that
can accept its complete material load. If none exists but a storage project is
active, it claims that project's reachable work instead of unrelated
construction. At the chest, `JobStore` atomically adds all general materials,
then removes them from the colonist. Raw rock, iron ore, water ice, and uranium
ore are general materials. Weapons remain equipped, while viscera and corpses
retain their dedicated incinerator route.

Snapshot storage views are sorted by position before publication, avoiding
nondeterministic map iteration order.

The TUI exposes contents in two places:

- Press `i` on the map to enter inspection mode. Arrows or `hjkl` move the
  one-tile cursor and the sidebar identifies the tile. On a storage container it
  lists occupied stacks; `enter` opens that chest in the storage details tab.
- Press `tab` three times from the map to reach **Details · Storage** directly.
  `up`/`down` or `j`/`k` selects among containers, and the right panel shows the
  selected chest's occupied slots, item count, and capacity.

Colonists deposit only when their inventory blocks further mining; they do not
continually shuttle every new item. There is no automatic withdrawal policy yet.

## Why it is this way

- **One trunk per order** makes placement intentional and avoids guessing a
  colony-wide desired capacity. One trunk already holds six full colonist loads.
- **Terrain plus sparse state** fits existing construction, pathing, and
  rendering without bloating the page-shared tile grid.
- **Shared stack rules** prevent colonist and container inventories from
  drifting on capacity or transaction semantics.
- **Work-blocked depositing** gives storage a concrete motivation without
  turning every mined item into a hauling trip. Weapons and refuse are excluded
  explicitly so unloading cannot disarm a colonist or bypass sanitation.
- **Complete-load destination checks** keep transfer atomic and simple. A nearly
  full chest is skipped rather than accepting part of a load and leaving the
  colonist ambiguously blocked.

## Extending it

A withdrawal/production system should add reservations before multiple jobs can
promise the same stored items. Deposits currently need no reservation for
correctness: the engine is single-threaded and rechecks capacity on arrival; a
colonist simply retries another destination if somebody filled its chosen chest.

If containers become destructible, define where their contents go before
allowing terrain replacement in normal play; `SetTerrain` currently discards
container state because demolition is not a player action.

## Related

- [inventory.md](./inventory.md) — item kinds and shared stack semantics.
- [construction.md](./construction.md) — project phases and room siting.
- [snapshot-tile-grid.md](./snapshot-tile-grid.md) — why large mutable state
  does not belong in every tile.

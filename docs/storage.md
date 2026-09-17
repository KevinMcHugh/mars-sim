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
site and builds a small walled room containing one trunk. Snapshot storage views
are sorted by position before publication, avoiding nondeterministic map
iteration order.

The TUI exposes contents in two places:

- Press `i` on the map to enter inspection mode. Arrows or `hjkl` move the
  one-tile cursor and the sidebar identifies the tile. On a storage container it
  lists occupied stacks; `enter` opens that chest in the storage details tab.
- Press `tab` three times from the map to reach **Details · Storage** directly.
  `up`/`down` or `j`/`k` selects among containers, and the right panel shows the
  selected chest's occupied slots, item count, and capacity.

There is no automatic hauling policy yet. The container is a complete placeable
storage primitive, while deciding what colonists should deposit and withdraw is
a separate AI/job feature rather than hidden behavior attached to construction.

## Why it is this way

- **One trunk per order** makes placement intentional and avoids guessing a
  colony-wide desired capacity. One trunk already holds six full colonist loads.
- **Terrain plus sparse state** fits existing construction, pathing, and
  rendering without bloating the page-shared tile grid.
- **Shared stack rules** prevent colonist and container inventories from
  drifting on capacity or transaction semantics.
- **No automatic hauling yet** avoids an arbitrary resource policy. Hauling
  needs priorities, reservations, and destination selection; silently emptying
  colonists into the nearest trunk would make weapons and uranium behavior
  surprising.

## Extending it

A hauling system should reserve a source stack and destination capacity before
assigning a job, then transfer only when the colonist reaches the adjacent
container tile. Preserve all-or-nothing additions and ensure abandoning a job
releases both reservations.

If containers become destructible, define where their contents go before
allowing terrain replacement in normal play; `SetTerrain` currently discards
container state because demolition is not a player action.

## Related

- [inventory.md](./inventory.md) — item kinds and shared stack semantics.
- [construction.md](./construction.md) — project phases and room siting.
- [snapshot-tile-grid.md](./snapshot-tile-grid.md) — why large mutable state
  does not belong in every tile.

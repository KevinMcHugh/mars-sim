# Layers and levels

> Part of the [mars-sim documentation](./README.md).

## What it is

The world is built to hold more than one level, though it has only one so
far. Everything that belongs to one 2D grid (tiles, occupancy, regions,
fixtures, refuse, scum and salt, the job board, worldgen state) lives in a
`Layer`. The `World` keeps what spans levels: entities, the economy, flow
fields, the region graph and scratch buffers. A `Level` numbers the layers: 0
is the surface, 1 the landing level where crash pods come down (the only one
that exists), and deeper levels count up. A `Loc` is a tile on a particular
level.

This is phase Z0 of [z-levels.md](./z-levels.md): the data model for levels,
with no behavior change. The golden hashes did not move.

## Source

- [`internal/sim/layer.go`](../internal/sim/layer.go): `Level`,
  `SurfaceLevel`, `LandingLevel`, `Loc`, `at`, `lessLoc`, `Entity.Loc`,
  `Layer`, `newLayer`, `World.layer`, `World.homeLoc`.
- [`internal/sim/world.go`](../internal/sim/world.go): `World.home` and
  `World.layers`, set up in `newWorld`.
- [`internal/sim/entity.go`](../internal/sim/entity.go): `Entity.Level`.
- [`internal/sim/market.go`](../internal/sim/market.go): `Order.Depot` and
  `bookKey.Depot` are `Loc`s.
- [`internal/sim/layer_test.go`](../internal/sim/layer_test.go): the
  one-layer shape, nothing on the surface after a long run, and `lessLoc`.

## How it works

### `w.home` marks every single-level assumption

`World.home` is the landing level's `Layer`, and the only one. Code reaches
per-level state through it: `w.home.tiles`, `w.home.occ`,
`w.home.storageContainers`. `World.layers` indexes layers by `Level`
(`layers[LandingLevel] == &w.home`, everything else nil), and `w.layer(l)`
returns one or nil.

The point of the split is that **per-level state cannot be reached without
naming a layer**, and in Z0 the only name is `home`. So `w.home.` is an
exact list of the code that assumes there is one level. When a second level
arrives, removing `home` turns every one of them into a compile error, and
each gets decided: which layer does this mean? That list is the work of the
next phase, found by the compiler instead of by a reviewer's memory.

`w.homeLoc(p)` is the same marker at the edge where grid code's bare `Point`
becomes a `Loc`: "this point is on the landing level".

### Where a level is part of an identity

- **Entities** carry `Level` next to `Pos`, and `e.Loc()` gives both. `Pos`
  stayed a `Point`: only `moveEntity` assigns it, so the two cannot drift
  apart, and the hundreds of reads of `e.Pos` that are about "where on this
  grid" did not need to change.
- **The order book** keys books by `bookKey{Item, Depot Loc}`, and every
  `Order.Depot` is a `Loc`. Two depots at the same `(x, y)` on different
  levels are different books. The market's functions (`post`, `bestAsk`,
  `bestBid`, `openQty`) still take a `Point` and pin it with `w.homeLoc`.
  That is the one place to widen when depots can be anywhere. Views handed
  to frontends (`OrderView`, `BookView`, `Trade`) still carry a `Point`.

### The zero `Loc` is on the surface

The landing level is 1, not 0, so a `Loc` built without a level (a keyed
literal `Loc{Point: p}`, or a zero-valued field used as "unset") lands on
the surface rather than on the colony's own level. That makes the mistake
visible: `TestNothingIsOnTheSurface` runs a trading colony for 1200 ticks and
fails if any entity, book or order is on any level but the landing level.
Build `Loc`s with `at(level, p)` or `w.homeLoc(p)`.

## Why it is this way

- **`home` is a value field, not a pointer.** `w.home.tiles.at` and
  `w.home.occ` are the hottest reads in the simulation, and as a value field
  they compile to the same offset load `w.tiles` did. `layers` holds a pointer
  to it for code that wants a `*Layer`. Benchmarks against the commit
  before the split were within noise.
- **Not `Entity.Pos Loc`.** The proposal had it. Making `Pos` a `Loc` meant
  appending `.Point` to most of the ~375 reads of `e.Pos`. That changes no
  behavior and hides the real question, which is always "on which layer?",
  behind a field access. The reads that touch per-level state already go
  through `w.home`, so the compiler finds them anyway.
- **Scratch stays on `World`.** The A\* scratch, the facility search's cells
  and the transit buffers are sized to one grid and only live inside one
  call. They are reused across levels, the same way they are reused across
  calls, not duplicated per layer.
- **What stays on `World` that will need a level.** The flow fields, the
  region graph (`regions`, `nextRegion`, rooms) and the market's cached
  silo (`marketDepotAt`, `siloWas`) are shared by design. Region IDs are
  global so a stair can link two levels' regions. Flow fields will span
  levels. The silo cache is a `Point` from a `w.home` lookup. These are
  listed here because the compiler will not flag them when `home` goes.

## Extending it

- New per-level state goes on `Layer`, initialized in `newLayer`. New
  cross-level state goes on `World`.
- New code that names a place other code will keep (a claim, a target, an
  order) should take a `Loc` if it can outlive the grid call it came from.
- Do not add `w.home` to code that could be told its layer: take a `*Layer`
  or a `Loc` instead. Every new `w.home` is one more site for Z1 to decide.
- Keep `TestNothingIsOnTheSurface` meaningful: if a new identity becomes a
  `Loc`, add it to the test's checks.

## Related

- [z-levels.md](./z-levels.md): the plan this is phase Z0 of.
- [determinism.md](./determinism.md): `lessLoc` extends `lessPoint`'s
  tie-break across levels.
- [sparse-grids.md](./sparse-grids.md): why a layer's grids cost nothing
  until something is written to them.
- [market.md](./market.md): the order book whose depots are now `Loc`s.

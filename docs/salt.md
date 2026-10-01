# Salt

> Part of the [mars-sim documentation](./README.md).

## What it is

Salt is a deposit on a rock tile, 3% of the map by default (`salt-percent`).
It rides on the rock the way [cave scum](./scumhouse.md) does, shares a tile
with scum never, and does not regenerate: world generation lays it down, and
from then on it can only be lost. **Nothing uses it yet.** No colonist can
gather it, no recipe needs it and no frontend draws it; this is the world-state
substrate those will build on.

## Source

- `internal/sim/worldgen_chunks.go`: `saltPlan` (the plan), `runPlan` (the walk
  it shares with scum), `chunkContent.isSalt`.
- `internal/sim/worldgen.go`: `applyChunk` writes a chunk's salt into `World.salt`.
- `internal/sim/salt.go`: `hasSalt` and `clearSalt`.
- `internal/sim/scumhouse.go`: `addScum` refuses a salt tile.
- `internal/sim/salt_test.go`, `worldgen_chunks_test.go`
  (`TestSaltNeverSharesATileWithScum`), `worldgen_drift_test.go`.

## How it works

`World.salt` is a set of tiles. Each chunk plans its share of `salt-percent` of
its area in the same short meandering runs as scum (`scumRunMin`–`scumRunMax`
steps, distinct tiles counted so the chunk lands on its share). Like scum it
sits on the rock rather than in it: a deposit can lie on what becomes cavern or
landing floor, and mining through the tile leaves the salt on the floor.

Salt and scum exclude each other in both directions:

- **At generation, salt steps around scum.** `saltPlan` reads the scum plans of
  the chunk and its eight neighbours, and its walk skips a scum tile without
  placing there. It keeps walking, so the chunk still reaches its budget.
- **After generation, scum steps around salt.** `addScum` returns early on a
  salt tile, so neither a spawn nor a spread starts a patch there.

It does not regenerate because nothing ever adds to `World.salt` after
`applyChunk`. There is no growth step to turn off. The only thing that changes
it is `clearSalt`, which `setTerrain` calls when a structure is built on the
tile, beside `clearScum`.

## Why it is this way

- **Salt yields to scum, not the other way round.** Scum's plan reads nothing
  and the food economy is tuned to its density (see the warning in
  `config.go`). Making scum avoid salt would have made scum's plan depend on
  salt's, and shifted every existing world's scum. As it is, adding salt moves
  no scum, no vein and no cavern: the golden hashes at tick 0 differ only by
  the new `salt=` segment.
- **Avoid, don't overwrite.** The first design was to generate both
  independently and drop the salt tiles that landed on scum. That works, but it
  loses about 6% of salt (scum's density) and the 3% target comes out as 2.8%.
  Steering the walk costs one set of scum tiles per chunk plan and hits the
  target to two decimals (see the drift report in
  [worldgen-chunks.md](./worldgen-chunks.md)).
- **Why neighbours' scum is enough.** A salt tile lies at most
  `scumRunMax - 1` outside its owner chunk, and any scum that could cover it
  started within as far again. Two such reaches fit inside one chunk, which a
  compile-time assertion next to `scumRunMax` checks. So the avoid set is the
  3×3 neighbourhood and the planning horizon is unchanged: chunk, then
  `saltPlan` (1), then `scumPlan` (2), inside the existing four.
- **A set, not an amount.** Scum has a quantity per tile because a colonist
  scrapes it a unit at a time. Salt has no consumer to dictate one, so it is a
  bare deposit. Give it an amount when a gatherer exists, and a `salt-max` to
  go with it.

## Extending it

- To let colonists gather salt, follow scum's path: an exposure set
  (`exposedScum` and `refreshScumExposure` are the model, driven by
  `TileChanged`), a snapshot field, a job and an item. Keep `clearSalt` the only
  thing that removes a deposit, or document why not.
- Keep the two invariants in
  `TestSaltNeverRegeneratesOrMeetsScum`: nothing adds salt after generation,
  and no tile holds salt and scum.
- A new deposit that must also avoid scum can reuse `runPlan`. Give it its own
  stream constant and put it in the reach assertions.

## Related

- [scumhouse.md](./scumhouse.md): cave scum, the other deposit that rides on rock.
- [worldgen-chunks.md](./worldgen-chunks.md): chunked generation and the
  abundance drift table.
- [rng-streams.md](./rng-streams.md): the worldgen stream table.

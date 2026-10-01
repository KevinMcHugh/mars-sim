# Chunked world generation

> Part of the [mars-sim documentation](./README.md).

## What it is

World generation lays down ore veins, cave scum, salt, hidden natural caverns
and the passages between them one 64×64 **chunk** at a time. What a chunk holds is a pure
function of `(Config, cx, cy)`. It does not depend on which other chunks
exist, or on the order anything was generated in. That property is what lets
chunks be generated **lazily**: a new game generates the landing site's
neighbourhood, and the rest of the map is generated as the colony explores
toward it. On a 10000×10000 map, starting a game went from 11.4 s and 1.35 GB
to 27 ms and 28 MB.

Only the simulation decides when a chunk is generated, never a frontend, so a
seed produces the same world by the same tick on every machine.

This replaced a whole-map generator (`growRockVeins` and `generateCaverns`)
whose output depended on the order it ran in. That generator is described
under [Why it is this way](#why-it-is-this-way).

## Source

- [`internal/sim/worldgen_chunks.go`](../internal/sim/worldgen_chunks.go):
  `worldGen`, the plans (`veinPlan`, `scumPlan`, `saltPlan`, `cavernCandidates`,
  `keptCaverns`, `passagePlan`), `runPlan` (the short runs scum and salt share),
  `chunk`, `featureRand`, `stratified` and `genCache`.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go): `generate`,
  `generateChunkAt` / `generateAround` / `generateChunk` (when chunks are
  generated), `applyChunk` (writing one into the tile grid), and
  `randomTile`'s sampling over generated chunks.
- [`internal/sim/world.go`](../internal/sim/world.go): `setTerrain`,
  `carveHidden` and `reveal`, the three places that trigger generation.
- [`internal/sim/preview.go`](../internal/sim/preview.go): `ChunkPreview`,
  which shows frontends ungenerated chunks without generating them.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go):
  `Snapshot.TileAt` / `TerrainAt` reading the preview, and
  `Stats.ChunksGenerated` / `Stats.Chunks`.
- [`internal/sim/preview_test.go`](../internal/sim/preview_test.go): the
  preview matches generation, stays behind the fog, never affects the game,
  and is safe for concurrent readers.
- [`internal/sim/worldgen_lazy_test.go`](../internal/sim/worldgen_lazy_test.go):
  lazy chunks match the pure generator, generation stays ahead of exploration
  every tick, reading never generates, and a new game on a huge map generates
  only the landing site.
- [`internal/sim/worldgen_chunks_test.go`](../internal/sim/worldgen_chunks_test.go):
  order independence, reach, features crossing chunk edges, and the proof that
  writing hidden floor without `TileChanged` changes nothing.
- [`internal/sim/worldgen_drift_test.go`](../internal/sim/worldgen_drift_test.go):
  how far abundance drifts from its targets, as a report and as a gate.
- [`internal/sim/worldgen_test.go`](../internal/sim/worldgen_test.go): the
  vein-connectivity sweep.
- [`internal/sim/bench_test.go`](../internal/sim/bench_test.go):
  `BenchmarkChunkCold` / `BenchmarkChunkWarm` (one chunk) and
  `BenchmarkGenerateMap` (a whole 1024×1024 map, with and without salt).
- [`internal/sim/golden_test.go`](../internal/sim/golden_test.go): pinned
  hashes of what fixed seeds produce (see [determinism.md](./determinism.md)).

## How it works

### Plans and chunks

Every feature is **owned** by the chunk its origin lies in, and has a bounded
**reach**:

| Feature | Owner | Reach from its origin |
| --- | --- | --- |
| Vein | chunk of its first tile | `veinReach` = 16 |
| Cave scum run | chunk it starts in | `scumRunMax` - 1 = 7 |
| Salt run | chunk it starts in | `scumRunMax` - 1 = 7 |
| Cavern | chunk of its center | `cavernReach` = 20 |
| Passage | chunk of the higher-ranked of its two caverns | `passageMaxSpan` + `passageSlack` = 44 |

All three reaches are under one chunk (a compile-time assertion checks it), so a
feature can only touch its owner and the eight chunks around it. Generating a
chunk takes two steps:

1. **Plan.** Roll a chunk's features from its own random stream. A plan reads
   no terrain and never reads the tiles it is about to write.
2. **Chunk.** Collect the plans of the chunk and its neighbours and keep the
   tiles that fall inside it.

The rule that keeps this bounded is **a chunk reads only plans, and a plan
never needs another chunk to be generated**. The planning horizon this
produces is described below. Without it, generating B would
generate A, A would generate its own neighbours, and the cascade would run
across the map.

A chunk only takes tiles from its eight neighbours, but the plans it reads
depend on further plans. That makes the **planning horizon** four chunks out:

- **Caverns:** chunk → `passagePlan` (radius 1) → `nearestCavern` (2) →
  `keptCaverns` (3) → `cavernCandidates` (4).
- **Salt:** chunk → `saltPlan` (radius 1) → `scumPlan` (2).
- **Veins:** each level reads the previous level's plans one chunk further out,
  so clay reaches iron plans three chunks out.

Every step reads only plans, so the horizon is fixed and does not grow into a
cascade. It does make the first chunk in a new area expensive:
`BenchmarkChunkCold` (empty cache) takes about 5.2 ms natively, and
`BenchmarkChunkWarm` (a neighbour with the plans already cached) about 28 µs.
The chunks around an area share nearly all their plans, so the cold cost is
paid once per area, not once per chunk.

Plans are cached in a `genCache`: two generations of map, where the older is
dropped whole when the newer fills. The cache is only a cache. Dropping a plan
and recomputing it gives the same answer, so its size can never change a
world. `TestChunkGenerationIsOrderIndependent` runs with a three-entry cache
to prove it.

The cache lives as long as the game, so it is sized to what one step of
exploration needs: generating the halo around a newly seen chunk plans a
square of `2*(halo + genHorizon) + 1` chunks (13×13 at the default halo),
and each kind of plan keeps two of those. The first version kept a flat 4096
per kind and grew by about 90 MB over 2,900 generated chunks; sized to the
halo, it is about 13 MB. Something that generates far more at once, like
the drift report sweeping a map in raster order, raises it with
`withCacheSize`.

### Streams

`featureRand(stream, ids...)` seeds a PCG from the world seed, a stream
constant, and the ids that name the feature (chunk coordinates, or both
caverns of a passage). Each id goes through splitmix64, so neighbouring
chunks get unrelated streams. There is no shared stream anywhere in worldgen,
and that is what makes the order irrelevant.

### Veins

Compositions have a priority order, `veinLevels`: iron, then ice, uranium,
clay. For each chunk and level:

- **Count.** `stratified(area × percent / 100 / mean vein size)`: the floor of
  the expected count, plus one with the leftover fraction as its probability.
- **Origin.** A random tile of the chunk that is not taken by an earlier
  level and has at least one free orthogonal neighbour. It gets
  `veinOriginTries` (8) attempts, then the vein is skipped.
- **Walk.** The same meandering walk as before: advance from the tip, branch
  from an earlier tile one time in six or when the tip is boxed in. It stays
  inside `veinReach` of the origin and inside the map. It avoids tiles of
  earlier levels, the vein's own tiles, and the chunk's earlier veins of the
  same level.

A level's plan reads the plans of earlier levels within one chunk. That is a
dependency four levels deep at most, and never on the same level's
neighbours. Veins of the same level from neighbouring chunks can still
overlap. They merge into one bigger deposit, which costs a little abundance
but no connectivity.

**Every deposit tile has an orthogonal neighbour of its own composition.**
The origin always has a free neighbour, so every vein has at least two tiles,
and no later level ever overwrites an earlier one. `RockVeinMin` is raised to
2 if it is set lower. `TestRockDepositsAreVeinsRatherThanIsolatedTiles` sweeps
300 seeds under three single-chunk configs (ordinary, crowded, and
one-to-two-tile veins), plus 30 seeds of a crowded multi-chunk map where veins
of every level meet their neighbours' veins.
The old generator failed that invariant on about 6% of seeds: see
[rng-streams.md](./rng-streams.md).

### Cave scum

Each chunk places its share of `scum-percent` of its area as distinct tiles,
in short meandering runs of `scumRunMin`–`scumRunMax` steps
(`scumPlan`). Scum rides on the rock rather than in it, so it reads nothing
and ignores caverns and ore, just as the old whole-map `growScum` did (it ran
before anything was carved). Counting distinct tiles matters: the first
version counted steps, and because short walks step back on themselves, it
came out a fifth short. `applyChunk` adds each patch to `World.scum` at full
strength and registers any that is already exposed. That is never the case
in practice, because discovered floor always has its neighbours generated
first.

### Salt

`saltPlan` is `runPlan` again with its own stream and one difference: its walk
skips every tile in the scum plans of the chunk and its eight neighbours. Scum
is the fixed point, so adding salt moved no existing scum, vein or cavern. Both
keep their sets (placed tiles, scum to avoid) as bitmaps over the run window:
the first version used maps and nearly doubled whole-map generation. The full
reasoning and the numbers are in [salt.md](./salt.md).

### Caverns

- **Candidates.** Each chunk plans caverns until their tiles reach its share of
  `CavernPercent`. A cavern that would cross the budget is kept only if less
  than half of it lies past the line, so on average a chunk lands on its share.
  Each cavern is the same cluster of small ellipses as before, clipped to
  `cavernReach` and the edge margin. Clipping an ellipse's rows to a box keeps
  them contiguous, so the cavern stays connected. The ellipse test is integer
  arithmetic, so it rounds the same on every CPU (see
  [determinism.md](./determinism.md#golden-hashes-and-other-machines)).
- **Landing site.** A candidate that touches the landing box (the landing
  cavern's bounding box plus `cavernLandingClearance`) is re-rolled. A chunk
  gives up after `cavernSiteFailures` (16) re-rolls. The landing box is a
  function of the config alone (`caveRadii`), so it is a pure exclusion.
- **Crowding.** Each candidate gets a random `key`. A candidate is **dropped** if
  any of its tiles is within `cavernSpacing` of a tile of any higher-ranked
  *candidate*. The rule looks at candidates, not survivors, so it cannot chain:
  whether a cavern is kept depends only on candidates within one chunk.
- **Passages.** Each kept cavern finds its nearest kept cavern within
  `passageMaxSpan`, with ties going to the higher-ranked one. Each such pair
  is rolled once against `CavernPassagePercent`, from a stream seeded by the
  pair. The random walk is the old one, plus a box: it may not stray more than
  `passageSlack` outside the rectangle spanned by the two centers.

### When chunks are generated

The invariant: **every chunk within `WorldgenHalo` (`worldgen-halo`, default
2) of a chunk holding a tile the colony has seen is generated, and no other
chunk is.** Three hooks keep it:

- `setTerrain`, `carveHidden` and `reveal` each call `generateChunkAt(p)`
  before touching `p`. With the invariant in place this is one bool check and
  never generates anything. It is a backstop, so that no write can ever reach
  a chunk before its content.
- The first time `reveal` marks a tile in a chunk as explored
  (`World.genSeen`), it calls `generateAround`, which generates every chunk
  within the halo.
- `generate` does nothing special. Carving the landing site goes through
  `SetTerrain`, which generates the chunks under it and reveals its rim, and
  that generates the halo.

Two mechanisms, with different jobs:

- **The backstop keeps writes and reveals correct.** `reveal` generates a
  tile's chunk before it reads the tile, and `setTerrain` before it writes one.
  So the breach flood is complete however far it runs: a cave system that
  runs into ungenerated ground has that ground generated as the flood reaches
  it (`TestGenerationStaysAheadOfExploration` insists on a run where this
  happens). Mining is covered the same way. This would hold with no halo at
  all.
- **The halo keeps plain reads correct.** Reads never generate (see below),
  so anything the simulation *reads* without revealing has to be generated
  already. The colony reads only explored tiles and their neighbours, which a
  halo of 1 covers. `randomTile` spawn sites and dormant aliens read the
  generated chunks by construction. The halo's real reach, then, is how far
  from the colony those can be: where aliens start, and how much cave they
  have to move in.

More specifically:

- **Nests.** A cavern's center is registered in `unfoundCaverns` when its
  owner chunk is generated. A flood can only reach that center after the
  chunk exists, so no nest roll is missed.
- **Dormant aliens.** They stay in generated chunks. A cave that continues
  into ungenerated ground reads as Rock there until the colony's exploration
  generates it. That is a difference from generating everything up front, but
  it is deterministic.

`applyChunk` panics if a chunk's tile page already exists when it is
generated, because that would mean something wrote there first.
`TestGenerationStaysAheadOfExploration` checks the invariant after every tick
of a run that generates chunks during play and breaks into caves.

**Reading never generates.** `TerrainAt`, `TileAt`, `Walkable`, snapshots and
the golden hash read an ungenerated chunk as unexplored Rock
(`TestReadingNeverGenerates`). Generation is triggered only by exploration,
which is simulation state, so the set of generated chunks at any tick is the
same on every machine, whatever a frontend is looking at. The golden hash
includes that set.

**Sampling.** `randomTile` (`randomFloor`, `alienSpawnSite`) picks a uniform
generated chunk (from `genChunks`, kept sorted by row and column so it
depends only on which chunks exist), then a uniform tile in it. A pick past
the map's edge counts as a miss, which keeps the draw uniform over tiles.
Every predicate these callers use needs a tile that is not Rock, so leaving
out ungenerated chunks changes nothing about which tiles can be picked. What
it avoids is thousands of guesses, and then a scan of the whole map, on
chunks that do not exist.

The one gameplay change: **aliens start only in generated caves**, meaning
within the halo of the landing site, where the colony will meet them. They
used to be spread over the whole map. Director spawns likewise land in caves
the generated frontier has reached.

### Previewing

With fog of war **on**, an ungenerated chunk is all unexplored, so it reads
as unexplored Rock and is drawn as fog, which is what it always was.

With fog **off**, a frontend draws the whole map, including chunks the colony
has not come near. `Snapshot.TileAt` and `TerrainAt` read those from a
`ChunkPreview`: a second `worldGen`, owned by the engine's snapshots and
never by the World, that computes a chunk's content without generating it.
Content is a pure function of the config and coordinates, so the preview is
exactly what the chunk will hold when exploration generates it.
`TestPreviewMatchesGeneration` checks that.

The preview is safe for concurrent use, because every frontend reads it
through the snapshots on its own goroutine. It caches 256 to 511 chunk
contents (`previewCacheChunks`, kept in two generations). A warm 300×90
viewport costs about 0.5 ms a frame on a 10000×10000 map, and panning into
new ground about 1.6 ms. A whole-map view such as a minimap should walk the
map chunk by chunk: scanning row by row across more than 256 chunks would
preview each chunk once per row. The preview exists only when fog is off,
since nothing reads it otherwise. `TestPreviewNeverAffectsTheSimulation` plays the same
seed with and without a frontend reading all over the map and demands the
same golden hash.

The TUI's lore tab shows `Generated: n/m chunks` (`Stats.ChunksGenerated`
out of `Stats.Chunks`), so a player can watch the world grow.

### Writing a chunk into the world

`applyChunk` writes composition and hidden floor straight into `World.tiles`.
It does not go through `SetTerrain` or `carveHidden`, so no `TileChanged`
fires, and it keeps `terrainCounts`, `hiddenFloor`, the dirty published pages
and the dirty region chunks in step itself. Nothing colony-facing can see
undiscovered floor (see [caverns.md](./caverns.md)), so the job board and flow
fields have nothing to learn from it. `TestChunkApplyNeedsNoTileEvents` plays
the same game with and without the events, through a breach, and demands
identical hashes.

### Abundance: expected, not exact

The old generator hit each percentage exactly, because it counted tiles across
the whole map. A chunk can only aim at its own share, so abundance is now an
expected value. Here is how far it drifts, over many seeds at the default
config (`MARS_DRIFT_REPORT=1 go test ./internal/sim -run
TestAbundanceDriftReport -v`):

| map | seeds | feature | target % | mean % | mean error | sd | min | max |
| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 80x40 | 500 | iron | 10 | 10.02 | +0.2% | 0.72 | 7.94 | 12.19 |
| 80x40 | 500 | ice | 5 | 5.01 | +0.2% | 0.48 | 3.44 | 6.31 |
| 80x40 | 500 | uranium | 1 | 1.01 | +0.8% | 0.41 | 0.12 | 2.00 |
| 80x40 | 500 | clay | 5 | 4.97 | -0.5% | 0.49 | 3.66 | 6.31 |
| 80x40 | 500 | cavern | 4 | 3.91 | -2.2% | 1.15 | 0.97 | 7.22 |
| 80x40 | 500 | scum | 6 | 6.00 | -0.1% | 0.02 | 5.88 | 6.03 |
| 80x40 | 500 | salt | 3 | 3.00 | +0.0% | 0.02 | 2.94 | 3.03 |
| 256x256 | 500 | iron | 10 | 9.97 | -0.3% | 0.26 | 9.29 | 10.54 |
| 256x256 | 500 | ice | 5 | 4.98 | -0.5% | 0.15 | 4.62 | 5.42 |
| 256x256 | 500 | uranium | 1 | 1.01 | +1.3% | 0.10 | 0.73 | 1.26 |
| 256x256 | 500 | clay | 5 | 4.98 | -0.4% | 0.14 | 4.60 | 5.39 |
| 256x256 | 500 | cavern | 4 | 4.11 | +2.7% | 0.33 | 3.09 | 4.99 |
| 256x256 | 500 | scum | 6 | 6.00 | -0.1% | 0.01 | 5.98 | 6.00 |
| 256x256 | 500 | salt | 3 | 3.00 | -0.0% | 0.00 | 2.98 | 3.00 |
| 1024x1024 | 100 | iron | 10 | 10.00 | -0.0% | 0.11 | 9.79 | 10.14 |
| 1024x1024 | 100 | ice | 5 | 4.97 | -0.6% | 0.04 | 4.88 | 5.02 |
| 1024x1024 | 100 | uranium | 1 | 1.00 | +0.1% | 0.04 | 0.95 | 1.07 |
| 1024x1024 | 100 | clay | 5 | 4.95 | -0.9% | 0.03 | 4.89 | 4.99 |
| 1024x1024 | 100 | cavern | 4 | 4.08 | +2.0% | 0.14 | 3.78 | 4.40 |
| 1024x1024 | 100 | scum | 6 | 5.99 | -0.1% | 0.00 | 5.99 | 6.00 |
| 1024x1024 | 100 | salt | 3 | 3.00 | -0.0% | 0.00 | 3.00 | 3.00 |
| 4096x4096 | 10 | iron | 10 | 9.95 | -0.5% | 0.00 | 9.95 | 9.96 |
| 4096x4096 | 10 | ice | 5 | 4.94 | -1.2% | 0.00 | 4.94 | 4.94 |
| 4096x4096 | 10 | uranium | 1 | 0.97 | -3.0% | 0.00 | 0.97 | 0.97 |
| 4096x4096 | 10 | clay | 5 | 4.94 | -1.2% | 0.00 | 4.94 | 4.94 |
| 4096x4096 | 10 | cavern | 4 | 4.16 | +3.9% | 0.02 | 4.12 | 4.18 |
| 4096x4096 | 10 | scum | 6 | 5.99 | -0.1% | 0.00 | 5.99 | 5.99 |
| 4096x4096 | 10 | salt | 3 | 3.00 | -0.0% | 0.00 | 3.00 | 3.00 |

Cavern floor now includes passages, which the old target did not count. The
cavern's crowding drops and its passages roughly cancel at the default
config. `TestAbundanceDriftWithinTolerance` is the gate: on 256×256 over 40
seeds, every mean is within 5% of its target, and on the default 80×40 map
over 200 seeds, every feature appears on every seed.

### Startup cost

`BenchmarkStartup*` measures `NewEngine` plus the first published frame, and
the live heap afterwards. Measured on an M-series Mac, seed 7, default config:

| Map | Whole-map generator | Chunked, all up front | Chunked, lazy |
| --- | ---: | ---: | ---: |
| 1000×1000 | 70 ms, 11 MB | 58 ms, 11 MB | 26 ms, 6 MB |
| 2500×2500 | 364 ms, 65 MB | 300 ms, 65 MB | 23 ms, 7 MB |
| 10000×10000 | 6.43 s, 1032 MB | 4.95 s, 1028 MB | 27 ms, 28 MB |
| 10000×10000, js/wasm in Node | 37.7 s ([PR 60](https://github.com/KevinMcHugh/mars-sim/pull/60)) | | 118 ms, 28 MB |

The first two columns were measured before the economy (crash pods, scum)
landed. With it, the whole-map generator took 11.4 s and 1353 MB at
10000×10000 (34.3 s under wasm). The lazy column includes the economy: most
of its startup time is landing the crash pods.

Generating every chunk up front was only a little faster than the old
generator. Most of what was left was writing 100M tiles for the first time
(page faults and zeroing) and publishing them, and only laziness removes that.
What remains at 10000×10000 is mostly the map-sized tables that are not tile
data: the page tables, and `chunkEntities` for the entity index.

**During play**, a tick that generates chunks costs 2 to 12 ms on a 4000×4000
map with 60 colonists, depending on how many plans the cache already holds.
That is below the ordinary p99 tick on the same run (8 to 11 ms), and it
happened on 2 of 3000 ticks. `BenchmarkChunkCold` / `BenchmarkChunkWarm` are
the building blocks: 5.2 ms / 28 µs natively, and 17 ms / 0.7 ms under wasm.

## Why it is this way

### The old generator depended on the order it ran in

`growRockVeins` and `generateCaverns` could not produce one chunk without
first producing everything generated before it:

- **One stream for the whole map.** Vein #50,000 was placed with whatever the
  composition stream had left after veins #1 to #49,999, and how many draws
  each of those took depended on its size and branches.
- **Draws depended on earlier writes.** `ordinaryRockSeed` probed forward from
  a random tile to the next plain rock, sometimes into another chunk.
  `ordinaryNeighbors` fed the count of free neighbours into `IntN`, so a
  neighbour taken earlier changed the value drawn. `cavernTileOK` rejected a
  cavern by reading the live terrain of caverns carved earlier.
- **Map-wide stopping conditions.** Veins grew until the map-wide tile count
  was met. Caverns stopped after 50 failed sites in a row, anywhere.
- **Visit order in passages.** `joinCaverns` walked caverns in creation order,
  and ties went to the lower index.

So changing one tile early on moved everything after it, and a chunk's
content was a function of the whole run. Running the old generator and keeping
only one chunk's tiles would have cost the whole run anyway.

### Why a stored "resume this cavern" note is not enough

An alternative to planning the neighbours: when chunk A generates a cavern
that crosses into B, it records a note for B. That makes B depend on whether A
was generated first. If the colony digs toward B first, B is generated as
plain rock and handed to the simulation, and A can never carve into it
later. The world would depend on which way the colony dug. The fix for that
is to ask A before generating B, which means planning A. So a note is only
ever a cache of a plan. That is what `genCache` is, and why it can be
forgotten.

### Why crowding looks at candidates, not survivors

"Dropped if too close to a higher-ranked *kept* cavern" is the obvious rule,
and it chains. Whether B is kept depends on whether C is, which depends on D,
and so on across the map with no bound. Looking at candidates means a cavern
dropped for crowding a neighbour that was itself dropped stays dropped. That
lowers density slightly, and the budget and passages make up for it (see the
table).

### Random keys, not chunk order, for ranking

Ranking caverns by chunk coordinate would make the top-left chunk of every
pair win every conflict, which biases density toward one side of each chunk
edge. A random key per cavern spreads it evenly.

### Pointer identity broke order independence

The first version compared caverns with `==`. A plan evicted from the cache
and recomputed is a new object, so a cavern held from before the eviction
never equalled itself, and `nearestCavern` could pick a cavern as its own
nearest neighbour. Only the tiny-cache run of the order test caught it.
Caverns are compared with `is` (owner chunk and index) everywhere.

### Stratified counts, not Poisson

A Poisson draw with mean 2 rolls zero about 13% of the time. Uranium at 1% on
80×40 is about two veins, so a Poisson count would leave about one small map
in eight with no uranium, and with it no mutation hazard. `stratified` never
rolls zero where one or more is expected.

### Veins avoid their own chunk's veins

The first version let same-level veins overlap freely. Iron came out 4.4%
under target at every map size. Overlaps between veins of the same chunk were
most of that, so each vein now steers around the ones its chunk planned
before it, which brought iron to within 0.5%.

### Why generation is triggered by exploration, not by the camera

The first sketch ([PR 60](https://github.com/KevinMcHugh/mars-sim/pull/60))
generated a chunk when "a dig reaches its border, pathfinding asks about it,
or the camera looks at it". The camera cannot be allowed to do this. Cameras
differ between machines, so the set of generated chunks would differ too, and
with it `randomTile`'s candidates, `hiddenFloor`, `unfoundCaverns`, and the
dormant aliens drawing from `World.rng`. Pathfinding cannot be a trigger
either: it would make generation depend on which searches happened to run.
Exploration is one bit per tile that only ever goes from false to true, set
in one place (`reveal`). It is already simulation state and already
deterministic. A frontend that wants to see ungenerated ground previews it
with its own generator instead (see [Previewing](#previewing)).

### Why a halo and not "generate on first touch"

Generating a chunk when it is first written or revealed is what
`generateChunkAt` does, and it is enough for mining and for the breach flood.
It is not enough for reads: `randomTile`, dormant aliens, and the colony's
own neighbour checks read tiles nobody is writing. Making reads generate
would be a trigger a frontend could pull too. The halo makes sure that what
the simulation reads without revealing is already there, so no read ever has
to generate. It is also what gives aliens room to start away from the
landing site.

## Extending it

- **A new composition** goes at the end of `veinLevels`, so no existing vein
  moves. Add its percentage to `veinPercent`.
- **A new feature** needs an owner, a reach that fits in one chunk (add a
  compile-time assertion next to the others), and its own stream constant.
  Its plan may read earlier plans but must never read terrain.
- **A feature bigger than a chunk** should not raise the reaches. Plan it at a
  coarser level (a region of several chunks), and let each chunk draw its own
  piece from the region plan.
- **Anything that reads tiles far from the colony** (a new spawn rule, a
  scan) must either stay within generated chunks (`forGeneratedTiles`,
  `randomTile`) or accept that ungenerated chunks read as Rock. Never make it
  generate: that is a trigger outside exploration.
- **A new writer of `World.tiles`** must call `generateChunkAt` first, or
  `applyChunk`'s panic will catch it the first time it runs ahead of the
  halo.
- **Any change to plans changes every seed.** Re-pin `goldenCases`, run
  `tools/determinism-check.sh`, and rerun the drift report.

## Related

- [world.md](./world.md): the tile grid and the rest of `generate`.
- [caverns.md](./caverns.md): what happens to caverns after generation.
- [determinism.md](./determinism.md): the golden hashes and the cross-platform
  check.
- [rng-streams.md](./rng-streams.md): every RNG stream in the sim.
- [sparse-grids.md](./sparse-grids.md): the paged grids a chunk lines up with.

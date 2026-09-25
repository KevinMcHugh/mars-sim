# Natural caverns

> Part of the [mars-sim documentation](./README.md).

## What it is

World generation hollows natural caverns out of the rock away from the landing
site, and sometimes joins one to its nearest neighbor with a winding one-tile
passage. They start hidden under the fog and are invisible to the colony's
systems. When a dig breaks into one, the whole connected cave system is revealed
at once and becomes part of the colony: its walls become mining frontier,
its floor becomes room for construction, and the log announces the find.

A few caverns (`cavern-nest-percent`, default 10%) hold an **alien nest**: a
handful of aliens of one species that lie dormant in the dark until the colony
breaks in, then wake up and behave like any other alien of their species.

## Source

- [`internal/sim/caverns.go`](../internal/sim/caverns.go) — `generateCaverns`, `planCavern`, `joinCaverns`, `planPassage`, and the placement constants.
- [`internal/sim/world.go`](../internal/sim/world.go) — `carveHidden`, `setTerrain`'s `discover` switch, the breach flood in `revealAround` / `reveal` / `discoverCavernTile`, `discovered`, and `hiddenFloor`.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go) — where `generate` calls it, and `randomFloor` / `freeFloorTilesIn` skipping undiscovered floor.
- [`internal/sim/rooms.go`](../internal/sim/rooms.go) — `region.discovered`, incremental `relabelRooms`, and `mainRoom` chosen from discovered rooms only.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `bordersFloor`, which only counts discovered floor, so cave walls are not frontier.
- [`internal/sim/project.go`](../internal/sim/project.go) — `roomSiteClear` refusing undiscovered floor.
- [`internal/sim/caverns.go`](../internal/sim/caverns.go) also holds the nests: `alienNest`, `seedAlienNests`, `dormant`, `nestTurn`, and `rouse`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `alienTurn`'s dormant branch, and `nearestAlien` / `observeNearby` skipping dormant aliens.
- [`internal/sim/world.go`](../internal/sim/world.go) — `spawnAs`, `spawn` with the species already chosen.
- [`internal/sim/config.go`](../internal/sim/config.go) — `cavern-percent`, `cavern-min`, `cavern-max`, `cavern-passage-percent`, `cavern-nest-percent`, `cavern-nest-min`, `cavern-nest-max`.
- [`internal/sim/caverns_test.go`](../internal/sim/caverns_test.go) — hidden at generation, the breach, passages, determinism, room labels under digging, and nests (placement, seed stability, dormancy and waking).
- [`internal/sim/rooms_test.go`](../internal/sim/rooms_test.go) — `checkRoomLabels`, the brute-force oracle for incremental relabeling.

## How it works

### Generation

`generate` carves the landing cavern first, then calls `generateCaverns` on a
dedicated seed-derived RNG stream:

1. Until about `CavernPercent` of the map is cavern floor, `planCavern` picks a
   random site and grows a cavern of `CavernMin`–`CavernMax` tiles there. Each
   cavern is a cluster of small overlapping ellipses, wider than they are tall
   to match the map. Each new lobe is centered on a tile already in the cavern,
   so the cavern is always one connected pocket. It overshoots the target size
   by at most one lobe.
2. A site is rejected if any tile falls within `cavernLandingClearance` (4) of
   the landing cavern's bounding box, within `cavernSpacing` (3) of any floor
   already carved, or on the map's outermost ring. After `cavernPlanFailures`
   rejections in a row, generation stops short of the target. That only
   happens on maps too small to fit it.
3. `joinCaverns` gives each cavern one roll of `CavernPassagePercent` for a
   passage to its nearest neighbor by center distance. A pair that are each
   other's nearest are rolled only once, and pairs further apart than
   `passageMaxSpan` are skipped. `planPassage` is a biased random walk in
   orthogonal steps: toward the goal 3/4 of the time, a random direction
   otherwise. A walk that strays near the landing site or runs too long is
   discarded, with up to `passageAttempts` tries.

Every tile goes down through `carveHidden`. This is `setTerrain` with
`discover=false`: counts, chunks and `TileChanged` all behave as usual, but no
fog lifts, the carved bounding box does not grow, and `hiddenFloor` goes up.

### The invariant: unexplored floor is undiscovered cavern

Every other terrain change goes through `SetTerrain`, which reveals the tile
*before* writing it (see [fog-of-war.md](./fog-of-war.md)). So a tile that is
not `Rock` and not `Explored` can only be undiscovered cavern floor. The rest
of the design depends on that one fact:

| System | What it does with undiscovered floor |
| --- | --- |
| `bordersFloor` / job board | Cave walls are not mining frontier, so nobody tries to mine rock they cannot reach. |
| `randomFloor`, `freeFloorTiles` | Colonists, cats, mice, arrivals, and mouse plagues are never placed in a cave. |
| `roomSiteClear` | No room is sited on cave floor or cave lanes. |
| `relabelRooms` → `mainRoom` | Only rooms with a discovered region can be the colony's main room (see [escape.md](./escape.md)). |
| `Stats.FloorDug`, `Stats.Rooms` | Undiscovered floor and rooms are left out. |

Because simulation code relies on the flag, `Explored` is now maintained even
with fog of war off. `World.Explored` still reports everything as explored for
frontends; simulation code uses `World.discovered`.

### Breaking in

Mining the last rock between the colony and a cave calls
`SetTerrain(rock, Floor)`. That calls `revealAround`, whose ring of `reveal`s
reaches a neighbor that is unexplored floor. `reveal` counts it off
`hiddenFloor` and pushes it on `caveStack`. `revealAround` then floods: for
each cave tile it runs `discoverCavernTile` and reveals that tile's own ring,
which queues any further cave floor. The result is that the whole connected
cave system, passages included, plus its rock rim, is revealed in the same
call. `discoverCavernTile` also:

- refreshes the job board's frontier around the tile, so the cave's walls
  become minable;
- grows the carved box, so room-site searches reach the cave;
- dirties the tile's chunk, so its region is re-flooded with `discovered` set.

The same tick, the breach tile's `TileChanged` marks the frontier flow field
stale, and `refreshSpatial` merges the cave into the colony's room.

### Alien nests

After everything else is placed, `generate` calls `seedAlienNests` with the
caverns `generateCaverns` returned. Each cavern gets one roll of
`CavernNestPercent`. A hit places `CavernNestMin`–`CavernNestMax` aliens, all
of one species, on distinct free tiles of that cavern (the tiles `planCavern`
carved, so never in a passage). Each member's `Entity.nest` points at its
entry in `World.nests`.

A nest alien is **dormant** while its tile is undiscovered (`World.dormant`).
A dormant alien:

- takes `nestTurn` instead of its species' behavior: an occasional step to
  neighboring floor, never through rock, so it stays in its cave system;
- is skipped by `nearestAlien` and by `observeNearby`, so colonists do not
  flee from, remember, or shoot at an alien behind rock they have never dug
  through.

When a breach floods the cave with discovery, every member stops being dormant
at once. On its next turn `rouse` clears `nest` and it becomes an ordinary
alien of its species: a Hostile nest hunts the colony, a Cautious one waits for
colonists to come close, a Friendly one wanders. The first member to wake logs
"The colony has broken into a nest of ... (n)!".


- **Hidden, not just unreached.** An early version could have left the caves
  visible. But with fog on, a breach would reveal a one-tile hole with fog
  beyond it, and every colony-facing system would still need a way to tell
  "floor we can use" from "floor in a cave nobody found". The explored flag
  already carried that information for the fog, so reusing it gave both at
  once.
- **Keeping `Explored` with fog off** changes an old promise: fog off used to
  mark nothing at all. The alternatives were a second per-tile bit (the tile
  record is pinned at three bytes, see [world.md](./world.md)) or a sparse set
  of cave tiles (millions of entries on a huge map). Marking only what the
  colony touches costs what fog-on always paid, and it keeps a huge map's
  untouched pages zero.
- **Reveal before writing** in `setTerrain` is what makes "unexplored and not
  Rock" exact. Revealing after the write would mark every freshly dug tile as
  cave floor for the moment it was revealed.
- **`mainRoom` from discovered rooms.** Previously `mainRoom` was simply the
  largest room. A cave larger than the landing site would have become the main
  room, and every colonist would have started counting ticks toward escaping a
  room they were already in (see [escape.md](./escape.md)).
- **Incremental relabeling came with this feature.** On a 2500×2500 map at 4%,
  caves add about 13,000 regions. The old `relabelRooms` walked every region
  whenever any chunk was dirty, so per-tick cost rose from ~0.6 ms to ~3.4 ms.
  Replacing the per-call visited map with a pass counter only reduced that to
  ~3 ms, because the walk over all regions was the real cost. Relabeling only
  the components touched by a change brought it back to baseline.
  `TestRoomsIncrementalMatchesBruteForce` and `TestRoomLabelsWithCaverns`
  check it against a from-scratch flood through `checkRoomLabels`.
- **`TileChanged` still fires for hidden carving.** Worldgen could write the
  tiles directly the way ore veins do. But terrain, unlike composition, feeds
  counts, chunks and facility sets, and duplicating `SetTerrain`'s bookkeeping
  is how derived state goes stale. The cost is about 0.4 s extra worldgen on
  an 8-million-tile map.
- **Own RNG stream.** Caves use `Seed ^ 0x13198A2E03707344`, so tuning them does
  not reshuffle the veins or the main simulation stream. Aliens still land on
  whatever rock is left, so turning caves on does change alien spawns for a
  seed.
- **Clearance from the landing box, not the ellipse.** A box test is O(1) per
  tile, while a neighborhood scan for landing floor would dominate worldgen on
  huge maps. The box is conservative, which is fine because caves are
  supposed to be a few tiles of digging away.
- **Nests sleep until found.** Aliens already hunt through rock, so a Hostile
  nest that was awake from tick one would burrow straight out and be just
  another starting alien. Dormancy is what makes a nest a place: danger you
  dig into, not danger that comes to you. Keying it off `discovered` (not a
  wake-up flag set by the breach) means the breach code did not have to learn
  about entities, and the whole cave system wakes together.
- **Dormant aliens are invisible to colonists.** Colonists sense aliens by
  distance, through rock. For a burrowing alien that is fair warning. For a
  nest it would make miners flee from a cave wall and never breach it.
- **Nests have their own RNG stream and are placed last.** The roll, size,
  species and tiles all come from `Seed ^ 0x0452821E638D0137`, and members go
  through `spawnAs` so the species is not drawn from `World.rng`. Placing them
  after the mice and cats keeps every other entity's ID. So a seed whose
  caverns roll no nest plays out exactly as before nests existed
  (`TestAlienNestsOffLeavesSeedUnchanged`). A nest's dormant wandering does
  draw from `World.rng`, as all gameplay does.
- **Tests opt in.** `testConfig` sets `CavernNestPercent = 0`, like
  `TraitChance`, because many tests set `StartAliens = 0` and expect no aliens.
  A test built on `DefaultConfig` that needs an alien-free world must turn
  nests off too (`TestColonyDoesNotStarveOverTime` found this the hard way: seed 2
  dug into a nest and was eaten).
- **Passages walk orthogonally.** Diagonal steps connect under 8-connectivity
  but look like a staircase of disconnected pockets on screen.

## Extending it

- **Nests:** tune them with `cavern-nest-percent`, `cavern-nest-min` and
  `cavern-nest-max`. A nest that should wake in other ways (noise, a colonist
  close by) belongs in `World.dormant`. Anything new that lets colonists sense
  or target aliens must skip dormant ones, as `nearestAlien` does.
- **Anything that places or targets floor** must decide whether undiscovered
  cave floor counts. Colony-facing code should check `w.discovered(p)`. Only
  `carveHidden` may create unexplored floor; any other terrain writer must go
  through `SetTerrain`, or `hiddenFloor` and the breach flood stop being true.
- **Different shapes:** change `planCavern`. Tune size with `cavern-min` /
  `cavern-max`, and passage behavior with the constants in `caverns.go`.

## Related

- [world.md](./world.md) — the generation order caverns slot into.
- [fog-of-war.md](./fog-of-war.md) — the explored flag the caverns hide behind.
- [pathfinding.md](./pathfinding.md) — regions, rooms, and incremental relabeling.
- [escape.md](./escape.md) — `mainRoom`, which undiscovered caves must not become.
- [construction.md](./construction.md) — room siting, which skips undiscovered floor.

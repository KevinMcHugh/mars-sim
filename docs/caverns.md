# Natural caverns

> Part of the [mars-sim documentation](./README.md).

## What it is

World generation hollows natural caverns out of the rock away from the landing
site, and sometimes joins one to its nearest neighbor with a winding one-tile
passage. They start hidden under the fog and are invisible to the colony's
systems. When a dig breaks into one, the whole connected cave system is revealed
at once and becomes part of the colony: its walls become mining frontier,
its floor becomes room for construction, and the log announces the find.

Aliens live in the caves too. They walk only on floor, so every alien that
spawns (at the start, or later from the director) is placed on hidden cave
floor when there is room, and lies dormant there until the colony breaks in.
When the colony breaks into a cave system, each cavern in it also rolls
`cavern-nest-percent` (default 10%) for an **alien nest**: a handful of aliens
of one species that appear right there, awake. Nests do not exist before that
moment.

## Source

- [`internal/sim/caverns.go`](../internal/sim/caverns.go) — `generateCaverns`, `planCavern`, `joinCaverns`, `planPassage`, and the placement constants.
- [`internal/sim/world.go`](../internal/sim/world.go) — `carveHidden`, `setTerrain`'s `discover` switch, the breach flood in `revealAround` / `reveal` / `discoverCavernTile`, `discovered`, and `hiddenFloor`.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go) — where `generate` calls it, and `randomFloor` / `freeFloorTilesIn` skipping undiscovered floor.
- [`internal/sim/rooms.go`](../internal/sim/rooms.go) — `region.discovered`, incremental `relabelRooms`, and `mainRoom` chosen from discovered rooms only.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `bordersFloor`, which only counts discovered floor, so cave walls are not frontier.
- [`internal/sim/project.go`](../internal/sim/project.go) — `roomSiteClear` refusing undiscovered floor.
- [`internal/sim/caverns.go`](../internal/sim/caverns.go) also holds the nests and dormancy: `trackCavernsForNests`, `rollNests`, `spawnNest`, `dormant`, and `dormantTurn`.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go) — `alienSpawnSite`, where every non-nest alien is placed.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `alienTurn`'s dormant branch, and `nearestAlien` / `observeNearby` skipping dormant aliens.
- [`internal/sim/world.go`](../internal/sim/world.go) — `spawnAs`, `spawn` with the species already chosen; `revealAround` collecting found cavern centers for `rollNests`.
- [`internal/sim/config.go`](../internal/sim/config.go) — `cavern-percent`, `cavern-min`, `cavern-max`, `cavern-passage-percent`, `cavern-nest-percent`, `cavern-nest-min`, `cavern-nest-max`.
- [`internal/sim/caverns_test.go`](../internal/sim/caverns_test.go) — hidden at generation, the breach, passages, determinism, room labels under digging, nests spawning at the breach, nests leaving generation alone, and cave aliens staying dormant until found.
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
   `passageMaxSpan` are skipped. Centers are bucketed into
   `passageMaxSpan`-sized cells so each cavern only looks at the 3×3 cells
   around it. Comparing every pair was quadratic, and with 150,000 caverns
   (a 10000×5000 map at 15%) it was most of worldgen. `planPassage` is a biased random walk in
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

### Aliens in the caves

Aliens walk only on floor (see [entities-and-ai.md](./entities-and-ai.md)).
`alienSpawnSite` places every starting alien, director swarm and trickle
spawn:

1. On random free floor in an undiscovered cavern, if any is left.
2. Otherwise on free discovered floor at least `minDist` from the landing
   site, and failing that on the free discovered floor farthest from it. This
   is the fallback for maps with no caves (`cavern-percent 0`, or a map too
   small to fit one). There the aliens start inside the colony and hunt right
   away.

An alien is **dormant** while its tile is undiscovered (`World.dormant`).
Undiscovered floor is always a sealed cave (the invariant above), and aliens
cannot leave the floor, so a dormant alien could not reach the colony anyway.
A dormant alien:

- takes `dormantTurn` instead of its species' behavior: an occasional step to
  neighboring floor, so it stays in its cave system;
- is skipped by `nearestAlien` and by `observeNearby`, so colonists do not
  flee from, remember, or shoot at an alien behind rock they have never dug
  through.

When a breach floods a cave with discovery, every alien in it stops being
dormant at once and acts like any alien of its species: a Hostile one hunts the
colony, a Cautious one waits for colonists to come close, a Friendly one
wanders.

### Alien nests

`generate` keeps nothing about nests except each cavern's center, in
`World.unfoundCaverns` (`trackCavernsForNests`). When `revealAround` floods a
breach, it notes every center it discovers. Once the whole system is revealed,
`rollNests` gives each of those caverns its single `CavernNestPercent` roll, in
discovery order. On a hit, `spawnNest` places `CavernNestMin`–`CavernNestMax`
aliens of one species on free discovered floor within `nestRadius` (4) of the
center and logs "The colony has broken into a nest of ... (n)!". They are on
discovered floor, so they are awake from the start and act like any alien of
their species.

## Why it is this way

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
  not reshuffle the veins or the main simulation stream. Aliens spawn on cave
  floor, so turning caves on or retuning them does change alien spawns for a
  seed.
- **Clearance from the landing box, not the ellipse.** A box test is O(1) per
  tile, while a neighborhood scan for landing floor would dominate worldgen on
  huge maps. The box is conservative, which is fine because caves are
  supposed to be a few tiles of digging away.
- **Caves, not rock, once aliens stopped burrowing.** Aliens used to spawn in
  rock and burrow in. Once they walk only on floor, rock would trap them, so
  they needed floor to start on. Hidden caves were chosen over colony floor:
  aliens become danger you dig into rather than danger that comes to you. The
  colony-floor fallback keeps cave-less maps from being alien-free.
- **Dormancy is keyed off `discovered`,** not a wake-up flag set by the
  breach. The breach code did not have to learn about entities, and a whole
  cave system wakes together. It also saves a sealed alien from running an A\* search every turn toward a colonist
  it can never reach.
- **Dormant aliens are invisible to colonists.** Colonists sense aliens by
  distance, and that distance ignores rock. For an alien in a sealed cave that
  would make miners flee from a cave wall and never breach it.
- **Nests are rolled at the breach, not at worldgen.** The first version
  placed every nest during generation. On a 10000×5000 map at 15% caves that
  was about 40,000 dormant aliens. Worldgen slowed down, every tick paid for
  their turns (50 ticks: 250 ms, versus 3 ms without them), and nobody would
  ever meet most of them. Now a nest costs nothing until its cavern is found,
  and all that is kept per cavern is its center (one map entry).
- **Nests have their own RNG stream.** The roll, size, species and tiles all
  come from `World.nestRNG` (`Seed ^ 0x0452821E638D0137`), and members go
  through `spawnAs` so the species is not drawn from `World.rng`. Nests never
  touch generation (`TestAlienNestsDoNotChangeGeneration`). Breaches happen in
  a deterministic order, so the nest rolls stay deterministic for a seed.
- **Tests opt in.** `testConfig` sets `CavernNestPercent = 0`, like
  `TraitChance`, because many tests set `StartAliens = 0` and expect no aliens.
  A test built on `DefaultConfig` that needs an alien-free world must turn
  nests off too. `TestColonyDoesNotStarveOverTime` found this the hard way,
  when seed 2 dug into a nest and was eaten.
- **Passages walk orthogonally.** Diagonal steps connect under 8-connectivity
  but look like a staircase of disconnected pockets on screen.

## Extending it

- **Nests:** tune them with `cavern-nest-percent`, `cavern-nest-min` and
  `cavern-nest-max`. Anything else that should appear only when a cave is
  found belongs next to `rollNests` in `revealAround`, not in worldgen.
  Aliens that should wake in other ways (noise, a colonist close by) belong in
  `World.dormant`. Anything new that lets colonists sense
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

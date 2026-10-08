# Depth

> Part of the [mars-sim documentation](./README.md).

## What it is

Depth is what the levels are for: the deeper the colony digs, the richer the
rock and the worse what lives in it. Every level below the landing level
generates more ore and much more uranium, has bigger caverns, rolls nests
more often and bigger, and weights those nests toward hostile species.
Caverns the colony breaks into can hold a natural shaft down into a cavern
below, or a sinkhole, so the deep sometimes comes up to meet the colony. And
a hostile alien with nothing it can walk to drops down a hole onto prey
below. This is phase Z4 of [z-levels.md](./z-levels.md); the links it uses
are [stairs.md](./stairs.md), [shafts.md](./shafts.md) and
[holes.md](./holes.md).

## Source

- [`internal/sim/depth.go`](../internal/sim/depth.go): `depthOf`,
  `depthConfig`, `nestSpecies`; natural features (`naturalFeatures`,
  `cavernFloor`, `below`, `peekGen`, `naturalShaft`, `sinkhole`,
  `rollNestsAndFeatures`); the raid (`raid`, `raidHole`, `colonistIn`).
- [`internal/sim/worldgen_chunks.go`](../internal/sim/worldgen_chunks.go):
  each level's generator takes `depthConfig` for its level.
- [`internal/sim/caverns.go`](../internal/sim/caverns.go): `rollNests` and
  `spawnNest` use the cavern's level's settings and `nestSpecies`.
- [`internal/sim/species.go`](../internal/sim/species.go): the hostile
  ladder's `raid` rung, after the hunt.
- [`internal/sim/depth_test.go`](../internal/sim/depth_test.go): the scaled
  settings, a deep level generating richer than the landing level, deep
  nests leaning hostile, a natural shaft joining two caverns, a sinkhole and
  no shaft over rock, a hostile alien raiding down a hole, and a colony
  digging down with features on, deterministic and surviving a save.

## How it works

### Richer and more dangerous, a level at a time

`depthConfig(cfg, l)` is the config as level `l` sees it: the landing level
gets `cfg` itself; each level below it (`depthOf`) scales:

| Setting | Per level down |
| --- | --- |
| iron, ice and clay vein percent | `depth-ore-percent` more (25) |
| uranium vein percent | `depth-uranium-percent` more (100: double one level down, triple two down) |
| cavern size (min and max) | `depth-cavern-percent` bigger (20) |
| nest chance | `depth-nest-percent` points added (10) |
| nest size (min and max) | `depth-nest-size` aliens added (1) |

Percentages are capped at 100. A level's generator is built from its own
`depthConfig` (`newWorldGenLanding`), so every vein, cavern, passage and
preview on that level is the scaled one; `rollNests` and `spawnNest` read
the cavern's level's settings. Uranium is the reward with the sharpest
curve, and also the one that hurts (see [mutation.md](./mutation.md)).

### Hostility by depth

`nestSpecies(l)` draws a nest's species from `nestRNG`. On the landing level
it is the plain draw it always was (`IntN` over the roster). Below it each
hostile species weighs `depth-hostility` percent more a level (100: twice as
likely one level down, three times two down), so the deep is where the
hunters live. The roster itself is the seed's; depth changes which species
turn up, not which exist.

### Natural shafts and sinkholes

When the colony breaks into a cavern, after its nests roll
(`rollNestsAndFeatures`), `naturalFeatures` rolls two things for it, each
from the cavern's level's generator (`featureRand` keyed by the cavern's
center, so a pure function of the seed and the cavern, drawing from no
stream):

- **A natural shaft** (`natural-shaft-percent`, 20): if part of the cavern
  lies straight over a cavern on the level below, a one-level shaft joins
  them at one such tile. Breaking into the lower cavern reveals it and rolls
  its nests, and its aliens can climb up.
- **A sinkhole** (`sinkhole-percent`, 10): a hole in the cavern floor onto
  the level below, whose tile opens to floor.

Each sits on discovered cavern floor whose eight neighbours are floor too
(`cavernFloor`), so it never plugs a passage. What is below comes from
`below`: the real tile once that part of the level has been generated, or a
preview of the lower level's generator (`peekGen`) before anyone has broken
into it. Neither is rolled where the level below is past `deepest-level`.

### Raids

A hostile alien's ladder is dormant, hunt, **raid**, wander. With no prey it
can walk to (the hunt declines), the raid looks for an open hole on its
level, beside its room, that lands in a room with a living colonist
(`raidHole`), walks to it, and drops in (`leap`). It takes the fall's damage
like anyone. A hole is a door that only opens inward: dig one under an alien
cave and the cave comes down to you.

## Why it is this way

- **One scaled config, not depth checks everywhere.** Worldgen reads its
  config in dozens of places. Handing each level's generator a scaled copy
  changes all of them at once, and keeps the landing level's generator
  byte-for-byte what it was: the golden hashes did not move.
- **The landing level's nest draw is untouched.** A weighted draw consumes
  the stream differently from `IntN(len)`, so it is used only below the
  landing level; a game that never digs down rolls exactly the nests it
  always did.
- **Features from `featureRand`, not `nestRNG`.** Rolling them on the nest
  stream would shift every later nest in any game with `deepest-level` above
  1. Keyed by the cavern, they depend on nothing but the seed and which
  cavern it is.
- **Natural shafts only between caverns.** A shaft into solid rock leads
  nowhere and joins nothing; one between two caverns is what makes the deep
  come up to the colony, which is the point.
- **The roster is not rolled per level.** The plan said "species rolled per
  level". The roster is rolled from the seed and now feeds species packs,
  sprites and the lore tab; a per-level roster would ripple through all of
  them. Weighting the nest draw by depth gets the gameplay (worse things
  deeper) without that.
- **Raids are holes only.** Aliens already hunt across stairs and shafts:
  rooms span levels, so a nest joined to the colony by any two-way link is
  hunting it. Holes were the one way in that nothing used.

## Extending it

- A new depth lever is a `Depth*` setting and a line in `depthConfig`, if
  the thing it scales already reads `Config` on that level's generator or
  nest roll.
- A new natural feature is a stream constant and a roll in
  `naturalFeatures`; keep it off any RNG stream.
- Supply chutes down to a deep work camp, the other half of the plan's
  chutes, would start from `fallTarget` and the hauling planner.

## Related

- [z-levels.md](./z-levels.md): the plan; this is phase Z4.
- [caverns.md](./caverns.md): caverns, breaking in, and nests.
- [lore.md](./lore.md): species and temperament.
- [worldgen-chunks.md](./worldgen-chunks.md): the generator and
  `featureRand`.
- [holes.md](./holes.md), [shafts.md](./shafts.md),
  [stairs.md](./stairs.md): the links.

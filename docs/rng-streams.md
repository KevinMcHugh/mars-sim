# RNG streams

> Part of the [mars-sim documentation](./README.md).

## What it is

Every random draw in the sim comes from a `math/rand/v2` PCG generator derived
from `Config.Seed`. There are several separate streams, so one system's draws
never shift another's. The four that live past world generation keep their
PCG source next to them, which lets their state be saved and restored. That is
the RNG half of save/load.

## Source

- `internal/sim/rng.go`: `newPCG` / `newRand` (seeding), `rngSources`,
  `rngState`, and `World.saveRNG` / `World.loadRNG`.
- `internal/sim/world.go`: `newWorld` builds `rng`, `prng` and `agePRNG`.
- `internal/sim/caverns.go`: `trackCavernsForNests` builds `nestRNG`.
- `internal/sim/worldgen_chunks.go`: `featureRand`, the per-chunk worldgen
  streams.
- `internal/sim/rng_test.go`: the save/load round trip.

## How it works

| Stream | Seed | Lifetime | Saved |
| --- | --- | --- | --- |
| `World.rng` (simulation) | `Seed` | whole game | yes |
| `World.prng` (personality) | `Seed ^ 0x5DEECE66D` | whole game | yes |
| `World.agePRNG` (ages) | `Seed ^ 0x6A09E667` | whole game | yes |
| `World.nestRNG` (alien nests) | `Seed ^ 0x0452821E638D0137` | whole game | yes |
| worldgen veins, per chunk and level | `featureRand(0x243F6A8885A308D3 + level, cx, cy)` | one plan | no |
| worldgen cave scum, per chunk | `featureRand(0x5CA1AB1E, cx, cy)` | one plan | no |
| worldgen caverns, per chunk | `featureRand(0x13198A2E03707344, cx, cy)` | one plan | no |
| worldgen passages, per cavern pair | `featureRand(0xA4093822299F31D0, both caverns)` | one plan | no |
| alien lore roster | `Seed ^ alienLoreSeed` | `newWorld` only | no |

Why the streams are split is covered per stream in
[personality.md](./personality.md), [caverns.md](./caverns.md) and
[lore.md](./lore.md).

**Seeding.** `newPCG(seed)` feeds the int64 through splitmix64 twice to fill
PCG's 128-bit state. Seeds that are next to each other, or differ only by one
of the XOR constants above, therefore start from unrelated states.

**Saving.** A v2 `*rand.Rand` has no state of its own and does not expose its
source. So `World.rngSrc` keeps the `*rand.PCG` behind each saved stream.
`saveRNG` returns each source's `MarshalBinary` bytes in an `rngState`.
`loadRNG` unmarshals them back into the same sources, and that rewinds the
`*rand.Rand` wrappers too. The save file will embed `rngState`. The worldgen-only
streams are used up before tick 0, so a load never needs them.

`newWorld` takes the simulation stream's `*rand.PCG` (not a `*rand.Rand`), so
the world always owns the source it would need to save. Tests that never draw
from `w.rng` pass `nil`.

## Why it is this way

- **v1 could not be saved.** `math/rand`'s `NewSource` sources keep their state
  private. The only way to "restore" one was to reseed it and replay every
  draw, which means counting draws across the whole game. PCG's
  `MarshalBinary` makes it 20 bytes per stream.
- **The switch changed every seed's world.** v2 has different generators and a
  different algorithm for `IntN`, so no v1 seed reproduces its old world.
  It was made in one go, before the browser frontend
  ([browser-frontend.md](./browser-frontend.md), section 6) gives anyone a
  seed worth sharing. Any seed quoted in an older issue or doc describes a
  different world now.
- **Tests that depended on a lucky seed broke with it.** Two did, and in both
  the fix was to find what the seed was covering for, not to hunt for a new
  lucky seed:
  - `TestUrgentColonistHelpsBuildWhenFacilityUndersupplied` left the random
    cats and mice on. About 1 seed in 18 put one on the wall task, which made
    it unclaimable. The test now spawns none.
  - `TestRockDepositsAreVeinsRatherThanIsolatedTiles` caught a real worldgen
    bug: a vein whose first tile had no free ordinary-rock neighbor stopped
    at one tile. That happened on about 6% of seeds under both v1 and v2, and
    the test was pinned to a seed that avoided it. Chunked generation
    ([worldgen-chunks.md](./worldgen-chunks.md)) fixed it, which was a second
    seed break right after this one, and the test now sweeps 300 seeds.
- **Splitmix, not `NewPCG(seed, 0)`.** Using the raw seed as one half of the
  state and a constant as the other would make small seeds and the XOR-derived
  seeds start close together in state space. Mixing costs nothing and removes
  that question.

## Extending it

- **A new stream that lives past generation**: build it with `newPCG`, keep the
  source in `rngSources`, and add a field to `rngState` and a row to
  `rngFields`. If it is missing from `rngFields`, a loaded game silently
  replays that stream from its seed.
- **A new worldgen-only stream**: `newRand(Seed ^ <new constant>)` is enough
  for something rolled once per world. Anything rolled per chunk or feature
  uses `worldGen.featureRand` with its own stream constant, so it stays
  independent of generation order.
- **Changing a stream's seed derivation or draw order** changes every existing
  seed, and later every existing save. Treat it as a breaking change.
- **Method names**: v2 uses `IntN`, `Int64`, `Int32N`, `Uint64` and so on.
  There is no `Seed`, `Read` or `Int63`.

## Related

- [personality.md](./personality.md): why flavor draws stay off `World.rng`.
- [determinism.md](./determinism.md): the other half of the invariant (map
  order), and the lockstep test a save/load test will copy.
- [browser-frontend.md](./browser-frontend.md): the save/load design this
  unblocks.

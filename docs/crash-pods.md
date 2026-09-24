# Crash pods

> Part of the [mars-sim documentation](./README.md).

## What it is

Every colonist arrives in a crash pod: a small prefab stamped into the world
where it lands, holding the colonist's own bunk, toilet, and locker, stocked
with a manifest of meals, and the colonist steps out carrying a gun and a purse.
It is the only way into the game — worldgen, the spawn command, and the
director's `arrival` occurrence all come through one function, `arrive`. This is
the first half of phase **E2** of the [economy plan](./economy.md); the meals it
brings are what [food.md](./food.md) is about.

## Source

- [`internal/sim/crashpod.go`](../internal/sim/crashpod.go) — the layout
  (`podFixtures`), `arrive`, `findPodSite`, `forEachRingPoint`, `podSiteRock`.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go) — `generate` lands
  the founders; `caveRadii` sizes the cavern for them.
- [`internal/sim/engine.go`](../internal/sim/engine.go) — the spawn command.
- [`internal/sim/director.go`](../internal/sim/director.go) — `OccArrival`,
  `fireArrival`.
- [`internal/sim/config.go`](../internal/sim/config.go) — the `crash-pod-*`
  settings.
- [`internal/sim/crashpod_test.go`](../internal/sim/crashpod_test.go).

## How it works

### The pod

```
B . T . L     B bunk, T toilet, L locker (a storage container)
. . @ . .     @ where the colonist steps out
```

Five wide, two tall. The fixtures sit one tile apart, as in any room, so each
keeps free access tiles; every other tile is floor. `arrive`:

1. finds a site (below) and clears any rock under the footprint — the displaced
   rock simply disappears;
2. places the three fixtures and spawns the colonist on the door tile;
3. makes all three **private** to it (`setFixtureOwner`, see
   [property.md](./property.md));
4. puts `crash-pod-meals` meals in the locker, credited to the colonist on its
   ledger;
5. hands it `crash-pod-pistols` pistols and `crash-pod-shotguns` shotguns;
6. records `podOrigin` on the colonist.

The purse (`crash-pod-purse`) is minted by `spawn` itself, so it reaches every
colonist however it was created (see [money.md](./money.md)).

| Setting | Default |
| --- | --- |
| `crash-pod-purse` | 100 dollars |
| `crash-pod-meals` | 10 |
| `crash-pod-pistols` | 1 |
| `crash-pod-shotguns` | 0 |

These replace the colony ship's `pistols`/`shotguns` settings, which issued two
guns among the whole founding party. See [combat.md](./combat.md) for what
arming everyone did to survival.

### Where a pod lands

`findPodSite` walks rings of candidate top-left corners outward from just below
the map's middle row and takes the first **clean** site: footprint and one-tile
margin all floor, nobody standing on the footprint, no construction designated
there, no room's reserved door approach, and no wall or fixture in the margin.
The first site it passes that is valid but not clean — some rock under the
footprint or in the margin, but touching existing floor so the colonist is not
sealed in — is held as a crash site, and taken once the search is
`podCrashSlack` (8) rings past it without finding open floor. A pod that clears
rock announces that it "smashes down through the rock".

Four rules shape where pods end up, and each was learned the hard way:

- **Lower half only.** Candidates above the middle row are skipped. Rooms are
  only ever sited against rock *above* them (see `roomSiteClear` in
  [construction.md](./construction.md)), so the top of the cavern is where the
  colony's first rooms go. The first version grew a square of pods out from the
  middle and filled that top rim; small colonies then had no site for even a
  two-facility room and never built anything.
- **Stretched rings, middle rows first.** A ring holds the points whose
  `max(ceil(|dx|/2), |dy|)` is r — twice as wide as tall, like the cavern — and
  visits the row below before the row above. Pods spread along the cavern's
  width instead of climbing its height.
- **A clean landing needs an open margin,** not just an open footprint. A pod
  that settles against the cavern wall takes exactly the rock-backed edge a
  room wants.
- **Only discovered floor counts.** The floor of a hidden natural cavern
  ([caverns.md](./caverns.md)) is open ground to a naive check, so pods used to
  land "cleanly" out in the rock, open the cavern around their colonist, and
  strand it with no way back to the colony (it starved). Undiscovered floor is
  never part of a footprint and never counts as a way out in the margin.

The cavern is sized for its pods (`caveRadii`: ten tiles per settler plus the
pod's footprint with margin, and never fewer than `minCaveRy` = 6 rows above
and below the middle). `TestPodsLeaveRoomForTheFirstRooms` checks that a room
can be sited the moment 1, 3, 6, 10, 16, or 40 settlers land.

`podRingHint` remembers roughly where the last pod came down, so the next search
starts a few rings inside it rather than rescanning the packed middle. With it,
landing 2000 founders on a 400×400 map takes about 0.6 s.

### Private fixtures near other people

A pod scatters three private fixtures through the cavern, so two rules that
assumed every fixture was communal had to learn otherwise (see
[property.md](./property.md)):

- `onFacilityAccess` — "is an idle colonist standing here in someone's way?" —
  now ignores private fixtures. Before it did, every tile near every pod counted
  as in the way, `availableToTalk` never found a free partner, social need
  pinned at its ceiling, and the colony stopped working to wait for
  conversations that never came (`TestColonyKeepsExcavatingOnceNeedsBite`
  caught it).
- The colony plans capacity from `plannedFacilities`, which counts private
  bunks and toilets too. That is deliberate: each settler has its own, so the
  colony builds no dormitories and few toilets until the population outgrows
  its pods.

## Why it is this way

- **A stamped prefab, not unpacked fixtures.** Carrying a bunk as an item and
  placing it would need a placed ↔ carried conversion that touches
  construction, pathing, and the tile grid. Stamping gets owned fixtures into
  the game without it; moving a bunk later is still open.
- **One arrival function.** Worldgen, the spawn command, and the director used
  to place colonists three ways. Now a change to what a settler brings lands
  everywhere at once.
- **Lockers are storage containers.** A private locker is an ordinary chest
  only its owner may use. So a colonist whose pockets fill up while mining
  unloads into its own locker, and the colony builds a shared storage room only
  once lockers are full or out of reach.
- **The fixture row never touches another fixture.** A fixture is never
  orthogonally adjacent to another, and every other footprint tile is floor, so
  stamping a pod can never cut a path that crossed the footprint before.

## Extending it

- **A new manifest item**: a `crash-pod-*` setting, and a line in `arrive` that
  puts it in the locker (credited to the colonist) or the colonist's pockets.
- **A bigger or different pod**: change `podWidth`/`podHeight`/`podFixtures`.
  Keep fixtures from touching orthogonally, and re-check
  `TestPodsLeaveRoomForTheFirstRooms`, which is what a bigger pod will break.
- **Choosing where to land** (a player-picked site, or landing near family)
  belongs in `findPodSite`; keep it free of randomness or draw from `World.rng`.

## Related

- [food.md](./food.md) — eating the meals a pod brings.
- [property.md](./property.md) — private fixtures and ledgers.
- [money.md](./money.md) — the purse.
- [director.md](./director.md) — the `arrival` occurrence.
- [world.md](./world.md) — worldgen and the landing cavern.
- [economy.md](./economy.md) — the plan this is part of.

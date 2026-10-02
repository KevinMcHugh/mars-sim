# Crash pods

> Part of the [mars-sim documentation](./README.md).

## What it is

Every colonist arrives in a crash pod: a small metal-hulled room stamped into
the world where it lands, holding the colonist's own bunk, toilet, and locker, stocked
with a manifest of meals, and the colonist steps out with a purse and one
rare item: a gun, a chicken (with a trough in the pod), or a cat.
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
H H H H H     H hull (metal wall, the Hull terrain)
H B T L H     B bunk, T toilet, L locker (a storage container)
H . @ . H     @ where the colonist steps out
H H . H H     the doorway
    ^         the approach, reserved like a room's
```

A 3×2 interior inside a one-tile **hull**: five wide, four tall. `Hull` is its
own terrain so the map can show a pod as salvaged metal (⬜ in the TUI) rather
than the colony's masonry (🧱), but it behaves like a `Wall`: it blocks
movement, and a sealed-in colonist may break it down (`nearestEscapeWall`).
Nothing builds it; it only arrives.

Pods that land side by side share their side hull as a **party wall**, as the
colony's own rooms do, so a row of them is one block of cabins:

```
□□□□□□□□□□□□□
□BTL□BTL□BTL□
□.@.□.@.□.@.□
□□.□□□.□□□.□□
.............   the walkway between rows, where the doorways open
□□□□□□□□□□□□□
```

`podPartyWalls` decides it: a side is shared when a pod landed exactly one
hull-width over in the same row (`World.pods` records every pod's origin) and
that neighbor's side hull is still whole. `arrive`:

1. finds a site (below) and stamps the hull, clearing any rock inside it — the
   displaced rock simply disappears;
2. clears a one-tile **crater** of any rock around the hull (except beyond a
   shared side, which is the neighbor's inside), and reserves the
   approach below the doorway in `doorTiles` so no room or later pod covers it;
3. places the three fixtures and spawns the colonist on the tile inside the
   door;
4. makes all three **private** to it (`setFixtureOwner`, see
   [property.md](./property.md));
5. puts `crash-pod-meals` meals in the locker, give or take up to
   `crash-pod-meal-spread` (`podMeals`), credited to the colonist on its
   ledger;
6. records `podOrigin` on the colonist;
7. hands it its one **rare item** (`podRareItem`): a gun in its pockets
   (`podGun`: a shotgun `crash-pod-shotgun-percent` of the time, else a
   pistol); or a chicken, which steps out beside it, and a **trough** stamped
   in front of the bunk, private to the keeper and stocked with
   `trough-fill` feed; or a cat, which steps out beside it. A pet records the
   colonist as its `keeper`. See [chickens.md](./chickens.md).

A keeper's pod, and where any pet steps out:

```
H H H H H
H B T L H
H ~ @ c H     ~ trough (a chicken keeper's pod only), c the chicken or cat
H H . H H
```

Every fixture is used from any of the eight tiles round it, so the trough in
front of the bunk costs nothing: the bunk is still reached from the door tile
`@`, diagonally.

The purse (`crash-pod-purse`) is minted by `spawn` itself, so it reaches every
colonist however it was created (see [money.md](./money.md)).

| Setting | Default |
| --- | --- |
| `crash-pod-purse` | 100 dollars |
| `crash-pod-meals` | 10 |
| `crash-pod-meal-spread` | 0 |
| `crash-pod-gun-weight` | 50 |
| `crash-pod-chicken-weight` | 25 |
| `crash-pod-cat-weight` | 25 |
| `crash-pod-shotgun-percent` | 25 |

**One rare item.** Every colonist lands with exactly one of a gun, a chicken,
or a cat, picked by the three relative weights: at the defaults half land
armed (three pistols to every shotgun), a quarter with a chicken, a quarter
with a cat. The weights are relative, not percents, so `1/0/0` arms everyone
and `0/0/0` lands everyone empty-handed. The mechanics tests' `testConfig`
sets `1/0/0` and no shotguns, so they can count on every colonist carrying
exactly one pistol and no pets on the map.

`podRareItem` and `podGun` are, like `podMeals`, pure functions of the seed and
the colonist's ID (each on its own salt), not draws from a stream: they shift
no other draw and ignore arrival order. Spawning a pet draws no randomness
either, but it does take an entity ID, so every later ID moves.

**What it replaced, and why.** The colony ship's `pistols`/`shotguns` issued
two guns among the whole founding party. Then each pod carried a manifest of
`crash-pod-pistols` and `crash-pod-shotguns`, each aboard with its own
`-percent` odds: about 76% of colonists landed armed, 14% with both guns, and
the colony was a crowd of near-identical gunmen. One rare item each makes the
colonists differ in what they brought rather than how much, and gives the
chicken and the cat a way into the game that is a colonist's own rather than
something worldgen scatters. Those old settings are retired with a pointer
here (`RetiredSettings`).

Fewer guns is a balance change. 20 colonists on a 200×200 map, seeds 1–16,
30,000 ticks, with the scum incubator: 1 colonist starved and 254 were alive
at the end at the defaults, against 2 starved and 262 alive with no chickens
(cats in their place). Before the incubator the same chickens cost colonists:
33 starved against 10, because hens and their keepers drew on the same wild
scum the colony scraped (see [chickens.md](./chickens.md)). See
[combat.md](./combat.md) for what arming everyone did to survival.

**Meal spread.** `podMeals` varies each pod's meals evenly within
`crash-pod-meal-spread` of `crash-pod-meals`. It's a pure function of the
seed and the colonist's ID, not a draw from a random stream, so it shifts no
other draw and doesn't depend on arrival order. It staggers when lockers run
dry. It's off by default because it makes starvation
worse: 100 colonists on a 300×150 map, seeds 1–4, 30,000 ticks:

| `crash-pod-meal-spread` | Starved (of 400) |
| --- | --- |
| 0 | 96 |
| 4 | 117 |
| 8 | 172 |

With a spread of 8 the colony produces less food, with fewer colonists cooking
for it (4–6 against 6–10) and less scum delivered, and its stock runs out
around tick 8,500 instead of 14,000. Why fewer colonists cook isn't
established. The lockers aren't what drives the die-off at the default
anyway: they're empty by about tick 3,500, and the colony starves around
tick 14,000, when its production falls behind (see [food.md](./food.md)).

### Where a pod lands

`findPodSite` walks rings of candidate top-left corners outward from just below
the map's middle row and takes the first **clean** site: footprint and one-tile
margin all floor, nobody standing on the footprint, no construction designated
there, no room's reserved door approach, and no wall, hull, or fixture in the
margin — except beyond a party wall, where the margin is a neighbor's inside.
The first site it passes that is valid but not clean — some rock under
the footprint or in the margin, but with some margin already floor so the
crater connects the doorway to the colony — is held as a crash site, and taken once the search is
`podCrashSlack` (8) rings past it without finding open floor. A pod that clears
rock announces that it "smashes down through the rock".

Five rules shape where pods end up, and each was learned the hard way:

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
- **The margin is the way out.** The door faces down, but a pod that crashes
  into the rock below a full cavern has only rock below it. The first hulled
  pods required the approach itself to touch floor and found no site at all
  once the cavern filled. The crater fixes that: whatever side of the margin
  touches floor, the cleared ring leads from the doorway round to it, and it
  keeps a walkway between neighboring pods.
- **Party walls, not a gap.** The first hulled pods kept a one-tile walkway
  on every side, so a colony of them was a maze of separate boxes with
  corridors between every pair. Sharing side walls halves the hull, packs a
  row tight, and leaves the walkway only where it is needed: between rows,
  in front of the doorways. The ring search finds the shared site first
  anyway — a pod one hull-width over is two rings out, and the nearest
  non-touching site is three.
- **Only discovered floor counts.** The floor of a hidden natural cavern
  ([caverns.md](./caverns.md)) is open ground to a naive check, so pods used to
  land "cleanly" out in the rock, open the cavern around their colonist, and
  strand it with no way back to the colony (it starved). Undiscovered floor is
  never part of a footprint and never counts as a way out in the margin.
- **Nor anywhere a landing would reveal.** Landing discovers the footprint,
  and `revealAround` reveals the ring around every tile it touches, so hidden
  floor up to two tiles out (`podRevealReach`) was flooded into view. That
  broke into the cavern and rolled its nests with nobody digging: on a 120×70
  map with 6 colonists, repeated arrivals breached a cavern in 29 of 30
  seeds. `hiddenFloorNear` rejects any such site
  (`TestPodsNeverRevealAHiddenCavern`).

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
- **A hull, not an open footprint.** The first pods were an open five-by-two
  patch of fixtures on the floor; on the map they looked like furniture left
  out in the cavern, not somewhere a settler lives. The hull makes the pod read
  as the colonist's own room.
- **Fixtures side by side.** An open pod had to space its fixtures a tile
  apart so stamping one could never cut a path across the footprint. A hulled
  pod has no path across it, so the three sit on the back row, each used from
  the floor tile in front of it — which is what fits them in a 3×2 interior.

## Extending it

- **A new manifest item**: a `crash-pod-*` setting, and a line in `arrive`
  that puts it in the locker (credited to the colonist) or the colonist's
  pockets. A new **rare item** is a fourth weight, a `rare*` value, and a case
  in `arrive`'s switch.
- **A bigger or different pod**: change `podWidth`/`podHeight`/`podFixtures`
  and the door offsets (`podDoor`, `podDoorway`, `podApproach`); `podHullAt`
  derives the hull from them. Every fixture needs a floor tile inside the hull
  beside it. Re-check `TestPodsLeaveRoomForTheFirstRooms`, which is what a
  bigger pod will break.
- **Choosing where to land** (a player-picked site, or landing near family)
  belongs in `findPodSite`; keep it free of randomness or draw from `World.rng`.

## Related

- [food.md](./food.md) — eating the meals a pod brings.
- [chickens.md](./chickens.md) — the chicken, its trough, and its keeper.
- [combat.md](./combat.md) — what the guns do.
- [property.md](./property.md) — private fixtures and ledgers.
- [money.md](./money.md) — the purse.
- [director.md](./director.md) — the `arrival` occurrence.
- [world.md](./world.md) — worldgen and the landing cavern.
- [economy.md](./economy.md) — the plan this is part of.

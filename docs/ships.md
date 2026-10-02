# Colony ships

> Part of the [mars-sim documentation](./README.md).

## What it is

Every colonist arrives aboard a **colony ship**: a hulled prefab stamped into
the world where it lands, carrying up to `ship-capacity` (20) settlers. Its
three rooms sit side by side behind one metal hull: a **bunkroom** of communal
bunks, a **latrine** of communal toilets, and a **hold** with a private
**locker** for each passenger (and a private trough for each chicken keeper).
There are bunks for half the passengers and a toilet for every four. That
is enough to get by on, and not enough to stop the colony building
dormitories and toilet rooms of its own. Each settler steps out with a purse,
meals in its locker, and one rare item: a gun, a chicken, or a cat.

Ships are the only way into the game. Worldgen, the spawn command, and the
director's `arrival` occurrence all go through `land` (by way of `arriveWave`
for a crowd). In the browser, a new game starts paused so the player can move
the founders' ships before the first tick. The TUI and headless runs keep
where they landed. This is the first half of phase **E2** of the
[economy plan](./economy.md). The meals in the lockers are what
[food.md](./food.md) is about.

Ships replaced **crash pods**: one 5×4 hulled cabin per colonist, each with
its own private bunk, toilet, and locker. "What it replaced" below explains
why.

## Source

- [`internal/sim/ship.go`](../internal/sim/ship.go): `Ship`, the layout
  (`planShip`), `arriveWave`, `land`, `stampShip`, `furnishShip`,
  `findShipSite`, `shipSiteRock`, `MoveShip` / `moveShip`, and the manifest's
  pure functions (`arrivalMeals`, `arrivalRareItem`, `arrivalGun`).
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go): `generate` lands
  the founders, and `caveRadii` sizes the cavern for their ships.
- [`internal/sim/engine.go`](../internal/sim/engine.go): the spawn command
  (a ship of one), `MoveShip`, and `start-paused`.
- [`internal/sim/director.go`](../internal/sim/director.go): `OccArrival`,
  `fireArrival`.
- [`internal/sim/config.go`](../internal/sim/config.go): the `ship-*` and
  `crash-pod-*` settings (the Arrivals section).
- [`internal/wire/topics.go`](../internal/wire/topics.go): the `ships` topic.
- [`web/src/ui/ShipsPanel.svelte`](../web/src/ui/ShipsPanel.svelte),
  [`web/src/main.ts`](../web/src/main.ts) (`landShip`, `showShipPreview`), and
  [`web/src/game.svelte.ts`](../web/src/game.svelte.ts) (`shipSiteAt`,
  `shipSiteFree`): placing ships in the browser.
- [`internal/sim/ship_test.go`](../internal/sim/ship_test.go).

## How it works

### The ship

A ship for 20, the default capacity and a full one:

```
H H H H H H H H H H H H H H H H H H H H H H     H hull (the Hull terrain)
H B B B B B H T T T H L L L L L L L L L L H     B bunk, T toilet, L locker
. . . . . . . . . . . . . . . . . . . . . .     the aisle, two wide, through
. . . . . . . . . . . . . . . . . . . . . .     both ends and every partition
H B B B B B H T T . H L L L L L L L L L L H
H H H H H H H H H H H H H H H H H H H H H H
  bunkroom    latrine         hold
```

A ship for six, one of them a chicken keeper (`~`, the trough):

```
H H H H H H H H H H H
H B B H T H L L L ~ H
. . . . . . . . . . .
. . . . . . . . . . .
H B . H T H L L L . H
H H H H H H H H H H H
```

`planShip` builds the layout from who is aboard. Every ship is six rows tall:
hull, a fixture row, two aisle rows, a fixture row, hull. Each room fills its
fixtures column by column, top then bottom, so it is one column wider for every
two fixtures and every fixture faces the aisle. Rooms are separated by a
partition of hull with a two-wide doorway in the aisle rows, and the aisle
leaves the ship through a two-wide doorway at each end. A room that would hold
nothing is left out. The approaches outside both end doorways go into
`doorTiles`, like a room's door, so no later room or ship seals the passengers
in.

| Room | Holds | Owner |
| --- | --- | --- |
| Bunkroom | `ship-bunk-percent` (50) of the passengers, rounded up, at least one | the community, communal |
| Latrine | `ship-toilet-percent` (25), rounded up, at least one | the community, communal |
| Hold | a locker (`Storage`) per passenger, then a trough per chicken keeper | that passenger, private |

`Hull` has its own terrain so the map can show a ship as salvaged metal (⬜ in
the TUI) rather than the colony's masonry (🧱). It behaves like a `Wall`: it
blocks movement, and a sealed-in colonist may break it down (`escapeTarget`,
see [escape.md](./escape.md)). Nothing builds it; it only arrives.

### Landing

`arriveWave(n)` splits n settlers into as few ships as `ship-capacity`
allows, loaded evenly (`shipLoads`: 30 come down as two ships of 15, not 20
and 10), and lands each with `land`:

1. Predict the passengers' IDs. They take the next n, in order, and their pets
   come after. `arrivalRareItem` is a pure function of the ID, so the ship
   knows who keeps a chicken, and so how many troughs the hold needs, before
   anyone exists. `land` panics if the IDs ever stop being consecutive.
2. `planShip`, then `findShipSite` (below).
3. `stampShip`: hull and floor over the whole footprint, obliterating
   whatever was there; a one-tile **crater** clearing any rock round the hull;
   the approaches reserved; the fixtures placed. Bunks and toilets come out
   communal, because that is what `SetTerrain` makes every fixture.
4. Spawn the passengers on the aisle (`layout.floor`, top row first), set
   `Entity.ship`, and roll each one's background (skills, then a flavor-only
   former employer — see [arms-makers.md](./arms-makers.md)).
5. `furnishShip`: make each locker and trough private to its passenger
   (`setFixtureOwner`, see [property.md](./property.md)), stock each locker
   with `arrivalMeals` meals credited on its ledger, and fill each trough with
   `trough-fill` feed.
6. Hand out the rare items: a gun goes in the pockets, and a chicken or a cat
   steps out onto the next aisle tile with `keeper` set (a hen also gets its
   keeper's trough). See [chickens.md](./chickens.md).

`planShip` guarantees there is aisle for every passenger and one pet each.
Bunks and toilets alone almost always leave enough; a ship with many cat
owners and few troughs gets a spare column at the end of the hold.

The spawn command lands a ship of one (seven tiles wide: one bunk, one toilet,
one locker). The director's `arrival` occurrence calls `arriveWave` with its
count.

### Where a ship lands

`findShipSite` is the crash pods' search with a wider footprint. It walks
stretched rings of top-left corners (`forEachRingPoint`, twice as wide as
tall, like the cavern) outward from just below the map's middle row, and
takes the first **clean** site: footprint and margin all discovered floor,
nobody standing on the footprint, nothing designated for construction, no
reserved door approach, and no wall, hull, or fixture in the margin. The first
site that is valid but not clean (rock under the footprint or in the margin,
with some margin already floor so the crater connects the doorways to the
colony) is held as a **crash site**. The search takes it once it is
`shipCrashSlack` (8) rings past that site without finding open floor. A ship
that clears rock announces that it "smashes down through the rock".

The rules the pods learned all still hold:

- **Lower half only**, so the top of the cavern stays free for the colony's
  first rooms, which prefer rock behind them (see `roomSiteClear` in
  [construction.md](./construction.md)). When pods grew outward as a square
  from the middle, small colonies had nowhere to site even one room.
- **A clean landing needs an open margin,** not just an open footprint, or the
  ship takes the rock-backed edge a room wants.
- **The margin is the way out.** The crater clears a path from the doorways
  round to whatever floor the margin touches.
- **Only discovered floor counts,** and **nothing within
  `shipRevealReach` (2) of hidden floor** (`hiddenFloorNear`). Pods once
  landed "cleanly" in hidden caverns and stranded their colonists. Repeated
  pod arrivals also breached a cavern in 29 of 30 seeds just by revealing
  the tiles around their footprint.

`shipRingHint` remembers roughly where the last ship landed, so the next
search starts a few rings inside it.

The cavern is sized for its ships: `caveRadii` gives each settler ten tiles
of elbow room plus its share of a full ship's footprint and crater
(`shipTilesPerColonist`, about 11 tiles at the defaults against a pod's 30).
The cavern is never less than `minCaveRy` (6) rows above and below the
middle. `TestShipsLeaveRoomForTheFirstRooms` checks that a room can be sited
the moment 1, 3, 6, 10, 16, 20, or 40 settlers land, and that every passenger
can walk out to the main room.

### Placing ships before the game starts (browser)

`MoveShip{Ship, X, Y}` relands a ship with its top-left at (X, Y). The engine
accepts it only at tick 0. The browser sets `start-paused: true` when it
starts a game and opens the **Ships** tab. That tab is fed by the `ships` topic
(see [wire-format.md](./wire-format.md)) and is described in
[frontend-web.md](./frontend-web.md). The player picks a ship up, sees where
it would land under the pointer, clicks to land it, and presses **Land and
start**. The TUI never sends `MoveShip`, so its ships stay where
`findShipSite` put them.

`moveShip`:

1. Checks the site (`shipSiteAllowed`): on the map with room for the crater,
   the footprint clear of every other ship's footprint *and the one-tile
   walkway round it* (which holds that ship's doorways), and no other ship's
   colonist underfoot.
2. Lifts the passengers and pets off the occupancy grid and turns the old
   footprint to bare floor. That drops its fixtures, lockers and all, and frees
   its approaches.
3. **Obliterates** the new site. Any creature under the footprint dies
   ("crushed by a landing ship", into the graveyard), and `stampShip` replaces
   rock, ore, and anything else with the ship.
4. Sets everyone down on the aisle in the same order as at landing, and calls
   `furnishShip` again, which restocks the lockers and troughs from the same
   pure functions.

A moved ship is therefore exactly the ship that would have landed there, and
moving one draws no randomness. Other ships aren't obliterated: crushing the
founders is never what the player meant. A ship moved into hidden rock is
the player's choice and is allowed. Stamping it reveals the ground around it,
and if that is a natural cavern, the cavern floods into view and rolls its
nests (see [caverns.md](./caverns.md)).

### The manifest

| Setting | Default |
| --- | --- |
| `ship-capacity` | 20 |
| `ship-bunk-percent` | 50 |
| `ship-toilet-percent` | 25 |
| `crash-pod-purse` | 100 dollars |
| `crash-pod-meals` | 10 |
| `crash-pod-meal-spread` | 0 |
| `crash-pod-gun-weight` | 50 |
| `crash-pod-chicken-weight` | 25 |
| `crash-pod-cat-weight` | 25 |
| `crash-pod-shotgun-percent` | 25 |
| `start-paused` | false (the browser sends true) |

The `crash-pod-*` settings kept their names from the pods; what they mean has
not changed. They now apply to each passenger. The purse is minted by `spawn`
itself, so it reaches every colonist however it was created (see
[money.md](./money.md)).

**One rare item.** Every colonist lands with exactly one of a gun, a chicken,
or a cat, picked by the three relative weights. At the defaults, half land
armed (three pistols to every shotgun), a quarter with a chicken, and a
quarter with a cat. `1/0/0` arms everyone and `0/0/0` lands everyone
empty-handed. The mechanics tests' `testConfig` sets `1/0/0` with no shotguns.
`arrivalRareItem`, `arrivalGun`, and `arrivalMeals` are pure functions of the
seed and the colonist's ID, each with its own salt. They are not draws from a
stream, so they shift no other draw and ignore arrival order. They kept the
pods' salts, so a given colonist ID still rolls what it did under pods.

The rare item replaced the colony ship's `pistols`/`shotguns` and then the
pods' per-item `crash-pod-pistols`/`-shotguns` odds. About 76% of colonists
used to land armed, 14% with both guns. Those settings are retired with a
pointer (`RetiredSettings`). See [combat.md](./combat.md).

**Meal spread.** `crash-pod-meal-spread` varies each locker's meals evenly
around `crash-pod-meals`. It is off by default because it made starvation
worse under pods. Measured with 100 colonists on a 300×150 map, seeds 1–4,
30,000 ticks:

| `crash-pod-meal-spread` | Starved (of 400) |
| --- | --- |
| 0 | 96 |
| 4 | 117 |
| 8 | 172 |

## Why it is this way

- **What it replaced.** One pod per colonist landed as rows of 5×4 cubicles
  that shared side walls. A colony of 20 was four or five rows of hull with a
  one-tile walkway between them, and every trip across the landing site went
  round the blocks. It caused real pathing and travel-time trouble. Each pod
  also brought a private bunk and toilet, so `plannedFacilities` saw a full
  colony as fully housed and the colony built no dormitories and almost no
  toilets until arrivals outgrew the pods. A ship packs the same settlers into
  one compact block with a straight aisle through it. A ship for 20 is 22×6,
  where its pods covered about 20×19 with their walkways. Its communal bunks
  and toilets are deliberately too few.
- **Why the rooms are clustered behind one hull.** It is one vessel that broke
  through the crust, not twenty. Rooms in a row share partitions, so the hull
  costs little, and the aisle running through every partition means nobody
  walks round the ship to reach the latrine. Two doorways, one at each end,
  keep one blocked side from trapping anyone.
- **Why two aisle rows.** A single-tile aisle would be the same corridor
  problem the pods had, inside the ship: twenty passengers and their pets
  stepping out and passing each other to reach their lockers and the bunks.
  Two rows let them pass and give everyone a tile to step out onto.
- **Why lockers stay private.** A private chest is an ordinary storage
  container only its owner may use (see [storage.md](./storage.md)). A
  miner's full pockets unload into it, and the colony builds a shared storage
  room only once lockers are full or out of reach.
- **Why some bunks, and not one each.** Half is a starting point, not a
  measured optimum: enough that the first night goes in shifts rather than
  nobody sleeping, few enough that the colony plans dormitories and toilet
  rooms straight away. Measured with 20 colonists on a 200×200 map, seeds 1–4,
  20,000 ticks: nobody starved with ships or with pods; ships ended with
  20/20/17/13 alive against the pods' 20/20/18/0, and the colony had built
  1–7 bunks and 1–7 toilets beyond the ship's 10 and 5. Pods ended with 21–25
  of each (20 of them the pods' own).
- **Placement is a command, not a worldgen option.** Worldgen lands the ships
  automatically everywhere. The browser then moves them before the first tick
  instead of asking for sites up front. The TUI, headless runs, the director,
  and the tests share one landing path, and a moved ship is guaranteed to be
  one `land` could have produced.
- **A stamped prefab, not unpacked fixtures.** Carrying a bunk as an item and
  placing it would need a placed ↔ carried conversion that touches
  construction, pathing, and the tile grid. Stamping avoids that.
- **The smaller cavern.** Ships take about a third of the ground pods did, so
  the landing cavern shrank with them. The lazy-worldgen golden case
  (`lazy-1000x1010`) needed a new seed and a longer run (seed 4, 3000 ticks)
  to keep digging past the chunks generated at the start.

## Extending it

- **Privacy.** The communal bunks and toilets are what a privacy drive, once
  drives land, should push against: a colonist sharing a bunkroom wants a room
  of its own, which is what `house-savings` commissions today (see
  [labor.md](./labor.md)).
- **Salvage.** The hull is salvaged spacecraft. Letting colonists strip it for
  metal (a dig-like job on `Hull` that yields an item) is the natural next
  step, once the colony has moved out of the ship.
- **A new manifest item**: a setting, and a line in `land` or `furnishShip`
  that puts it in the locker (credited to the colonist) or the pockets. A new
  **rare item** is a fourth weight, a `rare*` value, and a case in `land`'s
  switch. One that needs a fixture goes in the hold like the trough: add it to
  `planShip`'s hold room.
- **A new room**: another entry in `planShip`'s `rooms`. Every fixture must sit
  on a fixture row facing the aisle. Re-check `TestShipLayout` and
  `TestShipsLeaveRoomForTheFirstRooms`.
- **Renaming `crash-pod-*`** to `arrival-*` is a `RenamedSettings` entry per
  key and a regenerated `mars-sim.yaml`.

## Related

- [food.md](./food.md): eating the meals in the lockers.
- [chickens.md](./chickens.md): the chicken, its trough, and its keeper.
- [combat.md](./combat.md): what the guns do.
- [property.md](./property.md): private fixtures and ledgers.
- [construction.md](./construction.md): the dormitories and toilet rooms the
  ships leave the colony short of.
- [money.md](./money.md): the purse.
- [director.md](./director.md): the `arrival` occurrence.
- [world.md](./world.md): worldgen and the landing cavern.
- [frontend-web.md](./frontend-web.md): the Ships tab.
- [economy.md](./economy.md): the plan this is part of.

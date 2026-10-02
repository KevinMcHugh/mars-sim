# Colony ships

> Part of the [mars-sim documentation](./README.md).

## What it is

Every colonist arrives aboard a **colony ship**: a hulled prefab stamped into
the world where it lands, carrying up to `ship-capacity` (20) settlers. Its
three rooms sit behind one metal hull: a **bunkroom** of communal bunks, a
**latrine** of communal toilets, and a **hold** with a private **locker** for
each passenger (and a private trough for each chicken keeper). A ship comes
down in one of three **shapes**: a **stick** (the rooms in a row), a **hub and
spoke** (a concourse with a room down each spoke), or a **knobby cluster**
(rooms budding off a spine corridor).
There are bunks for half the passengers and a toilet for every four. That
is enough to get by on, and not enough to stop the colony building
dormitories and toilet rooms of its own. Each settler steps out with a purse,
meals in its locker, and one rare item: a gun, a chicken, or a cat.

Ships are the only way into the game. Worldgen, the spawn command, and the
director's `arrival` occurrence all go through `land` (by way of `arriveWave`
for a crowd). In the browser, a new game starts paused with the founders'
ships still **aloft**: the player lands them one after another, each where they
click, before the first tick. The TUI and headless runs land them
automatically. This is the first half of phase **E2** of the
[economy plan](./economy.md). The meals in the lockers are what
[food.md](./food.md) is about.

Ships replaced **crash pods**: one 5×4 hulled cabin per colonist, each with
its own private bunk, toilet, and locker. "What it replaced" below explains
why.

## Source

- [`internal/sim/ship_layout.go`](../internal/sim/ship_layout.go): the
  shapes. `shipLayout`, `shipCanvas` and its `segment`, `planShip` and the
  three planners (`planStick`, `planHub`, `planCluster`), `shipShapeFor`, and
  `nextShipLayout`.
- [`internal/sim/ship.go`](../internal/sim/ship.go): `Ship`, `arriveWave`,
  `land` / `landAt` / `landShip`, `LandShip` / `landAloft`, `stampShip`,
  `furnishShip`, `findShipSite`, `shipSiteRock`, `shipSiteAllowed`,
  `MoveShip` / `moveShip`, and the manifest's pure functions
  (`arrivalMeals`, `arrivalRareItem`, `arrivalGun`).
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go): `generate` lands
  the founders (or leaves them aloft), and `caveRadii` sizes the cavern for
  their ships.
- [`internal/sim/engine.go`](../internal/sim/engine.go): the spawn command
  (a ship of one), `LandShip`, `MoveShip`, and `start-paused`.
- [`internal/sim/director.go`](../internal/sim/director.go): `OccArrival`,
  `fireArrival`.
- [`internal/sim/config.go`](../internal/sim/config.go): the `ship-*` and
  `crash-pod-*` settings (the Arrivals section).
- [`internal/wire/topics.go`](../internal/wire/topics.go): the `ships` topic.
- [`web/src/ui/ShipsPanel.svelte`](../web/src/ui/ShipsPanel.svelte),
  [`web/src/main.ts`](../web/src/main.ts) (`landShip`, `showShipPreview`), and
  [`web/src/game.svelte.ts`](../web/src/game.svelte.ts) (`shipSiteAt`,
  `shipSiteFree`, `shipTiles`, `landAloft`): placing ships in the browser.
- [`internal/sim/ship_test.go`](../internal/sim/ship_test.go).

## How it works

### The shapes

Ships for six, one of them a chicken keeper, as `planShip` draws them. `#` is
hull (the `Hull` terrain), `.` deck, B a bunk, T a toilet, L a locker, `~` a
trough. Anything blank is outside the ship: landing leaves it as it was.

A **stick**: the rooms in a row behind one hull, the aisle running the
ship's length and out of a doorway at each end. A stick for 20 is 22×6 (a
column longer for every two chicken keepers' troughs).

```
# # # # # # # # # # #
# B B # T # L L L ~ #
. . . . . . . . . . .
. . . . . . . . . . .
# B . # T # L L L . #
# # # # # # # # # # #
```

A **hub and spoke**: a 6×6 hub round a 4×4 concourse with a doorway in every
side, and a room down each spoke: the bunkroom west, the hold east, the
latrine south. A hold of more than `shipHubSplit` (8) fixtures is split, its
back half down a fourth spoke to the north. Every spoke's far end is a
doorway, and so is any hub side with no spoke. One for 20 is 19×16.

```
# # # # # . . # # # # # # #
# B B # . . . . # L L L ~ #
. . . . . . . . . . . . . .
. . . . . . . . . . . . . .
# B . # . . . . # L L L . #
# # # # # . . # # # # # # #
      # T . . T #
      # # . . # #
```

A **knobby cluster**: a spine corridor, open at both ends, with the rooms
budding off it as dead-end **knobs**, alternately above and below, side by side
so neighbors share a wall. Each room is cut into knobs of the fixture counts in
`shipKnobDepths` (6, 4, 8, 2, …) in turn, so the knobs are one to four rows
deep and the silhouette is lumpy rather than a comb of equal teeth. One for
20 is 25×13.

```
            # # # # # #
            # L . . L #
  # # # # # # L . . L #
  # B . . B # L . . L #
  # B . . . # ~ . . . #
# # # . . # # # . . # # #
. . . . . . . . . . . . .
. . . . . . . . . . . . .
# # # # # . . # # # # # #
      # T . . T #
      # # # # # #
```

Every room in every shape is the same **segment** (`shipCanvas.segment`): six
tiles across (hull, a fixture row, two aisle rows, a fixture row, hull) and
one column longer for every two fixtures, laid along the map or down it, with
an end wall at each end that is either hull or a two-wide doorway. Fixtures
fill it column by column, one side then the other, so every fixture faces the
aisle. A shape is just segments placed so their end walls meet, plus the hub
or the spine. A wall two segments share is drawn twice, and deck always wins,
so a doorway through a shared wall stays open whichever room is drawn last.
A room that would hold nothing is left out.

`shipCanvas.layout` normalizes the drawing to a top-left of (0, 0) and works
out the rest from the shape: the **doors** (every outside tile beside deck is
the approach to a doorway) and the **margin** (the one-tile ring of outside
tiles round the shape, which is the walkway and the crater). Planners list the
step-out tiles themselves: aisle first, then the doorways through shared
walls.

| Room | Holds | Owner |
| --- | --- | --- |
| Bunkroom | `ship-bunk-percent` (50) of the passengers, rounded up, at least one | the community, communal |
| Latrine | `ship-toilet-percent` (25), rounded up, at least one | the community, communal |
| Hold | a locker (`Storage`) per passenger, then a trough per chicken keeper | that passenger, private |

**Which shape.** `shipShapeFor` picks by the relative weights
`ship-stick-weight`, `ship-hub-weight`, and `ship-cluster-weight` (1/1/1 by
default; all 0 is all sticks). Like the rare item, it is a pure function of
the seed and the first passenger's ID, with its own salt, so it draws nothing
from a stream. Two fallbacks keep every landing possible. A shape too big for
the map, crater and all, comes down as a stick (`nextShipLayout`). A shape
`findShipSite` finds no site for is replanned as a stick, the slimmest ship.
`planShip` also falls back to a stick if a shape ever left fewer step-out
tiles than two per passenger; none does today.

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
2. `nextShipLayout` (the shape and the plan), then `findShipSite` (below).
3. `stampShip`: hull and floor over every tile of the shape, obliterating
   whatever was there; a one-tile **crater** clearing any rock in the margin;
   the approaches reserved; the fixtures placed. Bunks and toilets come out
   communal, because that is what `SetTerrain` makes every fixture. Tiles
   inside the footprint's box but outside the shape (the corners between a
   hub's spokes, the gaps between knobs) are left alone.
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
Bunks and toilets alone almost always leave enough; a stick with many cat
owners and few troughs gets a spare column at the end of the hold. A hub's
concourse and a cluster's spine leave plenty.

If stamping breaks into a hidden cavern (only a player's landing can; see
below), its nest roll waits until everyone aboard is out: `landShip` sets
`holdNests`, and `revealAround` keeps the cavern centers it finds instead of
rolling them. A nest spawned mid-landing would take the IDs the passengers
were promised, and `land` would panic. `TestPlayerLandingBreaksIntoACavernAfterThePassengersAreOut`
pins it.

The spawn command lands a ship of one (a stick of one is seven tiles wide:
one bunk, one toilet, one locker). The director's `arrival` occurrence calls `arriveWave` with its
count.

### Where a ship lands

`findShipSite` is the crash pods' search with a wider footprint. It walks
stretched rings of top-left corners (`forEachRingPoint`, twice as wide as
tall, like the cavern) outward from just below the map's middle row, and
takes the first **clean** site: every tile of the shape and its margin
discovered floor,
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
  ship takes the rock-backed edge a room wants. The margin follows the shape,
  so a hub's corners can sit against rock.
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
of elbow room plus its share of a full ship's tiles and crater
(`shipTilesPerColonist`) for the roomiest shape that may land: about 11 tiles
for a stick, against a pod's 30. A cluster takes the most. `shipReach`, how
far worldgen looks past the cavern for floor to put critters on, likewise
takes the biggest shape.
The cavern is never less than `minCaveRy` (6) rows above and below the
middle. `TestShipsLeaveRoomForTheFirstRooms` checks that a room can be sited
the moment 1, 3, 6, 10, 16, 20, or 40 settlers land, for each shape alone and
for the mix, and that every passenger can walk out to the main room.

### Landing the founders by hand (browser)

The browser sets `place-ships: true` and `start-paused: true` when it starts
a game, and opens the **Ships** tab. With `place-ships`, `generate` lands
nobody: it puts the founders' ship loads in `World.aloft` (split by
`shipLoads`, as `arriveWave` would) and logs that they "circle Mars, waiting
to land". The tab is fed by the `ships` topic (see
[wire-format.md](./wire-format.md)) and is described in
[frontend-web.md](./frontend-web.md).

The ships land **in order, one at a time**. The tab hands the player the next
ship aloft, shows its shape, and tints where it would land under the pointer;
a click sends `LandShip{Ship, X, Y}`. `landAloft` accepts it only at tick 0,
only for the next ship aloft (so a click sent twice cannot land the ship
after it), and only on a site `shipSiteAllowed` passes; it then calls
`landAt`, which crushes anything under the shape ("crushed by a landing
ship") and lands the ship there. The tab then hands over the next ship.
**Start** stays disabled until all of them are down.

Only the **next** ship's shape is known. A ship's plan depends on which IDs
its passengers take (the troughs, and the shape's hash), and a landing that
breaks into a cavern can spawn a nest and move the next free ID on. So the
`ships` topic carries the next ship's shape, worked out fresh from the world
by `nextShipLayout` (the same call `landAt` makes), and only a count for the
ones behind it.

If the game starts with ships still aloft (a frontend that sets
`place-ships` and starts without landing them), `step` lands them all with
`land` before the first tick, so nobody is left in orbit.

Once every ship is down, **Move** picks a landed one up again.
`MoveShip{Ship, X, Y}` relands it with its top-left at (X, Y); the engine
accepts it only at tick 0. `moveShip`:

1. Checks the site (`shipSiteAllowed`): on the map with room for the crater,
   no tile of the shape on another ship *or the one-tile walkway round it*
   (which holds that ship's doorways), and no other ship's colonist
   underfoot.
2. Lifts the passengers and pets off the occupancy grid and turns the old
   shape's tiles to bare floor. That drops its fixtures, lockers and all, and
   frees its approaches.
3. **Obliterates** the new site. Any creature under the shape dies
   ("crushed by a landing ship", into the graveyard), and `stampShip` replaces
   rock, ore, and anything else with the ship.
4. Sets everyone down on the aisle in the same order as at landing, and calls
   `furnishShip` again, which restocks the lockers and troughs from the same
   pure functions.

A moved ship is therefore exactly the ship that would have landed there, and
moving one draws no randomness. Other ships aren't obliterated: crushing the
founders is never what the player meant. A ship landed or moved into hidden
rock is the player's choice and is allowed. Stamping it reveals the ground
around it, and if that is a natural cavern, the cavern floods into view and
rolls its nests (see [caverns.md](./caverns.md)).

The TUI and headless runs never set `place-ships`, so their founders land
where `findShipSite` puts them. Director arrivals during play land the same
way in the browser too: placing is for the founders, before the clock runs.

### The ground round a ship is residence

Each landed ship is a structure (`registerShip`, see [zoning.md](./zoning.md))
and holds its shape and the walkway round it as residence for as long as it
stands: painting a zone skips those tiles. `findShipSite` never lands a ship on
ground zoned for anything else (`shipZoneOK`), since the landing would take it
over; a ship the player lands by hand obliterates whatever is there, zones
included. `moveShip` unregisters the ship before it lifts and registers it again
where it comes down, so its hold moves with it and its old ground goes back to
unzoned. A ship can be cleared with the
Zones tab's clear tool. Its lockers' goods move to the nearest chest that will
take them, still their owners' (or are lost, and logged, when none will), and
once the last tile is down its hold and door steps go with it. The `Ship` record
stays: its passengers still came down in it.

### The manifest

| Setting | Default |
| --- | --- |
| `ship-capacity` | 20 |
| `ship-bunk-percent` | 50 |
| `ship-toilet-percent` | 25 |
| `ship-stick-weight` | 1 |
| `ship-hub-weight` | 1 |
| `ship-cluster-weight` | 1 |
| `place-ships` | false (the browser sends true) |
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
  one compact block with a straight aisle through it. A stick for 20 is 22×6,
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
- **Why shapes, and why from one segment.** Every colony used to open on the
  same 23×6 bar, so every landing site looked alike. A hub puts the rooms a
  few steps from one concourse; a cluster is a compact blob with a long
  spine to step out onto. Building every shape out of the stick's room
  segment keeps what the stick got right: a two-wide aisle everywhere,
  every fixture facing it, and the same fixture counts. A shape is new
  geometry, not new rules.
- **The shape, not its box, is the ship.** Treating the bounding box as the
  footprint would have stamped hull or floor over a hub's empty corners and
  made the margin a rectangle far from most of the hull. Everything that
  touches the ground (stamping, the crater, the site checks, the browser's
  preview and overlap check) walks the shape's own tiles and the ring round
  them.
- **The stick stays the fallback.** It is the slimmest shape and the only one
  guaranteed to fit wherever a ship fits at all: a 19×16 hub does not fit in
  the lower half of a 40×24 map. The mechanics tests' `testConfig` pins
  sticks (`1/0/0`) for the same reason, and so that a new shape does not
  shift every mechanics test's landing site. The shape tests opt in.
- **Founders wait aloft in the browser; they are not landed and then moved.**
  The browser used to let worldgen land everyone and then let the player move
  ships. That left the player tidying up a layout the game had chosen. With
  `place-ships`, nothing is on the map until the player puts it there, and
  the TUI, headless runs, the director, and the tests still share `land`.
  Landing one at a time, in order, is also what makes the preview honest:
  each ship's plan is fixed only once the ships before it are down.
- **Why `LandShip` names the ship.** A click lands "the next ship", and the
  topic that says which ship is next lags a command by one publish. Naming
  it makes a double click a no-op instead of landing the following ship on
  the same spot.
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
- **A new room**: another room from `shipRooms`, placed by each planner (a
  new stick segment, a spoke, or more knobs). Every fixture must face open
  deck. Re-check `TestShipLayout` and `TestShipsLeaveRoomForTheFirstRooms`.
- **A new shape**: a `shipShape`, a planner that draws segments on a
  `shipCanvas` and lists the step-out tiles, a weight in `sim.Config`, and a
  case in `planShip` and `shipShapeFor`. `TestShipLayout` then checks it
  (fixtures face open deck, enough step-out tiles, every bit of deck
  reachable from a door), and the browser needs nothing: it draws whatever
  `shape` rows the topic sends.
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
- [zoning.md](./zoning.md): why the ground round a ship is always residence.

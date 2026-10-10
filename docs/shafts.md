# Shafts

> Part of the [mars-sim documentation](./README.md).

## What it is

A shaft is the second way between levels, after the stair: a laddered column
straight down, at one (x, y), through as many levels as it has been dug. It
is cheaper to dig than a stair (one column, no landing, and many levels in one
job) and slow to use: every level climbed takes `shaft-climb-ticks` (a stair
takes one), longer with the climber's hands full, and an alien whose build
has no arms cannot climb one at all. Searches weigh the climb, so the colony
walks to a stair when it has one nearby and climbs when the stair is far.
This is phase Z2 of [z-levels.md](./z-levels.md); the stair it builds on, and
how every search crosses levels, is [stairs.md](./stairs.md).

## Source

- [`internal/sim/shafts.go`](../internal/sim/shafts.go): digging
  (`canDigShaft`, `digShaft`, `shaftBottom`), the order and planner
  (`OrderShaft`, `planShafts`, `designateShaft`, `finishShaft`), and climbing
  (`canClimb`, `laden`, `climbTicks`, `climbCost`, `crossesShaft`,
  `startClimb`, `climbing`).
- [`internal/sim/layered.go`](../internal/sim/layered.go): `links` (every
  vertical link, with its cost), `shaftCost`, `hasLinks`, `hasShafts`.
- [`internal/sim/stairs.go`](../internal/sim/stairs.go): `trackLinks` (the
  sorted `stairs` and `shafts` lists) and `travelEstimate` through both.
- [`internal/sim/flowfield.go`](../internal/sim/flowfield.go): the weighted
  rebuild (Dial's buckets in `later`) and `followField`'s shaft rules;
  [`flowrepair.go`](../internal/sim/flowrepair.go) falls back to a rebuild
  while a shaft exists.
- [`internal/sim/path.go`](../internal/sim/path.go): A\* weighs a shaft by
  the mover's climb (`pathfinder.climb`).
- [`internal/sim/systems.go`](../internal/sim/systems.go): `step` spends a
  climber's turn on the ladder; `travelTo` sets the climb and never passes
  a crowd across a shaft. `canReach` (shafts.go), which a hunter's
  `preyInRoom` asks (behaviors.go), keeps a non-climber to its level.
- [`internal/sim/shafts_test.go`](../internal/sim/shafts_test.go): the
  column and its links, the digging rules, fields against a reference
  Dijkstra, a climb taking its turns, laden climbs, shaft against stair
  routing, alien builds, no passing through a crowd, an ordered shaft in a
  real colony, a miner climbing down to work, and determinism with save and
  load.

## How it works

### The column

| Tile | Where | Links |
| --- | --- | --- |
| `ShaftTop` | the level the shaft starts from | down, if the tile below is `ShaftMid` or `ShaftBottom` |
| `ShaftMid` | every level the shaft passes through | up and down, to the shaft tiles there |
| `ShaftBottom` | the shaft's foot | up, if the tile above is `ShaftTop` or `ShaftMid` |

All three are walkable, so a mover stands on any of them like floor, and on a
`ShaftMid` it can get off at that level. `w.links(p)` is the one definition of
a link for stairs and shafts alike: it returns up to two `vlink`s (a
`ShaftMid` has two), each with the tile at the other end, its cost in steps (1
for a stair, `shaftCost` for a shaft) and whether it is a shaft. Links are
symmetric, so a shaft is a region link like a stair and joins the levels it
touches into one room.

`w.shafts` lists every shaft tile that can lead down (`ShaftTop`,
`ShaftMid`), sorted, beside `w.stairs`; `setTerrain` keeps both in step
(`trackLinks`). `hasLinks` (either list non-empty) gates every search's link
lookups, and `hasShafts` gates the weighted machinery, so a game without a
shaft pays nothing for any of it.

### Digging one

`OrderShaft{Levels, At}` with a tile (the browser's Dig tab: the Shaft
tool and its levels box, then a click on the map) is checked and marked out
at once, or refused in the log: open floor starts a new shaft, a shaft's
top deepens it, cut short at `deepest-level` (see [siting.md](./siting.md)).
Without a tile (`b` then `n` in the terminal, one level per press) it needs
`siting-auto` and adds to `manualShaftLevels`. `planShafts` runs with the stair planner: it
deepens the deepest shaft the colony can reach by that many levels, or, with
none to deepen, starts a new one where a stair would go (`findStairSite` on
the deepest level reached). The order is spent once a shaft is marked out.

A shaft is one task (`buildTask.depth` levels below its tile), mining work,
taking `shaft-ticks` for every level still to dig, and free of materials, as
a stair is. `canDigShaft` wants the top to be known open floor (a new shaft)
or a shaft's top (deepening it), the foot no deeper than `deepest-level`, and
every tile the column cuts through to be rock or floor: a shaft never cuts
through a wall, a fixture or a stair. `digShaft` works bottom up, making each
level it reaches and revealing around each tile it breaks into (caverns and
their nests included), then turns the old foot into a `ShaftMid` or the top
tile into the `ShaftTop`. A shaft task is done when its top's column reaches
its bottom (`shaftTaskDone`), since deepening leaves the top a `ShaftTop`
throughout.

### Weighing the climb

- **A\*** adds a shaft link's cost to `g`; `travelTo` sets the mover's own
  climb first (`climbCost`): the usual climb, the laden climb, or -1 for a
  mover that cannot climb, whose search leaves shafts out. The heuristic
  still counts one step per level, a lower bound on any link.
- **Flow fields** are a breadth-first search, which cannot weigh an edge. A
  shaft link's far end waits in a ring of buckets (`flowField.later`) for
  the distance it arrives at and joins the queue when the search's layers
  reach it: Dial's algorithm, cheap because every cost is a small integer.
  A cell is stamped when it joins the queue, so its first stamp is its least
  distance, exactly as in the plain search. With no shaft the buckets are
  never touched.
- **Repair** assumes unit steps, so while a shaft exists `repair` declines
  and the field is rebuilt (see "Why").
- **`followField`** judges each candidate by its field distance plus what
  getting there costs beyond a step, so it takes a shaft only when the
  shaft is on a shortest way.
- **`travelEstimate`** chains through stairs and shafts, adding each link's
  cost, so "nearest" choices know a shaft is a climb.
- **The facility search** that walks down the field from a colonist to the
  nearest facilities follows a link only where the field drops by exactly
  its cost.

### Climbing

`moveEntity` starts a climb whenever a move goes straight up or down between
two shaft tiles (`crossesShaft`): the mover is on the ladder until
`climbUntil`, `climbTicks` after the move (the move itself is the first
tick). `step` spends each of those turns climbing: the entity is `Climbing`,
does nothing else, cannot fight back or flee, and a colonist's drives go on
as they would walking. An alien beside either end gets its free bites.

A climb starts from one end and lands on the other. Movers normally pass
through a crowd in one turn (see `travelTo`), but never across a shaft:
`travelTo` stops its scan at a shaft crossing that is taken or that it could
only reach through someone, and `followField` takes a shaft link only from
where the mover stands and never as a way through. So a crossing is always
from end to end, and `crossesShaft` is always right about it.

### Who climbs

`canClimb`: colonists, cats and rats; chickens do not; an alien does if its
species has arms (`AlienSpecies.Arms > 0`), so a legs-only or winged build
cannot (no species flies yet). A non-climber's routes leave shafts out, and
it hunts only prey on its own level, since rooms join across shafts it
cannot use.

A climber carrying more than `shaft-carry` bulky goods (rock, ore, ice,
clay, carcasses) is laden and takes `shaft-laden-climb-ticks` a level.

## Why it is this way

- **A laden climb is slow, not forbidden.** The plan said a climber "cannot
  carry anything that would not fit in its hands". Forbidding it means a
  miner who fills its pockets below a shaft cannot get its ore home, and
  then cannot get itself to food either, since the shared fields know
  nothing about what one colonist carries. Every way out (dropping goods,
  capping loads by route, per-load fields) was more machinery than the
  phase is worth before there is a winch to haul with (a later fixture). A
  slow laden climb keeps the intent (shafts are bad for hauling, so the
  colony prefers stairs, and a deep mine wants a winch) without stranding
  anyone.
- **Repair falls back to a rebuild.** Incremental repair reasons about
  distances changing by one step at a time; a weighted edge breaks that,
  and getting it right would double `flowrepair.go`. Rebuilding is always
  correct, and only games that have dug a shaft pay. If shaft games get big,
  this is the first thing to revisit.
- **Clear the field's scratch queue.** A rebuild seeds from facility maps in
  map order. Its queue was left holding that order, and a saved world
  encodes its buffers, so two identical worlds saved differently once
  shafts made rebuilds frequent (`TestShaftRunsAreDeterministicAndSave`
  caught it). The queue is now emptied after every rebuild; its contents
  never mattered.
- **One task for the whole column.** Digging level by level would need the
  digger to stand beside the foot on each new level, which is rock until
  someone mines it out. Cutting the column from the top in one job is what
  "cheaper to dig" means, and deepening works from the top for the same
  reason.
- **The climb happens after the move.** The mover lands on the far end and
  then spends its climb there. Holding it at the near end until the climb
  is done would need a "between levels" position nothing else understands;
  this way occupancy, the chunk index and every search see an ordinary
  tile, and the time is the same.
- **What a game without shafts pays.** The per-turn `climbing` test, a
  range test in `Walkable` instead of two comparisons, and `hasShafts`
  checks before the weighted paths. Against the Z1 build, interleaved, four
  runs each of the step, room-refresh, need-seeking and pathfinding
  benchmarks: no difference outside this machine's noise.
- **Some approximations.** The HPA\* corridor routes over the region graph,
  which does not weigh links, so a long route may be corridored through a
  shaft and then cost more than it had to. The second facility search
  counts nodes, so a shaft looks as near as a stair there (the field it
  then follows does weigh it). A non-climbing alien's own-level hunting
  misses prey reachable by stairs down and back up. None changes what is
  reachable; each only makes a choice slightly worse.

## Extending it

- A new mover kind needs a line in `canClimb`.
- A new search that expands neighbours must ask `links` (behind
  `hasLinks`) and add each link's `cost`, or treat it as one step only if it
  never needs a shortest path.
- A winch (the haulage the plan wants) would change `laden`, or let goods
  cross a shaft without a carrier.
- Holes are one-way, so they are not links; a ladder fitted into one makes
  it a shaft through `canDigShaft` (see [holes.md](./holes.md)).

## Related

- [stairs.md](./stairs.md): the stair and how every search crosses levels.
- [layers.md](./layers.md): levels, `Point.Level` and layers.
- [z-levels.md](./z-levels.md): the plan; this is phase Z2.
- [pathfinding.md](./pathfinding.md): rooms, A\*, HPA\* and flow fields.
- [frontend-tui.md](./frontend-tui.md): the terminal's controls.

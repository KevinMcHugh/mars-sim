# Construction projects

> Part of the [mars-sim documentation](./README.md).

## What it is

The colony builds structures as **projects** it plans as a group, rather than one
colonist at a time. A project is a set of tile designations (`buildTask`s); any
number of colonists each claim and build individual tasks, so a room goes up
collaboratively and in parallel. Facility rooms (bays of nutrient pods and
toilets) are the first — and currently only — project kind.

> **This supersedes the top-level README's "No built walls" section.** Facility
> rooms are now **fully walled with a placed-wall perimeter and a doorway**,
> built in phases. That older design was abandoned; see *Why it is this way*.

## Source

- [`internal/sim/project.go`](../internal/sim/project.go) — `buildTask`, `project`, planning, room geometry, task claiming.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `jobBuild`, `assignTask`/`assignBuild`, the emergency-build fallback.

## How it works

The normal planner maintains life-support, bunk, and incinerator capacity
automatically, but the TUI can queue explicit room orders: `b` opens a menu,
then `f` requests one facility room, `d` one dormitory, `t` one trash room, and
`r` one storage room. Each order just increments a counter
(`manualFacilityRooms`/`manualDormitories`/`manualTrashRooms`/
`manualStorageRooms`) recorded on the engine-owned world, so several can be
queued at once; `planRooms` works
through them — one new project per call, life support before dormitories —
whenever the colony is below its current concurrent-project cap and a suitable
site exists (see *Planning cadence* below).

### Tasks, phases, projects

A `buildTask` converts one tile to a desired `Terrain`, and carries an `owner`
(the colonist building it, 0 if unclaimed) and a `phase`. A `project` is a named
list of tasks. **Phases order the work**: all tasks in the active (lowest
unfinished) phase can be built in parallel, and the project only advances to the
next phase when the current one is done. A room raises its whole perimeter (phase
0) before any facility (phase 1) comes online.

`claimNearestTask` hands a colonist the nearest reachable, unclaimed, workable
task in the active phase across all projects (reachability is an O(1) room check;
see [pathfinding.md](./pathfinding.md)). `jobBuild` travels to the tile and raises
the structure over `buildTicks` (scaled by the colonist's `workScale`).
`clearJob` releases the claim if the colonist abandons the task.

`buildTiles` is a set of every not-yet-built task tile in an active phase, rebuilt
each tick (`rebuildBuildTiles`). Colonists **route around** these tiles and never
idle on them, so a facility mobbed by its own neighbors can still be raised —
otherwise a builder could never reach the tile.

### Room recipes and dormitories

All rooms share the same wall-and-doorway shell. A `roomRecipe` supplies the
name, facility sequence, minimum useful size, and planning log message. The
current recipes are:

| Recipe | Contents | Minimum size | Planning priority |
| --- | --- | --- | --- |
| facility room | alternating nutrient pods and toilets (all toilets with `infinite-food` off, when pods feed nobody — see [food.md](./food.md)) | 2 facilities (1 when all toilets) | first, because food is fatal |
| dormitory | beds/bunks | 1 bed | after the desired pods and toilets exist |
| trash room | an incinerator | 1 incinerator (and at most 1, via `maxFac`) | last, and only once there is refuse to burn |
| storage room | one storage container | exactly 1 container via `maxFac` | player-ordered only |

`planRooms` checks each recipe's planned-or-built capacity, plans at most one
new room per call (see *Planning cadence*), and always chooses a life-support
room before a dormitory, and either before a trash room. Unlike the others, the
trash room's demand is not a headcount ratio: the colony wants exactly one
incinerator, and only once `refuseTotal() > 0` (see
[sanitation.md](./sanitation.md)). A dormitory can therefore be built in a cramped
cavern with a single bunk, and the colony adds more rooms — and, once the
population justifies it, more of them at once — as it grows.

Capacity counts private fixtures too: each settler's crash pod brings its own
bunk and toilet (see [crash-pods.md](./crash-pods.md)), and `plannedFacilities`
counts them, so a young colony builds no dormitories and few toilets until its
population outgrows its pods. Nutrient pods are never in a crash pod, so the
first facility room still goes up for the safety net.

Storage rooms are player-placeable and also demand-planned when a full
colonist has no reachable chest that can accept its complete material load.
Storage then outranks dormitories and may temporarily exceed the concurrent
project cap, because full builders otherwise cannot excavate active projects.
See [storage.md](./storage.md).

Beds use the same facility machinery as pods and toilets: a colonist approaches
an adjacent tile, spends the sleep need's `UseTicks` sleeping, and then resets
the need. A bunk is not walkable and has no permanently assigned owner; capacity
is represented by the number of `Bed` tiles, with the normal access and
crowd-flow rules deciding who can use one next.

### Facility-room geometry

A room is a row of facilities inside a complete placed-wall perimeter with a
one-tile front doorway. `designateRoom` lays out, into two phases:

- **Phase 0 (walls)**: the full rectangle of `Wall` tiles — back, sides, and front
  — except the centered front doorway.
- **Phase 1 (fit-out)**: `roomKinds` facilities (`NutrientPod`/`Toilet`,
  alternating so every room serves both needs) one tile inside the back wall,
  spaced one tile apart.

`findRoomSite` / `roomSiteClear` pick a site whose rear wall is backed by
solid rock or another room's already-placed wall, and whose two side walls are
each either freshly built (with an exterior lane kept clear beside it so every
wall task stays reachable even after its neighbors go up) or an already-placed,
unclaimed wall from a neighboring room — in which case the two rooms sit flush
and literally **share that one tile** as a party wall: this room adds no wall
task of its own there (`designateRoom` skips it), and needs no exterior lane
on that side either, since there is no wall task to reach. Sites nearest the
map center are preferred.

Sharing a boundary this way — on the back wall or a side wall — matters once a
cave's easy rock-backed edges are used up: rooms reuse floor and structure
that already exist instead of every new room needing its own fresh niche cut
from untouched rock.

The interior may be clear floor or still-solid rock (see *Excavating the
interior*), but `findRoomSite` tries a fully pre-cleared site first and only
falls back to a rock-interior one if no clear site exists anywhere — a clear
site finishes strictly faster (no dig phase), and picking a rock one just for
sitting slightly closer to map center measurably delayed food in testing (a
colonist starved because life support landed somewhere slower to finish than
it needed to be; see `findRoomSitePrefersClearOverRockNearCenter` for the
regression test).

Facilities stay spaced one tile apart because a colonist using a facility stands
on its neighbor tiles — two adjacent facilities would mean one could never be
built or used.

### The doorway tile is reserved forever, not just guaranteed once

The permanent doorway (see *Why it is this way*) only ever protected a room
from trapping **its own builders** while it went up: the exterior tile
directly outside the door is deliberately left as pre-existing floor with no
task of its own, so nothing marked it as load-bearing to anything else. In a
maturing colony, `findRoomSite` prefers sites nearest the map center — exactly
the direction an older room's door faces as the colony fills in around it — so
a later, unrelated room could legitimately back onto or sit flush against an
older room's wall (an intentional feature; see the party-wall sharing above)
while its own wall or facility row landed squarely on that older room's one
exit tile, sealing it shut behind a wall its own doorway was supposed to make
impossible.

`w.doorTiles` closes this: `designateRoom` reserves each room's door-exterior
tile the moment the room is designated, permanently (rooms are never
demolished or un-designated, so entries are only ever added), and
`roomSiteClear` rejects any candidate site whose own interior or side-wall
footprint would cover one. See `TestRoomSiteClearRejectsCoveringAnotherRoomsDoorway`.

This closes the single-tile case, not every way a room can end up sealed: a
big enough colony can still enclose a pocket where no individual room's wall
ever lands on another's reserved doorway tile — three rooms built around a
shared middle, say, none of them individually at fault. Enumerating every path
in to protect it does not scale, so that general case is handled downstream
instead, as a detect-and-correct backstop rather than up-front prevention: see
[escape.md](./escape.md).

### Excavating the interior

A room's interior (where its own back/front walls, clear rows, and facilities
go) need not be pre-mined. `designateRoom` gives every interior tile still
solid rock a `roomDigPhase` task (terrain `Floor`, phase `-1` — before
`roomWallPhase`), so the project excavates its footprint before raising walls.
Dig tasks share the ordinary claiming/reachability rules: a tile is claimable
only once some neighbor is already floor, so they naturally clear from the
edge inward as each newly-opened tile makes the next one reachable — no
special ordering logic needed, and the always-floor exterior lanes and front
approach guarantee at least one initially-reachable tile to start from.

A dig task and a wall or facility task can target the same position (excavate
it, then build on it). `jobBuild` treats `BuildKind == Floor` as a dig: it
requires the tile to still be `Rock` (rather than `Floor`, as every other
`BuildKind` does) and takes `MineTicks` (rather than `BuildTicks` /
`FacilityBuildTicks`), granting `RawRock` like ordinary mining on completion.
`taskDone` treats a dig task as done once its tile is merely *no longer*
`Rock` — not only when it equals `Floor` exactly — so a later wall or facility
task converting that tile past `Floor` still counts the dig as finished rather
than leaving it permanently unsatisfiable and stuck blocking the project's
phase from ever advancing.

This does not let a room conjure itself out of nowhere: the exterior (side
walls, their lanes, the front approach) must still already be floor, exactly
as before — only the interior can be rock.

### Planning cadence

`planRooms` runs every `planInterval` (16) ticks from `step`. It creates at
most one room project per call, and only when the colony is short of a
required room facility for its headcount (`desiredFacilities` = colonists /
`ColonistsPerFacility`, min 1) **and** the colony is below its current
concurrent-project cap. `plannedFacilities` counts existing + in-progress
(from the job board's O(1) counter) + designated-but-unbuilt facilities, so the
colony converges on the target instead of every idle colonist starting one at
once. `planRoom` prefers a full 4-facility room but falls back to the recipe's
minimum when only a shorter clear run is available.

`maxConcurrentProjects` scales that cap with population (one room at a time up
to `concurrentProjectColonists` (8) colonists, then one more room per that many
again), up to the `MaxConcurrentProjects` ceiling in `Config`. A tiny colony
still gets the original single-project behavior — splitting a handful of
builders across two sites is what gridlocks an early, cramped cavern — while a
larger one can run more crews in parallel so facility supply keeps pace with
growth instead of queued orders piling up behind one room at a time.

### Helping build instead of just queueing

An urgent colonist does not automatically queue at a reachable existing
facility. First it checks whether the colony still wants more of that
facility than it currently has planned or built
(`plannedFacilities(kind) < desiredFacilities(...)`); if so, and a reachable
project task is claimable, it helps build instead — only falling back to the
existing facility if no such task is available. Without this check, once a
single facility of a kind exists, every urgent colonist takes the simple path
of queueing at it, and none is ever free to help build a second: the colony
gets stuck at whatever capacity it happened to build first, no matter how far
behind population growth that falls.

Claiming here uses `claimNearestTaskProviding(pos, id, kind)`, **not** the
unrestricted `claimNearestTask` — it only claims from a project that actually
provides the needed facility kind somewhere in its task list (any task in
that project counts, not just the facility tile itself: helping dig or wall a
life-support room still counts as helping provide its pods and toilets).
This matters because the starvation grace period (see [needs.md](./needs.md))
only covers reachable construction that provides the specific facility a
colonist needs; claiming just any reachable task — digging an unrelated
dormitory while starving, say — still marks the colonist as "handling" its
need (so nothing else preempts it) without the grace period actually applying,
so it takes starvation damage the whole time it is "busy." That's exactly
what happened in testing once excavation gave projects many more claimable
tasks to keep a colonist perpetually occupied on the wrong one. Restricting
the claim to relevant projects makes helping build safe even for a fatal
need, as intended — it never trades a build for a death.

### The emergency fallback

Separately from projects, a colonist facing an urgent need — fatal or not —
with no reachable facility, no project task to help with, and no reachable
facility construction may `assignBuild` a lone facility at the nearest
suitable edge (`findBuildSpot`), rather than waiting indefinitely (fatal) or
mining until it starves (non-fatal). This is still guarded: only when nothing
reachable is already being built (`reachableFacilityConstruction`) — checked
first against the shared project pool — so a project in a disconnected room
does not suppress a stranded colonist's self-rescue, and a colonist that can
already help build one elsewhere in its room joins that instead of starting a
redundant one of its own.

## Why it is this way

The room design is the product of watching colonies starve around earlier ones:

- **One room at a time — for a small colony.** A second concurrent project
  splits builders across two sites and, in a tight early cavern, gridlocks the
  colony so nothing finishes and no one mines for space. One room keeps most
  colonists mining (growing the cavern) while a small crew finishes the
  current room. `maxConcurrentProjects` only raises the cap once population
  growth means a second crew is no longer the whole colony's worth of
  builders.
- **Phased walls, then facilities.** The earlier wall-less and single-phase
  designs each starved the colony a different way: an enclosed room trapped its
  own builders; a free-standing wall funneled seekers through its last unbuilt
  gap and deadlocked the crowd; a wall tile next to a facility could never be
  built because a user always stood on it. Raising **every wall before any
  facility** means no facility user ever stands on a pending wall tile, and a
  **permanent doorway** means the last wall can never trap the builders. This is
  what replaced the README's old "no built walls" rule; real walls became viable
  once colonists could pass through crowds and route around pending build tiles.
  That guarantee only ever covered a room trapping itself while it went up —
  see *The doorway tile is reserved forever, not just guaranteed once* above
  for the later, cross-room version of the same failure and how it's closed.
- **Rock-backed niches, or a shared wall with a neighbor,** keep a room from
  becoming a free-standing obstacle that splits an open route, and mean the
  back wall's tasks are reached from the future facility row.
- **Reachability-gated claiming and the tightly-guarded emergency build** keep
  the colony from either mobbing one site or letting a disconnected colonist die
  next to an unreachable project.
- **Life support before dormitories** makes the planner's priorities explicit:
  sleep improves quality of life, but missing a bed is not fatal, while missing
  food is. This is strict: even a planning cycle where life support fails to
  find a site never falls through to a dormitory instead. That fallthrough was
  tried and reverted — it let dormitories win a scarce concurrent-build slot
  ahead of life support and measurably delayed food. A colony that cannot site
  life support at all is a siting problem to fix (excavation, wall-sharing),
  not a priority order to bend.
- **Preferring a clear site over a rock one** (in `findRoomSite`) for the same
  reason: a rock-interior site is strictly slower to finish (it has a dig
  phase the clear one doesn't), so picking one merely because it happens to be
  closer to map center can delay a room's completion for no good reason.
- **The travelTo/stuck interaction.** `jobBuild`'s "someone is on my tile,
  wait, then give up after `StuckLimit`" safety valve only works because
  `travelTo`'s "already adjacent" fast path stopped resetting `e.stuck` to 0
  every tick (it used to, via `clearPath`). Before that fix, two colonists
  that ended up swapped onto each other's wall-task tiles — much likelier once
  excavation crowds a freshly-dug interior with diggers who then all start
  building walls at once — could deadlock forever: the timeout counter never
  survived a tick to accumulate, so it never fired. This starved a colonist in
  testing on a large, mature colony.

### Known soft spot

A fully mined-out map is one weak point: with nothing left to dig, the whole
idle population mobs the few facilities and a colonist can occasionally be crowded
out over a long run. This is a shared-facility crowd-flow limit, not a
room-building one, and is moot once maps are larger than the colony can exhaust or
colonists have other work. In practice it shows up sooner than "fully
mined-out" implies: a colony that has simply outgrown its current facility
count queues visibly at whichever bathroom or pod is nearest, well before the
map itself runs out of rock. Growing facility supply faster (a lower
`ColonistsPerFacility`, or a planner less conservative about concurrent
projects) reduces how often it bites; it is not yet fixed at the crowd-flow
level itself.

## Extending it

- **A new room recipe** is a `roomRecipe` plus a demand check in `planRooms`.
  Reuse the wall shell and make the recipe's facility spacing/access rules
  explicit.
- **A new project kind** (storage, workshops, ...) is a new **task generator** over
  the same machinery: produce a `project` with phased `buildTask`s and append it
  in a planner. Claiming, building, `buildTiles`, and pruning all work unchanged.
- **Reserving build tiles from through-traffic** (a movement model that keeps a
  crowd off a builder's tile more strongly) is the natural next step for more
  ambitious structures.

## Related

- [entities-and-ai.md](./entities-and-ai.md) — the build job and need-driven building.
- [pathfinding.md](./pathfinding.md) — room reachability and routing around build tiles.
- [needs.md](./needs.md) — why facilities exist and how many the colony wants.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the job board's in-progress counters.
- [escape.md](./escape.md) — what a colonist does when it ends up sealed off anyway.

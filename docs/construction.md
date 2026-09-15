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

The normal planner maintains life-support and bunk capacity automatically, but
the TUI can queue explicit room orders: `b` opens a menu, then `f` requests one
facility room and `d` requests one dormitory. Each order just increments a
counter (`manualFacilityRooms`/`manualDormitories`) recorded on the
engine-owned world, so several can be queued at once; `planRooms` works
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
| facility room | alternating nutrient pods and toilets | 2 facilities | first, because food is fatal |
| dormitory | beds/bunks | 1 bed | after the desired pods and toilets exist |

`planRooms` checks each recipe's planned-or-built capacity, plans at most one
new room per call (see *Planning cadence*), and always chooses a life-support
room before a dormitory. A dormitory can therefore be built in a cramped
cavern with a single bunk, and the colony adds more rooms — and, once the
population justifies it, more of them at once — as it grows.

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

`findRoomSite` / `roomSiteClear` pick a site whose footprint is clear floor, whose
rear wall is backed by solid rock (so the room is a niche at the cavern edge, not
a free-standing obstacle), and which keeps exterior lanes beside the side walls
and across the front so every wall task stays reachable even after its neighbors
go up. Sites nearest the map center are preferred.

Facilities stay spaced one tile apart because a colonist using a facility stands
on its neighbor tiles — two adjacent facilities would mean one could never be
built or used.

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
- **Rock-backed niches** keep a room from becoming a free-standing obstacle that
  splits an open route, and mean the back wall's tasks are reached from the
  future facility row.
- **Reachability-gated claiming and the tightly-guarded emergency build** keep
  the colony from either mobbing one site or letting a disconnected colonist die
  next to an unreachable project.
- **Life support before dormitories** makes the planner's priorities explicit:
  sleep improves quality of life, but missing a bed is not fatal, while missing
  food is.

### Known soft spot

A fully mined-out map is the one weak point: with nothing left to dig, the whole
idle population mobs the few facilities and a colonist can occasionally be crowded
out over a long run. This is a shared-facility crowd-flow limit, not a
room-building one, and is moot once maps are larger than the colony can exhaust or
colonists have other work.

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

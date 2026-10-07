# Z-levels (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

The plan for giving the colony somewhere to go **down**: a stack of
underground levels, each its own 2D grid, joined by **vertical links**:
stairs, shafts and holes. Depth is meant to be the game's main lever for
risk and reward. Each level down holds richer deposits and bigger caverns, and
it holds more and worse aliens. The colony chooses when to dig down, and every
way down is also a way up for whatever lives there.

Up is a direction too. Colony ships land on **level 1**. Level 0 above it is the
Martian **surface**, which has its own challenges and is planned separately
(Z6).

This is a design and a build plan. Z0 to Z3 are built (see
[layers.md](./layers.md), [stairs.md](./stairs.md),
[shafts.md](./shafts.md) and [holes.md](./holes.md)), and the rest is not.
Where the build departed from this plan, the plan below says so. When a phase
ships, its content moves into a present-tense doc and the table links to it.

| Phase | Status |
| --- | --- |
| Z0 — `Layer` split, one level | **Shipped** — [layers.md](./layers.md) |
| Z1 — Stairs and a second level | **Shipped** — [stairs.md](./stairs.md) |
| Z2 — Shafts | **Shipped** — [shafts.md](./shafts.md) |
| Z3 — Holes | **Shipped** — [holes.md](./holes.md) |
| Z4 — Depth gating (challenge and reward) | Proposed |
| Z5 — Browser frontend and wire format | Proposed |
| Z6 — The surface (level 0) | Proposed; needs its own doc |

## Source

Z0 and Z1 are in [`layer.go`](../internal/sim/layer.go),
[`layered.go`](../internal/sim/layered.go) and
[`stairs.go`](../internal/sim/stairs.go); see [layers.md](./layers.md) and
[stairs.md](./stairs.md). The code the remaining phases change:

- [`internal/sim/layer.go`](../internal/sim/layer.go): every `w.landing()`,
  each one a decision that something happens only on the landing level.
- [`internal/sim/layered.go`](../internal/sim/layered.go): `links`, the
  one definition of what connects two levels.
- [`internal/sim/geom.go`](../internal/sim/geom.go): `Point`, `neighbors8`.
- [`internal/sim/rooms.go`](../internal/sim/rooms.go),
  [`path.go`](../internal/sim/path.go), [`hpa.go`](../internal/sim/hpa.go),
  [`flowfield.go`](../internal/sim/flowfield.go),
  [`flowrepair.go`](../internal/sim/flowrepair.go): navigation.
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go),
  [`worldgen_chunks.go`](../internal/sim/worldgen_chunks.go),
  [`caverns.go`](../internal/sim/caverns.go): lazy generation and breaching.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go),
  [`tilegrid.go`](../internal/sim/tilegrid.go), `internal/wire`,
  `internal/ui/tui`, `web/src/map`: what a frontend sees.

## How it works

### The model: stacked layers, not a 3D grid

A level is a `Layer`: one full-size 2D grid plus every structure that only
makes sense inside one grid. `World` holds `layers []*Layer`, indexed by level:

| Level | What it is |
| --- | --- |
| 0 | **The surface.** Open Martian ground above the rock, with its own hazards (see "The surface" below). Nobody starts here. |
| 1 | **The landing level.** Colony ships come down here, and it is today's whole map. |
| 2+ | **The deeps.** Each level further down is richer and more dangerous. |

A level nobody has broken into has a nil `Layer` and costs nothing. Every
level has the same `Width × Height`, and a vertical link always joins **the
same `(x, y)` on two levels**. That one rule keeps the link model trivial:
a link is a column, not a pair of arbitrary points.

Movement inside a level is the existing 8-connected movement. The only way
between levels is a vertical link. Nothing sees, hears, smells or shoots
through a floor except through a link (see Z5 for the exceptions we might
want later).

Rejected: adding `Z` to `Point` and making every search 26- or 10-connected.
That changes every system at once and makes long searches slower. It also buys
the one thing we do not need: free vertical movement anywhere.

### Positions: every `Point` names its level

As built (Z1; see [layers.md](./layers.md)):

```go
type Level int // grows downward

const (
	SurfaceLevel Level = 0
	LandingLevel Level = 1 // where ships land; today's map
)

type Point struct {
	X, Y  int
	Level Level
}
```

Every position (an entity's, a target, a route step, a depot, a map key)
carries its level. `Add` stays on a level; `Adjacent` and `Within` never
reach across one; `lessPoint` sorts by level first.

**Departure from the plan.** This section first proposed `Point` for grid code
and a separate `Loc` (a level and a point) for places that cross levels, and
Z0 shipped that way. Z1 found nearly everything crosses levels: targets,
routes, depots, claims. Converting each between two position types was
hundreds of edits that asked nothing; giving `Point` its level made every one
of them carry it, and made every positional literal a compile error until it
said which level it meant. `Loc` was folded into `Point`.

Numbering the landing level 1 rather than 0 still does what it was for. A
keyed literal that forgets the level lands on the **surface**, where nothing
exists, so the mistake reads as nothing at all rather than silently as the
landing level. `TestNothingIsOnTheSurface` checks a long run for any place on
level 0.

### What moves into `Layer` and what stays on `World`

As built in Z0 and Z1 (see [layers.md](./layers.md)):

| Into `Layer` (per level) | Stays on `World` (shared) |
| --- | --- |
| `tiles`, `snapGrid`, `pageDirty`, `dirtyPages` | `entities`, `nextID`, `kindEntities` |
| `occ` (occupancy) | money, order book, plans, prices, haul claims |
| `terrainCounts`, `exploredCount`, `hiddenFloor` | `regions` map and the `nextRegion` counter |
| `refuse`, `goreTotal`, `corpseTotal` | `rooms`, `discoveredRooms`, `mainRoom` |
| `facilityTiles` | flow fields (`fields`, `frontier`): they span levels |
| `carvedAny/Min/Max` | `pf` and every scratch buffer (single-threaded; reuse) |
| `chunkEntities` (spatial index) | `stairs`, the sorted list of stair tops (Z1) |
| `regionOf`, `dirtyChunks` | relationships, memories, director, RNG streams |
| `salt`, `exposedSalt`, `scum`, `exposedScum`, `scumPatches`, `scumThin` | projects (their tasks' points carry levels) |
| the job `board` (frontier, claims, `frontierByChunk`) | the sparse maps keyed by tile: `storageContainers`, `fixtures` and their indexes, `buildTiles`, `doorTiles`, `pantryOf`/`pantryHouse`, `scumClaims`, `workshopClaims`, `unfoundCaverns`, `structureAt`, `roomFloor` |
| worldgen: `gen`, `genDone`, `genSeen`, `genChunks`, `preview` | colony ships (`ships`, `aloft`, `shipRingHint`) and the zone grid, both landing-level only |
| | the published-copy revisions (`saltRev`, `scumRev`, `fixtureRev`) and the market's silo cache |

**Departure:** Z1 first put the sparse tile-keyed maps on `Layer` too. Merging
main's colony ships, zoning and structures moved them back to `World`: a key
is a `Point`, which names its level, so one map serves every level with no
lookup of the layer first, and main's code that reads them needed no port. A
map belongs on `Layer` only when it is indexed by `(x, y)` alone (a grid) or
holds a per-level count or aggregate.

Region IDs stay **globally unique** (one counter on `World`) so the region
graph, and with it rooms, can span levels without renumbering anything. That is
what makes stairs nearly free in the navigation code (below).

### Vertical links

```go
type LinkKind uint8

const (
	LinkStair LinkKind = iota // walkable both ways, at walking pace
	LinkShaft                 // laddered column: both ways, slow, hands full = no
	LinkHole                  // open drop: down only, hurts
)

type vlink struct {
	Kind   LinkKind
	At     Point // the column
	Top    Level // the upper end
	Bottom Level // the lower end (Top+1 for a stair; may be deeper for a shaft or hole)
}
```

**As built (Z1, Z2)** there is no link table: `links` reads every link from
the terrains at both ends (see [stairs.md](./stairs.md) and
[shafts.md](./shafts.md)), and `w.stairs` and `w.shafts` list the tiles that
lead down, sorted. A shaft spanning several levels needed no table after all:
a `ShaftMid` on each level it passes through links up and down, so a span is
a chain of one-level links, each a terrain at both ends. Each end is a
terrain on its level:

| Kind | Upper tile | Lower tile | Who can use it | Cost |
| --- | --- | --- | --- | --- |
| Stair | `StairDown` | `StairUp` | anything that walks | 1 step, same as a floor tile |
| Shaft | `ShaftTop` (and `ShaftMid` on levels it passes through) | `ShaftBottom` | colonists, cats, rats; aliens by build (Z4) | `ShaftClimbTicks` per level, and nothing bulky carried |
| Hole | `Hole` | whatever is below (usually `Floor`; rubble) | anything, **down only** | one step plus fall damage by levels dropped |

All three are **walkable link tiles**: a mover stands on the upper or lower end
like a floor tile, and the link adds one extra neighbor to that tile on the
other level. For a shaft spanning several levels, each `ShaftMid` is a
stopping point. You can get off at any level the shaft passes through.

#### Stairs

The simplest link and the one Z1 ships. A stair is a construction project like
a room: the colony digs the tile on the level below (generating that level's
chunks around it, see "Generation" below) and builds both ends. It is two-way
and costs a normal step, so for navigation a stair is just one more
**region link** between the region holding `StairDown` and the region holding
`StairUp`. `relabelRooms` already walks `region.links`, so a stair merges the
two levels' areas into one room with no other change. `sameRoom` stays O(1)
and stays correct across levels.

#### Shafts

A shaft is a stair with consequences: it is cheaper to dig (one column, no
landing), it can span many levels at once, and it is slow and restrictive to
use. A colonist climbing is in a `Climbing` activity for
`ShaftClimbTicks` per level. While climbing it cannot fight, it cannot carry
anything that would not fit in its hands (no storage-sized hauls), and an alien
at the top or bottom gets a free attack. A shaft is **two-way**, so it also
joins regions into one room, but it carries an edge cost (see navigation) so
the colony prefers stairs when it has both.

Shafts are also the natural place for **haulage**: a winch at the top
(a later fixture) moves goods up and down without a colonist carrying them,
which is what makes a deep mine pay.

**As built (Z2, see [shafts.md](./shafts.md)):** a full load does not forbid
a climb; it slows it (`shaft-laden-climb-ticks`), because forbidding it
stranded miners below with their ore and away from food. Flow-field repair
falls back to a rebuild while a shaft exists. The climb happens after the
move, at the far end, rather than between levels.

#### Holes

A hole is the one **directed** link: you can go down it and not up it. That
breaks the assumption every reachability structure is built on, that "A can
reach B" iff "B can reach A". The decision:

**Holes do not join regions and are not region links.** Rooms stay equivalence
classes and `sameRoom` stays symmetric. A hole is a *one-way edge used only by
behaviors that want it*:

- **Chutes.** Dropping items down a hole is free haulage, but only downward.
  That suits refuse dumped into a pit and supplies (meals, ingots) sent down
  to a deep work camp. It does nothing for ore, which comes up. The hauling
  planner can price a chute drop as a near-free leg. The asymmetry is
  deliberate: getting the reward out of a deep level is the expensive part.
- **Falling.** A colonist (or rat, or gore) shoved, panicking, or standing on
  a tile that *becomes* a hole falls. Damage per level dropped goes to legs
  first, then the rest of the body ([combat.md](./combat.md)). A fallen
  colonist is now in a different room. It is the same situation as being
  walled in, and [escape.md](./escape.md)'s cutoff detection already handles
  "my room is not the colony's main room".
- **Raids.** Aliens on a deeper level can only reach the colony through a
  two-way link. Anything on a *shallower* level can drop in through any hole,
  and above the landing level is the surface. A hole dug in the wrong place
  is a door that only opens inward. A hole up to the surface is also a
  breach in the colony's shelter (see "The surface").

A hole with a ladder fitted becomes a shaft. That is the upgrade path.

**As built (Z3, see [holes.md](./holes.md)):** a hole is not walkable, so
nobody steps on one by accident; a fall is either the floor giving way or a
cornered colonist leaping. Chutes take refuse only, and a fallen colonist is
fetched back by a ladder the colony fits, not by escape. A
hole is what you get by digging straight down without building anything,
or naturally in a cavern (Z4).

### Navigation across levels

The existing two-tier structure does almost all of the work:

1. **Rooms (reachability).** Stairs and shafts are region links, so
   `roomOf`/`sameRoom` answer "can I get there?" across levels with no new
   code. Every job-assignment gate keeps working. Holes are not region links,
   so they never make two places look mutually reachable.
2. **HPA\* (`abstractCorridor`).** The region graph now has cross-level edges.
   The heuristic uses region representatives, so it needs a level term:
   `Chebyshev(xy) + |Δlevel| × minLinkCost`, where `minLinkCost` is 1 (a stair).
   That keeps it admissible. Expansion order stays sorted by region ID.
3. **Tile A\* (`path.go`).** `pfCell` scratch becomes per layer (scratch on
   `World` holding a `[]pagedGrid`, so nothing is allocated for a level that
   no search touches). The neighbor loop gains one case: on a link tile, also
   try the tile at the other end. The cell index used for tie-breaks becomes
   `(level, index)`, compared level first, then the existing order. The search
   is uniform-cost today. Z1's stairs need nothing more; Z2 adds an edge cost
   for shafts (`g += ShaftClimbTicks`) and switches the heuristic as in HPA\*.
4. **Flow fields.** A field is a multi-source BFS from its goals, and a
   colonist on level 3 who needs a toilet should be routed to one on level 1
   if that is the nearest. So fields **span levels**: `flowField.cells`
   becomes one `pagedGrid` per level, and BFS adds the link neighbor the same
   way A\* does. Two complications:
   - Shaft edges cost more than 1. A plain BFS cannot do weighted edges; use
     the standard 0-1-BFS trick generalized to a small bucket queue (Dial's
     algorithm), since costs are small integers. Stairs-only fields (Z1) stay
     plain BFS.
   - **Repair** (`flowrepair.go`) touches a point. Touching a link tile must
     touch the matching tile at the other end too, or a stair that gets walled
     off on one level leaves stale distances on the other.
   Holes are excluded from fields, as from rooms.
5. **`travelTo` and movement.** `moveEntity` across a link updates both
   layers' `occ` and `chunkEntities`, then fires the usual events. A move onto
   an occupied far end waits, the same as any blocked step.

### "Nearest" is no longer a distance

Many decisions pick the nearest thing by `Chebyshev` (64 call sites): nearest
prey, nearest chest, nearest scum, the hauling planner's leg lengths, the
producer's depot choice. Across levels a straight-line distance means nothing.
A tile directly below you might be a hundred steps away. The rule for Z1:

- **Spatial queries are per level.** `nearestMatch`'s chunk-ring search runs on
  one layer's `chunkEntities`. A caller that wants "anywhere" asks the shared
  flow field, or the job board, which already answer in walking distance.
- **Cost estimates across levels** (hauling, the producer, the market's
  depot choice, every "nearest" choice) use `travelEstimate(a, b)`. It is
  Chebyshev on the same level. Across levels it takes the cheapest chain of
  straight lines through the stairs that exist, level by level. Shipped in
  Z1; see [stairs.md](./stairs.md).
- **Perception** ([compositional-perception-and-events.md](./compositional-perception-and-events.md))
  is same-level only. Seeing up or down a shaft or hole is a later refinement.

### Generation

Worldgen is already lazy and per chunk: a chunk is a pure function of
`(Config, cx, cy)`, generated only when exploration reaches it
([worldgen-chunks.md](./worldgen-chunks.md)). As built, each layer has its own
generator and generated-chunk list, so the chunk key did not need a level;
the generator's `level` does the work. `featureRand` mixes `level -
LandingLevel` into every stream **only when it is nonzero**, so level 1
generates bit-for-bit what today's map does and the golden hashes did not
move. Mixing the raw level in would reseed the landing level and change every
seed's world. Scum growth follows the same rule.

Levels 2 and down use today's generator (rock, veins, caverns, passages, scum,
salt) with depth multipliers (Z4). The surface does not: it is open ground,
not rock to dig through, and gets its own generator in Z6.

A level is generated when something first breaks into it. Digging down for a
stair, shaft or hole generates the halo around that column on the level below,
exactly as digging sideways does now. If the column lands in a hidden cavern,
the existing breach (`revealAround`, the nest roll) fires on that level. That
is the moment the next level announces what it holds.

Planning horizons stay inside one level: a cavern plan on level 1 never reads
level 1's plans. Cross-level features (a natural shaft through two levels, a
sinkhole) are owned by the upper level's chunk and plant their lower end as a
plan the lower level reads. It is the same "read plans, never generated
chunks" rule, one level deeper.

### The surface (level 0)

The surface sits above the landing level and is not more of the same. It is
open ground, with no rock ceiling to dig through. Its challenges are
exposure, not monsters in the dark: cold, dust storms, and radiation (a
natural fit for the uranium dose in [mutation.md](./mutation.md)). Its
rewards are whatever the colony cannot get underground, such as sunlight,
ice, or supply drops the [director](./director.md) lands there instead of
inside the colony.

What this proposal fixes about it, so the earlier phases do not paint it into
a corner:

- **It is a `Layer` like any other.** Navigation, links, rooms and the
  frontend's level switching treat it the same way. Only its generator, its
  terrain kinds and its hazards differ.
- **Links go up as well as down.** Breaking out is digging a stair or shaft
  **up** from level 1. The column rule still holds. What the top end opens
  into is an airlock-like fixture, not a floor tile in a cave.
- **Shelter is a property of the landing level.** Today everything on level
  1 is implicitly sheltered. A hole to the surface, or an open stair with no
  airlock, is what lets the surface's hazards in. That makes a hole dug up the
  worst kind of hole.
- **Generation is lazy here too.** Nothing on the surface is generated until
  a link breaks through to it.

Everything else (the hazards, what is worth going up for, whether colonists
can work there unprotected) is for Z6's own doc.

### Determinism

Everything [determinism.md](./determinism.md) asks still applies, with the
level as the leading key:

- `lessPoint` orders by level, then Y, then X.
- `refreshSpatial` walks dirty chunks level by level, in chunk order within
  each. Region IDs come from one global counter, so the order across levels
  matters as much as within one.
- `w.stairs` is kept sorted; every loop over levels (`eachLayer`) runs in
  level order.

The golden test was the safety net for Z0 and Z1: with one level, **every
hash stayed the same** through both. `TestStairRunsAreDeterministic` covers a
run that digs down.

### Frontends

The snapshot gets one `TileGrid` per level the colony has broken into, and a
frontend picks which to draw. Publishing already costs only the pages a tick
touched, so a quiet level costs a page-table copy. Entities carry their level,
and the frontend filters to the viewed level.

- **TUI (shipped in Z1):** `<` and `>` change the viewed level. Once there
  is more than one level, the header shows `landing level` or `level N` and
  the legend shows the stair glyphs (🔽/🔼). Not done: the roster does not
  yet show each colonist's level.
- **Browser (Z5):** the view rectangle the page streams gains a level. Frame
  tile pages are keyed `(level, page)`. The entity section carries a level byte.
  The golden frames both decoders test against are re-pinned once, in Z5.

## Why it is this way

- **Stacked 2D layers instead of a 3D grid.** Every hot path (tile reads,
  A\*, flow BFS, the paged grids) is tuned for one 2D grid. Keeping each level
  a 2D grid keeps all that tuning. Vertical movement only at links is also the
  gameplay we want: links are chokepoints you defend, which is exactly what
  makes depth a gated risk.
- **Same `(x, y)` at both ends.** A link that could join arbitrary points
  would need its own placement rules, its own rendering ("where does this
  stair come out?") and its own generation constraints. A column needs none of
  them.
- **Holes are not region links.** The whole reachability layer, and every job
  gate built on `sameRoom`, assumes symmetry. Letting a one-way edge into rooms
  would make "same room" mean "one of us can reach the other", and every gate
  would need auditing. Keeping holes as opt-in edges for specific behaviors
  costs a little reach in exchange for none of that.
- **Global region IDs, per-layer `regionOf`.** One counter means a stair is
  just another entry in `region.links`, and the room relabeling, HPA\* and
  `sameRoom` work unchanged across levels. Per-level counters would need a
  `(level, id)` everywhere a region is named.
- **Z0 is a pure refactor.** Splitting `World` and giving positions a level
  touches hundreds of lines. Doing it with exactly one level, under unchanged golden
  hashes, separates "did the refactor break anything?" from "do levels work?".
  Mixing them would make every failure ambiguous.

## Extending it

Build order and what each phase must prove:

- **Z0 — `Layer` split, one level. Shipped**: see [layers.md](./layers.md).
  The per-level fields moved onto `Layer`, under unchanged golden hashes. Its
  `Loc` and `w.home` were stepping stones that Z1 replaced.
- **Z1 — stairs and a second level. Shipped**: see [stairs.md](./stairs.md).
  `Point` carries its level; `w.home` is gone and every per-level access names
  its layer; `deepest-level` (default 1) and `stair-ticks` in `Config`; stair
  terrain and the stair project, planned on a player order or when the
  frontier runs out; rooms, A\*, HPA\*, flow fields (and their repair) and
  the facility searches cross stairs; `travelEstimate`; per-level worldgen;
  `<`/`>` in the terminal. Golden hashes and golden frames unchanged. One-level
  games pay about 6% (see [layers.md](./layers.md)).
- **Z2 — shafts. Shipped** ([shafts.md](./shafts.md)): multi-level
  columns dug in one job (`OrderShaft`, `shaft-ticks`), `Climbing` for
  `shaft-climb-ticks` a level, laden climbs past `shaft-carry`
  (`shaft-laden-climb-ticks`), weighted links (Dial's buckets in fields,
  edge costs in A\*, `travelEstimate` through shafts), and alien access by
  build (arms climb). Golden hashes and frames unchanged.
- **Z3 — holes. Shipped** ([holes.md](./holes.md)): directed drops,
  falling and leg-first fall damage, a cornered colonist leaping, refuse
  chutes, ladders turning a hole into a shaft, and a ladder fitted for a
  colonist stranded below (in place of teaching escape to dig up). Supply
  chutes through the hauling planner wait for a deep work camp (Z4); raids
  through holes come with Z4's aliens below.
- **Z4 — depth gating.** Per-level worldgen multipliers in `sim.Config` (ore
  vein density, uranium, cavern size, nest chance and size), species rolled
  per level in [lore.md](./lore.md) with hostility weighted by depth, natural
  shafts and sinkholes in caverns, and aliens climbing up links toward the
  colony. This is the phase the feature exists for. Everything before it is
  plumbing.
- **Z5 — browser.** Wire format, view level, and `web/` rendering.
- **Z6 — the surface.** Level 0's own generator and hazards, and links dug
  **up** from level 1 to break out onto it. Its own doc before it is built.

Invariants every phase keeps:

- Level 1 generation and the one-level game are unchanged (golden hashes).
- Colony ships land on level 1 and nowhere else.
- A vertical link is a column: both ends share `(x, y)`.
- Rooms and `sameRoom` are symmetric: only two-way links are region links.
- No map is ranged to decide anything: links, layers and dirty chunks are
  walked in sorted order.

### Open questions

- **Bounded or unbounded depth?** A fixed deepest level is simpler for
  config, generation and the UI. Unbounded depth needs gating to be a
  formula, not a table. Proposal: fixed, default small (4–6 levels below the
  landing level), formula-driven multipliers so raising the cap needs no new
  tables.
- **Does digging down require building something?** Proposal: yes. Digging
  straight down makes a hole (a one-way drop), so the colony has to choose to
  spend the time on a stair or a shaft.
- **Can aliens use stairs?** Proposal: yes. That is the challenge. Shafts by
  build (small and medium species climb, the biggest do not), holes for
  anything above.

## Related

- [pathfinding.md](./pathfinding.md): regions, rooms, HPA\* and flow fields,
  all of which gain cross-level edges.
- [worldgen-chunks.md](./worldgen-chunks.md): lazy chunk generation, which
  gains a level in its key.
- [caverns.md](./caverns.md): breaching and nests, the model for how a new
  level reveals what it holds.
- [determinism.md](./determinism.md): sorted iteration and the golden
  hashes that guard Z0.
- [sparse-grids.md](./sparse-grids.md): why an unvisited level costs almost
  nothing.
- [escape.md](./escape.md): the cutoff handling a fallen colonist reuses.
- [lore.md](./lore.md): species and temperament, to be weighted by depth.

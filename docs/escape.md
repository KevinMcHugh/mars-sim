# Escaping a sealed room, and passages

> Part of the [mars-sim documentation](./README.md).

## What it is

A detect-and-correct backstop for whatever ends up cut off from the rest of
the colony, however that happened. Room siting avoids cutting the colony in
two (see [construction.md](./construction.md)), but prevention can be beaten,
so there are two corrections:

- **Escape.** A colonist whose room stays disconnected from the colony's main
  network for `EscapeGraceTicks` straight makes its own way out. It takes the
  cheapest route to the main network, digging rock and breaking down walls or
  hull, whichever costs less work, and goes round a structure through the
  rock where that is cheaper.
- **Passages.** A facility cut off with nobody beside it, which no escape will
  ever reach, gets a passage: a planner project along the cheapest route from
  the colony to it, dug and broken through from the colony's side.

## Source

- [`internal/sim/rooms.go`](../internal/sim/rooms.go) — `mainRoom`, the
  discovered room with the most floor tiles, recomputed by `relabelRooms`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) —
  `updateDisconnected`, `assignDemolish`, `jobDemolish`, and `jobBuild`'s
  breaking down of a wall for a passage.
- [`internal/sim/passage.go`](../internal/sim/passage.go) — `passageRoute`
  (the cheapest way through), `escapeTarget`, `fixtureCutOff`, `planPassage`.
- [`internal/sim/focus.go`](../internal/sim/focus.go) — `FocusEscape`
  eligibility and scoring.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `JobDemolish`,
  `Demolishing`, `Entity.disconnectedTicks`.
- [`internal/sim/rooms_test.go`](../internal/sim/rooms_test.go) —
  `TestMainRoomTracksLargestRoom`, `TestUpdateDisconnectedTracksCutoffRoom`.
- [`internal/sim/sim_test.go`](../internal/sim/sim_test.go) —
  `TestColonistEscapesSealedRoom`, `TestColonistDigsOutWhenSealedByRock`.
- [`internal/sim/passage_test.go`](../internal/sim/passage_test.go) — which
  way an escape goes, and `TestColonyDigsAPassageToACutOffFacility`.

## How it works

**`mainRoom`.** `relabelRooms` already knows every room's floor-tile size
(`w.rooms`, kept incrementally — see [pathfinding.md](./pathfinding.md)), and
keeps the largest *discovered* room as `w.mainRoom`, tie-broken by the smaller
`RoomID` for a deterministic result regardless of Go's randomized map
iteration order. This is "the colony's main body" — normally every room the
colony has built, linked through doorways and corridors into one component, so
`mainRoom` is that whole network's ID. Only discovered rooms qualify because an
undiscovered natural cavern is floor too, and can be bigger than the landing
site: were it eligible, every colonist would count as cut off from a cave none
of them has seen (see [caverns.md](./caverns.md)). Discovered rooms are few,
so choosing among them is a trivial scan.

**Detection.** `updateDisconnected(e)` runs in `colonistTurn` every tick,
unconditionally — before the resting/sleeping fast path, the same way
starvation and uranium exposure do — and compares `w.roomOf(e.Pos)` against
`w.mainRoom`. Equal or on non-floor: `disconnectedTicks` resets to 0 (marking
the mind dirty if it had been counting, so a just-reconnected colonist
reconsiders immediately rather than finishing out whatever it was doing while
trapped). Different: the counter increments, and crossing `EscapeGraceTicks`
also marks the mind dirty, forcing full re-arbitration on the very tick
escaping becomes available rather than waiting for the next scheduled think.

**Arbitration.** `FocusEscape` is eligible once `disconnectedTicks >=
EscapeGraceTicks` — and never while a threat is visible, so it can never
compete with `FocusFlee`/`FocusFight` (see *Why it is this way*). It carries
no matching need, so its score is a flat, high `Base`: see the config comment
for exactly what it needs to clear. `chooseFocus`'s normal switch-margin
hysteresis governs how readily an already-escaping colonist keeps at it, same
as any other focus.

**The cheapest way through.** `passageRoute` is a Dijkstra search from a set
of tiles to any open tile in a goal room, costed in ticks of work: 1 to step
onto open floor, `1 + mine-ticks` (7) to dig out rock, `1 + demolish-ticks`
(17) to break down a wall or hull. Facilities, chairs and anything else are
never broken through, and neither are tiles another project will build on.
With the defaults a single wall tile with rock beside it is dug round rather
than broken (one rock tile is 7, the wall 17), while a wall with three rock
tiles' depth round it is broken. The search settles at most
`passageSearchLimit` (16384) tiles and gives up past that. Ties break on
row-major position, so the route is the same every run.

**Execution.** `assignDemolish` calls `escapeTarget`, which runs
`passageRoute` from the colonist to `mainRoom` and takes the first tile on
the route that is not open floor. That tile borders the colonist's pocket, so
it can reach it. `jobDemolish` then travels next to it and clears it to Floor:
a wall or hull over `DemolishTicks`, rock over `MineTicks`, keeping whatever of
the rock's yield fits in its pockets (both scaled by `workScale`). Each cleared
tile widens the pocket, and the next think assigns the next tile, until
`updateDisconnected` sees the colonist in the main room again. If no route
exists within the search limit, `runFocus` falls back to idle behavior for
that tick rather than looping on a target that can't exist.

**Passages.** `planPassage` runs first in every `planRooms` pass, ahead of
the concurrency cap. It looks for a fixture (`w.fixtures`: pods, toilets,
beds, chests, workshops) none of whose neighbors is open floor in `mainRoom`
(`fixtureCutOff`), taking the row-major first. It runs `passageRoute` from the
fixture's neighbors to `mainRoom` and makes a `passage` project: every rock tile
on the route to dig and every wall or hull tile to break down, in one phase.
A dig task records what it clears (`buildTask.clears`, Rock by default), so
`taskDone`, `taskWorkable` and `jobBuild` treat breaking a wall down as one
more kind of dig. Builders claim the tiles from the colony's side inward, as
each cleared tile brings the next within reach, exactly as a room's interior
is dug. A passage is unpaid (issuer `Nobody`), like the colony's first
scumhouse, and there is one at a time.

## Why it is this way

- **Detect-and-correct instead of enumerating every path in.** `doorTiles`
  (see [construction.md](./construction.md)) prevents the single most common
  cause — a later room's wall landing on an older room's one exit tile — but a
  big enough colony can still enclose a pocket with no individual room at
  fault (three rooms built around a shared middle, say). Tracking every tile
  that must stay open to keep every possible pocket connected does not scale;
  checking actual connectivity after the fact and correcting it does, using
  infrastructure (`mainRoom`/`roomOf`) that already runs every tick for other
  reasons. This mirrors the existing emergency-build fallback's philosophy
  (see [construction.md](./construction.md)): prevent what's cheap to
  prevent, and give the colonist a way out of whatever isn't.
- **A flat, dominant `Base` instead of tying escape to need pressure.**
  Reachability is what actually broke, not any one need, so `FocusEscape`
  doesn't score like a need. It is deliberately tuned to clear even a maxed
  pressing fatal need (see the config comment's arithmetic): a starving,
  trapped colonist that could limp along on a facility already inside its own
  sealed pocket is usually better off breaking straight out anyway, since the
  nearest wall is very often the same one separating it from the rest of the
  colony's supply — the thing that was actually failing. Early drafts gave it
  a modest `Base` (just above `FocusWork`) on the theory that the existing
  emergency-build fallback already keeps a trapped colonist alive locally, so
  escaping didn't need to preempt a crisis — `TestColonistEscapesSealedRoom`
  (née `TestColonistStarvesWhenTrapped`, which asserted the *opposite* of the
  now-intended behavior) is a synthetic one-tile pocket with no room for a
  fallback build at all, and exposed the gap: a colonist could be both
  disconnected and fatally stuck with nothing to do, forever. The fallback
  helps often; it isn't a proof that escaping can always wait.
- **Excluded outright when a threat is visible, not merely outscored.**
  `FocusFlee`/`FocusFight` deliberately carry low bases (0 and 15) and only
  grow via affect during an actual encounter (see
  [combat.md](./combat.md)), so if `FocusEscape` simply competed on score
  alongside them, its dominant flat base would win at the very start of an
  encounter, before affect has caught up — a trapped, threatened colonist
  would calmly walk toward a wall instead of fleeing an alien. Making the two
  mutually exclusive by eligibility sidesteps the ordering problem entirely,
  and reflects the actual priority: nothing about a sealed room is more urgent
  than an immediate predator.
- **A bounded search instead of a global scan.** The first escape walked only
  the colonist's own cut-off room to the nearest wall, since a pocket is small
  by construction. Digging out means searching through rock beyond it, toward
  a main room that may be some way off, so the search is costed and capped
  (`passageSearchLimit`) instead: a way out longer than that is no rescue.
- **Rock as well as walls.** Escapes used to break only walls. A colonist
  sealed in by rock was meant to be a separate, pre-existing problem
  ("hasn't explored yet"), and `TestColonistStarvesWhenSealedByRock` pinned
  that it starved. But a colonist cut off by a structure with rock on either
  side is often better off digging round it than through it, and once rooms
  could stand free on open floor that was the common case. Escapes now cost
  rock and walls against each other, and that test became
  `TestColonistDigsOutWhenSealedByRock`.
- **Rubble is left behind on an escape.** A miner with full pockets stops
  digging; a colonist digging its way out does not, so whatever of the rock
  does not fit is lost. It is the one place rock leaves the economy.
- **A passage is a project, not a focus.** Nobody is beside a sealed-off
  facility to notice, so the colony has to. Making it a project reuses the
  claiming, reachability and build-tile machinery, and lets any number of
  builders work it. It is unpaid because what is walled off is already built
  and counted as capacity: nothing the planner builds would replace it, and
  an empty treasury must not leave it walled off.
- **One passage at a time, ahead of every room.** A passage is usually a tile
  or two. Running before the concurrency cap means a colony at its cap still
  reopens what it lost; one at a time keeps a bad patch from flooding the
  project list.

## Extending it

- **Doors, when they exist,** would give a trapped colonist (and the player)
  a cheaper, faster fix than demolition — a room's doorway could
  become a real placeable entity rather than a permanently-omitted wall tile,
  and `jobDemolish`'s target selection would prefer one if reachable. Until
  then, breaking a wall down is deliberately the more expensive path (see
  `DemolishTicks` vs `BuildTicks`), so it never substitutes for actually
  building a door.
- **A colonist that wants to widen a cramped room** (the player's own
  "resizing" framing) is the same `JobDemolish` machinery pointed at a
  player-chosen tile instead of an automatically nearest one — the job itself
  doesn't care why the wall needs to come down.
- **Tuning `EscapeGraceTicks`** trades reaction speed against tolerance for
  the ordinary one-tick lag between a terrain edit and `refreshSpatial`
  folding it in; it is not meant to be a difficulty knob (see the config
  comment).

## Related

- [construction.md](./construction.md) — how a room gets built, the doorway
  invariant, and `doorTiles`'s narrower, up-front fix for the same failure.
- [pathfinding.md](./pathfinding.md) — rooms/regions and `roomOf`/`sameRoom`,
  the reachability primitives this feature reuses rather than duplicating.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — the
  focus arbitration system `FocusEscape` plugs into.
- [drives.md](./drives.md) — the emergency-build fallback this feature
  complements rather than replaces.

# Escaping a sealed room

> Part of the [mars-sim documentation](./README.md).

## What it is

A detect-and-correct backstop for a colonist whose room ends up cut off from
the rest of the colony — however that happened. A colonist whose room stays
disconnected from the colony's main network for `EscapeGraceTicks` straight
breaks the nearest wall back down to Floor and rejoins it, rather than sitting
sealed in indefinitely.

## Source

- [`internal/sim/rooms.go`](../internal/sim/rooms.go) — `mainRoom`, the room
  with the most floor tiles, recomputed by `relabelRooms`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) —
  `updateDisconnected`, `assignDemolish`, `nearestEscapeWall`, `jobDemolish`.
- [`internal/sim/focus.go`](../internal/sim/focus.go) — `FocusEscape`
  eligibility and scoring.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `JobDemolish`,
  `Demolishing`, `Entity.disconnectedTicks`.
- [`internal/sim/rooms_test.go`](../internal/sim/rooms_test.go) —
  `TestMainRoomTracksLargestRoom`, `TestUpdateDisconnectedTracksCutoffRoom`.
- [`internal/sim/sim_test.go`](../internal/sim/sim_test.go) —
  `TestColonistEscapesSealedRoom`, `TestColonistStarvesWhenSealedByRock`.

## How it works

**`mainRoom`.** `relabelRooms` already walks every connected component of the
region graph once per dirty-chunk refresh (see
[pathfinding.md](./pathfinding.md)); it now also sums each component's floor
tiles along the way and keeps the largest as `w.mainRoom`, tie-broken by the
smaller `RoomID` for a deterministic result regardless of Go's randomized map
iteration order. This is "the colony's main body" — normally every room the
colony has built, linked through doorways and corridors into one component, so
`mainRoom` is that whole network's ID. It updates for free: no scan of its own,
just a running comparison already available while relabeling runs anyway.

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

**Execution.** `assignDemolish` calls `nearestEscapeWall`, which walks
outward from the colonist through its own (cut-off) room's floor — bounded to
that room, not the whole map, since a sealed pocket is by definition small —
and returns the first `Wall` tile it touches. `jobDemolish` then mirrors
`jobMine`: travel to a tile adjacent to the target, spend `DemolishTicks`
(scaled by `workScale`, like every other work tick count), then
`SetTerrain(target, Floor)`. `refreshSpatial` folds that into the region graph
at the end of the same tick, and `updateDisconnected` notices the room is
whole again on the next. If no `Wall` borders the room at all — a natural cave
separation with only rock between the colonist and everything else — there is
nothing to demolish, and `runFocus` falls back to idle behavior for that tick
rather than looping on a target that can't exist. `TestColonistStarvesWhenSealedByRock`
pins that this case is intentionally *not* rescued.

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
- **Bounded BFS instead of a global wall scan.** `nearestEscapeWall` walks
  only the colonist's own cut-off room, which is small by construction (that's
  what "cut off" means) — a colony-wide scan for one trapped colonist would
  cost far more than the pocket it's confined to ever could.
- **Only walls, never rock.** A colonist that ended up in a naturally
  separate cavern (never reachable, not sealed by anything the colony built)
  has nothing to demolish — `JobMine` already exists for rock, and folding
  rock-clearing into escape as well would blur "trapped by construction" into
  "hasn't explored yet," which is a different, pre-existing problem this
  feature doesn't try to solve.

## Extending it

- **Doors, when they exist,** would give a trapped colonist (and the player)
  a cheaper, faster fix than demolition — `roomFrontWallY`'s doorway could
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
- [needs.md](./needs.md) — the emergency-build fallback this feature
  complements rather than replaces.

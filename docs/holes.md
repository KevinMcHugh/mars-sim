# Holes

> Part of the [mars-sim documentation](./README.md).

## What it is

A hole is the one-way way down: an open drop through the floor to the level
below. Whatever goes in lands on the tile underneath, and nothing comes back
up. The colony breaks one through on the player's order; a cornered colonist
leaps down one to get away from a threat; haulers tip refuse down one instead
of carrying it to an incinerator; and anyone on a tile when it becomes a hole
falls. Falling hurts, legs first, by levels dropped. A hole with a ladder
fitted becomes a shaft, which is also how the colony fetches back someone who
fell. This is phase Z3 of [z-levels.md](./z-levels.md); the two-way links it
sits beside are [stairs.md](./stairs.md) and [shafts.md](./shafts.md).

## Source

- [`internal/sim/holes.go`](../internal/sim/holes.go): the drop
  (`fallTarget`, `openHole`), digging (`canDigHole`, `digHole`,
  `OrderHole`, `planHoles`, `designateHole`, `finishHole`), ladders (`planLadders`,
  `strandedIn`, `designateLadder`, `OrderLadder`), falling (`fall`,
  `bodyOf`, `freeTileNear`, `leap`, `openHoleBeside`) and chutes
  (`nearestChute`, `tipDown`).
- [`internal/sim/shafts.go`](../internal/sim/shafts.go): `canDigShaft` and
  `shaftWorkTicks` take a hole's tile as a shaft's top (the ladder).
- [`internal/sim/systems.go`](../internal/sim/systems.go): `step` drops
  anything standing on a hole; `fleeStep` leaps.
- [`internal/sim/cleaning.go`](../internal/sim/cleaning.go):
  `refuseDestinations`, `haulTarget` and `jobCleanHaul` use a hole as a
  chute.
- [`internal/sim/siting.go`](../internal/sim/siting.go): a hole or ladder
  ordered at a picked tile (`downRefusal`, `ladderRefusal`); see
  [siting.md](./siting.md).
- [`internal/sim/holes_test.go`](../internal/sim/holes_test.go): digging
  one, the site search below a fresh shaft stopping at the main room, a two-level fall's damage, a fatal fall and a crowded landing, a
  cornered colonist leaping (and not leaping to its death), the chute, a
  stranded colonist getting a ladder, and the ordered hole and ladder in a
  real colony, deterministic and surviving a save.

## How it works

### The hole

`Hole` is a terrain, the last one. It is **not walkable** and **not a link**
(`links` knows nothing of it), so it joins no regions and no rooms: the
reachability layer stays symmetric, and every job gate, field and route
keeps ignoring it. Only the behaviors below use a hole, each by asking
`fallTarget(p)`: the first walkable tile straight below, through any holes
on the way, and how many levels that is. A hole whose column is closed
(rock, a structure, or a level nobody has broken into) drops nothing.
`w.holes` lists every hole, sorted, kept by `trackLinks`.

### Digging one

`OrderHole{At}` asks for one. With a tile (the browser's Dig tab: the Hole
tool, then a click on the map) it is checked and marked out at once, or
refused in the log; see [siting.md](./siting.md) for the rules. Without one
(`b` then `o` in the terminal) it needs `siting-auto`, and adds to
`manualHoles`: `planHoles` sites it as a stair is sited (`findStairSite`, on
the deepest level reached), one at a time. Either way it is one task of
`hole-ticks` mining work, free of materials (`designateHole`). `digHole`
opens the tile below to floor (making the level and revealing around it) and
then turns the tile into the hole. The landing is a one-tile pocket in the
rock until someone mines it out.

### Falling

`step` drops any entity standing on a hole before its turn: it is moved to
the nearest free tile to the foot of the drop (`freeTileNear`) and takes
`fall-damage` for every level fallen, to a leg (left, then right, then
left...) while it has one, else its torso. A fall can kill; the body stays
where it landed (`bodyOf`). A colonist who falls loses its job and thinks
again.

Nothing walks onto a hole, so something is only ever on one because it was
put there: the floor gave way under it, or it leapt.

### Leaping

`fleeStep` moves a fleeing colonist to the neighbour farthest from the
threat. When there is none (it is cornered) and the threat is next to it,
it leaps down an open hole beside it (`leap`), if the fall will not kill it.
Nothing can follow it: no route, field or link goes down a hole.

### Chutes

With `hole-chutes` on (the default), a hole counts as somewhere refuse can
go (`refuseDestinations`), and a hauler takes its load to whichever is
nearer by `travelEstimate`, a hole it can stand beside or an incinerator (a
tie goes to the incinerator). At the hole it tips the load down at once
(`tipDown`): bodies land as bodies and viscera as gore on the tile below.
It is the incinerator's job for free, but only downward: the level below
keeps what it is sent. That includes colonists' bodies, which only an
incinerator otherwise takes; turn `hole-chutes` off to keep them out of the
pit.

### Ladders

A ladder turns a hole into a one-level shaft: a shaft task (`depth` 1) on
the hole's tile, which `canDigShaft` accepts as a top and `digShaft` turns
into a `ShaftTop` over a `ShaftBottom`. It takes `ladder-ticks`, since the
level below is already open. `planLadders` fits one when a colonist is
stranded below a hole (its landing is in a room with a living colonist that
is not the colony's main room; in either siting mode), or when the player
ordered one: into the hole picked (`OrderLadder{At}`, the Dig tab's Ladder
tool), or, unsited (`b` then `u`, with `siting-auto`), into the first hole
the colony can stand beside. The ladder rejoins the stranded colonist's pocket to the colony, and
everything else (rooms, fields, the climb) is a shaft's (see
[shafts.md](./shafts.md)).

## Why it is this way

- **The player can site a hole.** A hole is a deliberate drop (a chute
  over a pit, a way into a cavern below, a trap), so where it goes is the
  whole point, and the planner's pick was rarely it. Worse, after a shaft
  or stair down the deepest level is only its foot, so an unsited order
  can wait forever, and the map-wide site search it ran every planning
  round froze the browser game (see [stairs.md](./stairs.md) and
  [siting.md](./siting.md)). A picked hole is marked out or refused on the
  spot. Nothing checks that it keeps the colony whole: a hole in a one-tile
  corridor cuts it, which is the player's call.

- **Not a link.** A one-way edge in the region graph would make "A reaches
  B" stop meaning "B reaches A", and every job gate assumes it does. Holes
  stay out of reachability entirely and are used only by behaviors that want
  a drop. The plan decided this; the build kept it.
- **Not walkable.** If a hole were a floor tile that dropped whoever stepped
  on it, every route would have to avoid it and every crowd shove would be
  a fall. Not walkable means nobody steps on one by accident, so every fall
  is either a leap or the floor giving way.
- **The colony rescues with a ladder, not a dig.** A fallen colonist's own
  escape (see [escape.md](./escape.md)) digs through rock on its level
  toward the main room, which finds nothing when the colony is on the level
  above. Rather than teach escape to dig up, the colony fits a ladder into
  the hole the colonist fell through: the plan's upgrade path, doing the
  rescue for free.
- **Chutes only for refuse.** The plan also wanted supplies sent down to a
  deep work camp through the hauling planner. That needs a depot below to
  receive them and the hauling planner to price a drop as a leg; there is
  no deep work camp yet (depth pays in Z4). Refuse already has a hauler and
  a destination, so it was the chute worth building now.
- **Raids came with Z4.** A hostile alien with nothing to walk to drops
  down a hole onto prey below (the `raid` rung; see [depth.md](./depth.md)).

## Extending it

- A new way to fall (a shove, a collapse) only has to put the entity on a
  hole tile: `step` drops it.
- A new thing to drop down (a supply chute) should use `fallTarget` for the
  landing and leave reachability alone.
- A hole must never become a link; a two-way way down is a shaft.

## Related

- [shafts.md](./shafts.md): what a ladder makes of a hole.
- [stairs.md](./stairs.md): the other two-way link, and how searches cross
  levels.
- [z-levels.md](./z-levels.md): the plan; this is phase Z3.
- [sanitation.md](./sanitation.md): refuse, incinerators, and the chute.
- [combat.md](./combat.md): body parts, which a fall damages.
- [escape.md](./escape.md): what a cut-off colonist does on its own level.

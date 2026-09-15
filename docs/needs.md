# Needs

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists accumulate **needs** (hunger, bladder) over time and drop work to
satisfy them at a matching facility. Mice reuse the food need. Needs are stored
lazily — a base level plus a timestamp — so idle colonists cost nothing per tick.

## Source

- [`internal/sim/needs.go`](../internal/sim/needs.go) — `NeedKind`, `NeedSpec`, lazy level math, starvation, urgency.
- [`internal/sim/config.go`](../internal/sim/config.go) — the `Needs` table and `StarveDamage`, `ColonistsPerFacility`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — the per-entity need storage (`Needs`, `needSince`, `needRise`, `starvationDamage`, `carrying`).
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `jobUse`, `jobUseCarrying`, `finishUse`, `availableToTalk`.

## How it works

### The needs table

Each `NeedKind` (`NeedFood`, `NeedBladder`, `NeedSocial`, `NeedSleep`) has a `NeedSpec` in `Config.Needs`,
indexed by the kind:

| Need | Rise/tick | SeekAt | Max | Facility | UseTicks | GrabTicks | Fatal |
| --- | --- | --- | --- | --- | --- | --- | --- |
| food | 2 | 650 | 1000 | NutrientPod | 18 | 3 | **yes** |
| bladder | 3 | 600 | 1000 | Toilet | 10 | 0 | no |
| sleep | 1 | 700 | 1000 | Bed | 40 | 0 | no |
| social | 2 | 500 | 1000 | conversation | — | — | no |

Levels run `0..Max`; 0 means satisfied. At `SeekAt` the colonist drops work to
satisfy the need; a **fatal** need sitting at `Max` drains HP (`StarveDamage`).

### Lazy evaluation

Needs are **not** ticked per colonist per tick. Instead `Entity.Needs[i]` is the
level *as of* tick `needSince[i]`, and `needLevel` computes the current value on
read:

```
level = clamp(Needs[i] + needRise[i] * (now - needSince[i]), 0, Max)
```

`needRise[i]` is the entity's own per-tick rate: trait-scaled for colonists (see
[personality.md](./personality.md)) and much faster for mice. `resetNeed` sets the
base back to 0 as of the current tick when a facility is used.

Social need has no physical facility. Once urgent, it preempts ordinary work and
the colonist waits for a conversation partner; completing a conversation resets
social need for both participants. Asocial colonists resolve its rise rate to
zero, introverts rise more slowly, and extroverts rise faster.

`availableToTalk` gates who can be pulled into a chat as the *other* party: idle
(`Job == JobNone`), not fleeing, not parked somewhere blocking a facility — and,
importantly, not itself facing an urgent need **other than social**. A
candidate whose own most urgent need is social still counts as available.
Without that carve-out, two colonists who both urgently need company can never
talk to each other — each disqualifies the other as a partner — and social
need sits permanently pinned at its ceiling in any colony busy enough that
nobody is ever fully need-free. This alone does not fully fix chronic pinning
in a very busy colony, though: the deeper issue is that `Job == JobNone` itself
is rare when colonists are constantly mining, building, or queueing — a
colonist cannot yet chat *while* doing something else (mid-queue, mid-dig),
which would need bigger, riskier surgery to the job model than has been
attempted.

### Starvation and healing

`applyStarvation` drains HP for any fatal need at `Max`, and tracks that damage
separately per need in `starvationDamage`. Satisfying the need restores exactly
that deprivation damage (up to `MaxHP`) — so eating heals hunger damage but not an
unrelated alien bite. Three grace conditions prevent unfair deaths:

1. A colonist already committed to a *reachable* facility (`JobUse`) is not
   drained mid-queue — it gets time to traverse the crowd and finish eating.
2. A colonist that has already grabbed a portable need (see *Taking it to go*
   below) is never drained regardless of the facility's reachability or
   crowding — it is guaranteed to finish; it just isn't there anymore.
3. While reachable life support is *under construction*, fatal-need drain is
   suspended. This matters most at startup, when staggered hunger can hit `Max`
   just before the first facility room finishes.

### Which need wins

`mostUrgentNeed` returns the need furthest past its `SeekAt`, but a **fatal need
outranks any non-fatal one**. Without that rule, bladder (which rises faster and
caps further past its threshold) would permanently outrank food and let colonists
starve while relieving themselves.

Sleep is deliberately non-fatal. A tired colonist seeks a reachable bunk and
spends `UseTicks` (40 by default) sleeping beside it. Food and toilets are
still planned before dormitories, so a bunk usually arrives later than the
first facility room — but a colonist with no reachable bunk, no bunk task to
help with, and no dormitory under construction anywhere reachable does not
simply wait forever: like any other need (see *The emergency fallback* in
[construction.md](./construction.md)), it builds one for itself. This keeps
sleep a capacity and scheduling pressure in the common case, without letting a
delayed dormitory turn into an indefinite "stuck waiting" loop.

### Satisfying a need

When a need is urgent and a facility of the right kind is reachable, the
colonist normally takes a `JobUse` job and follows that facility's shared
**flow field** to the nearest one, stands adjacent, and uses it for
`UseTicks`. But if the colony still wants more of that facility than it has
planned or built, the colonist tries to help build that capacity first
(joining a reachable project task that actually provides it, via
`claimNearestTaskProviding` — see [construction.md](./construction.md)) rather
than just queueing — otherwise, once a single facility exists, no colonist is
ever free to build a second. See [pathfinding.md](./pathfinding.md) for the
flow fields and [construction.md](./construction.md) for how facilities get
built and for this priority in full. The colony keeps `ColonistsPerFacility`
colonists' worth of each facility planned or built.

### Taking it to go

`GrabTicks`, when positive and less than `UseTicks`, makes a need portable:
the colonist spends only `GrabTicks` at the facility, then carries it away and
spends the rest of `UseTicks` finishing elsewhere (`jobUseCarrying`), freeing
the facility's access tile immediately rather than occupying it for the whole
`UseTicks`. Food is the only portable need by default (`GrabTicks: 3` against
an 18-tick meal) — a colonist "sits there sucking down goop" for only 3 ticks,
then steps aside (`stepAside`, falling back to `wanderStep`) and finishes
eating out of everyone else's way. Bladder and sleep stay `GrabTicks: 0`
(in-place only): there is nothing to carry away from a toilet or a bed.

This is purely a throughput change — the total `UseTicks` a colonist spends
satisfying the need is unchanged — but it turns a facility's *access-tile*
capacity from "one user every `UseTicks`" into "one user every `GrabTicks`,"
which is the real bottleneck once a facility is shared by more than a couple
of colonists: a single pod that could serve at most `UseTicks`⁻¹ colonists per
tick before now serves `GrabTicks`⁻¹.

### Staggered start

At spawn, colonists get a **random** starting level in `[0, SeekAt)` for each
need, so a fresh colony does not all get hungry on the same tick and stampede the
facilities at once.

## Why it is this way

- **Lazy base+timestamp storage** is the key performance move: an idle colonist's
  needs stay correct without any per-tick work, which is what lets the resting AI
  skip the map scan entirely. `applyStarvation` and the snapshot both read levels
  lazily, so they are correct even after a colonist has rested for many ticks.
- **Per-need starvation damage** keeps healing intuitive and prevents food from
  accidentally patching combat wounds.
- **Fatal-beats-non-fatal urgency** is a balance rule learned from colonists
  starving with a full bladder — the table alone (thresholds) was not enough.
- **Grace periods** stop the frustrating startup deaths where hunger outraced the
  very first pod.
- **Sleep being non-fatal** keeps the planner's priority (life support before
  dormitories) meaningful: a tired colonist can wait for the next bunk instead
  of forcing dormitories to compete with life support at the moment life
  support is most needed. It no longer means *only* waiting, though — see the
  emergency fallback below.
- **Portable needs** (food) separate "how long satisfying this need takes"
  from "how long it occupies the one tile everyone else queues behind." A
  facility's real capacity is its access tile, not its `UseTicks`, and a
  crowded colony hits that limit long before population catches up with
  `ColonistsPerFacility`.

## Extending it

Adding a need is meant to be a **table edit**:

1. Append a `NeedKind` before `numNeeds` and add its `String()` case.
2. Add its `NeedSpec` to `Config.Needs` (rise, seek, max, facility, use ticks,
   fatal).
3. Give it a satisfying `Terrain` facility (a flow field is auto-allocated per
   facility terrain in `newWorld`) and a display `State` in `useState`.

The systems iterate needs generically, so no behavior code needs to change. A new
trait that scales the need slots in via `traitSpecs` (see
[personality.md](./personality.md)).

## Related

- [entities-and-ai.md](./entities-and-ai.md) — how needs preempt work.
- [personality.md](./personality.md) — traits that scale need rise rates.
- [construction.md](./construction.md) — how the facilities that satisfy needs are built.
- [configuration.md](./configuration.md) — where the needs table lives.

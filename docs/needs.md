# Needs

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists accumulate **needs** (hunger, bladder) over time and drop work to
satisfy them at a matching facility. Mice reuse the food need. Needs are stored
lazily — a base level plus a timestamp — so idle colonists cost nothing per tick.

## Source

- [`internal/sim/needs.go`](../internal/sim/needs.go) — `NeedKind`, `NeedSpec`, lazy level math, starvation, urgency.
- [`internal/sim/config.go`](../internal/sim/config.go) — the `Needs` table and `StarveDamage`, `ColonistsPerFacility`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — the per-entity need storage (`Needs`, `needSince`, `needRise`, `starvationDamage`).

## How it works

### The needs table

Each `NeedKind` (`NeedFood`, `NeedBladder`) has a `NeedSpec` in `Config.Needs`,
indexed by the kind:

| Need | Rise/tick | SeekAt | Max | Facility | UseTicks | Fatal |
| --- | --- | --- | --- | --- | --- | --- |
| food | 2 | 650 | 1000 | NutrientPod | 18 | **yes** |
| bladder | 3 | 600 | 1000 | Toilet | 10 | no |
| sleep | 1 | 700 | 1000 | Bed | 40 | no |

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

### Starvation and healing

`applyStarvation` drains HP for any fatal need at `Max`, and tracks that damage
separately per need in `starvationDamage`. Satisfying the need restores exactly
that deprivation damage (up to `MaxHP`) — so eating heals hunger damage but not an
unrelated alien bite. Two grace conditions prevent unfair deaths:

1. A colonist already committed to a *reachable* facility (`JobUse`) is not
   drained mid-queue — it gets time to traverse the crowd and finish eating.
2. While reachable life support is *under construction*, fatal-need drain is
   suspended. This matters most at startup, when staggered hunger can hit `Max`
   just before the first facility room finishes.

### Which need wins

`mostUrgentNeed` returns the need furthest past its `SeekAt`, but a **fatal need
outranks any non-fatal one**. Without that rule, bladder (which rises faster and
caps further past its threshold) would permanently outrank food and let colonists
starve while relieving themselves.

Sleep is deliberately non-fatal. A tired colonist seeks a reachable bunk and
spends `UseTicks` (40 by default) sleeping beside it. If no bunk is reachable,
the colonist waits rather than taking an emergency-build path; food and toilets
are planned first, and dormitories are added once life support is covered. This
makes sleep a capacity and scheduling pressure without turning an unfinished
dormitory into a death spiral.

### Satisfying a need

When a need is urgent and a facility of the right kind is reachable, the colonist
takes a `JobUse` job and follows that facility's shared **flow field** to the
nearest one, stands adjacent, and uses it for `UseTicks`. See
[pathfinding.md](./pathfinding.md) for the flow fields and
[construction.md](./construction.md) for how facilities get built. The colony
keeps `ColonistsPerFacility` colonists' worth of each facility planned or built.

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
- **Sleep being non-fatal** keeps dormitory construction from competing with
  life-support construction at the moment it is most needed. A tired colonist
  can wait for the next bunk.

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

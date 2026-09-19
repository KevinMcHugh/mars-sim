# Needs

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists accumulate **needs** (food, bladder, social contact, and sleep) over
time and may switch focus to satisfy them. Each need independently projects its
lazy numeric level into a discrete phase. Mice reuse the food level. Levels stay
lazy — a base plus a timestamp — so idle colonists do not need per-tick storage
updates.

## Source

- [`internal/sim/needs.go`](../internal/sim/needs.go) — `NeedKind`, `NeedPhase`, `NeedSpec`, lazy level math, phase synchronization, pressure, and starvation.
- [`internal/sim/config.go`](../internal/sim/config.go) — the `Needs` table and `StarveDamage`, `ColonistsPerFacility`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — the per-entity need storage (`Needs`, `needSince`, `needRise`, `starvationDamage`, `carrying`).
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `jobUse`, `jobUseCarrying`, `finishUse`, `availableToTalk`.

## How it works

### The needs table

Each `NeedKind` (`NeedFood`, `NeedBladder`, `NeedSocial`, `NeedSleep`) has a `NeedSpec` in `Config.Needs`,
indexed by the kind:

| Need | Rise/tick | SeekAt | CriticalAt | Max | Facility | UseTicks | GrabTicks | Fatal |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| food | 2 | 650 | 1000 | 1000 | NutrientPod | 18 | 3 | **yes** |
| bladder | 3 | 600 | 900 | 1000 | Toilet | 10 | 0 | no |
| sleep | 1 | 700 | 900 | 1000 | Bed | 40 | 0 | no |
| social | 2 | 500 | 850 | 1000 | conversation | — | — | no |

Levels run `0..Max`; 0 means satisfied. `SeekAt` makes the need actionable,
`CriticalAt` adds critical focus pressure, and a **fatal** need sitting at `Max`
drains HP (`StarveDamage`). Configuration enforces
`0 <= SeekAt <= CriticalAt <= Max`.

### Lazy evaluation

Needs are **not** ticked per colonist per tick. Instead `Entity.Needs[i]` is the
level *as of* tick `needSince[i]`, and `needLevel` computes the current value on
read:

```
level = clamp(Needs[i] + needRise[i] * (now - needSince[i]), 0, Max)
```

`needRise[i]` is the entity's own per-tick rate: trait-scaled for colonists (see
[personality.md](./personality.md)) and much faster for mice. `syncNeedPhase`
reads this lazy level and updates only its discrete projection:

| Phase | Level |
| --- | --- |
| `NeedSatisfied` | zero |
| `NeedGrowing` | above zero but below `SeekAt` |
| `NeedPressing` | `SeekAt` through just below `CriticalAt` |
| `NeedCritical` | `CriticalAt` or above |

Each need also caches the next tick at which its current rise rate can cross a
phase boundary. Zero-rise and already-critical needs schedule no crossing. This
cache does not change behavior yet; Phase 5 can use it to avoid needless focus
arbitration without estimating when a lazy need changes. `resetNeed` sets the
base back to 0 at the current tick and immediately synchronizes the phase and
next boundary.

Pressing and critical needs emit normalized pressure from 1 through 100 into
weighted focus arbitration. Pressure is 1 at `SeekAt`, reaches 75 at
`CriticalAt`, and reaches 100 at `Max`; satisfied and growing needs emit zero.
Critical and fatal bonuses preserve urgency without embedding another priority
ladder in the job executors.

Social need has no physical facility. Once urgent, it preempts ordinary work and
the colonist waits for a conversation partner; completing a conversation resets
social need for both participants. Asocial colonists resolve its rise rate to
zero, introverts rise more slowly, and extroverts rise faster.

A conversation already under way **is** how the need gets met, so the urgent-social
branch checks `talkPartner` and lets a live talk run, exactly as `handlingNeed`
does for `JobUse`/`JobBuild` further down the tick. Skipping that check was a
livelock: a socially urgent colonist cleared its own job and called
`tryStartTalk` every tick, `beginTalk` reset the shared `Progress` timer, and so
a mutually urgent pair restarted the same conversation forever without ever
reaching `TalkTicks`. Because an urgent social need preempts all ordinary work,
the colony then stopped digging and building permanently — measured on a default
6-colonist game, 2000 ticks produced **0** completed conversations, 989 mid-talk
partner switches, a mean social need of 919/1000, and 11 tiles excavated. With
the live talk left alone: 38 conversations, 3 switches, mean social 337, and 675
tiles. `TestMutuallyUrgentColonistsFinishConversation` and
`TestColonyKeepsExcavatingOnceNeedsBite` pin both halves of that.

`availableToTalk` gates who can be pulled into a chat as the *other* party: idle
(`Job == JobNone`), not fleeing, not parked somewhere blocking a facility — and,
importantly, not itself facing an urgent need **other than social**. A
candidate whose own most urgent need is social still counts as available.
Without that carve-out, two colonists who both urgently need company can never
talk to each other — each disqualifies the other as a partner — and social
need sits permanently pinned at its ceiling in any colony busy enough that
nobody is ever fully need-free. That carve-out plus leaving live talks alone (above) is
what actually unpins the need in a busy colony. One limitation remains, and it
is a design one rather than a bug: a partner must be at `Job == JobNone`, so a
colonist cannot chat *while* doing something else (mid-queue, mid-dig). Letting
them would need bigger, riskier surgery to the job model than has been
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
2. Add its `NeedSpec` to `Config.Needs` (rise, seek, critical, max, facility,
   use ticks, fatal).
3. Give it a satisfying `Terrain` facility (a flow field is auto-allocated per
   facility terrain in `newWorld`) and a display `State` in `useState`.
4. Regenerate the settings file (`go run . -print-config > mars-sim.yaml`): the
   spec's tagged fields become `needs.<name>.*` settings and `-need-<name>-*`
   flags automatically, named from the `String()` case in step 1. See
   [config-file.md](./config-file.md).

The systems iterate needs generically, so no behavior code needs to change. A new
trait that scales the need slots in via `traitSpecs` (see
[personality.md](./personality.md)).

## Related

- [entities-and-ai.md](./entities-and-ai.md) — how needs preempt work.
- [personality.md](./personality.md) — traits that scale need rise rates.
- [construction.md](./construction.md) — how the facilities that satisfy needs are built.
- [configuration.md](./configuration.md) — where the needs table lives.
- [config-file.md](./config-file.md) — tuning a need's spec from `mars-sim.yaml` or a flag.

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
| food | 2 | 650 | 1000 | 1000 | meals, then NutrientPod | 18 | 3 | **yes** |
| bladder | 3 | 600 | 900 | 1000 | Toilet | 10 | 0 | no |
| sleep | 1 | 700 | 900 | 1000 | Bed | 40 | 0 | no |
| social | 2 | 500 | 850 | 1000 | conversation | — | — | no |

Food is the one need met by an item as well as a facility: a hungry colonist
eats a real `Meal` it owns (or the colony owns) before it walks to a nutrient
pod, and with `infinite-food` off pods feed nobody at all. See
[food.md](./food.md); everything below about facilities applies to food only
when the safety net is what the colonist is using.

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

When a need is urgent and a facility of the right kind that the colonist may
use is reachable (`facilityReachable`: a communal one via the shared field, or
its own private one — see [property.md](./property.md)), the
colonist normally takes a `JobUse` job. With fewer than two facilities of that
kind — and none of them private — there's nothing to choose between, so it
just follows that facility's shared **flow field** to the nearest one. Once a second exists, `jobUse`
switches to routing at a *concrete* facility instead — see *Spreading users
across facilities* below — stands adjacent to whichever it ends up at, and
uses it for `UseTicks`. But if the colony still wants more of that facility
than it has planned or built, the colonist tries to help build that capacity
first (joining a reachable project task that actually provides it, via
`claimNearestTaskProviding` — see [construction.md](./construction.md))
rather than just queueing — otherwise, once a single facility exists, no
colonist is ever free to build a second. See [pathfinding.md](./pathfinding.md)
for the flow fields and [construction.md](./construction.md) for how
facilities get built and for this priority in full. The colony keeps
`ColonistsPerFacility` colonists' worth of each facility planned or built.

### Spreading users across facilities

`chooseFacility` (`facilitychoice.go`) picks *which* facility of a kind a
colonist commits to once more than one exists — skipping any private fixture
that is not its own — so a crowd doesn't all converge on the shared field's
single nearest seed. It ranks reachable facilities by
walkable distance and skips any that's **congested** — something actually
occupying one of its reachable access tiles right now, or another colonist
already committed to it (`Job == JobUse`, `useFacilitySet`, `useFacility` equal
to it) — falling back to nearest-even-if-congested only when every reachable
option is busy, so a need is never declared unreachable and left to starve
merely because everything is momentarily full. Ties go to the lower
`lessPoint`. The choice is retained on the colonist for the whole `JobUse` job
(`e.useFacility`), so it never re-litigates and ping-pongs between queues as
counts change tick to tick.

#### How the choice is computed, and why not with one BFS

It used to be one BFS from the colonist over everything it could reach,
followed by a scan of every facility. That flood covered the whole reachable
map on every need decision, even when the pod was three tiles away. On a big
colony it was a third of a real CPU profile. The congestion check was
expensive too: it scanned every entity once per access tile of every facility,
so on a crowded colony (`BenchmarkNeedSeek`) it cost more than the BFS.

The answer is the same, but it is now found in up to three steps, cheapest
first:

1. **The shared field names the nearest facilities.** The facility kind's flow
   field already holds, for every tile, the distance D to the nearest facility
   of that kind. `facilityByField` walks only *downhill* from the colonist
   (each step to a neighbour whose distance is one less). That visits exactly
   the tiles on shortest routes to the facilities at distance D, and nothing
   else. If one of those facilities is free, that's the answer.
2. **All busy?** If every nearest facility is congested, `anyFreeFacility`
   checks whether *any* reachable facility is free. If none is, the answer is
   the nearest one, already found in step 1. That is the common case exactly
   when it matters most, in a colony short of facilities with a crowd choosing
   at once, and without this check step 3 would flood the whole map looking for
   a free facility that doesn't exist.
3. **Bounded search.** Otherwise `facilityBySearch` runs the old BFS, but it
   stops at the first distance at which it reaches a free facility.

Commitments are counted once per call (`committedUsers`, one pass over the
entities) and only for facilities that are actually in contention.
`BenchmarkNeedSeek` went from 148 ms to 12 ms a tick. On a generated 400×250
map with 150 colonists, `chooseFacility` went from 47% of the tick to 3%.

Two decisions worth knowing before changing it:

- **The field stores distances, not "which facility is nearest".** Labelling
  each field cell with its nearest facility would make step 1 a single lookup,
  but fields are kept current by incremental repair (see
  [pathfinding.md](./pathfinding.md)), and a repair only follows *distance*
  changes. A newly built facility can change which facility is nearest across
  a whole region without changing a single distance, so the labels would
  silently go stale. Reading the nearest set off the distances can't go stale.
- **Reachability is judged by room.** To decide whether an occupied access tile
  counts, the fast steps ask whether it's in the colonist's room rather than
  whether a per-call BFS reached it. Rooms and fields are both refreshed
  between ticks, not mid-tick. Mid-tick, after a dig or build, the answer can
  therefore differ from a fresh BFS, but deterministically, and it matches the
  view the colonist navigates by. Between ticks it is exactly the old answer:
  `TestChooseFacilityMatchesReference` keeps the old implementation as a
  test-only oracle and compares every colonist's choice of every facility kind
  across several games. It also requires that all three steps get exercised.
  Full games with the change play out identically, tick for tick, to games
  without it.

Two bugs here were serious enough to leave written down:

- **"Congested" must mean actual contention, not nearby occupancy.** An
  earlier version also flagged a facility whenever *any* other colonist stood
  within one tile of *any* of its access tiles — whatever that colonist was
  doing. Facilities sit one tile apart in a room (see
  [construction.md](./construction.md)), so their access neighborhoods
  overlap; in a merely busy room (colonists resting, chatting, walking
  through, or using the facility next door) that overbroad check could flag
  every facility in it "congested" at once. This function's whole point —
  spread users across reachable facilities — then gave up and fell back to
  "nearest for everyone," funneling a crowd onto one facility while others
  sat genuinely idle beside it: from the outside, a long, static line for one
  bathroom while others nearby looked untouched. See
  `TestChooseFacilityIgnoresUnrelatedBystander`.
- **A candidate's distance must start from scratch, not from the running
  best.** The per-facility nearest-access-distance loop seeded its working
  distance from `bestDist` (the best found *so far*) instead of from
  "infinity." A facility whose true nearest access was farther than the
  current best then had no way for its real distance to ever come out lower
  than that seed, so it looked exactly *tied* with the current best — and the
  tie-break could swap the current best out for it, a strictly farther and
  wrong facility. Which candidate got treated as "best so far" when a worse
  one was evaluated depends on the order facilities are visited, and they're
  stored in a map — whose iteration order Go deliberately randomizes on every
  `range` — so the same seed and the same world state could send a colonist
  to a different, worse facility on different runs. That's a real break of
  "same seed => same game" (see `AGENTS.md`), not a cosmetic one: it directly
  caused colonists to queue at a busy facility while a closer, free one
  existed. See `TestChooseFacilityPicksNearestAmongManyCandidates`, which
  calls `chooseFacility` many times over an unchanged world specifically
  because the bug only shows up under some map-iteration orders, not all of
  them.

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

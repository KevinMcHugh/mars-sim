# Drives

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists have four **drives** — food (hunger), bladder, social (company) and
sleep — that build up over time and that they act to satisfy. A drive is a
score between a minimum and a maximum. How fast it grows is a base rate scaled
by stacked modifiers: what the colonist is doing, its traits, the bands other
drives sit in, and timed **effects** such as a substance. What a level does to
the colonist is declared as **bands**: below `SeekAt` it is ignored, past it the
colonist goes to satisfy it, and a fatal drive at its ceiling drains HP. Rats,
chickens and aliens have the food drive only.

Drives are evaluated lazily, as needs always were: a level is a base plus the
tick it was taken, folded forward only when something changes the rate. A
colonist whose rate holds costs nothing per tick.

Drives replaced **needs** (`NeedKind`, `needs.*` settings, `-need-*` flags).
Needs grew at one integer rate per tick, had their consequences hard-coded, and
paused in bed through a special case in `sleep.go`. Drives keep everything that
was true of needs — facilities, which drive wins, starvation and its graces —
and that material is below.

## Source

- [`internal/sim/drives.go`](../internal/sim/drives.go) — `DriveKind`, `DriveSpec`, the shipped `defaultDrives`, `driveState` and the lazy level (`driveTrue`, `driveFelt`, `driveLevel`), rate composition (`driveRate`), `refreshDrive`, `resetDrive`, pressure, `mostUrgentDrive`, and the colony calendar derived from the sleep drive.
- [`internal/sim/drive_activity.go`](../internal/sim/drive_activity.go) — `DriveActivity` (asleep, idle, working, labor), the `activityDrives` table that files every player-facing `Activity` under one, `driveActivityOf`, and `syncDriveActivity`.
- [`internal/sim/drive_bands.go`](../internal/sim/drive_bands.go) — `DriveConsequence`, `DriveRamp`, compiling a spec into a band table, `syncDriveBand`, `nextBandCrossing`, and `applyStarvation`.
- [`internal/sim/drive_effects.go`](../internal/sim/drive_effects.go) — `DriveEffectProfile` and its stages, `applyEffect`, `advanceEffects`.
- [`internal/sim/personality.go`](../internal/sim/personality.go) — the traits' `driveRate` percents, resolved into `Entity.driveTrait`.
- [`internal/sim/sleep.go`](../internal/sim/sleep.go) — the night in bed and banked sleep (see [days.md](./days.md)).
- [`internal/sim/systems.go`](../internal/sim/systems.go) — the turn hooks (`advanceEffects` first in `colonistTurn`, `syncDriveActivity` after it), `syncCognitionDeadlines`, `jobUse`, `finishUse`, `availableToTalk`.
- [`internal/sim/config.go`](../internal/sim/config.go) — `Config.Drives`, `Config.DriveEffects`, `StarveDamage`, `ColonistsPerFacility`, and the rat, chicken and alien hunger rates.
- [`internal/sim/drive_model_test.go`](../internal/sim/drive_model_test.go) — the model: lazy sums across rate changes, composition order, activity tables, caffeine, stacking, negative rates, ramps, couplings, validation. [`drives_test.go`](../internal/sim/drives_test.go) keeps the phase, pressure, urgency and calendar tests that predate drives.
- [`internal/sim/drive_tuning_test.go`](../internal/sim/drive_tuning_test.go) — `TestDriveTuningReport`, the harness the shipped numbers were tuned with (skipped unless `MARS_DRIVE_TUNING` is set).

## How it works

### The drives table

Each `DriveKind` has a `DriveSpec` in `Config.Drives`, indexed by the kind:

| Drive | Rate (milli/tick) | SeekAt | CriticalAt | Max | Facility | UseTicks | GrabTicks | Fatal |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| food | 1750 | 650 | 1000 | 1000 | meals, then NutrientPod | 18 | 3 | **yes** |
| bladder | 3000 | 600 | 900 | 1000 | Toilet | 10 | 0 | no |
| sleep | 1000 | 720 | 900 | 1000 | Bed | 360 | 0 | no |
| social | 2000 | 500 | 850 | 1000 | conversation | — | — | no |

Levels are whole points from `Min` (0) to `Max`; 0 means satisfied. Rates are in
thousandths of a point per tick (`driveUnit = 1000`), so 1750 is 1.75 points a
tick. Food is the one drive met by an item as well as a facility: a hungry
colonist eats a real `Meal` it owns (or the colony owns) before it walks to a
nutrient pod, and with `infinite-food` off pods feed nobody at all (see
[food.md](./food.md)). Configuration enforces `Min <= 0 <= SeekAt <=
CriticalAt <= Max`.

Every numeric field is a setting: `drives.food.rate` in `mars-sim.yaml`,
`-drive-food-rate` on the command line (see [config-file.md](./config-file.md)).

### The lazy level

`Entity.drives[d]` is a `driveState`:

```
true  = clamp(base + rate × (now − since), Min, Max)      // driveTrue, in driveUnits
felt  = clamp(true − offset, Min, Max)                     // driveFelt
level = floor(felt / 1000)                                 // driveLevel, whole points
```

Everything that acts on a drive reads `driveLevel`, the felt level. `offset` is
the masking active effects apply (see *Effects*); with none running, felt and
true are the same.

`refreshDrive` is the only place a rate changes. It folds the growth so far into
`base` at the **old** rate, sets `since` to now, recomposes the rate, recomputes
the masking, and re-places the level in its band. Every input to the rate calls
it when it changes:

- a change of drive activity (`syncDriveActivity`, after every colonist turn);
- traits being resolved (`resolveTraitEffects`, at spawn and on mutation);
- another drive entering or leaving a band that couples to this one;
- an effect starting, changing stage or ending;
- `resetDrive` and `setDrive`.

`TestLazyDriveMatchesPerTickSum` holds the invariant: across activity, trait and
effect changes, the lazy level equals the sum of the rate in force on every tick.

### Growth: base × modifiers

`driveRate` composes the rate in a fixed order, with integer arithmetic, so a
seed always gives the same rates:

```
rate = entity base rate + Σ effect Add
rate = rate × activity%             // colonists only: DriveSpec.Activity[driveActivity]
rate = rate × trait%                // Entity.driveTrait, from the traits' driveRate
rate = rate × (100 + coupling)%     // each ConsequenceRate on this drive from another drive's band
rate = rate × (100 + effect)%       // each running effect's RateChange
```

Each step truncates. The entity's base rate is `DriveSpec.Rate` for a colonist,
and `rat-hunger-rate`, `chicken-hunger-rate` or `alien-hunger-rate` for the food
drive of those kinds (their other drives have no base rate, so they never grow).

Traits are written as **changes**, not multipliers: `driveRate: {DriveFood: +50}`
is Big Eater (150%), Light Eater is −30, Introvert's social −50, Extrovert's
+50, and Asocial's social −100, which stops it. Zero means no effect, so a trait
that does not mention a drive needs nothing written. Effect `RateChange` and
coupling `Value` read the same way.

### Drive activities

How strenuous what a colonist is doing is, as far as its drives are concerned.
There are four classes, and each drive has a percent of its rate for each:

| Drive | asleep | idle | working | labor |
| --- | --- | --- | --- | --- |
| food | 10 | 75 | 100 | 140 |
| bladder | 10 | 100 | 100 | 100 |
| social | 0 | 100 | 100 | 100 |
| sleep | 100 | 100 | 100 | 100 |

They are settings too: `drives.food.activity.labor`, `-drive-food-activity-labor`.

Which class a colonist is in comes from the player-facing `Activity` the
Activity tab already tallies ([activity-screen.md](./activity-screen.md)), through
one table, `activityDrives`, with two columns. **Doing** applies while the
colonist's `State` names the activity: asleep means in bed, not merely tired.
**Walking** applies while it walks there. A colonist neither doing nor walking
(waiting for a free bed, say) is idle. Mining is `{labor, working}`, hauling
`{labor, labor}` because the load is carried on the walk, and fleeing is labor
both ways.

`syncDriveActivity` runs after each colonist's turn. It compares the class with
the one the rates were composed for and refreshes the drives only when it has
changed, so it costs one comparison on a tick when nothing changes.

### Effects

An event (a cup of coffee, a drink of water, later any substance) starts an
**effect profile** from `Config.DriveEffects`, indexed by `DriveEffectKind`. A
profile is a list of stages, each lasting `Ticks`, and for each drive a stage
can set:

- `Instant`: whole points added to the true level once, as the stage begins (a
  big meal sating hunger). Permanent.
- `Offset`: whole points of masking while the stage lasts. The colonist feels
  that much less than its true level, which keeps growing underneath.
- `Add`: thousandths of a point a tick added to the base rate (water filling the
  bladder).
- `RateChange`: a percent change to the rate (−60 slows it to 40%).

When the masking ends, the felt level jumps back to the true level: the crash.
Caffeine, the worked example (a test fixture, not shipped content), is three
stages on the sleep drive:

| Stage | Ticks | Offset | RateChange |
| --- | --- | --- | --- |
| kick-in | 90 (2 h) | 250 | −60 |
| fading | 45 (1 h) | 120 | −30 |
| crash | 22 (½ h) | 0 | +20 |

`applyEffect(e, kind)` starts a profile. If the profile `Stacks`, applying it
again adds a second instance (their offsets and rate changes both apply);
otherwise the running one starts over. `advanceEffects` runs first in every
colonist turn, before the sleeping fast path can return. It moves expired
instances on a stage, applies the new stages' instants, and refreshes every
drive, since an ending stage can drop masking or a rate change from any of them.
Running effects live in `Entity.effects`; `nextEffectTick` caches the earliest
stage end.

No effect ships yet: nothing in the game calls `applyEffect`. The config file
has no lists, so profiles are set in code.

### Bands and consequences

A drive's range is cut into **bands** wherever something changes: at 1
(satisfied to growing), `SeekAt`, `CriticalAt`, and the edges of every
consequence. Within a band every consequence is constant. `compileDrives` builds
each drive's table once, in `newWorld`, and `Config.CheckDrives` validates it
for `main`.

A `DriveConsequence` applies `Kind` with `Value` while the level is in `From..To`
(inclusive):

- `ConsequenceHPDrain` takes `Value` HP a tick. `fatal: true` compiles to one at
  `[Max, Max]` worth `StarveDamage`, so the old knob still works.
- `ConsequenceRate` changes `Target`'s rate by `Value` percent. This is how drives
  affect one another: "while hunger is at 650 or more, sleep builds 30% faster"
  is `{From: 650, To: 1000, Kind: ConsequenceRate, Target: DriveSleep, Value: 30}`
  on the food drive. None ships yet.

A `DriveRamp` is the shorthand for a consequence that should change smoothly: it
moves from `FromValue` at `From` to `ToValue` at `To` in `Steps` even steps, and
holds `ToValue` above `To`. Compilation expands it into `Steps + 1`
consequences. For example, "starting at 700 exhaustion movement slows, reaching
60% slower at 950" would be five 12% steps, once movement speed has a
consequence kind (see *Extending it*). Today a ramp can drive HP drain or a
coupling.

Each band also carries an urgency **phase**, read from its lowest level:

| Phase | Level |
| --- | --- |
| `DriveSatisfied` | zero (or below) |
| `DriveGrowing` | above zero but below `SeekAt` |
| `DrivePressing` | `SeekAt` through just below `CriticalAt` |
| `DriveCritical` | `CriticalAt` or above |

`syncDriveBand` places the felt level in its band, caches the phase, and
computes `nextCrossing`: the tick the current rate carries the level out of the
band, up through its top edge or (for a negative rate) down below its floor, or
0 if it never will. The calculation counts from the unclamped felt level, so a
masked level has to climb out from under its floor first. A phase change marks
the colonist's mind dirty. A band change whose rate consequences differ
refreshes the drives they target. If the crossing is sooner than the colonist's
next scheduled think, the think is pulled in to meet it, because the sleeping
fast path skips the deadline check until then. `syncCognitionDeadlines` re-syncs
each drive when its crossing comes due, and fatal drives every tick as a safety
belt.

Pressing and critical drives emit normalized pressure from 1 through 100 into
weighted focus arbitration: 1 at `SeekAt`, 75 at `CriticalAt`, 100 at `Max`.
Satisfied and growing drives emit none. See
[cascading_wsts_architecture.md](./cascading_wsts_architecture.md).

### Sleep

While a colonist is in bed, the asleep column is what keeps a night intact:
social does not grow and food and bladder barely do. Sleep's own rate is 100% in
every class, because the colony calendar is derived from it
(`Config.TicksPerDay`) and a day has to be one length for everyone. Banked
nights and sleep traits are in [days.md](./days.md).

### Staggered start

At spawn, colonists get a **random** starting level in `[0, SeekAt)` for each
drive, so a fresh colony does not all get hungry on the same tick and stampede
the facilities at once. `initDrives` sets those levels only after personality
has resolved the trait rates, so the first band and crossing are computed with
the right rates.

### Social

Social drive has no physical facility, but with a meeting hall built it has a
place: the colonist walks to a chair and pairs with someone else in the hall (see
[meeting-hall.md](./meeting-hall.md)); without one it looks within `talk-radius`
wherever it stands. Once urgent, it preempts ordinary work and
the colonist waits for a conversation partner; completing a conversation resets
social drive for both participants. Asocial colonists' social drive never
grows (a -100% trait rate), introverts' grows at half speed, and extroverts'
at 150%.

A conversation already under way **is** how the drive gets met, so the urgent-social
branch checks `talkPartner` and lets a live talk run, exactly as `handlingNeed`
does for `JobUse`/`JobBuild` further down the tick. Skipping that check was a
livelock: a socially urgent colonist cleared its own job and called
`tryStartTalk` every tick, `beginTalk` reset the shared `Progress` timer, and so
a mutually urgent pair restarted the same conversation forever without ever
reaching `TalkTicks`. Because an urgent social drive preempts all ordinary work,
the colony then stopped digging and building permanently — measured on a default
6-colonist game, 2000 ticks produced **0** completed conversations, 989 mid-talk
partner switches, a mean social drive of 919/1000, and 11 tiles excavated. With
the live talk left alone: 38 conversations, 3 switches, mean social 337, and 675
tiles. `TestMutuallyUrgentColonistsFinishConversation` and
`TestColonyKeepsExcavatingOnceNeedsBite` pin both halves of that.

`availableToTalk` gates who can be pulled into a chat as the *other* party: idle
(`Job == JobNone`), not fleeing, not parked somewhere blocking a facility — and,
importantly, not itself facing an urgent drive **other than social**. A
candidate whose own most urgent drive is social still counts as available.
Without that carve-out, two colonists who both urgently need company can never
talk to each other — each disqualifies the other as a partner — and social
drive sits permanently pinned at its ceiling in any colony busy enough that
nobody is ever fully drive-free. That carve-out plus leaving live talks alone (above) is
what actually unpins the drive in a busy colony. One limitation remains, and it
is a design one rather than a bug: a partner must be at `Job == JobNone`, so a
colonist cannot chat *while* doing something else (mid-queue, mid-dig). Letting
them would need bigger, riskier surgery to the job model than has been
attempted.

### Starvation and healing

`applyStarvation` drains HP for every drive whose current band carries a
`ConsequenceHPDrain` (a fatal drive at `Max`), and tracks that damage
separately per drive in `driveState.hpDrained`. Satisfying the drive restores exactly
that deprivation damage (up to `MaxHP`) — so eating heals hunger damage but not an
unrelated alien bite. Three grace conditions prevent unfair deaths:

1. A colonist already committed to a *reachable* facility (`JobUse`) is not
   drained mid-queue — it gets time to traverse the crowd and finish eating.
2. A colonist that has already grabbed a portable drive (see *Taking it to go*
   below) is never drained regardless of the facility's reachability or
   crowding — it is guaranteed to finish; it just isn't there anymore.
3. While reachable life support is *under construction*, fatal-drive drain is
   suspended. This matters most at startup, when staggered hunger can hit `Max`
   just before the first facility room finishes.

### Which drive wins

`mostUrgentDrive` returns the drive furthest past its `SeekAt`, but a **fatal drive
outranks any non-fatal one**. Without that rule, bladder (which rises faster and
caps further past its threshold) would permanently outrank food and let colonists
starve while relieving themselves.

Sleep is deliberately non-fatal. A tired colonist seeks a reachable bunk and
spends a night (`UseTicks`, 360 by default; see [days.md](./days.md)) sleeping beside it. Food and toilets are
still planned before dormitories, so a bunk usually arrives later than the
first facility room — but a colonist with no reachable bunk, no bunk task to
help with, and no dormitory under construction anywhere reachable does not
simply wait forever: like any other drive (see *The emergency fallback* in
[construction.md](./construction.md)), it builds one for itself. This keeps
sleep a capacity and scheduling pressure in the common case, without letting a
delayed dormitory turn into an indefinite "stuck waiting" loop.

### Satisfying a drive

When a drive is urgent and a facility of the right kind that the colonist may
use is reachable (`facilityReachable`: a communal one via the shared field, or
its own private one — see [property.md](./property.md)), the
colonist normally takes a `JobUse` job. With fewer than two facilities of that
kind — and none of them private — there's nothing to choose between, so it
just follows that facility's shared **flow field** to the nearest one. The field
knows terrain but not pending construction, which `followField` refuses to step
on; when that leaves only an uphill step, the colonist routes by A* instead for
`fieldDetourTicks`. Without that, construction across the field's route had a
colonist pacing beside the colony's only toilet for hundreds of ticks. Once a second exists, `jobUse`
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
option is busy, so a drive is never declared unreachable and left to starve
merely because everything is momentarily full. Ties go to the lower
`lessPoint`. The choice is retained on the colonist for the whole `JobUse` job
(`e.useFacility`), so it never re-litigates and ping-pongs between queues as
counts change tick to tick.

#### How the choice is computed, and why not with one BFS

It used to be one BFS from the colonist over everything it could reach,
followed by a scan of every facility. That flood covered the whole reachable
map on every drive decision, even when the pod was three tiles away. On a big
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

`GrabTicks`, when positive and less than `UseTicks`, makes a drive portable:
the colonist spends only `GrabTicks` at the facility, then carries it away and
spends the rest of `UseTicks` finishing elsewhere (`jobUseCarrying`), freeing
the facility's access tile immediately rather than occupying it for the whole
`UseTicks`. Food is the only portable drive by default (`GrabTicks: 3` against
an 18-tick meal) — a colonist "sits there sucking down goop" for only 3 ticks,
then steps aside (`stepAside`, falling back to `wanderStep`) and finishes
eating out of everyone else's way. Bladder and sleep stay `GrabTicks: 0`
(in-place only): there is nothing to carry away from a toilet or a bed.

This is purely a throughput change — the total `UseTicks` a colonist spends
satisfying the drive is unchanged — but it turns a facility's *access-tile*
capacity from "one user every `UseTicks`" into "one user every `GrabTicks`,"
which is the real bottleneck once a facility is shared by more than a couple
of colonists: a single pod that could serve at most `UseTicks`⁻¹ colonists per
tick before now serves `GrabTicks`⁻¹.

## Why it is this way

- **Bands, not curves.** Each drive already scheduled the tick it would next
  cross a threshold. If what a drive does is constant inside a band, nothing has
  to be computed between crossings, so drives stay lazy and deterministic. A
  true curve would need re-evaluating every tick for anyone inside it. Ramps give
  curve-like authoring with step semantics, and a handful of small steps reads
  as smooth in play.
- **Thousandths, not points.** Needs rose an integer number of points a tick.
  Food rose 2, so it could only be halved or stopped, which is why the old sleep
  rule had to pause hunger outright. Fixed point lets any percent apply.
- **Rebase on change.** Folding growth into the base at the old rate before any
  rate change keeps base-plus-timestamp exact. The old sleep pause
  (`rebaseWakingNeeds`) did this for one case; `refreshDrive` does it for all
  of them.
- **Changes, not multipliers, for traits and effects.** With multipliers, 0
  had to mean "unset" so a trait could leave a drive alone, and then Asocial
  could not say "stops it" without a special flag (`socialNoNeed`). A percent
  change of 0 is naturally "no effect", and −100 is "stops".
- **Activity from the Activity tab.** The player-facing `Activity` already
  classifies every colonist-tick, and it is what a new job has to add a case
  to anyway. Filing those under four drive classes in one table means a new
  activity cannot be forgotten (a test fails) and a new class is a name.
  "Doing" is keyed on `State` because focus says what a colonist wants, not
  what it is doing: a colonist waiting for a free bed is not asleep.
- **True and felt levels.** Masking fatigue must not erase it, or there is no
  crash. Keeping the true level growing under the offset makes the crash fall
  out of the model.
- **The sleep drive is activity-neutral.** The calendar is derived from the
  sleep drive's base rate ([days.md](./days.md)). Scaling it by activity would
  make a day a different length for every colonist.
- **Fatal beats non-fatal.** The bands did not change arbitration. A fatal drive
  past its threshold still outranks a non-fatal one, a rule learned from
  colonists starving with a full bladder.
- **Per-drive HP drain** keeps healing intuitive: eating gives back hunger
  damage, not an alien bite.
- **Grace periods** stop the startup deaths where hunger outraced the very first
  pod.
- **Portable drives** (food) separate how long satisfying a drive takes from how
  long it occupies the one tile everyone else queues behind.

### Tuning the activity percents

Turning on activity-scaled hunger changed how much colonists eat. It was
retuned against the old game with `TestDriveTuningReport`: six seeds × 10,800
ticks of the default game, counting meals per colonist-day, finished and
interrupted nights, time in bed and mining, and starvation deaths.

| | needs (before) | drives, food 2000 / asleep 25% | drives as shipped |
| --- | --- | --- | --- |
| meals per colonist-day | 2.00 | 2.47 | 2.07 |
| nights finished / interrupted | 225 / 6 | 239 / 71 | 263 / 21 |
| in bed | 30.2% | 29.7% | 30.6% |
| mining | 19.3% | 17.0% | 19.3% |
| starved | 3 | 5 | 0 |

The first try, keeping food's old rate and letting food and bladder grow at a
quarter speed in bed, had colonists eating a quarter more and being pulled out
of bed eleven times as often. That is the failure the old pause was written
for. Food's base rate came down to 1750 and both asleep percents to 10%. Of the
21 interrupted nights, 13 are hunger and the rest are aliens (flee and fight),
as before. Dropping food's asleep rate further, to 5%, still left 11 hunger
interruptions: the colonists it wakes went to bed already just short of
`SeekAt`, so any growth at all wakes them. Banked sleep makes each one a short
break rather than a lost night, and hunger that never stops at night was the
point.

Mechanics tests do not use these numbers. `testConfig` sets every awake activity
to 100%, pauses everything but sleep in bed, and keeps food at its old 2 a
tick, because those tests were written against the old timings. The drive
model's own tests opt back in.

## Extending it

- **A new drive** is a table edit: a `DriveKind` before `numDrives` with its
  `String()` case, and its `DriveSpec` in `defaultDrives`. Then give it a
  satisfying `Terrain` facility (a flow field is allocated per facility terrain
  in `newWorld`) and a display `State` in `useState`, and regenerate
  `mars-sim.yaml` (`go run . -print-config > mars-sim.yaml`). Its settings appear
  as `drives.<name>.*`, and its activity percents default to 100.
- **A new drive activity** is an enum value before `driveActivityEnd` and a name
  in `driveActivityNames`. Every drive gets a `drives.<drive>.activity.<name>`
  setting at 100%. Then point rows of `activityDrives` at it.
- **A new player-facing `Activity`** needs its row in `activityDrives`.
  `TestEveryActivityHasADriveActivity` fails until it has one.
- **A new consequence** (movement speed, collapse) is a `ConsequenceKind`, its
  consumer reading the band table (`driveTable` holds a per-band vector for each
  kind, like `hpDrain`), and consequences or a ramp on the drive's spec. Keep
  consumers reading the band, not the level, so they only change at crossings.
- **A substance or event** is a profile in `Config.DriveEffects`, started with
  `applyEffect`. To make it player-tunable, a profile would need the config file
  to grow lists; until then it is code.
- **One drive affecting another** is a `ConsequenceRate` on the source drive
  targeting the other.
- **A trait that scales a drive** sets `driveRate` in its `traitSpec`.

Invariants to keep: every rate change goes through `refreshDrive` (never write
`rate` directly), and anything a colonist does that should change its rates
has to show up either in its `State`/job (so `syncDriveActivity` sees it) or as
a refresh call.

## Related

- [days.md](./days.md) — the sleep drive the calendar is derived from, the night in bed, and banked sleep.
- [personality.md](./personality.md) — the traits that change drive rates.
- [activity-screen.md](./activity-screen.md) — the `Activity` classification drive activities are filed from.
- [affect.md](./affect.md) — the mood system, the other half of a colonist's inner state.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — how drive pressure feeds focus arbitration.
- [entities-and-ai.md](./entities-and-ai.md) — how drives preempt work.
- [construction.md](./construction.md) — how the facilities that satisfy drives are built.
- [food.md](./food.md) — meals, and what hunger buys.
- [config-file.md](./config-file.md) — tuning a drive from `mars-sim.yaml` or a flag.

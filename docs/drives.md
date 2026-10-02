# Drives: growth, modifiers, consequences (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan to rework colonist **needs** into **drives**. A drive is a score with a
minimum and a maximum. Its growth rate is a base rate plus stacked modifiers:
what the colonist is doing, its traits, timed effects such as substances, and
other drives. Its consequences are declared as level **bands**, each carrying
effects. Drives are still evaluated lazily: whenever an input to the rate
changes, the level is folded into its base before the new rate applies.

Nothing here is built yet. This doc is the design, checked in ahead of the
code so it can be reviewed. When the implementation lands, this doc becomes the
system's write-up and replaces [needs.md](./needs.md).

## Why

Needs today ([`internal/sim/needs.go`](../internal/sim/needs.go)) are four
fixed-rate counters:

- The level is `base + rise × (now − since)`, with an integer `rise` per tick.
- Trait multipliers are baked into `needRise` at spawn.
- Consequences are hard-coded: the seek and critical thresholds feed focus
  pressure, and a fatal need at `Max` drains HP.

There is no way to say any of these:

- Hunger grows faster while hauling.
- Caffeine slows exhaustion.
- Drinking water fills the bladder over the next hour.
- Hungry people tire faster.

The one place a rate changes is a special case. Other needs pause in bed by
swapping the rate in [`sleep.go`](../internal/sim/sleep.go) (`wakeRise`).
Integer rates also ruled out anything between full speed and zero: food rises
2 per tick, so it could only be halved. See
[days.md](./days.md), "Why other needs pause in bed".

Situations the system must describe:

- Hunger always increases, but at a rate that depends on activity: slowest
  asleep, middling idle, higher working, highest during physical labor.
- Bladder increases at a rate that depends on water intake. Water intake comes
  in a later change; it is used here only as an illustration.
- Traits (Big Eater, Light Eater, …) affect drive growth. Later, substances and
  temporary effects will too.
- Drives affect one another, for example exhaustion building faster the hungrier
  a colonist is.

### Decisions

- **Consequences are discrete bands, with a ramp shorthand.** The engine only
  knows bands whose effects are constant. A ramp (from/to level, from/to value,
  N steps) expands into bands when the config loads. This keeps evaluation lazy
  and makes every change a scheduled crossing. A true curve would need
  re-evaluating every tick for anyone inside the ramp.
- **Scope of the first change: the framework plus a port of the existing
  needs.** Activity-scaled hunger (asleep < idle < working < labor) is turned on
  and retuned so average hunger matches today. Golden hashes are re-pinned.
- **The rename goes everywhere:** Go types, the config section and flags,
  `cognition.yaml`, wire JSON, TUI and web labels, Scum Lab, and docs.

## How it works

### Storage and lazy level (`internal/sim/drives.go`, replaces `needs.go`)

- **Fixed point.** `driveUnit = 1000`. Each entity has
  `drives [numDrives]driveState{base, since, rate, band, nextCrossing, hpDrained}`,
  with `base` and `rate` in thousandths of a point (rate is milli-points per
  tick). `int` is 64-bit on every target, including wasm.
- **True and felt levels.**
  `driveMilli(e,d) = clamp(base + rate×(now−since), Min×1000, Max×1000)` is the
  *true* level. `driveLevel(e,d)` is the *felt* level (the true level minus any
  active effect offsets; see *Effects and events*), divided by 1000. Every
  existing caller (focus, food, valuation, snapshot) keeps reading whole-point
  levels.
- **`refreshDrive(e,d)`** folds the elapsed growth into `base`/`since`,
  recomputes `rate`, and resyncs `band` and `nextCrossing`. Every change to a
  rate input goes through it. It generalizes today's `rebaseWakingNeeds`.

### Growth: base rate × modifiers

`rate = (spec.Rate + Σadd) × Π(percent)/100`. Sources apply in a fixed order,
with no maps, so the result is deterministic:

1. **Drive activity**, colonists only. Each drive has a percent for each drive
   activity. See *Drive activities* below for how the activity is chosen and
   how to add one.
2. **Traits.** `traitSpec.needRiseScale [numNeeds]float64` becomes
   `driveRate [numDrives]int`, a percent where 0 means unset. Values: Big Eater
   150, Light Eater 70, Introvert social 50, Extrovert social 150, Asocial
   social 0. Asocial's 0 replaces `socialNoNeed`, and `socialScale` folds into
   this field.
3. **Couplings**: a percent on this drive's rate from another drive's current
   band (see *Consequences*).
4. **Effects**: timed, started by events (see *Effects and events*).

Negative rates are allowed and are clamped by `Min` (default 0). This lets a
drive decay after a trigger.

### Effects and events (caffeine, water, later substances)

One event can do several things to several drives, and what it does can change
over time. Caffeine both slows exhaustion growth for a while and temporarily
lowers exhaustion, then crashes when it wears off. So an event applies an
**effect profile**. Profiles live in a named Go table (`driveEffects`, keyed by
`DriveEffectKind`), extensible the same way traits are:

```go
type DriveEffectProfile struct {
    Name   string
    Stages []DriveEffectStage // played in order; the effect ends after the last
}
type DriveEffectStage struct {
    Ticks int
    Drive [numDrives]DriveEffectTerms // zero value = no influence on that drive
}
type DriveEffectTerms struct {
    Instant int // applied to the true level once, on entering the stage (permanent)
    Offset  int // masks the level while the stage lasts (temporary): felt = true − Offset
    Add     int // milli-points/tick added to the rate
    Percent int // multiplier on the rate (0 = unset → 100)
}
```

- **Two levels.** The *true* level is the lazy `base + rate×dt`. The *felt*
  level is the true level minus all active offsets, clamped to `[Min, Max]`.
  Bands, phases, pressure, consequences and the snapshot all read the felt
  level. Satisfying a drive (`resetDrive`) resets the true level. That is what
  "temporarily decreases" means: sleep debt keeps building underneath and is
  only masked.
- **Caffeine example.** This is a test fixture, not shipped content. It has
  three stages:

  | Stage | Length | Sleep offset | Growth |
  | --- | --- | --- | --- |
  | Kick-in | 2h | 250 | 40% |
  | Fading | 1h | 120 | 70% |
  | Crash | 30min | 0 | 120% |

  When the offset drops, the felt level jumps back to the true level, which
  grew more slowly in the meantime. The jump is a scheduled band change, so the
  crash is real and visible.
- **Bladder after water** is a single stage: a bladder `Add` for N ticks. A
  permanent bump, such as a big meal sating hunger, is `Instant`.
- **Lazy-safe.** Within a stage the offset and rate are constant, so the felt
  level is linear and its band crossings can still be scheduled. Stage
  boundaries are scheduled refreshes. A gradual wear-off is several stages,
  following the same steps-not-curves rule as consequences. A ramp shorthand
  could expand into stages later if needed.
- **Storage.** Each entity has `e.effects []activeEffect{kind, stage, stageEnds}`,
  usually empty and applied in insertion order. `e.nextEffectTick` holds the
  next stage boundary and is checked in `syncCognitionDeadlines`, next to
  stimulus expiry.
- **API.** `w.applyEffect(e, kind)` starts a profile. Applying a profile that is
  already active either restarts it or stacks a second instance, set per
  profile by `Stacks bool` (caffeine on caffeine stacks). Traits can scale
  effects later through the same percent path.
- No effect content ships in the first change; caffeine and water exist only as
  tests.

### Drive activities (pluggable)

Two tables, both in `drives.go`, so a new kind of activity is a row edit:

- **`driveActivities [numDriveActivities]DriveActivitySpec{Name string}`**
  defines the activity classes: `asleep`, `idle`, `working` and `labor` to
  start. Adding one, say `exercising`, means appending an enum value and a name.
- **`DriveSpec.Activity [numDriveActivities]int`** holds each drive's percent
  per class. It is cfg-tagged as an indexed array, using the same
  `configArrayElementName` mechanism as `needs`/`focuses`, so it appears as
  `drives.food.activity.labor: 140` and as the flag
  `-drive-food-activity-labor`. A new class gets its config keys and flags
  automatically, and an unset value defaults to 100.
- **`activityDrive [NumActivities]struct{Doing, Walking DriveActivity}`** maps
  every player-facing `Activity` ([`activity.go`](../internal/sim/activity.go))
  to the class used while doing it and the class used while walking to it.
  Mining is `{labor, working}`. Hauling is `{labor, labor}`, because a carried
  load is heavy. Sleeping is `{asleep, working}`.
- **A test fails if any `Activity` lacks a row**, so adding `ActFarming` forces a
  decision about its drive activity. The zero value is an unset sentinel, so a
  missing row can't silently fall into a real class.
- **`driveActivityOf(e)`** is the table lookup on `activityOf(e)`. Nothing else
  hard-codes classes.

**What triggers `refreshDrive`:**

- A change of activity class. A new `w.syncDriveActivity(e)` runs from `step()`
  right after `colonistTurn`, next to `tallyActivity`. It compares the class
  against the stored `e.driveActivity` and rebases only on a change.
- `resolveTraitEffects`.
- A coupled drive changing band.
- An effect starting, changing stage, or ending.
- `resetDrive`.

**Cognition timing.** If a refresh moves `nextCrossing` earlier than
`e.nextThinkTick`, `nextThinkTick` is pulled in to match. The sleeping fast
path (`tryCognitionFastPath`) returns before `syncCognitionDeadlines`, so
without this a sleeper whose hunger now grows would miss a crossing.

### The sleep pause becomes data

The special-case pause code goes away: the rate swap in `fallAsleep`/`wakeUp`,
`pauseNeedsWhileAsleep`, `rebaseWakingNeeds`, and `wakeRise`. Each drive's
`activity.asleep` percent replaces it: social 0, bladder low, and food low but
not zero. Hunger still grows during sleep, just slowly.

`asleep`, `sleepBanked`, `sleepTicks` and the wake detection in `colonistTurn`
stay, because banking a night still needs them. The sleep drive itself stays at
100% in every class: `TicksPerDay` is derived from it, and the calendar must
not depend on activity.

### Consequences: bands, effects, ramps

- **Declaring them.** `DriveSpec.Consequences []DriveConsequence{From, To int; Effect DriveEffect}`
  and `Ramps []DriveRamp{From, To, FromValue, ToValue, Steps int; Effect DriveEffect}`
  are Go-table fields. They are untagged, because the config file can't
  express lists.
- **Compiling them.** At config validation, `compileDrive` expands ramps into
  consequences and builds a sorted list of band edges: 1 (satisfied→growing),
  `SeekAt`, `CriticalAt`, `Max`, and every consequence edge. Each band gets an
  effect vector and an urgency phase.
- **Effect kinds** in the first change, limited to those with a consumer:
  - `EffectHPDrain` is the per-tick damage `applyStarvation` applies.
    `fatal: true` compiles into a band at `[Max, Max]` with a drain of
    `StarveDamage`, so the `fatal` knob keeps working. All grace conditions stay
    as they are, and the per-drive `hpDrained` keeps the rule that eating heals
    only hunger damage.
  - `EffectRate{Target DriveKind, Percent}` is a coupling, for example "at
    hunger ≥ 650, exhaustion grows ×130%". When a drive's band changes, every
    drive targeted by the old or the new band is refreshed. No coupling ships by
    default; one is tested with a test config.
- **Phases.** `NeedPhase` becomes `DrivePhase` (satisfied, growing, pressing,
  critical), read off the band table. `markMindDirty` fires only when the
  urgency phase changes, as it does now. Pressure (`needPressure`) is
  unchanged.
- **Crossings.** `nextNeedPhaseTick` becomes `nextBandCrossing`, which handles
  both directions:
  - rate > 0: up to the next edge;
  - rate < 0: down to the current band's floor;
  - rate == 0: no crossing (0).

## Why it is this way

- **Bands rather than curves.** Each drive already schedules the next tick at
  which it crosses a threshold. If an effect is constant inside a band, nothing
  needs computing between crossings, so the simulation stays lazy and
  deterministic. A ramp gives curve-like authoring without per-tick evaluation.
- **Fixed point.** Integer rates per tick were a dead end. Food rising 2 per
  tick could only be halved or zeroed, which is why sleep had to pause the
  other needs outright instead of slowing them (see [days.md](./days.md)).
- **Rebase on change.** Folding elapsed growth into the base before any rate
  change keeps the base-plus-timestamp model correct. It is the move
  `rebaseWakingNeeds` already makes for sleep, generalized to every rate input.
- **True and felt levels.** A substance that masks fatigue must not erase the
  debt underneath it, or there is no crash. Keeping the two levels separate
  makes the crash fall out of the model.
- **Sleep stays activity-neutral.** The colony calendar is derived from the
  sleep drive's base rate (see [days.md](./days.md)). Scaling it by activity
  would make "a day" mean something different for each colonist.
- **Fatal-beats-non-fatal still applies.** Arbitration between drives does not
  change: a fatal drive past its seek threshold still outranks a non-fatal one
  (see [needs.md](./needs.md)).

## Extending it

- **A new consequence** (movement speed, collapse) is a new effect kind, its
  consumer, and a ramp or consequence in the drive's table.
- **A new drive activity** is an enum value and a name in `driveActivities`. Its
  config keys and flags appear automatically.
- **A new player-facing `Activity`** needs a row in `activityDrive`; a test
  enforces this.
- **A new substance or event** is a profile in `driveEffects`, started with
  `applyEffect`.
- **A cross-drive interaction** is an `EffectRate` consequence on the source
  drive that targets the other drive.

## Implementation plan

### Rename map (everywhere)

| Today | After |
| --- | --- |
| `NeedKind`, `NeedFood`, … | `DriveKind`, `DriveFood`, … |
| `NeedSpec` (`Rise`) | `DriveSpec` (`Rate`, milli-units) |
| `NeedPhase` | `DrivePhase` |
| `Config.Needs` | `Config.Drives` (`cfg:"drives" sec:"Drives"`) |
| `Entity.Need` (the `JobUse` target) | `Entity.Drive` |
| `needLevel`, `resetNeed`, `mostUrgentNeed`, `needPressure`, `needForFocus` | `drive*` equivalents |
| `FocusSpec.NeedWeight`; `need_weight` in `cognition.yaml` | `DriveWeight`; `drive_weight` |
| `AlienHungerRise`, `RatHungerRise`, `ChickenHungerRise` | `*HungerRate` (milli) |
| snapshot `Needs`, `NeedsMeta` | `Drives`, `DrivesMeta` |
| wire inspect `needs`, `NeedLevel` | `drives`, `DriveLevel` |
| Lab `LabNeeds`, `LabSettings`, situation `needs` | `drives` equivalents |

Drive ids stay `food`, `bladder`, `social` and `sleep`, so config keys read
`drives.food.rate` and flags `-drive-food-rate`. Before renaming the `need`
noun in `cognition.yaml`'s grammar, check whether it refers to this system.

### Files

- **New** `internal/sim/drives.go`, from `needs.go`:
  - types, defaults, compile and validate;
  - lazy math, refresh, bands, crossings, pressure;
  - the drive activity tables and effect profiles with their stages;
  - `applyStarvation`, `mostUrgentDrive`;
  - calendar helpers, with `TicksPerDay` updated for milli rates:
    `awake = ceil(SeekAt×1000/Rate)`.
- **New** `internal/sim/drives_test.go`, from `needs_test.go`, plus the new
  tests below.
- `internal/sim/sleep.go`: remove the rate swap; keep banking.
- `internal/sim/personality.go`: trait `driveRate` percents;
  `resolveTraitEffects` calls `refreshDrive` for each drive.
- `internal/sim/entity.go`: `driveState`, `driveActivity`, `effects`,
  `nextEffectTick`. `newEntity` sets per-kind base rates; aliens, rats and
  chickens only get a food rate, with no activity modifier.
- Other sim files:
  - `world.go`: staggered start; flow fields per facility.
  - `systems.go`: the `step` hook, `syncCognitionDeadlines`, the `JobUse`
    paths.
  - `activity.go`: `driveActivityOf`.
  - `configfile.go`: `configArrayElementName`, and the `drive-` flag prefix in
    `knobFlagName`.
  - Renames only: `focus.go`, `food.go`, `hall.go`, `chickens.go`,
    `scavenge.go`, `valuation.go`, `snapshot.go`, `lab.go`, `config.go`.
- Frontends and tools: `main.go`, `internal/ui/tui/render_roster.go`,
  `internal/wire/inspect.go`, `web/src/ui/inspect.ts`,
  `web/src/ui/InspectPanel.svelte` (heading "Drives"),
  `tools/scum-lab/{wasm/main.go,shared/sim.js,tools/focus.js}`,
  `cmd/mars-sim-wasm/main.go`.
- Config files: `cognition.yaml`. Regenerate `mars-sim.yaml` with
  `go run . -print-config > mars-sim.yaml`, re-applying any values it had set.
- `internal/sim/testdata/golden-hashes.txt`: re-pin with `-update`, and say so in
  the commit message.
- Docs:
  - This doc replaces `needs.md` and is rewritten as the shipped system's
    write-up, from [TEMPLATE.md](./TEMPLATE.md).
  - Update `days.md`: the pause is now `activity.asleep`, and its numbers need
    re-measuring.
  - Update `personality.md`, `entities-and-ai.md` and
    `cascading_wsts_architecture.md`.
  - Update `configuration.md` and `config-file.md` for the flag prefix, and
    `wire-format.md` if it describes inspect.
  - Update the [README index](./README.md), the top-level `README.md`, and every
    link to `needs.md` (`grep -rn needs.md docs README.md`).

### Tuning

Starting defaults (percent of the drive's base rate):

| Drive | Base rate | asleep | idle | working | labor |
| --- | --- | --- | --- | --- | --- |
| food | 2000 milli | 25 | 75 | 100 | 140 |
| bladder | as today | 25 | 100 | 100 | 100 |
| social | as today | 0 | 100 | 100 | 100 |
| sleep | as today | 100 | 100 | 100 | 100 |

Compare a baseline from `main` with the new defaults on six seeds × 10,800
ticks, the method [days.md](./days.md) used. Measure:

- meals per colonist-day;
- interrupted vs finished nights;
- percent of time in bed;
- starvation deaths;
- tiles dug.

Then adjust:

- Change food's base rate until meals per colonist-day is within about 5% of
  the baseline.
- If interrupted nights rise noticeably, lower food's and bladder's
  `activity.asleep`. Hunger pulling colonists out of bed was the original
  failure behind the sleep pause.

Record the final numbers here and in `days.md`.

### Tests to add

- **Rebase correctness:** the lazy level equals a per-tick reference simulation
  across activity changes, trait changes and effect stages.
- **Stacking:** modifier order and integer rounding are deterministic. Big Eater
  × labor × a caffeine-like percent composes as specified.
- **Activity classes:** a sleeping colonist's food grows at the asleep rate,
  mining beats idle, and social doesn't grow in bed.
- **Effects:**
  - `Instant` bumps the true level and clamps.
  - `Offset` lowers only the felt level, while the true level keeps growing.
  - A stage change and the final expiry trigger a refresh, even on the sleeping
    fast path.
  - The caffeine fixture shows slowed growth and a lower felt level, then the
    crash back up to the true level.
  - Stacking and restarting work as specified.
  - Water's timed `Add` stops at expiry.
- **Drive activity tables:** every `Activity` has a row (no unset sentinel), and
  a test-only extra drive activity gets its config knobs and defaults to 100%.
- **Negative rate:** decays to `Min` and schedules a downward crossing.
- **Ramp expansion:** N steps produce the expected edges and values, and edges
  dedupe with `SeekAt`/`CriticalAt`.
- **Coupling:** entering hunger's band changes the target drive's rate from that
  tick, and leaving the band restores it.
- **Fatal:** `fatal` compiles to an HP-drain band, and the existing starvation
  grace tests still pass.
- **Config:** `-drive-food-rate` and `drives.food.activity.asleep` round-trip
  (`configfile_test.go`), and validation rejects a bad ramp or edge order.
- **Ports:** the existing needs, sleep, personality and focus tests, under the
  new names.

### Verification

1. `go build ./...` and `go test ./...` pass. Re-pin the golden hashes only
   after everything else passes, and confirm the lockstep determinism test
   still passes ([determinism.md](./determinism.md)).
2. In `web/`, `npm run check` and `npm test` pass; the wire inspect topic and
   the web panel both change.
3. Run `go run . -print-config > mars-sim.yaml` and diff it: the `drives:`
   section should be the only change.
4. The tuning numbers are within target of the baseline, as above.
5. In the browser build, open a colonist's inspect panel. The Drives bars should
   move faster while mining than while idle, and food should creep up slowly
   overnight.

## Related

- [needs.md](./needs.md) — the system this replaces.
- [days.md](./days.md) — the sleep pause and the calendar derived from sleep.
- [personality.md](./personality.md) — the traits that become rate modifiers.
- [affect.md](./affect.md) — the mood system this takes inspiration from.
- [activity-screen.md](./activity-screen.md) — the `Activity` classification
  drive activities map from.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — how need
  pressure feeds focus arbitration.

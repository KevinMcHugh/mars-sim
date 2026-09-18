# Cascading internal state and weighted focus: execution plan

> Part of the [mars-sim documentation](./README.md).

## Purpose

This is the mutable execution companion to
[`cascading_wsts_architecture.md`](./cascading_wsts_architecture.md). The design
document defines the intended architecture and remains fixed during
implementation. This document tracks what has been attempted, completed,
measured, changed, or blocked.

The executing agent must update this plan as part of each implementation phase.
Do not mark a phase complete until its acceptance checks and required commands
have passed. If implementation reveals a design problem, record it under
[Decision and deviation log](#decision-and-deviation-log); do not silently
rewrite the design document.

## Status protocol

Use exactly these phase states:

- `not started`
- `in progress`
- `blocked`
- `done`

When starting work:

1. Set `Current phase`.
2. Set that phase to `in progress` in the summary.
3. Record the baseline commit.
4. Check only tasks actually completed.

When finishing a phase:

1. Run the phase's focused tests.
2. Run `gofmt` on changed Go files.
3. Run `go build ./...` and `go test ./...`.
4. Update generated configuration when required.
5. Record commands and outcomes in the phase evidence section.
6. Record benchmark results when the phase changes a hot path.
7. Set the phase to `done`.
8. Set `Current phase` to the next phase or `complete`.

Use `[x]` only for verified work, `[ ]` for pending work, and `[-]` for work
intentionally omitted after a recorded decision. Prefer one reviewable
commit/checkpoint per phase. If commits are made, record their hashes.

## Current status

**Current phase:** Phase 4 — charge/grip affect

**Overall state:** in progress

**Implementation branch:** `cascading-wsts-implementation`

**Baseline commit:** `c67068b`

**Last updated by:** Delta agent, 2026-09-18

| Phase | Design mapping | State | Commit/checkpoint | Verification summary |
| --- | --- | --- | --- | --- |
| 0. Baseline and code map | prerequisite | done | `c8c1cd5` | Build/tests pass; full benchmark suite and idle baseline recorded |
| 1. Weighted focus | design milestone 1 | done | `0409ac2` | Deterministic weighted arbitration over existing executors; all tests pass |
| 2. Independent need phases | design milestone 2 | done | `54b8946` | Independent phases, exact pressure, and boundary scheduling; all tests pass |
| 3. Active stimuli | design milestone 3 | done | `26d8453` | Bounded deterministic stimuli integrated with life events and focus; all tests pass |
| 4. Charge/grip affect | design milestone 4 | not started | — | — |
| 5. Cognition caching and performance | performance follow-up | not started | — | — |
| 6. Colony tuning | design milestone 5 | not started | — | — |
| 7. Final integration review | completion gate | not started | — | — |

## Rules for the executing agent

- Read the complete fixed design before changing code.
- Inspect current code and tests; filenames and APIs may have moved since the
  plan was written.
- Preserve unrelated branch changes.
- Keep every intermediate phase buildable and tested.
- Do not combine focus selection with exact target search or pathfinding.
- Do not introduce random focus tie-breaking.
- Do not retain two behaviorally active sources of truth during a migration.
- Preserve the one life-event ingestion funnel.
- Preserve personality/simulation RNG stream separation.
- Put every new user-facing tunable in `sim.Config` with `cfg` and `doc` tags,
  add a default, test generated keys/flags, and regenerate `mars-sim.yaml`.
- Do not optimize by making off-screen entities simulate differently. Simulation
  scope is independent of the current frontend viewport.
- Do not edit the fixed design to make implementation appear compliant. Record
  deviations here.

## Phase 0 — Baseline and code map

### Objective

Establish a reproducible starting point, locate the invariants the refactor must
preserve, and collect performance evidence before changing the hot path.

### Tasks

- [x] Record `git rev-parse HEAD` in `Baseline commit`.
- [x] Confirm the worktree is clean except for intentional plan/design changes.
- [x] Read `AGENTS.md`.
- [x] Read:
  - [x] [`cascading_wsts_architecture.md`](./cascading_wsts_architecture.md)
  - [x] [`entities-and-ai.md`](./entities-and-ai.md)
  - [x] [`needs.md`](./needs.md)
  - [x] [`personality.md`](./personality.md)
  - [x] [`memories.md`](./memories.md)
  - [x] [`configuration.md`](./configuration.md)
  - [x] [`config-file.md`](./config-file.md)
- [x] Map current colonist decision order in `colonistTurn`.
- [x] Map every `JobKind` claim/acquire/release path.
- [x] Map every `remember`/life-event call site.
- [x] Map scalar mood reads, writes, snapshot fields, UI rendering, and tests.
- [x] Map need reads, resets, starvation grace, and facility fallbacks.
- [x] Identify existing benchmarks and long-running fixed-seed tests.
- [x] Run baseline build and test commands.
- [x] Run baseline simulation benchmarks with allocation reporting.
- [x] Add missing benchmark coverage before behavior changes if current
  benchmarks cannot measure an established or idle colony.
- [x] Record baseline results below.

### Required baseline commands

```sh
go build ./...
go test ./...
go test ./internal/sim -run '^$' -bench . -benchmem
```

Use narrower benchmark selectors if the full benchmark suite is prohibitively
slow, but record the exact command. Do not trim benchmark output before
recording it.

### Baseline evidence

| Command or benchmark | Result | Notes |
| --- | --- | --- |
| `go build ./...` | pass | Baseline `c67068b`, Apple M1 Pro / darwin arm64 |
| `go test ./...` | pass | All four packages passed |
| simulation benchmark | `BenchmarkStep500`: 21,904,075 ns/op, 2,328,241 B/op, 11,638 allocs/op | Full command: `go test ./internal/sim -run '^$' -bench . -benchmem` |
| established/idle colony benchmark | `BenchmarkStepIdle500`: 18,357,399 ns/op, 2,104,874 B/op, 2,010 allocs/op | Added at `c8c1cd5`; command: `go test ./internal/sim -run '^$' -bench '^BenchmarkStepIdle500$' -benchmem` |

### Code map and risks discovered

_Update during Phase 0. Link exact files/functions and note ownership,
ordering, or determinism traps._

- [`colonistTurn`](../internal/sim/systems.go) currently applies starvation and
  removes the dead, observes nearby entities/gore, applies uranium exposure,
  handles a visible alien, handles the single most urgent need, continues a
  current job, searches for work, then performs idle talk/stomp/rest behavior.
  Phase 1 must replace the threat/need/work priority ladder, while preserving
  the first three operations and their same-turn effects.
- `JobMine` claims through `jobBoard` either when a small-colony target is
  selected or when a flow-field miner reaches the frontier. `clearJob` releases
  only a held `mineClaimed` tile. `JobBuild` owns either a `buildTask` or an
  ad-hoc build counter; `clearJob` releases the task owner or decrements the
  counter. `JobClean` claims its refuse tile during `cleanGather` and releases
  it on completion or abandonment; the `cleanHaul` stage deliberately carries
  no tile claim. `JobUse` owns only its selected facility/path state. `JobTalk`
  owns a mutual partner link and shared progress enforced by `jobTalk`.
  `JobStore` has no reservation and revalidates destination capacity during
  execution. All common teardown converges on
  [`clearJob`](../internal/sim/systems.go).
- Production life events enter [`remember`](../internal/sim/world.go) from
  combat (`combat.go`), sanitation (`cleaning.go`), mutation (`mutation.go`),
  and observation, conversation, work completion, need completion, alien
  attacks, and cat catches (`systems.go`). Tests call the same funnel directly.
  `remember` collapses/appends memory and then applies mood once per occurrence;
  Phase 3/4 must extend this funnel rather than create parallel APIs.
- Scalar mood is stored on `Entity`, configured by `MoodMax`,
  `MoodCompanyWeight`, and `MoodConversationWeight`, written only through
  `remember` -> `applyMoodEffects` -> `adjustMood`, and read by snapshots and
  the TUI roster. Migration coverage lives in `lifeevents_test.go`,
  `memories_test.go`, `mutation_test.go`, `relationships_test.go`,
  `snapshot.go`, and `render_roster.go`.
- Need levels are lazy base-plus-timestamp values read by starvation, urgency,
  mouse behavior, availability checks, and snapshots. `resetNeed` is the sole
  reset and restores only deprivation damage. Starvation grace covers carried
  portable food, a reachable matching `JobUse`, and reachable matching
  construction. Need execution may use a reachable facility, claim a matching
  project task, perform an emergency build, or wait when all matching tasks are
  claimed.
- Determinism traps: entity turns are hunger-first with stable ID fallback;
  candidate ties must not consume RNG; personality uses `World.prng`, while
  gameplay uses `World.rng`; point and entity ties already use stable ordering.
  Focus generation must not claim targets or call A*.
- Existing representative coverage includes `TestDeterministicRun`,
  `TestColonyKeepsExcavatingOnceNeedsBite`,
  `TestColonyDoesNotStarveOverTime`,
  `TestLargeColonyDoesNotGridlockAtFacilities`,
  `TestColonyBuildsATrashRoomAndBurnsItsRefuse`, and the benchmark suite in
  `bench_test.go`. The idle benchmark also reveals that observation and entity
  turn setup still allocate and scan even when cognition rests; that is Phase
  5 work, not a Phase 1 shortcut.

### Phase 0 exit criteria

- [x] Baseline build and tests pass.
- [x] At least one representative simulation benchmark is recorded.
- [x] The code map identifies claim release, life-event ingestion, scalar mood,
  need urgency, and snapshot/UI migration points.
- [x] Any pre-existing failures are recorded and separated from implementation
  failures.

### Phase 0 verification log

_Record date/agent, commands, results, and checkpoint._

- 2026-09-17, Delta agent: baseline `c67068b`; `go build ./...`,
  `go test ./...`, and the full `internal/sim` benchmark suite passed. Added
  `BenchmarkStepIdle500` at checkpoint `c8c1cd5` because the existing
  benchmarks did not isolate the established/resting-colony path. No
  pre-existing failures found.

## Phase 1 — Weighted focus over existing behavior

### Objective

Introduce deterministic, explainable focus arbitration above the existing jobs
without yet changing need storage or scalar mood.

### Implementation tasks

- [x] Add `internal/sim/focus.go`.
- [x] Define `FocusKind` and stable `String` values.
- [x] Define `FocusSpec`, `FocusScore`, and `FocusCandidate`.
- [x] Add `focus` and `focusSince` to colonists.
- [x] Initialize colonists to a valid focus.
- [x] Add `Config.Focuses` and global commitment/switch settings.
- [x] Extend config traversal to name both `Needs` and `Focuses` arrays by their
  enums instead of assuming `Needs` is the only spec array.
- [x] Add config validation and default values from the design.
- [x] Add generated config/flag tests for every focus.
- [x] Regenerate `mars-sim.yaml`, preserving intentional committed overrides.
- [x] Implement allocation-free candidate generation.
- [x] Query each shared fact at most once per arbitration:
  - [x] visible threat
  - [x] current need levels
  - [x] current job validity
  - [-] cheap facility/flow-field distance where relevant (D-001)
- [x] Give a currently visible alien the final `EvtSawAlien` flee/fight score
  contribution directly, without storing the Phase 3 stimulus buffer yet.
  Phase 3 must route this through refreshed stimulus state without double
  counting it.
- [x] Implement additive score components and `Total`.
- [x] Implement current-focus commitment.
- [x] Implement strict switch margin.
- [x] Implement deterministic tie ordering.
- [x] Implement hard eligibility separately from weights.
- [x] Implement a stable debug formatter for candidate breakdowns.
- [x] Refactor `colonistTurn` to:
  - [x] preserve starvation/removal ordering
  - [x] preserve observation and uranium behavior
  - [x] arbitrate focus
  - [x] transition focus through `clearJob`
  - [x] run the existing executor
- [x] Map need-focused facility construction to the originating need focus.
- [x] Preserve live-conversation progress.
- [x] Preserve portable-food progress and starvation grace.
- [x] Preserve idle step-aside, opportunistic talk, and pest behavior under
  `FocusIdle`.
- [x] Keep scalar mood temporarily and confirm it does not affect focus scores.
- [x] Expose current focus through snapshots without removing display `State`.

### Required tests

- [x] Score components sum exactly to total.
- [x] Current focus receives commitment only while eligible.
- [x] Challenger must exceed current focus by the strict margin.
- [x] Equal scores resolve deterministically.
- [x] Ineligible candidates cannot win.
- [x] Fatal urgent food beats urgent non-fatal needs.
- [x] Starting threat weights beat critical hunger.
- [x] Armed threat response can select fight; unarmed cannot.
- [x] Lower grip/high grip tests are deferred until Phase 4 and marked TODO
  without being skipped failures.
- [x] Existing mutually urgent conversation test passes.
- [x] Existing portable-food tests pass.
- [x] Existing starvation/facility fallback tests pass.
- [x] Job/project claims are released on focus transition.
- [x] Fixed-seed outcomes are deterministic across repeated runs.

### Performance tasks

- [x] Add `BenchmarkFocusCandidates` or an equivalent direct benchmark.
- [x] Report allocations.
- [x] Use fixed-size/caller-owned candidate storage in the simulation hot path.
- [x] Confirm score explanation formatting is not called in the normal hot path.
- [x] Confirm candidate scoring performs no A*, full-map scan, or target claim.
- [x] Compare representative simulation benchmarks with Phase 0.
- [x] If steady-state CPU regresses by more than 10%, profile before proceeding
  and record the cause. Do not hide the regression with an arbitrary cadence.
- [x] `BenchmarkFocusCandidates` reports `0 allocs/op`.

### Phase 1 evidence

| Evidence | Result | Notes |
| --- | --- | --- |
| focused tests | pass | Focus arithmetic, eligibility, margins, ties, needs, threats, claim release, snapshots, and config names |
| `go build ./...` | pass | 2026-09-17 |
| `go test ./...` | pass | All four packages |
| `BenchmarkFocusCandidates` | 47.98 ns/op, 0 B/op, 0 allocs/op | Urgent-need candidate included |
| simulation comparison | `BenchmarkStep500` 21,723,804 ns/op; idle 18,480,421 ns/op | Versus Phase 0: -0.82% active, +0.67% idle; below profiling threshold |
| generated config diff reviewed | pass | Added all focus specs as commented defaults; no committed overrides existed |

### Phase 1 exit criteria

- [x] Existing jobs execute behind `FocusKind`.
- [x] No duplicated priority ladder remains behaviorally active.
- [x] Candidate scores are inspectable and allocation-free.
- [x] Existing safety, needs, claims, and conversations remain correct.
- [x] Required configuration and documentation are current.

### Phase 1 verification log

- 2026-09-17, Delta agent: implemented weighted focus arbitration at
  `0409ac2`. `go build ./...`, `go test ./...`, `git diff --check`, focused
  benchmarks, existing fixed-seed tests, long-running colony tests, and TUI
  tests pass. Active and idle CPU stayed within 1% of Phase 0; direct candidate
  generation is allocation-free.

## Phase 2 — Independent need phases

### Objective

Add one discrete phase machine per need while preserving lazy level storage and
existing starvation semantics.

### Implementation tasks

- [x] Add `NeedPhase` and one phase slot per need.
- [x] Add `CriticalAt` to `NeedSpec`.
- [x] Add validation for `0 <= SeekAt <= CriticalAt <= Max`.
- [x] Add defaults specified by the design.
- [x] Implement `syncNeedPhase`.
- [x] Implement pressure normalization exactly as specified.
- [x] Synchronize phases:
  - [x] before focus arbitration
  - [x] after `resetNeed`
  - [-] after any future partial need adjustment (no partial-adjustment API exists)
- [x] Make need candidate scoring consume phase and normalized pressure.
- [x] Remove duplicated threshold/urgency arithmetic from focus selection.
- [x] Preserve fatal-over-nonfatal semantics through `FocusFatalBonus` and hard
  fatal eligibility precedence.
- [x] Calculate and store/test the next phase-boundary tick for later caching.
- [x] Regenerate `mars-sim.yaml`.
- [x] Update [`needs.md`](./needs.md) to describe phases and `CriticalAt`.

### Required tests

- [x] `NeedSatisfied -> NeedGrowing`.
- [x] `NeedGrowing -> NeedPressing`.
- [x] `NeedPressing -> NeedCritical`.
- [x] reset returns to `NeedSatisfied`.
- [x] lazy elapsed time crosses phases correctly.
- [x] `SeekAt == CriticalAt`.
- [x] `CriticalAt == Max`.
- [x] zero rise does not schedule a crossing.
- [x] Asocial social need remains satisfied/growing without becoming pressing.
- [x] fatal critical food eventually beats ordinary work commitment.
- [x] non-fatal need pressure cannot indefinitely suppress fatal food.
- [x] starvation grace and healing remain unchanged.

### Phase 2 evidence

| Evidence | Result | Notes |
| --- | --- | --- |
| focused need tests | pass | Phase transitions, resets, lazy crossings, degenerate thresholds, zero rise, urgency, starvation, and healing |
| `go build ./...` | pass | 2026-09-17 |
| `go test ./...` | pass | All four packages |
| simulation benchmark | active 21,397,454 ns/op; idle 18,687,577 ns/op; focus 64.63 ns/op, 0 allocs | Versus Phase 1: -1.50% active, +1.12% idle; no profiling threshold crossed |
| generated config diff reviewed | pass | Added four commented `critical-at` defaults; no committed overrides existed |

### Phase 2 exit criteria

- [x] Every need has an independently correct phase.
- [x] Lazy need computation remains the only source of current level.
- [x] Focus scoring reads need pressure through one helper.
- [x] Config and need documentation are current.

### Phase 2 verification log

- 2026-09-17, Delta agent: implemented independent need phases at `54b8946`.
  `go build ./...`, `go test ./...`, `git diff --check`, focused need/focus tests,
  and representative benchmarks pass. Phase synchronization preserves one lazy
  level read per candidate, and direct focus generation remains allocation-free.

## Phase 3 — Active stimuli

### Objective

Separate actionable recent context from accumulated affect and long-term
memory, while preserving the single life-event ingestion funnel.

### Implementation tasks

- [x] Add fixed-capacity stimulus storage; avoid one heap allocation per event.
- [x] Add `StimulusSpec` indexed by `LifeEventKind`.
- [x] Add the initial non-zero stimulus table from the design.
- [x] Implement deterministic coalescing by `(Kind, Source)`.
- [x] Implement deterministic expiry.
- [x] Implement deterministic full-buffer eviction:
  - [x] lowest salience
  - [x] earliest expiry
  - [x] lowest source ID
- [x] Maintain or cheaply derive aggregate per-focus stimulus bias.
- [x] Route stimulus insertion through the life-event funnel.
- [x] Refresh ongoing visible-threat context.
- [x] Remove direct threat eligibility as soon as the threat is no longer live
  or visible.
- [x] Keep lingering affect/memory independent from threat eligibility.
- [x] Ensure zero-spec events still record memories normally.
- [x] Mark focus dirty/reconsiderable when a behaviorally relevant stimulus is
  inserted, changed, or expires.

### Required tests

- [x] Same-kind/source stimuli coalesce.
- [x] Different sources remain distinguishable.
- [x] Expiry is exact at the boundary tick.
- [x] Full-buffer eviction follows the total deterministic ordering.
- [x] New low-salience events do not hide a live alien.
- [x] Removing an alien removes flee/fight eligibility immediately.
- [x] Lingering stimulus expiry does not erase affect or memory.
- [x] Zero-spec life events do not create stimuli.
- [x] Repeated edge-triggered sightings do not spam memories.
- [x] Memory collapse behavior is unchanged.

### Performance tasks

- [x] Stimulus update has bounded work.
- [x] Normal arbitration does not allocate while reading stimuli.
- [x] Compare simulation and focus benchmarks with Phase 2.
- [x] Profile any regression over 10%.

### Phase 3 evidence

| Evidence | Result | Notes |
| --- | --- | --- |
| focused stimulus tests | pass | Coalescing, sources, exact expiry, eviction ordering, weak incoming events, zero specs, threat refresh/removal |
| memory/life-event tests | pass | Full suite pins edge-triggering, collapse, scalar mood independence, and deterministic ingestion |
| `go build ./...` | pass | 2026-09-18 |
| `go test ./...` | pass | All four packages |
| benchmark comparison | profiled | Focus 94.61 ns/op, update 22.27 ns/op, both 0 allocs; active 27.46–27.59 ms/op and idle 23.57–23.70 ms/op versus Phase 2's 21.40/18.69 ms. CPU profile attributes 84% cumulative idle time to pre-existing observation/entity sorting; `focusCandidates` is 3.5% cumulative. Phase 5 owns that hot path. |

### Phase 3 exit criteria

- [x] Stimulus, affect placeholder/scalar mood, and memory have distinct
  lifetimes and ownership.
- [x] The life-event funnel remains the only ingestion path.
- [x] Immediate threat behavior is correct and deterministic.

### Phase 3 verification log

- 2026-09-18, Delta agent: implemented bounded active stimuli at `26d8453`.
  `gofmt`, `go build ./...`, `go test ./...`, `git diff --check`, focused
  stimulus/life-event tests, and allocation benchmarks pass. Both update and
  arbitration remain allocation-free. The repeatable simulation benchmark
  increase exceeded 10%, so an idle CPU profile was captured: existing global
  entity sorting in `observeNearby` dominates while focus candidate generation
  accounts for 3.5% cumulative CPU; optimization remains scoped to Phase 5.

## Phase 4 — Charge/grip affect

### Objective

Replace scalar mood completely with charge/grip affect, event-vector appraisal,
trait transforms, decay, contextual labels, and focus contributions.

### Migration tasks

- [ ] Inventory scalar mood producers and consumers again immediately before
  migration.
- [ ] Add `internal/sim/affect.go`.
- [ ] Add `AffectState`, `MoodVector`, `MoodKind`, and attractor specs.
- [ ] Add the complete event-vector table from the design.
- [ ] Implement vector addition and clamping.
- [ ] Implement deterministic conversation outcome conversion.
- [ ] Implement trait transforms:
  - [ ] Tidy gore amplification
  - [ ] Tidy incineration relief
  - [ ] Industrious finished-work amplification
  - [ ] Mutant-Lover grip reflection
  - [ ] Introvert charge reflection
- [ ] Implement deterministic cartesian decay toward home.
- [ ] Implement contextual valence for display naming only.
- [ ] Implement attractor claims and deterministic ties.
- [ ] Implement label hysteresis.
- [ ] Add charge/grip score contributions to focus arbitration.
- [ ] Keep label strings out of behavioral branches.
- [ ] Migrate life-event scalar effects to vectors.
- [ ] Preserve computed per-occurrence conversation effects.
- [ ] Delete scalar mood storage and adjustment helpers.
- [ ] Delete or migrate scalar mood config fields that no longer have meaning.
- [ ] Update snapshots with charge, grip, and mood label.
- [ ] Update roster rendering without increasing required roster height.
- [ ] Update [`memories.md`](./memories.md),
  [`personality.md`](./personality.md), and other affected docs.

### Required tests

- [ ] Every non-zero event-vector row is covered by table or behavior tests.
- [ ] Vector addition clamps both axes.
- [ ] Positive, zero, and negative conversations map correctly.
- [ ] Every trait transform is pinned.
- [ ] Trait transform ordering is deterministic.
- [ ] Charge decays faster than grip with defaults.
- [ ] Decay does not overshoot home.
- [ ] Attractor ties use declaration order.
- [ ] Label hysteresis prevents boundary flicker.
- [ ] Context changes good/bad wording without changing affect coordinates.
- [ ] Derived valence does not feed focus scoring.
- [ ] Low grip favors flee over fight when both are eligible.
- [ ] High grip favors fight over flee when both are eligible.
- [ ] Low charge favors sleep.
- [ ] High charge favors work, subject to stronger needs/threats.
- [ ] Tidy/Industrious/Mutant-Lover/Introvert behavior remains semantically
  correct.
- [ ] No scalar mood field or behaviorally active scalar mood config remains.

### Performance tasks

- [ ] Affect update uses integer arithmetic.
- [ ] Mood labeling is cached or calculated only when affect/context changes.
- [ ] No label/debug string formatting occurs in normal focus scoring.
- [ ] Compare simulation and focus benchmarks with Phase 3.
- [ ] Profile any regression over 10%.

### Phase 4 evidence

| Evidence | Result | Notes |
| --- | --- | --- |
| affect tests | pending | |
| relationship/conversation tests | pending | |
| snapshot/TUI tests | pending | |
| `go build ./...` | pending | |
| `go test ./...` | pending | |
| benchmark comparison | pending | |

### Phase 4 exit criteria

- [ ] Charge/grip is the only mood representation.
- [ ] Affect influences focus only through numeric coordinates.
- [ ] UI and snapshots expose the new representation coherently.
- [ ] All scalar mood tests and docs have been migrated or intentionally
  removed with a recorded reason.

### Phase 4 verification log

- Pending.

## Phase 5 — Cognition caching and performance

### Objective

Avoid full arbitration for colonists whose cached focus cannot yet change,
especially idle and sleeping colonists, without changing simulation outcomes or
making off-screen entities special.

Implement optimizations in the listed order. Stop when benchmarks meet the
agreed target; mark later steps `[-]` and record why. Do not build a dormant
entity scheduler merely because it is listed as a possible future optimization.

### Step A: Cheap cached cognition

- [ ] Add `mindDirty` and `nextThinkTick`.
- [ ] Centralize `markMindDirty`.
- [ ] Mark dirty on:
  - [ ] need phase transition
  - [ ] salient event/stimulus insertion
  - [ ] relevant stimulus expiry
  - [ ] threat appearance/disappearance
  - [ ] job completion/failure/invalidation
  - [ ] focus transition
- [ ] Cache next need phase crossing.
- [ ] Cache next stimulus expiry.
- [ ] Reuse aggregate stimulus focus bias.
- [ ] Separate `chooseFocus` from `runFocus`.
- [ ] Skip arbitration while focus is valid, not dirty, and before
  `nextThinkTick`.
- [ ] Keep a bounded fallback reconsideration deadline.

### Step B: Idle and sleeping fast paths

- [ ] Add an O(1), allocation-free fast path for resting idle colonists.
- [ ] Add an O(1), allocation-free fast path for in-place sleeping.
- [ ] Preserve immediate checks for:
  - [ ] live threats
  - [ ] fatal need deadlines
  - [ ] job/facility invalidation
  - [ ] uranium exposure where applicable
- [ ] Derive timed action progress from start/deadline when safe instead of
  incrementing cognition state merely to count ticks.
- [ ] Confirm sleepers remain targetable, observable, and interruptible.
- [ ] Confirm snapshot state remains correct while cognition is skipped.

### Step C: Validity horizon

Implement only if benchmarks justify it.

- [ ] Record winner/runner-up margin after arbitration.
- [ ] Define and test a conservative maximum relative score drift per tick.
- [ ] Set a safe `nextThinkTick` from margin/drift.
- [ ] Cap the horizon at internal deadlines.
- [ ] External events invalidate the horizon immediately.
- [ ] Prove with differential tests that cached and always-arbitrate modes choose
  the same focuses for fixed scenarios.

### Step D: Revision-keyed world caches

Implement only when a profile identifies repeated world feasibility queries.

- [ ] Add narrow revisions for the measured dependency, not one global world
  revision.
- [ ] Candidate cache keys include relevant region/position identity.
- [ ] Facility changes invalidate facility facts only.
- [ ] Job-board/project changes invalidate work facts only.
- [ ] Region changes invalidate reachability facts only.
- [ ] Add stale-cache regression tests.

### Explicitly deferred unless separately approved

- [ ] `[-]` Push-based perception notifications from every moving entity.
- [ ] `[-]` A dormant-entity heap or timing wheel.
- [ ] `[-]` Active-only replacement for global entity turn sorting.
- [ ] `[-]` A precomputed cartesian product of all mental states.

If profiling identifies global turn sorting as the dominant remaining cost,
record it under [Open issues](#open-issues) and propose a separate scheduler
design. Do not fold that architectural change into this implementation without
approval.

### Required benchmarks and tests

- [ ] `BenchmarkFocusCandidates` remains `0 allocs/op`.
- [ ] Add resting-colonist fast-path benchmark.
- [ ] Add sleeping-colonist fast-path benchmark.
- [ ] Compare active, idle, and mixed colonies with Phase 0 and Phase 4.
- [ ] Add differential always-arbitrate versus cached-decision tests.
- [ ] Add interruption tests for a sleeping colonist.
- [ ] Add fatal-need deadline test for a sleeping colonist.
- [ ] Add stale-target/facility invalidation test.
- [ ] Confirm fixed-seed determinism.
- [ ] Run a CPU profile if the mixed-colony benchmark is slower than Phase 4.

### Performance results

| Scenario | Baseline | Phase 4 | Phase 5 | Delta vs. Phase 4 | Allocations | Notes |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| focus scoring | pending | pending | pending | pending | pending | |
| active colony | pending | pending | pending | pending | pending | |
| established/idle colony | pending | pending | pending | pending | pending | |
| sleeping colonists | pending | pending | pending | pending | pending | |
| mixed colony | pending | pending | pending | pending | pending | |

### Phase 5 exit criteria

- [ ] Focus scoring is allocation-free.
- [ ] Idle and sleeping colonists avoid unnecessary arbitration.
- [ ] Cached mode is behaviorally equivalent to always-arbitrate mode in
  differential tests.
- [ ] Threats, fatal needs, and invalidated jobs still interrupt correctly.
- [ ] Remaining deferred optimizations have evidence-based dispositions.

### Phase 5 verification log

- Pending.

## Phase 6 — Colony tuning

### Objective

Tune configuration rather than adding exceptions, then demonstrate that the
new system sustains useful colony behavior over long fixed-seed runs.

### Tasks

- [ ] Define fixed seeds, durations, and colony sizes before tuning.
- [ ] Record pre-tuning metrics.
- [ ] Tune only documented config weights/thresholds first.
- [ ] Add code special cases only for structural correctness, with a decision
  log entry.
- [ ] Exercise:
  - [ ] food
  - [ ] bladder
  - [ ] social
  - [ ] sleep
  - [ ] work/mining
  - [ ] facility construction
  - [ ] storage/cleaning
  - [ ] alien fight/flee
  - [ ] post-threat resumption
- [ ] Measure short-window A-B-A focus switches.
- [ ] Inspect candidate explanations for surprising transitions.
- [ ] Add regression tests for every discovered systemic failure.
- [ ] Regenerate committed config after final tuning.
- [ ] Update behavior docs with final default behavior.

### Metrics

| Seed/scenario | Conversations | Work completed | Focus transitions | A-B-A switches | Starvation deaths | Mean needs | Notes |
| --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| pending | | | | | | | |

### Phase 6 exit criteria

- [ ] Default colonies continue excavating after needs activate.
- [ ] No mutual-social livelock.
- [ ] No repeated focus oscillation under ordinary conditions.
- [ ] Life support is built and used.
- [ ] Threats interrupt ordinary focus.
- [ ] Survivors resume useful focus after threats.
- [ ] Surprising behavior can be explained through score breakdowns.
- [ ] Final defaults are committed to code and `mars-sim.yaml`.

### Phase 6 verification log

- Pending.

## Phase 7 — Final integration review

### Objective

Verify architecture compliance, remove migration scaffolding, and leave a
reviewable implementation with complete evidence.

### Review tasks

- [ ] Search for obsolete scalar mood fields/config/helpers.
- [ ] Search for the old behaviorally active priority ladder.
- [ ] Search for direct life-event mood/memory updates outside the funnel.
- [ ] Search for random focus tie-breaking.
- [ ] Search for allocations in focus and fast-path code.
- [ ] Search for focus scoring that performs target claims, A*, or map scans.
- [ ] Confirm every focus has config defaults, generated keys, and validation.
- [ ] Confirm every need has a valid `CriticalAt`.
- [ ] Confirm snapshots copy mutable slices/arrays safely.
- [ ] Confirm frontend viewport does not affect simulation scope.
- [ ] Confirm docs match final behavior.
- [ ] Review every decision/deviation entry and close or escalate it.
- [ ] Review every open issue and assign a disposition.

### Final commands

```sh
gofmt -w <changed-go-files>
go build ./...
go test ./...
go test ./internal/sim -run '^$' -bench . -benchmem
git diff --check
```

Add project-specific race, vet, or long-run commands if they are already part of
the repository workflow; record them rather than assuming.

### Final evidence

| Check | Result | Notes |
| --- | --- | --- |
| `gofmt` | pending | |
| `go build ./...` | pending | |
| `go test ./...` | pending | |
| benchmarks | pending | link Phase 5/6 results |
| `git diff --check` | pending | |
| design compliance review | pending | |
| docs review | pending | |

### Phase 7 exit criteria

- [ ] All prior phases are `done` or intentionally omitted with approved log
  entries.
- [ ] Build, tests, formatting, and diff checks pass.
- [ ] Performance evidence is recorded.
- [ ] No duplicate source of truth remains.
- [ ] Design deviations are explicit.
- [ ] The plan accurately describes the delivered state.

### Phase 7 verification log

- Pending.

## Decision and deviation log

Use this table for implementation choices not already settled by the fixed
design. A deviation is not approved merely because it is recorded. Mark it
`proposed`, `approved`, `rejected`, or `superseded`.

| ID | Phase | Status | Decision or deviation | Reason/evidence | Approved by | Resulting work |
| --- | --- | --- | --- | --- | --- | --- |
| D-001 | 1 | approved | Do not read facility flow fields while scoring Phase 1 need candidates. | A field can refresh and allocate; exact facility selection already belongs to the winning executor, and the fixed design makes distance optional. | Fixed design, Distance section | Keep the `Distance` component and configured weights; add measured cached facility facts only if later profiling justifies them. |

## Open issues

Track blockers, discovered defects, and follow-up work. Close an issue by adding
its resolution; do not delete history.

| ID | Phase found | Severity | Status | Issue | Owner | Resolution/follow-up |
| --- | --- | --- | --- | --- | --- | --- |
| I-001 | — | — | — | _none recorded_ | — | — |

Delete the placeholder row when recording the first real issue.

## Change log

Record material updates to this plan, not every checkbox.

| Phase/date | Author | Change |
| --- | --- | --- |
| initial | Delta agent | Created phased implementation, verification, and performance plan. |
| Phase 1 / 2026-09-17 | Delta agent | Completed weighted focus arbitration, configuration, tests, documentation, and baseline comparison. |
| Phase 2 / 2026-09-17 | Delta agent | Added independent need phases, normalized pressure, boundary scheduling, configuration, tests, and documentation. |
| Phase 3 / 2026-09-18 | Delta agent | Added bounded active stimuli, life-event integration, ongoing threat refresh, deterministic expiry/eviction, focus scoring, tests, documentation, and profiling evidence. |

## Related

- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — fixed
  architecture and behavioral specification.
- [entities-and-ai.md](./entities-and-ai.md) — current entity turns and jobs.
- [needs.md](./needs.md) — current need mechanics.
- [personality.md](./personality.md) — traits and resolved parameters.
- [memories.md](./memories.md) — life-event ingestion and memory invariants.
- [configuration.md](./configuration.md) — configuration rules.
- [config-file.md](./config-file.md) — generated committed configuration.

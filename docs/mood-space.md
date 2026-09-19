# Mood space (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A **design proposal. Only the parts it explicitly says are shipped exist in
Go.** This doc originally targeted the scalar `Entity.mood` this repo used to
have. That target moved: the WTS-based action system (`docs/affect.md`,
`docs/cascading_wsts_architecture.md`) landed a two-axis `(charge, grip)`
`AffectState` first, drawing on an earlier draft of this same proposal — which
is why its attractor table, decay-toward-home shape, and label hysteresis
already match what's below almost exactly.

This revision is written against that merged code, not against the old
scalar. It does three things:

1. **Keeps** what already shipped and matches: the attractor table, label
   hysteresis, charge/grip as the base two axes, and the whole focus-arbitration
   and stimulus layer (out of scope for this doc — see
   [`cascading_wsts_architecture.md`](./cascading_wsts_architecture.md)).
2. **Supersedes** the parts that shipped from the *earlier* draft and that this
   doc's own [dead ends](#why-it-is-this-way) already argue against — chiefly
   `contextualValence`'s live, needs/HP-derived valence, plain vector addition
   in `addAffect`, and the fixed per-trait `switch` in `transformMoodVector`.
3. **Adds** what the shipped system doesn't attempt at all: a stored valence
   axis, habituation, and (optionally, see Phase 4) a cheaper cost model.

None of this is a criticism of #28 — it shipped the two-axis core and a working
consumer (focus scoring) faster than this doc reached its own Phase 2. This
revision exists to land the parts of the original pitch that the version it
built from didn't have yet.

### Why now

Charge and grip now have a real consumer: `FocusSpec.ChargeWeight` /
`GripWeight` drive focus arbitration. That's good — it means the representation
matters now — but it also means every future change to how charge/grip are
computed must not disturb that consumer. Valence has no consumer yet (it only
selects a display word), which is the same "producers, no consumers" window
the original doc used to justify moving fast on the scalar. That window is
still open for valence specifically.

## Source

- [`../internal/sim/affect.go`](../internal/sim/affect.go) — `AffectState`,
  `lifeEventMoodVectors`, `transformMoodVector`, `addAffect`, `decayAffect`,
  `contextualValence`, `refreshMoodAttractor`: the shipped two-axis system this
  revision extends and partly supersedes.
- [`../internal/sim/focus.go`](../internal/sim/focus.go) — `FocusSpec`,
  `affectContribution`: the consumer. Valence must not gain a weight here; see
  [Phase 2](#phase-2--impact-the-pushpull-blend).
- [`../internal/sim/stimulus.go`](../internal/sim/stimulus.go) — the active-
  stimulus buffer; out of scope, but shares the life-event funnel this proposal
  must keep wired.
- [`mood-space.html`](./mood-space.html) — the model sandbox, rebuilt against
  the shipped attractor/event tables (Phase 0, done): valence, impact-weighted
  push/pull, wear, and tag-based trait rules, all tunable and exported as Go.
- [`../internal/sim/world.go`](../internal/sim/world.go) — `remember`, the
  single funnel every change must keep, and `maxColonistMemories`.
- [`../internal/sim/entity.go`](../internal/sim/entity.go) — `affect`,
  `Memory`, `Memories`.

## How it works

### Three axes, three timescales

| Axis | Timescale | Driven by | Would drive |
| --- | --- | --- | --- |
| **charge** | seconds–minutes | events, needs | work speed, reaction, whether they start things |
| **grip** | minutes–hours | events | confront vs. avoid: fight or flee, commit to a job or dither |
| **valence** | hours–days | events, needs, deaths | which name a region takes; long-run outlook |

Charge and grip are `AffectState.Charge`/`.Grip` today. Valence is not: it's
recomputed every call to `refreshMoodContext`/`contextualValence` from live
need pressure, HP, and whether a threat is currently visible — nothing about
history. That's the dead end this doc already names below, now with a concrete
address: a colonist who is well-fed, unhurt, and standing somewhere with no
alien in sight reads as good *no matter what they watched happen five ticks
ago*. `EvtWitnessedColonistKilled` crashes their grip hard enough to move them
near the `adrift` attractor, and `contextualValence` will still hand `adrift`
its good name over its bad one. This is currently cosmetic — focus scoring
reads `Charge`/`Grip` directly, never `Label` or the good/bad word — but it's
the exact failure this doc predicted, now sitting in `contextualValence`
instead of a hypothetical.

Valence is a real axis, not a display tint, and it is **per colonist** — two
people watching the same death must be able to diverge. Deriving it from needs
and HP instead does not hold up; see the dead ends below.

It is still never drawn as a cube. The frontend shows the charge/grip plane
colored by valence.

### Naming: attractors, not sectors

Named regions are a table of points with a radius. Nearest attractor by
`distance / radius` wins; anything no attractor claims within `1.0` has **no
name** and reads as plain. Most of a colonist's life should be unremarkable.

Each attractor carries a name pair, selected by the sign of valence: the same
`(charge, grip)` is `driven` or `furious` depending on how the colonist feels
about their world. That is what lets two plotted axes carry a vocabulary that
usually needs three.

**Already shipped as-is.** `affect.go`'s `moodAttractors` is this exact table —
same nine points, same radii, same declaration-order tie-breaking, same
switch-margin hysteresis. Nothing below needs to touch it; the only shipped
gap is that the good/bad choice per attractor reads `contextualValence`
instead of a stored axis (previous section).

The starting table, on a ±100 plane (tune in Phase 0):

| charge | grip | radius | valence ≥ 0 | valence < 0 |
| ---: | ---: | ---: | --- | --- |
| 70 | 60 | 46 | driven | furious |
| 88 | 0 | 42 | elated | frantic |
| 65 | −62 | 46 | giddy | panicked |
| 0 | −85 | 42 | adrift | anxious |
| −62 | −62 | 46 | listless | despairing |
| −85 | 0 | 42 | spent | numb |
| −60 | 60 | 46 | content | grim |
| 0 | 85 | 42 | composed | hardened |
| 0 | 0 | 28 | steady | flat |

### Impact: one scalar, four jobs

Every `LifeEventKind` carries an **impact**, and impact alone decides:

1. how hard the event moves emotional position (below),
2. whether it survives memory consolidation (below),
3. how much habituation it accrues,
4. what a colonist talks about — people tell you about a gunfight, not a mined
   rock.

Impact is **not** a constant per kind: it is a function of the colonist. Seeing a
mouse is near-zero impact unless you are a musophobe. That dependency is what the
tag system exists to express.

### The update: push and pull are one operation

This replaces `addAffect`'s plain `clampInt(current + v, ...)`.

Mundane events nudge. Catastrophic events *relocate* — your friend died, you are
not "still basically content, minus a bit". Rather than two operators with a flag,
derive the blend from impact:

```
k     = clamp((impact - pushThreshold) / pullRange, 0, 1)
state = state + v*(1-k) + (target - state)*k
```

`k = 0` is a pure push by `v`. `k = 1` is full relocation to `target`. Every row
in the table has the same shape; only the numbers differ.

This matters beyond flavor: under pure addition, ten good meals cancel one murder.
Under relocation they cannot, because the murder *moved* you rather than adding to
you.

### Wear: the first death and the tenth

Every event names **two** outcomes rather than one: where it puts a colonist the
first time, and where it puts them once they are used to it. A per-kind
habituation term `wear`, running 0 to 1 and rising with each occurrence, picks
between them.

    target = lerp(fresh, worn, wear)

For a traumatic event the pair reads as resolve and collapse:

| `EvtWitnessedColonistKilled` | charge | grip | valence | lands in |
| --- | ---: | ---: | ---: | --- |
| `fresh` | 70 | 40 | −60 | furious |
| `worn` | 60 | −70 | −80 | panicked |

The first death is a rallying cry, the tenth is catatonia, and the arc between
them is one number.

For a mundane event the same pair reads as satisfaction and drudgery — no
special case, no second mechanism:

| `EvtFinishedMining` | charge | grip | valence | lands in |
| --- | ---: | ---: | ---: | --- |
| `fresh` | −1 | 5 | +10 | steady |
| `worn` | −6 | −4 | −25 | flat |

Note what monotony does and does not do. A worn shift does not crater anyone by
itself; it lands them in `flat` rather than `despairing`. What it takes away is
the small lift that used to offset the bad days, so the colony's baseline sags
rather than breaking. That is the honest version of "monotony becomes
depressing".

Traits bend `wear` instead of transforming the vectors: **Resilient** accrues it
slowly and stays near `fresh` for far longer; **Cowardly** starts partway along
and reaches `worn` in a couple of deaths. This is why a trait cannot simply
scale an event's vector — scaling changes how hard something hits, never whether
it hardens you or breaks you. This argument, and [Phase 6](#phase-6--tags-and-trait-rules)'s
tag table, are what replace `transformMoodVector`'s fixed `switch` over
trait × event kind.

### Position is folded, not stored — now optional, pending profiling

Emotional position is a **pure fold over the memory log** from an anchor, the way
`needLevel` is computed from a base and elapsed ticks rather than updated per
tick. Pulls are order-dependent, but a fold handles order; `Memory.Tick` supplies
the gaps that decay needs.

This was written to solve one problem: nothing should recompute mood on a
tick, so cost scales with events rather than with colonists × ticks. #28 solved
that same problem a different way first — `markMindDirty`/`nextCognitionTick`
skip full re-arbitration (and, for a stably resting or sleeping colonist,
`decayAffect` itself) whenever nothing has changed. The two approaches are
redundant, not complementary; running both buys nothing. Keep `decayAffect`'s
current cartesian per-tick decay toward `(0, 0)` and treat folding-from-anchor
as [Phase 4](#phase-4--the-anchor-and-folded-position-deferred), revisited only
if a profile shows the dirty-flag cache isn't enough — not built alongside it
on spec. Cartesian decay toward a fixed home was itself the shipped system's
placeholder for polar decay toward a personality home point; reopening that is
a smaller, separable change from folding and isn't blocked on this doc.

### Sleep closes the books

This section only matters once Phase 4's anchor exists; see the deferral note
above. Compaction is lossy, so position cannot be re-derived across it. Therefore:

> **Sleep is the commit point.** Consolidation collapses everything before it into
> (surviving high-impact memories + an **anchor**: position and tick at wake).
> Position folds forward from the anchor.

The anchor *is* the memoization. This also splits short- and long-term cleanly:

- **position** folds from the anchor over post-anchor events — short-term.
- **wear** counts over the whole retained log, which is exactly what survives
  consolidation — long-term.

So death #10 is still death #10 several sleeps later, while the 100th meal is
gone.

**Why sleep works here.** There is no day/night cycle; sleep is need-driven
(`Rise: 1`, `SeekAt: 700`), so colonists sleep on drifting schedules and
consolidation is naturally staggered. No stop-the-world sweep, no coordination.

**Budget.** A bed is `UseTicks: 40` and `maxColonistMemories` is 64, so
consolidation is under two entries per tick if it is *spread across the sleep*
rather than run as one burst on wake. Use the sleep duration the needs system
already tracks.

**The problem is real, and sized.** At current need rates a colonist generates
roughly 15 notable events per waking period, so 64 slots is about four days. A
death last week is *already* flushed by this week's meals.

Two cases consolidation must handle explicitly:

- **Colonists who never sleep.** Sleep is non-fatal — a colonist with no bunk
  waits indefinitely and never consolidates. Cap unconsolidated memories and, at
  the cap, drop by impact anyway. Degrading under sleep deprivation should be a
  decision, not an accident.
- **Everyone is synchronized on day one.** All colonists spawn with sleep at 0
  and the same base rise, so early consolidations land together before bed
  contention drifts them apart. Jitter initial sleep need at spawn — on
  `World.rng`, **not** the personality stream, because it changes gameplay.

### Baselines

Decay returns position to a per-colonist **baseline**, not the origin: some people
are natively anxious, some relaxed. The baseline is resolved at spawn from traits,
matching `docs/personality.md`'s pay-once principle. `Optimist`/`Pessimist` is the
obvious missing trait pair and belongs in `groupTemperament` next to Tidy, whose
comment already anticipates an opposite.

### Tags: making trait × event tractable

Today, adding a trait means considering it against every event, and vice versa —
`t × e` decisions. Introduce an intermediate vocabulary:

**Events carry tags. Traits react to tags.** `t × e` becomes `t + e`.

```go
type TraitRule struct {
    Trait                     Trait
    AnyTags, AllTags, NotTags []EventTag
    Impact, Charge, Grip, Valence, WearRate float64
}
```

`EvtWitnessedColonistKilled` is tagged `death, violence, social-loss, gore`.
`TraitTidy` reacts to `gore`. A new trait is one rule against existing tags; a new
event is a tagging decision. Nobody ever has to answer "does ItemPurchased impact
Mutant?"

**Dynamic tags are what make this expressive enough.** Some reactions depend on
context a static tag cannot carry — watching a *friend* die differs from watching
someone you hated die. `remember()` already knows the relationship, so it stamps
`friend-died` on that instance at emit time. Relationship-, health- and
location-dependent reactions all fall out, and the rule table stays a plain struct.

**Not a string DSL.** A parser, an evaluator and error reporting is a lot of
machinery for a table that fits on one screen, and dynamic tags absorb most of
what the arithmetic would be for. Graduate to an expression language only when a
rule genuinely needs arithmetic over colonist state that a tag cannot precompute.

## Why it is this way

- **Dead end: valence derived from needs and HP.** Two axes are enough if
  valence is computed rather than stored, since the sim already knows whether
  things are going well. It does not survive a counterexample: witnessing a
  death must make things *worse*, and if only needs and HP feed valence, a
  well-fed colonist who just watched a friend die reads as `elated`. If naming
  takes valence as an argument, valence is part of the model. **This is not
  hypothetical** — `affect.go`'s `contextualValence` is exactly this dead end,
  inherited from the draft of this doc that predated the "dead end" write-up.
  It's currently harmless because nothing behavioral reads the label, but it's
  the reason this revision exists.
- **Dead end: one global "how things are going" control.** A single colony-wide
  valence makes divergence between colonists impossible — the exact thing the
  system exists to produce.
- **Dead end: two axes with "hot vs. cold" as the second.** Follow it through and
  good/bad disappears: low-charge/cold covers both *serene* and *depressed*. The
  temperature intuition was tracking the world's valence, not a second dimension
  of the colonist.
- **Dead end: agency alone as the second axis.** Appraisal theory's agency
  dimension separates anger from fear but collapses anger and joy — both are high
  charge, high agency.
- **Dead end: traits as linear transforms.** Scaling or reflecting an event vector
  cannot express "the first death hardens you, the tenth breaks you". Transforms
  survive only for low-impact pushes, where Tidy scaling gore really is a scale.
- **Why attractors and not wedges?** Wedges force every point to have a name, so a
  colonist who feels basically fine gets labelled anyway. A radius lets unclaimed
  space exist, and adding a mood stays a table edit.
- **Why hysteresis on the name?** Without it a colonist near a boundary strobes
  between two words in the roster. A challenger must beat the incumbent by a
  margin before the label flips.

## Build plan

Originally seven phases against a scalar `mood int`. Phases 1 (three-axis
representation) and most of 2 and 7 (a first consumer, then the rest of the
consumers) shipped in #28 against charge/grip specifically, ahead of this
revision. What's below is the plan against what's actually in the tree now:
smaller in places (Phase 1 is one field, not a type migration), unchanged in
others (wear, tags), and explicitly deferred in one (folding/consolidation).
Each remaining phase is independently shippable and leaves the tree green.

**Standing constraints for every phase:**

- `go build ./...` and `go test ./...` pass (`AGENTS.md`).
- The simulation stays deterministic for a given seed. Consolidation in
  particular must use stable ordering — no map iteration, no wall-clock.
- New tunables land in `sim.Config` with matching flags in `main.go`.
- The **one-funnel** invariant from `memories.md` holds: mood and memory move
  together through `remember`, so a mood-bearing event can never forget to wire
  its mood.
- Docs updated in the same change.

**Standing note: there is no persistence.** No save, no load, no serialization
of world state — every run starts fresh, and `Snapshot` is a frontend view, not
a save format. This simplifies the whole plan and is worth stating once: no
phase needs a migration path, the event and attractor tables can be re-tuned
freely between runs, and the blast radius of any bug is a single session. Where
this doc calls something risky, it means hard to test or hard to notice, never
hard to recover.

### Phase 0 — Rebuild the sandbox (no Go) — done

`mood-space.html` now models valence on the existing charge/grip plane,
impact-driven push/pull, wear with fresh/worn targets, and tag-based trait
rules, seeded from the shipped `lifeEventMoodVectors`/`moodAttractors` rather
than re-derived. Its `EvtWitnessedColonistKilled`/`EvtFinishedMining` fresh/worn
numbers are this doc's own worked examples; the other 23 events are first
guesses left for tuning. Wear is a raw occurrence counter rather than folded
from a memory log, and consolidation isn't modeled — both intentional per the
Phase 4/5 deferral above. "Generate Go" emits `MoodVector` (widened to three
fields), `lifeEventProfiles`, `EventTag` constants, and `TraitRule` rows,
flagging which trait names don't exist in `personality.go` yet.

- **Proved:** the numbers hold together — a fresh alien sighting and a worn one
  read differently, `EvtWitnessedColonistKilled` relocates near `furious` while
  a mundane event barely nudges, and the dynamic-tag mechanism (a "Loyal" rule
  reacting to a `friend-died` tag stamped only on one firing) works without a
  relationship graph.
- **Not resolved here:** the actual tuned values — they're first guesses, not
  playtested ones. That's Phase 5's job once Phases 1-3 exist to play against.
- **Risk retired:** the largest one — that the whole model feels arbitrary in
  play. Cheap to discover here, ruinous to discover in Phase 3.

### Phase 1 — Stored valence

Add `Valence int` to `AffectState`, updated by the event table like charge and
grip (plain addition is fine until Phase 2 lands the blend), decaying toward 0
like the other axes. Replace `contextualValence` with a read of the stored
field. `moodName`'s good/bad selection changes its input, not its shape.

Valence stays **display-only** — same invariant the shipped doc states for the
derived version: no `FocusSpec` weight reads it, on pain of double-counting
needs/HP/threat that charge and grip (and, directly, need pressure) already
express. If a genuinely distinct "outlook" effect on behavior is ever wanted,
that's a new, explicit non-goal to relitigate later, not a side effect of
making valence a real axis.

- **Ships:** per-colonist valence that reflects history, not a live snapshot.
- **Not in this phase:** pull/relocation, wear, consolidation, tags.
- **Verify:** the counterexample from the dead-ends section — a well-fed,
  unhurt colonist with no visible threat who just witnessed a colonist killed
  reads with the *bad* name, not the good one.

### Phase 2 — Impact and the push/pull blend

Add `Impact` to the event table, derive `k`, implement the blended update in
`addAffect` for all three axes. This replaces plain addition, not the
consumer — `FocusSpec.ChargeWeight`/`GripWeight` already read charge/grip
directly and don't change.

- **Proves:** that catastrophes relocate rather than accumulate, and that
  ten good meals no longer cancel one murder.
- **Verify:** existing affect tests that assert exact post-event integers move
  to assertions on direction/attractor membership instead, per the risk below.

### Phase 3 — Wear

Per-kind habituation, two-target interpolation, trait wear modifiers. Replaces
`transformMoodVector`'s fixed trait × event `switch`.

Start by counting wear from the existing memory log — it is free, it already
carries `Kind`, and it decays naturally as entries roll off. Measure whether the
window feels too short before paying for dedicated per-colonist state.

- **Verify:** the first witnessed death and the tenth produce visibly different
  outcomes for the same colonist.
- **Known interaction:** this window is wrong until Phase 5, because mundane
  events flush it. That is acceptable if Phase 5 is undertaken — it improves the
  window rather than reworking it. If Phase 5 is skipped, say so in the trait
  rule doc rather than leaving the gap implicit.

### Phase 4 — The anchor and folded position (deferred)

Refactor position from stored to folded-from-anchor. No behavior change if
done — but see the note above: `markMindDirty`/`nextCognitionTick` already
solve the per-tick cost problem this phase targets, by skipping decay and
re-arbitration for colonists nothing is happening to. Undertake this only if a
profile shows `decayAffect` costing real time despite that cache; otherwise
the two mechanisms compete to solve the same problem for no benefit.

- **Verify, if undertaken:** the strongest test in the plan — position computed
  the old way and the new way must match across a seeded run.
- **Why separate from Phase 5:** consolidation *requires* the anchor, and a
  refactor with a provable no-op is worth isolating from a change that alters
  behavior.

### Phase 5 — Sleep consolidation (deferred with Phase 4)

Retention by impact, spread across the bed's `UseTicks`, anchor committed at wake.
Plus the two edge cases: the unconsolidated cap for colonists who never sleep, and
spawn-time jitter on `World.rng`.

- **Verify:** after several days a colonist's memory log holds the notable events
  of the past week rather than the last four days of meals.
- **Risk:** still the phase to take most carefully, but not because anything
  durable is at stake — there is no persistence, so a bad run is one restart
  away from gone (see the standing note above). The reasons are:
  - **No test oracle.** Phase 4 can assert `old == new` across a seeded run.
    Consolidation has nothing to compare against, because the whole point is
    that the discarded entries are gone. Tests here assert properties ("a death
    outlives a week of meals", "retention is monotonic in impact") rather than
    equality, which is a weaker net.
  - **Errors are silent and delayed.** A wrong retention rule does not crash; it
    produces colonists who are subtly wrong an hour of play later. Position
    folds from the anchor, so a bug in the anchor quietly biases every mood that
    colonist has for the rest of the run, with no re-derivation path back.
  - **Determinism is easy to lose.** Consolidation is the first thing in the
    codebase doing amortized per-entity background work, and any unstable
    ordering makes seeded runs diverge — which breaks the `AGENTS.md` invariant
    and makes the delayed bugs above non-reproducible on top of it.
- **Mitigation:** a flag that disables consolidation, so any suspected mood bug
  can be bisected against a run that never consolidates.

### Phase 6 — Tags and trait rules

Replace `transformMoodVector`'s per-kind conditional `switch` with the tag
vocabulary and `TraitRule` table. Add dynamic tagging at emit time in
`remember`.

- **Why last:** you want to know the real shape of trait/event interactions
  before generalizing them. `transformMoodVector` today has five trait × event
  cases; that's still small enough to read at a glance. Do this when adding the
  next one or two starts feeling like guessing at a pattern, not on a schedule.
- **Verify:** adding a new trait touches one table and no event definitions.

### Phase 7 — The rest of the consumers

Flee/engage-on-grip and job-commitment-on-charge are **already shipped** —
`FocusSpec` gives every focus a `ChargeWeight`/`GripWeight`, which is this
phase's original goal. What's left:

- **Conversation topic selection by impact** — people talk about gunfights,
  not mined rocks. Needs `Impact` from Phase 2.
- **Later, optionally: secondhand memories from conversation — rumor.**
  Flagged as a real feedback-loop risk: without a retelling decay and an
  "already knows" check, a single death can amplify around the colony forever.
  The `seen` map is the existing precedent for the check.

## Open questions

These need a decision before the phase that depends on them; none block Phase 0.

1. **Does wear survive consolidation as a counter, or get recounted from retained
   memories?** Recommendation: recount. It is free and self-decaying. Moot
   unless Phase 5 is undertaken. (Phase 3/5)
2. **Are mundane events remembered at all, or only counted?** A "you ate 47 times"
   aggregate is cheaper than 47 entries and changes what consolidation does.
   Moot unless Phase 5 is undertaken. (Phase 5)
3. **Does `cfg.MoodMax` cover valence too, or does it get its own range?**
   Charge and grip already share one `cfg.MoodMax`; valence is new, so this is
   a real choice rather than an inherited default. (Phase 1)
4. **Resolved by #28:** the roster already shows both axes plus the resolved
   label (`moodLine` in `render_roster.go`: `affect C+8 G-10 furious`), not a
   compass. Phase 1 folds valence into the existing label word; it does not
   redesign this line.
5. **What pulls a colonist out of a doom spiral?** Low grip → worse choices →
   worse outcomes → lower grip is a real attractor in this design, and the
   consumer that would make it visible — focus scoring — already exists. This
   is live now, not deferred to a future phase.

## Risks

- **Doom spirals** — see open question 5. The most likely way this ships and
  feels bad, and the consumer that would expose it is already in the tree.
- **Tuning surface** — 24 event kinds × five numbers, plus targets in pairs, plus
  trait rules. Phase 0 exists to make this tractable; if Phase 0 feels
  unmanageable, the model is too big and should shrink before Phase 1.
- **Test brittleness** — tests asserting exact affect integers will break in
  Phase 2 when addition becomes a blend. Prefer assertions on named regions and
  on direction of change over exact values.
- **Individuality reading as noise** — colonists behaving differently is the
  goal; colonists appearing to refuse work at random is the failure mode. Every
  mood-driven decision should be legible in the roster before it is legible only
  in the colonist's behavior.

## Extending it

Once built, the intended shapes:

- **A new mood name** — a row in the attractor table.
- **A new life event** — a row in the event table, plus its tags.
- **A new trait** — one `TraitRule` against existing tags. No event definitions
  are touched.
- **A new consumer** — read `AffectState.Charge`/`.Grip` directly, the same way
  `FocusSpec` already does. Never derive valence live from needs/HP/threat
  again — that's the dead end this doc exists to close.

## Related

- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — the
  merged focus-arbitration system this proposal's charge/grip axes already feed,
  and the source of the attractor table and label-hysteresis shape this doc
  shares rather than re-derives.
- [affect.md](./affect.md) — the shipped `AffectState`, event-vector table, and
  trait transforms this proposal extends and partly supersedes.
- [memories.md](./memories.md) — life events and the one-funnel rule this
  proposal must keep.
- [personality.md](./personality.md) — traits, trait groups, and the
  resolve-at-spawn principle the baseline follows.
- [needs.md](./needs.md) — lazy need evaluation, the pattern folded position
  imitates if Phase 4 is undertaken, and the sleep spec consolidation
  piggybacks on if Phase 5 is undertaken.
- [configuration.md](./configuration.md) — where the new tunables and flags go.
- [frontend-tui.md](./frontend-tui.md) — the roster line the mood name lands in.

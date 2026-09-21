# Mood space (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

The unbuilt half of colonist affect. [`affect.md`](./affect.md) documents what
runs today: three stored axes, an impact that decides whether an event nudges a
colonist or relocates them, and wear that picks between an event's fresh and
worn readings. This doc covers what that leaves out.

Two things, in the order they are worth building:

1. **Tags.** Trait appraisal is a `switch` over trait × event kind, so every new
   trait has to be considered against every event and vice versa — and a trait
   can only change how hard an event lands, never how significant it is or how
   fast the colonist gets used to it.
2. **Baselines.** Affect decays to `(0, 0, 0)` for everyone. Nobody is natively
   anxious or natively hard to rattle.

Nothing here is implemented. It is written to be argued with before any of it
becomes Go.

## Source

- [`mood-space.html`](./mood-space.html) — the tuning sandbox. Its event table
  is seeded from the shipped one, so the tag mechanics below can be played with
  against real numbers. It emits the tuned tables as Go.
- [`../internal/sim/affect.go`](../internal/sim/affect.go) — `transformMoodVector`,
  the `switch` this proposal replaces, and `lifeEventAppraisals`, whose rows
  would gain tags.
- [`../internal/sim/personality.go`](../internal/sim/personality.go) — `Trait`,
  `traitGroup`, and the resolve-at-spawn principle baselines follow.
- [`../internal/sim/world.go`](../internal/sim/world.go) — `remember`, the one
  funnel a dynamic tag would be stamped in.

## How it works

### Tags: making trait × event tractable

Today `transformMoodVector` is a `switch` over trait × event kind, so adding a
trait means considering it against every event, and vice versa — `t × e`
decisions. Introduce an intermediate vocabulary:

**Events carry tags. Traits react to tags.** `t × e` becomes `t + e`.

```go
type TraitRule struct {
    Trait                     Trait
    AnyTags, AllTags, NotTags []EventTag
    Impact, Charge, Grip, Valence, WearRate float64
}
```

Every factor is a multiplier defaulting to 1 (0 or unset means no change,
matching the convention `traitSpec` already uses), so a reflection is just −1
and the four shipped transforms survive the translation unchanged. `WearRate`
is the factor that needs the shipped wear to mean anything: **Resilient**
accrues occasions slowly and stays near `fresh` for far longer, **Cowardly**
reaches `worn` in a couple of exposures.
`EvtWitnessedColonistKilled` is tagged `death, violence, social-loss, gore`;
`TraitTidy` reacts to `gore`. A new trait is one rule against existing tags; a
new event is a tagging decision. Nobody ever has to answer "does ItemPurchased
impact Mutant?"

The `Impact` and `WearRate` factors are what make this worth more than tidier
plumbing: a rule can change how *significant* an event is to one colonist, and
how fast they get used to it, not just how hard it lands. Seeing a mouse is
near-zero impact unless you are a musophobe, and that cannot be said at all
today — nor can "this one never gets used to it".

**Dynamic tags are what make this expressive enough.** Some reactions depend on
context a static tag cannot carry — watching a *friend* die differs from
watching someone you hated die. `remember` already knows the relationship, so it
stamps `friend-died` on that instance at emit time. Relationship-, health- and
location-dependent reactions all fall out, and the rule table stays a plain
struct.

**Not a string DSL.** A parser, an evaluator and error reporting is a lot of
machinery for a table that fits on one screen, and dynamic tags absorb most of
what the arithmetic would be for. Graduate to an expression language only when a
rule genuinely needs arithmetic over colonist state that a tag cannot
precompute.

### Baselines

Decay returns affect to a per-colonist **baseline**, not the origin: some people
are natively anxious, some relaxed. The baseline is resolved at spawn from
traits, matching [`personality.md`](./personality.md)'s pay-once principle.
`Optimist`/`Pessimist` is the obvious missing pair and belongs in
`groupTemperament`, which already holds Tidy and already anticipates gaining an
opposite.

This is the smallest of the three and the one most likely to be felt: it is what
makes two colonists in the same colony, doing the same work, read differently
for reasons the player can name.

## Why it is this way

- **Why multipliers rather than a second set of deltas?** A delta per trait per
  event is the `t × e` table this exists to avoid, and it cannot express "this
  matters more to them" — only "this hits them harder", which is a different
  claim. Multipliers keep the event's meaning in the event table where it
  belongs.
- **Why are valence, the push/pull blend and wear not in this doc any more?**
  They shipped. See [`affect.md`](./affect.md), which records the arguments for
  storing valence rather than deriving it, and for counting wear in remembered
  occasions rather than occurrences.

## Build plan

Two phases, roughly one PR each, in dependency order. Each must leave
`go build ./...` and `go test ./...` passing, keep seeded runs deterministic
(no map iteration, no wall-clock), put new tunables in `sim.Config` with a
regenerated `mars-sim.yaml`, keep the one-funnel invariant from
[`memories.md`](./memories.md), and update the docs it makes wrong.

### Phase 1 — Tags and trait rules

Replace `transformMoodVector`'s switch with the tag vocabulary and `TraitRule`
table. Add dynamic tagging at emit time in `remember`. Add `Resilient` and
`Cowardly`, which the shipped wear now gives something to bend.

- **Verify:** adding a new trait touches one table and no event definitions; the
  four shipped transforms produce identical results through the new path.

### Phase 2 — Baselines

Resolve a per-colonist home point at spawn, decay toward it, and add
`Optimist`/`Pessimist` to `groupTemperament`.

- **Verify:** two colonists with the same history and different temperaments
  settle to different resting affect, and a seeded run still reproduces.

### Deliberately not doing

**Folding affect from a memory anchor, and sleep consolidation.** An earlier
draft proposed computing affect as a fold over the memory log from an anchor
committed at sleep, so that nothing recomputed mood per tick. The cost problem
it solved is already solved: `markMindDirty`/`nextCognitionTick` skip decay and
arbitration for colonists nothing is happening to, and a valence-only change
deliberately does not dirty cognition. Building both would be two mechanisms
competing over one problem. Revisit only if a profile shows `decayAffect`
costing real time.

## Open questions

1. **Does wear needing recovery make "I got over it" and "I never saw it" look
   identical?** They do, now that occasions roll off with the memories holding
   them. Probably correct — a colonist who has genuinely moved on should meet
   the next one closer to fresh — but worth watching in play.
2. **Should a `WearRate` rule change how fast occasions accrue, or where the
   worn reading sits?** The first is what the struct says; the second would let
   Resilient end up somewhere different rather than just later. (Phase 1)
3. **What pulls a colonist out of a doom spiral?** Low grip → worse choices →
   worse outcomes → lower grip is a real attractor, and nothing opposes it. Now
   that focus scoring reads grip, this is live rather than hypothetical.
4. **Do baselines shift valence, or only charge and grip?** An optimist arguably
   recovers faster rather than starting higher. (Phase 2)

## Risks

- **Doom spirals** — see open question 3. The most likely way this ships and
  feels bad.
- **Saturation** — valence pegged at maximum the first time it ran, because
  routine work paid into an axis that drains slowly; wear then pegged for
  whatever a colonist does all day, which turned out to be the right answer but
  was not the expected one. Any new accumulating term needs a long-run check
  before its numbers are believed, not just a unit test.
- **Tuning surface** — trait rules add a third table to keep coherent with the
  fresh/worn pairs. The sandbox exists to make that tractable; if it feels
  unmanageable there, the model is too big.
- **Individuality reading as noise** — colonists behaving differently is the
  goal; colonists appearing to refuse work at random is the failure mode. Every
  mood-driven decision should be legible in the roster before it is legible only
  in the colonist's behavior.

## Extending it

Once built, the intended shapes:

- **A new life event** — a row in the appraisal table, plus its tags.
- **A new trait** — one `TraitRule` against existing tags. No event definitions
  are touched.
- **A new mood name** — a row in the attractor table (already true today).

## Related

- [affect.md](./affect.md) — what actually runs: the three axes, the appraisal
  table, and the push/pull blend this builds on.
- [memories.md](./memories.md) — the life-event funnel, and the memory log wear
  would count from.
- [personality.md](./personality.md) — traits, trait groups, and the
  resolve-at-spawn principle baselines follow.
- [configuration.md](./configuration.md) — where the new tunables and flags go.

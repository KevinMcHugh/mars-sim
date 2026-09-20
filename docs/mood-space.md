# Mood space (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

The last unbuilt piece of colonist affect. [`affect.md`](./affect.md) documents
what runs today: three stored axes, an impact that decides whether an event
nudges a colonist or relocates them, wear that picks between an event's fresh
and worn readings, and tag-based trait rules. This doc covers what is left.

**Baselines.** Affect decays to `(0, 0, 0)` for everyone. Nobody is natively
anxious, natively cheerful, or natively hard to rattle — two colonists with the
same history settle to exactly the same place.

Nothing here is implemented. It is written to be argued with before any of it
becomes Go.

## Source

- [`mood-space.html`](./mood-space.html) — the tuning sandbox, seeded from the
  shipped tables. Its home-point control is the thing this doc proposes making
  per-colonist.
- [`../internal/sim/affect.go`](../internal/sim/affect.go) — `decayAffect`,
  which currently settles every colonist toward the same origin.
- [`../internal/sim/personality.go`](../internal/sim/personality.go) — `Trait`,
  `traitGroup`, and the resolve-at-spawn principle baselines follow.

## How it works

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

- **Why resolve a baseline at spawn rather than derive it from traits each
  time?** [`personality.md`](./personality.md)'s pay-once principle: anything
  read every turn is resolved once at spawn, so the per-turn path stays a
  subtraction rather than a trait scan.
- **Why not just give the existing traits a resting offset?** Because the
  interesting version is a trait pair that exists *for* this — an optimist and
  a pessimist who genuinely settle in different places — rather than a side
  effect bolted onto Tidy or Lazy.
- **Why is everything else gone from this doc?** It shipped. See
  [`affect.md`](./affect.md), which records the arguments for storing valence
  rather than deriving it, for counting wear in remembered occasions rather
  than occurrences, and for tags over a trait × event switch.

## Build plan

One phase left. It must leave
`go build ./...` and `go test ./...` passing, keep seeded runs deterministic
(no map iteration, no wall-clock), put new tunables in `sim.Config` with a
regenerated `mars-sim.yaml`, keep the one-funnel invariant from
[`memories.md`](./memories.md), and update the docs it makes wrong.

### Baselines

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
   worn reading sits?** It changes the rate, as shipped; the alternative would
   let Resilient end up somewhere different rather than just later.
3. **What pulls a colonist out of a doom spiral?** Low grip → worse choices →
   worse outcomes → lower grip is a real attractor, and nothing opposes it. Now
   that focus scoring reads grip, this is live rather than hypothetical.
4. **Do baselines shift valence, or only charge and grip?** An optimist arguably
   recovers faster rather than starting higher.

## Risks

- **Doom spirals** — see open question 3. The most likely way this ships and
  feels bad.
- **Saturation** — valence pegged at maximum the first time it ran, because
  routine work paid into an axis that drains slowly; wear then pegged for
  whatever a colonist does all day, which turned out to be the right answer but
  was not the expected one. Any new accumulating term needs a long-run check
  before its numbers are believed, not just a unit test.
- **Individuality reading as noise** — colonists behaving differently is the
  goal; colonists appearing to refuse work at random is the failure mode. Every
  mood-driven decision should be legible in the roster before it is legible only
  in the colonist's behavior.

## Extending it

Once built, the intended shapes:

- **A new temperament** — a trait in `groupTemperament` and a home point, with
  nothing else touched.

The other extension points already work this way today: a new life event is a
row in the appraisal table plus its tags, a new trait is one rule against
existing tags, and a new mood name is a row in the attractor table.

## Related

- [affect.md](./affect.md) — what actually runs: the three axes, the appraisal
  table, and the push/pull blend this builds on.
- [memories.md](./memories.md) — the life-event funnel, and the memory log wear
  would count from.
- [personality.md](./personality.md) — traits, trait groups, and the
  resolve-at-spawn principle baselines follow.
- [configuration.md](./configuration.md) — where the new tunables and flags go.

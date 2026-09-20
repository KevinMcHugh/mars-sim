# Mood space (design record)

> Part of the [mars-sim documentation](./README.md).

## What it is

The reasoning behind colonist affect, and the tuning sandbox that produced its
numbers. Everything this document once proposed now runs:
[`affect.md`](./affect.md) is the doc for how it behaves. This one keeps the
arguments — the shapes that were tried and rejected, and the questions still
open — because those are the parts the code cannot say for itself.

What shipped, in the order it was built: a stored valence axis; an impact that
decides whether an event nudges a colonist or relocates them; wear, so the
first death and the tenth land differently; tag-based trait rules; and
per-colonist baselines.

## Source

- [`mood-space.html`](./mood-space.html) — the tuning sandbox, seeded from the
  shipped tables. Tuning a number there is tuning the real thing, and it emits
  the tables as Go.
- [`../internal/sim/affect.go`](../internal/sim/affect.go) — all of it.

## Why it is this way

### Rejected: valence derived from needs and HP

Two axes are enough if valence is computed rather than stored, since the sim
already knows whether things are going well. This is how it worked first, and
it does not survive a counterexample: witnessing a death must make things
*worse*, and if only needs and HP feed valence, a well-fed colonist who just
watched a friend die reads as `elated`. If naming takes valence as an argument,
valence is part of the model. Two colonists at identical coordinates now read
differently because they have had different lives, which is the entire point.

### Rejected: one global "how things are going" control

A single colony-wide valence makes divergence between colonists impossible —
the exact thing the system exists to produce.

### Rejected: "hot versus cold" as the second axis

Follow it through and good/bad disappears: low-charge/cold covers both *serene*
and *depressed*. The temperature intuition was tracking the world's valence,
not a second dimension of the colonist.

### Rejected: agency alone as the second axis

Appraisal theory's agency dimension separates anger from fear but collapses
anger and joy — both are high charge, high agency.

### Rejected: traits as linear transforms, only

Scaling or reflecting an event vector cannot express "the first death hardens
you, the tenth breaks you" — it changes how hard something hits, never which
direction the reaction turns. That gap is why wear exists. Transforms survive
for the low-impact cases, where Tidy scaling gore really is a scale, which is
why the tag rules kept them.

### Rejected: a dedicated wear counter per kind

A counter never forgets, so a colonist who saw three deaths a month ago would
meet the fourth as a veteran. Counting occasions in the bounded memory log
gives recovery for free, and recovery is most of what makes wear feel like a
person rather than a ratchet.

### Rejected: attractor wedges, and naming without hysteresis

Wedges force every point to have a name, so a colonist who feels basically fine
gets labelled anyway. A radius lets unclaimed space exist. Without hysteresis a
colonist near a boundary strobes between two words in the roster.

### Dropped: folding affect from a memory anchor, and sleep consolidation

An earlier draft proposed computing affect as a fold over the memory log from
an anchor committed at sleep, so nothing recomputed mood per tick. The cost
problem it solved is already solved: `markMindDirty`/`nextCognitionTick` skip
decay and arbitration for colonists nothing is happening to, and a
valence-only change deliberately does not dirty cognition. Building both would
be two mechanisms competing over one problem. Revisit only if a profile shows
`decayAffect` costing real time.

## What we learned tuning it

- **Saturation is the recurring failure.** Valence pegged at maximum the first
  time it ran, because routine work paid into an axis that drains slowly. Wear
  then pegged for whatever a colonist does all day, which turned out to be the
  right answer but was not the expected one. Every accumulating term needs a
  long run before its numbers are believed; a unit test will not show it.
- **A control nothing reads is worse than no control.** Both the sandbox's
  dynamic-tag checkbox and the shipped `TagFriend` were briefly stamped with no
  rule reacting to them, which looks identical to a bug.
- **Settled is not the same as neutral.** Baselines nearly defeated cognition
  caching, because the check asked whether affect was at the origin rather than
  whether it had stopped moving.

## Open questions

1. **What pulls a colonist out of a doom spiral?** Low grip → worse choices →
   worse outcomes → lower grip is a real attractor, and nothing opposes it. Now
   that focus scoring reads grip and baselines nudge it, this is live rather
   than hypothetical. The baseline grip offsets are small for this reason.
2. **Does wear recovering make "I got over it" and "I never saw it" look
   identical?** They do, now that occasions roll off with the memories holding
   them. Probably correct — a colonist who has genuinely moved on should meet
   the next one closer to fresh — but worth watching in play.
3. **Should a `WearRate` rule change how fast occasions accrue, or where the
   worn reading sits?** It changes the rate, as shipped; the alternative would
   let Resilient end up somewhere different rather than just later.
4. **Should baselines shift charge too?** Deliberately not, today: charge
   describes the energy behind the next action, so a standing offset
   misdescribes it. An argument exists for the other side.
5. **Individuality reading as noise** — colonists behaving differently is the
   goal; colonists appearing to refuse work at random is the failure mode.
   Every mood-driven decision should be legible in the roster before it is
   legible only in the colonist's behavior.

## Extending it

- **A new life event** — a row in the appraisal table, plus its tags.
- **A new trait reaction** — one rule in `traitRules` against tags that already
  exist. No event definition changes.
- **A new temperament** — a trait with an `affectHome`, and nothing else.
- **A new mood name** — a row in the attractor table.

## Related

- [affect.md](./affect.md) — how all of this behaves.
- [memories.md](./memories.md) — the life-event funnel, and the memory log wear
  counts from.
- [personality.md](./personality.md) — traits, groups, and the resolve-at-spawn
  principle baselines follow.
- [configuration.md](./configuration.md) — where the tunables live.

# Mood space (what's left)

> Part of the [mars-sim documentation](./README.md).

## What it is

The leftover half of colonist affect after wear, grammar-matched trait rules,
and per-colonist baselines shipped. [`affect.md`](./affect.md) documents what
runs today. This doc keeps the arguments that are still open and the dead ends
we already rejected, so the next pass does not rebuild the tag design.

Two things from the original proposal shipped, and one did not:

1. **Trait rules shipped as grammar matches, not tags.** Appraisal is no
   longer a `switch` over trait × event kind. Rules live in `cognition.yaml`
   and match actor/action/object/channel/role/phase/`object_relation`.
2. **Baselines shipped.** Affect decays toward `affectHome`. Optimist and
   Pessimist sit in `groupOutlook`.
3. **Tags did not ship.** They would have been a second vocabulary next to
   the perception grammar, and `friend` is an observer-relative relation, not
   a stamp on the occurrence.

Nothing in the "still open" section below is implemented. It is written to be
argued with before any of it becomes Go.

## Source

- [`mood-space.html`](./mood-space.html) — the older tuning sandbox. Prefer
  [`../tools/cognition_lab.html`](../tools/cognition_lab.html) for current
  reactions, wear, trait rules, and affect-home.
- [`../internal/sim/affect.go`](../internal/sim/affect.go) — wear policies,
  trait-rule application, and decay toward home.
- [`../cognition.yaml`](../cognition.yaml) — the sole event/trait-appraisal
  data surface.

## What shipped, briefly

Wear still counts remembered occasions, not `Memory.Count`, and recovers as
those memories leave the bounded log. Conversation uses wear policy `none`
instead of a hard-coded exemption. Resilient and Cowardly scale `wear_rate`.
See [affect.md](./affect.md).

Trait rules compose in file order. Extrovert's friend-loss bonus matches
`object_relation: friend`, computed from `MoodFriendAffinity`. See
[compositional-perception-and-events.md](./compositional-perception-and-events.md).

Optimist home is `{grip: 8, valence: 25}`; Pessimist is
`{grip: -8, valence: -25}`. `affectSettled()` keeps the cognition cache honest
when home is off the origin.

## Still open

### Relationship-aware conversation wear

The wear-policy registry exists so conversation can grow its own rule without
changing `rememberPercept`. The intended first specialist is something like
`relationship-conversation`: talking to a close friend stays fresh; talking
to a barely-liked neighbor goes rote. That immediately suggests temporary
frustration — you can be sick of a close friend for a while — which is why
this merger shipped the seam and not the policy.

### Doom spirals

Low grip → worse choices → worse outcomes → lower grip is a real attractor,
and nothing opposes it. Focus scoring reads grip, so this is live rather than
hypothetical. Baselines help a little (a Pessimist already lives nearer the
bad side; an Optimist has further to fall) but they do not create a recovery
force.

### Recovery versus never-saw-it

Wear needing recovery makes "I got over it" and "I never saw it" look
identical once the memory has rolled off. Probably correct — a colonist who
has genuinely moved on should meet the next one closer to fresh — but worth
watching in play.

### WearRate versus a different worn reading

A `wear_rate` rule changes how fast occasions accrue. It does not change
where the worn reading sits. Resilient stays near fresh longer; they do not
end somewhere else. If that feels wrong in play, the worn vector is the
knob, not a second wear formula.

## Why tags were dropped

The original proposal made events carry tags and traits react to tags, so
`t × e` became `t + e`. That is still the right *shape*. The wrong *carrier*
was a parallel vocabulary (`gore`, `finished-work`, `conversation`) that had
to stay aligned with actor/action/object the perception layer already names.

Dynamic tags were the other half of that proposal: `remember` would stamp
`friend` at emit time. Observer-relative `object_relation` does that job
without writing the observer's social graph onto a shared occurrence.

## Deliberately not doing

**Folding affect from a memory anchor, and sleep consolidation.** An earlier
draft proposed computing affect as a fold over the memory log from an anchor
committed at sleep, so that nothing recomputed mood per tick. The cost problem
it solved is already solved: `markMindDirty`/`nextThinkTick` skip decay and
arbitration for colonists nothing is happening to, and a valence-only change
deliberately does not dirty cognition. Building both would be two mechanisms
competing over one problem. Revisit only if a profile shows `decayAffect`
costing real time.

## Risks that remain

- **Doom spirals** — see above. The most likely way this ships and feels bad.
- **Saturation** — valence pegged at maximum the first time it ran, because
  routine work paid into an axis that drains slowly; wear then pegged for
  whatever a colonist does all day, which turned out to be the right answer
  but was not the expected one. Any new accumulating term needs a long-run
  check before its numbers are believed, not just a unit test.
- **Individuality reading as noise** — colonists behaving differently is the
  goal; colonists appearing to refuse work at random is the failure mode.
  Every mood-driven decision should be legible in the roster before it is
  legible only in the colonist's behavior.

## Extending it

- **A new reaction** — a row in `cognition.yaml`, plus perception if anyone
  besides the actor should notice.
- **A new appraisal trait** — one `trait_rules` row against the existing
  grammar. No event definitions are touched.
- **A specialist wear subsystem** — register a policy ID and select it on
  the reactions that own it. Do not add a branch in `applyAffect`.
- **A new mood name** — a row in the attractor table.

## Related

- [affect.md](./affect.md) — what actually runs: the three axes, wear,
  baselines, and the push/pull blend.
- [compositional-perception-and-events.md](./compositional-perception-and-events.md)
  — why the grammar replaced both `LifeEventKind` and tags.
- [cognition-config-and-lab.md](./cognition-config-and-lab.md) — authoring
  and the current sandbox.
- [memories.md](./memories.md) — the ingestion funnel, and the memory log
  wear counts from.
- [personality.md](./personality.md) — trait groups and the resolve-at-spawn
  principle baselines follow.
- [configuration.md](./configuration.md) — wear-per-occasion and
  friend-affinity knobs.

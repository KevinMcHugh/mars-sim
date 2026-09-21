# Mood space (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

The unbuilt half of colonist affect. [`affect.md`](./affect.md) documents what
runs today: three stored axes, and an impact that decides whether an event
nudges a colonist or relocates them. This doc covers what that leaves out.

The original proposal had three parts. Tags and config-driven trait rules have
now shipped through the compositional cognition system; two remain:

1. **Wear.** An event lands the same way the first time and the tenth. A
   colonist who has watched ten people die should not react like someone seeing
   their first.
2. **Baselines.** Affect decays to `(0, 0, 0)` for everyone. Nobody is natively
   anxious or natively hard to rattle.

Wear and baselines are not implemented. The tags section records the shipped
design because wear should build on its vocabulary rather than inventing
another event identity.

## Source

- [`mood-space.html`](./mood-space.html) — the tuning sandbox. Its event table
  is seeded from the shipped one, so the wear and tag mechanics below can be
  played with against real numbers. It emits the tuned tables as Go.
- [`../internal/sim/cognition_config.go`](../internal/sim/cognition_config.go) —
  shipped reaction tags and trait modifiers; the natural home for wear config.
- [`../internal/sim/personality.go`](../internal/sim/personality.go) — `Trait`,
  `traitGroup`, and the resolve-at-spawn principle baselines follow.
- [`../internal/sim/world.go`](../internal/sim/world.go) — `remember`, the one
  funnel a dynamic tag would be stamped in.

## How it works

### Wear: the first death and the tenth

Today each kind names one target. Let it name **two**: where the event puts a
colonist the first time, and where it puts them once they are used to it. A
per-kind habituation term `wear`, running 0 to 1 and rising with each
occurrence, picks between them.

    target = lerp(fresh, worn, wear)

This is also the change that lets the traumatic events say what they should.
The shipped `witnessed-colonist-killed` target is a compromise — one point
standing in for two very different reactions — and it had to be the *later* of
the two, because a colonist who reacts to every killing as a rallying cry is
worse than one who is always shaken. Wear splits it back apart:

| `witnessed-colonist-killed` | charge | grip | valence | lands in |
| --- | ---: | ---: | ---: | --- |
| `fresh` | 70 | 40 | −60 | furious |
| `worn` | 20 | −85 | −85 | despairing |
| *shipped today* | *28* | *−74* | *−60* | *anxious* |

The first death is a rallying cry, the tenth is catatonia, and the arc between
them is one number. Note which way round it goes: `fresh` is the *stronger*
reaction on grip, not the weaker one. Wear is not a volume knob.

For a mundane event the same pair reads as satisfaction and drudgery — no
special case, no second mechanism:

| `finished-mining` | charge | grip | valence | lands in |
| --- | ---: | ---: | ---: | --- |
| `fresh` | −1 | 5 | +3 | steady |
| `worn` | −6 | −4 | −3 | flat |

Note what monotony does and does not do. A worn shift does not crater anyone by
itself; it lands them in `flat` rather than `despairing`. What it takes away is
the small lift that used to offset the bad days, so the colony's baseline sags
rather than breaking. That is the honest version of "monotony becomes
depressing".

Traits bend `wear` instead of transforming the vectors: **Resilient** accrues it
slowly and stays near `fresh` for far longer; **Cowardly** starts partway along
and reaches `worn` in a couple of deaths. This is why a trait cannot simply
scale an event's vector — scaling changes how hard something hits, never whether
it hardens you or breaks you.

**Where the counter lives.** Count occurrences from the existing memory log: it
is free, it already carries `Kind`, and it decays naturally as entries roll off
at 64 per colonist. Measure whether that window feels too short before paying
for dedicated per-colonist state. Note the interaction with collapsed memories —
a run of digs folds into one entry, so counting entries and counting occurrences
are different numbers, and `Memory.Count` is the one to read.

### Tags: making trait × reaction tractable (shipped)

The compositional cognition system replaced the old trait × event switch with
an intermediate vocabulary:

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
and the four shipped transforms survive the translation unchanged.
`witnessed-colonist-killed` is tagged `gore` (the vocabulary can grow to
`death, violence, social-loss` as rules need them);
`TraitTidy` reacts to `gore`. A new trait is one rule against existing tags; a
new event is a tagging decision. Nobody ever has to answer "does ItemPurchased
impact Mutant?"

The `Impact` and `WearRate` factors are what make this worth more than tidier
plumbing: a rule can change how *significant* an event is to one colonist, not
just how hard it lands. Seeing a mouse is near-zero impact unless you are a
musophobe, and that cannot be said at all today.

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

- **Why not scale vectors per trait instead of wear?** Scaling or reflecting an
  event vector cannot express "the first death hardens you, the tenth breaks
  you" — it changes how hard something hits, never which direction the reaction
  turns. Transforms survive for low-impact events, where Tidy scaling gore
  really is a scale, which is why the tag rules keep them.
- **Why count wear from memories rather than a counter per kind?** A dedicated
  counter never forgets, so a colonist who saw three deaths a month ago would
  meet the fourth as a veteran. The memory log's eviction gives recovery for
  free, and recovery is most of what makes wear feel like a person rather than
  a ratchet.
- **Why is valence not in this doc any more?** It shipped. See
  [`affect.md`](./affect.md); the argument for storing it rather than deriving
  it from needs and HP is recorded there.

## Build plan

Three phases, roughly one PR each, in dependency order. Each must leave
`go build ./...` and `go test ./...` passing, keep seeded runs deterministic
(no map iteration, no wall-clock), put new tunables in `sim.Config` with a
regenerated `mars-sim.yaml`, keep the one-funnel invariant from
[`memories.md`](./memories.md), and update the docs it makes wrong.

### Phase 1 — Wear

Add `Worn` beside `Target` (renaming it `Fresh`), count occurrences from the
memory log, interpolate, and re-tune the traumatic events to the fresh/worn pair
they were always meant to be.

- **Verify:** the first witnessed death and the tenth produce visibly different
  outcomes for the same colonist, and a colonist who has not seen one in a long
  time meets the next one fresh again.
- **Watch for:** the same saturation trap valence hit. A wear term that only
  rises is a ratchet; check a long peaceful run before believing the numbers.

### Phase 2 — Tags and trait rules (shipped foundation)

Reaction tags, configured trait modifiers, and occurrence-level dynamic tags
now run in `rememberPercept`. `Resilient` and `Cowardly` still wait on Phase 1
because there is no wear rate for them to bend.

- **Verify:** adding a new trait touches one table and no event definitions; the
  four shipped transforms produce identical results through the new path.

### Phase 3 — Baselines

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

1. **Does wear survive its own recovery?** If the counter comes from memories,
   wear falls as entries roll off — but "I got over it" and "I never saw it" then
   look identical. Probably correct, worth watching in play. (Phase 1)
2. **Should `fresh`/`worn` interpolate per axis or as a whole point?** Per axis
   is simpler; a point lets the arc bend rather than run straight between two
   moods. (Phase 1)
3. **What pulls a colonist out of a doom spiral?** Low grip → worse choices →
   worse outcomes → lower grip is a real attractor, and nothing opposes it. Now
   that focus scoring reads grip, this is live rather than hypothetical.
4. **Do baselines shift valence, or only charge and grip?** An optimist arguably
   recovers faster rather than starting higher. (Phase 3)

## Risks

- **Doom spirals** — see open question 3. The most likely way this ships and
  feels bad.
- **Saturation** — valence pegged at maximum the first time it ran, because
  routine work paid into an axis that drains slowly. Wear has the same shape.
  Any new accumulating term needs a long-run check before its numbers are
  believed.
- **Tuning surface** — a fresh/worn pair per kind doubles the event table, and
  trait rules add another. The sandbox exists to make that tractable; if it
  feels unmanageable there, the model is too big.
- **Individuality reading as noise** — colonists behaving differently is the
  goal; colonists appearing to refuse work at random is the failure mode. Every
  mood-driven decision should be legible in the roster before it is legible only
  in the colonist's behavior.

## Extending it

Once built, the intended shapes:

- **A new cognitive reaction** — a compositional match in `cognition.yaml`,
  plus its tags.
- **A new trait effect** — one modifier against existing tags. No reaction
  definitions are touched.
- **A new mood name** — a row in the attractor table (already true today).

## Related

- [affect.md](./affect.md) — what actually runs: the three axes, the appraisal
  table, and the push/pull blend this builds on.
- [memories.md](./memories.md) — the life-event funnel, and the memory log wear
  would count from.
- [personality.md](./personality.md) — traits, trait groups, and the
  resolve-at-spawn principle baselines follow.
- [configuration.md](./configuration.md) — where the new tunables and flags go.

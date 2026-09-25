# Game design principles

> Part of the [mars-sim documentation](./README.md).

## What it is

The rules of thumb we keep coming back to, both for how a mechanic should
behave and for how it gets built. Each one is here because we learned it the
hard way at least once. The linked docs have the details.

This is a living list. When a design argument keeps coming up, write down how it
was settled here. If a principle stops being true, change it or remove it.

## Designing the game

### 1. Integers are a bad interface unless they are a quantity

A number is fine for *how much*: HP, a need level, a stack size, a distance. As
an interface between systems it is poor, because every consumer has to decide
for itself what the number means, and each consumer decides a little
differently. So expose an enum that other code can switch on, and keep the
number private to the system that owns it.

- **Morale as one scalar** was the first mistake. It said whether things were
  good or bad, and nothing could act on that ([affect.md](./affect.md)).
- **Having every system read charge/grip/valence itself** is the same mistake
  spread over three numbers. Every consumer ends up with its own thresholds.
  The direction we want is for affect to produce a *mood*, with other systems
  downstream of the mood.
- **Alien temperament** used to be a 0–100 aggression score where 0 happened to
  mean "never fights". Nothing had to handle that case. Now it is
  `Friendly`/`Cautious`/`Hostile`, and `alienTurn` switches on it. The compiler
  and the reviewer can both see every case ([lore.md](./lore.md)).
- **Height and weight for naming** are bucketed into tiers (`tiny` …) so the
  name conditions don't depend on raw centimetres.

**Where the code disagrees today:** `MoodKind` is still display-only. Focus
scoring reads `Charge` and `Grip` directly, and [affect.md](./affect.md) argues
the opposite ("labels are projections rather than states"). That argument was
about not hiding thresholds inside UI words, and it still matters. If a mood
becomes behavioral, its thresholds have to live in one named place and not in
string comparisons. Making mood the thing systems consume is the open work that
follows from this principle. When that lands, update affect.md.

### 2. Details create attachment, but they can't be noise

The more specific a colonist, an alien or the colony is, the easier the player
can recognize it and care about it. Flavor is what makes a run feel like a
story and not a sequence of dice rolls. Names, looks, family, memories,
species with their own names and glyphs: each one gives the player something
to hold on to.

Detail has to be legible to count, though. The test is whether the player can
*read* it:

- Heredity first copied looks directly, and families came out as visible
  clones. That is detail that erases individuality. The fix, a skin-tone nudge
  and per-feature donors, was checked against numbers over 40 seeds. Relatives
  share hair 61% of the time against a 32% baseline: close enough to see, loose
  enough to vary ([heredity.md](./heredity.md)).
- Two alien species that are both "Reptile" still draw different glyphs, so
  the map can tell them apart ([lore.md](./lore.md)).
- A mining shift collapses into one memory line and does not push twelve
  entries out of the log. A memory that is all digging hides the one that
  mattered ([memories.md](./memories.md)).

### 3. Flavor is free: it never changes the simulation

Principle 2 only works if adding detail is safe. Flavor draws from the
personality stream (`World.prng`), never from the simulation stream
(`World.rng`). A new name list, body type or alien emoji therefore can't
change a single AI decision, and nobody has to hold back on detail because of
what it might do to balance ([personality.md](./personality.md)).

The corollary: once a trait *does* change behavior, it is no longer flavor.
It needs a deliberate mechanical design and its own place in the rules.

### 4. One seed, one story

For a given seed and config, the run is the same every time. Players can share
and replay a colony, a bug can be reproduced, and two tunings can be compared
on the same history. This affects design choices as well as code. The director
settles its whole schedule up front, so that when an occurrence fires does not
depend on unrelated activity ([director.md](./director.md)). Map iteration
order must never decide an identity, an assignment or a tie
([determinism.md](./determinism.md)).

### 5. Characters carry their history, not just their circumstances

Two colonists standing on the same tile with the same needs should be able to
act differently, because they have lived different lives. That is why valence
is stored and not derived from the current situation. It is also why events
wear in ("fresh" vs "worn"), and why memories and affinity persist
([affect.md](./affect.md), [memories.md](./memories.md)). When you design a
mechanic, ask what it leaves behind on the colonist once the moment is over.

### 6. Every choice should be explainable

A player, or a contributor with a debugger, should be able to ask "why did they
do that?" and get an answer. Focus scores are additive named contributions
rather than a matrix, so a trace can show which term won
([cascading_wsts_architecture.md](./cascading_wsts_architecture.md)). Enums
beat emergent readings of formulas for the same reason (principle 1). Memories
are the player-facing version of the same idea.

A corollary: **count each signal once.** Valence stays out of focus scoring
because needs, HP and threats already reach the score directly. Letting their
summary back in would weight them twice and make the explanation wrong.

### 7. Death should come from the story, not from a bug

A colonist dying of an alien, a famine or a bad decision is a story. A colonist
dying because they were walled into a closet, or because hunger raced ahead of
the first food pod, just looks broken. Our pattern is to *prevent what is cheap
to prevent, and give a way out of whatever isn't*:

- Grace periods at startup, and fatal needs outranking non-fatal ones
  ([needs.md](./needs.md)).
- The emergency-build fallback ([construction.md](./construction.md)) and
  breaking out of sealed rooms ([escape.md](./escape.md)).

When a mechanic can strand a colonist, give it an exit in the same change.

### 8. Content is data; rules are code

Anything a designer should be able to tune or extend without a recompile goes
in data: alien names in YAML with a plain `all`/`any`/`not` condition tree,
the director schedule, the tables of life-event vectors, and tunables in
`sim.Config` ([configuration.md](./configuration.md), [lore.md](./lore.md)).
Structural correctness, such as job ownership, pathing and invariants, stays in
code.

## Building it

### 9. Build for big colonies that run for a long time

The target is thousands of colonists on maps thousands of tiles across, left
running for hours. A mechanic that is fine with six colonists for ten minutes
can be the thing that makes the game unplayable at that size. So performance at
scale is part of the design from the start. It is not a pass we save for later.

- **Never rescan the world.** Every "how many / who / where" question is
  answered by an index that is updated as things change. That took a
  2000-colonist tick from ~42 s to ~13 ms
  ([spatial-index-and-performance.md](./spatial-index-and-performance.md)).
- **Cost follows the colony, not the map.** Navigation grids allocate pages
  only where the colony has been, and a published frame copies only what the
  tick touched. A 10000x10000 world went from ~16 GB to ~590 MB
  ([sparse-grids.md](./sparse-grids.md),
  [snapshot-tile-grid.md](./snapshot-tile-grid.md)).
- **Pay once, not per tick.** Traits resolve at spawn, needs are computed when
  read, and a resting colonist does no work at all
  ([personality.md](./personality.md), [needs.md](./needs.md)).
- **Long runs have to stay bounded.** Per-colonist history is capped (the
  memory buffer holds 64 entries, and repeated events collapse into one), and
  dead colonists are archived so lookups by ID keep working
  ([memories.md](./memories.md), [combat.md](./combat.md)). Decide how anything that grows over a run
  stops growing.

Before a new per-tick or per-colonist system is done, ask what it costs with
2000 colonists and 100k ticks, and run the benchmarks in
`internal/sim/bench_test.go`. Optimize with measurements, not guesses. Most
of the numbers above come from a stress run that someone actually left
running.

### 10. Reuse an existing mechanic before inventing one

A new mechanic needs its own tests, its own edge cases and its own doc. So look
for an existing one first. Supply drops hand weapons straight to colonists
through the same path as the ship's starting equipment, instead of adding
items on the floor ([director.md](./director.md)). The focus system was
layered on top of jobs and left jobs in place, because jobs already encode ownership,
claims and pathing ([cascading_wsts_architecture.md](./cascading_wsts_architecture.md)).
When a feature really does need a new mechanic, that is fine. It just shouldn't
be the default.

### 11. Ship the simplest version, without closing off the next one

The director's first version picks a tick and a candidate uniformly at random.
Mood decay is linear per axis, and polar decay waits until a behavior needs it.
Colonists still treat every alien as a threat, whatever its temperament. Each of those was a deliberate
first cut. The doc says what the next step would be, and the design leaves
room for it. Start with the version you can explain in one sentence, and write
down what you left out and why.

### 12. The simulation doesn't know how it is drawn

The engine publishes immutable snapshots and frontends render them
([architecture.md](./architecture.md)). `sim` carries an alien's emoji as an
opaque string; checking glyph widths and choosing a fallback is entirely the
TUI's job ([lore.md](./lore.md), [terminal-cell-widths.md](./terminal-cell-widths.md)).
That is what lets a second frontend (the web UI we want) arrive without
touching gameplay. It also keeps the two sides from compromising each other,
so display concerns never become game rules.

## Related

- [personality.md](./personality.md), [heredity.md](./heredity.md),
  [lore.md](./lore.md): where most of the "detail" work lives.
- [affect.md](./affect.md), [mood-space.md](./mood-space.md): the mood work
  that principle 1 points at.
- [determinism.md](./determinism.md): the engineering side of principles 3 and 4.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md),
  [sparse-grids.md](./sparse-grids.md): the scale work behind principle 9.

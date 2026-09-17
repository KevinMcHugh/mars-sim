# Colonist ages and family

> Part of the [mars-sim documentation](./README.md).

## What it is

Every generated colonist has an adult age shown with their profile. Family
relationships use that age to prevent a parent from being less than 20 years
older than their child, and a grandparent from being less than 40 years older
than their grandchild. Once a colonist is placed in the tree, their family also
decides part of who they are — surname, appearance, and a warm start with their
relatives; see [heredity.md](./heredity.md).

## Source

- `internal/sim/personality.go` — age storage and deterministic generation.
- `internal/sim/relationships.go` — family-tree wiring and its age validation.
- `internal/sim/heredity.go` — what a colonist takes from the family they land in.
- `internal/ui/tui/render_roster.go` — profile display.
- `internal/sim/relationships_test.go` — family-tree behavior.

## How it works

`Profile.Age` is generated on a deterministic, personality-owned flavor stream in
the inclusive range 18–80. The age stream is isolated from the existing
personality/family stream, so adding age does not reshuffle names, traits, or
family topology. This keeps age as flavor data, like height and weight, without
changing simulation outcomes.

A colonist's relation list reaches two generations in each direction, so those
are the ties whose ages have to hang together: a parent is at least
`minParentAgeGap` (20) years older than their child, and a grandparent twice
that. Checking that one link at a time is not enough, because the generation in
between is frequently a phantom who never joined the colony and has no age to
check. So before `wireRelation` attaches anybody, `fitsBelowKin` walks up from
the slot the newcomer would occupy and `fitsAboveKin` walks down from it, each
comparing every *colonist* they reach against the number of parent links away it
is (`validAncestor`). Any phantom generations along the way contribute no check
of their own and are folded into one wider gap against the colonist beyond them.

That makes every tie go through the same rule rather than only the two that name
a parent. A sibling is a new child of whatever parents the existing sibling has,
so those parents have to be old enough for the newcomer too. An aunt/uncle is a
new child of the grandparents. A grandparent hung off a phantom shared by
siblings becomes a grandparent of all of them, so the walk checks each one. A
spouse is added as a second parent only when they pass the same walk.

The derived `Relations` lists in snapshots are cached per colonist and keyed to a
family-tree revision. Adding a relationship invalidates the revision; ordinary
simulation ticks reuse the existing lists instead of walking the family tree.

`assignKin` reports whether it actually tied the colonist to anyone. A colonist
who arrived alone skips the heredity pass entirely, which also avoids rebuilding
the child index for a tree that did not change.

## Why it is this way

Age belongs in `Profile` because it is identity data already copied through
snapshots, rather than a simulation stat that needs ticking. The family
generator can encounter arbitrary existing colonists, so validating the tie
against the tree is safer than trying to infer a compatible age before choosing
a relationship kind. `relate` already tries the relationship kinds in a random
order and takes the first that fits, so a rejected tie costs nothing: the
newcomer falls back to a kind their age does support rather than failing to get
a family.

Walking the tree, rather than giving phantoms an invented age, keeps the ages
that exist as the only ground truth. A phantom with an age would have to pick
one gap out of a plausible range, and that guess would then constrain everyone
later hung off it — a sibling set sharing one phantom parent would be capped at
whatever age the phantom happened to be given. Checking real colonists against
their generation distance imposes only what is actually implied.

The walk stops at `kinDisplayGenerations` because that is how far a relation
list reaches. Constraining great-grandparents nobody can see would reject
plausible families for an invariant with no observable effect.

An unknown age (zero in hand-built profiles) remains permissive for tooling and
older tests; generated colonists never have an unknown age.

## Extending it

If age begins affecting work, health, or other simulation behavior, keep those
rules separate from family validation and decide whether age should advance
with simulation time. Any new relationship kind must route the parent links it
creates through `fitsAboveKin`/`fitsBelowKin` rather than checking a single link
itself, or it will reintroduce ties that only look valid one hop at a time. If
relation lists ever reach a third generation, raise `kinDisplayGenerations` with
them so the checks keep pace with what is displayed.

## Related

- [personality.md](./personality.md) — profile generation and the personality RNG.
- [heredity.md](./heredity.md) — shared surnames, inherited appearance, and the
  affinity relatives start with.
- [entities-and-ai.md](./entities-and-ai.md) — colonist entities and their
  runtime behavior.

# Colonist ages and family

> Part of the [mars-sim documentation](./README.md).

## What it is

Every generated colonist has an adult age shown with their profile. Family
relationships use that age to prevent a parent from being less than 20 years
older than their child. Once a colonist is placed in the tree, their family also
decides part of who they are — surname, appearance, and a warm start with their
relatives; see [heredity.md](./heredity.md).

## Source

- `internal/sim/personality.go` — age storage and deterministic generation.
- `internal/sim/relationships.go` — parent/child age validation.
- `internal/sim/heredity.go` — what a colonist takes from the family they land in.
- `internal/ui/tui/render_roster.go` — profile display.
- `internal/sim/relationships_test.go` — family-tree behavior.

## How it works

`Profile.Age` is generated on a deterministic, personality-owned flavor stream in
the inclusive range 18–80. The age stream is isolated from the existing
personality/family stream, so adding age does not reshuffle names, traits, or
family topology. This keeps age as flavor data, like height and weight, without
changing simulation outcomes. When a generated family tie would make one colonist a
parent of another, `wireRelation` accepts it only if the proposed parent is at
least 20 years older. A spouse is added as a second parent only when that spouse
also satisfies the same rule.

The derived `Relations` lists in snapshots are cached per colonist and keyed to a
family-tree revision. Adding a relationship invalidates the revision; ordinary
simulation ticks reuse the existing lists instead of walking the family tree.

`assignKin` reports whether it actually tied the colonist to anyone. A colonist
who arrived alone skips the heredity pass entirely, which also avoids rebuilding
the child index for a tree that did not change.

## Why it is this way

Age belongs in `Profile` because it is identity data already copied through
snapshots, rather than a simulation stat that needs ticking. The family
generator can encounter arbitrary existing colonists, so validating each direct
parent link is safer than trying to infer a compatible age before choosing a
relationship kind. An unknown age (zero in hand-built profiles) remains
permissive for tooling and older tests; generated colonists never have an
unknown age.

## Extending it

If age begins affecting work, health, or other simulation behavior, keep those
rules separate from family validation and decide whether age should advance
with simulation time. Any new parent link must continue to pass `validParent`.

## Related

- [personality.md](./personality.md) — profile generation and the personality RNG.
- [heredity.md](./heredity.md) — shared surnames, inherited appearance, and the
  affinity relatives start with.
- [entities-and-ai.md](./entities-and-ai.md) — colonist entities and their
  runtime behavior.

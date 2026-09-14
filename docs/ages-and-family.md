# Colonist ages and family

> Part of the [mars-sim documentation](./README.md).

## What it is

Every generated colonist has an adult age shown with their profile. Family
relationships use that age to prevent a parent from being less than 20 years
older than their child.

## Source

- `internal/sim/personality.go` — age storage and deterministic generation.
- `internal/sim/relationships.go` — parent/child age validation.
- `internal/ui/tui/render_roster.go` — profile display.
- `internal/sim/relationships_test.go` — family-tree behavior.

## How it works

`Profile.Age` is generated on the personality RNG stream in the inclusive range
18–80. This keeps age as flavor data, like height and weight, without changing
simulation outcomes. When a generated family tie would make one colonist a
parent of another, `wireRelation` accepts it only if the proposed parent is at
least 20 years older. A spouse is added as a second parent only when that spouse
also satisfies the same rule.

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
- [entities-and-ai.md](./entities-and-ai.md) — colonist entities and their
  runtime behavior.

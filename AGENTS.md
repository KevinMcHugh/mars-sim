# Contributing to mars-sim

This file is the project's working agreement for both people and coding agents.
Keep it short; the detail lives in [`docs/`](./docs/README.md).

## Every feature gets a doc

We kept paying the same tax: each new thread (human or agent) rediscovered how a
system worked by re-reading the code from scratch. To stop that, **every new
feature or system ships with a Markdown write-up under [`docs/`](./docs/README.md)**,
in the same change that adds the feature.

A "feature" here means anything a future contributor would otherwise have to
reverse-engineer: a new simulation system (a need, a creature, an AI behavior),
a new subsystem or algorithm (pathfinding, a spatial index), a new frontend, or
a cross-cutting mechanic. A pure bug fix or refactor does not need a new doc, but
it **must** update any existing doc it makes inaccurate.

### The rule

1. Add or update a doc in `docs/` in the same PR/commit as the code.
2. Start from [`docs/TEMPLATE.md`](./docs/TEMPLATE.md) so docs stay consistent.
3. List the new doc in the index table in [`docs/README.md`](./docs/README.md).
4. Explain the *why* (the decisions and dead ends), not just the *what* — the
   code already says what it does. The most valuable lines are the ones that
   save the next person from repeating a mistake.
5. If a change makes an existing doc wrong, fix the doc. A stale doc is worse
   than no doc.

A change that adds a feature without a doc is incomplete. Reviewers should ask
for the doc the same way they would ask for tests.

## Other expectations

- `go build ./...` and `go test ./...` pass before you call a change done.
- The simulation must stay deterministic for a given seed: keep flavor/RNG that
  should not affect gameplay on the personality stream (`World.prng`), not the
  simulation stream (`World.rng`). See [`docs/personality.md`](./docs/personality.md).
- New tunables go in `sim.Config` with a matching command-line flag in
  `main.go`, defaulting to the `DefaultConfig` value. See
  [`docs/configuration.md`](./docs/configuration.md).

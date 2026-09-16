# Configuration

> Part of the [mars-sim documentation](./README.md).

## What it is

Every tunable knob for a run — world size, populations, timing, per-creature stats,
need specs — lives in one `Config` struct. `DefaultConfig()` is the single source
of truth for balance, and `main.go` exposes each field as a command-line flag that
defaults to the `DefaultConfig` value.

## Source

- [`internal/sim/config.go`](../internal/sim/config.go) — `Config`, `DefaultConfig`, `tickInterval`.
- [`main.go`](../main.go) — `bindConfigFlags` (one flag per field) and `validateConfig`.

## How it works

`Config` groups its fields by concern: world shape and rock composition, seed,
starting population and equipment, timing, colonist stats, needs, personality,
the mining-strategy switch, and per-creature stats for aliens, cats, mice, and
weapons. Zero values are not meaningful — always start from `DefaultConfig()`
and adjust.

`main.go` mirrors this: `bindConfigFlags(&cfg)` registers a flag for every field,
passing the current (default) value as the flag default, so the help text always
shows the real defaults. `DefaultConfig` stays the one place balance is defined.

A few application-level flags are *not* part of `Config` (they control the process,
not the simulation): `-duration`, `-headless`, and `-seed`. Their startup,
validation, and headless-reporting behavior is documented in the
[command-line guide](./cli.md). `-seed 0` (the default) means "pick a fresh
time-based seed each run"; any non-zero seed makes the run reproducible.

`validateConfig` rejects settings that would break world generation or the
renderer (too-small world, negative populations, rock composition percentages
whose sum exceeds 100, sub-1 rates) with a message a player can act on, before
the engine is built.

`tickInterval` converts `TicksPerSecond` into a sleep duration, clamped to
[1, 60].

## Why it is this way

- **One struct, one default set** means balancing the game is editing values in
  `config.go`, not hunting through the systems for magic numbers.
- **Flags derived from the struct** keep the CLI and the defaults from drifting:
  you cannot add a tunable and forget its default, because the default *is* the
  flag default.
- **Determinism** is a first-class config concern: same seed + same code => same
  game. Flavor generation uses a *separate* RNG so it never perturbs this (see
  [personality.md](./personality.md)).

## Extending it

Adding a tunable is a three-step edit:

1. Add the field to `Config`, grouped with related fields, with a comment.
2. Set its default in `DefaultConfig()`.
3. Register a flag for it in `bindConfigFlags` in `main.go`, defaulting to the
   `DefaultConfig` value; add a `validateConfig` case if bad values would crash.

Need specs are configured as data here too: `Config.Needs` is a `[numNeeds]NeedSpec`
indexed by `NeedKind`. See [needs.md](./needs.md).

## Related

- [architecture.md](./architecture.md) — how the config seeds the engine.
- [needs.md](./needs.md) — the `NeedSpec` table inside `Config`.
- [entities-and-ai.md](./entities-and-ai.md) — the creature stats these fields tune.
- [combat.md](./combat.md) — the weapon and starting-equipment stats these fields tune.

# Configuration

> Part of the [mars-sim documentation](./README.md).

## What it is

Every tunable knob for a run — world size, populations, timing, per-creature stats,
need and focus specs — lives in one `Config` struct. `DefaultConfig()` is the single source
of truth for balance. Each field carries a `cfg` tag naming the knob once; that
name becomes both its command-line flag and its key in the committed
[`mars-sim.yaml`](./config-file.md) settings file.

## Source

- [`internal/sim/config.go`](../internal/sim/config.go) — `Config`, `DefaultConfig`, `tickInterval`.
- [`internal/sim/configfile.go`](../internal/sim/configfile.go) — `Knobs`, which
  turns the `cfg` tags into the flag and settings-file surfaces.
- [`main.go`](../main.go) — `bindConfigFlags` and `validateConfig`.

## How it works

`Config` groups its fields by concern: world shape and rock composition, seed,
starting population and equipment, timing, colonist stats, needs, personality,
the mining-strategy switch, and per-creature stats for aliens, cats, mice, and
weapons. Zero values are not meaningful — always start from `DefaultConfig()`
and adjust.

`main.go` mirrors this: `bindConfigFlags` walks `sim.Knobs(&cfg)` — the `cfg`,
`doc` and `sec` struct tags, read by reflection — and registers a flag for every
field, passing the current value as the flag default. That current value is the
default as modified by the settings file, which is read first, so the help text
always shows the defaults the run will actually use. `DefaultConfig` stays the
one place balance is defined; see [config-file.md](./config-file.md) for the
layering and the tags.

A few application-level flags are *not* part of `Config` (they control the process,
not the simulation): `-duration`, `-headless`, and `-seed`. Their startup,
validation, and headless-reporting behavior is documented in the
[command-line guide](./cli.md). `-seed 0` (the default) means "pick a fresh
time-based seed each run"; any non-zero seed makes the run reproducible.

Fields without a `cfg` tag are not tunable from outside the code. `Seed` is the
notable one: the flag and the file both carry it as a special case where 0 means
"roll a fresh time-based seed". `NeedSpec.Name` and `NeedSpec.Facility` are
untagged too — a need's identity and its plumbing are not balance.

`validateConfig` rejects settings that would break world generation or the
renderer (too-small world, negative populations, rock composition percentages
whose sum exceeds 100, an invalid rock-vein or cavern size range, out-of-range
cavern percentages, sub-1 rates) with a
message a player can act on, before the engine is built.

`tickInterval` converts `TicksPerSecond` into a sleep duration, clamped to
[1, 60].

## Why it is this way

- **One struct, one default set** means balancing the game is editing values in
  `config.go`, not hunting through the systems for magic numbers.
- **Flags derived from the struct** keep the CLI and the defaults from drifting:
  you cannot add a tunable and forget its default, because the default *is* the
  flag default. Deriving them by reflection rather than by hand closes the other
  half of that gap — `StuckLimit` had been in `Config` for a while with no flag
  at all.
- **Determinism** is a first-class config concern: same seed + same code => same
  game. Flavor generation uses a *separate* RNG so it never perturbs this (see
  [personality.md](./personality.md)).

## Extending it

Adding a tunable is a two-step edit:

1. Add the field to `Config`, grouped with related fields, tagged
   `cfg:"flag-name" doc:"what it does"` (plus `sec:"Group"` if it starts a new
   section). Set its default in `DefaultConfig()`. The flag and the
   settings-file key come from the tag; there is no list to update.
2. Regenerate the settings file — `go run . -print-config > mars-sim.yaml` —
   and add a `validateConfig` case in `main.go` if bad values would crash.

Need specs are configured as data here too: `Config.Needs` is a `[numNeeds]NeedSpec`
indexed by `NeedKind`, and its tagged fields become nested settings
(`needs.food.rise` in the file, `-need-food-rise` on the command line). See
[needs.md](./needs.md).

Focus arbitration follows the same pattern: `Config.Focuses` is indexed by
`FocusKind`, producing settings such as `focuses.work.base` and flags such as
`-focus-work-base`. Global commitment, switch-margin, critical, and fatal
bonuses are top-level focus settings. See
[`cascading_wsts_architecture.md`](./cascading_wsts_architecture.md).

## Related

- [config-file.md](./config-file.md) — the committed `mars-sim.yaml` layer and the tags behind it.
- [architecture.md](./architecture.md) — how the config seeds the engine.
- [needs.md](./needs.md) — the `NeedSpec` table inside `Config`.
- [entities-and-ai.md](./entities-and-ai.md) — the creature stats these fields tune.
- [combat.md](./combat.md) — the weapon and starting-equipment stats these fields tune.

# Command-line application

> Part of the [mars-sim documentation](./README.md).

## What it is

`main.go` is the process entry point. It creates a default simulation
configuration, applies command-line overrides, validates the result, starts the
engine, and selects either the Bubble Tea frontend or a periodic headless
reporter. The application flags control the process; simulation flags are
forwarded into `sim.Config`.

## Source

- [`main.go`](../main.go) — `main`, `bindConfigFlags`, `validateConfig`,
  `runTUI`, and `runHeadless`.
- [`internal/sim/config.go`](../internal/sim/config.go) — simulation defaults
  and tunable fields.
- [`internal/sim/engine.go`](../internal/sim/engine.go) — engine startup and
  frontend communication.

## How it works

Startup follows this order:

1. Copy `sim.DefaultConfig()`.
2. Apply the settings file (`mars-sim.yaml`, or `-config PATH`) on top of it.
   This happens *before* the flags are registered, so the file's values become
   the flag defaults — which is why `-config` is located by scanning `os.Args`
   rather than by the flag package. See [config-file.md](./config-file.md).
2b. Apply the director's schedule file (`director.yaml`, or `-director PATH`)
    the same way, for the same reason. See [director.md](./director.md).
3. Register flags whose defaults come from that config.
4. Parse flags (with `?`, `-?`, and `--?` as help aliases).
5. Apply a non-zero `-seed` override.
6. Validate dimensions, populations, rates, and other safety constraints.
7. Create and subscribe to the engine before starting `Engine.Run`.
8. Run either the TUI or the headless snapshot consumer.

### Application flags

These flags control how the process runs rather than the simulated world:

| Flag | Behavior |
| --- | --- |
| `-headless` | Skip the TUI and print periodic population/facility/excavation statistics. Useful for CI, profiling, and non-TTY runs. |
| `-duration <time>` | Stop automatically after the duration, such as `10s` or `250ms`. The default `0` means run until quit/interruption. |
| `-seed <int64>` | Select a reproducible world seed. `0` leaves the time-based default seed in place. |
| `-config <path>` | Read this settings file instead of `mars-sim.yaml` in the working directory. A file named here that does not exist is an error; `-config ""` reads no file at all. |
| `-director <path>` | Read this director schedule file instead of `director.yaml` in the working directory. A file named here that does not exist is an error; `-director ""` runs with no scheduled occurrences. See [director.md](./director.md). |
| `-print-config` | Write a commented settings file with every setting at its default to stdout, then exit. Redirect it to `mars-sim.yaml` to regenerate the committed file. |
| `-glyphs <mode>` | How to draw map glyphs: `auto` (default) measures each glyph against the terminal at startup and falls back to ASCII if any is painted at an unexpected width; `emoji` skips the probe and trusts the built-in width table; `ascii` forces the fallback set. See [terminal-cell-widths.md](./terminal-cell-widths.md). |
| `-h`, `-help`, `?` | Print usage, examples, and all available flags. |

Headless mode prints a startup line, then the latest snapshot approximately once
per second. With a duration it prints a final summary before returning. It
consumes the same snapshot channel as the TUI; it does not access the mutable
world directly.

Examples:

```sh
# Interactive TUI with defaults.
go run .

# Reproducible ten-second smoke test with no terminal UI.
go run . -headless -duration 10s -seed 42

# Run a larger colony and tune the simulation speed.
go run . -colonists 20 -aliens 5 -cats 4 -mice 20 -tps 12

# Inspect every available option.
go run . -h

# Start a settings file you can commit, then edit it and run with it.
go run . -print-config > mars-sim.yaml
go run .
```

### Validation

`validateConfig` rejects a world smaller than 10x10, negative population counts,
less than one tick per second, fewer than one colonist per facility, fewer than
one max concurrent project, less than one rest tick, or a trait chance outside
0–100. Invalid settings are reported to stderr and exit with status 2 before
the engine starts.

The engine raises rates below 1 to 1, and interactive speed changes use the
same floor. There is deliberately no upper cap (there used to be one at 60):
asking for more ticks per second than the machine can simulate just runs the
sim flat out, because the engine catches up on late ticks only within
`maxTickLag` (see [architecture.md](./architecture.md)). CLI validation covers values whose bad settings would make world
generation or gameplay invalid; if a new tunable has stronger invariants, add
them to `validateConfig`.

## Why it is this way

- **Application flags are separate from `Config`** so test-run duration and
  presentation mode cannot accidentally become simulation state.
- **Headless mode uses snapshots** rather than a second simulation path, keeping
  CI and profiling behavior representative of the real engine.
- **Subscribe before `Run`** ensures the initial world frame is available to
  either consumer.
- **Defaults stay centralized**: every simulation flag is registered from the
  already-created `Config`, so `DefaultConfig` remains the balancing source of
  truth.

## Extending it

- Add a process-only option beside `duration`, `headless`, and `seed` when it
  changes application behavior rather than simulation behavior.
- Add a simulation tunable to `Config` and `DefaultConfig`, then expose it in
  `bindConfigFlags`; see [configuration.md](./configuration.md).
- If a new mode consumes the engine, subscribe before starting `Run` and consume
  immutable snapshots. Do not read `Engine` or `World` internals.
- Update this document and the index whenever the command's behavior changes.

## Related

- [configuration.md](./configuration.md) — simulation tunables and defaults.
- [config-file.md](./config-file.md) — the committed `mars-sim.yaml` settings layer.
- [architecture.md](./architecture.md) — the engine/snapshot contract.
- [frontend-tui.md](./frontend-tui.md) — the interactive frontend.

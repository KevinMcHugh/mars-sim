# Architecture

> Part of the [mars-sim documentation](./README.md).

## What it is

mars-sim is a headless simulation engine that can drive multiple frontends. The
simulation and any renderer run in **separate goroutines and never share mutable
state**: the engine ticks the world and publishes immutable snapshots; frontends
consume snapshots and send back commands. This is the single most important
structural decision in the project — everything else hangs off it.

## Source

- [`main.go`](../main.go) — wires the engine to a frontend (TUI or headless), owns the flags.
- [`internal/sim/engine.go`](../internal/sim/engine.go) — `Engine`: the tick loop, subscriptions, commands.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — the immutable per-frame copy handed to frontends.
- [`internal/sim/events.go`](../internal/sim/events.go) — the in-engine event bus derived systems react to.
- [`internal/sim/world.go`](../internal/sim/world.go) — the mutable game state the engine owns.

## How it works

```
                 Commands (pause, speed, spawn)
   ┌────────────┐  ───────────────────────────▶  ┌──────────────┐
   │  Frontend  │                                 │    Engine    │
   │ (Bubbletea)│  ◀───────────────────────────   │  (sim loop)  │
   └────────────┘        Snapshots (per tick)     └──────────────┘
```

### The engine goroutine owns everything mutable

`Engine.Run(ctx)` is launched in its own goroutine and is the **only** goroutine
that touches `World`. Its loop (`engine.go`) selects over three things:

- `ctx.Done()` — shut down and close all subscriber channels.
- an incoming `Command` — applied *between* ticks, so input never races the sim.
- the ticker — when not paused, `world.step()` then `publish()`.

Because commands are drained on the same goroutine that steps the world, there is
no locking around game state at all. The only shared state is the slice of
subscriber channels, guarded by a small mutex.

### Frontends are pure consumers

A frontend interacts through exactly two methods:

- `Engine.Subscribe() <-chan *Snapshot` — returns a channel of immutable frames.
  Call it *before* `Run` so the initial frame is not missed. The channel has
  **capacity 1 and the engine drops stale frames** rather than blocking, so a
  slow renderer can never stall the simulation (`publish` in `engine.go`).
- `Engine.Send(Command)` — submits input. It **never blocks**: if the command
  buffer is full the command is dropped, which is fine for interactive controls.

Any number of frontends can attach to one engine. The TUI is one consumer; the
headless stats printer in `main.go` is another; a web or GUI frontend would
implement the same contract unchanged.

### Snapshots are deep copies

`World.snapshot()` builds a `Snapshot`: a copied tile grid, a slice of
`EntityView` value copies (with deep-copied `Profile`s and by-value inventories),
the event log tail, and aggregate `Stats`. Nothing in a snapshot aliases live
state, so a frontend can read a frame on its own goroutine while the engine keeps
mutating the world. Need levels are computed *as of the snapshot tick* (see
[needs.md](./needs.md)).

### The event bus

Within the engine, systems avoid rescanning the world by reacting to events
(`events.go`). The bus is deliberately tiny and **synchronous**: `emit` calls
every subscriber inline on the engine goroutine, so there is no locking and no
ordering surprise. Today the only event is `TileChanged`; the job board and the
flow fields subscribe to it to stay incrementally up to date. See
[spatial-index-and-performance.md](./spatial-index-and-performance.md).

## Why it is this way

- **No shared mutable state** removes an entire class of data races and makes the
  sim reproducible: a run is a pure function of its seed and the command stream.
- **Drop-stale-frames** publishing decouples sim speed from render speed. The sim
  is the source of truth; the renderer shows the freshest frame it can keep up
  with and never applies backpressure.
- **Non-blocking commands** keep the UI responsive; dropping an occasional
  keypress under a full buffer is preferable to blocking a frontend.
- A **synchronous** event bus was chosen over channels/async because handlers are
  cheap and run on the owning goroutine; async would reintroduce ordering and
  locking concerns for no benefit at this scale. Note that boxing a value into
  the `Event` interface allocates, so producers emit only on real changes
  (`SetTerrain` no-ops when terrain is unchanged).

## Extending it

- **A new frontend**: implement a consumer that reads `Subscribe()` frames and
  sends `Command`s. Do not reach into `sim` internals; if you need more data,
  add a field to `Snapshot`/`EntityView`/`Stats`.
- **A new command**: add a type implementing `Command` (the `isCommand()` marker)
  and handle it in `Engine.apply`. Return `true` from `apply` if it changes the
  tick rate so the loop resets the ticker.
- **A new derived system**: subscribe to the event bus in `newWorld` and keep
  your own incremental state, rather than scanning the world each tick.

## Related

- [configuration.md](./configuration.md) — how a run is configured and seeded.
- [world.md](./world.md) — the state the engine owns.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the reactive, incremental systems.
- [frontend-tui.md](./frontend-tui.md) — the reference frontend.

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
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — the immutable per-frame view handed to frontends.
- [`internal/sim/tilegrid.go`](../internal/sim/tilegrid.go) — the page-shared terrain grid inside a `Snapshot`.
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
- the tick timer — when not paused, `world.step()` then, at most
  `maxPublishRate` (60) times a second, `publish()`.

Ticks are paced against fixed deadlines, not a `time.Ticker`: a ticker drops
any tick whose wakeup came late, and macOS routinely wakes timers late to save
power, so a ticker's achieved rate sinks to whatever cadence the OS delivers.
The loop remembers when each
tick was due (`nextDue`) and runs overdue ones straight away, up to
`maxTickLag` behind; past that it restarts the schedule from now and simply
runs flat out.

Two things keep a fast engine from paying for work nobody sees:

- **Publishing is capped at 60 frames a second** (`shouldPublish`). At or
  below 60 tps every tick publishes; above it, a tick publishes only once
  1/60 s has passed since the last frame. The TUI draws 30 frames a second and
  keeps only the newest snapshot, so at a few hundred tps nearly every
  snapshot used to be built only to be thrown away, and every send woke the
  frontend's goroutine. A pause publishes straight away, so the paused frame is
  always current.
- **An overdue tick runs without sleeping first.** Parking on a timer that has
  already fired still costs a goroutine wakeup, so the loop checks commands
  without blocking and ticks. While paused it waits only for commands, rather
  than waking every interval to skip a tick.

Measured headless with 100 colonists, 10 s runs: at 5000 tps the same number of
ticks took 1.5 s of CPU instead of 6.7 s (publishing fell from 34% of the
profile to 4%); at 300 tps, 1.35 s instead of 1.63 s.

Because commands are drained on the same goroutine that steps the world, there is
no locking around game state at all. The only shared state is the slice of
subscriber channels, guarded by a small mutex.

### Frontends are pure consumers

A frontend interacts through exactly two methods:

- `Engine.Subscribe() <-chan *Snapshot` — returns a channel of immutable frames.
  Call it *before* `Run` so the initial frame is not missed. The channel has
  **capacity 1 and the engine drops stale frames** rather than blocking, so a
  slow renderer can never stall the simulation (`publish` in `engine.go`).
  Frames arrive at most 60 times a second whatever the tick rate, so a
  consumer must not assume one snapshot per tick: compare `Snapshot.Tick`.
- `Engine.Send(Command)` — submits input. It **never blocks**: if the command
  buffer is full the command is dropped, which is fine for interactive controls.

Any number of frontends can attach to one engine. The TUI is one consumer; the
headless stats printer in `main.go` is another; a web or GUI frontend would
implement the same contract unchanged.

### Snapshots never alias live state

`World.snapshot()` builds a `Snapshot`: the terrain grid, a slice of
`EntityView` value copies (with deep-copied `Profile`s and by-value inventories),
the event log tail, and aggregate `Stats`. Nothing in a snapshot aliases live
state, so a frontend can read a frame on its own goroutine while the engine keeps
mutating the world. Need levels are computed *as of the snapshot tick* (see
[needs.md](./needs.md)).

Everything colony-sized is copied outright. The terrain is the exception: copying
the whole map every tick made a big map's tick rate a function of its area, so
`Snapshot.Tiles` is an immutable **page-shared** grid instead — a frame re-copies
only the pages whose tiles changed and shares the rest with the frames already
published. It is still safe to read on another goroutine, for the same reason a
copy would be: a published page is never written again. See
[snapshot-tile-grid.md](./snapshot-tile-grid.md).

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
  the `WorldEvent` interface allocates, so producers emit only on real changes
  (`SetTerrain` no-ops when terrain is unchanged).

## Extending it

- **A new frontend**: implement a consumer that reads `Subscribe()` frames and
  sends `Command`s. Do not reach into `sim` internals; if you need more data,
  add a field to `Snapshot`/`EntityView`/`Stats`.
- **A new command**: add a type implementing `Command` (the `isCommand()` marker)
  and handle it in `Engine.apply`. Return `true` from `apply` if it changes the
  tick rate so the loop restarts its tick schedule.
- **A new derived system**: subscribe to the event bus in `newWorld` and keep
  your own incremental state, rather than scanning the world each tick.

## Related

- [configuration.md](./configuration.md) — how a run is configured and seeded.
- [world.md](./world.md) — the state the engine owns.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) — the reactive, incremental systems.
- [frontend-tui.md](./frontend-tui.md) — the reference frontend.

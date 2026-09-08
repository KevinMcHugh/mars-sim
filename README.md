# mars-sim

A Mars-colonization simulation game, in the vein of Dwarf Fortress / RimWorld /
Crusader Kings — corporate espionage, buried secrets, and mutants under the
regolith (very much *Total Recall*). This is an early scaffold: colonists dig
out an underground colony on their own while burrowing aliens hunt them.

## Running

```sh
go run .
```

Terminal controls:

| Key            | Action                          |
| -------------- | ------------------------------- |
| `space`        | pause / resume                  |
| `+` / `-`      | faster / slower simulation      |
| `c`            | drop in another colonist        |
| `a`            | unleash another alien           |
| arrows / `hjkl`| pan the camera                  |
| `q` / `esc`    | quit                            |

Glyphs: 👷 colonist · 😱 fleeing colonist · 👽 alien · 🟫 rock · 🧱 wall · 🍽️ nutrient pod · 🚽 toilet · blank = open floor.

> The map uses one emoji per tile so it stays aligned. If it looks sheared, your
> terminal is sizing emoji as a single cell instead of two.

## Architecture

The design goal is a headless game engine that can drive **multiple frontends**.
The simulation and the renderer run in separate goroutines and never share
mutable state:

```
                 Commands (pause, speed, spawn)
   ┌────────────┐  ───────────────────────────▶  ┌──────────────┐
   │  Frontend  │                                 │    Engine    │
   │ (Bubbletea)│  ◀───────────────────────────   │  (sim loop)  │
   └────────────┘        Snapshots (per tick)      └──────────────┘
```

- **`internal/sim`** — the engine. `Engine.Run(ctx)` ticks the `World` on a timer
  in its own goroutine and is the *only* goroutine that touches game state.
  - `Engine.Subscribe()` hands a frontend a channel of immutable `Snapshot`s
    (deep copies). The channel has capacity 1 and stale frames are dropped, so a
    slow renderer can never stall the sim.
  - `Engine.Send(Command)` submits input (`TogglePause`, `SetTicksPerSecond`,
    `Spawn`) which is applied between ticks.
  - Any number of frontends can attach to one engine.
- **`internal/ui/tui`** — the first frontend, a [Bubble Tea](https://github.com/charmbracelet/bubbletea)
  model that is a pure consumer: it draws the latest snapshot and forwards keys
  as commands. A web or GUI frontend would implement the same consumer contract.

### Simulation model

- The world is a dense grid of `Tile`s (`Rock`, `Floor`, `Wall`). It starts as
  solid rock with a carved landing cavern (`internal/sim/worldgen.go`).
- Entities are one `Entity` struct interpreted by `Kind` (colonist / alien),
  rather than a strict ECS — pragmatic for a scaffold, and fields can graduate
  into real components as systems grow. Per-tick behavior lives in
  `systems.go`.
  - **Colonists** walk only on floor. They mine rock into floor, build walls and
    facilities along edges, tend to their needs, and flee when an alien gets
    close.
  - **Aliens** burrow through *any* terrain to reach the nearest colonist and
    eat it.

#### Needs

Colonists accumulate **needs** over time, stored as a `Needs` array indexed by
`NeedKind` (`internal/sim/needs.go`). Each need has a `NeedSpec` in `Config`
describing how fast it rises, when the colonist drops work to address it, which
facility satisfies it, and whether maxing out is fatal:

| Need    | Facility          | Fatal?                 |
| ------- | ----------------- | ---------------------- |
| food    | 🍽️ nutrient pod   | yes — starvation drains HP |
| bladder | 🚽 toilet         | no (nags only, for now)    |

When a need crosses its threshold the colonist walks to the nearest matching
facility and uses it, resetting the need. Facilities are ordinary buildable
structures (built from rock like walls — no material inventory yet). The colony
keeps enough life-support stocked for its population, and a hungry colonist with
nowhere to eat will build a pod rather than starve. Adding a new need is meant to
be a table edit: append a `NeedKind`, give it a `NeedSpec` and a facility.
- All tunables (world size, populations, HP, dig/build times, alien speed) live
  in `Config` (`config.go`). Runs are deterministic for a given `Seed`.

### Known scaffold limitations (next steps)

- Movement is greedy step-toward, not real pathfinding; colonists can get boxed
  in and simply pick a new task. A proper A\* pass is the obvious follow-up.
- A single underground level, no z-layers yet.
- No factions, needs, jobs queue, or inventory — hooks for those go through new
  systems in `sim` and new fields on `Entity`/`Tile`.

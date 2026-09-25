# Director

> Part of the [mars-sim documentation](./README.md).

## What it is

The director schedules major scripted occurrences — a mouse plague, an alien
swarm, a supply drop — for tick windows you define in `director.yaml`. It is
the "GM" layer on top of the ordinary simulation: worldgen and the ordinary
per-tick systems keep running as they always have, and the director just
injects a handful of extra happenings at chosen points in the run.

## Source

- [`internal/sim/director.go`](../internal/sim/director.go) — `Schedule`,
  `Occurrence`, `resolveSchedules`, `runDirector`, `LoadSchedules`.
- [`internal/sim/director_test.go`](../internal/sim/director_test.go) — parsing,
  determinism, and firing tests.
- [`main.go`](../main.go) — `-director`, `directorPathFromArgs`, `loadDirectorFile`.
- [`director.yaml.example`](../director.yaml.example) — a sample schedule file.

## How it works

A `director.yaml` document is a list of **schedules**:

```yaml
schedules:
  - name: "first winter"
    earliest_tick: 1000
    latest_tick: 1500
    occurrences:
      - kind: mouse-plague
        count: 25
      - kind: alien-swarm
        count: 5
      - kind: supply-drop
        pistols: 2
        shotguns: 1
```

Each `Schedule` has two parts, kept deliberately separate:

- **The window** (`earliest_tick`..`latest_tick`): *when* something happens.
- **The candidates** (`occurrences`): *what* might happen.

That split is the point: "something happens between tick 1000 and 1500" is one
decision, and "which of these three things" is another. A schedule can hold
just one occurrence (that thing happens, at an unpredictable tick in the
window) or several (one of them happens, and which one is also
unpredictable).

At world creation, `resolveSchedules` rolls every schedule down to one
concrete `scheduledEvent`: it picks one occurrence with flat (uniform) odds
across the list, and a tick with flat odds across `[earliest_tick,
latest_tick]`. This is the "easiest version": no ramping probability curve,
no per-tick chance roll — one dice roll per schedule, once, at startup. The
resolved queue is sorted by tick, and `World.step` fires whatever has come due
(`runDirector`) after the rest of the tick's work, each tick.

Resolution draws from `World.rng`, the simulation stream (not `prng`, the
flavor stream — see [personality.md](./personality.md)), because which
occurrence fires and when is gameplay: the same seed must produce the same
director script every run.

### Occurrence kinds

| Kind | Fields | Effect |
| --- | --- | --- |
| `mouse-plague` | `count` | Spawns `count` mice on open floor, same placement as starting mice. |
| `alien-swarm` | `count` | Spawns `count` aliens with the same placement as starting aliens: dormant on hidden cave floor, or colony floor far from the landing site when there is no cave room (see [caverns.md](./caverns.md#aliens-in-the-caves)). |
| `supply-drop` | `pistols`, `shotguns` | Hands weapons to that many distinct living colonists (in random order), same distribution as the colony ship's starting equipment (`equipColonyShip`, [combat.md](./combat.md)). |

A colonist with a full inventory is skipped rather than blocking the drop; an
item that cannot be placed on anyone is simply lost. A wave that runs out of
open floor or far rock stops early rather than looping forever.

### Loading the file

`director.yaml` loads the same way `mars-sim.yaml` does — before flags are
registered, so `-director PATH` can point elsewhere and `-director ""` can
turn it off:

| Flag | Behavior |
| --- | --- |
| *(none)* | Read `director.yaml` from the working directory if it exists. |
| `-director PATH` | Read that file instead. A file named here that does not exist is an error. |
| `-director ""` | Read no file at all — the director never fires. |

Unlike `mars-sim.yaml`, there is no generated template: a schedule is content
you write, not a tunable with a sensible default, so there is nothing to
print. An absent file is the default and entirely normal — most runs have no
script.

## Why it is this way

- **Two-file split.** `mars-sim.yaml` is entirely scalar knobs, driven by
  reflection over `cfg`-tagged `Config` fields ([config-file.md](./config-file.md)).
  A list of schedules with nested, kind-dependent occurrence data does not fit
  that shape, and forcing it in would have complicated the knob system for
  every other tunable. A second, independent file keeps both simple.
  `Config.Schedules` itself is an ordinary (untagged) field, loaded by
  `main.go` the same way `Config.Seed` is a special case in `configfile.go`.
- **Resolved once, up front, not rolled per tick.** Rolling a flat chance
  every tick inside the window would also reach a flat distribution over many
  runs, but not *within* one run: the exact firing tick would depend on how
  many other RNG draws happened first, coupling it to unrelated simulation
  activity in a way that is hard to reason about. Resolving immediately off
  a fixed number of `rng` calls keeps the whole director script — and the
  RNG draws consumed by everything after it — a pure function of the seed.
- **Flat probability, deliberately.** The brief for this first version was
  the simplest possible thing: pick a tick uniformly in the window, pick a
  candidate uniformly from the list. A shaped distribution (front-loaded,
  weighted candidates, escalating over a run) is a natural next step but
  adds a curve to choose and justify; nothing here forecloses it.
- **No ground-drop entity for supply crates.** The simulation has no notion
  of an item lying on the floor waiting to be picked up — inventory only
  exists on colonists and storage containers. Handing weapons straight to
  colonists, the same way the colony ship's starting equipment already does,
  reuses an existing, tested mechanic instead of inventing a new one for one
  occurrence kind.

## Extending it

- **A new occurrence kind** is: a new `OccurrenceKind` constant + `String()`
  case, a `parseOccurrenceKind` case, a validation case in
  `rawSchedule.toSchedule`, a `fire<Kind>` method, and a case in
  `fireOccurrence`. Add whatever `Occurrence` fields it needs; unused fields on
  other kinds already sit at zero.
- **A shaped distribution** (instead of flat) is a change to `resolveSchedules`
  alone — the `Schedule`/`Occurrence` types and the firing side do not need to
  know how the tick or candidate was chosen.
- **Recurring or conditional schedules** (fire again every N ticks, or only if
  some world condition holds) are out of scope for this version — today a
  schedule fires exactly once. That would mean giving `Schedule` a repeat or
  trigger field and teaching `runDirector` to re-arm rather than only advance.

## Related

- [config-file.md](./config-file.md) — `mars-sim.yaml`, the tunable-knobs file
  this one deliberately does not try to be.
- [architecture.md](./architecture.md) — the tick loop `runDirector` hooks into.
- [combat.md](./combat.md) — aliens, weapons, and `equipColonyShip`, which
  `supply-drop` mirrors.
- [personality.md](./personality.md) — why gameplay RNG and flavor RNG are
  separate streams.
- [lore.md](./lore.md) — the rolled alien species every `alien-swarm`
  occurrence's aliens belong to, and its own dedicated RNG stream.

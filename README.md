# mars-sim

A Mars-colonization simulation game, in the vein of Dwarf Fortress / RimWorld /
Crusader Kings — corporate espionage, buried secrets, and mutants under the
regolith (very much *Total Recall*). This is an early scaffold: colonists dig
out an underground colony on their own while burrowing aliens hunt them.

## Running

```sh
go run .
```

Every simulation tunable is a command-line flag (world size, populations, speed,
colonist/alien stats, build times, ...), each defaulting to the value in
`sim.DefaultConfig`. List them with `-h` or `?`:

```sh
go run . -h
go run . -colonists 20 -aliens 5 -width 120 -height 60
go run . -headless -duration 10s -seed 42   # reproducible, no TUI
```

For settings you want to keep rather than retype, [`mars-sim.yaml`](mars-sim.yaml)
is a committed file that sits between the compiled defaults and the flags. It
ships with every setting shown at its default and commented out, so it changes
nothing until you uncomment a line:

```yaml
## starting number of colonists
colonists: 20
```

Flags still override the file for a single run, and `-config PATH` picks a
different one. See the [settings file guide](docs/config-file.md).

Terminal controls:

| Key            | Action                          |
| -------------- | ------------------------------- |
| `space`        | pause / resume                  |
| `+` / `-`      | faster / slower simulation      |
| `c`            | drop in another colonist        |
| `a`            | unleash another alien           |
| `x`            | add another cat                 |
| `m`            | add another mouse               |
| arrows / `hjkl`| pan the camera                  |
| `tab`          | toggle the colonist roster      |
| `q` / `esc`    | quit                            |

The **roster** (`tab`) lists every colonist; `↑`/`↓` select one to inspect its
name, attributes, health, mood, needs, eight-slot inventory, traits, family,
affinities, and everything it remembers. The inspector is taller than the
panel, so `shift+↑`/`shift+↓` scroll it a line and `pgup`/`pgdn` a screenful —
the bottom row says where in the colonist you are. `tab` or `esc` returns to
the map.

The command also supports `-headless` for periodic stats without a TUI,
`-duration` for bounded runs, `-seed` for reproducibility, and `-glyphs` to
choose between emoji and ASCII map symbols. See the
[command-line guide](docs/cli.md) for application flags, validation, and
examples.

Glyphs: 👷 colonist · 😱 fleeing colonist · 💬 talking colonist · 🥾 stomping
colonist · 🧹 cleaning colonist · 📦 hauling colonist · 🧟 mutant colonist ·
👽 alien · 🐈 cat · 🐁 mouse · 🟥 ordinary rock · ⬛ iron-bearing rock ·
🟦 water ice-bearing rock · 🟩 uranium-bearing rock · 🟧 clay-bearing rock ·
🧱 wall · 🥫 nutrient pod · 🚽 toilet · 🛌 dormitory bunk · 🔥 incinerator ·
🩸 gore · 🦴 a body · blank = open floor.

The map is fogged. You see the landing cavern and the rim of rock the colony has
dug up to; everything past it is a faintly shaded blank, and an alien burrowing
toward you through it stays hidden until it breaks through. Mining peels the fog
back one tile at a time, and a tile once seen stays seen. Run with
`-fog-of-war=false` to show the whole map — see
[fog of war](docs/fog-of-war.md).

### Documentation

Contributor-facing system documentation lives in [`docs/`](docs/README.md).
Start with [architecture](docs/architecture.md), and add or update a Markdown
write-up in that directory whenever you add a feature or subsystem.

> The map allocates two terminal cells per tile and fits every glyph to that
> width, so it stays readable without emitting expensive cursor-position
> sequences for every tile. Because terminals disagree about how wide an emoji
> is, the glyphs are drawn from a vetted registry and measured against the real
> terminal at startup, with an ASCII fallback when they do not line up — see
> [terminal cell widths](docs/terminal-cell-widths.md).

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
  composition-bearing solid rock with a carved landing cavern
  (`internal/sim/worldgen.go`).
- Entities are one `Entity` struct interpreted by `Kind` (colonist / alien),
  rather than a strict ECS — pragmatic for a scaffold, and fields can graduate
  into real components as systems grow. Per-tick behavior lives in
  `systems.go`.
  - **Colonists** walk only on floor. They mine rock into floor (carrying one raw
    rock per excavated tile plus iron ore, water ice, or uranium ore from a
    bearing deposit),
    build the colony's life-support as coordinated projects (see *Construction
    projects*), clean up after the colony's dead (see *Sanitation*), tend to
    their needs, and flee when an alien gets close. Each colonist has eight
    inventory slots, each holding a homogeneous stack of up to 64 items.
  - **Aliens** burrow through *any* terrain to reach the nearest colonist and
    eat it.
  - **Cats** stalk the floor hunting mice, pouncing when adjacent (a single
    pounce is fatal). They have no needs; they hunt by instinct.
  - **Mice** are pests that scurry the floor and nibble the colony's nutrient
    pods, sharing the colonists' food need but hungering far faster. They flee
    cats, and the colony keeps them in check (see *Wildlife*).

#### Wildlife

Cats and mice form a small ecosystem on the cavern floor, and the colonists take
part in it:

- **Colonists stomp mice.** A colonist with nothing pressing to do — no alien to
  flee, no urgent need, and no reachable work — will chase down a mouse it notices
  (within `ColonistStompRadius`) and crush it. A stomp is instantly fatal. Pest
  control is strictly an idle whim: a threat, an urgent need, or any available
  job always wins, so stomping never pulls a colonist off real work.
- **Mice breed.** Two adjacent mice of opposite sex with nothing pressing to do
  mate; the female then carries a litter for `MouseGestationTicks` before giving
  birth to `MouseLitterMin`..`MouseLitterMax` pups on nearby floor. A newborn
  cannot breed until it matures (`MouseMaturityTicks`), and a female waits out
  `MouseBreedCooldown` before her next litter, so a warren grows but does not
  explode every tick. Cats, colonists' boots, and starvation without reachable
  food all push back the other way.

#### Needs

Colonists accumulate **needs** over time, stored as a `Needs` array indexed by
`NeedKind` (`internal/sim/needs.go`). Each need has a `NeedSpec` in `Config`
describing how fast it rises, when the colonist drops work to address it, which
facility satisfies it, and whether maxing out is fatal:

| Need    | Facility          | Fatal?                 |
| ------- | ----------------- | ---------------------- |
| food    | 🍽️ nutrient pod   | yes — starvation drains HP |
| bladder | 🚽 toilet         | no (nags only, for now)    |
| sleep   | 🛏️ dormitory bunk | no — a tired colonist waits for a free bunk |

When a need crosses its threshold the colonist walks to the nearest matching
facility and uses it, resetting the need. Facilities are ordinary buildable
structures (no material inventory yet). The colony keeps enough life-support
stocked for its population (`ColonistsPerFacility`), and a hungry colonist with
nowhere to eat will build a pod rather than starve (only *fatal* needs justify
that lone emergency build; a non-fatal need like bladder waits for a real
facility rather than having the whole colony storm into ad-hoc building at once).
Adding a new need is meant to be a table edit: append a `NeedKind`, give it a
`NeedSpec` and a facility.

#### Personality

Every colonist has a **profile** (`internal/sim/personality.go`): a name,
populated attributes (gender, orientation, height, weight), and any
**traits**. Attributes are flavor for now — nothing simulates against them yet —
but traits change how a colonist plays:

| Trait        | Effect                                    |
| ------------ | ----------------------------------------- |
| Big Eater    | hungers faster (food need rises quicker)  |
| Light Eater  | hungers slower                            |
| Industrious  | works faster and rests less               |
| Lazy         | works slower and rests more               |
| Asocial      | never develops a social need              |
| Introvert    | social need rises slowly; too much talking lowers mood |
| Extrovert    | social need rises quickly                 |
| Tidy         | the sight of gore hits morale harder      |
| Mutant-Lover | warms to mutants far faster than to anyone else |
| Mutant       | *not rolled at spawn* — what uranium does to a colonist |

Traits are drawn from mutually exclusive groups (appetite, work ethic, social); a
colonist gets at most one per group, each with `TraitChance` probability
(`-trait-chance`, default 30; 0 disables traits). At spawn a colonist's traits
resolve into per-colonist effective parameters (need rise rates, rest duration,
work speed) that the hot paths read directly, so traits never cost a per-tick
trait scan. Adding a trait is a table edit in `traitSpecs`; new needs and systems
will bring traits that suit them.

Personality is generated from a **separate RNG stream** so adding flavor never
perturbs the simulation's own RNG — with traits disabled a run plays exactly as
it did before personalities existed.

#### Relationships & affinities

Colonists are related and get to know each other (`internal/sim/relationships.go`):

- **Family.** A new colonist may be born into the colony's family tree
  (`-family-chance`, default 35%): tied to an existing colonist as a spouse,
  sibling, parent/child, aunt/uncle-nibling, or grandparent/grandchild. The
  ground truth is a small tree of parent and marriage links — the wider ties
  (sibling, aunt/uncle, grandparent, ...) are *derived* from it, so they stay
  mutually consistent however the colony grows, and ancestors who never joined
  the colony live on as phantom tree nodes that connect real colonists. Spouses
  are only paired when their orientations and genders are mutually compatible;
  a pairing involving a non-binary colonist isn't something an orientation
  label resolves on its own, so those are decided with a coin flip instead —
  anybody might marry an enby. Family is generated from the same separate RNG
  stream as personality, so it never perturbs the sim.
- **Heredity.** A family is visible at a glance, not just in the tree. A
  colonist born into one takes its **surname** (the earliest-arrived member of
  the line sets it; someone who marries in keeps their own name half the time,
  `-spouse-surname-chance`), and **inherits appearance** from their closest
  relatives feature by feature (`-appearance-inherit-chance`, default 75%) — so
  a child can have one parent's hair and the other's skin. What passes down is
  the *natural* form of a feature: a grandmother gone white passes on the brown
  she had, and a tall father gives a daughter who is tall for a woman rather
  than his own height. Relatives also **start out warm** rather than as
  strangers, scaled by how close the tie is (`-family-affinity`, default 55% of
  `-affinity-max`; `-family-affinity-spread` keeps cousins from all being
  equally close). See [docs/heredity.md](./docs/heredity.md).
- **Talking.** Colonists have a non-fatal **social need** that rises over time.
  Before looking for ordinary work, a colonist whose social need reaches its
  threshold seeks a nearby free colonist and must complete a conversation (the
  new **Talking** activity) to satisfy it. Needs and fleeing preempt a chat, and
  colonists never hold one on a facility's access tile or a pending build tile.
  Colonists may also talk opportunistically while idle; `-talk-chance` (default
  25%) controls that behavior, but does not suppress conversations required by
  an urgent social need.
- **Affinity.** Each pair of colonists has an **affinity** in
  `[-AffinityMax, AffinityMax]` (warmth to dislike). Talking is mostly a
  diminishing-returns positive-feedback loop: a conversation's quality leans
  toward the valence of the pair's existing affinity, so friends tend to grow
  closer and rivals to drift apart, with each step shrinking as affinity nears
  the extreme — talking alone saturates at half of `AffinityMax`, leaving the
  outer range for stronger forces added later. Random spread means any pair can
  still have a surprisingly good or bad chat. Affinity is tracked and displayed
  only; nothing simulates against it yet.
- **Mood.** Each colonist carries a **mood** in `[-MoodMax, MoodMax]` (0
  neutral). Nothing simulates against mood yet, but tasks move it — a finished
  conversation shifts both participants by a *company* term (how they feel about
  the other, from affinity) plus a *conversation* term (how the chat itself
  went, from quality). So a good chat with someone you dislike lifts your mood,
  a so-so chat with a friend still nets a small lift, and only a genuinely bad
  chat with a friend turns it negative.

Family ties, affinities, and mood are all shown per colonist in the roster
inspector.

#### Uranium & mutation

The regolith holds uranium as well as iron and water ice, and it is the one
deposit that acts back on the colonist who digs it. Standing beside an
unexcavated uranium vein — or carrying the ore, which with no way to drop
anything yet means carrying it forever — puts a colonist under a cumulative
**dose**. Every `-uranium-exposure-ticks` (100) of it is one roll at
`-mutation-chance` (25%) to **mutate**: grow a body part nobody is born with (a
third arm, an extra eye, a tail, a vestigial twin) and carry the **Mutant**
trait from then on, drawn as 🧟 on the map.

A grown part is extra flesh, not redistributed flesh: it adds its own HP and
becomes one more place an attack can land, which also thins the odds that any
one hit finds the head or torso. Mutants are, physically, slightly harder to
kill. The cost is social — mutating is a hard mood hit, and so is watching it
happen — except to a **Mutant-Lover**, who is delighted by both and warms to
mutants far faster than to anyone else. That last part is why affinity is
tracked per direction: a mutant-lover's regard is not returned in kind. Full
write-up in [docs/mutation.md](docs/mutation.md).

#### Sanitation

Violence leaves a mark. A messy kill splatters 🩸 gore across the tile, and a
death nothing eats — a starvation, a gunned-down alien, a stomped mouse —
leaves a 🦴 body lying where it fell. Both are **refuse**, and a colonist with
nothing urgent to do cleans it up: scrub the tile (🧹), carry the load to an
incinerator (📦), and burn it.

The catch is that there has to be somewhere to burn it. A colonist never picks
refuse up with no incinerator in reach — that would just hide the mess in an
inventory slot — so instead the colony plans itself a **trash room**: the same
walled shell as any other room, with a 🔥 incinerator in it. It queues one
automatically the first time there is refuse to burn (after life support and
bunks, which are what colonists die without), or on demand with `b` then `t`.

Cleaning is *work*, not an idle whim like stomping mice: the mining frontier
never runs out, so a chore that only happened when a colonist had nothing to do
would never happen at all. It sits between construction and mining in the
work-seeking order — a colony builds its life support first, then tidies up,
then goes back to digging. A `Tidy` colonist, who takes the sight of gore
hardest, ranges twice as far to find a mess and takes the most satisfaction in
burning it. Full details in [docs/sanitation.md](docs/sanitation.md).

#### Construction projects

The colony builds structures as **projects** it plans as a group rather than one
colonist at a time (`internal/sim/project.go`). A project is a set of tile
designations (`buildTask`s); any number of colonists each claim and build
individual tasks, so a room goes up collaboratively and in parallel. It is a
general coordination backbone — rooms are the first project kind, and storage,
workshops, and the like would be new task generators over the same machinery.

Every project kind today is a **room**: a bay of facilities in a rock-backed
niche at the cavern edge, inside a **complete placed-wall perimeter with a
one-tile front doorway** — nutrient pods and toilets in a facility room, bunks
in a dormitory, an incinerator in a trash room. Construction is **phased** — every wall
is raised (phase 0) before any facility comes online (phase 1) — and facilities
are spaced one tile apart so each keeps several access tiles. Only one room is
built at a time, so most colonists keep mining while a small crew finishes it.

This shape was learned by watching colonies starve around earlier designs (an
enclosed room trapping its own builders; a wall tile next to a facility that can
never be built because a user stands on it). The full rationale — including why an
earlier *wall-less* design was abandoned once colonists could pass through crowds
and route around pending build tiles — lives in
[docs/construction.md](docs/construction.md).

Rooms come in three recipes over that shared shell (`roomRecipe`), differing
only in what they line up along the back and how few facilities still make a
room worth building: a **facility room** alternates 🍽️ pods and 🚽 toilets for
the food and bladder needs, a **dormitory** is a bay of 🛏️ bunks for sleep, and
a **trash room** holds the 🔥 incinerator that refuse is burned in (see
*Sanitation*). The colony plans life support before bunks (food is fatal; a
missing bed only makes a colonist wait) and both before a trash room, and adding
a room kind is a recipe plus a demand check in `planRooms`.

One room is built at a time so most colonists keep mining (growing the cavern)
while a small crew finishes the current room. A fully mined-out map is the one
known soft spot: with nothing left to dig, the whole idle population mobs the few
facilities and a colonist or two can occasionally be crowded out over a long
run — a shared-facility crowd-flow limit, not a room-building one, and moot once
maps are larger than the colony can exhaust or colonists have other work.

- All tunables (world size, populations, HP, dig/build times, alien speed) live
  in `Config` (`config.go`). Runs are deterministic for a given `Seed`.

### Performance

The engine is being built to stay cheap as the colony scales. Two foundations
are in place:

- A dense **occupancy index** (`World.occ`) makes "who is on this tile?" an O(1)
  lookup instead of scanning every entity, and enforces one entity per tile.
- **Incrementally-maintained counts** (`terrainCounts`, `kindCounts`) answer
  "how many of X?" without rescanning the grid or entity set.

Together with allocation-free tile scans, these took a stress run from seconds
per tick to milliseconds:

| Colonists | Before  | After   |
| --------- | ------- | ------- |
| 500       | ~2.5 s  | ~8 ms   |
| 2000      | ~42 s   | ~144 ms |

Measure it yourself:

```sh
go test ./internal/sim/ -run '^$' -bench BenchmarkStep -benchmem
```

In place now:

- A chunk-based entity spatial index (neighbor queries scan nearby chunks, not
  every entity).
- A two-level region/room system (floor grouped into per-chunk regions, then
  rooms as connected components of the region graph), maintained incrementally in
  ~microseconds per terrain change.
- An event bus (`WorldEvent`/`TileChanged`) and a job board: the mineable
  frontier is tracked incrementally from tile events, so colonists claim the
  nearest reachable mine job instead of scanning the map, and in-progress builds
  are counted in O(1).
- Lazy needs and resting AI: needs are stored as a base level plus a timestamp
  and computed on read, so a colonist stays on its task until the task finishes
  or a need crosses its threshold (whichever comes first), and an idle colonist
  with no available work rests instead of re-scanning the map every tick.
- A* pathfinding with cached routes: colonists navigate the floor grid with A*
  (8-connected, room-gated for O(1) reachability) and follow the computed route
  one step per tick, so they route around walls instead of wedging against them,
  and the search runs once per job rather than every tick.
- Flow fields for shared destinations: one shared distance field per hot
  destination — facility kinds (nutrient pods, toilets) and the mining frontier
  — rebuilt lazily with a single multi-source BFS (generation-stamped, so a
  rebuild is O(reachable), not O(map)) and followed by every seeker in O(1) per
  tick. Miners follow the frontier field to the digging edge and claim a rock on
  arrival, so no per-miner path search is needed. This is the "everyone
  navigates the same" model: a 300x300 map with 3000 colonists runs ~10 ms/tick,
  about the same as 2000 colonists on a quarter of the map. The shared frontier
  sweep only pays off with enough miners, so mining switches strategy by
  threshold (`FrontierFieldMinColonists` / `FrontierFieldMinArea`): small
  colonies on small maps use cached A* to a claimed tile instead, checked
  dynamically as the colony grows.

Cumulative effect of the reactive work, on a 2000-colonist stress tick:

| Stage                         | ms/tick |
| ----------------------------- | ------- |
| Baseline (naive scans)        | ~42000  |
| + occupancy index & counts    | ~144    |
| + chunk index, rooms, board   | ~43     |
| + lazy needs & resting AI     | ~13     |

- Hierarchical A* (HPA*): long point-to-point trips first route over the region
  graph (region.links) to get a corridor of regions, then run the tile A*
  constrained to that corridor (painted into a generation-stamped cell mask for
  O(1) membership). This bounds tile exploration to the abstract route instead of
  the whole reachable area — on a map where a wall forces a long detour, a cross-
  fort search dropped ~1.0 ms -> ~0.28 ms (~3.6x, 4.5x fewer cells explored).
  Short/same-region trips still use the flat (optimal) search.

Still to come: z-levels (a multi-floor world) as its own track — the flow-field,
region, and HPA* machinery are built to extend into it. HPA* corridors are also a
natural thing to cache and share across agents, a further step.

### Known scaffold limitations (next steps)

- Movement is greedy step-toward, not real pathfinding; colonists can get boxed
  in and simply pick a new task. A proper A\* pass is the obvious follow-up.
- A single underground level, no z-layers yet.
- No factions, needs, jobs queue, or inventory — hooks for those go through new
  systems in `sim` and new fields on `Entity`/`Tile`.

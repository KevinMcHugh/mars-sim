# mars-sim documentation

This directory is the shared memory of the project. Every system has a write-up
here so a new contributor — human or agent — can understand *why* the code is the
way it is without re-deriving it from the source each time.

> **New feature? Write a doc.** See the project rule in
> [`../AGENTS.md`](../AGENTS.md). Start from [`TEMPLATE.md`](./TEMPLATE.md) and
> add your doc to the index below.

## Index

| Doc | What it covers |
| --- | --- |
| [architecture.md](./architecture.md) | The engine/frontend split, the tick loop, snapshots, commands, and the event bus. Start here. |
| [design-principles.md](./design-principles.md) | The game-design rules of thumb: enums over ints, detail without noise, flavor that never changes the simulation, explainable choices, deaths that come from the story, and building for big, long-running colonies. |
| [cli.md](./cli.md) | The `mars-sim` command, application modes, duration/seed controls, validation, and CLI examples. |
| [configuration.md](./configuration.md) | `sim.Config`, `DefaultConfig`, and how tunables become command-line flags. |
| [config-file.md](./config-file.md) | `mars-sim.yaml`: the committed settings file between the compiled defaults and the flags, and the struct tags that generate it. |
| [director.md](./director.md) | `director.yaml`: scheduling major occurrences (mouse plagues, alien swarms, supply drops) into tick windows, separate from what fires in them. |
| [world.md](./world.md) | The tile grid, terrain kinds, world state, and world generation. |
| [caverns.md](./caverns.md) | Natural caverns and the passages between them: generated hidden, ignored by the colony until a dig breaks in, then revealed all at once. |
| [lore.md](./lore.md) | The world beyond the colony: the alien species roster each seed rolls (build, name, temperament — friendly/cautious/hostile), and how damage and pace scale from it. |
| [entities-and-ai.md](./entities-and-ai.md) | The entity model, the per-tick systems, turn order, movement primitives, and the colonist / alien / cat / mouse behaviors. |
| [combat.md](./combat.md) | Per-body-part HP, weapons (pistol/shotgun), the colony ship's starting equipment, and gore. |
| [needs.md](./needs.md) | Colonist (and mouse) needs: hunger, bladder, lazy evaluation, starvation, and how to add a need. |
| [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) | Implementation design for independent need/affect processes feeding a weighted, explainable focus transition system. |
| [mutation.md](./mutation.md) | Uranium deposits, the exposure dose, mutant body parts, stature (two-foot to ten-foot colonists), and the Mutant / Mutant-Lover traits. |
| [determinism.md](./determinism.md) | One seed, one simulation: how map iteration order breaks it, the lockstep regression test, and the three bugs that motivated both. |
| [personality.md](./personality.md) | Names, attributes, traits, and the separate RNG stream that keeps flavor out of the simulation. |
| [ages-and-family.md](./ages-and-family.md) | Colonist ages and the age invariants family ties have to satisfy. |
| [heredity.md](./heredity.md) | What joining a family does to a colonist: a shared surname, inherited looks, and a warm start with relatives. |
| [construction.md](./construction.md) | Construction projects and the facility-room design, including the deadlocks that shaped it. |
| [escape.md](./escape.md) | What a colonist does when its room ends up sealed off from the colony: detecting the cutoff and breaking back out. |
| [sanitation.md](./sanitation.md) | Cleaning up gore and corpses, hauling refuse, the incinerator, and the trash room. |
| [pathfinding.md](./pathfinding.md) | Rooms/regions, tile A\*, shared flow fields, and hierarchical (HPA\*) routing. |
| [spatial-index-and-performance.md](./spatial-index-and-performance.md) | The occupancy index, incremental counts, chunk index, the job board, and the performance story. |
| [inventory.md](./inventory.md) | Colonist inventory slots and stacking. |
| [storage.md](./storage.md) | Placeable storage containers, their capacity, construction, and snapshot state. |
| [affect.md](./affect.md) | Charge/grip/valence affect, the impact-weighted push/pull blend, fresh/worn wear, event appraisal, trait transforms, decay, labels, and focus contributions. |
| [mood-space.md](./mood-space.md) | **Proposal.** What affect still lacks: tag-based trait rules and per-colonist baselines — plus the build plan and a tuning sandbox. |
| [memories.md](./memories.md) | Colonist memories and life events: notable experiences, sightings, affect vectors, stimuli, and snapshot exposure. |
| [sparse-grids.md](./sparse-grids.md) | `pagedGrid[T]`: why the flow fields, A\* scratch, region labels and occupancy index allocate with the colony instead of the map, and the fast paths that made it free. |
| [snapshot-tile-grid.md](./snapshot-tile-grid.md) | The terrain a frontend reads: an immutable, page-shared grid so publishing a frame costs what the tick touched, not the map's area. |
| [fog-of-war.md](./fog-of-war.md) | What the colony has seen: the explored flag, how digging lifts the fog, and how the map draws the unknown. |
| [frontend-tui.md](./frontend-tui.md) | The Bubble Tea terminal frontend: map, roster, controls, and glyphs. |
| [terminal-cell-widths.md](./terminal-cell-widths.md) | Why the grid used to shear: cell-accurate measurement, the vetted glyph registry, the startup width probe, and the ASCII fallback. |

## How the docs are organized

Each doc is self-contained and follows [`TEMPLATE.md`](./TEMPLATE.md):

- **What it is** — a one-paragraph summary.
- **Source** — the files that implement it.
- **How it works** — the model and the key decisions.
- **Why it is this way** — the tradeoffs and the dead ends we already hit.
- **Extending it** — the intended path for the next change.
- **Related** — links to adjacent docs.

The [top-level README](../README.md) is the player- and newcomer-facing tour; the
docs here are the contributor-facing depth behind it.

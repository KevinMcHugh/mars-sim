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
| [game-settings.md](./game-settings.md) | The browser Game tab's read-only list of every setting the running game uses, beside its default: the `config` topic built from `sim.Knobs`, and why the Snapshot shares the World's `Config`. |
| [director.md](./director.md) | `director.yaml`: scheduling major occurrences (rat plagues, alien swarms, supply drops) into tick windows, separate from what fires in them. |
| [world.md](./world.md) | The tile grid, terrain kinds, world state, and world generation. |
| [worldgen-chunks.md](./worldgen-chunks.md) | Chunked, lazy world generation: ore veins, caverns and passages as a pure function of seed and chunk, generated only as the colony explores (never because a frontend looked), why the old whole-map generator could not be made lazy, and how far abundance drifts from its targets. |
| [salt.md](./salt.md) | Salt deposits: 3% of the map, laid down on rock by worldgen, never on a scum tile, never regenerating. World state only so far; nothing gathers or draws it. |
| [caverns.md](./caverns.md) | Natural caverns and the passages between them: generated hidden, ignored by the colony until a dig breaks in, then revealed all at once. Breaking in can turn up an alien nest. |
| [layers.md](./layers.md) | Levels as stacked layers: every `Point` carries its `Level` (0 surface, 1 landing, deeper below), per-level state on a `Layer`, how code finds a point's layer, `layered[T]` scratch, what a frontend sees, and what one-level games pay for it (and why `Level` is an `int`). |
| [stairs.md](./stairs.md) | The way between levels: `StairDown`/`StairUp` and `links`, rooms, A\*, HPA\* and flow fields crossing a stair, `travelEstimate` for "nearest" across levels, digging a stair (`deepest-level`, the planner, `OrderStair`), per-level worldgen, and switching levels in the terminal. |
| [shafts.md](./shafts.md) | The slow way between levels: a laddered column through many levels (`ShaftTop`/`ShaftMid`/`ShaftBottom`), dug in one job by order (`OrderShaft`), weighted links (edge costs in A\*, Dial's buckets in the flow fields), the `Climbing` state, laden climbs, which aliens can climb, and why a full load slows a climb rather than forbidding it. |
| [holes.md](./holes.md) | The one-way way down: `Hole` (not walkable, not a link), digging one (`OrderHole`), falling and leg-first fall damage, a cornered colonist leaping, holes as refuse chutes (`hole-chutes`), and ladders (`OrderLadder`, and automatically for a colonist stranded below) turning a hole into a shaft. |
| [z-levels.md](./z-levels.md) | **Proposal**, phases Z0 to Z3 shipped. Underground levels as stacked 2D layers joined by stairs, shafts (slow, two-way) and holes (one-way drops); cross-level rooms, A\* and flow fields, lazy per-level generation, and depth as the lever for bigger challenges and rewards. Colony ships land on level 1; level 0 is the surface. Records where the build departed from the plan. |
| [lore.md](./lore.md) | The world beyond the colony: the alien species roster each seed rolls (build, a name unique in the roster drawn from condition-gated names and name groups, temperament — friendly/cautious/hostile), how damage and pace scale from it, the peaceful species' scum grazing, and what the hostile ones hunt. |
| [arms-makers.md](./arms-makers.md) | Lore corporations: the companies each seed rolls, the make and model ("MarsCorp M-117") every gun kind carries, corporations as conversation lore, and colonists' flavor-only former employers. |
| [alien-lifecycles.md](./alien-lifecycles.md) | Alien lifecycles and graded anatomy: some species live as several forms (egg, grub, pupa, joey; queen/worker/drone, bull/betty castes), each bigger and more extreme than the last but recognizably the same; most have one form; horns, antlers, quills, shells, claws, stingers and tail clubs in degrees; eggs that are no threat, young that graze, growth logged as hatching, molting and emerging; the egg that never hatched. |
| [alien-taxonomy.md](./alien-taxonomy.md) | Scientific names for alien species (*Pseudursus ares*): a genus built from a Greek/Latin prefix and root and an epithet, each drawn from condition-gated word parts that fit the build, on their own RNG stream so no roster re-rolls, with epithets that never need gender agreement. |
| [entities-and-ai.md](./entities-and-ai.md) | The entity model, the per-tick systems, turn order, movement primitives, and the colonist / alien / cat / rat behaviors. |
| [species-and-behaviors.md](./species-and-behaviors.md) | **Phases 1–5 and 4b of 7 built; 6–7 optional.** Toward an ECS: the species table every kind's identity and stats come from, cats/rats/chickens run as behavior ladders by `animalTurn`, rat breeding and pets as optional components on `Entity`, each rolled alien species as a species with a temperament ladder, prey and threat as tags (cats hunt vermin, rats flee mousers), and what remains optional: relationships split into tag-based rules and runtime edges, alien species as generated values, proposed alien lifecycles (egg to imago, castes, joeys; mostly single-form; each stage bigger and more extreme) with graded anatomy (horns, antlers, stingers, quills, tail clubs, shells), and a phased, fingerprint-preserving migration. No modeling language, no ECS library. |
| [combat.md](./combat.md) | Per-body-part HP, weapons (pistol/shotgun), the guns every settler lands with, and gore. |
| [drives.md](./drives.md) | Colonist (and rat) drives: hunger, bladder, company, sleep. Growth as a base rate times stacked modifiers (drive activity, traits, other drives' bands, timed effects like caffeine), consequences as level bands with a ramp shorthand, true vs felt levels, lazy evaluation, starvation, facilities, and how to add a drive or a drive activity. |
| [days.md](./days.md) | Colony days and time of day (derived from the sleep drive, shown in the top bar and TUI header), and the eight-hour night: the sleep drive falling in bed until it is met, sleep traits, other drives all but stopped in bed, interrupted nights. |
| [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) | Implementation design for independent need/affect processes feeding a weighted, explainable focus transition system. |
| [mutation.md](./mutation.md) | Uranium deposits, the exposure dose, mutant body parts, stature (two-foot to ten-foot colonists), and the Mutant / Mutant-Lover traits. |
| [determinism.md](./determinism.md) | One seed, one simulation: how map iteration order breaks it, the lockstep regression test, and the three bugs that motivated both. |
| [personality.md](./personality.md) | Names, attributes, traits, and the separate RNG stream that keeps flavor out of the simulation. |
| [rng-streams.md](./rng-streams.md) | Every seed-derived `math/rand/v2` PCG stream, how they are seeded, and how their state is saved and restored for save/load. |
| [save-load.md](./save-load.md) | Save files: the whole World (caches and RNG streams included) through a reflection codec that keeps shared objects shared, the commit and layout hash in the header, why saves don't load across layout changes, the test that a loaded game plays on byte-for-byte, and saving and loading from the CLI, the TUI and the browser. |
| [ages-and-family.md](./ages-and-family.md) | Colonist ages and the age invariants family ties have to satisfy. |
| [heredity.md](./heredity.md) | What joining a family does to a colonist: a shared surname, inherited looks, and a warm start with relatives. |
| [construction.md](./construction.md) | Construction projects and the facility-room design: rooms facing any way, standing free, never cutting the colony in two, and the deadlocks that shaped it. |
| [room-expansion.md](./room-expansion.md) | Rooms as walled rectangles holding any fixtures of one zone: the layout rule for where a fixture may stand, fitting fixtures into free floor, joining rooms side by side, back to back or facing each other, growing through any wall (a doorway moving with it), moving fixtures with their goods and links, emptying small rooms into bigger ones and clearing the shells, waiting for a room going up, the narrow fallback for the first, and the measurements behind it. |
| [escape.md](./escape.md) | What happens when part of the colony ends up cut off: a sealed-in colonist digs or breaks its way out, and a cut-off facility gets a passage dug to it. |
| [sanitation.md](./sanitation.md) | Cleaning up gore and corpses, hauling refuse, the incinerator, and the trash room. |
| [pathfinding.md](./pathfinding.md) | Rooms/regions, tile A\*, shared flow fields, and hierarchical (HPA\*) routing. |
| [spatial-index-and-performance.md](./spatial-index-and-performance.md) | The occupancy index, incremental counts, chunk index, the job board, and the performance story. |
| [inventory.md](./inventory.md) | Colonist inventory slots and stacking. |
| [storage.md](./storage.md) | Placeable storage containers, their capacity, construction, and snapshot state. |
| [affect.md](./affect.md) | Charge/grip/valence affect, the impact-weighted push/pull blend, fresh/worn wear, grammar-matched trait rules, per-colonist baselines, decay, labels, and focus contributions. |
| [compositional-perception-and-events.md](./compositional-perception-and-events.md) | Occurrence/percept/reaction grammar: who notices a world fact, how they react, wear policies, and why tags were dropped. |
| [scum-lab.md](./scum-lab.md) | Scum Lab: the shared shell, Focus Tester, Grammar Builder, Sprite Designer, and how to add a tool. |
| [sprite-designer.md](./sprite-designer.md) | Scum Lab's Sprite Designer: iterating on an SVG map sprite with the Claude API from the browser, tile-size previews, SVG/PNG export, and what the map needs before it can use one. |
| [wasm.md](./wasm.md) | The browser module Scum Lab calls, why importing `internal/sim` pulls in more than the focus functions, and the `internal/mind` split that fixes it. |
| [economy.md](./economy.md) | **Proposal.** Scarcity and economy: property and owners, crash-pod arrivals, dollars, recipes and slurry, the bid/ask book for goods and labor, and the phased build plan. |
| [skills.md](./skills.md) | Skills and professions: practice in base work ticks, a few labelled ranks per skill on a per-skill log curve, skills rolled at arrival on their own RNG stream, faster work and larger yields that pay more per rank on steeper curves, and profession by standing. Proposed next: opportunity cost in the producer planner, competition for bids, and workshops of one's own. |
| [work-market.md](./work-market.md) | **Proposal.** Communal work on the order book: removing the community ladder, every communal job as a paid order, one chooser ranking work and market plans by rate, colonists offering labor at skill-adjusted prices, and the colony stepping back (no standing ore bids, no reselling, no workshops of its own), with measurements of what dropping the ore bids does. |
| [labor.md](./labor.md) | Work orders: the colony buying its public works from the treasury, commissions and houses, paid (pay-per-use) fixtures, and the colony's cook. Economy phase E5. |
| [zoning.md](./zoning.md) | Zones and fixture zones: residence, storage and production painted per tile, every fixture tagged with its zone (a chest by its role) and a room zoned by its fixtures, building only inside a matching zone (or zoning-auto), colony ships' ground held as residence, the paid digging and clearing a paint implies, the clear tool, and how a cleared wall reroutes colonists. |
| [excavation.md](./excavation.md) | The player ordering an area mined out: the dig tool on the map, the `OrderExcavation` command, an excavation project whose tiles are `WorkDig` orders on the order book escrowed from the treasury, cancelling one, and why it is all or nothing. |
| [recruiting.md](./recruiting.md) | Hiring colonists from off-world: paying a recruiter from the treasury for a set of candidate cards (looks, skills, traits, savings), hiring any of them or re-rolling, recruits appearing beside a ship, the fee and passage leaving the money supply, log-normal savings, and the recruit RNG stream. |
| [colony-orders.md](./colony-orders.md) | The player trading for the colony from the Market tab: placing, repricing and removing the colony's bids and asks, the `standing-orders-build-only` default (the colony posts only its building-material bids), the `manual` flag that keeps upkeep from undoing them, suspending a standing order (by side and item) so it stops coming back, and colony-wide orders with no depot ("buy 100 meals at $10, sell at $5") that upkeep places where the good changes hands. |
| [hauling.md](./hauling.md) | Moving goods between depots: arbitrage on a colonist's own account, hauling for hire, the colony keeping meals at its silo, selling back what it bought, and building public works from its own stock. Economy phase E7. |
| [valuation.md](./valuation.md) | Prices that trades move, hungry colonists' bids, and the producer planner: filling bids at a profit and bidding for the inputs, so demand reaches down the recipe chain. The `-econ-trace` tuning harness. Economy phase E6. |
| [order-detail.md](./order-detail.md) | The Market tab's order detail: who posted an open bid or ask, its terms, what has filled and with whom, its escrow, and when it opened on the colony clock; the fills each order records, why a reprice keeps them, and the `book:` and `order:` topics. |
| [pricing.md](./pricing.md) | Price discovery, held until the colony's kitchens run: unsold food asks come down (a meal's no lower than its scum's cost), colonists offer idle food, hungry colonists' bids start low and rise toward what their hunger and money will pay, and cooks sell at what meals fetch. Measurements, and why survival didn't move (every starvation death is a colonist who passed out first). |
| [market.md](./market.md) | The order book: bids and asks at a depot, price-time matching, escrow, the colony's silo and paid prospecting, and buying and selling meals. Economy phase E4. |
| [money.md](./money.md) | Dollars: wallets, the treasury, the one `transfer` funnel, the fixed money supply and how it is audited, and the market tab. Economy phase E0. |
| [ships.md](./ships.md) | How every colonist arrives: colony ships of up to 20 with communal bunks and toilets and a private locker each, the rooms behind one hull as a stick, a hub and spoke, or a knobby cluster, the manifest, where ships land and why, landing the founders one by one in the browser, and what they replaced (one crash pod per colonist). Economy phase E2. |
| [meeting-hall.md](./meeting-hall.md) | The meeting hall: a room of chairs the colony commissions, where colonists walk to socialize and to eat, why a hall is just its chairs, and the fallbacks that keep a full or far hall from stranding anyone. |
| [food.md](./food.md) | Meals as items: eating your own, then buying one, then the safety net's gruel (off by default); `infinite-food`; what changes with it off. Economy phase E2. |
| [foraging.md](./foraging.md) | What a hungry colonist with no food does: one plan seen through (the drop-work/re-take-work loop it replaced), scraping to keep, and prospecting into rock nobody has seen for the scum there. The colony prospects too, once it is short. |
| [incubator.md](./incubator.md) | The scum incubator: seeded with wild scum, grows it on a schedule, harvested for the stoves; wild scraping is gated to dire times. |
| [scumhouse.md](./scumhouse.md) | Food production: the scumhouse, data-driven recipes, cave scum (seeding, accretion, exposure), and food work. Economy phase E3. |
| [chickens.md](./chickens.md) | Chickens: the one rare item a settler lands with (a gun, a chicken, or a cat), the keeper's trough, feed mixed from scum at a scumhouse, hens grazing scum when the trough is dry, why keepers tend before construction and share the stove, and why cats and chickens ignore each other. |
| [foundry.md](./foundry.md) | The first non-food supply chain: iron ore to steel ingots at a forge, steel to assault rifles at a gun bench, the foundry room, the colony's armory bid, and the planner fixes a three-link chain needed. |
| [property.md](./property.md) | Who owns what: fixture records with owners and access (communal/private), per-chest ledgers of whose goods are inside, and how private fixtures route. Economy phase E1. |
| [mood-space.md](./mood-space.md) | What is still open after wear, trait rules, and baselines shipped — plus the rejected tag design. |
| [memories.md](./memories.md) | Colonist memories and compositional events: notable experiences, sightings, affect vectors, stimuli, and snapshot exposure. |
| [conversation-topics.md](./conversation-topics.md) | What a conversation is about: a memory, another colonist (gossip that carries the speaker's opinion), or lore through the `LoreItem` interface (today alien species, later history), and the topic RNG stream. |
| [flow-field-view.md](./flow-field-view.md) | The map's flow-field overlay (`f`), in the TUI and the browser: opting a field into the snapshot with `ShowFlowField`, the sparse, version-cached copy, why freshening a field to show it keeps runs deterministic, the frame's view-limited flow section, and the shared colour ramp. |
| [sparse-grids.md](./sparse-grids.md) | `pagedGrid[T]`: why the flow fields, A\* scratch, region labels and occupancy index allocate with the colony instead of the map, and the fast paths that made it free. |
| [snapshot-tile-grid.md](./snapshot-tile-grid.md) | The terrain a frontend reads: an immutable, page-shared grid so publishing a frame costs what the tick touched, not the map's area, plus the opt-in live mode that skips the copy for a same-goroutine frontend. |
| [fog-of-war.md](./fog-of-war.md) | What the colony has seen: the explored flag, how digging lifts the fog, and how the map draws the unknown. |
| [population-screen.md](./population-screen.md) | The Population tab: colonists, meals in storage, colony size, and fixtures charted over the whole game, sampled on the simulation clock into a history that halves its resolution to stay whole. |
| [activity-screen.md](./activity-screen.md) | The Activity tab: every colonist-tick tallied under one activity (cooking, fleeing, fighting, …) and whether it was spent walking there, carried on the Population history, and drawn as a stacked half-block area chart. |
| [charts.md](./charts.md) | The general-purpose chart system: every countable thing as a metric (a row in one table), read per colony, good, account, fixture kind or skill rank, Levels and running Totals, hourly samples on the colony clock so half-day and day buckets are exact, per-series history that halves through 1, 2, 4, 12, 24 hours, the catalog and per-series topics, and the browser's Custom charts. |
| [perf-screen.md](./perf-screen.md) | The Perf tab: how the engine times each tick into quarter-second samples, and the gping-style braille charts of tick rate and tick cost. |
| [browser-frontend.md](./browser-frontend.md) | The browser build's plan and state: the engine as WASM in a Web Worker behind a Svelte + WebGL UI, `Engine.Advance`, what the spike measured, big worlds, save/load, hosting, and the plan for parity with the TUI. |
| [wire-format.md](./wire-format.md) | The browser build's messages: the JSON Hello, the binary frame layout (stats, entities, tile pages, refuse, scum), which pages go when, panel topics, and the golden frames both decoders are tested against. |
| [frontend-web.md](./frontend-web.md) | The browser game: the Svelte chrome (speed selector, side panel), and the map: WebGL2 terrain from chunk textures, the emoji atlas and when glyphs replace flat colors, filth (gore, scum) as tile tints, instanced entity sprites, the camera and the view it streams, and how to run `web/`. |
| [floating-panels.md](./floating-panels.md) | The browser's pop-out tabs: any side-panel tab as a draggable, resizable window over the map (the Market beside a chart), remembered per browser; why a tab is docked or floating but never both, and how the inspector's selection follows it. |
| [frontend-tui.md](./frontend-tui.md) | The Bubble Tea terminal frontend: map, roster, controls, and glyphs. |
| [colonist-looks.md](./colonist-looks.md) | The browser draws each colonist in their own skin tone and hair: Go sends a candidate list per look (👨🏿‍🦰, 👨🏿, 👨‍🦰, 👨) and the page measures and draws the first its emoji font fuses into one glyph. |
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

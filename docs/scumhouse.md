# Scumhouse, recipes, and cave scum

> Part of the [mars-sim documentation](./README.md).

## What it is

The colony's first food production. A **scumhouse** turns biomatter — cave
scum scraped off the rock, viscera scrubbed off the floor, and every body but a
colonist's — into meals of slurry, by a data table of **recipes**. **Cave
scum** is a biofilm seeded across the rock at worldgen that regrows after it is
scraped: the renewable base of the food chain. Colonists gather biomatter and
cook it as community work, so the meals belong to the colony and anyone may eat
them. This is phase **E3** of the [economy plan](./economy.md), with the
`construction-costs` switch documented in [construction.md](./construction.md).

## Source

- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) — `Recipe`,
  `SkillKind`, the `recipes` table; `scumPatch`, `scumAt`, `takeScum`,
  `growScum`, the exposure index; `foodWanted`, `tryAssignFoodWork`,
  `tryAssignCraft`/`jobCraft`, `tryAssignScrape`/`jobScrape`,
  `deliverBiomatter`, `carriedOwner`.
- [`internal/sim/cleaning.go`](../internal/sim/cleaning.go) — hauling
  biomatter to the scumhouse (see [sanitation.md](./sanitation.md)).
- [`internal/sim/project.go`](../internal/sim/project.go) — `scumhouseRoom` and
  its place in `planRooms`.
- [`internal/sim/world.go`](../internal/sim/world.go) — the `Scumhouse`
  terrain, its depot, the scum maps, and the exposure subscription.
- [`internal/sim/food.go`](../internal/sim/food.go) — a hungry colonist with
  nothing to eat does food work first.
- [`internal/sim/scumhouse_test.go`](../internal/sim/scumhouse_test.go).

## How it works

### The scumhouse

`Scumhouse` is a fixture terrain with a depot: a storage container on its tile
(`hasDepot`) holding its inputs and its meals, with a ledger like any chest.
It is not a chest, though — `chooseStorage` only unloads general materials into
`Storage` containers — and it is communal, owned by the colony.

The planner builds one in a walled room (`scumhouseRoom`, one scumhouse):

- with `infinite-food` off, **first**, before any other room — food is fatal,
  and it is the only place food comes from; crash-pod meals buy the time;
- otherwise only when ordered (`b` then `h` in the TUI, `OrderScumhouse`).

### Recipes

A `Recipe` consumes `Inputs` from a workshop's depot, takes `Ticks` of labor
there (scaled by `workScale`), and puts `Outputs` in the same depot. **Whoever
owns the inputs owns the outputs**; the workshop's owner does not. `Skill` is a
placeholder (`SkillNone`) so the table's shape survives skills arriving.

| Recipe | In | Out | Ticks |
| --- | --- | --- | --- |
| render an alien carcass | 1 alien carcass | 4 meals | 30 |
| render an animal carcass | 1 animal carcass | 1 meal | 10 |
| press viscera | 2 viscera | 1 meal | 10 |
| culture cave scum | 2 cave scum | 1 meal | 12 |

A cook works the first recipe (in table order) it has inputs for — its own, or
the colony's — at the nearest reachable scumhouse no other cook has claimed
(`workshopClaims`: one cook per workshop). It checks the outputs will fit once
the inputs are gone, and at the end `debit`s the inputs and `credit`s the
outputs to the same owner, so the depot's ledger always balances.

### Cave scum

Worldgen seeds `scum-percent` of the map's rock with patches in short
meandering runs (`growScum`, on its own seed-derived stream so it moved no
ore vein of an established seed). A patch holds up to `scum-max` units. It is
**lazy**, like a need: `scumAt` is the stored amount plus a unit for every
`scum-regrow-ticks` since it was last scraped, capped, with no per-tick work.

A patch can be scraped while it is **exposed**: on walkable floor, or on rock
with walkable floor beside it — floor the colony has **discovered**. The rim
of a natural cavern nobody has broken into is not exposed; `discoverCavernTile`
re-checks it the moment the cavern is found (see [caverns.md](./caverns.md)).
Mining a scummy rock tile leaves the patch on
the new floor; building a structure on it destroys it (`SetTerrain` →
`clearScum`). `exposedScum` is kept in step from `TileChanged` events, the way
the job board keeps the mining frontier, so finding scum never walks the map.

### Food work

`foodWanted` is true while the colony has a scumhouse and owns fewer than
`meal-reserve` meals per colonist (`communityMeals`, memoized per tick —
crash-pod lockers make one depot per settler, and every work-seeking colonist
asks). While it is, `assignWorkJob` offers, after construction:

1. **cooking** (`JobCraft`) what the scumhouse already holds;
2. **cleaning**, which feeds the scumhouse too (see
   [sanitation.md](./sanitation.md));
3. **scraping** (`JobScrape`): walk to the nearest unclaimed exposed patch with
   scum on it, scrape a unit per `scrape-ticks` until the patch is bare or the
   load (`scum-max`) is full, and haul it to a scumhouse with room.

A hungry colonist with nothing to eat and no safety net does food work first
whenever it picks a new job, whatever the reserve says (`hungryWithoutFood`).

Scum and biomatter gathered this way is **the colony's**: the scraper's or
cleaner's cargo record (`Entity.cargo`) says so, and `deliverBiomatter` credits
it to the community. That is what makes the meals communal — the colony owned
the inputs.

| Setting | Default |
| --- | --- |
| `scum-percent` | 6 |
| `scum-max` | 3 |
| `scum-regrow-ticks` | 400 |
| `scrape-ticks` | 6 |
| `meal-reserve` | 3 per colonist |

## Why it is this way

- **Scum is the renewable base.** Bodies and viscera only come from deaths, and
  a colony that eats only its dead is waiting to starve. Scum regrows, and
  digging keeps exposing fresh patches, so a colony that keeps working keeps
  eating.
- **Tuned against a gate, not a guess.**
  `TestColonyFeedsItselfWithoutTheSafetyNet` lands six colonists with three
  meals each — about 1750 ticks of food — with the safety net off, and requires
  every one alive at tick 8000. With `scum-percent` at 0 the same colony starves
  on schedule. Before changing the scum settings or the recipes, run it.
- **Community work, community food.** Before the order book (E4) there is no
  way to pay anyone for scraping, so the scraper works for the colony and the
  colony owns the result. The recipe rule — inputs' owner owns the outputs —
  already handles the other case: a colonist who brings its own scum gets its
  own meals.
- **Lazy regrowth, indexed exposure.** Thousands of patches on a big map must
  cost nothing while nobody touches them. Same trick as needs and the mining
  frontier.
- **Only discovered floor exposes scum.** When hidden caverns arrived, their
  floor counted as "walkable floor beside it", so every patch on every hidden
  cavern's rim was exposed: 5,851 of 5,864 exposed patches on a 1000×1000 map.
  Scrapers searched them, rats smelled them, and every frame published them.
- **Published scum is cached.** `publishedScum` reuses the last published map
  until something writes to the scum (`scumRev`) or a published patch's lazy
  regrowth ticks up a unit (`snapScumUntil`, the earliest such tick). It was
  rebuilt every frame, and since the engine publishes every tick, a profile of
  a big map was three-quarters `publishedScum`. `TestPublishedScumIsNeverStale`
  checks the cache against a fresh copy on every tick of a scraping colony.
- **A scumhouse is a depot, not a chest.** Keeping ore out of it
  (`chooseStorage` checks the terrain) keeps its 48 slots for biomatter and
  meals, and keeps "where do I unload?" from sending a miner to the kitchen.

## Extending it

- **A new recipe** is a row in `recipes`. A new workshop is a fixture terrain
  with a depot (`hasDepot`), a room recipe, and `tryAssignCraft` learning to
  look at more than scumhouses — it already matches recipes to the depot's
  terrain.
- **Skills** plug in at `Recipe.Skill` and the tick scaling in `jobCraft`.
- **Paying for food work** (E5) replaces the community cargo record with a
  labor order: the colony, or anyone, posts pay for scum delivered.
- **Rats eat biomatter too.** A hungry rat eats bodies, gore, and exposed scum
  where they lie, before it would raid a pod, and claims nothing — so every
  unit a rat reaches first is food the scumhouse never sees. A rat plague is a
  famine risk with the safety net off; cats are the defense. See
  [entities-and-ai.md](./entities-and-ai.md).

## Related

- [food.md](./food.md) — eating the meals, and the safety net.
- [sanitation.md](./sanitation.md) — cleaning, and where each kind of refuse goes.
- [property.md](./property.md) — ledgers, `debit`/`credit`, cargo records.
- [construction.md](./construction.md) — the room machinery, and construction costs.
- [world.md](./world.md) — worldgen.
- [economy.md](./economy.md) — the plan this is phase E3 of.

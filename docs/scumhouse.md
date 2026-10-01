# Scumhouse, recipes, and cave scum

> Part of the [mars-sim documentation](./README.md).

## What it is

The colony's first food production. A **scumhouse** turns biomatter — cave
scum scraped off the rock, viscera scrubbed off the floor, and every body but a
colonist's — into meals of slurry, by a data table of **recipes**. **Cave
scum** is a biofilm seeded across the rock at worldgen that then **accretes**:
it spawns at a low fixed rate anywhere and spreads from patches already there,
the renewable base of the food chain. The colony runs its scumhouses
as a business: it keeps standing bids for biomatter, so scrapers and cleaners
sell it what they bring in; it pays a cook to work it; and it **sells** the
meals. Nothing it cooks is free for the taking. This is phase **E3** of the
[economy plan](./economy.md), made a trade in E7, with the
`construction-costs` switch documented in [construction.md](./construction.md).

## Source

- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) — `Recipe`,
  the `recipes` table; `scumPatch`, `scumAt`, `takeScum`, the
  exposure index; `foodWanted`, `tryAssignFoodWork`,
  `tryAssignCraft`/`jobCraft`, `tryAssignScrape`/`jobScrape`; food on a
  colonist's own account (`foodPays`, `tryAssignScrapeToSell`,
  `offerOwnMeals`, `mealSellPrice`); the colony's meal price
  (`colonyMealPrice`, `storedMeals`);
  `deliverBiomatter`, `carriedOwner`; the colony's trade (`biomatterPrice`,
  `refreshBiomatterBids`, `sellBiomatter`, `refreshColonyMealAsks`).
- [`internal/sim/market.go`](../internal/sim/market.go) — `tryBuyMeal` buys
  at a scumhouse or the silo.
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

The planner builds one in a walled room, a **kitchen** (`scumhouseRoom`, see
[construction.md](./construction.md)):

- with `infinite-food` off (the default), **first**, before any other room —
  food is fatal, and it is the only place food comes from; crash-pod meals buy
  the time. If the treasury can't fund it, it is marked out anyway as unpaid
  community work;
- otherwise only when ordered (`b` then `h` in the TUI, `OrderScumhouse`).

### The kitchen is an assembly line

A kitchen is two fixtures a tile apart: the scumhouse (the stove, whose depot
holds the inputs) and a **pantry**, an ordinary `Storage` chest. Meals cooked
at the stove go straight into the pantry (`outputDepot`); every sale, queued
bid, and meal fetch happens there. The cook stands at the stove, so it never
stands between a hungry colonist and a meal.

`designateRoom` records the link when it marks the room out (`linkPantry`, in
`pantryOf` and `pantryHouse`), rather than inferring it later from which chest
is nearest: a silo or a locker could sit just as close. `pantryFor` checks the
chest is still there, and a scumhouse without one (a narrow one-fixture room,
or a pantry not yet built) keeps its meals in its own depot as before. A pantry
is a meal shelf and nothing else. `nearestStorage` and `marketDepot` skip it,
so ore is never unloaded there and it is never chosen as the silo. The TUI
labels it "pantry". `mealDepots` lists every place a meal can be: scumhouses,
their pantries, then the silo.

Before the pantry, meals stayed in the stove's own depot, and a cook at work
held a tile next to the depot everyone needed. The answer then was an aisle
and a rule that cooks give way (see below). Both are still there, but the
pantry is the fix: the stove and the shelf have separate access tiles.

A colonist cooking its own supper while hungry (`cooksOwnSupper`: its own
inputs, food pressing, room in its pockets) keeps one meal in hand instead
of walking round to the pantry for it. Without that, the extra walk starved a
penniless colonist on a small map.

### Recipes

A `Recipe` consumes `Inputs` from a workshop's depot, takes `Ticks` of labor
there (cut by the worker's rank in the recipe's `Skill`, then scaled by
`workScale`), and puts `Outputs` in the same depot, sometimes one more for a
skilled worker ([skills.md](./skills.md)). **Whoever owns the inputs owns the
outputs**; the workshop's owner does not.

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

Worldgen seeds `scum-percent` of the map with patches in short meandering
runs, chunk by chunk (`scumPlan` in `worldgen_chunks.go`, on its own
per-chunk stream so it moves no ore vein; see
[worldgen-chunks.md](./worldgen-chunks.md)). Each chunk places its share of
distinct tiles, so abundance lands within a fraction of a percent of the
target. Scum is laid down with the rest of a chunk, when exploration first
reaches it, and `applyChunk` registers any patch that is already exposed. A
patch holds up to `scum-max` units. This is only the **starting stock**: no
tile is a permanent source. A patch scraped to its last unit is deleted, and
the scum comes back the way it first arrived.

**Accretion** (`growScum`, once a tick). Each tick it visits a sample of the
tiles, on average one visit per tile per 16 ticks (`scumTrialDivisor`), and at
each visited tile:

- with chance `scum-spawn-ppm` (in millionths, default 20) scum appears there
  from nothing;
- otherwise it looks at one of the nine tiles in and around it, at random,
  and if that tile holds scum, adds a unit with chance `scum-spread-percent`
  (default 40).

So a tile's odds grow with the scum around it: with no scum near, only the
spawn chance applies; each neighbouring patch adds about `spread/9/16` per
tick, and a patch already there thickens the same way. New patches start at
one unit, appear only on `Rock` in a generated chunk (never on the colony's
floor, under a structure, or on a tile with a [salt](./salt.md) deposit), and stop once patches cover `scum-percent` of
the generated tiles, so scraping is what makes room. Left alone, scum
therefore creeps back toward the seeded density; a scraped bare patch beside
others fills in within a few hundred ticks, and an isolated one may not come
back for a long time.

That is the model. `growScum` doesn't walk the visits: almost all of them
change nothing, so it draws only the ones that roll, how many of each this
tick (`scumDraws`), and where each lands:

- **Spawns**: generated tiles × `scum-spawn-ppm` / 16 a tick, each on a
  uniform tile of the generated chunks (`scumDrawTile`).
- **Spreads**, turned around to start from the scum. A visit picks each of
  the nine tiles in and around it with chance 1/9, so every patch is picked
  by each of its nine tiles' visits at 1/16 × 1/9 a tick: the same as each
  patch sending a unit to one of its nine tiles, at random, at
  `scum-spread-percent` / 16 a tick. So it draws patches × spread / 16 rolls,
  each a patch from `scumPatches` and one of its nine tiles.
  `TestScumSpreadsAtItsRateOnAnyMap` passes under both forms.

The rate is the model's on any map, and the cost follows the scum, not the
map: on a 10,000×10,000 map with 36 chunks generated, about 220 spread draws
a tick. Draws are a hash of the seed, the tick and the draw index, not a
stream, so growth moves no other RNG. `scumPatches` is every patch sorted by
chunk, then row by row within the chunk (`cmpScumPatch`), the way `genChunks`
is sorted: which patch a draw picks depends on which patches exist, never on
the order they arrived in. A chunk's patches sit together, so `applyChunk`
adds them in one insert; `setScum` and `clearScum` keep the list in step
with the map one patch at a time. A test that puts scum down goes through
`setScum` (or `noScum` to clear it), never the map directly.

**Why not a fixed sample.** The first version visited a sample of tiles over
the whole map, skipped the ones in chunks not yet generated, capped the
sample at 4,096 a tick, and scaled the chances up to make the difference
good. Spawn's chance stayed small enough to scale. Spread's 40% passed 100%
on any map over about 405×405, and past that spread ran at 4,096 ×
generated / area samples a tick, a share of the model's rate that fell with
the map's size, not with anything on the ground: about 6 times too slow on
1000×1000 and 600 times on 10,000×10,000, for the same explored ground. On
the web game's 10,000×10,000 map, seed 1790737522337000000's colony scraped
every patch it could reach bare by tick 13,000, none came back, and all six
colonists starved by 14,750. The code from before accretion (patches
regrowing on their own tiles) never starved on that seed.
`TestScumSpawnsAtItsRateOnAnyMap` checks the rate on 256², 1000² and
10,000² maps.

Regrowth lands anywhere in the generated chunks, not where the colony can
reach it, so a colony that stays put scrapes its reachable scum bare. On
that seed the fixed rate held the colony past tick 16,000, then it starved
by 20,000. The answer is colonists going out to look for it, not scum that
favours the colony. Hungry colonists **forage**, and the colony
**prospects** when it is short: both dig into rock nobody has seen, where
the uneaten scum is (see [foraging.md](./foraging.md)).

A patch can be scraped while it is **exposed**: on walkable floor, or on rock
with walkable floor beside it — floor the colony has **discovered**. The rim
of a natural cavern nobody has broken into is not exposed; `discoverCavernTile`
re-checks it the moment the cavern is found (see [caverns.md](./caverns.md)).
Mining a scummy rock tile leaves the patch on
the new floor; building a structure on it destroys it (`SetTerrain` →
`clearScum`). `exposedScum` is kept in step from `TileChanged` events, the way
the job board keeps the mining frontier, so finding scum never walks the map.

### Food work

**Food on its own account comes first.** When a meal sells for enough more
than it costs a colonist to make (`foodPays`), `assignWorkJob` offers, right
after construction and before the colony's food work, cooking the colonist's
own scum (`tryAssignCraftFor`) or scraping scum to keep and cook
(`tryAssignScrapeToSell`). `foodPays` is the producer planner's test:

```
margin = mealSellPrice × meals − scum × its value − labor (scraping, cooking, the walk to the nearest scumhouse and back)
```

and it has to clear `plan-min-profit`. A meal a colonist cooks earns it the
meal's price, where the colony pays a dollar a unit of scum and a dollar a
recipe, so when food pays, a colonist does it for itself. `jobCraft` then
offers its meals beyond `meal-keep` for sale where they're made
(`offerOwnMeals`), at `mealSellPrice`: hungry colonists' bids queue at the
pantry, so the next of them buys it at once.

At the charter's $5 this pays only for colonists near a scumhouse. In a
6-colonist colony it never does, and seeds 1–48 play exactly as they would
without it. In a 100-colonist colony on a 300×150 map (seeds 1–4, 30,000
ticks) it starved 49 of 400 colonists, against 73 without, and two of the
four seeds lost nobody. The colony's own food chain runs on its treasury,
which building rooms for 100 colonists spends to $0 early on; food made on
colonists' own account doesn't wait for it.

`foodWanted` is true while the colony has a scumhouse and owns fewer than
`meal-reserve` meals per colonist (`communityMeals`, memoized per tick —
crash-pod lockers make one depot per settler, and every work-seeking colonist
asks). While it is, `assignWorkJob` offers, after construction and food on
a colonist's own account:

1. **cooking** (`JobCraft`) what the scumhouse already holds;
2. **cleaning**, which feeds the scumhouse too (see
   [sanitation.md](./sanitation.md));
3. **scraping** (`JobScrape`): walk to the nearest unclaimed exposed patch with
   scum on it, scrape a unit per `scrape-ticks` until the patch is bare or the
   load (`scum-max`) is full, and haul it to a scumhouse with room.
4. **prospecting**, when no exposed patch is left unclaimed: digging into
   rock nobody has seen, to expose more (`tryProspect`, see
   [foraging.md](./foraging.md)).

Scum and biomatter gathered this way is **the gatherer's own**.
`deliverBiomatter` puts it in the depot in its name, then `sellBiomatter`
offers it into the best bids there, which are normally the colony's. The
colony then owns it, and a cook working the colony's stock is paid
`wage-cook` per recipe by the colony (the recipe rule makes the colony own the
meal).

A hungry colonist with nothing to eat, no meal it can buy, and no safety net
forages (`hungryWithoutFood`, see [foraging.md](./foraging.md)). That food work
is for itself: it cooks its own scum, and it scrapes **to keep**
(`scrapeKeep`) rather than to sell, carrying a part load from patch to patch
until it has a meal's worth. So a colonist with no money can still feed
itself.

### The colony's trade

In the market's upkeep, the colony:

- **Buys**: `refreshBiomatterBids` keeps `scumhouse-bid-qty` units of
  standing bids at each scumhouse for each kind of biomatter, at
  `biomatterPrice`, as far as the treasury stretches and the depot has room,
  and only until the colony holds `scumhouse-stock-cap` units of that kind
  there. More than its cooks can get through soon is money spent on a pile.
  The prices follow the meals each input makes: two scum or two viscera to a
  $5 meal, four meals from an alien carcass.
- **Sells**: every meal the colony holds, at each scumhouse and at the silo,
  at `colonyMealPrice`, less any that a haul order is about to take to the
  silo. With `meal-price-max` above 100 the price rises as stored meals (every
  meal in storage, anyone's) fall short of `meal-reserve` per colonist, to
  that percent of `price-meal` with nothing stored; `refreshColonyMealAsks`
  reposts the colony's asks when it moves. It's off (100) by default: see
  *A scarcity price*.
  A cook's meal goes on sale in the pantry the moment it is made
  (`offerColonyMeals`, from `jobCraft`), and `refreshColonyMealAsks` sweeps up the rest each upkeep. The
  haul order withdraws the asks it needs first, since goods on offer are
  locked in escrow.

A hungry colonist buys from whichever reachable depot has the cheapest meal it
will pay for (`tryBuyMeal`). If none is on sale, its bid **queues at the
nearest kitchen's pantry**, so the next meal cooked there fills it at once, in
bid order. A meal on offer still counts toward the colony's
`meal-reserve` (`communityMeals`), so the colony doesn't keep cooking what
it has on the shelf.

### A scarcity price

A price that rises as stores fall is meant to ration the last meals toward
the hungriest (a colonist's bid rises with its hunger; see
[valuation.md](./valuation.md)) and make cooking to sell pay. `meal-price-max`
does that, and it's off by default because in every form measured it starved
more colonists, not fewer. 100 colonists on a 300×150 map, seeds 1–4, 30,000
ticks, starved of 400:

| | Starved |
| --- | --- |
| Fixed price, food on own account first (the default) | 49 |
| Without food on own account | 73 |
| `meal-price-max 300`, scarcity from the colony's stock | 273 |
| `meal-price-max 300`, scarcity from all stored meals | 127 |

Priced on the colony's own stock, the price starts at its maximum, because
the colony holds no meals at landing while the lockers hold 1,000. Every
colonist turns to cooking for itself, the colony never buys any scum, and
100 private cooks jam ten stoves. Priced on all stored meals, it starts at
$5 and rises as the lockers empty. The rising price moves money from wallets
to the treasury (in one run, wallets fell from $12,000 to $5,700 while the
treasury rose to $7,000), and the colony can't turn that money into food: its
scum bids are capped by quantity, not money. Its dearer meals also answer the
hungry colonists' resting bids that private producers used to fill, so less
food gets made. A useful scarcity price probably needs the colony not to be
the seller that captures it (see [work-market.md](./work-market.md)).

### Keeping a big colony fed

The planner wants a scumhouse for every `colonists-per-scumhouse` colonists
(`desiredScumhouses`), and one more whenever its kitchens are behind
(`kitchensBehind`: short of the meal reserve with, on average, half a
scumhouse's stock cap of biomatter waiting to be cooked), up to one per three
colonists. It adds that one only once every scumhouse it planned is built. A
chef's own kitchen isn't counted: the colony can't cook or buy scum there
(see *A workshop of one's own* in [skills.md](./skills.md)). A
kitchen per ten colonists is a guess at what a colony needs; kitchens that
are behind are a measurement of it.

Together with cooks staying at the stove and giving way only to someone at
the door (both below), this is what ended most big-colony die-offs: 100
colonists on a 300×150 map, seeds 1–8, 30,000 ticks, starved 46 of 800
(all on seed 2) where about 227 starved before, and 1 with pocket meals off.
Six-colonist colonies, seeds 1–32, were unchanged (2 starved).

Only the first is life support: it may be built unpaid
and in a narrow room, and it holds up every other room until it's planned.
Later ones are ordinary public works that need an aisle
(`roomRecipe.aisleRequired`). A few rules keep kitchens usable:

- **Scrape to sell only into a bid.** Scraping for money needs a buyer at the
  scumhouse (`tryAssignScrape`). Scraping to feed yourself doesn't.
- **Cook your own leftovers.** A colonist with scum of its own sitting in a
  scumhouse cooks it (`assignWorkJob`, after the producer planner), into
  meals it can eat or sell.
- **Cooks give way.** A cook doesn't start a recipe at a workshop when
  someone coming to fetch a meal from it is within `mealFetchRadius` (3)
  tiles (`mealFetchesAt`), so it steps off the counter instead of holding the
  only access tile; one arriving mid-recipe waits one recipe at most. With a
  pantry, nobody fetches from the stove, so this matters only for a kitchen
  without one, and cramped kitchens often have none. It counts only colonists
  at the door: counting everyone on their way from anywhere, in a
  100-colonist colony where six of ten kitchens had no pantry, kept stoves
  idle beside waiting scum, and 90% of that idle time was this rule.
- **Loiterers make way.** Someone idle, chatting, or eating a meal already in
  hand, on the one tile that reaches a depot, steps aside for a colonist who
  needs it (`nudgeLoiterer`, `makeWayAt`, from `travelTo`). A narrow silo
  room is a dead-end corridor one tile wide. Colonists fetched a meal there,
  stepped a tile back and ate it in the corridor, and the queue behind them
  starved with meals they had paid for three tiles away. Anyone working the
  tile, like a cook or a builder, keeps it. A cat or a rat always moves: a cat
  that settled on a narrow silo's one access tile starved eleven colonists
  queued behind it.
- **Cooks stay at the stove.** A cook starts the same recipe again rather
  than leaving (`cooksOn`), for as long as the stove holds the inputs, the
  cook isn't hungry and nobody is at the door for a meal; a colony cook also
  stops once the colony has its reserve. That covers a colonist cooking its
  own scum too. A cook who walks across the colony for one twelve-tick recipe
  and leaves keeps the stove waiting for the next: late in long runs, twenty
  colonists shared two stoves that stood idle most of the time while 238
  units of scum sat in the depots. Capped at six recipes, a 100-colonist
  colony's stoves were claimed by a cook who wasn't cooking 53% of the time
  and cooking 30%. A cook who stays is the division of labor the skills plan
  wants: scrapers bring the scum, and the cook gets better at cooking (see
  [skills.md](./skills.md)).

  Treating chests and scumhouses as facility access tiles, where nobody idles
  (`onFacilityAccess`), looked like the obvious fix and made things far worse:
  161 starved across the sweep instead of 2. `stepAside` and the chat-partner
  search avoid those tiles too, and every crash-pod row has a locker chest, so
  idle colonists ran out of places to stand.

With 40 colonists, a colony that kept one scumhouse lost 16 to 22 people to
starvation by tick 8000. That wasn't for lack of food: over a thousand units
of uncooked scum sat in its depot, the colony kept buying more, and meals
came out one at a time. Starving colonists had money and a price they would
pay; there was just never a meal on the counter when they looked, and their
standing bids were queued at the silo, not the kitchen. Across 10 seeds each
at 6, 20, and 40 colonists (10000 ticks, defaults), one colonist starved after
these changes, and that one was cornered by an alien. After the pantry and
loiterers making way, across 11 seeds at each size, one of 726 starved, again
while fleeing an alien.

This replaced a delivery bounty (a work order paid per unit brought in, with
the colony owning the result either way). Buying goods is the plan's own
instrument for this (see [economy.md](./economy.md)). It makes the colony a
producer that recovers its costs, and it makes scum an ordinary traded good
that the producer planner can see.

| Setting | Default |
| --- | --- |
| `scum-percent` | 6 |
| `scum-max` | 3 |
| `scum-spawn-ppm` | 20 |
| `scum-spread-percent` | 40 |
| `scrape-ticks` | 6 |
| `meal-reserve` | 3 per colonist |
| `colonists-per-scumhouse` | 10 |
| `scumhouse-stock-cap` | 40 per kind per scumhouse |
| `price-cave-scum` / `price-viscera` / `price-animal-corpse` / `price-alien-corpse` | 2 / 2 / 3 / 8 |
| `scumhouse-bid-qty` | 12 per kind per scumhouse |
| `wage-cook` | 1 per recipe |

## Why it is this way

- **Scum is the renewable base.** Bodies and viscera only come from deaths, and
  a colony that eats only its dead is waiting to starve. Scum accretes, and
  digging keeps exposing fresh patches, so a colony that keeps working keeps
  eating.
- **Tuned against a gate, not a guess.**
  `TestColonyFeedsItselfWithoutTheSafetyNet` lands six colonists with three
  meals each — about 1750 ticks of food — with the safety net off, and requires
  every one alive at tick 8000. With `scum-percent` at 0 the same colony starves
  on schedule. Before changing the scum settings or the recipes, run it.
- **The scumhouse charges.** At first the colony owned all gathered
  biomatter and anyone could eat its meals free, which was the only option
  before there was money. Once there was money, that left the colony spending
  on everything and earning on nothing, and it hid all food demand from the
  market. Now the colony buys its inputs, pays its cook, and sells its meals
  at about cost, so the treasury is not drained by feeding everyone, and a
  meal has a price a producer can undercut. The recipe rule (inputs' owner
  owns the outputs) still means a colonist who brings its own scum gets its
  own meals.
- **Broke is not starving.** Without the scrape-to-keep path, a colonist with
  no money and no safety net would starve beside a full scumhouse.
  `TestColonyFeedsItselfWithoutTheSafetyNet` still passes, and
  `TestAHungryScraperKeepsItsScum` pins the path.
- **Accretion, not a fixed source.** Scum used to regrow lazily on every seeded
  tile forever, so the map's scum was a fixed set of fountains and depleting one
  was never permanent. Now a patch is only scum, and it spreads: colonies that
  scrape everything near home have to go and find more, and unlucky spawns can
  seed new ground. Growth is sampled, not scheduled, because tests and worldgen
  write the scum map directly and a sampled tile needs no index to stay in step.
  Trials read one neighbour rather than counting eight, so a visit costs one
  lookup, not nine.
- **Indexed exposure.** Thousands of patches on a big map must cost nothing
  while nobody touches them. Same trick as the mining frontier.
- **Only discovered floor exposes scum.** When hidden caverns arrived, their
  floor counted as "walkable floor beside it", so every patch on every hidden
  cavern's rim was exposed: 5,851 of 5,864 exposed patches on a 1000×1000 map.
  Scrapers searched them, rats smelled them, and every frame published them.
- **Published scum is cached.** `publishedScum` reuses the last published map
  until something writes to an exposed patch (`scumRev`, which growth bumps
  too). It was rebuilt every frame, and since the engine publishes every tick, a
  profile of a big map was three-quarters `publishedScum`.
  `TestPublishedScumIsNeverStale` checks the cache against a fresh copy on every
  tick of a scraping colony with growth turned way up.
- **A scumhouse is a depot, not a chest.** Keeping ore out of it
  (`chooseStorage` checks the terrain) keeps its 48 slots for biomatter and
  meals, and keeps "where do I unload?" from sending a miner to the kitchen.

## Extending it

- **A new recipe** is a row in `recipes`. A new workshop is a fixture terrain
  that `isWorkshop` lists (which gives it a depot), a room recipe, and
  `trackFacility`. The forge and the gun bench are the worked example (see
  [foundry.md](./foundry.md)). The producer planner finds any workshop through
  `nearestWorkshop`. The colony's own cooking (`tryAssignCraft`) still looks
  only at scumhouses.
- **A recipe's skill** is `Recipe.Skill`; `jobCraft` applies its speed and yield
  and credits the practice (see [skills.md](./skills.md)).
- **Paying for food work** (E5) replaces the community cargo record with a
  labor order: the colony, or anyone, posts pay for scum delivered.
- **Rats eat biomatter too.** A hungry rat eats bodies, gore, and exposed scum
  where they lie, before it would raid a pod, and claims nothing — so every
  unit a rat reaches first is food the scumhouse never sees. A rat plague is a
  famine risk with the safety net off; cats are the defense. See
  [entities-and-ai.md](./entities-and-ai.md).
- **Peaceful aliens eat scum too.** A hungry Friendly or Cautious alien grazes
  exposed cave scum the same way (scum only, not bodies or gore). See
  [lore.md](./lore.md#what-peaceful-species-eat-cave-scum).

## Related

- [food.md](./food.md) — eating the meals, and the safety net.
- [sanitation.md](./sanitation.md) — cleaning, and where each kind of refuse goes.
- [property.md](./property.md) — ledgers, `debit`/`credit`, cargo records.
- [construction.md](./construction.md) — the room machinery, and construction costs.
- [world.md](./world.md) — worldgen.
- [economy.md](./economy.md) — the plan this is phase E3 of.

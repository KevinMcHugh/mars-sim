# The scum incubator

> Part of the [mars-sim documentation](./README.md).

## What it is

A **scum incubator** (🫙) is a facility colonists load scum into so it grows at a
steady rate, wherever the colony is, whatever the rock holds. It replaces
scraping scum off the rock as the colony's routine supply: stoves are stocked
from incubators, and **wild scraping is for seeding them and for dire times**.
It is the controllable half of the food chain in [scumhouse.md](./scumhouse.md):
the rock's accretion is random and runs out near home, an incubator's is a rate
you can plan on.

## Source

- [`internal/sim/incubator.go`](../internal/sim/incubator.go) — `incubatorRoom`,
  `desiredIncubators`, `growIncubators`, `wildScumAllowed`, seeding
  (`tryAssignSeed`) and harvesting (`tryAssignHarvest`, `jobHarvest`).
- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) — the gating in
  `tryAssignScrape`, `tryAssignScrapeToSell`, `refreshBiomatterBids`, and the
  `scrapeHarvest` stage of `jobScrape`; `deliverBiomatter` buys what is loaded.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — where the incubator
  sits in `assignWorkJob`; [`project.go`](../internal/sim/project.go) — where the
  room sits in `planRooms`; [`engine.go`](../internal/sim/engine.go) —
  `OrderIncubator` (`b` then `i` in the TUI).
- [`internal/sim/world.go`](../internal/sim/world.go) (`Incubator` terrain),
  [`property.go`](../internal/sim/property.go) (a fixture with a depot, but not a
  workshop), [`construction.go`](../internal/sim/construction.go) (2 raw rock, 1
  clay).
- [`internal/sim/incubator_test.go`](../internal/sim/incubator_test.go).

## How it works

### Growth

The incubator is a depot (a storage container on its tile with a ledger), like
a scumhouse, but it works no recipe. Every `incubator-grow-ticks` (15) it adds
one unit of cave scum, on the colony's account, as long as it holds at least its
seed (`incubator-seed`, 2) and fewer than `incubator-capacity` (12). There is no
chance in it: the rate is the point, and `growIncubators` draws no random
numbers, so it stays out of both RNG streams.

### Seeding

An incubator with less than its seed grows nothing. `tryAssignSeed` sends a
colonist to scrape a load of wild scum (`scrapeSeed`) and carry it to an
incubator short of its seed; one seeder per incubator (`workshopClaims`, with
`seedAt` to release it). **The colony buys what is loaded outright**, at the
price it pays for scum at a scumhouse, whether or not a bid is standing
(`deliverBiomatter`), so the incubator's stock is the colony's from the first
unit.

### Harvesting

What an incubator holds above its seed is **ripe** (`ripeScum`). While the
colony wants food (`foodWanted`), `tryAssignHarvest` sends a colonist to take up
to two loads of it (`harvestLoad`) and haul it to a colony kitchen with room,
under the kitchen's `scumhouse-stock-cap` so harvesters don't bury a stove.
The scum is the colony's the whole way (cargo owner `Community`) and the
colony pays `wage-harvest` for the trip. Harvesting is another stage
(`scrapeHarvest`) of `JobScrape`, so it reuses its claim, haul and delivery
code.

### When wild scum is allowed

`wildScumAllowed` is the one switch:

| Situation | Wild scraping for the colony |
| --- | --- |
| No incubator built (or `incubator-grow-ticks` 0) | allowed: nothing else to eat from |
| An incubator stands, stores are fine | **no** |
| An incubator stands, stores below `scum-dire-meals` a colonist, nothing ripe, no scum waiting at a stove | allowed: dire |

It gates scraping for the colony's stoves (`tryAssignScrape` when not keeping),
scraping to sell on a colonist's own account, prospecting for food, and the
colony's standing scum bid at its scumhouses. While it is off,
`refreshBiomatterBids` also withdraws the colony's open standing scum bids,
so their escrow returns to the treasury.

It does **not** gate:
- seeding;
- a hungry colonist foraging for itself (`planForage`, `scrapeKeep`): that
  colonist is already dire;
- planner gather plans (`planGather`). Scraping for somebody's bid goes on
  whatever the incubators do, so a player's order for scum at a scumhouse
  (see [colony-orders.md](./colony-orders.md)) and a chef's bid at its own
  kitchen are still filled.

Stores means every meal in every depot (`storedMeals`), not the colony's own:
at landing the colony owns no meals while colonists' lockers hold ten each, and
counting only the colony's made every moment of the first thousand ticks dire.

A colonist left carrying scum (a part load) takes it to a stove to cook for
itself rather than carrying it forever.

### Building it

`incubatorRoom` is one or two incubators with an aisle. The colony's first
may be narrow if the cavern is cramped; later ones wait for a site with an
aisle. The planner wants `ceil(colonists / colonists-per-incubator)` of them
(`desiredIncubators`, 4 a head by default), right after the first scumhouse,
while pods do not feed anyone (`!podsFeed`). It is an ordinary public work: if
the treasury can't fund it the planner moves on, and the colony scrapes as it
always did until one stands. With `infinite-food` on it is player-ordered only.
Either way, more incubators go into a production room the colony already has
(a kitchen with floor to spare will do: both are production), fitted into
free floor, or by joining two rooms or growing one, before a new room is
marked out, and no new one is marked out while one is still going up (see
[room-expansion.md](./room-expansion.md)).

## Why it is this way

- **A gate, not a ban.** An incubator needs a seed, and a colony without one
  must still eat, so wild scum is never forbidden outright. The colony scrapes
  until the first incubator stands, scrapes to seed it, and falls back on the
  rock if the incubators fail.
- **Buying the seed, not bidding for it.** The first version posted a colony
  bid at each incubator and let loaders sell into it. Whatever a colonist
  loaded after the bid filled stayed its own, stuck in the incubator where
  nobody could cook or harvest it. Paying on delivery has no such leak.
- **Leftover scum.** The first gating also stopped colonists delivering the
  part loads they were carrying (there was no scumhouse bid left to sell
  into), and harvesters refuse anyone whose pack has scum. Six colonists each
  holding a few units starved beside two full incubators. Carried scum now goes
  to a stove, kept.
- **The colony stops asking; colonists can still answer.** The gate first
  covered gather plans too, so with an incubator standing nobody would
  scrape for any bid. A player's order for 25 scum then sat unfilled, with
  nothing to say why. What the incubator replaces is the colony's own demand
  for wild scum, so the gate is on that demand: its standing bid is withdrawn
  and not reposted, and a bid anyone else posts is answered. Withdrawing
  matters, because the colony's standing orders never expire, and an old bid
  left open would have kept colonists scraping for it. (Under
  `standing-orders-build-only`, the default, the colony posts no scum bid in
  the first place.) Chefs' bids (`refreshChefBids`) and cooks' derived bids
  aren't gated either, so wild scum reaches chefs' kitchens and the meal
  chain from a hungry colonist's bid down to the rock works again. Over
  20,000 ticks on seeds 1–8 with the defaults, scum went from never trading
  to trading on 6 seeds (23–281 units). On 4 of them its price rose to
  $21–34, which is cooks paying their whole margin to fill hungry colonists'
  meal bids. Starvation was unchanged (5 deaths before and after).
- **Dire needs a measure of "nothing is coming".** Short stores alone would
  fire constantly while a kitchen catches up, so dire also needs nothing ripe
  and nothing waiting at a stove.
- **A latent bug this exposed.** `TestWithoutTheSafetyNetTheColonyStarvesOnSchedule`
  failed with incubators on: a colonist with no food kept being handed the same
  dig each tick and starved before the manifest ran out. It reproduces on the
  base commit for some seeds (seed 8 starves at tick 392 in that setup), so it
  predates the incubator; the test now turns incubators off, since the colony
  in it produces nothing. The fix belongs in the hungry-colonist path
  ([food.md](./food.md)).

## Extending it

- **Tuning:** `incubator-grow-ticks`, `incubator-capacity`, `incubator-seed`,
  `colonists-per-incubator`, `scum-dire-meals`, `wage-harvest`. Run
  `TestColonyFeedsItselfWithoutTheSafetyNet` and
  `TestTheColonyBuildsIncubatorsAndFeedsFromThem` after changing them.
- **Other cultures** (viscera, say) would make the incubator a recipe-driven
  workshop; today it is a depot with one hard-coded product.

## Related

- [scumhouse.md](./scumhouse.md) — the stoves it feeds, and cave scum in the wild.
- [foraging.md](./foraging.md) — what a hungry colonist does instead.
- [construction.md](./construction.md) — the room machinery.

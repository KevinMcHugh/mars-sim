# Power (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan for giving the colony an energy supply, and the colony's first real
**political choice** about it. Power comes from a **reactor** that burns a
little uranium and a lot of water ice, and it moves around the colony as a
good: **batteries**, charged at the reactor and spent at the stove. The ships
land with enough batteries to cook through the build-out. After that the colony
needs a reactor, and the player decides who gets to own one, and can change
their mind mid-game:

- **Monopoly.** The colony is the power company. Only it builds reactors, it
  buys the uranium and ice, and it sells batteries at a **tariff** the player
  sets.
- **Market.** The colony stays out of it. Any colonist rich enough can
  commission a reactor, own it, and sell batteries for whatever the book will
  pay. The first one to do it is the colony's power baron, at least until a
  second one can afford to compete.
- **Charter.** The colony grants an exclusive franchise to one of the seed's
  corporations. The company town: the charter builds and runs the reactors,
  sets its own price, pays the colony a royalty, and sends the rest of its
  profit off-world.
- **Public option.** A market, plus one reactor the colony runs itself at the
  tariff, built only once private owners have left the colony without power.
  The tariff becomes a ceiling that private owners have to undercut.

This is the boomtown's oil-baron story. Uranium is the strike: rare (3 tiles in
a thousand, deliberately), found by digging, deeper more often than not, and a
small vein is worth a fortune because a single unit of ore runs a kitchen for
days. Water ice is the dull bulk input that the reactor goes through steadily.

P0 (rarer uranium), P1 (batteries and powered cooking) and P2 (the reactor,
run by the colony) are built. The colony cooks on the batteries its ships
landed, buys the first uranium anyone digs up, builds a reactor, and charges
more. P3 adds the choice of who runs the reactors (`power-policy`: monopoly,
market or public option); the charter and changing the policy mid-game are
still to come. Today uranium and water ice
are mined and sold to the silo, and then nothing uses them. Power gives both a
reason to exist.

| Phase | What ships | Status |
| --- | --- | --- |
| P0 | Rarer uranium: a per-mille setting, fewer veins of the same size, a mother lode in every world, mutation re-measured | **Shipped** |
| P1 | Batteries and powered cooking: the `Battery` good, powered and cold recipe rows, batteries in the ships, battery supply drops | **Shipped** |
| P2 | The reactor under a monopoly: fuel pellets, the reactor room, the operator's wage, the colony's uranium and ice bids, the tariff | **Shipped** |
| P3 | The market and the public option: `power-policy`, reactor commissions, the colony buying batteries instead of making them, the colony's reactor of last resort | **Shipped** |
| P4 | The charter: the corporation as an owner, its capital, royalty and remittance | Proposed |
| P5 | Changing policy mid-game: buyouts, sales, and the cooldown | Proposed |
| P6 | The player's view: the policy on the new-game form and in game, the tariff in the Trade tab, power on the charts | Proposed |

## Source

P3 is in `power.go` (`PowerPolicy`, `colonyRunsPower`,
`colonyBuysBatteries`, `refreshColonyBatteryBids`, `commissionReactors`,
`refreshOwnerReactorBids`, `keepStockedFor`, `tryAssignOwnReactor`,
`notePowerShortage`), `Entity.reactorCommissioned`, the named-choice setting
in `configfile.go`, `main.go` and `wire/config.go`, and
`power_policy_test.go`.

P2 is in `power.go` (`reactorRoom`, `wantsReactor`, `tryAssignReactor`,
`batteriesWanted`, `refreshReactorFuel`, `fuelBid`, `keepStocked`,
`reactorFuelled`), the `Reactor` terrain (`world.go`, `property.go`,
`structures.go`, `construction.go`, the glyph ⚡), `FuelPellet`, the two
reactor rows in `recipes`, the operator's wage in `jobCraft`, the reactor's
place in `planRooms` and the work ladder, and the exposure check in
`mutation.go`.

P1 is in `power.go` (`recipeEnabled`, `needs`, `refreshKitchenBatteries`,
`deliverBatteries`), `scumhouse.go` (the powered and cold rows, `Recipe.Cold`,
`scumMealRecipe`), `ship.go` (the colony's batteries in each locker),
`director.go` (`batteries` on a supply drop), `food.go` (`colonyMealsIn`), and
`power_test.go`.

P0 is in `config.go` (`UraniumRockPermille`), `configfile.go` (the retired
`uranium-rock-percent`), `worldgen_chunks.go` (`veinShare`, `pickMotherLode`
and the mother lode in `growVeins`), `depth.go`, and
the vein, depth, drift and mutation tests. The expected footprint of the rest:

- `internal/sim/power.go`: `PowerPolicy`, the reactor's fixture and room
  recipe, `commissionReactors`, the colony's battery orders, reactor exposure,
  the charter, and `setPowerPolicy` with its buyouts and sales.
- `internal/sim/property.go` and `money.go`: `OwnerCorporation`, the owner
  kind the economy left as a TODO.
- `internal/sim/inventory.go`: `Battery` and `FuelPellet` item kinds.
- `internal/sim/scumhouse.go`: the `recipes` table gains powered and cold rows
  and the two reactor rows.
- `internal/sim/ship.go`: batteries in each locker and in the hold.
- `internal/sim/director.go`: `supply-drop` gains a `batteries` field.
- `internal/sim/worldgen_chunks.go`: `veinPercent` becomes per mille for
  uranium.
- `internal/sim/mutation.go`: a fuelled reactor counts as a dose source.
- `internal/sim/config.go` and `mars-sim.yaml`: the settings below.
- `web/`: the policy on the new-game form, the tariff (an existing colony ask),
  power metrics.

## Decisions already made

These came out of the conversation that started this doc and the review of it
(PR #199). Do not re-open them without a reason those didn't have.

| Topic | Decision |
| --- | --- |
| Fuel | Uranium and water. Uranium is the rare input, needed in tiny amounts; water ice is the bulk input. |
| The policies | Four: an electricity **monopoly** run by the colony, a **market** where colonists supply it, a **charter** granted to a corporation, and a **public option** (a market with a colony reactor of last resort). |
| Changing policy | The player can change the policy during the game, not just at the start. See [Changing policy](#changing-policy). |
| Build-out | Ships arrive with batteries, so the colony can cook before it has a reactor. |
| Batteries | Single-use for now. Rechargeable batteries with empties are a possible later step, not part of this plan. |
| No power | Cooking without a battery is slower and leaner (cold cooking), never impossible. |
| Rarity | Uranium gets rarer than the old 1%, for flavor: you truly don't need much of it. **3 per mille** on the landing level, for now. |
| No uranium in reach | Every world gets a **mother lode**: one full-size vein per level, in a chunk picked from the seed. |
| Not a concern yet | An import price ceiling for batteries, and the exact starting numbers. Both are left to tuning. |

## How it works (proposed)

### Power is a good, not a grid

A **battery** is an ordinary `ItemKind`: it stacks, sits in depots under a
ledger, has an owner, trades on the order book and gets hauled. One battery is
one recipe's worth of power. Using power means consuming a battery as a recipe
input.

This is the biggest decision in the design, so the reasons are spelled out in
[Why it is this way](#why-it-is-this-way). In short, it reuses the whole
economy (ownership, the book, escrow, hauling, the producer planner) instead of
inventing a network, and it costs nothing per tick.

### The chain

| Recipe | Facility | In | Out | Ticks |
| --- | --- | --- | --- | --- |
| press fuel pellets | reactor | 1 uranium ore | 40 fuel pellets | 60 |
| charge batteries | reactor | 1 fuel pellet, 1 water ice | 12 batteries | 20 |
| culture cave scum | scumhouse | 2 cave scum, 1 battery | 1 meal | 12 |
| culture cave scum cold | scumhouse | 3 cave scum | 1 meal | 36 |

The reactor rows are P2. The food rows are built (P1), and every food recipe
has the same pair:

| Powered (1 battery) | Cold |
| --- | --- |
| 1 alien carcass → 4 meals, 30 ticks | the same, 90 ticks |
| 1 animal carcass → 1 meal, 10 ticks | the same, 30 ticks |
| 2 viscera → 1 meal, 10 ticks | the same, 30 ticks |
| 2 cave scum → 1 meal, 12 ticks | the same, 36 ticks |

Every powered row comes before every cold row in `recipes`. A cook works the
first recipe it has inputs for, so a stove with a battery in its depot cooks
powered, and a stove without one still cooks. Cold is three times the ticks,
on the same inputs.

**Cold used to cost inputs too.** P1 shipped cold rows that also took half
again the biomatter (3 scum to a meal, not 2), as this doc first proposed. It
cost lives. Over 24 seeds, power on starved 20 colonists against 7 with it
off. Colonists cooking their own food always cook cold (they own no
batteries), so every hungry forager had to scrape half again as much. The
decision was only ever "slower cold cooking", so P2 took the extra inputs out
(see [What it measured](#what-p2-measured)).

One unit of uranium ore makes 40 pellets, and those make 480 batteries. At
about two meals per colonist-day ([drives.md](./drives.md)), that is a little
over twelve days of cooking for twenty colonists from one tile of ore. A
16-tile vein runs the same colony for about six months. That is why uranium can
be rare: the colony needs to find it, not to find a lot of it. Water ice goes
at one unit per twelve batteries, three or four a day for the same
colony: a steady trickle of work for an ice miner, which is the role ice
should have.

Pellets are a separate good so the rare input is spent once and the reactor
then runs on a stock of pellets, and so uranium ore does not sit in the
reactor's depot dosing nobody. (Ore in a depot doses nobody today; see
[Reactor exposure](#reactor-exposure).)

The numbers are first guesses. The cold rows are data in the recipe table, as
every recipe is ([scumhouse.md](./scumhouse.md)); the reactor's ratios will be
too.

### Cold cooking: power is an advantage, not a gate

A stove with no battery cooks **cold**: three times the ticks, on the same
inputs. A colony that runs out of power gets hungrier and poorer, but
nobody starves *because* the batteries ran out. That follows principle 7 in
[design-principles.md](./design-principles.md): running out of batteries is a
bad decision, and it should hurt like one. It should not read as a bug.

**The `power` switch** (on by default) turns all of this off: with it off,
every powered row works without a battery and spends none, and the cold rows
are never worked. That is the rule from before power. The mechanics tests run
with it off (`testConfig`), because dozens of them stock a stove with exactly
one powered meal's worth of scum, and a power test turns it on.

The gap between the two rows is what gives power its price. The producer
planner ([valuation.md](./valuation.md)) already compares recipes by what
they cost and pay. A battery is worth buying up to the scum and cook's time it
saves, and that sets the ceiling on what anyone will pay for one, the
tariff included.

### The build-out: batteries in the ships

Every passenger's **locker** lands with `ship-batteries` (default 10) of the
**colony's** batteries, on the colony's ledger line. The locker is private, so
only its passenger may take them out. Every market interval
`refreshKitchenBatteries` tops each colony stove up to
`kitchen-battery-stock` (default 12). For what's missing and not already on
its way, it posts a haul order (`WorkHaul`, `haul-pay` a unit) from wherever
the colony holds batteries, nearest first. At landing that means each
passenger is paid to carry its share from its own locker to the stove. The
supply drop's batteries at the silo go the same way. Arrivals bring their
share, so a growing colony's reserve grows with it.

The proposal also gave each settler `crash-pod-batteries` of their own, for
cooking their own supper. That is **not built**. A colonist's own scum would
need a battery of its own in the stove's depot, which means buying or hauling
it, and that is the market's job (P3). So a colonist cooking its own food
cooks cold for now, and `scumMealRecipe(own)` tells foraging and the
producer's sums so.

The director's `supply-drop` takes a `batteries` count, delivered to the silo
as the colony's own. A `director.yaml` can then script the ship that arrives
just as the batteries run low, or the one that doesn't. With no silo yet the
drop's batteries are lost, the way a weapon with no one to hold it is.

The point of the build-out is that the first reactor is a project, not a
precondition. The colony lands, cooks on imported power, and has time to find
uranium, dig ice, and build. A colony that dawdles finds out what cold cooking
costs.

**What it measured.** Eight seeds of a 120×60 map, 20 colonists, zoning and
siting automatic (as the committed `mars-sim.yaml` runs), 40,000 ticks (38
days):

| | starved | alive at the end | colony batteries left |
| --- | --- | --- | --- |
| `power: false` | 4 | 131 of 160 | n/a |
| `power: true` | 1 | 134 of 160 | 1 to 159 of 200, about half on average |

Survival didn't get worse; the difference is noise. **That was wrong**: 8
seeds weren't enough. P2's re-measure on 24 seeds found the cold rows'
extra scum cost lives, and took it out (see
[What P2 measured](#what-p2-measured)). The batteries last far
longer than the "ten days" this doc first guessed, because the colony's stoves
cook only part of what the colony eats. Carcasses, chefs and colonists cooking
their own scum make the rest, and those cook cold already. So in P1 the
colony's stoves mostly stay powered for the first month. The reactor (P2) is
what makes that last.

### The reactor

The reactor (⚡) is a fixture with a depot, like the forge
([foundry.md](./foundry.md)), alone in a walled **reactor room** with an
aisle, in a production zone. With `construction-costs` on it costs 4 raw rock,
4 iron ore and 2 clay. Its depot holds ore, pellets, ice and the batteries it
charges. There is no battery shelf yet (the proposal had one): nobody buys at
the reactor until P3, and the batteries leave by haul order, so nobody needs
to stand where the operator works.

Whoever owns the inputs owns the outputs, as with every recipe. Under the
monopoly that is the colony.

The two rows: **charge batteries** (1 fuel pellet and 1 water ice → 12
batteries, 20 ticks) comes before **press fuel pellets** (1 uranium ore → 40
pellets, 60 ticks). So the reactor charges from the pellets it has, and
presses the next ore only when the last pellet is gone. Working it practises
no skill yet (`SkillNone`): no trade fits, and a new skill touches arrivals
and the skill tables for little gain.

### Policy: monopoly (P2, built)

`power-policy: monopoly`, the default. The colony is the power company.

- **The colony builds the reactor once it holds uranium** (`wantsReactor`:
  any uranium ore of the colony's, anywhere, and no reactor planned or built).
  A reactor with no fuel would be a monument. It comes after the bunks and
  before the incinerator in `planRooms`. A colony without power cooks cold
  rather than starving, so the reactor isn't life support.
- **The colony buys the fuel at its silo** (`refreshReactorFuel`, market
  upkeep). It bids for uranium ore until it holds `uranium-reserve` (2),
  counting a reactor's pellets as one ore not yet spent. Once a reactor
  stands, it also bids for water ice until it holds `reactor-ice-stock` (8) a
  reactor. These bids are allowed under `standing-orders-build-only`, which
  otherwise only allows bids for building stone (`reactorFuel` in
  `standingAllowed`). The colony's general prospecting bids and its resale
  asks skip both goods, so it never resells its fuel. The colony is the only
  buyer of uranium, so its price is the price: `price-uranium-ore` went from 6
  to **50**.
- **Haul orders carry the fuel over** (`keepStocked`, the same machinery that
  stocks the stoves): `reactor-ice-stock` ice at each reactor, and one ore at
  each reactor that is out of pellets.
- **An operator** takes the reactor straight after the colony's cooking in the
  work ladder, while `batteriesWanted`: the colony holds fewer than
  `battery-reserve` (60) batteries, counted everywhere including haulers'
  hands and memoized for the tick. It is paid `wage-reactor` (2) a recipe and
  keeps going while the colony is still short.
- **The batteries reach the stoves** by `refreshKitchenBatteries`. The reactor
  is just another depot holding colony batteries, so its batteries are hauled
  out to the stoves the way the lockers' were.
- **The tariff** (`power-tariff`, $3) is what a battery is worth
  (`referenceValue`). A meal's cost floor counts its battery at that price
  (`mealInputCost`), so the tariff reaches the price of every meal the colony
  sells. The proposal's standing ask for batteries isn't built: in P2 no
  colonist can use a battery it buys. Colonists cooking their own food come
  with the market (P3), and so does the ask and repricing it from the Trade
  tab.

The tariff is the lever the policy gives the player. A high tariff is income:
it brings money back to a treasury that otherwise only pays out, and that is
one of the economy's standing risks ([economy.md](./economy.md)). But it makes
every meal dearer, and the colonists least able to pay are the ones who go
cold or hungry. A low tariff means cheap food and a treasury that has to find
its money elsewhere. In P2 only the meal floor carries it.

### What P2 measured

A 120×60 map, 20 colonists, zoning and siting automatic (as the committed
`mars-sim.yaml` runs), 40,000 ticks (38 days), colonists starved:

| seeds | `power: false` | power, no reactor (`uranium-reserve: 0`) | power and reactor |
| --- | --- | --- | --- |
| 1–24 | 7 | 11 (9 on one seed) | 19 (11 on one seed) |
| 25–48 | 4 | 4 | 2 |
| 49–72 | 3 | 3 | 3 |
| **all 72** | **14** | **18** | **24** |

That's with the cold rows only slower. With P1's hungrier cold rows, seeds
1–24 starved 20 with power on and no reactor (7 off), which is why those rows
changed.

**What the reactor column is.** Most of the deaths are single colonies
collapsing. The biggest, seed 16 with a reactor (11 starved), isn't a
famine. Its colony dug somewhere new for the reactor and the uranium, and a
gremlin nest broke in at about tick 8,400 and killed 8. The other 11
starved within the next 6,000 ticks, with 55 of the colony's meals in its
pantries: survivors cut off or too hunted to eat. Without a reactor the same
seed has no such attack. So that's a different history, not a cost of
running the reactor. Leaving it out, the three columns are within noise.

The operator used to sit straight after the colony's cooking in the work
ladder. It was moved below all the food work (cleaning, seeding, harvesting
and scraping), on the guess that it was taking colonists off food. Re-measured
on the same 72 seeds, that changed nothing (24 starved again, seed 16's 11
included). It stays there anyway, because a colony should be fed before it is
powered.

The chain works without a script. On 64 of the 72 seeds someone dug up
uranium and sold it to the colony, and the colony built a reactor. On the
first eight seeds, watched closely, the first ore came between about tick
4,000 and 13,000, and the reactor stood within about 2,000 ticks of it. It
starts charging once the landed batteries fall below `battery-reserve`, and
by tick 40,000 those colonies held 64 to 113 batteries, with pellets left
from their first ore. On the other eight seeds nobody happened to dig into
uranium: nothing sends a miner looking for it yet, so the mother lode is
found only where the colony happens to dig.

Mutation stays rare. On seeds 49–72 after 40,000 ticks there were 5 mutants
among the living with power off, 8 with power and no reactor, and 6 with the
reactor.

### Policy: market (P3, built)

`power-policy: market`. The setting is a named choice (see
[config-file.md](./config-file.md)), `PowerPolicy` in code, defaulting to
`monopoly`.

- **The colony builds no reactors and buys no fuel.** That is what letting
  the market supply power means. `wantsReactor` and `refreshReactorFuel` ask
  `colonyRunsPower`, which is false here.
- **The colony buys batteries** for its stoves: a standing bid at each colony
  stove for what it is short of `kitchen-battery-stock`, less what's already
  being hauled there, at `price-battery` ($4), as far as the treasury stretches
  (`refreshColonyBatteryBids`). It is allowed under
  `standing-orders-build-only`, and a player can suspend it like any standing
  order. The landing batteries in the lockers still go to the stoves first,
  so for the first weeks the bids mostly sit there.
- **Colonists commission reactors** (`commissionReactors`, each planning
  cycle). The first colonist by ID (never by map order; see
  [determinism.md](./determinism.md)) who holds `reactor-savings` ($300) and
  hasn't commissioned one pays for a reactor room from its wallet, as
  `commissionHouses` does for a house. It happens only while a colony stove's
  battery bid is open (real demand), no reactor is still going up, and every
  reactor already standing has fuel. So one would-be baron tries at a time,
  and a second follows only when the first is running. The room is the
  owner's, **communal** like a chef's kitchen so others can sell into it, but
  only its owner works it (`mayCookAt`, and the colony's operator skips it).
- **The owner fuels it as the colony would** (`refreshOwnerReactorBids`). It
  bids at the silo, from its own wallet, for water ice up to
  `reactor-ice-stock` and for one uranium ore whenever it has neither ore nor
  pellets, at a little over the reference price (`ownerFuelPrice`: a tenth
  more, at least $2, so uranium $55 and ice $4). Then it posts haul orders,
  paid from its wallet, to carry what it bought to its reactor
  (`keepStockedFor`, the colony's restocking with another payer).
- **The owner works it** whenever it holds its own inputs and fewer than
  `battery-reserve` batteries there (`tryAssignOwnReactor`, beside the
  colony's operator in the work ladder; `cooksOn` stops it at the cap too).
- **The owner sells** into the colony's battery bids by the producer
  planner's existing "supply your own stock to a bid" (`planSupply`). A
  battery is a reactor's output, so it counts as suppliable.

Three dead ends on the way, all worth not repeating:

- **Bidding for fuel at the reactor.** Miners take their ore to the silo and
  offer it there at the reference price. A bid across the colony at that
  same price pays no hauler to bring it. The first barons sat beside open
  bids while dozens of uranium ore sat on sale at the silo.
- **A premium on that bid** (half again over the reference price) got the
  uranium moving but not the ice: barons sat on 40 pellets with no ice to
  charge them. Buying at the silo, where the miners sell, and paying the haul
  separately is what the monopoly does, and it works for both.
- **Bidding at the silo at exactly the reference price.** Ice came, uranium
  didn't. Miners' unsold ore sits on their own lines at the silo, or in their
  lockers, after their asks expire. The planner offers a colonist's own stock
  to a bid only above the reference price (`planSupply`: the reference price
  is what the colony would pay anyway), so a $50 bid beside four ore of a
  miner's at the silo went unfilled for 12,000 ticks. Hence the premium.

**What isn't market yet.** The colony's bid is fixed at `price-battery`, so
mostly a baron's price is the colony's, not whatever the book bears. A lone
baron gets rich because nobody else sells, not because it can name its price.
There is other demand, and it came for free: a cook planning a meal for a
customer (`planCraft`) finds the powered row first, sees a battery is
something a reactor makes, and posts a derived bid for one at its kitchen.
Seen at $21 and $57 in the traces. So batteries do trade above the colony's
price now and then. A ceiling the public option's tariff could set needs the
colony to *sell* batteries, which it doesn't yet.

The risk of this policy is the story it is meant to tell. If nobody can afford
a reactor, the colony cooks cold until somebody can, or until the director's
next supply ship.

### Policy: public option (P3, built)

`power-policy: public-option`. Everything in the market policy holds, plus
one thing: the colony runs a reactor of its own once the market has failed
it.

- **The trigger** (`notePowerShortage`): once the colony has held no battery
  at all for `public-option-days` (3), `publicReactor` latches, for good, and
  `colonyRunsPower` turns true. "No battery at all" means the lockers and
  stoves are empty and nobody is selling.
- **Then it is the monopoly's reactor**: the colony bids for uranium, builds
  one reactor once it holds some (`wantsReactor` counts only the colony's own
  reactors, so barons' reactors don't stop it), fuels it, and pays its
  operator. It never builds a second. It keeps buying batteries too, so a
  baron still sells to it.
- **The tariff as a ceiling** isn't built: the colony doesn't sell batteries
  yet, so its tariff can't cap a baron's price.

PUBLIC_MEASURE

### Policy: charter

`power-policy: charter`. The colony grants an exclusive franchise to one of the
seed's corporations ([arms-makers.md](./arms-makers.md)), picked by the player
(the first in the roster by default).

- **The corporation is an owner.** The economy's `OwnerKind` has carried a
  commented-out `OwnerOrganization` since E0 ([economy.md](./economy.md)). The
  charter is the first thing that needs one, as `OwnerCorporation`, with a
  wallet like a colonist's. Nobody else may build a reactor.
- **It arrives with capital.** When the charter is granted, the corporation
  gets `charter-capital` dollars from off-world, plus enough to buy any
  reactors that already exist (see [Changing policy](#changing-policy)). That
  is money entering the supply, the opposite of a recruiter's fee
  ([recruiting.md](./recruiting.md)). The money audit counts it as issued,
  like the founding grant.
- **It runs like a private owner.** It commissions reactor rooms from its own
  wallet, buys uranium and ice on the book, hires colonists to operate the
  reactor with work orders, and sells batteries at whatever the book bears.
  It is the only seller, so the cold recipe is its only ceiling.
- **It pays the colony a royalty**, `charter-royalty` percent (default 10) of
  every battery sale, to the treasury.
- **It sends its profit home.** Once a day, whatever the corporation holds
  above `charter-capital` leaves the money supply. Over a long game the charter
  drains the colony's money, and that is the price of a company town.

### Changing policy

The player can change `power-policy` at any time with a command
(`SetPowerPolicy`). The rules for who may build and who sets the price change
at once. Reactors that already exist change hands at **build cost**: what the
room would cost to build today at current wages, plus the goods in its depot at
reference prices. A change is all or nothing, as funding a room is
([labor.md](./labor.md)). If the new owner can't pay for everything, the change
is refused and the log says why.

| From → to | What happens to the reactors |
| --- | --- |
| anything → monopoly | The colony buys every private and charter reactor from the treasury. This is nationalization. A charter ends, and its remaining money goes off-world. |
| anything → charter | The corporation buys every reactor, the colony's included, with the capital it arrives with. |
| monopoly or charter → market | The colony offers its reactors for sale at build cost. `commissionReactors` buys an existing reactor before commissioning a new one. Until one sells, the colony runs it at market prices. A charter's reactors are bought back by the colony first, then offered. |
| anything → public option | As the market, except that the colony keeps one reactor and offers only the rest. |
| market ↔ public option | Nothing changes hands. |

Owners can't refuse. A buyout at build cost is a compulsory purchase at a fair
price, so it needs no enforcement: nobody has to be made to leave anything
(see [economy.md](./economy.md), *Deliberately not doing*). An owner bought
out gets a memory of it, with a grip that depends on whether it sold at a profit.

`power-policy-cooldown` (default 2 days) has to pass between changes. Without
it a player can flip back and forth faster than anyone can respond, and the
log fills with buyouts.

### Reactor exposure

Built in P2. A reactor holding pellets or uranium ore counts as a uranium
deposit for the exposure check in [mutation.md](./mutation.md): standing
beside it accumulates dose. The operator becomes the colony's likeliest
mutant, which is a story, and a Mutant-Lover operator is one who doesn't
mind. This is
`reactor-exposure` (on by default). It adds one fixture lookup to an existing
eight-neighbour check, so it costs nothing measurable.

### Rarer uranium (P0, shipped)

`uranium-rock-percent` couldn't go below 1, because it was an `int` percent. It
is replaced by `uranium-rock-permille`, default **3 (0.3%)**, and the old key
is retired with a pointer (`RetiredSettings`; 1% is 10). The depth scaling
stays: twice as much one level down, three times as much two levels down, so
the surest strikes are below the landing level. That gives the colony one
more reason to dig down ([depth.md](./depth.md)).

Rarity comes from **fewer veins, not smaller ones.** A 64×64 chunk held about
2.5 uranium veins of the usual 8–24 tiles. At 3 per mille it holds about 0.8,
so many chunks have none at all, and the one that does holds a real strike.
Shrinking veins instead would spread the same small amount evenly across every
chunk, which is exactly not a boomtown.

`veinShare` gives every composition its share and its denominator: percent
for iron, ice and clay, per mille for uranium. The vein count's random draw is
bounded by that denominator, so converting everything to per mille would have
moved every iron and ice vein on every seed. Keeping each one's own
denominator left them exactly where they were (the drift table's iron and ice
rows came out identical).

What it measured ([worldgen-chunks.md](./worldgen-chunks.md),
[mutation.md](./mutation.md#after-uranium-got-rarer)):

- **Abundance holds.** The mean is 0.30–0.31% from 80×40 to 1024×1024 maps.
- **Small maps would have had none.** An 80×40 landing level expects less
  than one ordinary vein: 240 of 500 seeds would generate no uranium there,
  47 of 500 at 120×60, and none at 300×150. The default game is 80×40 with
  `deepest-level` 1, so half of default worlds would have had no uranium in
  reach, and no way to power a reactor.
- **So every world gets a mother lode.** One chunk per level, picked from the
  seed alone so lazy generation keeps working, always grows a full-size vein,
  outside the landing site and clear of where caverns will open, so the
  colony has to dig to it and finds it intact. It replaces one of its chunk's
  ordinary veins, so big maps keep their share. See
  [worldgen-chunks.md](./worldgen-chunks.md#the-mother-lode).
- **Mutation** fell by between half and four fifths with rarer uranium alone,
  and came back to about the old rate with the mother lode (0.7% of colonists
  after 10,000 ticks, against 1.7% at the old 1%, on only 8 seeds).
  `uranium-exposure-ticks` is deliberately not retuned yet: reactor exposure
  (P2) adds another dose source, and the retune belongs after that.
- **Worldgen moved a little.** Iron and ice come before uranium in
  `veinLevels` and did not move. Clay avoids uranium tiles, so a few clay veins
  shifted, and the golden tile hashes were re-pinned. Entities and the RNG
  streams were unchanged.
- **Price.** Uranium at `price-uranium-ore` 6 was set when it was just
  another ore. Once it fuels a reactor its real value is set by what 480
  batteries are worth, so P2 raised it to 50. Under a monopoly the colony's
  bid is the price: one ore is a strike, and the miner who finds a vein
  should feel rich.

## Why it is this way

- **Batteries, not a grid.** A grid (wires or conduits, connected networks,
  supply and demand balanced each tick) is the obvious model, and the wrong
  first one. It needs a new network structure that has to stay correct as
  rooms grow and walls move ([room-expansion.md](./room-expansion.md)), a
  per-tick solve that grows with the colony (principle 9), and a new answer to
  "who owns electricity in a wire". Batteries need none of those. They are
  goods, and the economy already handles goods: owners, ledgers, the book,
  escrow, hauling, and the producer planner reaching back from a stove's
  demand to a reactor and then to an ice miner. The monopoly-versus-market
  choice also becomes a choice about *who may own the reactor and who sets the
  ask*, which the economy can already express. This is principle 10. A grid can
  come later, as outlets that dispense from a reactor's depot (see
  [Extending it](#extending-it)).
- **One battery, one recipe.** A charge level on each battery would make it an
  item with state, and stacks have no identity ([inventory.md](./inventory.md)).
  An integer count of single-use batteries keeps power a quantity (principle
  1's good case), and the ratios do the tuning.
- **Pellets between ore and batteries.** Burning ore directly in the battery
  recipe would make the rare input a per-batch input, so the reactor would
  stall on every missing unit of uranium and bid for one every few minutes. A
  pellet stock spends the strike once and then runs on ice.
- **Cold cooking instead of no cooking.** A hard gate means a colony that
  runs out of batteries before its reactor is up starves, and that would happen
  to the colonies that were already struggling. The cold rows keep the
  shortage legible: food gets dearer and slower. Players see it, and it
  doesn't kill anyone outright.
- **The policy is an enum.** The four policies are four rule sets, not points
  on a dial, and a switch over them makes the compiler point out every place a
  new one has to be handled (principle 1).
- **A policy change is a sale at build cost.** Changing policy mid-game
  raised two questions: what a buyout pays, and what happens when an owner
  won't sell. Build cost answers the first without an appraisal model. Making
  the sale compulsory answers the second without enforcement, which the
  economy doesn't have yet.
- **The public option builds one reactor, late.** If it built at once, or
  more than one, it would be the monopoly by another name. Building only after
  days of unfilled demand means it only appears when the market has failed.
- **The charter's money comes from off-world.** A corporation funded from the
  treasury would just be the monopoly with extra steps. Outside capital coming
  in and profit going home is what makes it a company town, and it is the one
  policy whose cost is money leaving the colony for good.
- **The monopoly's own kitchens pay no tariff.** Charging itself would only
  move money from one colony pocket to the other. The tariff reaches the colony's
  meals through the meal price floor instead.

## Open questions

- **Who picks the charter's corporation, and does it matter?** Corporations
  are flavor today. A charter could make them differ (a higher royalty, a
  greedier price), but then corporations stop being flavor (principle 3).
- **What a nationalized owner feels.** The memory of being bought out needs
  numbers in `affect.go`, and a baron who loses their reactor at cost should
  probably resent it more than one who sold at a profit.
- **The target rarity.** 3 per mille is the number for now. The right one is
  the one where a colony usually finds uranium within its first two weeks of
  digging, but has to dig for it.

## Extending it

- **A new power consumer** is a recipe edit: add a battery to its inputs, and
  add a slower or leaner unpowered row after it if going without power
  shouldn't stop the work. The forge is the obvious next one
  ([foundry.md](./foundry.md)). Lights, heat and life support are needs-side
  consumers and would need a facility that consumes batteries per use, like a
  paid fixture ([labor.md](./labor.md)).
- **A grid**, once there is a reason for one, is an **outlet** fixture that
  dispenses power to an adjacent consumer from its reactor's depot, through
  ledger transfers instead of hauling. The owner, the price and the book stay
  as they are.
- **Rechargeable batteries.** Spent batteries could come back as empties that
  the reactor recharges: a finite battery fleet hauled back and forth, with
  deposits. That's more hauling and more flavor, and also a new way for a
  colony to jam (empties piling up in a pantry nobody clears). It was
  considered and set aside for single-use batteries.
- **Imports as a price anchor.** Batteries ordered off-world at a fixed, high
  price would cap what any baron can charge.
- **Spent fuel.** The battery recipe could leave a unit of waste every N
  pellets: refuse that doses whoever carries it, with somewhere to put it
  (a hole, see [holes.md](./holes.md)).
- **Keep it deterministic.** Every choice of who commissions, which reactor,
  and which tie wins goes by ID, never by map order.

## Related

- [mutation.md](./mutation.md): uranium deposits, the dose, and the mutation
  rate this changes.
- [depth.md](./depth.md): uranium's per-level scaling.
- [worldgen-chunks.md](./worldgen-chunks.md): veins, `veinLevels`, and chunk
  counts.
- [scumhouse.md](./scumhouse.md): recipes and the kitchen.
- [foundry.md](./foundry.md): the last supply chain built on the same pattern.
- [valuation.md](./valuation.md), [pricing.md](./pricing.md): how demand for
  batteries reaches the rock, and how prices move.
- [labor.md](./labor.md): commissions, wages, and paid fixtures.
- [colony-orders.md](./colony-orders.md): the tariff is a colony ask.
- [ships.md](./ships.md), [director.md](./director.md): batteries at landing
  and by supply drop.
- [economy.md](./economy.md): the plan this extends.

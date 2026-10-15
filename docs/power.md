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

Only P0, rarer uranium, is built. Today uranium and water ice are mined and
sold to the silo, and then nothing uses them. Power gives both a reason to
exist.

| Phase | What ships | Status |
| --- | --- | --- |
| P0 | Rarer uranium: a per-mille setting, fewer veins of the same size, mutation re-measured | **Shipped** |
| P1 | Batteries and powered cooking: the `Battery` good, powered and cold recipe rows, batteries in the ships, battery supply drops | Proposed |
| P2 | The reactor under a monopoly: fuel pellets, the reactor room, the operator's wage, the colony's uranium and ice bids, the tariff | Proposed |
| P3 | The market and the public option: `power-policy`, reactor commissions, the colony buying batteries instead of making them, the colony's reactor of last resort | Proposed |
| P4 | The charter: the corporation as an owner, its capital, royalty and remittance | Proposed |
| P5 | Changing policy mid-game: buyouts, sales, and the cooldown | Proposed |
| P6 | The player's view: the policy on the new-game form and in game, the tariff in the Trade tab, power on the charts | Proposed |

## Source

P0 is in `config.go` (`UraniumRockPermille`), `configfile.go` (the retired
`uranium-rock-percent`), `worldgen_chunks.go` (`veinShare`), `depth.go`, and
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
| culture cave scum, cold | scumhouse | 3 cave scum | 1 meal | 36 |

The other three food recipes get the same pair: a powered row first, a cold
row after it. A cook works the first recipe it has inputs for, so a stove with
a battery in its depot cooks powered, and a stove without one still cooks.

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

The numbers are first guesses, and all of them are tunables
(`reactor-pellets`, `reactor-batteries`, and the cold recipes' ratios).

### Cold cooking: power is an advantage, not a gate

A stove with no battery cooks **cold**: half again the inputs and three times
the ticks. A colony that runs out of power gets hungrier and poorer, but
nobody starves *because* the batteries ran out. That follows principle 7 in
[design-principles.md](./design-principles.md): running out of batteries is a
bad decision, and it should hurt like one. It should not read as a bug.

The gap between the two rows is what gives power its price. The producer
planner ([valuation.md](./valuation.md)) already compares recipes by what
they cost and pay. A battery is worth buying up to the scum and cook's time it
saves, and that sets the ceiling on what anyone will pay for one, the
tariff included.

### The build-out: batteries in the ships

Every ship's **hold** carries `ship-batteries` (default 10 per passenger),
owned by the colony. At two meals a colonist-day that is five days of powered
cooking, on top of the five days the meals in the lockers already cover, so
about ten days before the colony has to cook cold. Each **locker** carries
`crash-pod-batteries` (default 2) of the settler's own, for a colonist cooking
its own supper. Arrivals bring their share, so a growing colony's reserve grows
with it.

The director's `supply-drop` gains a `batteries` count, delivered to the silo
as the colony's own. A `director.yaml` can then script the ship that arrives
just as the batteries run low, or the one that doesn't.

The point of the build-out is that the first reactor is a project, not a
precondition. The colony lands, cooks on imported power, and has a week or two
to find uranium, dig ice, and build. A colony that dawdles finds out what cold
cooking costs.

### The reactor

The reactor is a fixture with a depot, like the forge
([foundry.md](./foundry.md)), in a walled **reactor room** built through the
ordinary room planner. Its depot holds ore, pellets and ice, and like a
kitchen it has a battery shelf (a linked chest, as the pantry is) so buyers
never stand where the operator works.

Whoever owns the inputs owns the outputs, as with every recipe. Who that is
depends on the policy.

### Policy: monopoly

`power-policy: monopoly` (the default).

- **Only the colony builds reactors.** The planner plans a reactor room once
  the colony holds any uranium ore or pellets, or its batteries fall below
  `power-reserve-days` of cooking. Before that, nothing is built: a reactor
  with no fuel is a monument.
- **The colony buys the fuel.** It posts standing bids at the silo for uranium
  ore and water ice. Those bids exist today but are suspended by
  `standing-orders-build-only` ([colony-orders.md](./colony-orders.md)). Under
  a monopoly they become standing orders the colony needs, the same as its
  building materials. The colony is also the only buyer for uranium, so it
  sets the price that prospectors get. That is a monopsony, and it is
  deliberate.
- **The colony pays an operator** `wage-reactor` per recipe, from the
  treasury, the same way it pays its cook ([labor.md](./labor.md)).
- **The colony sells batteries at the tariff.** The tariff is the colony's
  standing ask for batteries at the reactor's shelf, priced at `power-tariff`.
  The player changes it the way they reprice any colony ask, from the Trade
  tab, and it is marked `manual` so upkeep leaves it alone.
- **The colony's own kitchens use its own batteries** and pay no tariff. The
  tariff still reaches every meal, because the colony's meal price has a floor
  at the cost of its inputs ([pricing.md](./pricing.md)), and a battery is now
  one of those inputs, valued at the tariff.

The tariff is the lever the policy gives the player. A high tariff is income:
it brings money back to a treasury that otherwise only pays out, and that is
one of the economy's standing risks ([economy.md](./economy.md)). But it makes
every meal dearer, and the colonists least able to pay are the ones who go
cold or hungry. A low tariff means cheap food and a treasury that has to find
its money elsewhere.

### Policy: market

`power-policy: market`.

- **The colony builds no reactors.** That is what letting the market supply
  power means.
- **The colony buys batteries** for its kitchens with a standing bid at each
  kitchen's stove at `price-battery`, the same way it buys biomatter, and
  suspendable the same way.
- **Colonists commission reactors.** Once per planning cycle,
  `commissionReactors` has the first colonist (by ID, never by map order; see
  [determinism.md](./determinism.md)) who holds `reactor-savings` dollars and
  owns no reactor commission a reactor room, paid from its wallet, exactly as
  `commissionHouses` does for a house. It fires only while some battery bid is
  unfilled, which means only while there is real demand. The owner gets the
  fixture, buys uranium and ice on the book through the producer planner,
  works the reactor itself or posts work orders, and sells batteries at
  whatever the book bears.
- **No price is set for anyone.** A lone reactor owner faces only the cold
  recipe's ceiling and the colony's bid, so the first owner gets rich. A second
  colonist who reaches `reactor-savings` builds a competing reactor, and the
  asks come down ([pricing.md](./pricing.md)).

The risk of this policy is the story it is meant to tell. If nobody can afford
a reactor, the colony cooks cold until somebody can, or until the director's
next supply ship. If one colonist owns all the power, everyone else pays that
colonist's price.

### Policy: public option

`power-policy: public-option`. Everything in the market policy holds, plus
one thing: the colony keeps a reactor of its own in reserve.

- **The colony builds one reactor, and only as a last resort.** The planner
  plans it once the colony has gone `public-option-days` (default 3) without
  filling a battery bid, which means private owners have let the colony go
  without power. It never builds a second.
- **It runs as the monopoly's reactor does**: the colony buys its fuel, pays
  its operator `wage-reactor`, and sells at the tariff.
- **The tariff becomes a ceiling.** Nobody pays a private owner more than the
  colony's ask while the colony's reactor has stock, so a baron has to
  undercut it to sell. A tariff set high leaves room for private profit; set
  low, it crowds private owners out and the policy is a monopoly in all but
  name.

This is the policy that answers the market's worst case, a colony with no
power because nobody could afford a reactor, without taking the market away.

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

A reactor holding pellets or uranium ore counts as a uranium deposit for the
exposure check in [mutation.md](./mutation.md): standing beside it accumulates
dose. The operator becomes the colony's likeliest mutant, which is a story,
and a Mutant-Lover operator is one who doesn't mind. This is
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
- **Small maps can have none.** An 80×40 landing level expects less than one
  vein: 240 of 500 seeds generate no uranium there, 47 of 500 at 120×60, and
  none at 300×150. The default game is 80×40 with `deepest-level` 1, so about
  half of default worlds have no uranium they can reach. That is harmless
  today, when uranium only mutates. It matters once a reactor needs it. See
  [Open questions](#open-questions).
- **Mutation drops** by between half and four fifths (0.3% of colonists after
  10,000 ticks, against 1.7% at the old rate). `uranium-exposure-ticks` is
  deliberately not retuned yet: reactor exposure (P2) adds a dose source back,
  and the retune belongs after that.
- **Worldgen moved a little.** Iron and ice come before uranium in
  `veinLevels` and did not move. Clay avoids uranium tiles, so a few clay veins
  shifted, and the golden tile hashes were re-pinned. Entities and the RNG
  streams were unchanged.
- **Price**, still to do. Uranium at `price-uranium-ore` 6 was set when it was
  just another ore. Once it fuels a reactor its real value is set by what 480
  batteries are worth. Under a monopoly the colony's bid is the price, and it
  should go up a lot: one ore is a strike, and the miner who finds a vein
  should feel rich. That belongs to P2, where the colony first bids for it.

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
- **A world with no uranium in reach.** About half of default worlds (80×40,
  no digging down) have none on the landing level. Before P2 makes power
  depend on it, pick one: accept it (those colonies cook cold, or wait for
  supply drops), guarantee a **mother lode** (one chunk per world, chosen from
  the seed alone so lazy generation keeps working, always holds a vein), or
  raise the default map size or `deepest-level`.

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

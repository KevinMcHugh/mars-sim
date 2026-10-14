# Power (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan for giving the colony an energy supply, and the colony's first real
**political choice** about it. Power comes from a **reactor** that burns a
little uranium and a lot of water ice, and it moves around the colony as a
good: **batteries**, charged at the reactor and spent at the stove. The ships
land with enough batteries to cook through the build-out. After that the colony
needs a reactor, and the player decides at the start who gets to own one:

- **Monopoly.** The colony is the power company. Only it builds reactors, it
  buys the uranium and ice, and it sells batteries at a **tariff** the player
  sets.
- **Market.** The colony stays out of it. Any colonist rich enough can
  commission a reactor, own it, and sell batteries for whatever the book will
  pay. The first one to do it is the colony's power baron, at least until a
  second one can afford to compete.

This is the boomtown's oil-baron story. Uranium is the strike: rare (rarer
than today, deliberately), found by digging, deeper more often than not, and a
small vein is worth a fortune because a single unit of ore runs a kitchen for
days. Water ice is the dull bulk input that the reactor goes through steadily.

Nothing here is built yet. Today uranium and water ice are mined and sold to
the silo, and then nothing uses them. Power gives both a reason to exist.

| Phase | What ships | Status |
| --- | --- | --- |
| P0 | Rarer uranium: a per-mille setting, fewer veins of the same size, mutation re-measured | Proposed |
| P1 | Batteries and powered cooking: the `Battery` good, powered and cold recipe rows, batteries in the ships, battery supply drops | Proposed |
| P2 | The reactor under a monopoly: fuel pellets, the reactor room, the operator's wage, the colony's uranium and ice bids, the tariff | Proposed |
| P3 | The market policy: `power-policy`, reactor commissions, the colony buying batteries instead of making them | Proposed |
| P4 | The player's view: the policy on the new-game form, the tariff in the Trade tab, power on the charts | Proposed |

## Source

Nothing exists yet. The expected footprint:

- `internal/sim/power.go`: `PowerPolicy`, the reactor's fixture and room
  recipe, `commissionReactors`, the colony's battery orders, reactor exposure.
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

These came out of the conversation that started this doc.

| Topic | Decision |
| --- | --- |
| Fuel | Uranium and water. Uranium is the rare input, needed in tiny amounts; water ice is the bulk input. |
| The policy | The player chooses between an electricity **monopoly** run by the colony and a **market** where colonists supply it. |
| Build-out | Ships arrive with batteries, so the colony can cook before it has a reactor. |
| Rarity | Uranium gets rarer than today's 1%, for flavor: you truly don't need much of it. |

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
16-tile vein runs the same colony for most of a year. That is why uranium can
be rare: the colony needs to find it, not to find a lot of it. Water ice goes
at one unit per twelve batteries, about three and a half a day for the same
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
owned by the colony, which is enough for the colony's kitchens to cook powered
for roughly the first week and a half. Each **locker** carries
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

### Reactor exposure

A reactor holding pellets or uranium ore counts as a uranium deposit for the
exposure check in [mutation.md](./mutation.md): standing beside it accumulates
dose. The operator becomes the colony's likeliest mutant, which is a story,
and a Mutant-Lover operator is one who doesn't mind. This is
`reactor-exposure` (on by default). It adds one fixture lookup to an existing
eight-neighbour check, so it costs nothing measurable.

### Rarer uranium (P0)

`uranium-rock-percent` can't go below 1, because it is an `int` percent. It is
replaced by `uranium-rock-permille`, with the old key retired with a pointer
(`RetiredSettings`). The proposed default is **3‰ (0.3%)**, a third of
today's. The depth scaling stays: twice as much one level down, three times as
much two levels down, so the surest strikes are below the landing level. That
gives the colony one more reason to dig down ([depth.md](./depth.md)).

Rarity should come from **fewer veins, not smaller ones.** A 64×64 chunk holds
about 2.5 uranium veins of the usual 8–24 tiles today. At 0.3% that drops to
about 0.8 veins per chunk, so many chunks have none at all, and the one that
does holds a real strike. Shrinking veins instead would spread the same small
amount evenly across every chunk, which is exactly not a boomtown.

Knock-on effects that P0 must measure and write down:

- **Mutation drops.** Mutation was tuned against 1% uranium
  ([mutation.md](./mutation.md#tuning-the-rate)). With a third of the deposits,
  fewer colonists stand beside one. Reactor exposure adds a source back. Run
  the mutation table again with both changes and retune
  `uranium-exposure-ticks` if the rate falls well under 1%.
- **Worldgen moves a little.** Iron and ice come before uranium in
  `veinLevels`, so they don't move. Clay avoids uranium tiles and comes after
  it, so a few clay veins shift. The golden hashes change once, in P0.
- **Price.** Uranium at `price-uranium-ore` 6 was set when it was just
  another ore. Once it fuels a reactor its real value is set by what 480
  batteries are worth. Under a monopoly the colony's bid is the price, and it
  should go up a lot: one ore is a strike, and the miner who finds a vein
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
- **The policy is an enum.** `monopoly` and `market` are two different rule
  sets, not a dial. A third (see below) would be a third case every consumer
  has to handle, and the compiler will point out every place that needs one
  (principle 1).
- **The policy is fixed at the start, for now.** Changing it mid-game is the
  best story here (nationalizing the baron's reactor, or the colony selling
  its own), but it raises questions this economy can't answer yet: what a
  buyout pays, and what happens when an owner won't sell, which is
  enforcement ([economy.md](./economy.md), *Deliberately not doing*). The
  simple version ships first, and this one is written down here so it doesn't
  get lost.
- **The monopoly's own kitchens pay no tariff.** Charging itself would only
  move money from one colony pocket to the other. The tariff reaches the colony's
  meals through the meal price floor instead.

## Open questions

- **Rechargeable batteries.** Spent batteries could come back as empties
  that the reactor recharges, so the colony's battery fleet is a finite stock
  that has to be hauled back to the reactor (an ice-and-milk-bottle economy,
  with deposits). It is more hauling work and more flavor, and also a new way
  for a colony to jam: empties piling up in a pantry nobody clears. This doc
  proposes single-use batteries first.
- **Does the colony compete under the market policy?** As written it never
  builds a reactor. Letting it build one only when nobody has for N days would
  be a public option, and also a third policy in disguise.
- **A third policy: the charter.** The colony grants an exclusive franchise to
  one of the seed's corporations ([arms-makers.md](./arms-makers.md)). The
  charter company builds and runs the reactor, sets its own price, and sends
  its profit off-world, out of the money supply, like a recruiter's fee
  ([recruiting.md](./recruiting.md)). It is the company-town version of the
  story, and it makes the corporations more than lore.
- **The target rarity.** 3‰ is a guess. The right number is the one where a
  colony usually finds uranium within its first two weeks of digging, but has
  to dig for it.
- **Imports as a price anchor.** If the colony could always order batteries
  off-world at a fixed, high price, that price would cap what any baron can
  charge. That makes the market policy safer and also tamer.

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

# The foundry: ore to steel to rifles

> Part of the [mars-sim documentation](./README.md).

## What it is

The colony's first supply chain that isn't food, built to exercise the
economy with a chain deeper than meal ← scum. A **forge** smelts iron ore into
**steel ingots**, and a **gun bench** machines steel into **assault rifles**.
The planner builds the two together in a **foundry** room. Nobody runs the
foundry. The colony keeps a standing bid for rifles at its silo (the
**armory**; it starts suspended under `standing-orders-build-only`, so the
player resumes it from the Market tab — see
[colony-orders.md](./colony-orders.md)), and the [producer planner](./valuation.md) carries that demand
three links down to the ore vein, one ordinary bid at a time.

## Source

- [`internal/sim/foundry.go`](../internal/sim/foundry.go) — `foundryRoom`,
  `wantsFoundry`, `mined`; the armory (`armoryStock`, `refreshArmoryBids`);
  supplying your own stock to a bid (`suppliable`, `ownStockFor`,
  `planSupply`).
- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) — the two
  recipe rows, and `nearestWorkshop`, which `nearestScumhouse` now wraps.
- [`internal/sim/producer.go`](../internal/sim/producer.go) — the planner
  changes the chain needed: `producibleHere`, `planWaitingAt`,
  `tryDeliverPlan`, mined inputs in `planCraft`, and unloading before a fetch
  in `advancePlan`.
- [`internal/sim/world.go`](../internal/sim/world.go) — the `Forge` and
  `GunBench` terrains; [`property.go`](../internal/sim/property.go) —
  `isWorkshop`, and both get a depot and a fixture record;
  [`construction.go`](../internal/sim/construction.go) — their costs.
- [`internal/sim/inventory.go`](../internal/sim/inventory.go) — `SteelIngot`,
  `AssaultRifle`, `weaponRank`; [`combat.go`](../internal/sim/combat.go) — the
  rifle's stats.
- [`internal/sim/project.go`](../internal/sim/project.go) — where the foundry
  sits in `planRooms`; [`engine.go`](../internal/sim/engine.go) —
  `OrderFoundry` (`b` then `g` in the TUI).
- [`internal/sim/foundry_test.go`](../internal/sim/foundry_test.go).

## How it works

### The chain

| Link | Recipe | In | Out | Ticks | At |
| --- | --- | --- | --- | --- | --- |
| mine | — | a tile of iron-bearing rock | 1 iron ore | `mine-ticks` | the rock face |
| smelt | smelt steel | 2 iron ore | 1 steel ingot | 40 | forge |
| machine | machine an assault rifle | 3 steel ingots | 1 assault rifle | 60 | gun bench |

Both workshops are depots, like the scumhouse: inputs and outputs sit in a
storage container on the tile, with a ledger, and whoever owns the inputs
owns the outputs. Neither has a pantry: output stays in the workshop's own
depot (`outputDepot`).

### Building it

`foundryRoom` is the forge and the gun bench a tile apart, with an aisle
(five tiles wide), falling back to a narrow room in a cramped cavern. The
planner wants one while `armory-rifles` is above 0 and it has no forge or no
gun bench planned or built (`wantsFoundry`). It comes **last** in
`planRooms`, after beds and the trash room: nothing about rifles keeps anyone
alive. It's an ordinary public work, paid from the treasury.

| Structure | Cost (with `construction-costs` on) |
| --- | --- |
| forge | 4 clay, and nothing else: a clay furnace |
| gun bench | 2 raw rock, 2 iron ore |

On an 80×48 map with the defaults the foundry stands by about tick 1200
(`TestTheColonyBuildsAFoundry`).

### Demand: the armory

Once a gun bench stands, `refreshArmoryBids` (market upkeep) keeps a colony
bid at the silo for `armory-rifles` minus the rifles it already owns
(anywhere) minus what it already bids for, at `price-assault-rifle`, as far
as the treasury stretches. Rifles it buys stay in the silo. It never sells
them.

### How the demand reaches the rock

Everything below the armory's bid is the producer planner (see
[valuation.md](./valuation.md)), one link per colonist:

1. A **gunsmith** sees the $60 rifle bid, has no steel, and posts a derived
   bid at the gun bench for 3 ingots at whatever its margin allows (about $19
   each).
2. A **smith** sees the steel bid, has no ore, and posts a derived bid at the
   forge for 2 ore (about $8 each).
3. Somebody fills the ore bid:
   - a **hauler** buys the colony's surplus ore at the silo (it resells above
     `colony-stock-reserve` at `colony-markup`, $4) and carries it over
     (arbitrage, [hauling.md](./hauling.md)); or
   - a **miner** takes its own ore there, from its pockets or a chest
     (`planSupply`). That beats the colony's $3 prospecting bid.
4. The smith smelts, carries the ingot to the gun bench, and sells it into
   the gunsmith's bid. After three ingots the gunsmith machines a rifle,
   carries it to the silo, and sells it to the colony.

`TestARifleBidReachesTheSilo` is the gate: from one armory bid, ore trades at
the forge, steel at the bench, the colony ends up owning a rifle, and the
deepest plan is 3 links below the bid. Nothing is scripted but the stock at
the silo.

### The prices

`price-assault-rifle` defaults to $60. A rifle is six ore, $24 at the
colony's resale price, and each link has to clear `plan-min-profit` plus
its own labor and walking at `labor-price`. The gunsmith offers its whole
margin for steel, and the smith its whole margin for ore (see *A derived bid
offers the planner's whole margin* in valuation.md), so at $60 the ore bid
lands around $8: enough over $4 for a hauler to take it. Measured on seeds
1–4 to tick 16000, the armory still fills at $36. At $33 it barely runs,
and at $30 no ingot is ever made: the ore bid no longer covers the colony's
$4 ask plus the walk, so no hauler takes it. $60 leaves slack for prices
that move.

### What it did on the reference seeds

On defaults (6 colonists, scarcity on), across seeds 1–8 to tick 16000, the
armory filled to 4 rifles on 7 of 8 seeds. On the eighth the colony had died
out. Nobody starved, with the foundry or without (`-armory-rifles 0`). A
typical run trades 12–16 ingots for the 4 rifles. Check with:

```
mars-sim -econ-trace 16000 -econ-every 2000 -econ-seeds 1,2,3,4
```

The trace now has steel and rifle columns and an `armory` column (rifles the
colony owns).

| Setting | Default |
| --- | --- |
| `armory-rifles` | 4 (0: no foundry, no bid) |
| `price-assault-rifle` | 60 |
| `rifle-damage` / `rifle-range` / `rifle-fire-rest` | 15 / 5 / 1 |

## Why it is this way

The first version of the chain stalled or leaked at every link. Each fix
below is in the planner, not the foundry, since the next deep chain will hit
the same problems.

- **One waiting plan per workshop** (`planWaitingAt`). All four colonists took
  a plan to machine a rifle at the one gun bench, each waiting on steel, and
  a colonist with a plan takes no other. Nobody was left to smelt. The
  scumhouse never showed this because its chain is one link and scraping
  needs no plan to wait on.
- **Bids nothing here can make don't use up the planner's attention**
  (`producibleHere`). The planner looks at `plan-candidates` (4) bids, best
  price first. Hungry colonists' meal bids ($11) outrank the smith's ore bid
  ($8). With no scumhouse able to fill them, they still used up the four
  slots and the ore bid was never seen. Now a bid only counts if a workshop
  that makes it stands, or it could be hauled or supplied.
- **A plan whose goods exist is delivered before anything new**
  (`tryDeliverPlan`, top of `assignWorkJob`). A smith or gunsmith who had
  crafted was pulled onto construction and food work each time it looked
  for a job, and its plan ran out of time (`plan-ttl`) with the steel still
  in the forge.
- **Unload before fetching** (`advancePlan`). A gunsmith whose pockets were
  full of raw rock walked to the bench, found it could carry nothing, and
  repeated this until the plan expired. Three finished rifles sat in the bench
  for good.
- **Your own stock is worth what the colony pays for it, not its last
  trade.** The smith's first fill at $8 set iron's remembered price to $8
  (a first trade sets it outright). A miner reckoning its own ore at that
  value saw no profit in an $8 bid, while the colony was paying $3.
  `planSupply` costs own stock at the reference price: the sale it would make
  otherwise.
- **Own-stock supply covers foundry goods, not food** (`suppliable`). The same
  path picks up steel or rifles a finished plan left behind, and sells them on
  the spot if the buyer is at the same depot. Meals and biomatter are left
  out: selling a colonist's own meals from under it could starve it, and
  scum already has "cook your own leftovers".
- **The armory bids only once a gun bench stands.** Before that the bid is
  demand nobody can meet, and $240 of treasury locked in escrow.
- **A mined good is a valid input to plan for** (`mined` in `planCraft`).
  Before, `planCraft` refused any recipe with an input nothing *produces*.
  That was right for alien carcasses, but it ruled out the forge.

## Extending it

- **Arming colonists.** Rifles pile up in the armory. The natural next step
  is the colony issuing them, or selling them to colonists who want one: a
  willingness-to-pay rule like `mealBidLimit`, keyed on danger (aliens seen,
  wounds). An `AssaultRifle` is already a weapon: whoever carries one, even
  in transit, fights with it (`bestWeapon` ranks it above the shotgun).
- **Another link.** A recipe row with its workshop kind in `Facility`, the
  terrain (a depot via `isWorkshop`, a fixture record, `trackFacility`, a
  glyph), a construction cost, and a room. The planner needs nothing new.
  If an input comes out of the rock, add it to `prospectingGoods` so `mined`
  sees it.
- **Throughput.** A plan makes one recipe's worth: one ingot per smith plan,
  so a rifle is three smith plans in a row at the one forge. A plan that
  batches runs (qty up to the bid, inputs scaled) is the obvious speed-up.
  Keep `planWaitingAt` honest when it lands.
- **Leftover inputs.** A gunsmith whose plan expires keeps whatever steel it
  already bought, in the bench. It uses the steel on its next plan there
  (`planCraft` counts owned inputs), but nobody else can.

## Related

- [valuation.md](./valuation.md) — the producer planner that runs the chain.
- [hauling.md](./hauling.md) — arbitrage, and the colony reselling its ore.
- [market.md](./market.md) — the book and the silo.
- [scumhouse.md](./scumhouse.md) — the first workshop and the recipe table.
- [construction.md](./construction.md) — rooms and construction costs.
- [combat.md](./combat.md) — what a rifle does in a fight.
- [economy.md](./economy.md) — the plan this chain tests.

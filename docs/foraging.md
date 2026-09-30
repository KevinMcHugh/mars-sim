# Foraging and prospecting

> Part of the [mars-sim documentation](./README.md).

## What it is

What a colonist does when it is hungry and there is no food: it **forages**.
It cooks what is already in a scumhouse, scrapes any exposed scum, and
otherwise **prospects**: it digs into rock nobody has seen, because that is
where the colony's uneaten scum is. The colony prospects too, before anyone
is hungry, once it is short of meals and has scraped every exposed patch.
Foraging replaced a loop in which hungry colonists dropped their work, found
no food, were handed the same work straight back, and starved without
finishing it.

## Source

- [`internal/sim/food.go`](../internal/sim/food.go) — `hungryWithoutFood`,
  `planForage`, `foodCooking` (with `colonyMealsIn` and
  `hungryWithoutMeals`), `tryForageScrape`, `tryProspect`,
  `unexploredAround`, `prospectingForFood`, `tryForageUnload`.
- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) —
  `scrapeDestination` (split out of `tryAssignScrape`), and
  `finishScraping`, where a forager keeps a part load.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `assignWorkJob`
  prospects for the colony when it is short; `clearJob` clears `foraging`.
- [`internal/sim/needs.go`](../internal/sim/needs.go) — `resetNeed` ends a
  hunger's search; `applyStarvation` covers a colonist cooking its own supper.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `foraging`,
  `forageNoted`, `forageRetry`.
- [`internal/sim/forage_test.go`](../internal/sim/forage_test.go).

## How it works

### When a colonist forages

The eat focus is only eligible once hunger is **pressing** (650 of 1000; see
[needs.md](./needs.md)). At that point `runFoodFocus` tries, every turn, to
eat a meal it has, fetch one of its own, buy one, or be rationed one (see
[food.md](./food.md)). If none of those works, and the safety net is off,
the turn goes to `hungryWithoutFood`.

Pressing is the latest a colonist can start. It has about 175 ticks to
critical hunger and about 40 more to death, and in that time it has to dig,
scrape, walk the scum home, cook it and eat it. Waiting for critical
starved colonists with work half done.

### One plan, seen through

`hungryWithoutFood` keeps a job that already makes food: cooking or
scraping for itself (`feedingItself`), or cooking meals for anyone
(`makingMeals`). Any other job is dropped, once. Then `planForage` picks a
job and marks it `foraging`. A foraging job runs until it ends, and only
then does the colonist plan again. Unrelated work is never taken while
hungry.

`planForage`, in order:

1. **No scumhouse it can reach**: build one (`tryEmergencyScumhouse`).
2. **Something to cook**: its own scum in a scumhouse, or the colony's while
   the colony is short. The colony's meal goes on sale, where the colonist
   can buy it or be rationed it.
3. **Enough meals on the stove**: the colony's cooks, at scumhouses it can
   reach, will make at least as many meals from the stock at their stoves
   as there are hungry colonists without one (`foodCooking`). It waits for
   them (plans nothing) rather than digging. A meal bought while foraging
   still ends the forage at once, since `runFoodFocus` looks for food first
   every turn.
4. **A meal's worth of scum in hand**, counting what it already has in the
   scumhouse: take it in to cook.
5. **A pack with no room for scum**: unload all the rock and ore in one trip
   to the nearest chest (`tryForageUnload`).
6. **Exposed scum**: scrape it, and keep it (`tryForageScrape`).
7. **Prospect**: dig into rock nobody has seen (`tryProspect`).
8. **A little scum in hand and nothing else to do**: bank it in the
   scumhouse, so the next find makes a meal.

When nothing applies it waits and looks again after `forageRetryTicks` (8).
It still checks for food every turn. The first plan of each hunger logs
"… has nothing to eat and goes looking for scum."

A forager doesn't walk one unit of scum home. `finishScraping` lets it keep
a part load and look for more, until it holds a meal's worth (two units).

### Where to dig

Scum covers a fixed share of the rock (`scum-percent`), but only rock next
to discovered floor can be scraped. Worldgen stops adding patches once the
generated chunks hold their share, so on a big map the colony's walls get
scraped bare while thousands of patches sit a tile or two into the rock.
Digging a tile reveals its eight neighbours (see
[fog-of-war.md](./fog-of-war.md)) and exposes any scum on them. So
`tryProspect` picks the frontier rock that reveals the most unseen tiles
per tick of walking to it and digging it:

```
fresh / (dig ticks + distance)
```

`fresh` is `unexploredAround(p)`. The rule doesn't dig rock that has
nothing unseen round it, and it tunnels outward (three fresh tiles a dig)
rather than squaring off a room. The comparison is cross-multiplied, and
ties go by position, so the frontier map's iteration order never decides
the dig.

### The colony prospects too

`assignWorkJob` prospects for the colony, as ordinary paid mining, when all
of these hold:

- the colony wants food (`foodWanted`: short of `meal-reserve`);
- no exposed patch is unclaimed (`prospectingForFood`);
- the colonist has nothing ahead of mining.

Its slot comes right after scraping for the colony, so construction,
cooking, cleaning and scraping all come first. It runs before anyone is
hungry, which is what keeps a big colony fed (see below).

### The starvation grace

`applyStarvation` doesn't drain HP from a colonist cooking its own supper
(`JobCraft` for itself, on a meal recipe). The scum is already in the
scumhouse, the same way a colonist walking to its own meal (`JobEat`) is
already covered. A forager who dug the scum out, scraped it and carried it
home used to die at the stove, one recipe short of the meal.

## Why it is this way

- **The doom loop.** `hungryWithoutFood` used to drop any work that wasn't
  food work once hunger was pressing. Finding no food work, it called
  `assignWorkJob`, which handed the same dig straight back. The next turn
  dropped it again, and the job never got past its first tick. It also has
  a guard for "growing" hunger that never ran, since the eat focus isn't
  eligible before pressing. On seed 1790737522337000000 (web game,
  10000×10000) every colonist stood "walking to" the same rock for 200
  ticks and starved, with about 8,900 scum patches in the generated rock.
  This was a job flicker, not a focus flicker: the focus stayed "eat" the
  whole time. `TestAHungryColonistFinishesWhatItStarts` pins it and fails
  on the old code.
- **Waiting for the colony's cooking, not a neighbour's, and only if
  there's enough.** The first version waited whenever any cook was at work.
  A forager then stood beside 40 units of scum it had just dug out, waiting
  for a neighbour's own supper, which it could never buy. Counting only the
  colony's cooks, a 100-colonist colony still had 30 hungry colonists
  waiting on stoves that made one meal at a time, beside scum they could
  have scraped. `foodCooking` now compares the meals coming with the number
  of hungry colonists.
- **A full pack.** Miners carry seven stacks of rubble. Plain rock still fit,
  so a forager dug past patch after patch it had no room to scrape. It now
  unloads first. `tryAssignStore` takes ore to the silo on one trip and the
  rest to a chest on the next, which is fine for a miner and cost a forager
  the walk that would have fed it; `tryForageUnload` does it in one trip.
- **Hungry foragers alone weren't enough.** With only hungry colonists
  foraging, a 20-colonist colony on a 1000×1000 map still ran out: its
  exposed scum went at about tick 12,000–16,000, stored meals followed, and
  most of the colony was hungry at once. Ordinary mining exposes new scum at
  about half the rate 20 colonists eat it. Prospecting once the colony is
  short, before anyone is hungry, is what closed the gap.
- **Not scum that favours the colony.** Regrowth lands anywhere in the
  generated chunks (see [scumhouse.md](./scumhouse.md)). Making it grow near
  the colony was the other option. Colonists going to find it keeps the
  world honest, and gives digging a reason.

### Measured

Colonists starved, with the default settings (40,000 ticks except where
noted):

| Scenario | Before | Hungry foragers only | Plus colony prospecting |
| --- | --- | --- | --- |
| Web game, 10000×10000, seeds 0–15 (0 = 1790737522337000000), 60,000 ticks | 43 | 19 | 6 |
| 20 colonists, 1000×1000, seeds 1–8 | 148 of 160 | 122 | 74 |
| 100 colonists, 300×150, seeds 1–4, 30,000 ticks | 400 of 400 | — | 373 |
| Default 80×40, seeds 1–32 | 66 | 64 | 66 |

On seed 1790737522337000000 alone, all six colonists starved by tick
18,000 before. After, none starve in 60,000 ticks; four are still alive.

Two scenarios barely move, for reasons foraging can't reach:

- **The default 80×40 map.** Its colonies mine out every rock tile by
  about tick 12,000. Scum only grows on rock, so once the last patch is
  scraped nothing comes back, and there is nothing left to prospect.
- **100 colonists.** Kitchens can't keep up. During the famine (ticks
  6,000–8,000 on seed 1), hundreds of units of uncooked scum sat in
  scumhouse depots, much of it in chefs' own kitchens, which other
  colonists may not cook in. Every meal order was a hungry colonist's bid,
  with nothing for sale. The colony starved completely before this change
  too; `food.md`'s older figure of 49 of 400 predates cave-scum accretion
  and own workshops.

Prospecting also breaks into more natural caverns, and some hold alien
nests (see [caverns.md](./caverns.md)). In one run of the seed above, three
colonists who had survived the famine were killed by a nest the colony
dug into. Digging
into the unknown carries that risk.

## Extending it

- **A new food source** (a fungus on cavern floor, say): add a step to
  `planForage` above prospecting. Anything that gets a hungry colonist food
  belongs there, not in `assignWorkJob`, so it can't loop.
- **Smarter prospecting**: `tryProspect`'s score is the place, for example
  weighting rock beside patches the colony saw being scraped. Keep it a pure
  function of positions with a total order, for determinism.
- **Invariant**: while hungry, a colonist never takes a job that doesn't
  lead to food, and never drops a foraging job halfway. Break either and the
  loop comes back.

## Related

- [food.md](./food.md) — what a hungry colonist eats, and in what order.
- [scumhouse.md](./scumhouse.md) — scum, its growth, scraping and cooking.
- [fog-of-war.md](./fog-of-war.md) — what counts as seen.
- [needs.md](./needs.md) — hunger's phases and starvation.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — the
  focus arbiter. Foraging runs inside the eat focus.

# Food

> Part of the [mars-sim documentation](./README.md).

## What it is

Food is an item now: a `Meal`. A hungry colonist eats real meals when it has
them — its own first, then one it buys — and falls back on a nutrient pod's
free gruel only when it has none. That fallback is the **safety net**
(`infinite-food`), **off by default** since economy phase E8: pods feed
nobody, and the colony lives on what it landed with and what it makes. The
safety net stays a setting, for tests and balancing. This is the second half of phase
**E2** of the [economy plan](./economy.md); [crash-pods.md](./crash-pods.md)
covers where the first meals come from.

## Source

- [`internal/sim/food.go`](../internal/sim/food.go) — `runFoodFocus`,
  `tryStartEating`, `nearestMealDepot`, `jobEat`, `takeMeal`,
  `hungryWithoutFood`, `podsFeed`, `wantsFacility`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `runNeedFocus`
  hands food to `runFoodFocus` first; `finishUse` records gruel; rats only eat
  at pods that feed.
- [`internal/sim/needs.go`](../internal/sim/needs.go) — the starvation grace
  covers `JobEat`.
- [`internal/sim/project.go`](../internal/sim/project.go) — `toiletRoom`,
  `facilityRoomRecipe`.
- [`internal/sim/property.go`](../internal/sim/property.go) —
  `StorageContainer.debit`, which takes a meal out of a depot.
- [`internal/sim/food_test.go`](../internal/sim/food_test.go).

## How it works

### Who eats what

When the food need is pressing, `runNeedFocus` gives `runFoodFocus` the turn
first. It tries, in order:

1. **A meal in the colonist's pockets**, usually its pocket meal (see *Pocket
   meals*). Eat it where it stands.
2. **A meal of its own in a depot it can reach** — its crash pod's locker, to
   begin with. Walk there, take one out (`debit`), step aside, eat it.
3. **A meal bought.** The cheapest on offer at the silo or a scumhouse it can
   reach, up to its `mealBidLimit` (see [valuation.md](./valuation.md)). The
   colony's scumhouses sell what they cook (see [scumhouse.md](./scumhouse.md)).
   The meal is then the colonist's own, and step 2 fetches it. A colonist does
   **not** eat the colony's meals free: those are for sale.
4. **A ration.** With `rations` on (the default) and the safety net off, a
   colonist at **critical** hunger that could not buy is given one of the
   colony's meals from the nearest reachable depot holding one (`tryRation`).
   The meal changes hands on the ledger (`moveLine`) and step 2 fetches it.
5. **The safety net.** Only if `infinite-food` is on: the old `JobUse` at a
   nutrient pod, which makes gruel out of nothing.

Steps 1–4 end in `JobEat`, with two stages: `eatFetch` walks to the depot at
`Target`, `eatMeal` eats the meal in hand for the food need's `UseTicks`. A meal
of the colonist's own is never skipped for the pod, even if the pod is closer —
`TestColonistsEatTheirOwnMealsBeforeGruel` checks every tick of a 4000-tick run
that nobody with a meal of its own is queued at a pod.

A colonist never takes another colonist's meal: `takeMeal` only debits the
colonist's own ledger line, and `debit` refuses to take more than a line
holds.

### Gruel

Eating at a pod emits `colonist / eat / gruel` ("Ate a ration of nutrient-pod
gruel."), which the `ate-gruel` reaction in `cognition.yaml` answers, instead
of `eat / meal` and `ate`. Its appraisal is a small, slightly dispiriting non-event
next to a real meal's lift, and it wears into a mild grievance. The safety net
is meant to be the worst way to eat: it keeps a colonist alive, and not much
more.

### With the safety net off

`podsFeed` is false, so:

- a nutrient pod serves nothing — `jobUse` drops a food job at one, and rats
  live entirely on the bodies, gore, and scum they scavenge, in competition
  with the scumhouse (see [entities-and-ai.md](./entities-and-ai.md));
- the colony stops planning pods: `wantsFacility(NutrientPod)` is false, and
  a facility room becomes `toiletRoom`, all toilets;
- a hungry colonist with nothing to eat picks food work first — cooking its
  own scum, then scraping to keep (`hungryWithoutFood`). Once hunger is
  pressing it drops any other work under way to do so (`feedingItself`),
  except cooking the colony's meals (`makingMeals`), which makes the meal it
  will buy; before that it finishes what it started. If there is no
  scumhouse it can reach, it helps build the planned one, or raises one
  itself, unpaid (`tryEmergencyScumhouse`), the scarcity version of the
  emergency pod. Otherwise it keeps working, and looks for food again every
  turn, rather than waiting by an empty locker;
- the colony builds a scumhouse before anything else, and it does not wait on
  money: a colony that cannot fund it marks it out as unpaid community work
  (see [labor.md](./labor.md)).

`TestWithoutTheSafetyNetTheColonyStarvesOnSchedule` is the scarcity under
test. With three meals each and nothing produced, nobody may die before the
manifest runs out — two full meal cycles, the last meal, then 500 ticks for
hunger to climb from 0 to its max and 40 more to drain a colonist's HP — and
everyone must be dead within a few walks of the prediction. It also checks that
no pod was built and nobody queued at one.

### Rations, and why food work starts at pressing

Late in 40,000-tick runs with 20 colonists, the colonists who starved were
broke, holding a few dollars when a meal costs $5. They died a few tiles from
shelves holding a hundred of the colony's meals, scraping scum for a supper
they didn't live to cook. Two changes followed:

- **Food work starts at pressing hunger, not critical.** Waiting for critical
  left too little time to scrape, haul, and cook. The reviewer's suggestion
  was critical; pressing is what the long runs needed.
- **A ration at critical hunger.** It's a lifeline, not a living: only after
  buying failed, one meal at a time.

The first version of `tryRation` used `debit` then `credit`. `debit` takes the
physical meal as well as the ledger line, so the meal vanished and a phantom
stayed on the colonist's line. The colonist then walked to that shelf forever,
failed to take a meal that wasn't there, and never fell through to buying.
`TestTheColonyRationsTheStarving` checks `ledgerBalanced`.

Dropping work at pressing hunger once caused a livelock of its own. A hungry
colonist cooking the colony's scum isn't feeding itself, so
`hungryWithoutFood` dropped the job. With no scum of its own to cook,
`assignWorkJob` then handed it the same colony cooking job, and the next turn
dropped it again. The recipe never got past its first tick, and a colony's
last colonists starved at the stove beside 14 units of its scum, with money
to buy the meals they weren't finishing. Cooking meals for anyone is now food
work that hunger doesn't interrupt (`makingMeals`). A colony cook already stops
its batch once it's hungry (`cooksOn`), so this holds it for one recipe, not a
shift. `TestPressingHungerFinishesTheColonysCooking` pins it.

### Pocket meals

A colonist whose hunger reaches `pocket-meal-at` (300; it eats at 650) with no
meal on it, and a meal of its own in a depot it can reach, fetches that meal
to carry when it next picks work (`tryPocketMeal`, at the top of
`assignWorkJob`). It's a `JobEat` fetch with `eatKeep` set: the meal goes in
the pocket and the job ends. When hunger turns pressing, step 1 of *Who eats
what* eats it where the colonist stands. From pressing hunger to critical is
about 175 ticks, and about 40 more to death; on a big map, one walk to a
locker used most of that. With a pocket meal, the walk happens while there's
time to spare.

A pocket meal is only ever the colonist's own. It never buys one: a colonist
that isn't hungry yet buying a meal takes it off the shelf from one that is.
Measured on the 100-colonist shortage below (seeds 1–4, 30,000 ticks), buying
pocket meals starved 150 of 400 colonists, against 96 without pocket meals.
Own meals only starved 97, and on seeds 1–16 of the default six-colonist
colony nobody starved either way. So the pocket meal doesn't prevent a
shortage. It moves a colonist's own walk earlier, and the shortage is a
production problem (see *Die-offs in big colonies*).

Selling surplus meals (`tryAssignSellMeals`) never sells the pocket meal:
surplus counts at least one meal kept back.

### Die-offs in big colonies

A 100-colonist colony on a 300×150 map loses a quarter of its colonists
around tick 14,000–16,000 on every seed tried. Stored meals fall at a steady
rate from the start: production runs at about 85–90% of what 100 colonists
eat, and the stock only hides the shortfall. When the shelves empty, colonists
die until the colony is small enough to feed, 70–77, and stock recovers.

More stock makes it worse, not better (seeds 1–3, 30,000 ticks, starved of
300):

| Setting | Starved |
| --- | --- |
| Defaults | 79 |
| `founding-grant` 15,000 | 84 |
| `meal-reserve` 6 | 132 |
| `crash-pod-meals` 20 | 172 |

Two things hold production back. The colony's standing bids for scum are
funded from the treasury, which building rooms for 100 colonists drains to $0
by about tick 4,000, while colonists still eating their pod meals buy nothing.
And the colony sells every meal at the reference price however low its stock
is, so a shortage never raises the price that would draw more producers in.

### Interruptions

In `eatMeal` the meal is in hand: out of the pockets and off the ledger both.
If the job is cleared — an alien comes round the corner — `clearJob` puts it
back in the colonist's pockets, so a meal is never lost to a fright. The
starvation grace (`applyStarvation`) covers a colonist in `JobEat` the way it
covers one queued at a reachable pod.

## Why it is this way

- **Meals are items in depots, not charges on a pod.** An item can be owned,
  traded, carried, and produced. That is the whole reason for the change: the
  order book (E4) needs something to buy.
- **Own, then colony, then gruel.** The order is hard-coded rather than scored
  because there is no market yet to price the difference. When there is, the
  choice becomes "eat my own, or buy one" with a price on each.
- **Keep working while hungry.** Idling beside an empty locker helps nobody.
  Once the scumhouse exists, a hungry colonist with nothing to eat is the one
  most motivated to go and make food.
- **A bug worth remembering.** The first version put the finished meal back in
  the pocket: `clearJob` saw a meal "in hand" at the end of `jobEat` and
  treated it as an interrupted meal. Every colonist ate forever from a locker
  that never emptied. The food tests caught it because they count what each
  colonist owns rather than trusting that eating happened.

## Extending it

- **A new food**: an `ItemKind` and a producer. Eating does not care what
  kind of meal it is today; if foods differ (a better meal, a worse one), give
  each its own life event and pick one in `jobEat`.
- **Buying food**: a fourth source between "the colony's" and the safety net,
  once the order book exists (E4).

## Related

- [needs.md](./needs.md) — the food need, its thresholds, and starvation.
- [crash-pods.md](./crash-pods.md) — the meals a colonist lands with.
- [property.md](./property.md) — ledgers, `debit`, and who may use what.
- [construction.md](./construction.md) — the facility room, and what it holds
  with the safety net off.
- [economy.md](./economy.md) — the plan this is part of.

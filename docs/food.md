# Food

> Part of the [mars-sim documentation](./README.md).

## What it is

Food is an item now: a `Meal`. A hungry colonist eats real meals when it has
them — its own first, then the colony's — and falls back on a nutrient pod's
free gruel only when it has none. That fallback is the **safety net**, on by
default (`infinite-food`); with it off, pods feed nobody and the colony lives
on what it landed with and what it makes. This is the second half of phase
**E2** of the [economy plan](./economy.md); [crash-pods.md](./crash-pods.md)
covers where the first meals come from.

## Source

- [`internal/sim/food.go`](../internal/sim/food.go) — `runFoodFocus`,
  `tryStartEating`, `nearestMealDepot`, `jobEat`, `takeMeal`,
  `hungryWithoutFood`, `podsFeed`, `wantsFacility`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `runNeedFocus`
  hands food to `runFoodFocus` first; `finishUse` records gruel; mice only eat
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

1. **A meal in the colonist's pockets.** Eat it where it stands.
2. **A meal of its own in a depot it can reach** — its crash pod's locker, to
   begin with. Walk there, take one out (`debit`), step aside, eat it.
3. **A meal the colony owns in a depot it can reach.** Anything the community
   owns, everyone may use (see [property.md](./property.md)). Nothing produces
   community meals yet; the scumhouse will.
4. **The safety net.** Only if `infinite-food` is on: the old `JobUse` at a
   nutrient pod, which makes gruel out of nothing.

Steps 1–3 are `JobEat`, with two stages: `eatFetch` walks to the depot at
`Target`, `eatMeal` eats the meal in hand for the food need's `UseTicks`. A meal
of the colonist's own is never skipped for the pod, even if the pod is closer —
`TestColonistsEatTheirOwnMealsBeforeGruel` checks every tick of a 4000-tick run
that nobody with a meal of its own is queued at a pod.

A colonist never takes another colonist's meal: `takeMeal` only debits the
colonist's own ledger line or the colony's, and `debit` refuses to take more
than a line holds.

### Gruel

Eating at a pod records `EvtAteGruel` ("Ate a ration of nutrient-pod gruel.")
instead of `EvtAte`. Its appraisal is a small, slightly dispiriting non-event
next to a real meal's lift, and it wears into a mild grievance. The safety net
is meant to be the worst way to eat: it keeps a colonist alive, and not much
more.

### With the safety net off

`podsFeed` is false, so:

- a nutrient pod serves nothing — `jobUse` drops a food job at one, and mice
  (which only ever ate at pods) go hungry;
- the colony stops planning pods: `wantsFacility(NutrientPod)` is false, and
  a facility room becomes `toiletRoom`, all toilets;
- a hungry colonist with nothing to eat keeps working (`hungryWithoutFood`)
  and looks for food again every turn, rather than waiting by an empty locker.

`TestWithoutTheSafetyNetTheColonyStarvesOnSchedule` is the scarcity under
test. With three meals each and nothing produced, nobody may die before the
manifest runs out — two full meal cycles, the last meal, then 500 ticks for
hunger to climb from 0 to its max and 40 more to drain a colonist's HP — and
everyone must be dead within a few walks of the prediction. It also checks that
no pod was built and nobody queued at one.

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
- **Feeding mice without pods**: mice only ever ate at pods; with the safety
  net off they starve. Cave scum is the obvious thing for them to eat once it
  exists.

## Related

- [needs.md](./needs.md) — the food need, its thresholds, and starvation.
- [crash-pods.md](./crash-pods.md) — the meals a colonist lands with.
- [property.md](./property.md) — ledgers, `debit`, and who may use what.
- [construction.md](./construction.md) — the facility room, and what it holds
  with the safety net off.
- [economy.md](./economy.md) — the plan this is part of.

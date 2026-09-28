# Communal work on the order book (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan to remove the **community ladder**: the fixed list of work every
colonist tries, in the same order, before it considers its own interest.
Every communal job becomes an order on the book, with pay, posted by the
colony. A colonist chooses among colony work, work other colonists post, and
market plans in **one** comparison, by what each pays per tick of its own
time. The colony stops commanding work and starts bidding for it, and how
urgent a job is shows in what it pays, not in where it sits on a list.

Nothing here is built. It carries out the direction set in
[skills.md](./skills.md): colonists act in their own interest, compete, and
weigh opportunity cost. It replaces that doc's phase S4 (*Wages in the same
comparison*).

| Phase | What ships | Status |
| --- | --- | --- |
| W1 | One chooser: work orders and market plans in one candidate list, ranked by rate. The ladder's existing paid work (build tasks, haul orders) moves onto it. | Proposed |
| W2 | Every communal job becomes an order: cleaning, burning corpses, cooking for the colony, demolition, frontier digging. | Proposed |
| W3 | The colony's willingness to pay: wages that rise with urgency, and a budget. | Proposed |
| W4 | Delete the ladder. What's left of `assignWorkJob` is survival and finishing what the colonist started. | Proposed |

## Source

What changes, when built:

- [`internal/sim/systems.go`](../internal/sim/systems.go) — `assignWorkJob`,
  the ladder, shrinks to survival and commitments, then one call to the
  chooser.
- [`internal/sim/workorder.go`](../internal/sim/workorder.go) — new
  `WorkKind`s; the colony's wage-setting (`colonyWage`).
- [`internal/sim/producer.go`](../internal/sim/producer.go) —
  `tryAssignProduce` becomes the chooser over plans *and* work orders.
- [`internal/sim/cleaning.go`](../internal/sim/cleaning.go),
  [`scumhouse.go`](../internal/sim/scumhouse.go),
  [`project.go`](../internal/sim/project.go) — the colony posts orders for the
  work these files assign today.
- `internal/sim/jobboard.go` — per-kind nearest-order lookups, so candidate
  retrieval stays bounded.

## Decisions already made

| Topic | Decision |
| --- | --- |
| The ladder | Goes. Communal work isn't a special case. It's orders on the book, and colonists choose it the way they choose anything else. |
| Self-interest | A colonist does colony work because it pays, not because it's first on a list. |
| Competition | Colonists compete for work as for bids (see [skills.md](./skills.md), S5). |

## What the ladder does today

`assignWorkJob` tries each rung in turn and takes the first that yields a
job. How each rung is paid:

| Rung | Communal? | How it's paid today |
| --- | --- | --- |
| Deliver a plan's finished goods (`tryDeliverPlan`) | No: finishing its own deal | By the sale |
| Unload full pockets (`tryAssignStore`), or build the storage room that would take them | Unloading is personal; the room is communal | Room tasks pay their build order's wage |
| Construction (`claimNearestTask`) | Yes | A `WorkBuild` order per task at `wage-dig`/`wage-wall`/`wage-fixture`. But it's chosen first by everyone, whatever it pays. The colony's first scumhouse under scarcity is unpaid (issuer `Nobody`) |
| Cook the colony's biomatter (`tryAssignCraft`, while `foodWanted`) | Yes | `wage-cook` per recipe, transferred straight from the treasury. Not an order |
| Clean gore and corpses (`tryAssignClean`) | Yes | Viscera and animal/alien corpses are sold into the colony's biomatter bids on delivery. A colonist's corpse burned in the incinerator pays nothing |
| Scrape scum for the colony (`tryAssignScrape`, while `foodWanted`) | Partly | Sold into the colony's standing scum bid. Already a market sale, but started by the ladder |
| Sell surplus pod meals (`tryAssignSellMeals`) | No | By the sale |
| Haul for hire (`tryAssignHaul`) | Often (the colony's silo stock) | A `WorkHaul` order |
| The producer planner (`tryAssignProduce`) | No | By the sale |
| Cook its own scum (`tryAssignCraftFor`) | No | Eats it |
| Mine the frontier | Yes: it opens the colony up | Only through ore sold into the prospecting bids. Plain rock pays nothing |

Before any of this, survival runs outside the ladder: fleeing, fatal needs,
the emergency build for a need nothing else can meet, escaping a sealed room,
and a hungry colonist cooking its own scum (`tryAssignFoodWork(e, force)`).

So the ladder mixes three different things: survival and personal
commitments, which are fine where they are; communal work, which is the
target; and market work, which the planner should already own.

## How it works (proposed)

### A colonist's turn

```
1. Survival          flee, fatal needs, emergency build for its own need, escape.
                     Unchanged. Not a market decision.
2. Commitments       finish the unit it claimed; deliver goods a plan made;
                     unload if its pockets block work. Personal, and cheap.
3. Choose            one candidate list, one rate per candidate, best rate that
                     beats its reservation (see skills.md, S3).
4. Nothing clears    idle: rest, talk. An idle colonist in a demand-limited
                     colony is the correct outcome, not a bug.
```

### The candidate list

Every candidate is valued the same way:

```
rate = (pay − input cost) / (walk ticks + work ticks × own TicksPct / 100)
```

For a work order, pay is its wage per unit and there's no input cost. For a
plan, it's the bid's price times the output, minus what the inputs cost, as
the planner reckons it today. Own-account options (cook its own scum, sell a
surplus meal, dig ore to sell) are plans with the colonist as buyer or
seller.

The list stays bounded (principle 9), so the chooser never scans every order:

- the best `plan-candidates` goods bids, as today;
- the nearest open unit of each `WorkKind`, from a per-kind index next to the
  job board ([spatial-index-and-performance.md](./spatial-index-and-performance.md)).
  That's one candidate per kind, so the list grows with the number of
  **kinds** of work, not the number of orders;
- the colonist's own-account options.

Ties break by order ID, then position. Nothing ranges over a map to choose
([determinism.md](./determinism.md)).

### Every communal job is an order

Work orders already exist (`WorkBuild`, `WorkHaul`; see
[labor.md](./labor.md)): pay escrowed from the issuer at posting, paid per
unit on completion. W2 adds the kinds the ladder assigns without one:

| New kind | Unit | Posted by the colony when |
| --- | --- | --- |
| `WorkClean` | one tile of gore, or one corpse, to its destination | refuse is on the floor |
| `WorkBurn` | one colonist corpse to the incinerator | a colonist dies |
| `WorkCook` | one recipe run on the colony's biomatter | the colony's meals are below its reserve and it holds biomatter |
| `WorkDemolish` | one tile | the planner marks one |
| `WorkDig` | one frontier tile | the colony wants to grow |

Cleaning keeps its sale. The cleaner owns the viscera it scrubs up and sells
it into the biomatter bid, so a `WorkClean` wage is on top of the sale, and
can be $0 while biomatter prices alone make the job worth it. Burning a
colonist's corpse has no sale, so it's the one that most needs a wage.

Claims stay, per **unit**: two colonists can't scrub the same tile. But every
unit of every order is visible to every colonist, and a unit's claim lapses if
its taker drops the job. Competition is over which colonist takes which
units, and whether it's worth taking them at all.

`WorkCook` is a hiring decision by the colony: it owns the biomatter and
pays for labor. The more market-shaped alternative is for the colony to sell
its biomatter and bid for meals, leaving cooking to whoever does it
cheapest. Start with hiring, because it's the direct translation of
`wage-cook`. Moving to the pure market is then a change to the colony's
policy, not to the chooser.

### The colony's willingness to pay (W3)

The ladder's order was the colony's priorities. Without a ladder, those
priorities have to be prices. Like a hungry colonist's meal bid
(`mealBidLimit`, [valuation.md](./valuation.md)), the colony's wage for a
kind of work rises with its need for it:

| Work | Urgency |
| --- | --- |
| Life-support construction (the first scumhouse, missing toilets, beds) | Highest base wage. Rises with how long colonists have gone without the facility |
| Cooking | Rises as meals fall below `meal-reserve` per colonist |
| Cleaning, burning | Rise with how long the mess has lain there (it already costs everyone who walks past it mood) |
| Other construction, demolition, digging | Base wage |

`colonyWage(kind, urgency)` is an integer function of state the world
already tracks, so it adds no RNG and no per-tick scan. An open order whose
wage should rise is reposted at the new wage when the colony's upkeep runs.
Escrow makes a raise explicit: the colony tops up the escrow, or it can't.

**The budget.** Everything the colony posts is escrowed from the treasury,
so the treasury rations it, cheapest priorities first to go. Urgency raises
what the colony offers per unit, and it also decides what gets funded when
money is short.

### What's left of `assignWorkJob` (W4)

Survival, commitments and one call to the chooser. The rungs listed above are
deleted, and so is `foodWanted` as a gate on anyone's behavior. It becomes an
input to the colony's cooking wage.

## Why it is this way

- **One comparison.** With a ladder, the rungs above the planner are free of
  any cost-benefit test, so a Master smith raises a wall at $2 because walls
  come first. Put every job in one list and "what should I do" has one answer
  per colonist: whatever pays it best.
- **Urgency is price.** The colony still has priorities, but it has to pay
  for them. A priority it can't afford doesn't happen, which is honest. A
  ladder hides that, and it will make colonists work for nothing.
- **Survival stays outside.** A starving colonist feeding itself isn't
  trading with anyone. Pricing that would only let a pricing bug kill
  someone (principle 7).
- **One candidate per kind of work.** Offering every open unit to every
  colonist would cost a scan per decision at 2,000 colonists. Nearest per
  kind is what the job board already knows how to answer.
- **Claims per unit, not per opportunity.** A tile can only be scrubbed once,
  so a unit claim is physical. What S5 in skills.md removes are claims that
  hide an opportunity from others, not ones that stop two colonists doing the
  same physical work.

## Open questions

- **An empty treasury.** Today unpaid communal work still gets done because
  the ladder says so. On the book, a broke colony gets no cleaning, no
  burning, and no public works. The wealth levy ([money.md](./money.md))
  refills the treasury over time. Is that fast enough, or does the colony
  need taxes first? Paying wages **in kind** (meals from the colony's stock)
  is a stopgap that fits the market: a broke colony with food can still hire.
- **The first scumhouse.** It's unpaid under scarcity today because life
  support can't wait on money. With no ladder, whoever builds it needs a
  reason. The emergency build for its own need covers a starving colonist
  but not a colony that isn't starving yet.
- **Unpaid civic work.** Some colonists might clean for nothing because
  it's their colony. That's identity, not self-interest, and it belongs with
  the identity system ([economy.md](./economy.md)), not the chooser.
- **Frontier digging.** Should the colony pay to explore at all, or should
  exploring be left to prospectors chasing ore? Paying keeps the colony
  growing when ore is scarce. Not paying is the purer market.
- **Tuning.** Wages were set when construction came first regardless of
  pay. On the book, $2 a wall competes with $4 ore and $11 meals, and may
  never be taken. The first W3 run will need `-econ-trace` work.

## Related

- [skills.md](./skills.md) — opportunity cost, reservation rates, competition
  and workshop ownership; this doc replaces its S4.
- [labor.md](./labor.md) — work orders, public works, wages.
- [valuation.md](./valuation.md) — the producer planner, which becomes the
  chooser.
- [economy.md](./economy.md) — the plan behind all of it.
- [spatial-index-and-performance.md](./spatial-index-and-performance.md) —
  the job board the per-kind index extends.
- [design-principles.md](./design-principles.md) — principles 6, 7 and 9.

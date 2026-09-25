# Labor orders

> Part of the [mars-sim documentation](./README.md).

## What it is

Work is bought. A **work order** is pay, escrowed from its issuer when it is
posted, for a unit of work — raising one build task's tile, or delivering one
unit of biomatter to a scumhouse — and whoever does the work is paid when it
is done. The colony is an issuer like anyone else: the room planner still
decides what the colony needs, but it now buys the work from the treasury, so
an empty treasury halts public works. A colonist can commission a room the same
way from its own wallet and owns what gets built — a house with a private bunk
and a toilet it rents out by the use. This is phase **E5** of the
[economy plan](./economy.md).

## Source

- [`internal/sim/workorder.go`](../internal/sim/workorder.go) — `WorkOrder`,
  `postWork`, `payWork`, `closeWork`, `cancelWorkOf`; `wageFor`,
  `projectCost`, `fundProject`; `houseRoom`, `fixtureAccess`,
  `commissionHouses`; the biomatter bounty (`refreshBiomatterBounty`,
  `payBounty`).
- [`internal/sim/project.go`](../internal/sim/project.go) — `planRoomFor` and
  `designateRoom`, which fund a room before anything about it is marked out;
  `project.issuer`, `buildTask.order`/`proj`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `jobBuild` pays on
  completion and hands a commission's fixtures to its commissioner;
  `finishUse` charges for a paid fixture.
- [`internal/sim/property.go`](../internal/sim/property.go) — `AccessPaid`,
  `Fixture.Price`, `setFixturePrice`, `chargeForUse`.
- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go) —
  `deliverBiomatter` pays the bounty.
- [`internal/sim/labor_test.go`](../internal/sim/labor_test.go).

## How it works

### Work orders

`postWork(kind, issuer, pay, units, pos)` moves `pay × units` from the issuer
into the order's own account (the unexported `ownerWork` owner, the same trick
market orders use — see [market.md](./market.md)), or refuses if the issuer
cannot fund it. `payWork` pays one unit to the worker and closes the order when
none is left; `closeWork` returns any remainder to the issuer. The money audit
counts work escrow with the market's: `circulating + frozen + escrowed ==
issued`.

### Public works

When the planner designates a room, `fundProject` posts one `WorkBuild` order
per task at `wageFor` its terrain — `wage-dig` for a tile of rock, `wage-wall`
for a wall, `wage-fixture` for a fixture — all paid by the project's `issuer`.
It is **all or nothing**, and it happens before anything about the room is made
permanent (its reserved door tile, its project ID): an issuer that cannot pay
for the whole room gets nothing marked out, and `planRoomFor` reports it. So:

- **an empty treasury halts public works**, and the planner retries each
  cycle;
- **survival does not stop**: a colonist whose need has no facility it can
  reach still builds one for itself, unpaid, exactly as before (the emergency
  build in `runNeedFocus`), and crash pods bring everyone a bunk and a toilet.
  `TestAnEmptyTreasuryHaltsPublicWorksNotSurvival` runs a colony with no
  founding grant for 5000 ticks: no room is ever planned, nobody starves, and
  somebody builds themselves a nutrient pod.

A builder is paid when its task's tile is raised (or dug out): `jobBuild` calls
`payWork` on the task's order.

### Commissions and houses

`planRoomFor(recipe, issuer)` works for anyone. Once per planning cycle,
`commissionHouses` has the first colonist (by ID) with `house-savings` dollars
and no house yet commission a `houseRoom` — a bunk and a toilet behind its own
walls — paid from its wallet. Anyone may build it and be paid from that escrow.
When a commission's fixture goes up, it becomes the commissioner's: the bunk
private, the toilet **paid** at `toilet-fee` a use.

If the commissioner dies, `cancelWorkOf` closes its work orders (the escrow
freezes with its money) and drops its unbuilt projects; what was already built
stays built, and stays its.

### Paid fixtures

`AccessPaid` is the third access policy (see [property.md](./property.md)). Its
owner uses it free; anyone else may use it if they can pay its `Price`, and
`finishUse` charges them when they are done. Like a private fixture, a paid one
is not a goal of the shared flow field; colonists who may use it route there
through `facilityReachable` and `chooseFacility`. A colonist that can no longer
pay by the time it is done has had its use anyway — there is no debt to run up
yet.

### The biomatter bounty

The colony keeps `bounty-units` units of `WorkDeliver` bounty open at each
scumhouse, at `bounty-pay` each, as far as the treasury stretches
(`refreshBiomatterBounty`, in the market's upkeep). `deliverBiomatter` pays the
deliverer per unit from the oldest open bounty. Scraping and cleaning still
happen with no bounty or no money — the colony eats the result — so the bounty
is income, not the reason the work gets done.

| Setting | Default |
| --- | --- |
| `wage-dig` / `wage-wall` / `wage-fixture` | 2 / 2 / 5 |
| `bounty-pay` | 1 |
| `bounty-units` | 30 |
| `house-savings` | 300 (0 disables commissions) |
| `toilet-fee` | 2 (0 makes a house's toilet private) |

A typical room costs the colony about a hundred dollars, so the founding grant
funds some fifty rooms, less what it spends on ore at the silo. In default
colonies over 10000 ticks, wallets settled between about 170 and 300 and the
occasional colonist bought itself a house.

## Why it is this way

- **Fund before designating.** A room planned but unpaid would sit on the map
  with its door tile reserved forever and its tasks claimable by nobody. Buying
  first keeps "planned" meaning "paid for".
- **All or nothing.** Paying for half a room produces half a room. An issuer
  that cannot afford the whole thing waits.
- **Emergency builds stay unpaid.** They are a colonist meeting its own need,
  not work for anyone else — and they are what keeps an empty treasury from
  becoming a famine.
- **Pay on completion, not on claim.** A claim is released when a colonist
  drops a task (fleeing, eating); paying then would pay for nothing.
- **Public works are not chosen by pay.** Everyone still takes the room
  planner's tasks in the planner's order; pay only lands in their wallets.
  Market work (filling bids) is chosen by profit, by the producer planner
  ([valuation.md](./valuation.md)); weighing wages against it is still to do.

## Extending it

- **A new kind of paid work**: a `WorkKind`, an issuer that posts it, and the
  executor that calls `payWork` when a unit is done.
- **More commissions**: any `roomRecipe` through `planRoomFor` with a colonist
  issuer; decide its fixtures' access in `fixtureAccess`.
- **Choosing work by pay**: the orders are all in `w.workOrders` with their
  pay. Making them producer-planner candidates would let a colonist weigh a
  wage against a bid (see [valuation.md](./valuation.md)).

## Related

- [economy.md](./economy.md) — the plan this is phase E5 of.
- [market.md](./market.md) — the goods side: orders, escrow, the silo.
- [construction.md](./construction.md) — rooms, recipes, and the planner.
- [property.md](./property.md) — owners, access, and paid fixtures.
- [money.md](./money.md) — accounts and the audit.
- [scumhouse.md](./scumhouse.md) — biomatter, and where the bounty is earned.

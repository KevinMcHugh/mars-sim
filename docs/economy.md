# Scarcity and economy (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

The plan for moving the colony from infinite, free life support to a
hardscrabble frontier economy: property with owners, colonists arriving in
**crash pods** with their own gear and a finite food supply, a digital dollar,
data-driven **recipes**, and a **bid/ask order book** for goods *and* labor
that any actor — a colonist or the colony itself — can post to.

The point is behavior, not bookkeeping. When a need gets harder to meet,
colonists should start doing interesting things to meet it: scraping cave scum
to sell to the scumhouse, renting out a spare toilet, bidding on a machine they
could make money with. The long-term goal is for supply chains to form on their
own, tick over tick, as the order book fills in backwards from finished goods
to raw materials.

This is a design and a sequenced build plan. When a phase ships, its content
moves into a present-tense doc and the phase below links to it; everything
else here is still unbuilt.

| Phase | Status |
| --- | --- |
| E0 — Money | **Shipped** — [money.md](./money.md) |
| E1 — Property | **Shipped** — [property.md](./property.md) |
| E2 — Crash pods and meals | **Shipped** — [crash-pods.md](./crash-pods.md), [food.md](./food.md) |
| E3 — Recipes and slurry | **Shipped** — [scumhouse.md](./scumhouse.md); construction costs in [construction.md](./construction.md) |
| E4 — Order book | **Shipped** — [market.md](./market.md) |
| E5 — Labor orders | **Shipped** — [labor.md](./labor.md) |
| E6–E8 | Proposed |

## Source

Nothing exists yet. The expected footprint, by phase:

- `internal/sim/money.go` — `Money`, wallets, the community treasury, transfers.
- `internal/sim/property.go` — `Owner`, the depot ledger, fixture records and
  access policies.
- `internal/sim/crashpod.go` — the pod prefab, its manifest, landing-site search,
  and the arrival path shared by worldgen, the spawn command, and the director.
- `internal/sim/recipe.go` — the recipe table and the workshop executor.
- `internal/sim/market.go` — orders, books, matching, escrow, settlement.
- `internal/sim/workorder.go` — labor orders (bounties) and their executors.
- `internal/sim/valuation.go` — reference prices, willingness-to-pay, and the
  producer planner.
- `internal/ui/tui/render_market.go` — the market/ledger view.

Existing code this builds on or replaces:

- [`inventory.go`](../internal/sim/inventory.go) — `ItemKind`, stacks, storage.
- [`project.go`](../internal/sim/project.go) — the room planner that becomes the
  community's buyer.
- [`combat.go`](../internal/sim/combat.go) — `equipColonyShip`, replaced by the pod
  manifest.
- [`director.go`](../internal/sim/director.go) — gains an arrival occurrence.
- [`needs.go`](../internal/sim/needs.go) and [`focus.go`](../internal/sim/focus.go) —
  eating from owned food; earning money as a focus candidate.

## Decisions already made

These were settled in the design conversation that produced this doc. Do not
re-open them without a reason the conversation did not have.

| Topic | Decision |
| --- | --- |
| Owners | Nobody (abandoned), the community, or an individual. Organizations are **not defined**; leave a commented-out `// TODO: OwnerOrganization` where the enum would grow. |
| Money | "Dollars". No physical presence, no inventory slot; every exchange is a digital transfer. |
| Money supply | Fixed for now: a **founding grant** to the treasury plus each colonist's arrival purse. Taxes come later. We accept that a fixed supply is deflationary and pushes toward conservative behavior; revisit when it shows up. |
| Arrivals | Every colonist arrives in a crash pod — at worldgen, from the spawn command, and from a new director occurrence. There is no other way in. |
| Food | The first produced food is **slurry**: colonists haul biomatter (alien and animal corpses, viscera, and cave scum/biofilm) to a **scumhouse** that turns it into meals. Viscera is viscera: it isn't told apart by where it came from. |
| Safety net | The infinite community nutrient pod stays, behind a config switch, for a long while. Players can also spawn food during development. |
| Recipes | Data, not code. |
| Skills | Placeholder only. Everybody can do everything for now; specialization is what makes the market more interesting later. |
| Specialization | Not yet, and we know it. With equal skills nobody sticks to a trade; see *Who does what work*. Skills and identity will each make colonists tend to keep doing the same kind of work. |
| Labor | Any actor can post a labor order: a colonist can order a house the same way the colony orders a town hall. |
| Prospecting | An evergreen job: anyone may dig for ore and sell it. At first the colony is the buyer; later other colonists will want the ore too. |
| The colony trades | The colony buys to encourage early production, then either sells what it bought or spends it on community construction. It can make money, not just spend it. |
| Hauling | In scope, not a someday. Buying at one depot and selling at another for more is valuable work, and colonists should make money hauling. |
| Crash-through rubble | The rock a pod displaces disappears. |
| Enforcement | Later. For now colonists obey property rules. |
| Death | Specify as little as possible. A dead colonist's property may just sit there, or be disposed of with the body. Inheritance is a big TODO because it drags in families, organizations, and law. |
| Market UI | A TUI market/ledger view ships early. It is how we'll see balance problems. |

## How it works

### Money

`Money` is an **integer** (`int64` whole dollars, or cents if we find we need
fractions). It must never be a float: settlement must stay exactly reproducible
for a given seed (see [determinism.md](./determinism.md)), and float sums depend
on the order they were added in.

Each colonist has a wallet on its `Entity`; the community has a treasury on the
`World`. Every change goes through one transfer function
(`transfer(from, to Owner, amount)`), which **for now** rejects a transfer that
would make a balance negative. That's a v1 limit, not a principle: debt and
lending are a planned future goal (see *Deliberately not doing*), and the
no-negative-balance check is the one line they will relax. A single funnel means one place to log, one place for a
future tax, and one place a test can check that money is conserved: with no
taxes, the sum of all wallets, the treasury, and escrow is constant after the
founding grant.

Where the starting money comes from:

- `founding-grant` — the treasury's starting balance.
- `crash-pod-purse` — dollars each colonist lands with.

A dead colonist's wallet just stays frozen (see *Deliberately not doing*).

### Owners and property

```go
type OwnerKind uint8
const (
    OwnerNone      OwnerKind = iota // abandoned: anyone may claim it
    OwnerCommunity                  // everyone may use it; the treasury trades it
    OwnerColonist
    // TODO: OwnerOrganization — an organization could own property. Undefined for now.
)
type Owner struct { Kind OwnerKind; ID EntityID } // ID only for OwnerColonist
```

Ownership is recorded differently by the kind of thing, because stacks have no
identity and should keep none:

| Thing | Where ownership lives |
| --- | --- |
| Fungible goods in a depot (ore, meals, biomatter) | The **depot ledger**: `(depot, owner, item) → count`. The container's physical stacks carry no owner. |
| Goods a colonist is carrying | The carrier owns them unless the current job has a **cargo record** saying who they belong to (a hauler moving your ore). Carrying isn't owning. |
| Placed fixtures (beds, toilets, scumhouses, storage containers) | A sparse **fixture record** keyed by position, like `World.storageContainers`: owner plus access policy. |
| Unique carried items (weapons) | The carrier, same as other carried goods, until a floor-item concept exists. |

The ledger is what the original "silo" idea generalizes to. Deposit a
commodity and the depot credits your ledger line; sell it and the line moves to
the buyer. Nobody has to physically sort *my* iron from *yours*. It needs one
invariant: for every depot and item, the sum of ledger lines equals the physical
count in the container. Withdrawals debit the ledger, and reserved (escrowed)
lines can't be withdrawn.

**Access policy** on a fixture decides who may use it:

- `AccessPrivate` — owner only (the crash-pod bunk).
- `AccessCommunal` — anyone (everything the community builds; today's behavior).
- `AccessPaid{price}` — anyone, for a per-use charge to the owner. This is how
  renting out your spare toilet becomes a business.

`chooseFacility` and the facility flow fields must learn to skip fixtures a
colonist may not use. That's the one expensive part: today one flow field per
facility terrain serves everyone. The v1 plan: the shared field keeps serving
communal fixtures, and a colonist heading to a private or paid fixture routes to
that one concrete tile with A\* (the same path `chooseFacility` already takes
once two facilities exist). See [pathfinding.md](./pathfinding.md).

In the first property phase everything the planner builds is community-owned
and communal, so behavior doesn't change. That's what makes the phase safely
shippable.

### Crash pods

A crash pod is a **prefab** stamped into the world when a colonist arrives: a
small shell holding a bunk, a toilet, and a locker (a storage container acting
as a depot), all owned by the colonist. The locker's ledger is credited with the
pod manifest:

| Manifest item | Config key (proposed) | Notes |
| --- | --- | --- |
| Meals | `crash-pod-meals` | Finite food supply, the core of the scarcity. |
| Weapon | `crash-pod-pistols` / `crash-pod-shotguns` | Replaces the colony ship's `pistols`/`shotguns` and `equipColonyShip`. |
| Dollars | `crash-pod-purse` | Goes to the wallet, not the locker. |

Stamping a prefab rather than unpacking fixtures one by one lets pods ship
**before** the placed-item ↔ carried-item conversion exists. Packing up a bunk
and moving it is a later feature; the fixture record already has everything it
would need.

All three arrival paths share one function, `w.arrive(pos)` (name open):
worldgen, `Engine.spawn(Colonist)`, and a new director occurrence
(`arrival`, with a `count`), so a later change to what a pod carries lands
everywhere. The existing `supply-drop` occurrence keeps working. What it hands
out is owned by whoever receives it.

**Landing site.** A pod needs a clear footprint on explored floor. Search the
way `findRoomSite` does (nearest to the colony center first, deterministic tie
order). If no clear site exists, the pod crashes through rock: the impact turns
its footprint to floor, and the displaced rock simply disappears (decided: it
doesn't go into the locker or anyone's inventory).
This keeps arrivals from failing on a cramped map, and it's decent flavor.

### Meals, eating, and the safety net

Food becomes an item, `Meal`. The nutrient pod stops being "a tile that resets
hunger" and becomes a **dispenser**:

- The **community nutrient pod** (the safety net, `infinite-food: true` by
  default) creates a meal out of nothing and serves it free, exactly as today.
- A **scumhouse** (below) holds real meals in its depot ledger and serves them
  according to its access policy.

A hungry colonist eats in this order: a meal it's carrying, a meal in its own
locker, a meal it can buy (once the market exists), then the safety-net pod.

The safety net competes with the market: while a free pod exists nobody buys
food, so the food market can't be tested with it switched on. Make it the worst
option: a longer `UseTicks` or a bad-tasting-gruel affect event
([affect.md](./affect.md)). Then colonists who can afford real food prefer it,
and the market gets used even while nobody can starve.

### Recipes and slurry

A recipe is a table row:

```go
type Recipe struct {
    Name     string
    Inputs   []ItemStack // consumed from the workshop's depot ledger, owned by the worker
    Outputs  []ItemStack // credited to the worker's ledger line at the workshop
    Facility Terrain     // workshop tile it runs at
    Ticks    int         // labor, scaled by workScale (and later, skills)
}
```

Owning the inputs means owning the outputs. A worker who uses someone else's
workshop pays that workshop's access price. The workshop's owner doesn't
automatically own what it produces. That's what lets a machine be capital
someone rents out rather than a job someone has.

**Skills** get one placeholder field (`Skill SkillKind`, always `SkillNone`),
so the table's shape doesn't change when expertise arrives.

The first chain:

```
alien corpse  ─┐
animal corpse ─┤
viscera       ├─► scumhouse (recipe: N biomatter → M meals)
cave scum     ─┘
```

This needs three new pieces of world content:

- **Split `Corpse` into kinds.** Today `Corpse` covers any dead body, and all of
  them go to the incinerator (see [sanitation.md](./sanitation.md)). Alien and
  animal (rat, cat) corpses become biomatter. **Colonist corpses must not.**
  They keep the incinerator route. So `Corpse` splits into `ColonistCorpse` and
  `AlienCorpse`/`AnimalCorpse`, and `Tile.Corpses` must remember which kind
  lies there.
- **Viscera becomes biomatter, all of it.** Viscera is viscera: gore scrubbed
  up after a colonist dies feeds the scumhouse the same as gore from an alien,
  so `Tile.Gore` stays one undifferentiated count and needs no split. That makes
  cleaning a source of feedstock rather than pure upkeep, which is what later
  turns it into paid work. The incinerator's only remaining job is colonist
  corpses; [sanitation.md](./sanitation.md) will need its routes rewritten when
  this ships.
- **Cave scum.** A biofilm that grows on some cave surfaces: a worldgen tile
  feature, scraped off like mining but without destroying the tile, maybe
  growing back slowly. That makes it the renewable base of the food chain,
  which a fixed supply of corpses can't be.
- **The scumhouse** facility, and a planner or order path that builds one.

Before the order book exists, the community has biomatter hauled to the scumhouse
the same way cleaning works today: an unpaid, planner-assigned job. When labor
orders arrive it becomes paid work.

**Construction material costs** land in the same phase, behind a switch
(`construction-costs`, default off). Every `buildTask` gets an input list, and
a builder must bring or withdraw those materials before raising the tile. This
also gives mined rock a use at last: nothing consumes it today.

### The order book

A goods order is:

```go
type Order struct {
    ID     OrderID    // monotonic; the time-priority tiebreak
    Side   Side       // Bid or Ask
    Item   ItemKind
    Qty    int
    Price  Money      // per unit, limit price
    Actor  Owner      // a colonist or the community
    Depot  DepotID    // the location
    Posted int        // tick
    Expires int       // tick; stale orders fall off
}
```

**Location is a depot**: a storage container (including a pod locker, a
workshop's input store, or the community silo), not a tile. For now, matching
happens **only within a depot**. A bid at one depot and an ask at another never
cross. Closing that gap is a job, not a feature of the matcher: a hauler buys
at the cheap depot, carries the goods, and sells at the dear one. See *Hauling
and arbitrage*.

**Matching** is continuous and deterministic: a new order crosses the best
opposite order at the same `(item, depot)`, by price and then by the lower
`OrderID`, and trades at the *resting* order's price. Books are sorted slices
per `(item, depot)`. Nothing about which order matches is ever decided by
ranging over a map ([determinism.md](./determinism.md)).

**Escrow on posting.** Posting a bid locks `Qty × Price` from the wallet.
Posting an ask reserves `Qty` on the seller's ledger line at that depot.
Cancelling or expiring releases both. This makes double-selling and
overspending impossible without any settlement-time checks, at the cost of tying
up money in open orders. That's a cost worth keeping: it discourages spamming
the book.

**Settlement** is a ledger move plus a money transfer, both inside the same
depot, in one step. No goods move physically. That's the payoff of ledgers:
trading is instant and hauling is a separate, visible job. Taking a bought meal
out of the depot is an ordinary withdrawal.

Every trade emits a life event / log line and feeds a per-`(item, depot)` last
price and volume, which the market view and the valuation model both read.

### Labor orders

Most work is labor, not goods, so there's a second instrument that any actor
may post:

```go
type WorkOrder struct {
    ID      OrderID
    Issuer  Owner
    Task    WorkSpec // what "done" means: build project P, dig tile T, haul Q×item from depot A to depot B, run recipe R
    Pay     Money    // escrowed from the issuer at posting
    Depot   DepotID  // where the work (or delivery) happens
    Expires int
    Taker   EntityID // 0 while open
}
```

Taking a work order is a claim, owner-keyed like the job board's claims
([spatial-index-and-performance.md](./spatial-index-and-performance.md)), and
released by `clearJob`. Payment is released when the task's done condition
holds (a `buildTask` is already done when its tile holds the desired terrain).
Construction projects become work orders whose pay is split across their tasks,
so the parallel, many-builders property of projects survives.

**The colony as an actor.** The room planner in `project.go` stops assigning
work and starts *buying* it: it decides what the colony needs (as it does today),
then posts work orders funded by the treasury. If the treasury is empty, public
works stop. That's intended, and it's the lever taxes will later pull.

The colony is a trader, not only a spender. Early on it buys to encourage
production that nobody else wants yet. Its stock then goes one of two ways:

- **Sold back.** The colony posts asks for what it holds, at or above what it
  paid, so the colonists who start wanting ore buy it from the colony's silo.
  This is how the treasury gets back what it spent, and makes a profit.
- **Spent on community building.** With `construction-costs` on, public works
  are paid for in materials as well as labor, and the colony's own stock covers
  them before it buys any more.

That turns the founding grant into working capital rather than a countdown: a
treasury that buys low and sells high can keep funding public works
indefinitely, without taxes.

**Prospecting** is a standing community **goods** bid, not labor: the colony
posts evergreen bids for ore at the silo, and anyone who digs ore and deposits
it there can sell into them. When colonists start wanting ore themselves, their
bids compete with the colony's, and the colony can sell them what it already
bought. Gold would be a new `ItemKind` and worldgen
deposit.

### Valuation and the producer planner

This is the crux, and where the tuning time will go. The order book only moves
if agents have prices in their heads.

**Reference prices** are a config table (the colony charter's price list): a
starting guess for every item and for a tick of labor. They get the book going
while the market is thin — with 6 colonists most `(item, depot)` books will be
empty most of the time — and they're the fallback when an item has never
traded. Once an item trades, the last price and volume (smoothed) take over.

**Willingness to pay** for consumables comes from needs. A colonist's bid for a
meal rises with hunger phase (pressing, then critical). Its ask for a meal it
owns rises as its own food stock falls. Both scale with how much money it has,
because a dollar matters more to a broke colonist. All of this is integer math
on existing state: need phases ([needs.md](./needs.md)), wallet, owned stock.

**The producer planner** answers "can I fill that order profitably?". Each time
a colonist runs arbitration, it looks at a bounded set of open bids and work
orders it could fill and estimates, **one recipe level deep**:

```
profit = bid price − Σ input cost − labor ticks × own labor price − workshop access
input cost = best ask at that depot, else a price the colonist would bid (posting that bid)
```

If the profit clears a threshold, "work this plan" becomes a focus candidate,
scored with the rest ([cascading_wsts_architecture.md](./cascading_wsts_architecture.md)).
If an input has no ask, the planner posts a **derived bid** for it and moves on.
A derived bid is an ordinary bid that the *next* producer's planner can see and
fill. That's how the supply chain forms: every round of decisions, demand
reaches one step further down the recipe graph, and no agent ever has to plan
the whole chain.

Derived bids must expire and be withdrawn when the plan that made them dies, or
the book fills with orphaned demand. Tie each derived bid to its plan ID.

The planner has to look at a bounded, deterministic set of candidates (nearest
depots first, top N orders by margin), never the whole book. That keeps
expensive searches out of the scoring loop, which the focus design forbids.

### Hauling and arbitrage

A bid at one depot above an ask at another for the same item is money lying on
the floor, and picking it up is work. The producer planner treats moving goods
as one more recipe:

```
input:  Q × item at depot A      (bought at A's best ask)
output: Q × item at depot B      (sold into B's best bid)
ticks:  walking time from A to B, loaded
profit = Q × (bid_B − ask_A) − ticks × own labor price
```

So hauling needs no machinery of its own beyond what E4–E6 already build:
cargo records, depot-to-depot routes (the pathfinder already estimates them),
and the planner's margin check. It shows up in two forms:

- **Arbitrage (on your own account).** The hauler buys at A, owns the goods in
  transit, and takes the risk. The bid at B may be gone on arrival, an alien may
  intervene, and hauling uranium ore doses the carrier
  ([mutation.md](./mutation.md)). If the bid is gone, the hauler posts an ask
  at B and becomes an ordinary seller there. A bid can't be reserved in advance:
  that would let one colonist lock up a market by claiming to be on the way.
- **Hauling for hire.** A labor order: "move Q × item from A to B for $X." The
  issuer owns the cargo throughout, and the hauler's cargo record says so. This
  is the safe, low-margin version, and what the colony uses to stock its silo.

Arbitrage is also what makes prices at different depots agree with each other.
Without it, every depot is its own island market with a handful of colonists.

### Who does what work

For now every colonist is equally good at everything, so there's nothing to
keep anyone in a trade. Colonists here are perfectly spherical, perfectly
rational utility maximizers: any order that can be filled profitably will be,
by whoever's planner finds it first, and the same colonist may mine, haul, cook
slurry, and build walls in one afternoon. Expect that, and don't read it as a
bug: nobody has a career yet.

Two later systems will change it, and both push the same way, toward
individuals doing the same kind of work over time:

- **Skills.** Practice makes a colonist faster or better at a kind of work, so
  its margins there beat everyone else's, and its planner keeps picking it.
  That's specialization through comparative advantage.
- **Identity.** A colonist who has hauled for months starts to think of itself
  as a hauler: a preference that shows up in focus scoring even when the
  margin elsewhere is slightly better, and that ties into traits, memories,
  and affect ([personality.md](./personality.md), [memories.md](./memories.md)).

Until then, the recipe table's `Skill` placeholder and the planner's labor
price are where each of these will plug in.

### Market view

A TUI tab (next to Details · Storage) showing:

- wallets and the treasury;
- per-depot books: best bid and ask, depth, last price, volume;
- open work orders and who took them;
- a colonist's own open orders and ledger lines in the roster details.

The snapshot gets `MarketView`, sorted by depot and item before publication
([architecture.md](./architecture.md)). It ships in the first phase that has
money, and grows with each phase after.

## Why it is this way

- **Recipes before the market.** A market without production only redistributes
  a fixed amount of stuff. Mining was the only thing that ever produced
  anything, and nothing consumed what it produced.
- **Supply chains don't come free with an order book.** Someone has to reason
  backwards from a bid for a finished good to bids for its inputs. The producer
  planner does that one level at a time, which is cheap, easy to explain, and
  lets depth build up over time instead of being solved all at once.
- **Ledgers, not owners on every stack.** Stacks are `{Kind, Count}` and should
  stay that way. A ledger per depot gives fungible ownership, instant
  settlement, and the silo model in one mechanism.
- **Prefab pods before movable fixtures.** Turning a placed bunk back into an
  item and back again touches construction, pathing, and the tile grid. Stamping
  a pod gets owned fixtures into the game without any of that.
- **Escrow on posting.** It removes a whole class of settlement-time failures
  (buyer broke, goods gone) at the cost of locking up money in open orders,
  which also discourages order spam.
- **The colony buys work instead of commanding it.** Otherwise, once colonists
  act for profit, shared goods like walls, corridors, and the incinerator never
  get built. It also gives the future tax system something to fund.
- **Labor needs its own instrument.** `{bid/ask, item, quantity, actor,
  location}` can't express "dig this tile" or "build this room", and those make
  up most of what colonists do.
- **Integer money.** Floats would make settlement depend on the order sums were
  taken in.

## Build plan

Each phase builds, passes `go test ./...`, keeps the determinism lockstep test
green, and ships with the safety-net pod **on**, so the colony never depends on
an untuned economy to survive. Every phase updates this doc (or moves its
content into a present-tense doc) and adds new tunables to `sim.Config`
and `mars-sim.yaml`.

### E0 — Money (shipped)

`Money`, wallets, treasury, `transfer`, `founding-grant`, `crash-pod-purse`
(granted at spawn for now). A first market view listing wallets.
**Gate:** a conservation test (the total is constant across a long run).
Shipped as described; see [money.md](./money.md). One refinement: a dead
colonist's wallet is frozen into `moneyFrozen`, so the audited identity is
circulating + frozen == issued rather than a constant circulating total.

### E1 — Property (shipped)

`Owner`, depot ledgers on storage containers, fixture records with access
policies, owner-aware carried goods and cargo records. Planner-built facilities
are community-owned and communal. Starting weapons are owned by their carriers.
**Gate:** behavior identical to before (same lockstep trace for a given
seed); ledger ⇔ physical-count invariant test; private-access routing test.
Shipped; see [property.md](./property.md). All three gates are met (five seeds
fingerprinted identically before and after). Two parts were deliberately
deferred to the first job that needs them: **cargo records** (no job carries
someone else's goods yet, so "what you carry is yours" is implicit) and ledger
**withdrawal** (nothing takes goods out of storage yet). The lockstep
fingerprint also gained a `property` field.

### E2 — Crash pods and meals (shipped)

`Meal` item; the pod prefab, landing-site search, and crash-through fallback;
manifest config replacing `pistols`/`shotguns`; `arrive()` shared by worldgen,
the spawn command, and a director `arrival` occurrence; eating from owned meals
first; the safety net made the least attractive option.
**Gate:** every colonist (worldgen, command, director) owns a pod; a colonist
with meals doesn't use the safety net; `infinite-food: false` with no production
starves the colony on the timeline the manifest predicts (a deliberate
test of the scarcity itself).
Shipped; see [crash-pods.md](./crash-pods.md) and [food.md](./food.md). All
three gates are tests. What differs from the sketch above:

- The pod is an open five-by-two footprint, not a walled shell, and it lands
  only in the **lower half** of the map. Rooms site against rock above them,
  and pods that spread into the top of the cavern left small colonies unable
  to build anything. The landing cavern is sized for its pods for the same
  reason.
- The safety net is "least attractive" by rule and by mood: own meals, then the
  colony's, then the pod, in that order, and pod food records `EvtAteGruel`
  instead of a meal.
- Every settler lands with a pistol by default. Survival across 20 seeds went
  from 9 to 15 (see [combat.md](./combat.md)).
- Private fixtures had to stop counting as "in the way" (`onFacilityAccess`),
  or nobody near a pod could be talked to and the colony stalled on social need.
- Mice became **rats**, which scavenge bodies, gore, and exposed scum before
  they raid a pod, competing directly with the scumhouse for biomatter
  (shipped after E3; see [entities-and-ai.md](./entities-and-ai.md)).

### E3 — Recipes and slurry (shipped)

Recipe table; workshop executor; corpse kinds (colonist corpses stay refuse; viscera and other corpses become biomatter);
cave scum worldgen feature and harvesting; the scumhouse; community-assigned
biomatter hauling. `construction-costs` switch (off by default).
**Gate:** with the safety net off, a colony that harvests scum and hauls
corpses sustains itself for a defined horizon on a reference seed.
Shipped; see [scumhouse.md](./scumhouse.md), [sanitation.md](./sanitation.md)
and [construction.md](./construction.md). The gate is
`TestColonyFeedsItselfWithoutTheSafetyNet`: six colonists, three meals each,
safety net off, all alive at tick 8000 (and all starved on schedule with scum
turned off). Notes against the sketch:

- The recipe table is Go data (`recipes`), four rows at the scumhouse:
  alien carcass, animal carcass, viscera, cave scum.
- Scum lives on rock *surfaces*: scrapable while exposed, left on the floor when
  its rock is mined, destroyed when something is built over it. Regrowth is
  lazy.
- Gathered biomatter is the colony's through a **cargo record**, the first one;
  so the meals it becomes are communal and anyone eats them.
- The planner builds a scumhouse first when the safety net is off, and only on
  order when it is on.
- With construction costs on, builders pay from their own stock and donate it.
  A colonist never claims a task it cannot pay for, so it mines instead.

### E4 — Order book (shipped)

Goods orders, per-depot books, escrow, matching, settlement, expiry; reference
prices; standing community ore bids at a silo (paid prospecting); colonists
buying food when hungry and selling surplus from fixed reference prices.
Market view with books and trades.
**Gate:** matching determinism (same orders → same trades regardless of
insertion into maps), conservation with trades, no double-sale under
contention.
Shipped; see [market.md](./market.md). All three gates are tests. Notes:

- Escrow is held by the order itself, as an owner (`ownerOrder`): money in its
  own account, goods on its own ledger line. Every existing audit covers it.
- The silo is the colony's communal chest nearest the centre; the planner
  builds one if there is none, since crash-pod lockers mean nothing else would.
- Raw rock is not bought by default; ores are.
- A pre-existing routing bug surfaced: a colonist following the shared field
  to the only facility of its kind could pace forever beside construction that
  crossed the field's route. It now switches to A* when the field sends it
  uphill (see `fieldDetourTicks`).

### E5 — Labor orders and the colony as buyer (shipped)

`WorkOrder`; claiming and pay release; the room planner posting funded work
orders; colonists posting their own (a house); paid biomatter hauling;
`AccessPaid` fixtures.
**Gate:** an unfunded treasury halts public works but not survival; a colonist
can commission a structure and pay for it.
Shipped; see [labor.md](./labor.md). Both gates are tests. Notes:

- A room is funded in full before it is designated, or not at all; each task is
  one work order, paid on completion.
- Survival without money comes from unpaid emergency builds and crash pods.
- Colonists commission a house (bunk + toilet) once they have `house-savings`;
  its toilet is the first paid fixture.
- The biomatter bounty is a standing work order at each scumhouse; the work
  happens without it.
- Colonists do not yet choose work by what it pays — that is E6.
- The silo now takes only what sells, so unsold raw rock never crowds ore out
  of the market; it is general storage only as a last resort.

### E6 — Valuation and the producer planner

Need-driven willingness to pay; smoothed last prices; the producer planner and
derived bids as a focus candidate; plan-linked bid withdrawal. A tuning harness
that logs price series, volume, treasury, starvation, and chain depth per seed.
**Gate:** on a reference seed, a finished-good bid with no stock produces
trades at least two recipe levels down without any scripted help.

### E7 — Hauling and arbitrage

Transport as a planner recipe (buy at A, carry, sell at B); hauling-for-hire
labor orders; the colony selling its stock and paying public-works material
costs from it. Market view shows the price of each item across depots.
**Gate:** on a reference seed with two depots and a price gap, a colonist
hauls between them unprompted and the gap narrows; the treasury ends a long run
above what it would have with the colony only buying.

### E8 — Scarcity on

`infinite-food` and `construction-costs` flip defaults; the safety net
remains available for tests and balancing. Tuning.

## Deliberately not doing (yet)

Each item is a known TODO, not an oversight:

- **Death and inheritance.** Property stays where it is, owned by the dead
  colonist, or goes with the body. Their orders are cancelled, escrow goes back
  to their wallet, and the wallet freezes, taking that money out of circulation. Inheritance needs
  families-as-owners, which needs organizations and law.
- **Organizations.** Only the commented-out owner kind.
- **Enforcement, theft, begging.** Colonists follow the rules. A
  desperate colonist with no money and no food relies on the safety net.
- **Debt and lending.** A goal, not a maybe: debt is one of the most
  interesting things this economy could grow. Credit lets a colonist buy the
  machine before it pays for itself, which is exactly the up-front bet the
  producer planner otherwise has to fund from savings, and it lets the money
  supply grow beyond the fixed founding amount. For now balances can't go
  negative. When lending arrives it needs a loan instrument (lender, borrower,
  principal, interest, due tick), a rule for what happens on default (which
  pulls on enforcement), and a decision on whether the community treasury can
  lend or create money.
- **Skills and identity.** A placeholder field on recipes; everyone is equally
  able, so nobody specializes yet (see *Who does what work*).
- **Taxes and monetary policy.** The money supply is fixed; `transfer` is where a
  tax would hook in.
- **Moving fixtures.** Packing up and moving a bunk.

## Open questions

- **Meals per pod and purse size.** These set how long the early game lasts and
  how fast the market matters. Pick them in E2 with the starvation-timeline
  test.
- **Cave scum regrowth.** Does it grow back, how fast, and is it tied to water ice
  or darkness? It's the base of the renewable food supply, so it's the most
  important balance number here.
- **The colony's markup.** How far above its purchase price does the colony
  ask? Too low and it can't fund public works; too high and it undercuts the
  point of subsidizing early production.
- **Paid toilets and the bladder.** Bladder isn't fatal. What does a colonist
  who can't pay for any toilet do? This is the first place scarcity
  meets a non-fatal need, and a likely source of good emergent behavior (and
  mess — [sanitation.md](./sanitation.md)).

## Risks

- **Thin markets.** Few colonists means empty books and wide spreads. Reference
  prices and a willingness to trade at them are the mitigation; watch for
  items that never trade at all.
- **Deflation.** A fixed money supply, plus money locked in escrow, may freeze
  trade once wallets thin. Log the share of money in escrow.
- **Orphaned derived bids** filling the book with demand from abandoned plans.
- **Treasury exhaustion** stopping public works before any tax exists: the
  founding grant has to last until the colony's resales start paying (E7).
- **Churn without specialization.** Until skills and identity exist, colonists
  will hop between trades constantly. That's expected, but it can also look like
  thrashing between focuses; the focus system's commitment rules have to hold
  a plan long enough to finish it.
- **Performance.** Book scans inside focus arbitration would undo the resting-AI
  work. Keep the planner's candidate set bounded, and keep best-bid/ask per book
  as O(1) reads.
- **Determinism.** Every new identity (order IDs, depot IDs, plan IDs) is
  allocated from a counter, never derived from map order.

## Extending it

- **A new good**: an `ItemKind`, a reference price, and one or more recipe rows.
- **A new need-satisfying product**: a facility that dispenses from its depot
  ledger, plus a willingness-to-pay rule for that need.
- **A new owner kind**: extend `OwnerKind`, then `transfer`, the ledger, and the
  access checks. Everything else takes `Owner` and shouldn't care.

## Related

- [needs.md](./needs.md) — the needs that drive willingness to pay.
- [inventory.md](./inventory.md) and [storage.md](./storage.md) — stacks and
  containers the ledger sits on.
- [construction.md](./construction.md) — the planner that becomes a buyer.
- [sanitation.md](./sanitation.md) — corpses and viscera today, and why colonist corpses (but not viscera) stay refuse.
- [director.md](./director.md) — the arrival occurrence.
- [combat.md](./combat.md) — the colony ship equipment the pod manifest replaces.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md) — where the
  producer planner's plans compete for attention.
- [determinism.md](./determinism.md) — matching, IDs, and integer money.

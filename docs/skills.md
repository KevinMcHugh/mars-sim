# Skills and professions

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists get better at work by doing it. Each colonist has **practice** in
five skills (mining, foraging, cooking, construction, smithing), counted in
base work ticks of completed work, and arrives with some from its past. A
skill's **rank** is where that practice sits on the skill's logarithmic
curve. Each rank has a label ("journeyman smith") and makes the work faster
and, for recipes, more productive. A colonist's **profession** is the skill
it stands highest in.

Skills don't yet steer who does what. Any colonist still takes any work, so
the same colonist mines, scrapes, cooks and smelts steel in one afternoon, and
every colonist gets better at all of it. The next phase (S3) puts
**opportunity cost** in the producer planner, so colonists drift toward the
work that pays them best, which is more and more the work they're good at.
The goal is not to assign careers. It's to make careers pay, and let colonists
find them. Small colonies should stay mostly generalist, and specialization
should come with a deep economy.

S1 to S3 are built, and S4 in part. S5 is the plan.

| Phase | What ships | Status |
| --- | --- | --- |
| S1 | Practice, ranks and labels; skills rolled at character generation; rank-up memories; profession; roster display. | Shipped |
| S2 | Effects: skill makes work faster and increases yield, by more per rank for steeper skills. | Shipped |
| S3 | Opportunity cost in the producer planner: skill-aware costs, choosing the best-paying plan, and a reservation rate from what the colonist has been earning. | Shipped |
| S4 | Competition: drop the planner's reservations on opportunities, so colonists race for bids and undercut each other. | Shipped in part: bids are open to everyone; shrewd bidding and private price memory are still proposed |
| S5 | A workshop of one's own: a skilled colonist builds a forge or a scumhouse on its own account when the returns pay for it. | Proposed |

## Source

- [`internal/sim/skills.go`](../internal/sim/skills.go) — `SkillKind`, the
  `skillSpecs` table (curves, labels, effects), `rankAt`, `practise`,
  `updateProfession`, `workTicks`, `skillYield`, the background table and
  `rollBackground`, and `SkillView`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `practice`,
  `yieldAcc` and `profession` on `Entity`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — mining and build
  completion (`jobMine`, `jobBuild`, `buildSkill`);
  [`scumhouse.go`](../internal/sim/scumhouse.go) — recipe runs (`jobCraft`,
  with yield) and scraping (`jobScrape`); `Recipe.Skill` on every recipe.
- [`internal/sim/crashpod.go`](../internal/sim/crashpod.go) — `arrive` rolls
  the background.
- [`internal/sim/rng.go`](../internal/sim/rng.go),
  [`world.go`](../internal/sim/world.go) — the `skillRNG` stream.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go),
  [`cognition.yaml`](../cognition.yaml) — the `rose-in-trade` reaction.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) —
  `EntityView.Skills`, `Profession`, `ProfessionLabel`;
  [`render_roster.go`](../internal/ui/tui/render_roster.go) — the SKILLS
  section of a colonist's details.
- [`internal/sim/skills_test.go`](../internal/sim/skills_test.go).

- [`internal/sim/producer.go`](../internal/sim/producer.go) — best-rate
  plan choice (`tryAssignProduce`, `planOffer`, the `probe` on each plan
  function) and `notePlanEarned`;
  [`valuation.go`](../internal/sim/valuation.go) — `reservation`,
  `noteEarnings`, `laborCostFor`, `ownWorkTicks`.
- [`internal/sim/planner_test.go`](../internal/sim/planner_test.go).

## Decisions already made

These were settled in the design conversation. Don't re-open them without a
reason the conversation didn't have.

| Topic | Decision |
| --- | --- |
| Ranks | A few per skill, each with a label. Not a 0–100 number. |
| The curve | Rank is logarithmic in practice, with a per-skill base. Base 4 is the baseline. Base 2 is rare (a shallow skill). Base 10 is too steep. |
| Steepness pays | The steeper the curve, the bigger the payoff per rank. A master of a steep skill is dramatically better than a journeyman, not a little better. |
| Master smiths | Rare, and an incredible asset to a colony that needs smithing. Wasted on a small colony that doesn't, and that's fine. |
| Allocation | By reward, not by rule. Colonists weigh their opportunity cost in producer planning and tend toward what they're good at because it pays better, in higher wages or faster output. There is no "leave it for the professional" rule. |
| Small colonies | Specialize slowly. Specialization is a product of a deep economy. It should fall out of colonists in small colonies spending more of their time on constant-cost activities (walking, fetching, carrying), where skill doesn't help. |
| Character generation | Colonists arrive with skills already rolled. |
| Quality | Out of scope. Effects are speed and yield. |
| Competition | Each colonist acts in its own interest, even when that means competing with other colonists. The planner must not share out opportunities the way a central planner would. |
| Capital | A good smith builds its own forge, so it can take work without waiting on the colony's forge or bench. Workshops aren't only the colony's to build. |
| Traits and learning | Traits don't change learning. Practice is credited in base ticks, so an Industrious colonist gets through more work, and so more practice, but no more per unit. |
| Smithing | One skill, covering both the forge and the gun bench ([foundry.md](./foundry.md)). Splitting it would cut an already small volume in half; see *Open questions*. |

## Who does what without skills

These are the numbers the design is set against. They come from default-config
runs of the code before skills, with counters added at each completion site,
100,000 ticks, after the foundry landed. Running with `-skills=false
-skill-practice-percent 0` reproduces that code's results exactly.

**Units of work completed per colonist, seed 1, 6 colonists:**

| Colonist | Tiles mined | Scum scraped | Recipes cooked | Tiles built | Steel smelted | Rifles machined |
| --- | --- | --- | --- | --- | --- | --- |
| c1 | 461 | 601 | 197 | 17 | 1 | 0 |
| c2 | 520 | 804 | 385 | 22 | 0 | 2 |
| c3 | 349 | 76 | 131 | 10 | 4 | 1 |
| c4 | 447 | 57 | 262 | 19 | 3 | 0 |
| c5 | 556 | 864 | 302 | 32 | 2 | 1 |
| c6 | 413 | 574 | 194 | 14 | 2 | 0 |

**Colony totals and time use, 100,000 ticks:**

| Seed, colonists | Mined | Scraped | Cooked | Built | Smelted | Rifles | Hands-on work | Idle |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1, 6 | 2,746 | 2,976 | 1,471 | 114 | 12 | 4 | 12.6% | 42.3% |
| 2, 6 | 2,689 | 2,832 | 1,408 | 129 | 12 | 4 | 11.9% | 43.9% |
| 1, 30 | 10,951 | 12,901 | 5,628 | 125 | 12 | 4 | 13.6% | 20.2% |
| 2, 30 | 10,837 | 14,678 | 6,760 | 165 | 12 | 4 | 12.9% | 17.1% |

(The 30-colonist runs use a 160×80 map, and many colonists die: 11 and 13 are
left at tick 100,000. Take those rows as a direction, not a measurement of a
healthy big colony.)

What follows:

1. **Everyone does everything.** Nobody has a trade, and smithing in
   particular is spread thin. Five of six colonists smelted, and nobody smelted
   more than four ingots.
2. **Small colonies are short of demand, not labor.** Hands-on work is about
   12% of a colonist's time, and idle is over 40%. In the bigger colony idle
   falls to about 20%. Labor gets scarcer as the economy gets bigger, and
   that's the condition specialization needs.
3. **Smithing is capped.** The armory wants 4 rifles and never sells, so every
   game smelts 12 ingots and machines 4 rifles, at any colony size. That's
   **720 base ticks of smithing per game, for the whole colony**. Nobody can
   practise their way to master smith today. Masters exist only if they
   arrive as masters. That fits "rare", and it's also why one would be wasted.

## Problems with ranking skills by log_X(uses)

A skill's rank as `log_X(n)`, with `n` the number of successful uses, has
these problems. Each has an answer in the design below.

1. **Speed bonuses do little in an idle colony.** Skill only scales the work
   part of a job, never the walking, so in a small colony a skilled
   colonist's advantage is small. The design relies on that: see *Why small
   colonies stay generalist*.
2. **A skill level alone doesn't make a smith refuse to cook.** Opportunity
   cost (S3) does, and it also lets the smith cook in an emergency, when
   hungry colonists' meal bids climb.
3. **The base doesn't set how many ranks anyone reaches; volume does.** At
   base 10 with rank 1 at one gun, rank 3 takes 1,000 guns. A per-skill
   `Unit` sets where rank 1 is, and the label list caps how many ranks there
   are. The base sets the spacing and the payoff per rank.
4. **"Uses" aren't a common unit.** Practice is counted in base work ticks of
   completed work, so a 60-tick rifle teaches more than a 6-tick tile.
5. **Floating-point log gets exact powers wrong.** In Go,
   `math.Log(1000)/math.Log(10)` is `2.9999999999999996`. Ranks come from an
   integer walk up the thresholds.
6. **Starting at zero lets turn order pick careers.** Skills are rolled at
   character generation.
7. **Log curves favor generalists.** That's intended for small colonies.
   What pushes against it is the reward: a steeper skill pays more per rank,
   and opportunity cost makes the better-paying skill win.

## How it works

### Skills

| Skill | Practised by | Base | Why this base |
| --- | --- | --- | --- |
| `SkillMining` | mining a tile; digging a room's floor | 2 | Shallow: anyone gets a little better at digging, and there's a lot of it. |
| `SkillForaging` | scraping cave scum | 3 | Mostly walking; not much to master. |
| `SkillCooking` | every scumhouse recipe | 4 | The baseline. |
| `SkillConstruction` | raising walls and fixtures | 4 | The baseline. There's little of it, so `Unit` is small. |
| `SkillSmithing` | smelting steel, machining rifles | 6 | Steep, rare and very rewarding. |

Hauling and cleaning aren't skills. Hauling is walking, and a skill that made
walking faster would leak into everything (and would erase the constant-cost
effect below). Cleaning is under 0.1% of anyone's time. Recipes already carry
a `Skill` field, so cooking and smithing get their skill from the recipe
table (`Recipe.Skill`). Mining, foraging and construction are credited at
their completion sites: a mined tile, a scraped unit of scum, a raised wall or
fixture. Digging out a room's floor is mining.

### Practice

`Entity.practice [numSkills]uint32` holds **base** work ticks of completed
work per skill. It's a fixed array, not a map: no allocation, no iteration
order, 24 bytes per colonist. `w.practise(e, skill, baseTicks)` is called
where a unit of work completes, scaled by a global `skill-practice-percent`. Crediting base ticks, not actual ticks, means getting
faster doesn't slow a colonist's learning. Crediting on completion keeps
"successful": half a job teaches nothing. All completed work counts, paid or
unpaid. Practice never decays.

### Ranks

```go
type skillSpec struct {
    Name    string
    Unit    uint32       // base work ticks to reach rank 1
    Base    uint32       // each later rank needs Base× the previous threshold
    Labels  []string     // rank 0 ("untrained") first; the last is the top rank
    Effects []rankEffect // one per rank
}
```

Rank *r* ≥ 1 is reached at `Unit × Base^(r−1)` practice, which is
⌊log_Base(practice / Unit)⌋ + 1, computed by an integer walk and capped at the
top label. Other systems call `e.rank(SkillSmithing)` or read a label; only
`skills.go` sees practice.

The curves, all in `skillSpecs`:

**Smithing** — `Unit 80` (two smelts), `Base 6`

| Rank | Label | Practice | ≈ Work | Reached |
| --- | --- | --- | --- | --- |
| 0 | Untrained | 0 | | |
| 1 | Apprentice | 80 | 2 ingots | by anyone who helps at the forge |
| 2 | Journeyman | 480 | 12 ingots | by one colonist doing most of a game's smithing |
| 3 | Smith | 2,880 | 4 games' worth | practically only at character generation today |
| 4 | Master smith | 17,280 | 24 games' worth | only at character generation |

Nobody reaches Smith by practice until rifles are wanted more than 4 at a time
(see *Open questions*). That's what makes a Master smith rare. It's not a
separate rarity knob.

**Cooking** — `Unit 120` (~10 batches), `Base 4`

| Rank | Label | Practice | ≈ Batches |
| --- | --- | --- | --- |
| 0 | Untrained | 0 | 0 |
| 1 | Kitchen hand | 120 | 10 |
| 2 | Cook | 480 | 40 |
| 3 | Chef | 1,920 | 160 |
| 4 | Master chef | 7,680 | 640 |

In a small colony every colonist cooks a few hundred batches in 100,000
ticks and becomes a Chef (see *What skills do in a run*). A colonist who did
most of a colony's cooking (~1,400 batches) would be a Master chef.

**Mining** — `Unit 60` (10 tiles), `Base 2`: Rockbreaker (10 tiles), Digger
(20–40), Tunneler (80–160), Miner (320), Seasoned miner (640), Master miner
(1,280). Everyone in a small colony reaches Miner. Two labels can share a
rank range, which keeps a fine curve under fewer names.

**Foraging** — `Unit 60` (10 units of scum), `Base 3`: Gleaner (10), Scraper
(30), Forager (90), Seasoned forager (270), Master forager (810).

**Construction** — `Unit 40` (5 walls), `Base 4`: Laborer (5), Builder (20),
Mason (80), Master builder (320). A small colony builds only about a hundred
walls in 100,000 ticks, so few get past Builder.

### Effects: steeper skills pay more

A rank changes two things, both integer, both in one table per skill:

```go
type rankEffect struct {
    TicksPct int // percent of base work ticks; 100 = unchanged. Composes with workScale.
    YieldPct int // percent of the recipe's normal output; 100 = unchanged
}
```

- **Speed**: `workTicks` cuts the base ticks by `TicksPct` in integers, then
  `scaleTicks` applies the traits' `workScale` as before. (`workScale` stays a
  float; skill effects are integer percents so they add no rounding of their
  own.)
- **Yield** above 100 is paid out by an accumulator, not a roll. Each run adds
  `YieldPct − 100` to `e.yieldAcc[skill]`, and every 100 adds one more unit of
  the recipe's first output. If the output depot has no room for it, it waits
  for a run where there is. Yield adds no RNG draws and is exact over time.
- With `skills` off, every effect is neutral.

**Throughput** is `YieldPct / TicksPct`: output per base tick of work. The
decision that steeper skills pay more becomes a rule for writing these tables:
**each rank multiplies throughput by more the higher the base**. As a starting
point, about +10% per rank at base 2, +20% at base 3, +30% at base 4 and +50%
at base 6, compounded:

| Smithing rank | TicksPct | YieldPct | Throughput |
| --- | --- | --- | --- |
| Untrained / Apprentice | 100 | 100 | 1.0× |
| Journeyman | 75 | 110 | ~1.5× |
| Smith | 55 | 125 | ~2.3× |
| Master smith | 40 | 140 | ~3.5× |

| Cooking rank | TicksPct | YieldPct | Throughput |
| --- | --- | --- | --- |
| Untrained / Kitchen hand | 100 | 100 | 1.0× |
| Cook | 85 | 110 | ~1.3× |
| Chef | 70 | 120 | ~1.7× |
| Master chef | 60 | 130 | ~2.2× |

| Foraging rank | TicksPct | Throughput |
| --- | --- | --- |
| Untrained / Gleaner | 100 | 1.0× |
| Scraper | 83 | ~1.2× |
| Forager | 69 | ~1.45× |
| Seasoned forager | 58 | ~1.7× |
| Master forager | 48 | ~2.1× |

| Construction rank | TicksPct | Throughput |
| --- | --- | --- |
| Untrained / Laborer | 100 | 1.0× |
| Builder | 77 | ~1.3× |
| Mason | 59 | ~1.7× |
| Master builder | 45 | ~2.2× |

Mining is flat to rank 5, then `TicksPct` 80 at Miner and Seasoned miner and
70 at Master miner: it tops out around 1.4×. Mining, foraging and
construction have no yield: you can't mine more ore out of a tile than it
holds, or scrape more scum than a patch has.

A Master smith gets about 2.4× a Journeyman's throughput from the same ore,
which is the "dramatically better" the design asks for. Because a Master
smith's yield means fewer ingots per rifle, it's also better for the colony's
ore, not just faster.

In terms of practice, output grows as practice raised to the power
log_Base(gain per rank). That's about 0.14 for mining, 0.19 for cooking and
0.23 for smithing. So a steep skill really does return more for the same
practice: the curve makes you work harder for each rank and pays more for it.

### Backgrounds: skills at character generation

`arrive` rolls each colonist's past from a weighted table, most often one
skill at a modest rank, sometimes a second, rarely a master:

| Roll | Chance |
| --- | --- |
| One skill at rank 1 | 45% |
| One skill at rank 2 | 30% |
| One skill at rank 2 and another at rank 1 | 15% |
| One skill at rank 3 | 8% |
| One skill at its top rank | 2% |

Which skill is weighted per skill. Smithing gets half the weight of the
others, so a Master smith is about 1 colonist in 450, and a Smith or better
about 1 in 90. The roll sets practice to the rank's threshold, so a
background is ordinary practice from then on. It records no memory; it's
where the colonist starts. With `skills` off, nobody arrives with one.

The roll affects the simulation, so it can't come from the personality stream
(principle 3). Adding it to `World.rng` would shift every later draw on every
seed. It gets its own saved stream (`skillRNG`, per
[rng-streams.md](./rng-streams.md)). `TestBackgroundsDrawFromTheirOwnStream`
checks that a world generated with backgrounds is otherwise identical to one
without.

### Opportunity cost in the producer planner (S3)

Three parts, in `producer.go` and `valuation.go`.

**1. Costs are the colonist's own.** Every plan (`planCraft`, `planGather`,
`planSupply`, `planArbitrage`) and `foodPays` prices a colonist's time with
`laborCostFor(e, ticks)`, where the ticks split into the parts skill does and
doesn't touch:

```
ticks = constant ticks                          (walking, fetching, carrying: the same for everyone)
      + ownWorkTicks(work ticks, rank's TicksPct)   (the recipe or the scrape: the colonist's skill)
```

A skilled colonist sees more profit on the same bid. Yield isn't counted in
the plan: a plan delivers what its bid asks for, and a skilled worker's
extra unit is a bonus that stays in its name.

**2. The best plan, not the first.** `tryAssignProduce` reckons every one of
its `plan-candidates` bids without taking any on (each plan function takes a
`probe` and fills in a `planOffer` instead of acting), then takes the one with
the best **rate**, profit per 100 ticks of the colonist's time; ties go to the
first reckoned. The candidate set stays bounded (principle 9).
`TestThePlannerTakesTheBestRate` has a dearer bid across the map lose to a
cheaper one beside the scum.

**3. A reservation rate.** A colonist's time costs it its **reservation**:

```
reservation = max(labor-price, remembered rate at its best-paying work)
```

When a plan delivers (`notePlanEarned`), the colonist remembers what it paid:
the profit the plan expected, over the ticks it took, smoothed per skill
(`noteEarnings`; hauling and supplying, which have no skill, count under
`SkillNone`). A memory **fades back toward `labor-price`** over `rate-memory`
(4000) ticks without earning there again. A plan's profit has to clear
`plan-min-profit` after the colonist's time at that rate, so a colonist that
earns well passes over work that pays less. What it expects from a craft plan
with missing inputs is `plan-min-profit`, since it bids its whole margin for
them, so crafting only raises a reservation when the colonist has the inputs.

What this does:

- **A well-paid colonist passes on poor work**
  (`TestAWellPaidColonistPassesOnPoorWork`), until the price of that work
  rises past its rate.
- **Except in an emergency.** Hungry colonists bid more for meals as they get
  hungrier (see [valuation.md](./valuation.md)). A colonist with a critical
  need of its own still bypasses all of this (`tryAssignFoodWork(e,
  force=true)`).
- **A stranded specialist rejoins general work** as its memory fades.
- **A novice takes what it can get**: with no earnings its reservation is
  `labor-price`.

Wages from work orders don't count toward a reservation yet: public works
still come off the community ladder ([work-market.md](./work-market.md)).

### Competing, not coordinating (S4)

Each colonist decides for itself, from its own wallet and position, and
supply chains form through ordinary bids. S4 removes the rule that shared
opportunities out instead of letting colonists compete for them, and leaves
the rest for later:

| Rule | What it did | Now |
| --- | --- | --- |
| `plannedQty` | A bid that other plans already covered was invisible to everyone else. | **Gone.** Anyone may pursue any bid; the first to deliver fills it, and a later delivery rests as an ask (`TestColonistsCompeteForABid`). |
| `planWaitingAt` | One colonist plans at a workshop at a time. | **Kept.** It's the physical constraint of one bench: without it, four colonists each held a plan at the one gun bench waiting on steel, and nobody was left to smelt ([foundry.md](./foundry.md)). Pricing the queue into the rate is still to do. |
| Ask at the bid's price | Every seller is a price-taker. | Meals undercut the colony by a dollar (`mealSellPrice`); other goods still ask the bid's price. Proposed. |
| Derived bid at the whole margin | A buyer offers everything it can afford. | Proposed: offer less while more than one seller is around. |
| One global price memory | Everyone knows every trade instantly. | Proposed: a per-colonist price memory. |

Races are resolved by turn order, and that stays deterministic. Turn order is
by ID, so low IDs win every tie. Proximity (the nearer colonist arrives
first) should decide most races on its own. If it doesn't, rotate the turn
order by tick.

Duplicate effort is the price of competition, and it's the real one: two
smiths making steel for one bid is what a market with two smiths does. The
loser's steel isn't wasted. It's stock with an ask on it.

### A workshop of one's own (S5, proposed)

Today only the colony builds workshops (`planRooms`), and a colonist only
commissions a house, by a fixed savings rule. The plumbing for more is there:
`planRoomFor(recipe, issuer)` works for any issuer, a commission's fixtures
become the commissioner's (`fixtureAccess`), and recipes already give the
output to whoever owns the inputs, not to the workshop's owner.

A colonist decides to build a workshop as an investment:

```
gain per tick = rate with its own workshop − rate at the best shared one
                (no queue, no walk across the colony, no access fee)
build it when   gain per tick × horizon > room cost (wages + materials)
```

The horizon is a config value (how far ahead a colonist looks), and the rates
come from the earnings memory (S3). A Master smith with fresh smithing
earnings who keeps losing time waiting at the colony's forge crosses the line
first. A novice with no earnings history never does. That's how professions
turn into capital. Once a colonist owns a forge, walking and waiting drop out
of its plans, work becomes a bigger part of each plan's ticks, and its skill
counts for more. That's the deep-economy effect in *Why small colonies stay
generalist*, bought by one colonist.

A private workshop can be `AccessPaid`: the owner works it free, and other
smiths pay per use. That makes the forge capital someone rents out (see
[economy.md](./economy.md)).

The limit is demand. A private forge only pays if there's smithing to sell,
and the armory stops at 4 rifles (see *Open questions*). Until colonists
themselves want rifles, S5 will rarely trigger for smiths. It will for
cooks: meal demand never stops.

### What S3 and S4 do in a run

They don't end the big-colony die-off (see [scumhouse.md](./scumhouse.md) and
[food.md](./food.md)). 100 colonists on a 300×150 map, seeds 1–12, 30,000
ticks, starved of 1,200:

| | Pocket meals on (default) | Pocket meals off |
| --- | --- | --- |
| Before S3/S4 | 310 | 185 |
| S3 and S4 | 318 | 196 |

The difference is within the spread between seeds. The die-off is a
production problem, and choosing better plans doesn't make more food. Six
colonists, seeds 1–32: 2 starved against 10.

### Why small colonies stay generalist

This isn't a setting. It comes out of the cost split above.

Skill scales only a plan's work ticks. The constant ticks (walking to the scum
patch, carrying ore to the forge, the ingot to the bench, the rifle to the
silo) are the same for a Master and an Apprentice. In a small colony that's
most of the plan: culturing scum is 12 work ticks against a walk to a patch, a
walk to the scumhouse and a walk to the buyer. So the skilled colonist's rate
advantage is small, candidates come out close, and whoever happens to be free
takes the work. That's generalism, and it's what the colony should do while
40% of its time is idle.

In a deep economy two things change:

- **Colonists are busier.** The 30-colonist runs are 17–20% idle, not 40%.
  Busy colonists keep fresh earnings memories, reservation rates rise, and the
  low-rate options (a Master smith scraping scum) stop clearing.
- **Work becomes more of the plan.** More workshops, depots closer together,
  and longer chains (more links done at the same bench) shift ticks from
  walking to work. Skill then decides a larger share of each plan's rate.

Both are measurable in `-econ-trace` once S3 ships. That's how to check the
claim instead of trusting it.

### Profession, memories and the roster

- A colonist's **profession** is a label, not a rule: the skill it stands
  highest in, where standing is rank as a share of the skill's top rank. Raw
  ranks don't compare across curves: mining's shallow curve has eight ranks to
  cooking's four, so by rank every colonist who mines a little would be a
  miner before it was a chef. The profession changes only when another skill
  stands at least level even a rank down (`updateProfession`), so a colonist
  doesn't flip between trades at a threshold. It shows in the roster, and is
  the hook for the planned identity system ([economy.md](./economy.md)).
  Nothing in allocation reads it.
- Reaching a rank with a new label ("Became a journeyman smith.") and taking
  up a new trade ("Took up mining as a trade.") are memories, through the
  `rose-in-trade` reaction: a small, lasting lift in mood. They don't collapse
  into runs. Moving up within a label records nothing.
- A colonist's details list each skill it has a rank in, with its label and
  rank out of the top, and mark its profession, from `EntityView.Skills` in
  `SkillKind` order (principle 12). A dead master's skill dies with it. That's
  a real loss to the colony, and a story.

### What skills do in a run

After 100,000 ticks on seeds 1 and 2 (6 colonists each), whatever a colonist
arrived with, nearly all of them are a Miner, a Chef, and a Forager or
Seasoned forager, and an Apprentice smith. They're generalists, because
nothing yet makes a colonist prefer its best work (S3). Their professions
mostly come from the early game: mining dominates the first 10,000 ticks, the
colonist becomes a Miner first, and a Chef level with it a rank later doesn't
displace it. Two arrived as Journeyman smiths on seed 2 and are still
Journeymen: the armory's 4 rifles give no more smithing to do.

The effects are a colony-wide lift, not specialization: every colonist cooks
at a Chef's 120% yield by mid-game.

Starvation over 30,000 ticks on seeds 1–64, all with the cook livelock fix
from #67:

| | Starved | Seeds with any | Colonists alive at the end |
| --- | --- | --- | --- |
| Before skills | 2 | 2 | 306 |
| Skills on | 12 | 8 | 310 |
| Skills off | 9 | 6 | 301 |

"Skills off" differs from the code before skills only in the rank-up
memories' mood lift, and it starves 9 to 2. On seeds 17–64 alone, skills on
starves fewer than skills off. Starvation here is sensitive to any
perturbation of a run, not to skills as such. The deaths inspected (seed 9)
are colonists starving while they flee aliens, with the colony's meals on the
shelves.

### Configuration

The skill, effect and background tables are content in Go tables, like
`recipes`. The tunables are in `sim.Config`:

| Key | Default | |
| --- | --- | --- |
| `skills` | true | Off: no backgrounds and no effects; practice is still counted. For A/B runs. |
| `skill-practice-percent` | 100 | Percent of each unit's base ticks credited as practice. |
| `rate-memory` | 4000 | Ticks over which what a colonist earned fades back to `labor-price`. |

`plan-candidates` (4) matters more under S3. With best-of-N, it's the size of
the choice a colonist has.

## Why it is this way

- **Opportunity cost, not professions, decides who works.** A rule like
  "leave it for the professional" would force specialization in a six-person
  colony that has no use for it, and it would still need a special case for
  emergencies. Pricing time gets both, and specialization grows with the
  economy.
- **Reservation from remembered earnings, not a search.** The alternative is
  asking "what could I earn at my best skill right now?", which is a search
  over the book per decision. Remembered earnings are O(1). Letting them fade
  is what lets a stranded specialist return to general work.
- **Best rate, not first acceptable.** First-acceptable ignores comparative
  advantage entirely, and turn order decides. Best-of-N over a bounded
  candidate list is the cheapest way to let a colonist prefer what it's good
  at.
- **Competition over coordination.** Claims on bids and workshops made the
  planner a dispatcher: identical colonists with the same information, handed
  the work in turn order. Letting them race and undercut duplicates some
  work, but it's the only way a better producer wins business it wasn't
  handed.
- **Investing is a rate comparison, not a savings threshold.** A house is
  bought at `house-savings`. A workshop is bought when it pays back, so it
  goes to the colonists whose skill makes it pay.
- **Hauling isn't a skill.** Walking is the constant cost that keeps small
  colonies generalist. A skill that shrank it would erase the effect the
  design depends on.
- **Practice in base ticks, integer thresholds, yield by accumulator.**
  Comparable effort across skills; floor of a float log is wrong at exact
  powers; and nothing new draws from an RNG during play.
- **A separate RNG stream for backgrounds.** Backgrounds change behavior, so
  they aren't flavor. A new stream keeps every existing seed's other draws
  where they were.
- **No decay of practice.** Rust is good flavor, but the fading earnings
  memory (S3) will make an unused skill stop steering a colonist's choices,
  without taking the rank away.
- **Profession by standing, not rank.** By raw rank the shallowest curve wins
  every profession: in a 100,000-tick run, every colonist would be a miner.

## Extending it

- **A new skill**: a `SkillKind`, a `skillSpec` row, an effect table, a
  background weight, and `practise` calls at its completion sites. If it's
  recipe-driven, set `Skill` on its recipes.
- **Quality**: a column in the effect table. Stacks are `{Kind, Count}`, so
  graded goods would need a grade on the stack or an `ItemKind` per grade.
  That decision belongs to whoever builds it.
- **Teaching**: practising at a workshop beside a higher-ranked colonist could
  credit extra practice, which would give apprenticeship a mechanical reason
  to exist.
- **Identity**: a long-held profession can become a focus-scoring preference
  that outlasts a slightly better rate elsewhere.

Invariants: practice only rises, and only on completed work. Rank is always
derived, never stored. Nothing in skill code iterates a map to decide who
works or wins a tie.

## Open questions

- **Rifle demand is capped at 4.** Until rifles are wanted continuously
  (arming colonists, wear, a market among colonists; see
  [foundry.md](./foundry.md)), smithing is 720 base ticks per game and nobody
  can practise past Journeyman. That's fine for "rare master", but it means
  the smithing curve can't be tuned by play yet.
- **Contested bids.** With competition (S4), a novice and a Master can race
  for the same bid. The Master should usually win because it works faster,
  but a nearer novice will win some races. That's fine: it's a market, not an
  assignment. What still needs measuring is how much work is duplicated, and
  whether losers' stock clears or piles up.
- **Rifle demand.** Whether colonists should want rifles themselves (a
  willingness to pay keyed on danger, as [foundry.md](./foundry.md)
  suggests). Without it, smithing demand is fixed at 4 rifles and no smith
  ever has a reason to invest.
- **One smithing skill or two?** The foundry calls them the smith and the
  gunsmith. Splitting would halve an already tiny volume. Revisit when rifle
  demand stops being capped.
- **Professions without a skill.** Some professions are about capital or
  judgment, not practice. Shopkeeping is the example: its real skill is
  forecasting demand, which ranks and speed don't express. Shopkeeping is
  deferred ([work-market.md](./work-market.md)), but the profession label
  may need to come from what a colonist owns and does, not only from its
  highest skill.
- **Children.** When families produce colonists
  ([ages-and-family.md](./ages-and-family.md)), do they inherit a leaning, or
  learn from their parents?

## Related

- [economy.md](./economy.md) — the plan this extends; *Who does what work*.
- [work-market.md](./work-market.md) — communal work as paid orders, chosen
  by the same rate comparison.
- [valuation.md](./valuation.md) — the producer planner and `labor-price`,
  which S3 makes per colonist.
- [foundry.md](./foundry.md) — the smithing chain and the armory's cap.
- [labor.md](./labor.md) — wages and public works.
- [scumhouse.md](./scumhouse.md) — the recipe table and `Recipe.Skill`.
- [personality.md](./personality.md) — `workScale` and the traits effects
  compose with.
- [rng-streams.md](./rng-streams.md) — where the background stream goes.
- [design-principles.md](./design-principles.md) — principles 1, 3, 5, 9 and 11.

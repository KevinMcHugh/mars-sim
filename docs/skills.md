# Skills and professions (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan to give colonists **skills** they earn by doing work and start with
from their pasts, a few labelled **ranks** in each skill that make a skilled
colonist faster and more productive, and **opportunity cost** in the producer
planner so colonists drift toward the work that pays them best. That work is,
more and more, the work they're good at.

Today every colonist is equally good at everything, so the same colonist
mines, scrapes, cooks, builds and smelts steel in one afternoon (see *Who does
what work* in [economy.md](./economy.md)). The goal is not to assign careers.
It's to make careers pay, and let colonists find them. Small colonies should
stay mostly generalist, and specialization should come with a deep economy.

This is a design and a sequenced build plan. Nothing here is built. The
recipe table's `Skill SkillKind` field (always `SkillNone`) is the only piece
already in the code.

| Phase | What ships | Status |
| --- | --- | --- |
| S1 | Practice, ranks and labels; skills rolled at character generation; rank-up memories; roster display. No effect on behavior. | Proposed |
| S2 | Effects: skill makes work faster and increases yield, by more per rank for steeper skills. | Proposed |
| S3 | Opportunity cost in the producer planner: skill-aware costs, choosing the best-paying plan, and a reservation rate from what the colonist has been earning. | Proposed |
| S4 | Wages in the same comparison: public works and the colony's cook weighed against market work. | Proposed |

## Source

Nothing exists yet. The expected footprint:

- `internal/sim/skills.go` — `SkillKind` (moved from `scumhouse.go`), the skill
  and effect tables, `practise`, `rank`, the earnings memory, and the
  background table.
- `internal/sim/entity.go` — `Entity.practice [numSkills]uint32`, the
  per-skill yield accumulators, and the earnings memory.
- `internal/sim/systems.go`, `scumhouse.go` — the completion sites credit
  practice and apply speed and yield.
- `internal/sim/crashpod.go` — `arrive` rolls the background.
- `internal/sim/producer.go`, `valuation.go` — per-colonist labor cost, best-of-N
  plan choice, and the reservation rate (S3).
- `internal/sim/workorder.go`, `systems.go` (`assignWorkJob`) — wages weighed
  against plans (S4).
- `internal/sim/rng.go` — a new saved stream for backgrounds.
- `internal/sim/snapshot.go` and the TUI roster — rank labels and profession.

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
| Smithing | One skill, covering both the forge and the gun bench ([foundry.md](./foundry.md)). Splitting it would cut an already small volume in half; see *Open questions*. |

## What the colony does today

These are the numbers the design is set against. They come from default-config
runs with counters added at each completion site, 100,000 ticks, after the
foundry landed.

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

## Problems with the original brief

The first draft of this doc raised these. Most are now settled by the
decisions above. What's left of each is noted.

1. **Speed bonuses do little in an idle colony.** Still true, and now it's the
   intended mechanism. Skill only scales the work part of a job, never the
   walking. See *Why small colonies stay generalist*.
2. **A skill level alone doesn't make a smith refuse to cook.** Settled:
   opportunity cost (S3) does it, and it also lets the smith cook in an
   emergency, when hungry colonists' meal bids climb.
3. **The base doesn't set how many ranks anyone reaches; volume does.** Still
   true. Base 10 smithing with rank 1 at one gun needs 1,000 guns for rank 3.
   The fix stays: a per-skill `Unit` sets where rank 1 is, and the label list
   caps how many ranks there are. The base only sets the spacing and, now, the
   payoff.
4. **"Uses" aren't a common unit.** Practice is counted in base work ticks of
   completed work. A 60-tick rifle teaches more than a 6-tick tile.
5. **Floating-point log gets exact powers wrong.** In Go,
   `math.Log(1000)/math.Log(10)` is `2.9999999999999996`. Ranks come from an
   integer walk up the thresholds.
6. **Starting at zero lets turn order pick careers.** Settled: skills are
   rolled at character generation.
7. **Log curves favor generalists.** Now intended for small colonies. What
   pushes against it is the reward. A steeper skill pays more per rank, and
   opportunity cost makes the better-paying skill win.

## How it works (proposed)

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
table. Mining, foraging and construction credit it at their completion sites.

### Practice

`Entity.practice [numSkills]uint32` holds **base** work ticks of completed
work per skill. It's a fixed array, not a map: no allocation, no iteration
order, 20 bytes per colonist. `w.practise(e, skill, baseTicks)` is called
where a unit of work completes, the same places that compare
`e.Progress` against `scaleTicks(...)` today, scaled by a global
`skill-practice-percent`. Crediting base ticks, not actual ticks, means getting
faster doesn't slow a colonist's learning. Crediting on completion keeps
"successful": half a job teaches nothing. All completed work counts, paid or
unpaid. Practice never decays in v1.

### Ranks

```go
type skillSpec struct {
    Name   string
    Unit   uint32   // base work ticks to reach rank 1
    Base   uint32   // each later rank needs Base× the previous threshold
    Labels []string // rank 0 first; len(Labels)-1 is the top rank
}
```

Rank *r* ≥ 1 is reached at `Unit × Base^(r−1)` practice, which is
⌊log_Base(practice / Unit)⌋ + 1, computed by an integer walk and capped at the
top label. `SkillRank` is a small named type. Other systems call
`e.rank(SkillSmithing)`, and only `skills.go` sees practice.

Illustrative tables (all to be tuned):

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

Today every colonist in a small colony cooks 130–390 batches, so everyone is a
Cook or a Chef. A colonist who did most of a colony's cooking (~1,400 batches)
would be a Master chef.

**Mining** — `Unit 60` (10 tiles), `Base 2`: Rockbreaker (10 tiles), Digger
(20–40), Tunneler (80–160), Miner (320), Seasoned miner (640), Master miner
(1,280). Everyone reaches Miner in a small colony today. Two labels can share
a rank range, which keeps a fine curve under fewer names.

**Foraging** (`Base 3`) and **construction** (`Base 4`, `Unit ~40`, about 5
walls) follow the same pattern.

### Effects: steeper skills pay more

A rank changes two things, both integer, both in one table per skill:

```go
type rankEffect struct {
    TicksPct int // percent of base work ticks; 100 = unchanged. Composes with workScale.
    YieldPct int // percent of the recipe's normal output; 100 = unchanged
}
```

- **Speed** multiplies into the existing `scaleTicks` call. (`workScale` stays
  a float; skill effects are integer percents so they add no rounding of their
  own.)
- **Yield** above 100 is paid out by an accumulator, not a roll. Each run adds
  `YieldPct − 100` to `e.yieldAcc[skill]`, and every 100 adds one more output
  unit. Yield adds no RNG draws and is exact over time.

**Throughput** is `YieldPct / TicksPct`: output per base tick of work. The
decision that steeper skills pay more becomes a rule for writing these tables:
**each rank multiplies throughput by more the higher the base**. As a starting
point, about +10% per rank at base 2, +30% at base 4 and +50% at base 6,
compounded:

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

Mining tops out around 1.4× (`TicksPct 70`, no yield). You can't mine more ore
out of a tile.

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

| Roll | Chance (proposed) |
| --- | --- |
| One skill at rank 1 | 45% |
| One skill at rank 2 | 30% |
| One skill at rank 2 and another at rank 1 | 15% |
| One skill at rank 3 | 8% |
| One skill at its top rank | 2% |

Which skill is weighted per skill. Smithing gets half the weight of the
others, so a Master smith is about 1 colonist in 500, and a Smith or better
about 1 in 100. The roll sets practice to the rank's threshold, so a
background is ordinary practice from then on.

The roll affects the simulation, so it can't come from the personality stream
(principle 3). Adding it to `World.rng` would shift every later draw on every
seed. It gets its own saved stream (`skillRNG`, `Seed ^ const`, per
[rng-streams.md](./rng-streams.md)). The background is also a line in the
colonist's biography ("a smith in the Tharsis yards").

### Opportunity cost in the producer planner (S3)

This is the core of the design. It has three parts, each small.

**1. Costs are the colonist's own.** `planCraft`, `planGather` and
`planSupply` currently price labor as `laborCost(ticks)`, the same for
everyone. S3 splits a plan's ticks into the parts skill does and doesn't touch:

```
ticks   = constant ticks                  (walking, fetching, carrying: same for everyone)
        + work ticks × TicksPct / 100     (the recipe, scrape or dig: the colonist's skill)
revenue = bid price × output × YieldPct / 100
```

A skilled colonist sees more profit on the same bid, and sooner.

**2. The best plan, not the first.** `tryAssignProduce` takes the first of its
`plan-candidates` bids that clears `plan-min-profit`. S3 evaluates all of them
and takes the best **rate**: profit per tick of the plan. Ties go to the bid
ID. The candidate set stays bounded (principle 9), so this costs a few more
profit calculations per decision and no searches.

**3. A reservation rate.** A colonist takes a plan only if its rate beats what
its time is worth to it:

```
reservation = max(labor-price, remembered rate of its best-paying skill)
```

Each colonist remembers, per skill, a smoothed rate: profit per tick realized
on completed plans (and, in S4, wages per tick), in integer milli-dollars
like `priceMemory`, with the tick it last earned. A memory **fades back
toward `labor-price`** over `rate-memory` ticks without new earnings. What you
earned last year says little about what's on offer now.

What this does:

- **A smith doesn't cook.** While there are steel bids, a Master smith's
  smithing rate is high, so a scum-culture plan paying a Cook's rate fails its
  reservation.
- **Except in an emergency.** Hungry colonists bid more for meals as they get
  hungrier (up to `meal-willingness` × a meal's value; see
  [valuation.md](./valuation.md)). When meal bids climb past the smith's rate,
  it cooks. There's no special case for this. A colonist with a critical need
  of its own still bypasses all of this (`tryAssignFoodWork(e, force=true)`).
- **A master smith is wasted on a small colony.** Once the armory has its 4
  rifles, no steel bids come. The smith's smithing memory fades, its
  reservation falls to `labor-price`, and it scrapes scum with everyone else.
  If the colony posts a rifle bid again, the smith wins it: its rate on that
  bid beats anyone else's, so it's the one whose plan clears first and pays
  most.
- **A novice takes what it can get.** With no earnings history, its
  reservation is `labor-price`, and it takes any plan that clears the minimum.

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

### Wages in the same comparison (S4)

Work orders are piece rates ([labor.md](./labor.md)): a builder is paid per
tile, so a faster builder already earns more per tick. But `assignWorkJob` still
puts construction on everyone's ladder ahead of the market, so a Master smith
drops the forge to raise a wall at $2. S4 turns an open build task or the
colony's cook wage into one more candidate with a rate (wage over the
colonist's own ticks for it), judged against the same reservation. Life-support
construction (the first scumhouse, a missing toilet) stays exempt, like the
emergency builds.

### Profession, memories and the roster

- A colonist's **profession** is a label, not a rule: its highest-ranked
  skill, with hysteresis (it changes only when another skill pulls a full rank
  ahead). It shows in the roster ("Chef") and is the hook for the planned
  identity system ([economy.md](./economy.md)). Nothing in allocation reads it.
- Reaching a rank with a new label, and changing profession, are notable
  memories ("Became a journeyman smith."). They collapse the way a mining
  shift does ([memories.md](./memories.md)).
- A colonist's details show each skill's label, as snapshot fields sorted by
  `SkillKind` (principle 12). A dead master's skill dies with it. That's a
  real loss to the colony, and a story.

### Configuration

The skill, effect and background tables are content in Go tables, like
`recipes`. Tunables go in `sim.Config` with `cfg`/`doc` tags and are
regenerated into `mars-sim.yaml` in the phase that adds them:

| Key | Default (proposed) | Phase |
| --- | --- | --- |
| `skill-practice-percent` | 100 | S1 |
| `skills` | on (off: everyone untrained with no effects, for A/B runs) | S2 |
| `rate-memory` | a few thousand ticks | S3 |

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
  memory already makes an unused skill stop steering a colonist's choices,
  without taking the rank away.

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
- **Contested bids.** Colonists plan in turn order. In a small colony a novice
  will often take a bid a free Master was about to take, which is the intended
  generalism. In a deep economy it may still hand good work to the wrong
  colonist. If it does, the cheap fix is for the planner to skip a bid when a
  free colonist of higher rank in its skill is in range, using an incremental
  count, not a scan. Measure before adding it.
- **One smithing skill or two?** The foundry calls them the smith and the
  gunsmith. Splitting would halve an already tiny volume. Revisit when rifle
  demand stops being capped.
- **Traits and learning.** Should Industrious and Lazy change learning speed as
  well as work speed? Decide before S1 ships, so practice doesn't change
  meaning later.
- **Children.** When families produce colonists
  ([ages-and-family.md](./ages-and-family.md)), do they inherit a leaning, or
  learn from their parents?

## Related

- [economy.md](./economy.md) — the plan this extends; *Who does what work*.
- [valuation.md](./valuation.md) — the producer planner and `labor-price`,
  which S3 makes per colonist.
- [foundry.md](./foundry.md) — the smithing chain and the armory's cap.
- [labor.md](./labor.md) — wages and public works (S4).
- [scumhouse.md](./scumhouse.md) — the recipe table and its `Skill`
  placeholder.
- [personality.md](./personality.md) — `workScale` and the traits effects
  compose with.
- [rng-streams.md](./rng-streams.md) — where the background stream goes.
- [design-principles.md](./design-principles.md) — principles 1, 3, 5, 9 and 11.

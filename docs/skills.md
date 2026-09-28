# Skills and professions (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan to give colonists **skills** they earn by doing work, **ranks** in each
skill that the code can switch on, and a **profession**: the trade a colonist
goes back to first when it looks for work. Today every colonist is equally good
at everything, so the same colonist mines, scrapes, cooks and builds in one
afternoon (see *Who does what work* in [economy.md](./economy.md)). The point is
behavior, not a stat sheet. A trained cook should keep cooking. A master smith
should leave the cooking to someone else unless the colony is starving.

This is a design and a sequenced build plan. Nothing here is built. The
recipe table's `Skill SkillKind` field (always `SkillNone`) is the only piece
already in the code.

| Phase | What ships | Status |
| --- | --- | --- |
| S1 | Practice, ranks, labels, rank-up memories, roster display. No effect on behavior. | Proposed |
| S2 | Professions, arrival backgrounds, and professionals getting first claim on their work. | Proposed |
| S3 | Per-skill effects: speed and yield. | Proposed |
| S4 | Opportunity cost: a colonist's own labor price comes from its best trade. | Proposed |
| S5 | Smithing, once there is a metal-and-weapons recipe chain for it to be a skill in. | Proposed |

## Source

Nothing exists yet. The expected footprint:

- `internal/sim/skills.go` — `SkillKind` (moved from `scumhouse.go`), the skill
  table, `practice`, `rankOf`, `profession`, and the per-skill effect lookups.
- `internal/sim/entity.go` — `Entity.practice [numSkills]uint32` and
  `Entity.profession SkillKind`.
- `internal/sim/systems.go`, `scumhouse.go`, `cleaning.go` — the completion
  sites below each call `w.practise(e, skill, baseTicks)`, and `assignWorkJob`
  learns professions (S2).
- `internal/sim/crashpod.go` — the arrival background (S2).
- `internal/sim/valuation.go` / `producer.go` — per-colonist labor cost (S4).
- `internal/sim/snapshot.go` and the TUI roster — rank labels and profession.
- `internal/sim/rng.go` — a new saved stream for backgrounds (S2).

## The brief

This is the proposal as it was put to us, so the rest of the doc can say where
it keeps it and where it departs:

- Skills have only a few levels, not a 0–100 number.
- A colonist's level in a skill is `log_X(n)`, where `n` is how many times it
  has used the skill successfully and `X` is a per-skill base. A small base
  (`log2`) is a shallow learning curve with many gradations; a large one
  (`log10`) is a steep curve with few.
- Each level of each skill has its own label.
- Code for each skill decides what each level gets you. For example, mining
  could be `log2`: constant speed at levels 0–6, 25% faster at 6–10, and 10%
  less time per level above that. Smithing could be `log10` with three or four
  ranks, a master making guns faster (and, later, better).
- Artifact quality is out of scope. What matters first is letting colonists
  choose their own economic opportunities.

Most of this survives. Levels as a short, labelled enum is principle 1 of
[design-principles.md](./design-principles.md) (expose an enum other code can
switch on, keep the number private). A per-skill curve and a per-skill effect
table are the right split between data and code. The problems are below.

## What the colony does today

Numbers first, because they decide most of the design. These come from a
default-config run (6 colonists, 80×40 map) with counters added at each
completion site, 100,000 ticks, seeds 9 and 42.

**Units of work completed per colonist, seed 9:**

| Colonist | Tiles mined | Scum scraped | Recipes cooked | Walls built | Fixtures built |
| --- | --- | --- | --- | --- | --- |
| c1 | 409 | 406 | 301 | 13 | 2 |
| c2 | 507 | 566 | 194 | 15 | 1 |
| c3 | 478 | 582 | 202 | 15 | 2 |
| c4 | 457 | 40 | 289 | 12 | 3 |
| c5 | 486 | 653 | 242 | 23 | 5 |
| c6 | 422 | 665 | 213 | 7 | 1 |
| **Colony** | **2,759** | **2,912** | **1,441** | **85** | **14** |

Seed 42 looks the same. Almost all mining happens in the first 10,000 ticks,
until the reachable frontier runs out. After that, colonists alternate between
scraping and cooking.

**Where a colonist's time goes, seed 9, all colonists, 100,000 ticks:**

| State | Share | | State | Share |
| --- | --- | --- | --- | --- |
| idle | 40.6% | | eating | 6.1% |
| talking | 15.6% | | hauling | 5.9% |
| moving | 11.7% | | sleeping | 5.2% |
| crafting | 4.4% | | relieving | 4.0% |
| scraping | 3.3% | | building | 0.2% |
| mining | 3.0% | | cleaning | < 0.1% |

Three things follow from this:

1. **Everyone does everything.** No colonist has a trade. Mining and cooking
   are within about 1.6× between the busiest colonist and the least busy one.
   The gaps that do show up (c4 barely scraped on seed 9, c6 on seed 42) are
   accidents of the work ladder that don't persist from one seed to the next.
   They aren't careers.
2. **The colony is short of demand, not labor.** Hands-on work is about 11% of
   a colonist's time and idle is 41%. Output is capped by what's bid for (the
   meal reserve, the colony's standing bids), not by how fast anyone works.
3. **Volumes differ by two orders of magnitude between skills.** A colony
   mines thousands of tiles and builds dozens of walls. Guns it doesn't make
   at all.

## Problems with the brief

These are the flaws, roughly in order of how much they would hurt.

### 1. Faster work changes almost nothing while colonists are idle

The brief's effects are speed bonuses. In a colony that is idle 41% of the
time and demand-limited, a miner who digs 25% faster produces the same ore
and gets more idle time. Worse, the actual work is a small slice of each job.
A mined tile is 6 ticks of work. Moving and hauling take about 17% of
colonists' time, against 11% for all the hands-on work combined. So a 25% speedup on the work is a single-digit change in the
job's total cost, and too small to move any profit comparison.

Skills only change the economy if they change **who gets the work**. That's
why professions (S2) come before effects (S3) in this plan, and why the
effects that matter most in the current economy are **yield** (more output
from the same inputs, which matters when biomatter is scarce) and not speed.
Speed starts to matter once labor is scarce: a bigger colony with more kinds
of work, or a famine.

### 2. A skill level doesn't make a smith refuse to cook

A master smith is exactly as good a cook as a novice. If no smithing bid is
open and a meal bid is, a rational colonist cooks. Skills alone produce
*comparative* advantage, and that only turns into careers if something makes
colonists compare. Nothing does today:

- `assignWorkJob` is a fixed ladder (storage, construction, food, cleaning,
  scraping, selling meals, hauling, the producer planner, mining). Every
  colonist walks it in the same order.
- The producer planner takes the **first** bid that clears `plan-min-profit`.
  It doesn't compare that bid against what the colonist could earn doing
  something else.
- Colonists act in ascending ID order, so on a tie the lowest ID claims first.

"Won't cook except in an emergency" is two things the brief doesn't name.
One is **opportunity cost**: a master smith's hour is worth more, so a meal
bid has to be high to be worth its time (S4). The other is **identity**: the
smith thinks of itself as a smith (S2's profession, and later the identity
system [economy.md](./economy.md) already plans). With opportunity cost priced
in, the emergency case needs no special code. Hungry colonists' meal bids rise
toward `meal-willingness` × a meal's value ([valuation.md](./valuation.md)),
and once they are high enough, the smith cooks.

### 3. The base doesn't set how many ranks a skill has; how much work there is does

`log_X(n)` has no ceiling. A skill's rank count is `log_X` of the most practice
anyone gets in a lifetime, and that depends on how much of that work the colony
wants, which differs by skill:

- **Smithing at `log10`** puts rank 1 at 10 guns, rank 2 at 100 and rank 3 at
  1,000. A colony that wants a few dozen guns in a run never has a smith past
  rank 1. The "three or four ranks" are there on paper, and nobody ever
  reaches the top one.
- **Mining at `log2`**, counting tiles, puts the brief's 25% bonus (level 6)
  at 64 tiles and the next bonus (level 10) at 1,024. Today every colonist
  passes 64 tiles in its first 10,000 ticks, so the bonus says nothing about
  anyone. Level 10 would need a single colonist to dig over a third of
  everything the colony ever mines.
- **Construction** is 85 walls for a whole colony. At any base, nobody gets far.

To have *k* ranks and reach the top one in a lifetime of *N* units, the base
has to be about `N^(1/k)`. So the steep, rare craft (smithing) needs a
**small** base, the opposite of the intuition. The fix is to set two things
separately, not with one base: **how many ranks** (the label list, which also
caps rank) and **where the first one is** (a per-skill practice unit). The
base then only sets how the ranks are spaced. See *Ranks* below.

### 4. "Uses" aren't a common unit

A use is whatever unit the code happens to complete: a 6-tick tile, a 10-tick
animal carcass, a 30-tick alien carcass, a wall. Splitting a recipe in two
would double how fast cooks learn, and one alien carcass would teach no more
than one rat. Practice should be counted in **base work ticks of successful
work**, credited when the unit completes. That keeps "successful" (half a job
teaches nothing) and makes one number mean the same effort in every skill.
Base ticks, not the colonist's actual ticks: otherwise a master's speed would
slow its own learning.

### 5. Floating-point log gets exact powers wrong

In Go, `math.Log(1000)/math.Log(10)` is `2.9999999999999996`, so a colonist
with exactly 1,000 practice would be rank 2, not 3. `log_10(1e6)` floors to 5.
`log_5(125)` floors correctly only because it happens to round up. Ranks must
come from an integer walk up the thresholds (multiply by the base until the
threshold passes practice), never from `math.Log`. Integer thresholds are also
what the determinism rules ask for ([determinism.md](./determinism.md)).

### 6. Everyone starts at zero, so turn order picks the careers

If all colonists start untrained and practice feeds back into who gets the
work (S2), the first few jobs decide careers. Those go to whoever the ladder
and turn order reach first, which is the lowest IDs. The colony's first cook
would be colonist 1 because it was colonist 1. Colonists need a starting
**background** (some practice in one skill when they arrive), rolled from
the seed. Then careers start from something a player can read ("she was a
cook on Earth") instead of an ID.

### 7. Log curves favor generalists

With diminishing returns, splitting practice evenly across *k* skills costs
only `log_X(k)` ranks in each. At base 2, a colonist that does three kinds of
work is about 1.6 ranks behind a specialist in each of them. With no other
pressure, practice alone won't make careers. Specialization has to come from
the choice of work reinforcing itself (S2): the cook gets first claim on
cooking, so it cooks more and gets better. The curve's job is to make that
loop *settle* instead of running away. A log curve is good for that.

### 8. Some ranks do nothing

"Levels 0–6 mine at a constant rate" means six ranks that have no effect.
That's fine if each still has a label a player can read. It's also exactly the
"meaningless bits" the brief wants to avoid. Every rank should either change
something or name something. A rank that does neither should be merged into
its neighbor.

### 9. "25% faster" is ambiguous

It can mean 25% more throughput (80% of the ticks) or 25% fewer ticks (133%
throughput). Effects should be written as **percent of base ticks**, the way
`workScale` already works. Then "80" means one thing everywhere.

### 10. There is nothing to smith yet

No recipe makes metal or weapons. Guns arrive only in crash pods and supply
drops ([crash-pods.md](./crash-pods.md), [director.md](./director.md)), and
nobody posts a bid for one. Smithing needs a chain (iron ore to ingot to gun)
and a buyer before it can be a skill anyone practices, so it comes last (S5).
The design has to allow for it, but until then nothing can test it.

## How it works (proposed)

### Skills

The v1 skills map onto work that already exists:

| Skill | Practised by | Credited at |
| --- | --- | --- |
| `SkillMining` | mining a tile; digging a room's floor | `systems.go` mine completion, `jobBuild` floor |
| `SkillConstruction` | raising walls and fixtures | `jobBuild` completion |
| `SkillCooking` | working any scumhouse recipe | `jobCraft` completion |
| `SkillForaging` | scraping cave scum | scrape completion |
| `SkillSmithing` | (S5) metal and weapon recipes | recipe completion |

Hauling and cleaning are **not** skills in v1. Hauling is walking, and a
skill that makes walking faster would leak into everything. Cleaning is under
0.1% of anyone's time. Recipes already carry a `Skill` field, so cooking and
smithing get their skill from the recipe table. The others are credited
directly at their completion sites.

### Practice

`Entity.practice [numSkills]uint32` holds base work ticks of successful work
per skill. It's a fixed array, not a map, so there's no allocation and no
iteration order to worry about, and it costs 4 bytes per skill per colonist.
`w.practise(e, skill, baseTicks)` adds to it at the completion sites listed
above (the same places `scaleTicks(..., e.workScale)` is compared today),
scaled by a global `skill-practice-percent` so tuning runs and tests can speed
learning up. Practice never decays in v1 (see *Why it is this way*).

Work done for anyone counts: paid, unpaid, a colonist's emergency build for
itself. Practice is about what you did, not who paid for it.

### Ranks

Each skill has one row in a table in Go, like `recipes`:

```go
type skillSpec struct {
    Name   string
    Unit   uint32   // base work ticks to reach rank 1
    Base   uint32   // each later rank needs Base× the previous threshold
    Labels []string // one per rank, rank 0 first; len(Labels)-1 is the top rank
}
```

Rank *r* ≥ 1 is reached at `Unit × Base^(r−1)` ticks, and rank 0 is
"untrained". That's the brief's `log_X(n)` (rank = ⌊log_Base(n / Unit)⌋ + 1),
with the two knobs from problem 3 added: `Unit` sets where the curve starts,
and the label list caps how high it goes. `rankOf` walks thresholds in integer
arithmetic and stops at the top label.

`SkillRank` is a small named integer type, and other systems ask for
`e.rank(SkillCooking)`. The practice count stays private to `skills.go`.

Illustrative tables, set against the measured volumes above (all to be tuned):

**Mining** — `Unit 60` (10 tiles), `Base 2`

| Rank | Label | Ticks | ≈ Tiles | Who gets there today |
| --- | --- | --- | --- | --- |
| 0 | Untrained | 0 | 0 | |
| 1 | Rockbreaker | 60 | 10 | everyone, early |
| 2 | Digger | 120 | 20 | |
| 3 | Digger | 240 | 40 | |
| 4 | Tunneler | 480 | 80 | |
| 5 | Tunneler | 960 | 160 | |
| 6 | Miner | 1,920 | 320 | everyone (~450 tiles each) |
| 7 | Seasoned miner | 3,840 | 640 | a colonist who digs ~¼ of the colony's total |
| 8 | Master miner | 7,680 | 1,280 | a specialist, about half the colony's total |

With practice split evenly, as today, everyone ends at rank 6. A specialist,
as S2 would produce, reaches 8. Ranks 2 and 3 share a label, and so do 4 and
5. That's allowed. It keeps a fine-grained curve for effects while showing
fewer names.

**Cooking** — `Unit 120` (~10 batches), `Base 4`

| Rank | Label | Ticks | ≈ Batches |
| --- | --- | --- | --- |
| 0 | Untrained | 0 | 0 |
| 1 | Kitchen hand | 120 | 10 |
| 2 | Cook | 480 | 40 |
| 3 | Chef | 1,920 | 160 |
| 4 | Master chef | 7,680 | 640 |

Today every colonist cooks about 240 batches and reaches rank 3. A specialist
cooking for six colonists (~1,400 batches) reaches rank 4.

**Smithing (S5, hypothetical)** — if a gun is ~300 base ticks and a colony
wants ~30 in a run, `Unit 300, Base 3` gives Apprentice at 1 gun, Journeyman
at 3, Smith at 9 and Master at 27. With `Base 10`, the top rank would need
1,000 guns.

Construction (`Base 3`) and foraging (`Base 2`) follow the same pattern. With
85 walls a run, construction's `Unit` has to be small (~40 ticks, 5 walls) for
anyone to rank up at all.

### Effects

Each skill's own code decides what a rank does, through a per-skill table
indexed by rank. That keeps the effects in one named place (principle 1) and
not in ad hoc `if rank >= 6` checks:

```go
type rankEffect struct {
    TicksPct int // percent of base work ticks; 100 = no change. Composes with workScale.
    Yield    int // skill-specific: an extra output unit every Yield runs; 0 = none
}
```

- **Speed** multiplies into the existing `scaleTicks` call as a percent.
  (`workScale` is already a float and stays one; skill effects are integer
  percents so they can't add rounding of their own.)
- **Yield** is the lever that matters while labor is plentiful (problem 1). A
  master chef might get a fifth meal from an alien carcass, and a skilled
  forager an extra unit of scum per patch. Yield is counted, not rolled (an
  extra unit every N runs), so it adds no RNG draws.
- **Quality** is out of scope, as the brief says. The table is where it would
  go.

The brief's mining curve becomes: ranks 0–5 at `TicksPct 100`, 6–7 at `80`,
and 8 at `70`.

### Professions and who gets the work (S2)

This is what makes skills matter.

- **Profession.** A colonist's profession is its highest-ranked skill once
  that rank reaches `profession-rank` (a config value). Ties go to the skill
  it already has, then to `SkillKind` order. It has **hysteresis**: the
  profession changes only when another skill is ahead by a full rank, so a
  colonist doesn't flip-flop at a threshold. It's an enum on the entity, shown
  in the roster ("Chef"), and changing it is a life event.
- **Profession first.** In `assignWorkJob`, after unloading and after the
  life-support tier, a colonist tries its profession's work first. A cook
  cooks, then scrapes for the scumhouse, before looking at anything else.
- **Leave it for the professional.** A colonist whose profession is something
  else passes over work in a profession while a member of that profession is
  **available** (alive, without a job). "Available" is a per-profession count
  kept incrementally as jobs are set and cleared, like the job board's
  counts ([spatial-index-and-performance.md](./spatial-index-and-performance.md)),
  not a scan over colonists.
- **Emergencies override it.** A colonist with a critical fatal need already
  bypasses the ladder (`tryAssignFoodWork(e, force=true)`: cook your own scum,
  scrape to keep). That doesn't change. Life-support construction (the
  colony's first scumhouse, a missing toilet) is also exempt from yielding.
- **Arrival backgrounds.** `arrive` gives each colonist starting practice in
  one skill, enough for rank 2 or so, picked from a weighted table. The draw
  affects the simulation, so it can't come from the personality stream. Adding
  it to `World.rng` would shift every later draw on every seed, so it gets its
  own saved stream (`skillRNG`, `Seed ^ const`, per
  [rng-streams.md](./rng-streams.md)). A background is also a good line for
  the colonist's biography.

Construction is the awkward case. Today everyone claims build tasks before
anything else, at fixed wages ([labor.md](./labor.md)). Under S2, builders get
first claim and everyone else yields while a builder is available. That's the
same rule, but it will slow public works in a colony with no builders until
one emerges. That's accurate, and backgrounds make sure there are some.

### Opportunity cost (S4)

`laborCost(ticks)` is the same for every colonist today (`labor-price` per 100
ticks). S4 makes it per colonist and per job:

```
cost = own ticks for this job (after skill speed) × own labor price
own labor price = max(labor-price, what its profession has been earning per tick)
```

"What its profession has been earning" is a smoothed per-colonist figure,
updated as income arrives (sales from plans, wages from work orders), in
integer milli-dollars like `priceMemory`. A master smith's plan to fill a meal
bid now has to beat its smithing income, so it passes on ordinary meal bids and
takes them once hunger has pushed bids high enough. That's the brief's "except
in an emergency", and it needs no special case. A novice's cost is higher per
unit (more ticks), so for the same bid a skilled producer clears the profit
threshold before an unskilled one.

S4 comes after S2 because it needs income history that only professions
produce, and because a price-only mechanism can still let the lowest ID win
among equally priced colonists.

### Memories and the roster

- Reaching a rank with a new label is a notable memory ("Became a chef."), and
  so is changing profession. Both collapse the way a mining shift does
  ([memories.md](./memories.md)).
- The roster shows the profession, and a colonist's details show each skill's
  label, as snapshot fields sorted by `SkillKind` (the simulation doesn't know
  how it is drawn: principle 12).
- A dead master's practice dies with it. That's a real loss to the colony and
  a better story than a respawned stat.

### Configuration

The skill and effect tables are content in Go tables, like `recipes`. The
tunables go in `sim.Config` with `cfg`/`doc` tags and are regenerated into
`mars-sim.yaml` in the phase that adds them:

| Key | Default (proposed) | Phase |
| --- | --- | --- |
| `skill-practice-percent` | 100 | S1 |
| `profession-rank` | 2 | S2 |
| `background-practice` | rank 2 of the rolled skill | S2 |
| `skills` | on (off restores today's everyone-does-everything behavior for A/B runs) | S2 |

## Why it is this way

- **Allocation before speed.** The measurements say the colony has idle hands
  and limited demand. A speed bonus there changes nothing anyone can see. A
  rule about who gets first claim on the work changes careers immediately.
- **Practice in base ticks, credited on completion.** It's the one unit that
  means the same effort across skills, and it keeps the brief's "successful".
  Crediting base ticks, not actual ticks, means getting faster doesn't slow
  learning.
- **Label list plus unit, not only a base.** The brief's base conflated how
  steep the curve is with how many ranks exist. How many ranks are reachable
  depends on how much of that work a colony does, which the base can't know.
- **Integer thresholds.** Floor of a float log is wrong at exact powers, and
  the simulation has to be exactly reproducible.
- **No decay in v1.** Rust ("hasn't cooked in a year") is good flavor. It also
  means a per-tick or lazy decay model, and a way for professions to erode.
  The log curve already makes old ranks expensive to beat, which is most of
  what decay would do. Revisit when professions need to change more often.
- **A separate RNG stream for backgrounds.** Backgrounds change behavior, so
  they can't be flavor (principle 3). A new stream keeps every existing seed's
  other draws where they were.
- **Profession as an enum, not a score read everywhere.** Other systems (the
  work ladder, memories, the roster, later identity and affect) switch on "is
  a cook". Only `skills.go` knows the thresholds.

Alternatives considered and set aside:

- **A 0–100 skill.** The brief rejects it, and principle 1 agrees.
- **Pure market allocation** (only S4, no professions). It gets the emergency
  case right, but it depends on income history that doesn't exist at the
  start, and it leaves ID-order tie-breaks in charge. It's worth having as a
  layer on top of professions, not in their place.
- **A global matcher** that assigns each job to the best available colonist.
  It's correct, but it costs a scan per job per tick at 2,000 colonists
  (principle 9). The "available professionals" count gets most of the benefit
  at O(1).

## Extending it

- **A new skill**: a `SkillKind`, a `skillSpec` row, an effect table, and
  `practise` calls at its completion sites. If it is recipe-driven, set
  `Skill` on its recipes and the craft path does the rest.
- **Quality**: a column in the effect table. Items would need a quality on the
  stack (or a separate `ItemKind` per grade, which keeps stacks as
  `{Kind, Count}`); that decision belongs to whoever builds it.
- **Identity**: the profession enum is the hook. A colonist long in a
  profession can get a focus-scoring preference for its work that outlasts a
  better margin elsewhere (see [economy.md](./economy.md)).
- **Teaching**: practising alongside a higher-ranked colonist could credit
  extra practice. It would give apprenticeship a mechanical reason to exist.

Invariants a change must keep: practice only goes up, and only on completed
work. Rank is always derived and never stored. No skill code iterates a map to
decide who gets work.

## Open questions

- Should a background ever be a skill the colony has no work for yet
  (smithing before S5)? Probably yes, for flavor, with no effect until the
  work exists.
- Should traits (Industrious, Lazy) change learning speed as well as work
  speed? It's cheap, and it would make the traits read better. It needs
  deciding before S1 ships, so practice semantics don't change later.
- With professions, does a six-colonist colony have enough people to cover
  mining, construction, cooking and foraging? If not, `profession-rank` should
  scale with colony size, or small colonies should mostly stay generalists.

## Related

- [economy.md](./economy.md) — the plan this extends; *Who does what work*
  names skills and identity as the two forces toward careers.
- [valuation.md](./valuation.md) — the producer planner and `labor-price`,
  which S4 makes per colonist.
- [labor.md](./labor.md) — public works and wages, which professions reorder.
- [scumhouse.md](./scumhouse.md) — the recipe table and its `Skill`
  placeholder.
- [personality.md](./personality.md) — `workScale` and the traits that effects
  compose with.
- [rng-streams.md](./rng-streams.md) — where the background stream goes.
- [design-principles.md](./design-principles.md) — principles 1, 3, 5, 9 and
  11 are all in play here.

# Safety and bodyguards

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan for a fifth colonist drive, **safety**, and the first feeling that
turns into a ladder of purchases: a colonist who feels unsafe buys a gun,
then a better gun, and finally, if it is rich, hires an armed colonist to
walk with it as a **bodyguard**, paid by the hour from its own wallet.

Safety builds from what the colonist goes through: seeing an alien, a
gunfight or a colonist killed, hearing about those things in conversation,
and time spent out on the frontier rather than at home. It falls at home,
in a staffed defense zone, and with a bodyguard at its side. Once it presses,
the colonist **wants** protection, and a general want layer (the one
`mealBidLimit` is today, for hunger alone) turns that into an order on the
book: a bid for a gun, a bid for a better one, and at the top, an escort
contract an armed colonist takes the way it takes a post in a
[defense zone](./defense.md).

S1 to S4 are built: the safety drive, what moves it, the traits and the
guarding skill that temper it, hearsay, the nightly news, and wants, with
safety's whole ladder (a gun, a better gun, a bodyguard). S5 (what an afraid
colonist does) and S6 are proposals. *Built so far* below records where the build departed from the
plan and what it measured.

## Decisions already made

| Topic | Decision |
| --- | --- |
| Who guards | Only colonists who feel safe. A guard or bodyguard takes a post only while its own safety is calm, and gives the post up once it is uneasy. Guards never need guards. |
| Who is not afraid | Disposition, not luck: traits for outlook and boldness, and a guarding skill learned on duty, make a colonist less susceptible to fear. Bodyguards are the colonists those leave calm. |
| What a bodyguard knows | Only what is on the market: the contract's rate, hours and escrow, and where the client is. Never the client's drives, affect or memories. |
| How deaths get around | Two ways: hearsay as people talk, and the **nightly news**, a broadcast every evening of the day's major news. Deaths first; arrivals next; more kinds later. |
| What fear buys | A ladder, one rung at a time: a gun, a better gun, a bodyguard. A colonist climbs only while it still feels unsafe with what it has. |
| Who can afford a guard | Only the rich. A bodyguard costs several times a defense post's wage, and a colonist spends on one only out of savings it would not miss, so a typical purse buys guns and never a guard. |
| Defense over a home | Not a case. Home is already the safest place class, and a defense zone over it changes nothing for safety. |

| Phase | What ships | Status |
| --- | --- | --- |
| S1 | The safety drive: a place-dependent rate (home, settled, frontier, guarded), bumps from percepts through a `drives` field on reactions in `cognition.yaml`, the felt-unsafe experience at the ceiling, and the disposition traits (Bold/Timid, and safety rules on nerve and outlook). No behavior beyond affect. | Shipped |
| S2 | Hearsay and the nightly news: a recounted memory reaches the listener as a percept on a new `hearsay` channel, so hearing about a killing or a sighting is a reaction row; an evening broadcast on a `broadcast` channel reports the day's deaths and arrivals. | Shipped |
| S3 | Wants: one willingness-to-pay rule for any drive, ladders of rungs, the hook that posts an order when a drive presses, and the inspector line that explains it. Safety's first two rungs: a gun, then a better gun, with the old one sold on; pistols and shotguns machined at the gun bench (pulled forward from S6). | Shipped |
| S4 | Bodyguards: `WorkEscort` contracts escrowed from the client, `JobEscort` as a guard post that follows a colonist, protection as a place class, the calm-to-serve rule for every guard post, the guarding skill, and escorts on the Market tab. | Shipped |
| S5 | Critical safety: retreating home, turning down frontier work, restless nights. | Proposed |
| S6 | Chipping in to a defense zone's wages. | Proposed |

## Source

Built (S1, S2):

- [`internal/sim/drives.go`](../internal/sim/drives.go): `DriveSafety` and
  its `DriveSpec` in `defaultDrives`; `DriveSpec.Place`, `PlaceDepth`,
  `Armed` and `Unsought`; the place and weapon steps in `driveRate`;
  `mostUrgentDrive` skipping an unsought drive.
- [`internal/sim/place.go`](../internal/sim/place.go): `PlaceClass` (home,
  settled, frontier, guarded), `placeOf`, `guardedAt`, `atHome`,
  `homeRooms`, `placePercent`, and the daring stream's seed.
- [`internal/sim/drive_activity.go`](../internal/sim/drive_activity.go):
  `syncDriveActivity` also watches the place class and the best weapon.
- [`internal/sim/focus.go`](../internal/sim/focus.go): an unsought drive puts
  no focus forward and never wakes the colonist to think.
- [`internal/sim/drive_bands.go`](../internal/sim/drive_bands.go):
  `ConsequenceUnsafe`, an experience like loneliness.
- [`internal/sim/personality.go`](../internal/sim/personality.go),
  [`recruit.go`](../internal/sim/recruit.go), [`rng.go`](../internal/sim/rng.go):
  the `daring` group (Bold, Timid) on its own stream (`rollOwnGroup`,
  `addDaring`, `daringRNG`); safety rates on Resilient and Cowardly.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go):
  `ReactionSpec.Drives` (`DriveBump`) and its YAML `drives` field, the
  `hearsay` and `broadcast` channels (`deliveredChannel`: no perception rule
  may sense on them), the `danger`, `ship`, `die` and `arrive` vocabulary, the
  bumps, the hearsay and news reactions, and the daring and outlook trait
  rules.
- [`internal/sim/affect.go`](../internal/sim/affect.go): `applyDriveBumps`,
  at the wear and trait scale the affect was read at; hearsay's hop scaling.
- [`internal/sim/entity.go`](../internal/sim/entity.go),
  [`world.go`](../internal/sim/world.go): `Memory.Gist` (`MemoryGist`),
  recorded by `rememberPercept`; the news fields; reporting a death from
  `remove`.
- [`internal/sim/topics.go`](../internal/sim/topics.go),
  [`systems.go`](../internal/sim/systems.go): `tellHearsay` and `knows`,
  called from `finishTalk`; `recountableMemories` retelling what was heard;
  `airNews` and `hearNews` in `step`.
- [`internal/sim/news.go`](../internal/sim/news.go): `NewsItem`,
  `reportNews`, `airNews`, `hearNews`, `reportDeath`, `reportShipArrival`
  (from `landShip`), `reportRecruits` (from `hireRecruits`).
- [`cognition.yaml`](../cognition.yaml), [`mars-sim.yaml`](../mars-sim.yaml):
  the rows and settings above.
- [`internal/sim/safety_test.go`](../internal/sim/safety_test.go).

Built (S3):

- [`internal/sim/wants.go`](../internal/sim/wants.go): `wantRung`,
  `wantSpec`, `wantState`, `willingness`, `postWants` (`runWant`),
  `wantLine` (the inspector's sentence); safety's ladder (`safetyRungs`,
  `gunValue`, `bestGunOwned`, `bidForGun`); fetching a bought gun
  (`tryFetchGun`, `jobFetch`, `JobFetch`) and putting the old one on sale.
- [`internal/sim/scumhouse.go`](../internal/sim/scumhouse.go): the pistol
  and shotgun recipes at the gun bench.
- [`internal/sim/foundry.go`](../internal/sim/foundry.go): `ownStockFor`
  never counts the gun in a colonist's hand.
- [`internal/sim/systems.go`](../internal/sim/systems.go): `postWants` after
  every colonist turn; `tryFetchGun` first in `assignWorkJob`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go),
  [`internal/wire/inspect.go`](../internal/wire/inspect.go),
  [`web/src/ui/InspectPanel.svelte`](../web/src/ui/InspectPanel.svelte):
  `EntityView.Want`, the entity topic's `want`, shown under Drives.
- [`internal/sim/wants_test.go`](../internal/sim/wants_test.go).

Built (S4):

- [`internal/sim/escort.go`](../internal/sim/escort.go): `tryAssignEscort`,
  `jobEscort`, `serveEscort`, `releaseEscort`, `escortedNow`,
  `hireBodyguard`, `escortRate`, `lapseEscort`, `calmEnoughToGuard`,
  `nervePercent`.
- [`internal/sim/workorder.go`](../internal/sim/workorder.go): `WorkEscort`,
  and the contract's escort, hours served and lapse tick.
- [`internal/sim/wants.go`](../internal/sim/wants.go): the bodyguard rung;
  a rung may post a work order (`wantPosted`) and have a budget rule
  (`afford`).
- [`internal/sim/skills.go`](../internal/sim/skills.go): `SkillGuarding`.
- [`internal/sim/defense.go`](../internal/sim/defense.go): calm to serve,
  and guarding practice for time on station.
- [`internal/sim/place.go`](../internal/sim/place.go),
  [`drives.go`](../internal/sim/drives.go),
  [`affect.go`](../internal/sim/affect.go): beside one's bodyguard is
  guarded; a guarding rank cuts safety's growth and sight bumps.
- [`cognition.yaml`](../cognition.yaml): `guarded-client`,
  `hired-bodyguard`, and `guarded` (the client feeling its guard come
  alongside, delivered by `noteGuarded` in escort.go).
- [`internal/wire/boards.go`](../internal/wire/boards.go),
  [`web/src/ui/MarketPanel.svelte`](../web/src/ui/MarketPanel.svelte): the
  market topic's `escorts`, and the Market tab's Bodyguards section.
- [`internal/wire/zones.go`](../internal/wire/zones.go),
  [`web/src/map/renderer.ts`](../web/src/map/renderer.ts) (`setTethers`):
  the zones topic's `escorts` pairs, and the line from guard to client on
  the map.
- [`internal/sim/escort_test.go`](../internal/sim/escort_test.go).

## How it works

### The safety drive

Safety is a drive like the other four (see [drives.md](./drives.md)): a
level from 0 (feels safe) to `Max` (terrified), lazily evaluated, cut into
bands. It is a drive and not a fourth affect axis because it has to do the
things drives do and affect does not: accumulate rather than decay toward a
home, cross thresholds on a schedule, and (from S3) say how much a colonist
would pay. Affect still carries how it *feels*, through the reactions below.

It is the first **unsought** drive (`DriveSpec.Unsought`): felt, not acted
on. It is never the most urgent drive, puts no focus forward, and does not
make a pressing colonist think every tick. Its bands and its ceiling
consequence apply as for any drive. S5's retreat is what would give it a
focus. Colonists land calm: an unsought drive starts at 0 rather than at a
staggered random level, which also keeps the simulation stream's draws
where they were.

| Drive | Rate | SeekAt | CriticalAt | Max | At the ceiling |
| --- | --- | --- | --- | --- | --- |
| safety | 1000 × place% | 600 | 850 | 1000 | felt unsafe, every 200 ticks |

Unlike the other four, safety has no facility. It is met by *where the
colonist is and who is with it*, so most of the design is the rate.

The band names are for people, not code: **calm** (satisfied, growing),
**uneasy** (pressing), **afraid** (critical), **terrified** (ceiling). The
inspector shows them; other systems read the phase, never the number (see
[design-principles.md](./design-principles.md), rule 1).

### Where the colonist is: place classes

The roadmap in [drives.md](./drives.md) already planned for drives whose
rate depends on where the colonist is (beauty and comfort): a cached class,
folded in by `refreshDrive` when it changes, exactly as drive activity is.
Safety is the first. A new step joins `driveRate` after activity:

```
rate = rate × place%      // DriveSpec.Place[placeClass], 100 for every other drive
```

| Place | Where | safety place% |
| --- | --- | --- |
| home | the colonist's own house, or the rooms of the ship it landed in | −300 |
| settled | any zoned ground: every room stands in a zone (the player's or zoning-auto's), and ships hold theirs as residence | −50 |
| frontier | anywhere else: dug passages, caverns, open rock faces, and every level but the landing level, which has no zones | +200, plus `drives.safety.place-depth` (50) per level below the landing level |
| guarded | inside a defense zone that someone else is on station in (later: within `escort-radius` of its own bodyguard) | −200 |

Guarded outranks the rest: a colonist with a bodyguard in a cavern is
guarded, not on the frontier. At the shipped base rate a colonist out on
level 1's frontier goes from calm to uneasy in about 300 ticks, a little
under seven hours, and a night at home takes it from the ceiling to zero.
Two levels down it is about half that.

As built, `placeOf` runs after every colonist turn, in `syncDriveActivity`,
which refreshes the drives only when the class (or the drive activity, or
the best weapon) has changed. It is cheap enough not to need the cleverer
plan: a zone lookup, the colonist's ship's bounds, the rooms it
commissioned (indexed by commissioner once per change to the structure
registry, `homeRooms`), and the defense posts, which are few. Using zones
for "settled" rather than rooms means ground the player has zoned but not
yet built on counts as settled: the colony has claimed it.

Carrying a weapon is a trait-like modifier, not a place, and a better gun
calms more: an armed colonist's safety rate changes by its best weapon's
`drives.safety.armed.*` percent, −25 for a pistol, −35 for a shotgun and −50 for
an assault rifle. That difference is what makes the second rung of the
ladder (below) worth buying. Disposition (below) scales it further.

### Disposition: who is not afraid

Whether a colonist can work the frontier, and whether it can earn a living
guarding others, should come from who it is. Three layers, each already a
mechanism in the game, do different jobs:

| Layer | What it is | Lasts | Mechanism |
| --- | --- | --- | --- |
| Wear | habituation to a kind of sight | rolls off with the memories | fresh/worn on the `drives` bump |
| Traits | temperament the colonist arrived with | for life | `driveRate` on safety, and trait rules scaling bumps |
| Skill | nerve learned on duty | for life, grows with practice | `SkillGuarding` rank |

**Traits.** A new mutually exclusive group, `daring`, appended after the
existing groups so earlier rolls keep their order:

| Trait | Safety rate | Bumps (trait rule `impact`) | Reads as |
| --- | --- | --- | --- |
| Bold | −40 | 60% from sight, 50% from hearsay | steady under fire; shrugs off rumors |
| Timid | +40 | 140% | jumps at shadows |

The existing groups take on safety where it is in their character, as data,
not new traits:

- **nerve.** Resilient −20 to the rate; Cowardly +50, and its existing
  `cowardly-visible-alien` and `cowardly-bitten` rules scale the bumps too,
  since a trait rule's impact scale applies to them.
- **outlook.** Optimist discounts what it hears (hearsay bumps 70%);
  Pessimist takes hearsay at 130%. (Optimists recovering faster at home was
  planned and is not built: a per-trait place percent has no mechanism yet.)
  Outlook is about how a colonist reads the world, so it bites on rumor
  rather than on what it sees with its own eyes.

Trait percents multiply, as they do for every drive, so a Bold, Resilient
colonist carrying a rifle on level 1's frontier builds safety at a quarter
of an unarmed ordinary colonist's rate there (60% × 80% × 50%): about 28
hours from calm to uneasy, against seven.

**Skill.** `SkillGuarding` is practised by every tick on station, as a
defense guard inside its area or a bodyguard beside its client. Base 4, the
baseline, rank 1 at a shift (360 ticks): there is plenty of it once a colony
keeps a detail, and a veteran should be plainly better than a recruit. Its
labels run lookout, guard, sentinel, veteran guard. Each rank cuts safety's
growth (whenever it is growing) and the fear in what it sees by
`guarding-nerve-percent` (15), up to four ranks. Nobody arrives with it
(its background weight is 0, so no arrival's rolled skills moved). It is not wear: wear rolls off when the memories do, and a
veteran who has been home a month is still a veteran.

Planned and not built: skill feeding a guard's reservation, so veterans
hold out for better-paying contracts. Guards today take the best-paid
contract open, then defense posts.

### What a colonist sees

A sight bumps the level once, at the moment it is perceived, the way a big
meal's `Instant` sates hunger at once. Rather than a Go table of bumps,
reactions in `cognition.yaml` grow an optional `drives` field with the same
fresh/worn pair affect uses:

```yaml
  - id: witnessed-colonist-killed
    match: { actor_noun: alien, action: kill, object_noun: colonist,
             channel: sight, role: witness, phase: instant }
    affect: { ... }
    drives: { safety: { fresh: 400, worn: 250 } }
```

`applyAffect` already resolves a reaction's wear and trait scales; it hands
the same wear level and the product of the trait rules' `impact` scales to
`applyDriveBumps`, which adds each bump to the drive's true level through
`refreshDrive`'s instant (so the band, phase and next crossing follow). So
`cowardly-visible-alien` makes an alien scarier without a new rule, and the
Bold and Timid rules (`bold-sight`, `timid-sight`, on the sight channel)
scale every sight. A reaction with no `drives` row bumps nothing. In YAML a
bump of 0/0 drops a drive's row, so a file can switch one off.


Starting values (whole points):

| Reaction | fresh | worn |
| --- | --- | --- |
| `saw-alien` | 150 | 80 |
| `saw-gore` | 40 | 20 |
| `witnessed-gunfight` | 100 | 60 |
| `witnessed-colonist-attacked` | 250 | 150 |
| `witnessed-colonist-killed` | 400 | 250 |
| `bitten` | 350 | 300 |
| `witnessed-alien-killed` | −50 | −20 |
| `killed-alien` | −150 | −50 |

The last two are relief: watching the threat die makes the place feel safer.
Worn readings are lower, so a veteran hardens, and since wear counts
occasions in the bounded memory log, a colonist who stops seeing violence
softens again.

### What a colonist hears

Conversations already carried memories: `chooseTopic` draws one of the
speaker's own and both remember "Had a conversation with Bo, who recalled:
…". That was text only. S2 makes it a percept.

- `Memory` has a **gist** (`MemoryGist`): the occurrence's actor and object
  (noun, entity, label) and action, the colonist's own first-hand words for
  it, and a **hop** count (0 for something it saw itself, or heard on the
  news). `rememberPercept` records it with every memory.
- When the topic is a memory with a gist, `finishTalk` calls `tellHearsay`,
  which hands the listener a `Percept{Channel: hearsay, Role: witness,
  Hops: hop + 1}` built from the gist. Reactions match it like any other
  percept; the shipped ones are `heard-colonist-killed`,
  `heard-friend-killed`, `heard-colonist-attacked`, `heard-alien-sighting`,
  `heard-colonist-died` and `heard-friend-died`:

```yaml
# delivered by tellHearsay, like the direct channel: no perception rule
  - id: heard-colonist-killed
    match: { actor_noun: alien, action: kill, object_noun: colonist, channel: hearsay, phase: instant }
    memory: { record: false }
    affect: { impact: 40, fresh: { charge: 10, grip: -20, valence: -30 }, worn: { charge: 10, grip: -20, valence: -30 } }
    wear_policy: none
    drives: { safety: { fresh: 150, worn: 150 } }
```

- **No second memory.** A hearsay reaction records nothing of its own: the
  listener already remembers the conversation and what was recalled in it,
  and a separate "Heard from Ada…" line would say it twice. Instead the
  listener's memory of that conversation takes the gist, one hop on, and
  that is what it can retell: `recountableMemories` offers a conversation
  memory whose gist has `0 < hops < hearsay-max-hops`, with the first-hand
  words as its text, so a retelling never nests quotes. The cost is wear:
  with no memory per hearing, hearsay always lands at its fresh reading
  (`wear_policy: none`).
- **Hops.** News travels to `hearsay-max-hops` (2) retellings, and each one
  past the first lands at `hearsay-hop-percent` (50) of the last, affect
  impact and drive bump alike: third-hand is a rumor.
- **Already knew** (`knows`). A listener whose memories hold the same gist
  (the same who, what and whom), or for a death the killing, is not told
  again, and nothing is passed on that no hearsay reaction cares about.
- **Deaths.** Planned as the dead joining `gossipSubjects`; not built. A
  death travels by word of mouth anyway, as the memory of seeing it, or of
  hearing it on the news, retold.

Hearsay is slow and uneven on purpose: a death is known first by the people
who saw it and spreads as people talk. The nightly news makes sure it
reaches everyone by the end of the day.

### The nightly news

Every evening at `news-hour` (21:00, an hour before the base rhythm's
bedtime) the colony hears the day's major news. It is the colony's one
broadcast: one place where any system can put something everyone should
know, and one reaction channel that says how hearing it feels.

- **Reporting.** A system with news calls `reportNews(NewsItem)` at its
  point of truth: an occurrence in the perception grammar and the line the
  colony hears. Today: a colonist dies (`remove` calls `reportDeath`: object
  the colonist, action `die`, "Cy Ortega is dead: bitten to death by a
  grelk."), a ship lands in play (`reportShipArrival`, actor `ship`, action
  `arrive`; the founders' landing is not news), and recruits are hired
  (`reportRecruits`). An arrival lists its **newcomers**, who do not hear
  news of themselves. Later, anything major: a nest found, a new species
  sighted, a cavern broken into, a defense zone posted.
- **Airing.** On the first tick of `news-hour`, `airNews` marks everything
  reported since the last broadcast as aired and logs it as one line,
  "Evening news, day 4: …". It keeps aired items for `news-days` (30) days.
- **Hearing.** `hearNews` runs after every colonist turn: a colonist awake
  with aired news it has not heard (its cursor, `newsHeard`) gets each item
  as a percept on the `broadcast` channel, role witness, phase instant. A
  colonist asleep or passed out at news time hears it when it next wakes. A
  newcomer starts with the cursor at the end: no old news. No RNG is drawn.
- **Already knew.** As for hearsay: a colonist that saw the killing, or
  heard it over lunch, skips that item.
- **Reactions** are rows like any other: `news-colonist-died`,
  `news-friend-died` and `news-arrival`. A friend's death hits harder, as on
  every other channel. New arrivals ease safety a little: more hands, more
  guns. News is remembered ("Heard on the evening news: …") at hop 0, so it
  can be retold the next day.
- **Frontends.** The log prints each broadcast as one line. The browser's
  **News** tab (`web/src/ui/NewsPanel.svelte`, beside Log) lists every
  broadcast still kept, newest first, from the `news` topic
  (`internal/wire/news.go`; see [wire-format.md](./wire-format.md)); a story
  about a place (a death, a landing) is a button that centers the map
  there. `Snapshot.News` (`publishedNewsBroadcasts`) is rebuilt only when a
  broadcast airs or old news is dropped, and shared by every snapshot in
  between. The terminal has no News tab; its log has the broadcast line.

### At the ceiling

`ConsequenceLoneliness`'s shape, an **experience**, is reused for a new
`ConsequenceUnsafe`: at the ceiling the colonist feels unsafe (`feel
danger`, "Felt unsafe.") at once and every `drives.safety.consequence-every`
ticks. Its reaction takes grip away and its worn reading is worse than its
fresh one, so a colonist kept terrified slides toward anxious and then
despairing, and recovers as the memories roll off. Nothing new in Go
decides what terror feels like.

### Wants: feelings that buy things

Hunger is the one drive that buys anything today, through `mealBidLimit`
and `tryBuyMeal`. S3 names the pattern so safety can use it without a
second copy, and lets one drive want several things in turn:

```go
type wantRung struct {
    name  string                         // "a gun", "a better gun"
    held  func(*World, *Entity) bool     // already has what this rung buys
    value func(*World, *Entity) Money    // what it is worth to a calm colonist
    most  func(*World) int               // willingness percent at the ceiling
    post  func(*World, *Entity, Money, int) *Order // its order, at a limit, with a lifetime
}

type wantSpec struct {
    drive DriveKind
    rungs []wantRung // climbed in order
}
```

`willingness` is `mealBidLimitWith` generalized: it starts at the rung's
value when the drive first presses (`SeekAt`) and rises linearly to `most`%
of it at the ceiling. It spends only what the colonist has beyond
`want-reserve-meals` (4) meals' worth, and half of that until the drive is
critical, then all of it. The bodyguard rung (S4) will add its own spending
rule, a share of the wallet per shift.

`postWants` runs after every colonist turn; with an order open, or the
drive calm, it costs a comparison. It first notes what became of the open
order: gone with `Filled` set is a purchase, gone without is a lapse. Then,
if the drive presses and the colonist is not hungry (food comes first), it
wants the **first rung not yet held** and posts that one, at its
willingness; one want is open per drive per colonist. A colonist that
cannot post at all (no market depot, no money beyond its food) tries again
90 ticks later.
A colonist climbs the next rung only after `want-settle-ticks` (one day)
with the last one held and its safety still pressing: a new gun is given a
chance to calm it first.

**Posting is how demand is shown.** A colonist does not check whether
anything could fill its order. In a colony with no forge yet, a bid for a
rifle goes on the book anyway. That bid is the
signal. The producer planner already reads open bids and reaches down the
recipe chain for them ([valuation.md](./valuation.md)); a smith looking at a
book full of rifle bids is who builds a forge of its own
([skills.md](./skills.md), S5); a pistol owner who sees a shotgun bid above
what its pistol is worth to it sells. "No foundry yet" is a state the
colony passes through, and the orders are what end it.

**Orders expire, and the colonist decides again.** Every want posts with a
lifetime of `want-ttl` (one day, 1080 ticks), whether it is a bid for a gun
or an unstarted escort contract (its escrow comes back when it lapses). On
expiry, `postWants` runs and the colonist chooses afresh from where it
stands:

- **Calm now?** It lets the want lapse.
- **Still pressing?** It re-posts the same rung at today's willingness,
  which is higher if it is more afraid than it was, so an unfilled order
  bids itself up the way a hungry colonist's resting meal bid does
  ([pricing.md](./pricing.md)).
- **Waited long enough?** After `want-patience` (3) lapsed orders on one
  rung, it may look past it: a colonist that can afford the next rung up
  posts that instead. A rich colonist who has waited three days for a rifle
  that nobody makes hires a guard. A colonist that cannot afford anything
  higher keeps re-posting; its order stays on the book as standing demand.

The book therefore always holds the colony's unmet fear, priced, for a day
at a time, and a lapsed order never lingers after the fear behind it has
gone.

The inspector shows what the colonist wants and why, under its drives:
"Wants a better gun: will pay up to $12; has $61.", or "can't afford it".
That is rule 6 (every choice explainable), and it is most of the tuning
interface. (The planned sentence named the gun and the trade-in; the built
one is shorter.)

Hunger moving onto `wantSpec` is a refactor, not part of this plan; the
meal rule is the reference the general one is checked against.

### Safety's ladder

| Rung | Held when | Posts | Worth | Spends |
| --- | --- | --- | --- | --- |
| a gun | it carries any weapon | a bid for the best gun it can pay for | that gun's price | the meal rule |
| a better gun | it carries a gun above its own, up to a rifle | a bid for the next gun up or better; its old gun goes on sale when the new one arrives | the new gun's price, less what the old one fetches | the meal rule |
| a bodyguard | it has an escort contract open | an escort contract | `escort-wage` an hour | `escort-budget-percent` of the wallet a shift |

**Guns.** A gun is a good like a meal: a bid at the market depot (the
silo), filled from an ask, left there in the buyer's name, and fetched.
`bidForGun` bids for the best gun above the colonist's own it can pay for at
its price, or, if it can pay for none, for the cheapest one above at what it
can pay: a bid below the price is still demand. Prices: `price-pistol` (20),
`price-shotgun` (35) and the existing `price-assault-rifle` (60), each the
gun's value until guns of its kind trade (`gunValue`; kept apart from
`refPrice`, the colony's silo price, so the foundry's reckoning did not
move), and each up to `gun-willingness` (150%) at the ceiling.

Every gun is machined at the gun bench, for a bid, through the producer
planner, the way a hungry colonist's bid reaches the cave wall
([valuation.md](./valuation.md), [foundry.md](./foundry.md)): a pistol from
one steel ingot, a shotgun from two, a rifle from three. Guns also come
secondhand: a colonist trading up fetches its new gun (`JobFetch`, first in
`assignWorkJob`, so it does not go out unarmed) and leaves the old one in
the depot on sale at its value. A trade-in is why the second rung is cheaper
than it looks: a $35 shotgun costs a pistol owner $15. A colonist never
sells the gun in its hand to someone else's bid (`ownStockFor`).

A bought gun is remembered (`gunWaiting`) until it is fetched, rather than
found by searching every depot: the rung's `held` runs every turn for an
uneasy colonist and for the inspector every frame.

A colonist that buys a gun is armed, so it also becomes someone who can
guard: fear builds the colony's pool of guards from the bottom.

**A bodyguard is for the rich.** Two rules together make it so:

- **It costs what a guard could earn elsewhere, and more.** A bodyguard is
  calm, armed, and walking into danger on purpose; it is the scarcest labor
  in the colony. A contract may not be posted below `escort-wage` (6) an
  hour, three times `wage-guard`, so a shift (`escort-hours`, 8) costs at
  least $48. The armed colonists it competes for rank it against defense
  posts and other paid work by rate, so a cheaper contract would sit open.
- **It is paid from savings, not from need.** A gun is bought once and kept,
  so a frightened colonist will spend most of what it has on one, as it
  would on a meal when starving. A bodyguard is paid every day the fear
  lasts, so the rung's `Budget` lets a shift cost at most
  `escort-budget-percent` (10) of the wallet. At the minimum wage that is a
  wallet of $480 or more.

A colonist lands with `crash-pod-purse` ($100), commissions a house at
`house-savings` ($300), and recruits bring a log-normal purse averaging
$100. So the colonists who hire guards are the colony's richest: a lucky
recruit, a master smith, a landlord with paid toilets. Everyone else stops
at the second rung. Who is rich enough is a fact the economy produces, not
a flag; the inspector says "Can't afford a bodyguard" for the rest.

### Bodyguards

The ladder's top rung posts an **escort contract**: a `WorkEscort` work
order, issued by the client, for `escort-hours` (8) hours at the hourly
price `willingness` gives (never below `escort-wage`), escrowed up front
through `postWork`. Defense zones
pay from the treasury as they go because a detail has no end
([defense.md](./defense.md)); a contract has one, so it escrows like every
other work order, and the money audit already counts it.

A contract is **guard duty that follows a colonist**. As built it did not
generalize `defensePost` (a post is a rectangle the colony owns; a contract
is a client's), but `JobEscort` keeps every rule of `JobGuard`:

- **Who takes it.** An armed colonist looking for work whose own safety is
  calm (below `SeekAt`), in `assignWorkJob`, before defense posts: a
  contract never pays less than `escort-wage`, above `wage-guard`. It takes
  the best-paid open contract, then the oldest, whose client is alive, out
  of its home, and in its room. Never its own.
- **Only the market.** What an escort weighs is what the book shows: the
  contract's rate, hours left, escrow, and where its client stands. Where
  the client stands is public, the way any colonist is visible on the map.
  How the client feels is not. The escort never reads the client's drives,
  affect or memories, and the client's fear reaches the market only as the
  price it chose to post. That rules out the shortcut `companyInReach`
  takes for conversation ([drives.md](./drives.md), *Known problem*).
- **Calm to serve.** The same rule holds on station, for defense posts as
  well as contracts (`calmEnoughToGuard`): a guard whose own safety reaches
  pressing gives the post up, and it is released for the next calm, armed
  colonist. Guards out on the frontier still feel it, more slowly for being
  armed, Bold or skilled, so a detail turns over as its guards get uneasy,
  the way it already turns over for food and sleep. That is why guards
  never need guards: whoever is on station is, by construction, not afraid.
- **On station.** The escort walks to within `escort-radius` (3) of the
  client, on its level, re-pathing when the client moves, and stands by
  there. Every tick it looks for an alien within `guard-sight` of itself
  and of the client and fights it with `fightAlien`. The client is the area
  bound, so an escort does not chase.
- **Pay.** `serveEscort` counts ticks within the radius and calls `payWork`
  for each full hour, from the contract's escrow. Time walking to the
  client is not billed, nor time the client spends at home: the escort
  gives the contract up when the client goes home, and the next one takes
  it when the client is out again.
- **Released on leaving, like a guard.** An escort that leaves (a drive, its
  own fight, unease) releases the contract for the next armed colonist. So
  a contract is "this client guarded for eight hours", not "these people".
- **Ending.** The contract closes when its hours are served, or its client
  dies (`cancelWorkOf` closes a dead issuer's orders). One nobody has taken
  by its lapse tick (`want-ttl` after posting) closes too, its escrow comes
  back, and the client's want decides again. Any remainder goes back to the
  client.

Protection is felt, not read. While its escort is beside it on station
(`escortedNow`), the client's place class is guarded, so its safety falls.
When a bodyguard takes a client, one occurrence, `colonist / guard /
colonist`, reaches both: the guard remembers taking the client on
(`guarded-client`, a little pride) and the client hiring the guard
(`hired-bodyguard`, grip up, and safety eased by 60). After that the
client feels its guard come and go: `noteGuarded`, called with the place
class after every colonist turn, delivers a `colonist / guard / colonist`
percept on the proximity channel when the guard comes alongside (enter,
"Walked with Bo as a bodyguard.") and when it leaves (exit). The `guarded`
reaction records the enter: grip up, safety eased by 20, wearing in to 5 as
the occasions pile up. The affinity from time together is not built; it is
tracked in
[#216](https://github.com/KevinMcHugh/mars-sim/issues/216).

Escort contracts show on the Market tab's **Bodyguards** section (client,
who is on station, rate, hours served and left, escrow), from the market
topic's `escorts`, and in its work orders as kind `escort`. On the map a
dashed red line ties each bodyguard on contract to its client, from the
zones topic's `escorts` pairs (`[guard, client]`) and each frame's
positions.

### Critical safety (S5)

S1 to S4 change nothing a colonist *does* except buy. S5 gives the
critical band teeth, and is the phase most likely to need tuning:

- **Retreat.** Critical safety emits pressure like any drive, into a new
  `retreat` focus: walk home and stay there until the level is back below
  `SeekAt`. With a bodyguard on station the colonist is guarded, the drive
  is falling, and retreat is not eligible (`currentFocusEligible`), so a
  guard is how an afraid colonist keeps working.
- **Turning down frontier work.** While afraid, `assignWorkJob` skips jobs
  whose tile is on the frontier, unless the colonist is guarded or the job
  is inside a staffed defense zone.
- **Restless nights.** A `ConsequenceRate` from safety's critical band on
  the sleep drive: −30, so sleep falls slower in bed and an afraid colonist's
  night runs long.

The economics fall out: a colony that lets its miners get afraid loses
shifts at the rock face, and a defense zone over the mine or a bodyguard
for each miner buys them back.

## Built so far

### S4: bodyguards

Measured on three seeds of the 20-colonist game (zoning-auto, food safety
net on), two colonists given $5,000:

- **Nobody hired a guard** as the game played: the rich spent down to about
  $840 within the run, and spent their time at home and in the settled
  colony, where fear falls. Given rifles too, the richest was never even
  uneasy. As tuned, a bodyguard is rare: the frontier is where fear builds,
  and the rich are seldom there.
- **With one rich colonist held afraid** (the drive set to 900 each tick, a
  smoke test of the whole path), each game posted 8 to 10 contracts, served
  50 to 63 paid hours, and 4 to 7 colonists earned a guarding rank.
- **A bug the bodyguard probe found in S3**: a trade-in credited the old gun
  to the depot's ledger without putting it in the container, so a colonist
  who bought it paid and could never collect it. Fixed, and the trade-in
  test checks the depot's ledger against what it holds.

### S3: wants and guns

Measured on three seeds of a 20-colonist game with zoning-auto and the food
safety net on, for 10,800 ticks each:

| | seed 1 | seed 2 | seed 3 |
| --- | --- | --- | --- |
| gun bids posted | 11 | 24 | 25 |
| guns traded (new and secondhand) | 8 | 9 | 10 |
| armed colonists, start → end | 12 → 11 | 10 → 10 | 10 → 14 |
| carrying a shotgun or rifle at the end | 2 | 5 | 2 |
| starved | 0 | 0 | 0 |

- **Without a forge's sidearms, nothing filled.** Built as first planned
  (pistols and shotguns secondhand only), the same games posted 14 to 34
  gun bids and traded none: nobody trades up when nothing better is ever
  sold, and the colonists' rifle bids sat below what a rifle costs to make.
  Pulling S6's recipes forward is what made the market answer.
- **Food comes first.** Without a reserve, colonists spent the money they
  needed for meals, and the long-run treasury test (20 colonists, 30,000
  ticks) starved 3 against its limit of 2. Wants now spend only money beyond
  `want-reserve-meals` meals' worth, and post nothing while the colonist is
  hungry.
- **The default 6-colonist game has no market.** It never builds a silo
  before it starves (no communal container in six seeds), so its colonists
  want guns and have nowhere to bid. That is the game's scarcity, not the
  ladder's.

### S1 and S2

S1 and S2 as built, measured on the drive tuning harness's six seeds of the
default game for 10,800 ticks each (about 140 colonist-days):

| | |
| --- | --- |
| mean safety level | 445 of 1000 |
| time uneasy / afraid | 17.7% / 16.1% |
| time at home / settled / frontier / guarded | 39.5% / 2.6% / 57.9% / 0% |
| felt unsafe (the ceiling) | 165 times |
| deaths heard on the news | 41 |
| arrivals heard on the news | 0 (no arrivals in those games) |
| hearsay passed on | 0 |

- **Colonists spend most of the day on the frontier**, so a working day
  reliably takes a miner from calm to terrified, and a night aboard ship
  brings it back. That is the pressure S3 to S5 are meant to answer; on its
  own it only costs mood.
- **Hearsay is rare in practice.** The tests show it working, but those
  games had only 124 conversations, 44 of them about a memory, and every one
  of those memories was routine (a meal, a dig). Topics draw a memory
  uniformly from the speaker's log, and the log is mostly work. Weighting the
  draw toward what a colonist would bring up (a high-impact memory) would
  make hearsay matter; it is left as a decision, since it changes what every
  conversation is about.
- **Not built from the plan:** the dead as gossip subjects, and Optimists
  recovering faster at home.
- **Scum Lab** edits the `drives` rows: the Grammar Builder's reaction form
  has a drive and its fresh and worn bump, and its import and export carry
  them. Its baked-in defaults are now generated from the Go defaults (see
  [scum-lab.md](./scum-lab.md)).
- **Every seed's game changes.** The tick-0 world is untouched (an unsought
  drive starts at 0 without a draw, and the daring traits have their own
  stream), but fear moves affect and so focus, and topics now retell heard
  news. The golden hashes were re-pinned.

## Why it is this way

- **A drive, not a mood.** Charge and grip already go down when something
  frightening happens, but affect decays toward each colonist's home within
  hours and is a point on a plane, not a quantity with a threshold. Fear
  that should outlast a shift, cross thresholds, and be priced is drive-
  shaped. The two cooperate: the drive's ceiling is felt through a
  reaction, and the bumps come from the same percepts the reactions read.
- **Bumps on reactions, not a table in Go.** Every sight that should scare
  someone is already a reaction row with wear and trait rules. A Go table
  keyed by reaction id would duplicate the grammar and drift from it; a
  `drives` field reuses it, and Scum Lab can tune it.
- **Place classes, not distance from a bed.** "Hours since home" would need
  a timer per colonist and would not fall while the colonist sits in the
  meeting hall. A class with a negative rate for safe places gives the same
  curve, recovers naturally, and is the shape the beauty/comfort roadmap
  already asked for.
- **A broadcast, not instant knowledge.** Every colonist learning of a death
  the tick it happens would be the "universal fact stream" the perception
  design rejected, and it would flatten the story: nobody would ever be the
  one to break the news. Once a day, at a time the colony keeps, gives a
  death a few hours to travel by word of mouth first and still guarantees
  nobody is killed alone and forgotten. One channel for all major news means
  arrivals and the rest cost a `reportNews` call and a reaction row each.
- **Hearsay as a channel, not a conversation effect.** The obvious build
  was a function next to `applyGossip` that nudges the listener when the
  topic is scary. That is a second, hand-written list of what is scary.
  A `hearsay` channel means `cognition.yaml` decides what hearing about
  something does, the same way it decides what seeing it does, and
  traits like Cowardly apply without new code. It needs the gist on
  `Memory`, because `Memory.Rule` alone names a reaction, not the
  occurrence, and two reactions can match one occurrence for different
  observers.
- **Hops.** Without a bound, a retold memory becomes a new memory, which is
  retold, and one killing scares the colony forever. Two hops, halved each
  time, is enough to reach the people who were not there.
- **Escrowed, not pay-as-you-go.** Defense zones pay from the treasury as
  they go because a detail never ends. A contract ends, and pay-as-you-go
  from a colonist's wallet would let a guard work hours its client could
  no longer afford. Escrow also makes the price on the book honest.
- **A post that follows, not a new job.** Everything a guard does on
  station (fight within sight, leave for drives, release on leaving, pay by
  time inside the area) is what an escort does. Only the area moves. One
  post type keeps the invariants in one place.
- **Guards self-select by calm, not by a guard flag.** A bodyguard who is
  itself afraid is no protection, and hiring guards for guards has no
  bottom. Making calm the condition for taking and keeping a post means the
  colonists who end up guarding are the armed, Bold, skilled ones, without
  a role anyone assigns. It also changes defense zones: a detail on a
  dangerous mine now staffs itself from the brave, and runs short when the
  colony has none, which is a fact the Zones tab should show.
- **Disposition as traits and a skill, not one stat.** A courage number
  would be the integer-as-interface mistake (rule 1). Traits say who a
  colonist was when it landed; the skill says what duty has made of it;
  wear says what it has seen lately. Each has a different lifetime, so
  each is its own mechanism.
- **A ladder, not the most relief per dollar.** The obvious want would cost
  every option and buy the best value. That makes the poor and the rich
  want the same thing and differ only in whether they can pay, which hides
  the story. A ladder says what people actually do: get a gun, get a better
  one, and only then, if they can, pay someone. It also keeps the bodyguard
  rare, which is what makes one mean something.
- **Demand is posted, not predicted.** An earlier draft skipped a rung
  nothing could fill, so in a colony without a forge pistol owners would
  never ask for rifles. That hides exactly the demand a producer needs to see, and treats
  a short-lived state (no forge yet) as a permanent one. Market actors are
  meant to find needs by reading the book, so every want goes on it.
- **Expiring orders, not standing ones.** A bid that rests forever outlives
  the fear behind it and pins money in escrow. A day's lifetime lets the
  colonist re-decide on what it knows now: let it go, bid higher, or move
  up the ladder. It is the same reason a hungry colonist's bid rests for
  `demand-ttl` rather than for good.
- **Guns spend like meals, guards spend from savings.** A durable good is
  worth stretching for once; a service paid every day is not. Two spending
  rules, one per kind of rung, are what put guns in everyone's reach and
  guards in only the rich's, without a wealth threshold anyone tunes by
  hand.
- **Escorts read the book.** Colonists already read each other's minds to
  decide who will talk, and drives.md calls that a known problem. Hiring is
  new code, so it starts from the end state: the client's fear is private,
  and the price is how it speaks.
- **Not billed at home.** The client is safe there anyway, and keeping a
  guard outside a bedroom for eight hours is the empty-post problem defense
  zones solved by releasing on leaving.
- **Golden runs move.** Unlike defense zones, a new drive changes every
  seed: affect moves, and from S3 money does. S1 ships with
  `drives.safety.rate` set and the bumps in place but no want, so the first
  diff is small and measurable; `-drive-safety-rate 0` turns the drive off
  for comparisons.

## Settings

Every setting below is built.

| Setting | Default | Meaning |
| --- | --- | --- |
| `drives.safety.rate` | 1000 | base growth, before the place percent |
| `drives.safety.place.*` | −300 / −50 / 200 / −200 | home / settled / frontier / guarded |
| `drives.safety.place-depth` | 50 | extra frontier growth per level below the landing level |
| `drives.safety.armed.*` | −25 / −35 / −50 | safety rate change for the best gun carried: pistol / shotgun / rifle |
| `drives.safety.seek-at` / `critical-at` / `max` | 600 / 850 / 1000 | uneasy / afraid / terrified |
| `drives.safety.consequence-every` | 200 | ticks between feeling unsafe while terrified |
| `hearsay-max-hops` | 2 | how many retellings news survives |
| `hearsay-hop-percent` | 50 | how much of a bump survives each retelling |
| `news-hour` | 21 | the clock hour of the evening broadcast |
| `news-days` | 30 | days of broadcasts the colony keeps |
| `price-pistol` / `price-shotgun` | 20 / 35 | what those guns are worth before any has traded; the rifle keeps `price-assault-rifle` (60) |
| `gun-willingness` | 150 | percent of a gun's price a colonist will pay at the ceiling |
| `want-ttl` | 1080 | how long a want's order rests on the book before the colonist decides again |
| `want-patience` | 3 | lapsed orders on one rung before a colonist that can afford the next rung looks past it |
| `want-settle-ticks` | 1080 | how long a rung just taken is given to work before the next is wanted |
| `want-reserve-meals` | 4 | meals' worth of money a colonist keeps back from any want |
| `escort-hours` | 8 | length of a contract |
| `escort-radius` | 3 | how close an escort keeps, and counts as guarding |
| `escort-wage` | 6 | the least an escort contract may pay an hour |
| `escort-willingness` | 150 | percent of `escort-wage` a client will pay at the ceiling |
| `escort-budget-percent` | 10 | the most of its spare money (beyond its food money) a client spends on one shift |
| `guarding-nerve-percent` | 15 | how much each `SkillGuarding` rank cuts safety's frontier rate and sight bumps |

## Extending it

- **A new thing to be afraid of** (a sinkhole, a cave-in, an uncatalogued
  species) is an occurrence and a reaction with a `drives` field. No Go.
- **A new kind of news** is a `reportNews` call where the thing happens and
  a reaction on the `broadcast` channel. Nothing in the broadcast changes.
- **Another drive that buys** (comfort buying furniture, hygiene buying a
  wash) is a `wantSpec` row and its `Post`.
- **Another rung** (body armour between the guns and the guard, say) is a
  `wantRung` in safety's ladder and, if it should calm, a modifier on the
  rate. Order is the ladder's order.
- **Chipping in to a defense zone (S6).** A colonist whose frontier is a
  zone's area could add to the zone's wage pool instead of hiring alone,
  which is the first public good paid for by the people who use it
  (see [work-market.md](./work-market.md)).
- **Invariants:** every safety rate change goes through `refreshDrive`; the
  guarded flag changes only on the escort's enter/leave transition; a
  contract has at most one escort on station; escorts and posts are ranked
  by pay then id, never by map order; nobody holds a guard post or contract
  while its own safety is pressing; nothing in hiring reads another
  colonist's drives, affect or memories; hearsay never exceeds
  `hearsay-max-hops`.
- **A new disposition trait** (a veteran's Shell-Shocked, acquired by too
  many killings witnessed) is a `traitSpec` with a safety `driveRate` and,
  if it should change what sights do, trait rules in `cognition.yaml`.

## Open questions

- **What counts as major news.** Deaths and arrivals are clear. Should
  smaller things make it on a slow day (a rank-up, a new house), or should
  the broadcast stay rare enough that hearing it means something?
- **Hiring by reputation.** A client sees nothing about who takes its
  contract. Should a client be able to post "rank 2 guarding or better", at
  a higher price, once skill ranks are public on the roster?

## Related

- [drives.md](./drives.md): the drive model, bands, consequences, and the
  location-dependent rate roadmap this builds on.
- [defense.md](./defense.md): guard posts, which escorts generalize, and
  which take on the calm-to-serve rule.
- [personality.md](./personality.md): trait groups, where `daring` joins.
- [skills.md](./skills.md): ranks and reservations, where `SkillGuarding`
  joins.
- [compositional-perception-and-events.md](./compositional-perception-and-events.md):
  the grammar the bumps and the hearsay channel extend.
- [affect.md](./affect.md): wear, fresh/worn readings, trait scales.
- [conversation-topics.md](./conversation-topics.md): memory and colonist
  topics, which carry hearsay.
- [valuation.md](./valuation.md): `mealBidLimit`, the first want.
- [labor.md](./labor.md): work orders and escrow.
- [work-market.md](./work-market.md): ranking paid work by rate.

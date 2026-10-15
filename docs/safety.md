# Safety and bodyguards (proposal)

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

Nothing here is built.

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
| S1 | The safety drive: a place-dependent rate (home, settled, frontier, guarded), bumps from percepts through a `drives` field on reactions in `cognition.yaml`, the felt-unsafe experience at the ceiling, and the disposition traits (Bold/Timid, and safety rules on nerve and outlook). No behavior beyond affect. | Proposed |
| S2 | Hearsay and the nightly news: a recounted memory reaches the listener as a percept on a new `hearsay` channel, so hearing about a killing or a sighting is a reaction row; the dead become conversation topics; an evening broadcast on a `broadcast` channel reports the day's deaths, then arrivals. | Proposed |
| S3 | Wants: one willingness-to-pay rule for any drive, ladders of rungs, the hook that posts an order when a drive presses, and the inspector line that explains it. Safety's first two rungs: a gun, then a better gun, with the old one sold on. | Proposed |
| S4 | Bodyguards: `WorkEscort` contracts escrowed from the client, `JobEscort` as a guard post that follows a colonist, protection as a place class, the calm-to-serve rule for every guard post, the guarding skill, and escorts on the Market tab. | Proposed |
| S5 | Critical safety: retreating home, turning down frontier work, restless nights. | Proposed |
| S6 | Chipping in to a defense zone's wages; pistols and shotguns made at the gun bench. | Proposed |

## Source

What changes, when built:

- [`internal/sim/drives.go`](../internal/sim/drives.go): `DriveSafety` and
  its `DriveSpec` in `defaultDrives`; a place modifier in `driveRate`.
- `internal/sim/place.go` (new): `PlaceClass` (home, settled, frontier,
  guarded), `placeOf`, and `syncPlace`, the place twin of
  `syncDriveActivity`.
- [`internal/sim/personality.go`](../internal/sim/personality.go): the
  `daring` trait group (Bold, Timid); safety rates on Resilient, Cowardly,
  Optimist and Pessimist.
- [`internal/sim/skills.go`](../internal/sim/skills.go): `SkillGuarding`.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go):
  a `drives` field on reactions, the `hearsay` channel, and the `die`,
  `guard` and `hear` vocabulary.
- [`internal/sim/world.go`](../internal/sim/world.go): `rememberPercept`
  applies a reaction's drive bumps.
- [`internal/sim/topics.go`](../internal/sim/topics.go),
  [`systems.go`](../internal/sim/systems.go) (`finishTalk`): hearsay
  percepts for the listener; the dead as topics.
- [`internal/sim/entity.go`](../internal/sim/entity.go): `Memory` gains the
  gist of the occurrence it holds and a hop count.
- `internal/sim/news.go` (new): `NewsItem`, `reportNews`, `broadcastNews`,
  and each colonist's cursor into the news; deaths report from where a
  colonist is removed, arrivals from ship landings and recruiting.
- `internal/sim/wants.go` (new): `wantSpec`, `wantRung`, `willingness`,
  `postWants`; safety's ladder.
- [`internal/sim/combat.go`](../internal/sim/combat.go),
  [`inventory.go`](../internal/sim/inventory.go): reference prices for the
  pistol and shotgun; a bought gun fetched from its depot.
- [`internal/sim/workorder.go`](../internal/sim/workorder.go): `WorkEscort`.
- [`internal/sim/defense.go`](../internal/sim/defense.go): the post grows a
  second shape, an area that follows a client; `tryAssignEscort`,
  `serveEscort`.
- [`cognition.yaml`](../cognition.yaml): the safety bumps, the hearsay
  reactions, `felt-unsafe`, `guarded`, `hired-bodyguard`.
- [`internal/sim/config.go`](../internal/sim/config.go): the settings below,
  and `mars-sim.yaml` regenerated.

## How it works

### The safety drive

Safety is a drive like the other four (see [drives.md](./drives.md)): a
level from 0 (feels safe) to `Max` (terrified), lazily evaluated, cut into
bands. It is a drive and not a fourth affect axis because it has to do the
things drives do and affect does not: accumulate rather than decay toward a
home, cross thresholds on a schedule, emit pressure into focus, and say how
much a colonist would pay. Affect still carries how it *feels*, through the
reactions below.

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
| settled | any other built room, or a residence zone | −50 |
| frontier | anywhere else: dug passages, caverns, open rock faces | +200, plus `safety-depth-percent` (50) per level below the landing level |
| guarded | inside a staffed defense zone, or within `escort-radius` of its own bodyguard | −200 |

Guarded outranks the rest: a colonist with a bodyguard in a cavern is
guarded, not on the frontier. At the shipped base rate a colonist out on
level 1's frontier goes from calm to uneasy in about 300 ticks, a little
under seven hours, and a night at home takes it from the ceiling to zero.
Two levels down it is about half that.

`placeOf` is computed only when the colonist changes region or a room or
zone under it changes, the same moments that already invalidate its room;
`syncPlace` compares it with the class the rates were composed for and
refreshes only on a change, so it costs one comparison on a tick when
nothing moves. The guarded class is set from the guard's side (below): an
escort entering or leaving `escort-radius` flips the client's flag and
calls `refreshDrive`, so no client scans for its guard every tick.

Carrying a weapon is a trait-like modifier, not a place, and a better gun
calms more: an armed colonist's safety rate changes by its best weapon's
`safety-armed-*` percent, −25 for a pistol, −35 for a shotgun and −50 for
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
- **outlook.** Optimist discounts what it hears (hearsay bumps 70%) and
  recovers faster at home (place% −400); Pessimist takes hearsay at 130%.
  Outlook is about how a colonist reads the world, so it bites on rumor
  rather than on what it sees with its own eyes.

Trait percents multiply, as they do for every drive, so a Bold, Resilient
colonist carrying a rifle on level 1's frontier builds safety at a quarter
of an unarmed ordinary colonist's rate there (60% × 80% × 50%): about 28
hours from calm to uneasy, against seven.

**Skill.** `SkillGuarding` is practised by every tick on station, as a guard
or an escort, and by every alien a guard fights. Base 4, the baseline: there
is plenty of it once a colony keeps a detail, and a veteran should be
plainly better than a recruit. Each rank cuts the frontier and guarded
rates and the sight bumps by `guarding-nerve-percent` (15) per rank, up to
four ranks. It is not wear: wear rolls off when the memories do, and a
veteran who has been home a month is still a veteran.

Skill feeds the market the usual way (see [skills.md](./skills.md)): a
ranked guard's reservation is what it has been earning, so veterans hold
out for better-paying contracts and recruits take the cheap ones.

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

`rememberPercept` already resolves a reaction's wear and trait scales
before applying it; it applies the drive bump with the same resolved wear,
through a new `bumpDrive(e, d, points)` that folds the growth so far and
re-places the level in its band, like `setDrive`. Trait rules' `impact`
scale applies to it too, so `cowardly-visible-alien` makes an alien
scarier without a new rule.

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

Conversations already carry memories: `chooseTopic` draws one of the
speaker's own and both remember "Had a conversation with Bo, who recalled:
…". Today that is text only. S2 makes it a percept.

- `Memory` gains a **gist**: the occurrence's actor noun, action, object
  noun and object relation, copied when `rememberPercept` records it, and a
  **hop** count (0 for something the colonist saw itself).
- When the topic is a memory with a gist, `finishTalk` hands the listener a
  `Percept{Channel: hearsay, Role: witness}` built from the gist. Reactions
  match it like any other percept:

```yaml
# delivered by finishTalk, like the direct channel: no perception rule
reactions:
  - id: heard-colonist-killed
    match: { actor_noun: alien, action: kill, object_noun: colonist,
             channel: hearsay, phase: instant }
    memory: { record: true }
    affect: { impact: 40, fresh: { charge: 10, grip: -20, valence: -30 },
              worn: { charge: 5, grip: -25, valence: -30 } }
    drives: { safety: { fresh: 150, worn: 100 } }
    wear_policy: memory-occasions
  - id: heard-alien-sighting
    match: { actor_noun: alien, action: present, channel: hearsay }
    drives: { safety: { fresh: 40, worn: 20 } }
```

- The listener's memory reads "Heard from Ada that a grelk killed Cy.",
  keeps the gist, and has `hop + 1`. A hearsay memory is recountable, so
  news travels, but only to `hearsay-max-hops` (2), and each hop scales the
  bump by `hearsay-hop-percent` (50): third-hand is a rumor.
- **Deaths.** `gossipSubjects` lists living colonists only. The dead join
  it while anyone remembers them, and a colonist topic about a dead colonist
  carries the gist `colonist / die`, so "Talked with Bo about Cy, who was
  killed." is a hearsay percept too. Friends (`object_relation: friend`)
  take it hardest, as a trait-free rule.

Hearsay is slow and uneven on purpose: a death is known first by the people
who saw it and spreads as people talk. The nightly news makes sure it
reaches everyone by the end of the day.

### The nightly news

Every evening at `news-hour` (21:00, an hour before the base rhythm's
bedtime) the colony hears the day's major news. It is the colony's one
broadcast: one place where any system can put something everyone should
know, and one reaction channel that says how hearing it feels.

- **Reporting.** A system with news calls `reportNews(gist, subject,
  text)` at its point of truth. The item joins `World.news`, in tick order,
  with the same gist a memory carries (actor noun, action, object noun) and
  the entity or ship it is about. Today: a colonist dies (`colonist / die`,
  "Cy Ortega was killed by a grelk on level 2.", or starved, or fell). Next:
  arrivals (`ship / land`, "A ship landed with 12 aboard."; a recruit
  joining). Later, anything major: a nest found, a new species sighted, a
  cavern broken into, a defense zone posted.
- **Broadcasting.** At `news-hour`, `broadcastNews` walks living colonists
  in ID order and hands each the items since its cursor as percepts on a new
  `broadcast` channel, role witness, phase instant. A colonist asleep or
  passed out at news time hears it when it next wakes; the cursor is what
  makes that free. No RNG is drawn, so it is deterministic and costs one
  pass a day.
- **Already knew.** A colonist whose memories already hold the same gist
  about the same subject (it saw the killing, or heard it over lunch) skips
  that item, so a witness is not hit twice by one death.
- **Reactions** are rows like any other:

```yaml
  - id: news-colonist-died
    match: { actor_noun: colonist, action: die, channel: broadcast }
    memory: { record: true }
    affect: { impact: 35, fresh: { charge: 5, grip: -15, valence: -25 },
              worn: { charge: 0, grip: -20, valence: -25 } }
    drives: { safety: { fresh: 120, worn: 80 } }
    wear_policy: memory-occasions
  - id: news-arrival
    match: { actor_noun: ship, action: land, channel: broadcast }
    memory: { record: true }
    affect: { impact: 15, fresh: { charge: 5, grip: 5, valence: 10 },
              worn: { charge: 2, grip: 2, valence: 5 } }
    drives: { safety: { fresh: -30, worn: -10 } }
```

  A friend's death (`object_relation: friend`) hits harder, as on every
  other channel. New arrivals ease safety a little: more hands, more guns.
- **Retelling.** A news memory is hop 0, like something seen, so it can be
  recounted as hearsay the next day. The memory reads "Heard on the evening
  news: Cy Ortega was killed by a grelk."
- **Frontends.** The log prints the broadcast as one entry headed "Evening
  news, day 4", and a `news` topic lists past broadcasts for a News panel.
  `World.news` keeps the last `news-days` (30) days, so it stays bounded.

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
    Name   string                       // "a gun", "a better gun", "a bodyguard"
    Held   func(*World, *Entity) bool   // already has what this rung buys
    Open   func(*World, *Entity) bool   // something could fill it: an ask, a recipe, a calm guard
    Value  func(*World, *Entity) Money  // what it is worth to a calm colonist
    Most   int                          // willingness percent at the ceiling
    Budget int                          // percent of the wallet it may spend (0: the meal rule)
    Post   func(*World, *Entity, Money) bool
}

type wantSpec struct {
    Drive DriveKind
    From  DrivePhase // DrivePressing: when the want switches on
    Rungs []wantRung // climbed in order
}
```

`willingness` is `mealBidLimitWith` generalized: it starts at `Value` when
the drive first presses, rises linearly to `Most`% of it at the ceiling, and
is capped by what the colonist will spend. With `Budget` 0 that cap is the
meal rule: half the wallet until critical, then all of it. A rung with a
`Budget` may spend only that percent of the wallet, however afraid the
colonist is.

`postWants` runs when a drive's phase changes (the crossing `syncDriveBand`
already schedules) and every `want-recheck-ticks` while it stays pressing,
so it costs nothing for a calm colonist. It wants the **first rung not yet
held** that is open and affordable, and posts that one; one want is open
per drive per colonist. A rung nothing could fill (no ask on the book, no
recipe that makes it, no calm armed colonist) or the colonist could never
afford is passed over, so a colony without a foundry does not leave its
pistol owners stuck wanting rifles. A colonist climbs the next rung only
after `want-settle-ticks` (one day) with the last one held and its safety
still pressing: a new gun is given a chance to calm it first.

The inspector shows the ladder and where the colonist stands on it:
"Uneasy (720). Has a pistol. Wants a better gun: will pay up to $30 for a
shotgun, less $20 for its pistol; has $61. Can't afford a bodyguard." That is rule 6
(every choice explainable), and it is most of the tuning interface.

Hunger moving onto `wantSpec` is a refactor, not part of this plan; the
meal rule is the reference the general one is checked against.

### Safety's ladder

| Rung | Held when | Posts | Worth | Spends |
| --- | --- | --- | --- | --- |
| a gun | it carries any weapon | a bid for the best gun it can pay for | that gun's price | the meal rule |
| a better gun | it carries a gun above its own, up to a rifle | a bid for the next gun up or better; its old gun goes on sale when the new one arrives | the new gun's price, less what the old one fetches | the meal rule |
| a bodyguard | it has an escort contract open | an escort contract | `escort-wage` an hour | `escort-budget-percent` of the wallet a shift |

**Guns.** A gun is a good like a meal: a bid at a depot, filled from an ask,
left there in the buyer's name and fetched by an ordinary job, as a bought
meal is. Prices: `price-pistol` (20), `price-shotgun` (35) and the existing
`price-assault-rifle` (60), each up to `gun-willingness` (150%) at the
ceiling. Rifles already come down a supply chain, so a frightened
colonist's bid for one reaches the gun bench through the producer planner
the way a hungry colonist's bid reaches the cave wall
([valuation.md](./valuation.md), [foundry.md](./foundry.md)). Pistols and
shotguns have no recipe yet: they are bought secondhand, from the colonists
who landed with them and from those trading up, whose old gun goes on sale
at its value when the new one arrives. The ladder makes a gun market where
there was none. A trade-in is also why the second rung is cheaper than it
looks: a $35 shotgun costs a pistol owner $15.

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

A contract is a **post that follows a colonist**. The guard code generalizes
rather than forks: `defensePost` gets a second shape, an area of
`escort-radius` (3) round its client's position, and everything else is
shared. `JobEscort` is `JobGuard` on a moving area:

- **Who takes it.** An armed colonist looking for work whose own safety is
  calm (below `SeekAt`), in `assignWorkJob` where `tryAssignGuard` is, and
  in one comparison with it: open defense posts and escort contracts ranked
  by hourly pay, then id. Not the client itself, and only if the client is
  reachable. A contract is open only while its client is out of its home;
  a colonist at home needs no guard.
- **Only the market.** What an escort weighs is what the book shows: the
  contract's rate, hours left, escrow, and where its client stands. Where
  the client stands is public, the way any colonist is visible on the map.
  How the client feels is not. The escort never reads the client's drives,
  affect or memories, and the client's fear reaches the market only as the
  price it chose to post. That rules out the shortcut `companyInReach`
  takes for conversation ([drives.md](./drives.md), *Known problem*).
- **Calm to serve.** The same rule holds on station, for defense posts as
  well as contracts: a guard whose own safety reaches pressing gives the
  post up (`currentFocusEligible` for `JobGuard` and `JobEscort`), and the
  post is released for the next calm, armed colonist. Guards out on the
  frontier still feel it, more slowly for being armed, Bold or skilled, so
  a detail turns over as its guards get uneasy, the way it already turns
  over for food and sleep. That is why guards never need guards: whoever
  is on station is, by construction, not afraid.
- **On station.** The escort walks to within `escort-radius` of the client
  and keeps there, re-pathing when the client moves (a target it already
  follows the way a hauler follows a route). Every tick it looks for an
  alien within `guard-sight` of itself and of the client and fights it with
  `fightAlien`. The area bound is the client, so an escort does not chase.
- **Pay.** `serveEscort` counts ticks within the radius and calls `payWork`
  for each full hour, from the contract's escrow. Hours at home or with no
  escort on station are not billed.
- **Released on leaving, like a guard.** An escort leaves for a pressing
  drive or its own fight; leaving clears the job and releases the post, and
  the next armed colonist looking for work can take it. So a contract is
  "this client guarded for eight hours", not "these people".
- **Ending.** The contract closes when its hours are served, its client
  dies (`cancelWorkOf` already closes a dead issuer's orders), or its client
  cancels it. Any remainder goes back to the client. When it closes, the
  client's want re-checks and may post the next one.

Protection is felt, not read: the client gets a persistent percept,
`colonist / guard / colonist` on the proximity channel, entering when its
escort comes within radius (`guarded`: grip up, "Walked with Bo as a
bodyguard.") and leaving when it goes. That same transition sets the place
class to guarded. The escort remembers it too (`guarded-client`), and both
take affinity from time together the way a conversation gives it, slowly.

Escort contracts show on the Market tab beside the order book (client,
escort, rate, hours served, escrow), on a new `escorts` topic, and the map
draws a thin line from escort to client while the client is selected.

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

| Setting | Default | Meaning |
| --- | --- | --- |
| `drives.safety.rate` | 1000 | base growth, before the place percent |
| `drives.safety.place.*` | −300 / −50 / 200 / −200 | home / settled / frontier / guarded |
| `safety-depth-percent` | 50 | extra frontier growth per level below the landing level |
| `hearsay-max-hops` | 2 | how many retellings news survives |
| `hearsay-hop-percent` | 50 | how much of a bump survives each retelling |
| `news-hour` | 21 | the clock hour of the evening broadcast |
| `news-days` | 30 | days of broadcasts the colony keeps |
| `escort-hours` | 8 | length of a contract |
| `escort-radius` | 3 | how close an escort keeps, and counts as guarding |
| `escort-wage` | 6 | the least an escort contract may pay an hour |
| `escort-willingness` | 150 | percent of `escort-wage` a client will pay at the ceiling |
| `escort-budget-percent` | 10 | the most of its wallet a client spends on one shift |
| `price-pistol` / `price-shotgun` | 20 / 35 | reference prices; the rifle keeps `price-assault-rifle` (60) |
| `gun-willingness` | 150 | percent of a gun's price a colonist will pay at the ceiling |
| `safety-armed-pistol` / `-shotgun` / `-rifle` | −25 / −35 / −50 | safety rate change for the best gun carried |
| `want-settle-ticks` | 1080 | how long a rung is held, still pressing, before the next is wanted |
| `want-recheck-ticks` | 90 | how often an open want is reconsidered while it presses |
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
- **Pistols and shotguns at the gun bench (S6)** are recipe rows. Until
  then the secondhand market is all there is, and that is fine.
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

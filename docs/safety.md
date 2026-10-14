# Safety and bodyguards (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan for a fifth colonist drive, **safety**, and the first way a feeling
turns into a purchase: a colonist who feels unsafe hires an armed colonist
to walk with it as a **bodyguard**, paid by the hour from its own wallet.

Safety builds from what the colonist goes through: seeing an alien, a
gunfight or a colonist killed, hearing about those things in conversation,
and time spent out on the frontier rather than at home. It falls at home,
in a staffed defense zone, and with a bodyguard at its side. Once it presses,
the colonist **wants** protection, and a general want layer (the one
`mealBidLimit` is today, for hunger alone) turns that into an order on the
book: an escort contract an armed colonist takes the way it takes a post in
a [defense zone](./defense.md).

Nothing here is built.

| Phase | What ships | Status |
| --- | --- | --- |
| S1 | The safety drive: a place-dependent rate (home, settled, frontier, guarded), bumps from percepts through a `drives` field on reactions in `cognition.yaml`, and the felt-unsafe experience at the ceiling. No behavior beyond affect. | Proposed |
| S2 | Hearsay: a recounted memory reaches the listener as a percept on a new `hearsay` channel, so hearing about a killing or a sighting is a reaction row; the dead become conversation topics. | Proposed |
| S3 | Wants: one willingness-to-pay rule for any drive, the hook that posts an order when a drive presses, and the inspector line that explains it. | Proposed |
| S4 | Bodyguards: `WorkEscort` contracts escrowed from the client, `JobEscort` as a guard post that follows a colonist, protection as a place class, and escorts on the Market tab. | Proposed |
| S5 | Critical safety: retreating home, turning down frontier work, restless nights. | Proposed |
| S6 | Other things fear buys: a gun of one's own, chipping in to a defense zone's wages. | Proposed |

## Source

What changes, when built:

- [`internal/sim/drives.go`](../internal/sim/drives.go): `DriveSafety` and
  its `DriveSpec` in `defaultDrives`; a place modifier in `driveRate`.
- `internal/sim/place.go` (new): `PlaceClass` (home, settled, frontier,
  guarded), `placeOf`, and `syncPlace`, the place twin of
  `syncDriveActivity`.
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
- `internal/sim/wants.go` (new): `wantSpec`, `willingness`, `postWants`.
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

Carrying a weapon is a trait-like modifier, not a place: an armed colonist's
safety builds at `safety-armed-percent` (−40) of the rate. Resilient (−30)
and Cowardly (+50) set `driveRate` for safety in their trait specs.

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

This is deliberately slow and uneven: a death is known by the people who
saw it and spreads as people talk. A colony-wide death notice at the start
of each day is the alternative; see *Open questions*.

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
second copy:

```go
type wantSpec struct {
    Drive  DriveKind
    From   DrivePhase         // DrivePressing: when the want switches on
    Value  func(*World) Money // what it is worth to a calm colonist
    Most   int                // willingness percent at the ceiling
    Post   func(*World, *Entity, Money) bool
}
```

`willingness` is `mealBidLimitWith` generalized: it starts at `Value` when
the drive first presses, rises linearly to `Most`% of it at the ceiling,
and is capped by a share of the wallet that grows with urgency (half until
critical, then all of it, as for a meal). One want is open per drive per
colonist. `postWants` runs when a drive's phase changes (the crossing
`syncDriveBand` already schedules) and on a cooldown while it stays
pressing, so it costs nothing for a calm colonist.

The inspector shows the want and why: "Uneasy (720). Wants a bodyguard:
will pay up to $4/h for 8 h; has $61." A colonist that cannot afford one
says so. That is rule 6 (every choice explainable), and it is most of the
tuning interface.

Hunger moving onto `wantSpec` is a refactor, not part of this plan; the
meal rule is the reference the general one is checked against.

### Bodyguards

Safety's want posts an **escort contract**: a `WorkEscort` work order,
issued by the client, for `escort-hours` (8) hours at the hourly price
`willingness` gives, escrowed up front through `postWork`. Defense zones
pay from the treasury as they go because a detail has no end
([defense.md](./defense.md)); a contract has one, so it escrows like every
other work order, and the money audit already counts it.

A contract is a **post that follows a colonist**. The guard code generalizes
rather than forks: `defensePost` gets a second shape, an area of
`escort-radius` (3) round its client's position, and everything else is
shared. `JobEscort` is `JobGuard` on a moving area:

- **Who takes it.** An armed colonist looking for work, in `assignWorkJob`
  where `tryAssignGuard` is, and in one comparison with it: open defense
  posts and escort contracts ranked by hourly pay, then id. Not the client
  itself, and only if the client is reachable. A contract is open only
  while its client is out of its home; a colonist at home needs no guard.
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
| `safety-armed-percent` | −40 | rate change while carrying a weapon |
| `hearsay-max-hops` | 2 | how many retellings news survives |
| `hearsay-hop-percent` | 50 | how much of a bump survives each retelling |
| `escort-hours` | 8 | length of a contract |
| `escort-radius` | 3 | how close an escort keeps, and counts as guarding |
| `escort-willingness` | 250 | percent of `wage-guard` an unsafe colonist will pay at the ceiling |
| `want-recheck-ticks` | 90 | how often an open want is reconsidered while it presses |

## Extending it

- **A new thing to be afraid of** (a sinkhole, a cave-in, an uncatalogued
  species) is an occurrence and a reaction with a `drives` field. No Go.
- **Another drive that buys** (comfort buying furniture, hygiene buying a
  wash) is a `wantSpec` row and its `Post`.
- **Buying a gun instead (S6).** A second option on safety's want: a bid for
  the cheapest weapon on the book, costed as hours of escort it would
  replace. The want picks the option with the most relief per dollar.
  Guns already have a supply chain ([foundry.md](./foundry.md)).
- **Chipping in to a defense zone (S6).** A colonist whose frontier is a
  zone's area could add to the zone's wage pool instead of hiring alone,
  which is the first public good paid for by the people who use it
  (see [work-market.md](./work-market.md)).
- **Invariants:** every safety rate change goes through `refreshDrive`; the
  guarded flag changes only on the escort's enter/leave transition; a
  contract has at most one escort on station; escorts and posts are ranked
  by pay then id, never by map order; hearsay never exceeds
  `hearsay-max-hops`.

## Open questions

- **Death notices.** Gossip-only means a colonist killed alone, unseen, is
  barely mourned until someone notices. Should there be a daily notice, an
  occurrence everyone perceives at the day's turn, for each colonist who
  died the day before?
- **Do escorts count as frontier work?** An escort out on the frontier
  feels it too, less because it is armed. Should being the bodyguard itself
  count as guarded (strength in numbers), or should guards need guards?
- **Does a defense zone over the colonist's own home matter?** Home is
  already the safest place. Probably not, but a raid on the dormitories is
  exactly when it would.
- **Who sees fear?** Colonists currently read each other's drives to decide
  who will talk ([drives.md](./drives.md), *Known problem*). An escort
  should take a contract because it is on the book, not because it can see
  the client is afraid; keep it that way.

## Related

- [drives.md](./drives.md): the drive model, bands, consequences, and the
  location-dependent rate roadmap this builds on.
- [defense.md](./defense.md): guard posts, which escorts generalize.
- [compositional-perception-and-events.md](./compositional-perception-and-events.md):
  the grammar the bumps and the hearsay channel extend.
- [affect.md](./affect.md): wear, fresh/worn readings, trait scales.
- [conversation-topics.md](./conversation-topics.md): memory and colonist
  topics, which carry hearsay.
- [valuation.md](./valuation.md): `mealBidLimit`, the first want.
- [labor.md](./labor.md): work orders and escrow.
- [work-market.md](./work-market.md): ranking paid work by rate.

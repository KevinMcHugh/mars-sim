# Aid and emergency services (proposal)

> Part of the [mars-sim documentation](./README.md).

## What it is

A plan for what happens when a colonist is in trouble and someone else could
help. Today nobody can. A colonist who passes out can starve on the floor while
its friends walk past, and a wound never heals. This proposal covers two ways
help arrives:

- **Aid.** A colonist who sees someone in trouble may stop what it is doing
  and help, because it cares. No one pays for it. How likely it is to help
  depends on how bad things look, who the casualty is to it, what kind of
  person it is, and what else it needs right now.
- **Emergency services.** The colony pays colonists to be on call as EMTs.
  An EMT is told about every casualty in the colony, not just the ones it
  can see. It responds before other work, and it can do more than a
  bystander: move a casualty and treat wounds.

Both go through the same focus arbiter as eating, fleeing and fighting.
Neither one is a new rung on a ladder.

Nothing here is built. Issue:
[KevinMcHugh/mars-sim#154](https://github.com/KevinMcHugh/mars-sim/issues/154).

| Phase | What ships | Status |
| --- | --- | --- |
| A1 | The casualty model: a `Peril` enum, the casualty index, the `casualty / present` percept, and the roster and map showing who is in trouble. No behavior changes. | Proposed |
| A2 | Aid: `FocusAid` and `JobAid`. A bystander rouses a passed-out colonist and may give it a meal. The `Caring`/`Callous` traits. Aid leaves memories, affinity and guilt behind. Hunting aliens go for the helpless first. | Proposed |
| A3 | Wound care: first aid dresses wounds, dressed wounds heal in bed, the infirmary cot fixture, and `SkillMedicine`. | Proposed |
| A4 | EMTs: an on-call detail paid by the hour from the treasury. EMTs keep working while on call and drop it for a casualty anywhere in the colony. Dragging a casualty to the infirmary. | Proposed |
| A5 | Bleeding: severe hits get a clock that aid can beat. Off by default until measured. | Proposed, optional |
| Later | Prey preference as a per-species field. Medicine and prosthetics (limbs ordered on the next resupply), medkits as a supply chain, paying EMTs per call on the order book. | Not designed |

## Decisions already made

| Topic | Decision |
| --- | --- |
| Traits | Yes: a new `compassion` trait group holding `Caring` and `Callous`. |
| EMTs between calls | They work elsewhere. Being on call is a status the colonist carries, not a job it stands at. |
| Aliens and the helpless | Hunters prefer prey that cannot flee or fight back. All hostile species do this at first; it becomes a per-species setting later. |
| Guilt | Yes, but only for colonists the arbiter actually offered the rescue to and who chose something they did not need. See *Guilt: who could have helped*. |

## Source

What changes, when built:

- `internal/sim/casualty.go` (new): `Peril`, `perilOf`, the casualty index
  (`World.casualties`), `JobAid` and its stages, rousing, dressing, and
  dragging.
- [`internal/sim/focus.go`](../internal/sim/focus.go): `FocusAid`, and an
  `aid` contribution in `fillFocusCandidates`.
- [`cognition.yaml`](../cognition.yaml): the `aid` focus weights, the
  `casualty` noun, the `aid`/`rouse` actions, the perception rule, and the
  reactions (helped, was helped, watched someone die).
- [`internal/sim/relationships.go`](../internal/sim/relationships.go): the
  first thing that *simulates* against affinity, which today is "tracked and
  displayed only".
- [`internal/sim/drive_effects.go`](../internal/sim/drive_effects.go): the
  `roused` effect profile.
- [`internal/sim/defense.go`](../internal/sim/defense.go): the template for
  the EMT detail. `emt.go` should copy it, not generalize it on the first
  pass.
- [`internal/sim/skills.go`](../internal/sim/skills.go): `SkillMedicine`.
- [`internal/sim/combat.go`](../internal/sim/combat.go): wounds record that
  they need dressing, and later that they are bleeding.
- [`internal/sim/personality.go`](../internal/sim/personality.go): the
  `groupCompassion` trait group, `TraitCaring` and `TraitCallous`.
- [`internal/sim/behaviors.go`](../internal/sim/behaviors.go): the hostile
  `hunt` rung's `preyInRoom(hostilePrey)` finder learns to rank helpless
  prey ahead of nearer prey.

## How it works

### The tension: is this a ladder?

"Am I dying?" and "is someone near me dying?" seem to call for fixed rules
that override everything else. The focus system already handles the first
one without a hard rule. A pressing fatal drive gets `fatal_bonus` (150), and
`escape` has a base of 400. Those are scores. They are big enough that they
almost always win, but they can still be outweighed. The only hard rules are
**eligibility** rules, such as a pressing fatal drive making non-fatal drive
focuses ineligible.

Aid follows the same split:

- **Hard gates (code).** These are only rules that exist to keep things
  correct. A casualty does not choose anything. A responder cannot be a
  casualty itself. One colonist holds a casualty's aid claim at a time, the
  same ownership rule every job follows. Two can hold it when dragging (A4).
- **Everything else is a score.** That includes self-preservation against
  altruism. A colonist who is hungry, but not yet critical, will stop for its
  dying child. A colonist who is critically starving will not stop for a
  stranger. Each of those outcomes is a sum of named contributions that a
  trace can show (principle 6).

So the ladder-like behavior comes from the weights, not from the structure.
That means personality and relationships can change the order, which a
ladder never allows.

### Casualties and `Peril`

A **casualty** is a colonist that cannot help itself and will get worse
without help. It is described by an enum (principle 1). Other systems switch
on the enum and never read HP or drive levels themselves:

| `Peril` | Meaning | Today's cases |
| --- | --- | --- |
| `PerilNone` | Not a casualty | Everyone else |
| `PerilStable` | Down, but not getting worse | Passed out, hunger well below its ceiling |
| `PerilSerious` | Getting worse; death is hours away | Passed out with hunger past `SeekAt`; an undressed wound below half its part's HP (A3); bleeding (A5) |
| `PerilCritical` | Death is close | Passed out with hunger at its ceiling (starving); aggregate HP under `peril-critical-hp`; heavy bleeding (A5) |

`perilOf(e)` is the one place that maps state to `Peril`. It is computed when
the inputs change: passing out, coming to, drive crossings that are already
scheduled (`nextCrossing`), a hit, a dressing. It is not computed every tick.
Colonists whose peril is not `PerilNone` sit in `World.casualties`, a slice
sorted by `EntityID`. The slice is updated as colonists enter and leave it, so
nothing has to scan the world to find casualties (principle 9). It is almost
always empty or tiny.

A walking wounded colonist (hurt, but conscious and mobile) is not a
casualty. It can go to the infirmary itself (A3). This keeps aid for people
who are actually helpless.

### Noticing: the percept

There is a new persistent percept, `casualty / present`, on the sight
channel within `aid-sight` (default the same as `flee-radius`), with
`enter-and-ongoing` cadence. It goes through the existing perception funnel,
so the casualty marks the witness's mind dirty and the next arbitration sees
it. `interrupt_rest` stays off: a sleeping colonist does not wake for a
casualty it cannot see.

The existing `collapse` occurrence (`passOut`) and the alien `bite`
occurrences already reach witnesses. Their reactions get a stimulus bias
toward `FocusAid`, so having *just seen* someone collapse counts for more
than walking past someone who has been lying there for an hour. That is
realistic, and it uses machinery that already exists.

### Deciding to help: `FocusAid`

`FocusAid` is a new focus kind. It is eligible when the colonist perceives at
least one casualty that nobody else is helping, and the colonist is not a
casualty itself. Its score is built from named contributions:

| Contribution | Source | Default sketch |
| --- | --- | --- |
| Base | `focuses.aid.base` | 30 |
| Peril (in the `Drive` slot) | the casualty's `Peril` | Stable 20, Serious 60, Critical 150 |
| Bond (in the `Personality` slot) | `affinityBetween(helper, casualty)`, plus kin | `aid-affinity-weight` × affinity / `AffinityMax` (±60), +40 for close kin |
| Affect | `charge_weight`, `grip_weight` | charge 10, grip 30: a composed colonist acts, and a panicked one freezes |
| Traits | personality | Caring +40; Callous −40 for anyone but kin; Cowardly −40 while a threat is in sight |
| Stimulus | just saw the collapse or attack | as for other reactions |
| Commitment | already aiding | `current_bonus` |
| Distance | cheap distance to the casualty | −1 per unit |
| Bystander | someone else is already aiding | the casualty is ineligible, not just penalized |

A few worked cases with the shipped weights (eat: base 40,
drive_weight 100, critical +100, fatal +150; work: base 25):

| Situation | Aid | Best rival | Outcome |
| --- | --- | --- | --- |
| A stranger collapses 6 tiles away; the helper is fed | 30+20−6 = 44 | work ≈ 25 | Helps |
| A stranger is starving on the floor; the helper is pressing hungry | 30+150−6 = 174 | eat ≈ 40+50+150 = 240 | Eats first, then helps |
| Its child is starving on the floor; the helper is pressing hungry | 30+150+40+30−6 = 244 | eat ≈ 240 | Helps (barely) |
| Its child is starving; the helper is critically hungry | 244 | eat ≈ 390 | Eats |
| A rival (affinity −80) collapses; nothing is pressing | 30+20−48−6 ≈ −4 | idle 0 | Walks past |
| A friend is down with an alien 4 tiles away; the helper is armed and composed | about 150 | fight ≈ 15+grip | Usually helps; tuning decides |

The last row is the one that needs measuring. Threats stay eligible and
compete on score, as they do against a fatal drive today. A parent can still
run into danger for its child, and that is a story worth telling.

### The compassion traits

A new trait group, `groupCompassion`, holds two traits. A colonist gets at
most one, or neither, like every other group.

| Trait | Aid score | After a rescue | Guilt |
| --- | --- | --- | --- |
| `Caring` | +40 toward anyone | Bigger lift from `helped` | Heavier |
| (neither) | none | as described below | as described below |
| `Callous` | −40, except toward kin | Smaller lift | None |

`Callous` still helps family. That keeps it from being simply the villain
trait: it describes a colonist whose concern stops at its own people.

**Rolling the group changes golden runs once.** `rollProfile` draws traits
from the personality stream group by group, so a new group adds draws and
moves every trait rolled after it. These traits change behavior, so they are
not flavor any more (principle 3). Two options: append the group last and
accept a one-time golden update, or roll it on its own stream so existing
draws do not move. Prefer its own stream, the way the recruit and topic
streams were added ([rng-streams.md](./rng-streams.md)).

### Helping: `JobAid`

The job carries out the focus. It claims the casualty: one helper, keyed by
`EntityID`, released in `clearJob` the way a guard post is. It walks to an
adjacent tile, then does whatever the casualty's state calls for:

| Casualty state | What a bystander does | What an EMT adds (A4) |
| --- | --- | --- |
| Passed out (from exhaustion) | **Rouse** it (`rouse-ticks`): it ends `PassedOut` and applies the `roused` drive effect | The same |
| Passed out and hungry | Rouse it, then **give it a meal** if the helper carries one (a gift, through `transfer`'s item path). It eats where it lies | Drag it to the infirmary, where a cot holds hunger (A3/A4) |
| Wounded, undressed (A3) | **Dress** the wound (`dress-ticks`, faster with medicine rank) | The same, faster |
| Bleeding (A5) | **Hold pressure**: no bleeding while the helper stays; dressing it stops it for good | The same, faster |
| Can't be reached safely | Nothing; the focus loses on score | The same |

The **`roused` effect** is a drive-effect profile, like caffeine. When it
starts, it drops the sleep drive just below its ceiling with an `Instant`
term, and for a few hours it slows sleep growth (`RateChange`). Without it, a
roused colonist would pass out again on its next tick, because nothing has
lowered its sleep drive. The roused colonist is still exhausted. It is now
awake for long enough to eat and get to a bunk, and its own focus decides
which comes first.

**A gift is the first unpaid transfer of goods.** Every other movement of a
meal goes through the market. A gift should still go through the item-path
funnel that the money audit trusts. The giver is the owner, and nothing is
created. That keeps the audit honest.

### What it leaves behind

Principle 5: each rescue should leave a lasting mark on both people.

- **Helper:** a `helped` memory ("Helped Ana Ruiz up off the floor."). It
  gives a lift in charge and grip, and the lift is bigger when the casualty
  was `PerilCritical`.
- **Casualty:** a `was-helped` memory, plus a large one-way affinity bump
  toward its rescuer (`aid-gratitude`). Gratitude is per direction, like
  conversation credit.
- **A colonist who could have helped and didn't:** `guilt`, covered in the
  next section.
- **A helper whose casualty died anyway:** `couldnt-save`, which is grief,
  not guilt. Trying and failing should feel different from walking past.

This is also the first time affinity has a real effect in the simulation.
Friendships made in conversation now decide who gets picked up off the floor.

### Guilt: who could have helped

Guilt can be determined, because the arbiter already makes the decision that
guilt is about. Whenever a colonist's arbitration has `FocusAid` eligible for
a casualty and picks something else, that is a recorded choice not to help,
along with what was chosen instead. The casualty keeps a short record of
those choices:

- `passedBy`: the colonists who were offered the rescue and passed, each with
  the focus that won. It is a slice sorted by `EntityID`, holds one entry per
  colonist (the latest choice replaces an earlier one), and is capped at
  `aid-passed-by-max` (16). It is cleared when the casualty recovers.
- Nothing is added when no arbitration happens. A colonist that never
  noticed the casualty (asleep, out of sight, on another level) was never
  offered the rescue and owes nothing.

When the casualty dies, `remove` reads `passedBy` and sends each living entry
a `guilt` occurrence. How heavy it is depends on what won, so a player can
read why:

| What won | Reading | Why |
| --- | --- | --- |
| Flee, fight, escape, or a fatal drive at `DriveCritical` | none | It could not have helped. Self-preservation is not a failing |
| A fatal drive at `DrivePressing`, or another rescue | light | It had a reason |
| Work, idle, socialize, sleep, relieve | full | It chose something it did not need |
| On-call EMT, anything but a fatal or a threat | full ×1.5 | It was paid to come |

`Caring` doubles the reading and `Callous` removes it, except when the dead
colonist was its kin. The memory names the dead colonist ("Walked past Ana
Ruiz. She died."), using the dead colonist's own pronouns, so guilt stays
specific. It is never a vague mood.

The cost is one small append on an arbitration that chose something other
than an eligible aid. That only happens while a casualty exists, so a quiet
colony pays nothing.

### Emergency services: the EMT detail (A4)

This borrows from [defense.md](./defense.md). The player sets an **EMT
detail**: the number of colonists to keep on call. It goes on the Zones tab
next to defense, or in a small Medical section, and it is not tied to an
area. The treasury pays each on-call colonist `wage-emt` per hour, as far as
it goes. That is the guard and cook model: the detail has no end, so there
is nothing to escrow.

Unlike a guard post, **being on call is not a job.** It is a status
(`Entity.onCall`) that a colonist carries while it does other work. A
colonist looking for work takes an open place on the detail
(`tryTakeOnCall`, in `EntityID` order) and then goes on choosing work as
usual. It keeps the place until it goes to sleep, passes out, becomes a
casualty, or the detail is cut below it. Then the place is released and the
next colonist looking for work takes it. Pay is counted only while the
colonist is awake and on call.

What being on call changes:

1. **Colony-wide awareness.** An on-call EMT is eligible for `FocusAid`
   toward *any* entry in the casualty index on its level or a connected
   level. It does not need to see the casualty. It skips the sight percept
   and reads the index directly, which is cheap because the index is tiny.
   When a colonist enters the index, every on-call EMT is marked
   mind-dirty (`markMindDirty`), so the call reaches it on that tick. The
   on-call roster is short, so this is cheap.
2. **Duty.** It adds `aid-duty-bonus` (default 80). That is well above
   work's commitment bonus (25), so an EMT drops a dig or a recipe for any
   casualty, and a Stable stranger across the colony beats its own pressing
   hunger. A fatal critical drive still wins: a dead EMT saves nobody.
   Dropping work goes through `clearJob`, which releases its claims as any
   interruption does.
3. **Dispatch without a dispatcher.** Responders are pulled, not pushed.
   Each EMT scores the casualties it knows about, and the claim makes sure
   only one gets each casualty. Distance in the score already sends the
   nearest one, and ties go by `EntityID` (determinism).
4. **Dragging.** An EMT can move a casualty to the nearest infirmary cot.
   The casualty stays an entity and keeps its tile in the occupancy index.
   Each step, it moves into the tile the EMT just left. It is slow
   (`drag-speed-pct`), but it never needs two entities on one tile, which is
   the trap [sanitation.md](./sanitation.md) hit with corpse entities. A
   second EMT can join the claim to restore full speed.
5. **Between calls,** an EMT works like anyone else. The retainer pays for
   the promise to drop that work, not for standing by. Response time is the
   price: an EMT mining at the frontier is far from a collapse in the
   dormitory. That has to be measured (time from entering the index to the
   first aid tick). A later option is to have on-call EMTs give a small
   preference to work near the infirmary, but only if the measurement says
   it is needed.

Eating, the toilet and conversation do not release the place, since an EMT
can answer a call from any of them. Sleep does, so the detail staffs itself
in shifts across the colony's night.

**Why the colony pays at all**, when altruism exists: altruism is local,
depends on the casualty's friendships, and is unreliable at night. A colony
of strangers, or one where everyone is asleep, lets people die. Paying for
EMTs buys coverage. It is a public good, which is the one thing
[work-market.md](./work-market.md) says the colony should still pay for.

### Aliens hunt the helpless (A2)

A colonist lying on the floor is easy prey, and a hunter should know it.
Today the hostile ladder's `hunt` rung finds prey with
`preyInRoom(hostilePrey)`, which takes the nearest. The change is to how it
ranks: a casualty counts as nearer than it is, by `helpless-prey-discount`
tiles (default 6). The hunter still goes for whatever is close, but a downed
colonist a few tiles further away wins over a standing one. The finder reads
the casualty index, so this costs nothing while the index is empty, and
golden runs move only when someone is down.

This ships in A2, with aid, on purpose. On its own it makes collapsing more
deadly; together with aid it raises the stakes of a rescue. A friend who
stops to rouse someone now does it with something closing in. The
measurement to watch is deaths among the collapsed, with and without aid.

**Per species, later.** The preference should become a property of the
species, not a rule for all hunters: some scavenge the weak, some go for the
biggest threat, some ignore the prone. That is a `PreyPreference` enum
(`PreyNearest`, `PreyHelpless`, and later others) on the species table.
Rolling it would change the roster for every seed, which would break
[species-pack.md](./species-pack.md)'s check that a pack's species roll alike
in this build. So the field should start as data with a default. It can be
derived from traits a species already rolls (an apex or a stinger-bearer, for
instance), or set per species in Creature Lab. It should not get a new draw
from the roster stream.

### Wound care and the infirmary (A3)

- Every hit `applyDamage` lands on a part marks that part's wound as
  **undressed**. An undressed part never heals. That keeps today's behavior
  ("injuries are permanent") as the default when no one helps.
- **Dressing** a wound (first aid, by anyone) marks it dressed. A dressed
  part heals `heal-rate` HP per hour, but only while the colonist is lying
  down: in a bunk, or faster in an infirmary cot.
- A **destroyed** part (0 HP) never regrows. Replacing one is medicine and
  prosthetics, which the issue's comment imagines as limbs on the next
  resupply. That is out of scope here.
- The **infirmary cot** is a fixture like a bunk, and the room is a facility
  room like any other ([construction.md](./construction.md)). A casualty in
  a cot does not starve: the cot holds the food drive (an activity row in
  the drive table, like sleep's). Wounds heal at `cot-heal-pct`. A
  conscious colonist with an undressed or badly hurt part seeks a cot the
  way a tired one seeks a bunk.
- **`SkillMedicine`** is credited per tick of dressing, rousing, and
  dragging. Rank shortens dressing and raises the heal rate of wounds the
  medic dressed. It uses the same `rankEffect` curve as every other skill.

### Bleeding (A5, optional)

Today a wound is either fatal at once (head or torso at 0) or harmless
forever. There is no clock running in between, so aid has nothing to race
against for wounds. **Bleeding** adds that clock. A hit that takes a part
below `bleed-threshold-pct` of its max starts it bleeding, which drains
aggregate HP at a rate set by the size of the wound until the wound is
dressed. This would be the first thing to make combat more deadly, so it
ships behind `bleeding: false`. It turns on by default only after a
20-seed measurement shows that bleeding plus aid kills no more colonists
than the current game does.

## Why it is this way

- **One arbiter, not a rescue rung.** A rung such as "if a casualty is in
  sight, aid" would make every colonist the same Good Samaritan and would
  ignore their history (principle 5). A score lets a rival walk past and a
  parent run into danger, and either choice can be explained.
- **`Peril` is an enum.** If aid read hunger levels and HP directly, the
  EMT, the roster, the map and the memory text would each set their own
  thresholds (principle 1).
- **A hard claim, not a bystander penalty.** If the score were only
  discounted while someone else helped, five colonists would crowd around one
  casualty on a close call. The claim is the job board's ownership invariant,
  and it is what keeps the bystander effect from becoming a pile-up.
- **Rousing, not an IV, for bystanders.** The data says the main killer is
  starving while passed out. In [pricing.md](./pricing.md), 289 of 289
  starvation deaths were colonists who had passed out first. Waking someone
  up so its own drives can work is the smallest fix with the biggest effect,
  and it needs no new facility.
- **EMTs are a detail, not an order per incident.** A bounty posted per
  casualty fits the direction [work-market.md](./work-market.md) sets
  (urgency shows up in the price). It does not work in an emergency, though,
  because a bounty only reaches colonists whose focus is already work, and
  nobody is waiting nearby. The retainer pays for availability, which is
  what emergency services actually sell. A per-call fee on top is listed
  under *Extending it*.
- **On call is a status, not a post.** A guard is paid to be in a place;
  an EMT is paid to be available. Holding a job would keep it from working,
  and an idle detail is expensive in a small colony. A status that the duty
  bonus turns into a response gets the coverage without the idle hours. The
  price is response time, which the measurement in A4 decides.
- **Guilt comes from the arbiter's own record.** If guilt were worked out
  after a death ("who was nearby?"), it would blame colonists who never
  noticed and miss the ones who walked past an hour earlier. Recording the
  choice when the arbiter made it means guilt matches what the colonist
  actually did, and the trace can show it.
- **Drag, don't carry.** A carried living colonist would need either to
  leave the occupancy index or to share a tile, and both break invariants
  that much of the code relies on.
- **Nothing changes until there is a casualty.** With the casualty index
  empty, `FocusAid` is ineligible and costs one length check, and nothing
  draws on the RNG. Golden runs move only when someone is actually in
  trouble, and the change in those runs is exactly what we want to measure.

## Extending it

- **Per-call fee:** the colony pays `fee-rescue` when a casualty reaches a
  cot or drops to `PerilNone` under an EMT's care. This could be a
  `WorkRescue` order once [work-market.md](./work-market.md) W3 sets wages
  that rise with urgency.
- **The casualty pays.** A rescued colonist tipping its rescuer, or being
  billed for its cot time like a paid fixture, is the economy's call to make,
  not this doc's.
- **Medkits:** dressing could consume a `Bandage` item made from scum fiber
  or bought at the market. That would turn first aid into a supply chain the
  way [foundry.md](./foundry.md) did for guns.
- **Prosthetics** (from the issue): a destroyed limb becomes a line on the
  next resupply's manifest. It is paid from the casualty's wallet or the
  treasury, and fitted in the infirmary.
- **Limb injuries with teeth** ([combat.md](./combat.md)): once a ruined leg
  slows a colonist, "can walk to the infirmary itself" stops being true for
  some wounded, and they become casualties.
- **Per-species prey preference:** see *Aliens hunt the helpless*. Adding a
  preference is an enum value and a case in the hunt finder.
- **Invariants:** each casualty has at most one aid claim (two while
  dragging); a colonist is in `casualties` exactly while its peril is not
  `PerilNone`; the index is kept and searched in `EntityID` order, never in
  map order. The same holds for the on-call roster and `passedBy`. A
  colonist is on call only while awake.

## Open questions

1. **Response time with working EMTs.** Is the time from collapse to first
   aid good enough without any pull toward the infirmary? Measure it before
   adding one.
2. **Which existing species traits imply `PreyHelpless`?** This only matters
   if the per-species field is derived rather than set in Creature Lab.
3. **Is guilt lopsided in big colonies?** With 2000 colonists, a death in a
   busy corridor could add up to 16 guilty memories. The cap bounds it, but
   whether 16 is the right number should come from a run.

## Related

- [drives.md](./drives.md): passing out, starvation, drive effects.
- [cascading_wsts_architecture.md](./cascading_wsts_architecture.md),
  [focus.go](../internal/sim/focus.go): the arbiter `FocusAid` joins.
- [defense.md](./defense.md): the detail-and-retainer pattern EMTs copy.
- [combat.md](./combat.md): body parts and wounds.
- [memories.md](./memories.md), [affect.md](./affect.md): what aid leaves
  behind.
- [pricing.md](./pricing.md): the measurement that every starvation death
  passed out first.
- [work-market.md](./work-market.md), [labor.md](./labor.md): who pays for
  public goods.
- [skills.md](./skills.md): `SkillMedicine`.

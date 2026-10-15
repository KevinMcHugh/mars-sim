# Aid and emergency services

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

All five phases are built. Colonists help each other (A1, A2), wounds are
dressed and heal (A3), the player can keep EMTs on call (A4, off until the
player sets a detail), and bad wounds can bleed (A5, behind the `bleeding`
setting, off by default). *As built* at the end of this doc records where the
code departed from the plan, and what each phase did in measured runs. Issue:
[KevinMcHugh/mars-sim#154](https://github.com/KevinMcHugh/mars-sim/issues/154).

| Phase | What ships | Status |
| --- | --- | --- |
| A1 | The casualty model: a `Peril` enum, the casualty index, and the roster and map showing who is in trouble. No behavior changes. | Shipped |
| A2 | Aid: `FocusAid` and `JobAid`. A bystander rouses a passed-out colonist and may give it a meal. The `Caring`/`Callous` traits. Aid leaves memories, affinity and guilt behind. Hunting aliens go for the helpless first. | Shipped |
| A3 | Wound care: first aid dresses wounds (a colonist can dress its own, slowly), dressed wounds heal lying down, the infirmary cot, and `SkillMedicine`. | Shipped |
| A4 | EMTs: an on-call detail paid by the hour from the treasury. EMTs keep working while on call and drop it for a casualty anywhere in the colony. Dragging a casualty down from blood loss to an infirmary cot. | Shipped (detail 0 until the player sets one) |
| A5 | Bleeding: bad open wounds drain HP until dressed, and a colonist that loses too much collapses. | Shipped, off by default |
| Later | Prey preference as a per-species field. Medicine and prosthetics (limbs ordered on the next resupply), medkits as a supply chain, paying EMTs per call on the order book. | Not designed |

## Decisions already made

| Topic | Decision |
| --- | --- |
| Traits | Yes: a new `compassion` trait group holding `Caring` and `Callous`. |
| EMTs between calls | They work elsewhere. Being on call is a status the colonist carries, not a job it stands at. |
| Aliens and the helpless | Hunters prefer prey that cannot flee or fight back. All hostile species do this at first; it becomes a per-species setting later. |
| Guilt | Yes, but only for colonists the arbiter actually offered the rescue to and who chose something they did not need. See *Guilt: who could have helped*. |

## Source

Built (A1, A2):

- [`internal/sim/casualty.go`](../internal/sim/casualty.go): `Peril` and
  `perilOf`; the casualty index (`World.casualties`, `addCasualty`,
  `dropCasualty`, `isCasualty`); `aidOptionFor` and `aidRegard` (what
  helping scores); `noticeCasualty`; `notePass` (the passed-by record);
  `runAidFocus`, `rouse` and `releaseAid` (`JobAid`); `casualtyDied` (guilt
  and grief); the `roused` drive effect; `helplessBias`.
- [`internal/sim/focus.go`](../internal/sim/focus.go): `FocusAid`, its
  candidate in `fillFocusCandidates`, `FocusCandidate.Casualty`.
- [`internal/sim/systems.go`](../internal/sim/systems.go): `runCognition`
  records a pass after arbitration; `colonistTurn` calls `noticeCasualty`;
  `clearJob` releases `JobAid`.
- [`internal/sim/drive_bands.go`](../internal/sim/drive_bands.go): `passOut`
  adds a casualty, coming to drops it. [`world.go`](../internal/sim/world.go):
  `remove` settles a casualty's death first.
- [`internal/sim/personality.go`](../internal/sim/personality.go):
  `groupCompassion`, `TraitCaring`, `TraitCallous`, `ownStreamGroup`,
  `rollCompassion`; [`recruit.go`](../internal/sim/recruit.go) rolls a
  recruit's last; [`rng.go`](../internal/sim/rng.go): the compassion stream.
- [`internal/sim/behaviors.go`](../internal/sim/behaviors.go) and
  [`tags.go`](../internal/sim/tags.go): `preyInRoom` ranks through
  `nearestTaggedRanked` with `helplessBias`.
- [`internal/sim/cognition_config.go`](../internal/sim/cognition_config.go)
  and [`cognition.yaml`](../cognition.yaml): the `aid` focus; the `rescue`,
  `lose`, `abandon` and `fail` actions; the `aid-sight` radius and the
  `witness-collapse` perception; the `witnessed-collapse`, `helped`,
  `was-helped`, `couldnt-save`, `guilt` and `guilt-light` reactions; the
  Caring and Callous trait rules.
- [`internal/sim/config.go`](../internal/sim/config.go): the `Aid` settings
  section and the `roused` drive effect.
- `Aiding` (state), `JobAid`, `ActAiding` ("aiding" on the Activity tab);
  `EntityView.Peril`, shown after the state in both rosters
  ("passed out (critical)"); the 😵 and 🩹 map glyphs
  ([`glyphs.go`](../internal/glyphs/glyphs.go), `@_` and `@h` in ASCII). In
  the browser a helper is a firefighter in its own gender and skin tone
  (👨🏿‍🚒 👩🏻‍🚒 🧑🏽‍🚒, [`looks.go`](../internal/glyphs/looks.go)); the
  terminal keeps 🩹, since terminals disagree about the width of ZWJ
  sequences ([colonist-looks.md](./colonist-looks.md)).
- [`internal/sim/casualty_test.go`](../internal/sim/casualty_test.go).

- [`internal/sim/wounds.go`](../internal/sim/wounds.go): `Wound`, opening
  a wound (`Entity.wound`, from `applyDamage`) and `hurt`; `dressWound`,
  `healWounds`; `bleedRate`, `bleed`, `collapseFromBloodLoss`,
  `stillDownFromBloodLoss`; the infirmary (`Cot`, `infirmaryRoom`,
  `cotsShort`, `freeCot`, `layInCot`, `leaveCot`); dragging (`dragStep`,
  `releaseDrag`, `swapEntities`).
- [`internal/sim/emt.go`](../internal/sim/emt.go): `SetEMTDetail`,
  `tryTakeOnCall`, `releaseOnCall`, `serveOnCall` (pay), `EMTView`.
- [`internal/sim/casualty.go`](../internal/sim/casualty.go) gained
  `refreshCasualty`, `canHelp`, `comeRound`, `finishAid` and the
  `aidDone` record; [`skills.go`](../internal/sim/skills.go)
  `SkillMedicine`; [`drives.go`](../internal/sim/drives.go) holds hunger
  in a cot (`driveRate`).
- The Zones tab's *Emergency services* section and the `emt-detail`
  command ([`ZonesPanel.svelte`](../web/src/ui/ZonesPanel.svelte),
  [`zones.go`](../internal/wire/zones.go), `hostAPI` 24); each part's
  wound in the inspector and the TUI roster; the 🏥 cot glyph (`+H`).
- [`internal/sim/wounds_test.go`](../internal/sim/wounds_test.go).

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
| `PerilStable` | Needs someone, not getting worse | Passed out, hunger well below its ceiling; an open wound that is only a graze; down from blood loss but laid in a cot |
| `PerilSerious` | Getting worse; death is hours away | Passed out with hunger past `SeekAt`; an open wound below half its part's HP, or a destroyed part; bleeding; down from blood loss on the floor |
| `PerilCritical` | Death is close | Passed out with hunger at its ceiling (starving); HP under `peril-critical-hp` |

`perilOf(e)` is the one place that maps state to `Peril`. It is read on
demand, not stored: the hunger it reads is lazy anyway, and only casualties
are ever asked, so there is nothing to keep in step. Casualties sit in
`World.casualties`, a slice sorted by `EntityID`. `passOut` adds one and
coming to (on its own, or roused) or dying takes it out, so nothing has to
scan the world to find casualties (principle 9). It is almost always empty or
tiny. A casualty is a colonist passed out where it fell (from exhaustion or
blood loss) or one with an open wound; `refreshCasualty` puts it in or takes
it out whenever any of that changes.

**A walking wounded colonist is a casualty too** (the plan said not). It
needs a dressing before its wound can heal, and the aid focus is how a
dressing happens: a bystander offered it, or the colonist itself, which is
always offered its own open wounds (distance 0, no regard) and dresses them
at `aid-self-dress` (200%) of the time. Only what a helper can actually do
is offered (`canHelp`): dress an open wound, rouse a colonist down from
exhaustion, or, an on-call EMT only, drag one down from blood loss to a free
cot. Nobody is offered a colonist it can do nothing for, so nobody stands
over it.

### Noticing a casualty

The plan was a persistent `casualty / present` percept. As built, eligibility
is read straight from the casualty index, the way threats are read from the
alien index rather than from percepts: `aidOptionFor` walks the (tiny) index
for casualties on the colonist's level within `aid-sight` (6) that it can
reach and nobody else is helping. Two things make sure the arbiter looks:

- **Seeing the collapse.** `passOut`'s `collapse` occurrence reaches anyone
  within `aid-sight` through a new `witness-collapse` perception. The
  `witnessed-collapse` reaction is a small jolt and an aid stimulus (+40 for
  30 ticks), so having *just seen* someone go down counts for more than
  walking past someone who has lain there an hour.
- **Coming across one.** `noticeCasualty` marks a colonist's mind dirty the
  first time an unhelped casualty is in sight, so it thinks again that tick
  rather than at its next scheduled reconsideration (up to 32 ticks later,
  most of a starving casualty's time).

A sleeping or resting colonist on the cognition fast path does neither: it
does not wake for a casualty it cannot see.

### Deciding to help: `FocusAid`

`FocusAid` is a new focus kind. It is eligible when the colonist perceives at
least one casualty that nobody else is helping, and the colonist is not a
casualty itself. Its score is built from named contributions:

| Contribution | Source | Default sketch |
| --- | --- | --- |
| Base | `focuses.aid.base` | 30 |
| Peril (in the `Drive` slot) | the casualty's `Peril` | Stable 20, Serious 60, Critical 150 |
| Bond (in the `Personality` slot) | `affinityBetween(helper, casualty)`, plus kin | `aid-affinity-weight` × affinity / `AffinityMax` (±60), +40 for close kin (spouse, parent, child, sibling) |
| Affect | `charge_weight`, `grip_weight` | charge 10, grip 30: a composed colonist acts, and a panicked one freezes |
| Traits | personality | Caring +40; Callous −40 for anyone but kin; Cowardly −40 while a threat is in sight |
| Stimulus | just saw the collapse | +40 for 30 ticks (`witnessed-collapse`) |
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

**Rolled on its own stream.** `rollProfile` draws traits from the
personality stream group by group, so a new group in that loop would add
draws and move every trait rolled after it. These traits change behavior, so
they are not flavor (principle 3). So the group is skipped by `rollTraits`
(`ownStreamGroup`) and drawn after the profile from `compassionRNG`, the way
the recruit and topic streams were added ([rng-streams.md](./rng-streams.md)).
A recruit's is drawn last from the recruit stream. With aid off, every
golden run is unchanged.

### Helping: `JobAid`

The job carries out the focus. It claims the casualty: one helper, keyed by
`EntityID`, released in `clearJob` the way a guard post is. It walks to an
adjacent tile, then does whatever the casualty's state calls for:

| Casualty state | What a bystander does | What an EMT adds (A4) |
| --- | --- | --- |
| Passed out (from exhaustion) | **Rouse** it (`aid-rouse-ticks`, 8): it ends `PassedOut` and applies the `roused` drive effect | The same |
| Passed out and hungry | Rouse it and **give it a meal** if the helper carries one of its own and the casualty has none (a gift). Its own focus then eats it | Drag it to the infirmary, where a cot holds hunger (A3/A4) |
| Wounded, undressed (A3) | **Dress** the wound (`dress-ticks`, faster with medicine rank) | The same, faster |
| Bleeding (A5) | **Hold pressure**: no bleeding while the helper stays; dressing it stops it for good | The same, faster |
| Can't be reached safely | Nothing; the focus loses on score | The same |

The **`roused` effect** is a drive-effect profile, like caffeine, and the
first one that ships (`rousedEffect`, found by name). It takes 50 points off
the sleep drive at once (`Instant`) and slows its growth to a quarter for 135
ticks, three hours (`RateChange` −75). Without it, a roused colonist would
pass out again about nine ticks later, because nothing has lowered its sleep
drive: that is exactly what the first build did, and one test colony roused
the same colonist 14 times before it starved. The roused colonist is still exhausted. It is now
awake for long enough to eat and get to a bunk, and its own focus decides
which comes first.

**A gift is the first unpaid transfer of goods.** Every other movement of a
meal goes through the market. The meal moves from the helper's pocket to the
casualty's, and an item in a colonist's pocket with no cargo record is its
own ([cargo.go](../internal/sim/cargo.go)), so ownership moves with it.
Nothing is created and no money moves, so neither audit sees it.

### What it leaves behind

Principle 5: each rescue should leave a lasting mark on both people.

- **Helper:** a `helped` memory ("Helped Ana Ruiz up off the floor."). It
  gives a lift in grip and valence that stays good with repetition. Caring
  colonists feel it half again as much, Callous ones half as much.
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
| A fatal drive at `DrivePressing` | light (`guilt-light`) | It had a reason |
| Work, idle, socialize, sleep, relieve | full (`guilt`) | It chose something it did not need |
| On-call EMT, anything but a fatal or a threat | full ×1.5 | It was paid to come (A4, not built) |

The two weights are two reactions, `guilt` (`abandon`) and `guilt-light`
(`fail`), so each tier's reading is data in `cognition.yaml`. `Caring`
doubles either through a trait rule. `Callous` removes it unless the dead
colonist was close kin; that exception needs the family tree, so it is in
`casualtyDied`, not a rule. The memory names the dead colonist ("Walked past
Ana Ruiz, who died."), so guilt stays specific. It is never a vague mood.
Only the casualty the arbiter offered is recorded: with two down in sight,
a colonist owes the one it was offered, not both.

The cost is one small append on an arbitration that chose something other
than an eligible aid. That only happens while a casualty exists, so a quiet
colony pays nothing.

### Emergency services: the EMT detail (A4)

This borrows from [defense.md](./defense.md). The player sets an **EMT
detail**: the number of colonists to keep on call (0 to `MaxEMTDetail`, 20;
`emt-detail` sets where a game starts, 0 by default). It is the *Emergency
services* section of the Zones tab, under the defense zones: − / + for the
detail and for the wage, who is on call, what has been paid, how many cots
stand, and the market (below). It is not tied to an area. The treasury pays
each on-call colonist the wage for every hour on call and awake, as far as
it goes: the guard and cook model, since the detail has no end and there is
nothing to escrow.

**The wage is a market price, not a rule.** The player offers it, in cents
an hour (`SetEMTWage`, 0 to `MaxEMTWage`, `emt-wage` 25¢ where a game
starts), and each colonist decides whether being on call is worth it to
it. A colonist's **ask** (`emtAsk`) is:

- the share of an hour calls take (`emtShare`, per mille), at what an hour
  of its time is worth to it (its `reservation`, the same reckoning the
  producer planner uses: `labor-price`, or more if it has lately earned
  more), plus
- `emt-premium` (10%) of that hour's worth, for being the one who has to
  drop everything. A Caring colonist asks half the premium, a Callous one
  double.

At the defaults that is about 18¢ an hour (an hour of plain work is worth
90¢; calls are reckoned at 10% of it, `emt-call-share` 100‰, until the
colony has measured its own). `emtShare` is measured: once a day's worth of
on-call ticks has gathered, the share of them EMTs spent on calls (`JobAid`)
is folded in, a quarter new to three quarters old. A busier colony's EMTs
ask more.

A colonist looking for work takes an open place only if the wage meets its
ask, and on each payday quits if it no longer does: a colonist whose time
became worth more (a chef earning well) leaves the detail to someone
cheaper. So a wage set too low leaves places empty, and the colonists who
take them are those whose time is cheapest or who care most. Pay accrues in
cents and is paid in whole dollars as it adds up (`emtOwed`). The tab shows
the cheapest ask among colonists not on call, how many would take the place
at the wage offered, the measured call share, and how long casualties have
waited for an EMT to set out (`noteResponse`), so the player can price it.

**Paying by the call.** The player can also set a fee per call (`SetEMTFee`,
0 to `MaxEMTFee` $50, `emt-fee` where a game starts, $0). With a detail and
a fee, the colony posts a `WorkRescue` order on the work book for every
casualty as it goes down (`postRescue`), escrowing the fee from the
treasury as any work order does. It is settled when the casualty stops being
one (`settleRescue`, from `dropCasualty`, or from `finishAid` for one laid
in a cot, which is still a casualty while it heals there): paid to the
on-call EMT that saw it to, and otherwise, if it came round on its own, saw
to itself, or a bystander helped, back to the treasury. **Bystanders are
never paid**: a colonist who stops for a stranger does it for its own
reasons, and the fee is the colony's price for coverage, not for kindness.

The fee comes off the retainer a colonist asks. The colony measures how
many calls an EMT answers per thousand hours on call (`emtCallRate`, folded
in with `emtShare`, from a prior of `emt-call-rate` 20: without one, a
detail paid only by the call never staffed, since nobody on call meant no
calls measured and so no fee in anyone's reckoning), and a colonist's ask is cut by what that many calls
would pay it at the fee, never below nothing. So a colony can staff its
detail with a small retainer and pay mostly for what is done; a quiet
colony's EMTs still want the retainer, since few calls come. The tab shows
the fee, what has been paid in fees, and the call rate.

**The pay package is gameplay.** The wage and the fee are two controls, and
the player can set any mix of them. The mix changes who signs up and how
hard they work, not only what it costs:

- **Who signs up.** Fee income is earned call by call, so it is less sure
  than a wage, and colonists count on it differently (`emtFeeTrust`):
  `emt-fee-trust` (80%) of its expected value for most, 40 points more for
  an Industrious colonist (it expects to earn it), 30 fewer for a Lazy one,
  20 more for an Optimist and 20 fewer for a Pessimist. With no fee, work
  ethic does not change anyone's ask, so a pure wage draws whoever is
  cheapest, the lazy as readily as anyone; a fee-heavy package draws the
  hard workers.
- **How hard they work.** On call, an EMT adds `emtDuty` to helping any
  casualty: the duty a wage buys (`aid-duty-bonus`, 80), felt at half
  strength by a Lazy EMT and a quarter more by an Industrious one, plus the
  pull of the fee itself, `emt-fee-incentive` (4) points a dollar up to
  `emt-fee-incentive-max` (100), which every EMT feels alike. A wage pays
  the same however hard an EMT answers; a fee pays for answering.

The Zones tab shows the package (what wage and fee together pay an EMT an
hour, at the measured call rate, and how much of it is by the call) and
each on-call EMT's work ethic, so the player can see who a package draws.

Unlike a guard post, **being on call is not a job.** It is a status
(`Entity.onCall`) that a colonist carries while it does other work. A
colonist looking for work takes an open place on the detail
(`tryTakeOnCall`, first thing in `assignWorkJob`) and then goes on choosing
work as usual. The roster (`World.onCall`) is kept sorted by ID, and a cut
stands down the highest IDs first. It keeps the place until it goes to sleep, passes out, becomes a
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
4. **Dragging.** Only a casualty down from blood loss is dragged: one down
   from exhaustion is roused, and a walking wounded one walks. Once its
   wounds are dressed, the EMT claims the nearest free cot on its level and
   in its room (`freeCot`, ties by point) and plans the *casualty's* route
   to it. Each step at `drag-speed` (50%: one tile every other tick), the
   casualty moves one tile along it and the EMT steps into the tile it left,
   so the two stay side by side and nothing ever shares a tile; when the
   next tile is the EMT's own, they swap (`swapEntities`, since `moveEntity`
   needs its destination free). A blocked tile waits up to `StuckLimit`; a
   route that leaves the level, or a shut way, gives up and frees the cot.
   Beside the cot, the casualty is laid in it. The plan's second EMT
   joining a drag is not built.
5. **Between calls,** an EMT works like anyone else. The retainer pays for
   the promise to drop that work, not for standing by. Response time is the
   price: an EMT mining at the frontier is far from a collapse in the
   dormitory. It is not measured yet (time from entering the index to the
   first aid tick); a later option is to have on-call EMTs give a small
   preference to work near the infirmary, if that measurement says so.

Eating, the toilet and conversation do not release the place, since an EMT
can answer a call from any of them. Sleep does (`serveOnCall` releases it
once the colonist is `Sleeping`), and so does going down or becoming a
casualty itself, so the detail staffs itself in shifts across the colony's
night.

**Why the colony pays at all**, when altruism exists: altruism is local,
depends on the casualty's friendships, and is unreliable at night. A colony
of strangers, or one where everyone is asleep, lets people die. Paying for
EMTs buys coverage. It is a public good, which is the one thing
[work-market.md](./work-market.md) says the colony should still pay for.

### Aliens hunt the helpless (A2)

A colonist lying on the floor is easy prey, and a hunter should know it.
The hostile ladder's `hunt` rung finds prey with `preyInRoom(hostilePrey)`,
which used to take the nearest. It now ranks through `nearestTaggedRanked`:
a casualty counts as nearer than it is, by `helpless-prey-discount` tiles
(default 6; 0 turns it off). The hunter still goes for whatever is close,
but a downed colonist a few tiles further away wins over a standing one.
The bias is only passed while the casualty index is non-empty, so this
costs nothing in a colony where nobody is down.

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

- **Every hit opens a wound.** `applyDamage` marks the part `WoundOpen` (a
  free function, so the two callers that can hit a colonist, `strike` and a
  fall, then call `hurt` to put it in the casualty index). An open wound
  never heals, which keeps the old rule ("injuries are permanent") for a
  colonist nobody sees to.
- **Dressing** a wound takes `aid-dress-ticks` (15), less with medicine
  rank, twice that on oneself. A helper dresses every open wound, one at a
  time, before it does anything else for the casualty. A destroyed part is
  closed (`WoundNone`), and never grows back: replacing one is medicine and
  prosthetics, out of scope here.
- **Healing.** A dressed wound heals `heal-per-hour` (1) HP an hour while
  the colonist lies down: asleep in a bunk at 100%, in an infirmary cot at
  `heal-cot` (300%), down on the floor at `heal-floor` (50%). Points go one
  at a time to the dressed part furthest from whole, and to the HP pool with
  it. The accumulator counts in hundredths of an HP times ticks an hour, so
  the rate is exact; the first version truncated a 45-tick hour's healing to
  nothing.
- **Walking wounded heal in their own bunks at night.** The plan had them
  seek a cot like a bed; that would have meant a second sleep facility in
  the facility-choice code for little gain. Cots are for casualties an EMT
  brings, which is what makes them worth building.
- **The infirmary** is a room of cots, commissioned like the meeting hall
  (`infirmaryRoom`, `cotsShort`): one cot for each `colonists-per-cot` (8),
  at least two, and **only while the EMT detail is above zero**. A cot is
  furniture, like a chair: a casualty lies on the floor beside it.
  `cotClaims` says which casualty lies at, or is being dragged to, each.
  A casualty in a cot is not hungry (`driveRate` holds its food drive at 0,
  a drip in effect) and heals at the cot's rate.
- **`SkillMedicine`** is credited for every dressing, rousing and drag step.
  Rank shortens dressing and raises the heal rate of the wounds the medic
  dressed (its yield, kept as the casualty's `healPct`). Nobody arrives with
  it: `backgroundWeights` leaves it out, so adding it moved no seed's
  arrival rolls.

### Bleeding (A5, optional)

Without bleeding, a wound is either fatal at once (head or torso at 0) or
harmless forever, and aid has nothing to race. With `bleeding` on:

- Every open wound on a part below `bleed-threshold` (50%) of its HP bleeds
  `bleed-rate` (40) hundredths of an HP an hour for each point the part has
  lost: a torso down 10 points bleeds 4 HP an hour, a death in a few hours
  for a 40-HP colonist. A helper beside the casualty, aiding it, holds the
  wound (no bleeding while it works); dressing stops it for good.
- A colonist bled below `bleed-collapse` (30%) of its HP **collapses from
  blood loss**: down where it stands like a pass-out, but nobody can rouse
  it. It comes round on its own once healed (dressed, lying on the floor at
  half a bunk's rate) to `bleed-wake` (40%) and not bleeding. Until then it
  goes hungry on the floor, unless an EMT drags it to a cot.
- A death from blood loss is "bled to death", in the log and the graveyard.

It stays off by default: see the measurements under *As built*.

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

## As built (A1, A2)

Where the build departed from the plan above, and why:

- **No `casualty / present` percept.** Eligibility is read from the casualty
  index the way threats are read from the alien index; seeing a collapse
  (`witness-collapse`) and coming across one (`noticeCasualty`) make the
  arbiter look. See *Noticing a casualty*.
- **`Peril` is read on demand**, not cached on change.
- **The roused effect was not optional.** Without it the first build roused
  one colonist 14 times in 250 ticks, each time to collapse again nine ticks
  later, until it starved on the floor.
- **Guilt's tiers are two reactions** (`guilt`, `guilt-light`), so their
  weights are data; the Callous kin exception is code.
- **Aid off is the old game.** `aid: false` with `helpless-prey-discount: 0`
  reproduces every golden run exactly: the compassion stream moves nothing
  else. With the defaults, the goldens were re-pinned.

### What it does in a run

The scenario that [pricing.md](./pricing.md) found starvation hiding in:
100 colonists on a 300×150 map, seeds 1–4, to tick 30,000, totals over the
four colonies.

| | Passed out | Rescued | Starved | Killed | Alive at the end | Guilt (light) | Grief |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Aid off | 792 | 0 | 128 | 19 | 253 | 0 | 0 |
| Aid on | 1,274 | 1,075 | 40 | 1 | 359 | 5 (13) | 6 |

Aid cuts starvation by two thirds, and every one of the 128 starvations with
it off was a colonist who died where it lay. More pass-outs with aid on is a
survivor effect: colonists who would have starved live to be tired again.
Guilt is rare (18 colonists in four colonies of 100 over the whole run),
which is about right for something that should be remembered.

The hunting preference, measured with more aliens (30 colonists on 160×80,
12 aliens, seeds 1–8, to tick 20,000):

| | Starved | Killed | Alive |
| --- | --- | --- | --- |
| Aid off | 8 | 97 | 135 |
| Aid off, hunters prefer the helpless | 9 | 96 | 135 |
| Aid on (with or without the preference) | 10 | 82 | 148 |

With aid on, casualties are brought round within a few ticks, so a hunter
almost never finds one: the preference changed no outcome in these runs.
With aid off, it changes which runs play out but not how many die. It is a
story rule more than a balance one for now; it will matter more once
wounds keep colonists down (A3, A5). Aid itself saves colonists from aliens
too: a colonist brought round can run.

## As built (A3–A5)

Where the build departed from the plan, and why:

- **Walking wounded are casualties**, and a colonist can dress its own
  wounds at half speed: a dressing is how a wound starts to heal, and the
  aid focus is how a dressing happens. Guilt, though, is only for a
  casualty that died lying helpless: a colonist killed in a fight did not
  die for want of a dressing, so nobody who passed its graze is blamed.
- **Only what a helper can do is offered** (`canHelp`). Without it a
  bystander was offered a colonist down from blood loss whose wounds were
  dressed, could do nothing, finished, and was offered it again.
- **Walking wounded heal in their own bunks.** Cots are for casualties an
  EMT brings; the plan's "seek a cot like a bed" would have needed a second
  sleep facility in the facility-choice code.
- **Dragging is only for a colonist down from blood loss**, so with
  bleeding off an EMT dresses and rouses but never drags, and the infirmary
  is built only while there is an EMT detail.
- **A cot claimed for a casualty stays free for it.** The first build
  counted it as taken, so an EMT halfway through a drag, thinking again,
  found no free cot, dropped the drag and freed it, took it up again, and
  so on, two tiles from the cot.
- **A passed-out colonist is never nudged.** `nudgeLoiterer` steps aside
  anyone with no job, and a colonist on the floor has none: casualties were
  slid along the floor by passers-by. That was true before aid too; with
  aid off and `helpless-prey-discount` 0, every golden run still matches.
- **No arrivals with medicine**, so no seed's arrival rolls moved; it is
  learned in play.
- **Tests pin their seed.** `DefaultConfig` seeds from the clock, so a test
  world differs every run; three runs in ten, the "stranger" beside a
  casualty was its family. `aidConfig` fixes the seed and turns family off.

### What A3–A5 do in a run

Alien-heavy colonies: 30 colonists on 160×80, 12 aliens, seeds 1–32, to
tick 20,000, totals over the 32 colonies. The modes add up: aid on means
A2 and wound care; EMTs are a detail of 3.

| | Alive at the end | Starved | Killed | Bled to death | Collapsed from blood loss | Rescues | EMT wages |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Aid off | 583 | 35 | 342 | – | – | 0 | – |
| Aid on | 604 | 21 | 335 | – | – | 375 | – |
| Aid on, bleeding | 619 | 21 | 316 | 4 | 94 | 374 | – |
| Aid on, 3 EMTs | 596 | 23 | 341 | – | – | 409 | $71,418 |
| Aid on, 3 EMTs unpaid | 613 | 26 | 321 | – | – | 493 | $0 |

Eight-seed slices of this disagree with each other (on seeds 1–8 aid on
beat aid off by 16 colonists; on seeds 9–16 it lost by 21): an alien-heavy
colony's outcome is mostly the fights, and only the 32-seed totals say much.
What they say:

- **Wound care helps a little**, about as much as aid alone did here.
- **Bleeding kills no more colonists.** Four bled to death in 32 colonies,
  94 collapsed and were seen to, and fewer were killed. That is the bar this
  doc set for turning it on by default. It stays off until the call is
  made, because it also changes the game's feel: fights leave colonists
  down, not just hurt.
- **Wound care keeps what aid won in a hungry colony.** The A2 scarcity
  scenario (100 colonists, 300×150, seeds 1–4, to tick 30,000) rerun with
  A3: aid off 136 starved and 251 alive, aid on 36 and 349 (A2 alone: 40
  and 359), bleeding on 39 and 347 with three collapses. Few wounds, little
  change.
- **A paid EMT detail at a fixed $2 an hour cost more than it saved** in
  the first build: about $2,200 from each colony's treasury, and unpaid the
  same detail did better. That is what turned the wage into a market price.

### Market wages

The wage is now the player's offer and each colonist's ask decides who
takes it (see *Emergency services*). The same 32 alien-heavy colonies, with
a detail of three at each wage. "Places filled" counts EMTs on call at the
end, of 96 places; response is the average ticks from a colonist becoming a
casualty to a helper setting out to it.

| Wage offered | Places filled | Paid | Alive | Killed | Starved | EMT response | Bystander response |
| --- | --- | --- | --- | --- | --- | --- | --- |
| No detail | – | – | 604 | 335 | 21 | – | 14 |
| 0¢ | 0 | $0 | 598 | 339 | 23 | – | 14 |
| 25¢ | 22 | $3,912 | 582 | 348 | 30 | 2 | 10 |
| 50¢ | 34 | $11,375 | 594 | 345 | 21 | 3 | 8 |
| $1 | 61 | $30,614 | 595 | 336 | 29 | 2 | 7 |
| $2 | 75 | $70,932 | 651 | 293 | 16 | 2 | 1 |

- **The market does what a market does.** At 25¢, a little over the
  default ask, fewer than a quarter of the places are filled by the end:
  as the colony measures how much of an EMT's time calls take, and as
  colonists earn more at their trades, their asks rise past the offer and
  they quit. Places fill as the wage rises.
- **Distance is not the problem.** An on-call EMT sets out to a casualty
  within two or three ticks of it going down, wherever it is working;
  bystanders, when they are the ones to go, take 7 to 14. Being far away
  costs an EMT walking time after it sets out, which this does not measure.
- **Survival barely moves.** Between no detail and $1 an hour every result
  is within the spread 8-seed slices showed. Only $2 stands out, and the
  first build's run at a fixed $2 (596 alive) differed from it only in
  colonists being able to quit, so it is not safe to read it as the price
  of survival. What an EMT adds over a bystander here is speed and reach,
  and in colonies where bystanders already reach most casualties, that is
  worth little. With bleeding on, where a casualty has a clock and only an
  EMT can drag, the answer may differ; it is not measured yet.
- **With bleeding on, the same.** The same colonies with `bleeding` on: no
  detail 619 alive (4 bled to death), 25¢ 571 (6; 17 places filled at the
  end), $1 628 (4; 60), $2 625 (2; 69). EMTs set out within one or two
  ticks. Level from $1 up; 25¢ worst, as it was without bleeding. A guess,
  not yet tested: at 25¢ the detail fills early, which commissions the
  infirmary (every colony built its cots), then empties as asks rise past
  the offer, so the colony pays for the rooms and gets little coverage.
- **The infirmary follows the detail, not its staff.** At 0¢ nobody took a
  place, and every colony still built its cots. That is intended: the
  infirmary is built for the detail the player sets, not for whoever the
  wage has drawn so far, so a player who raises the offer finds the cots
  already standing.

### Paying by the call

The same 32 alien-heavy colonies with a detail of three and no retainer,
paid only by the call (with `emt-call-rate` 20 as the prior). "Paid" is
everything the colony paid its EMTs.

| Fee per call | Places filled (of 96, at the end) | Calls EMTs answered | Paid | Alive |
| --- | --- | --- | --- | --- |
| $1, $3, $5 | 0 | 0 | $0 | 598 |
| $10 | 8 | 324 | $1,970 | 622 |
| $20 | 5 | 380 | $4,160 | 596 |
| 25¢ an hour and $2 a call | 28 | 501 | $4,630 | 581 |

- **Below about $10 a call nobody comes.** At 20 calls per thousand hours,
  $5 a call is worth 10¢ an hour against an ask of about 18¢.
- **Above it the detail staffs, answers, and thins out.** Each colony
  measures fewer calls than the prior reckoned, so asks rise again and
  most EMTs leave by the end; those who stay are the cheapest.
- **It is cheap.** At $10 a call coverage cost about $62 a colony over the
  run, against about $2,200 at the first build's fixed $2 an hour, with
  survival as good as any measured. Survival differences are still within
  the noise.

### Pay packages

The same 32 alien-heavy colonies, a detail of three, three packages of
roughly comparable worth. EMT-hours are sampled each hour, by the on-call
EMTs' work ethic.

| Package | EMT-hours | Industrious | Lazy | Paid | Alive |
| --- | --- | --- | --- | --- | --- |
| $1 an hour | 30,866 | 14% | 17% | $29,565 | 575 |
| $15 a call | 13,335 | 21% | 9% | $3,345 | 650 |
| 50¢ an hour and $8 a call | 26,530 | 15% | 16% | $14,509 | 618 |

- **The structure picks the people.** Paid by the call, the detail is half
  as lazy and half again as industrious as on a wage; the mix sits with the
  wage, since its wage alone already covers most asks.
- **It trades coverage for cost.** By the call, the detail is on call for
  under half the hours the wage buys (fees alone cover fewer colonists'
  asks, and asks rise as the colony measures fewer calls), at about a ninth
  of the cost.
- **Survival leans the same way but is not settled.** The fee-only colonies
  did best, but by about as much as 32-seed runs of the same package have
  disagreed with each other before.

## Open questions

1. **Response time with working EMTs.** Is the time from collapse to first
   aid good enough without any pull toward the infirmary? Measure it before
   adding one.
2. **Which existing species traits imply `PreyHelpless`?** This only matters
   if the per-species field is derived rather than set in Creature Lab.
3. **Is guilt lopsided in big colonies?** With 2000 colonists, a death in a
   busy corridor could add up to 16 guilty memories. The cap bounds it, but
   whether 16 is the right number should come from a run.
4. **Bleeding on by default?** It met the bar this doc set (no more deaths
   over 32 seeds); turning it on is a call about how fights should feel.
5. **What is an EMT worth?** Wages are now the player's offer and the
   colonists' asks (*Market wages*). In alien-heavy colonies without
   bleeding or with it, a detail barely changes survival from $1 an hour
   up, and a detail priced too low to stay staffed does worse than none,
   perhaps because its infirmary is built either way.
6. **Cost at scale.** Walking wounded make the casualty index bigger in a
   big fight, and `noticeCasualty` walks it for every colonist each turn
   while it is not empty. Fine at tens of casualties; worth a benchmark
   with a 2000-colonist battle before it is a problem.

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

# Blood on colonists

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists carry the colony's violence on their clothes. Standing beside a
messy kill, being hit, bleeding, and scooping gore off the floor each add
blood to a colonist's jumpsuit, and it fades over the following hours. The
browser's paper dolls draw it as flecks, then spatter, then a soaking
([body-sprites.md](./body-sprites.md)). Nothing in the simulation reads it:
it is how the colony looks, not how it works.

## Source

- [`internal/sim/blood.go`](../internal/sim/blood.go): `maxBlood`,
  `BloodTier`, `Entity.bloody`, `World.splatter`, `World.fadeBlood`.
- [`internal/sim/blood_test.go`](../internal/sim/blood_test.go): who gets
  splattered, the cap, the fade and its restart, the tiers.
- The sources, each one line: `addGore` in
  [`world.go`](../internal/sim/world.go) (`splatter`), `Entity.wound` and
  `bleed` in [`wounds.go`](../internal/sim/wounds.go), `gatherRefuse` in
  [`cleaning.go`](../internal/sim/cleaning.go). `colonistTurn` in
  [`systems.go`](../internal/sim/systems.go) calls `fadeBlood`.
- [`internal/sim/config.go`](../internal/sim/config.go):
  `BloodSplashRadius`, `BloodFadeHours`.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go):
  `EntityView.Blood`. [`internal/glyphs/dolls.go`](../internal/glyphs/dolls.go)
  `DollMarks` puts its tier on the wire.

## How it works

`Entity.blood` runs from 0 to `maxBlood` (6). What adds to it:

| Event | Who | Blood |
| --- | --- | --- |
| gore is made on a tile (`addGore`: a bite, a gunshot, a stomp, a fall) | every living colonist within `blood-splash-radius` (1) of it, on the same level | +2 |
| a hit opens a wound (`Entity.wound`) | the colonist hit | +1 |
| an open wound bleeds a point of HP (`bleed`) | the bleeding colonist | +1 per point |
| gathering refuse (`gatherRefuse`) | the cleaner | +1 per load |

Every source goes through `bloody`, which caps the level and restarts the
fade clock. `fadeBlood` runs every colonist turn, before anything can return
early, and takes a step off once `blood-fade-hours` of in-game hours have
passed since the last step or the last fresh blood. From soaked, that is 18
in-game hours at the defaults (six steps of three hours).

`BloodTier` buckets the level for drawing: 0 clean, 1-2 flecked, 3-4
spattered, 5-6 soaked. Only the tier goes on the wire.

## Why it is this way

- **Cosmetic, on purpose.** A blood-soaked colonist could plausibly upset the
  people around them, or need a wash. Both are good ideas for later, but each
  is a gameplay change with its own balance questions. Keeping blood out of
  every decision means adding it changed no seed's outcome.
- **No randomness.** Who gets splattered is everyone within a radius, and the
  fade is an exact clock, so blood needs neither random stream and cannot
  shift one. `splatter` walks `colonistsWithin`, which is in ID order, though
  the order does not matter here: every colonist gets the same amount.
- **A clock per colonist, not a global tick.** Fading on a shared schedule
  (every N ticks of world time) would wipe a step off someone bloodied a
  moment ago. Restarting the clock on fresh blood means a colonist who keeps
  getting splattered stays bloody.
- **The fade lives in `colonistTurn`, beside `healWounds`.** It has to run
  every tick for every colonist, including the cognition fast path, which
  returns early for a resting colonist.
- **Gore is the splash point, not each death site.** Every messy death
  already calls `addGore`, so one line there covers bites, shots, stomps and
  falls, and any future messy death gets it for free.

## Extending it

- **Washing.** A shower or washroom facility could zero `blood` (and give the
  sanitation drive something to do). Clear it in one place, through a helper
  beside `bloody`.
- **Reactions.** A colonist seeing a soaked colleague could be a perception
  in `cognition.yaml`, like `saw-gore`. That makes blood affect behaviour, so
  it needs balance runs and a note here.
- **Other kinds.** `bloody` ignores everything but colonists, because nothing
  draws an alien's clothes. Widen it only along with a renderer that shows it.

## Related

- [body-sprites.md](./body-sprites.md): how the dolls draw blood and wounds.
- [combat.md](./combat.md), [sanitation.md](./sanitation.md): gore, and who cleans it.
- [emergency-services.md](./emergency-services.md): wounds, dressing and bleeding.

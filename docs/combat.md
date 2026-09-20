# Combat

> Part of the [mars-sim documentation](./README.md).

## What it is

Colonists can now fight back against aliens. The colony ship arrives with a
pistol and a shotgun (`Config.StartPistols`/`StartShotguns`); a colonist
carrying one stands its ground and shoots an alien that gets close instead of
only fleeing. Damage — from a bite or a gunshot — lands on one of six body
parts rather than a shared HP pool, so a wound can be a survivable graze or an
outright kill depending on where it lands. Violent deaths (a gunned-down
alien, a bitten colonist, a stomped mouse) leave gore behind on the tile and a
frozen record in the graveyard, so the roster can show what happened to
something after the fact. A death nothing eats also leaves a body on the tile
for someone to haul away — that half belongs to
[sanitation.md](./sanitation.md). Human-vs-human violence and weapon skill are
explicitly out of scope for this pass; mouse-killing (the existing
stomp/pounce mechanics) is unchanged except for the mess and the record it
now leaves.

## Source

- [`internal/sim/combat.go`](../internal/sim/combat.go) — hit location, damage
  application, weapon stats, `fightAlien`, `shoot`, colony-ship equipping.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `BodyPart`,
  `bodyPartWeight`, `distributeBodyParts`, `Entity.Parts`, `Entity.hasParts`,
  `Entity.Alive`.
- [`internal/sim/inventory.go`](../internal/sim/inventory.go) — the `Pistol`/
  `Shotgun` item kinds and `bestWeapon`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `colonistTurn`'s
  survival branch (fight vs. flee), `bite` (now body-part aware), `stomp`
  (now leaves gore).
- [`internal/sim/world.go`](../internal/sim/world.go) — `Tile.Gore`,
  `World.addGore`, `World.addCorpse`, `World.remove` (the graveyard funnel),
  `World.graveyard`, `World.deceasedColonists` (the permanent archive).
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) — `EntityView`'s
  `Dead`/`DiedTick`/`Cause` fields, `entityView`, `Snapshot.Graveyard`,
  `Snapshot.Deceased`.
- [`internal/sim/config.go`](../internal/sim/config.go) — weapon, starting
  equipment, and `GraveyardSize` tunables.
- [`internal/sim/combat_test.go`](../internal/sim/combat_test.go),
  [`internal/sim/graveyard_test.go`](../internal/sim/graveyard_test.go) — the
  tests that pin this behavior.
- [`internal/ui/tui/glyphs.go`](../internal/ui/tui/glyphs.go) — the fighting
  and gore glyphs.
- [`internal/ui/tui/render_roster.go`](../internal/ui/tui/render_roster.go) —
  the roster's per-body-part wound line and the dead/non-human filtered list;
  see [frontend-tui.md](./frontend-tui.md) for the UI side of the graveyard.

## How it works

### Body parts

`BodyPart` enumerates six base parts: `Head`, `Torso` (carries the vital
organs), `LeftArm`, `RightArm`, `LeftLeg`, `RightLeg` — everything below the
`numBaseBodyParts` marker. Above it sit the **mutant** parts (`ThirdArm`,
`ExtraEye`, `Tail`, `VestigialTwin`), which nobody is born with and which
uranium exposure grows during play; see [mutation.md](./mutation.md). Only
`Colonist` and `Alien` track parts at all (`Entity.hasParts`) — cats and mice
are still one-shot kills (pounce, stomp) regardless of HP, which is the
"human v mouse is fine as is" baseline the combat rework deliberately left
alone.

Which parts an individual entity has is per-entity data, not a property of the
enum: `Entity.MaxParts[p] > 0` (`Entity.hasPart`) is the test. That is also
what keeps "never grown" distinct from "destroyed" — a blown-off limb has
`Parts` zero but `MaxParts` intact.

`bodyPartWeight` gives each part a share (out of 100) of an entity's `MaxHP`:
Head 15, Torso 35, each arm 12, each leg 13. `distributeBodyParts(maxHP)`
turns that into a `[numBodyParts]int` of starting HP per base part, crediting
any rounding remainder to the torso so the parts always sum to exactly
`maxHP`. `newEntity` calls it once at spawn for anything with `hasParts()`,
storing the result as both `Parts` and `MaxParts`. Mutant parts have weights
of their own, deliberately *outside* that hundred: growing one adds its share
on top of the existing body rather than thinning the parts already there.

After spawn a body is no longer a pure function of its `MaxHP` tunable:
mutation resizes a colonist, and `scaleBody` scales `MaxHP` and every part by
the ratio their height changed by, so a ten-foot colonist is a bigger target
pool than a two-foot one all the way down to the individual limb (see
[mutation.md](./mutation.md)).

`Entity.Alive()` is no longer just `HP > 0`: for a body-part entity it also
requires `Parts[Head] > 0 && Parts[Torso] > 0`. A called shot to a vital part
kills outright even if the aggregate HP pool has plenty left — the whole
point of tracking parts instead of one number. A destroyed limb
(`Parts[LeftArm] == 0`, say) is not fatal by itself; today it has no further
gameplay effect (no aim penalty, no bleed-out) beyond being visible in the
roster — see Extending it.

### Landing a hit

`rollHit(target)` picks the body part an attack lands on, weighted by the same
`bodyPartWeight` table (the torso is the biggest target, so it's hit most
often) over the parts `target` actually has. Weighting over the entity's own
anatomy rather than the whole enum is what lets a mutation matter in combat: a
grown third arm is one more place to be hit, which also thins the odds that
any single hit finds a vital part. It draws from `w.rng`, the simulation
stream, never `w.prng` — which part gets hit decides who lives, so it has to
stay deterministic for a given seed (see `AGENTS.md`).

`applyDamage(target, part, dmg)` subtracts `dmg` from that part (floored at
zero) *and* from the entity's aggregate `HP` (also floored at zero, and still
what starvation drains — see [needs.md](./needs.md)), then returns whether
the hit was fatal by `Entity.Alive()`'s rule. Every damage source funnels
through it: `bite` and `shoot` both call `rollHit` then `applyDamage`.

### Weapons

Two `ItemKind`s, `Pistol` and `Shotgun`, live in a colonist's ordinary
`Inventory` (see [inventory.md](./inventory.md)) — there is no separate
equipment slot. `bestWeapon(inv)` scans it and returns the best one carried
(`Shotgun` beats `Pistol`); a colonist with neither is unarmed. `weaponStats`
resolves an `ItemKind` to `{damage, rng, fireRest}` from `Config`. Shotgun:
more damage, shorter range, slower to fire again. Pistol: the opposite
trade-off. There is no ammo and no reload beyond the `fireRest` cooldown —
deliberately, to keep this pass to "does combat work at all"; see Extending
it.

### Fight or flee

`colonistTurn`'s survival branch (`systems.go`) used to always flee a nearby
alien. Now: if `bestWeapon(e.Inventory)` finds a weapon, the colonist calls
`fightAlien` instead of fleeing; unarmed colonists flee exactly as before —
arming the colony ship changes nothing about colonists who never picked up a
gun.

`fightAlien` is deliberately simple: if the alien is farther than the
weapon's range, close the distance with `travelTo` (the same pathfinding
every job uses); once in range, hold position (`State = Fighting`) and fire
when the reload cooldown (`Entity.Cooldown`, the same field predators use to
pace their attacks) allows. It does not try to hold at exactly the weapon's
range — the alien is hunting independently and will keep closing to bite
range on its own turns regardless of what the colonist does, so a gunfight
often ends up at point-blank range anyway. The gun does not guarantee a
colonist's survival; it gives them a chance they didn't have before, and
losing colonists to a well-timed bite is expected, not a bug (see the
isolation technique `TestArmedColonistKillsAlien` uses to test the killing
mechanism separately from that balance question).

`shoot` fires one shot: `rollHit`, `applyDamage`, and either gore + `remove`
+ log/memories on a kill, or a "shot the alien in the X" memory and a
generic log line otherwise. `bite` was reworked the same way instead of
subtracting a flat `AlienDamage` from `prey.HP`.

### The colony ship's starting equipment

`generate()` (`worldgen.go`) spawns `StartColonists` colonists onto a
shuffled list of floor tiles — already a random draw of who lands where —
then calls `equipColonyShip(colonists, cfg)`, which hands out up to
`StartShotguns` shotguns and `StartPistols` pistols, one per colonist, to
distinct colonists in that same order. Defaults are one of each
(`DefaultConfig`); both are ordinary CLI flags (`-pistols`, `-shotguns`) like
every other tunable (see [configuration.md](./configuration.md)). There is no
in-game way to pick up, drop, or transfer a weapon after spawn — whoever the
colony ship armed is who stays armed.

### Gore

`Tile` gained a `Gore int` field alongside `Terrain` — never consulted by
`Walkable` or anything else, and not reset when a tile is *dug*, so a stain
outlives the floor being mined around it. (It was originally never reset at
all; raising a structure over a tile now clears it, because refuse sealed
under a wall could never be hauled away — see
[sanitation.md](./sanitation.md).) Gore is no longer purely cosmetic either:
it is half of what the cleaning job exists to remove.
`World.addGore(p)` bumps it, capped at `maxGore` (3; the cap just
stops the counter climbing forever, since the renderer today draws one
splatter glyph for any `Gore > 0` regardless of count — see Extending it).
Three call sites splatter: a fatal `bite`, a killing `shoot`, and every
`stomp` (a mouse is always fatal to crush, so it always leaves a mark). A
cat's `pounce` does not — the user's ask was specifically "stomping a mouse
should leave a mess," and a cat catching its natural prey reads as predation
rather than the same kind of violence. The same call sites decide whether a
*body* is left too (`addCorpse`): `shoot` and `stomp` leave one, while `bite`
and `pounce` — where the remains are eaten — leave only the stains.

`tileGlyph` (`glyphs.go`) draws the gore glyph in place of bare terrain when
`Gore > 0` (a body on the same tile outranks it, and both outrank the rock's
composition); `renderMap` (`view.go`) calls it via the new `Snapshot.TileAt`
(alongside the existing `TerrainAt`) for any tile with no entity standing on
it. Gore isn't just cosmetic to a colonist, either — coming within
`Config.GoreSightRadius` of a gored tile is a `LifeEvent` (`EvtSawGore`) that
lowers charge and grip, amplified for a `TraitTidy` colonist; see
[memories.md](./memories.md).

### The graveyard

Every death used to just erase the entity — `World.remove(id)` deleted it and
that was that, with only a log line as evidence. `remove` now takes a
`cause string` and, before deleting anything, freezes the entity into
`World.graveyard` via `entityView(e, nil, false)` (the same builder
`snapshot()` uses for living entities) with `Dead`, `DiedTick`, and `Cause`
set. `cause` is a short player-facing phrase built at the call site, where
the context (who did it, with what) is available — `"starved"`,
`"crushed by Zoe Vargas"`, `"devoured by an alien"`, `"caught by a cat"`,
`"shot by Zoe Vargas with a shotgun"`. Every one of the six places an entity
dies (colonist/mouse starvation, `stomp`, fatal `bite`, `pounce`, fatal
`shoot`) is a call to `remove`, so this one funnel is the whole feature.

The graveyard is capped at `Config.GraveyardSize` (default 50; 0 disables
tracking, and `remove` skips the freeze entirely rather than appending to and
immediately trimming an always-empty slice), dropping the oldest entries
first — a `stomp`/`pounce`/kill flood shouldn't grow it without bound.
`Snapshot.Graveyard` is a fresh copy of it on every frame, kept separate from
`Snapshot.Entities` (a dead entity is not a still-simulated thing standing on
a tile — its frozen `Pos` may not even be walkable anymore, or something
else may be standing there now). `entityView`'s `full` parameter is what
keeps a graveyard record cheap and honest: passing `false` skips computing
`Relations`/`Affinities`, which only make sense for a colonist among living kin.
Stored charge, grip, and the final cached mood word are frozen with the body.

The [frontend-tui.md](./frontend-tui.md) roster is what actually surfaces
this — the "dead" filter toggle and the per-entry cause of death.

### The deceased archive

The graveyard's bound is right for a kill flood of mice, but wrong for a
colonist: once a colonist's entry aged out of a 50-slot window, they were
gone from every by-ID lookup — a surviving relative's family tree lost them,
the roster couldn't name them, and their frozen inventory became
unreachable, even though the colonist's identity (their `EntityID`, their
node in the family tree) still meant something to the rest of the colony.
`World.remove` addresses this with a second, colonist-only record:
`World.deceasedColonists map[EntityID]EntityView`, written unconditionally
(not gated by `Config.GraveyardSize`) whenever `e.Kind == Colonist`, and
never trimmed. `Snapshot.Deceased` exposes a copy of it every frame.

Two things make a colonist's archived record more complete than a graveyard
entry: it is built with `entityView(e, w.cachedKinChildren(), true)` (`full`
true) rather than `false`, so `Relations` and `Affinities` are computed and
frozen as of the moment of death instead of left empty; and the kin node
`remove` leaves behind (see the family tree docs) keeps pointing at the dead
colonist's `EntityID` rather than going blank, so `relativesOf` keeps
returning a `Relation` for them to every surviving relative who queries it —
resolvable back to a name and the rest of their record through
`Snapshot.Deceased`. The roster's "dead" filter reads dead colonists from
here instead of `Graveyard` for exactly that reason: a colonist who died
outside the graveyard's recent window should still show up.

Unbounded is safe here in a way it would not be for `graveyard`: a colony's
population is small and dying doesn't create more of it, so
`deceasedColonists` can only ever grow to the number of colonists who ever
existed. Mice/cats/aliens stay graveyard-only, bounded, and without
`Relations`/`Affinities` — they have no family tree, and a horde of them
dying repeatedly is exactly the kill-flood case `GraveyardSize` exists to
cap.

## Why it is this way

- **Body parts instead of a bigger HP number** is what "pretty specific
  tracking of bodily harm" asked for: a headshot should be able to kill in
  one hit even at full aggregate HP, and a called shot to a limb should not
  be a death sentence. Splitting `MaxHP` by weight (rather than giving every
  part a separate, hand-tuned max) means every creature's parts automatically
  scale with its `MaxHP` tunable, and `distributeBodyParts` summing exactly
  to `maxHP` keeps that invariant checkable (`TestBodyPartsSumToMaxHP`).
- **HP stays the aggregate pool starvation drains** rather than localizing
  hunger damage to a part. Starvation is a systemic, whole-body decline, not
  a wound; conflating the two would have meant deciding which part
  malnutrition eats first for no real gameplay benefit.
- **Guns are ranged, bites are not** — the asymmetry is deliberate. If a
  weapon only worked at bite range there would be no reason to prefer it over
  running, since the existing flee mechanic already works fine for unarmed
  colonists. Range is what makes carrying a weapon a different, better
  option than fleeing.
- **No weapon skill, no ammo, no reload, no aim penalty from a destroyed
  limb** are explicit, named exclusions from the initial ask. Each is a
  reasonable next system, but bolting all of them on before the basic
  "colonists can shoot at aliens" loop was proven out would have made this
  change much larger and harder to review for not much more insight into
  whether the mechanic is fun.
- **Cosmetic, uncapped-in-practice gore** matched "we'll be pretty limited on
  what we can display": a single splatter glyph is the right amount of
  detail for a two-cell tile, and gore fading or being cleanable was called a
  reasonable follow-up rather than a requirement of the initial ask. Cleanable
  is what it became — see [sanitation.md](./sanitation.md).
- **Human-vs-human and mouse combat untouched** were explicit descopes: the
  request was about killing the alien, not a colonist-vs-colonist system, and
  the existing stomp/pounce mechanics for mice already work and needed only
  the gore hook, not a rework.
- **A frozen `EntityView` in a graveyard slice, not a corpse in the world**:
  keeping the dead entity around as a real, positioned `*Entity` would have
  meant teaching occupancy, the spatial index, and every "nearest living
  thing" query to ignore it — a lot of surface area for something that is
  purely for a player to look at afterward. A frozen read-only record costs
  none of that, at the price of not being a thing another system could ever
  interact with (no looting a corpse, no it blocking a tile) — a fair trade
  for what was asked, a way to review deaths, not a new interactable object.
- **A second, unbounded `deceasedColonists` map instead of just dropping
  `GraveyardSize`'s bound**: the bound is load-bearing for mice/cats/aliens
  (a kill flood must not grow the graveyard without limit), but colonists
  need durable by-ID lookups precisely because other live state (a surviving
  relative's family tree) keeps referencing their `EntityID` forever.
  Keeping these as two structures rather than one unbounded-or-bounded knob
  means the mundane case (most deaths, most kinds) stays exactly as cheap and
  bounded as before, and only colonists — whose population is naturally
  capped by the colony, not by combat volume — pay for permanence.
- **`remove` takes a cause string instead of inferring one from `w.log`'s
  last line**: the call site already knows exactly who or what did it and
  with what; reverse-engineering that from a log message would be both
  redundant and brittle to a wording change.

## Extending it

- **A new weapon**: add an `ItemKind`, a case in `weaponStats`, a
  `Config`/`DefaultConfig`/flag triple for its stats (see
  [configuration.md](./configuration.md)), and update `bestWeapon`'s ranking
  if it should ever be preferred over the shotgun.
- **Weapon skill**: the natural home is a per-colonist accuracy or damage
  multiplier read in `shoot`, likely alongside the personality-trait pattern
  in [personality.md](./personality.md) rather than a new subsystem.
- **Ammo/reloading**: give colonists a magazine count (probably another
  `Inventory` item, or a new field) and have `fightAlien` fall back to
  fleeing when a weapon it's carrying is out of ammo.
- **Limb injuries with teeth**: a destroyed limb could gate behavior (a ruined
  leg slows movement, a ruined arm can't hold a weapon) — `Entity.Parts` is
  already there to read; the missing piece is wiring a check into the
  relevant turn/travel code.
- **Equipping/dropping weapons**: today a weapon is only ever assigned at
  worldgen. Letting a colonist pick one up or hand it off would need a new
  job or command (see [architecture.md](./architecture.md) for how commands
  reach the engine) plus a floor-item concept the world doesn't have yet.
- **Human-vs-human combat**: explicitly out of scope for this change; if it's
  ever wanted, `applyDamage`/`rollHit` are already violence-source-agnostic,
  but the social/gameplay implications (rivalries, justice, motive) are a
  much bigger design question than the mechanical one.
- **Graduated or fading gore**: `Tile.Gore` is already a count, not a bool
  (capped at `maxGore`); a renderer that picks a glyph by intensity, or a
  system that decays it over time, only has to read/write that one field.
- **A real corpse** now exists, but not as this doc once predicted: a body is
  `Tile.Corpses`, a counter beside `Tile.Gore`, rather than a new non-acting
  `Kind`. An entity would have taken a tile in the occupancy index and walled
  off the spot where anything died. Lootable or decomposing remains would
  build on that counter; see [sanitation.md](./sanitation.md).

## Related

- [entities-and-ai.md](./entities-and-ai.md) — the alien's hunt-and-bite
  behavior this plugs into, and the colonist survival priority it changes.
- [inventory.md](./inventory.md) — the item-stack machinery weapons reuse.
- [configuration.md](./configuration.md) — how weapon/equipment tunables
  become CLI flags.
- [needs.md](./needs.md) — starvation, the other thing that drains HP.
- [memories.md](./memories.md) — the life events and affect vectors that
  bite/stomp/pounce/shoot and gore sightings feed.
- [frontend-tui.md](./frontend-tui.md) — the fighting glyph, the gore glyph,
  and the roster's dead/non-human filter and wound line.

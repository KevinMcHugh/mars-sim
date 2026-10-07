# Lore

> Part of the [mars-sim documentation](./README.md).

## What it is

Lore is the start of a layer describing the world beyond the colony itself —
what's out there, not just what's been dug out. The first (and so far only)
piece of it: every world rolls its own roster of alien species during
worldgen (one by default, more with `alien-species-count`) — each with a
build, a name colonists reach for instead of "alien," and a temperament that
decides whether it fights at all. A Friendly species never starts a fight, a
Cautious one only reacts once a colonist gets close, and a Hostile one hunts
the colony the way every alien always did. Every individual `Alien` belongs
to one rolled species, which fights the way its body allows — biting,
raking with claws, thrashing a tail, or strangling — and hits harder or
softer depending on how big it rolled. What a species can be *named* is itself
configurable data — a condition-gated pool of names, edited in YAML — rather
than a hardcoded list. Each species also gets a scientific name
(*Pseudursus ares*), covered in [alien-taxonomy.md](./alien-taxonomy.md).

## Source

- [`internal/sim/lore.go`](../internal/sim/lore.go) — `AlienSpecies`,
  `AlienTemperament`, `AlienSkin`, `AttackMode`/`AttackSet`/`rollAttackModes`, `AlienSizeTier`, `rollAlienSpecies`,
  `rollAlienSpeciesRoster`, `speciesDamage`, `scaledByTemperament`, and the
  `World.alienSpeciesFor`/`alienNounFor`/`alienPluralFor` helpers.
- [`internal/sim/alien_names.go`](../internal/sim/alien_names.go) —
  `AlienNameEntry` (including its `Emoji` candidates), name groups
  (`rawAlienNameEntry`, `AlienNameForm`), `distinctAlienName`,
  `nameCondition`/`intCondition` (the boolean condition tree),
  `pickAlienName`, `LoadAlienNames`, `defaultAlienNames` (the `//go:embed`ded
  built-in pool).
- [`internal/sim/alien-names.yaml`](../internal/sim/alien-names.yaml) — the
  built-in name pool, compiled into the binary, with the curated emoji
  candidates each entry can roll.
- [`alien-names.yaml.example`](../alien-names.yaml.example) — a commented,
  standalone example a player can copy and pass to `-alien-names`.
- [`internal/glyphs/glyphs.go`](../internal/glyphs/glyphs.go) — the curated
  reptile/bug/alien glyph set (`Lizard`, `Beetle`, ...) and `ForAlien`, which
  decides whether a species' rolled `Emoji` is safe to draw on the map. The
  TUI (`alienGlyph` in `internal/ui/tui/glyphs.go`) and the browser map both
  go through it.
- [`internal/sim/lore_test.go`](../internal/sim/lore_test.go),
  [`internal/sim/alien_names_test.go`](../internal/sim/alien_names_test.go) —
  determinism, invariants, temperament behavior, the naming-condition
  boolean logic, and the emoji draw.
- [`internal/ui/tui/glyphs_test.go`](../internal/ui/tui/glyphs_test.go) —
  `alienGlyph`'s registered/unregistered/empty cases, and that every emoji
  in the built-in `alien-names.yaml` is actually registered.
- [`internal/sim/world.go`](../internal/sim/world.go) — `World.alienSpecies`
  (now a roster, `[]AlienSpecies`) and where it's rolled, in `newWorld`; the
  per-`Alien` species draw in `spawn`; `Layer.exploredCount`, kept
  incrementally by `reveal`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Entity.Species`,
  the index into `World.alienSpecies` an individual alien was assigned.
- [`internal/sim/config.go`](../internal/sim/config.go) — `AlienSpeciesCount`,
  `AlienCautiousRadius`, `AlienHungerRate`, `AlienGrazeRadius`, `AlienNames`, and `AlienDamage`/`AlienBiteRest`/
  `AlienSlowness`/`AlienReferenceWeightKG` (now baselines a species scales).
- [`internal/sim/combat.go`](../internal/sim/combat.go),
  [`internal/sim/species.go`](../internal/sim/species.go) —
  `newAlienSpeciesTable`, where temperament picks each rolled species'
  behavior ladder ([species-and-behaviors.md](./species-and-behaviors.md));
  [`internal/sim/systems.go`](../internal/sim/systems.go) — `strike`,
  `alienGraze`, and where combat reads a specific alien's assigned
  species instead of a single world-wide value.
- [`internal/sim/director.go`](../internal/sim/director.go) — the
  alien-swarm occurrence's flavor line, named after whichever spawned alien
  came first.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) —
  `Snapshot.AlienSpecies` (the full roster), `EntityView.AlienSpecies` (the
  one a living alien belongs to), `Snapshot.Seed`, and `Stats.ExploredTiles`.
- [`internal/ui/tui/render_roster.go`](../internal/ui/tui/render_roster.go) —
  the roster's alien entry, reading `EntityView.AlienSpecies` directly.
- [`internal/ui/tui/render_lore.go`](../internal/ui/tui/render_lore.go) — the
  lore tab: world facts and every rolled species' full write-up.
- [`main.go`](../main.go) — `-alien-names`, loaded the same way `-director`
  loads `director.yaml`.

## How it works

### A roster of species, not one per world

`AlienSpecies` (`lore.go`) holds a build (height/weight ranges, eye count,
limb count split into arms vs. legs via `Arms`/`Legs()`, tail or not, wings or not, `Skin`,
`Color`, `Pattern`), a colloquial name (`Singular`/`Plural`, picked from a condition
pool — see below), a `Temperament`, a set of `AttackModes`, and three precomputed combat stats:
`BiteDamage`, `BiteRest`, `Slowness` (named for the bite, but they are the
baseline damage and pace of every attack mode).

`Config.AlienSpeciesCount` (default 1) says how many species a world rolls;
`rollAlienSpeciesRoster` rolls that many (clamped to at least 1) into
`World.alienSpecies`. Every `Alien` entity is assigned one at spawn time —
`Entity.Species` indexes into that slice, drawn uniformly in `World.spawn`
— so a world with more than one species can have a real mix wandering the
rock, each with its own build, name, and temperament. `alienSpeciesFor(e)`
is how every other system reads the species a specific alien belongs to.

### Hide and pattern

`Skin` is one of thirteen hides — `smooth`, `scaly`, `furry`, `armored`,
`bony`, `chitinous`, `slimy`, `rocky`, `woody`, `mossy`, `gelatinous`,
`hairy`, `feathered` — drawn uniformly. Each has its own `coveringPhrase`
("craggy gray stone", "red bark", "quivering pale jelly", "shaggy black
hair"); `feathered` is the second plural one after `scaly`, so a hostile
species' "feathers stand out" agrees its verb. `hairy` and `furry` are
deliberately both kept: fur is a dense pelt (bear, mouse), hair is long and
shaggy (yeti, mammoth), and the names and taxonomy treat them differently.
The six later hides were appended to the end of the enum and of
`alienSkins`, so every older hide keeps its numeric value. `Pattern` says how `Color`
is laid over that hide: `striped` and `spotted` are 5% each and `solid` is
the rest (`rollPattern`), so a patterned species is a genuine find rather
than a coin flip. `Color` itself is drawn uniformly from `alienColors`,
which includes `iridescent` alongside the ordinary hues. Pattern is its own field rather than more entries in
`alienColors` so a naming condition can say "anything striped" (`tiger`)
or "red and spotted" (`ladybug`) without enumerating every
color-times-pattern string; `ColorPhrase()` folds the two back together
("green-striped") for `Description()`. Both hide and pattern are plain
naming-condition leaves (`skin:`, `pattern:`) and carry no gameplay effect
today — they are flavor, like `Color`.

### Wings

`Wings` is a bool, rolled at a flat `alienWingsPercent` (20%) right after
`Pattern` and before the name, so both name pools can ask for it (`wings:`
is a naming-condition leaf alongside `tail:`). It is independent of hide on
purpose: a winged species can be a scaly *dragon*, a furry *bat*, a rocky
*gargoyle*, or a chitinous *moth*, and a feathered species without wings is
a *dodo* — tying wings to feathers would have lost every one of those.

Wings are **anatomy only**. No species flies: movement, pathing and combat
ignore the flag, and there is no wing attack mode. The description lists
"a pair of wings" with the other body parts, and a hostile species'
gait says it moves "with their wings folded" so the field notes never
promise flight the sim does not do. Flight (crossing rock or chasms, a
different `Slowness`) is the obvious follow-up; see *Extending it*.

Adding a hide or pattern shifts every later draw off the lore stream for a
given seed (the roll is uniform over the slice), so the same seed rolls a
different roster after such a change. That is fine — the stream is isolated
from the simulation stream (below) — but expect seed-pinned lore tests to
need new expectations.

### Where it rolls, and on what stream

The roster is rolled in `newWorld`, not `generate()` — a handful of tests
build a `World` with `newWorld` directly and still spawn and fight `Alien`
entities against it, and those need a valid roster too. It draws from a
dedicated stream, `newRand(cfg.Seed ^ alienLoreSeed)`, the
same pattern worldgen's own streams use (see
[worldgen-chunks.md](./worldgen-chunks.md)) and for the same two reasons:

- Not `World.prng` (the personality stream): a species' size and temperament
  are not flavor — they set actual bite damage and combat behavior — so they
  have to stay on the deterministic side of the personality/simulation split
  (see [personality.md](./personality.md) and `AGENTS.md`).
- Not `World.rng` (the simulation stream): rolling species must not perturb
  any `w.rng`-driven decision that runs afterward — where colonists and
  aliens spawn, first and foremost — so it cannot share that stream's draw
  sequence. Which *species* a given `Alien` entity is individually assigned,
  though, *does* draw from `w.rng` at spawn time: that's an ordinary
  gameplay decision, like where a colonist lands, not part of generating the
  species themselves.

### Temperament: whether a species fights at all

`AlienTemperament` is an enum, not the old continuous 0–100 aggression
score, specifically so "never initiates combat" is a real case the code has
to branch on rather than an incidental zero:

- **Friendly** never fights. Its ladder (`newAlienSpeciesTable`) has no
  `hunt` rung, so nothing ever looks for prey for it: it grazes cave scum when hungry, else wanders. (A
  colonist may still flee or fight one on its own initiative; that side of the
  interaction is explicitly unchanged for now — see Why it is this way.)
- **Cautious** does not hunt, but reacts once a colonist comes within
  `Config.AlienCautiousRadius`: its `hunt` rung finds prey with the
  radius-bounded `taggedWithin(TagColonist, radius)` instead of
  `preyInRoom(hostilePrey)`, so it
  only ever notices — and then closes in on and bites — a colonist already
  close by. Left alone, it grazes cave scum when hungry, else wanders.
- **Hostile** hunts the nearest prey anywhere it can walk to
  (`preyInRoom(hostilePrey)`: any distance, but only in its own room),
  unconditionally. Prey is a colonist, a rat, or an alien of **another
  species**; whichever is nearest, ties to the lower ID. Cautious uses the
  same room check within its radius, but only ever reacts to colonists.

A Hostile alien never hunts its own species: every alien of a species shares
its temperament, and a nest (one species, spawned together) would otherwise
eat itself on the first tick. With the default `alien-species-count: 1`,
then, a Hostile species hunts colonists and rats; alien-on-alien fights need
a roster of two or more. The victim does not fight back — a bitten Cautious
alien still reacts only to colonists, and a Friendly one never fights —
which keeps this change to the hunter's side. `strike` handles any prey
(`rollHit` falls back to the torso for a rat, which has no parts, drawing no
RNG), names it with `preyName`, and eats it on a kill: gore, no body, as
for a colonist. The colonists' perception rules match on a colonist victim,
so watching an alien eat a rat or another alien moves nobody's mood.

### What peaceful species eat: cave scum

A Hostile alien eats what it kills. Friendly and Cautious ones, which do not hunt,
**graze cave scum** instead (`alienGraze`, `systems.go`). Every alien has a
food drive growing at `alien-hunger-rate` thousandths of a point a tick (set
in `newEntity`; the other drives stay flat, as for rats). Once it passes the food drive's
`seek-at`, a Friendly alien — or a Cautious one with no colonist inside its
radius — looks for the nearest exposed scum patch within `alien-graze-radius`
that it can reach (`nearestEdible` with `grazeable`, `scavenge.go`: floor in
its own room, or a rock face that room touches), walks there, and eats one
unit, which sates it. It then rests `BiteRest`, as after a bite. With no scum
in reach, or not hungry, it wanders as before.

- Reacting beats grazing: a hungry Cautious alien with a colonist in its
  radius goes for the colonist. Grazing is what it does when left alone.
- Aliens **never starve**. Their species is not `Starves`, so `animalTurn`
  never applies drive consequences to them, and a
  grazer on a bare map is merely hungry. Starving aliens would quietly thin
  out the peaceful species on scum-poor seeds while Hostile ones (whose
  hunger is never read) lived forever — a balance change nobody asked for.
- Scum only — not bodies or gore, which rats also eat. "Peaceful species eat
  the biofilm" is the ask, and a Friendly alien picking at a colonist's
  corpse would read as anything but friendly.
- A grazer competes with the colony for scum exactly as a rat does: every
  unit it eats is one the scumhouse never sees (see
  [scumhouse.md](./scumhouse.md)). A peaceful species is not free.
- Dormant aliens (on undiscovered cavern floor) do not graze; `dormantTurn`
  runs first, and hidden scum is not exposed anyway.

`rollTemperament` makes Friendly rare (10%) and Cautious/Hostile common and
roughly even (45% each) — the "ET to Xenomorph" spread the ask described.

### Attack modes: how a species fights

`AttackMode` is one of six: **bite**, **claws** (scratching), **tail**
(thrashing), **strangle**, and two its anatomy grants rather than rolls,
**gore** and **sting** (see
[alien-lifecycles.md](./alien-lifecycles.md#features-that-fight)). Each
species rolls a set of the first four
(`rollAttackModes`, stored as the `AttackSet` bitmask `AlienSpecies.AttackModes`),
and the set has to make sense for the body (`canUse`):

| Mode | Needs | What it does |
| --- | --- | --- |
| bite | nothing — every species has a mouth | full damage to a random part |
| claws | at least one arm | full damage to a random part |
| tail | a tail | full damage to a random part |
| strangle | at least two arms (a grip) | half damage, always to the head |
| gore | horns or antlers (granted, not rolled) | scaled by horns and tines, to a random part |
| sting | a stinger (granted, not rolled) | half damage (barbed: three quarters), always to the torso |

Each allowed mode is kept on a coin flip, so two clawed, tailed species can
still fight differently; if every flip misses, the species bites. A tail-less
species never thrashes, an armless one never claws or strangles.

At each blow, `strike` (`systems.go`) picks one of the species' modes
uniformly — on the **simulation** stream (`w.rng`), because the mode decides
where the blow lands and how hard, so it is gameplay, not flavor. A species
with a single mode draws nothing extra. Strangling skips `rollHit` and goes
for the head (a rat, with no parts, falls back to `rollHit`'s torso): half
damage against a small vital part, a slower but surer kill than a bite that
might land on a limb. Everything else uses `rollHit` and full damage as the
old bite did.

Narration follows the mode, from `strikeVerbs`/`strikeTargetText`: the victim
remembers "Bitten in the arm by a grelk!", "Clawed across the torso by …",
"Lashed across the leg by a grelk's tail!", or "Half-strangled by …";
a witness "watched a grelk bite/claw/lash/throttle Ana"; a kill reads "A
grelk strangles Ana to death and devours the remains." and the graveyard
cause "strangled to death by a grelk". `Description()` says how the species
fights ("They kill by biting and raking with their claws." for a hostile
species, "Get too close and they lash out by …" for a cautious one; a
friendly one never fights, so its entry does not mention it), and the lore
tab (TUI and web) lists the modes on an `Attacks:` stat line.

The occurrence a non-fatal strike emits is still `ActionBite`, whatever the
mode. The perception and affect rules (`cognition_config.go`) key on that
action to mean "an alien hurt a colonist"; splitting it per mode would mean
duplicating every rule for no change in how anyone feels. If colonists ever
should react differently to being strangled than bitten, add actions then.

`rollAttackModes` runs after the name pick, so each species' build and name
are unchanged from before the feature; the extra lore-stream draws only shift
later species in a multi-species roster.

### Damage scales with size, not with temperament

`speciesDamage(sp, cfg)` scales `cfg.AlienDamage` by `sp`'s weight (the
midpoint of `WeightMinKG..WeightMaxKG`) relative to
`cfg.AlienReferenceWeightKG` — the weight at which a specimen deals exactly
the configured baseline. A species heavier than the reference hits harder; a
lighter one hits softer. `scaleRound` (the same rounding helper
`mutation.go`'s stature walk uses) does the arithmetic — the same
"ratio of a size" scaling that resizing a mutated colonist's body already
does (see [mutation.md](./mutation.md)).

One deliberate exception: a configured `AlienDamage` of 0 passes straight
through as 0 rather than being floored back up to 1. `combat_test.go` sets
`AlienDamage = 0` to neuter an alien's bite so it can test gunfire in
isolation; scaling must not turn that "off" switch back "on" for a
heavyweight roll.

### Temperament scales pace, not damage

`scaledByTemperament(base, t)` scales `AlienBiteRest`/`AlienSlowness`: a
Hostile species strikes and moves twice as fast as the configured baseline,
Cautious reproduces the baseline exactly, and Friendly (which never fights,
but still wanders) moves half again slower — a calmer creature, not merely
a slower killer.

Size and temperament are deliberately two separate knobs: a species being
*scary* (fast, relentless) and a species being *dangerous* (hits hard) are
different things a colony would come to know separately about its local
wildlife, and conflating them would mean a big, slow brute and a small,
frantic swarm could never both exist across different seeds.

### Naming: a condition-gated pool, not a flat table

A rolled species needs a name, and which names make sense depends on what it
turned out to look like — a six-legged armored thing reads as a "beetle," a
limbless scaly one as a "snake." `alien_names.go` expresses that as data:
`AlienNameEntry{Singular, Plural, When}`, where `When` is a `nameCondition`
— a small boolean tree over the rolled build:

```go
type nameCondition struct {
	All []nameCondition
	Any []nameCondition
	Not *nameCondition

	Temperament, Skin, Color, Pattern, Height, Weight string
	Tail, Wings                              *bool
	Legs, Arms, Limbs, Eyes                  *intCondition // {eq,gt,gte,lt,lte}
}
```

`nameCondition.matches(sp)` checks every set leaf (an unset leaf is simply
not checked, not a failure — a condition only constrains the traits it
names), ANDs the `All` list, ORs the `Any` list, and negates `Not`, and
these nest to any depth — the exact "boolean operations" the ask wanted:
`beetle` is `legs == 6 AND skin == armored`; `snake` is `limbs == 0 AND skin
== scaly`; the example file's `stalker` is `temperament == hostile AND legs
== 2 AND NOT tail`. `Height`/`Weight` read `AlienSpecies.HeightTier()`/
`WeightTier()` — the range's midpoint bucketed into `tiny/small/average/
large/huge` — so "titan" can mean "huge either way" (`any: [{height: huge},
{weight: huge}]`) without a raw centimetre or kilogram number in the
condition.

`pickAlienName(rng, sp, names, used)` collects every entry whose condition matches
the just-rolled species and draws one at random — overlapping conditions
(several names fit the same species) are normal, not an error, which is why
`alien-names.yaml`'s conditions are allowed to be loose and to overlap
freely. `rollAlienSpecies` calls it *last*, after every other trait is
rolled, since the name depends on the build, not the other way around.

### Name groups

Synonyms tend to share a condition — `reptile`, `reppy` and `rept` are all
just "scaly" — and writing each out with its own copy of the `when` and the
emoji list meant three places to edit for one idea. A **name group** lists
several names under one condition:

```yaml
- group:
    - reptile                         # plural defaults to "reptiles"
    - { name: reppy, plural: reppies }
    - rept
  emoji: ["🦎", "🐍", "🐢", "🐊", "🐉"]
  when:
    skin: scaly
```

`LoadAlienNames` expands a group in place, in file order, into one
`AlienNameEntry` per name, each with the group's `when` and `emoji`; nothing
after loading ever sees a group. It also fills in each entry's default plural.
An entry sets `name` or `group`, never both, and a group's plurals go on its
names, not on the group. `TestLoadAlienNamesExpandsGroups` pins the expansion.

### No repeated names

No two species in one roster share a name. `rollAlienSpeciesRoster` keeps a
`used` set of the lower-cased singulars it has handed out, and `pickAlienName`
drops any matching entry whose name is in it. Only when *every* matching name
is taken does it fall back to the full matching set, and then
`distinctAlienName` qualifies the result: first with the species'
`ColorPhrase()` ("green-striped grelk"), then with a number ("green-striped
grelk 2") until it is free. The built-in pool has four unconditional names, so
the fallback only shows up with an `alien-species-count` well past what a
world normally rolls, or a small `-alien-names` file.

The pool itself is `internal/sim/alien-names.yaml`, embedded into the binary
via `//go:embed` and parsed once as `defaultAlienNames()` — so the game
always has a working pool with zero configuration, including every
`sim`-package caller (tests included) that never goes through `main.go`'s
file loading at all. `Config.AlienNames` is the override: `-alien-names
somefile.yaml` (or the default `alien-names.yaml` in the working directory,
loaded the same optional way `mars-sim.yaml`/`director.yaml` are) replaces
the whole pool for that run. `rollAlienSpeciesRoster` falls back to
`defaultAlienNames()` whenever `cfg.AlienNames` is empty.

### Emoji: a shared source of truth, split at the render boundary

Each `AlienNameEntry` can also list `Emoji []string` — candidate glyphs for
that name. `pickAlienName` draws one of them the same way it draws the name
itself: independently, from the winning entry's own list, so two species
that land on the same name need not land on the same glyph. The result
lands on `AlienSpecies.Emoji`, one more plain string field alongside `Skin`
and `Color` — `lore.go`/`alien_names.go` never interpret it as anything but
data, the same way they never interpret `Skin` as anything but a word.

That YAML file is the shared source of truth the name says it is: `sim`
parses it into `AlienSpecies.Emoji`, and the TUI reads that same field back
off the `Snapshot`/`EntityView` it already gets everything else from — no
separate name-to-glyph table maintained twice. What differs is what each
side is allowed to *do* with the string. As flavor text (`RosterLabel()`,
prefixed with the emoji when one is present — `"🦎 Xeno · hostile"` — shown
in the roster and the lore tab) it is unconditionally safe: those are
variable-width lines a frontend already truncates correctly regardless of
what's in them. As a map glyph it is not: the map's tiles are a fixed two
cells each (see [terminal-cell-widths.md](./terminal-cell-widths.md)), and
nothing about an arbitrary runtime string guarantees a real terminal paints
it at the width `sim` — which has no concept of terminal cells at all —
would need it to be.

So the TUI draws it on the map only through `alienGlyph(sp)`, which is
`glyphs.ForAlien` (`internal/glyphs`, also used by the browser map). It checks
the exact string against `glyphs.All`, which the TUI's `glyphRegistry` must
cover exactly — the same vetted, width-tested, ASCII-fallback-carrying set
every other glyph on the map comes from — and falls back to the generic
`glyphAlien` for anything unregistered, `sp.Emoji == ""` included. The
registry ships a curated set of reptile/bug/alien glyphs for this
(`glyphLizard`, `glyphBeetle`, `glyphSaucer`, ...) that `internal/sim/
alien-names.yaml`'s own emoji lists draw from, so the built-in pool's
species always have a real, registered glyph to show on the map; a custom
`-alien-names` file is free to name anything as flavor text, but only shows
up on the map if it happens to spell one of those same registered symbols
exactly (see the note in `alien-names.yaml.example`).

The hide/pattern names (`bonehead`, `roach`, `slug`, `toad`, `tiger`,
`zebra`, `leopard`, `ladybug`) brought their own registered glyphs — 💀 🪳
🐌 🐸 🐅 🐯 🦓 🐆 🐞 — all single code points with no variation selector.
`bonehead` deliberately lists only 💀, not ☠️: the crossbones is U+2620 +
VS16, exactly the width-ambiguous shape the registry exists to keep off the
grid. The same rule pruned the older entries: 🐻‍❄️ (a ZWJ sequence),
⚫️/🕷️/♟️ (VS16) were dropped or swapped for single-code-point stand-ins
(⚫ 🌑 🦇), and every remaining emoji in the built-in pool is registered.

The rocky/woody/mossy/gelatinous/hairy/feathered hides and wings added their
own names and glyphs the same way: *rockling*/*pebble*/*golem*/*gargoyle*
(🪨 🗿), *stump*/*barkback*/*twig*/*treant* (🪵 🌳), *mossback*/*shambler*/
*bog thing* (🌿 🌱), *jelly*/*jiggler*/*ooze*/*jellyfish* (🍮), *shag*/
*mophead*/*yeti*/*mammoth* (🦍 🦣), *birdie*/*plume*/*dodo*/*chook*/*harpy*/
*angel* (🐦 🦜 🦉 🦤 🦅 👼), and for wings *flapper*/*bat*/*dragon*/*moth*/
*skeeter*/*griffin* (🪰 🦟 plus the existing 🦇 🦋 🐉). The obvious
jellyfish glyph 🪼 is Unicode 15, newer than most terminals' width tables,
so *jellyfish* borrows 🦑/🐙 and the gelatinous group uses 🍮 instead. The
new glyphs were appended to the end of `glyphs.All` so no existing glyph's
wire index moved.
`TestCuratedAlienEmojiAreAllRegistered` reads `internal/sim/alien-names.yaml`
itself, so adding a name whose emoji isn't in `glyphRegistry` fails the
build instead of silently rendering as 👽 on the map.

### Narration

`World.alienNounFor(e)` (`withArticle(w.alienSpeciesFor(e).Singular)`,
reusing `mutation.go`'s article helper) and `alienPluralFor(e)` are what
combat and the roster read instead of the literal word "alien" — "Killed a
gremlin with a shotgun!" instead of "Killed an alien with a shotgun!" (the gun
itself is named after its rolled make and model too — see
[arms-makers.md](./arms-makers.md)). Both
take the specific alien entity involved, not a single world-wide value, so a
world with more than one species narrates each encounter with the right
one. The director's alien-swarm log line is the one place that still picks
just one name for a whole spawn: with more than one species rolled, a swarm
can be a genuine mix, and naming it after whichever alien happened to spawn
first is a deliberate simplification for a single flavor line, not a
species-by-species breakdown. `Kind.String()` (`entity.go`) and the generic
creature-sighting line in `observeNearby` (`systems.go`) still say "alien"
deliberately — see Extending it.

`AlienSpecies.Description()` renders a short field-guide entry, written to
read like a wiki article rather than a stat block (the lore tab already lists
the raw numbers right above it). Sizes are metric with imperial in
parentheses (`1.9-3 m (6'3"-9'10")`, `35-54 kg (77-119 lb)`), and the
framing follows `Temperament`: a friendly species "interacts well with
humans", a cautious one is "skittish around humans; approach with caution",
and a hostile one is "the feared ...", hunting with its eyes and "fearsome
arms" while its hide blends into (red/orange/yellow/gray) or stands out
against the Martian rock. The phrasing is deliberately a pure function of
the species — no RNG draw — so rewording it can never shift a seed. The
helpers handle the awkward rolls the old one-line template got wrong (a
part the species has none of is simply not mentioned rather than read as "0
arms", a legless hostile that "slithers" rather than
"crawls on 0 legs", verb agreement for plural hides like scales).
`RosterLabel()` is the short form
(`"Xeno · hostile"`). Both are used in two places: the roster's alien entry
(`RosterLabel()`, in place of a colonist's pronouns/age line) and the lore
tab (both — see below).

### The lore tab

The TUI's fourth details panel (`internal/ui/tui/render_lore.go`, `tab` to
reach it — see [frontend-tui.md](./frontend-tui.md)) is where a player
actually reads all of this. Its list panel shows world facts —
`Snapshot.Width`/`Height`, `Snapshot.Seed`, and how much of the map has
been explored (`Stats.ExploredTiles`, kept incrementally the same way
`Stats.FloorDug` already is, out of `World.reveal` — see
[fog-of-war.md](./fog-of-war.md) and [world.md](./world.md)), and how many
worldgen chunks exist so far (`Stats.ChunksGenerated` of `Stats.Chunks`, see
[worldgen-chunks.md](./worldgen-chunks.md)) — above a
selectable list of `Snapshot.AlienSpecies`, each shown by `RosterLabel()`.
The detail panel shows the selected species' scientific name in italics
under its title, then lists its full build as explicit stat
lines (height/weight range, eyes, limb split, tail, wings, skin, color, bite
damage/pace, plus the color pattern) followed by `Description()`'s narrative paragraph,
word-wrapped to the panel width.

## Why it is this way

- **`sim` carries `Emoji` as an opaque string, never a "glyph."** The ask
  was explicit that `sim` shouldn't know anything about emoji, and it still
  doesn't: `AlienSpecies.Emoji` is data of exactly the same kind as `Skin`
  or `Color` — a string picked from YAML, never measured, registered, or
  validated by anything in the `sim` package. The width-safety machinery
  the map depends on (see [terminal-cell-widths.md](./terminal-cell-widths.md))
  lives entirely on the frontends' side, in `glyphs.ForAlien` (the TUI's
  `alienGlyph`), which is also the only
  place a fallback to `glyphAlien` can happen. Splitting it this way is what
  lets `sim`'s tests (and any other future consumer of a `Snapshot`) treat
  species data uniformly without either package needing to know the other's
  rules.
- **A curated registry set, not "any YAML string reaches the map."** Every
  other glyph on the map is a vetted, single-code-point,
  cross-library-width-agreed, ASCII-fallback-carrying entry in
  `glyphRegistry`, checked by a startup probe against the real terminal —
  the whole point of [terminal-cell-widths.md](./terminal-cell-widths.md) is
  that an unvetted string reaching a fixed two-cell tile is exactly the bug
  that kept recurring. A YAML file is runtime data, so nothing stops an
  emoji named there from carrying a variation selector or being one the
  probe never got to check. Rather than trust it directly, `alien-names.yaml`
  draws its candidates from a small set the TUI *also* ships
  (`glyphLizard`, `glyphBeetle`, `glyphSaucer`, ...), and `alienGlyph` checks
  the string against that same registry before ever handing it to `fitGlyph`.
  A custom `-alien-names` file that names something else still works — it's
  just flavor text until it happens to spell one of the registered symbols.
- **Two independent rolls (name, then emoji from that name's own list), not
  one combined table.** A flat `{name, emoji}` pairing would mean every
  "reptile" species looks identical on the map; drawing the emoji separately
  from the same entry's candidate list is what lets two "Reptile" species in
  different games (or even the same game, with `alien-species-count` > 1)
  come out as 🦎 and 🐍 respectively.
- **A roster (`[]AlienSpecies`), not one species per world.** The follow-up
  ask ("parameterize alien species count") turned the original one-species
  design into a genuine roster with per-entity assignment
  (`Entity.Species`). Keeping species-level data on `World`/`Snapshot` and
  only an index on each `Entity` (rather than copying the whole struct onto
  every alien) keeps an individual alien cheap while still letting every
  system resolve its specific species with one lookup
  (`alienSpeciesFor`).
- **Temperament as an enum, not a continuous score.** The old 0–100
  aggression score *could* land on "never fights" at exactly 0, but nothing
  forced any code to actually treat that as special — it was an emergent
  reading of a formula, not a case anything branched on. Naming the three
  tiers `TemperamentFriendly`/`Cautious`/`Hostile` and switching on them
  (now in `newAlienSpeciesTable`, to pick a ladder) makes "never initiates combat" something the compiler and a
  reviewer can see is handled, not something that happens to fall out of
  arithmetic.
- **Colonist-side behavior is unchanged "for now."** A colonist still
  treats *any* `Alien` — Friendly included — as a potential threat within
  `FleeRadius`, and can still flee or fight one. The ask was explicit that
  this is deliberate scope for this pass: teaching colonists to
  distinguish a docile species from a hostile one is a real feature (does a
  colony eventually stop fleeing a species it's learned is harmless?) but a
  separate one from giving aliens their own temperament in the first place.
- **Cautious's reaction radius is a new, separate config
  (`AlienCautiousRadius`) rather than reusing `FleeRadius` or
  `GoreSightRadius`.** Those two are about what a *colonist* notices; a
  Cautious alien's trigger distance is a fact about the alien, tuned
  independently — a colony's flee radius changing should not silently
  change how close you can walk past a wary species before it reacts.
- **Names as condition-gated data, not a Go switch statement.** A hardcoded
  `switch` over build traits would need a recompile for every new name or
  tweaked condition; a YAML pool edited at `internal/sim/alien-names.yaml`
  (compiled in) or swapped at runtime with `-alien-names` is what "easily
  configurable" asked for, and the same boolean-tree shape
  (`all`/`any`/`not`) as `director.yaml`'s occurrences gives it a
  transparently learnable format rather than a bespoke mini-language.
- **A struct condition tree, not a string expression parser.** "Boolean
  operations" could have meant a string DSL (`"legs == 6 && skin ==
  'armored'"`), but that needs a real parser and grammar to get right. A
  YAML-native tree of `all`/`any`/`not` plus typed leaf fields reaches the
  same expressiveness the ask's examples needed (arbitrary nesting,
  comparisons on counts) with no parser at all — just `yaml.Unmarshal` into
  Go structs — at the cost of being slightly more verbose to hand-author
  than an inline expression would be.
- **A group is shorthand, so each name in it weighs the same as a
  standalone entry.** The other reading — draw a group first, then a name in
  it — would make three synonyms count as one candidate, so folding
  `reptile`/`reppy`/`rept` into a group would have quietly made scaly
  species less likely to get a scaly name. Expanding at load keeps a group a
  pure editing convenience: the built-in pool after the change is
  byte-for-byte the pool before it, so no seed rolls a different roster.
  If a group should ever count as one candidate, that wants an explicit
  `weight`, not a change to what `group` means.
- **Uniqueness filters the draw rather than re-rolling.** Dropping taken
  names from the candidate list before the draw costs exactly the same RNG
  draws as before, and a roster whose names never collide rolls exactly what
  it did before uniqueness existed. Re-rolling until the name was new would
  have burned an unbounded number of lore-stream draws and shifted every
  later species. Qualifying by color on exhaustion, rather than allowing the
  repeat, keeps the narration unambiguous ("killed a green grelk") — two
  species with one name was the bug.
- **Height/weight as bucketed tiers for naming, not raw centimetres/
  kilograms in the condition.** The ask asked for "a height and weight enum
  for naming as well" specifically, not a numeric threshold — `tiny` through
  `huge` reads the way a colonist would actually describe a creature, and
  keeps a name's condition portable across however the roll ranges get
  retuned later (a raw `{gt: 300}` would silently stop matching anything if
  the height formula's scale ever changed).
- **The embedded default plus an optional override file, not two
  independently-maintained lists.** Baking `alien-names.yaml` into the
  binary via `go:embed` means there is exactly one source of truth for the
  built-in pool — no separate hardcoded Go literal that could drift from a
  committed YAML file the way `mars-sim.yaml` has a staleness test to guard
  against. `-alien-names` (or the default `alien-names.yaml` path, loaded
  the same optional way `director.yaml` is) is how a player changes it
  without recompiling.
- **`AlienDamage`/`AlienBiteRest`/`AlienSlowness` stay as config baselines**
  rather than being replaced outright by a species. A config tunable that a
  seed's roll could silently override would make `-alien-damage 0` (which
  `combat_test.go` relies on to isolate gunfire) stop meaning "off."
  Scaling a baseline, with an explicit zero-passthrough, keeps that
  guarantee while still letting size and temperament move the effective
  numbers per seed and per species.
- **Existing tests that assumed one species and unconditional hunting
  needed updating.** `memories_test.go` now reads
  `w.alienSpeciesFor(alien).BiteDamage` instead of a flat `cfg.AlienDamage`.
  `sim_test.go`'s `TestAliensEatColonists` (a cornered colonist must
  eventually be eaten) now force-sets every rolled species' `Temperament` to
  `TemperamentHostile` before running: the test is about the bite/remove
  path working at all, not about which temperament a given seed happened to
  roll for its one default species — leaving it seed-dependent would have
  made an unrelated, uninvolved test flip pass/fail based on incidental
  rolls of a feature it isn't testing.

## Extending it

- **More attack modes.** A mode a feature grants goes in
  `featureAttackModes` and `featureAttacks`, with no draw at all, as gore
  and sting do. A mode to roll goes at the end of `attackModes` (inserting
  mid-list shifts lore draws, and even appending shifts every later
  species' draws), a `canUse` gate, an `attackPhrase`
  phrase, a `strikeVerbs` entry, a `strikeTargetText` case, and whatever
  mechanics `strike` gives it (a stomp for heavy many-legged species, a
  sting for a tailed chitinous one). Modes could also gate names
  (`nameCondition` has no attack leaf yet).
- **Flight.** `Wings` is rolled and described but does nothing. Letting a
  winged species cross open floor faster, ignore rubble, or swoop (a wing
  buffet `AttackMode`, gated by `canUse` on `Wings`) would make it matter;
  keep the wording in `gaitPhrase` honest when it does.
- **Lifecycles and graded anatomy** are built: some species roll a life of
  several forms (egg, grub, pupa, joey; queen/worker/drone castes), each
  larger and more extreme than the last but recognizably the same, and every
  species rolls graded features (horns, antlers, stingers, quills, tail
  clubs, shells) that `Description` adds a sentence for, their adults
  or queens lay the next generation, and the features fight: horns gore,
  stingers sting, claws and clubs hit harder, shells blunt hits. See
  [alien-lifecycles.md](./alien-lifecycles.md).
- **Per-individual variation.** Every alien of a given species is still
  stat-for-stat identical to every other of that species. Giving each
  `Entity` its own height/weight rolled from its species' range (the way
  `rollBody` does for colonists) and scaling its own bite off that, rather
  than the species midpoint, is the natural next step —
  `mutation.go`'s `scaleBody` is already most of the machinery for "resize
  this entity and its parts by a ratio."
- **Colonists learning a species' temperament.** Right now a colonist
  treats every `Alien` as an equal threat regardless of `Temperament`. A
  colony that stops fleeing a species it has observed being Friendly (or
  gets more cautious around one it has seen being Hostile) is a real next
  step, deliberately left out of this pass — see Why it is this way.
- **A fuller codex.** The TUI's lore tab (see
  [frontend-tui.md](./frontend-tui.md)) already reaches `Description()` and
  `RosterLabel()` for every rolled species, plus world facts (size, how much
  is explored, the seed). Growing it into a genuine "what has the colony
  learned about this world" codex — sightings-gated reveal, a species' entry
  staying blank until the colony has actually met one, whatever
  organizations/corporations/other-colonies lore adds next — is the natural
  next step; today every species and fact is shown unconditionally, whether
  or not the colony has ever encountered it.
- **More names, more conditions.** `internal/sim/alien-names.yaml` (or a
  file passed via `-alien-names`) is the whole pool; add an entry with
  whatever `all`/`any`/`not` condition fits. `nameCondition` covers
  temperament, skin, color, height/weight tier, tail, wings, and counts on
  legs/arms/limbs/eyes today — a new leaf field is a small, mechanical
  addition (a struct field, a case in `matches`) if a new trait ever needs
  to gate a name.
- **More map-safe emoji.** Adding a new glyph a name can draw on the map is
  the same two-line edit any other glyph is (see
  [terminal-cell-widths.md](./terminal-cell-widths.md)'s Extending it): a
  constant (and `All` entry) in `internal/glyphs/glyphs.go` and a
  `glyphRegistry` entry in `internal/ui/tui/glyphs.go`, with a declared width
  and an ASCII fallback — the existing structural
  tests (`TestGlyphRegistryIsUnambiguous` and friends) vet it the same way
  they vet every other glyph. Then list the new emoji in whichever
  `alien-names.yaml` entries should be able to roll it. A name's emoji list
  can also freely include a string that is *not* in the registry — it still
  works as roster/lore flavor text, just never appears on the map (see Why
  it is this way).
- **Wiring `Kind.String()`/`observeNearby`'s sighting text to a species**
  would need a `*World` (or the resolved noun) threaded through, since
  `Kind.String()` today is a plain enum method and `observeNearby`'s "Saw %s
  #%d." line is shared with rats. Worth doing once there's a second
  world-scoped creature name to generalize the pattern for.
- **A precise per-species breakdown for a mixed alien-swarm occurrence**,
  instead of `fireAlienSwarm`'s current one-name-for-the-whole-spawn
  simplification, if `AlienSpeciesCount` > 1 swarms turn out to be common
  enough in play to be worth the extra bookkeeping.
- **Lore colonists can talk about.** `LoreItem` (see
  [conversation-topics.md](./conversation-topics.md)) is the interface any
  lore implements to become a conversation topic; species are wrapped as
  `speciesLore` and listed by `World.loreItems`. History and whatever comes
  next should implement it and append there.
- **Organizations, corporations, other colonies.** Corporations have
  started: [arms-makers.md](./arms-makers.md) rolls a roster of companies,
  gives every gun a make and model, makes them conversation lore, and gives
  colonists former employers, following this pattern. The pattern here — roll
  something once per seed (or per count), off its own RNG stream, store it
  on `World`, expose a copy through `Snapshot` — is meant to be the template
  the next piece of lore follows, not a one-off special case for aliens.

## Related

- [species-and-behaviors.md](./species-and-behaviors.md) — the plan that
  turns each rolled species (and, proposed, each lifecycle stage) into a
  species value with its own behavior ladder.
- [alien-taxonomy.md](./alien-taxonomy.md): the scientific name each
  species gets on top of its common name.
- [world.md](./world.md) — the worldgen pipeline (`generate`) lore's alien
  placement still uses, and the `Rock`/composition generation whose
  dedicated-RNG-stream pattern this reuses.
- [combat.md](./combat.md) — body parts, `applyDamage`, and the bite/shoot
  paths that now read a specific alien's assigned species instead of a flat
  `Config` value.
- [mutation.md](./mutation.md) — `scaleBody`/`scaleRound`, the same
  ratio-of-a-size scaling `speciesDamage` reuses for an alien instead of a
  mutated colonist.
- [director.md](./director.md) — the alien-swarm occurrence, and the
  simplification it makes when a swarm spans more than one rolled species.
- [personality.md](./personality.md) — the `prng`/`rng` stream split lore's
  own dedicated stream sits alongside.
- [frontend-tui.md](./frontend-tui.md) — the lore tab, where a player
  actually reads all of this.
- [terminal-cell-widths.md](./terminal-cell-widths.md) — the glyph registry
  and startup probe `alienGlyph` defers to, and why an unvetted string never
  reaches the map's fixed-width tile.
- [fog-of-war.md](./fog-of-war.md) — `World.reveal`, where
  `Stats.ExploredTiles` is kept.
- [configuration.md](./configuration.md) — how `AlienSpeciesCount`,
  `AlienCautiousRadius`, and the other alien tunables become CLI flags and
  settings-file keys.
- [config-file.md](./config-file.md) — the committed `mars-sim.yaml`
  pattern `alien-names.yaml`'s own optional-file loading mirrors.

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
to one rolled species, and its bite hits harder or softer depending on how
big that species rolled. What a species can be *named* is itself
configurable data — a condition-gated pool of names, edited in YAML — rather
than a hardcoded list.

## Source

- [`internal/sim/lore.go`](../internal/sim/lore.go) — `AlienSpecies`,
  `AlienTemperament`, `AlienSkin`, `AlienSizeTier`, `rollAlienSpecies`,
  `rollAlienSpeciesRoster`, `speciesDamage`, `scaledByTemperament`, and the
  `World.alienSpeciesFor`/`alienNounFor`/`alienPluralFor` helpers.
- [`internal/sim/alien_names.go`](../internal/sim/alien_names.go) —
  `AlienNameEntry` (including its `Emoji` candidates),
  `nameCondition`/`intCondition` (the boolean condition tree),
  `pickAlienName`, `LoadAlienNames`, `defaultAlienNames` (the `//go:embed`ded
  built-in pool).
- [`internal/sim/alien-names.yaml`](../internal/sim/alien-names.yaml) — the
  built-in name pool, compiled into the binary, with the curated emoji
  candidates each entry can roll.
- [`alien-names.yaml.example`](../alien-names.yaml.example) — a commented,
  standalone example a player can copy and pass to `-alien-names`.
- [`internal/ui/tui/glyphs.go`](../internal/ui/tui/glyphs.go) — the curated
  reptile/bug/alien glyph set (`glyphLizard`, `glyphBeetle`, ...) and
  `alienGlyph`, which decides whether a species' rolled `Emoji` is safe to
  draw on the map.
- [`internal/sim/lore_test.go`](../internal/sim/lore_test.go),
  [`internal/sim/alien_names_test.go`](../internal/sim/alien_names_test.go) —
  determinism, invariants, temperament behavior, the naming-condition
  boolean logic, and the emoji draw.
- [`internal/ui/tui/glyphs_test.go`](../internal/ui/tui/glyphs_test.go) —
  `alienGlyph`'s registered/unregistered/empty cases, and that every curated
  emoji is actually registered.
- [`internal/sim/world.go`](../internal/sim/world.go) — `World.alienSpecies`
  (now a roster, `[]AlienSpecies`) and where it's rolled, in `newWorld`; the
  per-`Alien` species draw in `spawn`; `World.exploredCount`, kept
  incrementally by `reveal`.
- [`internal/sim/entity.go`](../internal/sim/entity.go) — `Entity.Species`,
  the index into `World.alienSpecies` an individual alien was assigned.
- [`internal/sim/config.go`](../internal/sim/config.go) — `AlienSpeciesCount`,
  `AlienCautiousRadius`, `AlienNames`, and `AlienDamage`/`AlienBiteRest`/
  `AlienSlowness`/`AlienReferenceWeightKG` (now baselines a species scales).
- [`internal/sim/combat.go`](../internal/sim/combat.go),
  [`internal/sim/systems.go`](../internal/sim/systems.go) — `alienTurn`'s
  temperament dispatch, and where combat reads a specific alien's assigned
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
limb count split into arms vs. legs via `Arms`/`Legs()`, tail or not, `Skin`,
`Color`), a colloquial name (`Singular`/`Plural`, picked from a condition
pool — see below), a `Temperament`, and three precomputed combat stats:
`BiteDamage`, `BiteRest`, `Slowness`.

`Config.AlienSpeciesCount` (default 1) says how many species a world rolls;
`rollAlienSpeciesRoster` rolls that many (clamped to at least 1) into
`World.alienSpecies`. Every `Alien` entity is assigned one at spawn time —
`Entity.Species` indexes into that slice, drawn uniformly in `World.spawn`
— so a world with more than one species can have a real mix wandering the
rock, each with its own build, name, and temperament. `alienSpeciesFor(e)`
is how every other system reads the species a specific alien belongs to.

### Where it rolls, and on what stream

The roster is rolled in `newWorld`, not `generate()` — a handful of tests
build a `World` with `newWorld` directly and still spawn and fight `Alien`
entities against it, and those need a valid roster too. It draws from a
dedicated stream, `rand.New(rand.NewSource(cfg.Seed ^ alienLoreSeed))`, the
same pattern `growRockVeins`' `compositionRNG` uses in `worldgen.go` and for
the same two reasons:

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

- **Friendly** never fights. `alienTurn` (`systems.go`) returns immediately
  to `wanderStep` for it — it never looks for prey at all. (A colonist may
  still flee or fight one on its own initiative; that side of the
  interaction is explicitly unchanged for now — see Why it is this way.)
- **Cautious** does not hunt, but reacts once a colonist comes within
  `Config.AlienCautiousRadius`: `alienTurn` calls the radius-bounded
  `nearestOfKind` instead of the unbounded `nearestOfKindAnywhere`, so it
  only ever notices — and then closes in on and bites — a colonist already
  close by.
- **Hostile** hunts the nearest colonist anywhere on the map,
  unconditionally: the same `nearestOfKindAnywhere` + burrow-toward-it
  behavior every alien had before temperament existed.

`rollTemperament` makes Friendly rare (10%) and Cautious/Hostile common and
roughly even (45% each) — the "ET to Xenomorph" spread the ask described.

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

	Temperament, Skin, Color, Height, Weight string
	Tail                                     *bool
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

`pickAlienName(rng, sp, names)` collects every entry whose condition matches
the just-rolled species and draws one at random — overlapping conditions
(several names fit the same species) are normal, not an error, which is why
`alien-names.yaml`'s conditions are allowed to be loose and to overlap
freely. `rollAlienSpecies` calls it *last*, after every other trait is
rolled, since the name depends on the build, not the other way around.

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

So the TUI draws it on the map only through `alienGlyph(sp)`
(`internal/ui/tui/glyphs.go`), which checks the exact string against its own
`glyphRegistry` — the same vetted, width-tested, ASCII-fallback-carrying set
every other glyph on the map comes from — and falls back to the generic
`glyphAlien` for anything unregistered, `sp.Emoji == ""` included. The
registry ships a curated set of reptile/bug/alien glyphs for this
(`glyphLizard`, `glyphBeetle`, `glyphSaucer`, ...) that `internal/sim/
alien-names.yaml`'s own emoji lists draw from, so the built-in pool's
species always have a real, registered glyph to show on the map; a custom
`-alien-names` file is free to name anything as flavor text, but only shows
up on the map if it happens to spell one of those same registered symbols
exactly (see the note in `alien-names.yaml.example`).

### Narration

`World.alienNounFor(e)` (`withArticle(w.alienSpeciesFor(e).Singular)`,
reusing `mutation.go`'s article helper) and `alienPluralFor(e)` are what
combat and the roster read instead of the literal word "alien" — "Killed a
gremlin with a shotgun!" instead of "Killed an alien with a shotgun!" Both
take the specific alien entity involved, not a single world-wide value, so a
world with more than one species narrates each encounter with the right
one. The director's alien-swarm log line is the one place that still picks
just one name for a whole spawn: with more than one species rolled, a swarm
can be a genuine mix, and naming it after whichever alien happened to spawn
first is a deliberate simplification for a single flavor line, not a
species-by-species breakdown. `Kind.String()` (`entity.go`) and the generic
creature-sighting line in `observeNearby` (`systems.go`) still say "alien"
deliberately — see Extending it.

`AlienSpecies.Description()` renders a full narrative paragraph (build,
skin, color, temperament); `RosterLabel()` is the short form
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
[fog-of-war.md](./fog-of-war.md) and [world.md](./world.md)) — above a
selectable list of `Snapshot.AlienSpecies`, each shown by `RosterLabel()`.
The detail panel lists the selected species' full build as explicit stat
lines (height/weight range, eyes, limb split, tail, skin, color, bite
damage/pace) followed by `Description()`'s narrative paragraph,
word-wrapped to the panel width.

## Why it is this way

- **`sim` carries `Emoji` as an opaque string, never a "glyph."** The ask
  was explicit that `sim` shouldn't know anything about emoji, and it still
  doesn't: `AlienSpecies.Emoji` is data of exactly the same kind as `Skin`
  or `Color` — a string picked from YAML, never measured, registered, or
  validated by anything in the `sim` package. The width-safety machinery
  the map depends on (see [terminal-cell-widths.md](./terminal-cell-widths.md))
  lives entirely on the TUI side, in `alienGlyph`, which is also the only
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
  tiers `TemperamentFriendly`/`Cautious`/`Hostile` and switching on them in
  `alienTurn` makes "never initiates combat" something the compiler and a
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
  temperament, skin, color, height/weight tier, tail, and counts on
  legs/arms/limbs/eyes today — a new leaf field is a small, mechanical
  addition (a struct field, a case in `matches`) if a new trait ever needs
  to gate a name.
- **More map-safe emoji.** Adding a new glyph a name can draw on the map is
  the same two-line edit any other glyph is (see
  [terminal-cell-widths.md](./terminal-cell-widths.md)'s Extending it): a
  `glyphX` constant and a `glyphRegistry` entry in `internal/ui/tui/glyphs.go`,
  with a declared width and an ASCII fallback — the existing structural
  tests (`TestGlyphRegistryIsUnambiguous` and friends) vet it the same way
  they vet every other glyph. Then list the new emoji in whichever
  `alien-names.yaml` entries should be able to roll it. A name's emoji list
  can also freely include a string that is *not* in the registry — it still
  works as roster/lore flavor text, just never appears on the map (see Why
  it is this way).
- **Wiring `Kind.String()`/`observeNearby`'s sighting text to a species**
  would need a `*World` (or the resolved noun) threaded through, since
  `Kind.String()` today is a plain enum method and `observeNearby`'s "Saw %s
  #%d." line is shared with mice. Worth doing once there's a second
  world-scoped creature name to generalize the pattern for.
- **A precise per-species breakdown for a mixed alien-swarm occurrence**,
  instead of `fireAlienSwarm`'s current one-name-for-the-whole-spawn
  simplification, if `AlienSpeciesCount` > 1 swarms turn out to be common
  enough in play to be worth the extra bookkeeping.
- **Organizations, corporations, other colonies.** The pattern here — roll
  something once per seed (or per count), off its own RNG stream, store it
  on `World`, expose a copy through `Snapshot` — is meant to be the template
  the next piece of lore follows, not a one-off special case for aliens.

## Related

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

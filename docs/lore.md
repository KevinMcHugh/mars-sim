# Lore

> Part of the [mars-sim documentation](./README.md).

## What it is

Lore is the start of a layer describing the world beyond the colony itself —
what's out there, not just what's been dug out. The first (and so far only)
piece of it: every world rolls its own kind of alien during worldgen — a
build, a name colonists reach for instead of "alien," and a temperament —
so what's hunting the colony reads differently from one seed to the next,
and its bite hits harder or softer depending on how big it rolled.

## Source

- [`internal/sim/lore.go`](../internal/sim/lore.go) — `AlienSpecies`,
  `rollAlienSpecies`, `speciesDamage`, `scaledByAggression`, the descriptor
  table, and `World.alienNoun`/`alienPlural`.
- [`internal/sim/lore_test.go`](../internal/sim/lore_test.go) — determinism,
  invariants, variety across seeds, and the damage/pace scaling formulas.
- [`internal/sim/world.go`](../internal/sim/world.go) — `World.alienSpecies`
  and where it's rolled, in `newWorld`.
- [`internal/sim/config.go`](../internal/sim/config.go) — `AlienDamage`,
  `AlienBiteRest`, `AlienSlowness` (now baselines the species scales) and the
  new `AlienReferenceWeightKG`.
- [`internal/sim/combat.go`](../internal/sim/combat.go),
  [`internal/sim/systems.go`](../internal/sim/systems.go),
  [`internal/sim/director.go`](../internal/sim/director.go) — where combat and
  the director's alien-swarm occurrence read `World.alienSpecies` instead of
  a flat config value, and narrate with the rolled name.
- [`internal/sim/snapshot.go`](../internal/sim/snapshot.go) —
  `Snapshot.AlienSpecies`.
- [`internal/ui/tui/render_roster.go`](../internal/ui/tui/render_roster.go) —
  the roster's alien entry, showing the rolled species instead of the literal
  word "alien."

## How it works

### One species per world

`AlienSpecies` (`lore.go`) holds a build (height/weight ranges, eye count,
limb count split into arms vs. legs, tail or not), a colloquial name
(`Singular`/`Plural`, drawn from a fixed table — "xeno/xenos,"
"critter/critters," "gremlin/gremlins," and so on), a 0–100 `Aggression`
running from curious-and-passive to a relentless killing machine, and three
precomputed combat stats: `BiteDamage`, `BiteRest`, `Slowness`.

`rollAlienSpecies(rng, cfg)` fills all of that from one RNG stream. There is
exactly one species per world, stored as `World.alienSpecies` — not one per
`Alien` entity. Every alien the colony ever meets, from the ones worldgen
seeds in the starting rock to a later director swarm (`fireAlienSwarm`) to a
debug spawn, reads the same `World.alienSpecies` at combat time, so they are
all the same creature by construction. Per-individual variation within a
species (this one alien happens to be a runt) is future work — see Extending
it.

### Where it rolls, and on what stream

`w.alienSpecies` is rolled in `newWorld`, not `generate()` — a handful of
tests build a `World` with `newWorld` directly and still spawn and fight
`Alien` entities against it, and those need a valid species too. It draws
from a dedicated stream, `rand.New(rand.NewSource(cfg.Seed ^ alienLoreSeed))`,
the same pattern `growRockVeins`' `compositionRNG` uses in `worldgen.go` and
for the same two reasons:

- Not `World.prng` (the personality stream): a species' size and temperament
  are not flavor — they set actual bite damage and combat pace — so they
  have to stay on the deterministic side of the personality/simulation split
  (see [personality.md](./personality.md) and `AGENTS.md`).
- Not `World.rng` (the simulation stream): rolling the species must not
  perturb any `w.rng`-driven decision that runs afterward — where colonists
  and aliens spawn, first and foremost — so it cannot share that stream's
  draw sequence.

### Damage scales with size, not with aggression

`speciesDamage(sp, cfg)` scales `cfg.AlienDamage` by `sp`'s weight (the
midpoint of `WeightMinKG..WeightMaxKG`) relative to
`cfg.AlienReferenceWeightKG` — the weight at which a specimen deals exactly
the configured baseline. A species heavier than the reference hits harder; a
lighter one hits softer. `scaleRound` (the same rounding helper
`mutation.go`'s stature walk uses) does the arithmetic, so this is exactly
the same "ratio of a size" scaling that resizing a mutated colonist's body
already does — see [mutation.md](./mutation.md).

One deliberate exception: a configured `AlienDamage` of 0 passes straight
through as 0 rather than being floored back up to 1. `combat_test.go` sets
`AlienDamage = 0` to neuter an alien's bite so it can test gunfire in
isolation; scaling must not turn that "off" switch back "on" for a
heavyweight roll.

### Aggression scales pace, not damage

`scaledByAggression(base, aggression)` scales `AlienBiteRest`/`AlienSlowness`
by `Aggression`, 0–100: 50 (the midpoint) reproduces the baseline exactly, 0
(curious and passive) makes the species half again slower to strike and
move, and 100 (a relentless killer) makes it strike and move twice as fast.

Aggression and size are deliberately two separate knobs, not one: a species
being *scary* (fast, relentless) and a species being *dangerous* (hits hard)
are different things a colony would come to know separately about its local
wildlife, and conflating them would mean a big, slow brute and a small,
frantic swarm could never both exist across different seeds.

### Narration

`World.alienNoun()` (`withArticle(w.alienSpecies.Singular)`, reusing
`mutation.go`'s article helper) and `World.alienPlural()` are what combat,
the director's alien-swarm log line, and the roster read instead of the
literal word "alien" — "Killed a gremlin with a shotgun!" instead of "Killed
an alien with a shotgun!" `Kind.String()` (`entity.go`) and the generic
creature-sighting line in `observeNearby` (`systems.go`) still say "alien"
deliberately — see Extending it.

`AlienSpecies.Description()` renders a full narrative paragraph (build,
temperament) for a future lore/codex display; `RosterLabel()` is the short
form (`"Xeno · aggressive"`) the roster actually shows today in place of a
colonist's pronouns/age line.

## Why it is this way

- **One species per world, not one per entity.** The ask was "the fictional
  world should mean different things on different seeds" — that's a
  per-world fact, not a per-creature one. Storing it once on `World` and
  having every `Alien` entity read it at combat time is far less surface
  area than giving every entity its own rolled body (no `Entity` field, no
  `EntityView` field, no roster/snapshot plumbing per creature) while still
  landing the actual ask. Individual variation within a species is a natural
  next step, not a requirement of this one — see Extending it.
- **A dedicated RNG stream, not `w.rng` or `w.prng`.** `w.prng` would be
  wrong because the roll is gameplay (it sets real damage and pace), and
  `w.rng` would be wrong because generating it must not perturb entity
  placement or any other `w.rng`-driven decision — exactly
  `compositionRNG`'s reasoning in `worldgen.go`, reused rather than
  reinvented.
- **Rolled in `newWorld`, not `generate()`.** Several existing tests
  (`memories_test.go`, `lifeevents_test.go`, ...) build a `World` with
  `newWorld` directly, bypass `generate()` entirely, and still spawn and bite
  `Alien` entities. Rolling the species only in `generate()` would have left
  those with a zero-value species (a nameless creature dealing zero damage),
  silently wrong rather than obviously wrong.
- **Damage from size, pace from aggression — not one scale doing both.** The
  ask explicitly separated them ("the damage they do should scale with
  size" as its own sentence, aggression described as its own "ET to
  xenomorph" scale). Keeping them as two independent rolls is what lets a
  seed land on a huge, lumbering brute *or* a small, frenzied swarm, instead
  of collapsing "big" and "scary" into the same knob.
- **`AlienDamage`/`AlienBiteRest`/`AlienSlowness` stay as config baselines**
  rather than being replaced outright by the species. A config tunable that
  a seed's roll could silently override would make `-alien-damage 0` (which
  `combat_test.go` relies on to isolate gunfire) stop meaning "off." Scaling
  a baseline, with an explicit zero-passthrough, keeps that guarantee while
  still letting size and aggression move the effective numbers per seed.
- **Existing tests that assumed `cfg.AlienDamage` was the literal bite
  damage** (`memories_test.go`) needed updating once it became a baseline
  instead of the actual number: they now read `w.alienSpecies.BiteDamage`,
  and the ones that only needed a bite to definitely *not* be lethal now set
  the victim's full body (`HP` and every `Parts` entry), not just enough
  headroom over the old flat constant — a heavy roll's bite can still exceed
  a single body part's stock HP (Torso, Head) even while the aggregate pool
  has room, which the old margin didn't account for.
- **Narration is scoped to the strings that read as world-facing prose**
  (combat outcomes, the alien-swarm director line, the roster). `Kind.String()`
  and the generic sighting line in `observeNearby` were deliberately left
  saying "alien": the former has no `World` to consult (it's a method on the
  bare `Kind` enum), and the latter is shared with mice's sighting text — both
  would need real plumbing changes to specialize, which didn't feel
  proportionate to what was asked for a first pass. See Extending it.

## Extending it

- **Per-individual variation.** Today every alien in a world is stat-for-stat
  identical (`World.alienSpecies`). Giving each `Entity` its own height/weight
  rolled from the species range (the way `rollBody` does for colonists) and
  scaling its own bite off that, rather than the species midpoint, is the
  natural next step — `mutation.go`'s `scaleBody` is already most of the
  machinery for "resize this entity and its parts by a ratio."
  - Reaching `Description()` and `RosterLabel()` from a wider "codex" panel
  that surfaces everything the colony has learned about the world it landed
  in — a natural home for whatever organizations/corporations/other-colonies
  lore is added next.
- **More alien nouns.** `alienNouns` is a flat table; add an entry to widen
  what a seed can call its aliens.
- **Wiring `Kind.String()`/`observeNearby`'s sighting text to the species**
  would need a `*World` (or the resolved noun) threaded through, since
  `Kind.String()` today is a plain enum method and `observeNearby`'s "Saw %s
  #%d." line is shared with mice. Worth doing once there's a second
  world-scoped creature name to generalize the pattern for.
- **Organizations, corporations, other colonies.** The pattern here — roll
  something once per seed, off its own RNG stream, store it once on `World`,
  expose a copy through `Snapshot` — is meant to be the template the next
  piece of lore follows, not a one-off special case for aliens.

## Related

- [world.md](./world.md) — the worldgen pipeline (`generate`) lore's alien
  placement still uses, and the `Rock`/composition generation whose
  dedicated-RNG-stream pattern this reuses.
- [combat.md](./combat.md) — body parts, `applyDamage`, and the bite/shoot
  paths that now read `World.alienSpecies` instead of a flat `Config` value.
- [mutation.md](./mutation.md) — `scaleBody`/`scaleRound`, the same
  ratio-of-a-size scaling `speciesDamage` reuses for an alien instead of a
  mutated colonist.
- [director.md](./director.md) — the alien-swarm occurrence, whose spawned
  aliens are this same one rolled species.
- [personality.md](./personality.md) — the `prng`/`rng` stream split lore's
  own dedicated stream sits alongside.
- [configuration.md](./configuration.md) — how `AlienReferenceWeightKG` and
  the other alien tunables become CLI flags and settings-file keys.

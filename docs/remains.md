# Remains

> Part of the [mars-sim documentation](./README.md).

## What it is

A record of whose body lies on a tile, so the tile inspector can answer "who
is this?" when a player finds a body, and link through to the dead creature's
inspector (its cause and when it died, as a colony day and time, and for a colonist the whole archived
profile) to work out what happened.

## Source

- `internal/sim/remains.go` — `Remains`, `leaveBody`, `forgetRemains`,
  `clearAllRemains`, `publishedRemains`, `Snapshot.RemainsAt`.
- `internal/sim/world.go` — `takeCorpse` and `clearRefuse` keep the index in
  step with the refuse counts.
- `internal/wire/inspect.go` — `TileTopic.Remains` / `Body`, built by
  `remainsBody`.
- `web/src/ui/InspectPanel.svelte` — the tile inspector's "A body" and "On the
  floor" sections.
- `internal/sim/remains_test.go`, `internal/wire/inspect_test.go`
  (`TestInspectTileNamesTheDead`).

## How it works

Bodies on the floor are counts by kind in the refuse index (`refuseCell`, see
[sanitation.md](./sanitation.md)); that is all a cleaner needs. Beside it the
World keeps `remains map[Point][]Remains`, one `{ID, Body}` per named body,
oldest first.

- A death with a victim leaves its body through `leaveBody(e, p, kind)`, which
  calls `addCorpse` and appends the entry. Every death site that leaves a body
  does: starving (colonists and other species), being shot, being stomped,
  falling down a hole.
- `takeCorpse` (a cleaner gathering, a rat scavenging) forgets the oldest entry
  of that kind. `clearRefuse` (building over the tile) forgets them all.
- `publishedRemains` copies the map into `Snapshot.Remains` only when
  `remainsRev` moved, like salt and refuse.
- The wire's tile topic looks each ID up with `findEntity` (living, the
  colonist archive, then the bounded graveyard) for a name, glyph, cause and
  tick. A creature whose graveyard record has aged out still gets a name from
  its body kind ("animal carcass #41"), just no cause.

So `len(RemainsAt(p)) <= Tile.Corpses`, and the difference is shown as
"unidentified".

## Why it is this way

- **A side index, not IDs in `refuseCell`.** `refuseCell` is compared with
  `==` to decide a tile is clean, copied whole on publish, and walked to sum
  totals. A slice in it would break the first and make the others costlier for
  a feature only the inspector reads.
- **Guessing from the graveyard was rejected.** Matching deaths whose position
  is the tile looks free, but it is wrong in ordinary play: a colonist eaten by
  an alien dies on a tile and leaves only gore, a rat can eat a body, and a
  cleaner can carry one off, so the graveyard would name bodies that are not
  there.
- **A body loses its name when picked up.** Carried refuse is a plain item
  stack (`ColonistCorpse ×2`), so the identity cannot ride in an inventory
  without making stacks carry IDs. In practice a picked-up body is burned or
  rendered; the one way it reappears on the floor is a hauler tipping its load
  down a hole (`tipDown`), and those land unidentified.

## Extending it

- A new way to die that leaves a body should call `leaveBody`, not
  `addCorpse`. `addCorpse` is for refuse with no known victim.
- A new way for a body to leave a tile should go through `takeCorpse` or
  `clearRefuse` so the index stays no larger than the count.
- To keep names on carried bodies, the inventory would need per-unit identity
  for corpse kinds; `forgetRemains` is the place that currently drops it.

## Related

- [sanitation.md](./sanitation.md) — the refuse counts this sits beside.
- [combat.md](./combat.md) — the graveyard and causes of death.
- [wire-format.md](./wire-format.md) — the `tile:` topic.

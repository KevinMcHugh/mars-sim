# Sandbox

> Part of the [mars-sim documentation](./README.md).

## What it is

A debugging and tuning mode: a game that starts with an empty world, where the
player puts colonists and aliens on the map one click at a time. It exists to
set up a scene on purpose (three colonists and a docile alien, say) and watch
what the simulation does with it, instead of waiting for a real colony to
stumble into that scene. It is the `sandbox` setting, the **Sandbox** box on
the browser's New game form (or `?sandbox=true` in the URL), and the
**Sandbox** tab it opens.

## Source

- [`internal/sim/sandbox.go`](../internal/sim/sandbox.go) — the `Place`
  command, `placeRefusal` (which tiles take a creature) and `outfitPlaced`
  (what a placed colonist brings).
- [`internal/sim/worldgen.go`](../internal/sim/worldgen.go) — `generate`
  lands nobody and places no aliens, rats or cats in a sandbox.
- [`internal/sim/caverns.go`](../internal/sim/caverns.go) — `rollNests`
  rolls no nest in a sandbox; [`world.go`](../internal/sim/world.go)
  resolves no director schedule.
- [`internal/sim/sandbox_test.go`](../internal/sim/sandbox_test.go) — the
  empty start, placing and every refusal, and a placed colonist outliving
  its first day.
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go) — the page's
  `place` command (`hostAPI` 28); [`internal/wire/wire.go`](../internal/wire/wire.go)
  — `Hello.sandbox`, so the page knows a loaded save is a sandbox.
- [`web/src/ui/SandboxPanel.svelte`](../web/src/ui/SandboxPanel.svelte) — the
  tab, with its species search in [`sandbox.ts`](../web/src/ui/sandbox.ts)
  (tested by `sandbox.test.mjs`); `armPlace` and `placeAt` in
  [`web/src/game.svelte.ts`](../web/src/game.svelte.ts); the click and the
  hover tint (`showPlacePreview`) in [`web/src/main.ts`](../web/src/main.ts).

## How it works

**An empty start.** With `sandbox` on, `generate` still carves the landing
cavern, sized for one full ship (`sandboxCavernColonists`, 20) whatever
`colonists` says (`landingRadii`). Then it lands nobody (no ships, none
aloft) and skips the starting aliens, rats and cats. Caverns the colony breaks
into roll no nest, and the director's script is never resolved, so nothing
arrives that the player did not place. The roster takes **every** species in
the species pack, not `alien-species-count` of them, so any of them can be
placed (a world with no pack still rolls `alien-species-count`). The rest of
the world (ore, scum, caverns) is generated exactly as it would be: the seed
field works as in any game.

**The form asks for no counts.** Ticking *Sandbox* hides the New game form's
*Colonists* and *Alien species* fields, since the engine ignores the first
and uses the whole pack instead of the second.

**Placing.** The tab's **Colonist** and **Alien** buttons pick up a one-click
map tool (`ui.placeTool`), like the Dig tab's dig-down tool. It stays up after
each click, so a crowd is a few clicks, and Esc or the button puts it down.
The **Species** list holds the lore topic's species, in roster order, plus
*Random*; a species' roster index is what the `place` command sends. A search
box narrows it (every word must appear in the label, names, binomial or
temperament) and three chips, *Friendly*, *Cautious* and *Hostile*, narrow it
to any of the temperaments switched on (`matchSpecies`). Choosing a species
picks up the alien tool with it, and Enter in the search box chooses the
first match. The tile under the
pointer is tinted green on seen floor with no creature on it and red
elsewhere, but the engine has the last word. `Place{Kind, At, Species}` is
refused, with the reason in the log, when:

- the game is not a sandbox;
- the tile is off the map or on a level the colony has not reached;
- it is not walkable (rock, a wall);
- it is in a cavern the colony has not found;
- something already stands on it;
- the species index is past the roster.

**A placed colonist** is a fresh random colonist (`spawnAs`: name, looks,
traits, family ties, drives, a purse), outfitted by `outfitPlaced` as a
ship's passenger would be, minus the ship: a background and an employer, the
meals its locker would have held carried in its pockets, and its gun if its
rare item roll gives one. It has no bunk, locker, toilet or chicken.

**A placed alien** is `spawnAs(Alien, …)` with the chosen species (or one
drawn from the simulation stream for *Random*). It begins its species'
lifecycle like any other alien and, standing on known floor, is awake from
the first tick.

## Why it is this way

- **A setting, not a page-only trick.** The page could have sent
  `colonists: 0`, `aliens: 0` and `cavern-nest-percent: 0`. But a zero
  colonist count also shrinks the landing cavern to its minimum, the director
  would still fire, and the engine would have no way to tell a sandbox from a
  real game, so `Place` would be a cheat available everywhere. One `sandbox`
  flag gates the command, survives save and load (it is part of the saved
  `Config`), and reaches the page through `Hello` so the tab shows only when
  it applies.
- **Search, not a dropdown.** A sandbox's roster is the whole species pack,
  which only grows as Creature Lab adds species, and a `<select>` of every
  one stopped being usable once it filled the screen.
- **No counts to choose.** The first version sized the cavern from
  `colonists` and rolled `alien-species-count` species, so the form made you
  pick a non-zero number of colonists for a world that lands none, and pick
  in advance which species you would get to place. Both read as wrong for an
  empty world, so a sandbox fixes the cavern at one ship's worth and takes
  the whole pack.
- **Only discovered floor.** With fog of war off the map shows every cavern,
  but a creature placed in one the colony has not broken into would be
  *dormant* (`dormant`): it keeps to its sealed cave and nobody perceives it.
  That is never what a sandbox user wants, so it is refused with a reason
  rather than silently doing nothing.
- **Placed colonists carry their meals.** The first version spawned a bare
  colonist, and on the game's defaults (no free food) a lone one starved
  inside its first day: with no ship there is no locker, and an empty
  sandbox has no kitchen or silo to buy from (`TestPlacedColonistEats`).
  Pockets are the first place a hungry colonist looks
  ([food.md](./food.md)), so the locker's meals go there.
- **A frightened colonist can still starve.** A colonist kept fleeing from an
  alien does not stop to eat, even a meal in its own pocket, so a sandbox
  with a hostile alien loose can show "starved to death" beside a full
  pocket. That is the game's behavior, not the sandbox's; the sandbox just
  shows it quickly.
- **No pets.** A chicken needs its keeper's trough and a ship's hold; a cat
  is placeable in the engine (`Place` takes any kind) but the tab offers only
  what the issue asked for, colonists and aliens.

## Extending it

- **Another kind on the tab:** `Place` already takes any `Kind`; add a button
  and a `PlaceTool` variant in `game.svelte.ts`, and the page sends its
  `sim.Kind` name.
- **More sandbox controls** (remove a creature, set a drive) belong behind
  the same `Config.Sandbox` gate, as commands that refuse outside it.
- Anything that should not happen in an empty world on its own (a new kind of
  scheduled arrival, a new spawner) must check `Config.Sandbox` the way
  `generate`, `rollNests` and the director do.

## Related

- [ships.md](./ships.md) — how colonists normally arrive, and what
  `outfitPlaced` copies.
- [caverns.md](./caverns.md) — nests and dormant aliens.
- [director.md](./director.md) — the scheduled arrivals a sandbox skips.
- [lore.md](./lore.md) — the species roster the picker lists.
- [frontend-web.md](./frontend-web.md) — the New game form and the side
  panel's tabs.

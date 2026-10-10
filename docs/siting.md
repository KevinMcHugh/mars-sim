# Siting

> Part of the [mars-sim documentation](./README.md).

## What it is

Who picks where the colony digs down: the tile a stair, a shaft, a hole or a
ladder goes on. With `siting-auto` off (the compiled default, and how the
browser starts a game) the player picks every one, and the colony digs down
nowhere it was not told to. With it on, an order may leave the tile to the
planners, and the colony digs a stair down on its own when it runs out of
rock to mine. It is the dig-down counterpart of `zoning-auto` (see
[zoning.md](./zoning.md)): the terminal and headless runs have no way to
pick a tile, so the committed `mars-sim.yaml` turns both on, and the browser
lets the player be precise.

## Source

- [`internal/sim/siting.go`](../internal/sim/siting.go): `orderDigDown`
  (every dig-down order goes through it), `sited`, and the checks on a picked
  tile: `siteRefusal` (the rules every site shares), `levelRefusal`,
  `downRefusal` (a stair or a hole), `shaftRefusal`, `ladderRefusal`, and
  `taskAt`.
- [`internal/sim/config.go`](../internal/sim/config.go): `SitingAuto`
  (`siting-auto`).
- [`internal/sim/stairs.go`](../internal/sim/stairs.go),
  [`shafts.go`](../internal/sim/shafts.go),
  [`holes.go`](../internal/sim/holes.go): the commands (`OrderStair`,
  `OrderShaft`, `OrderHole`, `OrderLadder`, each with an `At`), the planners
  that site an unsited order (`planStairs`, `planShafts`, `planHoles`,
  `planLadders`, all through `findStairSite` but ladders), and the
  `designate*` functions both paths end in.
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go): the
  `dig-down` command; with a `level` it is sited at (`x`, `y`) on it.
- [`web/src/ui/DigPanel.svelte`](../web/src/ui/DigPanel.svelte) and
  `armSite`, `orderSite`, `digDownAuto` in
  [`web/src/game.svelte.ts`](../web/src/game.svelte.ts): the Dig tab's
  dig-down tool; `showSitePreview` in [`web/src/main.ts`](../web/src/main.ts)
  tints the tile under the pointer.
- [`internal/sim/siting_test.go`](../internal/sim/siting_test.go): unsited
  orders refused without siting-auto and queued with it, each kind marked out
  at a picked tile and refused where it cannot go, deepening a shaft and
  fitting a ladder by picking, and the colony digging down unasked only with
  siting-auto.

## How it works

### An order and its tile

Each dig-down order carries `At`, the tile to dig from. The zero `Point`
means "no tile": it can never be a site, since level 0 is the surface and
nothing is dug down from there. `orderDigDown` then does one of three
things:

| The order | siting-auto off | siting-auto on |
| --- | --- | --- |
| with a tile | checked at once: marked out, or refused in the log | the same |
| without a tile | refused in the log ("Pick a tile for the stair…") | queued for the planner (`manualStairs`, `manualShaftLevels`, `manualHoles`, `manualLadders`) |

A picked tile is never queued: it is checked and marked out (or refused) the
moment the order arrives, so it can never wait for a site that will not come.
The refusal names the reason, as `No hole at (14, 10): The colony has not
seen that tile.`

### What a picked tile must be

Every kind (`siteRefusal`): a tile on a level that exists, which the colony
has seen, not a doorway, not already marked for building, in the colony's
main room (so a digger can get to it). Then:

- **A stair or a hole** (`downRefusal`): open floor, over rock or floor (so
  it never cuts into a structure below), above a level `deepest-level`
  allows digging to.
- **A shaft** (`shaftRefusal`): open floor starts a new shaft; a shaft's top
  deepens that shaft. Either is cut short at `deepest-level`, and refused
  only if that leaves nothing to dig, or if a wall, fixture or stair is in
  the column's way (`canDigShaft`).
- **A ladder** (`ladderRefusal`): a hole one level deep, open below, that
  the colony can stand beside (`fixtureCutOff`).

Unlike the planner (`findStairSite`), a picked stair or hole needs no open
floor all round it. Whether a stair crowds a corridor, or a hole cuts one in
two, is the player's call.

### What the colony does by itself

With siting-auto on, the planners site unsited orders on the deepest level
the colony has reached (see [stairs.md](./stairs.md), [shafts.md](./shafts.md)
and [holes.md](./holes.md)), and `planStairs` digs a stair down unasked when
there is no mining frontier left on any level. With it off, neither happens.

One thing happens in either mode: a ladder into the hole a colonist fell
down, when it is stranded below (`planLadders`). The hole was already sited
(by the player, or by a cavern's sinkhole); fitting it a ladder is a rescue,
not a siting choice.

### The browser

The Dig tab's *Dig down* section is one row of buttons (Stair, Shaft, Hole,
Ladder), a *Shaft depth* box under them, and, in a siting-auto game, a
*Let the colony pick the tile* checkbox. Unticked (and always, in a game
without siting-auto), each button picks up a one-click tool (`ui.siteTool`,
`armSite`), and a line under the buttons says what to click. While it is
up, the tile under the pointer is tinted green where the page thinks the
order could go (seen open floor; for a shaft also a shaft's top; for a
ladder a hole) and red elsewhere, and a click orders it there on the level
the map shows (`orderSite`), then puts the tool down. Esc, another map tool,
or leaving the tab puts it down without ordering. The page only estimates:
the engine has the last word, and its refusal lands in the log.

Ticked, the same buttons send the order at once without a tile
(`digDownAuto`), and ticking it puts down a tool already in hand. The box
is offered only in a game started with siting-auto (the New game form's
"Colonists choose where to dig down", or `?siting-auto=true`); Hello's
`sitingAuto` says which mode the game is in. It is a toggle on one set of
buttons, not a second row of them, so each kind has exactly one button.

### The terminal

`b` then `v`, `n`, `o`, `u` send unsited orders, as they always did. They
rely on siting-auto, which the committed `mars-sim.yaml` turns on; run with
`-config ""` and they are refused in the log.

## Why it is this way

- **Picked sites exist because the planner's pick was a trap.** Holes were
  first sited like stairs, on the deepest level reached. After a shaft or
  stair down that level is only its foot, so a hole order waited forever,
  and the site search it ran every planning round swept the whole map
  (minutes a round on the browser's 10000x10000; see stairs.md "Why").
  Bounding the search fixed the freeze; letting the player pick fixed the
  order that could never be met, and the player usually knows better where
  a drop or a way down should go.
- **A mode, not just an optional tile.** Picking is the browser's way, and
  auto-siting the terminal's, exactly as with zoning, so the setting and its
  defaults copy `zoning-auto`'s: off compiled in (the browser game), on in
  the committed file (the terminal and headless runs, which cannot pick).
  Off means off: the colony does not dig a stair on its own either, since
  that is siting one. A picked tile is honoured in both modes, as a player's
  zone is used first in auto zoning.
- **The zero Point as "no tile".** An `At *Point` or a separate flag would
  work too, but every command is a plain value the engine copies and the
  save codec walks, and no dig-down site can be on level 0, so the zero
  value is free to mean "unsited".
- **Checked on arrival, refused in the log.** The excavation and clearing
  orders already work this way: the engine has the last word and says why
  in the log. A picked order that waited would need its own pending list,
  re-checks every round and a way to cancel it, for a tile that is either
  good now or needs the player to change something first.
- **The ladder rescue stays automatic.** Turning it off with siting-auto
  would leave a fallen colonist stranded in a game where the player never
  thinks to fit a ladder. It needs no site of its own.
- **Old saves still load.** Only the commands changed shape, and commands
  are not saved; the world's pending counters are as they were.

## Extending it

- A new way down (a ramp, an elevator) gets an `At` on its command, a case in
  `orderDigDown`, a `*Refusal` built on `siteRefusal`, and, if the colony may
  site it, a planner that runs only behind `SitingAuto`.
- A picked-site rule belongs in a `*Refusal` function, with a reason the
  log can show; keep `siteRefusal` to what every kind shares.
- The page's preview (`showSitePreview`) mirrors the cheap half of the
  rules. Keep it an estimate: the engine checks reachability and marks.

## Related

- [zoning.md](./zoning.md): `zoning-auto`, the mode this copies.
- [stairs.md](./stairs.md), [shafts.md](./shafts.md),
  [holes.md](./holes.md): what each kind is, and how the planner sites it.
- [frontend-web.md](./frontend-web.md): the Dig tab.
- [config-file.md](./config-file.md): why the committed file turns it on.

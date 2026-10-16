# Defense zones

> Part of the [mars-sim documentation](./README.md).

## What it is

The player can order an area kept safe. A **defense zone** is a rectangle on
any level and a **detail**: how many armed colonists to keep on station there.
Nothing is built in one. Armed colonists looking for work take its posts
(`JobGuard`), walk into the area, patrol it, and shoot any alien that comes
near it. The colony pays each guard `wage-guard` for every hour on station.
The use it was asked for: an area that is being mined, and the ground round a
shaft or hole that aliens climb out of.

## Source

- [`internal/sim/defense.go`](../internal/sim/defense.go): `OrderDefense`,
  `SetDefenseDetail`, `CancelDefense` (the commands), `defensePost`,
  `tryAssignGuard`, `guardStation`, `patrolStation`, `jobGuard`, `serveGuard`
  (pay), `releaseGuard`, `publishedDefense`, `MaxDefenseTiles`,
  `MaxDefenseDetail`.
- [`internal/sim/systems.go`](../internal/sim/systems.go): `assignWorkJob`
  offers guard duty after the trough and goods already sold and before any
  public work; `clearJob` releases a post, `runJob` runs it.
- [`internal/sim/entity.go`](../internal/sim/entity.go): `JobGuard`, the
  `Guarding` state, and the guard's fields (`guardPost`, `guardDwell`,
  `guardHour`).
- [`internal/sim/activity.go`](../internal/sim/activity.go),
  [`drive_activity.go`](../internal/sim/drive_activity.go): the `guarding`
  activity (drives grow as for ordinary work).
- [`internal/sim/config.go`](../internal/sim/config.go): `wage-guard`,
  `guard-sight`.
- [`internal/wire/zones.go`](../internal/wire/zones.go): `defense` on the
  `zones` topic (areas, for the map) and on the `zoning` topic (details,
  guards, pay, for the tab).
- [`cmd/mars-sim-wasm/main.go`](../cmd/mars-sim-wasm/main.go): `defend`,
  `defend-detail`, `defend-cancel` (`hostAPI` 22).
- [`web/src/ui/ZonesPanel.svelte`](../web/src/ui/ZonesPanel.svelte) (the
  Defense tool and the list), `showZones` and `showDefense` in
  [`web/src/main.ts`](../web/src/main.ts).
- [`internal/sim/defense_test.go`](../internal/sim/defense_test.go).

## How it works

### Ordering one

The Zones tab's **Defense** tool marks a rectangle on the level the map
shows, like the dig and clear tools; the panel takes a detail (1 to
`MaxDefenseDetail`), says what a guard is paid and how many colonists carry
a weapon, and **Post** sends `OrderDefense`. The engine refuses an area on a
level nobody has broken into, one with nothing the colony has seen, or one
over `MaxDefenseTiles`. It costs nothing to post: there is no escrow (see
below). Each zone is listed on the tab with its guards, how many are inside
the area now, what they have been paid, `−`/`+` for the detail, and
**Cancel**. The map tints every defense zone on the level shown red, over
whatever zone is under it, and outlines it in the same red (`setMarks`), on
any tab, fogged ground included. The wash alone was not enough: red over the
red rock, or mixed into a zone's colour, did not read as a defense zone, and
a line round the area does whatever is under it.

### Who guards

Guard duty is work, pulled rather than pushed. An armed colonist
(`bestWeapon`) looking for work in `assignWorkJob` takes the first zone, by
id, that is short of its detail and has a station in its room
(`tryAssignGuard`). That comes after the chicken trough and goods a plan has
already sold, which are a few ticks of work that starve hens or stall a buyer
if left, and before construction and everything else.

A guard leaves its post whenever anything outranks work in focus arbitration:
a pressing drive, or an alien close enough to fight on its own account.
Leaving clears the job, and `clearJob` releases the post, so the next armed
colonist looking for work takes it. A detail therefore staffs itself in
shifts: guards go to eat and sleep and others step in, and a guard comes back
to any post still short once it is free again. A guard that loses its weapon,
or whose zone is cancelled or cut below it, goes back to other work.


**Calm to serve.** Only a colonist whose own safety drive is calm takes a
post, and a guard whose safety reaches pressing gives it up for the next calm
one (`calmEnoughToGuard`). Time on station inside the area is guarding
practice (`SkillGuarding`), and each rank makes a guard slower to feel unsafe.
Bodyguards' escort contracts, which pay more, are offered before posts. See
[safety.md](./safety.md).
### On station

`guardStation` picks the walkable, seen tile of the area nearest the guard,
in its room, that no other guard of the zone is headed for (ties row-major,
so it is deterministic). The guard walks there, holds it in the `Guarding`
state, and after `guardDwellTicks` (or as soon as it is standing somewhere it
would block, `idleWouldBlock`) patrols to a random tile of the area
(`patrolStation`, on the simulation stream: where a guard stands decides
fights). If the way to its station is shut, it picks another; with none
reachable it gives the post up.

Every tick, before moving, a guard looks for an alien within `guard-sight` of
itself whose position is also within `guard-sight` of the area, and fights it
with `fightAlien`, the same gunfight any armed colonist has (see
[combat.md](./combat.md)). `guard-sight` (8) is wider than `flee-radius` (5),
so a guard opens fire on something an ordinary colonist would not yet have
noticed; once the alien is inside `flee-radius`, the ordinary fight focus may
take over, and the guard rejoins a post when the fight is done. The area
bound keeps a guard from chasing something across the colony.

### Pay

`serveGuard` counts the ticks a guard spends inside the area and pays
`wage-guard` (2) every `TicksPerHour`, from the treasury to the guard, as far
as the treasury goes, the way the cook is paid. A guard on an empty treasury
still stands its post. The zone keeps what it has paid, for the tab.

## Why it is this way

- **Not a kind on the zone grid.** The obvious build was a fourth
  `ZoneKind`. It fits badly: the grid holds one kind per tile, so painting
  defense over a residence would unzone it, and the evictions a paint
  implies would order the dormitory being guarded cleared; every kind on it
  implies digging out rock, which is wrong for an area being mined; it covers
  the landing level only, and the shaft mouths worth guarding are below it;
  and a tile kind has no identity to hang a detail on. A defense zone is a
  record of its own, drawn with the zones and offered on the Zones tab.
- **Pulled, not assigned.** Every other job is claimed by a colonist looking
  for work, and doing the same here means a guard's needs, fights and flight
  all work unchanged: guard duty is just work, which drives and threats
  already outrank. A roster that pushed named colonists onto posts would need
  its own rules for when they may leave.
- **Released on leaving.** Keeping a sleeping guard's post held for it would
  leave the area empty for a third of every day. Releasing it lets the
  detail mean "this many on station" rather than "these people".
- **Paid by the hour, not escrowed.** Every other work order escrows its pay
  up front, all or nothing. A detail has no end, so there is nothing to
  escrow; the cook's model (pay as you go, as far as the treasury goes) is
  the one that fits.
- **Only colonists who already carry a weapon.** Fetching a gun from storage
  for a guard is the obvious next step, but nothing yet hands out weapons
  (see [combat.md](./combat.md)); the tab says when the detail is bigger than
  the colony's armed colonists.
- **Golden runs are untouched.** With no defense zone, `tryAssignGuard`
  returns at once and draws nothing from the RNG, so no seed's outcome moves.

## Extending it

- **Arming a guard**: have `tryAssignGuard` accept an unarmed colonist when a
  communal chest holds a weapon, and give `JobGuard` a fetch stage first.
- **Guarding below the landing level automatically**: a shaft or hole
  ordered by the player could offer a defense zone round its mouth.
- **Shifts by schedule** rather than by need, or a guard who may leave only
  for a critical drive, would be a change to `currentFocusEligible` for
  `JobGuard`, not to the post.
- **The TUI** has no tool for it, as it has none for zones.
- **Invariants**: a post never holds more guards than its detail; a
  colonist is on a post's `guards` exactly while it holds `JobGuard` for it
  (only `tryAssignGuard` adds and only `releaseGuard`, through `clearJob`,
  removes); posts are kept and searched in id order, never by map order.

## Related

- [zoning.md](./zoning.md): the Zones tab and the zone overlay defense zones
  are drawn with.
- [combat.md](./combat.md): weapons and `fightAlien`.
- [labor.md](./labor.md): work orders, and pay that is not one.
- [shafts.md](./shafts.md), [holes.md](./holes.md), [depth.md](./depth.md):
  where aliens come up from below.
- [activity-screen.md](./activity-screen.md): the `guarding` activity.

# The meeting hall

> Part of the [mars-sim documentation](./README.md).

## What it is

A walled room of **chairs** the colony commissions, where colonists go to
socialize and to eat. Before it, "socializing" happened wherever a colonist
stood: a colonist whose social need crossed its threshold looked for anyone idle
within `talk-radius`, and if nobody was there it stood and waited, need pinned,
with nothing on the map to say whether it was chatting or only hoping to. In a
6-colonist, 6000-tick run (seed 7, no aliens) colonists spent **5767**
colonist-ticks in the social focus doing nothing, against **2587** actually
talking. With a hall, the waiting halves (2719 ticks) and most conversations and
meals happen in one place you can see on the map.

## Source

- [`internal/sim/hall.go`](../internal/sim/hall.go) — `hallRoom`, `wantsHall`,
  `nearestChair`, `socializeAtHall`, `mealSeat`, `hallTalkBonus`.
- [`internal/sim/food.go`](../internal/sim/food.go) — the `eatWalk` stage of
  `JobEat`.
- [`internal/sim/systems.go`](../internal/sim/systems.go) — `runNeedFocus` tries
  the hall before the old talk; `finishTalk` adds the hall bonus.
- [`internal/sim/project.go`](../internal/sim/project.go) — where the planner
  commissions it, and the `OrderMeetingHall` order.
- [`internal/sim/hall_test.go`](../internal/sim/hall_test.go).

## How it works

**The room.** `hallRoom` is an ordinary recipe (see
[construction.md](./construction.md)): a row of `Chair` terrain along the back
wall, two to four chairs, with two rows of open floor in front. A chair costs
2 raw rock, like a bunk. The planner commissions it as public work, paid from
the treasury, after everything fatal and after bunks and the incinerator, and
before the foundry. Demand is a headcount: one chair per `colonists-per-chair`
(default 4), never fewer than two, so a growing colony adds halls. The player
can order one too (`b`, `m` in the TUI; `OrderMeetingHall`). Setting
`colonists-per-chair` to 0 turns the whole feature off.

**There is no hall record.** A hall *is* its chairs: `Chair` is a tracked
facility terrain (`facilityTiles`), and "in the hall" means within `hallReach`
(2) tiles of one; "seated" means beside one. Nothing is stored that could go
stale when a wall is torn down.

**Socializing.** A colonist whose focus is social (`runNeedFocus`) calls
`socializeAtHall` first:

1. Not beside a chair: walk to the nearest reachable one within `hall-range`
   (default 48) that has fewer than `chairShare` (2) colonists beside it.
2. Seated: pair with the nearest available colonist *who is also in the hall*,
   and chat there. With no one, wait (State `Idle`, focus still social) so the
   next arrival finds them.
3. Anything that fails (no chair free, out of range, route blocked) falls back
   to the old behavior, so a hall that is full or far never leaves anyone
   stranded.

Pairing only hall-goers is what makes the hall a place people meet, rather than
somewhere two people who were already together happened to walk. Opportunistic
idle chats (`talk-chance`) are untouched and still happen anywhere.

A conversation with both partners in the hall gets `hall-talk-bonus` (15)
extra quality points (`hallTalkBonus`), clamped to ±100. It is a flat bonus on
the quality the existing roll produced, so it adds no RNG draw.

**Eating.** A colonist with a meal in hand takes it to a chair and eats there
(`eatWalk`, between `eatFetch` and `eatMeal`), whether it fetched the meal from
a locker or was already carrying one. This also frees the depot's access tile
the moment the meal is out. Two cases eat in place instead: a colonist at
**critical** hunger (the walk is time it may not have; checked again every tick
of the walk), and a meal fetched to keep (`eatKeep`).

## Why it is this way

- **Chairs, not a hall object.** The first idea was a hall rectangle recorded
  at `designateRoom`, as pantries are linked to scumhouses. Making the chair the
  facility removed the record, reused `travelTo` ("walk to beside this tile"),
  `trackFacility` and the construction machinery, and made "sit" mean "be
  adjacent to a chair" for free.
- **Chairs are not claimed.** A claim map needs release in `clearJob` on every
  path out of a job, and a leaked claim silently shrinks the hall. `chairShare`
  spreads a crowd without bookkeeping. The cost: two colonists can end up beside
  the same chair while another stands empty if they arrive in the same tick.
- **The fallback matters.** Without it a hall that was out of range, full, or
  walled off by a collapse froze every social colonist in place.
- **Critical hunger skips the hall.** Hunger runs about 175 ticks from pressing
  to critical and about 40 more to death, so a 48-tile walk with a meal in hand
  is affordable at pressing and fatal at critical.
- **Fewer talking ticks, fewer wasted ones.** In the same run total talking
  fell (2587 → 2087) because walking to the hall takes time, while the colonists
  stuck waiting with no partner dropped by more than half. If talking should
  rise, lower `hall-range` or raise chairs, not the bonus.

## Extending it

- Chairs could matter more: reserved seats (with a claim that is released in
  `clearJob`), a mood bonus for eating seated, or a table fixture that eating
  colonists gather around.
- Idle colonists with nothing to do could drift to the hall too.
- Hall `Activity`: the Activity tab counts waiting-in-hall as socializing; a
  separate "waiting for company" bucket would show how often the hall pays off.

## Related

- [drives.md](./drives.md) — the social need and how conversations meet it.
- [food.md](./food.md) — the eating job the `eatWalk` stage extends.
- [construction.md](./construction.md) — room recipes and the planner.

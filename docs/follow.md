# Following a creature

> Part of the [mars-sim documentation](./README.md).

## What it is

The browser map can keep its camera on one creature as it moves: **Follow** in
the inspector's title row (next to **Find**), for any living colonist, alien,
cat, rat or chicken. While following, the top bar shows a chip, "👁 🧑 Name ×":
the name inspects them, × stops. So you can watch where someone goes without
dragging the map after them.

## Source

- [`web/src/game.svelte.ts`](../web/src/game.svelte.ts) — `ui.follow` (the
  creature's id, or null), `follow()`, `followTo()`, and the stop rules in
  `centerOn` and `setLevel`.
- [`web/src/main.ts`](../web/src/main.ts) — `followCamera()`, run on every
  frame and when `ui.follow` changes; the input's `panned` hook stops it.
- [`web/src/map/input.ts`](../web/src/map/input.ts) — `InputHooks.panned`: a
  drag past the click slop, or an arrow/WASD key. Zooming is not a pan.
- [`web/src/ui/FollowChip.svelte`](../web/src/ui/FollowChip.svelte) — the top
  bar's chip, and the `entity:<id>` subscription that handles level changes
  and death.
- [`web/src/ui/InspectPanel.svelte`](../web/src/ui/InspectPanel.svelte) — the
  Follow / Following toggle.

## How it works

- **Each frame** (`followCamera`) looks the id up in the frame's entity arrays
  and centers the camera on its tile, then `viewChanged()`, which streams new
  pages only when the view crosses a page. Frames carry every entity on the
  level shown, so this costs one scan of an array the marker already scans.
- **Another level.** A frame holds only the level shown, so a creature that
  takes a stair or a shaft just vanishes from it. The chip holds the
  creature's `entity:<id>` topic (250 ms), which carries its level. When that
  differs from `ui.level`, `followTo` switches level (`setLevel(l, true)`,
  keeping the follow) and centers there; the next frames take over.
- **What stops it:** × on the chip or the toggle; a pan (drag or keys); any
  `centerOn` (Find, a log line, a job's tiles: going somewhere else); a level
  change the player makes; the creature dying or leaving the records (from
  the topic); a new or loaded game.
- **What does not:** zooming (wheel, pinch, `[` `]`), closing or switching the
  side panel, inspecting someone else. Following is not the selection: the
  usual reason to follow is to close the panel and watch the map.
- **Under the fog** the camera stays where it was, as the marker hides, or
  following an alien into an unexplored cavern would draw the cavern's shape.

## Why it is this way

- **A pan stops it, rather than fighting it.** Re-centering every frame while
  the player drags makes the map jitter between their hand and the creature.
  Only a drag past the 5-pixel click slop counts, or every click on the map
  (which inspects) would end the follow.
- **Level changes come from the topic, not the frame.** The worker streams one
  level at a time, so the page cannot see a creature on another level in a
  frame. Asking the worker for a second level just to track one creature
  would double the frame; the inspector's topic already says where it is.
- **`centerOn` stops following** because every caller is a "show me that"
  link. Leaving the follow on would yank the camera back on the next frame.
  The follow code itself moves the camera through `ctl.centerOn` directly,
  which does not stop it.
- **Centered on the canvas**, like Find, not on the part the side panel leaves
  uncovered. With the panel open, the creature sits a little left of the
  middle of the visible map.

## Extending it

- A follow button elsewhere (a roster row, the tile inspector's creature list)
  is a call to `follow(id)`; the chip and the camera need nothing more.
- Anything new that moves the camera on the player's behalf should go through
  `centerOn` (which stops following) or, if it is part of following, through
  `ctl.centerOn`.

## Related

- [frontend-web.md](./frontend-web.md) — the inspector, the marker, and the
  camera this drives.
- [z-levels.md](./z-levels.md) — the levels a followed creature moves between.
- [fog-of-war.md](./fog-of-war.md) — why it stops at the fog.

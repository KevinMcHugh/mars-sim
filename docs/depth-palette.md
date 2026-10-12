# Depth palette

> Part of the [mars-sim documentation](./README.md).

## What it is

The browser map darkens as the player steps down a level. Level 1 (the landing
level) keeps the original reds; by level 5 the fog is black and the floor is
charcoal, and every level below that stays fully dark. It is purely a display
choice: nothing in the simulation reads it.

## Source

- [`web/src/map/palette.ts`](../web/src/map/palette.ts) — `DARK_LEVEL`, `depthDarkness`, `depthPalette`, and the deep colors (`DEEP_FOG`, `CHARCOAL`, `DEEP_BACKDROP`, `DEEP_DIM`).
- [`web/src/map/renderer.ts`](../web/src/map/renderer.ts) — `Renderer.setDepth`, called from `reset` (the landing level) and from `applyFrame` whenever a frame's `level` differs from the one the ground is colored for.
- [`web/src/map/palette.test.mjs`](../web/src/map/palette.test.mjs) — darkness is monotonic, level 1 is unchanged, level 5 ends black and charcoal, and an unknown terrain stays magenta.

## How it works

`depthDarkness(level)` is `(level - 1) / (DARK_LEVEL - 1)` clamped to 0..1, so
levels 1..5 step through 0, ¼, ½, ¾, 1. `depthPalette` turns that into the
terrain program's uniforms:

| Ground | Level 1 | At `DARK_LEVEL` |
| --- | --- | --- |
| Fog (unexplored, and the map's own area before tiles arrive) | `FOG` `#2e0d0b` | black |
| Floor | `#f78765` | charcoal `#36322f` |
| Backdrop under a facility glyph | `GLYPH_BACKDROP` | `#2a2725`, a shade under charcoal |
| Everything else (rock by composition, walls, stairs, shafts, …) | its color | scaled to `DEEP_DIM` (45%) |

Each is a straight linear mix between the two columns. The renderer re-uploads
the four uniforms only when the frame's level changes, which is one
`uniform3fv` each — nothing per tile, no texture work.

## Why it is this way

- **The level comes from the frame, not from `ui.level`.** Every frame says
  which level it is of, and its tiles are that level's. Keying the colors off
  the frame means the ground can never be painted in one level's palette with
  another level's tiles, whatever order a level change and a stale frame arrive
  in (main.ts already drops frames of the old level).
- **Only the ground darkens.** Creatures, corpses, glyphs, filth, zones, the
  flow overlay and the selection ring keep their colors. Dimming them too
  would make the deep levels — the dangerous ones — the hardest to read; on a
  dark floor they stand out more, which is the point.
- **Floor and fog get named targets; the rest is a dim.** The two the eye reads
  as "the cave" are pinned to exact colors so the deep look is a deliberate
  black and charcoal rather than whatever a multiplier makes of salmon. Rock
  and structures keep their hue so ore compositions stay tellable apart.
- **Absolute level, not distance from landing.** `DARK_LEVEL` is level 5, not
  "four below wherever the ships land". Level 0 (the surface, if it ships)
  clamps to the landing palette.
- **A terrain with no color stays magenta** at every depth, so a missing entry
  in `terrains` is still loud.

## Extending it

- Change how fast it gets dark with `DARK_LEVEL`; change the end state with the
  `DEEP_*` constants and `CHARCOAL`. The test pins only "monotonic, black fog,
  dark near-grey floor", so retuning the shades needs no test change.
- A new terrain darkens automatically once it has a color in `terrains`. If it
  should go to a specific color instead (as floor does), add a case beside
  floor's in `depthPalette`.
- The TUI does not do this: its floor is a blank cell and its fog a terminal
  background colour, so there is nothing comparable to darken.

## Related

- [depth.md](./depth.md) — what the levels mean for gameplay.
- [fog-of-war.md](./fog-of-war.md) — what the fog is.
- [frontend-web.md](./frontend-web.md) — the renderer this lives in.

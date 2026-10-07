# Sprite Designer

> Part of the [mars-sim documentation](./README.md).

## What it is

A Scum Lab tab for drawing map sprites with Claude. You paste a Claude API key, describe a creature, and revise the SVG over several turns. The tab previews it at real tile sizes on the map's own colors, and you can download it as SVG or as a 128px PNG. Nothing in the game reads these sprites yet. The tab produces art for when the map stops drawing emoji. See "Using a sprite in the game" below.

## Source

- [`tools/scum-lab/tools/sprites.js`](../tools/scum-lab/tools/sprites.js): the whole tool. It holds the system prompt, the API call, SVG extraction, lint, previews, and export.
- [`tools/scum-lab/scum.css`](../tools/scum-lab/scum.css): the `.sprite-*` rules.
- [`tools/scum-lab/tools/registry.js`](../tools/scum-lab/tools/registry.js): where it joins the nav.

## How it works

- **Key and billing.** The page calls `https://api.anthropic.com/v1/messages` straight from the browser. The key comes from the Claude Console and bills per token there. A claude.ai subscription does not cover these calls. The request needs the `anthropic-dangerous-direct-browser-access: true` header, or the API refuses the cross-origin call. The key is kept in `localStorage` (`scum-lab.sprites.key`) and goes out only in the request header. "Forget key" removes it.
- **Model.** Opus 5.5 by default. Sonnet 5.5 and Haiku 4.5 are cheaper choices. Opus and Sonnet take an `effort` level (the default here is medium) and send `fallbacks: "default"` with the `server-side-fallback-2026-07-01` beta. A safety-classifier refusal then retries on another model instead of coming back empty. Haiku gets neither, because it rejects both. The running cost line uses list prices from the `MODELS` table. Update those prices when they change.
- **Conversation.** One sprite is one multi-turn conversation. Each assistant turn is stored exactly as returned, thinking blocks included. The next request replays it unchanged, which current models require. The tool takes the SVG from the reply's ```svg fence, or else the first `<svg>…</svg>`. Each reply becomes a version (v1, v2, …).
- **Field notes.** Paste a species' field notes from the game's Lore panel (`AlienSpecies.Description` in `internal/sim/lore.go`). They are sent inside `<field_notes>` tags with instructions on how to read them: count eyes, arms, legs, wings and tail exactly; use the stated covering and color; show temperament in the pose and the attack in the body part that delivers it; treat height and weight as build only, because every creature fills one tile. The notes go out with the first message and again only when you edit them (`notesSent`), since the conversation already carries them. With notes pasted, the description box is optional. The sprite name defaults to the species' plural, taken from "Grelks stand …" or "The feared Grelks stand …".
- **Working from an older version.** If the selected version is not Claude's latest (you clicked an older one, or used "Use my edit"), the next message includes that SVG and says to work from it. Claude then edits what you see, not what it last wrote.
- **Preview.** The sprite is shown at 16, 32 and 64 px on floor, rock and fog (colors copied from `web/src/map/palette.ts`), next to the emoji it replaces. 16px is where most designs fall apart.
- **Lint.** These warnings are advice, not a gate: no square `viewBox` at `0 0`; scripts or event handlers; `<foreignObject>`; external `href`/`url()`; `<text>`; very large files.
- **Export.** Download SVG, download a PNG at 128px (`ATLAS_CELL`, the map atlas's cell size), or copy the source. The file name comes from "Sprite name".
- **Persistence.** The session (versions, conversation, cost) is kept in `localStorage` (`scum-lab.sprites.session`). It survives reloads and tab switches. "New sprite" clears it. Every storage call is wrapped in try/catch, so a private window still works for the life of the page.

## Why it is this way

- **Raw `fetch`, not the Anthropic SDK.** Scum Lab is native ES modules with no bundler or npm step (see [scum-lab.md](./scum-lab.md)). One POST does not justify adding a package manager to the lab.
- **The SVG is untrusted text.** It is only rendered through `<img src="data:image/svg+xml,…">`, where scripts don't run, and its source only through `textContent` and textarea values. Do not inline it with `innerHTML` to "get better rendering". That turns a model's reply into script on the page that holds your API key.
- **No streaming.** SVGs are small, so the request is a plain JSON call with `max_tokens: 16000` and an elapsed-seconds counter. Streaming would mean an SSE parser for the sake of watching XML appear.
- **Square 128 viewBox, transparent background, thick strokes.** The map atlas draws each glyph into a 128px cell (`web/src/map/atlas.ts`) and shows it as small as 16 CSS px. The system prompt asks for what survives that. The [favicon note in scum-lab.md](./scum-lab.md) is the same lesson learned by hand.
- **The tool does not need the WASM module.** The shell still waits for `bootSim()` before mounting any tool, so run `tools/scum-lab/build.sh` first, as for the other tabs.

## Using a sprite in the game

The game cannot use these files yet. The browser map draws each symbol into a sprite sheet ([frontend-web.md](./frontend-web.md), `buildAtlas` in `web/src/map/atlas.ts`). Supporting sprites means:

1. Map an emoji to an SVG on the frontend, and in `buildAtlas` draw that SVG into the cell (`drawImage` after the image decodes) instead of `fillText`. The atlas build becomes async.
2. Keep the emoji as the fallback. The terminal UI stays on emoji, and so does a custom `-alien-names` file with no art.
3. For art per *species* rather than per emoji, the wire has to say which species an alien is. Several species can roll the same emoji ([lore.md](./lore.md)).

"Stands in for" in the tab records which emoji a sprite replaces, for that mapping.

## Extending it

- If `AlienSpecies.Description` changes its opening ("<Plural> stand …"), update `speciesName`. Only the default sprite name depends on it.
- New model: add a row to `MODELS` with its prices and whether it takes `effort` and `fallbacks`.
- Change house style (palette, outline, view angle) in `SYSTEM_PROMPT`. Keep the "one svg fence, whole file" output contract, because `extractSVG` depends on it.
- New backdrop or tile size: edit `BACKDROPS` / `TILE_SIZES`. Keep backdrop colors in step with `web/src/map/palette.ts`.

## Related

- [scum-lab.md](./scum-lab.md): the shell this tab mounts in.
- [frontend-web.md](./frontend-web.md): the atlas and renderer a sprite would feed.
- [lore.md](./lore.md): alien species and their rolled emoji.
- [creature-lab.md](./creature-lab.md): the server that keeps a catalog of species with a sprite per lifecycle form, drawn over MCP with the same house style.

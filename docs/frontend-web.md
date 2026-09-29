# Frontend: the browser map

> Part of the [mars-sim documentation](./README.md).

## What it is

The game in a browser tab: the engine as WASM in a Web Worker, and a WebGL2 map
on the page that draws its frames. Right now that is a map in flat colors when
zoomed out and in the TUI's emoji when zoomed in, pan and zoom, a hover
readout, pause and speed, and a new-game form. There are no panels
yet (roster, job board, lore). The plan it is part of is
[browser-frontend.md](./browser-frontend.md), and the messages it reads are
[wire-format.md](./wire-format.md).

## Source

- [`web/index.html`](../web/index.html), [`web/src/main.ts`](../web/src/main.ts)
  — the page: boot, the HUD, the new-game form, hover, the view it sends the
  worker.
- [`web/src/map/atlas.ts`](../web/src/map/atlas.ts) — the emoji atlas.
- [`internal/glyphs/glyphs.go`](../internal/glyphs/glyphs.go) — which emoji a
  terrain or entity draws as; shared with the TUI.
- [`web/src/map/renderer.ts`](../web/src/map/renderer.ts) — `MapRenderer`: the
  terrain textures, the shaders, and the entity and refuse sprites.
- [`web/src/map/camera.ts`](../web/src/map/camera.ts) — the camera: a center in
  tiles and a zoom in pixels per tile.
- [`web/src/map/input.ts`](../web/src/map/input.ts) — drag, wheel, pinch and
  keyboard.
- [`web/src/map/palette.ts`](../web/src/map/palette.ts) — the flat colors, by
  name.
- [`web/src/sim/client.ts`](../web/src/sim/client.ts) — `SimClient`, the page's
  side of the worker.
- [`web/public/worker.js`](../web/public/worker.js) — the worker's host loop
  (see [browser-frontend.md](./browser-frontend.md), "The engine in a worker").
- [`web/build-wasm.sh`](../web/build-wasm.sh) — builds `cmd/mars-sim-wasm` into
  `web/public/`.
- [`web/spike/index.html`](../web/spike/index.html) — the measurement page, on
  the same worker.

**Running it:** from `web/`, run `npm install` once, then `npm run wasm` (again
after any Go change) and `npm run dev`. That serves the game on
<http://localhost:5173/> and the spike on `/spike/`. The URL takes new-game
settings: `?width=2000&height=2000&seed=7&fog-of-war=false`. `npm run build`
writes a static site to `web/dist/`. `npm run check` type-checks and `npm test`
runs the wire decoder's tests. All of these need Go on the path.

**Hosting:** [`.github/workflows/pages.yml`](../.github/workflows/pages.yml)
publishes it to GitHub Pages, at <https://kevinmchugh.github.io/mars-sim/>,
with the spike at `/mars-sim/spike/`. It builds on every push to `main` that
touches `web/`, the engine or the wire, and can be run by hand from the Actions
tab. It runs `npm ci`, `npm test` and `npm run build`, then deploys `web/dist`.
The one-time setup is in the repo settings: **Pages → Build and deployment →
Source: GitHub Actions**. The build needs nothing special for the subpath:
Vite's `base: './'` keeps every URL relative, and the worker is found from
`document.baseURI`. Tested by serving `web/dist` under `/mars-sim/`, as Pages
does.

## How it works

**Terrain** lives in *chunk textures*: `RG8UI`, 2048×2048 tiles each, which is
32×32 wire pages. A chunk texture is created only when a page inside it
arrives, and WebGL zero-fills it, and zero reads as unexplored rock. Each page
is one `texSubImage2D` of its 64×64 two-byte tiles, straight from the frame's
buffer. Drawing is one quad per chunk on screen. The fragment shader
`texelFetch`es the tile and colors it:

- fog if the *visible* bit is clear;
- else the rock composition's color for rock;
- else the terrain's color.

At 14 or more pixels a tile, it also adds a faint grid. So the cost of a frame
is the number of pixels, not the number of tiles.

**Entities and refuse** are instanced quads: one draw each, with per-instance
tile positions (`int`) and a palette index (`uint`). They are uploaded fresh
each frame from the frame's typed arrays. Entities are round with a dark rim so
they read against any floor. Refuse is small squares, red for gore and bone-white
for a body. Both stay at least a few pixels across when zoomed out.

**Occupants of unseen tiles are not drawn.** An entity or refuse on a tile that
is not visible, or on a page not held, is filtered out before upload. Otherwise
a dormant alien would give away the undiscovered cavern it sleeps in, and the
TUI never draws an occupant on an unexplored tile
([fog-of-war.md](./fog-of-war.md)). Refuse arrives only when it changes, but
what is visible changes as the colony digs, so the renderer keeps the last list
and refilters it every frame.

**Glyphs, zoomed in.** From 10 CSS pixels a tile (`GLYPH_ZOOM`) and up, the
map draws the same emoji as the TUI:

- **The atlas.** `atlas.ts` draws every glyph in `Hello.glyphs.symbols` once,
  with the browser's emoji font, into 128-pixel cells of one canvas. It uploads
  that as a premultiplied, mipmapped texture.
- **Terrain.** A terrain with a glyph (a bed, a wall, a forge) draws its emoji
  over a floor-colored backdrop, straight from the atlas in the terrain
  shader.
- **Swatches.** A terrain whose TUI glyph is only a colored square
  (`glyphs.Swatch`: rock of every composition, floor, hull) keeps its flat
  color. The squares are a terminal's way of drawing a color, and a browser
  has real ones.
- **Entities and refuse** draw their emoji instead of dots. An entity's glyph
  index comes in the frame, picked in Go by `glyphs.ForEntity`: 👨 👩 🧑 👴 by
  gender and age, 🧟 for a mutant, 😱 🔫 🧹 📦 by what it is doing, and each
  alien species' own emoji. So the choice is the TUI's exactly, from data the
  page never gets.

Zoomed further out, a glyph would be a few pixels of mush, so the flat colors
and dots stay.

**Colors are keyed by name**, from Hello's enum lists, not by value. So
renumbering an enum in Go cannot recolor the map, and a new terrain with no
color here shows up magenta.

**The view.** The camera is a center in tiles and a zoom in CSS pixels per tile
(1 to 64). On every camera change the page works out the visible tiles plus one
page of margin, and sends it to the worker as the interest rectangle, but only
when the set of pages it covers changed. The worker streams the pages in
(nearest the middle first, 64 a frame). The page keeps a CPU copy of every
page it holds, for the hover readout.

**Redraws** happen on `requestAnimationFrame`, and only when something changed:
a frame arrived, or the camera moved. A paused game with a still camera draws
nothing.

## Why it is this way

- **WebGL2 from the start, not canvas 2D.** At 10000×10000 the map has to zoom
  out, and per-tile draw calls are what fall over there. Zoomed out to 1.5 px a
  tile, a single frame covers about 850×530 tiles (176 pages). It still draws
  as a handful of quads.
- **`textureGrad` for glyphs on terrain.** A tile's position in its atlas
  cell is `fract()` of the tile coordinate, which jumps at every tile edge.
  With implicit derivatives, the GPU picks a tiny mip level there and draws a
  seam around every tile. The gradients are taken from the unbroken tile
  coordinate before any branch.
- **Integer textures and `texelFetch`**, not a color texture. A tile's colors
  are decided in the shader from uniform palettes, so the upload is the wire's
  own bytes with no conversion. Changing a color is a uniform, not a
  re-upload. Glyph art will be an atlas lookup in the same place.
- **2048² chunks, lazily.** WebGL2 guarantees only a 2048 maximum texture size.
  One texture for a 10000² map would be too big for some GPUs and 200 MB for
  everyone. Chunks cost 8 MB each, only where the colony has been looked at.
- **Measured** (headless Chromium, software GL):
  - The renderer's own work is about 0.1 ms to draw and 0.1–0.2 ms to apply a
    frame.
  - With the game paused and the camera moving, it holds 60 fps.
  - With the sim flat out, it drops to about 30 fps. That is the container's
    CPU shared between the worker and the *software* rasterizer (SwiftShader):
    a bare WebGL clear alone holds 60 fps, and the renderer's JS is the 0.2 ms
    above.
  - Glyphs add nothing measurable: on a slower container, flat colors
    (zoom 9.9) and glyphs (zoom 12) both drew at about 33 ms a frame, the
    software rasterizer's ceiling there.
  - Check it on a real GPU before optimizing anything here.
  - The production JS is 16.5 KB (6.5 KB gzipped), plus the 7 MB WASM.
- **The worker is a classic script in `public/`**, not a Vite module worker:
  Go's `wasm_exec.js` is loaded with `importScripts`, and both pages (the game
  and the spike) share the one worker and the one WASM.

## Extending it

- **A new glyph**: add it in `internal/glyphs` (see
  [frontend-tui.md](./frontend-tui.md)); the page gets it through the Hello
  with no change here. A new terrain whose glyph is a picture draws it
  automatically; one that should stay a flat color belongs in
  `glyphs.Swatch`.
- **Cave scum** (the TUI's 🟢) is not drawn yet: `Snapshot.Scum` is not on the
  wire. It is a sparse overlay like refuse, so it would travel the same way.
- **A new terrain or kind**: add its color to `palette.ts` by the name its
  `String` method gives. Magenta on the map means one is missing.
- **Picking** (click to select): the hover code already finds the entity on a
  tile from the last frame's arrays. Selection needs the worker to send the
  entity's details (a topic; see [browser-frontend.md](./browser-frontend.md)).
- **Evicting pages**: nothing is dropped yet. A long session that pans across a
  whole 10000² map would end up holding it all, both on the GPU (in chunks)
  and on the CPU (the hover copies). An eviction needs a message telling the
  worker, so it stops counting the page as held.
- **Panels**: Svelte mounts beside the canvas and never touches the renderer's
  state (see [browser-frontend.md](./browser-frontend.md), "The other views").

## Related

- [browser-frontend.md](./browser-frontend.md) — the plan, the worker, and the spike's measurements.
- [wire-format.md](./wire-format.md) — the frames this draws.
- [frontend-tui.md](./frontend-tui.md) — the terminal frontend it is catching up to.
- [fog-of-war.md](./fog-of-war.md) — what the visible bit means.

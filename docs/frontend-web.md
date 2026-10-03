# Frontend: the browser map

> Part of the [mars-sim documentation](./README.md).

## What it is

The game in a browser tab: the engine as WASM in a Web Worker, and a WebGL2 map
on the page that draws its frames. Right now that is a map in flat colors when
zoomed out and in the TUI's emoji when zoomed in, with pan, zoom and a hover
readout. Around it is a Svelte chrome:

- **A top bar:** the clock (colony day and time of day, see [days.md](./days.md), then tick), a Pause / Normal / Fast / Faster / Max speed selector, and
  the TUI header's counts, as emoji (👷 👽 🐈 🐀, then each fixture's glyph).
- **A side panel,** whose tab strip has two labelled rows. **View** holds the
  tabs that show the colony: the Inspect tab (click the map), the Roster, the
  Log, Jobs, Storage, Market, Charts and the Lore tab. **Act** holds the tabs
  whose job is to change it: Zones, Dig, Ships and a new-game form (which can
  also start a colony that zones for itself: `zoning-auto`). Market stays
  under View although it hosts the colony's orders: it is mostly prices, and
  splitting one tab across both rows would make the line meaningless.
- **A log ticker** over the map's bottom-left corner: the last few colony-log
  lines, fading after a few seconds.

The rest of the TUI's tabs are planned in
[browser-frontend.md](./browser-frontend.md), "Parity with the TUI". The plan it is part of is
[browser-frontend.md](./browser-frontend.md), and the messages it reads are
[wire-format.md](./wire-format.md).

## Source

- [`web/index.html`](../web/index.html), [`web/src/main.ts`](../web/src/main.ts)
  — the page: boot, the map and the worker, keyboard shortcuts, hover, and the
  view it sends the worker.
- [`web/src/game.svelte.ts`](../web/src/game.svelte.ts) — where the map side
  meets Svelte: the UI state (`ui`), topic payloads (`topics`), and the
  actions panels take (`subscribe`, `setSpeed`, `newGame`).
- [`web/src/ui/`](../web/src/ui/App.svelte) — the Svelte chrome: `App`,
  `TopBar`, `SpeedControl`, `SidePanel`, `Bar` (a gauge), and one component
  per tab (`InspectPanel`, `RosterPanel`, `LogPanel`, `JobsPanel`,
  `StoragePanel`, `MarketPanel` with `AccountDetail` and `ColonyOrders`, `ZonesPanel`, `DigPanel`, `ChartsPanel`, `ShipsPanel`,
  `LorePanel`, `NewGamePanel`), `Section` (a foldable heading), `LogTicker`, and `FlowControl` (the
  flow-field picker and legend, see [flow-field-view.md](./flow-field-view.md)).
  `format.ts` formats money.
- [`web/src/ui/charts/`](../web/src/ui/charts/Chart.svelte) — the Charts tab:
  `Chart` (one uPlot chart with a tooltip), `PerfCharts`,
  `PopulationCharts`, `ActivityChart`, `theme.ts` (colors and the activity
  bands), and `population.ts` (the activity arithmetic, tested by
  `population.test.mjs`). `logkinds.ts` colors the log's types. `inspect.ts` types the
  inspector's payloads.
- [`web/src/speed.ts`](../web/src/speed.ts) — the speed selector's steps.
- [`web/src/settings.ts`](../web/src/settings.ts) — new-game settings from the
  URL.
- [`web/src/map/atlas.ts`](../web/src/map/atlas.ts) — the emoji atlas.
- [`internal/glyphs/glyphs.go`](../internal/glyphs/glyphs.go) — which emoji a
  terrain or entity draws as; shared with the TUI.
- [`web/src/map/renderer.ts`](../web/src/map/renderer.ts) — `MapRenderer`: the
  terrain textures, the shaders, the entity and body sprites, and the filth
  tint.
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

**Running it:** from `web/`, run `npm install` once, then `npm run dev`, which
rebuilds the WASM first (`predev`), then serves the game on
<http://localhost:5173/> and the spike on `/spike/`. The URL takes new-game
settings: `?width=2000&height=2000&seed=7&fog-of-war=false&zoning-auto=true`. `npm run build`
writes a static site to `web/dist/`. `npm run check` type-checks and `npm test`
runs the wire decoder's tests. All of these need Go on the path. After a Go
change with the dev server already running, run `npm run wasm` and reload.

**A stale WASM says so.** The WASM's exports carry a version (`hostAPI` in
`cmd/mars-sim-wasm/main.go`, `HOST_API` in `web/src/sim/client.ts`, bumped
together). The page checks it at start and, on a mismatch, shows "mars-sim.wasm
is out of date … rebuild it". It used to fail silently: a page newer than its
WASM called an export that did not exist, the error vanished inside the
worker's async message handler, and the Lore panel sat on "Loading…". The
worker now reports every failure to the page as well.

**Keys:** space toggles pause; `+` and `-` step the speed selector (Pause,
Normal, Fast, Faster, Max); arrows or WASD pan; `[` and `]` zoom; `f` steps the
flow-field overlay through each field and back to off, and `F` turns it off
(the top bar's *Flow* picker does the same, see
[flow-field-view.md](./flow-field-view.md)); Escape closes the side panel. A
click (or tap) on the map inspects what is there.

**The hover readout** names the tile under the pointer and who is on it. A
colonist is named from the `names` topic, which `main.ts` subscribes to for
the page's whole life: frames carry only ids. Other creatures read as
"alien #7", as in the TUI. With a flow field shown, it adds the tile's
distance to the field's goal ("mining frontier: 4 steps"), or that a floor
tile cannot reach one.

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

**Entities and bodies** are instanced quads: one draw each, with per-instance
tile positions (`int`) and a palette index (`uint`). They are uploaded fresh
each frame from the frame's typed arrays. Entities are round with a dark rim so
they read against any floor. A body is a small bone-white square (🦴 zoomed
in). Both stay at least a few pixels across when zoomed out.

**Filth is a tint, not a symbol.** Viscera (the refuse list's gore) and cave
scum (the scum list) color the tile they are on:

- **dark red** for gore;
- **dark green** for scum;
- **brown** for a tile with both.

A **salt** deposit (the salt list) is a pale wash through the same quads. It is
not filth and carries no amount, so it is one fixed tint (`SALT_TINT`). It
never shares a tile with scum, and gore on a salt tile shows as gore.

The more there is (gore out of `Hello.goreMax`, scum out of `Hello.scumMax`),
the deeper the color. One unit reads plainly, and a full tile is nearly
solid. The tint is a premultiplied full-tile quad per dirty tile, drawn between
the terrain and the sprites, so it works the same at every zoom and under
glyphs.

Scum is seeded on rock, and mining a scummy tile leaves the patch on the new
floor (see [scumhouse.md](./scumhouse.md)). So the green lands on both: cave
walls facing open floor, and the floor of anything dug through them. Viscera's
red lands mostly on floor. A tinted rock tile hides its ore color, and the
hover readout still names it. Uranium rock is drawn a bright yellow-green, so
it can't be mistaken for scum. Bodies keep their marker, because a body is something to
haul away, not a stain.

This replaced the TUI's 🟢 and 🩸 in the browser: a symbol per tile turned a
scum-lined cave into a field of dots, where a wash of color reads as a place.

**Occupants of unseen tiles are not drawn.** An entity, a body or a stain on a
tile that is not visible, or on a page not held, is filtered out before upload.
Otherwise a dormant alien would give away the undiscovered cavern it sleeps in,
and the TUI never draws an occupant on an unexplored tile
([fog-of-war.md](./fog-of-war.md)). Refuse, scum and salt arrive only when
they change, but what is visible changes as the colony digs. So the renderer
keeps the last lists and refilters them whenever a list or a page arrives.

**Glyphs, zoomed in.** From 10 CSS pixels a tile (`GLYPH_ZOOM`) and up, the
map draws the same emoji as the TUI, except that colonists wear their own
skin tone and hair ([colonist-looks.md](./colonist-looks.md)):

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
- **Entities and bodies** draw their emoji instead of dots. An entity's glyph
  index comes in the frame, picked in Go by `glyphs.ForEntity`: 👨 👩 🧑 👴 by
  gender and age, 🧟 for a mutant, 😱 🔫 🧹 📦 by what it is doing, and each
  alien species' own emoji. So the choice is the TUI's exactly, from data the
  page never gets — save that a resting colonist's index names their look
  (👨🏿‍🦰 rather than 👨), which `withLooks` appended to `symbols`.

Zoomed further out, a glyph would be a few pixels of mush, so the flat colors
and dots stay.

**The chrome is Svelte; the map is not.** Frames arrive at up to 60 a second
and go straight from the worker to the renderer. What the chrome shows (tick,
stats, pause and speed) is copied into `ui` in `game.svelte.ts` at most
`UI_HZ` (10) times a second, and at once when pause flips, so the DOM does not
re-render per frame. Panels get their data from topics: a tab component calls
`subscribe('lore')` in an `$effect`, whose cleanup unsubscribes. So closing
the panel stops the worker building that data. Subscriptions are
reference-counted, and payloads are `$state.raw`, replaced whole, never
deep-proxied.

**The inspector.** A press and release that moves less than 5 pixels, with no
second finger down, is a click (`input.ts`); anything more is a drag. A click
selects (`ui.selected`, via `inspect()` in `game.svelte.ts`) and opens the
Inspect tab:

- **What a click picks.** The creatures on that tile in turn, then the tile
  itself, so repeated clicks step through a crowd and end on the ground under
  it. Only creatures the colony can see count, from the last frame's arrays, as
  the hover does.
- **What it shows.** `InspectPanel` subscribes to `entity:<id>` or
  `tile:<x>,<y>` (see [wire-format.md](./wire-format.md), "Topics"). A
  colonist gets the TUI roster inspector's sections: identity, status, health
  and the three affect axes, body parts, needs, inventory, traits, skills
  (each rank as a bar out of the skill's top rank, with its label; the
  profession starred, and its title under the name), family, affinities, and
  every memory. Other creatures get status, health and body. A
  tile gets its terrain, its fixture's owner and access, a container's contents
  and ledger, and who stands on it. Names (kin, acquaintances, an owner, a
  creature on a tile) are links that inspect them in turn; **Find** centers the
  map on the creature.
- **The marker** is a square ring on the selected tile, or on the selected
  creature wherever the latest frame puts it (`MapRenderer.setMark`, its own
  small shader). It is at least 14 CSS pixels across, so it still rings a dot
  zoomed out. It hides while the creature is under fog, as the creature does.
- **Closing the Inspect tab** (or switching tabs) drops the selection and the
  marker, and unmounting the panel unsubscribes its topic.

**The roster** lists the colony from the `roster` topic, by ID, three lines a
row as in the TUI: name, pronouns and age (or an alien's species), and what it
is doing with its mood. A hurt creature shows its health.

- **Filters.** *Dead* and *Non-human* are the TUI's `f` filters. They pick the
  topic (`roster:dead,nonhuman`), so the Go side does the filtering, and they
  are kept in `ui` while the tab is closed. *Find by name* filters the rows in
  the page, by name or info line.
- **Virtualized.** Rows are a fixed 62 px, and only those on screen, plus six
  either side, are in the DOM. A long game lists every dead colonist, and the
  rows are replaced twice a second, so rendering all of them would be the
  cost of the tab.
- **A row opens the inspector,** which then offers *← Roster*. Going back
  keeps the selection, so the row stays highlighted and the map keeps its
  marker; switching to any other tab drops it. Links inside the inspector keep
  the way back.

**The log** is built up in the page (`colonyLog` in `game.svelte.ts`). The
engine keeps only the last `-log-size` lines (64), so the `log` topic sends
only lines newer than it last sent, numbered by `LogEntry.Seq`, and the page
appends them and keeps the last 2000 (`LOG_KEEP`). A new game's first send
says `reset`, and the page starts over. `main.ts` subscribes to it for the
page's whole life, like `names`, because the ticker needs it with the tab
closed.

- **The Log tab** shows each line with its type (colored), its tick, and the
  sentence, wrapped, never cut: the TUI found a cut sentence reads as a
  finished one. It follows the tail while scrolled to the bottom; scrolled
  back, it stays put and offers "↓ N new". A type menu and a search filter
  it. Lines use `content-visibility: auto` rather than a virtual list:
  wrapped lines have no fixed height, and the browser skips laying out the
  ones off screen.
- **The ticker** (`LogTicker.svelte`) shows the last four lines for 12
  seconds after they arrive, fading out; a click opens the Log tab, and it
  hides while that tab is open. Its fade timer stops once the newest line has
  faded, so a quiet colony runs no timer.
  - **×** hides it, remembered in this browser (`ui.ticker`, saved to
    `localStorage` as `mars-sim.ticker`, every access guarded so a blocked
    storage just forgets). *Show new lines on the map* in the Log tab brings
    it back.
  - **On a phone** it floated over the bottom sheet's tab strip, covering the
    tabs. So it is mounted twice, and CSS shows one: floating (App) on a wide
    screen, docked inside the side panel just above the tabs (SidePanel) on
    a phone. Docked, it shows the two newest lines, one line each, and hides
    while a tab's sheet is open.

**Foldable sections.** The Inspect, Market and Lore tabs are long lists of
headed parts (Skills, Affinities, Books, Alien species…), and most of the
time you only read a few of them. Each heading is a `Section`
(`ui/Section.svelte`): a click folds it shut (▸) or opens it (▾). Folded
sections are remembered in this browser (`ui.collapsed`, saved by
`toggleSection` to `localStorage` as `mars-sim.collapsed`, guarded like the
ticker's), until they are opened again.

- **Keyed by section, not by subject.** Ids are `<tab>.<section>`
  (`inspect.skills`, `market.books`), so folding Skills on one colonist
  folds it on every colonist and across reloads and new games. A
  per-colonist fold would be forgotten the moment you clicked someone else,
  which is exactly when you want it to stick.
- **A folded section is not rendered**, not just hidden, so a folded
  Memories or Books costs nothing while the topic keeps updating. Its title
  still carries any count (`Memories (12)`), so folded still tells you
  something.
- The heading lives inside `Section`, so its style does too: the panels'
  own `h2`/`h4` rules are scoped and would not reach it. A new foldable
  part is `<Section id="tab.part" title="…">…</Section>` (`tag="h2"` for a
  tab's top-level parts).
- Not folded: forms and tool tabs (Zones, Dig, Ships, Colony orders), where
  hiding a part would hide a control.

**The list tabs** are the TUI's details tabs, each from its own topic:

- **Jobs** lists the queued projects with a progress bar. Opening one lists
  its tasks and assignees and **lays its tiles over the map**: yellow queued,
  orange being built, green done (`MapRenderer.setHighlight`, the filth
  tint's shader with its own buffers, so it works at every zoom). The
  highlight goes when the project closes, finishes, or the tab unmounts.
  **Find** centers the map on the tiles still to build.
- **Storage** lists every container with a fill bar and what it holds most
  of. A row opens its tile in the inspector, which already shows contents and
  ledger, rather than a second copy of that view.
- **Market** shows the money supply, the accounts (an account opens in place,
  from its own `account:<key>` topic, so only the open one is built), and the
  books, prices, plans, work orders and recent trades. A depot or a planner is
  a link to the inspector. Its **Colony orders** desk (`ColonyOrders`) posts a
  bid or an ask in the colony's name, and reprices or removes the colony's
  open orders, with the `order-place`, `order-reprice` and `order-cancel`
  commands, and suspends or resumes a standing order with `order-suspend`
  and `order-resume` (see [colony-orders.md](./colony-orders.md)).
- **Zones** paints zones, removes them, and orders an area's structures
  cleared (see [zoning.md](./zoning.md)). One button per zone kind (from the
  `zones` topic, swatch and all), **Remove zone** and **Clear area** arm the
  same area tool the Dig tab uses. `showZone` estimates the outcome from the
  `zones` and `zoning` topics and the tile pages: tiles changed, colony ships'
  ground skipped, rock to dig, and the structures a paint would leave in the
  wrong zone, tinted red with a warning of the extra clearing order. The
  button sends `zone` or `clear`. Below are what the colony is waiting on a
  zone for, the tiles each kind covers and what it holds, and open clearing
  orders with **Cancel** (`clear-cancel`). The map's zone wash is drawn
  whatever tab is open: the page holds the `zones` topic for its life and
  `setZones` draws it as the lowest tint layer, under filth.
- **Dig** orders an area mined out (see [excavation.md](./excavation.md)).
  **Mark an area** arms a tool: while it is armed a drag on the map draws a
  rectangle instead of panning (`attachInput`'s `areaTool` and `area` hooks),
  the rock the colony has seen in it is tinted with the same highlight the
  Jobs tab uses, and the panel prices it from the market topic's `dig` terms.
  **Order** sends the `dig` command; the open orders are listed below with a **Cancel** that sends `dig-cancel`. The tool is one-shot, and leaving the tab
  clears it. The page counts rock itself from the tile pages it holds
  (terrain 0, visible); the engine recounts, so the two can differ by tiles
  already ordered.
- **Ships** lands the colony ships (see [ships.md](./ships.md)). A new game
  starts paused with the founders' ships aloft (the page sends
  `start-paused: true` and `place-ships: true` with its settings) and this
  tab open. The tab hands the player the next ship aloft and draws its shape
  from the topic's `shape` rows; the pointer shows where it would land, tile
  for tile (green, or red where any tile would touch another ship or the
  walkway round one, the check `shipSiteFree` mirrors from the engine), and a
  click lands it there with the `ship-land` command (host API 14). The tab
  then hands over the next one; `ui.shipSent` keeps it from picking the ship
  just sent down up again while the topic catches up, and a **Land** button
  re-arms it if the engine refused. Once every ship is down, **Move** picks
  a landed one up and a click relands it with `ship-move` (host API 13). A
  drag still pans. **Start** is disabled while any ship is aloft; it puts
  the tool down and runs the game at Normal. After the first tick the ships
  stay put and the tab only lists them. The `ships` topic is rebuilt on
  every advance, not on an interval: placing happens paused, where the only
  advance is the one a land or move itself causes.

A link from any of these into the inspector remembers its tab
(`ui.inspectFrom`), so the inspector offers **← Jobs**, **← Storage** or
**← Market**. Going back keeps the selection, so the row stays highlighted.

**Charts** are the TUI's three chart screens in one tab, a view at a time,
drawn with [uPlot](https://github.com/leeoniya/uPlot) on their own opaque
surface:

- **Perf** (the `perf` topic): ticks per second, averaged over the trailing
  second as the TUI does, and milliseconds per tick, with a gap where the
  game was paused, over the last five minutes. Two charts, never one with two
  y-axes: the measures share only the clock.
- **Population** (`population`): colonists, meals in storage, colony size and
  fixtures over the whole game, four small charts, since their scales differ
  by orders of magnitude.
- **Activity** (`population` too): a stacked area of what colonists spend
  their time on, as a share of colonist time or as average colonists. The
  game is summed into at most 48 columns, as the TUI sums per plot column: a
  sample covers as little as 50 ticks, and drawn one by one the stack was
  noise. Each band is split into doing it and walking there, the walking part
  hatched at 45° in the band's color rather than given a second hue.

The colors are the dataviz reference palette's dark steps, checked with its
validator against the charts' surface (`#1a1a19`). A chart gets at most eight
categorical hues and the game has 13 activities, so related ones share a band
(mining & building, hauling & cleaning, and escaping, fleeing and fighting
as "danger"), with idle as a neutral gray lid. The hover tooltip and the
table under the chart still name every activity, lately (the last tenth of
the game) and over the whole game. Every chart has a hover tooltip.

**uPlot loads on first use**, not with the page. It is only for this tab, and
it builds an `Intl.NumberFormat` from `navigator.language` as it loads: a
headless Chromium here reported `en-US@posix`, which `Intl` rejects, and the
throw took the whole page down before the map started. Loaded lazily, a
failure stays in the chart (which says so) and the bundle loses 50 KB.
Automation should give the browser a real locale (`locale: 'en-US'`).

**The top bar stops short of the panel column** (`--panel-reserve` is the
panel's width): with eight tabs the tab strip wraps to two rows there, and it
used to sit on the end of the top bar's counts.

**The speed selector** is Pause plus four running speeds (`speed.ts`): Normal
is the game's default 8 ticks a second, Fast is 32, Faster is 128, and Max is
flat out.
Pause is separate from the rate in the engine (`TogglePause` and
`SetTicksPerSecond`), so choosing a speed while paused sends both, and pausing
keeps the rate. After a press, the selector shows the press for half a second
before it trusts the engine's report again, so a quick `+ +` steps twice
instead of reading back the old speed.

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
- **TypeScript 6, not 7.** `svelte-check`, which type-checks `.svelte` files
  (and the `.ts` ones), needs TypeScript's JavaScript API, and TypeScript 7 is
  the native port without it. `npm run check` is `svelte-check`.
- **The worker is a classic script in `public/`**, not a Vite module worker:
  Go's `wasm_exec.js` is loaded with `importScripts`, and both pages (the game
  and the spike) share the one worker and the one WASM.
- **Every worker message waits for the WASM to load**, not only `start`. A
  panel opened while the page is still loading subscribes before the engine
  exists; handled at once, that subscription was lost, and the panel never
  loaded.

## Extending it

- **A new glyph**: add it in `internal/glyphs` (see
  [frontend-tui.md](./frontend-tui.md)); the page gets it through the Hello
  with no change here. A new terrain whose glyph is a picture draws it
  automatically; one that should stay a flat color belongs in
  `glyphs.Swatch`.
- **Filth colors** are `filthTint` in `palette.ts`; the levels come from
  `Hello.goreMax` and `Hello.scumMax`. A new kind of stain would be another
  sparse list on the wire (as scum is) and another input to `filthTint`.
- **A new terrain or kind**: add its color to `palette.ts` by the name its
  `String` method gives. Magenta on the map means one is missing.
- **More in the inspector**: add the field to `EntityTopic` or `TileTopic` in
  `internal/wire/inspect.go`, its type in `web/src/ui/inspect.ts`, and a row
  in `InspectPanel.svelte`. Anything the fog should hide goes after the
  `ExploredAt` check in `tileTopic`.
- **Something else to select** (a project's tiles, a room): another
  `Selection` variant in `game.svelte.ts`, with its own topic in
  `selectionTopic`.
- **Evicting pages**: nothing is dropped yet. A long session that pans across a
  whole 10000² map would end up holding it all, both on the GPU (in chunks)
  and on the CPU (the hover copies). An eviction needs a message telling the
  worker, so it stops counting the page as held.
- **A new tab**: a component in `web/src/ui/` that calls `subscribe('<topic>')`
  in an `$effect` and reads `topics.data.<topic>`, plus a topic in
  `internal/wire/topics.go` (see [wire-format.md](./wire-format.md)), and an
  entry in one of `SidePanel.svelte`'s `groups` (View if it only shows,
  Act if it arms a map tool or sends a command). It never touches the
  renderer's state.

## Related

- [browser-frontend.md](./browser-frontend.md) — the plan, the worker, and the spike's measurements.
- [wire-format.md](./wire-format.md) — the frames this draws.
- [frontend-tui.md](./frontend-tui.md) — the terminal frontend it is catching up to.
- [fog-of-war.md](./fog-of-war.md) — what the visible bit means.

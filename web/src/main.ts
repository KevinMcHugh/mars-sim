// The game page: start the worker, draw its frames on the map, forward the
// view and the controls, and mount the Svelte chrome (ui/App.svelte) around
// the map. See docs/frontend-web.md.
//
// The map and the worker stay outside Svelte: frames arrive at up to 60 a
// second and go straight to the renderer. What the panels read goes through
// game.svelte.ts, at most UI_HZ times a second.

import { mount } from 'svelte';
import type { Frame, Hello } from '../wire/decode.js';
import { namedStats, TILE_COMPOSITION_MASK, TILE_VISIBLE } from '../wire/decode.js';
import { install, stepSpeed, syncFrame, togglePause, topics, ui, UI_HZ } from './game.svelte';
import { attachInput } from './map/input';
import { MapRenderer } from './map/renderer';
import type { TileRect } from './map/camera';
import { initialSettings } from './settings';
import { SimClient } from './sim/client';
import type { Settings } from './sim/client';
import App from './ui/App.svelte';

function status(text: string | null, error = false): void {
  ui.status = text;
  ui.statusError = error;
}

mount(App, { target: document.getElementById('app')! });

const canvas = document.getElementById('map') as HTMLCanvasElement;
let map: MapRenderer;
try {
  map = new MapRenderer(canvas);
} catch (e) {
  status(String((e as Error).message), true);
  throw e;
}
const cam = map.camera;
const sim = new SimClient(new URL('./worker.js', document.baseURI));

let hello: Hello | null = null;
let last: Frame | null = null;
let centered = false;
let lastInterest = '';
let hoverAt: [number, number] | null = null;
let lastUI = 0;

// What the page shows and has received, for automation (Playwright) to read.
const debug = { frames: 0, bytes: 0, hello: null as Hello | null, genMs: 0, tick: 0, pagesHeld: 0, pagesOwed: 0 };
(window as any).marsMap = { debug, camera: cam, renderer: map, ui, topics };

install({
  command: (c) => sim.command(c),
  subscribe: (t) => sim.subscribe(t),
  unsubscribe: (t) => sim.unsubscribe(t),
  newGame: (s) => { void newGame(s); },
});

sim.onError = (m) => status(m, true);
sim.onTopics = (t) => {
  for (const [name, payload] of Object.entries(t)) topics.set(name, payload);
};
sim.onFrame = (f, bytes) => {
  if (!hello) return;
  last = f;
  map.applyFrame(f);
  debug.frames++;
  debug.bytes += bytes;
  debug.tick = f.tick;
  debug.pagesHeld = map.pages.size;
  debug.pagesOwed = f.pagesOwed;
  if (!centered && f.entities.count > 0) {
    centerOnColony(f);
    centered = true;
  }
  // The chrome refreshes at UI_HZ, and at once when pause flips, so the
  // selector never lags a press.
  const now = performance.now();
  if (now - lastUI >= 1000 / UI_HZ || f.paused !== ui.paused) {
    lastUI = now;
    syncFrame(f.tick, f.paused, f.tps, namedStats(f, hello));
    if (hoverAt) showHover(...hoverAt);
  }
};

attachInput(canvas, cam, {
  changed: () => { viewChanged(); },
  hover: (x, y) => { hoverAt = [x, y]; showHover(x, y); },
  leave: () => { hoverAt = null; ui.hover = null; },
});
window.addEventListener('resize', () => viewChanged());
window.addEventListener('keydown', (e) => {
  if (!hello || e.target instanceof HTMLInputElement || e.target instanceof HTMLSelectElement) return;
  switch (e.key) {
    case ' ': togglePause(); break;
    case '+': case '=': stepSpeed(1); break;
    case '-': case '_': stepSpeed(-1); break;
    default: return;
  }
  e.preventDefault();
});

async function newGame(settings: Settings): Promise<void> {
  status(`Generating a ${settings.width}×${settings.height} world…`);
  hello = null;
  ui.hello = null;
  centered = false;
  lastInterest = '';
  try {
    const started = await sim.start({ tps: 8, ...settings });
    hello = started.hello;
    ui.hello = hello;
    debug.hello = hello;
    debug.genMs = started.genMs;
    map.reset(hello);
    cam.cx = hello.width / 2;
    cam.cy = hello.height / 2;
    viewChanged();
    status(null);
  } catch (e) {
    status(String((e as Error).message), true);
  }
}

/** The camera moved: redraw, and tell the worker what is on screen. */
function viewChanged(): void {
  map.invalidate();
  if (!hello) return;
  cam.clampTo(hello.width, hello.height);
  // A page of margin, so a short pan finds its tiles already here.
  const r: TileRect = cam.visibleTiles(hello.width, hello.height, hello.pageSide);
  const side = hello.pageSide;
  const key = [r.x0, r.y0, r.x1, r.y1].map((v) => Math.floor(v / side)).join(',');
  if (key === lastInterest) return; // same pages as last time
  lastInterest = key;
  sim.setInterest(r);
}

function centerOnColony(f: Frame): void {
  const e = f.entities;
  let sx = 0, sy = 0, n = 0;
  const colonist = hello!.enums.kinds.indexOf('colonist');
  for (let i = 0; i < e.count; i++) {
    if (e.kind[i] !== colonist) continue;
    sx += e.x[i]; sy += e.y[i]; n++;
  }
  if (n === 0) return;
  cam.cx = sx / n + 0.5;
  cam.cy = sy / n + 0.5;
  viewChanged();
}

/** Describe the tile under the pointer: terrain, and whatever is on it. */
function showHover(sx: number, sy: number): void {
  if (!hello) return;
  const [fx, fy] = cam.toTile(sx, sy);
  const x = Math.floor(fx), y = Math.floor(fy);
  if (x < 0 || y < 0 || x >= hello.width || y >= hello.height) { ui.hover = null; return; }
  const parts = [`${x}, ${y}`];
  const cell = map.tileAt(x, y);
  if (!cell || !(cell[1] & TILE_VISIBLE)) {
    parts.push('unexplored');
  } else {
    const [terrain, flags] = cell;
    const glyph = hello.glyphs.terrain[terrain] ?? -1;
    const name = terrain === 0
      ? hello.enums.compositions[flags & TILE_COMPOSITION_MASK] ?? 'rock'
      : hello.enums.terrains[terrain] ?? `terrain ${terrain}`;
    parts.push(glyph >= 0 ? `${hello.glyphs.symbols[glyph]} ${name}` : name);
    const filth = map.filthAt(x, y);
    if (filth?.gore) parts.push(`viscera ${filth.gore}/${hello.goreMax}`);
    if (filth?.scum) parts.push(`scum ${filth.scum}/${hello.scumMax}`);
    if (filth?.corpses) parts.push(filth.corpses === 1 ? 'a body' : `${filth.corpses} bodies`);
    if (last) {
      const e = last.entities;
      for (let i = 0; i < e.count; i++) {
        if (e.x[i] !== x || e.y[i] !== y) continue;
        const g = hello.glyphs.symbols[e.glyph[i]] ?? '';
        parts.push(`${g} ${hello.enums.kinds[e.kind[i]]} #${e.id[i]}, ${hello.enums.states[e.state[i]]}`);
      }
    }
  }
  ui.hover = parts.join(' · ');
}

void newGame(initialSettings());

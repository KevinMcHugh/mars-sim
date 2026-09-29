// The game page: start the worker, draw its frames on the map, and forward the
// view and the controls. See docs/browser-frontend.md.

import type { Frame, Hello } from '../wire/decode.js';
import { namedStats, TILE_COMPOSITION_MASK, TILE_VISIBLE } from '../wire/decode.js';
import { attachInput } from './map/input';
import { kindCSS } from './map/palette';
import { MapRenderer } from './map/renderer';
import type { TileRect } from './map/camera';
import { SimClient } from './sim/client';
import type { Settings } from './sim/client';

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;
const canvas = $<HTMLCanvasElement>('map');
const statusEl = $('status');
const hoverEl = $('hover');
const pauseBtn = $<HTMLButtonElement>('pause');
const speedSel = $<HTMLSelectElement>('speed');

function status(text: string | null, error = false): void {
  statusEl.hidden = text === null;
  statusEl.textContent = text ?? '';
  statusEl.classList.toggle('error', error);
}

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

// What the page shows and has received, for automation (Playwright) to read.
const debug = { frames: 0, bytes: 0, hello: null as Hello | null, genMs: 0, tick: 0, pagesHeld: 0, pagesOwed: 0 };
(window as any).marsMap = { debug, camera: cam, renderer: map };

sim.onError = (m) => status(m, true);
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
  updateHUD(f);
  if (hoverAt) showHover(...hoverAt);
};

attachInput(canvas, cam, {
  changed: () => { viewChanged(); },
  hover: (x, y) => { hoverAt = [x, y]; showHover(x, y); },
  leave: () => { hoverAt = null; hoverEl.hidden = true; },
});
window.addEventListener('resize', () => viewChanged());
window.addEventListener('keydown', (e) => {
  if (e.key === ' ' && !(e.target instanceof HTMLInputElement)) {
    e.preventDefault();
    sim.command({ type: 'pause' });
  }
});
pauseBtn.onclick = () => sim.command({ type: 'pause' });
speedSel.onchange = () => sim.command({ type: 'speed', rate: Number(speedSel.value) });

// New game: the form, seeded from the URL (?width=…&seed=…&fog-of-war=false).
const form = $<HTMLFormElement>('newgameForm');
const params = new URLSearchParams(location.search);
for (const el of Array.from(form.elements) as HTMLInputElement[]) {
  if (!el.name || !params.has(el.name)) continue;
  if (el.type === 'checkbox') el.checked = params.get(el.name) !== 'false';
  else el.value = params.get(el.name)!;
}
form.onsubmit = (e) => {
  e.preventDefault();
  newGame(readForm());
};

function readForm(): Settings {
  const s: Settings = {};
  for (const el of Array.from(form.elements) as HTMLInputElement[]) {
    if (!el.name) continue;
    if (el.type === 'checkbox') s[el.name] = el.checked;
    else if (el.value !== '') s[el.name] = Number(el.value); // blank seed: the engine picks one
  }
  return s;
}

async function newGame(settings: Settings): Promise<void> {
  status(`Generating a ${settings.width}×${settings.height} world…`);
  pauseBtn.disabled = speedSel.disabled = true;
  hello = null;
  centered = false;
  lastInterest = '';
  try {
    const started = await sim.start({ tps: 8, ...settings });
    hello = started.hello;
    debug.hello = hello;
    debug.genMs = started.genMs;
    map.reset(hello);
    cam.cx = hello.width / 2;
    cam.cy = hello.height / 2;
    viewChanged();
    pauseBtn.disabled = speedSel.disabled = false;
    speedSel.value = '8';
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

function updateHUD(f: Frame): void {
  const h = hello!;
  const stats = namedStats(f, h);
  $('clock').textContent = `tick ${f.tick.toLocaleString()}${f.paused ? ' · paused' : ''}`;
  pauseBtn.textContent = f.paused ? 'Resume' : 'Pause';
  const counts: [string, number][] = [
    ['colonist', stats.Colonists], ['alien', stats.Aliens], ['cat', stats.Cats], ['rat', stats.Rats],
  ];
  $('census').innerHTML = counts
    .filter(([, n]) => n !== undefined)
    .map(([k, n]) => `<span><i class="dot" style="background:${kindCSS(k)}"></i>${n} ${k}${n === 1 ? '' : 's'}</span>`)
    .join('');
}

/** Describe the tile under the pointer: terrain, and whatever is on it. */
function showHover(sx: number, sy: number): void {
  if (!hello) return;
  const [fx, fy] = cam.toTile(sx, sy);
  const x = Math.floor(fx), y = Math.floor(fy);
  if (x < 0 || y < 0 || x >= hello.width || y >= hello.height) { hoverEl.hidden = true; return; }
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
    if (last) {
      const e = last.entities;
      for (let i = 0; i < e.count; i++) {
        if (e.x[i] !== x || e.y[i] !== y) continue;
        const g = hello.glyphs.symbols[e.glyph[i]] ?? '';
        parts.push(`${g} ${hello.enums.kinds[e.kind[i]]} #${e.id[i]}, ${hello.enums.states[e.state[i]]}`);
      }
    }
  }
  hoverEl.textContent = parts.join(' · ');
  hoverEl.hidden = false;
}

newGame(readForm());

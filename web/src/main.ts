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
import { colonyLog, cycleFlowField, inspect, install, setFlowField, stepSpeed, subscribe, syncFrame, togglePause, topics, ui, UI_HZ } from './game.svelte';
import { attachInput } from './map/input';
import { MapRenderer } from './map/renderer';
import type { TileRect } from './map/camera';
import { pickGlyph } from './emoji';
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
(window as any).marsMap = { debug, camera: cam, renderer: map, ui, topics, log: colonyLog };

install({
  command: (c) => sim.command(c),
  subscribe: (t) => sim.subscribe(t),
  unsubscribe: (t) => sim.unsubscribe(t),
  newGame: (s) => { void newGame(s); },
  centerOn: (x, y) => { cam.cx = x + 0.5; cam.cy = y + 0.5; viewChanged(); },
  selected: () => updateMark(),
  digChanged: () => showDig(),
  highlight: (tiles) => map.setHighlight(tiles),
});
// Colonists' names for the hover readout; frames carry only ids. Held for
// the page's life, across new games.
subscribe('names');
// The colony log, for the Log tab and the map's ticker: held for the page's
// life, like names.
subscribe('log');

sim.onError = (m) => status(m, true);
sim.onTopics = (t) => {
  for (const [name, payload] of Object.entries(t)) {
    // The log is a stream of deltas, kept in colonyLog, not a payload to hold.
    if (name === 'log') colonyLog.apply(payload as Parameters<typeof colonyLog.apply>[0]);
    else topics.set(name, payload);
  }
};
sim.onFrame = (f, bytes) => {
  if (!hello) return;
  last = f;
  map.applyFrame(f);
  updateMark();
  debug.frames++;
  debug.bytes += bytes;
  debug.tick = f.tick;
  debug.pagesHeld = map.pages.size;
  debug.pagesOwed = f.pagesOwed;
  if (!centered && f.entities.count > 0) {
    centerOnColony(f);
    centered = true;
  }
  // A flow section is rare (the field or the view changed) and may be the
  // only frame for a while if paused, so the legend takes it at once rather
  // than at the UI_HZ beat below.
  if (f.flow) ui.flowShown = map.flowShown;
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
  click: (x, y) => select(x, y),
  areaTool: () => ui.dig.armed && hello !== null,
  area: (phase, x, y) => dragArea(phase, x, y),
});
window.addEventListener('resize', () => viewChanged());
window.addEventListener('keydown', (e) => {
  if (!hello || e.target instanceof HTMLInputElement || e.target instanceof HTMLSelectElement) return;
  switch (e.key) {
    case ' ': togglePause(); break;
    case '+': case '=': stepSpeed(1); break;
    case '-': case '_': stepSpeed(-1); break;
    // Not with a modifier: Ctrl/Cmd+F is the browser's Find.
    case 'f': case 'F':
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      if (e.key === 'f') cycleFlowField(); else setFlowField(-1);
      break;
    default: return;
  }
  e.preventDefault();
});

/**
 * hello with each colonist look resolved to the candidate this browser's
 * emoji font draws as one glyph and appended to symbols, so every glyph index
 * a frame carries is a plain index into symbols: the map's atlas, the hover
 * line and the top bar need know nothing about looks.
 */
function withLooks(h: Hello): Hello {
  const symbols = [...h.glyphs.symbols, ...h.glyphs.looks.map((l) => pickGlyph(l))];
  return { ...h, glyphs: { ...h.glyphs, symbols } };
}

async function newGame(settings: Settings): Promise<void> {
  status(`Generating a ${settings.width}×${settings.height} world…`);
  hello = null;
  ui.hello = null;
  ui.selected = null;
  ui.flowPick = -1; // a new engine shows no field
  ui.flowShown = null;
  colonyLog.clear();
  map.setHighlight(null);
  map.setMark(null);
  digFrom = null;
  ui.dig = { armed: false, rect: null, tiles: 0 };
  centered = false;
  lastInterest = '';
  try {
    const started = await sim.start({ tps: 8, ...settings });
    hello = withLooks(started.hello);
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

/**
 * A click on the map inspects what is there: a creature if one stands on the
 * tile (clicking again steps through a crowd), else the tile. Only creatures
 * the colony can see count, as on the map itself.
 */
function select(sx: number, sy: number): void {
  if (!hello) return;
  const [fx, fy] = cam.toTile(sx, sy);
  const x = Math.floor(fx), y = Math.floor(fy);
  if (x < 0 || y < 0 || x >= hello.width || y >= hello.height) return;
  const here: number[] = [];
  if (last && map.visible(x, y)) {
    const e = last.entities;
    for (let i = 0; i < e.count; i++) if (e.x[i] === x && e.y[i] === y) here.push(e.id[i]);
  }
  // Each click steps one along: the creatures here in turn, then the tile.
  const sel = ui.selected;
  const at = sel && 'entity' in sel ? here.indexOf(sel.entity) : -1;
  inspect(at + 1 < here.length ? { entity: here[at + 1] } : { tile: [x, y] });
}

// The dig tool: where the drag began, in tiles, and the tint it puts on the map.
let digFrom: [number, number] | null = null;
const DIG_TINT = new Uint8Array([224, 112, 58, 110]);

/** The tile under a pixel, clamped onto the map. */
function tileUnder(sx: number, sy: number): [number, number] {
  const [fx, fy] = cam.toTile(sx, sy);
  return [
    Math.max(0, Math.min(hello!.width - 1, Math.floor(fx))),
    Math.max(0, Math.min(hello!.height - 1, Math.floor(fy))),
  ];
}

function dragArea(phase: 'start' | 'move' | 'end' | 'cancel', sx: number, sy: number): void {
  if (!hello) return;
  if (phase === 'cancel') { digFrom = null; ui.dig.rect = null; ui.dig.tiles = 0; showDig(); return; }
  const here = tileUnder(sx, sy);
  if (phase === 'start') digFrom = here;
  if (!digFrom) return;
  ui.dig.rect = {
    x0: Math.min(digFrom[0], here[0]), y0: Math.min(digFrom[1], here[1]),
    x1: Math.max(digFrom[0], here[0]), y1: Math.max(digFrom[1], here[1]),
  };
  showDig();
  if (phase === 'end') { digFrom = null; ui.dig.armed = false; } // one area per press of the tool
}

/** Count and tint the rock the colony has seen inside the marked area. */
function showDig(): void {
  const r = ui.dig.rect;
  if (!r || !hello) { ui.dig.tiles = 0; map.setHighlight(null); return; }
  const tiles: { x: number; y: number; color: Uint8Array }[] = [];
  let n = 0;
  for (let y = r.y0; y <= r.y1; y++) {
    for (let x = r.x0; x <= r.x1; x++) {
      const cell = map.tileAt(x, y);
      // Terrain 0 is rock; the order only covers what the colony has seen.
      if (!cell || cell[0] !== 0 || !(cell[1] & TILE_VISIBLE)) continue;
      n++;
      if (tiles.length < DIG_TINT_MAX) tiles.push({ x, y, color: DIG_TINT });
    }
  }
  ui.dig.tiles = n;
  map.setHighlight(tiles);
}
/** Tinting is for feedback; a huge drag counts its rock without drawing all of it. */
const DIG_TINT_MAX = 4000;

/** Put the map's marker on the selection: a tile, or where the creature is now. */
function updateMark(): void {
  const s = ui.selected;
  if (!s) { map.setMark(null); return; }
  if ('tile' in s) { map.setMark(s.tile); return; }
  const e = last?.entities;
  if (e) {
    for (let i = 0; i < e.count; i++) {
      if (e.id[i] !== s.entity) continue;
      // Not while the fog hides it, or the marker would give it away.
      map.setMark(map.visible(e.x[i], e.y[i]) ? [e.x[i], e.y[i]] : null);
      return;
    }
  }
  map.setMark(null); // dead or gone
}

/** Describe the tile under the pointer: terrain, and whatever is on it. */
function showHover(sx: number, sy: number): void {
  if (!hello) return;
  const [fx, fy] = cam.toTile(sx, sy);
  const x = Math.floor(fx), y = Math.floor(fy);
  if (x < 0 || y < 0 || x >= hello.width || y >= hello.height) { ui.hover = null; return; }
  const names = topics.data.names as Record<string, string> | undefined;
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
    if (filth?.salt) parts.push('salt');
    if (filth?.corpses) parts.push(filth.corpses === 1 ? 'a body' : `${filth.corpses} bodies`);
    const flow = map.flowAt(x, y);
    const shown = ui.flowShown;
    if (flow !== undefined && shown && (flow !== null || hello.enums.terrains[terrain] === 'floor')) {
      const field = hello.flowFields[shown.field] ?? 'flow';
      parts.push(flow === null ? `${field}: unreachable` : `${field}: ${flow} step${flow === 1 ? '' : 's'}`);
    }
    if (last) {
      const e = last.entities;
      for (let i = 0; i < e.count; i++) {
        if (e.x[i] !== x || e.y[i] !== y) continue;
        const g = hello.glyphs.symbols[e.glyph[i]] ?? '';
        const name = names?.[e.id[i]] ?? `${hello.enums.kinds[e.kind[i]]} #${e.id[i]}`;
        parts.push(`${g} ${name}, ${hello.enums.states[e.state[i]]}`);
      }
    }
  }
  ui.hover = parts.join(' · ');
}

void newGame(initialSettings());

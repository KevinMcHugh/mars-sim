// What the Svelte panels read, and the actions they take. The map and the
// worker live outside Svelte (main.ts, map/, sim/); this is the one place the
// two meet. Frame data reaches `ui` at most UI_HZ times a second, so the DOM
// does not re-render at the frame rate. See docs/frontend-web.md.

import type { Hello } from '../wire/decode.js';
import type { Command, Settings } from './sim/client';
import { SPEEDS, speedIndex } from './speed';

/** How often frame-driven UI (clock, stats, speed) refreshes. */
export const UI_HZ = 10;

// Per-browser preferences, in localStorage. Storage can be missing or throw
// (private windows, blocked site data), so every read and write is guarded and
// a failure falls back to the default.
const PREF_PREFIX = 'mars-sim.';
function loadPref<T>(key: string, fallback: T): T {
  try {
    const v = localStorage.getItem(PREF_PREFIX + key);
    return v === null ? fallback : (JSON.parse(v) as T);
  } catch {
    return fallback;
  }
}
function savePref(key: string, value: unknown): void {
  try { localStorage.setItem(PREF_PREFIX + key, JSON.stringify(value)); } catch { /* not remembered */ }
}

export const ui = $state({
  hello: null as Hello | null,
  tick: 0,
  paused: false,
  tps: 8,
  /** The selector's position: speedIndex of the engine, or of the last press until the engine catches up. */
  speed: 1,
  /** Frame stats by Hello.stats name. */
  stats: {} as Record<string, number>,
  hover: null as string | null,
  status: 'Loading…' as string | null,
  statusError: false,
  /** The open side-panel tab (SidePanel.svelte), or null. */
  panel: null as string | null,
  /** What the Inspect tab shows, and the map marks. */
  selected: null as Selection | null,
  /** The tab a selection was made from, for the inspector's way back. */
  inspectFrom: null as string | null,
  /** The roster's filters, kept while the tab is closed (the TUI's f). */
  rosterDead: false,
  rosterNonHuman: false,
  /** Whether the log ticker shows over the map (LogTicker.svelte); remembered. */
  ticker: loadPref('ticker', true),
  /** The Dig tab's tool and the area marked with it (DigPanel.svelte). */
  dig: { armed: false, rect: null, tiles: 0 } as DigState,
  /** The Ships tab's tool: the ship a click on the map relands, or null (ShipsPanel.svelte). */
  shipTool: null as number | null,
  /** The aloft ship last placed: the Ships tab does not pick it up again while the topic catches up. */
  shipSent: null as number | null,
  /** The Charts tab's view, kept while the tab is closed. */
  chartView: 'perf' as 'perf' | 'population' | 'activity',
  /** The flow field asked for (an index into Hello.flowFields), or -1 for none. */
  flowPick: -1,
  /**
   * The flow field the map is drawing, as the worker last reported it, or
   * null. It trails flowPick by a frame or two; the legend reads this one,
   * so it never names a field the map is not showing.
   */
  flowShown: null as { field: number; max: number; goals: number } | null,
});

/** Show a flow field on the map (an index into Hello.flowFields), or none (-1). */
export function setFlowField(i: number): void {
  const n = ui.hello?.flowFields.length ?? 0;
  if (i >= n) i = -1;
  ui.flowPick = i;
  ctl?.command({ type: 'flow', field: i });
}

/** Step to the next flow field, and to none after the last (the TUI's f). */
export function cycleFlowField(): void {
  setFlowField(ui.flowPick + 1);
}

/**
 * The dig tool: while armed, a drag on the map marks an area instead of
 * panning. `rect` is the marked area, in tiles and inclusive, and `tiles` how
 * many of them are rock the colony has seen (what an order would pay to dig).
 */
export interface DigState {
  armed: boolean;
  rect: { x0: number; y0: number; x1: number; y1: number } | null;
  tiles: number;
}

/** The ships topic (internal/wire/topics.go): every ship, and whether they may still land or move. */
export interface ShipsTopic {
  placing: boolean;
  ships: ShipLine[];
}
/**
 * One ship: its footprint's top-left, size and shape, and how many came down
 * in it. One still aloft has a position only once the player has placed it;
 * it lands there when the game starts.
 */
export interface ShipLine {
  id: number; x: number; y: number; w: number; h: number;
  /** The footprint row by row: '#' hull, '.' deck, ' ' not part of the ship. */
  shape?: string[];
  /** stick, hub-and-spoke, or cluster. */
  kind?: string;
  colonists: number;
  /** Still in orbit: nothing is stamped on the map until the game starts. */
  aloft?: boolean;
  /** Aloft, and placed by the player at (x, y): an overlay, not yet landed. */
  placed?: boolean;
}

/** The next ship waiting to be placed, if any: they are handed out in order (docs/ships.md). */
export function nextAloft(t: ShipsTopic): ShipLine | undefined {
  return t.ships.find((s) => s.aloft && !s.placed);
}

/** Whether (dx, dy) of ship s's footprint is part of the ship rather than the ground it leaves be. */
function onShip(s: ShipLine, dx: number, dy: number): boolean {
  if (dx < 0 || dy < 0 || dx >= s.w || dy >= s.h) return false;
  const row = s.shape?.[dy];
  return row === undefined || row[dx] !== ' ';
}

/** Every tile of ship s were its top-left at o, and whether it is hull rather than deck. */
export function shipTiles(s: ShipLine, o: { x: number; y: number }): { x: number; y: number; hull: boolean }[] {
  const out: { x: number; y: number; hull: boolean }[] = [];
  for (let dy = 0; dy < s.h; dy++) {
    for (let dx = 0; dx < s.w; dx++) {
      if (onShip(s, dx, dy)) out.push({ x: o.x + dx, y: o.y + dy, hull: s.shape?.[dy]?.[dx] === '#' });
    }
  }
  return out;
}

/**
 * Where ship s would land if centered on tile (x, y): its top-left, kept on
 * the map with room for the crater round it.
 */
export function shipSiteAt(s: ShipLine, x: number, y: number, width: number, height: number): { x: number; y: number } {
  return {
    x: Math.max(1, Math.min(width - s.w - 1, x - (s.w >> 1))),
    y: Math.max(1, Math.min(height - s.h - 1, y - (s.h >> 1))),
  };
}

/**
 * Whether ship s may be placed with its top-left at o: none of its tiles on
 * another landed or placed ship or the one-tile walkway round it, as the
 * engine's landAloft checks. (The engine also refuses to land on another
 * ship's colonist, which this cannot see; it never stands outside its own
 * ship before the first tick.)
 */
export function shipSiteFree(ships: ShipLine[], s: ShipLine, o: { x: number; y: number }): boolean {
  const others = ships.filter((t) => t.id !== s.id && (!t.aloft || t.placed));
  const near = (t: ShipLine, x: number, y: number) => {
    for (let ny = y - 1; ny <= y + 1; ny++) for (let nx = x - 1; nx <= x + 1; nx++) if (onShip(t, nx - t.x, ny - t.y)) return true;
    return false;
  };
  return shipTiles(s, o).every((p) => others.every((t) => !near(t, p.x, p.y)));
}

/** Pick a ship up: the next click on the map lands it there. null puts the tool down. */
export function armShip(id: number | null): void {
  ui.shipTool = id;
  ctl?.shipToolChanged();
}

/** Reland a ship with its top-left at (x, y), before the first tick. */
export function moveShip(id: number, x: number, y: number): void {
  ctl?.command({ type: 'ship-move', id, x, y });
}

/**
 * Place a ship waiting aloft with its top-left at (x, y), before the first
 * tick. It lands there when the game starts; until then it is only drawn.
 */
export function landAloft(id: number, x: number, y: number): void {
  ui.shipSent = id;
  ctl?.command({ type: 'ship-land', id, x, y });
}

/** A creature by id, or a tile. */
export type Selection = { entity: number } | { tile: [number, number] };

/** The topic that carries a selection (internal/wire/inspect.go). */
export function selectionTopic(s: Selection): string {
  return 'entity' in s ? `entity:${s.entity}` : `tile:${s.tile[0]},${s.tile[1]}`;
}

/** Inspect something: select it and open the Inspect tab. */
export function inspect(s: Selection, from: string | null = null): void {
  ui.selected = s;
  ui.inspectFrom = from;
  ui.panel = 'inspect';
  ctl?.selected();
}

/**
 * Open a side-panel tab, or close the panel (null). Leaving the inspector
 * drops the selection, so the map's marker goes with it, except for the tab
 * the selection came from (and the roster), which highlight its row.
 */
export function setPanel(id: string | null): void {
  if (id !== 'inspect' && id !== 'roster' && id !== ui.inspectFrom && ui.selected) {
    ui.selected = null;
    ctl?.selected();
  }
  ui.panel = id;
}

/**
 * Topic payloads by name. Raw, not deep, state: a payload is immutable data
 * from the worker, replaced whole on each update, and a deep proxy over a
 * thousand roster rows would be all cost (docs/browser-frontend.md, "The
 * other views"). Read topics.data.<name>; main.ts calls set.
 */
class TopicData {
  data: Record<string, unknown> = $state.raw({});
  set(name: string, payload: unknown): void {
    this.data = { ...this.data, [name]: payload };
  }
  drop(name: string): void {
    const { [name]: _, ...rest } = this.data;
    this.data = rest;
  }
}
export const topics = new TopicData();

/** One colony-log line (internal/wire/log.go), and when the page got it. */
export interface LogLine { seq: number; tick: number; kind: string; text: string; at: number }

/** How many log lines the page keeps. The engine keeps only -log-size (64). */
export const LOG_KEEP = 2000;

/**
 * The colony log, built up from the log topic's deltas: the engine's ring is
 * short, the page's history is LOG_KEEP lines. Raw state, replaced on each
 * change, like a topic payload.
 */
class ColonyLog {
  lines: LogLine[] = $state.raw([]);
  apply(p: { reset: boolean; entries: Omit<LogLine, 'at'>[] }): void {
    const at = performance.now();
    const add = p.entries.map((e) => ({ ...e, at }));
    const kept = p.reset ? add : this.lines.concat(add);
    this.lines = kept.length > LOG_KEEP ? kept.slice(kept.length - LOG_KEEP) : kept;
  }
  clear(): void { this.lines = []; }
}
export const colonyLog = new ColonyLog();

// The controller main.ts installs: how the panels reach the worker.
export interface Controller {
  command(c: Command): void;
  subscribe(topic: string): void;
  unsubscribe(topic: string): void;
  newGame(settings: Settings): void;
  /** Center the map on a tile. */
  centerOn(x: number, y: number): void;
  /** ui.selected changed: move the map's marker. */
  selected(): void;
  /** The dig tool's area changed (or was cleared): redraw its tint. */
  digChanged(): void;
  /** The ship tool was picked up or put down: redraw (or clear) its preview. */
  shipToolChanged(): void;
  /** Tint these tiles on the map (a job's), or none. */
  highlight(tiles: { x: number; y: number; color: Uint8Array }[] | null): void;
}
let ctl: Controller | null = null;
export function install(c: Controller): void { ctl = c; }

// Topics are reference-counted: two panels can want the same one.
const refs = new Map<string, number>();

/** Start receiving a topic; call the returned function to stop. For a component's $effect. */
export function subscribe(topic: string): () => void {
  const n = refs.get(topic) ?? 0;
  refs.set(topic, n + 1);
  if (n === 0) ctl?.subscribe(topic);
  return () => {
    const left = (refs.get(topic) ?? 1) - 1;
    refs.set(topic, left);
    if (left === 0) {
      ctl?.unsubscribe(topic);
      topics.drop(topic);
    }
  };
}

export function newGame(settings: Settings): void { ctl?.newGame(settings); }
export function centerOn(x: number, y: number): void { ctl?.centerOn(x, y); }
export function highlight(tiles: { x: number; y: number; color: Uint8Array }[] | null): void { ctl?.highlight(tiles); }

/** Arm or disarm the dig tool; disarming leaves the marked area alone. */
export function armDig(on: boolean): void {
  ui.dig.armed = on;
}

/** Forget the marked area, and put the tool down. */
export function clearDig(): void {
  ui.dig.armed = false;
  ui.dig.rect = null;
  ui.dig.tiles = 0;
  ctl?.digChanged();
}

/** Order the marked area mined out, paid for by the colony. */
export function orderDig(): void {
  const r = ui.dig.rect;
  if (!r) return;
  ctl?.command({ type: 'dig', ...r });
  clearDig();
}

/** Cancel an open excavation order; what it still held goes back to the treasury. */
export function cancelDig(id: number): void {
  ctl?.command({ type: 'dig-cancel', id });
}

/** Post an order in the colony's name; the outcome lands in the log. */
export function placeColonyOrder(o: { side: 'bid' | 'ask'; item: string; qty: number; price: number; x: number; y: number }): void {
  ctl?.command({ type: 'order-place', ...o });
}

/** Re-post one of the colony's open orders at a new price. */
export function repriceColonyOrder(id: number, price: number): void {
  ctl?.command({ type: 'order-reprice', id, price });
}

/** Take one of the colony's open orders off the book. */
export function cancelColonyOrder(id: number): void {
  ctl?.command({ type: 'order-cancel', id });
}

/** Stop the colony posting its standing orders for a side and item, and withdraw them. */
export function suspendColonyOrders(side: 'bid' | 'ask', item: string): void {
  ctl?.command({ type: 'order-suspend', side, item });
}

/** Let the colony post its standing orders for a side and item again. */
export function resumeColonyOrders(side: 'bid' | 'ask', item: string): void {
  ctl?.command({ type: 'order-resume', side, item });
}

export function togglePause(): void {
  ctl?.command({ type: 'pause' });
  pressed = performance.now();
  ui.speed = ui.paused ? speedIndex(false, ui.tps) : 0;
}

// When a speed was last pressed: until the engine reports it (a frame or two
// later), the selector shows the press, so a fast double-press of + steps
// twice instead of reading the stale speed back.
let pressed = 0;

/** Move the selector to SPEEDS[i]: pause, or set the rate and resume. */
export function setSpeed(i: number): void {
  i = Math.max(0, Math.min(SPEEDS.length - 1, i));
  const want = SPEEDS[i];
  const paused = pressed > 0 ? ui.speed === 0 : ui.paused;
  if (want.tps === undefined) {
    if (!paused) ctl?.command({ type: 'pause' });
  } else {
    ctl?.command({ type: 'speed', rate: want.tps });
    if (paused) ctl?.command({ type: 'pause' });
  }
  ui.speed = i;
  pressed = performance.now();
}

export const stepSpeed = (d: number) => setSpeed(ui.speed + d);

/** Take the engine's state from a frame (at most UI_HZ times a second). */
export function syncFrame(tick: number, paused: boolean, tps: number, stats: Record<string, number>): void {
  ui.tick = tick;
  ui.paused = paused;
  ui.tps = tps;
  ui.stats = stats;
  if (performance.now() - pressed > 500) {
    pressed = 0;
    ui.speed = speedIndex(paused, tps);
  }
}

/** Show or hide the log ticker on the map, and remember it in this browser. */
export function setTicker(on: boolean): void {
  ui.ticker = on;
  savePref('ticker', on);
}

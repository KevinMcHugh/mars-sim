// What the Svelte panels read, and the actions they take. The map and the
// worker live outside Svelte (main.ts, map/, sim/); this is the one place the
// two meet. Frame data reaches `ui` at most UI_HZ times a second, so the DOM
// does not re-render at the frame rate. See docs/frontend-web.md.

import type { Hello } from '../wire/decode.js';
import type { Command, Settings } from './sim/client';
import { reuse } from './reuse';
import { parseCharts, STARTER_CHARTS } from './ui/charts/builder';
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
  /**
   * The tabs popped out of the side panel into windows over the map
   * (FloatingPanels.svelte), back to front; remembered. A tab is docked or
   * floating, never both.
   */
  floats: loadPref('floats', [] as FloatWin[]),
  /** What the Inspect tab shows, and the map marks. */
  selected: null as Selection | null,
  /** The tab a selection was made from, for the inspector's way back. */
  inspectFrom: null as string | null,
  /** The roster's filters, kept while the tab is closed (the TUI's f). */
  rosterDead: false,
  rosterNonHuman: false,
  /** Whether the log ticker shows over the map (LogTicker.svelte); remembered. */
  ticker: loadPref('ticker', true),
  /** The tab sections folded shut, by Section id (Section.svelte); remembered. */
  collapsed: loadPref('collapsed', {} as Record<string, boolean>),
  /** The Dig tab's tool and the area marked with it (DigPanel.svelte). */
  dig: { armed: false, rect: null, tiles: 0 } as DigState,
  /** The Zones tab's tool, the area marked with it, and what applying it would do (ZonesPanel.svelte). */
  zone: { armed: false, tool: 'residence', rect: null, preview: null } as ZoneState,
  /** The Ships tab's tool: the ship a click on the map relands, or null (ShipsPanel.svelte). */
  shipTool: null as number | null,
  /** The aloft ship last sent down: the Ships tab does not pick it up again while the topic catches up. */
  shipSent: null as number | null,
  /** The Charts tab's view, kept while the tab is closed. */
  chartView: 'perf' as 'perf' | 'population' | 'activity' | 'custom',
  /** The charts the player has made in the Custom view, remembered in this browser (saveCharts). */
  charts: parseCharts(loadPref('charts', null)) ?? structuredClone(STARTER_CHARTS),
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

/** An area on the map, in tiles, inclusive. */
export interface Rect { x0: number; y0: number; x1: number; y1: number }

/**
 * The zone tool (docs/zoning.md): a zone kind's name to paint it, 'none' to
 * remove zoning, or 'clear' to order the structures in the area cleared.
 * While armed, a drag on the map marks `rect`; `preview` is the page's
 * estimate of what applying it would do (main.ts, showZone).
 */
export interface ZoneState {
  armed: boolean;
  tool: string;
  rect: Rect | null;
  preview: ZonePreview | null;
}

/** What a zone paint or a clearing would do, as far as the page can tell. */
export interface ZonePreview {
  /** Tiles whose zone changes (paint), or built tiles to clear (clear). */
  tiles: number;
  /** Tiles a colony ship holds as residence, which a paint skips. */
  locked: number;
  /** Seen rock a paint would have dug out. */
  dig: number;
  /** Structures a paint leaves outside a zone of their kind, so clears. */
  evicted: { id: number; type: string; built: number }[];
  /** Built tiles cleared: the evicted structures', or the clear tool's. */
  clear: number;
  /** Rooms still going up that the clear tool would call off. */
  rising: number;
}

/** The ships topic (internal/wire/topics.go): every ship, and whether they may still land or move. */
export interface ShipsTopic {
  placing: boolean;
  ships: ShipLine[];
}
/**
 * One ship: its footprint's top-left, size and shape, and how many came down
 * in it. One still aloft has no position yet, and only the next to land has a
 * size and shape.
 */
export interface ShipLine {
  id: number; x: number; y: number; w: number; h: number;
  /** The footprint row by row: '#' hull, '.' deck, ' ' not part of the ship. */
  shape?: string[];
  /** stick, hub-and-spoke, or cluster. */
  kind?: string;
  colonists: number;
  /** Still in orbit, waiting for the player to land it. */
  aloft?: boolean;
}

/** The next ship waiting to land, if any: they land in order (docs/ships.md). */
export function nextAloft(t: ShipsTopic): ShipLine | undefined {
  return t.ships.find((s) => s.aloft);
}

/** Whether (dx, dy) of ship s's footprint is part of the ship rather than the ground it leaves be. */
function onShip(s: ShipLine, dx: number, dy: number): boolean {
  if (dx < 0 || dy < 0 || dx >= s.w || dy >= s.h) return false;
  const row = s.shape?.[dy];
  return row === undefined || row[dx] !== ' ';
}

/** Every tile of ship s were its top-left at o. */
export function shipTiles(s: ShipLine, o: { x: number; y: number }): { x: number; y: number }[] {
  const out: { x: number; y: number }[] = [];
  for (let dy = 0; dy < s.h; dy++) for (let dx = 0; dx < s.w; dx++) if (onShip(s, dx, dy)) out.push({ x: o.x + dx, y: o.y + dy });
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
 * Whether ship s may land with its top-left at o: none of its tiles on
 * another landed ship or the one-tile walkway round it, as the engine's
 * shipSiteAllowed checks. (The engine also refuses to land on another ship's
 * colonist, which this cannot see; it never stands outside its own ship
 * before the first tick.)
 */
export function shipSiteFree(ships: ShipLine[], s: ShipLine, o: { x: number; y: number }): boolean {
  const others = ships.filter((t) => t.id !== s.id && !t.aloft);
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

/** Land the next ship waiting aloft with its top-left at (x, y), before the first tick. */
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

/** Inspect something: select it and open the Inspect tab (or raise its window). */
export function inspect(s: Selection, from: string | null = null): void {
  ui.selected = s;
  ui.inspectFrom = from;
  if (isFloating('inspect')) raiseFloat('inspect');
  else ui.panel = 'inspect';
  ctl?.selected();
}

/**
 * Open a side-panel tab, or close the panel (null). A tab that is popped out
 * is raised instead, and the docked tab stays.
 */
export function setPanel(id: string | null): void {
  if (id !== null && isFloating(id)) raiseFloat(id);
  else ui.panel = id;
  dropHiddenSelection();
}

/**
 * Leaving the inspector drops the selection, so the map's marker goes with
 * it, except while the tab the selection came from (or the roster) is open,
 * which highlights its row. "Open" is docked or floating. A docked null
 * matches an inspectFrom of null: closing the panel after a click on the map
 * keeps the marker.
 */
function dropHiddenSelection(): void {
  if (!ui.selected) return;
  const keeps = (id: string | null) => id === 'inspect' || id === 'roster' || id === ui.inspectFrom;
  if (keeps(ui.panel) || ui.floats.some((f) => keeps(f.id))) return;
  ui.selected = null;
  ctl?.selected();
}

/** A tab popped out into a window over the map: where it is, in CSS pixels. */
export interface FloatWin { id: string; x: number; y: number; w: number; h: number }

export function isFloating(id: string): boolean {
  return ui.floats.some((f) => f.id === id);
}

/** Remember the Custom view's charts (ui.charts) in this browser. */
export function saveCharts(): void {
  savePref('charts', $state.snapshot(ui.charts));
}

function saveFloats(): void {
  savePref('floats', $state.snapshot(ui.floats));
}

/**
 * Pop a tab out of the side panel into a window over the map. A new window
 * opens left of the side panel, in the first of a cascade of slots that no
 * window still sits in exactly, so a pop-out never lands squarely on another.
 */
export function popOut(id: string): void {
  if (isFloating(id)) return raiseFloat(id);
  const w = 380;
  const h = Math.max(240, Math.min(560, window.innerHeight - 140));
  const slot = (k: number) => ({ x: Math.max(10, window.innerWidth - 2 * w - 40 - 28 * k), y: 90 + 28 * k });
  let k = 0;
  while (ui.floats.some((f) => f.x === slot(k).x && f.y === slot(k).y)) k++;
  ui.floats.push({ id, ...slot(k), w, h });
  if (ui.panel === id) ui.panel = null;
  saveFloats();
}

/** Put a floating tab back in the side panel, as its open tab. */
export function dock(id: string): void {
  ui.floats = ui.floats.filter((f) => f.id !== id);
  ui.panel = id;
  dropHiddenSelection();
  saveFloats();
}

/** Close a floating tab's window. */
export function closeFloat(id: string): void {
  ui.floats = ui.floats.filter((f) => f.id !== id);
  dropHiddenSelection();
  saveFloats();
}

/** Bring a floating tab's window to the front. */
export function raiseFloat(id: string): void {
  const i = ui.floats.findIndex((f) => f.id === id);
  if (i < 0 || i === ui.floats.length - 1) return;
  const [f] = ui.floats.splice(i, 1);
  ui.floats.push(f);
  saveFloats();
}

/** Move or resize a floating tab's window; `save` remembers it (at the end of a drag). */
export function placeFloat(id: string, at: Partial<Omit<FloatWin, 'id'>>, save = false): void {
  const f = ui.floats.find((w) => w.id === id);
  if (!f) return;
  Object.assign(f, at);
  if (save) saveFloats();
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
    this.data = { ...this.data, [name]: reuse(this.data[name], payload) };
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
  /** Replace the game with a save file's. */
  loadGame(file: File): void;
  /** Download the running game as a save file. */
  saveGame(): void;
  /** Center the map on a tile. */
  centerOn(x: number, y: number): void;
  /** ui.selected changed: move the map's marker. */
  selected(): void;
  /** The dig tool's area changed (or was cleared): redraw its tint. */
  digChanged(): void;
  /** The zone tool, its area, or what it would cover changed: re-estimate and redraw. */
  zoneChanged(): void;
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
export function loadGame(file: File): void { ctl?.loadGame(file); }
export function saveGame(): void { ctl?.saveGame(); }
export function centerOn(x: number, y: number): void { ctl?.centerOn(x, y); }
export function highlight(tiles: { x: number; y: number; color: Uint8Array }[] | null): void { ctl?.highlight(tiles); }

/** Arm or disarm the dig tool; disarming leaves the marked area alone. */
export function armDig(on: boolean): void {
  ui.dig.armed = on;
  if (on) ui.zone.armed = false; // one area tool at a time
}

/** Pick the zone tool (a zone kind, 'none' or 'clear') and arm it, or disarm it. */
export function armZone(tool: string, on: boolean): void {
  ui.zone.tool = tool;
  ui.zone.armed = on;
  if (on) ui.dig.armed = false;
  ctl?.zoneChanged();
}

/** Forget the zone tool's area, and put the tool down. */
export function clearZoneTool(): void {
  ui.zone.armed = false;
  ui.zone.rect = null;
  ui.zone.preview = null;
  ctl?.zoneChanged();
}

/** Re-estimate the zone tool's preview (its topics changed). */
export function zoneChanged(): void { ctl?.zoneChanged(); }

/** Apply the zone tool to its area: paint the zone, unzone it, or order it cleared. */
export function applyZone(): void {
  const r = ui.zone.rect;
  if (!r) return;
  if (ui.zone.tool === 'clear') ctl?.command({ type: 'clear', ...r });
  else ctl?.command({ type: 'zone', kind: ui.zone.tool, ...r });
  clearZoneTool();
}

/** Cancel an open clearing order; what it still held goes back to the treasury. */
export function cancelClear(id: number): void {
  ctl?.command({ type: 'clear-cancel', id });
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

/** Set the colony's colony-wide order for a side and item, replacing any it had. */
export function setColonyWideOrder(o: { side: 'bid' | 'ask'; item: string; qty: number; price: number }): void {
  ctl?.command({ type: 'order-wide-set', ...o });
}

/** Drop the colony's colony-wide order for a side and item. */
export function clearColonyWideOrder(side: 'bid' | 'ask', item: string): void {
  ctl?.command({ type: 'order-wide-clear', side, item });
}

/** Pay the recruiter for a new set of candidates; the outcome lands in the log. */
export function rollRecruits(): void {
  ctl?.command({ type: 'recruit-roll' });
}

/** Hire the picked candidates from set id, or turn the set away with none. */
export function hireRecruits(id: number, picks: readonly number[]): void {
  // A copy: a panel's picks are a $state proxy, which postMessage cannot clone.
  ctl?.command({ type: 'recruit-hire', id, picks: [...picks] });
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

/** Fold a tab section (Section.svelte) shut or open it, and remember it in this browser. */
export function toggleSection(id: string): void {
  if (ui.collapsed[id]) delete ui.collapsed[id];
  else ui.collapsed[id] = true;
  savePref('collapsed', $state.snapshot(ui.collapsed));
}

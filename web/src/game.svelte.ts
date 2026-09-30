// What the Svelte panels read, and the actions they take. The map and the
// worker live outside Svelte (main.ts, map/, sim/); this is the one place the
// two meet. Frame data reaches `ui` at most UI_HZ times a second, so the DOM
// does not re-render at the frame rate. See docs/frontend-web.md.

import type { Hello } from '../wire/decode.js';
import type { Command, Settings } from './sim/client';
import { SPEEDS, speedIndex } from './speed';

/** How often frame-driven UI (clock, stats, speed) refreshes. */
export const UI_HZ = 10;

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
});

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

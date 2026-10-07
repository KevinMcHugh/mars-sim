// Flat colors for the first map, keyed by the names Hello carries rather than
// by enum value, so reordering an enum in Go cannot recolor the map. A name
// with no color here draws magenta, which is hard to miss.

type RGB = [number, number, number];

const hex = (h: string): RGB => [
  parseInt(h.slice(1, 3), 16) / 255,
  parseInt(h.slice(3, 5), 16) / 255,
  parseInt(h.slice(5, 7), 16) / 255,
];

export const MISSING = hex('#ff00ff');
export const OFF_MAP = hex('#050404');
export const FOG = hex('#2e0d0b');

const terrains: Record<string, string> = {
  // Rock takes its color from its composition instead; see compositions.
  rock: '#a8402a',
  floor: '#f78765',
  wall: '#8a8d91',
  'pod hull': '#6f7c86',
  'nutrient pod': '#4caf7a',
  toilet: '#dfe6ea',
  bed: '#6b7fd6',
  incinerator: '#e0602f',
  'storage container': '#b8894a',
  scumhouse: '#7fae3e',
  forge: '#d9822b',
  'gun bench': '#9aa3ad',
  chair: '#c9a24a',
  'scum incubator': '#5fd0a8',
  trough: '#8a6a3a',
  // Under their arrow glyphs: the way down reads dark, the way up light.
  'stair down': '#5a2a1e',
  'stair up': '#f2b49e',
};

const compositions: Record<string, string> = {
  'ordinary rock': '#a8402a',
  'iron-bearing rock': '#7a2a1a',
  'water ice-bearing rock': '#7fa6b8',
  // Bright, so it can't be mistaken for the dark green of scum (filthTint).
  'uranium-bearing rock': '#9cc93a',
  'clay-bearing rock': '#b8694a',
};

const kinds: Record<string, string> = {
  colonist: '#3fd0ff',
  alien: '#e03cff',
  cat: '#ffcc33',
  rat: '#a89c92',
  chicken: '#f4efe6',
};

/** Under a facility's glyph: the floor, a shade darker, so the room reads. */
export const GLYPH_BACKDROP = hex('#d96d4e');
export const CORPSE = hex('#efe6d4');
/** The inspector's selection ring. */
export const MARK = hex('#fff2a8');

function lookup(names: string[], table: Record<string, string>): RGB[] {
  return names.map((n) => (table[n] ? hex(table[n]) : MISSING));
}

export const terrainColors = (names: string[]) => lookup(names, terrains);
export const compositionColors = (names: string[]) => lookup(names, compositions);
export const kindColors = (names: string[]) => lookup(names, kinds);

// Filth tints (see renderer.ts): a wash over the tile, deeper with more on it.
const FILTH_GORE = hex('#6e0f0f');
const FILTH_SCUM = hex('#24561a');
const FILTH_BOTH = hex('#5a3614');

/**
 * The premultiplied RGBA (0..255) tint for a tile with gore and scum each at
 * a level in 0..1: dark red for gore, dark green for scum, brown for both.
 * One unit reads plainly; a full tile is nearly the solid color.
 */
export function filthTint(gore: number, scum: number): Uint8Array {
  const color = gore > 0 && scum > 0 ? FILTH_BOTH : gore > 0 ? FILTH_GORE : FILTH_SCUM;
  const level = Math.min(1, Math.max(gore, scum));
  const a = 0.4 + 0.5 * level;
  return new Uint8Array([color[0] * a * 255, color[1] * a * 255, color[2] * a * 255, a * 255]);
}

/**
 * A salt deposit: a pale wash, light enough to read as frosting on the red
 * rock and clear of the blue of water ice and the greens of scum and uranium.
 */
export const SALT_TINT = (() => {
  const c = hex('#efeae0'), a = 0.55;
  return new Uint8Array([c[0] * a * 255, c[1] * a * 255, c[2] * a * 255, a * 255]);
})();

// A job's tiles on the map (the Jobs tab's highlight), premultiplied RGBA.
const premul = (h: string, a: number) => {
  const c = hex(h);
  return new Uint8Array([c[0] * a * 255, c[1] * a * 255, c[2] * a * 255, a * 255]);
};
export const JOB_QUEUED = premul('#ffe066', 0.5);
export const JOB_BUILDING = premul('#ff9f1c', 0.6);
export const JOB_DONE = premul('#5fd38d', 0.35);

// The flow-field overlay (docs/flow-field-view.md): bright green on the goal
// tiles, then eight bands from near (green) to far (dark red), the TUI's
// xterm-256 ramp so the two frontends read the same.
export const FLOW_GOAL_HEX = '#00ff00';
export const FLOW_RAMP_HEX = ['#008700', '#5f8700', '#878700', '#af8700', '#af5f00', '#af0000', '#870000', '#5f0000'];
const FLOW_ALPHA = 0.65;
const FLOW_GOAL = premul(FLOW_GOAL_HEX, FLOW_ALPHA);
const FLOW_RAMP = FLOW_RAMP_HEX.map((h) => premul(h, FLOW_ALPHA));

/** The ramp band for distance d >= 1 on a field whose farthest tile is max. */
export function flowBand(d: number, max: number): number {
  if (max <= 1) return 0;
  return Math.min(Math.floor(((d - 1) * FLOW_RAMP.length) / max), FLOW_RAMP.length - 1);
}

/** The premultiplied RGBA tint for a tile at distance d. */
export function flowTint(d: number, max: number): Uint8Array {
  return d === 0 ? FLOW_GOAL : FLOW_RAMP[flowBand(d, max)];
}

// Zones (docs/zoning.md). Each kind's colour comes from Go (the zones topic),
// so a new kind needs no change here. On the map a zone is a faint wash; the
// zone tool's preview of what it is about to paint is a stronger one, and a
// structure the paint would have cleared is marked red.
export const ZONE_ALPHA = 0.36;
export const ZONE_PREVIEW_ALPHA = 0.55;
/** The premultiplied RGBA tint for a zone colour ("#rrggbb") at alpha a. */
export const zoneTint = (color: string, a: number) => premul(color, a);
/** The preview of unzoning: the ground goes grey-black. */
export const UNZONE_PREVIEW = premul('#1b1b1b', 0.5);
/** A structure the paint or the clear tool will take down. */
export const CLEAR_PREVIEW = premul('#ff3b30', 0.6);

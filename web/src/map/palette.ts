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
export const FOG = hex('#120d0b');

const terrains: Record<string, string> = {
  // Rock takes its color from its composition instead; see compositions.
  rock: '#5a3a2c',
  floor: '#c8a27c',
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
};

const compositions: Record<string, string> = {
  'ordinary rock': '#5a3a2c',
  'iron-bearing rock': '#8a3f22',
  'water ice-bearing rock': '#7fa6b8',
  // Bright, so it can't be mistaken for the dark green of scum (filthTint).
  'uranium-bearing rock': '#9cc93a',
  'clay-bearing rock': '#8c6a48',
};

const kinds: Record<string, string> = {
  colonist: '#3fd0ff',
  alien: '#e03cff',
  cat: '#ffcc33',
  rat: '#a89c92',
};

/** Under a facility's glyph: the floor, a shade darker, so the room reads. */
export const GLYPH_BACKDROP = hex('#b08b68');
export const CORPSE = hex('#efe6d4');

function lookup(names: string[], table: Record<string, string>): RGB[] {
  return names.map((n) => (table[n] ? hex(table[n]) : MISSING));
}

export const terrainColors = (names: string[]) => lookup(names, terrains);
export const compositionColors = (names: string[]) => lookup(names, compositions);
export const kindColors = (names: string[]) => lookup(names, kinds);

/** CSS form of a kind's color, for the HUD legend. */
export function kindCSS(name: string): string {
  return kinds[name] ?? '#ff00ff';
}

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

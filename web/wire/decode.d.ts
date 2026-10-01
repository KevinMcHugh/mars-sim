// Types for decode.js, which stays plain JS so node --test can run it as is.

export const VERSION: number;
export const PAGE_SIDE: number;
export const PAGE_TILES: number;
export const TILE_BYTES: number;
export const TILE_COMPOSITION_MASK: number;
export const TILE_VISIBLE: number;

export interface Hello {
  version: number;
  width: number;
  height: number;
  seed: number;
  pageSide: number;
  fogOfWar: boolean;
  enums: {
    terrains: string[];
    compositions: string[];
    kinds: string[];
    states: string[];
    focuses: string[];
  };
  stats: string[];
  glyphs: {
    /** The emoji; a frame's entities.glyph indexes this. */
    symbols: string[];
    /** Per terrain value: its glyph's index, or -1 to draw its color instead. */
    terrain: number[];
    /** Per kind value: its generic glyph's index (glyphs.ForKind), for counts. */
    kinds: number[];
    gore: number;
    corpse: number;
    /**
     * Colonist figures in their own skin and hair: a frame's glyph
     * symbols.length + i is looks[i]. Each is a candidate list, best first,
     * ending in a plain symbol (see src/emoji.ts, docs/colonist-looks.md).
     * The page resolves them on arrival (withLooks in src/main.ts), after
     * which symbols covers every index a frame carries.
     */
    looks: string[][];
  };
  /** The most gore / scum one tile holds, for shading by amount. */
  goreMax: number;
  scumMax: number;
}

export interface Frame {
  version: number;
  paused: boolean;
  fogOfWar: boolean;
  tilesReset: boolean;
  tick: number;
  tileFrame: number;
  tps: number;
  pagesOwed: number;
  stats: Int32Array;
  entities: {
    count: number;
    id: Uint32Array;
    x: Int32Array;
    y: Int32Array;
    glyph: Uint16Array;
    kind: Uint8Array;
    state: Uint8Array;
    focus: Uint8Array;
  };
  pages: {
    count: number;
    px: Int32Array;
    py: Int32Array;
    tiles: Uint8Array;
  };
  refuse: {
    count: number;
    x: Int32Array;
    y: Int32Array;
    corpses: Uint16Array;
    gore: Uint8Array;
  } | null;
  scum: {
    count: number;
    x: Int32Array;
    y: Int32Array;
    amount: Uint8Array;
  } | null;
  salt: {
    count: number;
    x: Int32Array;
    y: Int32Array;
  } | null;
}

export function decodeFrame(buffer: ArrayBuffer): Frame;
export function namedStats(frame: Frame, hello: Hello): Record<string, number>;

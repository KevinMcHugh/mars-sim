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
}

export function decodeFrame(buffer: ArrayBuffer): Frame;
export function namedStats(frame: Frame, hello: Hello): Record<string, number>;

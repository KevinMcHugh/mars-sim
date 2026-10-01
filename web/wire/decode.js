// Decoder for the engine's binary frames (internal/wire; layout in
// docs/wire-format.md). It parses nothing: every section comes back as a
// typed-array view over the frame's own buffer, so a frame with thousands of
// entities costs a handful of allocations, not one object per entity.
//
// The views alias the buffer. Keep the buffer (or copy out) for as long as you
// read them.

export const VERSION = 5;

const HEADER = 72;
export const PAGE_SIDE = 64;
export const PAGE_TILES = PAGE_SIDE * PAGE_SIDE;
export const TILE_BYTES = 2; // terrain, flags

export const TILE_COMPOSITION_MASK = 0x0f;
export const TILE_VISIBLE = 1 << 4;

const FLAG_PAUSED = 1 << 0;
const FLAG_FOG = 1 << 1;
const FLAG_RESET = 1 << 2;
const FLAG_REFUSE = 1 << 3;
const FLAG_SCUM = 1 << 4;
const FLAG_SALT = 1 << 5;
const FLAG_FLOW = 1 << 6;

// Typed arrays read in the platform's byte order, and the wire is little
// endian. Every browser that runs WASM is little endian; say so if not.
if (new Uint8Array(new Uint16Array([1]).buffer)[0] !== 1) {
  throw new Error('wire: big-endian platform; the frame decoder assumes little endian');
}

const align4 = (n) => (n + 3) & ~3;

/**
 * @param {ArrayBuffer} buffer one frame
 */
export function decodeFrame(buffer) {
  const dv = new DataView(buffer);
  if (dv.getUint32(0, true) !== 0x5246534d) throw new Error('wire: not a frame (bad magic)');
  const version = dv.getUint16(4, true);
  if (version !== VERSION) throw new Error(`wire: frame version ${version}, decoder speaks ${VERSION}`);
  const flags = dv.getUint16(6, true);
  const nStats = dv.getUint32(28, true);
  const n = dv.getUint32(32, true);
  const nPages = dv.getUint32(36, true);
  const nRefuse = dv.getUint32(40, true);
  const nScum = dv.getUint32(48, true);
  const nSalt = dv.getUint32(52, true);
  const nFlow = dv.getUint32(56, true);

  let at = HEADER;
  const stats = new Int32Array(buffer, at, nStats);
  at += 4 * nStats;

  const entities = {
    count: n,
    id: new Uint32Array(buffer, at, n),
    x: new Int32Array(buffer, at + 4 * n, n),
    y: new Int32Array(buffer, at + 8 * n, n),
    // Index into hello.glyphs.symbols: the emoji the engine picked for it.
    glyph: new Uint16Array(buffer, at + 12 * n, n),
    kind: null,
    state: null,
    focus: null,
  };
  at += 12 * n + align4(2 * n);
  entities.kind = new Uint8Array(buffer, at, n);
  entities.state = new Uint8Array(buffer, at + n, n);
  entities.focus = new Uint8Array(buffer, at + 2 * n, n);
  at += align4(3 * n);

  const pages = {
    count: nPages,
    px: new Int32Array(buffer, at, nPages),
    py: new Int32Array(buffer, at + 4 * nPages, nPages),
    // Page i's tiles are tiles.subarray(i * PAGE_TILES * TILE_BYTES, ...),
    // row by row, TILE_BYTES each: terrain, then flags.
    tiles: null,
  };
  at += 8 * nPages;
  pages.tiles = new Uint8Array(buffer, at, nPages * PAGE_TILES * TILE_BYTES);
  at += nPages * PAGE_TILES * TILE_BYTES;

  let refuse = null;
  if (flags & FLAG_REFUSE) {
    refuse = {
      count: nRefuse,
      x: new Int32Array(buffer, at, nRefuse),
      y: new Int32Array(buffer, at + 4 * nRefuse, nRefuse),
      corpses: new Uint16Array(buffer, at + 8 * nRefuse, nRefuse),
      gore: new Uint8Array(buffer, at + 8 * nRefuse + align4(2 * nRefuse), nRefuse),
    };
  }
  at += 8 * nRefuse + align4(2 * nRefuse) + align4(nRefuse);

  let scum = null;
  if (flags & FLAG_SCUM) {
    scum = {
      count: nScum,
      x: new Int32Array(buffer, at, nScum),
      y: new Int32Array(buffer, at + 4 * nScum, nScum),
      // 1..hello.scumMax
      amount: new Uint8Array(buffer, at + 8 * nScum, nScum),
    };
  }
  at += 8 * nScum + align4(nScum);

  let salt = null;
  if (flags & FLAG_SALT) {
    salt = {
      count: nSalt,
      x: new Int32Array(buffer, at, nSalt),
      y: new Int32Array(buffer, at + 4 * nSalt, nSalt),
    };
  }
  at += 8 * nSalt;

  let flow = null;
  if (flags & FLAG_FLOW) {
    flow = {
      // Index into hello.flowFields, or -1: no field shown, clear the overlay.
      field: dv.getInt32(60, true),
      // The field's largest distance and goal count, over the whole map.
      max: dv.getInt32(64, true),
      goals: dv.getInt32(68, true),
      // The tiles in view the field reaches, and their distances (saturating
      // at 65535).
      count: nFlow,
      x: new Int32Array(buffer, at, nFlow),
      y: new Int32Array(buffer, at + 4 * nFlow, nFlow),
      dist: new Uint16Array(buffer, at + 8 * nFlow, nFlow),
    };
  }
  at += 8 * nFlow + align4(2 * nFlow);
  if (at !== buffer.byteLength) {
    throw new Error(`wire: frame is ${buffer.byteLength} bytes, sections add up to ${at}`);
  }

  return {
    version,
    paused: (flags & FLAG_PAUSED) !== 0,
    fogOfWar: (flags & FLAG_FOG) !== 0,
    // Drop every page held before applying this frame's pages.
    tilesReset: (flags & FLAG_RESET) !== 0,
    tick: Number(dv.getBigUint64(8, true)),
    tileFrame: Number(dv.getBigUint64(16, true)),
    tps: dv.getUint32(24, true),
    // Pages in view the worker still owes; they follow in the next frames.
    pagesOwed: dv.getUint32(44, true),
    stats,
    entities,
    pages,
    // The whole refuse list when it changed, else null: keep the last one.
    refuse,
    // Likewise the whole scum list (cave scum on rock), or null.
    scum,
    // Likewise the whole salt list (deposits on rock), or null.
    salt,
    // The shown flow field's tiles in view when the field or the view
    // changed, else null: keep the last one.
    flow,
  };
}

/** Name the stats section with Hello.stats: { Colonists: 6, ... }. */
export function namedStats(frame, hello) {
  const out = {};
  hello.stats.forEach((name, i) => { out[name] = frame.stats[i]; });
  return out;
}

// The WebGL2 map: terrain from integer textures, entities and refuse as
// instanced quads. Framework-free on purpose (docs/browser-frontend.md, "The
// map"): it owns its canvas and takes frames and a camera, nothing else, so it
// could move into a worker with OffscreenCanvas later.
//
// Terrain lives in "chunk" textures of CHUNK_PAGES x CHUNK_PAGES wire pages
// (2048x2048 tiles), created only when a page inside one arrives. A page is
// one texSubImage2D of its 64x64 two-byte tiles, straight from the frame's
// buffer. The fragment shader reads a tile with texelFetch and colors it from
// uniform palettes, so drawing costs the same at any zoom: one quad per chunk
// on screen, not one draw per tile.
//
// Filth (viscera and cave scum) is not a symbol but a tint over the tile:
// dark red for gore, dark green for scum, brown for both, deeper the more
// there is (a full-tile quad per dirty tile, drawn between terrain and
// sprites). Bodies keep a marker: they are a thing to haul, not a stain.
//
// Zoomed in (GLYPH_ZOOM and up), glyphs replace the flat colors: a facility's
// emoji over a floor backdrop, and each entity's own emoji (picked in Go, see
// internal/glyphs) from the atlas (atlas.ts). Zoomed out, a glyph would be a
// few pixels of mush, so the flat colors and dots stay.

import type { Frame, Hello } from '../../wire/decode.js';
import { PAGE_SIDE, PAGE_TILES, TILE_BYTES, TILE_VISIBLE } from '../../wire/decode.js';
import { buildAtlas } from './atlas';
import type { Atlas } from './atlas';
import { Camera } from './camera';
import * as palette from './palette';

const CHUNK_PAGES = 32;
const CHUNK_TILES = CHUNK_PAGES * PAGE_SIDE; // 2048: under every WebGL2 max texture size
const MAX_TERRAINS = 32;
const MAX_COMPOSITIONS = 16;
const MAX_KINDS = 8;
// An instance "kind" past the entity kinds, for a body's marker.
const KIND_CORPSE = MAX_KINDS - 1;
/** CSS pixels per tile at which the map switches from flat colors to glyphs. */
export const GLYPH_ZOOM = 10;

const TERRAIN_VS = `#version 300 es
in vec2 aCorner;
uniform vec2 uOrigin;   // chunk's top-left tile
uniform vec2 uSize;     // chunk's size in tiles, clipped to the map
uniform vec2 uCam;      // camera center, tiles
uniform float uScale;   // device pixels per tile
uniform vec2 uView;     // viewport, device pixels
out vec2 vTile;         // tile coordinates within the chunk
void main() {
  vTile = aCorner * uSize;
  vec2 px = (uOrigin + vTile - uCam) * uScale;
  gl_Position = vec4(px.x / (uView.x * 0.5), -px.y / (uView.y * 0.5), 0.0, 1.0);
}`;

const TERRAIN_FS = `#version 300 es
precision highp float;
precision highp usampler2D;
in vec2 vTile;
uniform usampler2D uTerrain;
uniform vec3 uTerrainColors[${MAX_TERRAINS}];
uniform vec3 uRockColors[${MAX_COMPOSITIONS}];
uniform vec3 uFog;
uniform float uScale;
uniform bool uGlyphs;                       // draw glyphs (zoomed in)
uniform sampler2D uAtlas;
uniform vec2 uAtlasGrid;                    // atlas columns, rows
uniform int uTerrainGlyph[${MAX_TERRAINS}]; // atlas index per terrain, -1 for none
uniform vec3 uGlyphBackdrop;                // what a glyph terrain sits on
out vec4 outColor;
void main() {
  // Gradients before any branch, for textureGrad: fract() jumps at tile
  // edges, and implicit derivatives there would pick a tiny mip level and
  // draw a seam around every tile.
  vec2 gx = dFdx(vTile) / uAtlasGrid, gy = dFdy(vTile) / uAtlasGrid;
  ivec2 t = ivec2(floor(vTile));
  uvec2 cell = texelFetch(uTerrain, t, 0).rg;
  uint terrain = cell.r;
  uint flags = cell.g;
  vec3 c;
  if ((flags & 16u) == 0u) {
    c = uFog;
  } else if (terrain == 0u) {
    c = uRockColors[min(flags & 15u, ${MAX_COMPOSITIONS - 1}u)];
  } else {
    c = uTerrainColors[min(terrain, ${MAX_TERRAINS - 1}u)];
    int g = uGlyphs ? uTerrainGlyph[min(int(terrain), ${MAX_TERRAINS - 1})] : -1;
    if (g >= 0) {
      int cols = int(uAtlasGrid.x);
      vec2 uv = (vec2(float(g % cols), float(g / cols)) + fract(vTile)) / uAtlasGrid;
      vec4 s = textureGrad(uAtlas, uv, gx, gy); // premultiplied
      c = uGlyphBackdrop * (1.0 - s.a) + s.rgb;
    }
  }
  // A faint grid once tiles are big enough to tell apart.
  if (uScale >= 14.0) {
    vec2 f = fract(vTile);
    float edge = min(min(f.x, f.y), min(1.0 - f.x, 1.0 - f.y)) * uScale;
    if (edge < 0.6) c *= 0.88;
  }
  outColor = vec4(c, 1.0);
}`;

const TINT_VS = `#version 300 es
in vec2 aCorner;
in ivec2 aPos;          // per instance: tile
in vec4 aColor;         // per instance: premultiplied tint
uniform vec2 uCam;
uniform float uScale;
uniform vec2 uView;
out vec4 vColor;
void main() {
  vColor = aColor;
  vec2 px = (vec2(aPos) + aCorner - uCam) * uScale;
  gl_Position = vec4(px.x / (uView.x * 0.5), -px.y / (uView.y * 0.5), 0.0, 1.0);
}`;

const TINT_FS = `#version 300 es
precision highp float;
in vec4 vColor;
out vec4 outColor;
void main() { outColor = vColor; }`;

/** What is on one dirty tile, for the tint and the hover readout. */
export interface Filth { gore: number; scum: number; corpses: number }

const SPRITE_VS = `#version 300 es
in vec2 aCorner;
in ivec2 aPos;          // per instance: tile
in uint aKind;          // per instance: palette index
in uint aGlyph;         // per instance: atlas index
uniform vec2 uCam;
uniform float uScale;
uniform vec2 uView;
uniform float uSize;    // sprite size in tiles
out vec2 vCorner;
flat out uint vKind;
flat out uint vGlyph;
void main() {
  vCorner = aCorner;
  vKind = aKind;
  vGlyph = aGlyph;
  vec2 tile = vec2(aPos) + 0.5 + (aCorner - 0.5) * uSize;
  vec2 px = (tile - uCam) * uScale;
  gl_Position = vec4(px.x / (uView.x * 0.5), -px.y / (uView.y * 0.5), 0.0, 1.0);
}`;

const SPRITE_FS = `#version 300 es
precision highp float;
in vec2 vCorner;
flat in uint vKind;
flat in uint vGlyph;
uniform vec3 uColors[${MAX_KINDS}];
uniform bool uRound;
uniform bool uGlyphs;
uniform sampler2D uAtlas;
uniform vec2 uAtlasGrid;
out vec4 outColor;
void main() {
  if (uGlyphs) {
    int cols = int(uAtlasGrid.x);
    int g = int(vGlyph);
    vec2 uv = (vec2(float(g % cols), float(g / cols)) + vCorner) / uAtlasGrid;
    vec4 s = texture(uAtlas, uv); // premultiplied; blended ONE, ONE_MINUS_SRC_ALPHA
    if (s.a < 0.02) discard;
    outColor = s;
    return;
  }
  vec3 c = uColors[min(vKind, ${MAX_KINDS - 1}u)];
  if (uRound) {
    float d = length(vCorner - 0.5);
    if (d > 0.5) discard;
    if (d > 0.36) c *= 0.35; // a dark rim, so a sprite reads against any floor
  }
  outColor = vec4(c, 1.0);
}`;

// The selection marker: a square ring around one tile, a dark line outside a
// bright one so it reads on any ground. Sized in pixels, not tiles, so it
// stays visible zoomed out.
const MARK_VS = `#version 300 es
in vec2 aCorner;
uniform vec2 uTile;     // the marked tile
uniform float uPad;     // tiles of margin around it
uniform vec2 uCam;
uniform float uScale;
uniform vec2 uView;
out vec2 vPx;           // pixels from the ring's top-left corner
uniform float uSide;    // the ring's side, pixels
void main() {
  vPx = aCorner * uSide;
  vec2 tile = uTile - uPad + aCorner * (1.0 + 2.0 * uPad);
  vec2 px = (tile - uCam) * uScale;
  gl_Position = vec4(px.x / (uView.x * 0.5), -px.y / (uView.y * 0.5), 0.0, 1.0);
}`;

const MARK_FS = `#version 300 es
precision highp float;
in vec2 vPx;
uniform float uSide;
uniform float uLine;    // device pixels per line
uniform vec3 uColor;
out vec4 outColor;
void main() {
  float edge = min(min(vPx.x, vPx.y), min(uSide - vPx.x, uSide - vPx.y));
  if (edge > 2.0 * uLine) discard;
  outColor = edge < uLine ? vec4(0.0, 0.0, 0.0, 0.85) : vec4(uColor, 1.0);
}`;

interface Chunk { tex: WebGLTexture; cx: number; cy: number }

/** A copy of one page's tiles on the CPU, for hover lookups. */
export type PageTiles = Uint8Array;

export class MapRenderer {
  readonly canvas: HTMLCanvasElement;
  readonly camera = new Camera();
  private gl: WebGL2RenderingContext;
  private hello: Hello | null = null;

  private terrainProg: WebGLProgram;
  private spriteProg: WebGLProgram;
  private quad: WebGLBuffer;
  private terrainVAO: WebGLVertexArrayObject;
  private entityVAO: WebGLVertexArrayObject;
  private refuseVAO: WebGLVertexArrayObject;
  private entityPos: WebGLBuffer;
  private entityKind: WebGLBuffer;
  private entityGlyph: WebGLBuffer;
  private refusePos: WebGLBuffer;
  private refuseKind: WebGLBuffer;
  private refuseGlyph: WebGLBuffer;
  private atlas: Atlas | null = null;
  private u: Record<string, WebGLUniformLocation | null> = {};

  private chunks = new Map<number, Chunk>();
  /** CPU copies of the pages held, by page key; see pageKey. */
  readonly pages = new Map<number, PageTiles>();
  private entityCount = 0;
  private refuseCount = 0;
  private refuse: Frame['refuse'] = null;
  private scum: Frame['scum'] = null;
  private tintProg: WebGLProgram;
  private tintVAO: WebGLVertexArrayObject;
  private tintPos: WebGLBuffer;
  private tintColor: WebGLBuffer;
  private tintCount = 0;
  /** Filth on visible tiles, by tileKey. Rebuilt when refuse, scum or pages change. */
  readonly filth = new Map<number, Filth>();
  /** The last frame, for hover lookups of entities. */
  lastFrame: Frame | null = null;
  private markProg: WebGLProgram;
  private markVAO: WebGLVertexArrayObject;
  private mark: [number, number] | null = null;
  // Highlighted tiles (a job's): tint quads like filth's, in their own buffers.
  private hiVAO: WebGLVertexArrayObject;
  private hiPos: WebGLBuffer;
  private hiColor: WebGLBuffer;
  private hiCount = 0;
  private dirty = true;
  private raf = 0;

  constructor(canvas: HTMLCanvasElement) {
    this.canvas = canvas;
    const gl = canvas.getContext('webgl2', { antialias: false, alpha: false });
    if (!gl) throw new Error('This browser has no WebGL2.');
    this.gl = gl;

    this.terrainProg = program(gl, TERRAIN_VS, TERRAIN_FS);
    this.spriteProg = program(gl, SPRITE_VS, SPRITE_FS);
    this.tintProg = program(gl, TINT_VS, TINT_FS);
    for (const n of ['uCam', 'uScale', 'uView']) this.u['f.' + n] = gl.getUniformLocation(this.tintProg, n);
    this.markProg = program(gl, MARK_VS, MARK_FS);
    for (const n of ['uTile', 'uPad', 'uCam', 'uScale', 'uView', 'uSide', 'uLine', 'uColor']) {
      this.u['m.' + n] = gl.getUniformLocation(this.markProg, n);
    }
    for (const [prog, names] of [
      [this.terrainProg, ['uOrigin', 'uSize', 'uCam', 'uScale', 'uView', 'uTerrain', 'uTerrainColors', 'uRockColors', 'uFog',
        'uGlyphs', 'uAtlas', 'uAtlasGrid', 'uTerrainGlyph', 'uGlyphBackdrop']],
      [this.spriteProg, ['uCam', 'uScale', 'uView', 'uSize', 'uColors', 'uRound', 'uGlyphs', 'uAtlas', 'uAtlasGrid']],
    ] as const) {
      for (const n of names) this.u[(prog === this.terrainProg ? 't.' : 's.') + n] = gl.getUniformLocation(prog, n);
    }

    this.quad = buffer(gl, new Float32Array([0, 0, 1, 0, 0, 1, 1, 1]));
    this.terrainVAO = gl.createVertexArray()!;
    gl.bindVertexArray(this.terrainVAO);
    corner(gl, this.terrainProg, this.quad);

    this.entityPos = gl.createBuffer()!;
    this.entityKind = gl.createBuffer()!;
    this.entityGlyph = gl.createBuffer()!;
    this.entityVAO = this.spriteVAO(this.entityPos, this.entityKind, this.entityGlyph);
    this.refusePos = gl.createBuffer()!;
    this.refuseKind = gl.createBuffer()!;
    this.refuseGlyph = gl.createBuffer()!;
    this.refuseVAO = this.spriteVAO(this.refusePos, this.refuseKind, this.refuseGlyph);
    this.tintPos = gl.createBuffer()!;
    this.tintColor = gl.createBuffer()!;
    this.tintVAO = gl.createVertexArray()!;
    gl.bindVertexArray(this.tintVAO);
    corner(gl, this.tintProg, this.quad);
    const tPos = gl.getAttribLocation(this.tintProg, 'aPos');
    gl.bindBuffer(gl.ARRAY_BUFFER, this.tintPos);
    gl.enableVertexAttribArray(tPos);
    gl.vertexAttribIPointer(tPos, 2, gl.INT, 0, 0);
    gl.vertexAttribDivisor(tPos, 1);
    const tColor = gl.getAttribLocation(this.tintProg, 'aColor');
    gl.bindBuffer(gl.ARRAY_BUFFER, this.tintColor);
    gl.enableVertexAttribArray(tColor);
    gl.vertexAttribPointer(tColor, 4, gl.UNSIGNED_BYTE, true, 0, 0);
    gl.vertexAttribDivisor(tColor, 1);
    this.hiPos = gl.createBuffer()!;
    this.hiColor = gl.createBuffer()!;
    this.hiVAO = gl.createVertexArray()!;
    gl.bindVertexArray(this.hiVAO);
    corner(gl, this.tintProg, this.quad);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.hiPos);
    gl.enableVertexAttribArray(tPos);
    gl.vertexAttribIPointer(tPos, 2, gl.INT, 0, 0);
    gl.vertexAttribDivisor(tPos, 1);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.hiColor);
    gl.enableVertexAttribArray(tColor);
    gl.vertexAttribPointer(tColor, 4, gl.UNSIGNED_BYTE, true, 0, 0);
    gl.vertexAttribDivisor(tColor, 1);
    this.markVAO = gl.createVertexArray()!;
    gl.bindVertexArray(this.markVAO);
    corner(gl, this.markProg, this.quad);
    gl.bindVertexArray(null);

    new ResizeObserver(() => this.resize()).observe(canvas);
    this.resize();
    const loop = () => {
      if (this.dirty) this.draw();
      this.raf = requestAnimationFrame(loop);
    };
    this.raf = requestAnimationFrame(loop);
  }

  /** A new game: drop every page and take the game's names and palette. */
  reset(hello: Hello): void {
    this.hello = hello;
    for (const c of this.chunks.values()) this.gl.deleteTexture(c.tex);
    this.chunks.clear();
    this.pages.clear();
    this.entityCount = this.refuseCount = this.tintCount = this.hiCount = 0;
    this.refuse = null;
    this.scum = null;
    this.filth.clear();
    this.lastFrame = null;

    const gl = this.gl;
    gl.useProgram(this.terrainProg);
    gl.uniform3fv(this.u['t.uTerrainColors'], pad(palette.terrainColors(hello.enums.terrains), MAX_TERRAINS));
    gl.uniform3fv(this.u['t.uRockColors'], pad(palette.compositionColors(hello.enums.compositions), MAX_COMPOSITIONS));
    gl.uniform3fv(this.u['t.uFog'], palette.FOG);
    const terrainGlyphs = new Int32Array(MAX_TERRAINS).fill(-1);
    hello.glyphs.terrain.slice(0, MAX_TERRAINS).forEach((g, i) => { terrainGlyphs[i] = g; });
    gl.uniform1iv(this.u['t.uTerrainGlyph'], terrainGlyphs);
    gl.uniform3fv(this.u['t.uGlyphBackdrop'], palette.GLYPH_BACKDROP);

    if (this.atlas) gl.deleteTexture(this.atlas.texture);
    this.atlas = buildAtlas(gl, hello.glyphs.symbols);
    gl.useProgram(this.spriteProg);
    const kinds = pad(palette.kindColors(hello.enums.kinds), MAX_KINDS);
    kinds.set(palette.CORPSE, KIND_CORPSE * 3);
    gl.uniform3fv(this.u['s.uColors'], kinds);
    this.dirty = true;
  }

  /** Take one decoded frame: upload its pages, entities and refuse. */
  applyFrame(f: Frame): void {
    const gl = this.gl;
    if (f.tilesReset) {
      // The worker starts the pages over. Chunk textures are kept (they are
      // overwritten as pages arrive), but the CPU copies must go, or hover
      // would read pages the worker no longer counts as held.
      this.pages.clear();
      for (const c of this.chunks.values()) {
        gl.bindTexture(gl.TEXTURE_2D, c.tex);
        gl.texSubImage2D(gl.TEXTURE_2D, 0, 0, 0, CHUNK_TILES, CHUNK_TILES, gl.RG_INTEGER, gl.UNSIGNED_BYTE,
          new Uint8Array(CHUNK_TILES * CHUNK_TILES * TILE_BYTES));
      }
    }
    const pageBytes = PAGE_TILES * TILE_BYTES;
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
    for (let i = 0; i < f.pages.count; i++) {
      const px = f.pages.px[i], py = f.pages.py[i];
      const tiles = f.pages.tiles.subarray(i * pageBytes, (i + 1) * pageBytes);
      const chunk = this.chunk(px >> 5, py >> 5);
      gl.bindTexture(gl.TEXTURE_2D, chunk.tex);
      gl.texSubImage2D(gl.TEXTURE_2D, 0, (px & (CHUNK_PAGES - 1)) * PAGE_SIDE, (py & (CHUNK_PAGES - 1)) * PAGE_SIDE,
        PAGE_SIDE, PAGE_SIDE, gl.RG_INTEGER, gl.UNSIGNED_BYTE, tiles);
      this.pages.set(pageKey(px, py), tiles.slice());
    }

    // Entities and refuse only where the colony can see: a dormant alien in
    // an undiscovered cavern must not give the cavern away, as the TUI never
    // draws an occupant on an unexplored tile (docs/fog-of-war.md). A tile
    // on a page not held counts as unseen.
    const e = f.entities;
    const pos = new Int32Array(e.count * 2);
    const kind = new Uint8Array(e.count);
    const glyph = new Uint16Array(e.count);
    let n = 0;
    for (let i = 0; i < e.count; i++) {
      if (!this.visible(e.x[i], e.y[i])) continue;
      pos[2 * n] = e.x[i]; pos[2 * n + 1] = e.y[i]; kind[n] = e.kind[i]; glyph[n] = e.glyph[i];
      n++;
    }
    this.entityCount = n;
    gl.bindBuffer(gl.ARRAY_BUFFER, this.entityPos);
    gl.bufferData(gl.ARRAY_BUFFER, pos.subarray(0, 2 * n), gl.DYNAMIC_DRAW);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.entityKind);
    gl.bufferData(gl.ARRAY_BUFFER, kind.subarray(0, n), gl.DYNAMIC_DRAW);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.entityGlyph);
    gl.bufferData(gl.ARRAY_BUFFER, glyph.subarray(0, n), gl.DYNAMIC_DRAW);

    // Refuse and scum arrive only when they change, but what is visible
    // changes as the colony digs, so keep the lists and refilter them
    // whenever either list or any page changed.
    if (f.refuse) this.refuse = f.refuse;
    if (f.scum) this.scum = f.scum;
    if (f.refuse || f.scum || f.pages.count > 0 || f.tilesReset) this.rebuildFilth();
    this.lastFrame = f;
    this.dirty = true;
  }

  /**
   * Rebuild the filth on visible tiles: the tint quads (gore, scum, or both
   * on one tile) and the body markers, which stay sprites.
   */
  private rebuildFilth(): void {
    const gl = this.gl;
    const hello = this.hello!;
    this.filth.clear();
    const at = (x: number, y: number) => {
      const k = tileKey(x, y);
      let v = this.filth.get(k);
      if (!v) { v = { gore: 0, scum: 0, corpses: 0 }; this.filth.set(k, v); }
      return v;
    };
    const r = this.refuse;
    if (r) for (let i = 0; i < r.count; i++) {
      if (!this.visible(r.x[i], r.y[i])) continue;
      const v = at(r.x[i], r.y[i]);
      v.gore = r.gore[i];
      v.corpses = r.corpses[i];
    }
    const sc = this.scum;
    if (sc) for (let i = 0; i < sc.count; i++) {
      if (!this.visible(sc.x[i], sc.y[i])) continue;
      at(sc.x[i], sc.y[i]).scum = sc.amount[i];
    }

    const n = this.filth.size;
    const tpos = new Int32Array(n * 2);
    const tcolor = new Uint8Array(n * 4);
    const bpos = new Int32Array(n * 2);
    const bkind = new Uint8Array(n);
    const bglyph = new Uint16Array(n);
    let t = 0, b = 0;
    for (const [k, v] of this.filth) {
      const x = k % TILE_KEY_ROW, y = Math.floor(k / TILE_KEY_ROW);
      if (v.gore > 0 || v.scum > 0) {
        const tint = palette.filthTint(v.gore / Math.max(1, hello.goreMax), v.scum / Math.max(1, hello.scumMax));
        tpos[2 * t] = x; tpos[2 * t + 1] = y;
        tcolor.set(tint, 4 * t);
        t++;
      }
      if (v.corpses > 0) {
        bpos[2 * b] = x; bpos[2 * b + 1] = y;
        bkind[b] = KIND_CORPSE;
        bglyph[b] = hello.glyphs.corpse;
        b++;
      }
    }
    this.tintCount = t;
    gl.bindBuffer(gl.ARRAY_BUFFER, this.tintPos);
    gl.bufferData(gl.ARRAY_BUFFER, tpos.subarray(0, 2 * t), gl.DYNAMIC_DRAW);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.tintColor);
    gl.bufferData(gl.ARRAY_BUFFER, tcolor.subarray(0, 4 * t), gl.DYNAMIC_DRAW);
    this.refuseCount = b;
    gl.bindBuffer(gl.ARRAY_BUFFER, this.refusePos);
    gl.bufferData(gl.ARRAY_BUFFER, bpos.subarray(0, 2 * b), gl.DYNAMIC_DRAW);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.refuseKind);
    gl.bufferData(gl.ARRAY_BUFFER, bkind.subarray(0, b), gl.DYNAMIC_DRAW);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.refuseGlyph);
    gl.bufferData(gl.ARRAY_BUFFER, bglyph.subarray(0, b), gl.DYNAMIC_DRAW);
  }

  /** The filth on (x, y) if the colony can see it, for the hover readout. */
  filthAt(x: number, y: number): Filth | undefined {
    return this.filth.get(tileKey(x, y));
  }

  /** Whether the colony can see (x, y): its page is held and the tile visible. */
  visible(x: number, y: number): boolean {
    const t = this.tileAt(x, y);
    return t !== null && (t[1] & TILE_VISIBLE) !== 0;
  }

  /** The two tile bytes at (x, y), or null if that page is not held. */
  tileAt(x: number, y: number): [number, number] | null {
    const page = this.pages.get(pageKey(Math.floor(x / PAGE_SIDE), Math.floor(y / PAGE_SIDE)));
    if (!page) return null;
    const off = ((y & (PAGE_SIDE - 1)) * PAGE_SIDE + (x & (PAGE_SIDE - 1))) * TILE_BYTES;
    return [page[off], page[off + 1]];
  }

  /**
   * Tint a set of tiles (a job's), each with a premultiplied RGBA color, or
   * none. Drawn over filth and under creatures, at every zoom.
   */
  setHighlight(tiles: { x: number; y: number; color: Uint8Array }[] | null): void {
    const n = tiles?.length ?? 0;
    const pos = new Int32Array(n * 2);
    const color = new Uint8Array(n * 4);
    tiles?.forEach((t, i) => { pos[2 * i] = t.x; pos[2 * i + 1] = t.y; color.set(t.color, 4 * i); });
    const gl = this.gl;
    gl.bindBuffer(gl.ARRAY_BUFFER, this.hiPos);
    gl.bufferData(gl.ARRAY_BUFFER, pos, gl.DYNAMIC_DRAW);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.hiColor);
    gl.bufferData(gl.ARRAY_BUFFER, color, gl.DYNAMIC_DRAW);
    this.hiCount = n;
    this.dirty = true;
  }

  /** Mark one tile (the inspector's selection), or none. */
  setMark(at: [number, number] | null): void {
    const m = this.mark;
    if (m === at || (m && at && m[0] === at[0] && m[1] === at[1])) return;
    this.mark = at;
    this.dirty = true;
  }

  /** Ask for a redraw on the next animation frame (the camera moved). */
  invalidate(): void { this.dirty = true; }

  destroy(): void { cancelAnimationFrame(this.raf); }

  private chunk(cx: number, cy: number): Chunk {
    const key = pageKey(cx, cy);
    let c = this.chunks.get(key);
    if (!c) {
      const gl = this.gl;
      const tex = gl.createTexture()!;
      gl.bindTexture(gl.TEXTURE_2D, tex);
      // WebGL zero-fills new storage: zero is rock, not visible, i.e. fog.
      gl.texStorage2D(gl.TEXTURE_2D, 1, gl.RG8UI, CHUNK_TILES, CHUNK_TILES);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      c = { tex, cx, cy };
      this.chunks.set(key, c);
    }
    return c;
  }

  private spriteVAO(posBuf: WebGLBuffer, kindBuf: WebGLBuffer, glyphBuf: WebGLBuffer): WebGLVertexArrayObject {
    const gl = this.gl;
    const vao = gl.createVertexArray()!;
    gl.bindVertexArray(vao);
    corner(gl, this.spriteProg, this.quad);
    const aPos = gl.getAttribLocation(this.spriteProg, 'aPos');
    gl.bindBuffer(gl.ARRAY_BUFFER, posBuf);
    gl.enableVertexAttribArray(aPos);
    gl.vertexAttribIPointer(aPos, 2, gl.INT, 0, 0);
    gl.vertexAttribDivisor(aPos, 1);
    const aKind = gl.getAttribLocation(this.spriteProg, 'aKind');
    gl.bindBuffer(gl.ARRAY_BUFFER, kindBuf);
    gl.enableVertexAttribArray(aKind);
    gl.vertexAttribIPointer(aKind, 1, gl.UNSIGNED_BYTE, 0, 0);
    gl.vertexAttribDivisor(aKind, 1);
    const aGlyph = gl.getAttribLocation(this.spriteProg, 'aGlyph');
    gl.bindBuffer(gl.ARRAY_BUFFER, glyphBuf);
    gl.enableVertexAttribArray(aGlyph);
    gl.vertexAttribIPointer(aGlyph, 1, gl.UNSIGNED_SHORT, 0, 0);
    gl.vertexAttribDivisor(aGlyph, 1);
    return vao;
  }

  private resize(): void {
    const dpr = window.devicePixelRatio || 1;
    const w = this.canvas.clientWidth, h = this.canvas.clientHeight;
    this.canvas.width = Math.max(1, Math.round(w * dpr));
    this.canvas.height = Math.max(1, Math.round(h * dpr));
    this.camera.width = w;
    this.camera.height = h;
    this.dirty = true;
  }

  private draw(): void {
    this.dirty = false;
    const gl = this.gl;
    const cam = this.camera;
    const dpr = this.canvas.width / Math.max(1, cam.width);
    const scale = cam.zoom * dpr;
    gl.viewport(0, 0, this.canvas.width, this.canvas.height);
    gl.clearColor(palette.OFF_MAP[0], palette.OFF_MAP[1], palette.OFF_MAP[2], 1);
    gl.clear(gl.COLOR_BUFFER_BIT);
    const hello = this.hello;
    if (!hello) return;

    // The map's own area in fog, so tiles not received yet read as unknown
    // ground rather than as the void past the map's edge.
    const left = Math.round((0 - cam.cx) * scale + this.canvas.width / 2);
    const top = Math.round((0 - cam.cy) * scale + this.canvas.height / 2);
    const right = Math.round((hello.width - cam.cx) * scale + this.canvas.width / 2);
    const bottom = Math.round((hello.height - cam.cy) * scale + this.canvas.height / 2);
    gl.enable(gl.SCISSOR_TEST);
    gl.scissor(left, this.canvas.height - bottom, Math.max(0, right - left), Math.max(0, bottom - top));
    gl.clearColor(palette.FOG[0], palette.FOG[1], palette.FOG[2], 1);
    gl.clear(gl.COLOR_BUFFER_BIT);
    gl.disable(gl.SCISSOR_TEST);

    // Terrain: one quad per chunk that is on screen. A chunk with no texture
    // yet has nothing held in it; the clear color stands in.
    gl.useProgram(this.terrainProg);
    gl.bindVertexArray(this.terrainVAO);
    gl.uniform2f(this.u['t.uCam'], cam.cx, cam.cy);
    gl.uniform1f(this.u['t.uScale'], scale);
    gl.uniform2f(this.u['t.uView'], this.canvas.width, this.canvas.height);
    const glyphs = this.atlas !== null && cam.zoom >= GLYPH_ZOOM;
    gl.uniform1i(this.u['t.uGlyphs'], glyphs ? 1 : 0);
    if (this.atlas) {
      gl.activeTexture(gl.TEXTURE1);
      gl.bindTexture(gl.TEXTURE_2D, this.atlas.texture);
      gl.uniform1i(this.u['t.uAtlas'], 1);
      gl.uniform2f(this.u['t.uAtlasGrid'], this.atlas.cols, this.atlas.rows);
    }
    gl.uniform1i(this.u['t.uTerrain'], 0);
    gl.activeTexture(gl.TEXTURE0);
    const view = cam.visibleTiles(hello.width, hello.height);
    for (const c of this.chunks.values()) {
      const ox = c.cx * CHUNK_TILES, oy = c.cy * CHUNK_TILES;
      const w = Math.min(CHUNK_TILES, hello.width - ox), h = Math.min(CHUNK_TILES, hello.height - oy);
      if (ox >= view.x1 || oy >= view.y1 || ox + w <= view.x0 || oy + h <= view.y0) continue;
      gl.bindTexture(gl.TEXTURE_2D, c.tex);
      gl.uniform2f(this.u['t.uOrigin'], ox, oy);
      gl.uniform2f(this.u['t.uSize'], w, h);
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
    }

    // Filth tints the tiles under everything that stands on them.
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA); // tints and the atlas are premultiplied
    if (this.tintCount > 0) {
      gl.useProgram(this.tintProg);
      gl.bindVertexArray(this.tintVAO);
      gl.uniform2f(this.u['f.uCam'], cam.cx, cam.cy);
      gl.uniform1f(this.u['f.uScale'], scale);
      gl.uniform2f(this.u['f.uView'], this.canvas.width, this.canvas.height);
      gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, this.tintCount);
    }
    if (this.hiCount > 0) {
      gl.useProgram(this.tintProg);
      gl.bindVertexArray(this.hiVAO);
      gl.uniform2f(this.u['f.uCam'], cam.cx, cam.cy);
      gl.uniform1f(this.u['f.uScale'], scale);
      gl.uniform2f(this.u['f.uView'], this.canvas.width, this.canvas.height);
      gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, this.hiCount);
    }

    // Bodies under entities; both at least a few pixels wide when zoomed out.
    gl.useProgram(this.spriteProg);
    gl.uniform2f(this.u['s.uCam'], cam.cx, cam.cy);
    gl.uniform1f(this.u['s.uScale'], scale);
    gl.uniform2f(this.u['s.uView'], this.canvas.width, this.canvas.height);
    gl.uniform1i(this.u['s.uGlyphs'], glyphs ? 1 : 0);
    if (this.atlas) {
      gl.uniform1i(this.u['s.uAtlas'], 1);
      gl.uniform2f(this.u['s.uAtlasGrid'], this.atlas.cols, this.atlas.rows);
    }
    if (this.refuseCount > 0) {
      gl.bindVertexArray(this.refuseVAO);
      gl.uniform1f(this.u['s.uSize'], glyphs ? 0.85 : Math.max(0.45, 3 / cam.zoom));
      gl.uniform1i(this.u['s.uRound'], 0);
      gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, this.refuseCount);
    }
    if (this.entityCount > 0) {
      gl.bindVertexArray(this.entityVAO);
      gl.uniform1f(this.u['s.uSize'], glyphs ? 1.0 : Math.max(0.9, 5 / cam.zoom));
      gl.uniform1i(this.u['s.uRound'], 1);
      gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, this.entityCount);
    }

    if (this.mark) {
      // At least 14 CSS pixels across, so a zoomed-out marker still rings
      // the dot it belongs to.
      const pad = Math.max(0.15, (14 / cam.zoom - 1) / 2);
      gl.useProgram(this.markProg);
      gl.bindVertexArray(this.markVAO);
      gl.uniform2f(this.u['m.uTile'], this.mark[0], this.mark[1]);
      gl.uniform1f(this.u['m.uPad'], pad);
      gl.uniform2f(this.u['m.uCam'], cam.cx, cam.cy);
      gl.uniform1f(this.u['m.uScale'], scale);
      gl.uniform2f(this.u['m.uView'], this.canvas.width, this.canvas.height);
      gl.uniform1f(this.u['m.uSide'], (1 + 2 * pad) * scale);
      gl.uniform1f(this.u['m.uLine'], Math.max(1, Math.round(1.5 * dpr)));
      gl.uniform3fv(this.u['m.uColor'], palette.MARK);
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
    }
    gl.disable(gl.BLEND);
    gl.bindVertexArray(null);
  }
}

/** One number per tile, for the filth map. */
const TILE_KEY_ROW = 1 << 26; // wider than any map, and x + y*row stays exact in a double
function tileKey(x: number, y: number): number {
  return y * TILE_KEY_ROW + x;
}

/** One number per (x, y) pair of page or chunk coordinates. */
export function pageKey(x: number, y: number): number {
  return y * 0x100000 + x;
}

function pad(colors: number[][], n: number): Float32Array {
  const out = new Float32Array(n * 3);
  for (let i = 0; i < n; i++) out.set(colors[i] ?? palette.MISSING, i * 3);
  return out;
}

function buffer(gl: WebGL2RenderingContext, data: Float32Array): WebGLBuffer {
  const b = gl.createBuffer()!;
  gl.bindBuffer(gl.ARRAY_BUFFER, b);
  gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
  return b;
}

function corner(gl: WebGL2RenderingContext, prog: WebGLProgram, quad: WebGLBuffer): void {
  const a = gl.getAttribLocation(prog, 'aCorner');
  gl.bindBuffer(gl.ARRAY_BUFFER, quad);
  gl.enableVertexAttribArray(a);
  gl.vertexAttribPointer(a, 2, gl.FLOAT, false, 0, 0);
}

function program(gl: WebGL2RenderingContext, vs: string, fs: string): WebGLProgram {
  const p = gl.createProgram()!;
  for (const [type, src] of [[gl.VERTEX_SHADER, vs], [gl.FRAGMENT_SHADER, fs]] as const) {
    const s = gl.createShader(type)!;
    gl.shaderSource(s, src);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) throw new Error(`shader: ${gl.getShaderInfoLog(s)}`);
    gl.attachShader(p, s);
  }
  gl.linkProgram(p);
  if (!gl.getProgramParameter(p, gl.LINK_STATUS)) throw new Error(`program: ${gl.getProgramInfoLog(p)}`);
  return p;
}

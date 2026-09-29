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

import type { Frame, Hello } from '../../wire/decode.js';
import { PAGE_SIDE, PAGE_TILES, TILE_BYTES, TILE_VISIBLE } from '../../wire/decode.js';
import { Camera } from './camera';
import * as palette from './palette';

const CHUNK_PAGES = 32;
const CHUNK_TILES = CHUNK_PAGES * PAGE_SIDE; // 2048: under every WebGL2 max texture size
const MAX_TERRAINS = 32;
const MAX_COMPOSITIONS = 16;
const MAX_KINDS = 8;
// Instance "kinds" past the entity kinds, for refuse.
const KIND_GORE = MAX_KINDS - 2;
const KIND_CORPSE = MAX_KINDS - 1;

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
out vec4 outColor;
void main() {
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
  }
  // A faint grid once tiles are big enough to tell apart.
  if (uScale >= 14.0) {
    vec2 f = fract(vTile);
    float edge = min(min(f.x, f.y), min(1.0 - f.x, 1.0 - f.y)) * uScale;
    if (edge < 0.6) c *= 0.88;
  }
  outColor = vec4(c, 1.0);
}`;

const SPRITE_VS = `#version 300 es
in vec2 aCorner;
in ivec2 aPos;          // per instance: tile
in uint aKind;          // per instance: palette index
uniform vec2 uCam;
uniform float uScale;
uniform vec2 uView;
uniform float uSize;    // sprite size in tiles
out vec2 vCorner;
flat out uint vKind;
void main() {
  vCorner = aCorner;
  vKind = aKind;
  vec2 tile = vec2(aPos) + 0.5 + (aCorner - 0.5) * uSize;
  vec2 px = (tile - uCam) * uScale;
  gl_Position = vec4(px.x / (uView.x * 0.5), -px.y / (uView.y * 0.5), 0.0, 1.0);
}`;

const SPRITE_FS = `#version 300 es
precision highp float;
in vec2 vCorner;
flat in uint vKind;
uniform vec3 uColors[${MAX_KINDS}];
uniform bool uRound;
out vec4 outColor;
void main() {
  vec3 c = uColors[min(vKind, ${MAX_KINDS - 1}u)];
  if (uRound) {
    float d = length(vCorner - 0.5);
    if (d > 0.5) discard;
    if (d > 0.36) c *= 0.35; // a dark rim, so a sprite reads against any floor
  }
  outColor = vec4(c, 1.0);
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
  private refusePos: WebGLBuffer;
  private refuseKind: WebGLBuffer;
  private u: Record<string, WebGLUniformLocation | null> = {};

  private chunks = new Map<number, Chunk>();
  /** CPU copies of the pages held, by page key; see pageKey. */
  readonly pages = new Map<number, PageTiles>();
  private entityCount = 0;
  private refuseCount = 0;
  private refuse: Frame['refuse'] = null;
  /** The last frame, for hover lookups of entities. */
  lastFrame: Frame | null = null;
  private dirty = true;
  private raf = 0;

  constructor(canvas: HTMLCanvasElement) {
    this.canvas = canvas;
    const gl = canvas.getContext('webgl2', { antialias: false, alpha: false });
    if (!gl) throw new Error('This browser has no WebGL2.');
    this.gl = gl;

    this.terrainProg = program(gl, TERRAIN_VS, TERRAIN_FS);
    this.spriteProg = program(gl, SPRITE_VS, SPRITE_FS);
    for (const [prog, names] of [
      [this.terrainProg, ['uOrigin', 'uSize', 'uCam', 'uScale', 'uView', 'uTerrain', 'uTerrainColors', 'uRockColors', 'uFog']],
      [this.spriteProg, ['uCam', 'uScale', 'uView', 'uSize', 'uColors', 'uRound']],
    ] as const) {
      for (const n of names) this.u[(prog === this.terrainProg ? 't.' : 's.') + n] = gl.getUniformLocation(prog, n);
    }

    this.quad = buffer(gl, new Float32Array([0, 0, 1, 0, 0, 1, 1, 1]));
    this.terrainVAO = gl.createVertexArray()!;
    gl.bindVertexArray(this.terrainVAO);
    corner(gl, this.terrainProg, this.quad);

    this.entityPos = gl.createBuffer()!;
    this.entityKind = gl.createBuffer()!;
    this.entityVAO = this.spriteVAO(this.entityPos, this.entityKind);
    this.refusePos = gl.createBuffer()!;
    this.refuseKind = gl.createBuffer()!;
    this.refuseVAO = this.spriteVAO(this.refusePos, this.refuseKind);
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
    this.entityCount = this.refuseCount = 0;
    this.refuse = null;
    this.lastFrame = null;

    const gl = this.gl;
    gl.useProgram(this.terrainProg);
    gl.uniform3fv(this.u['t.uTerrainColors'], pad(palette.terrainColors(hello.enums.terrains), MAX_TERRAINS));
    gl.uniform3fv(this.u['t.uRockColors'], pad(palette.compositionColors(hello.enums.compositions), MAX_COMPOSITIONS));
    gl.uniform3fv(this.u['t.uFog'], palette.FOG);
    gl.useProgram(this.spriteProg);
    const kinds = pad(palette.kindColors(hello.enums.kinds), MAX_KINDS);
    kinds.set(palette.GORE, KIND_GORE * 3);
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
    let n = 0;
    for (let i = 0; i < e.count; i++) {
      if (!this.visible(e.x[i], e.y[i])) continue;
      pos[2 * n] = e.x[i]; pos[2 * n + 1] = e.y[i]; kind[n] = e.kind[i];
      n++;
    }
    this.entityCount = n;
    gl.bindBuffer(gl.ARRAY_BUFFER, this.entityPos);
    gl.bufferData(gl.ARRAY_BUFFER, pos.subarray(0, 2 * n), gl.DYNAMIC_DRAW);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.entityKind);
    gl.bufferData(gl.ARRAY_BUFFER, kind.subarray(0, n), gl.DYNAMIC_DRAW);

    // Refuse arrives only when it changes, but what is visible changes as
    // the colony digs, so keep the list and refilter it every frame.
    if (f.refuse) this.refuse = f.refuse;
    const r = this.refuse;
    if (r) {
      const rpos = new Int32Array(r.count * 2);
      const rkind = new Uint8Array(r.count);
      let m = 0;
      for (let i = 0; i < r.count; i++) {
        if (!this.visible(r.x[i], r.y[i])) continue;
        rpos[2 * m] = r.x[i]; rpos[2 * m + 1] = r.y[i];
        rkind[m] = r.corpses[i] > 0 ? KIND_CORPSE : KIND_GORE;
        m++;
      }
      this.refuseCount = m;
      gl.bindBuffer(gl.ARRAY_BUFFER, this.refusePos);
      gl.bufferData(gl.ARRAY_BUFFER, rpos.subarray(0, 2 * m), gl.DYNAMIC_DRAW);
      gl.bindBuffer(gl.ARRAY_BUFFER, this.refuseKind);
      gl.bufferData(gl.ARRAY_BUFFER, rkind.subarray(0, m), gl.DYNAMIC_DRAW);
    }
    this.lastFrame = f;
    this.dirty = true;
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

  private spriteVAO(posBuf: WebGLBuffer, kindBuf: WebGLBuffer): WebGLVertexArrayObject {
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

    // Refuse under entities; both at least a few pixels wide when zoomed out.
    gl.useProgram(this.spriteProg);
    gl.uniform2f(this.u['s.uCam'], cam.cx, cam.cy);
    gl.uniform1f(this.u['s.uScale'], scale);
    gl.uniform2f(this.u['s.uView'], this.canvas.width, this.canvas.height);
    if (this.refuseCount > 0) {
      gl.bindVertexArray(this.refuseVAO);
      gl.uniform1f(this.u['s.uSize'], Math.max(0.45, 3 / cam.zoom));
      gl.uniform1i(this.u['s.uRound'], 0);
      gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, this.refuseCount);
    }
    if (this.entityCount > 0) {
      gl.bindVertexArray(this.entityVAO);
      gl.uniform1f(this.u['s.uSize'], Math.max(0.9, 5 / cam.zoom));
      gl.uniform1i(this.u['s.uRound'], 1);
      gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, this.entityCount);
    }
    gl.bindVertexArray(null);
  }
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

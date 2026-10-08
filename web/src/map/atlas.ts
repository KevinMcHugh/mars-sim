// The emoji atlas: every glyph the engine can pick (Hello.glyphs.symbols),
// drawn once with the browser's own emoji font into a grid of square cells,
// uploaded as one mipmapped texture. Glyph i is at cell (i % cols, i / cols).
//
// Drawing text into a canvas is the only portable way to get color emoji into
// WebGL, and it means the map uses the same emoji font the player sees
// everywhere else (Apple, Segoe or Noto): the browser's equivalent of the
// terminal drawing them.

import { EMOJI_FONT } from '../emoji';

export interface Atlas {
  texture: WebGLTexture;
  cols: number;
  rows: number;
  /** Set when the renderer drops this atlas, so a late sprite load leaves it alone. */
  disposed?: boolean;
}

/** An SVG to paint over a symbol's cell: a species-pack sprite (docs/species-pack.md). */
export interface AtlasImage {
  index: number;
  svg: string;
}

// Big enough to stay crisp at the top zoom on a 2x screen (64 CSS px a tile).
const CELL = 128;
const COLS = 16;
const FONT = EMOJI_FONT;

/**
 * Draw symbols into a new atlas texture. images, if any, are painted over
 * their cells once they decode, and the texture is uploaded again; onPainted
 * runs then, so the map redraws. Until then (or if an image fails) the cell
 * shows its symbol.
 */
export function buildAtlas(
  gl: WebGL2RenderingContext,
  symbols: string[],
  images: AtlasImage[] = [],
  onPainted: () => void = () => {},
): Atlas {
  const cols = COLS;
  const rows = Math.max(1, Math.ceil(symbols.length / cols));
  const canvas = document.createElement('canvas');
  canvas.width = cols * CELL;
  canvas.height = rows * CELL;
  const ctx = canvas.getContext('2d')!;
  ctx.textAlign = 'center';
  ctx.textBaseline = 'alphabetic';
  ctx.font = `${Math.round(CELL * 0.8)}px ${FONT}`;
  symbols.forEach((s, i) => {
    if (s.trim() === '') return; // open floor is blank: nothing to draw
    const m = ctx.measureText(s);
    // Center the glyph's ink, not its text box: emoji fonts disagree on
    // where the baseline sits.
    const x = (i % cols) * CELL + CELL / 2;
    const ink = m.actualBoundingBoxAscent + m.actualBoundingBoxDescent;
    const y = Math.floor(i / cols) * CELL + CELL / 2 + ink / 2 - m.actualBoundingBoxDescent;
    ctx.fillText(s, x, y);
  });

  const texture = gl.createTexture()!;
  upload(gl, texture, canvas);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
  const atlas: Atlas = { texture, cols, rows };

  if (images.length > 0) {
    // One upload once every image has settled, not one per image.
    void Promise.allSettled(images.map(async ({ index, svg }) => {
      const img = await loadSVG(svg);
      const x = (index % cols) * CELL;
      const y = Math.floor(index / cols) * CELL;
      ctx.clearRect(x, y, CELL, CELL);
      ctx.drawImage(img, x, y, CELL, CELL);
    })).then(() => {
      if (atlas.disposed) return;
      upload(gl, texture, canvas);
      onPainted();
    });
  }
  return atlas;
}

function upload(gl: WebGL2RenderingContext, texture: WebGLTexture, canvas: HTMLCanvasElement): void {
  gl.bindTexture(gl.TEXTURE_2D, texture);
  // Premultiplied, so blending and mipmapping don't bleed dark fringes from
  // the transparent pixels around each glyph.
  gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, true);
  gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, canvas);
  gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false);
  gl.generateMipmap(gl.TEXTURE_2D);
}

/**
 * Decode an SVG as an image. An image element never runs a document's
 * scripts, which matters because a sprite is text a model wrote; never put
 * one into the page's DOM as markup instead.
 */
function loadSVG(svg: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error('sprite did not decode'));
    img.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
  });
}

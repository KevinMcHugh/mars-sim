// Hand-drawn SVG art for terrain whose emoji draws badly on the map
// (docs/tile-art.md). The atlas paints each over its glyph's cell, the same
// way it paints a species-pack sprite; the emoji stays everywhere text is
// shown (hover line, legend, the TUI), and on the map until the SVG decodes.
//
// A terrain glyph is drawn edge to edge across its tile (fract(vTile) in the
// terrain shader), so art drawn to the cell's edges joins its neighbours: a
// run of wall tiles reads as one stretch of masonry, not a row of stickers.

/** The cell is 128 units square; every piece of art uses this viewBox. */
const BOX = 128;
const COURSE = 32; // one course of bricks: four per tile, so courses tile vertically
const BRICK = 64; // two bricks per course, so the bond tiles horizontally
const MORTAR = 4; // the gap between bricks; half of it sits on each side of a seam

// Rust tones, picked per brick. A brick split across a tile edge is drawn as
// two halves on two tiles, so both halves must take the same tone: in an
// offset course the brick at -32 and the brick at 96 are the same brick.
const TONES = ['#a4492f', '#8f3d27', '#b5573a', '#9a4430'];

function brick(x: number, y: number, tone: string): string {
  const w = BRICK - MORTAR, h = COURSE - MORTAR;
  const x0 = x + MORTAR / 2, y0 = y + MORTAR / 2;
  return `<rect x="${x0}" y="${y0}" width="${w}" height="${h}" rx="2" fill="${tone}"/>`
    + `<rect x="${x0 + 2}" y="${y0 + 2}" width="${w - 4}" height="3" rx="1.5" fill="#ffffff" fill-opacity="0.16"/>`
    + `<rect x="${x0 + 2}" y="${y0 + h - 5}" width="${w - 4}" height="3" rx="1.5" fill="#000000" fill-opacity="0.18"/>`;
}

function brickWall(): string {
  const parts: string[] = [];
  for (let row = 0; row < BOX / COURSE; row++) {
    const y = row * COURSE;
    const tone = (i: number) => TONES[(row * 3 + i) % TONES.length];
    if (row % 2 === 0) {
      parts.push(brick(0, y, tone(0)), brick(BRICK, y, tone(1)));
    } else {
      // Running bond: this course is offset by half a brick. The brick at
      // -32 is the far half of the one at 96 on the tile to the left.
      parts.push(brick(-BRICK / 2, y, tone(2)), brick(BRICK / 2, y, tone(1)), brick(BOX - BRICK / 2, y, tone(2)));
    }
  }
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${BOX} ${BOX}">`
    + `<rect width="${BOX}" height="${BOX}" fill="#4a241a"/>${parts.join('')}</svg>`;
}

/** SVG art keyed by the glyph it replaces on the map. */
export const TILE_ART: Readonly<Record<string, string>> = {
  '\u{1F9F1}': brickWall(), // 🧱 a built wall (glyphs.Wall)
};

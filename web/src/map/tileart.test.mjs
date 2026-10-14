// Hand-drawn tile art (docs/tile-art.md).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { TILE_ART } from './tileart.ts';

test('the wall glyph has art, drawn on the atlas cell', () => {
  const svg = TILE_ART['\u{1F9F1}'];
  assert.ok(svg, 'no art for the wall glyph');
  assert.match(svg, /^<svg xmlns="http:\/\/www\.w3\.org\/2000\/svg" viewBox="0 0 128 128">/);
  assert.match(svg, /<\/svg>$/);
});

test('every piece of art is a lone SVG with no script or external reference', () => {
  for (const [glyph, svg] of Object.entries(TILE_ART)) {
    assert.doesNotMatch(svg, /<script|href=/i, glyph);
  }
});

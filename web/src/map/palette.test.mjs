// The map's depth palette (docs/depth-palette.md). node --test runs it
// against the TypeScript module itself: Node 22 strips the types on import.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { DARK_LEVEL, FOG, MISSING, depthDarkness, depthPalette } from './palette.ts';

const luma = (c) => 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
const terrains = ['rock', 'floor', 'wall', 'no such terrain'];
const rocks = ['ordinary rock'];

test('darkness runs from none at level 1 to full at DARK_LEVEL and stays there', () => {
  assert.equal(depthDarkness(0), 0);
  assert.equal(depthDarkness(1), 0);
  assert.equal(depthDarkness(DARK_LEVEL), 1);
  assert.equal(depthDarkness(DARK_LEVEL + 3), 1);
  for (let l = 1; l < DARK_LEVEL; l++) assert.ok(depthDarkness(l + 1) > depthDarkness(l));
});

test('level 1 is the landing palette unchanged', () => {
  const d = depthPalette(1, terrains, rocks);
  assert.deepEqual(d.fog, FOG);
});

test('every level down is darker, ending in black fog and charcoal floor', () => {
  let prev = depthPalette(1, terrains, rocks);
  for (let l = 2; l <= DARK_LEVEL; l++) {
    const d = depthPalette(l, terrains, rocks);
    assert.ok(luma(d.terrains[1]) < luma(prev.terrains[1]), `floor at ${l}`);
    assert.ok(luma(d.terrains[2]) < luma(prev.terrains[2]), `wall at ${l}`);
    assert.ok(luma(d.rocks[0]) < luma(prev.rocks[0]), `rock at ${l}`);
    assert.ok(luma(d.fog) < luma(prev.fog), `fog at ${l}`);
    prev = d;
  }
  assert.deepEqual(prev.fog, [0, 0, 0]);
  const floor = prev.terrains[1];
  // Charcoal: dark and nearly grey.
  assert.ok(luma(floor) < 0.25 && Math.max(...floor) - Math.min(...floor) < 0.05, `floor ${floor}`);
});

test('a terrain with no color stays magenta at depth', () => {
  assert.deepEqual(depthPalette(DARK_LEVEL, terrains, rocks).terrains[3], MISSING);
});

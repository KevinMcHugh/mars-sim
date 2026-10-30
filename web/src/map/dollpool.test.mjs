// The paper dolls' atlas cell pool (docs/body-sprites.md).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { DollPool, STALE_FRAMES } from './dollpool.ts';

const settle = () => new Promise((r) => setTimeout(r, 0));

function pool(size, fail = new Set()) {
  const painted = [];
  let redraws = 0;
  const p = new DollPool(100, size, async (key, cell) => {
    if (fail.has(key)) throw new Error('no');
    painted.push([key, cell]);
  }, () => { redraws++; });
  return { p, painted, redraws: () => redraws };
}

test('a doll draws its fallback until painted, then its own cell', async () => {
  const { p, painted, redraws } = pool(2);
  assert.equal(p.cell(7, 1), -1);
  assert.equal(p.cell(7, 1), -1); // asked twice, painted once
  await settle();
  assert.deepEqual(painted, [[7, 100]]);
  assert.equal(redraws(), 1);
  assert.equal(p.cell(7, 2), 100);
});

test('a full pool hands back the least recently drawn doll, once it has gone stale', async () => {
  const { p, painted } = pool(2);
  p.cell(1, 1); p.cell(2, 2);
  await settle();
  const later = 2 + STALE_FRAMES;
  p.cell(1, later); // 2 is now the oldest, and stale
  assert.equal(p.cell(3, later), -1);
  await settle();
  assert.deepEqual(painted.at(-1), [3, 101]);
  assert.equal(p.cell(2, later + 1), -1, 'evicted doll repaints, when a cell is free');
});

test('more dolls in view than cells: the ones with cells keep them', async () => {
  const { p, painted } = pool(2);
  for (let frame = 1; frame <= 3 * STALE_FRAMES; frame++) {
    for (const k of [1, 2, 3]) p.cell(k, frame);
    await settle();
  }
  assert.deepEqual(painted, [[1, 100], [2, 101]], 'no repainting');
  assert.equal(p.cell(3, 3 * STALE_FRAMES + 1), -1);
});

test('a doll that fails to paint stays on its fallback and frees its cell', async () => {
  const { p, painted } = pool(1, new Set([4]));
  assert.equal(p.cell(4, 1), -1);
  await settle();
  assert.equal(p.cell(4, 2), -1);
  p.cell(5, 2);
  await settle();
  assert.deepEqual(painted, [[5, 100]]);
});

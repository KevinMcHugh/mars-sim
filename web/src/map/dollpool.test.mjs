// The paper dolls' atlas cell pool (docs/body-sprites.md).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { DollPool } from './dollpool.ts';

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

test('a full pool hands back the least recently drawn doll, never one drawn this frame', async () => {
  const { p, painted } = pool(2);
  p.cell(1, 1); p.cell(2, 2);
  await settle();
  p.cell(1, 3); // 2 is now the oldest
  assert.equal(p.cell(3, 3), -1);
  await settle();
  assert.deepEqual(painted.at(-1), [3, 101]);
  assert.equal(p.cell(2, 4), -1, 'evicted doll repaints');
  await settle();
  assert.equal(painted.at(-1)[0], 2);
  // Everything drawn on frame 5: nothing to evict, the newcomer falls back.
  p.cell(2, 5); p.cell(3, 5);
  assert.equal(p.cell(9, 5), -1);
  await settle();
  assert.ok(!painted.some(([k]) => k === 9));
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

// Structural sharing for topic payloads: unchanged parts keep their identity,
// so a keyed {#each} skips their rows.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { reuse } from './reuse.ts';

test('an unchanged row keeps the old object; a changed one is the new', () => {
  const prev = { orders: [{ id: 1, price: 5 }, { id: 2, price: 7 }], treasury: 100 };
  const next = { orders: [{ id: 1, price: 5 }, { id: 2, price: 8 }], treasury: 100 };
  const out = reuse(prev, next);
  assert.notEqual(out, prev);
  assert.equal(out.orders[0], prev.orders[0]);
  assert.notEqual(out.orders[1], prev.orders[1]);
  assert.deepEqual(out, { orders: [{ id: 1, price: 5 }, { id: 2, price: 8 }], treasury: 100 });
});

test('a payload equal throughout is the old one', () => {
  const prev = { a: [1, 2, { b: 'x' }], c: null };
  assert.equal(reuse(prev, { a: [1, 2, { b: 'x' }], c: null }), prev);
});

test('length, keys and shape changes are not equal', () => {
  const prev = { rows: [{ id: 1 }], x: { y: 1 } };
  const grown = reuse(prev, { rows: [{ id: 1 }, { id: 2 }], x: { y: 1 } });
  assert.notEqual(grown.rows, prev.rows);
  assert.equal(grown.rows[0], prev.rows[0]);
  assert.equal(grown.x, prev.x);
  const prevA = { a: 1 };
  assert.notEqual(reuse(prevA, { a: 1, b: 2 }), prevA);
  assert.deepEqual(reuse({ a: 1, b: 2 }, { a: 1 }), { a: 1 });
  assert.deepEqual(reuse({ a: [1] }, { a: { 0: 1 } }), { a: { 0: 1 } });
  assert.equal(reuse(undefined, 3), 3);
});

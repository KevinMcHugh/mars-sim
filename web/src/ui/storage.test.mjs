// The Storage tab's pool and searches. node --test runs it against the
// TypeScript module itself: Node 22 strips the types on import.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { describe, items, owners, pool, search } from './storage.ts';

/** @type {import('./storage.ts').StorageRow[]} */
const rows = [
  {
    x: 1, y: 1, label: 'chest', used: 3, slots: 48, items: 70, capacity: 4800,
    contents: [{ item: 'iron ore', count: 50 }, { item: 'meal', count: 20 }],
    ledger: [
      { owner: 'Uma Xu', item: 'iron ore', count: 30 },
      { owner: 'for sale by Uma Xu', item: 'iron ore', count: 20 },
      { owner: 'the colony', item: 'meal', count: 20 },
    ],
  },
  {
    x: 4, y: 2, label: "Ada Lim's locker", used: 1, slots: 48, items: 15, capacity: 4800,
    contents: [{ item: 'raw rock', count: 10 }, { item: 'iron ore', count: 5 }],
    ledger: [{ owner: 'Ada Lim', item: 'raw rock', count: 10 }], // the ore is nobody's
  },
  { x: 9, y: 9, label: 'chest', used: 0, slots: 48, items: 0, capacity: 4800, contents: [], ledger: [] },
];

test('the pool sums every container, most first', () => {
  const p = pool(rows);
  assert.equal(p.containers, 3);
  assert.equal(p.used, 4);
  assert.equal(p.slots, 144);
  assert.equal(p.items, 85);
  assert.equal(p.capacity, 14400);
  assert.deepEqual(p.totals, [
    { item: 'iron ore', count: 55 }, { item: 'meal', count: 20 }, { item: 'raw rock', count: 10 },
  ]);
  assert.deepEqual(pool([]).totals, []);
});

test('suggestions are every owner and item, by name', () => {
  assert.deepEqual(owners(rows), ['Ada Lim', 'Uma Xu', 'for sale by Uma Xu', 'the colony']);
  assert.deepEqual(items(rows), ['iron ore', 'meal', 'raw rock']);
});

test('no terms is no search', () => {
  assert.equal(search(rows, { item: ' ', owner: '' }), null);
});

test('an item search reads contents, so it finds unowned stock too', () => {
  const f = search(rows, { item: 'ORE', owner: '' });
  assert.equal(f.total, 55);
  assert.deepEqual(f.hits.map((h) => [h.row.x, h.total]), [[1, 50], [4, 5]]);
  assert.deepEqual(f.hits[1].lines, [{ item: 'iron ore', count: 5 }]);
});

test('an owner search reads the ledger, listings included', () => {
  const f = search(rows, { item: '', owner: 'uma' });
  assert.equal(f.total, 50);
  assert.equal(f.hits.length, 1);
  assert.deepEqual(f.hits[0].lines.map((l) => l.owner), ['Uma Xu', 'for sale by Uma Xu']);
  assert.deepEqual(f.totals, [{ item: 'iron ore', count: 50 }]);
});

test('both terms narrow the ledger to that owner and item', () => {
  assert.equal(search(rows, { item: 'meal', owner: 'uma' }).hits.length, 0);
  const f = search(rows, { item: 'rock', owner: 'ada' });
  assert.deepEqual(f.hits.map((h) => h.row.label), ["Ada Lim's locker"]);
  assert.equal(f.total, 10);
});

test('a hit reads with each owner named once', () => {
  assert.equal(describe([
    { owner: 'Uma Xu', item: 'iron ore', count: 30 },
    { owner: 'Uma Xu', item: 'meal', count: 2 },
    { owner: 'the colony', item: 'meal', count: 20 },
  ]), 'Uma Xu: iron ore ×30, meal ×2 · the colony: meal ×20');
  assert.equal(describe([{ item: 'clay', count: 3 }, { item: 'raw rock', count: 1 }]), 'clay ×3, raw rock ×1');
});

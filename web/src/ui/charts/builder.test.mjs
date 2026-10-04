// The chart builder's arithmetic. node --test runs it against the TypeScript
// module itself: Node strips the types on import.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { bucketLabel, columns, effectiveBucket, hourLabel, parseCharts, seriesName } from './builder.ts';

/** @type {import('./builder.ts').Catalog} */
const cat = {
  metrics: [
    { key: 'money-moved', label: 'Money changed hands', group: 'Money', doc: '', kind: 'total', unit: 'dollars', per: 'colony' },
    { key: 'price', label: 'Price', group: 'Market', doc: '', kind: 'level', unit: 'dollars', per: 'item' },
  ],
  series: [
    { key: 'money-moved/', metric: 0 },
    { key: 'price/meal', metric: 1, subject: 'meal' },
  ],
};

// Hourly samples from day 1 07:00 (hour 7) to day 2 06:00 (hour 30): the
// running total gains its hour's number in dollars each hour.
const hours = Array.from({ length: 24 }, (_, i) => 7 + i);
let run = 0;
/** @type {import('./builder.ts').SeriesData} */
const moved = { key: 'money-moved/', found: true, every: 1, tick: hours.map((h) => h * 45), hour: hours, values: hours.map((h) => (run += h)) };
/** @type {import('./builder.ts').SeriesData} */
const price = { key: 'price/meal', found: true, every: 1, tick: [900, 945], hour: [20, 21], values: [5, 6] };

test('readings plot a running total as it is, and a level only where it has readings', () => {
  const c = columns({ id: 'a', title: '', series: ['money-moved/', 'price/meal'], bucket: 0 }, cat, { 'money-moved/': moved, 'price/meal': price });
  assert.deepEqual(c.x, hours);
  assert.deepEqual(c.ys[0], moved.values);
  assert.equal(c.ys[1][0], null);
  assert.deepEqual(c.ys[1].slice(13, 15), [5, 6]);
  assert.equal(c.ys[1][15], null);
});

test('half-day buckets sum a total exactly between midnight and noon', () => {
  const c = columns({ id: 'a', title: '', series: ['money-moved/'], bucket: 12 }, cat, { 'money-moved/': moved });
  // The sample at hour h covers hour h-1: hours 6..11 fall in D1 H1, 12..23
  // in D1 H2, 24..29 in D2 H1.
  assert.deepEqual(c.x, [0, 12, 24]);
  const sum = (lo, hi) => { let s = 0; for (let h = lo; h <= hi; h++) s += h + 1; return s; };
  assert.deepEqual(c.ys[0], [sum(6, 11), sum(12, 23), sum(24, 29)]);
  assert.equal(c.ys[0].reduce((a, b) => a + b, 0), moved.values.at(-1), 'buckets lose no money');
});

test('a level in buckets is its reading at the bucket end', () => {
  const c = columns({ id: 'a', title: '', series: ['price/meal'], bucket: 24 }, cat, { 'price/meal': price });
  assert.deepEqual(c.x, [0]);
  assert.deepEqual(c.ys[0], [6]);
});

test('buckets widen to a whole number of sampling intervals', () => {
  assert.equal(effectiveBucket(12, 1), 12);
  assert.equal(effectiveBucket(12, 4), 12);
  assert.equal(effectiveBucket(12, 8), 24);
  assert.equal(effectiveBucket(168, 16), 336);
  assert.equal(effectiveBucket(0, 8), 0);
});

test('labels name days the way the top bar does', () => {
  assert.equal(hourLabel(7), 'day 1 07:00');
  assert.equal(hourLabel(24 * 42 + 18), 'day 43 18:00');
  assert.equal(bucketLabel(24 * 42, 12), 'D43 H1');
  assert.equal(bucketLabel(24 * 42 + 12, 12), 'D43 H2');
  assert.equal(bucketLabel(24 * 42, 24), 'D43');
  assert.equal(bucketLabel(24 * 42 + 6, 6), 'D43 06–12');
  assert.equal(bucketLabel(0, 168), 'D1–D7');
});

test('series names carry their subject', () => {
  assert.equal(seriesName(cat, 'price/meal'), 'Price · meal');
  assert.equal(seriesName(cat, 'money-moved/'), 'Money changed hands');
  assert.equal(seriesName(cat, 'nope/x'), 'nope/x');
});

test('saved charts are checked, not trusted', () => {
  assert.equal(parseCharts('x'), null);
  const got = parseCharts([
    { id: 'a', title: 'ok', series: ['price/meal', 7], bucket: 12 },
    { id: 'b', title: 'odd bucket', series: [], bucket: 5 },
    { title: 'no id', series: [] },
    null,
  ]);
  assert.deepEqual(got, [
    { id: 'a', title: 'ok', series: ['price/meal'], bucket: 12 },
    { id: 'b', title: 'odd bucket', series: [], bucket: 0 },
  ]);
});

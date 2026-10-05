// The Activity chart's arithmetic. node --test runs it against the TypeScript
// module itself: Node 22 strips the types on import.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { scale, slice, trackedSeries } from './population.ts';

// Two samples, at ticks 100 and 300, of two colonists doing two things.
/** @type {import('./population.ts').Population} */
const pop = {
  tick: [100, 300],
  colonists: [2, 2], meals: [0, 0], colonySize: [0, 0], fixtures: [0, 0],
  activities: ['sleeping', 'eating'],
  activity: [[150, 100], [50, 300]],
  walking: [[0, 0], [20, 60]],
  fixtureKinds: ['scumhouse'],
  fixtureCounts: [[1, 2]],
  skills: [{ skill: 'mining', labels: ['', 'digger', 'digger', 'master miner'], ranks: [[1, 0], [1, 0], [0, 1], [0, 1]] }],
};

test('a slice splits doing from walking there, over the ticks it covers', () => {
  const s = slice(pop, 1, 2);
  assert.deepEqual(s.own, [100, 240]);
  assert.deepEqual(s.walk, [0, 60]);
  assert.equal(s.ticks, 200); // tick 100 to 300
  assert.equal(s.total, 400);
  const first = slice(pop, 0, 1);
  assert.equal(first.ticks, 100); // from the start of the game
});

test('shares sum to one; counts are average colonists', () => {
  const s = slice(pop, 0, 2);
  const shares = [0, 1].map((a) => scale(s.own[a] + s.walk[a], s, false));
  assert.equal(shares[0] + shares[1], 1);
  assert.equal(scale(s.own[0] + s.walk[0], s, true), 250 / 300);
  assert.equal(scale(5, { own: [], walk: [], ticks: 0, total: 0 }, true), 0);
});

test('columns cover every sample once, in order', async () => {
  const { columns } = await import('./population.ts');
  for (const [n, cols] of [[5, 96], [512, 96], [97, 96], [1, 3]]) {
    const runs = columns(n, cols);
    assert.equal(runs.length, Math.min(n, cols));
    assert.equal(runs[0][0], 0);
    assert.equal(runs.at(-1)[1], n);
    for (let i = 1; i < runs.length; i++) assert.equal(runs[i][0], runs[i - 1][1]);
  }
});

test('tracked series: all fixtures, each kind, each skill title or better', () => {
  const t = trackedSeries(pop);
  assert.deepEqual(t.map((s) => s.title),
    ['Fixtures', 'Scumhouse', 'Colonists: digger or better', 'Colonists: master miner']);
  assert.deepEqual(t[1].values, [1, 2]);
  // Ranks 1 and 2 are both "digger": one series, from rank 1 up.
  assert.deepEqual(t[2].values, [1, 2]);
  assert.deepEqual(t[3].values, [0, 1]);
  assert.equal(new Set(t.map((s) => s.key)).size, t.length);
});

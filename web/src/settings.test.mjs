// The New game form's ship count. node --test runs it against the TypeScript
// module itself: Node 22 strips the types on import.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describeShips, SHIP_CAPACITY, shipLoads } from './settings.ts';

test('founders split into as few ships as capacity allows, evenly', () => {
  assert.deepEqual(shipLoads(6), [6]);
  assert.deepEqual(shipLoads(20), [20]);
  assert.deepEqual(shipLoads(21), [11, 10]);
  assert.deepEqual(shipLoads(30), [15, 15]);
  assert.deepEqual(shipLoads(41), [14, 14, 13]);
  assert.deepEqual(shipLoads(5, 2), [2, 2, 1]);
});

test('no founders, no ships', () => {
  assert.deepEqual(shipLoads(0), []);
  assert.deepEqual(shipLoads(-3), []);
  assert.deepEqual(shipLoads(NaN), []);
});

test('the count reads as the player will land it', () => {
  assert.equal(describeShips(shipLoads(6)), '1 ship');
  assert.equal(describeShips(shipLoads(30)), '2 ships of 15');
  assert.equal(describeShips(shipLoads(41)), '3 ships of 14 or 13');
  assert.equal(describeShips([]), 'no ships');
});

test('SHIP_CAPACITY is the engine default', () => {
  const src = readFileSync(new URL('../../internal/sim/config.go', import.meta.url), 'utf8');
  const m = src.match(/^\s*ShipCapacity:\s*(\d+),/m);
  assert.ok(m, 'ShipCapacity not found in DefaultConfig');
  assert.equal(SHIP_CAPACITY, Number(m[1]));
});

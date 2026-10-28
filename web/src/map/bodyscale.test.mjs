// The map's body scale (docs/body-sprites.md).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { MAX_SCALE, MIN_SCALE, REFERENCE_CM, bodyScale } from './bodyscale.ts';

test('an average colonist, and a kind with no height, draw at one tile', () => {
  assert.equal(bodyScale(REFERENCE_CM), 1);
  assert.equal(bodyScale(0), 1);
});

test('the mutation range reads as visibly smaller and bigger', () => {
  assert.ok(Math.abs(bodyScale(305) - 1.5) < 0.01, `ten feet: ${bodyScale(305)}`);
  assert.ok(Math.abs(bodyScale(61) - 0.45) < 0.01, `two feet: ${bodyScale(61)}`);
});

test('the extremes clamp, and the curve rises in between', () => {
  assert.equal(bodyScale(5), MIN_SCALE);
  assert.equal(bodyScale(5000), MAX_SCALE);
  for (let cm = 20; cm < 700; cm += 10) assert.ok(bodyScale(cm + 10) >= bodyScale(cm));
});

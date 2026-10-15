// The shared number and time formats. node --test runs it against the
// TypeScript module itself: Node 22 strips the types on import.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clock, dayClock, money, span } from './format.ts';

test('money groups thousands and puts the sign before the dollar', () => {
  assert.equal(money(1234), '$1,234');
  assert.equal(money(-5), '-$5');
});

test('clock and dayClock read like the top bar', () => {
  assert.equal(clock(0), '00:00');
  assert.equal(clock(6 * 60 + 5), '06:05');
  assert.equal(dayClock(12, 12 * 60 + 43), 'Day 12 12:43');
});

test('span keeps the two largest units', () => {
  assert.equal(span(0), 'under a minute');
  assert.equal(span(40), '40 min');
  assert.equal(span(60), '1 hr');
  assert.equal(span(5 * 60 + 12), '5 hrs, 12 min');
  assert.equal(span(1440), '1 day');
  assert.equal(span(3 * 1440 + 2 * 60 + 59), '3 days, 2 hrs');
});

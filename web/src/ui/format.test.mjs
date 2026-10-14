// The shared number and time formats. node --test runs it against the
// TypeScript module itself: Node 22 strips the types on import.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clock, dayClock, money, span, when, whenRange } from './format.ts';

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

// The page must date a tick as Go does: the 740-tick cases are sim's
// TestDayOfCountsLandingAsDayOne and TestMinuteOfDayStartsAtLandingHour.
test('when dates a tick as sim.DayOf and sim.MinuteOfDay do', () => {
  const go = { ticksPerDay: 740, landingHour: 6 };
  assert.equal(when(370, go), 'Day 1 18:00');
  assert.equal(when(554, go), 'Day 1 23:58');
  assert.equal(when(555, go), 'Day 2 00:00');
  assert.equal(when(1295, go), 'Day 3 00:00');
  const cal = { ticksPerDay: 1080, landingHour: 6 };
  assert.equal(when(0, cal), 'Day 1 06:00');
  assert.equal(when(45, cal), 'Day 1 07:00');
  assert.equal(when(809, cal), 'Day 1 23:58');
  assert.equal(when(810, cal), 'Day 2 00:00');
  assert.equal(when(810 + 1080 * 10 + 270, cal), 'Day 12 06:00');
  assert.equal(when(17391, null), 'tick 17,391');
});

test('whenRange names the second day only when it changes', () => {
  const cal = { ticksPerDay: 1080, landingHour: 6 };
  assert.equal(whenRange(0, 0, cal), 'Day 1 06:00');
  assert.equal(whenRange(0, 90, cal), 'Day 1 06:00–08:00');
  assert.equal(whenRange(800, 830, cal), 'Day 1 23:46–Day 2 00:26');
});

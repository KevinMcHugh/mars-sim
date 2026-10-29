// The Power tab's words. node --test runs it against the TypeScript module
// itself: Node 22 strips the types on import.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { headline, poweredShare, stalled, statusText } from './power.ts';

/** @returns {import('./power.ts').Power} */
function power(over = {}) {
  return {
    on: true, policy: 'monopoly', policies: [], cooldownHours: 0, reactors: [], batteries: 0, price: 3,
    priceKind: 'tariff', building: [], charged: 0, cookedPowered: 0, cookedCold: 0, reserve: 60,
    uranium: 0, pellets: 0, uraniumReserve: 2, ...over,
  };
}
const reactor = (status, over = {}) => ({
  x: 1, y: 2, l: 0, owner: 'the colony', price: 100, forSale: false, batteries: 0, ore: 0, pellets: 0, ice: 0, status, ...over,
});

test('the headline counts working reactors and ones going up', () => {
  assert.equal(headline(power({ on: false })), 'Power is off in this game.');
  assert.equal(headline(power()), 'No reactor yet');
  assert.equal(headline(power({ building: [{ x: 1, y: 1, l: 0, owner: 'the colony' }] })), 'No reactor yet · 1 going up');
  assert.equal(headline(power({ reactors: [reactor('working'), reactor('full')] })), '1 of 2 reactors working');
  assert.equal(headline(power({ reactors: [reactor('idle')] })), '0 of 1 reactor working');
});

test('a reactor short of fuel is stalled, and says what it lacks', () => {
  assert.ok(stalled(reactor('no-uranium')));
  assert.ok(stalled(reactor('no-ice')));
  assert.ok(!stalled(reactor('full')));
  assert.equal(statusText(reactor('working', { operator: 'Ada' })), 'Ada is working it');
  assert.equal(statusText(reactor('no-ice')), 'no ice to charge its pellets');
});

test('the powered share is a whole percent, or nothing before any cooking', () => {
  assert.equal(poweredShare(power()), null);
  assert.equal(poweredShare(power({ cookedPowered: 1, cookedCold: 2 })), 33);
});

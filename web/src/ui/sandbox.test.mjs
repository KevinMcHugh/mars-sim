// The Sandbox tab's species search. node --test runs it against the
// TypeScript module itself: Node 22 strips the types on import.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { matchSpecies, TEMPERAMENTS } from './sandbox.ts';

const sp = (label, singular, plural, scientificName, temperament) => ({ label, singular, plural, scientificName, temperament });
const roster = [
  sp('🦂 Stingtail · hostile', 'stingtail', 'stingtails', 'Scorpiurus ares', 'hostile'),
  sp('🐸 Green · friendly', 'green', 'greens', 'Ranoides viridis', 'friendly'),
  sp('🦑 Squid · cautious', 'squid', 'squid', 'Teuthis ares', 'cautious'),
];

test('an empty query matches every species, in roster order', () => {
  assert.deepEqual(matchSpecies(roster, ''), [0, 1, 2]);
  assert.deepEqual(matchSpecies(roster, '   '), [0, 1, 2]);
});

test('a query matches any field, ignoring case', () => {
  assert.deepEqual(matchSpecies(roster, 'SQU'), [2]);
  assert.deepEqual(matchSpecies(roster, 'friendly'), [1]);
  assert.deepEqual(matchSpecies(roster, 'ares'), [0, 2]); // the binomial
});

test('every word must match', () => {
  assert.deepEqual(matchSpecies(roster, 'ares cautious'), [2]);
  assert.deepEqual(matchSpecies(roster, 'hostile green'), []);
});

test('temperaments narrow the matches: any of those given', () => {
  assert.deepEqual(matchSpecies(roster, '', ['hostile']), [0]);
  assert.deepEqual(matchSpecies(roster, '', ['friendly', 'cautious']), [1, 2]);
  assert.deepEqual(matchSpecies(roster, 'ares', ['cautious']), [2]);
  assert.deepEqual(matchSpecies(roster, '', []), [0, 1, 2]);
  assert.deepEqual([...TEMPERAMENTS].sort(), ['cautious', 'friendly', 'hostile']);
});

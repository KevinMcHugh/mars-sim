// Paper doll keys and art (docs/body-sprites.md).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { composeDoll, dollDigits } from './dolls.ts';

const layout = {
  base: 0, count: 6 * 5 * 6 * 16, fallback: 0, tones: 5,
  figures: ['man', 'woman', 'person', 'old man', 'old woman', 'older person'],
  hairs: ['black', 'brown', 'blonde', 'red', 'white', 'bald'],
  parts: ['third arm', 'extra eye', 'tail', 'vestigial twin'],
};
// The same packing as glyphs.dollKey in Go.
const key = (f, t, h, p) => ((f * 5 + t) * 6 + h) * 16 + p;

test('a key unpacks to the digits Go packed', () => {
  assert.deepEqual(dollDigits(key(4, 2, 3, 0b0101), layout),
    { figure: 'old woman', tone: 2, hair: 'red', parts: ['third arm', 'tail'] });
  assert.deepEqual(dollDigits(key(0, 0, 0, 0), layout), { figure: 'man', tone: 0, hair: 'black', parts: [] });
  assert.equal(dollDigits(layout.count - 1, layout).figure, 'older person');
});

test('every part shows, and a doll is one plain SVG', () => {
  const plain = composeDoll(key(0, 1, 0, 0), layout);
  const all = composeDoll(key(0, 1, 0, 15), layout);
  assert.ok(all.length > plain.length + 400, 'parts add drawing');
  for (const svg of [plain, all, composeDoll(key(5, 4, 5, 3), layout)]) {
    assert.match(svg, /^<svg xmlns="http:\/\/www.w3.org\/2000\/svg" viewBox="0 0 128 128" width="128" height="128">/);
    assert.ok(!/script|href|undefined|NaN/.test(svg), svg);
  }
});

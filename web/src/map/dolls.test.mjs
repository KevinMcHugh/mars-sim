// Paper doll keys and art (docs/body-sprites.md).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { composeDoll, dollDigits, dollFallback, expressionOf, textGlyph } from './dolls.ts';

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
  for (const svg of [plain, all, composeDoll(key(5, 4, 5, 3), layout), composeDoll(key(1, 0, 2, 15), layout, true)]) {
    assert.match(svg, /^<svg xmlns="http:\/\/www.w3.org\/2000\/svg" viewBox="[0-9 ]+" width="128" height="128">/);
    assert.ok(!/script|href|undefined|NaN/.test(svg), svg);
  }
});

test('a doll stands in as the mutant if it grew parts, else as its look', () => {
  const l = { ...layout, base: 1000, fallback: 7, fallbacks: Array.from({ length: layout.count / 16 }, (_, i) => 500 + i) };
  assert.equal(dollFallback(key(1, 2, 3, 0), l), 500 + key(1, 2, 3, 0) / 16);
  assert.equal(dollFallback(key(1, 2, 3, 4), l), 7);
  assert.equal(textGlyph(1000 + key(1, 2, 3, 0), l), 500 + key(1, 2, 3, 0) / 16);
  assert.equal(textGlyph(42, l), 42);
  assert.equal(textGlyph(1000 + l.count, l), 1000 + l.count, 'past the dolls is not a doll');
});

test('a body keeps one expression as it mutates, and the colony gets every one', () => {
  const seen = new Set();
  for (let body = 0; body < layout.count / 16; body++) seen.add(expressionOf(body));
  assert.equal(seen.size, 6);
  // Growing parts does not change the face: same body, same mouth.
  const mouth = (svg) => svg.match(/<path d="M5[78][^"]*" fill="(?:none|#f4ece4)"/)?.[0];
  assert.equal(mouth(composeDoll(key(1, 2, 3, 0), layout)), mouth(composeDoll(key(1, 2, 3, 15), layout)));
});

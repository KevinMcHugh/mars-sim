// Checks decode.js against the Go encoder's golden frames: each
// internal/wire/testdata/NAME.bin must decode to NAME.json. Run with
//   node --test web/wire/decode.test.mjs
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { decodeFrame, PAGE_TILES, TILE_BYTES } from './decode.js';

const testdata = fileURLToPath(new URL('../../internal/wire/testdata/', import.meta.url));

// FNV-1a over a page's tile bytes, as tileSum in internal/wire.
function tileSum(bytes) {
  let h = 2166136261;
  for (const c of bytes) h = Math.imul(h ^ c, 16777619) >>> 0;
  return h;
}

// Reshape a decoded frame into the JSON the Go test wrote.
function asGolden(f) {
  const e = f.entities, p = f.pages, r = f.refuse;
  const pageBytes = PAGE_TILES * TILE_BYTES;
  return {
    version: f.version, paused: f.paused, fogOfWar: f.fogOfWar,
    tilesReset: f.tilesReset, refuseFrame: r !== null,
    tick: f.tick, tileFrame: f.tileFrame, tps: f.tps, pagesOwed: f.pagesOwed,
    stats: Array.from(f.stats),
    entities: Array.from({ length: e.count }, (_, i) => ({
      id: e.id[i], x: e.x[i], y: e.y[i], glyph: e.glyph[i], kind: e.kind[i], state: e.state[i], focus: e.focus[i],
    })),
    pages: Array.from({ length: p.count }, (_, i) => ({
      px: p.px[i], py: p.py[i], tileSum: tileSum(p.tiles.subarray(i * pageBytes, (i + 1) * pageBytes)),
    })),
    refuse: r === null ? [] : Array.from({ length: r.count }, (_, i) => ({
      x: r.x[i], y: r.y[i], corpses: r.corpses[i], gore: r.gore[i],
    })),
  };
}

for (const name of ['first', 'changed', 'capped']) {
  test(`decodes the ${name} golden frame`, () => {
    const bin = readFileSync(testdata + name + '.bin');
    // A fresh, exactly sized ArrayBuffer, as a transferred frame would be.
    const buffer = bin.buffer.slice(bin.byteOffset, bin.byteOffset + bin.byteLength);
    const want = JSON.parse(readFileSync(testdata + name + '.json', 'utf8'));
    assert.deepEqual(asGolden(decodeFrame(buffer)), want);
  });
}

test('rejects a frame of another version', () => {
  const bin = readFileSync(testdata + 'first.bin');
  const buffer = bin.buffer.slice(bin.byteOffset, bin.byteOffset + bin.byteLength);
  new DataView(buffer).setUint16(4, 99, true);
  assert.throws(() => decodeFrame(buffer), /version 99/);
});

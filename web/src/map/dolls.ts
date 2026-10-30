// Paper dolls (docs/body-sprites.md): a colonist drawn as a figure composed
// from layered SVG parts, in their own skin and hair, with whatever uranium
// grew on them. The engine sends only a key (Hello.glyphs.dolls); the art
// lives here, like the terrain's tile art (tileart.ts).
//
// Every part is drawn against one skeleton, the same for all six figures, so
// a part drawn once fits every body:
//
//   head centre   (64, 40), radius 22
//   shoulders     (46, 68) and (82, 68)
//   hips          (54, 100) and (74, 100)
//   feet          y = 120
//
// The viewBox is the atlas cell's 128 square, figure standing on its bottom
// edge like the emoji it replaces, so the map's feet-anchored scaling
// (bodyscale.ts) puts the doll's feet on the floor.

import type { HelloDolls } from '../../wire/decode.js';

/** A key unpacked into the digits Hello.glyphs.dolls names. */
export interface DollDigits {
  figure: string;
  tone: number;
  hair: string;
  parts: string[];
}

/** Unpack key with layout's radixes: parts least significant, figure most. */
export function dollDigits(key: number, layout: HelloDolls): DollDigits {
  const bits = layout.parts.length;
  const partBits = key % (1 << bits);
  let rest = Math.floor(key / (1 << bits));
  const hair = layout.hairs[rest % layout.hairs.length] ?? 'black';
  rest = Math.floor(rest / layout.hairs.length);
  const tone = rest % layout.tones;
  const figure = layout.figures[Math.floor(rest / layout.tones)] ?? 'person';
  return { figure, tone, hair, parts: layout.parts.filter((_, i) => partBits & (1 << i)) };
}

/**
 * The symbol to draw doll key as until its art is painted, and wherever a
 * glyph is shown as text: the mutant for a body with grown parts, else the
 * colonist's look, as the map drew them before dolls.
 */
export function dollFallback(key: number, layout: HelloDolls): number {
  const bits = layout.parts.length;
  if (key % (1 << bits) !== 0) return layout.fallback;
  return layout.fallbacks?.[Math.floor(key / (1 << bits))] ?? layout.fallback;
}

/** The symbol index to show glyph g as text: a doll's fallback, or g itself. */
export function textGlyph(g: number, layout: HelloDolls | undefined): number {
  return layout && g >= layout.base && g < layout.base + layout.count ? dollFallback(g - layout.base, layout) : g;
}

// Fills and outlines. Skin follows the five-point emoji scale, lightest first.
const SKIN = ['#f6d3b8', '#e2ae87', '#c48a62', '#8e5b3c', '#5e3b27'];
const SKIN_SHADE = ['#d9ab8c', '#c08a63', '#9e6a46', '#6e432a', '#432818'];
const HAIR: Record<string, string> = {
  black: '#24201f', brown: '#5b3a26', blonde: '#e2c069', red: '#b4462a', white: '#e9e7e2',
};
const LINE = '#2b211c';
// Colony jumpsuits, a colour per figure, so the gender reads at a glance the
// way the emoji's figures did.
const SUIT: Record<string, string> = { man: '#3d6fa8', woman: '#8c4ea6', person: '#3f8c6a' };
const BOOTS = '#3a3330';

const stroke = `stroke="${LINE}" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round"`;

function gender(figure: string): 'man' | 'woman' | 'person' {
  if (figure.endsWith('woman')) return 'woman';
  if (figure.endsWith('man')) return 'man';
  return 'person';
}

/** A limb: a thick rounded stroke from a to b, ending in a hand or foot. */
function arm(x1: number, y1: number, x2: number, y2: number, sleeve: string, skin: string): string {
  return `<path d="M${x1} ${y1} L${x2} ${y2}" stroke="${LINE}" stroke-width="13" stroke-linecap="round"/>` +
    `<path d="M${x1} ${y1} L${x2} ${y2}" stroke="${sleeve}" stroke-width="8" stroke-linecap="round"/>` +
    `<circle cx="${x2}" cy="${y2}" r="5.5" fill="${skin}" ${stroke}/>`;
}

// ---- Layers, back to front ------------------------------------------------

function tail(skin: string, shade: string): string {
  return `<path d="M72 100 C 96 112, 116 108, 114 82 C 113 74, 106 72, 104 78" fill="none" stroke="${LINE}" stroke-width="12" stroke-linecap="round"/>` +
    `<path d="M72 100 C 96 112, 116 108, 114 82 C 113 74, 106 72, 104 78" fill="none" stroke="${skin}" stroke-width="7" stroke-linecap="round"/>` +
    `<path d="M88 106 C 98 108, 108 104, 111 92" fill="none" stroke="${shade}" stroke-width="2" stroke-linecap="round"/>`;
}

function twin(skin: string, hair: string | null): string {
  // A small second head budding from behind the left shoulder, eyes shut.
  const tuft = hair ? `<path d="M22 48 Q 30 36, 40 44 Q 32 40, 22 48 Z" fill="${hair}" ${stroke}/>` : '';
  return `<circle cx="31" cy="56" r="12" fill="${skin}" ${stroke}/>${tuft}` +
    `<path d="M25 56 q 2.5 2, 5 0 M33 56 q 2.5 2, 5 0" fill="none" stroke="${LINE}" stroke-width="1.8" stroke-linecap="round"/>` +
    `<path d="M29 62 q 2 1, 4 0" fill="none" stroke="${LINE}" stroke-width="1.5" stroke-linecap="round"/>`;
}

function backHair(g: string, hair: string | null): string {
  if (!hair) return '';
  if (g === 'woman') return `<path d="M40 38 C 38 62, 40 74, 48 76 L 80 76 C 88 74, 90 62, 88 38 Z" fill="${hair}" ${stroke}/>`;
  if (g === 'person') return `<path d="M41 38 C 40 54, 42 62, 48 62 L 80 62 C 86 62, 88 54, 87 38 Z" fill="${hair}" ${stroke}/>`;
  return '';
}

function body(suit: string, skin: string): string {
  return arm(46, 70, 38, 94, suit, skin) +
    // legs, then boots
    `<path d="M56 98 L56 116 M72 98 L72 116" stroke="${LINE}" stroke-width="13" stroke-linecap="round"/>` +
    `<path d="M56 98 L56 116 M72 98 L72 116" stroke="${suit}" stroke-width="8" stroke-linecap="round"/>` +
    `<path d="M49 120 Q 49 113, 56 113 Q 62 113, 62 120 Z M66 120 Q 66 113, 72 113 Q 79 113, 79 120 Z" fill="${BOOTS}" ${stroke}/>` +
    // torso
    `<path d="M46 66 Q 64 58, 82 66 L 80 102 Q 64 106, 48 102 Z" fill="${suit}" ${stroke}/>` +
    `<path d="M64 66 L64 100" stroke="${LINE}" stroke-width="1.5" opacity="0.5"/>` +
    arm(82, 70, 90, 94, suit, skin);
}

function thirdArm(suit: string, skin: string): string {
  // Out of the right side, waving.
  return arm(80, 78, 104, 62, suit, skin);
}

function head(skin: string, shade: string, senior: boolean): string {
  const wrinkles = senior
    ? `<path d="M50 52 q 3 2, 6 1 M72 53 q 3 1, 6 -1" fill="none" stroke="${shade}" stroke-width="1.6" stroke-linecap="round"/>`
    : '';
  return `<rect x="58" y="56" width="12" height="9" fill="${skin}" ${stroke}/>` +
    `<circle cx="64" cy="40" r="22" fill="${skin}" ${stroke}/>` +
    `<circle cx="42" cy="43" r="4" fill="${skin}" ${stroke}/><circle cx="86" cy="43" r="4" fill="${skin}" ${stroke}/>` +
    `<ellipse cx="56" cy="42" rx="2.6" ry="3.4" fill="${LINE}"/><ellipse cx="72" cy="42" rx="2.6" ry="3.4" fill="${LINE}"/>` +
    `<path d="M57 51 Q 64 56, 71 51" fill="none" stroke="${LINE}" stroke-width="2" stroke-linecap="round"/>` +
    wrinkles;
}

function frontHair(g: string, hair: string | null): string {
  if (!hair) return '';
  if (g === 'man') return `<path d="M42 38 C 42 18, 86 14, 86 36 C 80 28, 72 26, 64 30 C 56 25, 48 28, 42 38 Z" fill="${hair}" ${stroke}/>`;
  if (g === 'woman') return `<path d="M41 40 C 40 14, 88 14, 87 40 C 80 30, 70 24, 60 28 C 52 30, 46 34, 41 40 Z" fill="${hair}" ${stroke}/>`;
  return `<path d="M42 38 C 42 16, 86 16, 86 38 C 78 30, 68 30, 62 26 C 56 30, 48 32, 42 38 Z" fill="${hair}" ${stroke}/>`;
}

function glasses(): string {
  return `<g fill="none" stroke="${LINE}" stroke-width="1.8"><circle cx="56" cy="42" r="6"/><circle cx="72" cy="42" r="6"/><path d="M62 42 L66 42"/></g>`;
}

function extraEye(): string {
  // On the forehead, over any fringe: the thing everyone notices first.
  return `<ellipse cx="64" cy="29" rx="6" ry="4.5" fill="#fbfaf5" ${stroke}/>` +
    `<circle cx="64" cy="29" r="2.4" fill="#5a8f3a"/><circle cx="64" cy="29" r="1.1" fill="${LINE}"/>`;
}

/**
 * The SVG for doll key: a self-contained image, sized to the atlas cell. A
 * portrait is the same drawing cropped to head and shoulders, for a line of
 * text, where a whole figure is too small to read.
 */
export function composeDoll(key: number, layout: HelloDolls, portrait = false): string {
  const d = dollDigits(key, layout);
  const g = gender(d.figure);
  const senior = d.figure.startsWith('old') || d.figure.startsWith('older');
  const skin = SKIN[d.tone] ?? SKIN[2];
  const shade = SKIN_SHADE[d.tone] ?? SKIN_SHADE[2];
  const hair = d.hair === 'bald' ? null : HAIR[d.hair] ?? HAIR.black;
  const suit = SUIT[g];
  const has = (p: string) => d.parts.includes(p);
  const layers = [
    has('tail') ? tail(skin, shade) : '',
    has('vestigial twin') ? twin(skin, hair) : '',
    backHair(g, hair),
    body(suit, skin),
    has('third arm') ? thirdArm(suit, skin) : '',
    head(skin, shade, senior),
    frontHair(g, hair),
    senior ? glasses() : '',
    has('extra eye') ? extraEye() : '',
  ];
  const view = portrait ? '24 12 80 80' : '0 0 128 128';
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${view}" width="128" height="128">${layers.join('')}</svg>`;
}

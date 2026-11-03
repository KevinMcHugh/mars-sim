// Paper dolls (docs/body-sprites.md): a colonist drawn as a figure composed
// from layered SVG parts, in their own skin and hair, with whatever uranium
// grew on them. The engine sends only a key (Hello.glyphs.dolls); the art
// lives here, like the terrain's tile art (tileart.ts).
//
// Every part is drawn against one skeleton, the same for all six figures, so
// a part drawn once fits every body:
//
//   head centre   (64, 30), 26 wide and 31 tall, with a jaw
//   neck          y = 44 to 52
//   shoulders     (50, 54) and (78, 54)
//   hips          (57, 86) and (71, 86)
//   feet          y = 120
//
// Proportions are a person's, not a mascot's: the head is under a third of
// the figure, ears sit flat, and the face has brows and a nose. An earlier
// round head on a stubby body, with ears sticking out, read as a cartoon
// mascot rather than a colonist.
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

const stroke = `stroke="${LINE}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"`;

function gender(figure: string): 'man' | 'woman' | 'person' {
  if (figure.endsWith('woman')) return 'woman';
  if (figure.endsWith('man')) return 'man';
  return 'person';
}

/** A limb: a rounded stroke from a to b, ending in a hand. */
function arm(x1: number, y1: number, x2: number, y2: number, sleeve: string, skin: string): string {
  return `<path d="M${x1} ${y1} L${x2} ${y2}" stroke="${LINE}" stroke-width="11" stroke-linecap="round"/>` +
    `<path d="M${x1} ${y1} L${x2} ${y2}" stroke="${sleeve}" stroke-width="7" stroke-linecap="round"/>` +
    `<circle cx="${x2}" cy="${y2 + 1}" r="4.2" fill="${skin}" ${stroke}/>`;
}

// ---- Layers, back to front ------------------------------------------------

function tail(skin: string, shade: string): string {
  const d = 'M70 84 C 92 98, 112 94, 110 70 C 109 63, 103 61, 101 67';
  return `<path d="${d}" fill="none" stroke="${LINE}" stroke-width="11" stroke-linecap="round"/>` +
    `<path d="${d}" fill="none" stroke="${skin}" stroke-width="7" stroke-linecap="round"/>` +
    `<path d="M86 93 C 96 95, 105 90, 107 79" fill="none" stroke="${shade}" stroke-width="1.6" stroke-linecap="round"/>`;
}

function twin(skin: string, hair: string | null): string {
  // A small second head budding from behind the left shoulder, eyes shut.
  const tuft = hair ? `<path d="M33 46 C 33 37, 47 37, 47 45 C 43 41, 38 41, 33 46 Z" fill="${hair}" ${stroke}/>` : '';
  return `<ellipse cx="40" cy="49" rx="7.5" ry="8.5" fill="${skin}" ${stroke}/>${tuft}` +
    `<path d="M36 49 q 1.6 1.3, 3.2 0 M41 49 q 1.6 1.3, 3.2 0" fill="none" stroke="${LINE}" stroke-width="1.3" stroke-linecap="round"/>` +
    `<path d="M38.5 53.5 q 1.5 0.8, 3 0" fill="none" stroke="${LINE}" stroke-width="1.1" stroke-linecap="round"/>`;
}

function backHair(g: string, hair: string | null): string {
  if (!hair) return '';
  if (g === 'woman') return `<path d="M50 26 C 46 42, 47 56, 54 60 L 74 60 C 81 56, 82 42, 78 26 Z" fill="${hair}" ${stroke}/>`;
  if (g === 'person') return `<path d="M50 26 C 48 37, 49 45, 53 47 L 75 47 C 79 45, 80 37, 78 26 Z" fill="${hair}" ${stroke}/>`;
  return '';
}

function body(suit: string, skin: string): string {
  return `<rect x="59" y="42" width="10" height="11" fill="${skin}" ${stroke}/>` +
    arm(51, 58, 45, 83, suit, skin) +
    // legs, then boots
    `<path d="M58 86 L57.5 112 M70 86 L70.5 112" stroke="${LINE}" stroke-width="11" stroke-linecap="round"/>` +
    `<path d="M58 86 L57.5 112 M70 86 L70.5 112" stroke="${suit}" stroke-width="7" stroke-linecap="round"/>` +
    `<path d="M51 120 Q 51 113, 57.5 113 Q 63 113, 63 120 Z M65 120 Q 65 113, 70.5 113 Q 77 113, 77 120 Z" fill="${BOOTS}" ${stroke}/>` +
    // torso, with a collar and a seam
    `<path d="M49 55 Q 64 49, 79 55 L 77 87 Q 64 90, 51 87 Z" fill="${suit}" ${stroke}/>` +
    `<path d="M59 52 L64 58 L69 52" fill="none" stroke="${LINE}" stroke-width="1.4" stroke-linejoin="round"/>` +
    `<path d="M64 58 L64 86" stroke="${LINE}" stroke-width="1.2" opacity="0.45"/>` +
    arm(77, 58, 83, 83, suit, skin);
}

function thirdArm(suit: string, skin: string): string {
  // Out of the right side, waving.
  return arm(76, 66, 98, 48, suit, skin);
}

function head(skin: string, shade: string, senior: boolean): string {
  const wrinkles = senior
    ? `<path d="M55.5 34 q 1.5 2, 1 4 M72.5 34 q -1.5 2, -1 4 M60 21 q 4 -1, 8 0" fill="none" stroke="${shade}" stroke-width="1.2" stroke-linecap="round"/>`
    : '';
  return (
    // ears first, so the head covers all but their outer half
    `<ellipse cx="51.5" cy="31" rx="2.6" ry="4.2" fill="${skin}" ${stroke}/>` +
    `<ellipse cx="76.5" cy="31" rx="2.6" ry="4.2" fill="${skin}" ${stroke}/>` +
    `<path d="M51 28 C 51 17, 77 17, 77 28 L 77 33 C 77 41, 70 46, 64 46 C 58 46, 51 41, 51 33 Z" fill="${skin}" ${stroke}/>` +
    // brows, eyes, nose, mouth
    `<path d="M56.5 26.5 q 2.5 -1.4, 5 0 M66.5 26.5 q 2.5 -1.4, 5 0" fill="none" stroke="${LINE}" stroke-width="1.4" stroke-linecap="round"/>` +
    `<ellipse cx="59" cy="30.5" rx="1.7" ry="2.1" fill="${LINE}"/><ellipse cx="69" cy="30.5" rx="1.7" ry="2.1" fill="${LINE}"/>` +
    `<path d="M64 31 q -1.6 3.4, -0.6 4.6 q 1 0.6, 2.2 0" fill="none" stroke="${shade}" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"/>` +
    `<path d="M60.5 39.5 q 3.5 1.6, 7 0" fill="none" stroke="${LINE}" stroke-width="1.5" stroke-linecap="round"/>` +
    wrinkles
  );
}

function frontHair(g: string, hair: string | null): string {
  if (!hair) return '';
  if (g === 'man') return `<path d="M50 30 C 48 13, 80 11, 78 30 C 76 23, 70 20.5, 63 22.5 C 57 21.5, 52.5 24.5, 50 30 Z" fill="${hair}" ${stroke}/>`;
  if (g === 'woman') return `<path d="M50 32 C 48 12, 80 12, 78 32 C 74 23, 67 19.5, 59 22 C 55 23.5, 51.5 27, 50 32 Z" fill="${hair}" ${stroke}/>`;
  return `<path d="M50 31 C 48 13, 80 13, 78 31 C 75 24, 69 22, 64 21 C 58 22.5, 53 25, 50 31 Z" fill="${hair}" ${stroke}/>`;
}

function glasses(): string {
  return `<g fill="none" stroke="${LINE}" stroke-width="1.3"><circle cx="59" cy="30.5" r="4"/><circle cx="69" cy="30.5" r="4"/><path d="M63 30 L65 30"/></g>`;
}

function extraEye(): string {
  // On the forehead, over any fringe: the thing everyone notices first.
  return `<ellipse cx="64" cy="21.5" rx="4.4" ry="3.1" fill="#fbfaf5" ${stroke}/>` +
    `<circle cx="64" cy="21.5" r="1.8" fill="#5a8f3a"/><circle cx="64" cy="21.5" r="0.8" fill="${LINE}"/>`;
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
  const view = portrait ? '40 9 48 48' : '0 0 128 128';
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${view}" width="128" height="128">${layers.join('')}</svg>`;
}

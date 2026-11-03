// Paper dolls (docs/body-sprites.md): a colonist drawn as a figure composed
// from layered SVG parts, in their own skin and hair, with whatever uranium
// grew on them. The engine sends only a key (Hello.glyphs.dolls); the art
// lives here, like the terrain's tile art (tileart.ts).
//
// Every part is drawn against one skeleton, the same for all six figures, so
// a part drawn once fits every body:
//
//   head          a pear, x 44 to 84, y 27 to 66; eyes at (56, 47), (72, 47)
//   shoulders     (44, 70) and (84, 70), no neck
//   hips          (55, 102) and (73, 102)
//   feet          y = 120
//
// Colonists are squat little weirdos: a big, wide, lumpy head straight on a
// barrel body, stubby legs, big boots, a bulbous nose, and a face with an
// expression of its own (EXPRESSIONS). Two looks were tried and dropped: a
// round head with ears sticking out and a dot-eyes-and-smile face read as
// Reddit's Snoo; a tidy, human-proportioned figure fixed that but had no
// personality, and the same vacant smile on everyone.
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

/** A limb: a stubby rounded stroke from a to b, ending in a hand. */
function arm(x1: number, y1: number, x2: number, y2: number, sleeve: string, skin: string): string {
  return `<path d="M${x1} ${y1} L${x2} ${y2}" stroke="${LINE}" stroke-width="13" stroke-linecap="round"/>` +
    `<path d="M${x1} ${y1} L${x2} ${y2}" stroke="${sleeve}" stroke-width="9" stroke-linecap="round"/>` +
    `<circle cx="${x2}" cy="${y2 + 1}" r="5" fill="${skin}" ${stroke}/>`;
}

// ---- Layers, back to front ------------------------------------------------

function tail(skin: string, shade: string): string {
  const d = 'M80 100 C 100 112, 118 104, 114 82 C 113 75, 107 74, 106 80';
  return `<path d="${d}" fill="none" stroke="${LINE}" stroke-width="12" stroke-linecap="round"/>` +
    `<path d="${d}" fill="none" stroke="${skin}" stroke-width="8" stroke-linecap="round"/>` +
    `<path d="M95 107 C 104 108, 111 101, 112 90" fill="none" stroke="${shade}" stroke-width="1.6" stroke-linecap="round"/>`;
}

function twin(skin: string, shade: string, hair: string | null): string {
  // A small second head budding from behind the left shoulder, asleep, with
  // a nose of its own.
  const tuft = hair ? `<path d="M23 56 C 24 47, 38 46, 40 54 C 35 51, 29 51, 23 56 Z" fill="${hair}" ${stroke}/>` : '';
  return `<path d="M23 60 C 22 50, 41 49, 40 59 C 41 67, 35 71, 31 71 C 26 71, 22 67, 23 60 Z" fill="${skin}" ${stroke}/>${tuft}` +
    `<path d="M26 60 q 1.8 1.4, 3.6 0 M33 60 q 1.8 1.4, 3.6 0" fill="none" stroke="${LINE}" stroke-width="1.3" stroke-linecap="round"/>` +
    `<ellipse cx="31.5" cy="64" rx="2" ry="1.6" fill="${shade}"/>` +
    `<path d="M30 67.5 q 1.5 -0.8, 3 0" fill="none" stroke="${LINE}" stroke-width="1.1" stroke-linecap="round"/>`;
}

function backHair(g: string, hair: string | null): string {
  if (!hair) return '';
  // Long hair falls behind the shoulders, topped with a bun.
  if (g === 'woman') {
    return `<path d="M45 40 C 38 58, 38 76, 46 80 L 82 80 C 90 76, 90 58, 83 40 Z" fill="${hair}" ${stroke}/>` +
      `<circle cx="64" cy="24" r="7" fill="${hair}" ${stroke}/>`;
  }
  return '';
}

function body(suit: string, skin: string): string {
  return arm(44, 72, 34, 95, suit, skin) +
    // stubby legs, then big boots
    `<path d="M55 100 L55 110 M73 100 L73 110" stroke="${LINE}" stroke-width="13" stroke-linecap="round"/>` +
    `<path d="M55 100 L55 110 M73 100 L73 110" stroke="${suit}" stroke-width="9" stroke-linecap="round"/>` +
    `<path d="M45 120 Q 45 110, 55 110 Q 62 110, 62 120 Z M66 120 Q 66 110, 73 110 Q 83 110, 83 120 Z" fill="${BOOTS}" ${stroke}/>` +
    // a barrel of a torso, with a pocket and a seam
    `<path d="M43 66 Q 64 58, 85 66 C 92 78, 91 96, 84 102 Q 64 108, 44 102 C 37 96, 36 78, 43 66 Z" fill="${suit}" ${stroke}/>` +
    `<path d="M64 66 L64 104" stroke="${LINE}" stroke-width="1.2" opacity="0.45"/>` +
    `<path d="M69 78 h 9 v 7 h -9 Z" fill="none" stroke="${LINE}" stroke-width="1.2" opacity="0.6"/>` +
    arm(84, 72, 94, 95, suit, skin);
}

function thirdArm(suit: string, skin: string): string {
  // Out of the right side, waving.
  return arm(83, 82, 106, 62, suit, skin);
}

/** A face's eyes and mouth: how a colonist looks at the world. */
interface Expression {
  /** Pupil offset, x and y, from each eye's centre. */
  look: [number, number];
  /** Top lid: 0 open, up to 1 shut. */
  lid: number;
  /** Brow ends, inner then outer, as y offsets (negative is up); left eye, right eye. */
  brows: [[number, number], [number, number]];
  pupil: number;
  mouth: string;
}

// Mouths are paths centred on (64, 60). None of them is a plain smile.
const EXPRESSIONS: Expression[] = [
  // deadpan: half-lidded, flat
  { look: [0, 0.5], lid: 0.5, brows: [[0, 0], [0, 0]], pupil: 2.1, mouth: 'M58 60.5 L70 60.5' },
  // suspicious: side-eye, one brow down, a smirk
  { look: [2.2, 0], lid: 0.35, brows: [[1.5, -1], [-2, -2.5]], pupil: 2.1, mouth: 'M58 61 Q 65 61, 70 58.5' },
  // worried: brows up in the middle, a small o
  { look: [0, -0.6], lid: 0, brows: [[-3, 1], [-3, 1]], pupil: 1.9, mouth: 'M61.5 60.5 a 2.5 2.2 0 1 0 5 0 a 2.5 2.2 0 1 0 -5 0 Z' },
  // grumpy: brows down in the middle, a frown
  { look: [0, 0.6], lid: 0.25, brows: [[2.5, -1.5], [2.5, -1.5]], pupil: 2.1, mouth: 'M58 62 Q 64 57.5, 70 62' },
  // manic: wide eyes, pin pupils, teeth
  { look: [0, 0], lid: 0, brows: [[-2.5, -3], [-2.5, -3]], pupil: 1.3, mouth: 'M57.5 58.5 L70.5 58.5 L69.5 62.5 L58.5 62.5 Z M61 58.5 L61 62.5 M64 58.5 L64 62.5 M67 58.5 L67 62.5' },
  // sly: looking aside, lopsided
  { look: [-2.2, 0.3], lid: 0.45, brows: [[0, 0], [-1, -3]], pupil: 2.1, mouth: 'M58 59 Q 61 61.5, 66 60.5 L 70 59.5' },
];

/**
 * The expression a body wears: a hash of its figure, tone and hair (the key
 * without its parts), so a colonist keeps their face as they mutate. Flavour
 * only, on the page: it never touches the simulation.
 */
export function expressionOf(body: number): number {
  let h = Math.imul(body ^ 0x5bd1e995, 0x27d4eb2d);
  h ^= h >>> 15;
  h = Math.imul(h, 0x165667b1);
  h ^= h >>> 13;
  return (h >>> 0) % EXPRESSIONS.length;
}

function eye(cx: number, cy: number, e: Expression, skin: string, brow: [number, number], side: -1 | 1): string {
  const lid = e.lid > 0
    ? `<path d="M${cx - 5.5} ${cy} A 5.5 4.6 0 0 1 ${cx + 5.5} ${cy} L ${cx + 5.5} ${cy - 4.6 + 9.2 * e.lid} L ${cx - 5.5} ${cy - 4.6 + 9.2 * e.lid} Z" fill="${skin}"/>` +
      `<path d="M${cx - 5.2} ${cy - 4.6 + 9.2 * e.lid} L${cx + 5.2} ${cy - 4.6 + 9.2 * e.lid}" stroke="${LINE}" stroke-width="1.4" stroke-linecap="round"/>`
    : '';
  // inner end towards the nose
  const inner = cx - side * 5, outer = cx + side * 5;
  return `<ellipse cx="${cx}" cy="${cy}" rx="5.5" ry="4.6" fill="#fbfaf5" ${stroke}/>` +
    `<circle cx="${cx + e.look[0]}" cy="${cy + e.look[1]}" r="${e.pupil}" fill="${LINE}"/>` + lid +
    `<path d="M${inner} ${cy - 7.5 + brow[0]} L${outer} ${cy - 7.5 + brow[1]}" stroke="${LINE}" stroke-width="2.2" stroke-linecap="round"/>`;
}

function head(skin: string, shade: string, senior: boolean, e: Expression): string {
  const bags = senior
    ? `<path d="M51.5 52.5 q 4.5 2, 9 0 M67.5 52.5 q 4.5 2, 9 0" fill="none" stroke="${shade}" stroke-width="1.3" stroke-linecap="round"/>`
    : '';
  return (
    // a wide pear, flat on top, heavy in the jaw; ears tucked flat into it
    `<path d="M46 40 C 46 27, 82 27, 82 40 L 84 50 C 86 61, 76 67, 64 67 C 52 67, 42 61, 44 50 Z" fill="${skin}" ${stroke}/>` +
    `<path d="M45.5 46 q -2.5 2.5, -0.5 6 M82.5 46 q 2.5 2.5, 0.5 6" fill="none" stroke="${shade}" stroke-width="1.6" stroke-linecap="round"/>` +
    eye(56, 47, e, skin, e.brows[0], -1) +
    eye(72, 47, e, skin, e.brows[1], 1) +
    bags +
    // the nose: bulbous, the thing you remember
    `<path d="M62 50 C 57 53, 57.5 58, 62 57.5 Q 64 58.5, 66 57.5 C 70.5 58, 71 53, 66 50 Z" fill="${shade}" ${stroke}/>` +
    `<ellipse cx="62.5" cy="53.5" rx="1.3" ry="1" fill="#ffffff" opacity="0.35"/>` +
    `<path d="${e.mouth}" fill="${e.mouth.endsWith('Z') ? '#f4ece4' : 'none'}" stroke="${LINE}" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/>`
  );
}

function frontHair(g: string, hair: string | null): string {
  if (!hair) return '';
  // a messy crop with tufts
  if (g === 'man') return `<path d="M45 42 C 43 30, 50 25, 56 27 L 58 21 L 62 26 L 67 20 L 70 26 L 76 23 L 76 28 C 84 29, 86 36, 83 42 C 78 35, 70 33, 64 34 C 56 33, 50 35, 45 42 Z" fill="${hair}" ${stroke}/>`;
  // a fringe swept to one side, framing the face
  if (g === 'woman') return `<path d="M44 46 C 42 27, 86 25, 84 46 C 80 38, 76 35, 72 34 C 66 37, 56 36, 50 40 C 48 41, 46 43, 44 46 Z" fill="${hair}" ${stroke}/>`;
  // a bowl cut, dead straight across the brow
  return `<path d="M43 46 C 41 26, 87 26, 85 46 L 85 41 L 43 41 Z" fill="${hair}" ${stroke}/>`;
}

function glasses(): string {
  return `<g fill="none" stroke="${LINE}" stroke-width="1.5"><circle cx="56" cy="47" r="7"/><circle cx="72" cy="47" r="7"/><path d="M63 46 L65 46"/></g>`;
}

function extraEye(): string {
  // On the forehead, over any fringe: the thing everyone notices first.
  return `<ellipse cx="64" cy="35" rx="5" ry="3.6" fill="#fbfaf5" ${stroke}/>` +
    `<circle cx="64" cy="35" r="2" fill="#5a8f3a"/><circle cx="64" cy="35" r="0.9" fill="${LINE}"/>`;
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
    has('vestigial twin') ? twin(skin, shade, hair) : '',
    backHair(g, hair),
    body(suit, skin),
    has('third arm') ? thirdArm(suit, skin) : '',
    head(skin, shade, senior, EXPRESSIONS[expressionOf(Math.floor(key / (1 << layout.parts.length)))]),
    frontHair(g, hair),
    senior ? glasses() : '',
    has('extra eye') ? extraEye() : '',
  ];
  const view = portrait ? '28 14 72 72' : '0 0 128 128';
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${view}" width="128" height="128">${layers.join('')}</svg>`;
}

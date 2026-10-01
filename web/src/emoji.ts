// Picking, from a list of emoji candidates, the first this browser's emoji
// font draws as one glyph. A colonist's look (Hello.glyphs.looks, see
// docs/colonist-looks.md) is such a list: 👨🏿‍🦰, then 👨🏿, then 👨‍🦰, then
// the plain 👨 every font has. A font that does not know a sequence draws its
// parts side by side instead (👨🏿🦰, or 👨 beside a brown square), and the
// only way to tell from here is to measure it: a fused sequence is as wide as
// one emoji, an unfused one is two or more.

/** The emoji fonts, in the order the map's atlas asks for them. */
export const EMOJI_FONT = '"Apple Color Emoji", "Segoe UI Emoji", "Noto Color Emoji", "Twemoji Mozilla", sans-serif';

/** Wider than this many of the plain figure means the font drew pieces. */
const FUSED = 1.4;

let ctx: CanvasRenderingContext2D | null | undefined;
const picked = new Map<string, string>();

function measure(s: string): number {
  if (ctx === undefined) {
    ctx = document.createElement('canvas').getContext('2d');
    if (ctx) ctx.font = `64px ${EMOJI_FONT}`;
  }
  return ctx ? ctx.measureText(s).width : 0;
}

/**
 * The first candidate that draws as a single glyph, or the last (the plain
 * figure) if none does; `fallback` when there are no candidates. Each list is
 * measured once per page.
 */
export function pickGlyph(look: readonly string[] | null | undefined, fallback = ''): string {
  if (!look || look.length === 0) return fallback;
  const key = look.join('\u0000');
  const hit = picked.get(key);
  if (hit !== undefined) return hit;
  const plain = look[look.length - 1];
  const one = measure(plain);
  // No canvas, or a font that measures nothing: trust the plain figure.
  const choice = one > 0 ? look.find((c) => measure(c) <= one * FUSED) ?? plain : plain;
  picked.set(key, choice);
  return choice;
}

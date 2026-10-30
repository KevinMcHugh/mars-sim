// A species-pack sprite (Hello.glyphs.sprites) or a colonist's paper doll as
// an image source, for the panels that show a creature's art beside its name
// (docs/species-pack.md, docs/body-sprites.md).
import { ui } from '../game.svelte';
import { sized } from '../map/atlas';
import { composeDoll } from '../map/dolls';

/**
 * Sprite i as a data: URL, or '' for none. It only ever goes into an <img>,
 * where an SVG runs nothing; sized gives it the dimensions every browser needs.
 */
export function spriteURL(i: number | undefined): string {
  const svg = i === undefined || i < 0 ? undefined : ui.hello?.glyphs.sprites?.[i];
  return svg ? `data:image/svg+xml;charset=utf-8,${encodeURIComponent(sized(svg))}` : '';
}

/**
 * Paper doll key as a data: URL, or '' for none (-1, or no layout yet); a
 * portrait is cropped to head and shoulders.
 */
export function dollURL(key: number | undefined, portrait = false): string {
  const layout = ui.hello?.glyphs.dolls;
  if (key === undefined || key < 0 || !layout || key >= layout.count) return '';
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(composeDoll(key, layout, portrait))}`;
}

// A species-pack sprite (Hello.glyphs.sprites) as an image source, for the
// panels that show an alien's art beside its name (docs/species-pack.md).
import { ui } from '../game.svelte';
import { sized } from '../map/atlas';

/**
 * Sprite i as a data: URL, or '' for none. It only ever goes into an <img>,
 * where an SVG runs nothing; sized gives it the dimensions every browser needs.
 */
export function spriteURL(i: number | undefined): string {
  const svg = i === undefined || i < 0 ? undefined : ui.hello?.glyphs.sprites?.[i];
  return svg ? `data:image/svg+xml;charset=utf-8,${encodeURIComponent(sized(svg))}` : '';
}

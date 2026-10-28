// How big a body draws on the map, from its height (docs/body-sprites.md).
// Display only: the engine sends centimetres, and this curve never touches
// the simulation.

/** The height that draws exactly one tile: an average colonist. */
export const REFERENCE_CM = 178;
/** Below 1, so a ten-foot colonist is visibly bigger and an egg still shows. */
export const SCALE_EXPONENT = 0.75;
export const MIN_SCALE = 0.35;
export const MAX_SCALE = 2.0;

/** A body's size in tiles; 0 cm (a kind with no height) draws at 1. */
export function bodyScale(heightCM: number): number {
  if (!(heightCM > 0)) return 1;
  const s = Math.pow(heightCM / REFERENCE_CM, SCALE_EXPONENT);
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, s));
}

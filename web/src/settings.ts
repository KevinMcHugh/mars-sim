// New-game settings, from the URL (?width=2000&seed=7&fog-of-war=false&zoning-auto=true&siting-auto=true&deepest-level=3) over
// these defaults. Keys are mars-sim.yaml's (see docs/config-file.md); a seed
// left out lets the engine pick one. They fill the New game form, which a
// cold load opens rather than starting a game (docs/frontend-web.md).

import type { Settings } from './sim/client';

// deepest-level 1 is the engine's default: the colony stays on the landing
// level, and the page shows no level controls (docs/z-levels.md).
const DEFAULTS: Settings = { width: 10000, height: 10000, colonists: 6, 'fog-of-war': true, 'zoning-auto': false, 'siting-auto': false, 'deepest-level': 1 };

/**
 * Most settlers one colony ship carries: sim.DefaultConfig's ship-capacity.
 * The page never sends its own, so the engine uses this one too
 * (settings.test.mjs pins the two together).
 */
export const SHIP_CAPACITY = 20;

/**
 * How n founders come down: as few ships as capacity allows, loaded as evenly
 * as can be (30 come down as two ships of 15, not 20 and 10). shipLoads in
 * internal/sim/ship.go, which decides it; this only tells the player.
 */
export function shipLoads(n: number, capacity = SHIP_CAPACITY): number[] {
  if (!Number.isFinite(n) || n <= 0) return [];
  n = Math.floor(n);
  capacity = Math.max(1, Math.floor(capacity));
  const ships = Math.ceil(n / capacity);
  return Array.from({ length: ships }, (_, i) => Math.floor(n / ships) + (i < n % ships ? 1 : 0));
}

/** shipLoads in words, for the New game form: "1 ship", "2 ships of 15", "3 ships of 7 or 6". */
export function describeShips(loads: number[]): string {
  if (loads.length === 0) return 'no ships';
  if (loads.length === 1) return '1 ship';
  const most = loads[0], least = loads[loads.length - 1];
  return `${loads.length} ships of ${most === least ? most : `${most} or ${least}`}`;
}

export function initialSettings(): Settings {
  const s: Settings = { ...DEFAULTS };
  const params = new URLSearchParams(location.search);
  for (const key of ['width', 'height', 'colonists', 'seed', 'deepest-level']) {
    const v = params.get(key);
    if (v !== null && v !== '') s[key] = Number(v);
  }
  const fog = params.get('fog-of-war');
  if (fog !== null) s['fog-of-war'] = fog !== 'false';
  for (const key of ['zoning-auto', 'siting-auto']) {
    const v = params.get(key);
    if (v !== null) s[key] = v !== 'false';
  }
  return s;
}

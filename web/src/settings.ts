// New-game settings, from the URL (?width=2000&seed=7&fog-of-war=false&zoning-auto=true) over
// these defaults. Keys are mars-sim.yaml's (see docs/config-file.md); a seed
// left out lets the engine pick one.

import type { Settings } from './sim/client';

const DEFAULTS: Settings = { width: 10000, height: 10000, colonists: 6, 'fog-of-war': true, 'zoning-auto': false };

export function initialSettings(): Settings {
  const s: Settings = { ...DEFAULTS };
  const params = new URLSearchParams(location.search);
  for (const key of ['width', 'height', 'colonists', 'seed']) {
    const v = params.get(key);
    if (v !== null && v !== '') s[key] = Number(v);
  }
  const fog = params.get('fog-of-war');
  if (fog !== null) s['fog-of-war'] = fog !== 'false';
  const auto = params.get('zoning-auto');
  if (auto !== null) s['zoning-auto'] = auto !== 'false';
  return s;
}

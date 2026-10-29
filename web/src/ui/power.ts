// The Power tab's data (the power topic, internal/wire/boards.go PowerDesk)
// and the words it shows for it. See docs/power.md.

export interface Spot { x: number; y: number; l: number }

export interface Reactor extends Spot {
  owner: string;
  price: number;
  forSale: boolean;
  batteries: number;
  ore: number;
  pellets: number;
  ice: number;
  operator?: string;
  status: 'working' | 'full' | 'no-uranium' | 'no-ice' | 'idle';
}

export interface Power {
  on: boolean;
  policy: string;
  policies: string[];
  cooldownHours: number;
  reactors: Reactor[];
  charter?: { name: string; wallet: number; sold: number; royalties: number; remitted: number };
  batteries: number;
  price: number;
  priceKind: 'tariff' | 'bid' | 'charter';
  building: (Spot & { owner: string })[];
  charged: number;
  cookedPowered: number;
  cookedCold: number;
  reserve: number;
  uranium: number;
  pellets: number;
  uraniumReserve: number;
  prospector?: Spot & { name: string };
  publicOption?: { latched: boolean; dryHours: number; days: number };
  commission?: { savings: number; able: number };
}

/** How a reactor is running, in a few words. */
export function statusText(r: Reactor): string {
  switch (r.status) {
    case 'working': return r.operator ? `${r.operator} is working it` : 'working';
    case 'full': return 'resting: its owner has batteries enough';
    case 'no-uranium': return 'no uranium';
    case 'no-ice': return 'no ice to charge its pellets';
    default: return 'fuelled, waiting for an operator';
  }
}

/** Whether a reactor's status is trouble the player may want to look into. */
export function stalled(r: Reactor): boolean {
  return r.status === 'no-uranium' || r.status === 'no-ice';
}

/** "1 of 2 reactors working · 1 going up", or why there is none. */
export function headline(p: Power): string {
  if (!p.on) return 'Power is off in this game.';
  const n = p.reactors.length;
  const working = p.reactors.filter((r) => r.status === 'working').length;
  const up = p.building.length ? ` · ${p.building.length} going up` : '';
  if (n === 0) return p.building.length ? `No reactor yet${up}` : 'No reactor yet';
  return `${working} of ${n} reactor${n === 1 ? '' : 's'} working${up}`;
}

/** The share of food recipes worked with power, as a whole percent, or null before any. */
export function poweredShare(p: Power): number | null {
  const all = p.cookedPowered + p.cookedCold;
  return all > 0 ? Math.round((p.cookedPowered * 100) / all) : null;
}

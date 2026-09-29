// The speed selector: Pause, then three running speeds. + and - step through
// them in this order, and space toggles pause. The engine's speed is ticks per
// second (sim.SetTicksPerSecond), and pause is separate from it, so "Pause"
// keeps the last running speed and resuming goes back to it.

export interface Speed {
  name: string;
  /** Ticks per second; absent for Pause. */
  tps?: number;
  title: string;
}

export const SPEEDS: Speed[] = [
  { name: 'Pause', title: 'Pause (space)' },
  // The game's default pace (tps in sim.DefaultConfig).
  { name: 'Fast', tps: 8, title: '8 ticks a second' },
  { name: 'Faster', tps: 64, title: '64 ticks a second' },
  // As fast as the machine can: the engine has no upper cap.
  { name: 'Max', tps: 1_000_000, title: 'As fast as it will go' },
];

/** The selector position for an engine state: Pause, or the running speed nearest tps. */
export function speedIndex(paused: boolean, tps: number): number {
  if (paused) return 0;
  let best = 1;
  for (let i = 1; i < SPEEDS.length; i++) {
    if (Math.abs(Math.log(SPEEDS[i].tps! / tps)) < Math.abs(Math.log(SPEEDS[best].tps! / tps))) best = i;
  }
  return best;
}

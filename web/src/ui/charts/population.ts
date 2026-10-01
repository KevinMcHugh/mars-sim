// The population topic (internal/wire/charts.go: PopulationTopic), and the
// activity arithmetic the Activity chart needs, kept out of the component so
// it can be tested.

export interface Population {
  tick: number[];
  colonists: number[];
  meals: number[];
  colonySize: number[];
  fixtures: number[];
  /** Activity names, indexing activity and walking. */
  activities: string[];
  /** activity[a][i]: colonist-ticks on activity a since the sample before i. */
  activity: number[][];
  /** walking[a][i]: the part of activity[a][i] spent walking there. */
  walking: number[][];
}

/** One sample's tally per activity, and the ticks it covers. */
export interface Slice { own: number[]; walk: number[]; ticks: number; total: number }

/**
 * Sum samples [lo, hi) per activity: own time (doing it) and walking time
 * (getting there). Each sample covers the ticks since the one before it, or
 * since the start of the game.
 */
export function slice(p: Population, lo: number, hi: number): Slice {
  const n = p.activities.length;
  const own = new Array(n).fill(0), walk = new Array(n).fill(0);
  let total = 0;
  for (let a = 0; a < n; a++) {
    for (let i = lo; i < hi; i++) {
      walk[a] += p.walking[a][i];
      own[a] += p.activity[a][i] - p.walking[a][i];
      total += p.activity[a][i];
    }
  }
  const ticks = hi > lo ? p.tick[hi - 1] - (lo > 0 ? p.tick[lo - 1] : 0) : 0;
  return { own, walk, ticks, total };
}

/**
 * Colonist-ticks as what the chart plots: a share of all colonist time
 * (0..1), or the average number of colonists doing it.
 */
export function scale(v: number, s: Slice, counts: boolean): number {
  if (counts) return s.ticks > 0 ? v / s.ticks : 0;
  return s.total > 0 ? v / s.total : 0;
}

/**
 * Split n samples into at most cols runs, [lo, hi), for one chart column
 * each. A single sample covers as little as 50 ticks, and plotted one by one
 * the stack is noise; summed per column, as the TUI does, it reads.
 */
export function columns(n: number, cols: number): [number, number][] {
  const c = Math.max(1, Math.min(n, cols));
  const out: [number, number][] = [];
  for (let x = 0; x < c; x++) {
    const lo = Math.floor((x * n) / c);
    out.push([lo, Math.max(lo + 1, Math.floor(((x + 1) * n) / c))]);
  }
  return out;
}

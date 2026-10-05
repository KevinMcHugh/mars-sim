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
  /** Fixture kinds tracked, indexing fixtureCounts. */
  fixtureKinds: string[];
  /** fixtureCounts[f][i]: how many of kind f stood at sample i. */
  fixtureCounts: number[][];
  /** Every skill's colonists by rank. */
  skills: SkillRanks[];
}

/** One skill's colonists by rank: ranks[r][i] stood at exactly rank r at sample i. */
export interface SkillRanks {
  skill: string;
  /** One title per rank, '' (untrained) first; a title may cover several ranks. */
  labels: string[];
  ranks: number[][];
}

/** A series the Population view's tracked chart can show. */
export interface Tracked { key: string; title: string; group: string; values: number[] }

/**
 * Every series the tracked chart can show, as the TUI's Population tab steps
 * through them: all fixtures, each kind of fixture, then for each skill the
 * colonists at each title or better ("chef or better" counts chefs and master
 * chefs). A title covering several ranks is one series, from its lowest rank.
 */
export function trackedSeries(p: Population): Tracked[] {
  const out: Tracked[] = [{ key: 'fixtures', title: 'Fixtures', group: 'Fixtures', values: p.fixtures }];
  p.fixtureKinds.forEach((kind, f) => {
    out.push({ key: `fixture:${kind}`, title: cap(kind), group: 'Fixtures', values: p.fixtureCounts[f] });
  });
  for (const s of p.skills) {
    for (let r = 1; r < s.labels.length; r++) {
      if (s.labels[r] === s.labels[r - 1]) continue;
      const top = r === s.labels.length - 1;
      const values = p.tick.map((_, i) => {
        let n = 0;
        for (let q = r; q < s.ranks.length; q++) n += s.ranks[q][i];
        return n;
      });
      out.push({
        key: `skill:${s.skill}:${r}`,
        title: `Colonists: ${s.labels[r]}${top ? '' : ' or better'}`,
        group: `Skill: ${s.skill}`,
        values,
      });
    }
  }
  return out;
}

function cap(s: string): string { return s.charAt(0).toUpperCase() + s.slice(1); }

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

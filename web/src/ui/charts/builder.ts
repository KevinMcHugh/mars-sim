// The chart builder's arithmetic, kept out of the components so node --test
// can run it: the metrics catalog and series topics (internal/wire/charts.go:
// MetricsTopic, SeriesTopic), the charts a player has made, and turning a
// chart's series into the columns uPlot draws. See docs/charts.md.

/** One metric the sim measures (MetricLine). */
export interface MetricLine {
  key: string;
  label: string;
  group: string;
  doc: string;
  /** A level is a reading; a total is a running total since the landing. */
  kind: 'level' | 'total';
  unit: 'count' | 'dollars';
  per: 'colony' | 'item' | 'account' | 'fixture' | 'skill-rank';
}

/** What a metric's subjects are, for the picker: "good" for a per-item metric. */
export const SUBJECT_NOUN: Record<MetricLine['per'], string> = {
  colony: '', item: 'good', account: 'account', fixture: 'kind', 'skill-rank': 'rank',
};

/**
 * Orders a metric's series for the picker. The catalog lists them in the
 * order they started, which for ranks is the order colonists happened to
 * reach them. Fixture kinds go alphabetically, and skill ranks by skill and
 * then by rank as a number ("cooking 0: untrained", "cooking 2: cook", …,
 * "mining 0: untrained"). Goods and accounts keep the catalog's order.
 */
export function pickerOrder(per: MetricLine['per'], series: SeriesLine[]): SeriesLine[] {
  if (per !== 'fixture' && per !== 'skill-rank') return series;
  return [...series].sort((a, b) => (a.subject ?? '').localeCompare(b.subject ?? '', 'en', { numeric: true }));
}

/** One series the picker can offer (SeriesLine): a metric for one subject. */
export interface SeriesLine { key: string; metric: number; subject?: string; ended?: boolean }

/** The metrics topic: everything there is to chart. */
export interface Catalog { metrics: MetricLine[]; series: SeriesLine[] }

/**
 * A series topic: readings, with the tick and the clock hour of each (hours
 * since the landing day's midnight). `every` is the hours between samples.
 */
export interface SeriesData {
  key: string;
  found: boolean;
  every: number;
  tick: number[];
  hour: number[];
  values: number[];
}

/**
 * A chart the player made. `bucket` is 0 to plot the readings as they are
 * (levels as levels, totals as running totals), or a width in clock hours to
 * plot how much of each total happened in each bucket (levels as their
 * reading at the bucket's end).
 */
export interface ChartSpec { id: string; title: string; series: string[]; bucket: number }

/** The bucket widths offered, in clock hours. */
export const BUCKETS: { hours: number; label: string }[] = [
  { hours: 0, label: 'readings' },
  { hours: 1, label: 'per hour' },
  { hours: 6, label: 'per 6 hours' },
  { hours: 12, label: 'per half-day' },
  { hours: 24, label: 'per day' },
  { hours: 168, label: 'per week' },
];

/** Most series one chart takes: the categorical palette has eight hues. */
export const MAX_SERIES = 8;

/** The metric a series key belongs to: "price/meal" is "price". */
export function metricKeyOf(key: string): string {
  const i = key.indexOf('/');
  return i < 0 ? key : key.slice(0, i);
}

/** The catalog's metric for a series key, if any. */
export function metricOf(cat: Catalog | undefined, key: string): MetricLine | undefined {
  const k = metricKeyOf(key);
  return cat?.metrics.find((m) => m.key === k);
}

/** A series' name for a legend: "Price · meal", or just "Colonists". */
export function seriesName(cat: Catalog | undefined, key: string): string {
  const m = metricOf(cat, key);
  const line = cat?.series.find((s) => s.key === key);
  const subject = line?.subject ?? key.slice(key.indexOf('/') + 1);
  if (!m) return key;
  return subject ? `${m.label} · ${subject}` : m.label;
}

function gcd(a: number, b: number): number {
  while (b) [a, b] = [b, a % b];
  return a;
}

/**
 * The bucket width actually used: the one asked for, widened to a whole
 * number of sampling intervals (the history samples every `every` hours, so
 * a bucket that splits an interval cannot be filled exactly).
 */
export function effectiveBucket(bucket: number, every: number): number {
  if (bucket <= 0) return 0;
  every = Math.max(1, every);
  return (bucket / gcd(bucket, every)) * every;
}

/** A clock hour as the top bar names it: "day 43 06:00". */
export function hourLabel(h: number): string {
  return `day ${Math.floor(h / 24) + 1} ${String(((h % 24) + 24) % 24).padStart(2, '0')}:00`;
}

/** A bucket starting at clock hour `start`, `width` hours wide: "D43", "D43 H1", "D43 06–12". */
export function bucketLabel(start: number, width: number): string {
  const day = Math.floor(start / 24) + 1;
  const hh = (n: number) => String(n).padStart(2, '0');
  if (width % 24 === 0) return width === 24 ? `D${day}` : `D${day}–D${day + width / 24 - 1}`;
  if (width === 12 && start % 12 === 0) return `D${day} H${(start % 24) / 12 + 1}`;
  const from = start % 24;
  return `D${day} ${hh(from)}–${hh(from + width)}`;
}

/** What a chart plots: an x column (clock hours) and a y column per series. */
export interface Columns {
  x: number[];
  ys: (number | null)[][];
  /** The bucket width used (0 for readings), and the history's interval. */
  bucket: number;
  every: number;
}

/**
 * A chart's columns. Every series is sampled on one shared hourly grid, so
 * the x column is the union of their hours (or buckets). A total is zero
 * before its first reading (that is why it has none); a level has no value
 * there, so it is a gap.
 */
export function columns(spec: ChartSpec, cat: Catalog | undefined, data: Record<string, SeriesData | undefined>): Columns {
  const found = spec.series.map((k) => data[k]).map((d) => (d && d.found && d.values.length ? d : null));
  const every = Math.max(1, ...found.map((d) => d?.every ?? 1));
  const bucket = effectiveBucket(spec.bucket, every);
  const totals = spec.series.map((k) => metricOf(cat, k)?.kind === 'total');
  // Per series, its value at each x.
  const maps = found.map((d, i) => {
    const m = new Map<number, number>();
    if (!d) return m;
    if (!bucket) {
      d.hour.forEach((h, j) => m.set(h, d.values[j]));
      return m;
    }
    for (let j = 0; j < d.values.length; j++) {
      // A sample at hour h covers the interval before it.
      const b = Math.floor((d.hour[j] - 1) / bucket) * bucket;
      if (totals[i]) {
        const before = j > 0 ? d.values[j - 1] : 0;
        m.set(b, (m.get(b) ?? 0) + d.values[j] - before);
      } else {
        m.set(b, d.values[j]); // the last reading in the bucket
      }
    }
    return m;
  });
  const xs = new Set<number>();
  for (const m of maps) for (const x of m.keys()) xs.add(x);
  const x = [...xs].sort((a, b) => a - b);
  const ys = maps.map((m, i) => {
    let started = false;
    return x.map((at) => {
      const v = m.get(at);
      if (v !== undefined) {
        started = true;
        return v;
      }
      // A total before its first reading was zero; after it, a missing
      // bucket is one where nothing happened. A level just has no reading.
      if (totals[i] && found[i] && (bucket || !started)) return 0;
      return null;
    });
  });
  return { x, ys, bucket, every };
}

/** Lowest, highest and latest of a column, ignoring gaps. */
export function stats(v: readonly (number | null)[]): { now: number; min: number; max: number } | null {
  let min = Infinity, max = -Infinity, now = NaN, n = 0;
  for (const x of v) {
    if (x == null) continue;
    min = Math.min(min, x); max = Math.max(max, x); now = x; n++;
  }
  return n ? { now, min, max } : null;
}

let idSeq = 0;
/** A fresh chart id, unique on this page. */
export function newChartId(): string {
  return `c${Date.now().toString(36)}${(idSeq++).toString(36)}`;
}

/**
 * The charts a new player starts with: the ones the chart system was asked
 * for (money changing hands per half-day, open and posted bids, the
 * treasury, a price), to edit or throw away.
 */
export const STARTER_CHARTS: ChartSpec[] = [
  { id: 'start-moved', title: 'Money changing hands', series: ['money-moved/'], bucket: 12 },
  { id: 'start-open-bids', title: 'Open bids', series: ['open-bids/all'], bucket: 0 },
  { id: 'start-bids-posted', title: 'Bids posted', series: ['bids-posted/all'], bucket: 0 },
  { id: 'start-treasury', title: 'Treasury', series: ['balance/treasury'], bucket: 0 },
  { id: 'start-meal', title: 'Meal price', series: ['price/meal'], bucket: 0 },
];

/** Saved charts, checked: anything malformed is dropped, not trusted. */
export function parseCharts(v: unknown): ChartSpec[] | null {
  if (!Array.isArray(v)) return null;
  const out: ChartSpec[] = [];
  for (const c of v) {
    if (!c || typeof c !== 'object') continue;
    const { id, title, series, bucket } = c as Record<string, unknown>;
    if (typeof id !== 'string' || typeof title !== 'string' || !Array.isArray(series)) continue;
    const keys = series.filter((s): s is string => typeof s === 'string').slice(0, MAX_SERIES);
    const b = typeof bucket === 'number' && BUCKETS.some((x) => x.hours === bucket) ? bucket : 0;
    out.push({ id, title: title.slice(0, 80), series: keys, bucket: b });
  }
  return out;
}

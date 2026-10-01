<script lang="ts">
  // Perf: ticks per second and milliseconds per tick over the last five
  // minutes, from the perf topic, as the TUI's Perf screen draws them
  // (docs/perf-screen.md). Two charts, not one with two axes: the measures
  // have nothing in common but the clock.
  import type uPlot from 'uplot';
  import { subscribe, topics } from '../../game.svelte';
  import Chart, { type Tip } from './Chart.svelte';
  import { axis, SERIES } from './theme';

  interface Perf { bucketMs: number; start: number; tps: number; ticks: number[]; busyMs: number[]; maxMs: number[] }

  $effect(() => subscribe('perf'));
  const perf = $derived(topics.data.perf as Perf | undefined);

  // A tick rate per bucket is noise (a quarter second at 8 tps is two ticks,
  // or one, or three), so each point averages the trailing second, as the
  // TUI does.
  const series = $derived.by(() => {
    if (!perf) return null;
    const n = perf.ticks.length;
    const per = Math.max(1, Math.round(1000 / perf.bucketMs));
    const x = new Float64Array(n), tps = new Float64Array(n);
    const ms: (number | null)[] = new Array(n);
    let sum = 0;
    for (let i = 0; i < n; i++) {
      x[i] = ((i + 1 - n) * perf.bucketMs) / 1000; // seconds before the newest
      sum += perf.ticks[i];
      if (i >= per) sum -= perf.ticks[i - per];
      tps[i] = sum / ((Math.min(i + 1, per) * perf.bucketMs) / 1000);
      // No ticks, no cost: a gap, not an impossibly fast zero.
      ms[i] = perf.ticks[i] > 0 ? perf.busyMs[i] / perf.ticks[i] : null;
    }
    return { x, tps, ms };
  });

  function stats(v: ArrayLike<number | null>): { now: number; min: number; max: number; mean: number } | null {
    let min = Infinity, max = -Infinity, sum = 0, n = 0, now = NaN;
    for (let i = 0; i < v.length; i++) {
      const x = v[i];
      if (x == null) continue;
      min = Math.min(min, x); max = Math.max(max, x); sum += x; n++; now = x;
    }
    return n ? { now, min, max, mean: sum / n } : null;
  }
  const tpsStats = $derived(series ? stats(series.tps) : null);
  const msStats = $derived(series ? stats(series.ms) : null);

  const fmtTps = (v: number) => (v >= 100 ? v.toFixed(0) : v.toFixed(1));
  const fmtMs = (v: number) => (v >= 100 ? v.toFixed(1) : v >= 10 ? v.toFixed(2) : v.toFixed(3)) + ' ms';
  const ago = (s: number) => (s === 0 ? 'now' : `${-s}s ago`);

  const xAxis = axis({ values: (_u, vals) => vals.map((v) => (v === 0 ? 'now' : `${-v}s`)) });
  const base = (color: string, label: string, fmt: (v: number) => string): Omit<uPlot.Options, 'width' | 'height'> => ({
    scales: { x: { time: false }, y: { range: (_u, _lo, hi) => [0, hi > 0 ? hi * 1.1 : 1] } },
    axes: [xAxis, axis({ size: 52, values: (_u, vals) => vals.map((v) => fmt(v)) })],
    series: [{}, { label, stroke: color, width: 2, fill: color + '22', points: { show: false }, spanGaps: false }],
    cursor: { points: { size: 8, fill: color } },
    legend: { show: false },
  });
  const tpsOpts = base(SERIES[0], 'ticks/s', fmtTps);
  const msOpts = base(SERIES[1], 'ms/tick', (v) => (v >= 10 ? v.toFixed(0) : v.toFixed(v >= 1 ? 1 : 2)));

  const tpsTip = (i: number): Tip | null => series ? {
    title: ago(series.x[i]), rows: [{ color: SERIES[0], label: 'ticks/s', value: fmtTps(series.tps[i]) }],
  } : null;
  const msTip = (i: number): Tip | null => {
    if (!series || !perf) return null;
    const v = series.ms[i];
    return {
      title: ago(series.x[i]),
      rows: v == null
        ? [{ label: 'no ticks (paused)', value: '' }]
        : [{ color: SERIES[1], label: 'mean tick', value: fmtMs(v) }, { label: 'slowest tick', value: fmtMs(perf.maxMs[i]) }],
    };
  };
</script>

{#if !series}
  <p class="muted">Loading…</p>
{:else}
  <h3>Ticks per second{perf && perf.tps < 100000 ? ` · set to ${perf.tps}` : ''}</h3>
  {#if tpsStats}<p class="stats">now {fmtTps(tpsStats.now)} · min {fmtTps(tpsStats.min)} · max {fmtTps(tpsStats.max)} · mean {fmtTps(tpsStats.mean)}</p>{/if}
  <Chart opts={tpsOpts} data={[series.x, series.tps]} height={150} tip={tpsTip} label="Ticks per second over the last five minutes" />

  <h3>Milliseconds per tick</h3>
  {#if msStats}<p class="stats">now {fmtMs(msStats.now)} · min {fmtMs(msStats.min)} · max {fmtMs(msStats.max)} · mean {fmtMs(msStats.mean)}</p>{/if}
  <Chart opts={msOpts} data={[series.x, series.ms]} height={150} tip={msTip} label="Milliseconds per tick over the last five minutes" />
{/if}

<style>
  h3 { font-size: 13px; margin: 12px 0 2px; color: #ffffff; }
  h3:first-child { margin-top: 0; }
  .stats { margin: 0 0 4px; font-size: 12px; color: #c3c2b7; font-variant-numeric: tabular-nums; }
  .muted { color: var(--muted); font-weight: normal; }
</style>

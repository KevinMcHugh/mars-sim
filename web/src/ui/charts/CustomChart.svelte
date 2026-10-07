<script lang="ts">
  // One chart the player made (docs/charts.md): up to eight series from the
  // metrics catalog, each read from its own series topic. Readings plot as
  // they are; with a bucket width, each total plots as bars of how much of
  // it happened in each bucket ("money changing hands per half-day") and
  // each level as its reading at the bucket's end. Dollars and counts get an
  // axis each, so a price and a count of bids can share a chart.
  import type uPlot from 'uplot';
  import { saveCharts, topics } from '../../game.svelte';
  import { money } from '../format';
  import { BUCKETS, bucketLabel, columns, hourLabel, MAX_SERIES, metricOf, seriesName, stats,
    type Catalog, type ChartSpec, type SeriesData } from './builder';
  import Chart, { type Tip } from './Chart.svelte';
  import SeriesPicker from './SeriesPicker.svelte';
  import Subscribe from './Subscribe.svelte';
  import { axis, SERIES } from './theme';

  let { spec, cat, onremove }: { spec: ChartSpec; cat: Catalog | undefined; onremove: () => void } = $props();

  let picking = $state(false);

  const data = $derived(Object.fromEntries(spec.series.map((k) => [k, topics.data[`series:${k}`] as SeriesData | undefined])));
  const cols = $derived(columns(spec, cat, data));
  const any = $derived(cols.x.length > 0);

  const grouped = new Intl.NumberFormat();
  const compact = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 });
  const fmt = (unit: string, v: number) => (unit === 'dollars' ? money(Math.round(v)) : grouped.format(Math.round(v)));

  // What the chart's shape depends on (not its data): rebuilt only when this
  // string changes, so a new reading is a setData, never a rebuild.
  const shape = $derived(JSON.stringify({
    bucket: cols.bucket,
    series: spec.series.map((k) => {
      const m = metricOf(cat, k);
      return { unit: m?.unit ?? 'count', total: m?.kind === 'total' };
    }),
  }));

  /**
   * Bars for a total in buckets: each bucket's slot split between the bars
   * drawn there, drawn from x (the bucket's middle) half a bucket each way.
   */
  function bars(slot: number, slots: number, width: number): uPlot.Series.PathBuilder {
    return (u, sidx, i0, i1) => {
      const xs = u.data[0], ys = u.data[sidx];
      const scale = u.series[sidx].scale ?? 'y';
      const p = new Path2D();
      const y0 = u.valToPos(0, scale, true);
      for (let i = i0; i <= i1; i++) {
        const v = ys[i];
        if (v == null) continue;
        const left = u.valToPos(xs[i] - width / 2, 'x', true), right = u.valToPos(xs[i] + width / 2, 'x', true);
        const gap = Math.min(2 * devicePixelRatio, (right - left) * 0.15);
        const w = (right - left - gap) / slots;
        const y = u.valToPos(v, scale, true);
        p.rect(left + gap / 2 + slot * w, Math.min(y, y0), Math.max(1, w - (slots > 1 ? 1 : 0)), Math.abs(y0 - y));
      }
      return { stroke: p, fill: p };
    };
  }

  const opts = $derived.by((): Omit<uPlot.Options, 'width' | 'height'> => {
    const s = JSON.parse(shape) as { bucket: number; series: { unit: string; total: boolean }[] };
    const units = [...new Set(s.series.map((x) => x.unit))];
    const barred = s.series.map((x) => s.bucket > 0 && x.total);
    const slots = barred.filter(Boolean).length;
    let slot = 0;
    const half = s.bucket / 2;
    const yRange: uPlot.Range.Function = (_u, lo, hi) => [Math.min(0, lo), hi > 0 ? hi * 1.1 : 1];
    return {
      scales: {
        x: { time: false, range: (_u, lo, hi) => [lo - half, hi + half] },
        ...Object.fromEntries(units.map((u) => [u, { range: yRange }])),
      },
      axes: [
        axis({
          incrs: [1, 2, 3, 6, 12, 24, 48, 72, 168, 336, 672, 1344, 2688],
          values: (u, vals) => {
            const wide = (u.scales.x.max ?? 0) - (u.scales.x.min ?? 0) > 96;
            return vals.map((v) => (wide ? `D${Math.floor(v / 24) + 1}` : `D${Math.floor(v / 24) + 1} ${String(v % 24).padStart(2, '0')}h`));
          },
        }),
        ...units.map((unit, i) => axis({
          scale: unit, side: i === 0 ? 3 : 1, size: 48, grid: { show: i === 0, stroke: '#2c2c2a', width: 1 },
          values: (_u, vals) => vals.map((v) => (unit === 'dollars' ? '$' : '') + compact.format(v)),
        })),
      ],
      series: [
        {},
        ...s.series.map((x, i) => {
          const color = SERIES[i % SERIES.length];
          return barred[i]
            ? { scale: x.unit, stroke: color, fill: color, width: 0, paths: bars(slot++, slots, s.bucket), points: { show: false } }
            : { scale: x.unit, stroke: color, width: 2, points: { show: false }, spanGaps: s.bucket > 0 };
        }),
      ],
      cursor: { points: { size: 7 } },
      legend: { show: false },
    };
  });

  // In buckets x is the bucket's middle, so the bars straddle it and a
  // level's point sits in the middle of the span it closes.
  const plotData = $derived([
    Float64Array.from(cols.x, (x) => x + cols.bucket / 2),
    ...cols.ys,
  ] as uPlot.AlignedData);

  const tip = (i: number): Tip | null => {
    if (i >= cols.x.length) return null;
    const x = cols.x[i];
    return {
      title: cols.bucket ? bucketLabel(x, cols.bucket) : hourLabel(x),
      rows: spec.series.map((k, s) => {
        const v = cols.ys[s][i];
        return { color: SERIES[s % SERIES.length], label: seriesName(cat, k), value: v == null ? '—' : fmt(metricOf(cat, k)?.unit ?? 'count', v) };
      }),
    };
  };

  const bucketName = $derived(BUCKETS.find((b) => b.hours === cols.bucket)?.label ?? `per ${cols.bucket} hours`);

  function add(key: string): void {
    if (!spec.series.includes(key) && spec.series.length < MAX_SERIES) spec.series.push(key);
    picking = false;
    saveCharts();
  }
  function drop(key: string): void {
    spec.series.splice(spec.series.indexOf(key), 1);
    saveCharts();
  }
</script>

{#each spec.series as key (key)}<Subscribe topic="series:{key}" />{/each}

<div class="card">
  <div class="head">
    <input class="title" aria-label="Chart title" bind:value={spec.title} onchange={saveCharts} />
    <select aria-label="Buckets" bind:value={spec.bucket} onchange={saveCharts}>
      {#each BUCKETS as b (b.hours)}<option value={b.hours}>{b.label}</option>{/each}
    </select>
    <button type="button" class="quiet" title="Remove this chart" aria-label="Remove chart" onclick={onremove}>✕</button>
  </div>
  {#if spec.bucket > 0 && cols.bucket !== spec.bucket && any}
    <p class="note">In buckets of {cols.bucket} hours: the history is sampled every {cols.every} hours now.</p>
  {/if}

  {#if !spec.series.length}
    <p class="muted">Add a series to start.</p>
  {:else if !any}
    <p class="muted">No readings yet: samples come at the top of each colony hour.</p>
  {:else}
    <Chart {opts} data={plotData} height={150} {tip} label="{spec.title}: {spec.series.map((k) => seriesName(cat, k)).join(', ')}" />
  {/if}

  <ul class="legend">
    {#each spec.series as key, i (key)}
      {@const m = metricOf(cat, key)}
      {@const st = stats(cols.ys[i] ?? [])}
      <li>
        <span class="key" style="--c: {SERIES[i % SERIES.length]}"></span>
        <span class="name">{seriesName(cat, key)}{#if m?.kind === 'total'}<span class="how">{cols.bucket ? bucketName : 'running total'}</span>{/if}</span>
        <span class="value">{st ? fmt(m?.unit ?? 'count', st.now) : data[key]?.found === false ? 'no data' : ''}</span>
        <button type="button" class="x" aria-label="Remove {seriesName(cat, key)}" onclick={() => drop(key)}>×</button>
      </li>
    {/each}
  </ul>

  {#if picking && cat}
    <SeriesPicker {cat} taken={spec.series} onpick={add} oncancel={() => (picking = false)} />
  {:else if spec.series.length < MAX_SERIES}
    <button type="button" class="add" disabled={!cat} onclick={() => (picking = true)}>+ Add series</button>
  {/if}
</div>

<style>
  .card { margin-bottom: 14px; padding-bottom: 12px; border-bottom: 1px solid rgba(255, 255, 255, 0.08); }
  .card:last-child { border-bottom: none; }
  .head { display: flex; gap: 6px; align-items: center; margin-bottom: 4px; }
  .title { flex: 1; min-width: 0; font-size: 13px; font-weight: 600; color: #ffffff; background: transparent; border: 1px solid transparent; padding: 2px 4px; }
  .title:hover, .title:focus { border-color: rgba(255, 255, 255, 0.2); }
  .quiet { background: transparent; border-color: transparent; color: #898781; }
  .note, .muted { margin: 2px 0 4px; font-size: 12px; color: #898781; }
  .legend { list-style: none; margin: 6px 0 0; padding: 0; font-size: 12px; }
  .legend li { display: flex; align-items: center; gap: 6px; color: #c3c2b7; line-height: 1.7; }
  .key { width: 9px; height: 9px; border-radius: 2px; background: var(--c); flex: none; }
  .name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .how { color: #898781; margin-left: 6px; }
  .value { color: #ffffff; font-variant-numeric: tabular-nums; }
  .x { background: transparent; border: none; color: #898781; padding: 0 4px; cursor: pointer; }
  .x:hover { color: #ffffff; }
  .add { margin-top: 6px; background: transparent; }
</style>

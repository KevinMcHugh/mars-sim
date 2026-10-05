<script lang="ts">
  // Population: the colony's vital signs over the whole game, four small
  // charts on the simulation clock, from the population topic, as the TUI's
  // Population screen draws them (docs/population-screen.md). Four charts,
  // not four lines on one: their scales differ by orders of magnitude. The
  // fourth tracks a series picked from a list: all fixtures, one kind of
  // fixture, or the colonists at a skill title or better.
  import type uPlot from 'uplot';
  import { topics, ui } from '../../game.svelte';
  import Chart, { type Tip } from './Chart.svelte';
  import { axis, SERIES } from './theme';
  import { trackedSeries, type Population } from './population';

  const pop = $derived(topics.data.population as Population | undefined);

  const charts = [
    { key: 'colonists', title: 'Colonists', color: SERIES[0] },
    { key: 'meals', title: 'Meals in storage', color: SERIES[1] },
    { key: 'colonySize', title: 'Colony size (floor tiles)', color: SERIES[2] },
  ] as const;

  const tracked = $derived(pop ? trackedSeries(pop) : []);
  const pick = $derived(tracked.find((t) => t.key === ui.popTrack) ?? tracked[0]);
  const groups = $derived([...new Set(tracked.map((t) => t.group))]);
  const trackColor = SERIES[3];

  const x = $derived(pop ? Float64Array.from(pop.tick) : new Float64Array());

  const opts = (color: string, label: string): Omit<uPlot.Options, 'width' | 'height'> => ({
    scales: { x: { time: false }, y: { range: (_u, _lo, hi) => [0, hi > 0 ? Math.ceil(hi * 1.1) : 1] } },
    axes: [
      axis({ values: (_u, vals) => vals.map((v) => `t${Math.round(v).toLocaleString()}`) }),
      axis({ size: 44, space: 18, values: (_u, vals) => vals.map((v) => (Number.isInteger(v) ? v.toLocaleString() : '')) }),
    ],
    series: [{}, { label, stroke: color, width: 2, fill: color + '22', points: { show: false } }],
    cursor: { points: { size: 8, fill: color } },
    legend: { show: false },
  });
  const allOpts = charts.map((c) => opts(c.color, c.title));
  const trackOpts = opts(trackColor, 'Tracked');

  const tip = (c: (typeof charts)[number]) => (i: number): Tip | null => pop ? {
    title: `tick ${pop.tick[i].toLocaleString()}`,
    rows: [{ color: c.color, label: c.title, value: pop[c.key][i].toLocaleString() }],
  } : null;
  const trackTip = (i: number): Tip | null => pop && pick ? {
    title: `tick ${pop.tick[i].toLocaleString()}`,
    rows: [{ color: trackColor, label: pick.title, value: pick.values[i].toLocaleString() }],
  } : null;

  function range(v: number[]): string {
    if (!v.length) return '';
    let lo = v[0], hi = v[0];
    for (const n of v) { lo = Math.min(lo, n); hi = Math.max(hi, n); }
    return `now ${v[v.length - 1].toLocaleString()} · min ${lo.toLocaleString()} · max ${hi.toLocaleString()}`;
  }
</script>

{#if !pop}
  <p class="muted">Loading…</p>
{:else if pop.tick.length === 0}
  <p class="muted">No samples yet: the first comes at tick 50.</p>
{:else}
  {#each charts as c, i (c.key)}
    <h3>{c.title}</h3>
    <p class="stats">{range(pop[c.key])}</p>
    <Chart opts={allOpts[i]} data={[x, Float64Array.from(pop[c.key])]} height={130} tip={tip(c)} label="{c.title} over the game" />
  {/each}
  {#if pick}
    <h3>
      <select aria-label="Tracked series" value={pick.key} onchange={(e) => (ui.popTrack = e.currentTarget.value)}>
        {#each groups as g (g)}
          <optgroup label={g}>
            {#each tracked.filter((t) => t.group === g) as t (t.key)}
              <option value={t.key}>{t.title}</option>
            {/each}
          </optgroup>
        {/each}
      </select>
    </h3>
    <p class="stats">{range(pick.values)}</p>
    <Chart opts={trackOpts} data={[x, Float64Array.from(pick.values)]} height={130} tip={trackTip} label="{pick.title} over the game" />
  {/if}
{/if}

<style>
  h3 { font-size: 13px; margin: 12px 0 2px; color: #ffffff; }
  h3:first-child { margin-top: 0; }
  h3 select { font-size: 13px; font-weight: 600; }
  .stats { margin: 0 0 2px; font-size: 12px; color: #c3c2b7; font-variant-numeric: tabular-nums; }
  .muted { color: var(--muted); }
</style>

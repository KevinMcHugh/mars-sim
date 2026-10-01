<script lang="ts">
  // Activity: what the colonists spend their time on over the whole game, a
  // stacked area chart from the population topic's activity tallies, as the
  // TUI's Activity screen draws it (docs/activity-screen.md). Each band is
  // split: doing it, in its color, and walking there just above, hatched.
  // "Share" stacks to 100% of colonist time; "colonists" to the average
  // number of colonists.
  import type uPlot from 'uplot';
  import { topics } from '../../game.svelte';
  import Chart, { type Tip } from './Chart.svelte';
  import { columns, scale, slice, type Population } from './population';
  import { ACTIVITY_GROUPS, activityColor, axis, CHART_SURFACE, hatch } from './theme';

  const pop = $derived(topics.data.population as Population | undefined);
  let counts = $state(false);

  // The stack, bottom to top: each group's own time, then its walking time.
  const segments = ACTIVITY_GROUPS.flatMap((g) => [
    { group: g, walk: false },
    { group: g, walk: true },
  ]);

  /** How many columns the game is summed into. */
  const COLS = 48;
  const runs = $derived(pop ? columns(pop.tick.length, COLS) : []);

  // uPlot fills each series down to the axis, so the stack is drawn as
  // running totals, tallest first: each lower band paints over the one
  // above it, leaving only its own slice of the tall one showing.
  const data = $derived.by((): uPlot.AlignedData | null => {
    if (!pop || pop.tick.length === 0) return null;
    const n = runs.length;
    const idx = (name: string) => pop.activities.indexOf(name);
    const groups = ACTIVITY_GROUPS.map((g) => g.activities.map(idx).filter((a) => a >= 0));
    const cum = segments.map(() => new Float64Array(n));
    for (let i = 0; i < n; i++) {
      const s = slice(pop, runs[i][0], runs[i][1]);
      let run = 0;
      segments.forEach((seg, k) => {
        const ix = groups[ACTIVITY_GROUPS.indexOf(seg.group)];
        let v = 0;
        for (const a of ix) v += seg.walk ? s.walk[a] : s.own[a];
        run += scale(v, s, counts);
        cum[k][i] = run;
      });
    }
    return [Float64Array.from(runs, ([, hi]) => pop.tick[hi - 1]), ...cum.reverse()];
  });

  const opts = $derived.by((): Omit<uPlot.Options, 'width' | 'height'> => ({
    scales: {
      x: { time: false },
      y: counts ? { range: (_u, _lo, hi) => [0, Math.max(1, Math.ceil(hi))] } : { range: [0, 1] },
    },
    axes: [
      axis({ values: (_u, vals) => vals.map((v) => `t${Math.round(v).toLocaleString()}`) }),
      axis({ size: 40, values: (_u, vals) => vals.map((v) => (counts ? (Number.isInteger(v) ? String(v) : '') : `${Math.round(v * 100)}%`)) }),
    ],
    series: [
      {},
      // Tallest first (see data). A hairline in the surface color between
      // bands keeps neighbors apart.
      ...[...segments].reverse().map((seg) => ({
        label: seg.group.label + (seg.walk ? ' (walking)' : ''),
        stroke: CHART_SURFACE + "99", // a soft seam: full strength reads as noise this dense
        width: 1,
        fill: seg.walk ? () => hatch(seg.group.color) : seg.group.color,
        points: { show: false },
        paths: undefined,
      })),
    ],
    cursor: { points: { show: false } },
    legend: { show: false },
  }));

  const pct = (v: number) => `${(v * 100).toFixed(v < 0.1 ? 1 : 0)}%`;
  const fmt = (v: number) => (counts ? v.toFixed(1) : pct(v));

  // The tooltip names every activity, grouped or not, top of the stack first.
  const tip = (i: number): Tip | null => {
    if (!pop || !runs[i]) return null;
    const [lo, hi] = runs[i];
    const s = slice(pop, lo, hi);
    const rows: Tip['rows'] = [];
    for (const g of [...ACTIVITY_GROUPS].reverse()) {
      for (const name of g.activities) {
        const a = pop.activities.indexOf(name);
        if (a < 0 || s.own[a] + s.walk[a] === 0) continue;
        rows.push({ color: g.color, label: name, value: fmt(scale(s.own[a], s, counts)) });
        if (s.walk[a] > 0) rows.push({ color: g.color, fill: 'hatch', label: `  walking there`, value: fmt(scale(s.walk[a], s, counts)) });
      }
    }
    const from = lo > 0 ? pop.tick[lo - 1] : 0;
    return { title: `ticks ${from.toLocaleString()}–${pop.tick[hi - 1].toLocaleString()}`, rows };
  };

  // The legend: each activity's share lately (the last tenth of the game)
  // and over the whole game, as the TUI's legend does.
  const legend = $derived.by(() => {
    if (!pop || pop.tick.length === 0) return [];
    const n = pop.tick.length;
    const recent = slice(pop, Math.max(0, n - Math.max(1, Math.round(n / 10))), n);
    const all = slice(pop, 0, n);
    return [...ACTIVITY_GROUPS].reverse().flatMap((g) => g.activities.map((name) => {
      const a = pop.activities.indexOf(name);
      return {
        name, color: activityColor(name),
        recent: a < 0 ? 0 : scale(recent.own[a] + recent.walk[a], recent, counts),
        all: a < 0 ? 0 : scale(all.own[a] + all.walk[a], all, counts),
      };
    }));
  });
</script>

<div class="mode" role="group" aria-label="Units">
  <button type="button" class:on={!counts} aria-pressed={!counts} onclick={() => (counts = false)}>Share of time</button>
  <button type="button" class:on={counts} aria-pressed={counts} onclick={() => (counts = true)}>Colonists</button>
</div>

{#if !pop}
  <p class="muted">Loading…</p>
{:else if !data}
  <p class="muted">No samples yet: the first comes at tick 50.</p>
{:else}
  {#key counts}
    <Chart {opts} {data} height={240} {tip} label="Colonist activity over the game, stacked" />
  {/key}
  <p class="note">Hatched: walking there.</p>
  <table>
    <thead><tr><th></th><th>activity</th><th class="num">lately</th><th class="num">all game</th></tr></thead>
    <tbody>
      {#each legend as r (r.name)}
        <tr>
          <td><span class="key" style="background: {r.color}"></span></td>
          <td>{r.name}</td>
          <td class="num">{fmt(r.recent)}</td>
          <td class="num">{fmt(r.all)}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

<style>
  .mode { display: flex; gap: 4px; margin-bottom: 8px; }
  .mode button { font-size: 12px; padding: 2px 10px; }
  .mode button.on { background: var(--accent); color: #fff; border-color: transparent; }
  .muted { color: var(--muted); }
  .note { margin: 4px 0 8px; font-size: 12px; color: var(--muted); }
  table { width: 100%; border-collapse: collapse; font-size: 12px; }
  th { text-align: left; color: var(--muted); font-weight: normal; padding: 2px 4px; }
  td { padding: 2px 4px; color: #c3c2b7; border-top: 1px solid var(--line); }
  .num { text-align: right; font-variant-numeric: tabular-nums; color: #ffffff; }
  th.num { color: var(--muted); }
  .key { display: inline-block; width: 9px; height: 9px; border-radius: 2px; }
</style>

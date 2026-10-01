<script lang="ts">
  // The Charts tab: the TUI's three chart screens, Perf, Population and
  // Activity, one at a time. Each view subscribes to its own topic, so a
  // closed tab, or another view, costs the worker nothing.
  import { subscribe, ui } from '../game.svelte';
  import ActivityChart from './charts/ActivityChart.svelte';
  import PerfCharts from './charts/PerfCharts.svelte';
  import PopulationCharts from './charts/PopulationCharts.svelte';

  const views = [
    { id: 'perf', label: 'Perf' },
    { id: 'population', label: 'Population' },
    { id: 'activity', label: 'Activity' },
  ] as const;

  // Population and Activity draw from the same topic.
  $effect(() => {
    if (ui.chartView !== 'perf') return subscribe('population');
  });
</script>

<div class="views" role="tablist" aria-label="Charts">
  {#each views as v (v.id)}
    <button type="button" role="tab" class:on={ui.chartView === v.id} aria-selected={ui.chartView === v.id}
      onclick={() => (ui.chartView = v.id)}>{v.label}</button>
  {/each}
</div>

<div class="surface">
  {#if ui.chartView === 'perf'}
    <PerfCharts />
  {:else if ui.chartView === 'population'}
    <PopulationCharts />
  {:else}
    <ActivityChart />
  {/if}
</div>

<style>
  .views { display: flex; gap: 4px; margin-bottom: 10px; }
  .views button { flex: 1; border-color: var(--line); background: transparent; }
  .views button.on { background: rgba(255, 255, 255, 0.12); border-color: rgba(255, 255, 255, 0.3); }
  /* The charts' own surface (charts/theme.ts), which their colors were
     validated against: opaque, so the map behind the panel cannot shift it. */
  .surface { background: #1a1a19; border-radius: 6px; padding: 10px; margin: 0 -6px; }
</style>

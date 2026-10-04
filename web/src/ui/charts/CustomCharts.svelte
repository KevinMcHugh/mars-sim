<script lang="ts">
  // Custom: the charts the player makes from anything the sim measures (the
  // metrics topic's catalog; docs/charts.md). They are remembered in this
  // browser, not in the save: a chart is a way of looking, and the same
  // charts should greet every game.
  import { saveCharts, subscribe, topics, ui } from '../../game.svelte';
  import { newChartId, STARTER_CHARTS, type Catalog } from './builder';
  import CustomChart from './CustomChart.svelte';

  $effect(() => subscribe('metrics'));
  const cat = $derived(topics.data.metrics as Catalog | undefined);

  function addChart(): void {
    ui.charts.push({ id: newChartId(), title: 'New chart', series: [], bucket: 0 });
    saveCharts();
  }
  function remove(id: string): void {
    ui.charts = ui.charts.filter((c) => c.id !== id);
    saveCharts();
  }
  function reset(): void {
    ui.charts = structuredClone(STARTER_CHARTS);
    saveCharts();
  }
</script>

{#if !cat}<p class="muted">Loading…</p>{/if}
{#each ui.charts as spec (spec.id)}
  <CustomChart {spec} {cat} onremove={() => remove(spec.id)} />
{/each}
<div class="foot">
  <button type="button" onclick={addChart}>+ New chart</button>
  <button type="button" class="quiet" onclick={reset}>Reset to the starter charts</button>
</div>

<style>
  .muted { color: var(--muted); }
  .foot { display: flex; gap: 6px; justify-content: space-between; }
  .quiet { background: transparent; border-color: transparent; color: #898781; }
</style>

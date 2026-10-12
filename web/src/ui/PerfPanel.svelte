<script lang="ts">
  // The Perf tab: the engine's tick rate and tick cost (PerfCharts,
  // docs/perf-screen.md). It used to be a view of the Charts tab; it is a tab
  // of its own so it can be popped out and watched beside the colony's charts.
  import PerfCharts from './charts/PerfCharts.svelte';
</script>

<div class="surface"><PerfCharts /></div>

<style>
  /* The charts' own opaque surface, as in the Charts tab (charts/theme.ts). */
  .surface { background: #1a1a19; border-radius: 6px; padding: 10px; margin: 0 -6px; }
</style>

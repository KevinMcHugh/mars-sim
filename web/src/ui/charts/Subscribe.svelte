<script lang="ts">
  // Holds one topic subscription while it is mounted: one per series in an
  // {#each} keyed by series, so adding a series to a chart subscribes to it
  // alone, rather than restarting every topic the chart reads.
  import { subscribe } from '../../game.svelte';

  let { topic }: { topic: string } = $props();
  $effect(() => subscribe(topic));
</script>

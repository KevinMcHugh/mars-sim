<script lang="ts">
  // The page's chrome around the map: the top bar, the side panel, the hover
  // readout and the status line. The map itself is a canvas main.ts draws.
  import { ui } from '../game.svelte';
  import TopBar from './TopBar.svelte';
  import SidePanel from './SidePanel.svelte';
  import LogTicker from './LogTicker.svelte';
  import FlowControl from './FlowControl.svelte';
</script>

<TopBar />
<SidePanel />
<LogTicker />
<FlowControl legend />

{#if ui.hover}
  <div class="hud hover">{ui.hover}</div>
{/if}

{#if ui.status}
  <div class="status" class:error={ui.statusError}>{ui.status}</div>
{/if}

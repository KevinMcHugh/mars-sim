<script lang="ts">
  // The level the map shows (docs/z-levels.md): a picker in the top bar, with
  // a step up and a step down. It lists the levels the colony has broken
  // into, and shows only once the world lets the colony dig below the
  // landing level. < and > step through them too (main.ts).
  import { levelName, setLevel, stepLevel, ui } from '../game.svelte';

  const shown = $derived(!!ui.hello && ui.hello.deepestLevel > ui.hello.landingLevel);
  const i = $derived(ui.levels.indexOf(ui.level));
</script>

{#if shown}
  <span class="pick" title="The level the map shows: step down a stair, shaft or hole to see the levels the colony has broken into (< and >)">
    <button type="button" aria-label="Up a level" title="Up a level (<)" disabled={i <= 0} onclick={() => stepLevel(-1)}>▲</button>
    <select aria-label="Level" value={ui.level} onchange={(e) => setLevel(Number(e.currentTarget.value))}>
      {#each ui.levels as l (l)}
        <option value={l}>{levelName(l)}</option>
      {/each}
    </select>
    <button type="button" aria-label="Down a level" title="Down a level (>)" disabled={i < 0 || i >= ui.levels.length - 1} onclick={() => stepLevel(1)}>▼</button>
  </span>
{/if}

<style>
  .pick { display: flex; align-items: center; gap: 4px; }
  .pick select { padding: 3px 6px; }
  .pick button { width: 26px; height: 26px; padding: 0; line-height: 1; font-size: 11px; }
</style>

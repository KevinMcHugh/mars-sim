<script lang="ts">
  // The map's log ticker: the last few colony-log lines, over the map's
  // bottom-left corner, fading once they are TICKER_MS old, as the TUI's map
  // sidebar keeps the log's tail in view. Clicking it opens the Log tab; it
  // hides while that tab is open, which already shows the tail.
  import { colonyLog, setPanel, ui } from '../game.svelte';
  import { logKindColor } from './logkinds';

  const TICKER_LINES = 4;
  const TICKER_MS = 12_000;

  // A clock for the fade, ticking only until the newest line has faded:
  // an idle page with a quiet colony runs no timer.
  let now = $state(performance.now());
  $effect(() => {
    const newest = colonyLog.lines.at(-1);
    if (!newest) return;
    now = performance.now();
    const t = setInterval(() => {
      now = performance.now();
      if (now - newest.at > TICKER_MS) clearInterval(t);
    }, 500);
    return () => clearInterval(t);
  });

  const shown = $derived(
    ui.panel === 'log' ? [] : colonyLog.lines.slice(-TICKER_LINES).filter((l) => now - l.at < TICKER_MS),
  );
</script>

{#if shown.length > 0}
  <button type="button" class="ticker" title="Open the colony log" onclick={() => setPanel('log')}>
    {#each shown as l (l.seq)}
      <span class="line" style="opacity: {Math.min(1, (TICKER_MS - (now - l.at)) / 3000)}">
        <span class="dot" style="background: {logKindColor(l.kind)}"></span>{l.text}
      </span>
    {/each}
  </button>
{/if}

<style>
  .ticker {
    position: fixed; left: 10px; bottom: calc(max(10px, env(safe-area-inset-bottom)) + 42px);
    max-width: min(520px, calc(100vw - var(--panel-width) - 40px));
    display: flex; flex-direction: column; align-items: flex-start; gap: 3px;
    background: none; border: none; padding: 0; text-align: left; cursor: pointer;
  }
  .line {
    background: var(--panel); border: 1px solid var(--line); border-radius: 6px;
    padding: 3px 8px; backdrop-filter: blur(6px); line-height: 1.35;
    transition: opacity 0.5s linear;
  }
  .dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; margin-right: 7px; vertical-align: 1px; }
  @media (max-width: 700px) {
    .ticker { max-width: calc(100vw - 20px); }
  }
</style>

<script lang="ts">
  // The map's log ticker: the last few colony-log lines, over the map's
  // bottom-left corner, fading once they are TICKER_MS old, as the TUI's map
  // sidebar keeps the log's tail in view. Clicking it opens the Log tab; it
  // hides while that tab is open, which already shows the tail. × hides it
  // for good (remembered in this browser); the Log tab turns it back on. On a
  // phone it shows fewer, smaller lines, since there the map is small.
  //
  // Two are mounted: a floating one (App.svelte) for wide screens, and a
  // docked one inside the side panel (SidePanel.svelte) for phones, where it
  // sits just above the tab strip instead of on top of it. CSS shows one.
  import { colonyLog, setPanel, setTicker, ui } from '../game.svelte';

  let { docked = false }: { docked?: boolean } = $props();
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
    // Docked (a phone), an open tab's sheet covers the map: nothing to add.
    ui.panel === 'log' || !ui.ticker || (docked && ui.panel !== null) ? [] : colonyLog.lines.slice(-TICKER_LINES).filter((l) => now - l.at < TICKER_MS),
  );
</script>

{#if shown.length > 0}
  <div class="ticker" class:docked>
    <button type="button" class="close" title="Hide these (the Log tab brings them back)" aria-label="Hide the log ticker"
      onclick={() => setTicker(false)}>×</button>
    <button type="button" class="lines" title="Open the colony log" onclick={() => setPanel('log')}>
      {#each shown as l, i (l.seq)}
        <span class="line" class:older={i < shown.length - 2} style="opacity: {Math.min(1, (TICKER_MS - (now - l.at)) / 3000)}">
          <span class="dot" style="background: {logKindColor(l.kind)}"></span>{l.text}
        </span>
      {/each}
    </button>
  </div>
{/if}

<style>
  .ticker {
    position: fixed; left: 10px; bottom: calc(max(10px, env(safe-area-inset-bottom)) + 42px);
    max-width: min(520px, calc(100vw - var(--panel-width) - 40px));
    display: flex; align-items: flex-end; gap: 4px;
  }
  .lines {
    display: flex; flex-direction: column; align-items: flex-start; gap: 3px; min-width: 0;
    background: none; border: none; padding: 0; text-align: left; cursor: pointer;
  }
  .close {
    order: 2; flex: none; width: 28px; height: 28px; padding: 0; border-radius: 14px;
    background: var(--panel); border: 1px solid var(--line); color: var(--muted);
    font-size: 16px; line-height: 1; backdrop-filter: blur(6px);
  }
  .close:hover { color: var(--fg); }
  .line {
    background: var(--panel); border: 1px solid var(--line); border-radius: 6px;
    padding: 3px 8px; backdrop-filter: blur(6px); line-height: 1.35;
    transition: opacity 0.5s linear;
  }
  .dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; margin-right: 7px; vertical-align: 1px; }
  .ticker.docked { display: none; position: static; max-width: none; pointer-events: auto; }
  /* A phone: docked above the tabs, the two newest lines, smaller, each one
     line long. */
  @media (max-width: 700px) {
    .ticker:not(.docked) { display: none; }
    .ticker.docked { display: flex; }
    .line { font-size: 12px; padding: 2px 7px; max-width: calc(100vw - 60px); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .line.older { display: none; }
  }
</style>

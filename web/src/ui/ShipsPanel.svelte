<script lang="ts">
  // The Ships tab: where the colony ships came down. A new game starts paused
  // with this tab open, so the player can pick each ship up and land it
  // somewhere else before anyone moves; it obliterates whatever it lands on.
  // Once the game is running the ships stay put and this is just a list.
  // See docs/ships.md.
  import { armShip, centerOn, setSpeed, subscribe, topics, ui } from '../game.svelte';
  import type { ShipsTopic } from '../game.svelte';

  $effect(() => subscribe('ships'));
  // Leaving the tab puts the tool down and clears its preview.
  $effect(() => () => armShip(null));

  const t = $derived(topics.data.ships as ShipsTopic | undefined);

  // The game started while a ship was held: put it down.
  $effect(() => { if (t && !t.placing && ui.shipTool !== null) armShip(null); });

  function hold(id: number) { armShip(ui.shipTool === id ? null : id); }
</script>

<h2>Ships</h2>
{#if !t}
  <p class="muted">Loading…</p>
{:else}
  {#if t.placing}
    <p class="muted">
      The colony is still aboard. Pick a ship up and click the map to land it there: it smashes
      through rock and crushes anything under it, but needs a tile of clear ground from every other
      ship. Pan by dragging as usual.
    </p>
  {:else}
    <p class="muted">The ships have landed. Their bunks and toilets are shared; each settler has a locker of their own.</p>
  {/if}
  <ul class="lines">
    {#each t.ships as s (s.id)}
      <li>
        <button type="button" class="link" onclick={() => centerOn(s.x + (s.w >> 1), s.y + (s.h >> 1))}>
          Ship {s.id}
        </button>
        {s.colonists} aboard <span class="muted">at {s.x},{s.y}</span>
        {#if t.placing}
          <button type="button" class="move" class:on={ui.shipTool === s.id} aria-pressed={ui.shipTool === s.id}
            onclick={() => hold(s.id)}>{ui.shipTool === s.id ? 'Click the map…' : 'Move'}</button>
        {/if}
      </li>
    {/each}
  </ul>
  {#if t.placing}
    <div class="row">
      <button type="button" class="go" onclick={() => { armShip(null); setSpeed(1); }}>Land and start</button>
    </div>
  {/if}
{/if}

<style>
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 4px 0 6px; }
  p { margin: 0 0 8px; }
  .muted { color: var(--muted); }
  ul { list-style: none; padding: 0; margin: 0 0 8px; }
  .lines li { line-height: 1.8; font-size: 13px; }
  .move { margin-left: 6px; padding: 0 8px; font-size: 12px; }
  .move.on, .row button.go { background: var(--accent); color: #fff; }
  .row { display: flex; gap: 6px; margin: 8px 0; }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; }
  .link:hover { text-decoration: underline; }
</style>

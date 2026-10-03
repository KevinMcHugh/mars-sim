<script lang="ts">
  // The Ships tab: the colony ships, landing and landed. A new game starts
  // paused with the founders' ships still aloft and this tab open: the player
  // lands them one after another, each where they click, and may then pick a
  // landed ship up and land it somewhere else before anyone moves. A ship
  // obliterates whatever it lands on. Once the game is running the ships
  // stay put and this is just a list. See docs/ships.md.
  import { armShip, centerOn, nextAloft, setSpeed, subscribe, topics, ui } from '../game.svelte';
  import type { ShipsTopic } from '../game.svelte';

  $effect(() => subscribe('ships'));
  // Leaving the tab puts the tool down and clears its preview.
  $effect(() => () => armShip(null));

  const t = $derived(topics.data.ships as ShipsTopic | undefined);
  const next = $derived(t?.placing ? nextAloft(t) : undefined);
  const aloft = $derived(t ? t.ships.filter((s) => s.aloft).length : 0);

  // The game started while a ship was held: put it down.
  $effect(() => { if (t && !t.placing && ui.shipTool !== null) armShip(null); });
  // With the tool down, hand the player the next ship waiting to land — but
  // not the one just sent down, which the topic has yet to show landed.
  $effect(() => { if (next && ui.shipTool === null && ui.shipSent !== next.id) armShip(next.id); });

  function hold(id: number) { armShip(ui.shipTool === id ? null : id); }
</script>

{#if !t}
  <p class="muted">Loading…</p>
{:else}
  {#if next}
    <p class="muted">
      {aloft} {aloft === 1 ? 'ship is' : 'ships are'} still in orbit. Click the map to land ship {next.id}
      there: it smashes through rock and crushes anything under it, but needs a tile of clear ground
      from every other ship. Pan by dragging as usual.
    </p>
    {#if next.shape}
      <p class="muted">Ship {next.id}: a {next.kind}, {next.colonists} aboard.</p>
      <pre class="shape" aria-label="the shape of ship {next.id}">{next.shape.join('\n')}</pre>
    {/if}
  {:else if t.placing}
    <p class="muted">
      Every ship is down. Pick one up to land it somewhere else, or start when you are happy.
    </p>
  {:else}
    <p class="muted">The ships have landed. Their bunks and toilets are shared; each settler has a locker of their own.</p>
  {/if}
  <ul class="lines">
    {#each t.ships as s (s.id)}
      <li>
        {#if s.aloft}
          Ship {s.id}: {s.colonists} aboard <span class="muted">{s.id === next?.id ? 'landing next' : 'in orbit'}</span>
          {#if s.id === next?.id && ui.shipTool !== s.id}
            <button type="button" class="move" onclick={() => hold(s.id)}>Land</button>
          {/if}
        {:else}
          <button type="button" class="link" onclick={() => centerOn(s.x + (s.w >> 1), s.y + (s.h >> 1))}>
            Ship {s.id}
          </button>:
          {s.colonists} aboard <span class="muted">{s.kind ?? ''} at {s.x},{s.y}</span>
          {#if t.placing && !next}
            <button type="button" class="move" class:on={ui.shipTool === s.id} aria-pressed={ui.shipTool === s.id}
              onclick={() => hold(s.id)}>{ui.shipTool === s.id ? 'Click the map…' : 'Move'}</button>
          {/if}
        {/if}
      </li>
    {/each}
  </ul>
  {#if t.placing}
    <div class="row">
      <button type="button" class="go" disabled={aloft > 0} onclick={() => { armShip(null); setSpeed(1); }}>
        {aloft > 0 ? `Land ${aloft} more to start` : 'Start'}
      </button>
    </div>
  {/if}
{/if}

<style>
  p { margin: 0 0 8px; }
  .muted { color: var(--muted); }
  ul { list-style: none; padding: 0; margin: 0 0 8px; }
  .lines li { line-height: 1.8; font-size: 13px; }
  .move { margin-left: 6px; padding: 0 8px; font-size: 12px; }
  .move.on, .row button.go { background: var(--accent); color: #fff; }
  .row button.go:disabled { opacity: 0.5; cursor: default; }
  .row { display: flex; gap: 6px; margin: 8px 0; }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; }
  .link:hover { text-decoration: underline; }
  .shape { font: 9px/9px monospace; letter-spacing: 1px; margin: 0 0 8px; color: var(--muted); }
</style>

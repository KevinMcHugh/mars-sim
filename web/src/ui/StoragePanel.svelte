<script lang="ts">
  // The Storage tab: every container (the storage topic,
  // internal/wire/boards.go), as the TUI's storage tab lists them. A row
  // opens its tile in the inspector, which has the contents and the ledger.
  import { inspect, subscribe, topics, ui } from '../game.svelte';

  interface Row { x: number; y: number; label: string; used: number; slots: number; items: number; capacity: number; top?: string }

  $effect(() => subscribe('storage'));
  const rows = $derived(topics.data.storage as Row[] | undefined);
  const sel = $derived(ui.selected && 'tile' in ui.selected ? ui.selected.tile : null);

  const totals = $derived(rows?.reduce((a, r) => ({ used: a.used + r.used, slots: a.slots + r.slots }), { used: 0, slots: 0 }));
</script>

{#if !rows}
  <p class="muted">Loading…</p>
{:else}
  <h2>Storage ({rows.length})</h2>
  {#if rows.length === 0}
    <p class="muted">No storage containers have been built.</p>
  {:else if totals}
    <p class="muted">{totals.used}/{totals.slots} slots used across the colony.</p>
  {/if}
  <ul>
    {#each rows as r (r.x + ',' + r.y)}
      <li>
        <button type="button" class:on={sel !== null && sel[0] === r.x && sel[1] === r.y}
          onclick={() => inspect({ tile: [r.x, r.y] }, 'storage')}>
          <span class="name">{r.label}</span>
          <span class="where">({r.x}, {r.y})</span>
          <span class="bar"><span style="width: {r.slots ? (r.used * 100) / r.slots : 0}%"></span></span>
          <span class="sub">{r.used}/{r.slots} slots{r.top ? ` · mostly ${r.top}` : ' · empty'}</span>
        </button>
      </li>
    {/each}
  </ul>
{/if}

<style>
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 4px 0 8px; }
  p { margin: 0 0 8px; }
  .muted { color: var(--muted); }
  ul { list-style: none; padding: 0; margin: 0; display: grid; gap: 2px; }
  li button {
    width: 100%; display: grid; grid-template-columns: 1fr auto; gap: 3px 8px; text-align: left;
    border-color: transparent; background: transparent; padding: 6px 8px;
  }
  li button:hover { background: rgba(255, 255, 255, 0.05); }
  li button.on { background: rgba(224, 112, 58, 0.22); border-color: var(--accent); }
  .name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .where { color: var(--muted); font-variant-numeric: tabular-nums; }
  .bar { grid-column: 1 / -1; height: 5px; border-radius: 3px; background: rgba(255, 255, 255, 0.08); overflow: hidden; }
  .bar span { display: block; height: 100%; background: #b8894a; }
  .sub { grid-column: 1 / -1; color: var(--muted); font-size: 12px; }
</style>

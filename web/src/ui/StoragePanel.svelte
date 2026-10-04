<script lang="ts" module>
  // The search outlives the panel, so a row opened in the inspector comes
  // back (← Storage) to the same results.
  const query = $state({ item: '', owner: '' });
</script>

<script lang="ts">
  // The Storage tab: the colony's pooled stock, a search by item and by
  // owner, and every container (the storage topic, internal/wire/boards.go),
  // as the TUI's storage tab lists them. A container opens its tile in the
  // inspector, which has the contents and the ledger.
  import { inspect, subscribe, topics, ui } from '../game.svelte';
  import Section from './Section.svelte';
  import { describe, items, owners, pool, search, type StorageRow } from './storage';

  $effect(() => subscribe('storage'));
  const rows = $derived(topics.data.storage as StorageRow[] | undefined);
  const sel = $derived(ui.selected && 'tile' in ui.selected ? ui.selected.tile : null);

  const p = $derived(rows && pool(rows));
  const found = $derived(rows && search(rows, query));
  const itemNames = $derived(rows ? items(rows) : []);
  const ownerNames = $derived(rows ? owners(rows) : []);

  const at = (r: StorageRow) => sel !== null && sel[0] === r.x && sel[1] === r.y;
  const open = (r: StorageRow) => inspect({ tile: [r.x, r.y] }, 'storage');
</script>

{#if !rows || !p}
  <p class="muted">Loading…</p>
{:else if rows.length === 0}
  <h2>Storage</h2>
  <p class="muted">No storage containers have been built.</p>
{:else}
  <Section id="storage.pool" title="Pool" tag="h2" note="({p.containers} containers)">
    <p class="muted">{p.used}/{p.slots} slots · {p.items}/{p.capacity} items</p>
    <span class="bar"><span style="width: {p.slots ? (p.used * 100) / p.slots : 0}%"></span></span>
    {#if p.totals.length === 0}
      <p class="muted">Every container is empty.</p>
    {:else}
      <ul class="totals">
        {#each p.totals as h (h.item)}
          <li>
            <button type="button" class="link" title="Search for {h.item}" onclick={() => (query.item = h.item)}>{h.item}</button>
            <span class="n">{h.count}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </Section>

  <Section id="storage.search" title="Search" tag="h2">
    <div class="fields">
      <label>Item <input type="search" list="storage-items" bind:value={query.item} placeholder="any item" /></label>
      <label>Owner <input type="search" list="storage-owners" bind:value={query.owner} placeholder="anyone" /></label>
      <datalist id="storage-items">{#each itemNames as n (n)}<option value={n}></option>{/each}</datalist>
      <datalist id="storage-owners">{#each ownerNames as n (n)}<option value={n}></option>{/each}</datalist>
    </div>
    {#if found}
      {#if found.hits.length === 0}
        <p class="muted">Nothing in storage matches.</p>
      {:else}
        <p class="muted">
          {found.total} in {found.hits.length} {found.hits.length === 1 ? 'container' : 'containers'}{#if found.totals.length > 1}: {found.totals.map((h) => `${h.item} ×${h.count}`).join(', ')}{/if}
        </p>
        <ul>
          {#each found.hits as h (h.row.x + ',' + h.row.y)}
            <li>
              <button type="button" class="row" class:on={at(h.row)} onclick={() => open(h.row)}>
                <span class="name">{h.row.label}</span>
                <span class="where">({h.row.x}, {h.row.y})</span>
                <span class="sub">{describe(h.lines)}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    {/if}
  </Section>

  <Section id="storage.containers" title="Containers" tag="h2" note="({rows.length})">
    <ul>
      {#each rows as r (r.x + ',' + r.y)}
        <li>
          <button type="button" class="row" class:on={at(r)} onclick={() => open(r)}>
            <span class="name">{r.label}</span>
            <span class="where">({r.x}, {r.y})</span>
            <span class="bar"><span style="width: {r.slots ? (r.used * 100) / r.slots : 0}%"></span></span>
            <span class="sub">{r.used}/{r.slots} slots{r.top ? ` · mostly ${r.top}` : ' · empty'}</span>
          </button>
        </li>
      {/each}
    </ul>
  </Section>
{/if}

<style>
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 4px 0 8px; }
  p { margin: 0 0 8px; }
  .muted { color: var(--muted); }
  ul { list-style: none; padding: 0; margin: 0; display: grid; gap: 2px; }
  .row {
    width: 100%; display: grid; grid-template-columns: 1fr auto; gap: 3px 8px; text-align: left;
    border-color: transparent; background: transparent; padding: 6px 8px;
  }
  .row:hover { background: rgba(255, 255, 255, 0.05); }
  .row.on { background: rgba(224, 112, 58, 0.22); border-color: var(--accent); }
  .name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .where { color: var(--muted); font-variant-numeric: tabular-nums; }
  .bar { display: block; grid-column: 1 / -1; height: 5px; border-radius: 3px; background: rgba(255, 255, 255, 0.08); overflow: hidden; }
  .bar span { display: block; height: 100%; background: #b8894a; }
  .sub { grid-column: 1 / -1; color: var(--muted); font-size: 12px; }

  .totals { margin-top: 8px; gap: 0; }
  .totals li { display: flex; justify-content: space-between; gap: 8px; padding: 1px 8px; font-size: 13px; }
  .n { font-variant-numeric: tabular-nums; }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; text-align: left; }
  .link:hover { text-decoration: underline; }

  .fields { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; margin-bottom: 8px; }
  label { display: grid; gap: 2px; font-size: 12px; color: var(--muted); }
  input { width: 100%; min-width: 0; box-sizing: border-box; }
</style>

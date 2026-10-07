<script lang="ts">
  // The open orders in one (item, depot) book, opened in place under its row
  // in the Market tab's Books: bids dearest first and asks cheapest first,
  // the order they match in, from the book:<x>,<y>:<item> topic
  // (internal/wire/orders.go). An order opens its detail on a click.
  import { subscribe, topics } from '../game.svelte';
  import OrderDetail from './OrderDetail.svelte';
  import { money } from './format';

  let { item, x, y }: { item: string; x: number; y: number } = $props();

  interface Row { id: number; owner: string; ownerKey: string; qty: number; price: number; filled: number }
  interface Book { item: string; depot: string; bids: Row[]; asks: Row[] }

  const topic = $derived(`book:${x},${y}:${item}`);
  $effect(() => subscribe(topic));
  const b = $derived(topics.data[topic] as Book | undefined);
  let openId: number | null = $state(null);
</script>

<div class="book">
  {#if !b}
    <p class="muted">Loading…</p>
  {:else}
    {#each [['Bids', b.bids], ['Asks', b.asks]] as const as [title, rows] (title)}
      <h4>{title}</h4>
      {#if rows.length === 0}
        <p class="muted">none open</p>
      {:else}
        <ul>
          {#each rows as r (r.id)}
            <li>
              <button type="button" class="row" class:on={r.id === openId} aria-expanded={r.id === openId}
                onclick={() => (openId = openId === r.id ? null : r.id)}>
                <span class="who">{r.owner}</span>
                <span class="num">{r.qty} @ {money(r.price)}{#if r.filled > 0}<span class="muted filled">({r.filled} filled)</span>{/if}</span>
              </button>
              {#if r.id === openId}<OrderDetail id={r.id} />{/if}
            </li>
          {/each}
        </ul>
      {/if}
    {/each}
  {/if}
</div>

<style>
  .book { padding: 2px 0 6px 8px; }
  p { margin: 0; font-size: 12px; }
  .muted { color: var(--muted); }
  h4 { font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 6px 0 2px; font-weight: normal; }
  ul { list-style: none; padding: 0; margin: 0; }
  .row { width: 100%; display: flex; justify-content: space-between; gap: 8px; border-color: transparent; background: transparent; padding: 1px 6px; font-size: 12px; }
  .row:hover, .row.on { background: rgba(255, 255, 255, 0.05); }
  .who { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: left; }
  .filled { margin-left: 0.4em; }
  .num { font-variant-numeric: tabular-nums; white-space: nowrap; }
</style>

<script lang="ts">
  // One market account's page, opened in place in the Market tab: balance,
  // share of the money, holdings in storage, fixtures, open orders and plans,
  // from the account:<key> topic (internal/wire/boards.go). An open order
  // opens its detail (OrderDetail) on a click.
  import { inspect, subscribe, topics } from '../game.svelte';
  import { money } from './format';
  import OrderDetail from './OrderDetail.svelte';

  let { key }: { key: string } = $props();

  interface Account {
    found: boolean; label: string; balance: number; sharePct: number;
    holdings: { item: string; count: number }[]; fixtures: number;
    orders: { id: number; side: string; qty: number; item: string; price: number; x: number; y: number }[];
    plans: string[];
  }

  const topic = $derived(`account:${key}`);
  $effect(() => subscribe(topic));
  const a = $derived(topics.data[topic] as Account | undefined);
  let openId: number | null = $state(null);
</script>

<div class="detail">
  {#if !a}
    <p class="muted">Loading…</p>
  {:else if !a.found}
    <p class="muted">This account is closed.</p>
  {:else}
    <p>{[money(a.balance), `${a.sharePct}% of circulating money`, a.fixtures > 0 && `${a.fixtures} fixtures owned`].filter(Boolean).join(' · ')}</p>
    {#if key !== 'colony'}
      <button type="button" class="link" onclick={() => inspect({ entity: Number(key) }, 'market')}>Inspect {a.label}</button>
    {/if}
    <h3>Holdings in storage</h3>
    {#if a.holdings.length === 0}<p class="muted">nothing</p>{/if}
    <p>{a.holdings.map((h) => `${h.item} ×${h.count}`).join(', ')}</p>
    {#if a.orders.length > 0}
      <h3>Open orders</h3>
      <ul>
        {#each a.orders as o (o.id)}
          <li>
            <button type="button" class="link" aria-expanded={o.id === openId} title="Show this order's detail"
              onclick={() => (openId = openId === o.id ? null : o.id)}>{o.side} {o.qty} {o.item} @ {money(o.price)}</button>
            <span class="muted">({o.x},{o.y})</span>
            {#if o.id === openId}<OrderDetail id={o.id} />{/if}
          </li>
        {/each}
      </ul>
    {/if}
    {#if a.plans.length > 0}
      <h3>Plan</h3>
      <ul>{#each a.plans as p, i (i)}<li>{p}</li>{/each}</ul>
    {/if}
  {/if}
</div>

<style>
  .detail { padding: 2px 8px 8px; font-size: 13px; }
  h3 { font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 8px 0 2px; font-weight: normal; }
  p { margin: 0; line-height: 1.5; }
  .muted { color: var(--muted); }
  ul { list-style: none; padding: 0; margin: 0; }
  li { line-height: 1.5; }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; }
  .link:hover { text-decoration: underline; }
</style>

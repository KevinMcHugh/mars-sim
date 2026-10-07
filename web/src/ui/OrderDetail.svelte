<script lang="ts">
  // One open order's detail, opened in place under the row that lists it (a
  // book, an account's open orders, the colony's desk): who posted it, its
  // terms, what has filled and with whom, what it still holds, and when it
  // opened on the colony clock, from the order:<id> topic
  // (internal/wire/orders.go, docs/order-detail.md).
  import { centerOn, inspect, subscribe, topics } from '../game.svelte';
  import { dayClock, money, span } from './format';

  let { id }: { id: number } = $props();

  interface Order {
    found: boolean; side: 'bid' | 'ask'; item: string; owner: string; ownerKey: string; manual: boolean;
    price: number; qty: number; filled: number; open: number; total: number; escrow: number;
    x: number; y: number; depot: string;
    postedDay: number; postedMinute: number; openMinutes: number; expiresMinutes?: number;
    fills: { key: string; label: string; qty: number; total: number }[]; paid: number;
  }

  const topic = $derived(`order:${id}`);
  $effect(() => subscribe(topic));
  const o = $derived(topics.data[topic] as Order | undefined);
  const buy = $derived(o?.side === 'bid');
  // A counterparty or owner that is a colonist opens in the inspector.
  const person = (key: string) => (key === 'colony' ? null : Number(key));
</script>

<div class="order">
  {#if !o}
    <p class="muted">Loading…</p>
  {:else if !o.found}
    <p class="muted">This order has left the book: filled, withdrawn, expired or repriced.</p>
  {:else}
    <dl>
      <dt>Owner</dt>
      <dd>
        {#if person(o.ownerKey) != null}
          <button type="button" class="link" onclick={() => inspect({ entity: person(o.ownerKey)! }, 'market')}>{o.owner}</button>
        {:else}{o.owner}{/if}
        {#if o.manual}<span class="muted">(placed by you)</span>{/if}
      </dd>
      <dt>Type</dt><dd>{buy ? 'buy' : 'sell'} {o.item}</dd>
      <dt>Price</dt><dd>{money(o.price)} each</dd>
      <dt>Filled</dt><dd>{o.filled} of {o.qty} <span class="muted">({o.open} open)</span></dd>
      {#if buy}
        <dt>Total</dt><dd>{money(o.total)} <span class="muted">for all {o.qty} at the limit</span></dd>
        <dt>In escrow</dt><dd>{money(o.escrow)}</dd>
      {:else}
        <dt>Total</dt><dd>{money(o.total)} <span class="muted">if all {o.qty} sell at the limit</span></dd>
        <dt>In escrow</dt><dd>{o.open} {o.item}</dd>
      {/if}
      <dt>Depot</dt><dd><button type="button" class="link" onclick={() => centerOn(o.x, o.y)}>{o.depot} at {o.x},{o.y}</button></dd>
      <dt>Opened</dt><dd>{dayClock(o.postedDay, o.postedMinute)} — {span(o.openMinutes)} open</dd>
      {#if o.expiresMinutes != null}<dt>Expires</dt><dd>in {span(o.expiresMinutes)}</dd>{/if}
    </dl>
    <h4>{buy ? 'Bought from' : 'Sold to'}</h4>
    {#if o.fills.length === 0}
      <p class="muted">nobody yet</p>
    {:else}
      <table>
        <tbody>
          {#each o.fills as f (f.key)}
            <tr>
              <td>
                {#if person(f.key) != null}
                  <button type="button" class="link" onclick={() => inspect({ entity: person(f.key)! }, 'market')}>{f.label}</button>
                {:else}{f.label}{/if}
              </td>
              <td class="num">{f.qty} {o.item}</td>
              <td class="num">{money(f.total)}</td>
            </tr>
          {/each}
          {#if o.fills.length > 1}
            <tr class="sum"><td>{buy ? 'spent' : 'earned'}</td><td class="num">{o.filled} {o.item}</td><td class="num">{money(o.paid)}</td></tr>
          {/if}
        </tbody>
      </table>
    {/if}
  {/if}
</div>

<style>
  .order { padding: 4px 8px 8px; font-size: 12px; border-left: 2px solid var(--accent); margin: 2px 0 6px; }
  p { margin: 0; }
  .muted { color: var(--muted); }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 2px 10px; margin: 0; }
  dt { color: var(--muted); }
  dd { margin: 0; font-variant-numeric: tabular-nums; }
  h4 { font-size: 11px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 8px 0 2px; font-weight: normal; }
  table { width: 100%; border-collapse: collapse; }
  td { padding: 1px 4px 1px 0; }
  .num { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
  .sum td { border-top: 1px solid var(--line); color: var(--muted); }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; }
  .link:hover { text-decoration: underline; }
</style>

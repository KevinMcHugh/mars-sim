<script lang="ts">
  // The Market tab: the money supply, every account (the treasury, then the
  // colonists richest first), and the market's books, prices, plans, work
  // orders and trades, from the market topic (internal/wire/boards.go). An
  // account opens its own page in place, from account:<key>. The colony's
  // order desk (ColonyOrders) places, reprices and withdraws its orders.
  import { inspect, subscribe, topics } from '../game.svelte';
  import AccountDetail from './AccountDetail.svelte';
  import ColonyOrders from './ColonyOrders.svelte';
  import { money } from './format';
  import type { ComponentProps } from 'svelte';

  interface Market {
    accounts: { key: string; label: string; balance: number }[];
    supply: { treasury: number; circulating: number; escrowed: number; frozen: number; issued: number; starved: number };
    books: { item: string; x: number; y: number; bestBid: number; bidQty: number; bestAsk: number; askQty: number; traded: boolean; last: number; volume: number }[];
    prices: { item: string; value: number; traded: boolean }[];
    plans: { actor: { id: number; name: string }; summary: string; waiting: boolean }[];
    chainDepth: number;
    work: { issuer: string; kind: string; units: number; held: number }[];
    trades: { tick: number; seller: string; buyer: string; qty: number; item: string; price: number }[];
    colony: ComponentProps<typeof ColonyOrders>['desk'];
  }

  $effect(() => subscribe('market'));
  const m = $derived(topics.data.market as Market | undefined);
  let openKey: string | null = $state(null);

  const side = (price: number, qty: number) => (qty === 0 ? '—' : `${money(price)}×${qty}`);
</script>

{#if !m}
  <p class="muted">Loading…</p>
{:else}
  <h2>Money supply</h2>
  <dl>
    <dt>Treasury</dt><dd>{money(m.supply.treasury)}</dd>
    <dt>Circulating</dt><dd>{money(m.supply.circulating)}</dd>
    <dt>In escrow</dt><dd>{money(m.supply.escrowed)} <span class="muted">held by open bids</span></dd>
    <dt>Frozen</dt><dd>{money(m.supply.frozen)} <span class="muted">held by the dead</span></dd>
    <dt>Issued</dt><dd>{money(m.supply.issued)}</dd>
    {#if m.supply.starved > 0}<dt>Starved</dt><dd>{m.supply.starved}</dd>{/if}
  </dl>

  <h2>Accounts ({m.accounts.length})</h2>
  <ul class="accounts">
    {#each m.accounts as a (a.key)}
      <li class:on={a.key === openKey}>
        <button type="button" class="acct" aria-expanded={a.key === openKey} onclick={() => (openKey = openKey === a.key ? null : a.key)}>
          <span class="name">{a.label}</span><span class="num">{money(a.balance)}</span>
        </button>
        {#if a.key === openKey}<AccountDetail key={a.key} />{/if}
      </li>
    {/each}
  </ul>

  <h2>Books</h2>
  {#if m.books.length === 0}
    <p class="muted">no orders yet</p>
  {:else}
    <table>
      <thead><tr><th>item</th><th>depot</th><th>bid</th><th>ask</th><th>last</th></tr></thead>
      <tbody>
        {#each m.books as b (b.item + b.x + ',' + b.y)}
          <tr>
            <td>{b.item}</td>
            <td><button type="button" class="link" onclick={() => inspect({ tile: [b.x, b.y] }, 'market')}>{b.x},{b.y}</button></td>
            <td class="num">{side(b.bestBid, b.bidQty)}</td>
            <td class="num">{side(b.bestAsk, b.askQty)}</td>
            <td class="num" title={b.traded ? `${b.volume} traded` : 'never traded'}>{b.traded ? money(b.last) : '—'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}

  <ColonyOrders desk={m.colony} treasury={m.supply.treasury} prices={m.prices} books={m.books} />

  <h2>Prices</h2>
  {#if m.prices.length === 0}
    <p class="muted">none</p>
  {:else}
    <dl>
      {#each m.prices as p (p.item)}
        <dt>{p.item}</dt><dd>{money(p.value)} <span class="muted">{p.traded ? 'traded' : 'reference'}</span></dd>
      {/each}
    </dl>
  {/if}

  <h2>Plans <span class="muted">(chain depth {m.chainDepth})</span></h2>
  {#if m.plans.length === 0}
    <p class="muted">none</p>
  {:else}
    <ul class="lines">
      {#each m.plans as p, i (i)}
        <li>
          <button type="button" class="link" onclick={() => inspect({ entity: p.actor.id }, 'market')}>{p.actor.name || `#${p.actor.id}`}</button>:
          {p.summary}{#if p.waiting}<span class="muted"> (waiting on inputs)</span>{/if}
        </li>
      {/each}
    </ul>
  {/if}

  <h2>Work orders</h2>
  {#if m.work.length === 0}
    <p class="muted">none open</p>
  {:else}
    <ul class="lines">
      {#each m.work as w, i (i)}
        <li>{w.issuer}: {w.units} {w.kind === 'haul' ? 'units to haul' : w.kind === 'dig' ? 'tiles to dig' : `${w.kind} tasks`}, {money(w.held)} held</li>
      {/each}
    </ul>
  {/if}

  <h2>Recent trades</h2>
  {#if m.trades.length === 0}
    <p class="muted">none yet</p>
  {:else}
    <ul class="lines">
      {#each m.trades as t, i (i)}
        <li><span class="muted">t{t.tick}</span> {t.seller} sold {t.buyer} {t.qty} {t.item} @ {money(t.price)}</li>
      {/each}
    </ul>
  {/if}
{/if}

<style>
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 16px 0 6px; }
  h2:first-child { margin-top: 4px; }
  h2 .muted { text-transform: none; letter-spacing: 0; }
  p { margin: 0; }
  .muted { color: var(--muted); }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 0; }
  dt { color: var(--muted); }
  dd { margin: 0; font-variant-numeric: tabular-nums; }
  ul { list-style: none; padding: 0; margin: 0; }
  .accounts li { border: 1px solid transparent; border-radius: 6px; }
  .accounts li.on { border-color: var(--accent); background: rgba(224, 112, 58, 0.1); }
  .acct { width: 100%; display: flex; justify-content: space-between; gap: 8px; border-color: transparent; background: transparent; padding: 3px 8px; }
  .acct:hover { background: rgba(255, 255, 255, 0.05); }
  .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: left; }
  .num { font-variant-numeric: tabular-nums; white-space: nowrap; }
  table { width: 100%; border-collapse: collapse; font-size: 12px; }
  th { text-align: left; color: var(--muted); font-weight: normal; padding: 2px 4px; }
  td { padding: 2px 4px; border-top: 1px solid var(--line); }
  td.num { text-align: right; }
  .lines li { line-height: 1.5; font-size: 13px; }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; }
  .link:hover { text-decoration: underline; }
</style>

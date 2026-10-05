<script lang="ts">
  // The colony's order desk, in the Market tab: post a bid or an ask in the
  // colony's name at a communal depot, and reprice or withdraw the colony's
  // open orders, or suspend a standing order (by side and item, colony-wide)
  // so upkeep stops posting it. The orders are ordinary ones on the book, escrowed from the
  // treasury or the colony's stock (docs/colony-orders.md). The data is the
  // market topic's colony desk (internal/wire/boards.go); the outcome of a
  // command lands in the log. An order opens its detail (OrderDetail) on a
  // click.
  import { cancelColonyOrder, centerOn, placeColonyOrder, repriceColonyOrder, resumeColonyOrders, suspendColonyOrders } from '../game.svelte';
  import { money } from './format';
  import OrderDetail from './OrderDetail.svelte';

  interface Desk {
    orders: { id: number; side: 'bid' | 'ask'; item: string; qty: number; price: number; x: number; y: number; posted: number; manual: boolean }[];
    depots: { x: number; y: number; label: string; silo: boolean; holdings: { item: string; count: number }[] }[];
    items: string[];
    suspended: { side: 'bid' | 'ask'; item: string }[];
  }

  interface Props {
    desk: Desk;
    treasury: number;
    prices: { item: string; value: number }[];
    books: { item: string; x: number; y: number; bestBid: number; bidQty: number; bestAsk: number; askQty: number }[];
  }

  let { desk, treasury, prices, books }: Props = $props();

  // ---- placing -------------------------------------------------------------

  let side = $state<'bid' | 'ask'>('bid');
  let item = $state('');
  let qty: number | null = $state(1);
  // Empty means "the item's market value", shown as the placeholder.
  let priceIn: number | null = $state(null);
  let depotKey = $state('');

  const held = (d: Desk['depots'][number] | undefined, it: string) => d?.holdings.find((h) => h.item === it)?.count ?? 0;
  // Selling, offer only what there is to sell: the depots where the colony
  // holds something, and the items it holds at the chosen one. Buying, any
  // depot and any item will do.
  const depots = $derived(side === 'ask' ? desk.depots.filter((d) => d.holdings.some((h) => h.count > 0)) : desk.depots);
  const depot = $derived(depots.find((d) => `${d.x},${d.y}` === depotKey) ?? depots[0]);
  const items = $derived(side === 'ask' ? desk.items.filter((it) => held(depot, it) > 0) : desk.items);
  // A choice the filter hides (switching side or depot) falls back to the first.
  const chosenItem = $derived(items.includes(item) ? item : (items[0] ?? ''));
  const value = $derived(prices.find((p) => p.item === chosenItem)?.value ?? 0);
  const price = $derived(priceIn != null && !Number.isNaN(priceIn) ? priceIn : value);
  const heldHere = $derived(held(depot, chosenItem));
  const book = $derived(depot && books.find((b) => b.item === chosenItem && b.x === depot.x && b.y === depot.y));
  const n = $derived(qty ?? 0);

  // Why the order cannot go, or null if it can. The engine checks all of it
  // again; this only saves a round trip to the log.
  const blocked = $derived.by(() => {
    if (!depot) return side === 'ask' && desk.depots.length > 0 ? 'The colony holds nothing at any depot to sell.' : 'The colony has no communal depot to trade at yet.';
    if (!chosenItem) return 'Pick an item.';
    if (!Number.isInteger(n) || n < 1) return 'The quantity must be a whole number, at least 1.';
    if (!Number.isInteger(price) || price < 1) return 'The price must be a whole number of dollars, at least $1.';
    if (side === 'bid' && n * price > treasury) return `The treasury (${money(treasury)}) cannot pay ${money(n * price)}.`;
    if (side === 'ask' && n > heldHere) return `The colony holds only ${heldHere} ${chosenItem} at this depot.`;
    return null;
  });

  // What will happen when it is posted: does it cross the book?
  const crosses = $derived.by(() => {
    if (!book) return false;
    return side === 'bid' ? book.askQty > 0 && price >= book.bestAsk : book.bidQty > 0 && price <= book.bestBid;
  });

  function place() {
    if (blocked || !depot) return;
    placeColonyOrder({ side, item: chosenItem, qty: n, price, x: depot.x, y: depot.y });
    priceIn = null;
  }

  // ---- repricing -----------------------------------------------------------

  let editing: number | null = $state(null);
  let newPrice: number | null = $state(null);

  function startEdit(o: Desk['orders'][number]) {
    editing = o.id;
    newPrice = o.price;
  }

  function repriceBlocked(o: Desk['orders'][number]): string | null {
    const p = newPrice ?? 0;
    if (!Number.isInteger(p) || p < 1) return 'at least $1';
    if (p === o.price) return 'unchanged';
    // The order's own escrow comes back before it is re-posted.
    if (o.side === 'bid' && o.qty * p > treasury + o.qty * o.price) return `the treasury cannot cover ${money(o.qty * p)}`;
    return null;
  }

  function reprice(o: Desk['orders'][number]) {
    if (repriceBlocked(o) || newPrice == null) return;
    repriceColonyOrder(o.id, newPrice);
    editing = null;
  }

  // The order whose detail is open under its row (OrderDetail).
  let detail: number | null = $state(null);

  // The row being edited or detailed leaves the book (filled, or repriced,
  // which re-posts it under a new id): stop editing or showing it.
  $effect(() => {
    if (editing != null && !desk.orders.some((o) => o.id === editing)) editing = null;
    if (detail != null && !desk.orders.some((o) => o.id === detail)) detail = null;
  });

  const verb = (side: 'bid' | 'ask') => (side === 'bid' ? 'buying' : 'selling');

  const depotLabel = (x: number, y: number) => {
    const d = desk.depots.find((d) => d.x === x && d.y === y);
    return d ? (d.silo ? 'silo' : d.label) : 'depot';
  };
</script>

<h2>Colony orders</h2>
<p class="muted">Trade in the colony's name. A bid is paid from the treasury, an ask from what the colony holds at the depot.</p>

<form class="place" onsubmit={(e) => { e.preventDefault(); place(); }}>
  <div class="sides" role="group" aria-label="Side">
    <button type="button" class:on={side === 'bid'} aria-pressed={side === 'bid'} onclick={() => (side = 'bid')}>Buy</button>
    <button type="button" class:on={side === 'ask'} aria-pressed={side === 'ask'} onclick={() => (side = 'ask')}>Sell</button>
  </div>
  <label>
    <span>Item</span>
    <select value={chosenItem} onchange={(e) => (item = e.currentTarget.value)}>
      {#each items as it (it)}
        <option value={it}>{it}{side === 'ask' ? ` (${held(depot, it)} held)` : ''}</option>
      {/each}
    </select>
  </label>
  <label>
    <span>Depot</span>
    <select value={depot ? `${depot.x},${depot.y}` : ''} onchange={(e) => (depotKey = e.currentTarget.value)} disabled={depots.length === 0}>
      {#each depots as d (d.x + ',' + d.y)}
        <option value={`${d.x},${d.y}`}>{d.silo ? 'silo' : d.label} at {d.x},{d.y}</option>
      {/each}
    </select>
  </label>
  <label>
    <span>Quantity</span>
    <input type="number" min="1" step="1" bind:value={qty} />
  </label>
  <label>
    <span>Price each</span>
    <input type="number" min="1" step="1" placeholder={value > 0 ? `${value} (market value)` : ''} bind:value={priceIn} />
  </label>
  <p class="terms">
    {#if book && (book.bidQty > 0 || book.askQty > 0)}
      <span class="muted">Book here: bid {book.bidQty ? `${money(book.bestBid)}×${book.bidQty}` : '—'}, ask {book.askQty ? `${money(book.bestAsk)}×${book.askQty}` : '—'}.</span>
    {/if}
    {#if side === 'bid'}
      Escrows {money(n * price)} of the treasury's {money(treasury)}.
    {:else}
      The colony holds {heldHere} here.
    {/if}
    {#if crosses && !blocked}<span class="warn">It will trade at once against the book.</span>{/if}
  </p>
  {#if blocked}<p class="warn">{blocked}</p>{/if}
  <button type="submit" class="go" disabled={blocked !== null}>
    {side === 'bid' ? 'Bid' : 'Offer'} {n} {chosenItem} at {money(price)}
  </button>
</form>

{#if desk.orders.length === 0}
  <p class="muted">The colony has no orders open.</p>
{:else}
  <table>
    <thead><tr><th></th><th>order</th><th class="num">price</th><th>depot</th></tr></thead>
    <tbody>
      {#each desk.orders as o (o.id)}
        <tr>
          <td><span class="tag" class:manual={o.manual} title={o.manual ? 'placed or repriced by you; the colony leaves it alone' : 'a standing order the colony keeps topped up: withdrawn, it is posted again'}>{o.manual ? 'yours' : 'auto'}</span></td>
          <td>
            <button type="button" class="link" aria-expanded={o.id === detail} title="Show this order's detail"
              onclick={() => (detail = detail === o.id ? null : o.id)}>{o.side === 'bid' ? 'buy' : 'sell'} {o.qty} {o.item}</button>
            <div class="acts">
            {#if editing !== o.id}
              <button type="button" onclick={() => startEdit(o)} title="Re-post at another price; it joins the back of the queue">Reprice</button>
            {/if}
            {#if !o.manual}
              <button type="button" onclick={() => suspendColonyOrders(o.side, o.item)}
                title="Stop {verb(o.side)} {o.item} everywhere: withdraw the colony's standing orders for it and post no more until resumed">Suspend</button>
            {/if}
            <button type="button" onclick={() => cancelColonyOrder(o.id)}
              title={o.side === 'bid' ? `Withdraw, returning ${money(o.qty * o.price)} to the treasury` : `Withdraw, returning ${o.qty} ${o.item} to the colony's stock`}>Remove</button>
            </div>
          </td>
          <td class="num">
            {#if editing === o.id}
              <form class="reprice" onsubmit={(e) => { e.preventDefault(); reprice(o); }}>
                <input type="number" min="1" step="1" bind:value={newPrice} aria-label="New price" />
                <button type="submit" disabled={repriceBlocked(o) !== null} title={repriceBlocked(o) ?? `Re-post at ${money(newPrice ?? 0)}`}>OK</button>
                <button type="button" onclick={() => (editing = null)} aria-label="Stop repricing">×</button>
              </form>
            {:else}
              {money(o.price)}
            {/if}
          </td>
          <td><button type="button" class="link" onclick={() => centerOn(o.x, o.y)}>{depotLabel(o.x, o.y)}</button></td>
        </tr>
        {#if o.id === detail}
          <tr class="detail"><td colspan="4"><OrderDetail id={o.id} /></td></tr>
        {/if}
      {/each}
    </tbody>
  </table>
  <p class="muted">Repricing re-posts the order, so it joins the back of the queue at its new price. Standing orders (auto) are topped up again by the colony after you remove them; suspend one to keep it off the book.</p>
{/if}

{#if desk.suspended.length > 0}
  <h3>Suspended</h3>
  <ul class="lines">
    {#each desk.suspended as su (su.side + su.item)}
      <li>
        The colony has stopped {verb(su.side)} {su.item}.
        <button type="button" class="resume" onclick={() => resumeColonyOrders(su.side, su.item)}
          title="Let the colony post its standing orders for {su.item} again">Resume</button>
      </li>
    {/each}
  </ul>
{/if}

<style>
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 16px 0 6px; }
  p { margin: 0 0 6px; }
  .muted { color: var(--muted); }
  .warn { color: #ffb36b; }
  .place { display: grid; grid-template-columns: max-content 1fr; gap: 4px 10px; align-items: center; margin: 8px 0 10px; }
  .place label { display: contents; }
  .place label span { color: var(--muted); }
  .place select, .place input { min-width: 0; }
  .place .terms, .place .warn, .place .sides, .place .go { grid-column: 1 / -1; }
  .terms { font-size: 12px; display: flex; flex-wrap: wrap; gap: 0 6px; }
  .sides { display: flex; gap: 6px; }
  .sides button.on, .go { background: var(--accent); color: #fff; }
  .go:disabled { background: transparent; color: inherit; }
  table { width: 100%; border-collapse: collapse; font-size: 12px; }
  th { text-align: left; color: var(--muted); font-weight: normal; padding: 2px 4px; }
  td { padding: 3px 4px; border-top: 1px solid var(--line); vertical-align: top; }
  .num { text-align: right; font-variant-numeric: tabular-nums; white-space: nowrap; }
  td:first-child, td.num, td:last-child { width: 1%; white-space: nowrap; }
  .acts { display: flex; flex-wrap: wrap; gap: 3px; margin: 2px 0; }
  .acts button, .reprice button { padding: 0 5px; font-size: 11px; }
  .reprice { display: inline-flex; gap: 3px; }
  .reprice input { width: 5em; }
  h3 { font-size: 12px; color: var(--muted); font-weight: normal; margin: 10px 0 4px; }
  ul { list-style: none; padding: 0; margin: 0 0 8px; }
  .lines li { line-height: 1.6; font-size: 13px; }
  .resume { margin-left: 6px; padding: 0 8px; font-size: 12px; }
  .tag { font-size: 11px; color: var(--muted); border: 1px solid var(--line); border-radius: 4px; padding: 0 4px; }
  .tag.manual { color: #fff; border-color: var(--accent); background: rgba(224, 112, 58, 0.25); }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; }
  .link:hover { text-decoration: underline; }
  tr.detail td { border-top: none; padding: 0 4px; white-space: normal; width: auto; }
</style>

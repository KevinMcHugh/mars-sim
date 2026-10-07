<script lang="ts">
  // The Dig tab: order an area mined out. Mark it on the map with the tool,
  // see what it would cost, and order it: the colony buys the digging from the
  // treasury as work orders on the order book (docs/excavation.md). The prices
  // and the open orders come from the market topic.
  import { armDig, cancelDig, centerOn, clearDig, orderDig, subscribe, topics, ui } from '../game.svelte';
  import { money } from './format';

  interface Market {
    supply: { treasury: number };
    work: { issuer: string; kind: string; units: number; held: number }[];
    dig: { wage: number; maxTiles: number };
    digs: { id: number; x0: number; y0: number; x1: number; y1: number; tiles: number; done: number; held: number }[];
  }

  $effect(() => subscribe('market'));
  // Leaving the tab puts the tool down and clears the tint it left on the map.
  $effect(() => () => clearDig());

  const m = $derived(topics.data.market as Market | undefined);
  const r = $derived(ui.dig.rect);
  const cost = $derived(m ? ui.dig.tiles * m.dig.wage : 0);

  // Why the order cannot go, or null if it can.
  const blocked = $derived.by(() => {
    if (!m || !r) return null;
    if (ui.dig.tiles === 0) return 'No rock the colony has seen in this area.';
    if (ui.dig.tiles > m.dig.maxTiles) return `Too large: an order covers at most ${m.dig.maxTiles} tiles.`;
    if (cost > m.supply.treasury) return `The treasury (${money(m.supply.treasury)}) cannot pay for it.`;
    return null;
  });
</script>

<p class="muted">
  Order an area mined out. The colony pays for it from the treasury: every tile of rock it has seen
  becomes a work order, paid to whoever digs it, who keeps the ore.
</p>

<div class="row">
  <button type="button" class:on={ui.dig.armed} aria-pressed={ui.dig.armed} onclick={() => armDig(!ui.dig.armed)}>
    {ui.dig.armed ? 'Drag on the map…' : r ? 'Mark another area' : 'Mark an area'}
  </button>
  {#if r}<button type="button" onclick={clearDig}>Clear</button>{/if}
</div>

{#if r && m}
  <dl>
    <dt>Area</dt>
    <dd>
      <button type="button" class="link" onclick={() => centerOn((r.x0 + r.x1) >> 1, (r.y0 + r.y1) >> 1)}>
        {r.x0},{r.y0} to {r.x1},{r.y1}
      </button>
      <span class="muted">{r.x1 - r.x0 + 1}×{r.y1 - r.y0 + 1}</span>
    </dd>
    <dt>Rock to dig</dt><dd>{ui.dig.tiles} tiles</dd>
    <dt>Price</dt><dd>{money(cost)} <span class="muted">at {money(m.dig.wage)} a tile</span></dd>
    <dt>Treasury</dt><dd>{money(m.supply.treasury)}</dd>
  </dl>
  {#if blocked}<p class="warn">{blocked}</p>{/if}
  <div class="row">
    <button type="button" class="go" disabled={blocked !== null || ui.dig.armed} onclick={orderDig}>
      Order for {money(cost)}
    </button>
  </div>
{/if}

<h2>Open dig orders</h2>
{#if !m}
  <p class="muted">Loading…</p>
{:else if m.digs.length === 0}
  <p class="muted">none open</p>
{:else}
  <ul class="lines">
    {#each m.digs as d (d.id)}
      <li>
        <button type="button" class="link" onclick={() => centerOn((d.x0 + d.x1) >> 1, (d.y0 + d.y1) >> 1)}>
          {d.x0},{d.y0} to {d.x1},{d.y1}
        </button>
        {d.done}/{d.tiles} dug, {money(d.held)} held
        <button type="button" class="cancel" onclick={() => cancelDig(d.id)}
          title="Close the order and return {money(d.held)} to the treasury">Cancel</button>
      </li>
    {/each}
  </ul>
  <p class="muted">The Jobs tab shows who is digging. Cancelling keeps what is already dug and paid.</p>
{/if}

<style>
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 16px 0 6px; }
  h2:first-child { margin-top: 4px; }
  p { margin: 0 0 8px; }
  .muted { color: var(--muted); }
  .warn { color: #ffb36b; }
  .row { display: flex; gap: 6px; margin: 8px 0; }
  .row button.on { background: var(--accent); color: #fff; }
  .row button.go { background: var(--accent); color: #fff; }
  .row button.go:disabled { background: transparent; color: inherit; }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 8px 0; }
  dt { color: var(--muted); }
  dd { margin: 0; font-variant-numeric: tabular-nums; }
  ul { list-style: none; padding: 0; margin: 0 0 8px; }
  .lines li { line-height: 1.5; font-size: 13px; }
  .cancel { margin-left: 6px; padding: 0 8px; font-size: 12px; }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; }
  .link:hover { text-decoration: underline; }
</style>

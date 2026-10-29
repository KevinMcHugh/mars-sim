<script lang="ts">
  // The Trade tab: the colony's order desk (ColonyOrders, docs/colony-orders.md),
  // buying and selling in the colony's name. It reads the market topic, as the
  // Market tab does, but sits under Act: placing orders changes the colony,
  // while the Market tab only shows prices, books and accounts.
  // The power policy and the battery price moved to the Power tab
  // (PowerPanel, docs/power.md); a line here points there.
  import { setPanel, subscribe, topics } from '../game.svelte';
  import ColonyOrders from './ColonyOrders.svelte';
  import type { ComponentProps } from 'svelte';

  interface Market {
    supply: { treasury: number };
    books: ComponentProps<typeof ColonyOrders>['books'];
    prices: ComponentProps<typeof ColonyOrders>['prices'];
    colony: ComponentProps<typeof ColonyOrders>['desk'];
  }

  $effect(() => subscribe('market'));
  // Leaving the tab drops the topic before ColonyOrders is torn down, and its
  // props would then read through undefined: keep the last payload instead.
  let last: Market | undefined;
  const m = $derived.by(() => (last = (topics.data.market as Market | undefined) ?? last));
</script>

{#if !m}
  <p class="muted">Loading…</p>
{:else}
  <p class="muted">The power policy and the battery price are on the <button type="button" class="link" onclick={() => setPanel('power')}>Power tab</button>.</p>
  <ColonyOrders desk={m.colony} treasury={m.supply.treasury} prices={m.prices} books={m.books} />
{/if}

<style>
  p { margin: 0; }
  .muted { color: var(--muted); }
  .link { background: none; border: none; padding: 0; color: var(--accent); cursor: pointer; font: inherit; }
</style>

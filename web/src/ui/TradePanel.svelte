<script lang="ts">
  // The Trade tab: the colony's order desk (ColonyOrders, docs/colony-orders.md),
  // buying and selling in the colony's name. It reads the market topic, as the
  // Market tab does, but sits under Act: placing orders changes the colony,
  // while the Market tab only shows prices, books and accounts.
  import { subscribe, topics } from '../game.svelte';
  import ColonyOrders from './ColonyOrders.svelte';
  import type { ComponentProps } from 'svelte';

  interface Market {
    supply: { treasury: number };
    books: ComponentProps<typeof ColonyOrders>['books'];
    prices: ComponentProps<typeof ColonyOrders>['prices'];
    colony: ComponentProps<typeof ColonyOrders>['desk'];
  }

  $effect(() => subscribe('market'));
  const m = $derived(topics.data.market as Market | undefined);
</script>

{#if !m}
  <p class="muted">Loading…</p>
{:else}
  <ColonyOrders desk={m.colony} treasury={m.supply.treasury} prices={m.prices} books={m.books} />
{/if}

<style>
  p { margin: 0; }
  .muted { color: var(--muted); }
</style>

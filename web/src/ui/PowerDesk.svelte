<script lang="ts">
  // The power policy, on the Power tab: who runs the colony's reactors
  // (monopoly, market, public option, charter), changeable in a game. A change
  // moves reactors at once, all or nothing, at their buyout price, and waits
  // out a cooldown after the last; the log says why one was refused. The data
  // is the power topic (internal/wire/boards.go, PowerDesk). See
  // docs/power.md.
  import { setBatteryPrice, setPowerPolicy } from '../game.svelte';
  import { money } from './format';
  import type { Power } from './power';
  import Section from './Section.svelte';

  let { power }: { power: Power } = $props();
  // What the battery price means under each policy, and whether it is the
  // player's to set (docs/power.md).
  const priceLabel = $derived(
    power.priceKind === 'bid' ? 'The colony pays' : power.priceKind === 'charter' ? 'The charter company charges' : 'Tariff',
  );
  const priceHelp = $derived(
    power.priceKind === 'bid'
      ? 'what the colony bids for a battery at its stoves'
      : power.priceKind === 'charter'
        ? 'its own price; take the charter back to set one'
        : "what a colony battery is worth: part of every meal's cost",
  );
  let choice = $state('');
  const picked = $derived(choice && choice !== power.policy ? choice : '');

  const label: Record<string, string> = {
    monopoly: 'Monopoly: the colony runs the reactors',
    market: 'Market: colonists who can afford one',
    'public-option': 'Public option: a market, and a colony reactor of last resort',
    charter: 'Charter: one corporation from back home',
  };
  // How each policy works, in a sentence.
  const how: Record<string, string> = {
    monopoly: 'The colony builds a reactor once it holds uranium, buys its fuel, pays an operator, and stocks the stoves itself.',
    market: 'The colony builds no reactor. It bids for batteries at its stoves, and a colonist with the savings may build one and sell into those bids.',
    'public-option': 'A market, until the colony goes too long without a battery: then it runs a reactor of its own beside any colonist’s.',
    charter: 'One corporation holds the only right to make power. It builds and fuels its reactor with money from home, sells at its own price, and pays the colony a royalty.',
  };
  // What changing to a policy would do to the reactors standing now.
  function consequence(to: string): string {
    const n = power.reactors.length;
    if (n === 0) return 'No reactor stands yet, so nothing changes hands.';
    const total = power.reactors.reduce((s, r) => s + r.price, 0);
    switch (to) {
      case 'monopoly': {
        const bought = power.reactors.filter((r) => r.owner !== 'the colony');
        return bought.length ? `The colony buys ${bought.length} reactor(s) for ${money(bought.reduce((s, r) => s + r.price, 0))} from the treasury.` : 'The colony already owns every reactor.';
      }
      case 'charter':
        return `The charter company buys every reactor, ${money(total)} in all, with money from back home.`;
      default:
        if (power.policy === 'charter') return 'The colony buys the charter company out: its reactors and its batteries.';
        return 'The colony offers its reactors for sale; it runs them until a colonist buys one.';
    }
  }
  const po = $derived(power.publicOption);
</script>

<Section id="power.policy" title="Policy" note={`(${power.policy})`}>
  <p><b>{label[power.policy] ?? power.policy}</b></p>
  <p class="muted">{how[power.policy] ?? ''}</p>
  <p class="price">
    <b>{priceLabel}</b> {money(power.price)} a battery
    {#if power.priceKind !== 'charter'}
      <button type="button" disabled={power.price <= 0} onclick={() => setBatteryPrice(power.price - 1)} aria-label="A dollar less">−</button>
      <button type="button" onclick={() => setBatteryPrice(power.price + 1)} aria-label="A dollar more">+</button>
    {/if}
    <span class="muted">{priceHelp}</span>
  </p>
  {#if power.charter}
    <p>{power.charter.name}: sold {power.charter.sold} · paid the colony {money(power.charter.royalties)} · sent home {money(power.charter.remitted)} · holds {money(power.charter.wallet)}</p>
  {/if}
  {#if po}
    {#if po.latched}
      <p>The market left the colony without power: it runs a reactor of its own now.</p>
    {:else if po.dryHours >= 0}
      <p class="warn">The colony has gone {po.dryHours} hours without a battery. At {po.days * 24} it builds a reactor of its own.</p>
    {:else}
      <p class="muted">The colony steps in after {po.days} days without a battery.</p>
    {/if}
  {/if}
  {#if power.commission}
    <p class="muted">A colonist needs {money(power.commission.savings)} to build a reactor;
      {power.commission.able === 0 ? 'nobody has that yet' : `${power.commission.able} could now`}.
      One does once the stoves’ battery bids go unfilled.</p>
  {/if}
  <div class="change">
    <select bind:value={choice} aria-label="Power policy">
      <option value="">Change policy…</option>
      {#each power.policies as p (p)}
        <option value={p} disabled={p === power.policy}>{label[p] ?? p}</option>
      {/each}
    </select>
    <button type="button" disabled={!picked || power.cooldownHours > 0} onclick={() => { setPowerPolicy(picked); choice = ''; }}>Change</button>
  </div>
  {#if power.cooldownHours > 0}
    <p class="muted">The policy changed recently: it can change again in {power.cooldownHours} hours.</p>
  {:else if picked}
    <p class="muted">{consequence(picked)}</p>
  {/if}
</Section>

<style>
  p { margin: 0.2em 0; }
  .muted { color: var(--muted); }
  .warn { color: var(--accent); font-weight: bold; }
  .change { display: flex; gap: 0.4em; margin-top: 0.3em; }
  .price button { padding: 0 0.5em; }
  select { flex: 1; min-width: 0; }
</style>

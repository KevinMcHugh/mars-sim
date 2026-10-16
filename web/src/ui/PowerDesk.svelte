<script lang="ts">
  // The power policy, on the Trade tab: who runs the colony's reactors
  // (monopoly, market, public option, charter), changeable in a game. A change
  // moves reactors at once, all or nothing, at their buyout price, and waits
  // out a cooldown after the last; the log says why one was refused. The data
  // is the market topic's colony desk power section (internal/wire/boards.go,
  // PowerDesk). See docs/power.md.
  import { centerOn, setPowerPolicy } from '../game.svelte';
  import { money } from './format';
  import Section from './Section.svelte';

  export interface Power {
    on: boolean;
    policy: string;
    policies: string[];
    cooldownHours: number;
    reactors: { x: number; y: number; owner: string; price: number; forSale: boolean; batteries: number }[];
    charter?: { name: string; wallet: number; sold: number; royalties: number; remitted: number };
    batteries: number;
  }

  let { power }: { power: Power } = $props();
  let choice = $state('');
  const picked = $derived(choice && choice !== power.policy ? choice : '');

  const label: Record<string, string> = {
    monopoly: 'Monopoly: the colony runs the reactors',
    market: 'Market: colonists who can afford one',
    'public-option': 'Public option: a market, and a colony reactor of last resort',
    charter: 'Charter: one corporation from back home',
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
</script>

<Section id="trade.power" title="Power" note={power.on ? `(${power.policy})` : '(off)'}>
  {#if !power.on}
    <p class="muted">Power is off in this game: stoves need no batteries.</p>
  {:else}
    <p>{label[power.policy] ?? power.policy} · the colony holds {power.batteries} batteries</p>
    {#if power.charter}
      <p class="muted">{power.charter.name}: sold {power.charter.sold} · paid the colony {money(power.charter.royalties)} · sent home {money(power.charter.remitted)} · holds {money(power.charter.wallet)}</p>
    {/if}
    {#if power.reactors.length > 0}
      <ul>
        {#each power.reactors as r (`${r.x},${r.y}`)}
          <li>
            <button type="button" class="link" onclick={() => centerOn(r.x, r.y)}>⚡ ({r.x}, {r.y})</button>
            {r.owner} · {r.batteries} batteries · worth {money(r.price)}{#if r.forSale} · <b>for sale</b>{/if}
          </li>
        {/each}
      </ul>
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
  {/if}
</Section>

<style>
  p { margin: 0.2em 0; }
  ul { margin: 0.2em 0; padding-left: 1.2em; }
  .muted { color: var(--muted); }
  .change { display: flex; gap: 0.4em; margin-top: 0.3em; }
  select { flex: 1; min-width: 0; }
  .link { background: none; border: none; padding: 0; color: var(--accent); cursor: pointer; font: inherit; }
</style>

<script lang="ts">
  // The Power tab: the colony's power at a glance (the power topic,
  // internal/wire/boards.go PowerDesk). How many reactors stand and how each
  // runs, the ones going up, the colony's batteries and fuel, how much of
  // the cooking had power, and the policy, with the controls to change it
  // (PowerDesk). A reactor or a prospector opens its tile in the inspector.
  // See docs/power.md.
  import { inspect, subscribe, topics } from '../game.svelte';
  import { money } from './format';
  import { headline, poweredShare, stalled, statusText, type Power, type Spot } from './power';
  import PowerDesk from './PowerDesk.svelte';
  import Section from './Section.svelte';

  $effect(() => subscribe('power'));
  const p = $derived(topics.data.power as Power | undefined);
  const share = $derived(p && poweredShare(p));
  const open = (s: Spot) => inspect({ tile: [s.x, s.y, s.l] }, 'power');
  const priceKind = { tariff: 'the tariff', bid: "the colony's bid", charter: "the charter's price" } as const;
  const at = (s: Spot) => `(${s.x}, ${s.y}, level ${s.l})`;
</script>

{#if !p}
  <p class="muted">Loading…</p>
{:else if !p.on}
  <h2>Power</h2>
  <p class="muted">Power is off in this game: stoves need no batteries, and nobody builds a reactor.</p>
{:else}
  <Section id="power.overview" title="Power" tag="h2" note={`(${p.policy})`}>
    <p class="headline">⚡ {headline(p)}</p>
    <dl>
      <dt>Colony batteries</dt>
      <dd>{p.batteries} <span class="muted">(a reactor charges while its owner holds fewer than {p.reserve})</span></dd>
      <dt>Battery price</dt>
      <dd>{money(p.price)} <span class="muted">({priceKind[p.priceKind]})</span></dd>
      <dt>Charged so far</dt>
      <dd>{p.charged} batteries</dd>
      <dt>Cooking with power</dt>
      <dd>
        {#if share === null}<span class="muted">nothing cooked yet</span>
        {:else}{share}% <span class="muted">({p.cookedPowered} powered, {p.cookedCold} cold: cold cooking is slower)</span>{/if}
      </dd>
    </dl>
  </Section>

  <Section id="power.plants" title="Reactors" note={`(${p.reactors.length})`}>
    {#if p.reactors.length === 0 && p.building.length === 0}
      <p class="muted">
        {#if p.policy === 'monopoly' || p.policy === 'charter' || p.publicOption?.latched}
          No reactor yet: one goes up once there is uranium to burn.
        {:else}
          No reactor yet: a colonist builds one once the stoves' battery bids go unfilled.
        {/if}
      </p>
    {/if}
    <ul class="plants">
      {#each p.reactors as r (`${r.x},${r.y},${r.l}`)}
        <li class:stalled={stalled(r)}>
          <div>
            <button type="button" class="link" onclick={() => open(r)}>⚡ {at(r)}</button>
            <b>{r.owner}</b>{#if r.forSale} · <b>for sale</b>{/if}
            <span class="muted">· worth {money(r.price)}</span>
          </div>
          <div class="state">{statusText(r)}</div>
          <div class="muted">
            {r.batteries} batteries · {r.pellets} pellets · {r.ore} uranium ore · {r.ice} ice
          </div>
        </li>
      {/each}
      {#each p.building as b (`${b.x},${b.y},${b.l}`)}
        <li class="building">
          <button type="button" class="link" onclick={() => open(b)}>🏗 {at(b)}</button>
          <b>{b.owner}</b> <span class="muted">· going up</span>
        </li>
      {/each}
    </ul>
  </Section>

  <Section id="power.fuel" title="Fuel">
    <p>The colony holds {p.uranium} uranium ore and {p.pellets} fuel pellets.
      {#if p.policy === 'monopoly' || p.publicOption?.latched}<span class="muted">It keeps {p.uraniumReserve} ore in hand.</span>{/if}</p>
    {#if p.prospector}
      <p>⛏ <b>{p.prospector.name}</b> is prospecting for uranium at
        <button type="button" class="link" onclick={() => open(p.prospector!)}>{at(p.prospector)}</button>.</p>
    {:else}
      <p class="muted">Nobody is prospecting for uranium right now.</p>
    {/if}
  </Section>

  <PowerDesk power={p} />
{/if}

<style>
  p { margin: 0.2em 0; }
  .muted { color: var(--muted); }
  .headline { font-size: 1.1em; font-weight: bold; }
  dl { display: grid; grid-template-columns: auto 1fr; gap: 0.1em 0.8em; margin: 0.3em 0; }
  dt { color: var(--muted); }
  dd { margin: 0; }
  .plants { list-style: none; margin: 0.2em 0; padding: 0; }
  .plants li { border-left: 3px solid var(--line); padding: 0.2em 0 0.2em 0.5em; margin: 0.3em 0; }
  .plants li.stalled { border-left-color: var(--accent); }
  .plants li.stalled .state { color: var(--accent); font-weight: bold; }
  .plants li.building { border-left-style: dashed; }
  .link { background: none; border: none; padding: 0; color: var(--accent); cursor: pointer; font: inherit; }
</style>

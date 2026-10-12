<script lang="ts">
  // The Zones tab (docs/zoning.md): paint zones on the map, remove them, or
  // order an area's structures cleared. Zoning is free; the work it implies
  // (digging out rock in a new zone, clearing a structure a paint leaves in
  // the wrong zone) is bought from the treasury, so the tab prices it first.
  // The overlay itself comes from the zones topic, which the page always
  // holds; this tab adds the zoning topic. The Defense tool posts a detail of
  // armed colonists to keep an area safe, on any level (docs/defense.md).
  import { anyLevelTool, applyZone, armZone, cancelClear, cancelDefense, centerOn, clearZoneTool, setDefenseDetail, subscribe, topics, ui, zoneChanged } from '../game.svelte';
  import { untrack } from 'svelte';
  import { money } from './format';

  interface Zones { kinds: { name: string; color: string }[]; defenseColor: string }
  interface Defense {
    id: number; level: number; x0: number; y0: number; x1: number; y1: number;
    detail: number; guards: { id: number; name: string }[]; onStation: number; paid: number;
  }
  interface Zoning {
    auto: boolean;
    treasury: number;
    clearWage: number;
    digWage: number;
    types: { name: string; zone: string }[];
    tiles: Record<string, number>;
    structures: { id: number; type: string; ship: boolean }[];
    clears: { id: number; x0: number; y0: number; x1: number; y1: number; level: number; tiles: number; done: number; held: number }[];
    waiting: { name: string; zone: string }[];
    defense: Defense[];
    guardWage: number;
    armed: number;
    maxDefense: number;
    maxDetail: number;
  }

  $effect(() => subscribe('zoning'));
  // Leaving the tab puts the tool down and clears the tint it left on the map.
  $effect(() => () => clearZoneTool());

  const zones = $derived(topics.data.zones as Zones | undefined);
  const z = $derived(topics.data.zoning as Zoning | undefined);
  // The structures the preview reads change as the colony builds: estimate again.
  $effect(() => { if (z) untrack(zoneChanged); });

  const kinds = $derived(zones?.kinds.slice(1) ?? []);
  const r = $derived(ui.zone.rect);
  const pv = $derived(ui.zone.preview);
  const tool = $derived(ui.zone.tool);
  const color = (name: string) => zones?.kinds.find((k) => k.name === name)?.color ?? 'transparent';
  // The fixtures a zone holds. A chest also stands beside a stove (its pantry) or in a ship.
  const holds = (zone: string) => (z?.types ?? []).filter((t) => t.zone === zone).map((t) => t.name).join(', ');

  const digCost = $derived(z && pv ? pv.dig * z.digWage : 0);
  const clearCost = $derived(z && pv ? pv.clear * z.clearWage : 0);
  const cost = $derived(digCost + clearCost);

  const defenseColor = $derived(zones?.defenseColor ?? '#e53935');
  const area = $derived(r ? (r.x1 - r.x0 + 1) * (r.y1 - r.y0 + 1) : 0);

  // Why it cannot be applied, or null if it can.
  const blocked = $derived.by(() => {
    if (!z || !r || !pv) return null;
    if (tool === 'defense') {
      if (area > z.maxDefense) return `A defense zone can cover at most ${z.maxDefense} tiles; this area is ${area}.`;
      if (pv.tiles === 0) return 'The colony has seen nothing here to defend.';
      return null;
    }
    if (tool === 'clear' && pv.tiles === 0 && pv.rising === 0) return 'Nothing the colony has seen is built here.';
    if (tool !== 'clear' && pv.tiles === 0 && pv.dig === 0) {
      return pv.locked > 0 ? 'This ground is held as residence by the colony ship on it.' : `This area is already ${tool === 'none' ? 'unzoned' : `zoned ${tool}`}.`;
    }
    if (cost > z.treasury) return `The treasury (${money(z.treasury)}) cannot pay for the work.`;
    return null;
  });

  const evictedSummary = $derived.by(() => {
    if (!pv) return '';
    const counts = new Map<string, number>();
    for (const e of pv.evicted) counts.set(e.type, (counts.get(e.type) ?? 0) + 1);
    return [...counts].map(([t, n]) => (n === 1 ? `a ${t}` : `${n} ${t}s`)).join(', ');
  });

  const verb = $derived(tool === 'clear' ? `Clear for ${money(cost)}` : tool === 'none' ? 'Remove zoning'
    : tool === 'defense' ? `Post ${ui.zone.detail} guard${ui.zone.detail === 1 ? '' : 's'}` : `Zone ${tool}`);
  const toolLabel = (t: string) => (t === 'none' ? 'Remove zone' : t === 'clear' ? 'Clear area' : t);

  function detailInput(ev: Event) {
    const n = Math.round(Number((ev.currentTarget as HTMLInputElement).value));
    if (Number.isFinite(n)) ui.zone.detail = Math.max(1, Math.min(z?.maxDetail ?? 20, n));
  }

  // Zones are the landing level's (docs/zoning.md); below it only the
  // clear and defense tools work.
  const landing = $derived(ui.hello ? ui.level === ui.hello.landingLevel : true);

  function pick(t: string) {
    armZone(t, !(ui.zone.armed && tool === t));
  }
</script>

{#if z?.auto}
  <p class="muted">The colony is zoning for itself (zoning-auto): it builds where it chooses and zones each room as it marks it out, using your zones first.</p>
{:else}
  <p class="muted">Colonists build only inside a zone of the right kind: a room goes where its fixtures belong, and fixtures of one zone may share a room (a stove and an incubator are both production). Zoning is free; digging out rock in a new zone, and clearing a structure left in the wrong zone, are paid work.</p>
{/if}

<div class="tools">
  {#each kinds as k (k.name)}
    <button type="button" class:on={ui.zone.armed && tool === k.name} aria-pressed={ui.zone.armed && tool === k.name}
      disabled={!landing} onclick={() => pick(k.name)}>
      <span class="swatch" style:background={k.color}></span>{k.name}
    </button>
  {/each}
  <button type="button" class:on={ui.zone.armed && tool === 'defense'} aria-pressed={ui.zone.armed && tool === 'defense'}
    onclick={() => pick('defense')} title="Post armed colonists to keep an area safe">
    <span class="swatch" style:background={defenseColor}></span>defense
  </button>
  {#each ['none', 'clear'] as t (t)}
    <button type="button" class:on={ui.zone.armed && tool === t} aria-pressed={ui.zone.armed && tool === t}
      disabled={!landing && !anyLevelTool(t)} onclick={() => pick(t)}>
      {toolLabel(t)}
    </button>
  {/each}
</div>
{#if !landing}
  <p class="muted">Zones are painted on the landing level; on this level you can order structures cleared or post a defense detail.</p>
{/if}
{#if ui.zone.armed}
  <p class="muted">Drag on the map to mark the area ({toolLabel(tool)}).</p>
{/if}

{#if r && pv && z}
  <dl>
    <dt>Area</dt>
    <dd>
      <button type="button" class="link" onclick={() => centerOn((r.x0 + r.x1) >> 1, (r.y0 + r.y1) >> 1)}>
        {r.x0},{r.y0} to {r.x1},{r.y1}
      </button>
      <span class="muted">{r.x1 - r.x0 + 1}×{r.y1 - r.y0 + 1}</span>
    </dd>
    {#if tool === 'defense'}
      <dt>Seen</dt><dd>{pv.tiles} tiles</dd>
      <dt>Detail</dt>
      <dd>
        <input type="number" min="1" max={z.maxDetail} value={ui.zone.detail} oninput={detailInput} aria-label="Armed colonists to post" />
        armed colonist{ui.zone.detail === 1 ? '' : 's'}
      </dd>
      <dt>Wage</dt><dd>{money(z.guardWage)} an hour each, from the treasury</dd>
      <dt>Armed</dt><dd>{z.armed} colonist{z.armed === 1 ? '' : 's'} carry a weapon</dd>
    {:else if tool === 'clear'}
      <dt>To clear</dt><dd>{pv.tiles} tiles <span class="muted">at {money(z.clearWage)} a tile</span></dd>
    {:else}
      <dt>{tool === 'none' ? 'To unzone' : 'To zone'}</dt><dd>{pv.tiles} tiles</dd>
      {#if pv.dig > 0}<dt>Rock to dig</dt><dd>{pv.dig} tiles, {money(digCost)}</dd>{/if}
    {/if}
    {#if tool !== 'defense'}
      <dt>Price</dt><dd>{money(cost)}</dd>
    {/if}
    <dt>Treasury</dt><dd>{money(z.treasury)}</dd>
  </dl>
  {#if tool === 'defense'}
    <p class="muted">Nothing is built. Armed colonists looking for work take the posts and patrol the area, shooting any alien that comes near it; a guard who goes to eat or sleep leaves the post to the next.</p>
    {#if z.armed < ui.zone.detail}
      <p class="warn">Only {z.armed} colonist{z.armed === 1 ? '' : 's'} carry a weapon: the detail will be short until more are armed.</p>
    {/if}
  {/if}
  {#if pv.locked > 0}<p class="muted">{pv.locked} tiles round colony ships stay residence.</p>{/if}
  {#if pv.evicted.length > 0}
    <p class="warn">
      {evictedSummary} in this area will have to be cleared: an additional paid work order of up to
      {money(clearCost)} ({pv.clear} tiles; a wall shared with a room that stays is kept).
    </p>
  {/if}
  {#if tool === 'clear' && pv.rising > 0}
    <p class="warn">{pv.rising} room{pv.rising === 1 ? '' : 's'} going up here will be called off.</p>
  {/if}
  {#if blocked}<p class="warn">{blocked}</p>{/if}
  <div class="row">
    <button type="button" class="go" disabled={blocked !== null || ui.zone.armed} onclick={applyZone}>{verb}</button>
    <button type="button" onclick={clearZoneTool}>Cancel</button>
  </div>
{/if}

{#if z && !z.auto && z.waiting.length > 0}
  <h2>Waiting for a zone</h2>
  <ul class="lines">
    {#each z.waiting as w (w.name)}
      <li><span class="swatch" style:background={color(w.zone)}></span>The colony wants a {w.name}: no {w.zone} zone has room for one.</li>
    {/each}
  </ul>
{/if}

<h2>Zoned</h2>
{#if !z}
  <p class="muted">Loading…</p>
{:else}
  <ul class="lines">
    {#each kinds as k (k.name)}
      <li>
        <span class="swatch" style:background={k.color}></span><b>{k.name}</b> {z.tiles[k.name] ?? 0} tiles
        <div class="muted holds">{holds(k.name)}</div>
      </li>
    {/each}
  </ul>
{/if}

<h2>Defense zones</h2>
{#if !z}
  <p class="muted">Loading…</p>
{:else if z.defense.length === 0}
  <p class="muted">none posted</p>
{:else}
  <ul class="lines">
    {#each z.defense as d (d.id)}
      <li>
        <span class="swatch" style:background={defenseColor}></span>
        <button type="button" class="link" onclick={() => centerOn((d.x0 + d.x1) >> 1, (d.y0 + d.y1) >> 1, d.level)}>
          {d.x0},{d.y0} to {d.x1},{d.y1}{#if d.level !== ui.hello?.landingLevel}, level {d.level}{/if}
        </button>
        {d.guards.length}/{d.detail} posted, {d.onStation} on station
        <span class="stepper">
          <button type="button" disabled={d.detail <= 1} onclick={() => setDefenseDetail(d.id, d.detail - 1)} aria-label="One guard fewer">−</button>
          <button type="button" disabled={d.detail >= z.maxDetail} onclick={() => setDefenseDetail(d.id, d.detail + 1)} aria-label="One guard more">+</button>
        </span>
        <button type="button" class="cancel" onclick={() => cancelDefense(d.id)} title="Stand the detail down">Cancel</button>
        <div class="muted holds">
          {#if d.guards.length > 0}{d.guards.map((g) => g.name).join(', ')}{:else}waiting for armed colonists{/if}
          · {money(d.paid)} paid
        </div>
      </li>
    {/each}
  </ul>
{/if}

<h2>Open clearing orders</h2>
{#if !z}
  <p class="muted">Loading…</p>
{:else if z.clears.length === 0}
  <p class="muted">none open</p>
{:else}
  <ul class="lines">
    {#each z.clears as c (c.id)}
      <li>
        <button type="button" class="link" onclick={() => centerOn((c.x0 + c.x1) >> 1, (c.y0 + c.y1) >> 1, c.level)}>
          {c.x0},{c.y0} to {c.x1},{c.y1}{#if c.level !== ui.hello?.landingLevel}, level {c.level}{/if}
        </button>
        {c.done}/{c.tiles} cleared, {money(c.held)} held
        <button type="button" class="cancel" onclick={() => cancelClear(c.id)}
          title="Close the order and return {money(c.held)} to the treasury">Cancel</button>
      </li>
    {/each}
  </ul>
  <p class="muted">Digging out a new zone shows on the Dig tab.</p>
{/if}

<style>
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 16px 0 6px; }
  h2:first-child { margin-top: 4px; }
  p { margin: 0 0 8px; }
  .muted { color: var(--muted); }
  .warn { color: #ffb36b; }
  .tools { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0; }
  .tools button { text-transform: capitalize; display: inline-flex; align-items: center; gap: 6px; }
  .tools button.on { background: var(--accent); color: #fff; }
  .swatch { display: inline-block; width: 10px; height: 10px; border-radius: 2px; margin-right: 6px; vertical-align: -1px; }
  .tools .swatch { margin-right: 0; }
  .row { display: flex; gap: 6px; margin: 8px 0; }
  .row button.go { background: var(--accent); color: #fff; }
  .row button.go:disabled { background: transparent; color: inherit; }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 8px 0; }
  dt { color: var(--muted); }
  dd { margin: 0; font-variant-numeric: tabular-nums; }
  ul { list-style: none; padding: 0; margin: 0 0 8px; }
  .lines li { line-height: 1.5; font-size: 13px; }
  .holds { font-size: 12px; margin-left: 16px; }
  .cancel { margin-left: 6px; padding: 0 8px; font-size: 12px; }
  .stepper { margin-left: 6px; }
  .stepper button { padding: 0 6px; font-size: 12px; }
  input[type='number'] { width: 4em; }
  .link { border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer; font: inherit; }
  .link:hover { text-decoration: underline; }
</style>

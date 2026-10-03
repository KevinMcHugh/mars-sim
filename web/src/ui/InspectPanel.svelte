<script lang="ts">
  // The Inspect tab: whatever the map's click selected (ui.selected), from
  // the entity:<id> or tile:<x>,<y> topic (internal/wire/inspect.go). The
  // colonist view follows the TUI's roster inspector; the tile view its map
  // cursor's. Names of kin, acquaintances, owners and creatures on a tile
  // inspect them in turn.
  import { pickGlyph } from '../emoji';
  import { centerOn, inspect, selectionTopic, setPanel, subscribe, topics, ui } from '../game.svelte';
  import Bar from './Bar.svelte';
  import Section from './Section.svelte';
  import type { EntityInfo, TileInfo } from './inspect';

  // The tabs that open the inspector, for its way back.
  const backLabels: Record<string, string> = { roster: 'Roster', jobs: 'Jobs', storage: 'Storage', market: 'Market' };

  const topic = $derived(ui.selected ? selectionTopic(ui.selected) : null);
  $effect(() => {
    if (topic) return subscribe(topic);
  });
  const data = $derived(topic ? topics.data[topic] : undefined);
  const entity = $derived(ui.selected && 'entity' in ui.selected ? (data as EntityInfo | undefined) : undefined);
  const tile = $derived(ui.selected && 'tile' in ui.selected ? (data as TileInfo | undefined) : undefined);
  const c = $derived(entity?.colonist);

  function memoryTick(m: { tick: number; lastTick: number; count: number }): string {
    return m.count > 1 ? `t${m.tick}–${m.lastTick} (×${m.count})` : `t${m.tick}`;
  }
</script>

{#if ui.inspectFrom && backLabels[ui.inspectFrom]}
  <button type="button" class="link back" onclick={() => setPanel(ui.inspectFrom)}>← {backLabels[ui.inspectFrom]}</button>
{/if}
{#if !ui.selected}
  <p class="muted">Click a colonist, a creature or a tile on the map, or pick one from the Roster.</p>
{:else if !data}
  <p class="muted">Loading…</p>
{:else if entity}
  {#if !entity.found}
    <p class="muted">Creature #{entity.id} is gone: it died and has left the records.</p>
  {:else}
    <header>
      <h3><span class="glyph">{pickGlyph(entity.look, entity.glyph)}</span> {entity.name}</h3>
      {#if !entity.dead}
        <button type="button" class="small" title="Center the map on it" onclick={() => centerOn(entity.x, entity.y)}>Find</button>
      {/if}
    </header>
    {#if c}
      {#if c.professionLabel}<p class="profession">{c.professionLabel}</p>{/if}
      <p class="sub">{c.pronouns} · {c.orientation}</p>
      <p class="sub">age {c.age} · {c.height} ({c.heightCm} cm) · {c.weightKg} kg</p>
      <p class="sub">{c.skin} skin · {c.hair} hair · ${c.wallet.toLocaleString()}</p>
      {#if c.backstory}<p class="sub">{c.backstory}</p>{/if}
    {:else}
      <p class="sub">{[entity.kind, `(${entity.x}, ${entity.y})`, entity.species].filter(Boolean).join(' · ')}</p>
    {/if}

    <Section id="inspect.status" title="Status">
      {#if entity.dead}
        <p>Dead at tick {entity.diedTick} — {entity.cause}</p>
      {:else}
        <p>{entity.state}{entity.focus && entity.focus !== 'idle' ? ` · focus: ${entity.focus}` : ''}</p>
        <div class="bars"><Bar label="health" value={entity.hp} max={entity.maxHp} /></div>
      {/if}
      {#if c}
        <div class="bars">
          <Bar label="charge" value={c.mood.charge} max={c.mood.max} diverging title="Activation" />
          <Bar label="grip" value={c.mood.grip} max={c.mood.max} diverging title="Control" />
          <Bar label="valence" value={c.mood.valence} max={c.mood.max} diverging title="How life has been going" />
        </div>
        <p class="sub">feeling {c.mood.label}</p>
      {/if}
    </Section>

    {#if entity.parts.length > 0}
      <Section id="inspect.body" title="Body">
        <div class="bars">
          {#each entity.parts as p (p.name)}<Bar label={p.name} value={p.hp} max={p.max} />{/each}
        </div>
      </Section>
    {/if}

    {#if c}
      <Section id="inspect.needs" title="Needs">
        <div class="bars">
          {#each c.needs as n (n.name)}
            <Bar label={n.name + (n.fatal ? '!' : '')} value={n.value} max={n.max} danger={n.fatal}
              title={n.fatal ? 'Fatal when full' : undefined} />
          {/each}
        </div>
      </Section>

      <Section id="inspect.inventory" title="Inventory">
        {#if c.inventory.length === 0}
          <p class="muted">empty</p>
        {:else}
          <ul class="plain">
            {#each c.inventory as s (s.slot)}<li>{s.item} ×{s.count}</li>{/each}
          </ul>
          {#if c.inventory.length < c.slots}<p class="muted">{c.slots - c.inventory.length} empty slots</p>{/if}
        {/if}
      </Section>

      <Section id="inspect.traits" title="Traits">
        {#if c.traits.length === 0}
          <p class="muted">none — steady and average</p>
        {:else}
          <dl class="traits">
            {#each c.traits as t (t.name)}<dt>{t.name}</dt><dd>{t.desc}</dd>{/each}
          </dl>
        {/if}
      </Section>

      <Section id="inspect.skills" title="Skills">
        {#if c.skills.length === 0}
          <p class="muted">untrained</p>
        {:else}
          <div class="bars">
            {#each c.skills as sk (sk.name)}
              <div class="skill" class:trade={sk.name === c.profession}>
                <Bar label={(sk.name === c.profession ? '★ ' : '') + sk.name} value={sk.rank} max={sk.maxRank}
                  title={`${sk.practice.toLocaleString()} ticks of practice`} />
                <span class="skill-label">{sk.label}</span>
              </div>
            {/each}
          </div>
        {/if}
      </Section>

      <Section id="inspect.family" title="Family">
        {#if c.family.length === 0}
          <p class="muted">no known kin</p>
        {:else}
          <ul class="plain">
            {#each c.family as k (k.relation + k.id)}
              <li>{k.relation} — <button type="button" class="link" onclick={() => inspect({ entity: k.id }, ui.inspectFrom)}>{k.name || `#${k.id}`}</button></li>
            {/each}
          </ul>
        {/if}
      </Section>

      <Section id="inspect.affinities" title="Affinities">
        {#if c.affinities.length === 0}
          <p class="muted">no acquaintances yet</p>
        {:else}
          <div class="bars">
            {#each c.affinities as a (a.id)}
              <div class="aff">
                <button type="button" class="link" onclick={() => inspect({ entity: a.id }, ui.inspectFrom)}>{a.name || `#${a.id}`}</button>
                <Bar label="" value={a.value} max={c.affinityMax} diverging />
              </div>
            {/each}
          </div>
        {/if}
      </Section>

      <Section id="inspect.memories" title={`Memories (${c.memories.length})`}>
        {#if c.memories.length === 0}
          <p class="muted">no memories yet</p>
        {:else}
          <ul class="memories">
            {#each c.memories as m, i (i)}<li><span class="tick">{memoryTick(m)}</span> {m.text}</li>{/each}
          </ul>
        {/if}
      </Section>
    {/if}
  {/if}
{:else if tile}
  <header>
    <h3>{#if tile.glyph}<span class="glyph">{tile.glyph}</span>{/if}{tile.glyph ? ' ' : ''}{tile.explored ? tile.terrain : 'Unexplored'}</h3>
  </header>
  <p class="sub">({tile.x}, {tile.y})</p>
  {#if tile.fixture}
    <Section id="inspect.fixture" title="Fixture">
      <dl>
        <dt>Owner</dt>
        <dd>
          {#if tile.fixture.ownerId}
            <button type="button" class="link" onclick={() => inspect({ entity: tile.fixture!.ownerId! }, ui.inspectFrom)}>{tile.fixture.owner}</button>
          {:else}{tile.fixture.owner}{/if}
        </dd>
        <dt>Access</dt><dd>{tile.fixture.access}{#if tile.fixture.price}, ${tile.fixture.price} a use{/if}</dd>
      </dl>
    </Section>
  {/if}
  {#if tile.storage}
    {@const st = tile.storage}
    <Section id="inspect.storage" title={st.label}>
      <p class="sub">{st.used}/{st.slots} slots · {st.items}/{st.capacity} items</p>
      {#if st.contents.length === 0}
        <p class="muted">empty</p>
      {:else}
        <ul class="plain">
          {#each st.contents as s (s.slot)}<li><span class="tick">{s.slot}</span> {s.item} ×{s.count}</li>{/each}
        </ul>
      {/if}
    </Section>
    <Section id="inspect.owners" title="Owned by">
      {#if st.ledger.length === 0}
        <p class="muted">nobody (empty)</p>
      {:else}
        <ul class="plain">
          {#each st.ledger as l, i (i)}<li>{l.owner}: {l.item} ×{l.count}</li>{/each}
        </ul>
      {/if}
    </Section>
  {/if}
  {#if tile.creatures.length > 0}
    <Section id="inspect.here" title="Here">
      <ul class="plain">
        {#each tile.creatures as cr (cr.id)}
          <li><button type="button" class="link" onclick={() => inspect({ entity: cr.id }, ui.inspectFrom)}>{pickGlyph(cr.look, cr.glyph)} {cr.name}</button> · {cr.state}</li>
        {/each}
      </ul>
    </Section>
  {/if}
{/if}

<style>
  header { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  h3 { font-size: 16px; margin: 0 0 4px; }
  .glyph { font-size: 20px; }
  p { margin: 0 0 2px; line-height: 1.45; }
  .sub, .muted { color: var(--muted); }
  .bars { display: grid; gap: 4px; margin: 4px 0; }
  .aff { display: grid; grid-template-columns: 8em 1fr; align-items: center; gap: 4px; }
  .aff :global(.bar) { grid-template-columns: 0 1fr 3.5em; }
  .aff .link { text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 0; }
  dt { color: var(--muted); }
  dd { margin: 0; }
  .profession { color: #e9c46a; }
  .skill-label { display: block; color: var(--muted); font-size: 12px; padding-left: 6.5em; margin-left: 8px; }
  .skill.trade :global(.label) { color: #e9c46a; }
  .traits { grid-template-columns: 1fr; gap: 0; }
  .traits dt { color: #e9c46a; margin-top: 4px; }
  .traits dd { color: var(--muted); }
  ul { list-style: none; padding: 0; margin: 0; }
  .plain li { line-height: 1.5; }
  .memories li { line-height: 1.4; margin-bottom: 4px; }
  .tick { color: var(--muted); font-variant-numeric: tabular-nums; margin-right: 4px; }
  .link {
    border: none; background: none; padding: 0; color: #8fd0ff; cursor: pointer;
  }
  .link:hover { text-decoration: underline; }
  .small { padding: 2px 8px; font-size: 12px; }
  .back { display: block; margin-bottom: 8px; }
</style>

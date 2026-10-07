<script lang="ts">
  // The Lore tab: world facts, the rolled alien species, and the arms makers
  // behind the colony's guns (docs/arms-makers.md), from the "lore"
  // topic (internal/wire/topics.go), as the TUI's lore tab shows them.
  import { subscribe, topics } from '../game.svelte';
  import Section from './Section.svelte';

  interface Species {
    // label is the TUI's roster label, emoji included ("🦗 Bug · hostile").
    label: string; glyph: string; singular: string; plural: string; scientificName: string; temperament: string;
    heightMinCm: number; heightMaxCm: number; weightMinKg: number; weightMaxKg: number;
    eyes: number; limbs: number; arms: number; legs: number; tail: boolean; wings: boolean;
    skin: string; color: string; pattern: string;
    attacks: string; biteDamage: number; biteRest: number; slowness: number; description: string;
    // etymology is the binomial taken apart into its word parts (docs/alien-taxonomy.md).
    etymology?: { part: 'prefix' | 'root' | 'epithet'; form: string; meaning: string }[];
  }
  interface Lore {
    world: { width: number; height: number; fogOfWar: boolean; exploredTiles: number; chunksGenerated: number; chunks: number; seed: number };
    species: Species[];
    guns: { kind: string; maker: string; model: string }[];
    corporations: { name: string; hq: string; founded: number; description: string }[];
  }

  $effect(() => subscribe('lore'));
  const lore = $derived(topics.data.lore as Lore | undefined);

  let selected = $state(0);
  const sp = $derived(lore?.species[Math.min(selected, lore.species.length - 1)]);

  // The (?) beside the binomial: a click/tap disclosure rather than a hover
  // tooltip, so it works on touch. It stays open while browsing species.
  let showEtymology = $state(false);
  const elided = $derived.by(() => {
    const [p, r] = sp?.etymology ?? [];
    return p && r && /[aeiou]$/.test(p.form) && /^[aeiou]/.test(r.form) ? p.form.slice(-1) : '';
  });
  const affix = (g: { part: string; form: string }) =>
    g.part === 'prefix' ? `${g.form}-` : g.part === 'root' ? `-${g.form}` : g.form;

  const area = $derived(lore ? lore.world.width * lore.world.height : 0);
  const explored = $derived(lore && area > 0 ? Math.floor((lore.world.exploredTiles * 100) / area) : 0);
</script>

{#if !lore}
  <p class="muted">Loading…</p>
{:else}
  <Section id="lore.world" tag="h2" title="World">
    <dl>
      <dt>Size</dt><dd>{lore.world.width.toLocaleString()} × {lore.world.height.toLocaleString()} ({area.toLocaleString()} tiles)</dd>
      <dt>Explored</dt>
      <dd>
        {#if lore.world.fogOfWar}{explored}% ({lore.world.exploredTiles.toLocaleString()} tiles){:else}100% (fog off){/if}
      </dd>
      {#if lore.world.chunks > 0}
        <dt>Generated</dt><dd>{lore.world.chunksGenerated.toLocaleString()} of {lore.world.chunks.toLocaleString()} chunks</dd>
      {/if}
      <dt>Seed</dt><dd><code>{lore.world.seed}</code></dd>
    </dl>
  </Section>

  <Section id="lore.species" tag="h2" title={`Alien species (${lore.species.length})`}>
    {#if lore.species.length === 0}
      <p class="muted">None rolled.</p>
    {:else}
      <ul class="list">
        {#each lore.species as s, i (s.label + i)}
          <li>
            <button type="button" class:on={sp === s} onclick={() => (selected = i)}>{s.label}</button>
          </li>
        {/each}
      </ul>
      {#if sp}
        <h3>{sp.label}</h3>
        {#if sp.scientificName}
          <p class="binomial">
            {sp.scientificName}
            {#if sp.etymology?.length}
              <button
                type="button" class="why" aria-expanded={showEtymology} aria-controls="lore-etymology"
                title="What does the name mean?" aria-label="What does the name mean?"
                onclick={() => (showEtymology = !showEtymology)}>?</button>
            {/if}
          </p>
          {#if showEtymology && sp.etymology?.length}
            <div class="etymology" id="lore-etymology">
              <dl>
                {#each sp.etymology as g (g.part)}
                  <dt><i>{affix(g)}</i></dt><dd>{g.meaning}</dd>
                {/each}
              </dl>
              <p class="muted">
                <i>{sp.scientificName.split(' ')[0]}</i> is
                <i>{sp.etymology[0].form}</i> + <i>{sp.etymology[1].form}</i>{#if elided}, dropping the
                  “{elided}” before a vowel{/if}; <i>{sp.etymology[2].form}</i> is the species epithet.
              </p>
            </div>
          {/if}
        {/if}
        <dl>
          <dt>Height</dt><dd>{sp.heightMinCm}–{sp.heightMaxCm} cm</dd>
          <dt>Weight</dt><dd>{sp.weightMinKg}–{sp.weightMaxKg} kg</dd>
          <dt>Eyes</dt><dd>{sp.eyes}</dd>
          <dt>Limbs</dt><dd>{sp.limbs} ({sp.arms} arms, {sp.legs} legs)</dd>
          <dt>Tail</dt><dd>{sp.tail ? 'yes' : 'no'}</dd>
          <dt>Wings</dt><dd>{sp.wings ? 'yes' : 'no'}</dd>
          <dt>Skin</dt><dd>{sp.skin}</dd>
          <dt>Color</dt><dd>{sp.color}</dd>
          <dt>Pattern</dt><dd>{sp.pattern}</dd>
          <dt>Attacks</dt><dd>{sp.attacks}</dd>
          <dt>Attack damage</dt><dd>{sp.biteDamage}</dd>
          <dt>Attack pace</dt><dd>{sp.biteRest} ticks</dd>
          <dt>Move pace</dt><dd>every {sp.slowness} ticks</dd>
        </dl>
        <h4>Field notes</h4>
        <p>{sp.description}</p>
      {/if}
    {/if}
  </Section>

  {#if lore.guns?.length}
    <Section id="lore.guns" tag="h2" title="Guns">
      <dl>
        {#each lore.guns as g (g.kind)}
          <dt>{g.kind}</dt><dd>{g.maker} {g.model}</dd>
        {/each}
      </dl>
    </Section>
  {/if}

  {#if lore.corporations?.length}
    <Section id="lore.corporations" tag="h2" title={`Corporations (${lore.corporations.length})`}>
      {#each lore.corporations as c (c.name)}
        <p class="corp">{c.description}</p>
      {/each}
    </Section>
  {/if}
{/if}

<style>
  h3 { font-size: 15px; margin: 14px 0 8px; }
  h3:has(+ .binomial) { margin-bottom: 2px; }
  .binomial { font-style: italic; color: var(--muted); margin: 0 0 8px; display: flex; align-items: center; gap: 6px; }
  /* A round (?) with a touch-sized hit area around a small glyph. */
  .why {
    font-style: normal; font-size: 11px; line-height: 1; width: 18px; height: 18px; padding: 0;
    border-radius: 50%; border: 1px solid var(--line); background: transparent; color: var(--muted);
    position: relative; cursor: pointer;
  }
  .why::after { content: ''; position: absolute; inset: -10px; }
  .why[aria-expanded='true'] { color: var(--accent); border-color: var(--accent); }
  .etymology { border-left: 2px solid var(--line); padding: 2px 0 2px 10px; margin: -2px 0 12px; }
  .etymology dl { margin: 0 0 6px; }
  .etymology dd { font-variant-numeric: normal; }
  .etymology p { font-size: 12px; }
  h4 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 12px 0 4px; }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 0 0 16px; }
  dt { color: var(--muted); }
  dd { margin: 0; font-variant-numeric: tabular-nums; }
  .list { list-style: none; padding: 0; margin: 0; display: grid; gap: 2px; }
  .list button { width: 100%; text-align: left; border-color: transparent; background: transparent; }
  .list button.on { background: rgba(255, 255, 255, 0.1); border-color: var(--line); }
  p { margin: 0; line-height: 1.5; }
  .corp + .corp { margin-top: 8px; }
  .muted { color: var(--muted); }
</style>

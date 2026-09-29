<script lang="ts">
  // The Lore tab: world facts and the rolled alien species, from the "lore"
  // topic (internal/wire/topics.go), as the TUI's lore tab shows them.
  import { subscribe, topics } from '../game.svelte';

  interface Species {
    // label is the TUI's roster label, emoji included ("🦗 Bug · hostile").
    label: string; glyph: string; singular: string; plural: string; temperament: string;
    heightMinCm: number; heightMaxCm: number; weightMinKg: number; weightMaxKg: number;
    eyes: number; limbs: number; arms: number; legs: number; tail: boolean;
    skin: string; color: string; pattern: string;
    biteDamage: number; biteRest: number; slowness: number; description: string;
  }
  interface Lore {
    world: { width: number; height: number; fogOfWar: boolean; exploredTiles: number; chunksGenerated: number; chunks: number; seed: number };
    species: Species[];
  }

  $effect(() => subscribe('lore'));
  const lore = $derived(topics.data.lore as Lore | undefined);

  let selected = $state(0);
  const sp = $derived(lore?.species[Math.min(selected, lore.species.length - 1)]);

  const area = $derived(lore ? lore.world.width * lore.world.height : 0);
  const explored = $derived(lore && area > 0 ? Math.floor((lore.world.exploredTiles * 100) / area) : 0);
</script>

{#if !lore}
  <p class="muted">Loading…</p>
{:else}
  <h2>World</h2>
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

  <h2>Alien species ({lore.species.length})</h2>
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
      <dl>
        <dt>Height</dt><dd>{sp.heightMinCm}–{sp.heightMaxCm} cm</dd>
        <dt>Weight</dt><dd>{sp.weightMinKg}–{sp.weightMaxKg} kg</dd>
        <dt>Eyes</dt><dd>{sp.eyes}</dd>
        <dt>Limbs</dt><dd>{sp.limbs} ({sp.arms} arms, {sp.legs} legs)</dd>
        <dt>Tail</dt><dd>{sp.tail ? 'yes' : 'no'}</dd>
        <dt>Skin</dt><dd>{sp.skin}</dd>
        <dt>Color</dt><dd>{sp.color}</dd>
        <dt>Pattern</dt><dd>{sp.pattern}</dd>
        <dt>Bite damage</dt><dd>{sp.biteDamage}</dd>
        <dt>Bite cooldown</dt><dd>{sp.biteRest} ticks</dd>
        <dt>Move pace</dt><dd>every {sp.slowness} ticks</dd>
      </dl>
      <h4>Field notes</h4>
      <p>{sp.description}</p>
    {/if}
  {/if}
{/if}

<style>
  h2 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 4px 0 8px; }
  h2 + dl { margin-top: 0; }
  h3 { font-size: 15px; margin: 14px 0 8px; }
  h4 { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 12px 0 4px; }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 0 0 16px; }
  dt { color: var(--muted); }
  dd { margin: 0; font-variant-numeric: tabular-nums; }
  .list { list-style: none; padding: 0; margin: 0; display: grid; gap: 2px; }
  .list button { width: 100%; text-align: left; border-color: transparent; background: transparent; }
  .list button.on { background: rgba(255, 255, 255, 0.1); border-color: var(--line); }
  p { margin: 0; line-height: 1.5; }
  .muted { color: var(--muted); }
</style>

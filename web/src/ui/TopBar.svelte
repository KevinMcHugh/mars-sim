<script lang="ts">
  // The top bar: the clock, the speed selector, and the TUI header's counts
  // (creatures, fixtures, refuse, rooms, excavated). Refreshes at UI_HZ.
  import { ui } from '../game.svelte';
  import SpeedControl from './SpeedControl.svelte';
  import FlowControl from './FlowControl.svelte';

  // Creatures: their generic glyph (glyphs.ForKind), as the TUI's header shows them.
  const creatures: [stat: string, kind: string][] = [
    ['Colonists', 'colonist'], ['Aliens', 'alien'], ['Cats', 'cat'], ['Rats', 'rat'],
  ];
  // Fixtures: their map glyph, looked up by terrain name through the Hello.
  const fixtures: [stat: string, terrain: string][] = [
    ['Pods', 'nutrient pod'], ['Toilets', 'toilet'], ['Beds', 'bed'],
    ['Incinerators', 'incinerator'], ['StorageContainers', 'storage container'],
  ];

  function kindGlyph(name: string): string {
    const h = ui.hello;
    if (!h) return '';
    const g = h.glyphs.kinds[h.enums.kinds.indexOf(name)] ?? -1;
    return g >= 0 ? h.glyphs.symbols[g] : '';
  }

  function terrainGlyph(name: string): string {
    const h = ui.hello;
    if (!h) return '';
    const g = h.glyphs.terrain[h.enums.terrains.indexOf(name)] ?? -1;
    return g >= 0 ? h.glyphs.symbols[g] : '';
  }
  const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`;
</script>

<header class="hud top">
  <strong class="title">mars-sim</strong>
  <span class="readout">tick {ui.tick.toLocaleString()}</span>
  <SpeedControl />
  <FlowControl />
  {#if ui.hello}
    <span class="counts">
      {#each creatures as [stat, kind] (stat)}
        <span title={`${kind}s`}>{kindGlyph(kind)} {ui.stats[stat] ?? 0}</span>
      {/each}
      {#each fixtures as [stat, terrain] (stat)}
        <span title={`${terrain}s`}>{terrainGlyph(terrain)} {ui.stats[stat] ?? 0}</span>
      {/each}
      <span title="refuse: stains and bodies waiting to be cleaned up">{ui.hello.glyphs.symbols[ui.hello.glyphs.corpse]} {ui.stats.Refuse ?? 0}</span>
      <span>{plural(ui.stats.Rooms ?? 0, 'room')}</span>
      <span title="tiles of floor dug out">{(ui.stats.FloorDug ?? 0).toLocaleString()} dug</span>
    </span>
  {/if}
</header>

<style>
  .top {
    top: max(10px, env(safe-area-inset-top));
    left: 10px;
    right: calc(var(--panel-reserve) + 20px);
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 14px;
    padding: 7px 12px;
    width: max-content;
    max-width: calc(100vw - var(--panel-reserve) - 30px);
  }
  .title { letter-spacing: 0.02em; }
  .readout { font-variant-numeric: tabular-nums; color: var(--muted); min-width: 10ch; }
  .counts { display: flex; gap: 12px; flex-wrap: wrap; font-variant-numeric: tabular-nums; }

  /* A phone: the panel's tabs move to the bottom sheet, so the bar can span the top. */
  @media (max-width: 700px) {
    .top { right: 10px; max-width: calc(100vw - 20px); }
  }
</style>

<script lang="ts">
  // The top bar: the clock (the colony day and time, then the raw tick), the speed selector, and the colonist and alien
  // counts. Refreshes at UI_HZ.
  import { ui } from '../game.svelte';
  import SpeedControl from './SpeedControl.svelte';
  import FlowControl from './FlowControl.svelte';
  import LevelControl from './LevelControl.svelte';
  import FollowChip from './FollowChip.svelte';
  import { clock } from './format';

  // Only the colonist and alien counts: the rest (pets, fixtures, refuse, rooms, dug) was noise up here.
  // Glyphs are the kinds' generic ones (glyphs.ForKind), as the TUI's header shows them.
  const creatures: [stat: string, kind: string][] = [['Colonists', 'colonist'], ['Aliens', 'alien']];

  function kindGlyph(name: string): string {
    const h = ui.hello;
    if (!h) return '';
    const g = h.glyphs.kinds[h.enums.kinds.indexOf(name)] ?? -1;
    return g >= 0 ? h.glyphs.symbols[g] : '';
  }
</script>

<header class="hud top">
  <strong class="title">mars-sim</strong>
  <span class="readout" title="days since landing, and the time of day: one day is one waking stretch plus a night's sleep, and the colony landed at 06:00 (docs/days.md)">day {(ui.stats.Day ?? 1).toLocaleString()} {clock(ui.stats.MinuteOfDay ?? 0)}</span>
  <span class="readout tick">tick {ui.tick.toLocaleString()}</span>
  <SpeedControl />
  <FlowControl />
  <LevelControl />
  <FollowChip />
  {#if ui.hello}
    <span class="counts">
      {#each creatures as [stat, kind] (stat)}
        <span title={`${kind}s`}>{kindGlyph(kind)} {ui.stats[stat] ?? 0}</span>
      {/each}
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
  .readout { font-variant-numeric: tabular-nums; }
  .readout.tick { color: var(--muted); min-width: 10ch; }
  .counts { display: flex; gap: 12px; flex-wrap: wrap; font-variant-numeric: tabular-nums; }

  /* A phone: the panel's tabs move to the bottom sheet, so the bar can span the top. */
  @media (max-width: 700px) {
    .top { right: 10px; max-width: calc(100vw - 20px); }
  }
</style>

<script lang="ts">
  // The top bar's "following" chip (docs/follow.md): who the camera follows,
  // a click to inspect them, and × to stop. main.ts moves the camera each
  // frame; this holds the creature's entity:<id> topic for what a frame
  // cannot say: that it went to another level (the map goes after it), or
  // that it died (following stops). Shown only while following.
  import { pickGlyph } from '../emoji';
  import { follow, followTo, inspect, subscribe, topics, ui } from '../game.svelte';
  import type { EntityInfo } from './inspect';

  const topic = $derived(ui.follow !== null ? `entity:${ui.follow}` : null);
  $effect(() => {
    if (topic) return subscribe(topic);
  });
  const e = $derived(topic ? (topics.data[topic] as EntityInfo | undefined) : undefined);

  $effect(() => {
    if (!e || e.id !== ui.follow) return;
    if (!e.found || e.dead) { follow(null); return; }
    if (e.level !== ui.level) followTo(e.x, e.y, e.level);
  });
</script>

{#if ui.follow !== null}
  <span class="chip">
    <button type="button" class="who" title="Inspect who the map is following" onclick={() => ui.follow !== null && inspect({ entity: ui.follow })}>
      👁 {#if e}<span class="glyph">{pickGlyph(e.look, e.glyph)}</span> {e.name}{:else}#{ui.follow}{/if}
    </button>
    <button type="button" class="close" title="Stop following (or pan the map)" aria-label="Stop following" onclick={() => follow(null)}>×</button>
  </span>
{/if}

<style>
  .chip { display: flex; align-items: center; gap: 2px; border: 1px solid var(--accent); border-radius: 14px; padding: 0 2px 0 4px; }
  .who { background: none; border: none; padding: 2px 4px; color: var(--fg); white-space: nowrap; max-width: 22ch; overflow: hidden; text-overflow: ellipsis; }
  .close { width: 22px; height: 22px; padding: 0; border: none; background: none; border-radius: 11px; line-height: 1; color: var(--muted); }
  .close:hover { color: var(--fg); }
</style>

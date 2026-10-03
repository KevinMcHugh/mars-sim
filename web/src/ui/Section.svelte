<script lang="ts">
  // A titled part of a tab that folds shut on a click of its heading. The
  // folded ones are remembered in this browser by id (ui.collapsed, saved by
  // toggleSection), not per colonist or per game: fold Skills once and it
  // stays shut on every inspector until it is opened again. Ids are
  // "<tab>.<section>", so the same title in two tabs folds separately.
  import type { Snippet } from 'svelte';
  import { toggleSection, ui } from '../game.svelte';

  let { id, title, note, tag = 'h4', children }: {
    id: string;
    title: string;
    /** Shown after the title, in plain case (e.g. "(chain depth 3)"). */
    note?: string;
    tag?: 'h2' | 'h4';
    children: Snippet;
  } = $props();
  const open = $derived(!ui.collapsed[id]);
</script>

<svelte:element this={tag} class="section-head">
  <button type="button" aria-expanded={open} onclick={() => toggleSection(id)}>
    <span class="caret" aria-hidden="true">{open ? '▾' : '▸'}</span>{title}{#if note}<span class="note"> {note}</span>{/if}
  </button>
</svelte:element>
{#if open}{@render children()}{/if}

<style>
  .section-head { font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted); margin: 14px 0 6px; }
  .section-head:first-child { margin-top: 4px; }
  button {
    border: none; background: none; padding: 0; border-radius: 0;
    font: inherit; color: inherit; text-transform: inherit; letter-spacing: inherit; text-align: left;
  }
  button:hover { color: var(--fg); }
  .caret { display: inline-block; width: 1.1em; }
  .note { text-transform: none; letter-spacing: 0; }
</style>

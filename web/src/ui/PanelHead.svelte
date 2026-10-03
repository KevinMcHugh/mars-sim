<script lang="ts">
  // A tab's title row, docked (SidePanel.svelte) or floating
  // (FloatingPanels.svelte): its name, then its buttons at the right.
  import type { Snippet } from 'svelte';

  let { label, children }: { label: string; children: Snippet } = $props();
</script>

<header class="panel-head">
  <span class="label">{label}</span>
  <span class="btns">{@render children()}</span>
</header>

<style>
  .panel-head {
    display: flex; align-items: center; gap: 6px; flex: none;
    padding: 5px 6px 3px 14px;
    font-size: 12px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--muted);
  }
  .label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .btns { display: flex; gap: 2px; }
  .btns :global(button) {
    width: 24px; height: 24px; padding: 0; border: none; background: none;
    color: var(--muted); font-size: 15px; line-height: 1;
  }
  .btns :global(button:hover) { color: var(--fg); background: rgba(255, 255, 255, 0.08); }
</style>

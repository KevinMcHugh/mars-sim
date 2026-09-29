<script lang="ts">
  // The side panel: one tab open at a time, beside the map, which stays
  // visible (docs/browser-frontend.md, "Parity with the TUI"). A closed panel
  // unmounts its tab, and with it the tab's topic subscription, so the worker
  // stops building data nobody is looking at.
  import LorePanel from './LorePanel.svelte';
  import NewGamePanel from './NewGamePanel.svelte';

  const tabs = [
    { id: 'lore', label: 'Lore', component: LorePanel },
    { id: 'game', label: 'New game', component: NewGamePanel },
  ] as const;
  type TabId = (typeof tabs)[number]['id'];

  let open: TabId | null = $state(null);
  const current = $derived(tabs.find((t) => t.id === open));

  function toggle(id: TabId) { open = open === id ? null : id; }

  $effect(() => {
    // Escape closes the panel, unless a field has the focus.
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && open && !(e.target instanceof HTMLInputElement)) open = null;
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
</script>

<aside class="side" class:open={open !== null}>
  <nav class="tabs hud" aria-label="Panels">
    {#each tabs as t (t.id)}
      <button type="button" class:on={open === t.id} aria-pressed={open === t.id} onclick={() => toggle(t.id)}>{t.label}</button>
    {/each}
  </nav>
  {#if current}
    <section class="body hud" aria-label={current.label}>
      <current.component />
    </section>
  {/if}
</aside>

<style>
  .side {
    position: fixed;
    top: max(10px, env(safe-area-inset-top));
    right: 10px;
    bottom: max(10px, env(safe-area-inset-bottom));
    width: var(--panel-width);
    display: flex;
    flex-direction: column;
    gap: 8px;
    pointer-events: none; /* the map stays draggable around a closed panel */
  }
  .tabs, .body { position: static; pointer-events: auto; }
  .tabs { display: flex; flex-wrap: wrap; gap: 4px; padding: 5px; align-self: flex-end; }
  .tabs button { border-color: transparent; background: transparent; }
  .tabs button.on { background: var(--accent); color: #fff; }
  .body { flex: 1; overflow: auto; padding: 12px 14px; min-height: 0; }

  /* A phone: the panel becomes a sheet along the bottom. */
  @media (max-width: 700px) {
    .side { top: auto; left: 10px; width: auto; max-height: 60vh; }
    .side:not(.open) { max-height: none; }
    .tabs { align-self: stretch; }
  }
</style>

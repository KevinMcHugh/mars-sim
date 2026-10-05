<script lang="ts">
  // The side panel: one tab open at a time, beside the map, which stays
  // visible (docs/browser-frontend.md, "Parity with the TUI"). A closed panel
  // unmounts its tab, and with it the tab's topic subscription, so the worker
  // stops building data nobody is looking at. Which tab is open is ui.panel,
  // so a click on the map can open the inspector. A tab can also be popped out
  // into a window over the map (FloatingPanels.svelte), to watch it beside
  // the docked one; its button then raises the window.
  import { isFloating, popOut, setPanel, ui } from '../game.svelte';
  import LogTicker from './LogTicker.svelte';
  import PanelHead from './PanelHead.svelte';
  import { groups, tabById } from './tabs';

  const current = $derived(ui.panel === null ? undefined : tabById.get(ui.panel));

  function toggle(id: string) { setPanel(ui.panel === id ? null : id); }

  $effect(() => {
    // Escape closes the panel, unless a field has the focus.
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && ui.panel && !(e.target instanceof HTMLInputElement)) setPanel(null);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });
</script>

<aside class="side" class:open={current !== undefined}>
  <LogTicker docked />
  <nav class="tabs hud" aria-label="Panels">
    {#each groups as g (g.id)}
      <div class="group {g.id}" role="group" aria-label={g.label}>
        <span class="glabel" aria-hidden="true">{g.label}</span>
        <div class="btns">
          {#each g.tabs as t (t.id)}
            {@const out = isFloating(t.id)}
            <button type="button" class:on={ui.panel === t.id} class:out aria-pressed={ui.panel === t.id || out}
              title={out ? `${t.label} is popped out: bring it to the front` : undefined}
              onclick={() => toggle(t.id)}>{t.label}</button>
          {/each}
        </div>
      </div>
    {/each}
  </nav>
  {#if current}
    <section class="body hud" aria-label={current.label}>
      <PanelHead label={current.label}>
        <button type="button" class="pop" title="Pop out into a window over the map" aria-label="Pop out {current.label}"
          onclick={() => popOut(current.id)}>⧉</button>
        <button type="button" title="Close (Esc)" aria-label="Close {current.label}" onclick={() => setPanel(null)}>×</button>
      </PanelHead>
      <div class="content"><current.component /></div>
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
  .tabs { display: flex; flex-direction: column; gap: 4px; padding: 5px; align-self: flex-end; }
  .group { display: flex; align-items: baseline; gap: 4px; }
  .btns { display: flex; flex-wrap: wrap; gap: 4px; }
  .group.act { border-top: 1px solid rgba(255, 255, 255, 0.12); padding-top: 4px; }
  .glabel { flex: none; width: 2.6em; font-size: 0.75em; text-transform: uppercase; letter-spacing: 0.05em; opacity: 0.55; }
  .tabs button { border-color: transparent; background: transparent; }
  .tabs button.on { background: var(--accent); color: #fff; }
  /* Popped out: open, but somewhere else. */
  .tabs button.out { border-color: var(--accent); border-style: dashed; }
  .body { flex: 1; display: flex; flex-direction: column; min-height: 0; }
  .content { flex: 1; overflow: auto; padding: 4px 14px 12px; min-height: 0; }

  /* A phone: the panel becomes a sheet along the bottom, and there is no room
     for windows over the map. */
  @media (max-width: 700px) {
    .side { top: auto; left: 10px; width: auto; max-height: 60vh; }
    .side:not(.open) { max-height: none; }
    .tabs { align-self: stretch; }
    .pop { display: none; }
  }
</style>

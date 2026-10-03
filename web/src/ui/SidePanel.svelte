<script lang="ts">
  // The side panel: one tab open at a time, beside the map, which stays
  // visible (docs/browser-frontend.md, "Parity with the TUI"). A closed panel
  // unmounts its tab, and with it the tab's topic subscription, so the worker
  // stops building data nobody is looking at. Which tab is open is ui.panel,
  // so a click on the map can open the inspector.
  import type { Component } from 'svelte';
  import { setPanel, ui } from '../game.svelte';
  import ChartsPanel from './ChartsPanel.svelte';
  import DigPanel from './DigPanel.svelte';
  import InspectPanel from './InspectPanel.svelte';
  import JobsPanel from './JobsPanel.svelte';
  import MarketPanel from './MarketPanel.svelte';
  import StoragePanel from './StoragePanel.svelte';
  import ZonesPanel from './ZonesPanel.svelte';
  import LogPanel from './LogPanel.svelte';
  import LogTicker from './LogTicker.svelte';
  import LorePanel from './LorePanel.svelte';
  import NewGamePanel from './NewGamePanel.svelte';
  import RosterPanel from './RosterPanel.svelte';
  import ShipsPanel from './ShipsPanel.svelte';

  // Two groups, one strip each: tabs that only show the colony, and tabs whose
  // job is to change it (arm a map tool, send a command). Market stays with the
  // information: its colony orders are a corner of a tab that is mostly prices.
  type Tab = { id: string; label: string; component: Component };
  const groups: { id: string; label: string; tabs: Tab[] }[] = [
    {
      id: 'info', label: 'View', tabs: [
        { id: 'inspect', label: 'Inspect', component: InspectPanel },
        { id: 'roster', label: 'Roster', component: RosterPanel },
        { id: 'log', label: 'Log', component: LogPanel },
        { id: 'jobs', label: 'Jobs', component: JobsPanel },
        { id: 'storage', label: 'Storage', component: StoragePanel },
        { id: 'market', label: 'Market', component: MarketPanel },
        { id: 'charts', label: 'Charts', component: ChartsPanel },
        { id: 'lore', label: 'Lore', component: LorePanel },
      ],
    },
    {
      id: 'act', label: 'Act', tabs: [
        { id: 'zones', label: 'Zones', component: ZonesPanel },
        { id: 'dig', label: 'Dig', component: DigPanel },
        { id: 'ships', label: 'Ships', component: ShipsPanel },
        { id: 'game', label: 'New game', component: NewGamePanel },
      ],
    },
  ];
  const tabs = groups.flatMap((g) => g.tabs);

  const current = $derived(tabs.find((t) => t.id === ui.panel));

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
            <button type="button" class:on={ui.panel === t.id} aria-pressed={ui.panel === t.id} onclick={() => toggle(t.id)}>{t.label}</button>
          {/each}
        </div>
      </div>
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
  .tabs { display: flex; flex-direction: column; gap: 4px; padding: 5px; align-self: flex-end; }
  .group { display: flex; align-items: baseline; gap: 4px; }
  .btns { display: flex; flex-wrap: wrap; gap: 4px; }
  .group.act { border-top: 1px solid rgba(255, 255, 255, 0.12); padding-top: 4px; }
  .glabel { flex: none; width: 2.6em; font-size: 0.75em; text-transform: uppercase; letter-spacing: 0.05em; opacity: 0.55; }
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

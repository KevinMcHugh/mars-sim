// The side panel's tabs, shared by the panel (SidePanel.svelte) and the
// windows a tab pops out into (FloatingPanels.svelte).
import type { Component } from 'svelte';
import ChartsPanel from './ChartsPanel.svelte';
import DigPanel from './DigPanel.svelte';
import InspectPanel from './InspectPanel.svelte';
import JobsPanel from './JobsPanel.svelte';
import LogPanel from './LogPanel.svelte';
import LorePanel from './LorePanel.svelte';
import MarketPanel from './MarketPanel.svelte';
import NewGamePanel from './NewGamePanel.svelte';
import RosterPanel from './RosterPanel.svelte';
import ShipsPanel from './ShipsPanel.svelte';
import StoragePanel from './StoragePanel.svelte';
import ZonesPanel from './ZonesPanel.svelte';

export type Tab = { id: string; label: string; component: Component };

// Two groups, one strip each: tabs that only show the colony, and tabs whose
// job is to change it (arm a map tool, send a command). Market stays with the
// information: its colony orders are a corner of a tab that is mostly prices.
export const groups: { id: string; label: string; tabs: Tab[] }[] = [
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
      { id: 'game', label: 'Game', component: NewGamePanel },
    ],
  },
];

export const tabs = groups.flatMap((g) => g.tabs);
export const tabById = new Map(tabs.map((t) => [t.id, t]));

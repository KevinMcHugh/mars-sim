// The side panel's tabs, shared by the panel (SidePanel.svelte) and the
// windows a tab pops out into (FloatingPanels.svelte).
import type { Component } from 'svelte';
import { ui } from '../game.svelte';
import ChartsPanel from './ChartsPanel.svelte';
import DigPanel from './DigPanel.svelte';
import InspectPanel from './InspectPanel.svelte';
import JobsPanel from './JobsPanel.svelte';
import LogPanel from './LogPanel.svelte';
import LorePanel from './LorePanel.svelte';
import MarketPanel from './MarketPanel.svelte';
import NewsPanel from './NewsPanel.svelte';
import NewGamePanel from './NewGamePanel.svelte';
import PerfPanel from './PerfPanel.svelte';
import PowerPanel from './PowerPanel.svelte';
import RecruitPanel from './RecruitPanel.svelte';
import RosterPanel from './RosterPanel.svelte';
import SandboxPanel from './SandboxPanel.svelte';
import ShipsPanel from './ShipsPanel.svelte';
import StoragePanel from './StoragePanel.svelte';
import TradePanel from './TradePanel.svelte';
import ZonesPanel from './ZonesPanel.svelte';

/** A side-panel tab; `when`, if given, says whether the strip shows it (the Sandbox tab, only in a sandbox). */
export type Tab = { id: string; label: string; component: Component; when?: () => boolean };

// Two groups, one strip each: tabs that only show the colony, and tabs whose
// job is to change it (arm a map tool, send a command). The market is split
// the same way: Market shows prices, books and accounts; Trade places the
// colony's orders.
export const groups: { id: string; label: string; tabs: Tab[] }[] = [
  {
    id: 'info', label: 'View', tabs: [
      { id: 'inspect', label: 'Inspect', component: InspectPanel },
      { id: 'roster', label: 'Roster', component: RosterPanel },
      { id: 'log', label: 'Log', component: LogPanel },
      { id: 'news', label: 'News', component: NewsPanel },
      { id: 'jobs', label: 'Jobs', component: JobsPanel },
      { id: 'storage', label: 'Storage', component: StoragePanel },
      { id: 'market', label: 'Market', component: MarketPanel },
      { id: 'power', label: 'Power', component: PowerPanel },
      { id: 'charts', label: 'Charts', component: ChartsPanel },
      { id: 'perf', label: 'Perf', component: PerfPanel },
      { id: 'lore', label: 'Lore', component: LorePanel },
    ],
  },
  {
    id: 'act', label: 'Act', tabs: [
      { id: 'zones', label: 'Zones', component: ZonesPanel },
      { id: 'dig', label: 'Dig', component: DigPanel },
      { id: 'ships', label: 'Ships', component: ShipsPanel },
      { id: 'trade', label: 'Trade', component: TradePanel },
      { id: 'recruit', label: 'Recruit', component: RecruitPanel },
      { id: 'sandbox', label: 'Sandbox', component: SandboxPanel, when: () => !!ui.hello?.sandbox },
      { id: 'game', label: 'Game', component: NewGamePanel },
    ],
  },
];

export const tabs = groups.flatMap((g) => g.tabs);
export const tabById = new Map(tabs.map((t) => [t.id, t]));

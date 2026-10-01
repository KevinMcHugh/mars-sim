<script lang="ts">
  // The Log tab: the colony log, oldest first, as the TUI's log tab shows it
  // (a type column beside each sentence, wrapped, never cut short), from
  // colonyLog (the log topic, held for the page's life in main.ts).
  //
  // It follows the live tail while scrolled to the bottom. Scrolled back, it
  // stays put and counts what arrived meanwhile.
  import { tick } from 'svelte';
  import { colonyLog, LOG_KEEP, setTicker, ui } from '../game.svelte';
  import { logKindColor } from './logkinds';

  let kind = $state('');
  let query = $state('');
  const kinds = $derived([...new Set(colonyLog.lines.map((l) => l.kind))].sort());
  const lines = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return colonyLog.lines.filter((l) => (!kind || l.kind === kind) && (!q || l.text.toLowerCase().includes(q)));
  });

  let list: HTMLDivElement | undefined = $state();
  let live = $state(true);
  // The newest seq when the reader scrolled away, for the "new" count.
  let leftAt = $state(-1);
  const unseen = $derived(live ? 0 : lines.filter((l) => l.seq > leftAt).length);

  function atBottom(el: HTMLElement): boolean {
    return el.scrollHeight - el.scrollTop - el.clientHeight < 8;
  }
  function onScroll(): void {
    if (!list) return;
    const bottom = atBottom(list);
    if (live && !bottom) leftAt = lines.at(-1)?.seq ?? -1;
    live = bottom;
  }
  async function toBottom(): Promise<void> {
    await tick();
    if (list) list.scrollTop = list.scrollHeight;
    live = true;
  }

  // Follow the tail: new lines (or a new filter) keep a live view at the bottom.
  $effect(() => {
    void lines;
    if (live) void toBottom();
  });
</script>

<div class="log">
  <div class="controls">
    <select bind:value={kind} aria-label="Type">
      <option value="">All types</option>
      {#each kinds as k (k)}<option value={k}>{k}</option>{/each}
    </select>
    <input class="search" type="search" placeholder="Search" bind:value={query} aria-label="Search the log" />
  </div>
  <label class="ticker-opt">
    <input type="checkbox" checked={ui.ticker} onchange={(e) => setTicker(e.currentTarget.checked)} />
    Show new lines on the map
  </label>
  <p class="count">
    {lines.length === colonyLog.lines.length ? lines.length : `${lines.length} of ${colonyLog.lines.length}`}
    {colonyLog.lines.length === 1 ? 'line' : 'lines'}{colonyLog.lines.length >= LOG_KEEP ? ` (the last ${LOG_KEEP})` : ''}
    {#if live}<span class="live">· live</span>{/if}
  </p>

  <div class="list" bind:this={list} onscroll={onScroll}>
    {#if lines.length === 0}
      <p class="muted">{colonyLog.lines.length === 0 ? 'Nothing logged yet.' : 'Nothing matches.'}</p>
    {/if}
    {#each lines as l (l.seq)}
      <div class="line" class:odd={l.seq % 2 === 1}>
        <span class="kind" style="color: {logKindColor(l.kind)}">{l.kind}</span>
        <span class="tick">t{l.tick}</span>
        <span class="text">{l.text}</span>
      </div>
    {/each}
  </div>
  {#if !live}
    <button type="button" class="jump" onclick={toBottom}>↓ {unseen > 0 ? `${unseen} new` : 'Latest'}</button>
  {/if}
</div>

<style>
  .log { display: flex; flex-direction: column; height: 100%; min-height: 0; position: relative; }
  .controls { display: flex; gap: 8px; }
  .search { flex: 1; min-width: 6em; }
  .ticker-opt { display: flex; align-items: center; gap: 6px; margin-top: 8px; font-size: 12px; color: var(--muted); cursor: pointer; }
  .ticker-opt input { accent-color: var(--accent); padding: 0; }
  .count { color: var(--muted); margin: 8px 0 4px; font-size: 12px; }
  .live { color: #7fcf96; }
  .list { flex: 1; min-height: 120px; overflow-y: auto; margin: 0 -6px; }
  .line {
    display: grid; grid-template-columns: 7.5em 1fr; gap: 0 8px;
    padding: 3px 6px; border-radius: 4px; line-height: 1.4;
    /* Off-screen lines skip layout and paint: a long history stays cheap. */
    content-visibility: auto; contain-intrinsic-size: auto 40px;
  }
  .line.odd { background: rgba(255, 255, 255, 0.035); }
  .kind { font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; padding-top: 1px; }
  .tick { grid-column: 1; grid-row: 2; font-size: 11px; color: var(--muted); font-variant-numeric: tabular-nums; }
  .text { grid-column: 2; grid-row: 1 / 3; overflow-wrap: anywhere; }
  .muted { color: var(--muted); }
  .jump {
    position: absolute; bottom: 8px; left: 50%; transform: translateX(-50%);
    background: var(--accent); color: #fff; border-color: transparent; border-radius: 14px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.4);
  }
</style>

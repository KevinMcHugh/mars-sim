<script lang="ts">
  // The Roster tab: every colonist, and with the TUI's filters the dead and
  // the non-humans, from the roster topic (internal/wire/roster.go). A row
  // opens the creature in the inspector.
  //
  // The list is virtualized: a long game has hundreds of dead colonists, and
  // the rows are rebuilt twice a second, so only the rows on screen (and a
  // few either side) are in the DOM.
  import { inspect, subscribe, topics, ui } from '../game.svelte';

  interface Row {
    id: number; glyph: string; name: string; info: string; state: string;
    kind: string; dead: boolean; hp: number; maxHp: number;
  }

  /** Each row's height in pixels; the CSS below matches it. */
  const ROW = 62;
  const OVERSCAN = 6;

  const topic = $derived.by(() => {
    const f = [ui.rosterDead && 'dead', ui.rosterNonHuman && 'nonhuman'].filter(Boolean);
    return f.length ? `roster:${f.join(',')}` : 'roster';
  });
  $effect(() => subscribe(topic));
  const all = $derived(topics.data[topic] as Row[] | undefined);

  let query = $state('');
  const rows = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!all || !q) return all ?? [];
    return all.filter((r) => r.name.toLowerCase().includes(q) || r.info.toLowerCase().includes(q));
  });

  let scrollTop = $state(0);
  let height = $state(0);
  const first = $derived(Math.max(0, Math.floor(scrollTop / ROW) - OVERSCAN));
  const last = $derived(Math.min(rows.length, Math.ceil((scrollTop + height) / ROW) + OVERSCAN));
  const visible = $derived(rows.slice(first, last));

  const selectedId = $derived(ui.selected && 'entity' in ui.selected ? ui.selected.entity : null);
  const count = $derived(all ? all.length : 0);
</script>

<div class="roster">
  <div class="controls">
    <label><input type="checkbox" bind:checked={ui.rosterDead} /> Dead</label>
    <label><input type="checkbox" bind:checked={ui.rosterNonHuman} /> Non-human</label>
    <input class="search" type="search" placeholder="Find by name" bind:value={query} aria-label="Find by name" />
  </div>
  <p class="count">
    {#if !all}Loading…{:else if query.trim()}{rows.length} of {count}{:else}{count}{/if}
  </p>

  <div class="list" bind:clientHeight={height} onscroll={(e) => (scrollTop = e.currentTarget.scrollTop)}>
    {#if all && rows.length === 0}
      <p class="muted">{count === 0 ? 'No colonists in the colony.' : 'Nothing matches.'}</p>
    {/if}
    <div class="spacer" style="height: {rows.length * ROW}px">
      {#each visible as r, i (r.id)}
        <button type="button" class="row" class:on={r.id === selectedId} class:dead={r.dead}
          style="top: {(first + i) * ROW}px" onclick={() => inspect({ entity: r.id }, 'roster')}>
          <span class="name"><span class="glyph">{r.glyph}</span> {r.name}</span>
          {#if !r.dead && r.maxHp > 0 && r.hp < r.maxHp}
            <span class="hp" title="health">{r.hp}/{r.maxHp}</span>
          {/if}
          <span class="line">{r.info}</span>
          <span class="line">{r.state}</span>
        </button>
      {/each}
    </div>
  </div>
</div>

<style>
  .roster { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .controls { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 12px; }
  .controls label { display: flex; align-items: center; gap: 4px; cursor: pointer; }
  .controls input[type='checkbox'] { accent-color: var(--accent); padding: 0; }
  .search { flex: 1; min-width: 8em; }
  .count { color: var(--muted); margin: 8px 0 4px; font-size: 12px; }
  .list { flex: 1; min-height: 120px; overflow-y: auto; position: relative; margin: 0 -6px; }
  .spacer { position: relative; }
  .row {
    position: absolute; left: 0; right: 0; height: 62px; box-sizing: border-box;
    display: grid; grid-template-columns: 1fr auto; align-content: center;
    text-align: left; border-color: transparent; background: transparent;
    padding: 4px 8px; border-radius: 6px;
  }
  .row:hover { background: rgba(255, 255, 255, 0.05); }
  .row.on { background: rgba(224, 112, 58, 0.22); border-color: var(--accent); }
  .row.dead { opacity: 0.6; }
  .name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hp { color: #f2877e; font-size: 12px; font-variant-numeric: tabular-nums; }
  .line {
    grid-column: 1 / -1; color: var(--muted); font-size: 12px;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .muted { color: var(--muted); }
</style>

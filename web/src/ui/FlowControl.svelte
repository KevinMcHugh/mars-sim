<script lang="ts">
  // The flow-field overlay's controls (docs/flow-field-view.md): a picker in
  // the top bar, and, while a field is on the map, a legend under it naming
  // the field, its extent, and the step range of each color band. f steps
  // through the fields and F turns the overlay off (main.ts).
  import { setFlowField, ui } from '../game.svelte';
  import { FLOW_GOAL_HEX, FLOW_RAMP_HEX, flowBand } from '../map/palette';

  let { legend = false }: { legend?: boolean } = $props();

  // Each band in use on the shown field, with the distances it covers.
  const bands = $derived.by(() => {
    const s = ui.flowShown;
    if (!s || s.max < 1) return [];
    const out: { color: string; lo: number; hi: number }[] = [];
    for (let d = 1; d <= s.max; d++) {
      const color = FLOW_RAMP_HEX[flowBand(d, s.max)];
      const last = out.at(-1);
      if (last && last.color === color) last.hi = d;
      else out.push({ color, lo: d, hi: d });
    }
    return out;
  });
  const name = $derived(ui.flowShown ? ui.hello?.flowFields[ui.flowShown.field] ?? 'flow field' : '');
  const steps = (lo: number, hi: number) => `${lo === hi ? lo : `${lo}–${hi}`} step${hi === 1 ? '' : 's'}`;
</script>

{#if !legend}
  <label class="pick" title="Show a shared flow field: how far each tile is from the nearest goal (f / F)">
    <span>Flow</span>
    <select disabled={!ui.hello} value={ui.flowPick} onchange={(e) => setFlowField(Number(e.currentTarget.value))}>
      <option value={-1}>off</option>
      {#each ui.hello?.flowFields ?? [] as f, i (f)}
        <option value={i}>{f}</option>
      {/each}
    </select>
  </label>
{:else if ui.flowShown}
  <aside class="hud legend" aria-label="Flow field legend">
    <header>
      <strong>{name}</strong>
      <button type="button" class="close" title="Hide the flow field (F)" aria-label="Hide the flow field" onclick={() => setFlowField(-1)}>×</button>
    </header>
    {#if ui.flowShown.goals === 0}
      <p class="muted">No goal tiles, so no one routes by this field.<br />A facility's field leads only to fixtures<br />everyone may use, not private ones.</p>
    {:else}
      <p class="muted">{ui.flowShown.goals.toLocaleString()} goal tiles · farthest {steps(ui.flowShown.max, ui.flowShown.max)}</p>
      <ul>
        <li><span class="swatch" style="background: {FLOW_GOAL_HEX}"></span>goal</li>
        {#each bands as b (b.lo)}
          <li><span class="swatch" style="background: {b.color}"></span>{steps(b.lo, b.hi)}</li>
        {/each}
      </ul>
      <p class="muted">Untinted floor can't reach a goal.</p>
    {/if}
    <p class="muted keys">f next field · F off</p>
  </aside>
{/if}

<style>
  .pick { display: flex; align-items: center; gap: 6px; color: var(--muted); }
  .pick select { padding: 3px 6px; }
  /* Bottom right, clear of the side panel: the top bar wraps, and the
     bottom left has the log ticker and the hover readout. */
  .legend {
    bottom: max(10px, env(safe-area-inset-bottom));
    right: calc(var(--panel-reserve) + 20px);
    padding: 8px 12px 6px;
    min-width: 190px;
    max-width: calc(100vw - 20px);
  }
  header { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
  strong { text-transform: capitalize; }
  .close { width: 24px; height: 24px; padding: 0; border-radius: 12px; line-height: 1; color: var(--muted); }
  .close:hover { color: var(--fg); }
  p { margin: 4px 0; }
  .muted { color: var(--muted); }
  .keys { font-size: 12px; }
  ul { list-style: none; margin: 6px 0; padding: 0; display: grid; gap: 2px; font-variant-numeric: tabular-nums; }
  li { display: flex; align-items: center; gap: 8px; }
  .swatch { width: 14px; height: 14px; border-radius: 3px; flex: none; }
  /* A phone: the side panel is a bottom sheet, so sit above its tab strip. */
  @media (max-width: 700px) {
    .legend { right: 10px; bottom: calc(max(10px, env(safe-area-inset-bottom)) + 96px); font-size: 12px; }
    ul { grid-template-columns: 1fr 1fr; }
  }
</style>

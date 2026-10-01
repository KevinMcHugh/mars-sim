<script lang="ts">
  // One uPlot chart: built when mounted, fed new data in place (setData, no
  // rebuild), sized to its container, and destroyed with the component. It
  // draws its own hover tooltip from tip(), so every chart has one.
  //
  // uPlot itself is loaded on first use, not with the page: it is only for
  // this tab, and it reads navigator.language when it loads, so a browser
  // with an odd locale tag must not be able to take the map down with it.
  import type uPlot from 'uplot';
  import 'uplot/dist/uPlot.min.css';
  import { untrack } from 'svelte';

  export interface Tip { title: string; rows: { color?: string; fill?: 'hatch'; label: string; value: string }[] }

  interface Props {
    /** Everything but width, height and data; the caller's series and scales. */
    opts: Omit<uPlot.Options, 'width' | 'height'>;
    data: uPlot.AlignedData;
    height: number;
    /** The tooltip for the point under the cursor, or null for none. */
    tip?: (idx: number) => Tip | null;
    label: string;
  }
  let { opts, data, height, tip, label }: Props = $props();

  let host: HTMLDivElement | undefined = $state();
  let plot: uPlot | null = null;
  let hover: { x: number; y: number; t: Tip } | null = $state(null);

  // Built once per mount (and per height or option change): data changes go
  // through setData below, untracked here so they never rebuild the chart.
  let failed = $state('');
  $effect(() => {
    if (!host) return;
    const el = host;
    const o = opts;
    const h = height;
    let u: uPlot | null = null;
    let ro: ResizeObserver | null = null;
    let gone = false;
    import('uplot').then(({ default: UPlot }) => {
      if (gone) return;
      u = untrack(() => new UPlot({
        ...o,
        width: el.clientWidth,
        height: h,
        hooks: {
          ...o.hooks,
          setCursor: [(u) => {
            const i = u.cursor.idx;
            const t = i == null || !tip ? null : tip(i);
            hover = t && u.cursor.left != null && u.cursor.left >= 0
              ? { x: u.cursor.left + u.bbox.left / devicePixelRatio, y: (u.cursor.top ?? 0) + u.bbox.top / devicePixelRatio, t }
              : null;
          }],
        },
      }, data, el));
      plot = u;
      const built = u;
      ro = new ResizeObserver(() => built.setSize({ width: el.clientWidth, height: h }));
      ro.observe(el);
    }).catch((e: Error) => { failed = `Charts could not load: ${e.message}`; });
    return () => { gone = true; ro?.disconnect(); u?.destroy(); plot = null; };
  });

  // New data in place: keeps the cursor and zoom, and costs no rebuild.
  $effect(() => {
    const d = data;
    untrack(() => plot?.setData(d));
  });
</script>

{#if failed}<p class="failed">{failed}</p>{/if}
<div class="chart" bind:this={host} role="img" aria-label={label} onmouseleave={() => (hover = null)}>
  {#if hover}
    <div class="tip" style="left: {hover.x}px; top: {hover.y}px" class:left={host && hover.x > host.clientWidth / 2}>
      <div class="title">{hover.t.title}</div>
      {#each hover.t.rows as r, i (i)}
        <div class="row">
          {#if r.color}<span class="key" class:hatched={r.fill === 'hatch'} style="--c: {r.color}"></span>{/if}
          <span class="label">{r.label}</span><span class="value">{r.value}</span>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .chart { position: relative; width: 100%; }
  .failed { color: #f2877e; font-size: 12px; margin: 0; }
  .chart :global(.u-legend) { display: none; }
  .chart :global(.u-cursor-x) { border-right: 1px dashed #c3c2b7; }
  .chart :global(.u-cursor-y) { display: none; }
  .tip {
    position: absolute; z-index: 2; pointer-events: none; transform: translate(12px, -50%);
    background: #262624; border: 1px solid rgba(255, 255, 255, 0.1); border-radius: 6px;
    padding: 6px 8px; font-size: 12px; min-width: 9em; white-space: nowrap;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
  }
  .tip.left { transform: translate(calc(-100% - 12px), -50%); }
  .title { color: #ffffff; font-weight: 600; margin-bottom: 3px; }
  .row { display: flex; align-items: center; gap: 6px; color: #c3c2b7; line-height: 1.5; }
  .label { flex: 1; }
  .value { color: #ffffff; font-variant-numeric: tabular-nums; }
  .key { width: 9px; height: 9px; border-radius: 2px; background: var(--c); flex: none; }
  .key.hatched {
    background: repeating-linear-gradient(135deg, var(--c) 0 2px, color-mix(in srgb, var(--c) 45%, #1a1a19) 2px 4px);
  }
</style>

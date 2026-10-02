<script lang="ts">
  // A labelled gauge. Plain: filled from the left, value/max. Diverging: a
  // signed value in [-max, max], filled out from the middle, right for
  // positive (the TUI's divergeGauge).
  interface Props {
    label: string;
    value: number;
    max: number;
    diverging?: boolean;
    /** Red once full: a drive that kills when it tops out. */
    danger?: boolean;
    title?: string;
  }
  let { label, value, max, diverging = false, danger = false, title }: Props = $props();

  const m = $derived(Math.max(1, max));
  const frac = $derived(Math.min(1, Math.abs(value) / m));
  const style = $derived(
    diverging
      ? value >= 0
        ? `left: 50%; width: ${frac * 50}%`
        : `left: ${50 - frac * 50}%; width: ${frac * 50}%`
      : `left: 0; width: ${frac * 100}%`,
  );
  const tone = $derived(diverging ? (value >= 0 ? 'good' : 'bad') : danger && frac > 0.75 ? 'bad' : '');
</script>

<div class="bar" {title}>
  <span class="label">{label}</span>
  <span class="track" class:diverging>
    <span class="fill {tone}" {style}></span>
  </span>
  <span class="value">{diverging ? (value > 0 ? '+' : '') + value : `${value}/${max}`}</span>
</div>

<style>
  .bar { display: grid; grid-template-columns: 6.5em 1fr 5.5em; align-items: center; gap: 8px; }
  .label { color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .track { position: relative; height: 8px; border-radius: 4px; background: rgba(255, 255, 255, 0.08); overflow: hidden; }
  .track.diverging::after {
    content: ''; position: absolute; left: 50%; top: 0; bottom: 0; width: 1px; background: var(--line);
  }
  .fill { position: absolute; top: 0; bottom: 0; background: var(--accent); }
  .fill.good { background: #6cc28a; }
  .fill.bad { background: #e0564a; }
  .value { text-align: right; font-variant-numeric: tabular-nums; }
</style>

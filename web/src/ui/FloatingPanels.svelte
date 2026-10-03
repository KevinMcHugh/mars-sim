<script lang="ts">
  // The tabs popped out of the side panel (ui.floats): each one a window over
  // the map, dragged by its title row and sized from its corner, so the
  // Market, say, can stay open beside a chart. A window mounts its tab exactly
  // as the side panel would, topic subscription and all; closing it unmounts
  // the tab. See docs/floating-panels.md.
  import { innerHeight, innerWidth } from 'svelte/reactivity/window';
  import { closeFloat, dock, placeFloat, raiseFloat, ui, type FloatWin } from '../game.svelte';
  import PanelHead from './PanelHead.svelte';
  import { tabs } from './tabs';

  const MIN_W = 260;
  const MIN_H = 160;
  /** How much of a window must stay on screen: enough of its title row to grab. */
  const GRAB = 80;

  // Drawn in the tab strip's order and stacked by z-index from ui.floats, so
  // raising a window never moves its DOM node (which would drop the pointer
  // capture of the drag that raised it).
  const open = $derived(tabs.filter((t) => ui.floats.some((f) => f.id === t.id)));

  // Where a window is drawn: its stored place, pulled back on screen if the
  // viewport has shrunk since. The stored place is left alone, so a window
  // comes back where it was when the viewport grows again.
  function placed(f: FloatWin) {
    const vw = innerWidth.current ?? 1024;
    const vh = innerHeight.current ?? 768;
    const w = Math.min(f.w, vw - 20);
    const h = Math.min(f.h, vh - 20);
    const x = Math.min(Math.max(f.x, GRAB - w), vw - GRAB);
    const y = Math.min(Math.max(f.y, 0), vh - 32);
    return { x, y, w, h };
  }

  /**
   * Follow a pointer from a pointerdown until it lifts, calling move with how
   * far it has gone; the window's place is saved once, at the end.
   */
  function track(e: PointerEvent, id: string, move: (dx: number, dy: number) => void) {
    if (e.button !== 0) return;
    const el = e.currentTarget as HTMLElement;
    const x0 = e.clientX;
    const y0 = e.clientY;
    el.setPointerCapture(e.pointerId);
    const onMove = (m: PointerEvent) => move(m.clientX - x0, m.clientY - y0);
    const onUp = () => {
      el.removeEventListener('pointermove', onMove);
      el.removeEventListener('pointerup', onUp);
      el.removeEventListener('pointercancel', onUp);
      placeFloat(id, {}, true);
    };
    el.addEventListener('pointermove', onMove);
    el.addEventListener('pointerup', onUp);
    el.addEventListener('pointercancel', onUp);
    e.preventDefault();
  }

  function startMove(e: PointerEvent, f: FloatWin) {
    if ((e.target as HTMLElement).closest('button')) return;
    // Start from where the window is drawn, so a clamped one does not jump.
    const { x, y } = placed(f);
    track(e, f.id, (dx, dy) => placeFloat(f.id, { x: x + dx, y: y + dy }));
  }

  function startResize(e: PointerEvent, f: FloatWin) {
    const { w, h } = placed(f);
    track(e, f.id, (dx, dy) => placeFloat(f.id, { w: Math.max(MIN_W, w + dx), h: Math.max(MIN_H, h + dy) }));
  }
</script>

{#each open as t (t.id)}
  {@const f = ui.floats.find((w) => w.id === t.id)!}
  {@const p = placed(f)}
  <section class="float hud" aria-label={t.label}
    style="left: {p.x}px; top: {p.y}px; width: {p.w}px; height: {p.h}px; z-index: {10 + ui.floats.indexOf(f)}"
    onpointerdowncapture={() => raiseFloat(t.id)}>
    <!-- A drag handle for the pointer; the keyboard has Dock and Close, and the window's place is not content. -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="grab" onpointerdown={(e) => startMove(e, f)}>
      <PanelHead label={t.label}>
        <button type="button" title="Put back in the side panel" aria-label="Dock {t.label}" onclick={() => dock(t.id)}>⇥</button>
        <button type="button" title="Close" aria-label="Close {t.label}" onclick={() => closeFloat(t.id)}>×</button>
      </PanelHead>
    </div>
    <div class="content"><t.component /></div>
    <div class="resize" aria-hidden="true" onpointerdown={(e) => startResize(e, f)}></div>
  </section>
{/each}

<style>
  .float { display: flex; flex-direction: column; box-shadow: 0 6px 24px rgba(0, 0, 0, 0.45); }
  .grab { cursor: move; touch-action: none; user-select: none; }
  .content { flex: 1; overflow: auto; padding: 4px 14px 12px; min-height: 0; }
  .resize {
    position: absolute; right: 0; bottom: 0; width: 16px; height: 16px;
    cursor: nwse-resize; touch-action: none;
    background: linear-gradient(135deg, transparent 50%, var(--line) 50%, var(--line) 60%, transparent 60%, transparent 70%, var(--line) 70%, var(--line) 80%, transparent 80%);
    border-bottom-right-radius: 8px;
  }
</style>

// Pan and zoom: drag (mouse, pen or one finger), wheel or trackpad pinch,
// two-finger pinch, and the keyboard. Pointer events throughout, so touch
// works without a separate path (docs/browser-frontend.md, "Mobile, later").

import type { Camera } from './camera';

export interface InputHooks {
  changed: () => void;               // the camera moved
  hover: (sx: number, sy: number) => void;
  leave: () => void;
}

export function attachInput(canvas: HTMLCanvasElement, cam: Camera, hooks: InputHooks): void {
  const pointers = new Map<number, { x: number; y: number }>();
  let pinchDist = 0;

  const local = (e: PointerEvent | WheelEvent) => {
    const r = canvas.getBoundingClientRect();
    return [e.clientX - r.left, e.clientY - r.top] as const;
  };

  canvas.addEventListener('pointerdown', (e) => {
    canvas.setPointerCapture(e.pointerId);
    const [x, y] = local(e);
    pointers.set(e.pointerId, { x, y });
    if (pointers.size === 2) pinchDist = spread(pointers);
  });

  canvas.addEventListener('pointermove', (e) => {
    const [x, y] = local(e);
    const p = pointers.get(e.pointerId);
    if (!p) { hooks.hover(x, y); return; }
    if (pointers.size === 1) {
      cam.panPixels(x - p.x, y - p.y);
    } else if (pointers.size === 2) {
      p.x = x; p.y = y;
      const d = spread(pointers);
      const [mx, my] = middle(pointers);
      if (pinchDist > 0) cam.zoomAt(d / pinchDist, mx, my);
      pinchDist = d;
      hooks.changed();
      return;
    }
    p.x = x; p.y = y;
    hooks.changed();
    hooks.hover(x, y);
  });

  const up = (e: PointerEvent) => {
    pointers.delete(e.pointerId);
    pinchDist = pointers.size === 2 ? spread(pointers) : 0;
  };
  canvas.addEventListener('pointerup', up);
  canvas.addEventListener('pointercancel', up);
  canvas.addEventListener('pointerleave', () => { if (pointers.size === 0) hooks.leave(); });

  canvas.addEventListener('wheel', (e) => {
    e.preventDefault();
    const [x, y] = local(e);
    // Trackpad pinch arrives as a ctrl+wheel with small deltas; a mouse wheel
    // as larger ones. One curve serves both.
    const k = e.ctrlKey ? 0.01 : 0.0015;
    cam.zoomAt(Math.exp(-e.deltaY * k), x, y);
    hooks.changed();
    hooks.hover(x, y);
  }, { passive: false });

  window.addEventListener('keydown', (e) => {
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLSelectElement) return;
    const step = 80;
    switch (e.key) {
      case 'ArrowLeft': case 'a': cam.panPixels(step, 0); break;
      case 'ArrowRight': case 'd': cam.panPixels(-step, 0); break;
      case 'ArrowUp': case 'w': cam.panPixels(0, step); break;
      case 'ArrowDown': case 's': cam.panPixels(0, -step); break;
      // + and - are the speed selector's (main.ts); zoom by key is [ and ].
      case ']': cam.zoomAt(1.25, cam.width / 2, cam.height / 2); break;
      case '[': cam.zoomAt(0.8, cam.width / 2, cam.height / 2); break;
      default: return;
    }
    e.preventDefault();
    hooks.changed();
  });
}

function spread(ps: Map<number, { x: number; y: number }>): number {
  const [a, b] = [...ps.values()];
  return Math.hypot(a.x - b.x, a.y - b.y);
}

function middle(ps: Map<number, { x: number; y: number }>): [number, number] {
  const [a, b] = [...ps.values()];
  return [(a.x + b.x) / 2, (a.y + b.y) / 2];
}

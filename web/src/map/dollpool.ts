// The atlas cells paper dolls are painted into (docs/body-sprites.md). There
// are thousands of possible dolls and only a few in any game, so dolls get a
// small fixed pool of cells, filled the first time a frame names a doll and
// handed back, least recently drawn first, when the pool runs out.

interface Slot {
  cell: number;
  state: 'painting' | 'ready' | 'failed';
  /** The frame that last drew this doll. */
  used: number;
}

export class DollPool {
  private slots = new Map<number, Slot>();
  private free: number[];
  private paint: (key: number, cell: number) => Promise<void>;
  private ready: () => void;

  /**
   * size cells starting at atlas cell first. paint draws doll key into cell
   * (rejecting if it cannot); ready runs after each one lands, so the map
   * redraws.
   */
  constructor(
    first: number,
    size: number,
    paint: (key: number, cell: number) => Promise<void>,
    ready: () => void,
  ) {
    // Plain fields, not parameter properties: node --test strips types and
    // cannot run those.
    this.paint = paint;
    this.ready = ready;
    this.free = Array.from({ length: size }, (_, i) => first + size - 1 - i);
  }

  /**
   * The atlas cell to draw doll key in on frame, or -1 to draw its fallback
   * for now: while it is being painted, if it failed to paint, or if every
   * cell is in use by a doll drawn on this same frame.
   */
  cell(key: number, frame: number): number {
    let slot = this.slots.get(key);
    if (slot) {
      slot.used = frame;
      return slot.state === 'ready' ? slot.cell : -1;
    }
    const cell = this.free.pop() ?? this.evict(frame);
    if (cell < 0) return -1;
    const fresh: Slot = { cell, state: 'painting', used: frame };
    slot = fresh;
    this.slots.set(key, fresh);
    this.paint(key, cell).then(
      () => {
        if (this.slots.get(key) !== fresh) return;
        fresh.state = 'ready';
        this.ready();
      },
      (err) => {
        // Keep the slot, so the doll is not retried every frame; its cell
        // goes back to the pool.
        console.warn(`doll ${key} not drawn:`, err);
        fresh.state = 'failed';
        this.free.push(fresh.cell);
        fresh.cell = -1;
      },
    );
    return -1;
  }

  /**
   * Take back the cell of the least recently drawn doll not drawn on frame,
   * or -1 if there is none. A doll still being painted is never taken: its
   * paint would land on the cell's next owner.
   */
  private evict(frame: number): number {
    let oldest: number | undefined;
    let oldestUsed = frame;
    for (const [key, s] of this.slots) {
      if (s.state === 'ready' && s.used < oldestUsed) {
        oldest = key;
        oldestUsed = s.used;
      }
    }
    if (oldest === undefined) return -1;
    const s = this.slots.get(oldest)!;
    this.slots.delete(oldest);
    return s.cell;
  }
}

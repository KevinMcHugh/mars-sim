// The page's side of the worker (web/public/worker.js): start a game, send
// commands and the view, and receive decoded frames.

import { decodeFrame } from '../../wire/decode.js';
import type { Frame, Hello } from '../../wire/decode.js';
import type { TileRect } from '../map/camera';

/** New-game settings: mars-sim.yaml keys (see docs/config-file.md). */
export type Settings = Record<string, number | boolean | string>;

export type Command =
  | { type: 'pause' }
  | { type: 'speed'; rate: number }
  | { type: 'spawn'; kind: string }
  /** Show a flow field (an index into Hello.flowFields), or none (-1). */
  | { type: 'flow'; field: number }
  /** Order the rectangle (tiles, inclusive) mined out, paid for by the colony. */
  /** Cancel an excavation order by id, refunding what is unspent. */
  | { type: 'dig-cancel'; id: number }
  | { type: 'dig'; x0: number; y0: number; x1: number; y1: number }
  /** Zone the rectangle as a kind by name ('none' unzones it); the work it implies is paid by the colony (docs/zoning.md). */
  | { type: 'zone'; kind: string; x0: number; y0: number; x1: number; y1: number }
  /** Order every structure in the rectangle cleared, paid by the colony. */
  | { type: 'clear'; x0: number; y0: number; x1: number; y1: number }
  /** Cancel a clearing order by id, refunding what is unspent. */
  | { type: 'clear-cancel'; id: number }
  /** Reland a ship with its top-left at (x, y); only before the first tick (docs/ships.md). */
  | { type: 'ship-move'; id: number; x: number; y: number }
  /** Land the next ship still aloft with its top-left at (x, y); only before the first tick. */
  | { type: 'ship-land'; id: number; x: number; y: number }
  /** Post an order in the colony's name at a communal depot (docs/colony-orders.md). */
  | { type: 'order-place'; side: 'bid' | 'ask'; item: string; qty: number; price: number; x: number; y: number }
  /** Move one of the colony's open orders to a new price. */
  | { type: 'order-reprice'; id: number; price: number }
  /** Take one of the colony's open orders off the book, returning its escrow. */
  | { type: 'order-cancel'; id: number }
  /** Stop the colony's standing orders for a side and item, everywhere, until resumed. */
  | { type: 'order-suspend'; side: 'bid' | 'ask'; item: string }
  | { type: 'order-resume'; side: 'bid' | 'ask'; item: string };

export interface Started { hello: Hello; genMs: number; loadMs: number }

/**
 * The WASM exports this page expects (hostAPI in cmd/mars-sim-wasm/main.go).
 * A mismatch means mars-sim.wasm is from another build: usually a pull without
 * rerunning npm run wasm.
 */
export const HOST_API = 15;

export class SimClient {
  private worker: Worker;
  private pending: ((s: Started) => void) | null = null;
  private failed: ((e: Error) => void) | null = null;
  onFrame: (f: Frame, bytes: number) => void = () => {};
  /** Panel topics that changed, by name (see internal/wire/topics.go). */
  onTopics: (topics: Record<string, unknown>) => void = () => {};
  onError: (message: string) => void = () => {};

  constructor(workerURL: URL) {
    this.worker = new Worker(workerURL);
    this.worker.onmessage = (e: MessageEvent) => this.receive(e.data);
    this.worker.onerror = (e) => this.onError(e.message || 'worker failed to load');
  }

  start(settings: Settings, budgetMs = 8): Promise<Started> {
    return new Promise((resolve, reject) => {
      this.pending = resolve;
      this.failed = reject;
      this.worker.postMessage({ type: 'start', settings, budgetMs });
    });
  }

  command(c: Command): void {
    this.worker.postMessage({ type: 'command', command: c });
  }

  subscribe(topic: string): void {
    this.worker.postMessage({ type: 'subscribe', topic });
  }

  unsubscribe(topic: string): void {
    this.worker.postMessage({ type: 'unsubscribe', topic });
  }

  setInterest(r: TileRect): void {
    this.worker.postMessage({ type: 'interest', rect: r });
  }

  private receive(msg: any): void {
    switch (msg.type) {
      case 'started':
        if (msg.api !== HOST_API) {
          this.failed?.(new Error(
            `mars-sim.wasm is out of date (it speaks version ${msg.api}; this page needs ${HOST_API}). ` +
            'Rebuild it with npm run wasm (npm run dev does this for you) and reload.'));
        } else if (msg.result.error) {
          this.failed?.(new Error(msg.result.error));
        } else {
          this.pending?.({ hello: msg.result.hello, genMs: msg.result.genMs, loadMs: msg.loadMs });
        }
        this.pending = this.failed = null;
        break;
      case 'frame':
        try {
          this.onFrame(decodeFrame(msg.buffer), msg.buffer.byteLength);
        } catch (err) {
          this.onError(String((err as Error).message ?? err));
        }
        break;
      case 'topics':
        this.onTopics(msg.topics);
        break;
      case 'error':
        this.onError(msg.error);
        break;
    }
  }
}

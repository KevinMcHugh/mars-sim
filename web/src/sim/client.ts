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
  | { type: 'spawn'; kind: string };

export interface Started { hello: Hello; genMs: number; loadMs: number }

export class SimClient {
  private worker: Worker;
  private pending: ((s: Started) => void) | null = null;
  private failed: ((e: Error) => void) | null = null;
  onFrame: (f: Frame, bytes: number) => void = () => {};
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

  setInterest(r: TileRect): void {
    this.worker.postMessage({ type: 'interest', rect: r });
  }

  private receive(msg: any): void {
    switch (msg.type) {
      case 'started':
        if (msg.result.error) {
          this.failed?.(new Error(msg.result.error));
        } else {
          this.pending?.({ hello: msg.result.hello, genMs: msg.result.genMs, loadMs: msg.loadMs });
        }
        this.pending = this.failed = null;
        break;
      case 'frame':
        this.onFrame(decodeFrame(msg.buffer), msg.buffer.byteLength);
        break;
      case 'error':
        this.onError(msg.error);
        break;
    }
  }
}

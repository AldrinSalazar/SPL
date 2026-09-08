import SplWorker from './worker/spl.worker.ts?worker';
import type { Diagnostic, RenderOpts, RenderResult, WorkerResponse } from './types';

let nextId = 1;

export class WorkerClient {
  private worker: Worker | null = null;
  private pending = new Map<number, {
    resolve: (v: any) => void;
    reject: (e: any) => void;
    onProgress?: (done: number, total: number, block: number, numBlocks: number) => void;
  }>();
  private readyPromise: Promise<void> | null = null;
  private readyResolve: (() => void) | null = null;
  private wasmUrl: string;
  private jsUrl: string;

  constructor() {
    const base = import.meta.env.BASE_URL || './';
    // BASE_URL ends with '/'; public assets are copied to dist root.
    this.wasmUrl = new URL(base + 'spl.wasm', window.location.href).href;
    this.jsUrl = new URL(base + 'wasm_exec.js', window.location.href).href;
    this.spawn();
  }

  private spawn() {
    this.terminateOnly();
    this.worker = new SplWorker();
    this.worker.onmessage = (ev: MessageEvent<WorkerResponse>) => this.handle(ev.data);
    this.worker.onerror = (e) => {
      for (const [, p] of this.pending) p.reject(new Error('worker error: ' + (e.message || 'unknown')));
      this.pending.clear();
    };
    this.readyPromise = new Promise((res) => { this.readyResolve = res; });
    const id = nextId++;
    this.pending.set(id, {
      resolve: () => { this.readyResolve?.(); },
      reject: () => { this.readyResolve?.(); }
    });
    this.worker.postMessage({ id, type: 'init', wasmUrl: this.wasmUrl, jsUrl: this.jsUrl });
  }

  private terminateOnly() {
    if (this.worker) {
      try { this.worker.terminate(); } catch { /* ignore */ }
      this.worker = null;
    }
  }

  /** Terminate and recreate the worker for Cancel; ready for next request. */
  async cancelAndRecreate(): Promise<void> {
    for (const [, p] of this.pending) p.reject(new Error('cancelled'));
    this.pending.clear();
    this.spawn();
    await this.ready();
  }

  async ready(): Promise<void> {
    if (this.readyPromise) await this.readyPromise;
  }

  private handle(msg: WorkerResponse) {
    if (msg.type === 'ready' || msg.type === 'progress') {
      const p = this.pending.get(msg.id);
      if (!p) return; // stale
      if (msg.type === 'ready') {
        this.pending.delete(msg.id);
        p.resolve(undefined);
      } else {
        p.onProgress?.(msg.done, msg.total, msg.block, msg.numBlocks);
      }
      return;
    }
    const p = this.pending.get(msg.id);
    if (!p) return; // stale response discarded by request ID
    this.pending.delete(msg.id);
    if (msg.type === 'error') {
      p.reject(new Error(msg.message));
      return;
    }
    p.resolve(msg);
  }

  private call<T>(msg: any, onProgress?: (done: number, total: number, block: number, numBlocks: number) => void): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      if (!this.worker) {
        reject(new Error('worker unavailable'));
        return;
      }
      this.pending.set(msg.id, { resolve: resolve as any, reject, onProgress });
      this.worker.postMessage(msg);
    });
  }

  async validate(source: string): Promise<{ ok: boolean; diagnostics: Diagnostic[] }> {
    await this.ready();
    const id = nextId++;
    const res: any = await this.call({ id, type: 'validate', source });
    return { ok: res.ok, diagnostics: res.diagnostics ?? [] };
  }

  async render(source: string, opts: RenderOpts, revision: number,
    onProgress?: (done: number, total: number, block: number, numBlocks: number) => void
  ): Promise<{ ok: boolean; revision: number; result?: RenderResult; diagnostics?: Diagnostic[] }> {
    await this.ready();
    const id = nextId++;
    const res: any = await this.call({ id, type: 'render', source, opts, revision }, onProgress);
    return res;
  }

  async spectrogram(opts: RenderOpts): Promise<{ ok: boolean; png?: ArrayBuffer; display?: ArrayBuffer; displayMeta?: any; diagnostics?: Diagnostic[] }> {
    await this.ready();
    const id = nextId++;
    const res: any = await this.call({ id, type: 'spectrogram', opts });
    return res;
  }

  dispose() {
    this.terminateOnly();
    this.pending.clear();
  }
}

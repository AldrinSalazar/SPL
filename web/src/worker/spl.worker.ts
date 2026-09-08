/// <reference lib="webworker" />
// Dedicated worker: runs Go WASM runtime and all expensive Go work.
// Main thread stays responsive; cancel is implemented by terminating this
// worker (Go work is synchronous and blocks worker messages).
import type { WorkerRequest, WorkerResponse } from '../types';

declare const self: DedicatedWorkerGlobalScope & {
  splValidate?: (src: string) => string;
  splRender?: (src: string, opts: string, progress: ((done: number, total: number, block: number, numBlocks: number) => void) | null) => any;
  splSpectrogram?: (opts: string) => any;
};

let ready = false;

function post(msg: WorkerResponse, transfer?: Transferable[]) {
  self.postMessage(msg, transfer ?? []);
}

function uint8ToBuffer(v: any): ArrayBuffer | null {
  if (!v) return null;
  // v is a Uint8Array from Go (owned copy). Copy into a fresh transferable buffer.
  const u8 = v as Uint8Array;
  const buf = new ArrayBuffer(u8.byteLength);
  new Uint8Array(buf).set(u8);
  return buf;
}

async function initWasm(wasmUrl: string, jsUrl: string): Promise<void> {
  // Load matching Go runtime JS from toolchain-built asset.
  const jsText = await (await fetch(jsUrl)).text();
  // Evaluate in worker scope to define globalThis.Go.
  (0, eval)(jsText + '\n//# sourceURL=wasm_exec.js');
  const GoCtor = (globalThis as any).Go;
  if (!GoCtor) throw new Error('Go runtime failed to load');
  const go = new GoCtor();
  let instance: WebAssembly.Instance;
  try {
    if (typeof (WebAssembly as any).instantiateStreaming === 'function') {
      const res = await WebAssembly.instantiateStreaming(fetch(wasmUrl), go.importObject);
      instance = res.instance;
    } else {
      throw new Error('no streaming');
    }
  } catch {
    const buf = await (await fetch(wasmUrl)).arrayBuffer();
    const res = await WebAssembly.instantiate(buf, go.importObject);
    instance = res.instance;
  }
  (go as any).run(instance);
  // main() registers globals synchronously before blocking.
  for (let i = 0; i < 100 && !self.splRender; i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  if (!self.splRender || !self.splValidate || !self.splSpectrogram) {
    throw new Error('WASM exports missing after init');
  }
}

self.onmessage = async (ev: MessageEvent<WorkerRequest>) => {
  const msg = ev.data;
  try {
    switch (msg.type) {
      case 'init': {
        await initWasm(msg.wasmUrl, msg.jsUrl);
        ready = true;
        post({ id: msg.id, type: 'ready' });
        break;
      }
      case 'validate': {
        if (!ready) throw new Error('worker not initialized');
        const out = self.splValidate!(msg.source);
        const parsed = JSON.parse(out);
        post({ id: msg.id, type: 'validateResult', ok: parsed.ok, diagnostics: parsed.diagnostics ?? [] });
        break;
      }
      case 'render': {
        if (!ready) throw new Error('worker not initialized');
        const optsJson = JSON.stringify({
          fftLen: msg.opts.fftLen, hop: msg.opts.hop,
          dbMin: msg.opts.dbMin, dbMax: msg.opts.dbMax,
          plotW: msg.opts.plotW, plotH: msg.opts.plotH,
          wantWav: true, wantPng: true, wantPcm: true, wantDisplay: true
        });
        const id = msg.id;
        const progress = (done: number, total: number, block: number, numBlocks: number) => {
          post({ id, type: 'progress', done, total, block, numBlocks });
        };
        const ret = self.splRender!(msg.source, optsJson, progress);
        if (!ret || ret.ok !== true) {
          let diags: any[] = [];
          try { diags = JSON.parse(ret?.diagnostics ?? '[]'); } catch { /* ignore */ }
          post({ id, type: 'renderResult', ok: false, diagnostics: diags });
          break;
        }
        const stats = JSON.parse(ret.stats);
        const warnings = JSON.parse(ret.warnings ?? '[]');
        const wav = uint8ToBuffer(ret.wav);
        const pcm = uint8ToBuffer(ret.pcm);
        const png = uint8ToBuffer(ret.png);
        const display = uint8ToBuffer(ret.display);
        const displayMeta = ret.displayMeta ? JSON.parse(ret.displayMeta) : null;
        const transfer: Transferable[] = [];
        for (const b of [wav, pcm, png, display]) if (b) transfer.push(b);
        post({
          id, type: 'renderResult', ok: true, revision: (msg as any).revision,
          result: { stats, warnings, wav, pcm, png, display, displayMeta }
        }, transfer);
        break;
      }
      case 'spectrogram': {
        if (!ready) throw new Error('worker not initialized');
        const optsJson = JSON.stringify({
          fftLen: msg.opts.fftLen, hop: msg.opts.hop,
          dbMin: msg.opts.dbMin, dbMax: msg.opts.dbMax,
          plotW: msg.opts.plotW, plotH: msg.opts.plotH
        });
        const ret = self.splSpectrogram!(optsJson);
        if (!ret || ret.ok !== true) {
          let diags: any[] = [];
          try { diags = JSON.parse(ret?.diagnostics ?? '[]'); } catch { /* ignore */ }
          post({ id: msg.id, type: 'spectrogramResult', ok: false, diagnostics: diags });
          break;
        }
        const png = uint8ToBuffer(ret.png);
        const display = uint8ToBuffer(ret.display);
        const displayMeta = ret.displayMeta ? JSON.parse(ret.displayMeta) : null;
        const transfer: Transferable[] = [];
        if (png) transfer.push(png);
        if (display) transfer.push(display);
        post({ id: msg.id, type: 'spectrogramResult', ok: true, png: png ?? undefined, display: display ?? undefined, displayMeta }, transfer);
        break;
      }
    }
  } catch (e: any) {
    post({ id: (msg as any).id ?? 0, type: 'error', message: e?.message ?? String(e) });
  }
};

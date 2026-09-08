export interface Diagnostic {
  code: string;
  message: string;
  line: number;
  column?: number;
  field?: string;
  blockLine?: number;
}

export interface RenderStats {
  duration: number;
  rate: number;
  samples: number;
  peak: number;
  overCount: number;
  elapsedMs: number;
}

export interface DisplayMeta {
  plotW: number;
  plotH: number;
  originX: number;
  originY: number;
  totalW: number;
  totalH: number;
  duration: number;
  nyquist: number;
  dbMin: number;
  dbMax: number;
  fftLen: number;
  hop: number;
  rate: number;
  frames: number;
  bins: number;
}

export interface RenderOpts {
  fftLen: number;
  hop: number;
  dbMin: number;
  dbMax: number;
  plotW: number;
  plotH: number;
}

export interface RenderResult {
  stats: RenderStats;
  warnings: Diagnostic[];
  wav: ArrayBuffer | null;
  pcm: ArrayBuffer | null; // float32 LE bytes
  png: ArrayBuffer | null;
  display: ArrayBuffer | null; // float32 LE dB matrix
  displayMeta: DisplayMeta | null;
}

export type WorkerRequest =
  | { id: number; type: 'init'; wasmUrl: string; jsUrl: string }
  | { id: number; type: 'validate'; source: string }
  | { id: number; type: 'render'; source: string; opts: RenderOpts; revision: number }
  | { id: number; type: 'spectrogram'; opts: RenderOpts };

export type WorkerResponse =
  | { id: number; type: 'ready' }
  | { id: number; type: 'progress'; done: number; total: number; block: number; numBlocks: number }
  | { id: number; type: 'validateResult'; ok: boolean; diagnostics: Diagnostic[] }
  | { id: number; type: 'renderResult'; ok: boolean; revision?: number; result?: RenderResult; diagnostics?: Diagnostic[] }
  | { id: number; type: 'spectrogramResult'; ok: boolean; png?: ArrayBuffer; display?: ArrayBuffer; displayMeta?: DisplayMeta; diagnostics?: Diagnostic[] }
  | { id: number; type: 'error'; message: string; diagnostics?: Diagnostic[] };

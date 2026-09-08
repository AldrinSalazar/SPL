import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  AudioWaveform, Play, Pause, Square, RotateCcw, Download, FileAudio,
  FileImage, FileText, TriangleAlert, CircleAlert, Moon, Sun, Loader2,
  Activity, Clock, Hash, Gauge, Timer, SlidersHorizontal, ListMusic, X, Sparkles
} from 'lucide-react';
import { EXAMPLES } from './examples';
import { WorkerClient } from './workerClient';
import { AudioEngine } from './audioEngine';
import type { Diagnostic, DisplayMeta, RenderOpts, RenderStats } from './types';
import { Button } from './components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './components/ui/card';
import { Badge } from './components/ui/badge';
import { Label } from './components/ui/label';
import { Input } from './components/ui/input';
import { NativeSelect } from './components/ui/select';
import { Slider } from './components/ui/slider';
import { Progress } from './components/ui/progress';
import { Separator } from './components/ui/separator';
import { Alert, AlertDescription, AlertTitle } from './components/ui/alert';
import { cn } from './lib/utils';

const DEFAULT_OPTS: RenderOpts = { fftLen: 2048, hop: 256, dbMin: -100, dbMax: 0, plotW: 1000, plotH: 500 };

function fmtFreq(f: number): string {
  if (f >= 1000) return `${(f / 1000).toFixed(2)} kHz`;
  return `${f.toFixed(1)} Hz`;
}

function useTheme() {
  const [dark, setDark] = useState(() =>
    typeof document !== 'undefined' ? document.documentElement.classList.contains('dark') : false
  );
  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark);
    try {
      localStorage.setItem('spl-theme', dark ? 'dark' : 'light');
    } catch { /* ignore */ }
  }, [dark]);
  return { dark, toggle: () => setDark((d) => !d) };
}

export default function App() {
  const [source, setSource] = useState(EXAMPLES[3].source);
  const [exampleId, setExampleId] = useState('metallic');
  const [revision, setRevision] = useState(0);
  const [resultRevision, setResultRevision] = useState<number | null>(null);
  const [diags, setDiags] = useState<Diagnostic[]>([]);
  const [warnings, setWarnings] = useState<Diagnostic[]>([]);
  const [stats, setStats] = useState<RenderStats | null>(null);
  const [rendering, setRendering] = useState(false);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [status, setStatus] = useState('initializing worker…');
  const [workerError, setWorkerError] = useState<string | null>(null);
  const [wavUrl, setWavUrl] = useState<string | null>(null);
  const [pngUrl, setPngUrl] = useState<string | null>(null);
  const [displayMeta, setDisplayMeta] = useState<DisplayMeta | null>(null);
  const [displayData, setDisplayData] = useState<Float32Array | null>(null);
  const [cursor, setCursor] = useState<string>('hover the spectrogram');
  const [volume, setVolume] = useState(0.2);
  const [playing, setPlaying] = useState(false);
  const [pos, setPos] = useState(0);
  const [playbackNote, setPlaybackNote] = useState<string | null>(null);
  const [opts, setOpts] = useState<RenderOpts>({ ...DEFAULT_OPTS });
  const [applyingSpec, setApplyingSpec] = useState(false);
  const { dark, toggle } = useTheme();

  const clientRef = useRef<WorkerClient | null>(null);
  const audioRef = useRef<AudioEngine | null>(null);
  const editorRef = useRef<HTMLTextAreaElement | null>(null);
  const gutterRef = useRef<HTMLDivElement | null>(null);
  const imgRef = useRef<HTMLImageElement | null>(null);
  const reqIdRef = useRef(0);

  useEffect(() => {
    const client = new WorkerClient();
    clientRef.current = client;
    const audio = new AudioEngine();
    audioRef.current = audio;
    audio.onEnded = () => { setPlaying(false); setPos(audio.position); };
    audio.onTick = (p) => setPos(p);
    client.ready().then(() => setStatus('ready')).catch((e) => {
      setWorkerError('WASM initialization failed: ' + (e?.message ?? e));
      setStatus('worker failed');
    });
    return () => {
      client.dispose();
      audio.dispose();
      if (wavUrl) URL.revokeObjectURL(wavUrl);
      if (pngUrl) URL.revokeObjectURL(pngUrl);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    audioRef.current?.setVolume(volume);
  }, [volume]);

  const lineCount = useMemo(() => source.split('\n').length, [source]);

  function onEdit(v: string) {
    setSource(v);
    setRevision((r) => r + 1);
    setExampleId('custom');
  }

  function syncScroll() {
    if (editorRef.current && gutterRef.current) {
      gutterRef.current.scrollTop = editorRef.current.scrollTop;
    }
  }

  function jumpToLine(line: number) {
    const ta = editorRef.current;
    if (!ta) return;
    const lines = source.split('\n');
    let off = 0;
    for (let i = 0; i < Math.min(line - 1, lines.length); i++) off += lines[i].length + 1;
    ta.focus();
    ta.setSelectionRange(off, off + (lines[line - 1]?.length ?? 0));
  }

  function releaseUrls() {
    if (wavUrl) { URL.revokeObjectURL(wavUrl); setWavUrl(null); }
    if (pngUrl) { URL.revokeObjectURL(pngUrl); setPngUrl(null); }
  }

  async function handleRender() {
    const client = clientRef.current;
    if (!client || rendering) return;
    const myReq = ++reqIdRef.current;
    setRendering(true);
    setProgress(null);
    setDiags([]);
    setWarnings([]);
    setStatus('rendering…');
    setWorkerError(null);
    try {
      const res = await client.render(source, opts, revision, (done, total) => {
        if (myReq !== reqIdRef.current) return;
        setProgress({ done, total });
      });
      if (myReq !== reqIdRef.current) return; // stale
      if (!res.ok) {
        setDiags(res.diagnostics ?? [{ code: 'RENDER_ERROR', message: 'render failed', line: 0 }]);
        setStatus('render failed');
        return;
      }
      const r = res.result!;
      setStats(r.stats);
      setWarnings(r.warnings ?? []);
      setResultRevision(res.revision ?? revision);
      releaseUrls();
      if (r.wav) {
        const blob = new Blob([r.wav as ArrayBuffer], { type: 'audio/wav' });
        setWavUrl(URL.createObjectURL(blob));
      }
      if (r.png) {
        const blob = new Blob([r.png as ArrayBuffer], { type: 'image/png' });
        setPngUrl(URL.createObjectURL(blob));
      }
      if (r.display && r.displayMeta) {
        setDisplayData(new Float32Array(r.display.slice(0)));
        setDisplayMeta(r.displayMeta);
      } else {
        setDisplayData(null);
        setDisplayMeta(null);
      }
      if (r.pcm && audioRef.current) {
        const note = await audioRef.current.setData(r.pcm.slice(0), r.stats.rate);
        setPlaybackNote(note.message ?? null);
        setPos(0);
        setPlaying(false);
      }
      setStatus('render complete');
    } catch (e: any) {
      if (myReq !== reqIdRef.current) return;
      setWorkerError(e?.message ?? String(e));
      setStatus('render failed');
    } finally {
      if (myReq === reqIdRef.current) {
        setRendering(false);
        setProgress(null);
      }
    }
  }

  async function handleCancel() {
    const client = clientRef.current;
    if (!client) return;
    reqIdRef.current++; // discard stale responses
    setStatus('cancelling…');
    try {
      await client.cancelAndRecreate();
      setStatus('cancelled; worker ready');
    } catch (e: any) {
      setWorkerError('cancel failed: ' + (e?.message ?? e));
      setStatus('worker failed');
    } finally {
      setRendering(false);
      setProgress(null);
    }
  }

  async function handleApplySpec() {
    const client = clientRef.current;
    if (!client || !stats) return;
    setApplyingSpec(true);
    try {
      const res = await client.spectrogram(opts);
      if (!res.ok) {
        setDiags(res.diagnostics ?? [{ code: 'RENDER_ERROR', message: 'spectrogram failed', line: 0 }]);
        return;
      }
      if (pngUrl) URL.revokeObjectURL(pngUrl);
      if (res.png) {
        const blob = new Blob([res.png as ArrayBuffer], { type: 'image/png' });
        setPngUrl(URL.createObjectURL(blob));
      }
      if (res.display && res.displayMeta) {
        setDisplayData(new Float32Array((res.display as ArrayBuffer).slice(0)));
        setDisplayMeta(res.displayMeta);
      }
      setStatus('spectrogram updated (audio not rerendered)');
    } catch (e: any) {
      setWorkerError(e?.message ?? String(e));
    } finally {
      setApplyingSpec(false);
    }
  }

  function onSpecHover(e: React.MouseEvent<HTMLImageElement>) {
    const img = imgRef.current;
    if (!img || !displayMeta || !displayData) return;
    const rect = img.getBoundingClientRect();
    const scaleX = img.naturalWidth / rect.width;
    const scaleY = img.naturalHeight / rect.height;
    const px = Math.floor((e.clientX - rect.left) * scaleX);
    const py = Math.floor((e.clientY - rect.top) * scaleY);
    const lx = px - displayMeta.originX;
    const ly = py - displayMeta.originY;
    if (lx < 0 || lx >= displayMeta.plotW || ly < 0 || ly >= displayMeta.plotH) {
      setCursor('outside plot area');
      return;
    }
    const t = ((lx + 0.5) / displayMeta.plotW) * displayMeta.duration;
    const f = (1 - (ly + 0.5) / displayMeta.plotH) * displayMeta.nyquist;
    const db = displayData[ly * displayMeta.plotW + lx];
    setCursor(`t=${t.toFixed(3)} s  f=${fmtFreq(f)}  ${db.toFixed(1)} dBFS`);
  }

  async function togglePlay() {
    const eng = audioRef.current;
    if (!eng || !stats) return;
    if (playing) {
      eng.pause();
      setPlaying(false);
      setPos(eng.position);
    } else {
      await eng.play(pos >= eng.duration - 1e-3 ? 0 : pos);
      setPlaying(true);
    }
  }

  function downloadSpl() {
    const blob = new Blob([source], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'sound.spl';
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  const stale = resultRevision !== null && resultRevision !== revision;
  const peakWarn = stats && stats.peak > 1;
  const progressPct = progress && progress.total > 0 ? Math.round((progress.done / progress.total) * 100) : 0;
  const seekMax = audioRef.current?.duration || stats?.duration || 0;

  return (
    <div className="min-h-screen bg-gradient-to-b from-violet-50/80 via-background to-background dark:from-violet-950/20 dark:via-background">
      {/* Header */}
      <header className="sticky top-0 z-10 border-b bg-background/80 backdrop-blur">
        <div className="mx-auto flex max-w-7xl items-center gap-3 px-4 py-3">
          <div className="flex size-9 items-center justify-center rounded-lg bg-gradient-to-br from-violet-600 to-fuchsia-500 text-white shadow">
            <AudioWaveform className="size-5" aria-hidden="true" />
          </div>
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-lg font-bold leading-tight tracking-tight">SPL</h1>
            <p className="hidden truncate text-xs text-muted-foreground sm:block">
              Spectral shapes for audio · rendered locally in your browser
            </p>
          </div>
          <Badge
            variant={rendering ? 'default' : 'secondary'}
            role="status"
            aria-live="polite"
            className="gap-1.5"
          >
            <span className={cn('size-1.5 rounded-full', rendering ? 'animate-pulse bg-current' : 'bg-emerald-500')} />
            {status}
          </Badge>
          <Button variant="ghost" size="icon" onClick={toggle} aria-label="toggle theme">
            {dark ? <Sun aria-hidden="true" /> : <Moon aria-hidden="true" />}
          </Button>
        </div>
      </header>

      <main className="mx-auto grid max-w-7xl gap-4 px-4 py-6 lg:grid-cols-2">
        {/* Editor column */}
        <section aria-label="SPL editor" className="flex min-w-0 flex-col gap-4">
          <Card>
            <CardHeader className="flex-row items-start justify-between gap-3 space-y-0">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <ListMusic className="size-4 text-primary" aria-hidden="true" />
                  Source
                </CardTitle>
                <CardDescription>Write SPL, pick an example, then render.</CardDescription>
              </div>
              <div className="flex items-center gap-2">
                <Label htmlFor="example" className="sr-only">Example</Label>
                <NativeSelect
                  id="example"
                  value={exampleId}
                  className="w-48"
                  onChange={(e) => {
                    const ex = EXAMPLES.find((x) => x.id === e.target.value);
                    if (ex) {
                      setSource(ex.source);
                      setExampleId(ex.id);
                      setRevision((r) => r + 1);
                    }
                  }}
                >
                  {EXAMPLES.map((x) => (
                    <option key={x.id} value={x.id}>{x.label}</option>
                  ))}
                  {exampleId === 'custom' && <option value="custom">Custom (edited)</option>}
                </NativeSelect>
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              {/* Keep visible label text for a11y tests via aria-label on the textarea */}
              <Label htmlFor="spl-source" className="sr-only">SPL source</Label>
              <div className="overflow-hidden rounded-lg border bg-muted/30 focus-within:ring-2 focus-within:ring-ring">
                <div className="flex max-h-[420px]">
                  <div
                    ref={gutterRef}
                    aria-hidden="true"
                    className="spl-gutter select-none overflow-hidden px-3 py-3 text-right text-muted-foreground"
                  >
                    {Array.from({ length: lineCount }, (_, i) => (
                      <div key={i + 1} className="h-5">{i + 1}</div>
                    ))}
                  </div>
                  <Separator orientation="vertical" />
                  <textarea
                    ref={editorRef}
                    id="spl-source"
                    aria-label="SPL source"
                    value={source}
                    spellCheck={false}
                    onChange={(e) => onEdit(e.target.value)}
                    onScroll={syncScroll}
                    onKeyDown={(e) => {
                      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') void handleRender();
                    }}
                    rows={22}
                    className="spl-editor min-h-[400px] flex-1 resize-y bg-transparent px-3 py-3 outline-none placeholder:text-muted-foreground"
                  />
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-2">
                <Button onClick={handleRender} disabled={rendering} size="lg" className="gap-2">
                  {rendering ? <Loader2 className="animate-spin" aria-hidden="true" /> : <Sparkles aria-hidden="true" />}
                  Render
                </Button>
                {rendering && (
                  <Button variant="outline" onClick={handleCancel} className="gap-2">
                    <X aria-hidden="true" /> Cancel
                  </Button>
                )}
                <span className="text-xs text-muted-foreground">Ctrl+Enter to render</span>
              </div>

              {rendering && progress && progress.total > 0 && (
                <div className="flex flex-col gap-1.5">
                  <Progress value={progressPct} aria-label="render progress" />
                  <p className="text-xs text-muted-foreground">{progressPct}% · block render in worker</p>
                </div>
              )}

              {stale && (
                <Alert variant="warning" role="status">
                  <TriangleAlert aria-hidden="true" />
                  <AlertDescription>Displayed results belong to an earlier editor revision.</AlertDescription>
                </Alert>
              )}
              {workerError && (
                <Alert variant="destructive">
                  <CircleAlert aria-hidden="true" />
                  <AlertTitle>Worker error</AlertTitle>
                  <AlertDescription>{workerError}</AlertDescription>
                </Alert>
              )}
              {diags.length > 0 && (
                <Alert variant="destructive" aria-label="diagnostics">
                  <CircleAlert aria-hidden="true" />
                  <AlertTitle>Diagnostics</AlertTitle>
                  <AlertDescription>
                    <ul className="flex flex-col gap-1">
                      {diags.map((d, i) => (
                        <li key={i} className="leading-relaxed">
                          {d.line > 0 ? (
                            <Button
                              variant="link"
                              size="sm"
                              className="h-auto p-0 text-inherit underline"
                              onClick={() => jumpToLine(d.line)}
                            >
                              line {d.line}
                            </Button>
                          ) : (
                            <span>global</span>
                          )}
                          {': '}{d.code}: {d.message}
                        </li>
                      ))}
                    </ul>
                  </AlertDescription>
                </Alert>
              )}
            </CardContent>
          </Card>
        </section>

        {/* Results column */}
        <section aria-label="results" className="flex min-w-0 flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Activity className="size-4 text-primary" aria-hidden="true" />
                Results
              </CardTitle>
              <CardDescription>Stats, playback, spectrogram, and downloads.</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              {stats ? (
                <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <dt className="flex items-center gap-1 text-xs text-muted-foreground"><Gauge className="size-3" aria-hidden="true" />Sample rate</dt>
                    <dd className="mt-0.5 text-sm font-semibold">{stats.rate} Hz</dd>
                  </div>
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <dt className="flex items-center gap-1 text-xs text-muted-foreground"><Clock className="size-3" aria-hidden="true" />Duration</dt>
                    <dd className="mt-0.5 text-sm font-semibold">{stats.duration.toFixed(4)} s</dd>
                  </div>
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <dt className="flex items-center gap-1 text-xs text-muted-foreground"><Hash className="size-3" aria-hidden="true" />Samples</dt>
                    <dd className="mt-0.5 text-sm font-semibold">{stats.samples}</dd>
                  </div>
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <dt className="flex items-center gap-1 text-xs text-muted-foreground"><Activity className="size-3" aria-hidden="true" />Peak</dt>
                    <dd className="mt-0.5 text-sm font-semibold">
                      {stats.peak.toFixed(4)}
                      {peakWarn ? <span className="text-destructive"> · over full scale</span> : ''}
                    </dd>
                  </div>
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <dt className="flex items-center gap-1 text-xs text-muted-foreground"><TriangleAlert className="size-3" aria-hidden="true" />Over count</dt>
                    <dd className="mt-0.5 text-sm font-semibold">{stats.overCount}</dd>
                  </div>
                  <div className="rounded-lg border bg-muted/30 p-3">
                    <dt className="flex items-center gap-1 text-xs text-muted-foreground"><Timer className="size-3" aria-hidden="true" />Render time</dt>
                    <dd className="mt-0.5 text-sm font-semibold">{stats.elapsedMs} ms</dd>
                  </div>
                </dl>
              ) : (
                <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed p-8 text-center">
                  <AudioWaveform className="size-8 text-muted-foreground" aria-hidden="true" />
                  <p className="text-sm text-muted-foreground">No render yet. Press Render to synthesize audio.</p>
                </div>
              )}
              <p className="text-xs text-muted-foreground">Exports keep the source sample rate; hardware playback may resample.</p>
              {warnings.map((w, i) => (
                <Alert key={i} variant="warning" role="status">
                  <TriangleAlert aria-hidden="true" />
                  <AlertDescription>{w.code}: {w.message}</AlertDescription>
                </Alert>
              ))}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Play className="size-4 text-primary" aria-hidden="true" />
                Playback
              </CardTitle>
              <CardDescription>Browser audio only · volume is playback-only.</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <div className="flex flex-wrap items-center gap-2">
                <Button onClick={togglePlay} disabled={!stats} className="gap-2">
                  {playing ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}
                  {playing ? 'Pause' : 'Play'}
                </Button>
                <Button variant="outline" disabled={!stats} onClick={() => { audioRef.current?.stop(); setPlaying(false); setPos(0); }} className="gap-2">
                  <Square aria-hidden="true" /> Stop
                </Button>
                <Button variant="outline" disabled={!stats} onClick={() => { audioRef.current?.seek(0); void audioRef.current?.play(0); setPlaying(true); }} className="gap-2">
                  <RotateCcw aria-hidden="true" /> Replay
                </Button>
              </div>
              <div className="grid gap-4 sm:grid-cols-2" role="group" aria-label="playback volume">
                <div className="flex flex-col gap-2">
                  <div className="flex items-center justify-between">
                    <Label>Volume</Label>
                    <span className="font-mono text-xs text-muted-foreground">{volume.toFixed(2)}</span>
                  </div>
                  <Slider
                    value={[volume]}
                    min={0}
                    max={1}
                    step={0.01}
                    disabled={!stats}
                    onValueChange={([v]) => setVolume(v)}
                    aria-label="playback volume"
                  />
                </div>
                <div className="flex flex-col gap-2" role="group" aria-label="seek seconds">
                  <div className="flex items-center justify-between">
                    <Label>Seek</Label>
                    <span className="font-mono text-xs text-muted-foreground">
                      {pos.toFixed(2)} / {seekMax.toFixed(2)} s
                    </span>
                  </div>
                  <Slider
                    value={[Math.min(pos, seekMax)]}
                    min={0}
                    max={Math.max(seekMax, 0.001)}
                    step={0.01}
                    disabled={!stats}
                    onValueChange={([v]) => { audioRef.current?.seek(v); setPos(v); }}
                    aria-label="seek seconds"
                  />
                </div>
              </div>
              {playbackNote && (
                <Alert variant="warning" role="status">
                  <TriangleAlert aria-hidden="true" />
                  <AlertDescription>{playbackNote}</AlertDescription>
                </Alert>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <SlidersHorizontal className="size-4 text-primary" aria-hidden="true" />
                Spectrogram
              </CardTitle>
              <CardDescription>dBFS amplitude · linear frequency · hover for readout.</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="fft">FFT</Label>
                  <NativeSelect
                    id="fft"
                    value={String(opts.fftLen)}
                    onChange={(e) => setOpts({ ...opts, fftLen: parseInt(e.target.value, 10) })}
                  >
                    {[256, 512, 1024, 2048, 4096, 8192].map((v) => <option key={v} value={v}>{v}</option>)}
                  </NativeSelect>
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="hop">Hop</Label>
                  <Input
                    id="hop"
                    type="number"
                    min={1}
                    max={8192}
                    value={opts.hop}
                    onChange={(e) => setOpts({ ...opts, hop: parseInt(e.target.value || '256', 10) })}
                  />
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="dbmin">dB min</Label>
                  <Input
                    id="dbmin"
                    type="number"
                    step={1}
                    value={opts.dbMin}
                    onChange={(e) => setOpts({ ...opts, dbMin: parseFloat(e.target.value) })}
                  />
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="dbmax">dB max</Label>
                  <Input
                    id="dbmax"
                    type="number"
                    step={1}
                    value={opts.dbMax}
                    onChange={(e) => setOpts({ ...opts, dbMax: parseFloat(e.target.value) })}
                  />
                </div>
              </div>
              <div>
                <Button variant="secondary" onClick={handleApplySpec} disabled={!stats || applyingSpec} className="gap-2">
                  {applyingSpec && <Loader2 className="animate-spin" aria-hidden="true" />}
                  {applyingSpec ? 'Applying…' : 'Apply (no audio rerender)'}
                </Button>
              </div>
              {pngUrl ? (
                <>
                  <img
                    ref={imgRef}
                    src={pngUrl}
                    alt="spectrogram with time, frequency, and dBFS legend"
                    className="w-full cursor-crosshair rounded-lg border"
                    onMouseMove={onSpecHover}
                  />
                  <Badge variant="secondary" role="status" aria-live="polite" className="w-fit font-mono font-normal">
                    {cursor}
                  </Badge>
                  {displayMeta && (
                    <p className="text-xs text-muted-foreground">
                      Analysis: {displayMeta.fftLen}-pt Hann, hop {displayMeta.hop}, {displayMeta.frames} frames × {displayMeta.bins} bins.
                      Display max-hold to {displayMeta.plotW}×{displayMeta.plotH}; STFT resolution unchanged.
                    </p>
                  )}
                </>
              ) : (
                <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed p-8 text-center">
                  <FileImage className="size-8 text-muted-foreground" aria-hidden="true" />
                  <p className="text-sm text-muted-foreground">No spectrogram yet.</p>
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Download className="size-4 text-primary" aria-hidden="true" />
                Downloads
              </CardTitle>
              <CardDescription>Shared Go exporters · identical to the CLI.</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-wrap gap-2">
              {wavUrl && (
                <Button asChild variant="outline" className="gap-2">
                  <a href={wavUrl} download="spl.wav"><FileAudio aria-hidden="true" /> WAV (float32)</a>
                </Button>
              )}
              {pngUrl && (
                <Button asChild variant="outline" className="gap-2">
                  <a href={pngUrl} download="spectrogram.png"><FileImage aria-hidden="true" /> Spectrogram PNG</a>
                </Button>
              )}
              <Button variant="outline" onClick={downloadSpl} className="gap-2">
                <FileText aria-hidden="true" /> SPL source
              </Button>
            </CardContent>
          </Card>
        </section>
      </main>

      <footer className="mx-auto max-w-7xl px-4 pb-8">
        <Separator className="mb-3" />
        <p className="text-xs text-muted-foreground">
          All synthesis runs locally — the Go engine compiled to WebAssembly. No uploads, no servers.
        </p>
      </footer>
    </div>
  );
}

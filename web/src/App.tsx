import { useEffect, useMemo, useRef, useState } from 'react';
import { AudioWaveform, Play, Pause, Square, Download, SlidersHorizontal, Volume2, Plus, FileCode2, Loader2, X, Pencil, Sparkles } from 'lucide-react';
import { EXAMPLES } from './examples';
import { highlightSPL } from './highlight';
import { WorkerClient } from './workerClient';
import { AudioEngine } from './audioEngine';
import type { Diagnostic, DisplayMeta, RenderOpts, RenderResult } from './types';

const DEFAULTS: RenderOpts = { fftLen: 2048, hop: 256, dbMin: -100, dbMax: 0, plotW: 1000, plotH: 500 };
const time = (s: number) => `${Math.floor(s / 60).toString().padStart(2, '0')}:${(s % 60).toFixed(2).padStart(5, '0')}`;

interface UserSound { id: string; name: string; source: string; updatedAt: number; }

const STORAGE_KEY = 'spl-studio:sounds:v1';
const MAX_SOUNDS = 60;
const MAX_DROP_BYTES = 1_000_000; // matches Go BrowserLimits MaxInputBytes

function loadSounds(): UserSound[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((s: any): s is UserSound =>
      s && typeof s.id === 'string' && typeof s.name === 'string' && typeof s.source === 'string'
    ).slice(0, MAX_SOUNDS);
  } catch { return []; }
}

function persistSounds(next: UserSound[]): boolean {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
    return true;
  } catch { return false; }
}

const uid = () => `user-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
const safeFileStem = (name: string) => name.trim().replace(/[\\/:*?"<>|]/g, '').replace(/\s+/g, ' ').slice(0, 60) || 'sound';

export default function App() {
  const [source, setSource] = useState(EXAMPLES[3].source);
  const [example, setExample] = useState(EXAMPLES[3].id);
  const [revision, setRevision] = useState(0);
  const [resultRevision, setResultRevision] = useState(-1);
  const [result, setResult] = useState<RenderResult | null>(null);
  const [status, setStatus] = useState('Initializing');
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState<'render' | 'analysis' | null>(null);
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState('');
  const [diagnostics, setDiagnostics] = useState<Diagnostic[]>([]);
  const [urls, setUrls] = useState({ wav: '', png: '' });
  const [opts, setOpts] = useState(DEFAULTS);
  const [settings, setSettings] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [position, setPosition] = useState(0);
  const [volume, setVolume] = useState(1);
  const [cursor, setCursor] = useState('');
  const [cacheAvailable, setCacheAvailable] = useState(false);
  const [buildInfo, setBuildInfo] = useState({ go: '', builtAt: '' });
  const [sounds, setSounds] = useState<UserSound[]>(loadSounds);
  const [renaming, setRenaming] = useState(false);
  const [draftName, setDraftName] = useState('');
  const [dragging, setDragging] = useState(false);
  const renameDone = useRef(false);
  const dragCount = useRef(0);
  const client = useRef<WorkerClient | null>(null);
  const audio = useRef<AudioEngine | null>(null);
  const editor = useRef<HTMLTextAreaElement>(null);
  const gutter = useRef<HTMLDivElement>(null);
  const highlight = useRef<HTMLPreElement>(null);
  const highlighted = useMemo(() => highlightSPL(source), [source]);
  const canvas = useRef<HTMLCanvasElement>(null);
  const request = useRef(0);
  const loadedPCM = useRef<ArrayBuffer | null>(null);
  const urlsRef = useRef(urls);
  const volumeRef = useRef(volume);
  volumeRef.current = volume;
  const selection = useRef(0);

  useEffect(() => {
    let active = true;
    const c = new WorkerClient(); client.current = c;
    const a = new AudioEngine(); audio.current = a;
    a.onEnded = () => { setPlaying(false); setPosition(a.position); };
    a.onTick = setPosition;
    c.ready().then(() => { if (active) { setReady(true); setStatus('Ready'); } })
      .catch(e => { if (active) { setError(`WASM initialization failed: ${e.message}`); setStatus('Worker failed'); } });
    return () => {
      active = false; c.dispose(); a.dispose();
      Object.values(urlsRef.current).forEach(u => { if (u) URL.revokeObjectURL(u); });
    };
  }, []);

  // Build metadata (Go toolchain + compilation date) written by scripts/build-wasm.mjs.
  useEffect(() => {
    let active = true;
    const base = import.meta.env.BASE_URL || './';
    fetch(new URL(base + 'wasm-info.json', window.location.href).href)
      .then(r => (r.ok ? r.json() : null))
      .then(info => {
        if (!active || !info) return;
        const parts = String(info.goVersion || '').split(' ').filter(Boolean);
        const go = parts.length >= 3 ? parts[2] : String(info.goVersion || '');
        setBuildInfo({ go, builtAt: String(info.builtAt || '').split('T')[0] });
      })
      .catch(() => { /* footer falls back to SPL / 2.0 */ });
    return () => { active = false; };
  }, []);

  // Crop the shared Go PNG to the plot; exported PNGs retain their axes.
  useEffect(() => {
    const target = canvas.current;
    const meta = result?.displayMeta;
    if (!target || !urls.png || !meta) return;
    let active = true;
    const image = new Image();
    image.onload = () => {
      if (!active) return;
      target.width = meta.plotW; target.height = meta.plotH;
      target.getContext('2d')?.drawImage(image, meta.originX, meta.originY, meta.plotW, meta.plotH, 0, 0, meta.plotW, meta.plotH);
    };
    image.src = urls.png;
    return () => { active = false; };
  }, [urls.png, result?.displayMeta]);

  // Keep the highlight backdrop aligned when the source swaps programmatically
  // (scroll events cover typing/scrolling; this covers selection switches).
  useEffect(() => {
    const ta = editor.current, pre = highlight.current;
    if (ta && pre) { pre.scrollTop = ta.scrollTop; pre.scrollLeft = ta.scrollLeft; }
  });

  function replaceUrls(next: { wav: string; png: string }) {
    const previous = urlsRef.current;
    for (const key of ['wav', 'png'] as const) if (previous[key] && previous[key] !== next[key]) URL.revokeObjectURL(previous[key]);
    urlsRef.current = next; setUrls(next);
  }
  const blobURL = (buffer: ArrayBuffer, type: string) => URL.createObjectURL(new Blob([buffer], { type }));

  function clearResults() {
    selection.current++;
    audio.current?.stop();
    loadedPCM.current = null;
    setPlaying(false); setPosition(0); setCursor('');
    setResult(null); setResultRevision(-1); setCacheAvailable(false);
    replaceUrls({ wav: '', png: '' });
    const c = canvas.current;
    if (c) c.getContext('2d')?.clearRect(0, 0, c.width, c.height);
    if (!busy) { setStatus('Ready'); setError(''); setDiagnostics([]); }
  }

  async function render() {
    if (!client.current || busy || !ready) return;
    const id = ++request.current;
    const sel = selection.current;
    setBusy('render'); setError(''); setDiagnostics([]); setProgress(0); setStatus('Rendering');
    try {
      const response = await client.current.render(source, opts, revision, (done, total) => {
        if (id === request.current) setProgress(total ? Math.min(100, done / total * 100) : 0);
      });
      if (id !== request.current) return;
      if (sel !== selection.current) { setStatus('Ready'); return; }
      if (!response.ok || !response.result) { setDiagnostics(response.diagnostics ?? []); setStatus('Render failed'); return; }
      const next = response.result;
      audio.current?.stop(); loadedPCM.current = null; setPlaying(false); setPosition(0);
      setResult(next); setResultRevision(response.revision); setCacheAvailable(true);
      replaceUrls({ wav: next.wav ? blobURL(next.wav, 'audio/wav') : '', png: next.png ? blobURL(next.png, 'image/png') : '' });
      setStatus('Render complete');
      // Play the sound once right after rendering finishes.
      try {
        const engine = audio.current;
        if (engine && next.pcm) {
          const note = await engine.setData(next.pcm, next.stats.rate);
          if (id !== request.current || sel !== selection.current) return;
          loadedPCM.current = next.pcm;
          engine.setVolume(volumeRef.current);
          if (note.message) setError(note.message);
          await engine.play(0);
          if (id !== request.current || sel !== selection.current) { engine.stop(); return; }
          setPlaying(true); setPosition(0);
        }
      } catch (e) {
        if (id === request.current && sel === selection.current) setError(`Playback failed: ${String(e)}`);
      }
    } catch (e) { if (id === request.current && sel === selection.current) { setError(String(e)); setStatus('Render failed'); } }
    finally { if (id === request.current) setBusy(null); }
  }

  async function cancel() {
    ++request.current; setReady(false); setStatus('Cancelling'); setCacheAvailable(false);
    try { await client.current?.cancelAndRecreate(); setReady(true); setStatus('Cancelled'); }
    catch (e) { setError(String(e)); setStatus('Worker failed'); }
    finally { setBusy(null); }
  }

  async function apply() {
    if (!client.current || busy || !cacheAvailable) return;
    const id = ++request.current;
    const sel = selection.current;
    setBusy('analysis'); setError(''); setDiagnostics([]); setStatus('Analyzing');
    try {
      const response = await client.current.spectrogram(opts);
      if (id !== request.current) return;
      if (sel !== selection.current) { setStatus('Ready'); return; }
      if (!response.ok) { setDiagnostics(response.diagnostics ?? []); setStatus('Analysis failed'); return; }
      if (response.png) replaceUrls({ ...urlsRef.current, png: blobURL(response.png, 'image/png') });
      setResult(previous => previous ? { ...previous, display: response.display ?? null, displayMeta: response.displayMeta ?? null } : previous);
      setStatus('Spectrogram updated');
    } catch (e) { if (id === request.current && sel === selection.current) setError(String(e)); }
    finally { if (id === request.current) setBusy(null); }
  }

  async function togglePlay() {
    const engine = audio.current;
    if (!engine || !result?.pcm) return;
    try {
      if (playing) { engine.pause(); setPlaying(false); return; }
      if (loadedPCM.current !== result.pcm) {
        const note = await engine.setData(result.pcm, result.stats.rate);
        loadedPCM.current = result.pcm;
        engine.setVolume(volume);
        if (note.message) setError(note.message);
      }
      await engine.play(position); setPlaying(true);
    } catch (e) { setError(`Playback failed: ${String(e)}`); }
  }

  function seek(value: number) { audio.current?.seek(value); setPosition(value); }
  function jump(line: number) {
    const lines = source.split('\n'); const start = lines.slice(0, line - 1).join('\n').length + (line > 1 ? 1 : 0);
    editor.current?.focus(); editor.current?.setSelectionRange(start, start + (lines[line - 1]?.length ?? 0));
  }
  function switchEntry(nextSource: string, nextId: string) {
    if (nextId === example) return;
    clearResults();
    setSource(nextSource); setExample(nextId); setRevision(r => r + 1);
  }
  function selectExample(index: number) { const ex = EXAMPLES[index]; if (!ex) return; switchEntry(ex.source, ex.id); }
  function selectUserSound(id: string) { const s = sounds.find(x => x.id === id); if (!s) return; switchEntry(s.source, s.id); }
  function storeSounds(next: UserSound[]) {
    setSounds(next);
    if (!persistSounds(next)) setError('Could not save to local storage (quota exceeded?).');
  }
  function nextSoundName(base: string): string {
    const taken = new Set(sounds.map(s => s.name));
    if (!taken.has(base)) return base;
    for (let i = 2; ; i++) { const name = `${base} ${i}`; if (!taken.has(name)) return name; }
  }
  function libraryFull(): boolean {
    if (sounds.length < MAX_SOUNDS) return false;
    setError(`Sound library is full (${MAX_SOUNDS} sounds). Remove one first.`);
    return true;
  }
  function newSound() {
    if (libraryFull()) return;
    const s: UserSound = { id: uid(), name: nextSoundName('New sound'), source: '', updatedAt: Date.now() };
    storeSounds([...sounds, s]);
    clearResults();
    setSource(''); setExample(s.id); setRevision(r => r + 1); editor.current?.focus();
  }
  function removeSound(id: string) {
    storeSounds(sounds.filter(s => s.id !== id));
    if (example === id) { clearResults(); setExample('custom'); setRevision(r => r + 1); } // keep editor text as an unsaved draft
  }
  function updateSource(value: string) {
    setSource(value);
    setRevision(r => r + 1);
    if (sounds.some(s => s.id === example)) {
      storeSounds(sounds.map(s => s.id === example ? { ...s, source: value, updatedAt: Date.now() } : s));
    } else {
      setExample('custom');
    }
  }
  function startRename() { renameDone.current = false; setDraftName(soundTitle); setRenaming(true); }
  function finishRename(cancelled: boolean) {
    if (renameDone.current) return; // blur fires again when Enter/Escape unmounts the input
    renameDone.current = true;
    setRenaming(false);
    if (cancelled) return;
    const name = draftName.trim().slice(0, 80);
    if (!name) return;
    const current = sounds.find(s => s.id === example);
    if (current) {
      if (current.name !== name) storeSounds(sounds.map(s => s.id === current.id ? { ...s, name, updatedAt: Date.now() } : s));
    } else if (!libraryFull()) {
      // Renaming a built-in example or an unsaved draft forks it into a saved user sound.
      const s: UserSound = { id: uid(), name, source, updatedAt: Date.now() };
      storeSounds([...sounds, s]);
      setExample(s.id);
    }
  }
  function downloadSource() {
    const url = URL.createObjectURL(new Blob([source], { type: 'text/plain' }));
    const link = document.createElement('a'); link.href = url; link.download = `${safeFileStem(soundTitle)}.spl`; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  function onDragEnter(e: React.DragEvent) {
    if (!e.dataTransfer.types.includes('Files')) return;
    e.preventDefault();
    dragCount.current++;
    setDragging(true);
  }
  function onDragOver(e: React.DragEvent) {
    if (!e.dataTransfer.types.includes('Files')) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = 'copy';
  }
  function onDragLeave(e: React.DragEvent) {
    if (!e.dataTransfer.types.includes('Files')) return;
    dragCount.current = Math.max(0, dragCount.current - 1);
    if (dragCount.current === 0) setDragging(false);
  }
  async function onDrop(e: React.DragEvent) {
    e.preventDefault();
    dragCount.current = 0; setDragging(false);
    const files = [...(e.dataTransfer.files ?? [])];
    const file = files.find(f => f.name.toLowerCase().endsWith('.spl'));
    if (!file) {
      if (files.length) setError('Only .spl files can be dropped onto the source editor.');
      return;
    }
    if (file.size > MAX_DROP_BYTES) { setError(`"${file.name}" exceeds the 1 MB source limit.`); return; }
    if (libraryFull()) return;
    try {
      const text = await file.text();
      const s: UserSound = { id: uid(), name: nextSoundName(safeFileStem(file.name.replace(/\.spl$/i, ''))), source: text, updatedAt: Date.now() };
      storeSounds([...sounds, s]);
      clearResults();
      setSource(text); setExample(s.id); setRevision(r => r + 1);
    } catch { setError(`Could not read "${file.name}".`); }
  }
  function inspect(e: React.MouseEvent<HTMLCanvasElement>, meta: DisplayMeta) {
    const bounds = e.currentTarget.getBoundingClientRect();
    const x = Math.max(0, Math.min(meta.plotW - 1, Math.floor((e.clientX - bounds.left) / bounds.width * meta.plotW)));
    const y = Math.max(0, Math.min(meta.plotH - 1, Math.floor((e.clientY - bounds.top) / bounds.height * meta.plotH)));
    const db = result?.display ? new DataView(result.display).getFloat32((y * meta.plotW + x) * 4, true) : 0;
    setCursor(`${((x + 0.5) / meta.plotW * meta.duration).toFixed(2)} s · ${((1 - (y + 0.5) / meta.plotH) * meta.nyquist / 1000).toFixed(2)} kHz · ${db.toFixed(1)} dBFS`);
  }
  const duration = result ? result.stats.samples / result.stats.rate : 0;
  const meta = result?.displayMeta;
  const stale = result && revision !== resultRevision;
  const selectedUserSound = sounds.find(s => s.id === example) ?? null;
  const soundTitle = selectedUserSound ? selectedUserSound.name : example === 'custom' ? 'Untitled sound' : (EXAMPLES.find(x => x.id === example)?.label ?? 'Untitled sound');
  const footerVersion = (buildInfo.go || buildInfo.builtAt)
    ? `SPL / 2.0 / ${buildInfo.go || 'Go ?'} / ${buildInfo.builtAt || '?'}`
    : 'SPL / 2.0';

  return <div className="studio">
    <main className="main">
      <header className="topbar"><div><h1>{renaming ? <input className="title-input" aria-label="Sound name" value={draftName} autoFocus maxLength={80} onFocus={e => e.currentTarget.select()} onChange={e => setDraftName(e.target.value)} onBlur={() => finishRename(false)} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); finishRename(false); } else if (e.key === 'Escape') { e.preventDefault(); finishRename(true); } }} /> : <button className="title-button" onClick={startRename} title="Click to rename" aria-label={`Rename sound ${soundTitle}`}><span>{soundTitle}</span><Pencil size={17} className="title-pencil" /></button>}</h1></div><div className="sr-only" role="status" aria-live="polite">{status}</div></header>
      <div className="workbench">
        <section className={`source-panel${dragging ? ' drag-over' : ''}`} aria-label="SPL editor" onDragEnter={onDragEnter} onDragOver={onDragOver} onDragLeave={onDragLeave} onDrop={e => { void onDrop(e); }}>
          {dragging && <div className="drop-hint"><Download size={20} /> Drop .spl file to load</div>}
          <div className="panel-heading"><span><FileCode2 size={19} /> Source</span><span className="file-label" title={`${safeFileStem(soundTitle)}.spl`}>{safeFileStem(soundTitle)}.spl</span></div>
          <div className="editor-wrap"><div ref={gutter} className="line-numbers" aria-hidden="true">{source.split('\n').map((_, i) => <div key={i}>{i + 1}</div>)}</div><div className="editor-stack"><pre ref={highlight} className="editor-highlight" aria-hidden="true"><code dangerouslySetInnerHTML={{ __html: highlighted }} /></pre><textarea ref={editor} aria-label="SPL source" spellCheck={false} autoCapitalize="off" autoCorrect="off" value={source} onChange={e => updateSource(e.target.value)} onScroll={() => { const ta = editor.current; if (!ta) return; if (gutter.current) gutter.current.scrollTop = ta.scrollTop; if (highlight.current) { highlight.current.scrollTop = ta.scrollTop; highlight.current.scrollLeft = ta.scrollLeft; } }} onKeyDown={e => { if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') { e.preventDefault(); void render(); } }} /></div></div>
          <div className="editor-actions"><span>{source === '' ? 'Empty' : `${source.split('\n').length} lines`}</span><div className="editor-buttons"><button className="action-button" onClick={downloadSource} title="Download sound.spl"><Download size={16} /> Save source</button>{busy ? <button className="action-button" onClick={cancel}><X size={17} /> Cancel</button> : <button className="action-button primary" disabled={!ready} onClick={render}><Sparkles size={16} /> Render</button>}</div></div>
          {busy && <div className="render-progress" role="progressbar" aria-label="render progress" aria-valuenow={Math.round(progress)} aria-valuemin={0} aria-valuemax={100}><span style={{ width: `${progress}%` }} /></div>}
        </section>

        <section className="viewer-panel" aria-label="Spectrogram player">
          <div className="panel-heading"><span><AudioWaveform size={20} /> Spectrogram {stale && <span className="stale" title="Displayed results belong to an earlier editor revision.">Edited</span>}</span><button className={`icon-button ${settings ? 'active' : ''}`} aria-label="Analysis settings" aria-expanded={settings} onClick={() => setSettings(!settings)}><SlidersHorizontal size={19} /></button></div>
          {settings && <div className="analysis-settings"><label>FFT<select value={opts.fftLen} onChange={e => setOpts({ ...opts, fftLen: +e.target.value })}>{[256, 512, 1024, 2048, 4096, 8192].map(v => <option key={v}>{v}</option>)}</select></label><label>Hop<input type="number" min="1" max="8192" value={opts.hop} onChange={e => setOpts({ ...opts, hop: +e.target.value })} /></label><label>dB min<input type="number" value={opts.dbMin} onChange={e => setOpts({ ...opts, dbMin: +e.target.value })} /></label><label>dB max<input type="number" value={opts.dbMax} onChange={e => setOpts({ ...opts, dbMax: +e.target.value })} /></label><button className="action-button" disabled={!cacheAvailable || !!busy} onClick={apply}>{busy === 'analysis' ? <Loader2 className="spin" size={16} /> : 'Apply'}</button></div>}
          <div className="visualizer">
            <div className="frequency-axis">{[1, 0.75, 0.5, 0.25, 0].map(v => <span key={v}>{((meta?.nyquist ?? 12000) * v / 1000).toFixed(1)}k</span>)}</div>
            <div className="plot">
              <canvas ref={canvas} aria-label="spectrogram with time and frequency axes" onMouseMove={e => { if (meta) inspect(e, meta); }} onMouseLeave={() => setCursor('')} onClick={e => { if (duration) { const rect = e.currentTarget.getBoundingClientRect(); seek(Math.max(0, Math.min(duration, (e.clientX - rect.left) / rect.width * duration))); } }} />
              {!result && <div className="empty-plot"><AudioWaveform size={38} strokeWidth={1} /><span>No audio rendered</span></div>}
              {result && <div className="playhead" style={{ left: `${Math.min(100, duration ? position / duration * 100 : 0)}%` }}><span /></div>}
            </div>
            <div className="time-axis">{[0, 0.25, 0.5, 0.75, 1].map(v => <span key={v}>{(duration * v).toFixed(2)} s</span>)}</div>
          </div>
          <div className="plot-caption"><span>{cursor || (meta ? `${meta.fftLen} FFT / ${meta.hop} HOP` : 'SPECTRAL VIEW')}</span><span className="color-key"><span />{meta?.dbMin ?? -100} → {meta?.dbMax ?? 0} dBFS</span></div>
          <div className="transport"><button className="play-button" disabled={!result?.pcm} onClick={togglePlay} aria-label={playing ? 'Pause' : 'Play'}>{playing ? <Pause size={20} fill="currentColor" /> : <Play size={20} fill="currentColor" />}</button><button className="icon-button" disabled={!result} aria-label="Stop" onClick={() => { audio.current?.stop(); setPosition(0); setPlaying(false); }}><Square size={16} /></button><span className="time-display">{time(position)} <span>/ {time(duration)}</span></span><div className="volume"><Volume2 size={18} /><input type="range" aria-label="playback volume" min="0" max="1" step="0.01" value={volume} onChange={e => { const v = +e.target.value; setVolume(v); audio.current?.setVolume(v); }} /></div></div>
          <input className="seek" type="range" aria-label="seek seconds" min="0" max={duration || 1} step="0.001" disabled={!result} value={Math.min(position, duration)} onChange={e => seek(+e.target.value)} />
          <div className="viewer-footer"><div className="metrics">{result ? <><span aria-label="Sample rate">{(result.stats.rate / 1000).toFixed(1)} <small>kHz</small></span><span>MONO</span><span className={result.stats.peak > 1 ? 'hot' : ''}>{result.stats.peak.toFixed(3)} <small>peak</small></span></> : <span>MONO / FLOAT32</span>}</div><div className="exports">{urls.wav && <a href={urls.wav} download="sound.wav"><Download size={15} /> WAV</a>}{urls.png && <a href={urls.png} download="spectrogram.png"><Download size={15} /> PNG</a>}</div></div>
        </section>
      </div>
      <section className="library-bar" aria-label="Sound library">
        <a className="brand" href="./" aria-label="SPL home"><span className="brand-icon"><AudioWaveform size={20} /></span><strong>SPL</strong></a>
        <nav className="library-chips" aria-label="Examples">{EXAMPLES.map((ex, index) => <button key={ex.id} className={`chip ${example === ex.id ? 'selected' : ''}`} onClick={() => selectExample(index)} aria-current={example === ex.id ? 'true' : undefined}><span className={`example-dot dot-${index % 5}`} /><span>{ex.label}</span></button>)}{sounds.map((s, i) => <span key={s.id} className={`chip chip-user${example === s.id ? ' selected' : ''}`}><button className="chip-main" onClick={() => selectUserSound(s.id)} aria-current={example === s.id ? 'true' : undefined}><span className={`example-dot dot-${(EXAMPLES.length + i) % 5}`} /><span>{s.name}</span></button><button className="chip-remove" onClick={() => removeSound(s.id)} aria-label={`Remove ${s.name}`} title={`Remove ${s.name}`}><X size={14} /></button></span>)}</nav>
        <button className="new-button" onClick={newSound} aria-label="Create new sound" title="New sound"><Plus size={19} /><span>New sound</span></button>
      </section>
      {(error || diagnostics.length > 0 || (result?.warnings.length ?? 0) > 0) && <section className="diagnostics" role="alert" aria-label="diagnostics">{error && <p>{error}</p>}{diagnostics.map((d, i) => <p key={i}><button onClick={() => jump(d.line)}>{d.line ? `Line ${d.line}` : d.code}</button> {d.code}: {d.message}</p>)}{result?.warnings.map((d, i) => <p key={`warning-${i}`} className="hot">{d.code === 'PEAK_WARNING' ? `${result.stats.overCount} samples exceed full scale. Peak ${result.stats.peak.toFixed(3)}.` : d.message}</p>)}</section>}
      <footer className="version-footer"><span>{footerVersion}</span></footer>
    </main>
  </div>;
}

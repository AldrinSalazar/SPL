// Minimal Web Audio playback engine: mono AudioBuffer at source rate,
// explicit float32 handling, playback-only volume, seek/replay.
export class AudioEngine {
  private ctx: AudioContext | null = null;
  private buffer: AudioBuffer | null = null;
  private source: AudioBufferSourceNode | null = null;
  private gain: GainNode | null = null;
  private startedAt = 0;
  private offset = 0;
  private playing = false;
  private rate = 0;
  onEnded: (() => void) | null = null;
  onTick: ((pos: number) => void) | null = null;
  private tickTimer: number | null = null;

  get currentRate(): number { return this.rate; }
  get duration(): number { return this.buffer?.duration ?? 0; }
  get isPlaying(): boolean { return this.playing; }
  get position(): number {
    if (!this.buffer || !this.ctx) return this.offset;
    if (!this.playing) return this.offset;
    return Math.min(this.offset + (this.ctx.currentTime - this.startedAt), this.buffer.duration);
  }

  /** Create/resume context on user gesture; build buffer from float32 LE bytes. */
  async setData(pcmBytes: ArrayBuffer, rate: number): Promise<{ resampled: boolean; message?: string }> {
    const count = pcmBytes.byteLength / 4;
    const view = new DataView(pcmBytes);
    const data = new Float32Array(count);
    for (let i = 0; i < count; i++) data[i] = view.getFloat32(i * 4, true);
    if (!this.ctx) {
      this.ctx = new AudioContext();
      this.gain = this.ctx.createGain();
      this.gain.gain.value = 0.2;
      this.gain.connect(this.ctx.destination);
    }
    if (this.ctx.state === 'suspended') {
      try { await this.ctx.resume(); } catch { /* ignore */ }
    }
    let resampled = false;
    let message: string | undefined;
    let buf: AudioBuffer | null = null;
    try {
      buf = new AudioBuffer({ numberOfChannels: 1, length: Math.max(1, count), sampleRate: rate });
      buf.getChannelData(0).set(data.subarray(0, buf.length));
    } catch (e: any) {
      // Playback-only resampling fallback; exports retain source rate.
      resampled = true;
      message = `Playback rate ${rate} Hz rejected by browser (${e?.message ?? e}); using resampled playback at 48000 Hz. Exports keep ${rate} Hz.`;
      const target = 48000;
      const ratio = target / rate;
      const newLen = Math.max(1, Math.round(count * ratio));
      buf = new AudioBuffer({ numberOfChannels: 1, length: newLen, sampleRate: target });
      const out = buf.getChannelData(0);
      for (let i = 0; i < newLen; i++) {
        const srcPos = i / ratio;
        const i0 = Math.floor(srcPos);
        const frac = srcPos - i0;
        const a = data[Math.min(i0, count - 1)] ?? 0;
        const b = data[Math.min(i0 + 1, count - 1)] ?? 0;
        out[i] = a + (b - a) * frac;
      }
    }
    this.stopInternal();
    this.buffer = buf;
    this.rate = rate;
    this.offset = 0;
    return { resampled, message };
  }

  setVolume(v: number) {
    if (this.gain && this.ctx) this.gain.gain.value = v;
  }

  async play(fromOffset?: number): Promise<void> {
    if (!this.ctx || !this.buffer || !this.gain) return;
    if (this.ctx.state === 'suspended') {
      try { await this.ctx.resume(); } catch { /* ignore */ }
    }
    this.stopInternal();
    if (fromOffset !== undefined) this.offset = fromOffset;
    if (this.offset >= this.buffer.duration) this.offset = 0;
    const src = this.ctx.createBufferSource();
    src.buffer = this.buffer;
    src.connect(this.gain);
    src.onended = () => {
      if (this.source === src) {
        const wasPlaying = this.playing;
        this.playing = false;
        this.source = null;
        this.stopTick();
        if (wasPlaying) {
          // Natural end: reset offset to end (allow replay).
          this.offset = this.buffer ? Math.min(this.offset + 1e9, this.buffer.duration) : 0;
          // Actually set to duration for seek display; replay resets.
          if (this.buffer && this.position >= this.buffer.duration - 1e-6) {
            this.offset = this.buffer.duration;
          }
          this.onEnded?.();
        }
      }
    };
    this.source = src;
    this.startedAt = this.ctx.currentTime;
    src.start(0, this.offset % Math.max(1e-9, this.buffer.duration));
    this.playing = true;
    this.startTick();
  }

  pause() {
    if (!this.playing || !this.ctx) return;
    this.offset = this.position;
    this.stopInternal();
    this.playing = false;
  }

  stop() {
    this.offset = 0;
    this.stopInternal();
    this.playing = false;
  }

  seek(pos: number) {
    const wasPlaying = this.playing;
    if (wasPlaying) {
      this.stopInternal();
      this.playing = false;
    }
    this.offset = Math.max(0, Math.min(pos, this.duration));
    if (wasPlaying) void this.play(this.offset);
    this.onTick?.(this.offset);
  }

  private stopInternal() {
    if (this.source) {
      try { this.source.onended = null; this.source.stop(); } catch { /* ignore */ }
      try { this.source.disconnect(); } catch { /* ignore */ }
      this.source = null;
    }
    this.stopTick();
  }

  private startTick() {
    this.stopTick();
    this.tickTimer = window.setInterval(() => this.onTick?.(this.position), 100);
  }

  private stopTick() {
    if (this.tickTimer !== null) {
      clearInterval(this.tickTimer);
      this.tickTimer = null;
    }
  }

  dispose() {
    this.stopInternal();
    this.playing = false;
    if (this.ctx) {
      void this.ctx.close().catch(() => undefined);
      this.ctx = null;
    }
    this.buffer = null;
  }
}

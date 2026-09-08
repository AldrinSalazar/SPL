# SPL audio compiler and browser application

SPL version 2 describes sound by drawing frequency tracks, harmonic spectra, noise regions, and attacks. This repo implements the language normatively defined in `SPL-2-Language-Specification.md` plus:

1. Reusable Go library: parse/validate, deterministic float64 render, spectrogram, WAV/PNG export.
2. Native Go CLI (`spl`).
3. Static React/TypeScript/Vite web app running the same Go engine compiled to WebAssembly in a dedicated Worker.

No synthesis server, uploads, or separate JS renderer. All audio work runs locally (native or in-browser WASM).

## Studio interface

The dark workspace has an example library, source editor, and an integrated spectrogram player. Click the plot or use the seek slider to move playback; Play/Pause and Stop sit directly below the plot. Audio initialization happens on Play. Volume affects playback only. WAV and labeled PNG exports are in the viewer footer.

Open **Analysis settings** to change FFT, hop, or display range, then Apply. Settings and rendering are serialized so results stay together. Failed exports preserve the previous successful audio and analysis cache. Cancelling recreates the worker; render again before applying new analysis settings. An **Edited** badge marks results from an earlier source revision.

Regression coverage includes failed WASM initialization, cache preservation after a failed export, playback and plot seeking, mobile layout, exact harmonic spectrum endpoints, comments containing fences, and spectrogram analysis of very large finite signals. Run browser checks with `npm run test:e2e --prefix web` (use `npm.cmd` on Windows if PowerShell blocks the script launcher).

## Prerequisites and tested versions

- Go `go1.27.1 windows/amd64` (any supported Go ≥1.23 with `GOOS=js GOARCH=wasm` should work; WASM runtime JS is resolved from the exact toolchain via `go env GOROOT`).
- Node `v24.13.1`, npm `11.8.0`.
- Web deps (see `web/package-lock.json`): `react 18.3.1`, `react-dom 18.3.1`, `vite 6.4.3`, `typescript 5.9.3`, `@vitejs/plugin-react 4.7.0`, `tailwindcss 3.4.19`, `lucide-react 1.42.0`, `vitest 2.1.9`, `@playwright/test 1.63.0`.
- Go deps (`go.mod`): `golang.org/x/image v0.45.0` (PNG font rendering), plus `x/sys`, `x/text` (indirect).

## Quick start

```sh
# Native CLI
go build -o spl ./cmd/spl
./spl validate examples/metallic-impact.spl
./spl render examples/metallic-impact.spl --wav output.wav --force
./spl render examples/metallic-impact.spl --wav output.wav --spectrogram output.png --force
./spl render examples/metallic-impact.spl --spectrogram output.png --force

# Web production build (single command; also builds/verifies fresh WASM assets)
npm run build --prefix web
# Serve statically, e.g.:
npm run preview --prefix web -- --port 4173
# open http://localhost:4173/

# Web development
npm run dev --prefix web
```

Reproducible scripts: `scripts/build-all.sh` / `scripts/build-all.ps1` run vet, Go tests, native + WASM builds, and the web production build.

## CLI usage

```
spl validate [--json] <file.spl>
spl render [--json] [--force] --wav <out.wav> [--spectrogram <out.png>] [spec opts] <file.spl>
spl render [--json] [--force] --spectrogram <out.png> [spec opts] <file.spl>
spl --help | --version
```

Spectrogram opts: `--fft <pow2>`, `--hop <n>`, `--db-min <f>`, `--db-max <f>`, `--plot-width <n>`, `--plot-height <n>`.

- Mono IEEE float32 WAV default; preserves rate, count, and over-full-scale values. float64→float32 conversion is explicit and overflow-checked. No integer WAV, normalization, clipping, or fades.
- Diagnostics, warnings, and stats (`duration rate samples peak over elapsedMs`) go to stderr; exit 0 on success, nonzero on validation/resource/render/export errors. `--json` emits structured `{ok, diagnostics, stats}` on stdout.
- Outputs use temp files + atomic rename; existing files require `--force`. No partial artifacts on failure.
- Spectrogram-only output still renders PCM first (analysis runs on final mixed float64 PCM).

## Web architecture (local processing)

```
React UI (main thread)
  ↕ typed request/response {id, type, ...} + transferable ArrayBuffers
Dedicated Worker (web/src/worker/spl.worker.ts)
  ↕ Go WASM bridge (cmd/splwasm): splValidate / splRender / splSpectrogram
  Go engine: pkg/spl + pkg/synth + pkg/spectrogram + pkg/audio (same code as CLI)
```

- WASM built with `GOOS=js GOARCH=wasm go build -o web/public/spl.wasm ./cmd/splwasm`; runtime JS copied from `$(go env GOROOT)/lib/wasm/wasm_exec.js` (fallback `misc/wasm`) by `web/scripts/build-wasm.mjs`, which also writes `web/public/wasm-info.json`. `npm run build` / `npm run dev` run this automatically (pre hooks), so users never assemble runtime files manually.
- Worker loads `wasm_exec.js` + `spl.wasm` via `WebAssembly.instantiateStreaming` with ArrayBuffer fallback. All synthesis/analysis/export runs in the worker; UI stays responsive.
- Binary results (WAV, float32 PCM, PNG, display dB matrix) transfer as owned `ArrayBuffer`s (Go `CopyBytesToJS` → worker copy → transfer to main). Never JSON arrays for PCM/matrices.
- Cancel terminates and recreates the worker (Go work is synchronous and blocks worker messages); next request initializes cleanly. Stale responses discarded by request ID; blob URLs revoked on replace.
- Render caches float64 PCM + spectrogram matrix in the worker. Spectrogram “Apply” reuses cache: FFT/hop changes recompute STFT from cached PCM (no synthesis); dB-range-only changes re-encode PNG from cached matrix (no STFT). Display colors never alter PCM/analysis values.
- Playback uses Web Audio `AudioBuffer` (mono, source rate, explicit float32 data). Context created/resumed on Play gesture. Volume (default 0.2) is a playback-only `GainNode`; exports unaltered. If the browser rejects the source rate, UI shows a playback-only error and offers resampled (48 kHz linear) playback; exports keep the source rate.
- Editor never executes input as JS/HTML (plain `<textarea>`, text-only diagnostics).

## Synthesis conformance notes

- Exact syntax: header `spl 2 RATE DURATION SEED`, `track`, `harmonics` (`spectrum` then `curve`), `noise`, `hit`, `end`. No extensions. Integer `[0-9]+` and general-number `-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?` enforced before conversion; nonfinite, fences, unknown commands, extra/missing fields rejected. Comments (`#`), blank lines, spaces/tabs, LF/CRLF, omitted final newline supported; physical line numbers preserved.
- `N = ceil(RATE * DURATION)` via exact decimal (`math/big`) on the original token, including exponents; token length and exponent bounded before big-int construction. Bounds checked before allocation.
- Track/harmonics integrate piecewise-linear frequency analytically (`f0*d + m*d*d/2`), cosine phase 0 at `t0`, carried through silent sections. Harmonics normalize by `Z(t)` (zero → silence), increasing `k`, spectrum-linear in Hz, phase agreement for shared pitch/start. Optimized to spectrum-overlapping `k` only (zeros skipped, order preserved).
- Noise: `L=2048 H=256` periodic Hann, fractional cell coverage, slope `k^-SLOPE`, DC/Nyquist zero, negative frames included, clamped frame-center params, per-sample gain, overlap normalization `1/sqrt(sum w²)`. Hash uses wrapping `uint32`, logical shifts, `m mod 2³²`; only noise blocks numbered. Fast path uses IFFT (`X[k]=(L/2)(w/Z)e^{i2πφ}`, conjugate symmetry, `1/L` scaling); verified against direct cosine-sum oracle (max diff ≤1e-9 in tests). Each nonzero frame has unit RMS before windowing.
- Hits: `w/Z` normalization, squared-sine envelope at actual sample times, center-phase alignment, no frame quantization, row-order summation, `|signal|≤GAIN`.
- Mixing in document/row order, single-threaded accumulation (no order-changing parallelism). No normalize/clip/limit/fade/filter/oversample. Peak>1 warns (`PEAK_WARNING`). Nonfinite/overflow → `RENDER_ERROR`, no partial audio.
- IFFT used only for noise frames per spec allowance; never for off-grid hits.
- Determinism: repeated same-build renders are bit-identical (verified in `integration` tests). Cross-platform bit identity not promised; native (amd64) vs browser (wasm) compared with absolute/relative tolerance `1e-5` on float32 PCM (see `web/tests/parity.spec.ts`). Observed on Ryzen 5 560X + Chromium 153: `maxAbs=0`, PNG byte-identical for minimal/metallic/two-noises. Tolerance rationale: different libm/assembly for `cos/sin/pow/log`, FFT rounding, and floating summation order may differ in last ulps; float32 quantization adds ~1e-7 relative.

Genuine ambiguities and interpretations (semantics preserved):

- `f(k)=k*RATE/L` computed as `float64(k)*float64(RATE)/float64(L)`; `(f(k)/(RATE/L))^-SLOPE` computed as `k^-SLOPE` (mathematically equal, avoids extra rounding).
- Frame-center `(s+L/2)/RATE` with `s=m*H` as float64; half-open intervals via `float64(n)/RATE` comparisons with integer-boundary adjustment (no epsilon).
- Hit `TIME+LENGTH<=DURATION` via binary64 addition.
- Noise denominator includes all covering windows (8 per sample) even for zero-spectrum frames, per formula.

## Spectrogram conventions (analysis, not language semantics)

- Analyzes final mixed float64 PCM before WAV quantization. Defaults: 2048 periodic Hann, hop 256, one-sided real STFT with DC+Nyquist.
- Frames centered at `0,hop,... < N`, zero-padded outside; ≥1 frame always. First/last frames are half-padded (boundary effect documented in UI metadata).
- Amplitude `|FFT|/sum(window)` (DC/Nyquist), doubled interior; `20log10(amplitude)` dBFS amplitude (not PSD). Bin-centered amplitude-1 interior tone ≈0 dB away from edges. Floor `-200` dBFS (finite, no NaN/Inf); display default `-100..0`, configurable upper bound for hot signals.
- Linear frequency `0..Nyquist`; time seconds; PNG includes axes (s, Hz/kHz) and dBFS legend. No log view (optional per spec; would handle DC explicitly if added).
- Time-major matrix `[frames][bins]`, explicit `FFTLen/hop/window/dB-ref/binFreqs/frameTimes`. Display raster bounded (`plotW×plotH`, default 1000×500) via max-hold aggregation (peak-preserving); analysis resolution unchanged. Cursor maps PNG plot pixels → time/freq/display-dB via returned geometry + matrix.

## Resource limits

Native (`spl.DefaultLimits`) vs browser (`spl.BrowserLimits`, stricter):

| Limit | Native | Browser |
|---|---|---|
| Input bytes | 2,000,000 | 1,000,000 |
| Token length | 1024 | 1024 |
| Exponent abs | 10000 | 10000 |
| Blocks | 1000 | 500 |
| Rows/block | 10000 | 5000 |
| Total rows | 50000 | 20000 |
| Samples | 12,000,000 | 6,000,000 |
| Render memory (est.) | 256 MiB | 128 MiB |
| Harmonic evals (est.) | 300M | 50M |
| Noise frames | 50000 | 25000 |
| Hit bin evals (est.) | 300M | 50M |
| Spectrogram cells | 64M | 32M |
| FFT length | 256..8192 pow2 | same |
| Hop | 1..8192 | same |

Work estimated before allocation (including tiny-pitch harmonic blowup); violations return actionable `RESOURCE_LIMIT`, never silent truncation. Progress callbacks and `context.Context` cancellation checked per block/chunk (native API); browser Cancel uses worker termination.

## Playback vs export

- Exports (WAV/PNG) use shared Go exporters in CLI and browser; sample rate/count preserved; over-scale preserved in float WAV.
- Playback creates `AudioBuffer` at source rate from Go-converted float32; hardware may resample (noted in UI). Volume affects only the `GainNode`. Clipping warnings displayed; no normalization.

## Testing

```sh
go test ./... -count=1
go test ./pkg/spl/ -run=NONE -fuzz=FuzzParse -fuzztime=15s
go test -count=1 -run=NONE -bench . ./pkg/synth/ -benchtime=2x
npm run typecheck --prefix web
npm run build --prefix web
npx playwright test --prefix web  # serves dist via preview; 10 tests incl. parity
```

Measured (Ryzen 5 5600X, Windows, Go 1.26.1):

- `BenchmarkTonalThreeNotes`: ~13.5 ms/op (1.5 s harmonics)
- `BenchmarkNoiseTwoNoises`: ~44 ms/op (3 s, 2 noise blocks)
- `BenchmarkMetallicImpact`: ~5.8 ms/op (2 s mixed)
- Fuzz 15 s: 553k execs, no panics.
- Playwright 10/10 pass (Chromium 153): init, diagnostics, resource-limit, stale, WAV/PNG content verification, spectrogram apply, cancel→rerender, 3× native/browser parity (bit-identical observed).

Visually inspect exported PNGs (e.g., `output.png` from CLI) for axes/legend readability.

## Project layout

```
cmd/spl/            native CLI
cmd/splwasm/        js/wasm bridge (validate/render/spectrogram + cache)
pkg/spl/            lexer/parser/AST/validation/diagnostics/limits/preflight
pkg/synth/          deterministic float64 rendering + IFFT noise + hash
pkg/spectrogram/    STFT/dB/color/PNG + display raster for cursor mapping
pkg/audio/          float32 WAV encode + independent decoder
integration/        examples/determinism/CLI/WAV/PNG/over-scale tests
examples/           5 valid SPL example files
testdata/           conformance samples
web/                React/TS/Vite app + worker + e2e tests
scripts/            reproducible builds
```

Key APIs: `spl.Parse`, `spl.Validate`, `synth.Render`, `spectrogram.ComputeSpectrogram`, `audio.EncodeFloatWAV`, `spectrogram.EncodeSpectrogramPNG`. Core has no DOM/fs/`syscall/js`; errors are structured `Diagnostic{Code,Message,Line,...}` identical in CLI/browser.

## Dependencies and licenses

- Go: `golang.org/x/image` BSD-3 (font `basicfont` for PNG labels). Indirect `x/sys`, `x/text` BSD-3.
- Web: React/React-DOM MIT, Vite MIT, `@vitejs/plugin-react` MIT, TypeScript Apache-2.0, Tailwind CSS MIT, lucide-react ISC, Vitest MIT, Playwright Apache-2.0, `@types/*` MIT. See `web/package-lock.json` for full tree.
- No audio/network runtime deps; production web bundle is static + `spl.wasm` + `wasm_exec.js` (Go BSD-style, versioned in `web/public/wasm-info.json`).

## Known limitations

- No integer WAV export (float32 only, by design); no log-frequency spectrogram view.
- Noise/hit low-frequency resolution limited by specified `L=2048` window (use tracks for narrow lows).
- Signals beyond float32 range are rejected by WAV export. Spectrogram analysis scales large finite input before the FFT, retaining its original dB reference.
- Browser Cancel uses termination (no cooperative mid-render progress for tiny renders); progress most visible on multi-second noise/harmonic workloads.
- Real-time performance not promised; see measured benchmarks for guidance.
```

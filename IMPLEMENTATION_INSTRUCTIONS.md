# Implementation task: SPL audio compiler and browser application

Implement the system described below. Deliver working code, tests, examples, build scripts, and usage documentation. Do not stop at a design or scaffolding.

## Source of truth and scope

Read `SPL_Clean_DSL_Specification.md` completely before implementation. It is the normative SPL version 2 language and audio synthesis specification. Preserve it unchanged. When this document summarizes rendering behavior, the specification takes precedence.

Build:

1. A reusable Go library that parses and validates SPL, renders mono audio, computes a spectrogram of that audio, and exports audio and spectrogram artifacts.
2. A Go command-line utility exposing those capabilities.
3. A Node.js-based web project using React, TypeScript, and Vite, which runs the same Go engine compiled to WebAssembly in the browser.

Node.js supplies development and production build tooling. Production is a static application: parsing, validation, synthesis, spectrogram computation, and export run locally in the browser. No synthesis server, file uploads, external audio services, or separate JavaScript renderer are required.

Inspect applicable repository instructions and existing files before editing. Use supported Go and Node.js versions available at implementation time, verify toolchain-specific APIs against official documentation, and record the tested versions. Keep dependencies small and record their licenses. Resolve routine choices autonomously. Document genuine specification ambiguities and any chosen interpretation; do not silently change SPL semantics.

## Architecture

Separate platform-independent Go code from CLI and browser bindings. A suggested layout is:

```text
cmd/spl/                 Native CLI
cmd/splwasm/             js/wasm entry point and bridge
internal/ or pkg/
  spl/                  Lexer/parser, typed AST, validation, diagnostics
  synth/                Deterministic binary64 rendering
  spectrogram/          STFT, dB conversion, color mapping, PNG
  audio/                WAV encoding
examples/               Valid SPL source files
testdata/               Conformance inputs and expected results
web/                    React/TypeScript/Vite application and worker
scripts/                Reproducible build support
README.md
```

Expose testable APIs equivalent to `Parse`, `Validate`, `Render`, `ComputeSpectrogram`, `EncodeFloatWAV`, and `EncodeSpectrogramPNG`. Exact names and package structure are implementation choices. Keep DOM, filesystem, and `syscall/js` dependencies out of the core. Return structured errors rather than panicking on user input.

The compile pipeline is source → parse → validate whole document → resource preflight → render binary64 PCM → optional WAV and spectrogram. No audio may be returned for invalid input or a failed render. Preserve document and event order in the AST and mixer.

## Parser and semantic validation

Implement every construct in the exact syntax: header, `track`, `harmonics` with mandatory `spectrum` then `curve`, `noise`, and `hit`. Do not add expressions, instruments, units, optional fields, nesting, or syntax extensions.

Enforce the exact integer and general-number lexical patterns before numeric conversion. Reject nonfinite binary64 values, unknown commands, extra fields, malformed or incomplete blocks, and Markdown fences. Support comments, blank lines, indentation, tabs, LF, CRLF, and an omitted final newline. Keep original physical line numbers through comment and blank-line processing.

Implement every range, ordering, positivity, row-count, section, and duration constraint in the specification. Hit start times are nondecreasing, whereas trajectory times are strictly increasing. Spectrum weights must include at least one positive value. All upper frequency bounds are strictly below Nyquist.

Keep the original duration token and calculate `ceil(RATE * DURATION)` using exact decimal arithmetic, including exponent notation; do not derive the frame count from a rounded float. Check conversion and allocation bounds before creating buffers. Bound token lengths and exponent sizes before constructing arbitrarily large exact numbers. Other DSP arithmetic follows the specification's binary64 reference.

Diagnostics must contain at least a stable code, human-readable message, and source line; include column, field, and block-opening line where useful. Report concrete constraints and offending values. Collect multiple independent validation errors when practical without inventing misleading recovery errors. Expose identical diagnostics in CLI and browser.

## Audio synthesis requirements

Implement the specification's formulas directly first, with small independent reference calculations for verification. Use float64 throughout the rendering core. Do not normalize, clip, limit, fade, oversample, or filter the generated mix. Warn when final absolute peak exceeds 1. Detect numerical overflow and nonfinite intermediate or final results and return an error.

- Evaluate output samples at `n / RATE` for exactly the specified frame count. Respect half-open active intervals, exact knot values, silence outside blocks, and deliberately nonzero boundary gains.
- Tracks integrate piecewise-linear frequency analytically from the first trajectory time. Carry phase through all segments, including zero-gain sections. Initial phase is cosine phase zero.
- Harmonics use the same integrated pitch phase, linearly interpolated absolute-frequency spectrum, per-sample weight normalization, increasing harmonic order, and zero output when the weight sum is zero. Shared pitch trajectories and start times must retain phase agreement across blocks. Avoid evaluating harmonics outside the contributing spectrum, without changing results.
- Noise uses exactly `L = 2048`, `H = 256`, the periodic Hann window, fractional bin-cell coverage, and the specified spectral slope. DC and Nyquist are zero. Include negative frame indices and every frame whose window overlaps active output samples. Clamp frame-center parameter evaluation to the trajectory endpoints. Apply interpolated gain per output sample and divide overlap accumulation by the square root of the sum of squared overlapping windows.
- Implement the noise hash with wrapping unsigned 32-bit arithmetic, logical shifts, and negative frame indices reduced modulo 2^32. Number only noise blocks, starting at zero. Do not replace it with a standard pseudorandom generator.
- Hits use the specified centered cosine sum, sum-of-weights normalization, and squared-sine event envelope evaluated at actual sample times. Do not quantize event start, length, or center to synthesis frames. Simultaneous hits add in row order. Zero-weight regions produce silence.
- Sum blocks in document order and hit events in row order. Do not parallelize accumulation in a way that changes summation order. Repeat rendering on a given implementation/version must be deterministic; native/browser comparisons use a documented numerical tolerance rather than claiming cross-platform bit identity.

An inverse FFT is permitted for noise synthesis, as the specification explicitly allows it. Derive conjugate symmetry, phase sign, and transform scaling against a direct cosine-sum oracle before using it. Preserve the required RMS and overlap normalization. Do not substitute generic filtered noise or use an FFT to quantize off-grid hits.

Publish explicit limits for input bytes, token size, rows, blocks, sample count, render memory, harmonic work, noise frames, hit-bin evaluations, and spectrogram cells. Estimate work before rendering, including tiny positive pitches that imply huge harmonic counts. Browser limits may be stricter than native limits. Reject excessive work with an actionable `RESOURCE_LIMIT` diagnostic; never silently discard content or lower resolution. Support cancellation and progress at practical rendering boundaries.

## Audio output and CLI

Provide these commands, or document an equally clear equivalent:

```sh
spl validate examples/metallic-impact.spl
spl render examples/metallic-impact.spl --wav output.wav
spl render examples/metallic-impact.spl --wav output.wav --spectrogram output.png
spl render examples/metallic-impact.spl --spectrogram output.png
```

Use mono IEEE float32 WAV as the default audio export, preserving sample rate, sample count, and values above full scale. The conversion from core float64 samples to WAV float32 must be explicit and checked for representability. Implement a standards-compliant WAV header and required format metadata/chunks; test decoding with an independent reader. Reject files that exceed the supported WAV size rather than writing an invalid header.

Integer WAV is optional. If implemented, require an explicit clipping/conversion policy. Do not silently turn the default into integer PCM.

Send diagnostics, warnings, and concise render statistics to stderr. Return zero on success and nonzero on validation, resource, rendering, or export errors. Provide `--help`, version information, and an optional structured diagnostics mode. Avoid leaving apparently successful partial artifacts on failure; use temporary output files and finalize completed exports. Require explicit overwrite permission for existing output files.

Report duration, sample rate, sample count, peak, and over-full-scale sample count. Spectrogram-only output still renders the PCM first.

## Spectrogram contract

The language specification does not define a spectrogram. Treat the following as application analysis settings, independent of SPL synthesis semantics:

- Compute the spectrogram from the final mixed float64 PCM, before WAV quantization or playback gain. Do not draw the declared DSL regions as a substitute for analyzing generated audio.
- Default to a 2048-sample periodic Hann analysis window, 256-sample hop, and a one-sided real STFT, including DC and Nyquist. Allow configurable power-of-two FFT lengths and positive hops within documented limits.
- Center frames at sample positions `0, hop, 2*hop, ...` strictly below the PCM length. Zero-pad samples outside the audio. This gives at least one frame for every valid nonempty render. Document this framing convention and its boundary effects.
- Define one-sided amplitude as `abs(FFT) / sum(window)` for DC/Nyquist and twice that value for interior bins. Display `20*log10(amplitude)` relative to amplitude 1, with a finite floor. An interior, bin-centered cosine of amplitude 1 has approximately 0 dB in its center bin away from signal boundaries. Label values as dBFS amplitude; this is not a power spectral density.
- Default display range is -100 to 0 dBFS. Permit changing the display range, including its upper bound for over-full-scale signals. Saturating the color scale must never alter PCM or underlying analysis values. Silence must produce finite floor values without NaN or infinity.
- Default frequency axis is linear from 0 to Nyquist. Include time in seconds, frequency in Hz/kHz, and a labeled color legend in exported PNGs and the browser view. An optional logarithmic view must handle DC explicitly.
- Keep time-major matrix layout, dimensions, bin frequencies, frame-center times, FFT length, hop, window definition, and dB reference explicit in the analysis result. Use the same Go analysis and color mapping for CLI and browser exports.

Produce a readable PNG with axes and legend. Use a bounded display raster for long signals and document the matrix-to-pixel aggregation method; do not silently reduce the requested STFT resolution. Distinguish display resizing from analysis settings. Preserve metadata needed to map browser cursor positions to time, frequency, and analyzed dB values.

## Browser implementation and WebAssembly integration

Compile the shared Go code with `GOOS=js GOARCH=wasm`. Include the matching Go runtime support JavaScript from the exact toolchain used to build the module; resolve its location from that toolchain rather than relying on a hardcoded legacy path. Automate copying/versioning these assets in the web build. Do not require users to manually assemble runtime files.

Run the Go runtime and all expensive Go work in a dedicated Web Worker. The UI thread must remain responsive during compilation, rendering, spectrogram generation, and export. Design a small typed request/response protocol with request IDs, initialization state, validation/render requests, progress, warnings, result metadata, and structured errors. Never serialize PCM or large matrices as JSON arrays; transfer binary ArrayBuffers through a documented bridge with clearly owned copies.

Support Cancel. Account for synchronous Go work preventing worker messages from being handled: either yield cooperatively or terminate and recreate the worker. A terminated worker must initialize cleanly for the next request. Discard stale responses by request ID and release buffers and object URLs when results are replaced. Handle failed WASM initialization, memory/resource limits, worker failures, and export errors visibly.

The application must provide:

- An editable SPL text area/editor with line numbers, a clearly labeled Render action, and source-linked diagnostics.
- Selectable examples corresponding to the specification's four complete examples, plus a minimal track example. Use the exact example source where provided.
- Loading/rendering/progress/cancel states and a clear indicator when displayed results belong to an earlier editor revision.
- Play, pause/stop, seek, and replay using browser audio facilities; create or resume the audio context following a user gesture. Preserve the source sample rate in data and exports, and explain that hardware playback may resample.
- A separate playback volume control with a conservative initial value. It affects playback only. Display clipping/headroom warnings and do not normalize the generated buffer or downloaded WAV.
- A spectrogram with readable axes, legend, and cursor readout. Expose analysis settings and dB display range; changing only colors/range should not rerender audio.
- Downloads for float WAV, the spectrogram PNG, and the current SPL source. Native and browser WAV/PNG exports must use the shared Go exporters.
- Sample rate, duration, sample count, peak, warnings, and elapsed render time.
- Responsive layout, keyboard-accessible controls, visible focus, and accessible status/error text.

For Web Audio, create a mono AudioBuffer using the generated sample rate and an explicit float64-to-float32 conversion. Some browsers may reject rates permitted by SPL; show a playback-specific error while retaining valid render/export results, or use a documented playback-only resampling path. Do not alter the exported sample rate to work around playback restrictions.

Use `WebAssembly.instantiateStreaming` when supported with a correct MIME type, and an ArrayBuffer fallback when necessary. Test production assets under the documented deployment base path. Avoid a shared-memory/threading requirement unless justified and documented. Never execute editor input as JavaScript or inject it as HTML.

## Verification and acceptance criteria

Use meaningful automated tests, including independently calculated fixtures rather than snapshots generated solely by the renderer under test.

1. Parser/validation: every construct; comments and line numbering; CRLF and missing final newline; malformed number tokens; missing/extra fields; unknown sections; invalid rates/seeds; negative values; strict Nyquist bounds; time ordering; nonzero spectrum requirement; simultaneous hits; events exceeding duration; header-only silence; oversized input; exact decimal/exponent sample-count boundary cases.
2. Tracks/harmonics: constant cosine samples, analytical chirp integration, phase across multiple knots and silent sections, half-open boundaries, spectral interpolation, harmonic normalization, zero-weight output, and phase agreement between matching harmonic trajectories.
3. Noise: published hash fixtures including negative m, direct-sum versus IFFT frames, frame RMS, fractional cell coverage, moving spectra, denominator and negative-frame boundary coverage, short gain envelopes, silent regions, seed repeatability, and invariance when non-noise blocks are inserted. Use statistical tolerances for finite-segment RMS; do not assert exact requested RMS.
4. Hits/mixing: off-grid event timing, sampled versus continuous center peak, simultaneous events, silent regions, overlap ordering, over-full-scale output preserved in float WAV, and no partial result after an error.
5. Spectrogram: silence is finite; a bin-centered tone has the expected peak frequency and amplitude; chirp frequency rises with time; two-tone separation; short audio padding; frame counts; explicit matrix orientation; and correct PNG dimensions/labels. Visually inspect representative exported PNGs.
6. Integration: all complete specification examples validate and render through both CLI and WASM. Compare native and browser PCM and spectrogram values with stated absolute/relative tolerances and an explanation for them. Verify repeated same-build renders are deterministic.
7. Browser: use a real browser automation test for initialization, editing, invalid-input diagnostics, successful render, playback initiation, downloads, cancel followed by rerender, stale-result protection, and an actionable resource-limit error. Verify WAV contents and PNG decoding, not just download filenames. Smoke-test a production build served as static files.
8. Robustness/performance: fuzz or property-test malformed parser input for panics and unbounded allocations. Benchmark representative tonal and noise examples and a workload near each chosen limit. Publish measured timings, environments, peak memory where available, and known limitations; do not promise unmeasured real-time performance.

Run the Go tests, native build, WASM build, TypeScript checks, web production build, and relevant browser tests. Fix failures before handoff. Report any unavailable test environment honestly and distinguish unverified behavior from passing checks.

## Delivery sequence and handoff

Implement in reviewable stages: parser/validation and fixtures; reference synthesis and CLI; verified synthesis optimizations; WAV and spectrogram exports; WASM worker bridge; browser UI; integration and documentation. Keep the system usable at each stage and avoid changing language semantics to solve performance problems.

The README must include prerequisites and tested versions, reproducible build/test commands, CLI examples, web development and static-serving instructions, WASM/runtime build details, the browser's local-processing architecture, synthesis conformance notes, spectrogram conventions, resource limits, playback/export distinctions, and known limitations. Include lockfiles and a single documented web build command that also builds or verifies fresh WASM assets.

Finish by summarizing the delivered behavior, key files, commands to run it, checks actually performed, and remaining limitations. The task is complete when a clean checkout can produce audio and a spectrogram from the supplied examples through both the native CLI and the browser, using the same Go implementation.

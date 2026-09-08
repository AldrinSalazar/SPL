# SPL: spectral shapes for audio

Version: 2, revised proposal. This replaces the earlier SPL 2 proposal.

SPL describes sound by drawing structures in time and frequency. An LLM writes curves, harmonic spectra, noise regions, and attacks. A deterministic renderer supplies phase and generates the waveform.

The intended workflow is one-shot generation. The model receives the authoring guide, writes SPL, and the application validates and renders it. Listening feedback is not required. An application may return validation errors for a repair attempt.

There are no sample values, FFT bins, external recordings, instrument names, or learned sound generators in the language.

## Authoring guide

This section is the instruction sheet supplied to the model. Renderer implementation details appear later.

### File and numbers

Start every file with:

```text
spl 2 RATE DURATION SEED
```

For example, `spl 2 24000 2 0` means 24000 samples per second, two seconds, random seed zero. All output is mono. Use 48000 for sounds needing frequencies above 12000 Hz.

Write ordinary numbers without units. Time is seconds, frequency is Hz, and gain is linear amplitude. Gain 0 is silence.

Use these four block types. Every block ends with `end`. Blocks never nest. Blank lines and `#` comments are allowed.

### Track: draw one frequency curve

```text
track
TIME FREQUENCY GAIN
TIME FREQUENCY GAIN
end
```

Example:

```text
track
0 500 0
0.02 500 0.3
0.4 1500 0.2
0.8 800 0
end
```

This draws a rising and falling sinusoidal frequency track. Add tracks for independent resonances, beating, or inharmonic sounds. Phase remains continuous as frequency changes.

### Harmonics: draw a pitch curve and its spectral shape

```text
harmonics
spectrum
FREQUENCY WEIGHT
FREQUENCY WEIGHT
curve
TIME PITCH GAIN
TIME PITCH GAIN
end
```

Example:

```text
harmonics
spectrum
0 1
1000 0.5
5000 0
curve
0 220 0
0.01 220 0.4
0.6 330 0.2
0.8 330 0
end
```

The renderer places harmonics at pitch, twice pitch, three times pitch, and so on. The spectrum gives their relative amplitudes at absolute frequencies, with straight lines between the spectrum points. Frequencies outside the spectrum receive no energy.

Use spectrum peaks to describe resonances or formants. As pitch moves, those peaks stay at their specified frequencies. Row gain bounds the sum of harmonic amplitudes.

For changing spectral shape, overlap harmonic blocks with the same pitch curve, different spectra, and different gain curves. Their phases agree when their pitch curves and starting times agree. This lets one set of resonances fade into another.

### Noise: draw a moving frequency region

```text
noise
TIME LOW HIGH GAIN SLOPE
TIME LOW HIGH GAIN SLOPE
end
```

Example:

```text
noise
0 1000 8000 0 0
0.03 1000 8000 0.08 0
0.5 3000 10000 0.05 0.5
0.8 4000 10000 0 0.5
end
```

LOW and HIGH are the region boundaries. SLOPE 0 gives flat spectral power, 0.5 gives pink noise, and 1 gives brown noise within those boundaries. Gain specifies the underlying noise's RMS scale, before the time envelope. Peaks can exceed this gain.

Use overlapping regions for more detailed spectra. Each noise block gets a separate deterministic noise pattern from the file seed.

### Hit: draw short broadband attacks

```text
hit
TIME LENGTH LOW HIGH GAIN SLOPE
TIME LENGTH LOW HIGH GAIN SLOPE
end
```

Each row is a separate event, not an interpolation point. TIME is the event's start and LENGTH is its duration. LOW, HIGH, and SLOPE describe its spectrum, as for noise. GAIN bounds its peak amplitude. The renderer aligns spectral phases at the center of the event.

Example:

```text
hit
0.1 0.004 100 10000 0.3 0
0.6 0.004 100 10000 0.2 0
end
```

Use hits for clicks and attacks. Add tracks or noise for the ringing or noisy decay afterward.

### Rules for all sounds

- All times are absolute seconds from the start of the file.
- Track, harmonic curve, and noise blocks need at least two rows with strictly increasing times.
- Their values change linearly between rows. They are silent outside their first and last time.
- Start and finish gain curves at 0 when smooth boundaries are wanted. Nonzero boundary gains deliberately create abrupt changes.
- Keep frequencies below half the sample rate. Track frequency and pitch must be positive. Spectra and regions may start at 0 Hz.
- Keep all times and complete hit events within DURATION.
- Gains, spectrum weights, and slopes must be nonnegative.
- Sounds overlap and add. Leave headroom: several simultaneous gains of 0.5 can exceed full scale.
- Return SPL text only.

These are the complete authoring constructs. There are no optional fields, named objects, expressions, loops, units, selectable interpolation modes, or phase settings.

## What this representation covers

Tracks describe coherent tonal energy, including arbitrary inharmonic trajectories. Harmonic groups compress repeated tracks and specify resonances independently of pitch. Noise regions describe distributed stochastic energy. Hits describe brief events with coherent broadband phase.

Speech-like structure can combine harmonic groups with formant-shaped spectra, noise regions for unvoiced energy, and hits for closures or releases. Musical and environmental structures use the same commands. There is no built-in speech or instrument model.

More tracks, spectrum points, and regions provide more detail without changing the syntax. This is a broad audio synthesis representation, not a guarantee of exact reconstruction of every waveform. A spectrogram-like description leaves phase choices to the renderer, and one-shot perceptual quality still depends on the model's acoustic choices. The language deliberately has no waveform-data escape hatch.

## Exact syntax

Keywords are lowercase and case-sensitive. Spaces and tabs separate fields; indentation has no meaning. LF and CRLF are accepted, and the final newline is optional. Strip text beginning with `#` through the end of its line, then ignore empty lines.

An integer matches `[0-9]+`. A general number matches `-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?`. Numbers must parse to finite binary64 values. Infinity, NaN, unit suffixes, commas, and Markdown fences are invalid.

Each line has exactly its declared fields. The grammar below uses NEWLINE for the end of a nonempty logical line and NUMBER for a general number:

```text
document = header block*
header = "spl" "2" INTEGER NUMBER INTEGER NEWLINE
block = track | harmonics | noise | hit
track = "track" NEWLINE trajectory "end" NEWLINE
harmonics = "harmonics" NEWLINE
            "spectrum" NEWLINE spectrum_row spectrum_row+
            "curve" NEWLINE trajectory "end" NEWLINE
noise = "noise" NEWLINE noise_row noise_row+ "end" NEWLINE
hit = "hit" NEWLINE hit_row+ "end" NEWLINE
trajectory = trajectory_row trajectory_row+
trajectory_row = NUMBER NUMBER NUMBER NEWLINE
spectrum_row = NUMBER NUMBER NEWLINE
noise_row = NUMBER NUMBER NUMBER NUMBER NUMBER NEWLINE
hit_row = NUMBER NUMBER NUMBER NUMBER NUMBER NUMBER NEWLINE
```

The optional physical final newline is treated as a logical NEWLINE. Required section labels in harmonics select its two fixed row formats; they do not introduce nested blocks.

## Header, bounds, and mixing

RATE is an integer from 8000 through 192000. DURATION is positive. SEED is an integer from 0 through 4294967295.

The file has `N = ceil(RATE * DURATION)` sample frames, calculated using the exact decimal duration to avoid rounding across an integer boundary. Output frame n is evaluated at `t = n / RATE` for `0 <= n < N`.

Output is mono. Each block contributes directly to the single output channel.

Sum blocks in document order, and events within a hit block in row order. Use floating-point output without normalization, limiting, or clipping. Warn when final magnitude exceeds 1. Float WAV is the recommended export. Integer conversion is a separate export operation with an explicitly selected clipping policy.

All bounds are inclusive unless explicitly strict. Numerical overflow, nonfinite results, and documented resource-limit violations are errors. Never silently reduce resolution, truncate audio, or discard blocks.

## Trajectories and spectra

Trajectory times lie in [0, DURATION] and strictly increase. Evaluate each segment by linear interpolation in time, independently for each column. At an interior knot, use the row's exact values. Each block is active on `[first_time, last_time)` and zero elsewhere. There are no automatic fades.

Track frequency and harmonic pitch lie strictly in `(0, RATE/2)`. Gains are nonnegative.

A harmonic spectrum has at least two rows. Frequencies strictly increase and lie in `[0, RATE/2)`. Weights are nonnegative and at least one is positive. Interpolate weight linearly in Hz, not in log frequency. Weight is zero outside the declared frequency range. Endpoint weights may be nonzero, intentionally producing a hard spectral boundary.

Noise and hit bounds satisfy `0 <= LOW < HIGH < RATE/2`. SLOPE is nonnegative. Noise interpolates LOW, HIGH, GAIN, and SLOPE in time. These endpoint constraints also hold between rows because the interpolation is linear.

Hit rows have nondecreasing start times; simultaneous hits are valid. LENGTH is positive and `TIME + LENGTH <= DURATION`. Hit rows do not connect to one another.

## Track rendering

Let f(t) and g(t) be the interpolated frequency and gain. With t0 the first row's time:

```text
cycles(t) = integral from t0 to t of f(u) du
signal(t) = g(t) * cos(2*pi*cycles(t))
```

The initial phase is always cosine phase zero at t0. Integrate each linear frequency segment analytically and carry phase through every row, including zero-gain sections. A segment with starting frequency f0, slope m, and elapsed time d contributes `f0*d + m*d*d/2` cycles.

## Harmonic rendering

Compute cycles(t) from the pitch trajectory exactly as for a track. Let S(f) be the interpolated spectrum. For each positive integer k with `k*pitch(t) < RATE/2`, set:

```text
w(k,t) = S(k*pitch(t))
Z(t) = sum of w(k,t)
signal(t) = gain(t) * sum of w(k,t)*cos(2*pi*k*cycles(t)) / Z(t)
```

When Z is zero, the output is zero. Only harmonics within the declared spectrum can contribute. Sum in increasing k order.

This normalizes the sum of harmonic amplitudes to the row gain. It does not normalize the final mix. Harmonics enter or leave according to S; zero weights at spectral endpoints avoid abrupt entries. The absolute block signal is bounded by its gain apart from numerical rounding.

Tracks and harmonics define sampled oscillators directly. There is no hidden antialiasing filter or oversampling. Fast modulation and abrupt boundaries can create aliasing. A future change to that behavior requires a language version change.

## Noise and hit rendering

The following fixed synthesis method defines spectral resolution, phase, timing, and amplitude. The model does not emit any of these internal coefficients.

Use transform length `L = 2048`, hop `H = 256`, and periodic Hann window:

```text
window(j) = 0.5 - 0.5*cos(2*pi*j/L), 0 <= j < L
```

Interior bins are `k = 1 ... L/2-1` at frequencies `f(k) = k*RATE/L`. DC and Nyquist are always zero. The spacing is RATE/2048 Hz. Track synthesis remains independent of this resolution.

To describe a region with bounds a and b, treat each bin as the frequency cell `[f(k)-RATE/(2*L), f(k)+RATE/(2*L)]`. Let q(k) be the length of that cell's intersection with [a,b], divided by RATE/L. Define:

```text
w(k) = sqrt(q(k)) * (f(k)/(RATE/L))^(-SLOPE)
```

Fractional cell coverage lets moving boundaries enter and leave bins continuously. If every w is zero, the region contributes silence. This can occur for a region entirely below the lowest interior cell. Narrow coherent low frequencies should use tracks.

### Noise

Number noise blocks starting at zero in their document order. For a block active between t0 and t1, use every integer frame index m whose window overlaps its active output samples. Frame m starts at sample `s = m*H`; negative frame indices are included for boundary coverage.

At frame center time `(s + L/2)/RATE`, evaluate the block's LOW, HIGH, and SLOPE, clamping that time to [t0,t1] before interpolation. Construct w(k) as above and set `Z = sqrt(sum of w(k)^2 / 2)`.

For each local window sample j, the unwindowed frame is:

```text
u(m,j) = sum over k of w(k)/Z
         * cos(2*pi*(k*j/L + phase(block_index,m,k)))
```

An all-zero spectrum gives an all-zero frame. Sum bins in increasing k order. Each nonzero frame has unit RMS before windowing. At output sample n, combine frames as:

```text
v(n) = sum over overlapping m of window(n-m*H)*u(m,n-m*H)
       / sqrt(sum over overlapping m of window(n-m*H)^2)
signal(n) = interpolated_gain(n/RATE) * v(n)
```

Evaluate only within the block's active interval; output zero outside. Frame phases differ deterministically, so noise evolves over time rather than repeating one frozen spectrum. The denominator preserves the expected power scale across overlapping random frames. Finite segments need not have exactly the requested RMS. Gain is evaluated at each output sample, so short envelopes are not restricted to hop boundaries.

The spectrum evolves at frame resolution. Abrupt or very narrow noise structures are limited by the specified window. Hits provide a separate rendering path for short coherent attacks.

### Noise phase hash

All integer operations below are unsigned 32-bit. Multiplication and addition wrap modulo 2^32. Convert negative m to its residue modulo 2^32. Right shift is logical.

```text
mix(x):
  x = x + 0x9e3779b9
  x = (x xor (x >> 16)) * 0x85ebca6b
  x = (x xor (x >> 13)) * 0xc2b2ae35
  return x xor (x >> 16)

x = mix(SEED xor mix(block_index))
x = mix(x xor mix(m))
x = mix(x xor mix(k))
phase(block_index,m,k) = x / 4294967296
```

Adding tracks, harmonic blocks, or hits does not change noise. Inserting a noise block changes the identities of later noise blocks.

### Hits

For each hit, calculate w(k) from its fixed bounds and slope. Let `Z = sum of w(k)`. If Z is zero, the event is silent.

For `TIME <= t < TIME + LENGTH`, define:

```text
center = TIME + LENGTH/2
u = (t-TIME)/LENGTH
envelope = sin(pi*u)^2
signal(t) = GAIN * envelope
            * sum over k of w(k)*cos(2*pi*f(k)*(t-center)) / Z
```

Output zero outside the event. Sum bins in increasing k order. Phases align at the event center. The continuous-time center reaches GAIN; sampled peaks may be lower. The event is bounded in magnitude by GAIN.

The duration window broadens the resulting spectrum beyond LOW and HIGH. This is intentional: a finite event cannot also have perfectly bounded frequency support. The bounds describe the carrier spectrum before the duration envelope.

## Determinism and conformance

The formulas specify one interpretation, with no selectable renderer profiles. Binary64 is the reference arithmetic. Implementations may use equivalent inverse FFT operations for frame synthesis; transform scaling must preserve the formulas above.

Repeated rendering with the same implementation and version must be deterministic. Different FFT and mathematical libraries can differ in their final floating-point bits; cross-platform bit-identical synthesis is not promised.

A reference renderer should publish numerical fixtures for phase integration, harmonic weighting, moving noise, seeded frames, boundary coverage, hit timing, and mixing. This specification does not claim those implementation checks have been completed.

## Complete examples

### Three notes as harmonic curves

```text
spl 2 24000 1.5 0

harmonics
spectrum
0 1
1000 0.5
5000 0
curve
0 330 0
0.01 330 0.4
0.4 330 0.3
0.45 330 0
0.46 294 0
0.47 294 0.4
0.9 294 0.3
0.95 294 0
0.96 262 0
0.97 262 0.4
1.4 262 0.3
1.5 262 0
end
```

### A pitched sound with two spectral resonances

```text
spl 2 24000 1 0

harmonics
spectrum
0 0
200 0.1
600 1
1000 0.1
1400 0.1
1900 0.7
2400 0.05
4000 0
curve
0 120 0
0.03 120 0.3
0.4 126 0.3
0.8 118 0.2
1 118 0
end

noise
0 2000 7000 0 0
0.05 2000 7000 0.015 0
0.8 2500 8000 0.01 0
1 2500 8000 0 0
end
```

### Metallic impact

```text
spl 2 24000 2 42

hit
0.1 0.004 100 10000 0.25 0
end

track
0.1 440 0
0.102 440 0.25
0.3 440 0.1
1 440 0.02
2 440 0
end

track
0.1 703 0
0.102 703 0.15
0.2 703 0.06
0.9 703 0
end

track
0.1 1188 0
0.102 1188 0.1
0.18 1188 0.02
0.5 1188 0
end

noise
0.1 1000 10000 0 0.5
0.104 1000 10000 0.03 0.5
0.2 2000 8000 0.01 0.5
0.5 3000 6000 0 0.5
end
```

### Two moving noise regions

```text
spl 2 24000 3 8

noise
0 200 4000 0 0.5
0.3 200 4000 0.06 0.5
1.5 1000 9000 0.08 0
3 3000 11000 0 0
end

noise
0 3000 11000 0 0
0.3 3000 11000 0.06 0
1.5 1000 9000 0.08 0.5
3 200 4000 0 0.5
end
```

## Validation and experimental use

Validate the whole document before rendering. Do not guess missing fields, ignore unknown commands, reorder rows, or emit partial audio after an error.

Return line numbers and specific constraints, for example:

```text
line 8: FIELD_COUNT: noise rows require TIME LOW HIGH GAIN SLOPE; found 4 numbers
line 12: TIME_ORDER: time 0.2 must be greater than previous time 0.3
line 15: FREQUENCY_RANGE: high frequency 14000 must be less than 12000
line 21: SECTION_REQUIRED: harmonics requires spectrum before curve
line 30: UNCLOSED_BLOCK: expected end for track opened on line 25
```

The application may return these errors to the model. Report repaired validity separately from first-pass validity.

For the one-shot experiment, measure first-pass validity, output tokens, clipping, and perceptual correspondence to the requested sound. Independent listening evaluation measures the experiment's results; it is not feedback to the generating model.

SPL keeps synthesis mathematics in the renderer and asks the model only for acoustic shapes. Further compression constructs should be added only when experiments show that they improve one-shot results enough to justify additional syntax.

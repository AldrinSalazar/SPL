# SPL 2 Language Specification

## Status

This document specifies version 2 of the SPL domain-specific language.

SPL is a declarative language for describing audio as structures over time and frequency. An SPL document contains a header followed by zero or more `track`, `harmonics`, `noise`, and `hit` blocks.

This specification defines only the language: its lexical structure, grammar, fields, constraints, interpolation rules, and composition semantics. Rendering algorithms and implementation techniques are outside its scope.

## 1. Conformance

The key words **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** indicate normative requirements.

A conforming SPL 2 document MUST satisfy all syntax and validation requirements in this specification.

An implementation MUST reject a document that violates a normative requirement. It MUST NOT infer omitted fields, ignore unknown constructs, reorder rows to make an invalid document valid, or partially accept an otherwise invalid document.

## 2. Units and Numeric Meaning

SPL numbers do not carry unit suffixes.

The following units are implicit:

| Quantity | Unit |
| --- | --- |
| Time | seconds |
| Frequency | hertz |
| Gain | linear amplitude |
| Spectrum weight | unitless |
| Noise slope | unitless |

A gain of `0` denotes silence.

## 3. Lexical Structure

Keywords are lowercase and case-sensitive.

Spaces and tabs separate fields. Indentation has no semantic meaning.

Both LF and CRLF line endings are valid. A final physical newline is optional.

A comment begins with `#` and continues through the end of the physical line. Comments are removed before parsing. Empty lines are ignored.

An integer matches:

```text
[0-9]+
```

A general number matches:

```text
-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?
```

Every numeric token MUST parse to a finite binary64 value.

The following are invalid:

- `NaN`
- infinity
- unit suffixes
- thousands separators or commas
- Markdown fences
- numeric forms not matched by the grammar above

Each nonempty logical line MUST contain exactly the fields declared for that line.

## 4. Document Structure

An SPL 2 document consists of exactly one header followed by zero or more blocks.

The header MUST be the first nonempty logical line.

Blocks MUST NOT nest.

Every block is terminated by `end`.

The only block types are:

- `track`
- `harmonics`
- `noise`
- `hit`

No other commands, optional fields, expressions, variables, named objects, loops, units, interpolation modes, or phase controls are defined by SPL 2.

## 5. Grammar

The grammar below uses `NEWLINE` for the end of a nonempty logical line, `INTEGER` for an integer token, and `NUMBER` for a general number token.

```text
document = header block*

header = "spl" "2" INTEGER NUMBER INTEGER NEWLINE

block = track | harmonics | noise | hit

track =
    "track" NEWLINE
    trajectory
    "end" NEWLINE

harmonics =
    "harmonics" NEWLINE
    "spectrum" NEWLINE
    spectrum_row spectrum_row+
    "curve" NEWLINE
    trajectory
    "end" NEWLINE

noise =
    "noise" NEWLINE
    noise_row noise_row+
    "end" NEWLINE

hit =
    "hit" NEWLINE
    hit_row+
    "end" NEWLINE

trajectory =
    trajectory_row trajectory_row+

trajectory_row =
    NUMBER NUMBER NUMBER NEWLINE

spectrum_row =
    NUMBER NUMBER NEWLINE

noise_row =
    NUMBER NUMBER NUMBER NUMBER NUMBER NEWLINE

hit_row =
    NUMBER NUMBER NUMBER NUMBER NUMBER NUMBER NEWLINE
```

If the physical file does not end with a newline, the end of file is treated as the `NEWLINE` terminating the final nonempty logical line.

The `spectrum` and `curve` keywords inside `harmonics` are fixed section labels. They do not create nested blocks.

## 6. Header

The header has the form:

```text
spl 2 RATE DURATION SEED
```

### 6.1 `RATE`

`RATE` is the document sample rate.

It MUST be an integer in the inclusive range:

```text
8000 <= RATE <= 192000
```

The document Nyquist frequency is:

```text
RATE / 2
```

Frequency constraints elsewhere in the language are defined relative to this value.

### 6.2 `DURATION`

`DURATION` is the total document duration in seconds.

It MUST be greater than `0`.

All time values and complete `hit` events MUST lie within this duration.

### 6.3 `SEED`

`SEED` is the deterministic seed associated with stochastic constructs.

It MUST be an integer in the inclusive range:

```text
0 <= SEED <= 4294967295
```

The mapping from `SEED` to generated stochastic samples is a renderer concern and is not defined by this language specification.

## 7. Common Time-Series Semantics

The `track` trajectory, `harmonics` curve, and `noise` block are time series.

Each such time series:

1. MUST contain at least two rows.
2. MUST use times in the inclusive range `[0, DURATION]`.
3. MUST use strictly increasing row times.
4. Uses linear interpolation independently for each interpolated field between adjacent rows.
5. Takes the exact row values at a row time.
6. Is active from its first row time, inclusive, to its last row time, exclusive.
7. Contributes zero outside that active interval.

In interval notation, a time series with first time `t0` and last time `t1` is active on:

```text
[t0, t1)
```

SPL defines no automatic fade-in or fade-out. A zero gain at a boundary expresses silence at that boundary; a nonzero boundary gain expresses an abrupt boundary.

## 8. `track`

A `track` block describes one frequency trajectory with a gain trajectory.

Syntax:

```text
track
TIME FREQUENCY GAIN
TIME FREQUENCY GAIN
...
end
```

Each row contains:

| Field | Meaning |
| --- | --- |
| `TIME` | absolute time |
| `FREQUENCY` | instantaneous frequency |
| `GAIN` | instantaneous gain |

Constraints:

```text
0 <= TIME <= DURATION
0 < FREQUENCY < RATE / 2
0 <= GAIN
```

Rows MUST satisfy the common time-series rules in Section 7.

`FREQUENCY` and `GAIN` are linearly interpolated between rows.

A `track` represents one coherent tonal component whose frequency may vary over time.

## 9. `harmonics`

A `harmonics` block describes a pitch trajectory, a gain trajectory, and an absolute-frequency spectral weighting function.

Syntax:

```text
harmonics
spectrum
FREQUENCY WEIGHT
FREQUENCY WEIGHT
...
curve
TIME PITCH GAIN
TIME PITCH GAIN
...
end
```

The `spectrum` section MUST appear before the `curve` section.

### 9.1 Spectrum

A spectrum MUST contain at least two rows.

Each row contains:

| Field | Meaning |
| --- | --- |
| `FREQUENCY` | absolute frequency |
| `WEIGHT` | relative spectral weight |

Spectrum constraints:

```text
0 <= FREQUENCY < RATE / 2
0 <= WEIGHT
```

Spectrum frequencies MUST strictly increase.

At least one spectrum weight MUST be greater than `0`.

Weights are linearly interpolated in hertz between adjacent spectrum rows.

The spectrum weight is `0` below the first declared spectrum frequency and above the last declared spectrum frequency.

Endpoint weights MAY be nonzero.

The spectrum is expressed in absolute frequency, not relative to pitch. Therefore a spectral feature remains at its declared frequency as pitch changes.

### 9.2 Curve

The `curve` section is a trajectory with rows of:

| Field | Meaning |
| --- | --- |
| `TIME` | absolute time |
| `PITCH` | fundamental pitch |
| `GAIN` | instantaneous gain |

Constraints:

```text
0 <= TIME <= DURATION
0 < PITCH < RATE / 2
0 <= GAIN
```

The curve MUST satisfy the common time-series rules in Section 7.

`PITCH` and `GAIN` are linearly interpolated between rows.

A `harmonics` block represents harmonic components at positive integer multiples of the current pitch. The declared spectrum determines their relative weighting according to each harmonic's absolute frequency.

Only harmonic frequencies below `RATE / 2` are within the language's valid frequency domain.

## 10. `noise`

A `noise` block describes a time-varying stochastic frequency region.

Syntax:

```text
noise
TIME LOW HIGH GAIN SLOPE
TIME LOW HIGH GAIN SLOPE
...
end
```

Each row contains:

| Field | Meaning |
| --- | --- |
| `TIME` | absolute time |
| `LOW` | lower frequency bound |
| `HIGH` | upper frequency bound |
| `GAIN` | stochastic amplitude scale |
| `SLOPE` | spectral falloff parameter |

Constraints at every row:

```text
0 <= TIME <= DURATION
0 <= LOW < HIGH < RATE / 2
0 <= GAIN
0 <= SLOPE
```

Rows MUST satisfy the common time-series rules in Section 7.

`LOW`, `HIGH`, `GAIN`, and `SLOPE` are linearly interpolated between rows.

The interpolated frequency bounds MUST retain:

```text
0 <= LOW < HIGH < RATE / 2
```

A `noise` block represents stochastic energy inside the interval from `LOW` to `HIGH`.

`SLOPE` controls spectral falloff within that region. The following values have conventional interpretations:

| `SLOPE` | Interpretation |
| ---: | --- |
| `0` | flat spectral power |
| `0.5` | pink-noise-like falloff |
| `1` | brown-noise-like falloff |

Other nonnegative values are valid.

## 11. `hit`

A `hit` block contains one or more independent short events.

Syntax:

```text
hit
TIME LENGTH LOW HIGH GAIN SLOPE
TIME LENGTH LOW HIGH GAIN SLOPE
...
end
```

Each row is a complete event. Rows are not interpolation points.

Each row contains:

| Field | Meaning |
| --- | --- |
| `TIME` | event start time |
| `LENGTH` | event duration |
| `LOW` | lower frequency bound |
| `HIGH` | upper frequency bound |
| `GAIN` | event gain |
| `SLOPE` | spectral falloff parameter |

Constraints:

```text
0 <= TIME
0 < LENGTH
TIME + LENGTH <= DURATION

0 <= LOW < HIGH < RATE / 2
0 <= GAIN
0 <= SLOPE
```

Hit row start times MUST be nondecreasing.

Multiple rows MAY have the same `TIME`; simultaneous hits are valid.

Each event is active on:

```text
[TIME, TIME + LENGTH)
```

A `hit` represents a finite-duration broadband event whose carrier spectrum is described by `LOW`, `HIGH`, and `SLOPE`.

Rows within a `hit` block have no interpolation or continuity relationship with one another.

## 12. Composition

All block times are absolute seconds from the start of the document.

Blocks MAY overlap in time.

Overlapping blocks are additive contributions to one document output.

Multiple blocks of the same or different types MAY coexist.

SPL 2 has no grouping, routing, bus, channel, or layer construct. The document describes a single mono composition.

## 13. Frequency Domain Rules

Unless a stricter rule is stated for a specific field:

- track frequency MUST be strictly greater than `0` and strictly less than `RATE / 2`;
- harmonic pitch MUST be strictly greater than `0` and strictly less than `RATE / 2`;
- spectrum frequencies MAY begin at `0` but MUST be strictly less than `RATE / 2`;
- noise and hit lower bounds MAY be `0`;
- noise and hit upper bounds MUST be strictly less than `RATE / 2`.

No SPL 2 frequency field may equal or exceed `RATE / 2`.

## 14. Gain, Weight, and Slope Rules

All of the following MUST be nonnegative:

- track gain;
- harmonic curve gain;
- harmonic spectrum weight;
- noise gain;
- noise slope;
- hit gain;
- hit slope.

SPL does not impose a global normalization rule on overlapping blocks.

A document MAY therefore describe a summed amplitude greater than `1`.

## 15. Validation

Validation applies to the complete document.

A conforming validator MUST reject at least the following classes of errors:

- invalid or missing header;
- unsupported SPL version;
- unknown keywords or block types;
- incorrect field counts;
- invalid numeric tokens;
- nonfinite numeric values;
- missing required `harmonics` sections;
- unclosed blocks;
- nested blocks;
- insufficient row counts;
- invalid time ordering;
- invalid spectrum frequency ordering;
- invalid hit ordering;
- values outside their declared ranges;
- hit events extending beyond `DURATION`.

A validator SHOULD identify the source line and violated constraint.

Validation failure MUST NOT alter the language meaning by silently repairing the document.

## 16. Complete Example

The following is a syntactically and semantically valid SPL 2 document:

```text
spl 2 24000 1 42

harmonics
spectrum
0 0
200 0.1
600 1
1000 0.1
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

hit
0.1 0.004 100 10000 0.2 0
end
```

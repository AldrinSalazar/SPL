package synth

import (
	"context"
	"math"

	"spl/pkg/spl"
)

// renderNoise implements specified overlap-add with IFFT frame synthesis.
func renderNoise(b *spl.NoiseBlock, doc *spl.Document, out []float64, ctx context.Context, add func(int64)) *spl.Diagnostic {
	rate := doc.Rate
	n := doc.NumSamples
	rows := b.Rows
	if len(rows) < 2 {
		return nil
	}
	t0 := rows[0].Time
	t1 := rows[len(rows)-1].Time
	ns, ne := spl.ActiveRange(t0, t1, rate, n)
	if ne <= ns {
		return nil
	}
	// Fast path: all gains zero -> silence.
	allZero := true
	for _, r := range rows {
		if r.Gain != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return nil
	}
	const L = SynthL
	const H = SynthH
	window := HannWindow(L)
	mMin := floorDiv(ns-(L-1), H)
	mMax := floorDiv(ne-1, H)
	width := ne - ns
	acc := make([]float64, width)
	// Per-frame synthesis and accumulation.
	X := make([]complex128, L)
	u := make([]float64, L)
	for m := mMin; m <= mMax; m++ {
		if ctx != nil && (m-mMin)%8 == 0 {
			if err := ctx.Err(); err != nil {
				return &spl.Diagnostic{Code: spl.CodeRenderError, Message: "render cancelled: " + err.Error()}
			}
		}
		tc := (float64(m*H) + float64(L)/2) / float64(rate)
		clamped := tc
		if clamped < t0 {
			clamped = t0
		}
		if clamped > t1 {
			clamped = t1
		}
		lo, hi, slope := interpNoiseParams(rows, clamped)
		w := RegionWeights(lo, hi, slope, rate)
		sum2 := 0.0
		for k := 1; k < L/2; k++ {
			sum2 += w[k] * w[k]
		}
		z := math.Sqrt(sum2 / 2)
		if z == 0 {
			// All-zero frame: numerator contributes nothing.
			continue
		}
		// Build conjugate-symmetric spectrum.
		for i := range X {
			X[i] = 0
		}
		scale := float64(L) / 2
		for k := 1; k < L/2; k++ {
			if w[k] == 0 {
				continue
			}
			ph := NoisePhase(doc.Seed, b.Index, m, k)
			ang := 2 * math.Pi * ph
			mag := scale * w[k] / z
			c := complex(mag*math.Cos(ang), mag*math.Sin(ang))
			X[k] = c
			X[L-k] = complex(real(c), -imag(c))
		}
		FFT(X, true)
		for j := 0; j < L; j++ {
			u[j] = real(X[j])
		}
		s := m * H
		// Accumulate window*u into acc.
		loJ := 0
		if ns-s > loJ {
			loJ = ns - s
		}
		hiJ := L
		if ne-s < hiJ {
			hiJ = ne - s
		}
		for j := loJ; j < hiJ; j++ {
			acc[s+j-ns] += window[j] * u[j]
		}
		if add != nil {
			add(int64(H))
		}
	}
	// Finalize per-sample: divide by window-energy denominator, apply gain.
	for s := ns; s < ne; s++ {
		if ctx != nil && (s-ns)%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return &spl.Diagnostic{Code: spl.CodeRenderError, Message: "render cancelled: " + err.Error()}
			}
		}
		t := float64(s) / float64(rate)
		g := interpNoiseGain(rows, t)
		if g == 0 {
			continue
		}
		den2 := 0.0
		// Covering frames: m in [floor(s/H)-7, floor(s/H)].
		q := floorDiv(s, H)
		for m := q - (L/H - 1); m <= q; m++ {
			j := s - m*H
			if j < 0 || j >= L {
				continue
			}
			den2 += window[j] * window[j]
		}
		if den2 <= 0 {
			continue
		}
		v := acc[s-ns] / math.Sqrt(den2)
		out[s] += g * v
	}
	return nil
}

func interpNoiseParams(rows []spl.NoiseRow, t float64) (lo, hi, slope float64) {
	i := locateNoiseSeg(rows, t)
	d := rows[i+1].Time - rows[i].Time
	frac := 0.0
	if d != 0 {
		frac = (t - rows[i].Time) / d
	}
	lo = rows[i].Low + (rows[i+1].Low-rows[i].Low)*frac
	hi = rows[i].High + (rows[i+1].High-rows[i].High)*frac
	slope = rows[i].Slope + (rows[i+1].Slope-rows[i].Slope)*frac
	return lo, hi, slope
}

func interpNoiseGain(rows []spl.NoiseRow, t float64) float64 {
	i := locateNoiseSeg(rows, t)
	d := rows[i+1].Time - rows[i].Time
	frac := 0.0
	if d != 0 {
		frac = (t - rows[i].Time) / d
	}
	return rows[i].Gain + (rows[i+1].Gain-rows[i].Gain)*frac
}

func locateNoiseSeg(rows []spl.NoiseRow, t float64) int {
	if t <= rows[0].Time {
		return 0
	}
	for i := 0; i+1 < len(rows); i++ {
		if t < rows[i+1].Time {
			return i
		}
		if t == rows[i+1].Time && i+1 < len(rows)-1 {
			return i + 1
		}
	}
	return len(rows) - 2
}

func floorDiv(a, b int) int {
	if b < 0 {
		a, b = -a, -b
	}
	if a >= 0 {
		return a / b
	}
	return -((-a + b - 1) / b)
}

// DirectNoiseFrame is the cosine-sum oracle for verification (not the fast path).
// Returns u(m,j) for j=0..L-1.
func DirectNoiseFrame(seed uint32, blockIndex, m int, lo, hi, slope float64, rate int) []float64 {
	const L = SynthL
	w := RegionWeights(lo, hi, slope, rate)
	sum2 := 0.0
	for k := 1; k < L/2; k++ {
		sum2 += w[k] * w[k]
	}
	z := math.Sqrt(sum2 / 2)
	u := make([]float64, L)
	if z == 0 {
		return u
	}
	for j := 0; j < L; j++ {
		sum := 0.0
		for k := 1; k < L/2; k++ {
			if w[k] == 0 {
				continue
			}
			ph := NoisePhase(seed, blockIndex, m, k)
			sum += w[k] / z * math.Cos(2*math.Pi*(float64(k)*float64(j)/float64(L)+ph))
		}
		u[j] = sum
	}
	return u
}

// IFFTNoiseFrame synthesizes one frame via the IFFT path (for testing).
func IFFTNoiseFrame(seed uint32, blockIndex, m int, lo, hi, slope float64, rate int) []float64 {
	const L = SynthL
	w := RegionWeights(lo, hi, slope, rate)
	sum2 := 0.0
	for k := 1; k < L/2; k++ {
		sum2 += w[k] * w[k]
	}
	z := math.Sqrt(sum2 / 2)
	u := make([]float64, L)
	if z == 0 {
		return u
	}
	X := make([]complex128, L)
	scale := float64(L) / 2
	for k := 1; k < L/2; k++ {
		if w[k] == 0 {
			continue
		}
		ph := NoisePhase(seed, blockIndex, m, k)
		ang := 2 * math.Pi * ph
		mag := scale * w[k] / z
		c := complex(mag*math.Cos(ang), mag*math.Sin(ang))
		X[k] = c
		X[L-k] = complex(real(c), -imag(c))
	}
	FFT(X, true)
	for j := range u {
		u[j] = real(X[j])
	}
	return u
}

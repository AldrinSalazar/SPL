package synth

import (
	"context"
	"math"

	"spl/pkg/spl"
)

func renderTrack(b *spl.TrackBlock, doc *spl.Document, out []float64, ctx context.Context, add func(int64)) *spl.Diagnostic {
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
	// Cumulative cycles at knots.
	cum := make([]float64, len(rows))
	for i := 0; i+1 < len(rows); i++ {
		d := rows[i+1].Time - rows[i].Time
		f0 := rows[i].Freq
		m := (rows[i+1].Freq - rows[i].Freq) / d
		cum[i+1] = cum[i] + f0*d + m*d*d/2
	}
	seg := 0
	// Advance seg so rows[seg].Time <= t < rows[seg+1].Time.
	var processed int64
	for s := ns; s < ne; s++ {
		if ctx != nil && (s-ns)%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return &spl.Diagnostic{Code: spl.CodeRenderError, Message: "render cancelled: " + err.Error()}
			}
		}
		t := float64(s) / float64(rate)
		for seg+1 < len(rows)-1 && t >= rows[seg+1].Time {
			seg++
		}
		// Clamp seg for safety.
		if seg+1 >= len(rows) {
			seg = len(rows) - 2
		}
		dt := t - rows[seg].Time
		d := rows[seg+1].Time - rows[seg].Time
		frac := dt / d
		g := rows[seg].Gain + (rows[seg+1].Gain-rows[seg].Gain)*frac
		if g == 0 {
			processed++
			continue
		}
		f0 := rows[seg].Freq
		m := (rows[seg+1].Freq - rows[seg].Freq) / d
		cyc := cum[seg] + f0*dt + m*dt*dt/2
		out[s] += g * math.Cos(2*math.Pi*cyc)
		processed++
		if processed%16384 == 0 && add != nil {
			add(16384)
			processed = 0
		}
	}
	if add != nil && processed > 0 {
		add(processed)
	}
	return nil
}

func renderHarmonics(b *spl.HarmonicsBlock, doc *spl.Document, out []float64, ctx context.Context, add func(int64)) *spl.Diagnostic {
	rate := doc.Rate
	n := doc.NumSamples
	nyquist := float64(rate) / 2
	curve := b.Curve
	spec := b.Spectrum
	if len(curve) < 2 || len(spec) < 2 {
		return nil
	}
	t0 := curve[0].Time
	t1 := curve[len(curve)-1].Time
	ns, ne := spl.ActiveRange(t0, t1, rate, n)
	if ne <= ns {
		return nil
	}
	cum := make([]float64, len(curve))
	for i := 0; i+1 < len(curve); i++ {
		d := curve[i+1].Time - curve[i].Time
		f0 := curve[i].Freq
		m := (curve[i+1].Freq - curve[i].Freq) / d
		cum[i+1] = cum[i] + f0*d + m*d*d/2
	}
	s0 := spec[0].Freq
	sLast := spec[len(spec)-1].Freq
	seg := 0
	var processed int64
	// Reusable spectrum segment pointer per sample is reset; spectrum is small.
	for s := ns; s < ne; s++ {
		if ctx != nil && (s-ns)%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return &spl.Diagnostic{Code: spl.CodeRenderError, Message: "render cancelled: " + err.Error()}
			}
		}
		t := float64(s) / float64(rate)
		for seg+1 < len(curve)-1 && t >= curve[seg+1].Time {
			seg++
		}
		if seg+1 >= len(curve) {
			seg = len(curve) - 2
		}
		d := curve[seg+1].Time - curve[seg].Time
		dt := t - curve[seg].Time
		frac := dt / d
		pitch := curve[seg].Freq + (curve[seg+1].Freq-curve[seg].Freq)*frac
		gain := curve[seg].Gain + (curve[seg+1].Gain-curve[seg].Gain)*frac
		if gain == 0 {
			processed++
			continue
		}
		if !(pitch > 0) {
			processed++
			continue
		}
		f0 := curve[seg].Freq
		m := (curve[seg+1].Freq - curve[seg].Freq) / d
		cyc := cum[seg] + f0*dt + m*dt*dt/2
		// Harmonic k range from spectrum extent.
		kMin := int(math.Ceil(s0 / pitch))
		if kMin < 1 {
			kMin = 1
		}
		kMax := int(math.Floor(sLast / pitch))
		// Adjust for float rounding.
		for kMin <= kMax && float64(kMin)*pitch < s0 {
			kMin++
		}
		for kMax >= kMin && float64(kMax)*pitch > sLast {
			kMax--
		}
		// Enforce Nyquist strictly (normally redundant since sLast < Nyquist).
		for kMax >= kMin && !(float64(kMax)*pitch < nyquist) {
			kMax--
		}
		if kMax < kMin {
			processed++
			continue
		}
		// First pass: Z = sum w.
		z := 0.0
		// Walk spectrum segments for increasing fk.
		sp := 0
		for k := kMin; k <= kMax; k++ {
			fk := float64(k) * pitch
			w := interpSpectrum(spec, fk, &sp)
			z += w
		}
		if z == 0 {
			processed++
			continue
		}
		sum := 0.0
		sp = 0
		for k := kMin; k <= kMax; k++ {
			fk := float64(k) * pitch
			w := interpSpectrum(spec, fk, &sp)
			if w == 0 {
				continue
			}
			sum += w * math.Cos(2*math.Pi*float64(k)*cyc)
		}
		out[s] += gain * sum / z
		processed++
		if processed%4096 == 0 && add != nil {
			add(4096)
			processed = 0
		}
	}
	if add != nil && processed > 0 {
		add(processed)
	}
	return nil
}

// interpSpectrum linearly interpolates weight at f; *seg is a hint advanced
// monotonically for increasing f. Weight is zero outside [s0,sLast].
func interpSpectrum(spec []spl.SpectrumRow, f float64, seg *int) float64 {
	if f < spec[0].Freq || f > spec[len(spec)-1].Freq {
		return 0
	}
	i := *seg
	if i < 0 {
		i = 0
	}
	if i >= len(spec)-1 {
		i = len(spec) - 2
	}
	for i+1 < len(spec)-1 && f > spec[i+1].Freq {
		i++
	}
	for i > 0 && f < spec[i].Freq {
		i--
	}
	*seg = i
	f0 := spec[i].Freq
	f1 := spec[i+1].Freq
	w0 := spec[i].Weight
	w1 := spec[i+1].Weight
	if f == f0 {
		return w0
	}
	if f == f1 {
		return w1
	}
	if f1 == f0 {
		return w0
	}
	frac := (f - f0) / (f1 - f0)
	return (1-frac)*w0 + frac*w1
}

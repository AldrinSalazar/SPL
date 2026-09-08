package synth

import (
	"context"
	"math"

	"spl/pkg/spl"
)

func renderHit(b *spl.HitBlock, doc *spl.Document, out []float64, ctx context.Context, add func(int64)) *spl.Diagnostic {
	rate := doc.Rate
	n := doc.NumSamples
	const L = SynthL
	for _, ev := range b.Rows {
		if d := func() *spl.Diagnostic {
			if ev.Gain == 0 {
				return nil
			}
			w := RegionWeights(ev.Low, ev.High, ev.Slope, rate)
			z := 0.0
			for k := 1; k < L/2; k++ {
				z += w[k]
			}
			if z == 0 {
				return nil
			}
			ns, ne := spl.ActiveRange(ev.Time, ev.Time+ev.Length, rate, n)
			if ne <= ns {
				return nil
			}
			// Precompute normalized weights and freqs for contributing bins.
			type bin struct {
				f  float64
				wn float64
			}
			var bins []bin
			for k := 1; k < L/2; k++ {
				if w[k] == 0 {
					continue
				}
				fk := float64(k) * float64(rate) / float64(L)
				bins = append(bins, bin{f: fk, wn: w[k] / z})
			}
			if len(bins) == 0 {
				return nil
			}
			center := ev.Time + ev.Length/2
			var processed int64
			for s := ns; s < ne; s++ {
				if ctx != nil && (s-ns)%2048 == 0 {
					if err := ctx.Err(); err != nil {
						return &spl.Diagnostic{Code: spl.CodeRenderError, Message: "render cancelled: " + err.Error()}
					}
				}
				t := float64(s) / float64(rate)
				u := (t - ev.Time) / ev.Length
				sv := math.Sin(math.Pi * u)
				env := sv * sv
				if env == 0 {
					processed++
					continue
				}
				dt := t - center
				sum := 0.0
				for _, bn := range bins {
					sum += bn.wn * math.Cos(2*math.Pi*bn.f*dt)
				}
				out[s] += ev.Gain * env * sum
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
		}(); d != nil {
			return d
		}
	}
	return nil
}

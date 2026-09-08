package spl

import (
	"fmt"
	"math"
)

// Preflight estimates rendering work before allocation and rejects excessive
// work with an actionable RESOURCE_LIMIT diagnostic. It includes tiny positive
// pitches that imply huge harmonic counts.
func Preflight(doc *Document, lim Limits) *Diagnostic {
	rate := float64(doc.Rate)
	nyquist := rate / 2
	n := doc.NumSamples
	// Render memory: output PCM float64 + one noise temp buffer + overhead.
	mem := int64(n)*8*2 + 16<<20
	if mem > lim.MaxRenderMemory {
		return &Diagnostic{Code: CodeResourceLimit,
			Message: fmt.Sprintf("estimated render memory %d bytes exceeds limit %d bytes for %d samples; shorten DURATION or lower RATE", mem, lim.MaxRenderMemory, n)}
	}
	// Harmonic work.
	var harmTotal int64
	for _, blk := range doc.Blocks {
		hb, ok := blk.(*HarmonicsBlock)
		if !ok {
			continue
		}
		if len(hb.Curve) < 2 || len(hb.Spectrum) < 2 {
			continue
		}
		t0 := hb.Curve[0].Time
		t1 := hb.Curve[len(hb.Curve)-1].Time
		active := activeSampleCount(t0, t1, doc.Rate, n)
		if active <= 0 {
			continue
		}
		minPitch := math.Inf(1)
		for _, r := range hb.Curve {
			if r.Freq < minPitch {
				minPitch = r.Freq
			}
		}
		if !(minPitch > 0) {
			continue
		}
		sMax := hb.Spectrum[len(hb.Spectrum)-1].Freq
		// Upper bound on harmonics per sample: spectrum extent / min pitch,
		// also bounded by Nyquist / min pitch.
		per := sMax / minPitch
		alt := (nyquist - 1e-12) / minPitch
		if alt < per {
			per = alt
		}
		// per may be +Inf or huge for tiny pitches.
		if math.IsInf(per, 1) || math.IsNaN(per) || per > 1e12 {
			return &Diagnostic{Code: CodeResourceLimit, BlockLine: hb.Open,
				Message: fmt.Sprintf("harmonics block opened on line %d: pitch %g implies excessive harmonic count (spectrum max %g); raise pitch or narrow spectrum", hb.Open, minPitch, sMax)}
		}
		est := int64(math.Ceil(per+1)) * int64(active)
		// Guard overflow.
		if est < 0 || harmTotal+est < harmTotal {
			return &Diagnostic{Code: CodeResourceLimit, BlockLine: hb.Open,
				Message: fmt.Sprintf("estimated harmonic work exceeds limit %d; raise pitch or shorten block opened on line %d", lim.MaxHarmonicEvals, hb.Open)}
		}
		harmTotal += est
		if harmTotal > lim.MaxHarmonicEvals {
			return &Diagnostic{Code: CodeResourceLimit, BlockLine: hb.Open,
				Message: fmt.Sprintf("estimated harmonic evaluations %d exceed limit %d; raise pitch, narrow spectrum, or shorten audio", harmTotal, lim.MaxHarmonicEvals)}
		}
	}
	// Noise frames.
	totalNoiseFrames := 0
	for _, blk := range doc.Blocks {
		nb, ok := blk.(*NoiseBlock)
		if !ok {
			continue
		}
		if len(nb.Rows) < 2 {
			continue
		}
		t0 := nb.Rows[0].Time
		t1 := nb.Rows[len(nb.Rows)-1].Time
		ns, ne := activeRange(t0, t1, doc.Rate, n)
		if ne <= ns {
			continue
		}
		const L = 2048
		const H = 256
		mMin := floorDiv(ns-(L-1), H)
		mMax := floorDiv(ne-1, H)
		frames := mMax - mMin + 1
		if frames < 0 {
			frames = 0
		}
		totalNoiseFrames += frames
		if totalNoiseFrames > lim.MaxNoiseFrames {
			return &Diagnostic{Code: CodeResourceLimit, BlockLine: nb.Open,
				Message: fmt.Sprintf("estimated noise frames %d exceed limit %d; shorten noise blocks or DURATION", totalNoiseFrames, lim.MaxNoiseFrames)}
		}
	}
	// Hit bin evaluations.
	var hitTotal int64
	const maxBins = 1023 // L/2-1
	for _, blk := range doc.Blocks {
		hb, ok := blk.(*HitBlock)
		if !ok {
			continue
		}
		for _, r := range hb.Rows {
			if !(r.Length > 0) {
				continue
			}
			ns, ne := activeRange(r.Time, r.Time+r.Length, doc.Rate, n)
			active := ne - ns
			if active <= 0 {
				continue
			}
			est := int64(active) * int64(maxBins)
			if est < 0 || hitTotal+est < hitTotal {
				return &Diagnostic{Code: CodeResourceLimit, BlockLine: hb.Open,
					Message: fmt.Sprintf("estimated hit work exceeds limit %d", lim.MaxHitBinEvals)}
			}
			hitTotal += est
			if hitTotal > lim.MaxHitBinEvals {
				return &Diagnostic{Code: CodeResourceLimit, BlockLine: hb.Open,
					Message: fmt.Sprintf("estimated hit bin evaluations %d exceed limit %d; shorten hit LENGTH values", hitTotal, lim.MaxHitBinEvals)}
			}
		}
	}
	return nil
}

func activeRange(t0, t1 float64, rate, n int) (int, int) {
	return ActiveRange(t0, t1, rate, n)
}

func activeSampleCount(t0, t1 float64, rate, n int) int {
	ns, ne := ActiveRange(t0, t1, rate, n)
	if ne <= ns {
		return 0
	}
	return ne - ns
}

func floorDiv(a, b int) int {
	// b > 0.
	if b < 0 {
		a, b = -a, -b
	}
	if a >= 0 {
		return a / b
	}
	return -((-a + b - 1) / b)
}

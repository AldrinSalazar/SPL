package spectrogram

import (
	"fmt"
	"math"

	"spl/pkg/spl"
	"spl/pkg/synth"
)

// Options configures STFT analysis. Zero values select defaults.
type Options struct {
	FFTLen  int     // power of two, default 2048
	Hop     int     // positive, default 256
	FloorDB float64 // finite floor for silence; default -200
}

// Defaults.
const (
	DefaultFFTLen  = 2048
	DefaultHop     = 256
	DefaultFloorDB = -200
)

// Result is a time-major spectrogram: Matrix[frame*Bins+bin] in dBFS amplitude.
type Result struct {
	Matrix     []float64 // Frames*Bins, time-major
	Frames     int
	Bins       int // FFTLen/2+1
	FFTLen     int
	Hop        int
	Rate       int
	WindowSum  float64
	BinFreqs   []float64
	FrameTimes []float64
	FloorDB    float64
	Duration   float64 // PCM length / rate
	NumSamples int
}

// At returns dB at frame, bin.
func (r *Result) At(frame, bin int) float64 { return r.Matrix[frame*r.Bins+bin] }

// ComputeSpectrogram analyzes final mixed float64 PCM (before WAV quantization).
// Framing: centers at 0, hop, 2*hop, ... strictly below PCM length; zero-pad outside.
func ComputeSpectrogram(pcm []float64, rate int, opts *Options, lim spl.Limits) (*Result, *spl.Diagnostic) {
	fftLen := DefaultFFTLen
	hop := DefaultHop
	floor := float64(DefaultFloorDB)
	if opts != nil {
		if opts.FFTLen != 0 {
			fftLen = opts.FFTLen
		}
		if opts.Hop != 0 {
			hop = opts.Hop
		}
		if opts.FloorDB != 0 {
			floor = opts.FloorDB
		}
	}
	if !synth.IsPow2(fftLen) || fftLen < lim.MinFFTLen || fftLen > lim.MaxFFTLen {
		return nil, &spl.Diagnostic{Code: spl.CodeResourceLimit,
			Message: fmt.Sprintf("FFT length %d must be a power of two in [%d,%d]", fftLen, lim.MinFFTLen, lim.MaxFFTLen)}
	}
	if hop < lim.MinHop || hop > lim.MaxHop {
		return nil, &spl.Diagnostic{Code: spl.CodeResourceLimit,
			Message: fmt.Sprintf("hop %d out of range [%d,%d]", hop, lim.MinHop, lim.MaxHop)}
	}
	if math.IsNaN(floor) || math.IsInf(floor, 0) {
		return nil, &spl.Diagnostic{Code: spl.CodeExportError, Message: "invalid floor dB"}
	}
	n := len(pcm)
	if n <= 0 {
		return nil, &spl.Diagnostic{Code: spl.CodeExportError, Message: "empty PCM"}
	}
	frames := (n-1)/hop + 1
	bins := fftLen/2 + 1
	cells := int64(frames) * int64(bins)
	if cells > lim.MaxSpectrogramCells {
		return nil, &spl.Diagnostic{Code: spl.CodeResourceLimit,
			Message: fmt.Sprintf("spectrogram cells %d exceed limit %d; increase hop or shorten audio", cells, lim.MaxSpectrogramCells)}
	}
	window := synth.HannWindow(fftLen)
	wsum := 0.0
	for _, w := range window {
		wsum += w
	}
	if wsum <= 0 {
		return nil, &spl.Diagnostic{Code: spl.CodeExportError, Message: "invalid window sum"}
	}
	matrix := make([]float64, cells)
	binFreqs := make([]float64, bins)
	for b := 0; b < bins; b++ {
		binFreqs[b] = float64(b) * float64(rate) / float64(fftLen)
	}
	frameTimes := make([]float64, frames)
	for i := 0; i < frames; i++ {
		frameTimes[i] = float64(i*hop) / float64(rate)
	}
	X := make([]complex128, fftLen)
	half := fftLen / 2
	for i := 0; i < frames; i++ {
		c := i * hop
		// Find max abs for scaling guard.
		maxAbs := 0.0
		for j := 0; j < fftLen; j++ {
			idx := c - half + j
			var v float64
			if idx >= 0 && idx < n {
				v = pcm[idx]
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, &spl.Diagnostic{Code: spl.CodeExportError, Message: "nonfinite PCM sample"}
			}
			a := math.Abs(v * window[j])
			if a > maxAbs {
				maxAbs = a
			}
		}
		if maxAbs == 0 {
			for b := 0; b < bins; b++ {
				matrix[i*bins+b] = floor
			}
			continue
		}
		// Scale to avoid FFT overflow for huge signals.
		scale := 1.0
		// Worst-case sum magnitude <= fftLen*maxAbs.
		if float64(fftLen)*maxAbs > 1e300 {
			scale = float64(fftLen) * maxAbs / 1e300
		}
		for j := 0; j < fftLen; j++ {
			idx := c - half + j
			var v float64
			if idx >= 0 && idx < n {
				v = pcm[idx]
			}
			X[j] = complex(v*window[j]/scale, 0)
		}
		synth.FFT(X, false)
		correction := 0.0
		if scale != 1 {
			correction = 20 * math.Log10(scale)
		}
		for b := 0; b < bins; b++ {
			mag := math.Hypot(real(X[b]), imag(X[b]))
			amp := mag / wsum
			if b != 0 && b != fftLen/2 {
				amp *= 2
			}
			var db float64
			if !(amp > 0) {
				db = floor
			} else {
				db = 20*math.Log10(amp) + correction
				if math.IsNaN(db) || math.IsInf(db, -1) {
					db = floor
				}
				if db < floor {
					db = floor
				}
				if math.IsInf(db, 1) {
					db = 1000 // cap huge positive (should be rare due to scaling)
				}
			}
			matrix[i*bins+b] = db
		}
	}
	return &Result{Matrix: matrix, Frames: frames, Bins: bins, FFTLen: fftLen, Hop: hop,
		Rate: rate, WindowSum: wsum, BinFreqs: binFreqs, FrameTimes: frameTimes,
		FloorDB: floor, Duration: float64(n) / float64(rate), NumSamples: n}, nil
}

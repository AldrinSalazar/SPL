package synth

import "math"

// Synthesis constants.
const (
	SynthL = 2048
	SynthH = 256
)

// BinFreq returns f(k) = k*RATE/L in binary64.
func BinFreq(k, rate int) float64 {
	return float64(k) * float64(rate) / float64(SynthL)
}

// RegionWeights computes w(k) for k=1..L/2-1 for bounds [a,b] and slope.
// Returns slice indexed by k (length L/2, entries 0 and L/2 unused/zero).
func RegionWeights(a, b float64, slope float64, rate int) []float64 {
	l := SynthL
	half := l / 2
	w := make([]float64, half)
	binW := float64(rate) / float64(l)
	halfBin := binW / 2
	for k := 1; k < half; k++ {
		fk := float64(k) * float64(rate) / float64(l)
		cellLow := fk - halfBin
		cellHigh := fk + halfBin
		lo := cellLow
		if a > lo {
			lo = a
		}
		hi := cellHigh
		if b < hi {
			hi = b
		}
		length := hi - lo
		if !(length > 0) {
			continue
		}
		q := length / binW
		if q > 1 {
			q = 1
		}
		factor := 1.0
		if slope != 0 {
			// (f(k)/(RATE/L))^-slope = k^-slope mathematically.
			factor = math.Pow(float64(k), -slope)
		}
		w[k] = math.Sqrt(q) * factor
	}
	return w
}

// HannWindow returns the periodic Hann window of length L.
func HannWindow(L int) []float64 {
	w := make([]float64, L)
	for j := 0; j < L; j++ {
		w[j] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(j)/float64(L))
	}
	return w
}

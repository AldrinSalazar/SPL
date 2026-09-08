package synth

import (
	"math"
	"testing"

	"spl/pkg/spl"
)

func renderSrc(t *testing.T, src string) []float64 {
	t.Helper()
	doc, diags := spl.Parse([]byte(src))
	if len(diags) > 0 {
		t.Fatalf("parse diags: %+v", diags)
	}
	res, ed := Render(doc, nil)
	if ed != nil {
		t.Fatalf("render err: %+v", ed)
	}
	return res.Samples
}

func TestSpectrumExactEndpointWithLargeDynamicRange(t *testing.T) {
	pcm := renderSrc(t, "spl 2 8000 0.001 0\nharmonics\nspectrum\n100 1e20\n200 1\ncurve\n0 200 1\n0.001 200 1\nend\n")
	if pcm[0] != 1 {
		t.Fatalf("endpoint sample = %g, want 1", pcm[0])
	}
}

func TestConstantCosine(t *testing.T) {
	// 440Hz constant, gain envelope 0->0.5 quickly then hold.
	src := "spl 2 24000 0.05 0\ntrack\n0 440 0.5\n0.05 440 0.5\nend\n"
	s := renderSrc(t, src)
	rate := 24000.0
	for _, n := range []int{0, 1, 100, 1199} {
		tt := float64(n) / rate
		want := 0.5 * math.Cos(2*math.Pi*440*tt)
		if math.Abs(s[n]-want) > 1e-12 {
			t.Fatalf("n=%d got %g want %g", n, s[n], want)
		}
	}
}

func TestChirpIntegration(t *testing.T) {
	// Linear chirp 400->800 over 0.1s, gain 1.
	src := "spl 2 24000 0.1 0\ntrack\n0 400 1\n0.1 800 1\nend\n"
	s := renderSrc(t, src)
	rate := 24000.0
	f0, f1, T := 400.0, 800.0, 0.1
	m := (f1 - f0) / T
	for _, n := range []int{0, 600, 1200, 2399} {
		tt := float64(n) / rate
		cyc := f0*tt + m*tt*tt/2
		want := math.Cos(2 * math.Pi * cyc)
		if math.Abs(s[n]-want) > 1e-12 {
			t.Fatalf("n=%d got %g want %g", n, s[n], want)
		}
	}
}

func TestHalfOpenBoundary(t *testing.T) {
	// Track active [0.1,0.2). Sample at exactly 0.2 (n=4800) must be silent.
	src := "spl 2 24000 0.5 0\ntrack\n0.1 440 0.5\n0.2 440 0.5\nend\n"
	s := renderSrc(t, src)
	if s[4800] != 0 {
		t.Fatalf("boundary sample should be 0, got %g", s[4800])
	}
	// First active sample n=2400 (t=0.1): cycles=0 -> cos=1 -> 0.5.
	if math.Abs(s[2400]-0.5) > 1e-12 {
		t.Fatalf("start sample got %g want 0.5", s[2400])
	}
	// Just before start silent.
	if s[2399] != 0 {
		t.Fatalf("pre-start should be 0, got %g", s[2399])
	}
}

func TestPhaseAcrossSilentSection(t *testing.T) {
	// Gain goes to 0 in middle but phase must carry.
	src := "spl 2 24000 0.1 0\ntrack\n0 440 0.5\n0.05 440 0\n0.1 440 0.5\nend\n"
	s := renderSrc(t, src)
	rate := 24000.0
	// At t=0.075 (n=1800), gain interp 0.25, cycles=440*0.075.
	n := 1800
	tt := float64(n) / rate
	want := 0.25 * math.Cos(2*math.Pi*440*tt)
	if math.Abs(s[n]-want) > 1e-12 {
		t.Fatalf("got %g want %g", s[n], want)
	}
}

func TestHarmonicsNormalization(t *testing.T) {
	// Spectrum flat 0..5000 weight 1 except taper; pitch 220.
	// With flat spectrum, sum of harmonic amplitudes normalized to gain.
	src := "spl 2 24000 0.05 0\nharmonics\nspectrum\n0 1\n5000 1\ncurve\n0 220 0.4\n0.05 220 0.4\nend\n"
	s := renderSrc(t, src)
	// At t=0, cycles=0, all cos=1, sum w/Z = 1 -> signal = gain.
	if math.Abs(s[0]-0.4) > 1e-9 {
		t.Fatalf("t=0 got %g want 0.4", s[0])
	}
	// Bound: |signal| <= gain + rounding.
	for i, v := range s {
		if math.Abs(v) > 0.4+1e-9 {
			t.Fatalf("sample %d exceeds gain: %g", i, v)
		}
	}
}

func TestHarmonicsZeroWeight(t *testing.T) {
	// Spectrum peaks away from harmonics? pitch 100, spectrum 5000..6000.
	// Harmonics at multiples of 100: some fall in 5000..6000 (50..60th).
	// Choose spectrum 5050..5060 with pitch 100: harmonics at 5000,5100 miss -> Z=0? Actually 5050..5060 contains no multiple of 100 -> silent.
	src := "spl 2 24000 0.02 0\nharmonics\nspectrum\n5050 1\n5060 1\ncurve\n0 100 0.5\n0.02 100 0.5\nend\n"
	s := renderSrc(t, src)
	for i, v := range s {
		if v != 0 {
			t.Fatalf("sample %d expected silence, got %g", i, v)
		}
	}
}

func TestHarmonicsPhaseAgreement(t *testing.T) {
	// Two harmonic blocks same pitch curve/start, different spectra/gains.
	// Each alone has phase from same cycles; combined deterministic.
	// Here verify single-harmonic blocks agree with track at harmonic freq.
	// Spectrum narrow around 440 with pitch 440 -> only k=1 contributes -> equals track 440.
	spec := "spl 2 24000 0.03 0\nharmonics\nspectrum\n439 0\n440 1\n441 0\ncurve\n0 440 0.3\n0.03 440 0.3\nend\n"
	h := renderSrc(t, spec)
	tr := renderSrc(t, "spl 2 24000 0.03 0\ntrack\n0 440 0.3\n0.03 440 0.3\nend\n")
	for i := range h {
		if math.Abs(h[i]-tr[i]) > 1e-9 {
			t.Fatalf("sample %d harm %g track %g", i, h[i], tr[i])
		}
	}
}

func TestNoiseHashDeterminism(t *testing.T) {
	a := NoisePhase(42, 0, -1, 5)
	b := NoisePhase(42, 0, -1, 5)
	if a != b {
		t.Fatal("hash not deterministic")
	}
	c := NoisePhase(42, 1, -1, 5)
	if a == c {
		t.Fatal("block index should change hash")
	}
	d := NoisePhase(43, 0, -1, 5)
	if a == d {
		t.Fatal("seed should change hash")
	}
	// Negative m wraps: m=-1 -> 0xFFFFFFFF.
	e := NoisePhase(0, 0, -1, 1)
	f := NoisePhase(0, 0, 1<<32-1, 1) // m as int 4294967295 -> int32(-1) -> same u32
	if e != f {
		t.Fatalf("negative m wrap mismatch %g %g", e, f)
	}
	if a < 0 || a >= 1 {
		t.Fatalf("phase out of range %g", a)
	}
}

func TestDirectVsIFFT(t *testing.T) {
	for _, tc := range [][4]float64{
		{1000, 8000, 0, 24000},
		{200, 4000, 0.5, 48000},
		{0, 100, 1, 8000},
	} {
		lo, hi, slope, rate := tc[0], tc[1], tc[2], int(tc[3])
		for _, m := range []int{-8, -1, 0, 7} {
			d := DirectNoiseFrame(8, 0, m, lo, hi, slope, rate)
			f := IFFTNoiseFrame(8, 0, m, lo, hi, slope, rate)
			maxd := 0.0
			for j := range d {
				if dd := math.Abs(d[j] - f[j]); dd > maxd {
					maxd = dd
				}
			}
			if maxd > 1e-9 {
				t.Fatalf("lo=%g hi=%g slope=%g m=%d maxdiff=%g", lo, hi, slope, m, maxd)
			}
		}
	}
}

func TestNoiseFrameRMS(t *testing.T) {
	u := IFFTNoiseFrame(8, 0, 3, 1000, 8000, 0, 24000)
	sum := 0.0
	for _, v := range u {
		sum += v * v
	}
	rms := math.Sqrt(sum / float64(len(u)))
	if math.Abs(rms-1) > 1e-9 {
		t.Fatalf("rms %g want 1", rms)
	}
	// All-zero spectrum (region below lowest cell) -> silence.
	u2 := IFFTNoiseFrame(8, 0, 0, 0, 0.001, 0, 48000)
	for _, v := range u2 {
		if v != 0 {
			t.Fatalf("expected zero frame, got %g", v)
		}
	}
}

func TestFractionalCoverage(t *testing.T) {
	// Narrow region partially covering one bin cell should give partial weight, continuous.
	rate := 24000
	binW := float64(rate) / 2048
	f1 := binW // k=1 center
	// Region covering half of k=1 cell: [f1-halfBin, f1] -> q=0.5 -> w=sqrt(0.5).
	w := RegionWeights(f1-binW/2, f1, 0, rate)
	if math.Abs(w[1]-math.Sqrt(0.5)) > 1e-12 {
		t.Fatalf("w1 %g want %g", w[1], math.Sqrt(0.5))
	}
	if w[2] != 0 {
		t.Fatalf("w2 should be 0, got %g", w[2])
	}
}

func TestNoiseSeedRepeatability(t *testing.T) {
	src := "spl 2 24000 0.3 8\nnoise\n0 200 4000 0.06 0.5\n0.3 200 4000 0.06 0.5\nend\n"
	a := renderSrc(t, src)
	b := renderSrc(t, src)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("not deterministic at %d", i)
		}
	}
}

func TestNoiseInvarianceNonNoiseInsert(t *testing.T) {
	// Inserting a track (silent) should not change noise (block_index only counts noise).
	n1 := renderSrc(t, "spl 2 24000 0.2 8\nnoise\n0 500 3000 0.05 0\n0.2 500 3000 0.05 0\nend\n")
	n2 := renderSrc(t, "spl 2 24000 0.2 8\ntrack\n0 440 0\n0.2 440 0\nend\nnoise\n0 500 3000 0.05 0\n0.2 500 3000 0.05 0\nend\n")
	for i := range n1 {
		if n1[i] != n2[i] {
			t.Fatalf("noise changed at %d: %g vs %g", i, n1[i], n2[i])
		}
	}
}

func TestNoiseBoundaryCoverage(t *testing.T) {
	// Short block at start: negative frames must contribute (no abrupt cut).
	s := renderSrc(t, "spl 2 24000 0.5 0\nnoise\n0 1000 8000 0.05 0\n0.5 1000 8000 0.05 0\nend\n")
	// First sample should generally be nonzero (random but deterministic).
	// Just check rendering completes and edge samples finite.
	if math.IsNaN(s[0]) || math.IsInf(s[0], 0) {
		t.Fatal("bad first sample")
	}
	// Silence outside block.
	s2 := renderSrc(t, "spl 2 24000 0.5 0\nnoise\n0.1 1000 8000 0.05 0\n0.2 1000 8000 0.05 0\nend\n")
	if s2[0] != 0 {
		t.Fatalf("expected silence before block, got %g", s2[0])
	}
	if s2[len(s2)-1] != 0 {
		t.Fatalf("expected silence after block, got %g", s2[len(s2)-1])
	}
}

func TestHitCenterPeak(t *testing.T) {
	// Single hit, sampled peak <= GAIN, and max near center.
	src := "spl 2 48000 0.5 0\nhit\n0.1 0.004 100 10000 0.3 0\nend\n"
	s := renderSrc(t, src)
	peak := 0.0
	for _, v := range s {
		if math.Abs(v) > peak {
			peak = math.Abs(v)
		}
	}
	if peak > 0.3+1e-9 {
		t.Fatalf("peak %g exceeds GAIN 0.3", peak)
	}
	if peak <= 0 {
		t.Fatal("expected nonzero hit")
	}
	// Outside event silence.
	if s[0] != 0 || s[len(s)-1] != 0 {
		t.Fatal("expected silence outside event")
	}
}

func TestHitSimultaneousOrder(t *testing.T) {
	a := renderSrc(t, "spl 2 24000 0.5 0\nhit\n0.1 0.004 100 5000 0.2 0\n0.1 0.004 100 5000 0.3 0\nend\n")
	b := renderSrc(t, "spl 2 24000 0.5 0\nhit\n0.1 0.004 100 5000 0.3 0\n0.1 0.004 100 5000 0.2 0\nend\n")
	// Addition commutes mathematically but float order may differ in last ulp;
	// both should be close and equal to sum of individual? Check determinism of row order:
	// rendering same file twice must be identical.
	a2 := renderSrc(t, "spl 2 24000 0.5 0\nhit\n0.1 0.004 100 5000 0.2 0\n0.1 0.004 100 5000 0.3 0\nend\n")
	for i := range a {
		if a[i] != a2[i] {
			t.Fatalf("nondeterministic at %d", i)
		}
		if math.Abs(a[i]-b[i]) > 1e-12 {
			t.Fatalf("row-order swap differs more than fp tolerance at %d: %g vs %g", i, a[i], b[i])
		}
	}
}

func TestMixingOrderDeterminism(t *testing.T) {
	src := "spl 2 24000 0.2 0\ntrack\n0 440 0.2\n0.2 440 0.2\nend\ntrack\n0 550 0.2\n0.2 550 0.2\nend\n"
	a := renderSrc(t, src)
	b := renderSrc(t, src)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic at %d", i)
		}
	}
}

func TestNoPartialAfterError(t *testing.T) {
	doc, _ := spl.Parse([]byte("spl 2 24000 0.01 0\ntrack\n0 440 0.1\n0.01 440 0.1\nend\n"))
	lim := spl.DefaultLimits()
	lim.MaxSamples = 1 // force preflight failure
	_, ed := Render(doc, &Options{Limits: &lim})
	if ed == nil {
		t.Fatal("expected error")
	}
}

func TestFFTRoundtrip(t *testing.T) {
	x := []complex128{1, 2, 3, 4, 5, 6, 7, 8}
	orig := append([]complex128(nil), x...)
	FFT(x, false)
	FFT(x, true)
	for i := range x {
		if math.Abs(real(x[i])-real(orig[i])) > 1e-12 || math.Abs(imag(x[i])) > 1e-12 {
			t.Fatalf("roundtrip failed at %d: %v vs %v", i, x[i], orig[i])
		}
	}
}

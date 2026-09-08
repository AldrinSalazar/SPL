package spectrogram

import (
	"bytes"
	"image/png"
	"math"
	"testing"

	"spl/pkg/spl"
	"spl/pkg/synth"
)

func TestHugeFiniteSignalRemainsVisible(t *testing.T) {
	pcm := make([]float64, 4096)
	for i := range pcm {
		pcm[i] = 1e308
	}
	r, d := ComputeSpectrogram(pcm, 8000, nil, spl.DefaultLimits())
	if d != nil {
		t.Fatal(d)
	}
	if math.Abs(r.At(8, 0)-6160) > 1e-8 {
		t.Fatalf("DC = %g dBFS, want 6160", r.At(8, 0))
	}
}

func renderPCM(t *testing.T, src string) ([]float64, int) {
	t.Helper()
	doc, diags := spl.Parse([]byte(src))
	if len(diags) > 0 {
		t.Fatalf("parse: %+v", diags)
	}
	res, ed := synth.Render(doc, nil)
	if ed != nil {
		t.Fatalf("render: %+v", ed)
	}
	return res.Samples, res.Rate
}

func TestSilenceFinite(t *testing.T) {
	pcm := make([]float64, 5000)
	res, ed := ComputeSpectrogram(pcm, 24000, nil, spl.DefaultLimits())
	if ed != nil {
		t.Fatalf("spec: %+v", ed)
	}
	for i, v := range res.Matrix {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("cell %d nonfinite %g", i, v)
		}
		if v != float64(DefaultFloorDB) {
			t.Fatalf("cell %d got %g want floor %g", i, v, float64(DefaultFloorDB))
		}
	}
}

func TestBinCenteredTone(t *testing.T) {
	// Bin-centered cosine amplitude 1: freq = k*rate/L, interior bin.
	rate, L := 24000, 2048
	k := 20
	freq := float64(k) * float64(rate) / float64(L)
	n := 24000 // 1s
	pcm := make([]float64, n)
	for i := range pcm {
		pcm[i] = math.Cos(2 * math.Pi * freq * float64(i) / float64(rate))
	}
	res, ed := ComputeSpectrogram(pcm, rate, nil, spl.DefaultLimits())
	if ed != nil {
		t.Fatalf("spec: %+v", ed)
	}
	// Middle frame away from boundaries.
	mid := res.Frames / 2
	peakBin := 0
	peakVal := math.Inf(-1)
	for b := 0; b < res.Bins; b++ {
		if v := res.At(mid, b); v > peakVal {
			peakVal = v
			peakBin = b
		}
	}
	if peakBin != k {
		t.Fatalf("peak bin %d want %d", peakBin, k)
	}
	if math.Abs(peakVal-0) > 1.5 {
		t.Fatalf("peak dB %g want ~0", peakVal)
	}
}

func TestChirpRises(t *testing.T) {
	pcm, rate := renderPCM(t, "spl 2 24000 0.5 0\ntrack\n0 500 0.5\n0.5 4000 0.5\nend\n")
	res, ed := ComputeSpectrogram(pcm, rate, nil, spl.DefaultLimits())
	if ed != nil {
		t.Fatalf("spec: %+v", ed)
	}
	peakAt := func(frame int) int {
		best, bv := 0, math.Inf(-1)
		for b := 0; b < res.Bins; b++ {
			if v := res.At(frame, b); v > bv {
				bv, best = v, b
			}
		}
		return best
	}
	early := peakAt(res.Frames / 4)
	late := peakAt(3 * res.Frames / 4)
	if !(late > early) {
		t.Fatalf("chirp bins early=%d late=%d", early, late)
	}
}

func TestTwoToneSeparation(t *testing.T) {
	rate := 24000
	n := 24000
	pcm := make([]float64, n)
	for i := range pcm {
		tm := float64(i) / float64(rate)
		pcm[i] = 0.5*math.Cos(2*math.Pi*1000*tm) + 0.5*math.Cos(2*math.Pi*5000*tm)
	}
	res, ed := ComputeSpectrogram(pcm, rate, nil, spl.DefaultLimits())
	if ed != nil {
		t.Fatalf("spec: %+v", ed)
	}
	mid := res.Frames / 2
	// Find top two local peaks.
	type pb struct {
		b int
		v float64
	}
	var peaks []pb
	for b := 1; b < res.Bins-1; b++ {
		v := res.At(mid, b)
		if v > res.At(mid, b-1) && v > res.At(mid, b+1) && v > -40 {
			peaks = append(peaks, pb{b, v})
		}
	}
	if len(peaks) < 2 {
		t.Fatalf("expected 2 peaks, got %v", peaks)
	}
}

func TestShortAudioPadding(t *testing.T) {
	pcm := []float64{0.5}
	res, ed := ComputeSpectrogram(pcm, 24000, nil, spl.DefaultLimits())
	if ed != nil {
		t.Fatalf("spec: %+v", ed)
	}
	if res.Frames != 1 {
		t.Fatalf("frames %d want 1", res.Frames)
	}
	if res.Bins != DefaultFFTLen/2+1 {
		t.Fatalf("bins %d", res.Bins)
	}
}

func TestFrameCountsAndOrientation(t *testing.T) {
	pcm := make([]float64, 1000)
	res, ed := ComputeSpectrogram(pcm, 8000, &Options{FFTLen: 512, Hop: 100}, spl.DefaultLimits())
	if ed != nil {
		t.Fatalf("spec: %+v", ed)
	}
	wantFrames := (1000-1)/100 + 1
	if res.Frames != wantFrames || res.Bins != 257 {
		t.Fatalf("got %dx%d", res.Frames, res.Bins)
	}
	if len(res.Matrix) != res.Frames*res.Bins {
		t.Fatal("bad matrix length")
	}
	// Time-major: consecutive bins of same frame contiguous.
	if res.FrameTimes[1]-res.FrameTimes[0] <= 0 {
		t.Fatal("frame times must increase")
	}
}

func TestPNGDimensions(t *testing.T) {
	pcm, rate := renderPCM(t, "spl 2 24000 0.2 0\ntrack\n0 440 0.3\n0.2 440 0.3\nend\n")
	res, ed := ComputeSpectrogram(pcm, rate, nil, spl.DefaultLimits())
	if ed != nil {
		t.Fatalf("spec: %+v", ed)
	}
	b, disp, perr := EncodeSpectrogramPNG(res, &PNGOptions{PlotWidth: 400, PlotHeight: 200})
	if perr != nil {
		t.Fatalf("png: %+v", perr)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != disp.TotalW || bounds.Dy() != disp.TotalH {
		t.Fatalf("dims %v vs %+v", bounds, disp)
	}
	// Cursor mapping inside plot.
	_, _, _, ok := disp.Cursor(disp.OriginX, disp.OriginY)
	if !ok {
		t.Fatal("cursor should hit plot origin")
	}
	_, _, _, ok = disp.Cursor(0, 0)
	if ok {
		t.Fatal("cursor should miss outside plot")
	}
}

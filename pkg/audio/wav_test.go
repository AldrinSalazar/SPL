package audio

import (
	"math"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	in := []float64{0, 0.5, -0.5, 1.5, -2.0, 1e-30}
	b, ed := EncodeFloatWAV(in, 24000)
	if ed != nil {
		t.Fatalf("encode: %+v", ed)
	}
	dec, err := DecodeFloatWAV(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dec.Rate != 24000 || len(dec.Samples) != len(in) {
		t.Fatalf("bad header %+v", dec)
	}
	for i, v := range in {
		if math.Abs(float64(dec.Samples[i])-v) > 1e-7 {
			t.Fatalf("sample %d got %g want %g", i, dec.Samples[i], v)
		}
	}
}

func TestOverflowRejected(t *testing.T) {
	_, ed := EncodeFloatWAV([]float64{1e300}, 24000)
	if ed == nil {
		t.Fatal("expected overflow error")
	}
}

func TestNonfiniteRejected(t *testing.T) {
	_, ed := EncodeFloatWAV([]float64{math.NaN()}, 24000)
	if ed == nil {
		t.Fatal("expected error")
	}
}

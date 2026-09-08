package spl

import (
	"strings"
	"testing"
)

func TestFencesInsideComments(t *testing.T) {
	mustParse(t, "spl 2 8000 0.01 0 # ```\n# ``` ignored\n")
	expectCode(t, "spl 2 8000 0.01 0\n```", CodeMarkdownFence)
}

func mustParse(t *testing.T, src string) *Document {
	t.Helper()
	doc, diags := Parse([]byte(src))
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %+v", diags)
	}
	if doc == nil {
		t.Fatal("nil doc")
	}
	return doc
}

func expectCode(t *testing.T, src string, code string) {
	t.Helper()
	_, diags := Parse([]byte(src))
	if len(diags) == 0 {
		t.Fatalf("expected diagnostic %s, got none", code)
	}
	for _, d := range diags {
		if d.Code == code {
			return
		}
	}
	t.Fatalf("expected code %s, got %+v", code, diags)
}

func TestHeaderOnlySilence(t *testing.T) {
	doc := mustParse(t, "spl 2 24000 1.5 0\n")
	if doc.NumSamples != 36000 {
		t.Fatalf("got %d want 36000", doc.NumSamples)
	}
	if len(doc.Blocks) != 0 {
		t.Fatalf("expected 0 blocks")
	}
}

func TestExactSampleCountBoundary(t *testing.T) {
	// 0.1 * 24000 = 2400 exactly in decimal; float64 0.1*24000 may round.
	doc := mustParse(t, "spl 2 24000 0.1 0\n")
	if doc.NumSamples != 2400 {
		t.Fatalf("got %d want 2400", doc.NumSamples)
	}
	// Exponent notation: 1e-1 = 0.1.
	doc = mustParse(t, "spl 2 24000 1e-1 0\n")
	if doc.NumSamples != 2400 {
		t.Fatalf("got %d want 2400", doc.NumSamples)
	}
	// Tiny positive duration -> 1 sample.
	doc = mustParse(t, "spl 2 8000 1e-9 0\n")
	if doc.NumSamples != 1 {
		t.Fatalf("got %d want 1", doc.NumSamples)
	}
	// Non-integer boundary: ceil(24000*0.00002)=ceil(0.48)=1.
	doc = mustParse(t, "spl 2 24000 0.00002 0\n")
	if doc.NumSamples != 1 {
		t.Fatalf("got %d want 1", doc.NumSamples)
	}
}

func TestCRLFAndMissingNewline(t *testing.T) {
	src := "spl 2 24000 1 0\r\ntrack\r\n0 440 0\r\n1 440 0\r\nend"
	doc := mustParse(t, src)
	if len(doc.Blocks) != 1 {
		t.Fatal("expected 1 block")
	}
}

func TestCommentsAndLineNumbers(t *testing.T) {
	src := "# comment\nspl 2 24000 1 0 # trailing\n\ntrack # c\n0 440 0\n0.5 440\nend\n"
	_, diags := Parse([]byte(src))
	if len(diags) == 0 {
		t.Fatal("expected field count error")
	}
	found := false
	for _, d := range diags {
		if d.Code == CodeFieldCount && d.Line == 6 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected FIELD_COUNT on line 6, got %+v", diags)
	}
}

func TestMalformedNumbers(t *testing.T) {
	expectCode(t, "spl 2 24000 1 0\ntrack\n0 440 0\n1hz 440 0\nend\n", CodeNumberFormat)
	expectCode(t, "spl 2 24000 1 0\ntrack\n0 440 0\n1 440 0,5\nend\n", CodeNumberFormat)
	expectCode(t, "spl 2 24000 1,5 0\n", CodeDurationFormat)
	expectCode(t, "spl 2 24k 1 0\n", CodeRateFormat)
}

func TestMarkdownFence(t *testing.T) {
	expectCode(t, "```text\nspl 2 24000 1 0\n```\n", CodeMarkdownFence)
}

func TestUnknownAndExtra(t *testing.T) {
	expectCode(t, "spl 2 24000 1 0\nfoo\n", CodeUnknownBlock)
	expectCode(t, "spl 2 24000 1 0 extra\n", CodeHeaderFieldCount)
	expectCode(t, "spl 2 24000 1 0\ntrack extra\n0 440 0\n1 440 0\nend\n", CodeFieldCount)
}

func TestTimeOrdering(t *testing.T) {
	expectCode(t, "spl 2 24000 1 0\ntrack\n0.5 440 0.1\n0.5 440 0.1\nend\n", CodeTimeOrder)
	// Hit nondecreasing allows equal.
	mustParse(t, "spl 2 24000 1 0\nhit\n0.1 0.01 100 1000 0.1 0\n0.1 0.01 100 1000 0.1 0\nend\n")
	expectCode(t, "spl 2 24000 1 0\nhit\n0.2 0.01 100 1000 0.1 0\n0.1 0.01 100 1000 0.1 0\nend\n", CodeHitTimeOrder)
}

func TestNyquistStrict(t *testing.T) {
	// Nyquist = 12000; 12000 must fail for track (strict <).
	expectCode(t, "spl 2 24000 1 0\ntrack\n0 12000 0.1\n1 440 0\nend\n", CodeFrequencyRange)
	// Spectrum upper bound strict <: 12000 fails.
	expectCode(t, "spl 2 24000 1 0\nharmonics\nspectrum\n0 1\n12000 1\ncurve\n0 440 0\n1 440 0\nend\n", CodeSpectrumRange)
	// Noise HIGH strict.
	expectCode(t, "spl 2 24000 1 0\nnoise\n0 100 12000 0.1 0\n1 100 11000 0 0\nend\n", CodeBoundsRange)
}

func TestSpectrumAllZero(t *testing.T) {
	expectCode(t, "spl 2 24000 1 0\nharmonics\nspectrum\n0 0\n1000 0\ncurve\n0 440 0\n1 440 0\nend\n", CodeSpectrumAllZero)
}

func TestHitExceedsDuration(t *testing.T) {
	expectCode(t, "spl 2 24000 1 0\nhit\n0.9 0.2 100 1000 0.1 0\nend\n", CodeHitDuration)
}

func TestUnclosed(t *testing.T) {
	expectCode(t, "spl 2 24000 1 0\ntrack\n0 440 0\n1 440 0\n", CodeUnclosedBlock)
}

func TestHarmonicsSection(t *testing.T) {
	expectCode(t, "spl 2 24000 1 0\nharmonics\ncurve\n0 440 0\n1 440 0\nend\n", CodeSectionRequired)
}

func TestOversizedInput(t *testing.T) {
	lim := DefaultLimits()
	lim.MaxInputBytes = 10
	_, diags := ParseWithLimits([]byte("spl 2 24000 1 0\n"), lim)
	found := false
	for _, d := range diags {
		if d.Code == CodeResourceLimit {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected RESOURCE_LIMIT, got %+v", diags)
	}
}

func TestNonfinite(t *testing.T) {
	expectCode(t, "spl 2 24000 1e10000 0\n", CodeNonfiniteNumber)
}

func TestRowCounts(t *testing.T) {
	expectCode(t, "spl 2 24000 1 0\ntrack\n0 440 0\nend\n", CodeRowCount)
	expectCode(t, "spl 2 24000 1 0\nnoise\n0 100 1000 0 0\nend\n", CodeRowCount)
}

func TestHeaderOnlyWithComments(t *testing.T) {
	doc := mustParse(t, "# x\n  spl 2 48000 2 7  # y\n")
	if doc.Rate != 48000 || doc.Seed != 7 || doc.NumSamples != 96000 {
		t.Fatalf("bad header %+v", doc)
	}
}

func TestTabsAndIndentation(t *testing.T) {
	src := "spl 2 24000 1 0\n\t track\n\t 0\t440\t0\n\t 1\t440\t0\n\t end\n"
	// Leading tab before "track" is fine since tokenize skips whitespace.
	_ = src
	doc := mustParse(t, "spl 2 24000 1 0\n\ttrack\n\t0\t440\t0\n\t1\t440\t0\nend\n")
	if len(doc.Blocks) != 1 {
		t.Fatal("expected 1 block")
	}
}

func TestInvalidRatesSeeds(t *testing.T) {
	expectCode(t, "spl 2 7999 1 0\n", CodeRateRange)
	expectCode(t, "spl 2 192001 1 0\n", CodeRateRange)
	expectCode(t, "spl 2 24000 1 4294967296\n", CodeSeedRange)
	expectCode(t, "spl 2 24000 -1 0\n", CodeDurationRange)
}

func TestNegativeGains(t *testing.T) {
	expectCode(t, "spl 2 24000 1 0\ntrack\n0 440 -0.1\n1 440 0\nend\n", CodeGainRange)
}

func TestLargeExponentBound(t *testing.T) {
	expectCode(t, "spl 2 24000 1e10001 0\n", CodeResourceLimit)
}

func TestSimultaneousHitsOK(t *testing.T) {
	mustParse(t, strings.Join([]string{
		"spl 2 24000 1 0",
		"hit",
		"0.1 0.004 100 10000 0.3 0",
		"0.1 0.004 100 10000 0.2 0",
		"end",
	}, "\n"))
}

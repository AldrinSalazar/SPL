package spl

import "fmt"

// validateSemantics enforces range, ordering, positivity, row-count, and
// duration constraints on successfully parsed rows.
func validateSemantics(doc *Document, nyquist float64) []Diagnostic {
	var diags []Diagnostic
	dur := doc.Duration
	for _, blk := range doc.Blocks {
		switch b := blk.(type) {
		case *TrackBlock:
			if len(b.Rows) < 2 {
				diags = append(diags, Diagnostic{Code: CodeRowCount,
					Message: fmt.Sprintf("track requires at least 2 rows; found %d", len(b.Rows)),
					Line: b.Open, BlockLine: b.Open})
			}
			for i, r := range b.Rows {
				if r.Time < 0 || r.Time > dur {
					diags = append(diags, Diagnostic{Code: CodeTimeRange, Field: "TIME",
						Message: fmt.Sprintf("time %g out of range [0, %g]", r.Time, dur),
						Line: r.Line, BlockLine: b.Open})
				}
				if i > 0 && !(r.Time > b.Rows[i-1].Time) {
					diags = append(diags, Diagnostic{Code: CodeTimeOrder, Field: "TIME",
						Message: fmt.Sprintf("time %g must be greater than previous time %g", r.Time, b.Rows[i-1].Time),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Freq > 0 && r.Freq < nyquist) {
					diags = append(diags, Diagnostic{Code: CodeFrequencyRange, Field: "FREQUENCY",
						Message: fmt.Sprintf("frequency %g must be in (0, %g)", r.Freq, nyquist),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Gain >= 0) {
					diags = append(diags, Diagnostic{Code: CodeGainRange, Field: "GAIN",
						Message: fmt.Sprintf("gain %g must be nonnegative", r.Gain),
						Line: r.Line, BlockLine: b.Open})
				}
			}
		case *HarmonicsBlock:
			if len(b.Spectrum) < 2 {
				ln := b.Open
				if b.SpectrumLine != 0 {
					ln = b.SpectrumLine
				}
				diags = append(diags, Diagnostic{Code: CodeRowCount,
					Message: fmt.Sprintf("spectrum requires at least 2 rows; found %d", len(b.Spectrum)),
					Line: ln, BlockLine: b.Open})
			}
			if len(b.Curve) < 2 {
				ln := b.Open
				if b.CurveLine != 0 {
					ln = b.CurveLine
				}
				diags = append(diags, Diagnostic{Code: CodeRowCount,
					Message: fmt.Sprintf("curve requires at least 2 rows; found %d", len(b.Curve)),
					Line: ln, BlockLine: b.Open})
			}
			anyPositive := false
			for i, r := range b.Spectrum {
				if r.Freq < 0 || r.Freq >= nyquist {
					diags = append(diags, Diagnostic{Code: CodeSpectrumRange, Field: "FREQUENCY",
						Message: fmt.Sprintf("spectrum frequency %g must be in [0, %g)", r.Freq, nyquist),
						Line: r.Line, BlockLine: b.Open})
				}
				if i > 0 && !(r.Freq > b.Spectrum[i-1].Freq) {
					diags = append(diags, Diagnostic{Code: CodeSpectrumOrder, Field: "FREQUENCY",
						Message: fmt.Sprintf("spectrum frequency %g must be greater than previous %g", r.Freq, b.Spectrum[i-1].Freq),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Weight >= 0) {
					diags = append(diags, Diagnostic{Code: CodeWeightRange, Field: "WEIGHT",
						Message: fmt.Sprintf("weight %g must be nonnegative", r.Weight),
						Line: r.Line, BlockLine: b.Open})
				}
				if r.Weight > 0 {
					anyPositive = true
				}
			}
			if len(b.Spectrum) >= 2 && !anyPositive {
				diags = append(diags, Diagnostic{Code: CodeSpectrumAllZero, Field: "WEIGHT",
					Message: "spectrum weights must include at least one positive value",
					Line: b.SpectrumLine, BlockLine: b.Open})
			}
			for i, r := range b.Curve {
				if r.Time < 0 || r.Time > dur {
					diags = append(diags, Diagnostic{Code: CodeTimeRange, Field: "TIME",
						Message: fmt.Sprintf("time %g out of range [0, %g]", r.Time, dur),
						Line: r.Line, BlockLine: b.Open})
				}
				if i > 0 && !(r.Time > b.Curve[i-1].Time) {
					diags = append(diags, Diagnostic{Code: CodeTimeOrder, Field: "TIME",
						Message: fmt.Sprintf("time %g must be greater than previous time %g", r.Time, b.Curve[i-1].Time),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Freq > 0 && r.Freq < nyquist) {
					diags = append(diags, Diagnostic{Code: CodeFrequencyRange, Field: "PITCH",
						Message: fmt.Sprintf("pitch %g must be in (0, %g)", r.Freq, nyquist),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Gain >= 0) {
					diags = append(diags, Diagnostic{Code: CodeGainRange, Field: "GAIN",
						Message: fmt.Sprintf("gain %g must be nonnegative", r.Gain),
						Line: r.Line, BlockLine: b.Open})
				}
			}
		case *NoiseBlock:
			if len(b.Rows) < 2 {
				diags = append(diags, Diagnostic{Code: CodeRowCount,
					Message: fmt.Sprintf("noise requires at least 2 rows; found %d", len(b.Rows)),
					Line: b.Open, BlockLine: b.Open})
			}
			for i, r := range b.Rows {
				if r.Time < 0 || r.Time > dur {
					diags = append(diags, Diagnostic{Code: CodeTimeRange, Field: "TIME",
						Message: fmt.Sprintf("time %g out of range [0, %g]", r.Time, dur),
						Line: r.Line, BlockLine: b.Open})
				}
				if i > 0 && !(r.Time > b.Rows[i-1].Time) {
					diags = append(diags, Diagnostic{Code: CodeTimeOrder, Field: "TIME",
						Message: fmt.Sprintf("time %g must be greater than previous time %g", r.Time, b.Rows[i-1].Time),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Low >= 0 && r.High < nyquist) || !(r.Low < r.High) {
					if !(r.Low >= 0) || !(r.High < nyquist) {
						diags = append(diags, Diagnostic{Code: CodeBoundsRange, Field: "LOW/HIGH",
							Message: fmt.Sprintf("bounds LOW %g HIGH %g must satisfy 0 <= LOW < HIGH < %g", r.Low, r.High, nyquist),
							Line: r.Line, BlockLine: b.Open})
					} else {
						diags = append(diags, Diagnostic{Code: CodeBoundsOrder, Field: "LOW/HIGH",
							Message: fmt.Sprintf("LOW %g must be less than HIGH %g", r.Low, r.High),
							Line: r.Line, BlockLine: b.Open})
					}
				}
				if !(r.Gain >= 0) {
					diags = append(diags, Diagnostic{Code: CodeGainRange, Field: "GAIN",
						Message: fmt.Sprintf("gain %g must be nonnegative", r.Gain),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Slope >= 0) {
					diags = append(diags, Diagnostic{Code: CodeSlopeRange, Field: "SLOPE",
						Message: fmt.Sprintf("slope %g must be nonnegative", r.Slope),
						Line: r.Line, BlockLine: b.Open})
				}
			}
		case *HitBlock:
			if len(b.Rows) < 1 {
				diags = append(diags, Diagnostic{Code: CodeRowCount,
					Message: "hit requires at least 1 row; found 0",
					Line: b.Open, BlockLine: b.Open})
			}
			for i, r := range b.Rows {
				if r.Time < 0 || r.Time > dur {
					diags = append(diags, Diagnostic{Code: CodeTimeRange, Field: "TIME",
						Message: fmt.Sprintf("time %g out of range [0, %g]", r.Time, dur),
						Line: r.Line, BlockLine: b.Open})
				}
				if i > 0 && !(r.Time >= b.Rows[i-1].Time) {
					diags = append(diags, Diagnostic{Code: CodeHitTimeOrder, Field: "TIME",
						Message: fmt.Sprintf("hit time %g must be nondecreasing (previous %g)", r.Time, b.Rows[i-1].Time),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Length > 0) {
					diags = append(diags, Diagnostic{Code: CodeLengthRange, Field: "LENGTH",
						Message: fmt.Sprintf("LENGTH %g must be positive", r.Length),
						Line: r.Line, BlockLine: b.Open})
				} else if r.Time+r.Length > dur {
					// Use addition in float64; spec requires TIME+LENGTH <= DURATION.
					// Allow tiny rounding? No: strict per binary64 comparison.
					diags = append(diags, Diagnostic{Code: CodeHitDuration, Field: "LENGTH",
						Message: fmt.Sprintf("hit end %g (TIME %g + LENGTH %g) exceeds DURATION %g", r.Time+r.Length, r.Time, r.Length, dur),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Low >= 0 && r.High < nyquist) || !(r.Low < r.High) {
					if !(r.Low >= 0) || !(r.High < nyquist) {
						diags = append(diags, Diagnostic{Code: CodeBoundsRange, Field: "LOW/HIGH",
							Message: fmt.Sprintf("bounds LOW %g HIGH %g must satisfy 0 <= LOW < HIGH < %g", r.Low, r.High, nyquist),
							Line: r.Line, BlockLine: b.Open})
					} else {
						diags = append(diags, Diagnostic{Code: CodeBoundsOrder, Field: "LOW/HIGH",
							Message: fmt.Sprintf("LOW %g must be less than HIGH %g", r.Low, r.High),
							Line: r.Line, BlockLine: b.Open})
					}
				}
				if !(r.Gain >= 0) {
					diags = append(diags, Diagnostic{Code: CodeGainRange, Field: "GAIN",
						Message: fmt.Sprintf("gain %g must be nonnegative", r.Gain),
						Line: r.Line, BlockLine: b.Open})
				}
				if !(r.Slope >= 0) {
					diags = append(diags, Diagnostic{Code: CodeSlopeRange, Field: "SLOPE",
						Message: fmt.Sprintf("slope %g must be nonnegative", r.Slope),
						Line: r.Line, BlockLine: b.Open})
				}
			}
		}
	}
	return diags
}

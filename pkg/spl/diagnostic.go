package spl

import "fmt"

// Diagnostic is a structured validation or resource error.
// Line is 1-based physical source line; 0 means no specific line (e.g. global limits).
// Column is 1-based byte offset in the physical line; 0 means unknown.
// Field names the offending field where useful (e.g. "RATE", "TIME").
// BlockLine is the opening line of the enclosing block where useful; 0 means none.
type Diagnostic struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Line      int    `json:"line"`
	Column    int    `json:"column,omitempty"`
	Field     string `json:"field,omitempty"`
	BlockLine int    `json:"blockLine,omitempty"`
}

func (d Diagnostic) Error() string {
	if d.Line > 0 {
		return fmt.Sprintf("line %d: %s: %s", d.Line, d.Code, d.Message)
	}
	return fmt.Sprintf("%s: %s", d.Code, d.Message)
}

// Stable diagnostic codes.
const (
	CodePeakWarning      = "PEAK_WARNING"
	CodeResourceLimit    = "RESOURCE_LIMIT"
	CodeMarkdownFence    = "MARKDOWN_FENCE"
	CodeHeaderMissing    = "HEADER_MISSING"
	CodeHeaderFieldCount = "HEADER_FIELD_COUNT"
	CodeHeaderKeyword    = "HEADER_KEYWORD"
	CodeHeaderVersion    = "HEADER_VERSION"
	CodeRateFormat       = "RATE_FORMAT"
	CodeRateRange        = "RATE_RANGE"
	CodeDurationFormat   = "DURATION_FORMAT"
	CodeDurationRange    = "DURATION_RANGE"
	CodeSeedFormat       = "SEED_FORMAT"
	CodeSeedRange        = "SEED_RANGE"
	CodeSampleCount      = "SAMPLE_COUNT"
	CodeUnknownBlock     = "UNKNOWN_BLOCK"
	CodeUnclosedBlock    = "UNCLOSED_BLOCK"
	CodeUnexpectedEnd    = "UNEXPECTED_END"
	CodeUnexpectedSect   = "UNEXPECTED_SECTION"
	CodeSectionRequired  = "SECTION_REQUIRED"
	CodeRowCount         = "ROW_COUNT"
	CodeFieldCount       = "FIELD_COUNT"
	CodeNumberFormat     = "NUMBER_FORMAT"
	CodeNonfiniteNumber  = "NONFINITE_NUMBER"
	CodeTimeRange        = "TIME_RANGE"
	CodeTimeOrder        = "TIME_ORDER"
	CodeHitTimeOrder     = "HIT_TIME_ORDER"
	CodeHitDuration      = "HIT_DURATION"
	CodeLengthRange      = "LENGTH_RANGE"
	CodeFrequencyRange   = "FREQUENCY_RANGE"
	CodeSpectrumRange    = "SPECTRUM_RANGE"
	CodeSpectrumOrder    = "SPECTRUM_ORDER"
	CodeWeightRange      = "WEIGHT_RANGE"
	CodeSpectrumAllZero  = "SPECTRUM_ALL_ZERO"
	CodeGainRange        = "GAIN_RANGE"
	CodeSlopeRange       = "SLOPE_RANGE"
	CodeBoundsOrder      = "BOUNDS_ORDER"
	CodeBoundsRange      = "BOUNDS_RANGE"
	CodeRenderError      = "RENDER_ERROR"
	CodeExportError      = "EXPORT_ERROR"
)

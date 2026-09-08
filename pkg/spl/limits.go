package spl

// Limits publishes explicit bounds for input size, structure, rendering work,
// and analysis size. Renderers must estimate work before allocating and reject
// excessive work with a RESOURCE_LIMIT diagnostic.
type Limits struct {
	MaxInputBytes       int
	MaxTokenLen         int
	MaxExponentAbs      int
	MaxBlocks           int
	MaxRowsPerBlock     int
	MaxTotalRows        int
	MaxSamples          int
	MaxRenderMemory     int64
	MaxHarmonicEvals    int64
	MaxNoiseFrames      int
	MaxHitBinEvals      int64
	MaxSpectrogramCells int64
	// Spectrogram analysis bounds.
	MinFFTLen int
	MaxFFTLen int
	MinHop    int
	MaxHop    int
}

// DefaultLimits are native CLI limits.
func DefaultLimits() Limits {
	return Limits{
		MaxInputBytes:       2_000_000,
		MaxTokenLen:         1024,
		MaxExponentAbs:      10000,
		MaxBlocks:           1000,
		MaxRowsPerBlock:     10000,
		MaxTotalRows:        50000,
		MaxSamples:          12_000_000,
		MaxRenderMemory:     256 << 20,
		MaxHarmonicEvals:    300_000_000,
		MaxNoiseFrames:      50000,
		MaxHitBinEvals:      300_000_000,
		MaxSpectrogramCells: 64_000_000,
		MinFFTLen:           256,
		MaxFFTLen:           8192,
		MinHop:              1,
		MaxHop:              8192,
	}
}

// BrowserLimits are stricter for WebAssembly in-browser rendering.
func BrowserLimits() Limits {
	l := DefaultLimits()
	l.MaxInputBytes = 1_000_000
	l.MaxBlocks = 500
	l.MaxRowsPerBlock = 5000
	l.MaxTotalRows = 20000
	l.MaxSamples = 6_000_000
	l.MaxRenderMemory = 128 << 20
	l.MaxHarmonicEvals = 50_000_000
	l.MaxNoiseFrames = 25000
	l.MaxHitBinEvals = 50_000_000
	l.MaxSpectrogramCells = 32_000_000
	return l
}

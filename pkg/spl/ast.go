package spl

// Document is a validated SPL version 2 file.
type Document struct {
	Rate          int
	DurationToken string
	Duration      float64
	Seed          uint32
	NumSamples    int
	HeaderLine    int
	Blocks        []Block
}

// Block is a track, harmonics, noise, or hit block in document order.
type Block interface {
	BlockType() string
	OpenLine() int
	CloseLine() int
}

type TrajRow struct {
	Line int
	Time float64
	Freq float64 // frequency or pitch
	Gain float64
}

type SpectrumRow struct {
	Line   int
	Freq   float64
	Weight float64
}

type NoiseRow struct {
	Line  int
	Time  float64
	Low   float64
	High  float64
	Gain  float64
	Slope float64
}

type HitRow struct {
	Line   int
	Time   float64
	Length float64
	Low    float64
	High   float64
	Gain   float64
	Slope  float64
}

type TrackBlock struct {
	Open  int
	Close int
	Rows  []TrajRow
}

func (b *TrackBlock) BlockType() string { return "track" }
func (b *TrackBlock) OpenLine() int     { return b.Open }
func (b *TrackBlock) CloseLine() int    { return b.Close }

type HarmonicsBlock struct {
	Open         int
	Close        int
	SpectrumLine int
	CurveLine    int
	Spectrum     []SpectrumRow
	Curve        []TrajRow
}

func (b *HarmonicsBlock) BlockType() string { return "harmonics" }
func (b *HarmonicsBlock) OpenLine() int     { return b.Open }
func (b *HarmonicsBlock) CloseLine() int    { return b.Close }

type NoiseBlock struct {
	Open  int
	Close int
	Rows  []NoiseRow
	Index int // noise block number starting at 0 in document order
}

func (b *NoiseBlock) BlockType() string { return "noise" }
func (b *NoiseBlock) OpenLine() int     { return b.Open }
func (b *NoiseBlock) CloseLine() int    { return b.Close }

type HitBlock struct {
	Open  int
	Close int
	Rows  []HitRow
}

func (b *HitBlock) BlockType() string { return "hit" }
func (b *HitBlock) OpenLine() int     { return b.Open }
func (b *HitBlock) CloseLine() int    { return b.Close }

// Nyquist returns RATE/2 as float64.
func (d *Document) Nyquist() float64 { return float64(d.Rate) / 2 }

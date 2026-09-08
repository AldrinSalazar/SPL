package spectrogram

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"spl/pkg/spl"
)

// PNGOptions configures spectrogram PNG export.
type PNGOptions struct {
	PlotWidth  int     // default 1000
	PlotHeight int     // default 500
	DBMin      float64 // default -100
	DBMax      float64 // default 0
	Title      string  // optional; default describes analysis
}

// Display holds the bounded display raster and geometry for cursor mapping.
// Values are aggregated dB (max-hold); Data is PlotW*PlotH row-major with
// y=0 at the top (Nyquist) matching image orientation.
type Display struct {
	Data       []float32
	PlotW      int
	PlotH      int
	OriginX    int // plot origin in PNG pixels
	OriginY    int
	TotalW     int
	TotalH     int
	Duration   float64
	Nyquist    float64
	DBMin      float64
	DBMax      float64
	FFTLen     int
	Hop        int
	Rate       int
	FloorDB    float64
	LegendX    int
	LegendY    int
	LegendW    int
	LegendH    int
	BinFreqs   []float64
	FrameTimes []float64
}

// BuildDisplay aggregates Result to a bounded raster using max-hold:
// each display cell covers a rectangular block of analysis cells
// (framesPerPixel x binsPerPixel, fractional edges handled by index ranges)
// and stores the maximum dB in that block. Time maps linearly 0..Duration
// across PlotW; frequency maps linearly 0..Nyquist across PlotH (top=Nyquist).
func BuildDisplay(res *Result, plotW, plotH int) *Display {
	if plotW <= 0 {
		plotW = 1000
	}
	if plotH <= 0 {
		plotH = 500
	}
	data := make([]float32, plotW*plotH)
	for px := 0; px < plotW; px++ {
		f0 := px * res.Frames / plotW
		f1 := (px+1)*res.Frames/plotW
		if f1 <= f0 {
			f1 = f0 + 1
		}
		if f0 < 0 {
			f0 = 0
		}
		if f1 > res.Frames {
			f1 = res.Frames
		}
		for py := 0; py < plotH; py++ {
			// py=0 top -> highest bins.
			bTop := res.Bins - 1 - (py+1)*res.Bins/plotH
			bBot := res.Bins - 1 - py*res.Bins/plotH
			if bBot < bTop {
				bTop, bBot = bBot, bTop
			}
			if bBot < bTop {
				bBot = bTop
			}
			if bTop < 0 {
				bTop = 0
			}
			if bBot >= res.Bins {
				bBot = res.Bins - 1
			}
			mx := math.Inf(-1)
			for f := f0; f < f1; f++ {
				base := f * res.Bins
				for b := bTop; b <= bBot; b++ {
					v := res.Matrix[base+b]
					if v > mx {
						mx = v
					}
				}
			}
			if math.IsInf(mx, -1) {
				mx = res.FloorDB
			}
			data[py*plotW+px] = float32(mx)
		}
	}
	return &Display{Data: data, PlotW: plotW, PlotH: plotH,
		Duration: res.Duration, Nyquist: float64(res.Rate) / 2,
		FFTLen: res.FFTLen, Hop: res.Hop, Rate: res.Rate, FloorDB: res.FloorDB,
		BinFreqs: append([]float64(nil), res.BinFreqs...),
		FrameTimes: append([]float64(nil), res.FrameTimes...)}
}

// Cursor maps a PNG pixel (total-image coordinates) to time/frequency/dB.
// Returns ok=false when outside the plot area.
func (d *Display) Cursor(px, py int) (t, f, db float64, ok bool) {
	lx := px - d.OriginX
	ly := py - d.OriginY
	if lx < 0 || lx >= d.PlotW || ly < 0 || ly >= d.PlotH {
		return 0, 0, 0, false
	}
	t = (float64(lx) + 0.5) / float64(d.PlotW) * d.Duration
	f = (1 - (float64(ly)+0.5)/float64(d.PlotH)) * d.Nyquist
	db = float64(d.Data[ly*d.PlotW+lx])
	return t, f, db, true
}

// EncodeSpectrogramPNG renders a readable PNG with axes and legend.
func EncodeSpectrogramPNG(res *Result, opts *PNGOptions) ([]byte, *Display, *spl.Diagnostic) {
	plotW, plotH := 1000, 500
	dbMin, dbMax := -100.0, 0.0
	title := ""
	if opts != nil {
		if opts.PlotWidth > 0 {
			plotW = opts.PlotWidth
		}
		if opts.PlotHeight > 0 {
			plotH = opts.PlotHeight
		}
		if opts.DBMin != 0 || opts.DBMax != 0 {
			dbMin, dbMax = opts.DBMin, opts.DBMax
		}
		title = opts.Title
	}
	if dbMax <= dbMin {
		return nil, nil, &spl.Diagnostic{Code: spl.CodeExportError, Message: "spectrogram display max must exceed min"}
	}
	if plotW <= 0 || plotH <= 0 || plotW > 4096 || plotH > 4096 {
		return nil, nil, &spl.Diagnostic{Code: spl.CodeExportError, Message: "invalid PNG plot size"}
	}
	if int64(plotW)*int64(plotH) > 8_000_000 {
		return nil, nil, &spl.Diagnostic{Code: spl.CodeResourceLimit, Message: "PNG raster too large"}
	}
	disp := BuildDisplay(res, plotW, plotH)
	disp.DBMin, disp.DBMax = dbMin, dbMax
	if title == "" {
		title = fmt.Sprintf("Spectrogram %d-pt Hann hop %d  %.1f s @ %d Hz  dBFS", res.FFTLen, res.Hop, res.Duration, res.Rate)
	}
	// Layout.
	left, top, right, bottom := 72, 46, 110, 52
	totalW := left + plotW + right
	totalH := top + plotH + bottom
	disp.OriginX, disp.OriginY = left, top
	disp.TotalW, disp.TotalH = totalW, totalH
	img := image.NewRGBA(image.Rect(0, 0, totalW, totalH))
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}
	for y := 0; y < totalH; y++ {
		for x := 0; x < totalW; x++ {
			img.Set(x, y, white)
		}
	}
	// Plot pixels.
	for py := 0; py < plotH; py++ {
		for px := 0; px < plotW; px++ {
			db := float64(disp.Data[py*plotW+px])
			img.Set(left+px, top+py, DBToColor(db, dbMin, dbMax))
		}
	}
	// Border.
	for x := left - 1; x <= left+plotW; x++ {
		img.Set(x, top-1, black)
		img.Set(x, top+plotH, black)
	}
	for y := top - 1; y <= top+plotH; y++ {
		img.Set(left-1, y, black)
		img.Set(left+plotW, y, black)
	}
	face := basicfont.Face7x13
	drawText := func(x, y int, s string, c color.Color) {
		d := &font.Drawer{Dst: img, Src: image.NewUniform(c), Face: face,
			Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	// Title.
	drawText(left, 16, title, black)
	// X ticks: time.
	for i := 0; i <= 5; i++ {
		tt := res.Duration * float64(i) / 5
		x := left + plotW*i/5
		for y := top + plotH; y < top+plotH+5; y++ {
			img.Set(x, y, black)
		}
		drawText(x-20, top+plotH+20, fmt.Sprintf("%.2fs", tt), black)
	}
	drawText(left+plotW/2-15, top+plotH+36, "time", black)
	// Y ticks: frequency.
	nyq := float64(res.Rate) / 2
	for i := 0; i <= 4; i++ {
		f := nyq * float64(i) / 4
		y := top + plotH - plotH*i/4
		for x := left - 5; x < left; x++ {
			img.Set(x, y, black)
		}
		var lab string
		if nyq >= 10000 {
			if f >= 1000 {
				lab = fmt.Sprintf("%.1fk", f/1000)
			} else {
				lab = fmt.Sprintf("%.0f", f)
			}
		} else {
			lab = fmt.Sprintf("%.0f", f)
		}
		drawText(8, y+4, lab, black)
	}
	drawText(8, top-16, nyqLabel(nyq), black)
	// Legend.
	legW, legH := 22, plotH
	legX := left + plotW + 18
	legY := top
	disp.LegendX, disp.LegendY, disp.LegendW, disp.LegendH = legX, legY, legW, legH
	for y := 0; y < legH; y++ {
		db := dbMax - (dbMax-dbMin)*(float64(y)+0.5)/float64(legH)
		c := DBToColor(db, dbMin, dbMax)
		for x := 0; x < legW; x++ {
			img.Set(legX+x, legY+y, c)
		}
	}
	for x := legX - 1; x <= legX+legW; x++ {
		img.Set(x, legY-1, black)
		img.Set(x, legY+legH, black)
	}
	for y := legY - 1; y <= legY+legH; y++ {
		img.Set(legX-1, y, black)
		img.Set(legX+legW, y, black)
	}
	for i := 0; i <= 4; i++ {
		db := dbMax - (dbMax-dbMin)*float64(i)/4
		y := legY + legH*i/4
		drawText(legX+legW+4, y+4, fmt.Sprintf("%.0f", db), black)
	}
	drawText(legX-2, legY+legH+36, "dBFS", black)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, nil, &spl.Diagnostic{Code: spl.CodeExportError, Message: "PNG encode failed: " + err.Error()}
	}
	return buf.Bytes(), disp, nil
}

func nyqLabel(nyq float64) string {
	if nyq >= 10000 {
		return "freq (Hz/kHz)"
	}
	return "freq (Hz)"
}

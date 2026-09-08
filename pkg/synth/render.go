package synth

import (
	"context"
	"fmt"
	"math"

	"spl/pkg/spl"
)

// Progress reports rendering progress at practical boundaries.
type Progress struct {
	Block     int   // completed blocks
	NumBlocks int   // total blocks
	Done      int64 // completed work units (samples processed across blocks)
	Total     int64 // total work units (NumSamples * NumBlocks, 0 if no blocks)
}

// Options configures rendering.
type Options struct {
	Limits   *spl.Limits
	Context  context.Context
	Progress func(Progress)
}

// Result is a successful render.
type Result struct {
	Samples   []float64
	Rate      int
	Peak      float64
	OverCount int
	Warnings  []spl.Diagnostic
}

func checkCtx(ctx context.Context) *spl.Diagnostic {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return &spl.Diagnostic{Code: spl.CodeRenderError, Message: fmt.Sprintf("render cancelled: %v", err)}
	}
	return nil
}

// Render renders validated SPL to mono float64 PCM.
// Returns (nil, errDiag) on failure with no partial audio.
func Render(doc *spl.Document, opts *Options) (*Result, *spl.Diagnostic) {
	if doc == nil {
		return nil, &spl.Diagnostic{Code: spl.CodeRenderError, Message: "nil document"}
	}
	var lim spl.Limits
	var ctx context.Context
	var prog func(Progress)
	if opts != nil {
		if opts.Limits != nil {
			lim = *opts.Limits
		} else {
			lim = spl.DefaultLimits()
		}
		ctx = opts.Context
		prog = opts.Progress
	} else {
		lim = spl.DefaultLimits()
	}
	if pd := spl.Preflight(doc, lim); pd != nil {
		return nil, pd
	}
	if d := checkCtx(ctx); d != nil {
		return nil, d
	}
	n := doc.NumSamples
	if n <= 0 || n > lim.MaxSamples {
		return nil, &spl.Diagnostic{Code: spl.CodeResourceLimit,
			Message: fmt.Sprintf("sample count %d exceeds limit %d", n, lim.MaxSamples)}
	}
	out := make([]float64, n)
	numBlocks := len(doc.Blocks)
	total := int64(n) * int64(numBlocks)
	var done int64
	emit := func(block int) {
		if prog != nil {
			prog(Progress{Block: block, NumBlocks: numBlocks, Done: done, Total: total})
		}
	}
	for bi, blk := range doc.Blocks {
		if d := checkCtx(ctx); d != nil {
			return nil, d
		}
		var d *spl.Diagnostic
		switch b := blk.(type) {
		case *spl.TrackBlock:
			d = renderTrack(b, doc, out, ctx, func(ad int64) {
				done += ad
				emit(bi)
			})
		case *spl.HarmonicsBlock:
			d = renderHarmonics(b, doc, out, ctx, func(ad int64) {
				done += ad
				emit(bi)
			})
		case *spl.NoiseBlock:
			d = renderNoise(b, doc, out, ctx, func(ad int64) {
				done += ad
				emit(bi)
			})
		case *spl.HitBlock:
			d = renderHit(b, doc, out, ctx, func(ad int64) {
				done += ad
				emit(bi)
			})
		default:
			d = &spl.Diagnostic{Code: spl.CodeRenderError, Message: "unknown block type"}
		}
		if d != nil {
			return nil, d
		}
		// Account remaining samples of this block for progress determinism.
		// Per-block renderers report active samples; pad to n for stable totals.
		// Instead compute: done should equal (bi+1)*n after each block.
		want := int64(bi+1) * int64(n)
		if done < want {
			done = want
		}
		emit(bi + 1)
		// Fast nonfinite check after each block.
		for i, v := range out {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, &spl.Diagnostic{Code: spl.CodeRenderError,
					Message: fmt.Sprintf("nonfinite sample at index %d after block %d (%s)", i, bi, blk.BlockType())}
			}
		}
	}
	peak := 0.0
	over := 0
	for _, v := range out {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, &spl.Diagnostic{Code: spl.CodeRenderError, Message: "nonfinite final sample"}
		}
		a := math.Abs(v)
		if a > peak {
			peak = a
		}
		if a > 1 {
			over++
		}
	}
	var warns []spl.Diagnostic
	if peak > 1 {
		warns = append(warns, spl.Diagnostic{Code: spl.CodePeakWarning,
			Message: fmt.Sprintf("peak magnitude %g exceeds 1.0 (%d samples over full scale); export preserves values without clipping", peak, over)})
	}
	return &Result{Samples: out, Rate: doc.Rate, Peak: peak, OverCount: over, Warnings: warns}, nil
}

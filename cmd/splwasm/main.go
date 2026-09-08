//go:build js && wasm

package main

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"syscall/js"
	"time"

	"spl/pkg/audio"
	"spl/pkg/spectrogram"
	"spl/pkg/spl"
	"spl/pkg/synth"
)

// Cached render for spectrogram-only requests (no audio rerender).
var (
	cachedPCM  []float64
	cachedRate int
	cachedSpec *spectrogram.Result
	cachedFFT  int
	cachedHop  int
	cachedDur  float64
)

type renderOpts struct {
	FFTLen   int     `json:"fftLen"`
	Hop      int     `json:"hop"`
	DBMin    float64 `json:"dbMin"`
	DBMax    float64 `json:"dbMax"`
	PlotW    int     `json:"plotW"`
	PlotH    int     `json:"plotH"`
	WantWAV  bool    `json:"wantWav"`
	WantPNG  bool    `json:"wantPng"`
	WantPCM  bool    `json:"wantPcm"`
	WantDisp bool    `json:"wantDisplay"`
}

func main() {
	js.Global().Set("splValidate", js.FuncOf(validateFn))
	js.Global().Set("splRender", js.FuncOf(renderFn))
	js.Global().Set("splSpectrogram", js.FuncOf(spectrogramFn))
	js.Global().Set("splVersion", js.FuncOf(versionFn))
	select {}
}

func versionFn(this js.Value, args []js.Value) any {
	return "spl-wasm 0.1.0"
}

func validateFn(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(`{"ok":false,"diagnostics":[{"code":"RENDER_ERROR","message":"missing source","line":0}]}`)
	}
	src := args[0].String()
	lim := spl.BrowserLimits()
	_, diags := spl.ParseWithLimits([]byte(src), lim)
	out := map[string]any{"ok": len(diags) == 0, "diagnostics": diags}
	if diags == nil {
		out["diagnostics"] = []spl.Diagnostic{}
	}
	b, _ := json.Marshal(out)
	return js.ValueOf(string(b))
}

func parseOpts(s string) renderOpts {
	o := renderOpts{FFTLen: 2048, Hop: 256, DBMin: -100, DBMax: 0, PlotW: 1000, PlotH: 500,
		WantWAV: true, WantPNG: true, WantPCM: true, WantDisp: true}
	if s == "" {
		return o
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return o
	}
	if v, ok := m["fftLen"].(float64); ok {
		o.FFTLen = int(v)
	}
	if v, ok := m["hop"].(float64); ok {
		o.Hop = int(v)
	}
	if v, ok := m["dbMin"].(float64); ok {
		o.DBMin = v
	}
	if v, ok := m["dbMax"].(float64); ok {
		o.DBMax = v
	}
	if v, ok := m["plotW"].(float64); ok {
		o.PlotW = int(v)
	}
	if v, ok := m["plotH"].(float64); ok {
		o.PlotH = int(v)
	}
	if v, ok := m["wantWav"].(bool); ok {
		o.WantWAV = v
	}
	if v, ok := m["wantPng"].(bool); ok {
		o.WantPNG = v
	}
	if v, ok := m["wantPcm"].(bool); ok {
		o.WantPCM = v
	}
	if v, ok := m["wantDisplay"].(bool); ok {
		o.WantDisp = v
	}
	return o
}

func uint8FromBytes(b []byte) js.Value {
	u8 := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(u8, b)
	return u8
}

func float32BytesLE(f []float32) []byte {
	b := make([]byte, len(f)*4)
	for i, v := range f {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return b
}

func renderFn(this js.Value, args []js.Value) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = errorObj("render panic: " + sprintf(r))
		}
	}()
	if len(args) < 1 {
		return errorObj("missing source")
	}
	src := args[0].String()
	optsStr := ""
	if len(args) >= 2 && args[1].Type() == js.TypeString {
		optsStr = args[1].String()
	}
	var progress js.Value
	if len(args) >= 3 && args[2].Type() == js.TypeFunction {
		progress = args[2]
	}
	opts := parseOpts(optsStr)
	lim := spl.BrowserLimits()
	start := time.Now()
	doc, diags := spl.ParseWithLimits([]byte(src), lim)
	if len(diags) > 0 {
		return diagErrorObj(diags)
	}
	var progFn func(synth.Progress)
	if progress.Truthy() {
		progFn = func(p synth.Progress) {
			func() {
				defer func() { _ = recover() }()
				progress.Invoke(p.Done, p.Total, p.Block, p.NumBlocks)
			}()
		}
	}
	res, ed := synth.Render(doc, &synth.Options{Limits: &lim, Progress: progFn})
	if ed != nil {
		return diagErrorObj([]spl.Diagnostic{*ed})
	}
	elapsed := time.Since(start)
	// Commit the complete result only after every requested export succeeds.
	var nextSpec *spectrogram.Result

	obj := js.Global().Get("Object").New()
	obj.Set("ok", true)
	stats, _ := json.Marshal(map[string]any{
		"duration": doc.Duration, "rate": doc.Rate, "samples": doc.NumSamples,
		"peak": res.Peak, "overCount": res.OverCount, "elapsedMs": elapsed.Milliseconds(),
	})
	obj.Set("stats", string(stats))
	warns, _ := json.Marshal(res.Warnings)
	if res.Warnings == nil {
		warns = []byte("[]")
	}
	obj.Set("warnings", string(warns))
	if opts.WantWAV || opts.WantPCM {
		// WAV bytes (shared exporter) and raw PCM32 bytes (same conversion).
		wavBytes, wd := audio.EncodeFloatWAV(res.Samples, res.Rate)
		if wd != nil {
			return diagErrorObj([]spl.Diagnostic{*wd})
		}
		if opts.WantWAV {
			obj.Set("wav", uint8FromBytes(wavBytes))
		}
		if opts.WantPCM {
			pcm32 := make([]float32, len(res.Samples))
			for i, v := range res.Samples {
				f := float32(v)
				if math.IsInf(float64(f), 0) {
					return errorObj("sample overflows float32")
				}
				pcm32[i] = f
			}
			obj.Set("pcm", uint8FromBytes(float32BytesLE(pcm32)))
		}
	}
	if opts.WantPNG || opts.WantDisp {
		specRes, sd := spectrogram.ComputeSpectrogram(res.Samples, res.Rate,
			&spectrogram.Options{FFTLen: opts.FFTLen, Hop: opts.Hop}, lim)
		if sd != nil {
			return diagErrorObj([]spl.Diagnostic{*sd})
		}
		nextSpec = specRes
		pngBytes, disp, pd := spectrogram.EncodeSpectrogramPNG(specRes,
			&spectrogram.PNGOptions{PlotWidth: opts.PlotW, PlotHeight: opts.PlotH, DBMin: opts.DBMin, DBMax: opts.DBMax})
		if pd != nil {
			return diagErrorObj([]spl.Diagnostic{*pd})
		}
		if opts.WantPNG {
			obj.Set("png", uint8FromBytes(pngBytes))
		}
		if opts.WantDisp {
			obj.Set("display", uint8FromBytes(float32BytesLE(disp.Data)))
			meta, _ := json.Marshal(map[string]any{
				"plotW": disp.PlotW, "plotH": disp.PlotH,
				"originX": disp.OriginX, "originY": disp.OriginY,
				"totalW": disp.TotalW, "totalH": disp.TotalH,
				"duration": disp.Duration, "nyquist": disp.Nyquist,
				"dbMin": disp.DBMin, "dbMax": disp.DBMax,
				"fftLen": disp.FFTLen, "hop": disp.Hop, "rate": disp.Rate,
				"frames": specRes.Frames, "bins": specRes.Bins,
			})
			obj.Set("displayMeta", string(meta))
		}
	}
	cachedPCM, cachedRate, cachedDur = res.Samples, res.Rate, doc.Duration
	cachedSpec = nextSpec
	cachedFFT, cachedHop = opts.FFTLen, opts.Hop
	return obj
}

// splSpectrogram uses cached PCM: recomputes STFT only when FFT/hop changed;
// display-range-only changes re-encode PNG from cached matrix.
func spectrogramFn(this js.Value, args []js.Value) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = errorObj("spectrogram panic: " + sprintf(r))
		}
	}()
	if cachedPCM == nil {
		return errorObj("no cached render; render audio first")
	}
	optsStr := ""
	if len(args) >= 1 && args[0].Type() == js.TypeString {
		optsStr = args[0].String()
	}
	opts := parseOpts(optsStr)
	lim := spl.BrowserLimits()
	var specRes *spectrogram.Result
	if cachedSpec != nil && cachedFFT == opts.FFTLen && cachedHop == opts.Hop {
		specRes = cachedSpec
	} else {
		var sd *spl.Diagnostic
		specRes, sd = spectrogram.ComputeSpectrogram(cachedPCM, cachedRate,
			&spectrogram.Options{FFTLen: opts.FFTLen, Hop: opts.Hop}, lim)
		if sd != nil {
			return diagErrorObj([]spl.Diagnostic{*sd})
		}
		cachedSpec = specRes
		cachedFFT, cachedHop = opts.FFTLen, opts.Hop
	}
	pngBytes, disp, pd := spectrogram.EncodeSpectrogramPNG(specRes,
		&spectrogram.PNGOptions{PlotWidth: opts.PlotW, PlotHeight: opts.PlotH, DBMin: opts.DBMin, DBMax: opts.DBMax})
	if pd != nil {
		return diagErrorObj([]spl.Diagnostic{*pd})
	}
	obj := js.Global().Get("Object").New()
	obj.Set("ok", true)
	obj.Set("png", uint8FromBytes(pngBytes))
	obj.Set("display", uint8FromBytes(float32BytesLE(disp.Data)))
	meta, _ := json.Marshal(map[string]any{
		"plotW": disp.PlotW, "plotH": disp.PlotH,
		"originX": disp.OriginX, "originY": disp.OriginY,
		"totalW": disp.TotalW, "totalH": disp.TotalH,
		"duration": disp.Duration, "nyquist": disp.Nyquist,
		"dbMin": disp.DBMin, "dbMax": disp.DBMax,
		"fftLen": disp.FFTLen, "hop": disp.Hop, "rate": disp.Rate,
		"frames": specRes.Frames, "bins": specRes.Bins,
	})
	obj.Set("displayMeta", string(meta))
	return obj
}

func errorObj(msg string) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("ok", false)
	d, _ := json.Marshal([]spl.Diagnostic{{Code: spl.CodeRenderError, Message: msg}})
	obj.Set("diagnostics", string(d))
	return obj
}

func diagErrorObj(diags []spl.Diagnostic) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("ok", false)
	b, _ := json.Marshal(diags)
	obj.Set("diagnostics", string(b))
	return obj
}

func sprintf(v any) string {
	b, _ := json.Marshal(map[string]any{"v": v})
	_ = b
	if s, ok := v.(string); ok {
		return s
	}
	return "internal error"
}

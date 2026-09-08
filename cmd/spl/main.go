package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"spl/pkg/audio"
	"spl/pkg/spectrogram"
	"spl/pkg/spl"
	"spl/pkg/synth"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage())
		return 2
	}
	switch args[0] {
	case "--help", "-h", "help":
		fmt.Fprint(os.Stderr, usage())
		return 0
	case "--version", "version":
		fmt.Fprintf(os.Stderr, "spl %s (Go %s)\n", version, goVersion())
		return 0
	case "validate":
		return cmdValidate(args[1:])
	case "render":
		return cmdRender(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n%s", args[0], usage())
		return 2
	}
}

func goVersion() string {
	// Avoid runtime dependency version skew; report compile-time version via helper.
	return runtimeVersion()
}

func usage() string {
	return `spl - SPL audio compiler

Usage:
  spl validate [--json] <file.spl>
  spl render [--json] [--force] --wav <out.wav> [--spectrogram <out.png>] [spec opts] <file.spl>
  spl render [--json] [--force] --spectrogram <out.png> [spec opts] <file.spl>
  spl --help | --version

Spectrogram options:
  --fft <len>        FFT length, power of two (default 2048)
  --hop <n>          hop in samples (default 256)
  --db-min <float>   display minimum dBFS (default -100)
  --db-max <float>   display maximum dBFS (default 0)
  --plot-width <n>   PNG plot width (default 1000)
  --plot-height <n>  PNG plot height (default 500)

Other:
  --json   structured JSON diagnostics on stdout
  --force  allow overwriting existing output files
`
}

func cmdValidate(args []string) int {
	var jsonMode bool
	var files []string
	for _, a := range args {
		switch a {
		case "--json":
			jsonMode = true
		case "--help", "-h":
			fmt.Fprint(os.Stderr, "Usage: spl validate [--json] <file.spl>\n")
			return 0
		default:
			files = append(files, a)
		}
	}
	if len(files) != 1 {
		fmt.Fprint(os.Stderr, "validate requires exactly one input file\n")
		return 2
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		emitDiag(nil, []spl.Diagnostic{{Code: spl.CodeRenderError, Message: "read failed: " + err.Error()}}, jsonMode)
		return 1
	}
	_, diags := spl.Parse(data)
	if len(diags) > 0 {
		emitDiag(nil, diags, jsonMode)
		return 1
	}
	if jsonMode {
		fmt.Fprintln(os.Stdout, `{"ok":true,"diagnostics":[]}`)
	} else {
		fmt.Fprintln(os.Stderr, "valid")
	}
	return 0
}

type renderFlags struct {
	jsonMode bool
	force    bool
	wav      string
	spec     string
	fft      int
	hop      int
	dbMin    float64
	dbMax    float64
	dbSet    bool
	plotW    int
	plotH    int
}

func cmdRender(args []string) int {
	var fl renderFlags
	fl.dbMin, fl.dbMax = -100, 0
	fl.fft, fl.hop = 2048, 256
	fl.plotW, fl.plotH = 1000, 500
	var files []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json":
			fl.jsonMode = true
		case "--force":
			fl.force = true
		case "--wav":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--wav requires a path")
				return 2
			}
			i++
			fl.wav = args[i]
		case "--spectrogram":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--spectrogram requires a path")
				return 2
			}
			i++
			fl.spec = args[i]
		case "--fft":
			i++
			v, ok := needInt(args, i, "--fft")
			if !ok {
				return 2
			}
			fl.fft = v
		case "--hop":
			i++
			v, ok := needInt(args, i, "--hop")
			if !ok {
				return 2
			}
			fl.hop = v
		case "--db-min":
			i++
			v, ok := needFloat(args, i, "--db-min")
			if !ok {
				return 2
			}
			fl.dbMin = v
			fl.dbSet = true
		case "--db-max":
			i++
			v, ok := needFloat(args, i, "--db-max")
			if !ok {
				return 2
			}
			fl.dbMax = v
			fl.dbSet = true
		case "--plot-width":
			i++
			v, ok := needInt(args, i, "--plot-width")
			if !ok {
				return 2
			}
			fl.plotW = v
		case "--plot-height":
			i++
			v, ok := needInt(args, i, "--plot-height")
			if !ok {
				return 2
			}
			fl.plotH = v
		case "--help", "-h":
			fmt.Fprint(os.Stderr, usage())
			return 0
		default:
			if len(a) > 0 && a[0] == '-' {
				fmt.Fprintf(os.Stderr, "unknown flag %q\n", a)
				return 2
			}
			files = append(files, a)
		}
	}
	if len(files) != 1 {
		fmt.Fprintln(os.Stderr, "render requires exactly one input file")
		return 2
	}
	if fl.wav == "" && fl.spec == "" {
		fmt.Fprintln(os.Stderr, "render requires at least one of --wav or --spectrogram")
		return 2
	}
	if !fl.dbSet {
		fl.dbMin, fl.dbMax = -100, 0
	}
	start := time.Now()
	data, err := os.ReadFile(files[0])
	if err != nil {
		emitDiag(nil, []spl.Diagnostic{{Code: spl.CodeRenderError, Message: "read failed: " + err.Error()}}, fl.jsonMode)
		return 1
	}
	lim := spl.DefaultLimits()
	doc, diags := spl.ParseWithLimits(data, lim)
	if len(diags) > 0 {
		emitDiag(nil, diags, fl.jsonMode)
		return 1
	}
	res, ed := synth.Render(doc, &synth.Options{Limits: &lim})
	if ed != nil {
		emitDiag(nil, []spl.Diagnostic{*ed}, fl.jsonMode)
		return 1
	}
	elapsed := time.Since(start)
	// Exports via temp files.
	if fl.wav != "" {
		wavBytes, wd := audio.EncodeFloatWAV(res.Samples, res.Rate)
		if wd != nil {
			emitDiag(nil, []spl.Diagnostic{*wd}, fl.jsonMode)
			return 1
		}
		if err := writeAtomic(fl.wav, wavBytes, fl.force); err != nil {
			emitDiag(nil, []spl.Diagnostic{{Code: spl.CodeExportError, Message: err.Error()}}, fl.jsonMode)
			return 1
		}
	}
	if fl.spec != "" {
		specRes, sd := spectrogram.ComputeSpectrogram(res.Samples, res.Rate,
			&spectrogram.Options{FFTLen: fl.fft, Hop: fl.hop}, lim)
		if sd != nil {
			emitDiag(nil, []spl.Diagnostic{*sd}, fl.jsonMode)
			return 1
		}
		pngBytes, _, pd := spectrogram.EncodeSpectrogramPNG(specRes,
			&spectrogram.PNGOptions{PlotWidth: fl.plotW, PlotHeight: fl.plotH, DBMin: fl.dbMin, DBMax: fl.dbMax})
		if pd != nil {
			emitDiag(nil, []spl.Diagnostic{*pd}, fl.jsonMode)
			return 1
		}
		if err := writeAtomic(fl.spec, pngBytes, fl.force); err != nil {
			emitDiag(nil, []spl.Diagnostic{{Code: spl.CodeExportError, Message: err.Error()}}, fl.jsonMode)
			return 1
		}
	}
	stats := map[string]any{
		"duration":  doc.Duration,
		"rate":      doc.Rate,
		"samples":   doc.NumSamples,
		"peak":      res.Peak,
		"overCount": res.OverCount,
		"elapsedMs": elapsed.Milliseconds(),
	}
	emitDiag(stats, res.Warnings, fl.jsonMode)
	return 0
}

func needInt(args []string, i int, name string) (int, bool) {
	if i >= len(args) {
		fmt.Fprintf(os.Stderr, "%s requires a value\n", name)
		return 0, false
	}
	v, err := strconv.Atoi(args[i])
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s must be an integer: %v\n", name, err)
		return 0, false
	}
	return v, true
}

func needFloat(args []string, i int, name string) (float64, bool) {
	if i >= len(args) {
		fmt.Fprintf(os.Stderr, "%s requires a value\n", name)
		return 0, false
	}
	v, err := strconv.ParseFloat(args[i], 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s must be a number: %v\n", name, err)
		return 0, false
	}
	return v, true
}

func emitDiag(stats map[string]any, diags []spl.Diagnostic, jsonMode bool) {
	if jsonMode {
		obj := map[string]any{"ok": len(diags) == 0 || isOnlyWarnings(diags), "diagnostics": diags}
		// Warnings have PEAK_WARNING; ok should be true when only warnings.
		hasError := false
		for _, d := range diags {
			if d.Code != spl.CodePeakWarning {
				hasError = true
			}
		}
		obj["ok"] = !hasError
		if stats != nil {
			obj["stats"] = stats
		}
		b, _ := json.Marshal(obj)
		fmt.Fprintln(os.Stdout, string(b))
		return
	}
	for _, d := range diags {
		if d.Line > 0 {
			fmt.Fprintf(os.Stderr, "line %d: %s: %s\n", d.Line, d.Code, d.Message)
		} else {
			fmt.Fprintf(os.Stderr, "%s: %s\n", d.Code, d.Message)
		}
	}
	if stats != nil {
		fmt.Fprintf(os.Stderr, "duration=%.6g rate=%v samples=%v peak=%.6g over=%v elapsedMs=%v\n",
			stats["duration"], stats["rate"], stats["samples"], stats["peak"], stats["overCount"], stats["elapsedMs"])
	}
}

func isOnlyWarnings(diags []spl.Diagnostic) bool {
	for _, d := range diags {
		if d.Code != spl.CodePeakWarning {
			return false
		}
	}
	return true
}

func writeAtomic(path string, data []byte, force bool) error {
	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("output %s exists; pass --force to overwrite", path)
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, ".spl-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

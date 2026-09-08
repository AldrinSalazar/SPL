package integration

import (
	"bytes"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"spl/pkg/audio"
	"spl/pkg/spectrogram"
	"spl/pkg/spl"
	"spl/pkg/synth"
)

var examples = []string{
	"minimal-track.spl",
	"three-notes.spl",
	"resonances.spl",
	"metallic-impact.spl",
	"two-noises.spl",
}

func TestAllExamplesRender(t *testing.T) {
	for _, ex := range examples {
		path := filepath.Join("..", "examples", ex)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", ex, err)
		}
		doc, diags := spl.Parse(data)
		if len(diags) > 0 {
			t.Fatalf("%s diags: %+v", ex, diags)
		}
		res, ed := synth.Render(doc, nil)
		if ed != nil {
			t.Fatalf("%s render: %+v", ex, ed)
		}
		if len(res.Samples) != doc.NumSamples {
			t.Fatalf("%s samples %d want %d", ex, len(res.Samples), doc.NumSamples)
		}
		// Determinism: second render bit-identical.
		res2, _ := synth.Render(doc, nil)
		for i := range res.Samples {
			if res.Samples[i] != res2.Samples[i] {
				t.Fatalf("%s nondeterministic at %d", ex, i)
			}
		}
		// WAV roundtrip.
		wav, wd := audio.EncodeFloatWAV(res.Samples, res.Rate)
		if wd != nil {
			t.Fatalf("%s wav: %+v", ex, wd)
		}
		dec, err := audio.DecodeFloatWAV(wav)
		if err != nil {
			t.Fatalf("%s decode: %v", ex, err)
		}
		if dec.Rate != res.Rate || len(dec.Samples) != len(res.Samples) {
			t.Fatalf("%s wav mismatch", ex)
		}
		// Spectrogram + PNG.
		spec, sd := spectrogram.ComputeSpectrogram(res.Samples, res.Rate, nil, spl.DefaultLimits())
		if sd != nil {
			t.Fatalf("%s spec: %+v", ex, sd)
		}
		pngBytes, disp, pd := spectrogram.EncodeSpectrogramPNG(spec, nil)
		if pd != nil {
			t.Fatalf("%s png: %+v", ex, pd)
		}
		img, err := png.Decode(bytes.NewReader(pngBytes))
		if err != nil {
			t.Fatalf("%s png decode: %v", ex, err)
		}
		if img.Bounds().Dx() != disp.TotalW || img.Bounds().Dy() != disp.TotalH {
			t.Fatalf("%s png dims", ex)
		}
	}
}

func TestResourceLimitTinyPitch(t *testing.T) {
	// Tiny pitch implies huge harmonic count.
	src := "spl 2 24000 1 0\nharmonics\nspectrum\n0 1\n10000 1\ncurve\n0 0.001 0.5\n1 0.001 0.5\nend\n"
	_, diags := spl.Parse([]byte(src))
	found := false
	for _, d := range diags {
		if d.Code == spl.CodeResourceLimit {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected RESOURCE_LIMIT, got %+v", diags)
	}
}

func TestCLIEndToEnd(t *testing.T) {
	tmp := t.TempDir()
	cli := filepath.Join(tmp, "spl-test-cli.exe")
	build := exec.Command("go", "build", "-o", cli, "../cmd/spl")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, out)
	}
	// validate
	cmd := exec.Command(cli, "validate", filepath.Join("..", "examples", "metallic-impact.spl"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
	// render wav+png
	wavPath := filepath.Join(tmp, "out.wav")
	pngPath := filepath.Join(tmp, "out.png")
	cmd = exec.Command(cli, "render", filepath.Join("..", "examples", "metallic-impact.spl"),
		"--wav", wavPath, "--spectrogram", pngPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("render: %v\n%s", err, out)
	}
	wavData, _ := os.ReadFile(wavPath)
	dec, err := audio.DecodeFloatWAV(wavData)
	if err != nil {
		t.Fatalf("wav decode: %v", err)
	}
	if dec.Rate != 24000 || len(dec.Samples) != 48000 {
		t.Fatalf("wav stats %+v len %d", dec.Rate, len(dec.Samples))
	}
	pngData, _ := os.ReadFile(pngPath)
	if _, err := png.Decode(bytes.NewReader(pngData)); err != nil {
		t.Fatalf("png decode: %v", err)
	}
	// Overwrite without --force must fail and preserve original.
	cmd = exec.Command(cli, "render", filepath.Join("..", "examples", "minimal-track.spl"), "--wav", wavPath)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected overwrite error")
	}
	wavData2, _ := os.ReadFile(wavPath)
	if !bytes.Equal(wavData, wavData2) {
		t.Fatal("existing file was modified without --force")
	}
	// Invalid input leaves no partial artifact.
	badPath := filepath.Join(tmp, "bad.spl")
	os.WriteFile(badPath, []byte("spl 2 24000 1 0\ntrack\n0 440 0\n"), 0o644)
	outWav := filepath.Join(tmp, "should-not-exist.wav")
	cmd = exec.Command(cli, "render", badPath, "--wav", outWav)
	if err := cmd.Run(); err == nil {
		t.Fatal("expected render error")
	}
	if _, err := os.Stat(outWav); !os.IsNotExist(err) {
		t.Fatal("partial artifact left after error")
	}
}

func TestOverFullScalePreserved(t *testing.T) {
	// Two simultaneous loud tracks exceed 1.
	src := "spl 2 24000 0.1 0\ntrack\n0 440 0.8\n0.1 440 0.8\nend\ntrack\n0 440 0.8\n0.1 440 0.8\nend\n"
	doc, _ := spl.Parse([]byte(src))
	res, _ := synth.Render(doc, nil)
	if !(res.Peak > 1) {
		t.Fatalf("expected peak>1, got %g", res.Peak)
	}
	wav, _ := audio.EncodeFloatWAV(res.Samples, res.Rate)
	dec, _ := audio.DecodeFloatWAV(wav)
	peak := 0.0
	for _, v := range dec.Samples {
		if math.Abs(float64(v)) > peak {
			peak = math.Abs(float64(v))
		}
	}
	if !(peak > 1) {
		t.Fatalf("WAV peak %g should exceed 1", peak)
	}
}

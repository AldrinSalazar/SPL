package synth

import (
	"os"
	"testing"

	"spl/pkg/spl"
)

func loadDoc(b *testing.B, path string) *spl.Document {
	b.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	doc, diags := spl.Parse(data)
	if len(diags) > 0 {
		b.Fatalf("diags %+v", diags)
	}
	return doc
}

func BenchmarkTonalThreeNotes(b *testing.B) {
	doc := loadDoc(b, "../../examples/three-notes.spl")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ed := Render(doc, nil); ed != nil {
			b.Fatal(ed)
		}
	}
}

func BenchmarkNoiseTwoNoises(b *testing.B) {
	doc := loadDoc(b, "../../examples/two-noises.spl")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ed := Render(doc, nil); ed != nil {
			b.Fatal(ed)
		}
	}
}

func BenchmarkMetallicImpact(b *testing.B) {
	doc := loadDoc(b, "../../examples/metallic-impact.spl")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ed := Render(doc, nil); ed != nil {
			b.Fatal(ed)
		}
	}
}

package spl

import "testing"

// FuzzParse ensures malformed input never panics and allocations stay bounded
// via input/token limits. Run briefly: go test -fuzz=FuzzParse -fuzztime=20s ./pkg/spl/
func FuzzParse(f *testing.F) {
	seeds := []string{
		"spl 2 24000 1 0\n",
		"spl 2 24000 1 0\ntrack\n0 440 0\n1 440 0\nend\n",
		"```\n",
		"# comment\n",
		"spl 2 8000 0.001 0\nhit\n0 0.001 0 100 0.1 0\nend\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := DefaultLimits()
		// Bound fuzz input to limit to avoid OOM in fuzzer itself.
		if len(data) > lim.MaxInputBytes {
			data = data[:lim.MaxInputBytes]
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			ParseWithLimits(data, lim)
		}()
	})
}

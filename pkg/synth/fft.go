package synth

import "math"

// FFT performs an in-place radix-2 FFT on a (length must be power of two).
// If invert is false, computes forward DFT X[k]=sum x[j]*exp(-2i pi k j/n).
// If invert is true, computes inverse DFT x[j]=(1/n) sum X[k]*exp(+2i pi k j/n).
func FFT(a []complex128, invert bool) {
	n := len(a)
	// Bit-reversal permutation.
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := 2 * math.Pi / float64(length)
		if !invert {
			ang = -ang
		}
		wlen := complex(math.Cos(ang), math.Sin(ang))
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for k := 0; k < length/2; k++ {
				u := a[i+k]
				v := a[i+k+length/2] * w
				a[i+k] = u + v
				a[i+k+length/2] = u - v
				w *= wlen
			}
		}
	}
	if invert {
		inv := complex(1/float64(n), 0)
		for i := range a {
			a[i] *= inv
		}
	}
}

// IsPow2 reports whether n is a power of two (n>0).
func IsPow2(n int) bool { return n > 0 && n&(n-1) == 0 }

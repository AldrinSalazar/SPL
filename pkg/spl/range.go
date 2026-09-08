package spl

import "math"

// LowerBound returns the smallest sample index m with m/rate >= t,
// clamped to [0,n]. Comparison uses float64 division to match the
// half-open active-interval semantics (t = n/RATE).
func LowerBound(t float64, rate, n int) int {
	if math.IsNaN(t) {
		return n
	}
	est := int(math.Ceil(t * float64(rate)))
	for est > 0 && float64(est-1)/float64(rate) >= t {
		est--
	}
	for est <= n && float64(est)/float64(rate) < t {
		est++
	}
	if est < 0 {
		est = 0
	}
	if est > n {
		est = n
	}
	return est
}

// ActiveRange returns [ns,ne) sample indices active for [t0,t1).
func ActiveRange(t0, t1 float64, rate, n int) (int, int) {
	ns := LowerBound(t0, rate, n)
	ne := LowerBound(t1, rate, n)
	if ns < 0 {
		ns = 0
	}
	if ne > n {
		ne = n
	}
	if ns > n {
		ns = n
	}
	if ne < 0 {
		ne = 0
	}
	return ns, ne
}

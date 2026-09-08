package synth

func mix(x uint32) uint32 {
	x = x + 0x9e3779b9
	x = (x ^ (x >> 16)) * 0x85ebca6b
	x = (x ^ (x >> 13)) * 0xc2b2ae35
	return x ^ (x >> 16)
}

// NoisePhase implements the specified hash with wrapping uint32 arithmetic.
// m may be negative; it is reduced modulo 2^32.
func NoisePhase(seed uint32, blockIndex int, m int, k int) float64 {
	mU := uint32(int32(m))
	x := mix(seed ^ mix(uint32(blockIndex)))
	x = mix(x ^ mix(mU))
	x = mix(x ^ mix(uint32(k)))
	return float64(x) / 4294967296.0
}

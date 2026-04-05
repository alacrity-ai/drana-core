package types

import "math/bits"

// MulDiv computes (a * b) / c without intermediate overflow.
// Uses 128-bit intermediate product via math/bits.Mul64.
// Panics if c is zero.
func MulDiv(a, b, c uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	if hi == 0 {
		return lo / c
	}
	// 128-bit division: (hi:lo) / c.
	// If hi >= c, result overflows uint64 — clamp to max.
	if hi >= c {
		return ^uint64(0)
	}
	q, _ := bits.Div64(hi, lo, c)
	return q
}

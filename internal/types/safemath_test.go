package types

import (
	"math"
	"testing"
)

func TestMulDivBasic(t *testing.T) {
	// 3000 DRANA boost, 3% burn
	got := MulDiv(3_000_000_000, 3, 100)
	if got != 90_000_000 {
		t.Fatalf("MulDiv(3B, 3, 100) = %d, want 90000000", got)
	}
}

func TestMulDivIdentity(t *testing.T) {
	got := MulDiv(math.MaxUint64, 1, 1)
	if got != math.MaxUint64 {
		t.Fatalf("MulDiv(max, 1, 1) = %d, want maxUint64", got)
	}
}

func TestMulDivLargeProduct(t *testing.T) {
	// 1e18 * 1e18 / 1e18 = 1e18. Intermediate product is 1e36 which overflows uint64.
	got := MulDiv(1e18, 1e18, 1e18)
	if got != 1e18 {
		t.Fatalf("MulDiv(1e18, 1e18, 1e18) = %d, want 1000000000000000000", got)
	}
}

func TestMulDivZeroAmount(t *testing.T) {
	got := MulDiv(0, 3, 100)
	if got != 0 {
		t.Fatalf("MulDiv(0, 3, 100) = %d, want 0", got)
	}
}

func TestMulDivProRata(t *testing.T) {
	// Pro-rata: 10M reward * 500M stake / 1B total = 5M
	got := MulDiv(10_000_000, 500_000_000, 1_000_000_000)
	if got != 5_000_000 {
		t.Fatalf("MulDiv(10M, 500M, 1B) = %d, want 5000000", got)
	}
}

func TestMulDivLargeProRata(t *testing.T) {
	// Large pro-rata: 100B reward * 5T stake / 10T total = 50B
	// Intermediate: 100B * 5T = 5e23 which overflows uint64 (max 1.8e19).
	got := MulDiv(100_000_000_000, 5_000_000_000_000, 10_000_000_000_000)
	if got != 50_000_000_000 {
		t.Fatalf("MulDiv(100B, 5T, 10T) = %d, want 50000000000", got)
	}
}

func TestMulDivOverflowClamp(t *testing.T) {
	// Result itself overflows: maxUint64 * 2 / 1 → clamped to maxUint64
	got := MulDiv(math.MaxUint64, 2, 1)
	if got != math.MaxUint64 {
		t.Fatalf("MulDiv(max, 2, 1) = %d, want maxUint64", got)
	}
}

func TestMulDivQuorum(t *testing.T) {
	// Quorum: total * 2 / 3. With total = 10B.
	got := MulDiv(10_000_000_000, 2, 3)
	if got != 6_666_666_666 {
		t.Fatalf("MulDiv(10B, 2, 3) = %d, want 6666666666", got)
	}
}
